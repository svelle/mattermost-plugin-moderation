package main

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/svelle/mattermost-plugin-moderation/server/store/kvstore"
)

const (
	// indexClusterEventID tells other nodes about a new ban or restriction.
	indexClusterEventID = "moderation_index_add"

	// indexRebuildInterval is how often each node reloads its index from the
	// KV store, dropping lifted bans and restrictions and picking up any
	// cluster event it missed.
	indexRebuildInterval = 10 * time.Minute

	indexPageSize = 1000
)

// moderationIndex is this node's in-memory record of which members are
// banned and which members are restricted in which channel. The post and
// reaction hooks run for every message on the server, so they check the
// index first and only read the KV store when it says a record may exist.
//
// The index can still list a member whose ban or restriction was lifted —
// the KV read that follows sorts that out — but it must never miss an active
// one. Until it has loaded, it reports that anyone may be restricted, so
// callers fall back to reading the KV store.
type moderationIndex struct {
	mu         sync.RWMutex
	ready      bool
	banned     map[string]bool
	restricted map[string]map[string]bool // channel ID -> user IDs

	// While a rebuild runs, additions are also kept in pending so the
	// rebuilt index doesn't drop them.
	rebuilding bool
	pending    []indexEntry

	rebuildMu sync.Mutex
}

// indexEntry is a ban (no channel) or a restriction in a channel.
type indexEntry struct {
	ChannelID string `json:"channel_id,omitempty"`
	UserID    string `json:"user_id"`
}

func (idx *moderationIndex) mayBeBanned(userID string) bool {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return !idx.ready || idx.banned[userID]
}

func (idx *moderationIndex) mayBeRestricted(channelID, userID string) bool {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return !idx.ready || idx.restricted[channelID][userID]
}

func (idx *moderationIndex) add(e indexEntry) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.rebuilding {
		idx.pending = append(idx.pending, e)
	}
	if idx.banned == nil {
		idx.banned = map[string]bool{}
		idx.restricted = map[string]map[string]bool{}
	}
	addEntry(idx.banned, idx.restricted, e)
}

func addEntry(banned map[string]bool, restricted map[string]map[string]bool, e indexEntry) {
	if e.ChannelID == "" {
		banned[e.UserID] = true
		return
	}
	if restricted[e.ChannelID] == nil {
		restricted[e.ChannelID] = map[string]bool{}
	}
	restricted[e.ChannelID][e.UserID] = true
}

func (idx *moderationIndex) entries() []indexEntry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	var entries []indexEntry
	for userID := range idx.banned {
		entries = append(entries, indexEntry{UserID: userID})
	}
	for channelID, users := range idx.restricted {
		for userID := range users {
			entries = append(entries, indexEntry{ChannelID: channelID, UserID: userID})
		}
	}
	return entries
}

func (idx *moderationIndex) beginRebuild() {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.rebuilding = true
	idx.pending = nil
}

func (idx *moderationIndex) endRebuild(banned map[string]bool, restricted map[string]map[string]bool, ok bool) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if ok {
		for _, e := range idx.pending {
			addEntry(banned, restricted, e)
		}
		idx.banned = banned
		idx.restricted = restricted
		idx.ready = true
	}
	idx.rebuilding = false
	idx.pending = nil
}

// rebuildIndex reloads the index from the KV store.
func (p *Plugin) rebuildIndex() {
	idx := &p.index
	idx.rebuildMu.Lock()
	defer idx.rebuildMu.Unlock()

	idx.beginRebuild()
	banned := map[string]bool{}
	restricted := map[string]map[string]bool{}
	for page := 0; ; page++ {
		keys, err := p.kvstore.ListKeys(page, indexPageSize)
		if err != nil {
			p.API.LogError("Failed to load moderation index", "error", err.Error())
			idx.endRebuild(nil, nil, false)
			return
		}
		for _, key := range keys {
			if userID, ok := kvstore.ParseBanKey(key); ok {
				addEntry(banned, restricted, indexEntry{UserID: userID})
			} else if channelID, userID, ok := kvstore.ParseRestrictionKey(key); ok {
				addEntry(banned, restricted, indexEntry{ChannelID: channelID, UserID: userID})
			}
		}
		if len(keys) < indexPageSize {
			break
		}
	}

	// Keys deleted while paging shift later keys onto pages already read,
	// so recheck anything the old index had that the scan didn't find.
	for _, e := range idx.entries() {
		if e.ChannelID == "" && !banned[e.UserID] {
			if ban, err := p.kvstore.GetBan(e.UserID); err != nil || ban != nil {
				addEntry(banned, restricted, e)
			}
		} else if e.ChannelID != "" && !restricted[e.ChannelID][e.UserID] {
			if restriction, err := p.kvstore.GetRestriction(e.ChannelID, e.UserID); err != nil || restriction != nil {
				addEntry(banned, restricted, e)
			}
		}
	}
	idx.endRebuild(banned, restricted, true)
}

// maintainIndex loads the index and keeps reloading it until stop closes.
func (p *Plugin) maintainIndex(stop <-chan struct{}) {
	p.rebuildIndex()
	ticker := time.NewTicker(indexRebuildInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			p.rebuildIndex()
		}
	}
}

// indexBan and indexRestriction record a new ban or restriction on this
// node and every other node. Call them before writing the KV record.
func (p *Plugin) indexBan(userID string) {
	p.addToIndex(indexEntry{UserID: userID})
}

func (p *Plugin) indexRestriction(channelID, userID string) {
	p.addToIndex(indexEntry{ChannelID: channelID, UserID: userID})
}

func (p *Plugin) addToIndex(e indexEntry) {
	p.index.add(e)
	data, err := json.Marshal(e)
	if err != nil {
		return
	}
	if err := p.API.PublishPluginClusterEvent(
		model.PluginClusterEvent{Id: indexClusterEventID, Data: data},
		model.PluginClusterEventSendOptions{SendType: model.PluginClusterEventSendTypeReliable},
	); err != nil {
		p.API.LogError("Failed to share moderation index update", "error", err.Error())
	}
}

// OnPluginClusterEvent applies index updates from other nodes.
func (p *Plugin) OnPluginClusterEvent(_ *plugin.Context, ev model.PluginClusterEvent) {
	if ev.Id != indexClusterEventID {
		return
	}
	var e indexEntry
	if err := json.Unmarshal(ev.Data, &e); err != nil || e.UserID == "" {
		return
	}
	p.index.add(e)
}
