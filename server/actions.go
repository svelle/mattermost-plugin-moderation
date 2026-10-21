package main

import (
	"fmt"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/pkg/errors"

	"github.com/svelle/mattermost-plugin-moderation/server/store/kvstore"
)

// timeoutDurations are the options offered in the timeout modal.
var timeoutDurations = map[string]struct {
	Duration time.Duration
	Label    string
}{
	"5m":  {5 * time.Minute, "5 minutes"},
	"1h":  {time.Hour, "1 hour"},
	"24h": {24 * time.Hour, "24 hours"},
	"7d":  {7 * 24 * time.Hour, "7 days"},
}

const bannedMessagePlaceholder = "*Message hidden — this member was banned from the server.*"

// tombstoneProp marks a message replaced with bannedMessagePlaceholder.
const tombstoneProp = "community_moderation_tombstone"

// tombstoneScanLimit bounds how many recent channel posts are scanned when
// hiding a banned member's messages.
const tombstoneScanLimit = 500

// memberScanPages and memberScanPageSize bound the unfiltered members-tab
// scan; channels beyond this size should rely on the members search.
const (
	memberScanPages    = 5
	memberScanPageSize = 200
)

func (p *Plugin) muteUser(actorID, channelID, targetID string) error {
	if err := p.checkCanModerateMember(actorID, targetID, channelID); err != nil {
		return err
	}
	restriction, err := p.kvstore.GetRestriction(channelID, targetID)
	if err != nil {
		return err
	}
	if restriction == nil {
		restriction = &kvstore.Restriction{}
	}
	restriction.Muted = true
	restriction.ByUserID = actorID
	restriction.CreateAt = p.now()
	if err := p.kvstore.SetRestriction(channelID, targetID, restriction); err != nil {
		return err
	}
	p.publishMembersChanged(channelID)
	p.dmFromBot(targetID, fmt.Sprintf(
		"You've been muted in %s by a moderator. You can read the channel but can't post or react until a moderator lifts the mute.",
		p.channelLink(channelID),
	))
	return nil
}

func (p *Plugin) timeoutUser(actorID, channelID, targetID, durationID string) (string, error) {
	if err := p.checkCanModerateMember(actorID, targetID, channelID); err != nil {
		return "", err
	}
	duration, ok := timeoutDurations[durationID]
	if !ok {
		return "", errors.New("unknown timeout duration")
	}
	restriction, err := p.kvstore.GetRestriction(channelID, targetID)
	if err != nil {
		return "", err
	}
	if restriction == nil {
		restriction = &kvstore.Restriction{}
	}
	restriction.TimeoutUntil = p.now() + duration.Duration.Milliseconds()
	restriction.TimeoutLabel = duration.Label
	restriction.ByUserID = actorID
	restriction.CreateAt = p.now()
	if err := p.kvstore.SetRestriction(channelID, targetID, restriction); err != nil {
		return "", err
	}
	p.publishMembersChanged(channelID)
	p.dmFromBot(targetID, fmt.Sprintf(
		"You've been timed out in %s for %s. You stay in the channel but can't post or react until the timeout ends.",
		p.channelLink(channelID), duration.Label,
	))
	return duration.Label, nil
}

// liftRestrictions clears any mute or timeout on a member in a channel.
func (p *Plugin) liftRestrictions(actorID, channelID, targetID string) error {
	if !p.isModerator(actorID, channelID) {
		return errors.New("you don't have permission to moderate this channel")
	}
	restriction, err := p.kvstore.GetRestriction(channelID, targetID)
	if err != nil {
		return err
	}
	if !restriction.Active(p.now()) {
		return errors.New("this member has no active mute or timeout in this channel")
	}
	if err := p.kvstore.DeleteRestriction(channelID, targetID); err != nil {
		return err
	}
	p.publishMembersChanged(channelID)
	p.dmFromBot(targetID, fmt.Sprintf("A moderator lifted your restrictions in %s. You can post again.", p.channelLink(channelID)))
	return nil
}

func (p *Plugin) warnUser(actorID, channelID, targetID string) error {
	if err := p.checkCanModerateMember(actorID, targetID, channelID); err != nil {
		return err
	}
	p.dmFromBot(targetID, fmt.Sprintf(
		"⚠️ You've received a warning from the moderation team of %s. Please review the community guidelines — repeated issues can lead to a timeout or a ban.",
		p.channelLink(channelID),
	))
	return nil
}

// deletePostAsModerator deletes a message and returns the channel it was in.
func (p *Plugin) deletePostAsModerator(actorID, postID string) (string, error) {
	post, err := p.client.Post.GetPost(postID)
	if err != nil {
		return "", errors.New("message not found")
	}
	if !p.isModerator(actorID, post.ChannelId) {
		return "", errors.New("you don't have permission to moderate this channel")
	}
	// The same outranking rules as user-targeted actions apply: moderators
	// can't delete messages written by admins or fellow moderators.
	if post.UserId != actorID {
		if err := p.checkCanActOn(actorID, post.UserId, post.ChannelId); err != nil {
			return "", err
		}
	}
	if err := p.client.Post.DeletePost(postID); err != nil {
		p.API.LogError("Failed to delete message", "post_id", postID, "error", err.Error())
		return "", errors.New("failed to delete message")
	}
	if post.UserId != actorID {
		p.dmFromBot(post.UserId, fmt.Sprintf(
			"A moderator removed your message in %s:\n> %s",
			p.channelLink(post.ChannelId), truncate(post.Message, maxExcerptLength),
		))
	}
	return post.ChannelId, nil
}

// banUser deactivates the member server-wide, resolves their open reports in
// the channel, and — depending on plugin settings — hides their recent
// messages in the channel where the ban was issued.
func (p *Plugin) banUser(actorID, channelID, targetID string) error {
	if !p.isAdmin(actorID, channelID) {
		return errors.New("only admins can ban members")
	}
	if err := p.checkCanModerateMember(actorID, targetID, channelID); err != nil {
		return err
	}
	target, err := p.client.User.Get(targetID)
	if err != nil {
		return errors.Wrap(err, "member not found")
	}
	if target.IsBot {
		return errors.New("bots can't be banned with this plugin")
	}
	if err := p.client.User.UpdateActive(targetID, false); err != nil {
		return errors.Wrap(err, "failed to deactivate member")
	}
	teamID := ""
	if channel, chErr := p.client.Channel.Get(channelID); chErr == nil {
		teamID = channel.TeamId
	}
	config := p.getConfiguration()
	ban := &kvstore.Ban{
		UserID:    targetID,
		ByUserID:  actorID,
		ChannelID: channelID,
		TeamID:    teamID,
		CreateAt:  p.now(),
	}
	if config.HideBannedIdentity {
		// Record the original profile before changing it, so an unban can
		// always restore it.
		ban.Identity = &kvstore.BannedIdentity{
			Username:  target.Username,
			FirstName: target.FirstName,
			LastName:  target.LastName,
			Nickname:  target.Nickname,
		}
	}
	if err := p.kvstore.SetBan(targetID, ban); err != nil {
		// Keep account state and ban record consistent: reactivate rather
		// than leave the member deactivated without a plugin ban record.
		if rbErr := p.client.User.UpdateActive(targetID, true); rbErr != nil {
			p.API.LogError("Failed to roll back deactivation after ban record failure", "user_id", targetID, "error", rbErr.Error())
		}
		return errors.Wrap(err, "failed to record ban")
	}

	if ban.Identity != nil {
		p.hideBannedIdentity(target, ban)
	}

	p.resolveOpenReportsForUser(channelID, targetID, "Member banned", actorID)
	p.publishMembersChanged(channelID)

	if config.HideBannedMessages {
		go p.hideMessagesFromUser(channelID, targetID, config.BannedMessageMode)
	}
	return nil
}

// unbanUser reverses a ban issued through the plugin. Accounts deactivated
// any other way are left alone — reactivating them is a System Console
// decision, not something a team admin should be able to do from a channel.
// The returned warning is set when the member's original username and name
// couldn't be restored.
func (p *Plugin) unbanUser(actorID, channelID, targetID string) (string, error) {
	if !p.isAdmin(actorID, channelID) {
		return "", errors.New("only admins can unban members")
	}
	ban, err := p.kvstore.GetBan(targetID)
	if err != nil {
		return "", err
	}
	if ban == nil {
		return "", errors.New("this member wasn't banned with the moderation tools, so they can only be reactivated from the System Console")
	}
	if !p.canUnban(actorID, ban) {
		return "", errors.New("only a system admin or an admin of the team where the ban was issued can lift it")
	}
	if err := p.client.User.UpdateActive(targetID, true); err != nil {
		return "", errors.Wrap(err, "failed to reactivate member")
	}
	if err := p.kvstore.DeleteBan(targetID); err != nil {
		// Keep account state and ban record consistent: deactivate again
		// rather than leave the member active while still recorded as banned.
		if rbErr := p.client.User.UpdateActive(targetID, false); rbErr != nil {
			p.API.LogError("Failed to roll back reactivation after unban record failure", "user_id", targetID, "error", rbErr.Error())
		}
		return "", errors.Wrap(err, "failed to clear ban record")
	}
	warning := ""
	if ban.Identity != nil {
		warning = p.restoreBannedIdentity(targetID, ban.Identity)
	}
	p.publishMembersChanged(channelID)
	return warning, nil
}

// bannedNickname is shown in place of a banned member's name.
const bannedNickname = "Banned member"

// hideBannedIdentity replaces a banned member's username and name with a
// placeholder, so an offensive or impersonating name stops showing up in
// messages, mentions, and member lists. Failures are logged rather than
// failing the ban, which has already taken effect.
func (p *Plugin) hideBannedIdentity(target *model.User, ban *kvstore.Ban) {
	// Prefer a short placeholder; the full user ID is the unique fallback.
	candidates := []string{"banned-" + target.Id[:8], "banned-" + target.Id}
	for _, username := range candidates {
		updated := target.DeepCopy()
		updated.Username = username
		updated.FirstName = ""
		updated.LastName = ""
		updated.Nickname = bannedNickname
		err := p.client.User.Update(updated)
		if err == nil {
			ban.Identity.PlaceholderUsername = username
			if saveErr := p.kvstore.SetBan(target.Id, ban); saveErr != nil {
				p.API.LogError("Failed to record banned member's placeholder username", "user_id", target.Id, "error", saveErr.Error())
			}
			return
		}
		p.API.LogWarn("Failed to replace banned member's username", "user_id", target.Id, "username", username, "error", err.Error())
	}
}

// restoreBannedIdentity puts back a member's original username and name
// after an unban, and returns a warning for the admin when it can't.
func (p *Plugin) restoreBannedIdentity(userID string, identity *kvstore.BannedIdentity) string {
	if identity.PlaceholderUsername == "" {
		// The profile was never changed.
		return ""
	}
	user, err := p.client.User.Get(userID)
	if err != nil {
		p.API.LogError("Failed to load member to restore their username", "user_id", userID, "error", err.Error())
		return fmt.Sprintf("their username couldn't be restored — it was @%s", identity.Username)
	}
	if user.Username != identity.PlaceholderUsername {
		// Someone changed the profile since the ban; don't overwrite that.
		return ""
	}
	user.Username = identity.Username
	user.FirstName = identity.FirstName
	user.LastName = identity.LastName
	user.Nickname = identity.Nickname
	if err := p.client.User.Update(user); err != nil {
		p.API.LogWarn("Failed to restore unbanned member's username", "user_id", userID, "username", identity.Username, "error", err.Error())
		return fmt.Sprintf("their original username @%s couldn't be restored, most likely because someone else has taken it — update it from the System Console", identity.Username)
	}
	return ""
}

// canUnban reports whether the actor may lift a ban: system admins always
// can, team admins only for bans issued in their own team.
func (p *Plugin) canUnban(actorID string, ban *kvstore.Ban) bool {
	if p.client.User.HasPermissionTo(actorID, model.PermissionManageSystem) {
		return true
	}
	return ban.TeamID != "" && p.client.User.HasPermissionToTeam(actorID, ban.TeamID, model.PermissionManageTeam)
}

func (p *Plugin) removeFromChannel(actorID, channelID, targetID string) error {
	if !p.isAdmin(actorID, channelID) {
		return errors.New("only admins can remove members from the channel")
	}
	if err := p.checkCanModerateMember(actorID, targetID, channelID); err != nil {
		return err
	}
	if err := p.client.Channel.DeleteMember(channelID, targetID); err != nil {
		return errors.Wrap(err, "failed to remove member from channel")
	}
	p.publishMembersChanged(channelID)
	p.dmFromBot(targetID, fmt.Sprintf(
		"You've been removed from %s by a moderator. You keep your account and your other channels.",
		p.channelLink(channelID),
	))
	return nil
}

func (p *Plugin) setModerator(actorID, channelID, targetID string, moderator bool) error {
	if !p.isAdmin(actorID, channelID) {
		return errors.New("only admins can assign moderators")
	}
	target, err := p.client.User.Get(targetID)
	if err != nil {
		return errors.Wrap(err, "member not found")
	}
	if target.IsBot {
		return errors.New("bots can't be moderators")
	}
	if moderator && !p.isChannelMember(channelID, targetID) {
		return errors.New("only members of this channel can be made moderators")
	}

	unlock, err := p.lockChannel(lockPrefixModerators, channelID)
	if err != nil {
		return err
	}
	moderators, err := p.kvstore.GetModerators(channelID)
	if err != nil {
		unlock()
		return err
	}
	updated := make([]string, 0, len(moderators)+1)
	for _, id := range moderators {
		if id != targetID {
			updated = append(updated, id)
		}
	}
	if moderator {
		updated = append(updated, targetID)
	}
	err = p.kvstore.SaveModerators(channelID, updated)
	unlock()
	if err != nil {
		return err
	}

	p.publishMembersChanged(channelID)
	if moderator {
		p.dmFromBot(targetID, fmt.Sprintf("You're now a moderator of %s. Open the Moderation panel in the channel header to work the report queue.", p.channelLink(channelID)))
	}
	return nil
}

// hideMessagesFromUser tombstones or deletes a banned member's recent
// messages in a channel, per the plugin's banned-message settings.
func (p *Plugin) hideMessagesFromUser(channelID, targetID, mode string) {
	perPage := 100
	for page := 0; page*perPage < tombstoneScanLimit; page++ {
		postList, err := p.client.Post.GetPostsForChannel(channelID, page, perPage)
		if err != nil {
			p.API.LogError("Failed to scan channel posts for ban cleanup", "channel_id", channelID, "error", err.Error())
			return
		}
		if postList == nil || len(postList.Order) == 0 {
			return
		}
		for _, postID := range postList.Order {
			post := postList.Posts[postID]
			// System messages ("joined the channel") can't be edited and
			// carry nothing the member wrote.
			if post == nil || post.UserId != targetID || post.DeleteAt != 0 || post.IsSystemMessage() {
				continue
			}
			if mode == BannedMessageModeRemove {
				if err := p.client.Post.DeletePost(post.Id); err != nil {
					p.API.LogError("Failed to delete banned member's post", "post_id", post.Id, "error", err.Error())
				}
				continue
			}
			tombstone := post.Clone()
			tombstone.Message = bannedMessagePlaceholder

			// Strip attachments, embeds, and other rich content so the
			// tombstone doesn't keep rendering the banned member's media.
			tombstone.FileIds = nil
			tombstone.Hashtags = ""
			tombstone.Metadata = nil
			tombstone.SetProps(model.StringInterface{tombstoneProp: true})
			if err := p.client.Post.UpdatePost(tombstone); err != nil {
				p.API.LogError("Failed to tombstone banned member's post", "post_id", post.Id, "error", err.Error())
			}
		}
		if len(postList.Order) < perPage {
			return
		}
	}
}

// MemberInfo is the members-tab view of one channel member.
type MemberInfo struct {
	UserID       string `json:"user_id"`
	Username     string `json:"username"`
	DisplayName  string `json:"display_name"`
	IsModerator  bool   `json:"is_moderator"`
	IsAdmin      bool   `json:"is_admin"`
	IsBanned     bool   `json:"is_banned"`
	Muted        bool   `json:"muted"`
	TimeoutUntil int64  `json:"timeout_until,omitempty"`
	TimeoutLabel string `json:"timeout_label,omitempty"`
}

// listMembers returns the members-tab view of a channel. Without a search
// term it lists only members who need attention — moderators, admins, and
// members who are banned, muted, or timed out. With a term it searches all
// channel members so admins can find anyone to elevate or manage.
func (p *Plugin) listMembers(channelID, term string) ([]*MemberInfo, error) {
	channel, err := p.client.Channel.Get(channelID)
	if err != nil {
		return nil, errors.Wrap(err, "channel not found")
	}
	var users []*model.User
	if term == "" {
		for page := range memberScanPages {
			batch, listErr := p.client.User.ListInChannel(channelID, model.ChannelSortByUsername, page, memberScanPageSize)
			if listErr != nil {
				return nil, errors.Wrap(listErr, "failed to list channel members")
			}
			users = append(users, batch...)
			if len(batch) < memberScanPageSize {
				break
			}
		}
	} else {
		users, err = p.client.User.Search(&model.UserSearch{
			Term:          term,
			InChannelId:   channelID,
			AllowInactive: true,
			Limit:         20,
		})
		if err != nil {
			return nil, errors.Wrap(err, "failed to search channel members")
		}
	}
	moderators, err := p.kvstore.GetModerators(channelID)
	if err != nil {
		return nil, err
	}
	moderatorSet := make(map[string]bool, len(moderators))
	for _, id := range moderators {
		moderatorSet[id] = true
	}
	channelAdminSet := map[string]bool{}
	for page := range memberScanPages {
		channelMembers, listErr := p.client.Channel.ListMembers(channelID, page, memberScanPageSize)
		if listErr != nil || len(channelMembers) == 0 {
			break
		}
		for _, member := range channelMembers {
			if member.SchemeAdmin {
				channelAdminSet[member.UserId] = true
			}
		}
		if len(channelMembers) < memberScanPageSize {
			break
		}
	}

	now := p.now()
	members := make([]*MemberInfo, 0, len(users))
	for _, user := range users {
		if user.IsBot {
			continue
		}
		isAdmin := p.client.User.HasPermissionTo(user.Id, model.PermissionManageSystem) ||
			(channel.TeamId != "" && p.client.User.HasPermissionToTeam(user.Id, channel.TeamId, model.PermissionManageTeam))
		info := &MemberInfo{
			UserID:      user.Id,
			Username:    user.Username,
			DisplayName: user.GetDisplayName(model.ShowNicknameFullName),
			IsModerator: isAdmin || moderatorSet[user.Id] || channelAdminSet[user.Id],
			IsAdmin:     isAdmin,
		}
		if ban, banErr := p.kvstore.GetBan(user.Id); banErr == nil && ban != nil {
			info.IsBanned = true
		}
		if restriction, rErr := p.kvstore.GetRestriction(channelID, user.Id); rErr == nil && restriction != nil {
			info.Muted = restriction.Muted
			if restriction.TimedOutNow(now) {
				info.TimeoutUntil = restriction.TimeoutUntil
				info.TimeoutLabel = restriction.TimeoutLabel
			}
		}
		// The unfiltered listing only surfaces members with an elevated role
		// or an active moderation flag; everyone else is found via search.
		if term == "" && !info.IsModerator && !info.IsBanned && !info.Muted && info.TimeoutUntil == 0 {
			continue
		}
		members = append(members, info)
	}
	return members, nil
}
