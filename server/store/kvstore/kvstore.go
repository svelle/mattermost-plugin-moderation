package kvstore

import "time"

// KVStore is the data-access layer for all moderation state kept in the
// plugin KV store. Reports are stored as one list per channel, restrictions
// (mutes and timeouts) per channel and user, bans per user, and the
// moderator list per channel.
type KVStore interface {
	GetReports(channelID string) ([]*Report, error)
	SaveReports(channelID string, reports []*Report) error

	// Report timestamps per reporter, used for rate limiting.
	GetReportStamps(userID string) ([]int64, error)
	SaveReportStamps(userID string, stamps []int64) error

	GetRestriction(channelID, userID string) (*Restriction, error)
	SetRestriction(channelID, userID string, restriction *Restriction) error
	DeleteRestriction(channelID, userID string) error
	ListRestrictionKeys(page, perPage int) (keys []string, hasMore bool, err error)
	GetRestrictionByKey(key string) (*Restriction, error)
	DeleteByKey(key string) error

	// ListKeys returns one page of every key the plugin has stored.
	ListKeys(page, perPage int) ([]string, error)

	GetBan(userID string) (*Ban, error)
	SetBan(userID string, ban *Ban) error
	DeleteBan(userID string) error

	// Conversations or channels a member started posting in recently, per
	// spam limit scope, kept for ttl after the last change, and the block
	// that stops them posting in new ones.
	GetSpamContacts(scope, userID string) ([]SpamContact, error)
	SaveSpamContacts(scope, userID string, contacts []SpamContact, ttl time.Duration) error
	GetSpamBlock(scope, userID string) (*SpamBlock, error)
	// TrySetSpamBlock stores the block only if none exists, and reports
	// whether this call created it.
	TrySetSpamBlock(scope, userID string, block *SpamBlock, ttl time.Duration) (bool, error)

	GetModerators(channelID string) ([]string, error)
	SaveModerators(channelID string, userIDs []string) error
}
