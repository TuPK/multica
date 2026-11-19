// Package accountguard answers "what is this account's state?" with a
// cache-aside lookup. It does not block requests itself; the server decides
// what to do with the answer. Standard library only.
package accountguard

import (
	"context"
	"encoding/json"
	"time"
)

// State is the account lifecycle status.
type State string

const (
	Active          State = "active"
	Suspended       State = "suspended"
	PendingDeletion State = "pending_deletion"
	Deleted         State = "deleted"
)

// TTL bounds how long a cached state can be stale if invalidation fails, or
// if the status was changed in SQL with no invalidation at all. An open
// WebSocket rechecks every 20s (server/internal/realtime), so a SQL change
// closes it within TTL + 20s = 50s, inside the 60-second bound.
const TTL = 30 * time.Second

const keyPrefix = "mul:auth:acct:"

// Record is the state the guard acts on. A zero SessionsValidAfter means no
// revocation mark.
type Record struct {
	Status             State
	SessionsValidAfter time.Time
}

// Store reads account state. found == false means no row, which is Active.
type Store interface {
	AccountState(ctx context.Context, userID string) (rec Record, found bool, err error)
}

// Cache is a byte cache with TTL.
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Del(ctx context.Context, key string) error
}

// Guard is a cache-aside reader of account state.
type Guard struct {
	store Store
	cache Cache
}

// New returns a Guard over store and cache.
func New(store Store, cache Cache) *Guard {
	return &Guard{store: store, cache: cache}
}

type cached struct {
	Found              bool      `json:"found"`
	Status             State     `json:"status,omitempty"`
	SessionsValidAfter time.Time `json:"sessions_valid_after,omitempty"`
}

func key(userID string) string { return keyPrefix + userID }

func activeRecord() Record { return Record{Status: Active} }

// State returns the account's state. Cache failures are ignored; store
// failures are returned.
func (g *Guard) State(ctx context.Context, userID string) (Record, error) {
	k := key(userID)
	if b, ok, err := g.cache.Get(ctx, k); err == nil && ok {
		var c cached
		if json.Unmarshal(b, &c) == nil {
			if !c.Found {
				return activeRecord(), nil
			}
			return Record{Status: c.Status, SessionsValidAfter: c.SessionsValidAfter}, nil
		}
	}

	rec, found, err := g.store.AccountState(ctx, userID)
	if err != nil {
		return Record{}, err
	}
	if !found {
		rec = activeRecord()
	}
	c := cached{Found: found}
	if found {
		c.Status = rec.Status
		c.SessionsValidAfter = rec.SessionsValidAfter
	}
	if b, err := json.Marshal(c); err == nil {
		_ = g.cache.Set(ctx, k, b, TTL)
	}
	return rec, nil
}

// Invalidate drops the cached state. The caller logs the error and does not
// fail the write; the stale window is at most TTL.
func (g *Guard) Invalidate(ctx context.Context, userID string) error {
	return g.cache.Del(ctx, key(userID))
}
