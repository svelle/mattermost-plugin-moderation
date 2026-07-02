package main

import (
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/mattermost/mattermost/server/public/pluginapi/cluster"
	"github.com/pkg/errors"

	"github.com/svelle/mattermost-plugin-moderation/server/store/kvstore"
)

const (
	botUsername    = "community-moderation"
	botDisplayName = "Community Moderation"
	botDescription = "Bot for the Community Moderation plugin. Sends moderation notices."
)

// Plugin implements the interface expected by the Mattermost server to communicate between the server and plugin processes.
type Plugin struct {
	plugin.MattermostPlugin

	// kvstore is the client used to read/write KV records for this plugin.
	kvstore kvstore.KVStore

	// client is the Mattermost server API client.
	client *pluginapi.Client

	// botUserID is the user ID of the plugin bot used for moderation notices.
	botUserID string

	// router is the HTTP router for handling API requests.
	router *mux.Router

	backgroundJob *cluster.Job

	// configurationLock synchronizes access to the configuration.
	configurationLock sync.RWMutex

	// configuration is the active plugin configuration. Consult getConfiguration and
	// setConfiguration for usage.
	configuration *configuration
}

// OnActivate is invoked when the plugin is activated. If an error is returned, the plugin will be deactivated.
func (p *Plugin) OnActivate() error {
	p.client = pluginapi.NewClient(p.API, p.Driver)

	p.kvstore = kvstore.NewKVStore(p.client)

	botUserID, err := p.client.Bot.EnsureBot(&model.Bot{
		Username:    botUsername,
		DisplayName: botDisplayName,
		Description: botDescription,
	})
	if err != nil {
		return errors.Wrap(err, "failed to ensure moderation bot")
	}
	p.botUserID = botUserID

	p.router = p.initRouter()

	job, err := cluster.Schedule(
		p.API,
		"CleanupExpiredTimeouts",
		cluster.MakeWaitForRoundedInterval(1*time.Hour),
		p.runJob,
	)
	if err != nil {
		return errors.Wrap(err, "failed to schedule background job")
	}

	p.backgroundJob = job

	return nil
}

// OnDeactivate is invoked when the plugin is deactivated.
func (p *Plugin) OnDeactivate() error {
	if p.backgroundJob != nil {
		if err := p.backgroundJob.Close(); err != nil {
			p.API.LogError("Failed to close background job", "err", err)
		}
	}
	return nil
}

func (p *Plugin) now() int64 {
	return model.GetMillis()
}

// Cluster mutex key prefixes for channel-scoped read-modify-write cycles.
const (
	lockPrefixReports    = "lock_reports_"
	lockPrefixModerators = "lock_mods_"
)

// lockChannel acquires a cluster-wide mutex for a channel-scoped KV value,
// keeping read-modify-write cycles safe across nodes in HA deployments. The
// returned function releases the lock.
func (p *Plugin) lockChannel(prefix, channelID string) (func(), error) {
	mutex, err := cluster.NewMutex(p.API, prefix+channelID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create cluster mutex")
	}
	mutex.Lock()
	return mutex.Unlock, nil
}

// See https://developers.mattermost.com/extend/plugins/server/reference/
