package main

import (
	"reflect"

	"github.com/pkg/errors"
)

// Banned message display modes.
const (
	BannedMessageModePlaceholder = "placeholder"
	BannedMessageModeRemove      = "remove"
)

// Actions taken when a member hits the mass direct message or cross-posting
// limit.
const (
	SpamActionBlock   = "block"
	SpamActionSuspend = "suspend"
)

// Defaults and upper bounds for the spam limit and cleanup settings.
const (
	defaultMassDMThreshold            = 20
	defaultMassDMWindowMinutes        = 5
	defaultMassDMDailyThreshold       = 100
	defaultMassDMBlockMinutes         = 60
	defaultCrossPostThreshold         = 10
	defaultCrossPostWindowMinutes     = 5
	defaultCrossPostBlockMinutes      = 60
	defaultFloodThreshold             = 15
	defaultFloodWindowSeconds         = 30
	defaultFloodTimeoutMinutes        = 10
	defaultBannedMessageLookbackHours = 72

	maxMassDMThreshold            = 500
	maxMassDMDailyThreshold       = 1000
	maxCrossPostThreshold         = 200
	maxFloodThreshold             = 200
	maxFloodWindowSeconds         = 600
	maxWindowMinutes              = 24 * 60
	maxBlockMinutes               = 7 * 24 * 60
	maxBannedMessageLookbackHours = 30 * 24
)

// configuration captures the plugin's external configuration as exposed in the Mattermost server
// configuration, as well as values computed from the configuration. Any public fields will be
// deserialized from the Mattermost server configuration in OnConfigurationChange.
//
// As plugins are inherently concurrent (hooks being called asynchronously), and the plugin
// configuration can change at any time, access to the configuration must be synchronized. The
// strategy used in this plugin is to guard a pointer to the configuration, and clone the entire
// struct whenever it changes. You may replace this with whatever strategy you choose.
//
// If you add non-reference types to your configuration struct, be sure to rewrite Clone as a deep
// copy appropriate for your types.
type configuration struct {
	// HideBannedMessages hides a banned member's recent messages in the
	// channel where the ban was issued, and their messages from the last
	// BannedMessageLookbackHours in every other channel and conversation.
	HideBannedMessages bool

	// BannedMessageLookbackHours bounds how far back HideBannedMessages
	// reaches outside the channel where the ban was issued.
	BannedMessageLookbackHours int

	// HideBannedIdentity replaces a banned member's username and name with a
	// placeholder everywhere, restoring them if the ban is lifted.
	HideBannedIdentity bool

	// BannedMessageMode controls how hidden messages are displayed:
	// "placeholder" replaces the message text with a tombstone, "remove"
	// deletes the messages entirely.
	BannedMessageMode string

	// ReportRateLimit is the maximum number of reports one member can
	// submit within ReportRateWindowMinutes. Zero disables rate limiting.
	ReportRateLimit int

	// ReportRateWindowMinutes is the rolling window for ReportRateLimit.
	ReportRateWindowMinutes int

	// MassDMThreshold is how many people a member can start direct or
	// group conversations with within MassDMWindowMinutes before new ones
	// are blocked. Zero disables this limit.
	MassDMThreshold int

	// MassDMWindowMinutes is the rolling window for MassDMThreshold.
	MassDMWindowMinutes int

	// MassDMDailyThreshold is the same limit over 24 hours, so a member
	// can't wait out a block and start again. Zero disables this limit.
	MassDMDailyThreshold int

	// MassDMBlockMinutes is how long new conversations stay blocked.
	MassDMBlockMinutes int

	// CrossPostThreshold is how many different team channels a member can
	// post in within CrossPostWindowMinutes before posting in others is
	// blocked. Zero disables this limit.
	CrossPostThreshold int

	// CrossPostWindowMinutes is the rolling window for CrossPostThreshold.
	CrossPostWindowMinutes int

	// CrossPostBlockMinutes is how long posting in other channels stays
	// blocked.
	CrossPostBlockMinutes int

	// SpamAction is what happens besides the block when a member hits the
	// mass direct message or cross-posting limit: SpamActionBlock alerts
	// system admins, SpamActionSuspend also bans the account.
	SpamAction string

	// FloodThreshold is how many messages a member can post in one team
	// channel within FloodWindowSeconds before they're timed out there.
	// Zero disables this limit.
	FloodThreshold int

	// FloodWindowSeconds is the rolling window for FloodThreshold.
	FloodWindowSeconds int

	// FloodTimeoutMinutes is how long the automatic timeout lasts.
	FloodTimeoutMinutes int

	// ExemptBotsFromSpamLimits leaves bot accounts out of the mass direct
	// message, cross-posting, and flood limits.
	ExemptBotsFromSpamLimits bool
}

// Clone shallow copies the configuration. Your implementation may require a deep copy if
// your configuration has reference types.
func (c *configuration) Clone() *configuration {
	clone := *c
	return &clone
}

// getConfiguration retrieves the active configuration under lock, making it safe to use
// concurrently. The active configuration may change underneath the client of this method, but
// the struct returned by this API call is considered immutable.
func (p *Plugin) getConfiguration() *configuration {
	p.configurationLock.RLock()
	defer p.configurationLock.RUnlock()

	if p.configuration == nil {
		return &configuration{
			HideBannedMessages:         true,
			HideBannedIdentity:         true,
			BannedMessageMode:          BannedMessageModePlaceholder,
			ReportRateLimit:            5,
			ReportRateWindowMinutes:    10,
			BannedMessageLookbackHours: defaultBannedMessageLookbackHours,
			MassDMThreshold:            defaultMassDMThreshold,
			MassDMWindowMinutes:        defaultMassDMWindowMinutes,
			MassDMDailyThreshold:       defaultMassDMDailyThreshold,
			MassDMBlockMinutes:         defaultMassDMBlockMinutes,
			CrossPostThreshold:         defaultCrossPostThreshold,
			CrossPostWindowMinutes:     defaultCrossPostWindowMinutes,
			CrossPostBlockMinutes:      defaultCrossPostBlockMinutes,
			SpamAction:                 SpamActionBlock,
			FloodThreshold:             defaultFloodThreshold,
			FloodWindowSeconds:         defaultFloodWindowSeconds,
			FloodTimeoutMinutes:        defaultFloodTimeoutMinutes,
			ExemptBotsFromSpamLimits:   true,
		}
	}

	return p.configuration
}

// setConfiguration replaces the active configuration under lock.
//
// Do not call setConfiguration while holding the configurationLock, as sync.Mutex is not
// reentrant. In particular, avoid using the plugin API entirely, as this may in turn trigger a
// hook back into the plugin. If that hook attempts to acquire this lock, a deadlock may occur.
//
// This method panics if setConfiguration is called with the existing configuration. This almost
// certainly means that the configuration was modified without being cloned and may result in
// an unsafe access.
func (p *Plugin) setConfiguration(configuration *configuration) {
	p.configurationLock.Lock()
	defer p.configurationLock.Unlock()

	if configuration != nil && p.configuration == configuration {
		// Ignore assignment if the configuration struct is empty. Go will optimize the
		// allocation for same to point at the same memory address, breaking the check
		// above.
		if reflect.ValueOf(*configuration).NumField() == 0 {
			return
		}

		panic("setConfiguration called with the existing configuration")
	}

	p.configuration = configuration
}

// OnConfigurationChange is invoked when configuration changes may have been made.
func (p *Plugin) OnConfigurationChange() error {
	configuration := new(configuration)

	// Load the public configuration fields from the Mattermost server configuration.
	if err := p.API.LoadPluginConfiguration(configuration); err != nil {
		return errors.Wrap(err, "failed to load plugin configuration")
	}

	if configuration.BannedMessageMode != BannedMessageModeRemove {
		configuration.BannedMessageMode = BannedMessageModePlaceholder
	}
	if configuration.ReportRateLimit < 0 {
		configuration.ReportRateLimit = 5
	}
	if configuration.ReportRateWindowMinutes <= 0 {
		configuration.ReportRateWindowMinutes = 10
	}
	configuration.normalizeSpamLimits()

	p.setConfiguration(configuration)

	return nil
}

// clampSetting replaces a value below minimum with def and caps it at maximum.
func clampSetting(value, minimum, maximum, def int) int {
	if value < minimum {
		return def
	}
	return min(value, maximum)
}

// normalizeSpamLimits fills in defaults for invalid spam limit and cleanup
// settings.
// An explicit zero threshold is kept, since it turns that limit off.
func (c *configuration) normalizeSpamLimits() {
	c.MassDMThreshold = clampSetting(c.MassDMThreshold, 0, maxMassDMThreshold, defaultMassDMThreshold)
	c.MassDMDailyThreshold = clampSetting(c.MassDMDailyThreshold, 0, maxMassDMDailyThreshold, defaultMassDMDailyThreshold)
	c.MassDMWindowMinutes = clampSetting(c.MassDMWindowMinutes, 1, maxWindowMinutes, defaultMassDMWindowMinutes)
	c.MassDMBlockMinutes = clampSetting(c.MassDMBlockMinutes, 1, maxBlockMinutes, defaultMassDMBlockMinutes)
	c.CrossPostThreshold = clampSetting(c.CrossPostThreshold, 0, maxCrossPostThreshold, defaultCrossPostThreshold)
	c.CrossPostWindowMinutes = clampSetting(c.CrossPostWindowMinutes, 1, maxWindowMinutes, defaultCrossPostWindowMinutes)
	c.CrossPostBlockMinutes = clampSetting(c.CrossPostBlockMinutes, 1, maxBlockMinutes, defaultCrossPostBlockMinutes)
	c.FloodThreshold = clampSetting(c.FloodThreshold, 0, maxFloodThreshold, defaultFloodThreshold)
	c.FloodWindowSeconds = clampSetting(c.FloodWindowSeconds, 1, maxFloodWindowSeconds, defaultFloodWindowSeconds)
	c.FloodTimeoutMinutes = clampSetting(c.FloodTimeoutMinutes, 1, maxBlockMinutes, defaultFloodTimeoutMinutes)
	c.BannedMessageLookbackHours = clampSetting(c.BannedMessageLookbackHours, 1, maxBannedMessageLookbackHours, defaultBannedMessageLookbackHours)
	if c.SpamAction != SpamActionSuspend {
		c.SpamAction = SpamActionBlock
	}
}
