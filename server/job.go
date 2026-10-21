package main

// runJob removes expired timeout records. Expired timeouts already stop
// being enforced the moment they lapse — this is housekeeping so the KV
// store and the members tab don't accumulate stale records.
func (p *Plugin) runJob() {
	now := p.now()
	// Collect expired keys before deleting any: deleting while paging
	// shifts later keys onto pages that were already read, skipping them.
	var expired []string
	for page := range 100 {
		keys, hasMore, err := p.kvstore.ListRestrictionKeys(page, 100)
		if err != nil {
			p.API.LogError("Failed to list restriction keys", "error", err.Error())
			return
		}
		for _, key := range keys {
			restriction, err := p.kvstore.GetRestrictionByKey(key)
			if err != nil || restriction == nil {
				continue
			}
			if !restriction.Muted && restriction.TimeoutUntil != 0 && !restriction.TimedOutNow(now) {
				expired = append(expired, key)
			}
		}
		if !hasMore {
			break
		}
	}
	for _, key := range expired {
		if err := p.kvstore.DeleteByKey(key); err != nil {
			p.API.LogError("Failed to delete expired timeout", "key", key, "error", err.Error())
		}
	}
}
