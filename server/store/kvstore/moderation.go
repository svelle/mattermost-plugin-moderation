package kvstore

import (
	"strings"

	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/pkg/errors"
)

const (
	reportsKeyPrefix      = "reports_"
	reportStampsKeyPrefix = "ratelimit_report_"
	restrictionKeyPrefix  = "restrict_"
	banKeyPrefix          = "ban_"
	moderatorsKeyPrefix   = "mods_"
)

type Client struct {
	client *pluginapi.Client
}

func NewKVStore(client *pluginapi.Client) KVStore {
	return Client{
		client: client,
	}
}

func (kv Client) GetReports(channelID string) ([]*Report, error) {
	var reports []*Report
	if err := kv.client.KV.Get(reportsKeyPrefix+channelID, &reports); err != nil {
		return nil, errors.Wrap(err, "failed to get reports")
	}
	return reports, nil
}

func (kv Client) SaveReports(channelID string, reports []*Report) error {
	if _, err := kv.client.KV.Set(reportsKeyPrefix+channelID, reports); err != nil {
		return errors.Wrap(err, "failed to save reports")
	}
	return nil
}

func (kv Client) GetReportStamps(userID string) ([]int64, error) {
	var stamps []int64
	if err := kv.client.KV.Get(reportStampsKeyPrefix+userID, &stamps); err != nil {
		return nil, errors.Wrap(err, "failed to get report stamps")
	}
	return stamps, nil
}

func (kv Client) SaveReportStamps(userID string, stamps []int64) error {
	if _, err := kv.client.KV.Set(reportStampsKeyPrefix+userID, stamps); err != nil {
		return errors.Wrap(err, "failed to save report stamps")
	}
	return nil
}

func RestrictionKey(channelID, userID string) string {
	return restrictionKeyPrefix + channelID + "_" + userID
}

// IsRestrictionKey reports whether a KV key holds a restriction record.
func IsRestrictionKey(key string) bool {
	return strings.HasPrefix(key, restrictionKeyPrefix)
}

func (kv Client) GetRestriction(channelID, userID string) (*Restriction, error) {
	return kv.GetRestrictionByKey(RestrictionKey(channelID, userID))
}

func (kv Client) GetRestrictionByKey(key string) (*Restriction, error) {
	var restriction *Restriction
	if err := kv.client.KV.Get(key, &restriction); err != nil {
		return nil, errors.Wrap(err, "failed to get restriction")
	}
	return restriction, nil
}

func (kv Client) SetRestriction(channelID, userID string, restriction *Restriction) error {
	if _, err := kv.client.KV.Set(RestrictionKey(channelID, userID), restriction); err != nil {
		return errors.Wrap(err, "failed to set restriction")
	}
	return nil
}

func (kv Client) DeleteRestriction(channelID, userID string) error {
	return kv.DeleteByKey(RestrictionKey(channelID, userID))
}

func (kv Client) ListRestrictionKeys(page, perPage int) ([]string, bool, error) {
	keys, err := kv.client.KV.ListKeys(page, perPage)
	if err != nil {
		return nil, false, errors.Wrap(err, "failed to list keys")
	}
	restrictionKeys := []string{}
	for _, key := range keys {
		if IsRestrictionKey(key) {
			restrictionKeys = append(restrictionKeys, key)
		}
	}
	return restrictionKeys, len(keys) == perPage, nil
}

func (kv Client) DeleteByKey(key string) error {
	if err := kv.client.KV.Delete(key); err != nil {
		return errors.Wrap(err, "failed to delete key")
	}
	return nil
}

func (kv Client) GetBan(userID string) (*Ban, error) {
	var ban *Ban
	if err := kv.client.KV.Get(banKeyPrefix+userID, &ban); err != nil {
		return nil, errors.Wrap(err, "failed to get ban")
	}
	return ban, nil
}

func (kv Client) SetBan(userID string, ban *Ban) error {
	if _, err := kv.client.KV.Set(banKeyPrefix+userID, ban); err != nil {
		return errors.Wrap(err, "failed to set ban")
	}
	return nil
}

func (kv Client) DeleteBan(userID string) error {
	return kv.DeleteByKey(banKeyPrefix + userID)
}

func (kv Client) GetModerators(channelID string) ([]string, error) {
	var userIDs []string
	if err := kv.client.KV.Get(moderatorsKeyPrefix+channelID, &userIDs); err != nil {
		return nil, errors.Wrap(err, "failed to get moderators")
	}
	return userIDs, nil
}

func (kv Client) SaveModerators(channelID string, userIDs []string) error {
	if _, err := kv.client.KV.Set(moderatorsKeyPrefix+channelID, userIDs); err != nil {
		return errors.Wrap(err, "failed to save moderators")
	}
	return nil
}
