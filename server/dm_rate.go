package main

import (
	"fmt"

	"github.com/mattermost/mattermost/server/public/model"

	"github.com/svelle/mattermost-plugin-moderation/server/store/kvstore"
)

// The mass direct message limit counts the direct and group conversations a
// member starts: a conversation counts the first time they post in it,
// weighted by how many people it reaches. Replies in conversations they
// already posted in never count, so ordinary chatting isn't limited however
// busy it gets.

// firstContactScanSize is how many recent posts are checked for an earlier
// post by the member. A conversation busier than that is treated as one they
// already take part in.
const firstContactScanSize = 200

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

// massDMBlockedReason returns why a member can't start this direct or group
// conversation, or an empty string when they can.
func (p *Plugin) massDMBlockedReason(post *model.Post, info *spamChannelInfo) string {
	config := p.getConfiguration()
	if config.MassDMThreshold == 0 && config.MassDMDailyThreshold == 0 {
		return ""
	}
	userID, channelID := post.UserId, post.ChannelId
	// Conversations the member is known to take part in stay known.
	if p.spamGuard.seenSince(kvstore.SpamScopeDM, userID, channelID, 0) {
		return ""
	}
	posted, err := p.hasPostedIn(channelID, userID)
	if err != nil {
		p.API.LogError("Failed to check earlier posts for the mass direct message limit", "channel_id", channelID, "error", err.Error())
		return ""
	}
	if posted {
		p.spamGuard.markSeen(kvstore.SpamScopeDM, userID, channelID, p.now())
		return ""
	}

	exempt := func() bool {
		isExempt, exemptErr := p.exemptFromSpamLimits(userID, config)
		if exemptErr != nil {
			p.API.LogError("Failed to check member for the mass direct message limit", "user_id", userID, "error", exemptErr.Error())
			return true
		}
		return isExempt
	}
	limits := config.massDMLimits()
	decision, err := p.recordContact(userID, channelID, info.people, limits, exempt)
	if err != nil {
		p.API.LogError("Failed to record new conversation", "user_id", userID, "channel_id", channelID, "error", err.Error())
		return ""
	}
	if decision.block == nil {
		p.spamGuard.markSeen(kvstore.SpamScopeDM, userID, channelID, decision.firstAt)
		return ""
	}
	if decision.created {
		window := plural(int64(config.MassDMWindowMinutes), "minute")
		if decision.daily {
			window = "24 hours"
		}
		p.handleSpamLimit(userID, channelID, spamAlert{
			summary: func(member string) string {
				return fmt.Sprintf("**Possible direct message spam:** %s tried to start conversations with %d people within %s.", member, decision.people, window)
			},
			blocked:  "start new direct or group messages",
			allowed:  "can still reply in conversations they already started",
			blockFor: limits.blockFor,
		}, config)
	}
	return fmt.Sprintf(
		"You've started conversations with a lot of people in a short time, so you can't start new direct messages for %s. You can still reply in conversations you've already started.",
		remainingLabel(decision.block.Until-p.now()),
	)
}
