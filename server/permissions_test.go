package main

import (
	"net/http"
	"slices"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/svelle/mattermost-plugin-moderation/server/store/kvstore"
)

// fakeKVStore is an in-memory kvstore.KVStore for tests.
type fakeKVStore struct {
	reports      map[string][]*kvstore.Report
	restrictions map[string]*kvstore.Restriction
	bans         map[string]*kvstore.Ban
	moderators   map[string][]string
}

func newFakeKVStore() *fakeKVStore {
	return &fakeKVStore{
		reports:      map[string][]*kvstore.Report{},
		restrictions: map[string]*kvstore.Restriction{},
		bans:         map[string]*kvstore.Ban{},
		moderators:   map[string][]string{},
	}
}

func (f *fakeKVStore) GetReports(channelID string) ([]*kvstore.Report, error) {
	return f.reports[channelID], nil
}

func (f *fakeKVStore) SaveReports(channelID string, reports []*kvstore.Report) error {
	f.reports[channelID] = reports
	return nil
}
func (f *fakeKVStore) GetReportStamps(string) ([]int64, error) { return nil, nil }
func (f *fakeKVStore) SaveReportStamps(string, []int64) error  { return nil }
func (f *fakeKVStore) GetRestriction(channelID, userID string) (*kvstore.Restriction, error) {
	return f.restrictions[kvstore.RestrictionKey(channelID, userID)], nil
}

func (f *fakeKVStore) SetRestriction(channelID, userID string, restriction *kvstore.Restriction) error {
	f.restrictions[kvstore.RestrictionKey(channelID, userID)] = restriction
	return nil
}

func (f *fakeKVStore) DeleteRestriction(channelID, userID string) error {
	delete(f.restrictions, kvstore.RestrictionKey(channelID, userID))
	return nil
}
func (f *fakeKVStore) ListRestrictionKeys(int, int) ([]string, bool, error) { return nil, false, nil }
func (f *fakeKVStore) GetRestrictionByKey(key string) (*kvstore.Restriction, error) {
	return f.restrictions[key], nil
}

func (f *fakeKVStore) ListKeys(page, perPage int) ([]string, error) {
	keys := []string{}
	for key := range f.restrictions {
		keys = append(keys, key)
	}
	for userID := range f.bans {
		keys = append(keys, "ban_"+userID)
	}
	slices.Sort(keys)
	start := min(page*perPage, len(keys))
	return keys[start:min(start+perPage, len(keys))], nil
}

func (f *fakeKVStore) DeleteByKey(key string) error {
	delete(f.restrictions, key)
	return nil
}
func (f *fakeKVStore) GetBan(userID string) (*kvstore.Ban, error) { return f.bans[userID], nil }
func (f *fakeKVStore) SetBan(userID string, ban *kvstore.Ban) error {
	f.bans[userID] = ban
	return nil
}

func (f *fakeKVStore) DeleteBan(userID string) error {
	delete(f.bans, userID)
	return nil
}

func (f *fakeKVStore) GetModerators(channelID string) ([]string, error) {
	return f.moderators[channelID], nil
}

func (f *fakeKVStore) SaveModerators(channelID string, userIDs []string) error {
	f.moderators[channelID] = userIDs
	return nil
}

const (
	testChannelID = "channel1"
	testTeamID    = "team1"
	sysAdminID    = "sysadmin"
	teamAdminID   = "teamadmin"
	otherTeamID   = "team2"
)

// newTestPlugin wires a plugin to a mocked server API with one channel in
// testTeamID. members lists the channel's members; channel admins among them
// count as moderators.
func newTestPlugin(t *testing.T, members map[string]bool) (*Plugin, *plugintest.API, *fakeKVStore) {
	api := &plugintest.API{}
	t.Cleanup(func() { api.AssertExpectations(t) })

	api.On("HasPermissionTo", mock.Anything, model.PermissionManageSystem).Return(func(userID string, _ *model.Permission) bool {
		return userID == sysAdminID
	}).Maybe()
	api.On("HasPermissionToTeam", mock.Anything, mock.Anything, model.PermissionManageTeam).Return(func(userID, teamID string, _ *model.Permission) bool {
		return userID == teamAdminID && teamID == testTeamID
	}).Maybe()
	api.On("GetChannel", testChannelID).Return(&model.Channel{Id: testChannelID, TeamId: testTeamID, Type: model.ChannelTypeOpen, Name: "general"}, nil).Maybe()
	api.On("GetChannelMember", testChannelID, mock.Anything).Return(
		func(_, userID string) *model.ChannelMember {
			if schemeAdmin, ok := members[userID]; ok {
				return &model.ChannelMember{ChannelId: testChannelID, UserId: userID, SchemeAdmin: schemeAdmin}
			}
			return nil
		},
		func(_, userID string) *model.AppError {
			if _, ok := members[userID]; ok {
				return nil
			}
			return model.NewAppError("GetChannelMember", "not_found", nil, "", http.StatusNotFound)
		},
	).Maybe()
	api.On("LogError", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
	api.On("PublishPluginClusterEvent", mock.Anything, mock.Anything).Return(nil).Maybe()

	kv := newFakeKVStore()
	p := &Plugin{botUserID: "bot"}
	p.SetAPI(api)
	p.client = pluginapi.NewClient(api, nil)
	p.kvstore = kv
	return p, api, kv
}

func TestModerationRequiresTargetInChannel(t *testing.T) {
	// "owner" created the channel and is its channel admin, so the plugin
	// treats them as a moderator here; "stranger" isn't in the channel.
	p, _, kv := newTestPlugin(t, map[string]bool{"owner": true, "member": false})

	err := p.warnUser("owner", testChannelID, "stranger")
	assert.EqualError(t, err, "that person isn't a member of this channel")

	err = p.muteUser("owner", testChannelID, "stranger")
	assert.EqualError(t, err, "that person isn't a member of this channel")
	assert.Empty(t, kv.restrictions, "no restriction should be recorded for a non-member")

	_, err = p.timeoutUser("owner", testChannelID, "stranger", "1h")
	assert.EqualError(t, err, "that person isn't a member of this channel")
}

func TestBanRequiresTargetInChannel(t *testing.T) {
	p, api, kv := newTestPlugin(t, map[string]bool{teamAdminID: false})

	_, err := p.banUser(teamAdminID, testChannelID, "other-team-user", false)
	assert.EqualError(t, err, "that person isn't a member of this channel")
	assert.Empty(t, kv.bans)
	api.AssertNotCalled(t, "UpdateUserActive", mock.Anything, mock.Anything)
}

func TestUnbanRequiresPluginBan(t *testing.T) {
	p, api, _ := newTestPlugin(t, map[string]bool{teamAdminID: false})

	// An account deactivated outside the plugin has no ban record and must
	// not be reactivated from a channel, even by a system admin.
	_, err := p.unbanUser(teamAdminID, testChannelID, "deactivated")
	assert.ErrorContains(t, err, "System Console")
	_, err = p.unbanUser(sysAdminID, testChannelID, "deactivated")
	assert.ErrorContains(t, err, "System Console")
	api.AssertNotCalled(t, "UpdateUserActive", mock.Anything, mock.Anything)
}

func TestUnbanScopedToBanTeam(t *testing.T) {
	p, api, kv := newTestPlugin(t, map[string]bool{teamAdminID: false})

	kv.bans["elsewhere"] = &kvstore.Ban{UserID: "elsewhere", TeamID: otherTeamID}
	_, err := p.unbanUser(teamAdminID, testChannelID, "elsewhere")
	assert.ErrorContains(t, err, "team where the ban was issued")

	kv.bans["legacy"] = &kvstore.Ban{UserID: "legacy"}
	_, err = p.unbanUser(teamAdminID, testChannelID, "legacy")
	assert.ErrorContains(t, err, "team where the ban was issued")
	api.AssertNotCalled(t, "UpdateUserActive", mock.Anything, mock.Anything)

	kv.bans["here"] = &kvstore.Ban{UserID: "here", TeamID: testTeamID}
	api.On("UpdateUserActive", "here", true).Return(nil).Once()
	api.On("PublishWebSocketEvent", mock.Anything, mock.Anything, mock.Anything).Once()
	_, err = p.unbanUser(teamAdminID, testChannelID, "here")
	assert.NoError(t, err)
	assert.NotContains(t, kv.bans, "here")
}

func TestLiftRequiresActiveRestriction(t *testing.T) {
	p, _, _ := newTestPlugin(t, map[string]bool{"owner": true})

	err := p.liftRestrictions("owner", testChannelID, "anyone")
	assert.EqualError(t, err, "this member has no active mute or timeout in this channel")
}

func TestEscalateWithoutOpenReports(t *testing.T) {
	p, api, kv := newTestPlugin(t, map[string]bool{"owner": true})
	api.On("KVSetWithOptions", mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Maybe()
	api.On("KVDelete", mock.Anything).Return(nil).Maybe()

	err := p.escalateReportsForUser(testChannelID, "rex", "owner")
	assert.ErrorContains(t, err, "no open reports")

	kv.reports[testChannelID] = []*kvstore.Report{{ID: "r1", TargetUserID: "rex", Status: kvstore.ReportStatusEscalated}}
	err = p.escalateReportsForUser(testChannelID, "rex", "owner")
	assert.ErrorContains(t, err, "already escalated")
}

func TestResolveActionedReportChecksTarget(t *testing.T) {
	p, api, kv := newTestPlugin(t, map[string]bool{"owner": true})
	api.On("KVSetWithOptions", mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Maybe()
	api.On("KVDelete", mock.Anything).Return(nil).Maybe()
	api.On("PublishWebSocketEvent", mock.Anything, mock.Anything, mock.Anything).Maybe()

	kv.reports[testChannelID] = []*kvstore.Report{
		{ID: "about-rex", TargetUserID: "rex", Status: kvstore.ReportStatusOpen},
		{ID: "about-sam", TargetUserID: "sam", Status: kvstore.ReportStatusOpen},
	}

	// A warning to rex can't be used to close the report about sam.
	p.resolveActionedReport(&memberActionRequest{ChannelID: testChannelID, UserID: "rex", ReportID: "about-sam"}, "owner", "Warning sent")
	assert.Equal(t, kvstore.ReportStatusOpen, kv.reports[testChannelID][1].Status)

	p.resolveActionedReport(&memberActionRequest{ChannelID: testChannelID, UserID: "rex", ReportID: "about-rex"}, "owner", "Warning sent")
	assert.Equal(t, kvstore.ReportStatusResolved, kv.reports[testChannelID][0].Status)
}

func TestSentence(t *testing.T) {
	assert.Equal(t, "You can't report yourself.", sentence("you can't report yourself"))
	assert.Equal(t, "Already done!", sentence("already done!"))
	assert.Equal(t, "", sentence(""))
}
