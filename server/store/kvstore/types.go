package kvstore

// Report statuses.
const (
	ReportStatusOpen      = "open"
	ReportStatusEscalated = "escalated"
	ReportStatusResolved  = "resolved"
)

// Report reasons. These match the options offered in the report modal.
var ReportReasons = map[string]string{
	"spam":   "Spam or scam",
	"harass": "Harassment or abuse",
	"imp":    "Impersonation",
	"nsfw":   "Inappropriate content",
	"other":  "Something else",
}

// Report is a member-submitted report about a message or another member.
type Report struct {
	ID             string `json:"id"`
	ChannelID      string `json:"channel_id"`
	PostID         string `json:"post_id,omitempty"`
	TargetUserID   string `json:"target_user_id"`
	ReporterUserID string `json:"reporter_user_id"`
	Reason         string `json:"reason"`
	Note           string `json:"note,omitempty"`
	Excerpt        string `json:"excerpt,omitempty"`
	Status         string `json:"status"`
	Resolution     string `json:"resolution,omitempty"`
	ResolvedByID   string `json:"resolved_by_id,omitempty"`
	CreateAt       int64  `json:"create_at"`
	UpdateAt       int64  `json:"update_at"`
}

// Restriction limits what a member can do in a single channel. A muted
// member can't post or react until the mute is lifted; a timed-out member
// can't post or react until TimeoutUntil (milliseconds since epoch).
type Restriction struct {
	Muted        bool   `json:"muted"`
	TimeoutUntil int64  `json:"timeout_until,omitempty"`
	TimeoutLabel string `json:"timeout_label,omitempty"`
	ByUserID     string `json:"by_user_id,omitempty"`
	CreateAt     int64  `json:"create_at,omitempty"`
}

// TimedOutNow reports whether the timeout is still active at the given time.
func (r *Restriction) TimedOutNow(nowMillis int64) bool {
	return r != nil && r.TimeoutUntil > nowMillis
}

// Active reports whether the restriction still limits the member.
func (r *Restriction) Active(nowMillis int64) bool {
	return r != nil && (r.Muted || r.TimedOutNow(nowMillis))
}

// Ban is a server-wide ban. Banned members are deactivated and can't sign in.
type Ban struct {
	UserID   string `json:"user_id"`
	ByUserID string `json:"by_user_id"`
	CreateAt int64  `json:"create_at"`
}
