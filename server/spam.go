package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/pluginapi/cluster"
	"github.com/pkg/errors"

	"github.com/svelle/mattermost-plugin-moderation/server/store/kvstore"
)

// Spam limits count the conversations and channels a member starts posting
// in, and refuse new ones past a threshold: the mass direct message limit
// (see dm_rate.go) and the cross-posting limit (see spam_channels.go) share
// the machinery here. The flood limit, also in spam_channels.go, counts
// posts in a single channel instead.
const (
	// maxStoredSpamContacts caps the per-member list so the KV value stays
	// small even with the daily limit turned off.
	maxStoredSpamContacts = 1000

	// spamLockTimeout bounds the wait for the per-member lock. If it
	// can't be taken, the post is let through rather than held up.
	spamLockTimeout = 5 * time.Second

	// maxSeenEntries bounds each in-memory cache below; a cache is
	// cleared when full.
	maxSeenEntries = 100000
)

// spamGuard holds this node's in-memory state for the spam limits. Apart
// from the flood counts, it only skips lookups; the KV store stays the
// source of truth.
type spamGuard struct {
	// channels maps channel ID to *spamChannelInfo. Channel type and group
	// membership don't change, so entries never go stale.
	channels sync.Map

	mu sync.Mutex
	// seen maps scope + ":" + userID + ":" + channelID to when the member
	// was counted there, for posts that need no further lookup.
	seen map[string]int64
	// floods maps userID + ":" + channelID to the times of their recent
	// posts in that channel.
	floods map[string][]int64
}

type spamChannelInfo struct {
	team   bool // an open or private team channel
	direct bool // a direct or group message
	self   bool // a direct message with oneself
	people int  // how many others a direct or group message reaches
}

// seenSince reports whether the member was counted in the channel at or
// after since.
func (g *spamGuard) seenSince(scope, userID, channelID string, since int64) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	at, ok := g.seen[scope+":"+userID+":"+channelID]
	return ok && at >= since
}

func (g *spamGuard) markSeen(scope, userID, channelID string, at int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.seen == nil || len(g.seen) >= maxSeenEntries {
		g.seen = map[string]int64{}
	}
	g.seen[scope+":"+userID+":"+channelID] = at
}

// spamChannel returns what the spam limits need to know about a channel.
func (p *Plugin) spamChannel(channelID string) (*spamChannelInfo, error) {
	if info, ok := p.spamGuard.channels.Load(channelID); ok {
		return info.(*spamChannelInfo), nil
	}
	channel, err := p.client.Channel.Get(channelID)
	if err != nil {
		return nil, err
	}
	info := &spamChannelInfo{}
	switch channel.Type {
	case model.ChannelTypeOpen, model.ChannelTypePrivate:
		info.team = true
	case model.ChannelTypeDirect:
		info.direct = true
		info.people = 1
		ids := strings.Split(channel.Name, "__")
		info.self = len(ids) == 2 && ids[0] == ids[1]
	case model.ChannelTypeGroup:
		info.direct = true
		stats, statsErr := p.client.Channel.GetChannelStats(channelID)
		if statsErr != nil {
			return nil, statsErr
		}
		info.people = max(int(stats.MemberCount)-1, 1)
	}
	p.spamGuard.channels.Store(channelID, info)
	return info, nil
}

// spamBlockedReason returns why a post breaks a spam limit, or an empty
// string when it doesn't. Any lookup error lets the post through: a plugin
// failure must never stop posting server-wide.
func (p *Plugin) spamBlockedReason(post *model.Post) string {
	// Incoming webhooks post as the person who created them, so a busy
	// integration would otherwise count against that person. The server
	// rejects this prop on posts from ordinary users, so it can't be faked.
	if post.GetProp(model.PostPropsFromWebhook) == "true" && p.getConfiguration().ExemptBotsFromSpamLimits {
		return ""
	}
	info, err := p.spamChannel(post.ChannelId)
	if err != nil {
		p.API.LogError("Failed to check channel for spam limits", "channel_id", post.ChannelId, "error", err.Error())
		return ""
	}
	switch {
	case info.direct && !info.self:
		return p.massDMBlockedReason(post, info)
	case info.team:
		if reason := p.crossPostBlockedReason(post); reason != "" {
			return reason
		}
		return p.floodBlockedReason(post)
	}
	return ""
}

// exemptFromSpamLimits reports whether the member is a system admin, or a
// bot while bots (and incoming webhooks) are exempt.
func (p *Plugin) exemptFromSpamLimits(userID string, config *configuration) (bool, error) {
	user, err := p.client.User.Get(userID)
	if err != nil {
		return false, err
	}
	if user.IsBot && config.ExemptBotsFromSpamLimits {
		return true, nil
	}
	return p.client.User.HasPermissionTo(userID, model.PermissionManageSystem), nil
}

// spamLimits configures one scope of the contact-counting spam limits.
type spamLimits struct {
	scope     string
	threshold int           // people within window; zero turns it off
	window    time.Duration // rolling window for threshold
	daily     int           // people within history; zero turns it off
	history   time.Duration // how long contacts are kept
	blockFor  time.Duration // how long a block lasts
}

func (c *configuration) massDMLimits() spamLimits {
	return spamLimits{
		scope:     kvstore.SpamScopeDM,
		threshold: c.MassDMThreshold,
		window:    time.Duration(c.MassDMWindowMinutes) * time.Minute,
		daily:     c.MassDMDailyThreshold,
		history:   24 * time.Hour,
		blockFor:  time.Duration(c.MassDMBlockMinutes) * time.Minute,
	}
}

func (c *configuration) crossPostLimits() spamLimits {
	window := time.Duration(c.CrossPostWindowMinutes) * time.Minute
	return spamLimits{
		scope:     kvstore.SpamScopeChannel,
		threshold: c.CrossPostThreshold,
		window:    window,
		history:   window,
		blockFor:  time.Duration(c.CrossPostBlockMinutes) * time.Minute,
	}
}

// spamDecision is the outcome of recording a new contact. block is set when
// the post is refused; created is true for the one caller that put the
// block in place and so must alert admins.
type spamDecision struct {
	block   *kvstore.SpamBlock
	created bool
	firstAt int64 // when an allowed contact was first counted
	people  int   // people reached in the window that tripped the limit
	daily   bool  // the daily limit tripped rather than the short one
}

// recordContact decides, under a per-member cluster lock, whether a member
// may post in a conversation or channel they haven't been counted in, and
// records it if so. The lock keeps parallel posts to different channels
// from all slipping under the limit. exempt is consulted only when the
// limit would trip, so normal posting doesn't pay for the lookup.
func (p *Plugin) recordContact(userID, channelID string, people int, limits spamLimits, exempt func() bool) (*spamDecision, error) {
	mutex, err := cluster.NewMutex(p.API, "lock_"+limits.scope+"rate_"+userID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create cluster mutex")
	}
	ctx, cancel := context.WithTimeout(context.Background(), spamLockTimeout)
	defer cancel()
	if err = mutex.LockWithContext(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to lock spam limit contacts")
	}
	defer mutex.Unlock()

	now := p.now()
	contacts, err := p.kvstore.GetSpamContacts(limits.scope, userID)
	if err != nil {
		return nil, err
	}
	contacts = pruneSpamContacts(contacts, now-limits.history.Milliseconds())
	for _, c := range contacts {
		// Already counted, perhaps by a post another node let through:
		// conversations and channels a member started posting in stay
		// open even while blocked.
		if c.ChannelID == channelID {
			return &spamDecision{firstAt: c.FirstAt}, nil
		}
	}
	block, err := p.kvstore.GetSpamBlock(limits.scope, userID)
	if err != nil {
		return nil, err
	}
	if block != nil && block.Until > now {
		if exempt() {
			return &spamDecision{firstAt: now}, nil
		}
		return &spamDecision{block: block}, nil
	}

	windowStart := now - limits.window.Milliseconds()
	recent, daily := 0, 0
	for _, c := range contacts {
		daily += c.People
		if c.FirstAt >= windowStart {
			recent += c.People
		}
	}
	overShort := limits.threshold > 0 && recent+people > limits.threshold
	overDaily := limits.daily > 0 && daily+people > limits.daily
	// A single group message bigger than the limit is still let through
	// when nothing else was started in the window, so the limit can't
	// make large group messages impossible.
	if (overShort && recent > 0) || (overDaily && daily > 0) {
		if exempt() {
			// Not recorded, so exempt members never build up a history.
			return &spamDecision{firstAt: now}, nil
		}
		block = &kvstore.SpamBlock{Until: now + limits.blockFor.Milliseconds(), CreateAt: now}
		created, setErr := p.kvstore.TrySetSpamBlock(limits.scope, userID, block, limits.blockFor)
		if setErr != nil {
			// The member is over the limit either way; refuse the post but
			// leave alerting to a caller that manages to store the block.
			p.API.LogError("Failed to store spam limit block", "user_id", userID, "scope", limits.scope, "error", setErr.Error())
		}
		decision := &spamDecision{block: block, created: created, people: recent + people, daily: !overShort}
		if decision.daily {
			decision.people = daily + people
		}
		return decision, nil
	}

	contacts = append(contacts, kvstore.SpamContact{ChannelID: channelID, FirstAt: now, People: people})
	if over := len(contacts) - maxStoredSpamContacts; over > 0 {
		contacts = contacts[over:]
	}
	if err := p.kvstore.SaveSpamContacts(limits.scope, userID, contacts, limits.history); err != nil {
		return nil, err
	}
	return &spamDecision{firstAt: now}, nil
}

// pruneSpamContacts keeps only contacts first counted at or after the cutoff.
func pruneSpamContacts(contacts []kvstore.SpamContact, cutoff int64) []kvstore.SpamContact {
	recent := make([]kvstore.SpamContact, 0, len(contacts)+1)
	for _, c := range contacts {
		if c.FirstAt >= cutoff {
			recent = append(recent, c)
		}
	}
	return recent
}

// spamAlert is the scope-specific wording of an admin alert.
type spamAlert struct {
	summary  func(member string) string // what the member, an @mention, did
	blocked  string                     // what the block stops, for example "post in other channels"
	allowed  string                     // what still works during the block
	blockFor time.Duration
}

// handleSpamLimit alerts system admins that a member hit a spam limit and,
// when configured, suspends the account first so the alert can say so.
func (p *Plugin) handleSpamLimit(userID, channelID string, alert spamAlert, config *configuration) {
	user, err := p.client.User.Get(userID)
	if err != nil {
		p.API.LogError("Failed to load member who hit a spam limit", "user_id", userID, "error", err.Error())
		return
	}
	// Read the username before a suspension replaces it with a placeholder.
	summary := alert.summary("@" + user.Username)
	blockLabel := plural(int64(alert.blockFor/time.Minute), "minute")

	action := fmt.Sprintf(
		"They can't %s for %s, but %s. To stop them completely, ban them from the Members tab of the Moderation panel in a channel they belong to.",
		alert.blocked, blockLabel, alert.allowed,
	)
	if config.SpamAction == SpamActionSuspend {
		action = p.suspendForSpam(user, channelID, alert.blocked, blockLabel, config)
	}
	message := summary + "\n\n" + action
	for _, id := range p.systemAdminIDs() {
		p.dmFromBot(id, message)
	}
}

// suspendForSpam bans the member on behalf of the moderation bot and
// returns the sentence admins see about it.
func (p *Plugin) suspendForSpam(user *model.User, channelID, blocked, blockLabel string, config *configuration) string {
	if user.IsBot {
		return fmt.Sprintf(
			"Bots can't be suspended by the plugin, so only the block is in place: it can't %s for %s. Disable the bot under Integrations > Bot Accounts if it's misbehaving.",
			blocked, blockLabel,
		)
	}
	if ban, err := p.kvstore.GetBan(user.Id); err == nil && ban != nil {
		return "Their account was already banned."
	}
	if _, err := p.applyBan(p.botUserID, channelID, user, false); err != nil {
		p.API.LogError("Failed to suspend member who hit a spam limit", "user_id", user.Id, "error", err.Error())
		return fmt.Sprintf(
			"Suspending their account failed (%s), so only the block is in place: they can't %s for %s. Ban them from the Moderation panel in a channel they belong to.",
			err.Error(), blocked, blockLabel,
		)
	}
	hidden := ""
	if config.HideBannedMessages {
		hidden = " Their recent messages are being hidden."
	}
	return "Their account was suspended automatically: it's deactivated and banned." + hidden +
		" A system admin can lift the suspension from the Members tab of the Moderation panel in any channel they belong to: search for them and choose Unban."
}
