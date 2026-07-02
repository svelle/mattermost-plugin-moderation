package main

import (
	"fmt"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

// MessageWillBePosted blocks posts from members who are muted or timed out in
// the channel, and from banned members in case their account was reactivated
// outside the plugin.
func (p *Plugin) MessageWillBePosted(_ *plugin.Context, post *model.Post) (*model.Post, string) {
	if post == nil || post.UserId == "" || post.UserId == p.botUserID || post.IsSystemMessage() {
		return post, ""
	}

	if ban, err := p.kvstore.GetBan(post.UserId); err == nil && ban != nil {
		return nil, "You've been banned from this server and can't post."
	}

	restriction, err := p.kvstore.GetRestriction(post.ChannelId, post.UserId)
	if err != nil {
		p.API.LogError("Failed to check restrictions", "user_id", post.UserId, "channel_id", post.ChannelId, "error", err.Error())
		return post, ""
	}
	if restriction == nil {
		return post, ""
	}
	if restriction.Muted {
		return nil, "You're muted in this channel. A moderator has to lift the mute before you can post again."
	}
	if restriction.TimedOutNow(p.now()) {
		return nil, fmt.Sprintf("You're timed out in this channel. You can post again in %s.", remainingLabel(restriction.TimeoutUntil-p.now()))
	}
	return post, ""
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
	restriction, err := p.kvstore.GetRestriction(post.ChannelId, reaction.UserId)
	if err != nil || !restriction.Active(p.now()) {
		return
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

func remainingLabel(millis int64) string {
	minutes := (millis + 59999) / 60000
	switch {
	case minutes < 60:
		return fmt.Sprintf("%d minutes", minutes)
	case minutes < 48*60:
		return fmt.Sprintf("%d hours", (minutes+59)/60)
	default:
		return fmt.Sprintf("%d days", (minutes+24*60-1)/(24*60))
	}
}
