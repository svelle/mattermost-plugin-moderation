package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/svelle/mattermost-plugin-moderation/server/store/kvstore"
)

func TestServeHTTPAuthorizationRequired(t *testing.T) {
	assert := assert.New(t)
	plugin := Plugin{}
	plugin.router = plugin.initRouter()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/reports", nil)

	plugin.ServeHTTP(nil, w, r)

	result := w.Result()
	assert.NotNil(result)
	defer func() { _ = result.Body.Close() }()
	assert.Equal(http.StatusUnauthorized, result.StatusCode)
}

func TestRemainingLabel(t *testing.T) {
	assert := assert.New(t)
	assert.Equal("5 minutes", remainingLabel(5*60*1000))
	assert.Equal("1 minute", remainingLabel(30*1000))
	assert.Equal("1 hour", remainingLabel(60*60*1000))
	assert.Equal("24 hours", remainingLabel(24*60*60*1000))
	assert.Equal("7 days", remainingLabel(7*24*60*60*1000))
}

func TestCapReports(t *testing.T) {
	assert := assert.New(t)

	small := []*kvstore.Report{{ID: "a"}, {ID: "b"}}
	assert.Equal(small, capReports(small))

	// Newest-first list one over the cap: the oldest resolved goes first.
	reports := make([]*kvstore.Report, 0, maxStoredReports+1)
	for i := 0; i <= maxStoredReports; i++ {
		status := kvstore.ReportStatusOpen
		if i == 10 || i == maxStoredReports {
			status = kvstore.ReportStatusResolved
		}
		reports = append(reports, &kvstore.Report{ID: string(rune('a' + (i % 26))), Status: status, CreateAt: int64(-i)})
	}
	capped := capReports(reports)
	assert.Len(capped, maxStoredReports)
	assert.Equal(reports[:maxStoredReports], capped, "the oldest resolved report should be dropped")

	// All open: the oldest reports are dropped once nothing resolved is left.
	allOpen := make([]*kvstore.Report, 0, maxStoredReports+5)
	for i := range maxStoredReports + 5 {
		allOpen = append(allOpen, &kvstore.Report{Status: kvstore.ReportStatusOpen, CreateAt: int64(-i)})
	}
	cappedOpen := capReports(allOpen)
	assert.Len(cappedOpen, maxStoredReports)
	assert.Equal(allOpen[:maxStoredReports], cappedOpen)
}

func TestTruncate(t *testing.T) {
	assert := assert.New(t)
	assert.Equal("short", truncate("short", 10))
	assert.Equal("aaaa…", truncate("aaaaaaaaaa", 5))
}

func TestPruneStamps(t *testing.T) {
	assert := assert.New(t)
	assert.Empty(pruneStamps(nil, 100))
	assert.Equal([]int64{100, 150}, pruneStamps([]int64{50, 99, 100, 150}, 100))
	assert.Empty(pruneStamps([]int64{1, 2, 3}, 100))
}

func TestHasDuplicateReport(t *testing.T) {
	assert := assert.New(t)
	reports := []*kvstore.Report{
		{ReporterUserID: "alice", TargetUserID: "rex", PostID: "post1", Status: kvstore.ReportStatusOpen},
		{ReporterUserID: "alice", TargetUserID: "sam", PostID: "", Status: kvstore.ReportStatusResolved},
		{ReporterUserID: "bob", TargetUserID: "rex", PostID: "", Status: kvstore.ReportStatusOpen},
	}

	// Same reporter, same post: duplicate.
	assert.True(hasDuplicateReport(reports, "alice", "rex", "post1"))

	// Same reporter, different post or a member report: allowed.
	assert.False(hasDuplicateReport(reports, "alice", "rex", "post2"))
	assert.False(hasDuplicateReport(reports, "alice", "rex", ""))

	// Resolved reports don't block a new one.
	assert.False(hasDuplicateReport(reports, "alice", "sam", ""))

	// Different reporters don't collide.
	assert.False(hasDuplicateReport(reports, "alice", "bob", "post1"))
	assert.True(hasDuplicateReport(reports, "bob", "rex", ""))
}
