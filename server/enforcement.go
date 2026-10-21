package main

import (
	"fmt"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

// MessageWillBePosted blocks posts from members who are muted or timed out in
// the channel, from banned members in case their account was reactivated
// outside the plugin, and new direct messages from members who started too
// many conversations too quickly.
func (p *Plugin) MessageWillBePosted(_ *plugin.Context, post *model.Post) (*model.Post, string) {
	if post == nil || post.UserId == "" || post.UserId == p.botUserID || post.IsSystemMessage() {
		return post, ""
	}
	reason := p.postingBlockedReason(post.ChannelId, post.UserId)
	if reason == "" {
		reason = p.massDMBlockedReason(post)
	}
	if reason != "" {
		// The webapp only shows a failed post with "Retry" for a rejected
		// message, so tell the member why in the channel as well.
		p.client.Post.SendEphemeralPost(post.UserId, &model.Post{
			ChannelId: post.ChannelId,
			RootId:    post.RootId,
			Message:   reason,
		})
		return nil, reason
	}
	return post, ""
}

// MessageWillBeUpdated stops restricted members from editing their messages,
// which would otherwise let a muted member rewrite what they already posted.
// It also keeps ban tombstones from showing an "Edited" marker.
func (p *Plugin) MessageWillBeUpdated(c *plugin.Context, newPost, oldPost *model.Post) (*model.Post, string) {
	if newPost == nil || oldPost == nil || newPost.UserId == p.botUserID || newPost.IsSystemMessage() {
		return newPost, ""
	}
	if p.isBanTombstone(newPost) {
		newPost.EditAt = oldPost.EditAt
		return newPost, ""
	}
	// Pins, reactions, and other metadata updates don't change content.
	if newPost.Message == oldPost.Message && newPost.FileIds.Equals(oldPost.FileIds) {
		return newPost, ""
	}
	// Admins editing someone else's message (to redact it, for example)
	// aren't affected by the author's restrictions.
	if c != nil && c.SessionId != "" {
		if session, err := p.API.GetSession(c.SessionId); err == nil && session.UserId != newPost.UserId {
			return newPost, ""
		}
	}
	if reason := p.postingBlockedReason(newPost.ChannelId, newPost.UserId); reason != "" {
		return nil, reason
	}
	return newPost, ""
}

// isBanTombstone reports whether an update is the plugin replacing a banned
// member's message with the placeholder.
func (p *Plugin) isBanTombstone(post *model.Post) bool {
	if post.Message != bannedMessagePlaceholder || post.GetProp(tombstoneProp) != true {
		return false
	}
	ban, err := p.kvstore.GetBan(post.UserId)
	return err == nil && ban != nil
}

// postingBlockedReason returns why a member can't post in a channel, or an
// empty string when they can.
func (p *Plugin) postingBlockedReason(channelID, userID string) string {
	if p.index.mayBeBanned(userID) {
		if ban, err := p.kvstore.GetBan(userID); err == nil && ban != nil {
			return "You've been banned from this server and can't post."
		}
	}
	if !p.index.mayBeRestricted(channelID, userID) {
		return ""
	}

	restriction, err := p.kvstore.GetRestriction(channelID, userID)
	if err != nil {
		p.API.LogError("Failed to check restrictions", "user_id", userID, "channel_id", channelID, "error", err.Error())
		return ""
	}
	if restriction == nil {
		return ""
	}
	if restriction.Muted {
		return "You're muted in this channel. A moderator has to lift the mute before you can post again."
	}
	if restriction.TimedOutNow(p.now()) {
		return fmt.Sprintf("You're timed out in this channel. You can post again in %s.", remainingLabel(restriction.TimeoutUntil-p.now()))
	}
	return ""
}

// ReactionHasBeenAdded removes reactions from restricted members — timeouts
// and mutes cover reactions too, and there is no pre-hook for reactions.
func (p *Plugin) ReactionHasBeenAdded(_ *plugin.Context, reaction *model.Reaction) {
	if reaction == nil || reaction.UserId == p.botUserID {
		return
	}
	post, err := p.client.Post.GetPost(reaction.PostId)
	if err != nil {
		return
	}
	banned := false
	if p.index.mayBeBanned(reaction.UserId) {
		// Same fallback as MessageWillBePosted: banned members can't react
		// even if their account was reactivated outside the plugin.
		ban, banErr := p.kvstore.GetBan(reaction.UserId)
		banned = banErr == nil && ban != nil
	}
	if !banned {
		if !p.index.mayBeRestricted(post.ChannelId, reaction.UserId) {
			return
		}
		restriction, rErr := p.kvstore.GetRestriction(post.ChannelId, reaction.UserId)
		if rErr != nil || !restriction.Active(p.now()) {
			return
		}
	}
	if err := p.client.Post.RemoveReaction(reaction); err != nil {
		p.API.LogError("Failed to remove restricted member's reaction", "user_id", reaction.UserId, "error", err.Error())
		return
	}
	p.client.Post.SendEphemeralPost(reaction.UserId, &model.Post{
		ChannelId: post.ChannelId,
		Message:   "You can't react in this channel while you're muted or timed out.",
	})
}

func plural(n int64, unit string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", unit)
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

func remainingLabel(millis int64) string {
	minutes := (millis + 59999) / 60000
	switch {
	case minutes < 60:
		return plural(minutes, "minute")
	case minutes < 48*60:
		return plural((minutes+59)/60, "hour")
	default:
		return plural((minutes+24*60-1)/(24*60), "day")
	}
}
