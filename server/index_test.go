package main

import (
	"encoding/json"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"

	"github.com/svelle/mattermost-plugin-moderation/server/store/kvstore"
)

func TestModerationIndexFallsBackUntilLoaded(t *testing.T) {
	var idx moderationIndex
	assert.True(t, idx.mayBeBanned("anyone"))
	assert.True(t, idx.mayBeRestricted("channel", "anyone"))
}

func TestRebuildIndex(t *testing.T) {
	p, _, store := newTestPlugin(t, map[string]bool{})
	store.bans["banned"] = &kvstore.Ban{UserID: "banned"}
	store.restrictions[kvstore.RestrictionKey(testChannelID, "muted")] = &kvstore.Restriction{Muted: true}

	p.rebuildIndex()

	assert.True(t, p.index.mayBeBanned("banned"))
	assert.False(t, p.index.mayBeBanned("muted"))
	assert.True(t, p.index.mayBeRestricted(testChannelID, "muted"))
	assert.False(t, p.index.mayBeRestricted(testChannelID, "banned"))
	assert.False(t, p.index.mayBeRestricted("other-channel", "muted"))
	assert.Empty(t, p.postingBlockedReason(testChannelID, "regular"))
	assert.NotEmpty(t, p.postingBlockedReason(testChannelID, "muted"))
}

func TestRebuildIndexDropsLiftedAndKeepsMissedEntries(t *testing.T) {
	p, _, store := newTestPlugin(t, map[string]bool{})
	p.rebuildIndex()
	p.indexRestriction(testChannelID, "lifted")
	p.indexBan("missed")
	// Present in the KV store but not returned by the scan, as when a
	// concurrent delete shifts it onto a page that was already read.
	p.kvstore = &hiddenBanStore{fakeKVStore: store}
	store.bans["missed"] = &kvstore.Ban{UserID: "missed"}

	p.rebuildIndex()

	assert.False(t, p.index.mayBeRestricted(testChannelID, "lifted"))
	assert.True(t, p.index.mayBeBanned("missed"))
}

type hiddenBanStore struct{ *fakeKVStore }

func (hiddenBanStore) ListKeys(int, int) ([]string, error) { return nil, nil }

func TestModerationIndexKeepsAdditionsDuringRebuild(t *testing.T) {
	var idx moderationIndex
	idx.beginRebuild()
	idx.add(indexEntry{ChannelID: "channel", UserID: "user"})
	idx.endRebuild(map[string]bool{}, map[string]map[string]bool{}, true)
	assert.True(t, idx.mayBeRestricted("channel", "user"))
}

func TestOnPluginClusterEventAddsToIndex(t *testing.T) {
	p, _, _ := newTestPlugin(t, map[string]bool{})
	p.rebuildIndex()
	data, err := json.Marshal(indexEntry{UserID: "banned-elsewhere"})
	assert.NoError(t, err)

	p.OnPluginClusterEvent(nil, model.PluginClusterEvent{Id: indexClusterEventID, Data: data})

	assert.True(t, p.index.mayBeBanned("banned-elsewhere"))
}

func TestParseModerationKeys(t *testing.T) {
	channelID, userID, ok := kvstore.ParseRestrictionKey(kvstore.RestrictionKey("c1", "u1"))
	assert.True(t, ok)
	assert.Equal(t, "c1", channelID)
	assert.Equal(t, "u1", userID)
	_, _, ok = kvstore.ParseRestrictionKey("reports_c1")
	assert.False(t, ok)

	userID, ok = kvstore.ParseBanKey("ban_u1")
	assert.True(t, ok)
	assert.Equal(t, "u1", userID)
	_, ok = kvstore.ParseBanKey("mods_c1")
	assert.False(t, ok)
}
