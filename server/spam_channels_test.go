package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/svelle/mattermost-plugin-moderation/server/store/kvstore"
)

func crossPostConfig() *configuration {
	return &configuration{
		CrossPostThreshold:       3,
		CrossPostWindowMinutes:   5,
		CrossPostBlockMinutes:    60,
		SpamAction:               SpamActionBlock,
		ExemptBotsFromSpamLimits: true,
	}
}

func floodConfig() *configuration {
	return &configuration{
		FloodThreshold:           3,
		FloodWindowSeconds:       30,
		FloodTimeoutMinutes:      10,
		ExemptBotsFromSpamLimits: true,
	}
}

func TestCrossPostBlocksNewChannelsPastThreshold(t *testing.T) {
	d := newDMTest(t, crossPostConfig())

	for i := range 3 {
		assert.Empty(t, d.post(spammerID, fmt.Sprintf("ch-%d", i)))
	}
	reason := d.post(spammerID, "ch-3")
	assert.Contains(t, reason, "can't post in other channels for 1 hour")
	assert.EqualValues(t, 1, d.alerts.Load(), "admins are alerted once")
	assert.Contains(t, d.alertTexts[0], "@spammer tried to post in 4 different channels within 5 minutes")

	assert.NotEmpty(t, d.post(spammerID, "ch-4"), "other channels stay blocked")
	assert.EqualValues(t, 1, d.alerts.Load(), "a second refusal doesn't alert again")

	// Channels posted in before the block still work.
	d.p.spamGuard = spamGuard{}
	assert.Empty(t, d.post(spammerID, "ch-0"))
	assert.Len(t, d.kv.spamContacts[kvstore.SpamScopeChannel+spammerID], 3)
}

func TestCrossPostCountsAChannelAgainAfterTheWindow(t *testing.T) {
	d := newDMTest(t, crossPostConfig())

	// Three channels posted in ten minutes ago no longer count.
	tenMinutesAgo := model.GetMillis() - (10 * time.Minute).Milliseconds()
	for i := range 3 {
		d.kv.spamContacts[kvstore.SpamScopeChannel+spammerID] = append(d.kv.spamContacts[kvstore.SpamScopeChannel+spammerID],
			kvstore.SpamContact{ChannelID: fmt.Sprintf("old-%d", i), FirstAt: tenMinutesAgo, People: 1})
	}
	for i := range 3 {
		assert.Empty(t, d.post(spammerID, fmt.Sprintf("ch-%d", i)))
	}
	assert.Len(t, d.kv.spamContacts[kvstore.SpamScopeChannel+spammerID], 3)
}

func TestCrossPostParallelPostsCantSlipUnderTheLimit(t *testing.T) {
	config := crossPostConfig()
	config.CrossPostThreshold = 5
	d := newDMTest(t, config)

	var allowed atomic.Int32
	var wg sync.WaitGroup
	for i := range 30 {
		wg.Go(func() {
			if d.post(spammerID, fmt.Sprintf("ch-%d", i)) == "" {
				allowed.Add(1)
			}
		})
	}
	wg.Wait()

	assert.EqualValues(t, 5, allowed.Load())
	assert.EqualValues(t, 1, d.alerts.Load())
}

func TestCrossPostExemptsModeratorsAndAdmins(t *testing.T) {
	d := newDMTest(t, crossPostConfig())

	for i := range 6 {
		assert.Empty(t, d.post(channelModID, fmt.Sprintf("ch-%d", i)), "channel moderators aren't limited")
		assert.Empty(t, d.post(sysAdminID, fmt.Sprintf("ch-%d", i)), "system admins aren't limited")
	}
	assert.Empty(t, d.kv.spamBlocks)
	assert.Zero(t, d.alerts.Load())
}

func TestDMAndChannelLimitsCountSeparately(t *testing.T) {
	config := crossPostConfig()
	config.MassDMThreshold = 3
	config.MassDMWindowMinutes = 5
	config.MassDMBlockMinutes = 60
	d := newDMTest(t, config)

	for i := range 3 {
		assert.Empty(t, d.post(spammerID, fmt.Sprintf("dm-%d", i)))
		assert.Empty(t, d.post(spammerID, fmt.Sprintf("ch-%d", i)))
	}
	assert.Len(t, d.kv.spamContacts[kvstore.SpamScopeDM+spammerID], 3)
	assert.Len(t, d.kv.spamContacts[kvstore.SpamScopeChannel+spammerID], 3)
	assert.Zero(t, d.alerts.Load())
}

func TestFloodTimesOutMember(t *testing.T) {
	d := newDMTest(t, floodConfig())
	d.kv.moderators["ch-flood"] = []string{"moddy"}

	for range 3 {
		assert.Empty(t, d.post(spammerID, "ch-flood"))
	}
	assert.Contains(t, d.post(spammerID, "ch-flood"), "posting too quickly")

	restriction := d.kv.restrictions[kvstore.RestrictionKey("ch-flood", spammerID)]
	if assert.NotNil(t, restriction) {
		assert.Equal(t, "bot", restriction.ByUserID)
		assert.Equal(t, "10 minutes", restriction.TimeoutLabel)
		assert.True(t, restriction.TimedOutNow(model.GetMillis()))
	}
	assert.Contains(t, d.post(spammerID, "ch-flood"), "timed out in this channel")
	assert.Empty(t, d.post(spammerID, "ch-other"), "the timeout only covers the flooded channel")

	// Only the channel's assigned moderators hear about it.
	assert.EqualValues(t, 1, d.alerts.Load())
	assert.Contains(t, d.alertTexts[0], "@spammer was timed out in ~ch-flood for 10 minutes")
	d.api.AssertNotCalled(t, "GetUsers", mock.Anything)
}

func TestFloodWindowSlides(t *testing.T) {
	var g spamGuard
	window := 30 * time.Second
	assert.Equal(t, 1, g.recordFloodPost("u", "c", 0, window))
	assert.Equal(t, 2, g.recordFloodPost("u", "c", 20_000, window))
	assert.Equal(t, 2, g.recordFloodPost("u", "c", 40_000, window), "the post at 0 left the window")
	assert.Equal(t, 1, g.recordFloodPost("u", "other", 40_000, window), "channels count separately")
}

func TestFloodExemptsModerators(t *testing.T) {
	d := newDMTest(t, floodConfig())
	for range 6 {
		assert.Empty(t, d.post(channelModID, "ch-flood"))
	}
	assert.Empty(t, d.kv.restrictions)
}

func TestBotToggle(t *testing.T) {
	config := dmConfig()
	config.FloodThreshold = 3
	config.FloodWindowSeconds = 30
	config.FloodTimeoutMinutes = 10
	d := newDMTest(t, config)

	// Exempt by default.
	for i := range 5 {
		assert.Empty(t, d.post("helper-bot", fmt.Sprintf("dm-%d", i)))
		assert.Empty(t, d.post("helper-bot", "ch-busy"))
	}
	assert.Empty(t, d.kv.spamBlocks)
	assert.Empty(t, d.kv.restrictions)

	// With the toggle off, a bot is limited like anyone else, and a
	// suspension falls back to the block.
	limited := dmConfig()
	limited.ExemptBotsFromSpamLimits = false
	limited.SpamAction = SpamActionSuspend
	d.p.configuration = limited
	d.p.spamGuard = spamGuard{}
	// Exemption is only checked once a limit would trip, so conversations
	// below the limit were still recorded; start the bot from a clean slate.
	d.kv.spamContacts = map[string][]kvstore.SpamContact{}
	refused := 0
	for i := range 5 {
		if d.post("helper-bot", fmt.Sprintf("dm-new-%d", i)) != "" {
			refused++
		}
	}
	assert.Equal(t, 2, refused, "the bot is refused past the limit")
	assert.EqualValues(t, 1, d.alerts.Load())
	assert.Contains(t, d.alertTexts[0], "Bots can't be suspended by the plugin")
	assert.Empty(t, d.kv.bans)
	d.api.AssertNotCalled(t, "UpdateUserActive", mock.Anything, mock.Anything)
}

func TestWebhookPostsFollowTheBotToggle(t *testing.T) {
	d := newDMTest(t, floodConfig())
	webhookPost := func() string {
		post := &model.Post{UserId: spammerID, ChannelId: "ch-ci", Message: "build passed"}
		post.AddProp(model.PostPropsFromWebhook, "true")
		_, reason := d.p.MessageWillBePosted(nil, post)
		return reason
	}
	for range 6 {
		assert.Empty(t, webhookPost())
	}
	assert.Empty(t, d.kv.restrictions)

	limited := floodConfig()
	limited.ExemptBotsFromSpamLimits = false
	d.p.configuration = limited
	refused := 0
	for range 6 {
		if webhookPost() != "" {
			refused++
		}
	}
	assert.Positive(t, refused)
}

func TestPluginBotIsAlwaysExempt(t *testing.T) {
	config := dmConfig()
	config.ExemptBotsFromSpamLimits = false
	d := newDMTest(t, config)
	for i := range 5 {
		assert.Empty(t, d.post("bot", fmt.Sprintf("dm-%d", i)))
	}
	assert.Empty(t, d.kv.spamContacts)
}

func TestSpamLimitDefaults(t *testing.T) {
	assert.True(t, (&Plugin{}).getConfiguration().ExemptBotsFromSpamLimits)

	c := &configuration{CrossPostThreshold: 500, FloodThreshold: -3, FloodWindowSeconds: 5000}
	c.normalizeSpamLimits()
	assert.Equal(t, maxCrossPostThreshold, c.CrossPostThreshold)
	assert.Equal(t, defaultFloodThreshold, c.FloodThreshold)
	assert.Equal(t, maxFloodWindowSeconds, c.FloodWindowSeconds)
	assert.Equal(t, defaultCrossPostWindowMinutes, c.CrossPostWindowMinutes)
	assert.Equal(t, defaultCrossPostBlockMinutes, c.CrossPostBlockMinutes)
	assert.Equal(t, defaultFloodTimeoutMinutes, c.FloodTimeoutMinutes)
}
