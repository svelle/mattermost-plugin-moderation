package main

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/svelle/mattermost-plugin-moderation/server/store/kvstore"
)

const lookback = 72 * time.Hour

func postList(posts ...*model.Post) *model.PostList {
	list := model.NewPostList()
	for _, post := range posts {
		list.AddPost(post)
		list.AddOrder(post.Id)
	}
	return list
}

// mockCleanupPosts serves the banned member's posts: an old one in the
// channel where the ban was issued, and a mix in their direct message.
func mockCleanupPosts(api *plugintest.API) (since int64, updated func() []string) {
	now := model.GetMillis()
	since = now - lookback.Milliseconds()
	old := now - (10 * 24 * time.Hour).Milliseconds()

	api.On("GetPostsForChannel", testChannelID, 0, sweepPageSize).Return(postList(
		&model.Post{Id: "issued-old", UserId: bannedUserID, ChannelId: testChannelID, CreateAt: old},
	), nil)
	api.On("GetPostsForChannel", "dm-1", 0, sweepPageSize).Return(postList(
		&model.Post{Id: "dm-recent", UserId: bannedUserID, ChannelId: "dm-1", CreateAt: now},
		&model.Post{Id: "dm-other", UserId: "someone-else", ChannelId: "dm-1", CreateAt: now},
		&model.Post{Id: "dm-system", UserId: bannedUserID, ChannelId: "dm-1", CreateAt: now, Type: model.PostTypeJoinChannel},
		&model.Post{Id: "dm-hidden", UserId: bannedUserID, ChannelId: "dm-1", CreateAt: now, Props: model.StringInterface{tombstoneProp: true}},
		&model.Post{Id: "dm-old", UserId: bannedUserID, ChannelId: "dm-1", CreateAt: old},
	), nil)
	api.On("GetPostsForChannel", "team-channel", 0, sweepPageSize).Return(model.NewPostList(), nil).Maybe()

	var mu sync.Mutex
	var ids []string
	api.On("UpdatePost", mock.Anything).Return(func(post *model.Post) *model.Post {
		mu.Lock()
		defer mu.Unlock()
		ids = append(ids, post.Id)
		return &model.Post{Id: post.Id}
	}, nil)
	return since, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), ids...)
	}
}

func TestHideMessagesEverywhere(t *testing.T) {
	p, api, _ := newTestPlugin(t, nil)
	since, updated := mockCleanupPosts(api)

	dm := &model.Channel{Id: "dm-1", Type: model.ChannelTypeDirect}
	teamChannel := &model.Channel{Id: "team-channel", TeamId: testTeamID, Type: model.ChannelTypeOpen}
	issued := &model.Channel{Id: testChannelID, TeamId: testTeamID, Type: model.ChannelTypeOpen}
	api.On("GetChannelsForTeamForUser", "", bannedUserID, false).Return([]*model.Channel{dm}, nil)
	api.On("GetTeamsForUser", bannedUserID).Return([]*model.Team{{Id: testTeamID}, {Id: otherTeamID}}, nil)
	// Each team repeats the member's direct messages.
	api.On("GetChannelsForTeamForUser", testTeamID, bannedUserID, false).Return([]*model.Channel{issued, teamChannel, dm}, nil)
	api.On("GetChannelsForTeamForUser", otherTeamID, bannedUserID, false).Return(nil, model.NewAppError("GetChannels", "not_found", nil, "", http.StatusNotFound))

	hidden, channels := p.hideMessagesEverywhere(testChannelID, bannedUserID, BannedMessageModePlaceholder, since)

	// The issued channel hides older posts too; elsewhere only posts within
	// the lookback go, once each, and never other members' or system posts.
	assert.ElementsMatch(t, []string{"issued-old", "dm-recent"}, updated())
	assert.Equal(t, 2, hidden)
	assert.Equal(t, 2, channels)
	api.AssertNumberOfCalls(t, "GetPostsForChannel", 3)
}

func TestHideMessagesEverywhereWithoutTeams(t *testing.T) {
	p, api, _ := newTestPlugin(t, nil)
	since, updated := mockCleanupPosts(api)

	api.On("GetChannelsForTeamForUser", "", bannedUserID, false).Return([]*model.Channel{{Id: "dm-1", Type: model.ChannelTypeDirect}}, nil)
	api.On("GetTeamsForUser", bannedUserID).Return(nil, model.NewAppError("GetTeamsForUser", "not_found", nil, "", http.StatusNotFound))

	hidden, _ := p.hideMessagesEverywhere(testChannelID, bannedUserID, BannedMessageModePlaceholder, since)
	assert.Equal(t, 2, hidden)
	assert.ElementsMatch(t, []string{"issued-old", "dm-recent"}, updated())
}

func TestHideRecentStopsAtLookback(t *testing.T) {
	p, api, _ := newTestPlugin(t, nil)
	now := model.GetMillis()
	since := now - lookback.Milliseconds()

	// A full page that is entirely older than the lookback ends the scan.
	page := make([]*model.Post, 0, sweepPageSize)
	for i := range sweepPageSize {
		page = append(page, &model.Post{Id: model.NewId(), UserId: bannedUserID, CreateAt: since - int64(i) - 1})
	}
	api.On("GetPostsForChannel", "busy", 0, sweepPageSize).Return(postList(page...), nil).Once()

	cleanup := &banCleanup{p: p, targetID: bannedUserID, mode: BannedMessageModePlaceholder}
	assert.Zero(t, cleanup.hideRecent("busy", maxSweepPages, since))
	api.AssertNotCalled(t, "UpdatePost", mock.Anything)
}

func TestBanResolvesReportsEverywhere(t *testing.T) {
	p, api, kv := newTestPlugin(t, map[string]bool{bannedUserID: false})
	p.configuration = &configuration{}
	api.On("GetUser", bannedUserID).Return(&model.User{Id: bannedUserID, Username: "someone"}, nil)
	api.On("UpdateUserActive", bannedUserID, false).Return(nil)
	api.On("KVSetWithOptions", mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Maybe()
	api.On("PublishWebSocketEvent", mock.Anything, mock.Anything, mock.Anything).Maybe()

	kv.reports[testChannelID] = []*kvstore.Report{{ID: "here", TargetUserID: bannedUserID, Status: kvstore.ReportStatusOpen}}
	kv.reports["dm-1"] = []*kvstore.Report{{ID: "dm", TargetUserID: bannedUserID, Status: kvstore.ReportStatusEscalated}}
	kv.reports["dm-2"] = []*kvstore.Report{{ID: "other", TargetUserID: "someone-else", Status: kvstore.ReportStatusOpen}}

	_, err := p.banUser(sysAdminID, testChannelID, bannedUserID, false)
	assert.NoError(t, err)
	assert.Equal(t, kvstore.ReportStatusResolved, kv.reports[testChannelID][0].Status)
	assert.Equal(t, kvstore.ReportStatusResolved, kv.reports["dm-1"][0].Status)
	assert.Equal(t, "Member banned", kv.reports["dm-1"][0].Resolution)
	assert.Equal(t, kvstore.ReportStatusOpen, kv.reports["dm-2"][0].Status)
}
