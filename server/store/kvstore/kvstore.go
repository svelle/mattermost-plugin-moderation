package kvstore

// KVStore is the data-access layer for all moderation state kept in the
// plugin KV store. Reports are stored as one list per channel, restrictions
// (mutes and timeouts) per channel and user, bans per user, and the
// moderator list per channel.
type KVStore interface {
	GetReports(channelID string) ([]*Report, error)
	SaveReports(channelID string, reports []*Report) error

	GetRestriction(channelID, userID string) (*Restriction, error)
	SetRestriction(channelID, userID string, restriction *Restriction) error
	DeleteRestriction(channelID, userID string) error
	ListRestrictionKeys(page, perPage int) (keys []string, hasMore bool, err error)
	GetRestrictionByKey(key string) (*Restriction, error)
	DeleteByKey(key string) error

	GetBan(userID string) (*Ban, error)
	SetBan(userID string, ban *Ban) error
	DeleteBan(userID string) error

	GetModerators(channelID string) ([]string, error)
	SaveModerators(channelID string, userIDs []string) error
}
