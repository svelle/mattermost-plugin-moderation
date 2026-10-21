package main

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/svelle/mattermost-plugin-moderation/server/store/kvstore"
)

const (
	spammerID    = "spammerspammerspammerspamm"
	channelModID = "channelmod"
)

// dmTest is a plugin wired for spam limit tests: direct message channels
// named "dm-*" (with the spammer), group messages named "gm-*" (five
// members), team channels with any other name, conversations where the
// spammer already posted, and a count of the bot's DMs.
type dmTest struct {
	p      *Plugin
	api    *plugintest.API
	kv     *fakeKVStore
	alerts atomic.Int32

	mu         sync.Mutex
	postedIn   map[string]bool
	alertTexts []string
}

func newDMTest(t *testing.T, config *configuration) *dmTest {
	p, api, kv := newTestPlugin(t, nil)
	p.configuration = config
	d := &dmTest{p: p, api: api, kv: kv, postedIn: map[string]bool{}}

	// A working in-memory cluster mutex, so concurrent posts really
	// contend for the per-member lock.
	var lockMu sync.Mutex
	locks := map[string]bool{}
	api.On("KVSetWithOptions", mock.Anything, mock.Anything, mock.Anything).Return(func(key string, value []byte, opts model.PluginKVSetOptions) bool {
		lockMu.Lock()
		defer lockMu.Unlock()
		switch {
		case value == nil:
			delete(locks, key)
		case opts.Atomic && opts.OldValue == nil:
			if locks[key] {
				return false
			}
			locks[key] = true
		}
		return true
	}, nil).Maybe()

	api.On("GetChannel", mock.Anything).Return(func(channelID string) *model.Channel {
		switch {
		case strings.HasPrefix(channelID, "dm-"):
			return &model.Channel{Id: channelID, Type: model.ChannelTypeDirect, Name: spammerID + "__" + channelID}
		case strings.HasPrefix(channelID, "gm-"):
			return &model.Channel{Id: channelID, Type: model.ChannelTypeGroup, Name: channelID}
		}
		return &model.Channel{Id: channelID, TeamId: testTeamID, Type: model.ChannelTypeOpen, Name: channelID}
	}, nil).Maybe()
	api.On("GetChannelStats", mock.Anything).Return(&model.ChannelStats{MemberCount: 5}, nil).Maybe()
	api.On("GetPostsForChannel", mock.Anything, 0, firstContactScanSize).Return(func(channelID string, _, _ int) *model.PostList {
		list := model.NewPostList()
		d.mu.Lock()
		defer d.mu.Unlock()
		if d.postedIn[channelID] {
			list.AddPost(&model.Post{Id: "earlier-" + channelID, UserId: spammerID, ChannelId: channelID})
			list.AddOrder("earlier-" + channelID)
		}
		return list
	}, nil).Maybe()
	api.On("GetUser", spammerID).Return(&model.User{Id: spammerID, Username: "spammer"}, nil).Maybe()
	api.On("GetUser", "helper-bot").Return(&model.User{Id: "helper-bot", Username: "helper", IsBot: true}, nil).Maybe()
	api.On("GetUser", sysAdminID).Return(&model.User{Id: sysAdminID, Username: "admin"}, nil).Maybe()
	api.On("SendEphemeralPost", mock.Anything, mock.Anything).Return(&model.Post{}).Maybe()
	api.On("GetUsers", mock.Anything).Return([]*model.User{{Id: sysAdminID}}, nil).Maybe()
	api.On("GetUser", mock.Anything).Return(func(userID string) *model.User {
		return &model.User{Id: userID, Username: userID}
	}, nil).Maybe()
	// channelModID is a channel admin, and so a moderator, everywhere.
	api.On("GetChannelMember", mock.Anything, mock.Anything).Return(func(channelID, userID string) *model.ChannelMember {
		return &model.ChannelMember{ChannelId: channelID, UserId: userID, SchemeAdmin: userID == channelModID}
	}, nil).Maybe()
	api.On("PublishWebSocketEvent", mock.Anything, mock.Anything, mock.Anything).Maybe()
	api.On("GetDirectChannel", "bot", mock.Anything).Return(func(_, userID string) *model.Channel {
		return &model.Channel{Id: "bot-dm-" + userID}
	}, nil).Maybe()
	api.On("CreatePost", mock.Anything).Return(func(post *model.Post) *model.Post {
		d.alerts.Add(1)
		d.mu.Lock()
		d.alertTexts = append(d.alertTexts, post.Message)
		d.mu.Unlock()
		// pluginapi copies the result onto the post, so it must be a
		// different value.
		return &model.Post{Id: model.NewId(), Message: post.Message}
	}, nil).Maybe()
	return d
}

func (d *dmTest) post(userID, channelID string) string {
	_, reason := d.p.MessageWillBePosted(nil, &model.Post{UserId: userID, ChannelId: channelID, Message: "hi"})
	return reason
}

func dmConfig() *configuration {
	return &configuration{
		MassDMThreshold:     3,
		MassDMWindowMinutes: 5,
		MassDMBlockMinutes:  60,
		SpamAction:          SpamActionBlock,

		ExemptBotsFromSpamLimits: true,
	}
}

func TestMassDMBlocksNewConversationsPastThreshold(t *testing.T) {
	d := newDMTest(t, dmConfig())

	for i := range 3 {
		assert.Empty(t, d.post(spammerID, fmt.Sprintf("dm-%d", i)))
	}
	reason := d.post(spammerID, "dm-3")
	assert.Contains(t, reason, "can't start new direct messages for 1 hour")
	assert.EqualValues(t, 1, d.alerts.Load(), "admins are alerted once")
	assert.Contains(t, d.alertTexts[0], "@spammer tried to start conversations with 4 people within 5 minutes")

	assert.NotEmpty(t, d.post(spammerID, "dm-4"), "new conversations stay blocked")
	assert.EqualValues(t, 1, d.alerts.Load(), "a second refusal doesn't alert again")

	// Conversations started before the block can still continue.
	d.p.spamGuard = spamGuard{}
	assert.Empty(t, d.post(spammerID, "dm-0"))
	assert.Len(t, d.kv.spamContacts[kvstore.SpamScopeDM+spammerID], 3)
}

func TestMassDMIgnoresReplies(t *testing.T) {
	d := newDMTest(t, dmConfig())
	for i := range 21 {
		d.postedIn[fmt.Sprintf("dm-%d", i)] = true
	}
	for i := range 21 {
		assert.Empty(t, d.post(spammerID, fmt.Sprintf("dm-%d", i)))
	}
	assert.Empty(t, d.kv.spamContacts[kvstore.SpamScopeDM+spammerID])
	assert.Zero(t, d.alerts.Load())
}

func TestMassDMParallelPostsCantSlipUnderTheLimit(t *testing.T) {
	config := dmConfig()
	config.MassDMThreshold = 5
	d := newDMTest(t, config)

	var allowed atomic.Int32
	var wg sync.WaitGroup
	for i := range 30 {
		wg.Go(func() {
			if d.post(spammerID, fmt.Sprintf("dm-%d", i)) == "" {
				allowed.Add(1)
			}
		})
	}
	wg.Wait()

	assert.EqualValues(t, 5, allowed.Load())
	assert.Len(t, d.kv.spamContacts[kvstore.SpamScopeDM+spammerID], 5)
	assert.EqualValues(t, 1, d.alerts.Load())
	assert.NotEmpty(t, d.post(spammerID, "dm-new"))
}

func TestMassDMDailyLimitOutlivesTheBlock(t *testing.T) {
	config := dmConfig()
	config.MassDMDailyThreshold = 4
	d := newDMTest(t, config)

	// Four conversations started two hours ago, after a block that has
	// since expired.
	twoHoursAgo := model.GetMillis() - (2 * time.Hour).Milliseconds()
	for i := range 4 {
		d.kv.spamContacts[kvstore.SpamScopeDM+spammerID] = append(d.kv.spamContacts[kvstore.SpamScopeDM+spammerID], kvstore.SpamContact{ChannelID: fmt.Sprintf("old-%d", i), FirstAt: twoHoursAgo, People: 1})
	}
	d.kv.spamBlocks[kvstore.SpamScopeDM+spammerID] = &kvstore.SpamBlock{Until: model.GetMillis() - 1}

	assert.NotEmpty(t, d.post(spammerID, "dm-new"))
	assert.EqualValues(t, 1, d.alerts.Load())
	assert.Contains(t, d.alertTexts[0], "within 24 hours")
}

func TestMassDMCountsGroupMembers(t *testing.T) {
	d := newDMTest(t, dmConfig())

	// A group message reaching four people is allowed on its own...
	assert.Empty(t, d.post(spammerID, "gm-1"))
	// ...but counts as four toward the limit.
	assert.NotEmpty(t, d.post(spammerID, "dm-1"))
}

func TestMassDMLeavesOthersAlone(t *testing.T) {
	d := newDMTest(t, dmConfig())

	for range 5 {
		assert.Empty(t, d.post(spammerID, testChannelID), "team channels aren't limited")
	}
	for i := range 5 {
		assert.Empty(t, d.post("helper-bot", fmt.Sprintf("dm-bot-%d", i)), "bots aren't limited")
		assert.Empty(t, d.post(sysAdminID, fmt.Sprintf("dm-admin-%d", i)), "system admins aren't limited")
	}
	assert.Empty(t, d.kv.spamContacts[kvstore.SpamScopeDM+spammerID])
	assert.Empty(t, d.kv.spamBlocks)
	assert.Zero(t, d.alerts.Load())

	disabled := dmConfig()
	disabled.MassDMThreshold = 0
	d.p.configuration = disabled
	for i := range 5 {
		assert.Empty(t, d.post(spammerID, fmt.Sprintf("dm-off-%d", i)))
	}
}

func TestMassDMFailsOpen(t *testing.T) {
	p, api, kv := newTestPlugin(t, nil)
	p.configuration = dmConfig()
	api.On("GetChannel", "dm-broken").Return(nil, model.NewAppError("GetChannel", "boom", nil, "", http.StatusInternalServerError))

	_, reason := p.MessageWillBePosted(nil, &model.Post{UserId: spammerID, ChannelId: "dm-broken", Message: "hi"})
	assert.Empty(t, reason)
	assert.Empty(t, kv.spamContacts)
}

func TestMassDMCanSuspend(t *testing.T) {
	config := dmConfig()
	config.SpamAction = SpamActionSuspend
	d := newDMTest(t, config)
	d.api.On("UpdateUserActive", spammerID, false).Return(nil).Once()

	for i := range 4 {
		d.post(spammerID, fmt.Sprintf("dm-%d", i))
	}
	ban := d.kv.bans[spammerID]
	if assert.NotNil(t, ban) {
		assert.Equal(t, "bot", ban.ByUserID)
		assert.Equal(t, "dm-3", ban.ChannelID)
		assert.Empty(t, ban.TeamID)
	}
	assert.EqualValues(t, 1, d.alerts.Load())
	assert.Contains(t, d.alertTexts[0], "suspended automatically")
	assert.Contains(t, d.alertTexts[0], "A system admin can lift")
}

func TestMassDMSuspendSkipsBannedAndReportsFailure(t *testing.T) {
	config := dmConfig()
	config.SpamAction = SpamActionSuspend
	d := newDMTest(t, config)

	user := &model.User{Id: spammerID, Username: "spammer"}
	d.kv.bans[spammerID] = &kvstore.Ban{UserID: spammerID}
	assert.Equal(t, "Their account was already banned.", d.p.suspendForSpam(user, "dm-1", "start new direct or group messages", "60 minutes", config))
	d.api.AssertNotCalled(t, "UpdateUserActive", mock.Anything, mock.Anything)

	delete(d.kv.bans, spammerID)
	d.api.On("UpdateUserActive", spammerID, false).Return(model.NewAppError("UpdateUserActive", "boom", nil, "", http.StatusInternalServerError)).Once()
	assert.Contains(t, d.p.suspendForSpam(user, "dm-1", "start new direct or group messages", "60 minutes", config), "only the block is in place")
	assert.Empty(t, d.kv.bans)
}

func TestNormalizeSpamLimitSettings(t *testing.T) {
	c := &configuration{MassDMThreshold: -1, MassDMDailyThreshold: 5000, SpamAction: "nuke"}
	c.normalizeSpamLimits()
	assert.Equal(t, defaultMassDMThreshold, c.MassDMThreshold)
	assert.Equal(t, maxMassDMDailyThreshold, c.MassDMDailyThreshold)
	assert.Equal(t, defaultMassDMWindowMinutes, c.MassDMWindowMinutes)
	assert.Equal(t, defaultMassDMBlockMinutes, c.MassDMBlockMinutes)
	assert.Equal(t, defaultBannedMessageLookbackHours, c.BannedMessageLookbackHours)
	assert.Equal(t, SpamActionBlock, c.SpamAction)

	off := &configuration{MassDMThreshold: 0, MassDMDailyThreshold: 0}
	off.normalizeSpamLimits()
	assert.Zero(t, off.MassDMThreshold, "an explicit zero turns the limit off")
	assert.Zero(t, off.MassDMDailyThreshold)
}
