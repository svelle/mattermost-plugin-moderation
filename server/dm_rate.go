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

// Mass-DM detection counts the direct and group conversations a member
// starts: a conversation counts the first time they post in it, weighted by
// how many people it reaches. Replies in conversations they already posted
// in never count, so ordinary chatting isn't limited however busy it gets.
const (
	lockPrefixDMRate = "lock_dmrate_"

	// dmContactHistory is how long started conversations are remembered,
	// which is also the window for MassDMDailyThreshold.
	dmContactHistory = 24 * time.Hour

	// maxStoredDMContacts caps the per-member list so the KV value stays
	// small even with the daily limit turned off.
	maxStoredDMContacts = 1000

	// firstContactScanSize is how many recent posts are checked for an
	// earlier post by the member. A conversation busier than that is
	// treated as one they already take part in.
	firstContactScanSize = 200

	// dmRateLockTimeout bounds the wait for the per-member lock. If it
	// can't be taken, the post is let through rather than held up.
	dmRateLockTimeout = 5 * time.Second

	// maxKnownConversations bounds the in-memory set of conversations a
	// member is known to have posted in; it is cleared when full.
	maxKnownConversations = 100000
)

// dmGuard holds this node's in-memory caches for mass-DM detection. They
// only skip lookups; the KV store stays the source of truth.
type dmGuard struct {
	// channels maps channel ID to *dmChannelInfo. Channel type and group
	// membership don't change, so entries never go stale.
	channels sync.Map

	mu    sync.Mutex
	known map[string]bool // userID + ":" + channelID
}

type dmChannelInfo struct {
	direct bool // a direct or group message, not a team channel
	self   bool // a direct message with oneself
	people int  // how many others the conversation reaches
}

func (g *dmGuard) isKnown(userID, channelID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.known[userID+":"+channelID]
}

func (g *dmGuard) markKnown(userID, channelID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.known == nil || len(g.known) >= maxKnownConversations {
		g.known = map[string]bool{}
	}
	g.known[userID+":"+channelID] = true
}

// dmChannel returns what mass-DM detection needs to know about a channel.
func (p *Plugin) dmChannel(channelID string) (*dmChannelInfo, error) {
	if info, ok := p.dmGuard.channels.Load(channelID); ok {
		return info.(*dmChannelInfo), nil
	}
	channel, err := p.client.Channel.Get(channelID)
	if err != nil {
		return nil, err
	}
	info := &dmChannelInfo{}
	switch channel.Type {
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
	p.dmGuard.channels.Store(channelID, info)
	return info, nil
}

// hasPostedIn reports whether the member has a recent post in the channel.
func (p *Plugin) hasPostedIn(channelID, userID string) (bool, error) {
	postList, err := p.client.Post.GetPostsForChannel(channelID, 0, firstContactScanSize)
	if err != nil {
		return false, err
	}
	if postList == nil {
		return false, nil
	}
	if len(postList.Order) >= firstContactScanSize {
		return true, nil
	}
	for _, postID := range postList.Order {
		if post := postList.Posts[postID]; post != nil && post.UserId == userID && !post.IsSystemMessage() {
			return true, nil
		}
	}
	return false, nil
}

// exemptFromDMLimit reports whether the member is a bot or a system admin.
func (p *Plugin) exemptFromDMLimit(userID string) (bool, error) {
	user, err := p.client.User.Get(userID)
	if err != nil {
		return false, err
	}
	return user.IsBot || p.client.User.HasPermissionTo(userID, model.PermissionManageSystem), nil
}

// massDMBlockedReason returns why a member can't start this conversation, or
// an empty string when they can. Any lookup error lets the post through: a
// plugin failure must never stop direct messages server-wide.
func (p *Plugin) massDMBlockedReason(post *model.Post) string {
	config := p.getConfiguration()
	if config.MassDMThreshold == 0 && config.MassDMDailyThreshold == 0 {
		return ""
	}
	userID, channelID := post.UserId, post.ChannelId
	if p.dmGuard.isKnown(userID, channelID) {
		return ""
	}
	info, err := p.dmChannel(channelID)
	if err != nil {
		p.API.LogError("Failed to check channel for mass-DM detection", "channel_id", channelID, "error", err.Error())
		return ""
	}
	if !info.direct || info.self {
		return ""
	}
	posted, err := p.hasPostedIn(channelID, userID)
	if err != nil {
		p.API.LogError("Failed to check earlier posts for mass-DM detection", "channel_id", channelID, "error", err.Error())
		return ""
	}
	if posted {
		p.dmGuard.markKnown(userID, channelID)
		return ""
	}
	exempt, err := p.exemptFromDMLimit(userID)
	if err != nil {
		p.API.LogError("Failed to check member for mass-DM detection", "user_id", userID, "error", err.Error())
		return ""
	}
	if exempt {
		return ""
	}

	decision, err := p.recordFirstContact(userID, channelID, info.people, config)
	if err != nil {
		p.API.LogError("Failed to record new conversation", "user_id", userID, "channel_id", channelID, "error", err.Error())
		return ""
	}
	if decision.block == nil {
		p.dmGuard.markKnown(userID, channelID)
		return ""
	}
	if decision.created {
		p.handleMassDM(userID, channelID, decision, config)
	}
	return fmt.Sprintf(
		"You've started conversations with a lot of people in a short time, so you can't start new direct messages for %s. You can still reply in conversations you've already started.",
		remainingLabel(decision.block.Until-p.now()),
	)
}

// dmDecision is the outcome of recording a new conversation. block is set
// when the conversation is refused; created is true for the one caller that
// put the block in place and so must alert admins.
type dmDecision struct {
	block   *kvstore.DMBlock
	created bool
	people  int  // people reached in the window that tripped the limit
	daily   bool // the 24-hour limit tripped rather than the short one
}

// recordFirstContact decides, under a per-member cluster lock, whether a
// member may start a conversation, and records it if so. The lock keeps
// parallel posts to different conversations from all slipping under the
// limit.
func (p *Plugin) recordFirstContact(userID, channelID string, people int, config *configuration) (*dmDecision, error) {
	mutex, err := cluster.NewMutex(p.API, lockPrefixDMRate+userID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create cluster mutex")
	}
	ctx, cancel := context.WithTimeout(context.Background(), dmRateLockTimeout)
	defer cancel()
	if err = mutex.LockWithContext(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to lock direct message contacts")
	}
	defer mutex.Unlock()

	contacts, err := p.kvstore.GetDMContacts(userID)
	if err != nil {
		return nil, err
	}
	for _, c := range contacts {
		// Already counted, perhaps by a post another node let through:
		// conversations a member started stay open even while blocked.
		if c.ChannelID == channelID {
			return &dmDecision{}, nil
		}
	}
	now := p.now()
	block, err := p.kvstore.GetDMBlock(userID)
	if err != nil {
		return nil, err
	}
	if block != nil && block.Until > now {
		return &dmDecision{block: block}, nil
	}

	contacts = pruneDMContacts(contacts, now-dmContactHistory.Milliseconds())
	windowStart := now - (time.Duration(config.MassDMWindowMinutes) * time.Minute).Milliseconds()
	recent, daily := 0, 0
	for _, c := range contacts {
		daily += c.People
		if c.FirstAt >= windowStart {
			recent += c.People
		}
	}
	overShort := config.MassDMThreshold > 0 && recent+people > config.MassDMThreshold
	overDaily := config.MassDMDailyThreshold > 0 && daily+people > config.MassDMDailyThreshold
	// A single group message bigger than the limit is still let through
	// when nothing else was started in the window, so the limit can't
	// make large group messages impossible.
	if (overShort && recent > 0) || (overDaily && daily > 0) {
		blockFor := time.Duration(config.MassDMBlockMinutes) * time.Minute
		block = &kvstore.DMBlock{Until: now + blockFor.Milliseconds(), CreateAt: now}
		created, setErr := p.kvstore.TrySetDMBlock(userID, block, blockFor)
		if setErr != nil {
			// The member is over the limit either way; refuse the post but
			// leave alerting to a caller that manages to store the block.
			p.API.LogError("Failed to store direct message block", "user_id", userID, "error", setErr.Error())
		}
		decision := &dmDecision{block: block, created: created, people: recent + people, daily: !overShort}
		if decision.daily {
			decision.people = daily + people
		}
		return decision, nil
	}

	contacts = append(contacts, kvstore.DMContact{ChannelID: channelID, FirstAt: now, People: people})
	if over := len(contacts) - maxStoredDMContacts; over > 0 {
		contacts = contacts[over:]
	}
	if err := p.kvstore.SaveDMContacts(userID, contacts, dmContactHistory); err != nil {
		return nil, err
	}
	return &dmDecision{}, nil
}

// pruneDMContacts keeps only conversations started at or after the cutoff.
func pruneDMContacts(contacts []kvstore.DMContact, cutoff int64) []kvstore.DMContact {
	recent := make([]kvstore.DMContact, 0, len(contacts)+1)
	for _, c := range contacts {
		if c.FirstAt >= cutoff {
			recent = append(recent, c)
		}
	}
	return recent
}

// handleMassDM alerts system admins that a member was blocked and, when
// configured, suspends the account first so the alert can say so.
func (p *Plugin) handleMassDM(userID, channelID string, decision *dmDecision, config *configuration) {
	user, err := p.client.User.Get(userID)
	if err != nil {
		p.API.LogError("Failed to load member blocked for mass DMs", "user_id", userID, "error", err.Error())
		return
	}
	// Read the username before a suspension replaces it with a placeholder.
	username := user.Username

	window := plural(int64(config.MassDMWindowMinutes), "minute")
	if decision.daily {
		window = "24 hours"
	}
	summary := fmt.Sprintf(
		"**Possible direct message spam:** @%s tried to start conversations with %d people within %s.",
		username, decision.people, window,
	)

	action := fmt.Sprintf(
		"They can't start new direct or group messages for %s, but can still reply in conversations they already started. "+
			"To stop them completely, ban them from the Members tab of the Moderation panel in a channel they belong to.",
		plural(int64(config.MassDMBlockMinutes), "minute"),
	)
	if config.MassDMAction == MassDMActionSuspend {
		action = p.suspendForMassDM(user, channelID, config)
	}
	message := summary + "\n\n" + action
	for _, id := range p.systemAdminIDs() {
		p.dmFromBot(id, message)
	}
}

// suspendForMassDM bans the member on behalf of the moderation bot and
// returns the sentence admins see about it.
func (p *Plugin) suspendForMassDM(user *model.User, channelID string, config *configuration) string {
	if ban, err := p.kvstore.GetBan(user.Id); err == nil && ban != nil {
		return "Their account was already banned."
	}
	if _, err := p.applyBan(p.botUserID, channelID, user, false); err != nil {
		p.API.LogError("Failed to suspend member for mass DMs", "user_id", user.Id, "error", err.Error())
		return fmt.Sprintf(
			"Suspending their account failed (%s), so only the block is in place: they can't start new direct messages for %s. Ban them from the Moderation panel in a channel they belong to.",
			err.Error(), plural(int64(config.MassDMBlockMinutes), "minute"),
		)
	}
	hidden := ""
	if config.HideBannedMessages {
		hidden = " Their recent messages are being hidden."
	}
	return "Their account was suspended automatically: it's deactivated and banned." + hidden +
		" A system admin can lift the suspension from the Members tab of the Moderation panel in any channel they belong to: search for them and choose Unban."
}
