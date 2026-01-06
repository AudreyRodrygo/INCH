package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Deduplicator checks whether an event has already been processed.
//
// In a distributed system with at-least-once delivery, the same event
// may arrive multiple times (agent retries, network issues, Kafka redelivery).
// The deduplicator uses Redis SET NX (Set if Not eXists) to detect duplicates.
//
// How it works:
//
//	First time event_id="abc" arrives:
//	  SET "dedup:abc" "1" NX EX 300  →  OK (key created)  →  NOT a duplicate
//
//	Same event_id="abc" arrives again within 5 minutes:
//	  SET "dedup:abc" "1" NX EX 300  →  nil (key exists)  →  IS a duplicate
//
// The TTL ensures keys expire automatically — no manual cleanup needed.
// 5 minutes is enough because events older than that are considered stale
// and won't arrive as retries.
type Deduplicator struct {
	client *redis.Client
	ttl    time.Duration
}

// NewDeduplicator creates a deduplicator backed by Redis.
func NewDeduplicator(client *redis.Client, ttlSeconds int) *Deduplicator {
	ttl := time.Duration(ttlSeconds) * time.Second
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}

	return &Deduplicator{
		client: client,
		ttl:    ttl,
	}
}

// IsDuplicate returns true if the event ID has been seen before.
//
// Uses Redis SET NX (set-if-not-exists) with a TTL:
//   - If the key doesn't exist: creates it, returns false (new event)
//   - If the key already exists: returns true (duplicate)
//
// This is atomic — even with concurrent calls, only one will "win" and
// set the key. All others will see it as a duplicate. This is why Redis
// SET NX is the standard approach for distributed deduplication.
func (d *Deduplicator) IsDuplicate(ctx context.Context, eventID string) (bool, error) {
	// If Redis is not connected, skip dedup (accept all events).
	if d.client == nil {
		return false, nil
	}

	key := fmt.Sprintf("dedup:%s", eventID)

	// SetArgs with NX mode: set only if key does not exist, with expiration.
	// Returns "OK" if the key was created (new event), redis.Nil if it already existed.
	result, err := d.client.SetArgs(ctx, key, "1", redis.SetArgs{
		Mode: "NX",
		TTL:  d.ttl,
	}).Result()
	if err != nil {
		// redis.Nil means key already exists (NX condition failed) — this is a duplicate.
		if err == redis.Nil {
			return true, nil
		}
		return false, fmt.Errorf("checking dedup for %s: %w", eventID, err)
	}

	// result="OK" means key was created → event is NEW (not duplicate).
	return result != "OK", nil
}
