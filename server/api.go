package main

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/svelle/mattermost-plugin-moderation/server/store/kvstore"
)

// initRouter initializes the HTTP router for the plugin.
func (p *Plugin) initRouter() *mux.Router {
	router := mux.NewRouter()

	// Middleware to require that the user is logged in
	router.Use(p.MattermostAuthorizationRequired)

	apiRouter := router.PathPrefix("/api/v1").Subrouter()

	apiRouter.HandleFunc("/channel/{channel_id:[a-z0-9]+}/status", p.handleStatus).Methods(http.MethodGet)
	apiRouter.HandleFunc("/channel/{channel_id:[a-z0-9]+}/reports", p.handleListReports).Methods(http.MethodGet)
	apiRouter.HandleFunc("/channel/{channel_id:[a-z0-9]+}/members", p.handleListMembers).Methods(http.MethodGet)
	apiRouter.HandleFunc("/channel/{channel_id:[a-z0-9]+}/moderators", p.handleSetModerator).Methods(http.MethodPost)

	apiRouter.HandleFunc("/reports", p.handleCreateReport).Methods(http.MethodPost)
	apiRouter.HandleFunc("/reports/{report_id:[a-z0-9]+}/dismiss", p.handleDismissReport).Methods(http.MethodPost)

	apiRouter.HandleFunc("/actions/mute", p.handleMute).Methods(http.MethodPost)
	apiRouter.HandleFunc("/actions/timeout", p.handleTimeout).Methods(http.MethodPost)
	apiRouter.HandleFunc("/actions/lift", p.handleLiftRestrictions).Methods(http.MethodPost)
	apiRouter.HandleFunc("/actions/warn", p.handleWarn).Methods(http.MethodPost)
	apiRouter.HandleFunc("/actions/escalate", p.handleEscalate).Methods(http.MethodPost)
	apiRouter.HandleFunc("/actions/delete_post", p.handleDeletePost).Methods(http.MethodPost)
	apiRouter.HandleFunc("/actions/ban", p.handleBan).Methods(http.MethodPost)
	apiRouter.HandleFunc("/actions/unban", p.handleUnban).Methods(http.MethodPost)
	apiRouter.HandleFunc("/actions/remove", p.handleRemoveFromChannel).Methods(http.MethodPost)

	return router
}

// ServeHTTP handles HTTP requests to the plugin.
// The root URL is <siteUrl>/plugins/com.mattermost.community-moderation/api/v1/.
func (p *Plugin) ServeHTTP(_ *plugin.Context, w http.ResponseWriter, r *http.Request) {
	p.router.ServeHTTP(w, r)
}

func (p *Plugin) MattermostAuthorizationRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := r.Header.Get("Mattermost-User-ID")
		if userID == "" {
			http.Error(w, "Not authorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func requestUserID(r *http.Request) string {
	return r.Header.Get("Mattermost-User-ID")
}

func (p *Plugin) writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		p.API.LogError("Failed to write response", "error", err.Error())
	}
}

func (p *Plugin) writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// "message" is included so the webapp's ClientError surfaces the text.
	if err := json.NewEncoder(w).Encode(map[string]string{"error": message, "message": message}); err != nil {
		p.API.LogError("Failed to write error response", "error", err.Error())
	}
}

// requireChannelMember checks that the user is a member of the channel and
// writes an error response when they aren't.
func (p *Plugin) requireChannelMember(w http.ResponseWriter, userID, channelID string) bool {
	if _, err := p.client.Channel.GetMember(channelID, userID); err != nil {
		p.writeError(w, http.StatusForbidden, "You're not a member of this channel.")
		return false
	}
	return true
}

func (p *Plugin) requireModerator(w http.ResponseWriter, userID, channelID string) bool {
	if !p.isModerator(userID, channelID) {
		p.writeError(w, http.StatusForbidden, "You don't have permission to moderate this channel.")
		return false
	}
	return true
}

type statusResponse struct {
	IsModerator     bool `json:"is_moderator"`
	IsAdmin         bool `json:"is_admin"`
	OpenReportCount int  `json:"open_report_count"`
}

func (p *Plugin) handleStatus(w http.ResponseWriter, r *http.Request) {
	userID := requestUserID(r)
	channelID := mux.Vars(r)["channel_id"]
	if !p.requireChannelMember(w, userID, channelID) {
		return
	}

	status := statusResponse{
		IsModerator: p.isModerator(userID, channelID),
		IsAdmin:     p.isAdmin(userID, channelID),
	}
	if status.IsModerator {
		reports, err := p.kvstore.GetReports(channelID)
		if err != nil {
			p.writeError(w, http.StatusInternalServerError, "Failed to load reports.")
			return
		}
		status.OpenReportCount = openReportCount(reports)
	}
	p.writeJSON(w, status)
}

// reportView decorates a report with display names for the webapp.
type reportView struct {
	*kvstore.Report
	TargetUsername    string `json:"target_username"`
	TargetDisplayName string `json:"target_display_name"`
	ReporterName      string `json:"reporter_name"`
	ResolvedByName    string `json:"resolved_by_name,omitempty"`
}

func (p *Plugin) displayName(userID string, cache map[string][2]string) (username, display string) {
	if names, ok := cache[userID]; ok {
		return names[0], names[1]
	}
	user, err := p.client.User.Get(userID)
	if err != nil {
		cache[userID] = [2]string{"unknown", "Unknown member"}
	} else {
		cache[userID] = [2]string{user.Username, user.GetDisplayName("full_name_nickname")}
	}
	return cache[userID][0], cache[userID][1]
}

func (p *Plugin) handleListReports(w http.ResponseWriter, r *http.Request) {
	userID := requestUserID(r)
	channelID := mux.Vars(r)["channel_id"]
	if !p.requireModerator(w, userID, channelID) {
		return
	}
	reports, err := p.kvstore.GetReports(channelID)
	if err != nil {
		p.writeError(w, http.StatusInternalServerError, "Failed to load reports.")
		return
	}

	nameCache := map[string][2]string{}
	views := make([]*reportView, 0, len(reports))
	for _, report := range reports {
		view := &reportView{Report: report}
		view.TargetUsername, view.TargetDisplayName = p.displayName(report.TargetUserID, nameCache)
		_, view.ReporterName = p.displayName(report.ReporterUserID, nameCache)
		if report.ResolvedByID != "" {
			_, view.ResolvedByName = p.displayName(report.ResolvedByID, nameCache)
		}
		views = append(views, view)
	}
	p.writeJSON(w, views)
}

func (p *Plugin) handleListMembers(w http.ResponseWriter, r *http.Request) {
	userID := requestUserID(r)
	channelID := mux.Vars(r)["channel_id"]
	if !p.isAdmin(userID, channelID) {
		p.writeError(w, http.StatusForbidden, "Only admins can manage members.")
		return
	}
	members, err := p.listMembers(channelID, r.URL.Query().Get("q"))
	if err != nil {
		p.writeError(w, http.StatusInternalServerError, "Failed to load members.")
		return
	}
	p.writeJSON(w, members)
}

type createReportRequest struct {
	ChannelID    string `json:"channel_id"`
	PostID       string `json:"post_id"`
	TargetUserID string `json:"target_user_id"`
	Reason       string `json:"reason"`
	Note         string `json:"note"`
}

func (p *Plugin) handleCreateReport(w http.ResponseWriter, r *http.Request) {
	userID := requestUserID(r)
	var req createReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		p.writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	if !p.requireChannelMember(w, userID, req.ChannelID) {
		return
	}
	report, err := p.createReport(userID, req.ChannelID, req.PostID, req.TargetUserID, req.Reason, req.Note)
	if err != nil {
		p.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p.writeJSON(w, report)
}

type reportActionRequest struct {
	ChannelID string `json:"channel_id"`
}

func (p *Plugin) handleDismissReport(w http.ResponseWriter, r *http.Request) {
	userID := requestUserID(r)
	reportID := mux.Vars(r)["report_id"]
	var req reportActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		p.writeError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	if !p.requireModerator(w, userID, req.ChannelID) {
		return
	}
	_, actorName := p.displayName(userID, map[string][2]string{})
	report, err := p.updateReportStatus(req.ChannelID, reportID, kvstore.ReportStatusResolved, "Dismissed by "+actorName, userID)
	if err != nil {
		p.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p.writeJSON(w, report)
}

type memberActionRequest struct {
	ChannelID string `json:"channel_id"`
	UserID    string `json:"user_id"`
	PostID    string `json:"post_id"`
	ReportID  string `json:"report_id"`
	Duration  string `json:"duration"`
	Moderator bool   `json:"moderator"`
}

func (p *Plugin) decodeMemberAction(w http.ResponseWriter, r *http.Request) (*memberActionRequest, bool) {
	var req memberActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		p.writeError(w, http.StatusBadRequest, "Invalid request.")
		return nil, false
	}
	return &req, true
}

// resolveActionedReport resolves the report a queue action was taken from,
// or every open report against the member when the action came from a
// message menu or the members tab.
func (p *Plugin) resolveActionedReport(req *memberActionRequest, actorID, resolution string) {
	if req.ReportID != "" {
		if _, err := p.updateReportStatus(req.ChannelID, req.ReportID, kvstore.ReportStatusResolved, resolution, actorID); err != nil {
			p.API.LogError("Failed to resolve report", "report_id", req.ReportID, "error", err.Error())
		}
	}
}

func (p *Plugin) handleMute(w http.ResponseWriter, r *http.Request) {
	userID := requestUserID(r)
	req, ok := p.decodeMemberAction(w, r)
	if !ok {
		return
	}
	if err := p.muteUser(userID, req.ChannelID, req.UserID); err != nil {
		p.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p.resolveActionedReport(req, userID, "Muted by moderator")
	p.writeJSON(w, map[string]string{"status": "ok"})
}

func (p *Plugin) handleTimeout(w http.ResponseWriter, r *http.Request) {
	userID := requestUserID(r)
	req, ok := p.decodeMemberAction(w, r)
	if !ok {
		return
	}
	label, err := p.timeoutUser(userID, req.ChannelID, req.UserID, req.Duration)
	if err != nil {
		p.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p.resolveActionedReport(req, userID, "Timed out for "+label)
	p.writeJSON(w, map[string]string{"status": "ok", "label": label})
}

func (p *Plugin) handleLiftRestrictions(w http.ResponseWriter, r *http.Request) {
	userID := requestUserID(r)
	req, ok := p.decodeMemberAction(w, r)
	if !ok {
		return
	}
	if err := p.liftRestrictions(userID, req.ChannelID, req.UserID); err != nil {
		p.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p.writeJSON(w, map[string]string{"status": "ok"})
}

func (p *Plugin) handleWarn(w http.ResponseWriter, r *http.Request) {
	userID := requestUserID(r)
	req, ok := p.decodeMemberAction(w, r)
	if !ok {
		return
	}
	if err := p.warnUser(userID, req.ChannelID, req.UserID); err != nil {
		p.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p.resolveActionedReport(req, userID, "Warning sent")
	p.writeJSON(w, map[string]string{"status": "ok"})
}

func (p *Plugin) handleEscalate(w http.ResponseWriter, r *http.Request) {
	userID := requestUserID(r)
	req, ok := p.decodeMemberAction(w, r)
	if !ok {
		return
	}
	if !p.requireModerator(w, userID, req.ChannelID) {
		return
	}
	if err := p.escalateReportsForUser(req.ChannelID, req.UserID, userID); err != nil {
		p.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p.writeJSON(w, map[string]string{"status": "ok"})
}

func (p *Plugin) handleDeletePost(w http.ResponseWriter, r *http.Request) {
	userID := requestUserID(r)
	req, ok := p.decodeMemberAction(w, r)
	if !ok {
		return
	}
	if err := p.deletePostAsModerator(userID, req.PostID); err != nil {
		p.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p.resolveActionedReport(req, userID, "Message deleted")
	p.writeJSON(w, map[string]string{"status": "ok"})
}

func (p *Plugin) handleBan(w http.ResponseWriter, r *http.Request) {
	userID := requestUserID(r)
	req, ok := p.decodeMemberAction(w, r)
	if !ok {
		return
	}
	if err := p.banUser(userID, req.ChannelID, req.UserID); err != nil {
		p.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p.writeJSON(w, map[string]string{"status": "ok"})
}

func (p *Plugin) handleUnban(w http.ResponseWriter, r *http.Request) {
	userID := requestUserID(r)
	req, ok := p.decodeMemberAction(w, r)
	if !ok {
		return
	}
	if err := p.unbanUser(userID, req.ChannelID, req.UserID); err != nil {
		p.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p.writeJSON(w, map[string]string{"status": "ok"})
}

func (p *Plugin) handleRemoveFromChannel(w http.ResponseWriter, r *http.Request) {
	userID := requestUserID(r)
	req, ok := p.decodeMemberAction(w, r)
	if !ok {
		return
	}
	if err := p.removeFromChannel(userID, req.ChannelID, req.UserID); err != nil {
		p.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p.writeJSON(w, map[string]string{"status": "ok"})
}

func (p *Plugin) handleSetModerator(w http.ResponseWriter, r *http.Request) {
	userID := requestUserID(r)
	channelID := mux.Vars(r)["channel_id"]
	req, ok := p.decodeMemberAction(w, r)
	if !ok {
		return
	}
	if err := p.setModerator(userID, channelID, req.UserID, req.Moderator); err != nil {
		p.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p.writeJSON(w, map[string]string{"status": "ok"})
}
