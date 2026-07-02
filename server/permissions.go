package main

import (
	"slices"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/pkg/errors"
)

// The plugin distinguishes three tiers per channel:
//
//   - admins: system admins and admins of the channel's team. They can do
//     everything moderators can, plus ban or remove members and assign
//     moderators.
//   - moderators: members granted the role by an admin, plus channel admins.
//     They can mute, timeout, warn, delete messages, and work the report
//     queue.
//   - members: everyone else. They can report messages and people.
const (
	tierMember = iota
	tierModerator
	tierAdmin
)

func (p *Plugin) isAdmin(userID, channelID string) bool {
	if p.client.User.HasPermissionTo(userID, model.PermissionManageSystem) {
		return true
	}
	channel, err := p.client.Channel.Get(channelID)
	if err != nil || channel.TeamId == "" {
		return false
	}
	return p.client.User.HasPermissionToTeam(userID, channel.TeamId, model.PermissionManageTeam)
}

func (p *Plugin) isModerator(userID, channelID string) bool {
	if p.isAdmin(userID, channelID) {
		return true
	}
	if member, err := p.client.Channel.GetMember(channelID, userID); err == nil && member.SchemeAdmin {
		return true
	}
	moderators, err := p.kvstore.GetModerators(channelID)
	if err != nil {
		p.API.LogError("Failed to get moderators", "channel_id", channelID, "error", err.Error())
		return false
	}
	return slices.Contains(moderators, userID)
}

func (p *Plugin) tier(userID, channelID string) int {
	if p.isAdmin(userID, channelID) {
		return tierAdmin
	}
	if p.isModerator(userID, channelID) {
		return tierModerator
	}
	return tierMember
}

// checkCanActOn verifies that the actor outranks the target: moderators can
// act on regular members, admins can also act on moderators, and nobody can
// act on admins or themselves.
func (p *Plugin) checkCanActOn(actorID, targetID, channelID string) error {
	if actorID == targetID {
		return errors.New("you can't moderate yourself")
	}
	if targetID == p.botUserID {
		return errors.New("you can't moderate the moderation bot")
	}
	actorTier := p.tier(actorID, channelID)
	if actorTier < tierModerator {
		return errors.New("you don't have permission to moderate this channel")
	}
	if p.tier(targetID, channelID) >= actorTier {
		return errors.New("you can't moderate a member with the same or a higher moderation role")
	}
	return nil
}
