package main

import (
	"fmt"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/pkg/errors"

	"github.com/svelle/mattermost-plugin-moderation/server/store/kvstore"
)

const (
	maxExcerptLength = 300
	maxNoteLength    = 1000

	wsEventReportsChanged = "reports_changed"
	wsEventMembersChanged = "members_changed"
)

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-1]) + "…"
}

// createReport files a report from reporterID about targetID in a channel,
// optionally tied to a specific post, and notifies the moderation team.
func (p *Plugin) createReport(reporterID, channelID, postID, targetID, reasonID, note string) (*kvstore.Report, error) {
	reasonLabel, ok := kvstore.ReportReasons[reasonID]
	if !ok {
		return nil, errors.New("unknown report reason")
	}
	if reporterID == targetID {
		return nil, errors.New("you can't report yourself")
	}
	if _, err := p.client.User.Get(targetID); err != nil {
		return nil, errors.Wrap(err, "reported member not found")
	}

	excerpt := ""
	if postID != "" {
		post, err := p.client.Post.GetPost(postID)
		if err != nil {
			return nil, errors.Wrap(err, "reported message not found")
		}
		if post.ChannelId != channelID {
			return nil, errors.New("reported message is not in this channel")
		}
		if post.UserId != targetID {
			return nil, errors.New("reported message was not written by the reported member")
		}
		excerpt = truncate(post.Message, maxExcerptLength)
	}

	report := &kvstore.Report{
		ID:             model.NewId(),
		ChannelID:      channelID,
		PostID:         postID,
		TargetUserID:   targetID,
		ReporterUserID: reporterID,
		Reason:         reasonLabel,
		Note:           truncate(note, maxNoteLength),
		Excerpt:        excerpt,
		Status:         kvstore.ReportStatusOpen,
		CreateAt:       p.now(),
		UpdateAt:       p.now(),
	}

	p.reportsLock.Lock()
	defer p.reportsLock.Unlock()
	reports, err := p.kvstore.GetReports(channelID)
	if err != nil {
		return nil, err
	}
	reports = append([]*kvstore.Report{report}, reports...)
	if err := p.kvstore.SaveReports(channelID, reports); err != nil {
		return nil, err
	}

	p.publishReportsChanged(channelID, reports)
	p.notifyModeratorsOfReport(report)

	return report, nil
}

// updateReportStatus moves a report to a new status with a human-readable
// resolution such as "Dismissed by @jess".
func (p *Plugin) updateReportStatus(channelID, reportID, status, resolution, actorID string) (*kvstore.Report, error) {
	p.reportsLock.Lock()
	defer p.reportsLock.Unlock()

	reports, err := p.kvstore.GetReports(channelID)
	if err != nil {
		return nil, err
	}
	var updated *kvstore.Report
	for _, r := range reports {
		if r.ID == reportID {
			r.Status = status
			r.Resolution = resolution
			r.ResolvedByID = actorID
			r.UpdateAt = p.now()
			updated = r
			break
		}
	}
	if updated == nil {
		return nil, errors.New("report not found")
	}
	if err := p.kvstore.SaveReports(channelID, reports); err != nil {
		return nil, err
	}
	p.publishReportsChanged(channelID, reports)
	return updated, nil
}

// resolveOpenReportsForUser resolves every open or escalated report against a
// member in a channel, used when a moderation action settles the matter.
func (p *Plugin) resolveOpenReportsForUser(channelID, targetID, resolution, actorID string) {
	p.reportsLock.Lock()
	defer p.reportsLock.Unlock()

	reports, err := p.kvstore.GetReports(channelID)
	if err != nil {
		p.API.LogError("Failed to get reports", "channel_id", channelID, "error", err.Error())
		return
	}
	changed := false
	for _, r := range reports {
		if r.TargetUserID == targetID && r.Status != kvstore.ReportStatusResolved {
			r.Status = kvstore.ReportStatusResolved
			r.Resolution = resolution
			r.ResolvedByID = actorID
			r.UpdateAt = p.now()
			changed = true
		}
	}
	if !changed {
		return
	}
	if err := p.kvstore.SaveReports(channelID, reports); err != nil {
		p.API.LogError("Failed to save reports", "channel_id", channelID, "error", err.Error())
		return
	}
	p.publishReportsChanged(channelID, reports)
}

// escalateReportsForUser marks a member's open reports as escalated and
// notifies the admins.
func (p *Plugin) escalateReportsForUser(channelID, targetID, actorID string) error {
	p.reportsLock.Lock()
	reports, err := p.kvstore.GetReports(channelID)
	if err != nil {
		p.reportsLock.Unlock()
		return err
	}
	for _, r := range reports {
		if r.TargetUserID == targetID && r.Status == kvstore.ReportStatusOpen {
			r.Status = kvstore.ReportStatusEscalated
			r.UpdateAt = p.now()
		}
	}
	if err := p.kvstore.SaveReports(channelID, reports); err != nil {
		p.reportsLock.Unlock()
		return err
	}
	p.publishReportsChanged(channelID, reports)
	p.reportsLock.Unlock()

	p.notifyAdminsOfEscalation(channelID, targetID, actorID)
	return nil
}

func openReportCount(reports []*kvstore.Report) int {
	count := 0
	for _, r := range reports {
		if r.Status != kvstore.ReportStatusResolved {
			count++
		}
	}
	return count
}

func (p *Plugin) publishReportsChanged(channelID string, reports []*kvstore.Report) {
	p.client.Frontend.PublishWebSocketEvent(wsEventReportsChanged, map[string]any{
		"channel_id": channelID,
		"open_count": openReportCount(reports),
	}, &model.WebsocketBroadcast{ChannelId: channelID})
}

func (p *Plugin) publishMembersChanged(channelID string) {
	p.client.Frontend.PublishWebSocketEvent(wsEventMembersChanged, map[string]any{
		"channel_id": channelID,
	}, &model.WebsocketBroadcast{ChannelId: channelID})
}

func (p *Plugin) channelLink(channelID string) string {
	channel, err := p.client.Channel.Get(channelID)
	if err != nil {
		return "the channel"
	}
	switch channel.Type {
	case model.ChannelTypeOpen, model.ChannelTypePrivate:
		return fmt.Sprintf("~%s", channel.Name)
	default:
		return "the conversation"
	}
}

func (p *Plugin) dmFromBot(userID, message string) {
	err := p.client.Post.DM(p.botUserID, userID, &model.Post{Message: message})
	if err != nil {
		p.API.LogError("Failed to send moderation DM", "user_id", userID, "error", err.Error())
	}
}

// notifyModeratorsOfReport DMs the channel's moderators about a new report.
// If the channel has no assigned moderators, system admins are notified so
// reports never go unseen.
func (p *Plugin) notifyModeratorsOfReport(report *kvstore.Report) {
	target, err := p.client.User.Get(report.TargetUserID)
	if err != nil {
		p.API.LogError("Failed to get reported user", "user_id", report.TargetUserID, "error", err.Error())
		return
	}

	recipients, err := p.kvstore.GetModerators(report.ChannelID)
	if err != nil {
		p.API.LogError("Failed to get moderators", "channel_id", report.ChannelID, "error", err.Error())
	}
	if len(recipients) == 0 {
		recipients = p.systemAdminIDs()
	}

	message := fmt.Sprintf(
		"New report in %s: @%s was reported for **%s**. Open the Moderation panel in the channel to review it.",
		p.channelLink(report.ChannelID), target.Username, report.Reason,
	)
	for _, id := range recipients {
		if id == report.ReporterUserID {
			continue
		}
		p.dmFromBot(id, message)
	}
}

func (p *Plugin) notifyAdminsOfEscalation(channelID, targetID, actorID string) {
	target, err := p.client.User.Get(targetID)
	if err != nil {
		return
	}
	actor, err := p.client.User.Get(actorID)
	if err != nil {
		return
	}
	message := fmt.Sprintf(
		"@%s escalated reports about @%s in %s. Open the Moderation panel in the channel to review them.",
		actor.Username, target.Username, p.channelLink(channelID),
	)
	for _, id := range p.systemAdminIDs() {
		if id == actorID {
			continue
		}
		p.dmFromBot(id, message)
	}
}

func (p *Plugin) systemAdminIDs() []string {
	admins, err := p.client.User.List(&model.UserGetOptions{
		Role:    model.SystemAdminRoleId,
		Page:    0,
		PerPage: 50,
	})
	if err != nil {
		p.API.LogError("Failed to list system admins", "error", err.Error())
		return nil
	}
	ids := make([]string, 0, len(admins))
	for _, admin := range admins {
		if admin.IsBot {
			continue
		}
		ids = append(ids, admin.Id)
	}
	return ids
}
