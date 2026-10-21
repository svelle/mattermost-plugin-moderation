package main

import (
	"fmt"
	"slices"
	"time"

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

	// Message reports are scoped by the message itself; member reports need
	// the member to be in the channel the report is filed in.
	if postID == "" && !p.isChannelMember(channelID, targetID) {
		return nil, errors.New("that person isn't a member of this channel")
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

	unlock, err := p.lockChannel(lockPrefixReports, channelID)
	if err != nil {
		return nil, err
	}
	defer unlock()
	reports, err := p.kvstore.GetReports(channelID)
	if err != nil {
		return nil, err
	}
	if hasDuplicateReport(reports, reporterID, targetID, postID) {
		return nil, errors.New("you've already reported this — it's in the moderation team's queue")
	}
	if err := p.checkReportRateLimit(reporterID); err != nil {
		return nil, err
	}
	reports = capReports(append([]*kvstore.Report{report}, reports...))
	if err := p.kvstore.SaveReports(channelID, reports); err != nil {
		return nil, err
	}

	p.publishReportsChanged(channelID, reports)
	p.notifyModeratorsOfReport(report)

	return report, nil
}

// maxStoredReports caps how many reports are kept per channel so the KV
// value doesn't grow unbounded; the oldest resolved reports are dropped
// first, and open reports are only dropped once nothing resolved is left.
const maxStoredReports = 200

func capReports(reports []*kvstore.Report) []*kvstore.Report {
	over := len(reports) - maxStoredReports
	if over <= 0 {
		return reports
	}
	drop := make(map[int]bool, over)
	// Reports are newest-first, so walk from the end (oldest) dropping
	// resolved ones, then oldest of any status if still over the cap.
	for i := len(reports) - 1; i >= 0 && over > 0; i-- {
		if reports[i].Status == kvstore.ReportStatusResolved {
			drop[i] = true
			over--
		}
	}
	for i := len(reports) - 1; i >= 0 && over > 0; i-- {
		if !drop[i] {
			drop[i] = true
			over--
		}
	}
	kept := make([]*kvstore.Report, 0, maxStoredReports)
	for i, r := range reports {
		if !drop[i] {
			kept = append(kept, r)
		}
	}
	return kept
}

// hasDuplicateReport reports whether the reporter already has an unresolved
// report for the same message — or, for member reports, the same member —
// so repeat submissions don't pile up in the queue.
func hasDuplicateReport(reports []*kvstore.Report, reporterID, targetID, postID string) bool {
	for _, r := range reports {
		if r.ReporterUserID != reporterID || r.TargetUserID != targetID || r.Status == kvstore.ReportStatusResolved {
			continue
		}
		if r.PostID == postID {
			return true
		}
	}
	return false
}

// pruneStamps keeps only timestamps at or after the cutoff.
func pruneStamps(stamps []int64, cutoff int64) []int64 {
	recent := make([]int64, 0, len(stamps))
	for _, stamp := range stamps {
		if stamp >= cutoff {
			recent = append(recent, stamp)
		}
	}
	return recent
}

// checkReportRateLimit enforces a rolling per-member cap on report
// submissions and records the new submission when it's allowed.
func (p *Plugin) checkReportRateLimit(reporterID string) error {
	config := p.getConfiguration()
	if config.ReportRateLimit <= 0 {
		return nil
	}
	stamps, err := p.kvstore.GetReportStamps(reporterID)
	if err != nil {
		return err
	}
	window := time.Duration(config.ReportRateWindowMinutes) * time.Minute
	recent := pruneStamps(stamps, p.now()-window.Milliseconds())
	if len(recent) >= config.ReportRateLimit {
		return errors.Errorf(
			"you've submitted %d reports in the last %d minutes, which is the limit — please give the moderation team a moment to catch up",
			len(recent), config.ReportRateWindowMinutes,
		)
	}
	recent = append(recent, p.now())
	return p.kvstore.SaveReportStamps(reporterID, recent)
}

// updateReportStatus moves a report to a new status with a human-readable
// resolution such as "Dismissed by @jess".
func (p *Plugin) updateReportStatus(channelID, reportID, status, resolution, actorID string) (*kvstore.Report, error) {
	unlock, err := p.lockChannel(lockPrefixReports, channelID)
	if err != nil {
		return nil, err
	}
	defer unlock()

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
	p.resolveReports(channelID, resolution, actorID, func(r *kvstore.Report) bool {
		return r.TargetUserID == targetID
	})
}

// resolveOpenReportsForUserEverywhere resolves every open or escalated report
// against a member in any channel or conversation, used when a ban settles
// the matter server-wide. issuedChannelID, where the ban was issued, is
// always covered even if the reports can't be listed.
func (p *Plugin) resolveOpenReportsForUserEverywhere(issuedChannelID, targetID, resolution, actorID string) {
	channelIDs, err := p.reportChannelIDs()
	if err != nil {
		p.API.LogError("Failed to list report queues", "error", err.Error())
	}
	if !slices.Contains(channelIDs, issuedChannelID) {
		channelIDs = append(channelIDs, issuedChannelID)
	}
	for _, channelID := range channelIDs {
		// Check without the lock first, so only queues with reports about
		// this member are locked and rewritten.
		reports, getErr := p.kvstore.GetReports(channelID)
		if getErr != nil {
			p.API.LogError("Failed to get reports", "channel_id", channelID, "error", getErr.Error())
			continue
		}
		if slices.ContainsFunc(reports, func(r *kvstore.Report) bool {
			return r.TargetUserID == targetID && r.Status != kvstore.ReportStatusResolved
		}) {
			p.resolveOpenReportsForUser(channelID, targetID, resolution, actorID)
		}
	}
}

// reportChannelIDs lists every channel and conversation with a report queue.
func (p *Plugin) reportChannelIDs() ([]string, error) {
	const perPage = 1000
	var ids []string
	for page := 0; ; page++ {
		keys, err := p.kvstore.ListKeys(page, perPage)
		if err != nil {
			return ids, err
		}
		for _, key := range keys {
			if channelID, ok := kvstore.ParseReportsKey(key); ok {
				ids = append(ids, channelID)
			}
		}
		if len(keys) < perPage {
			return ids, nil
		}
	}
}

// resolveOpenReportsForPost resolves every open or escalated report about a
// specific message, used when the message is deleted.
func (p *Plugin) resolveOpenReportsForPost(channelID, postID, resolution, actorID string) {
	p.resolveReports(channelID, resolution, actorID, func(r *kvstore.Report) bool {
		return r.PostID == postID
	})
}

// resolveReports resolves the unresolved reports in a channel that match.
func (p *Plugin) resolveReports(channelID, resolution, actorID string, match func(*kvstore.Report) bool) {
	unlock, err := p.lockChannel(lockPrefixReports, channelID)
	if err != nil {
		p.API.LogError("Failed to lock reports", "channel_id", channelID, "error", err.Error())
		return
	}
	defer unlock()

	reports, err := p.kvstore.GetReports(channelID)
	if err != nil {
		p.API.LogError("Failed to get reports", "channel_id", channelID, "error", err.Error())
		return
	}
	changed := false
	for _, r := range reports {
		if r.Status != kvstore.ReportStatusResolved && match(r) {
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
	unlock, err := p.lockChannel(lockPrefixReports, channelID)
	if err != nil {
		return err
	}
	reports, err := p.kvstore.GetReports(channelID)
	if err != nil {
		unlock()
		return err
	}
	escalated, alreadyEscalated := 0, 0
	for _, r := range reports {
		if r.TargetUserID != targetID {
			continue
		}
		switch r.Status {
		case kvstore.ReportStatusOpen:
			r.Status = kvstore.ReportStatusEscalated
			r.UpdateAt = p.now()
			escalated++
		case kvstore.ReportStatusEscalated:
			alreadyEscalated++
		}
	}
	// Only notify admins about new escalations, so repeated clicks can't
	// spam every system admin.
	if escalated == 0 {
		unlock()
		if alreadyEscalated > 0 {
			return errors.New("reports about this member are already escalated and waiting on an admin")
		}
		return errors.New("there are no open reports about this member to escalate — report the message first")
	}
	if err := p.kvstore.SaveReports(channelID, reports); err != nil {
		unlock()
		return err
	}
	p.publishReportsChanged(channelID, reports)
	unlock()

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
	const perPage = 100
	var ids []string
	for page := 0; ; page++ {
		admins, err := p.client.User.List(&model.UserGetOptions{
			Role:    model.SystemAdminRoleId,
			Page:    page,
			PerPage: perPage,
		})
		if err != nil {
			p.API.LogError("Failed to list system admins", "error", err.Error())
			return ids
		}
		for _, admin := range admins {
			if !admin.IsBot {
				ids = append(ids, admin.Id)
			}
		}
		if len(admins) < perPage {
			return ids
		}
	}
}
