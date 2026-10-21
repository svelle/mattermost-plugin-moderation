package main

import (
	"fmt"
	"time"

	"github.com/mattermost/mattermost/server/public/model"

	"github.com/svelle/mattermost-plugin-moderation/server/store/kvstore"
)

// Team channels have two spam limits. The cross-posting limit counts the
// different channels a member posts in within a few minutes, using the same
// cluster-wide counting as the mass direct message limit. The flood limit
// counts a member's posts in one channel within seconds, in memory on each
// node, and times them out in that channel when they post too many.

// exemptInChannel reports whether the member is exempt from the channel spam
// limits: system admins, bots while bots are exempt, and the channel's
// moderators and admins. Lookup errors count as exempt, so a plugin failure
// never stops someone posting.
func (p *Plugin) exemptInChannel(userID, channelID string, config *configuration) bool {
	exempt, err := p.exemptFromSpamLimits(userID, config)
	if err != nil {
		p.API.LogError("Failed to check member for channel spam limits", "user_id", userID, "error", err.Error())
		return true
	}
	return exempt || p.isModerator(userID, channelID)
}

// crossPostBlockedReason returns why a member can't post in this channel
// after posting in too many others, or an empty string when they can.
func (p *Plugin) crossPostBlockedReason(post *model.Post) string {
	config := p.getConfiguration()
	if config.CrossPostThreshold == 0 {
		return ""
	}
	userID, channelID := post.UserId, post.ChannelId
	limits := config.crossPostLimits()
	if p.spamGuard.seenSince(kvstore.SpamScopeChannel, userID, channelID, p.now()-limits.window.Milliseconds()) {
		return ""
	}

	exempt := func() bool { return p.exemptInChannel(userID, channelID, config) }
	decision, err := p.recordContact(userID, channelID, 1, limits, exempt)
	if err != nil {
		p.API.LogError("Failed to record channel for the cross-posting limit", "user_id", userID, "channel_id", channelID, "error", err.Error())
		return ""
	}
	if decision.block == nil {
		p.spamGuard.markSeen(kvstore.SpamScopeChannel, userID, channelID, decision.firstAt)
		return ""
	}
	if decision.created {
		p.handleSpamLimit(userID, channelID, spamAlert{
			summary: func(member string) string {
				return fmt.Sprintf("**Possible channel spam:** %s tried to post in %d different channels within %s.",
					member, decision.people, plural(int64(config.CrossPostWindowMinutes), "minute"))
			},
			blocked:  "post in channels they haven't posted in recently",
			allowed:  "can keep posting in the channels they already posted in",
			blockFor: limits.blockFor,
		}, config)
	}
	return fmt.Sprintf(
		"You've posted in a lot of channels in a short time, so you can't post in other channels for %s. You can keep posting in the channels you've already posted in.",
		remainingLabel(decision.block.Until-p.now()),
	)
}

// recordFloodPost adds a post to the member's recent posts in the channel
// and returns how many fall within the window.
func (g *spamGuard) recordFloodPost(userID, channelID string, now int64, window time.Duration) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.floods == nil || len(g.floods) >= maxSeenEntries {
		g.floods = map[string][]int64{}
	}
	key := userID + ":" + channelID
	stamps := append(pruneStamps(g.floods[key], now-window.Milliseconds()), now)
	g.floods[key] = stamps
	return len(stamps)
}

func (g *spamGuard) resetFlood(userID, channelID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.floods, userID+":"+channelID)
}

// floodBlockedReason times a member out in the channel when they post more
// than FloodThreshold messages within FloodWindowSeconds, and returns why
// the post was refused, or an empty string when it wasn't.
func (p *Plugin) floodBlockedReason(post *model.Post) string {
	config := p.getConfiguration()
	if config.FloodThreshold == 0 {
		return ""
	}
	userID, channelID := post.UserId, post.ChannelId
	window := time.Duration(config.FloodWindowSeconds) * time.Second
	if p.spamGuard.recordFloodPost(userID, channelID, p.now(), window) <= config.FloodThreshold {
		return ""
	}
	if p.exemptInChannel(userID, channelID, config) {
		return ""
	}
	p.spamGuard.resetFlood(userID, channelID)

	duration := time.Duration(config.FloodTimeoutMinutes) * time.Minute
	label := plural(int64(config.FloodTimeoutMinutes), "minute")
	if err := p.applyTimeout(p.botUserID, channelID, userID, duration, label); err != nil {
		p.API.LogError("Failed to time out member for flooding", "user_id", userID, "channel_id", channelID, "error", err.Error())
	} else {
		p.notifyModeratorsOfFlood(userID, channelID, config, label)
	}
	return fmt.Sprintf(
		"You're posting too quickly, so you've been timed out in this channel for %s. You stay in the channel but can't post or react until the timeout ends.",
		label,
	)
}

// notifyModeratorsOfFlood tells the channel's assigned moderators about an
// automatic timeout. There's deliberately no fallback to system admins, so a
// flood doesn't page them.
func (p *Plugin) notifyModeratorsOfFlood(userID, channelID string, config *configuration, label string) {
	moderators, err := p.kvstore.GetModerators(channelID)
	if err != nil {
		p.API.LogError("Failed to get moderators", "channel_id", channelID, "error", err.Error())
		return
	}
	if len(moderators) == 0 {
		return
	}
	user, err := p.client.User.Get(userID)
	if err != nil {
		p.API.LogError("Failed to load member timed out for flooding", "user_id", userID, "error", err.Error())
		return
	}
	message := fmt.Sprintf(
		"@%s was timed out in %s for %s automatically for posting more than %d messages within %s. Lift the timeout from the Members tab of the Moderation panel if it was a mistake.",
		user.Username, p.channelLink(channelID), label, config.FloodThreshold, plural(int64(config.FloodWindowSeconds), "second"),
	)
	for _, id := range moderators {
		if id != userID && id != p.botUserID {
			p.dmFromBot(id, message)
		}
	}
}
