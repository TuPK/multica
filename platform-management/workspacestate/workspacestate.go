// Package workspacestate answers "what is this workspace's state?" with a
// cache-aside lookup. It does not block requests or claims itself; the server
// decides what to do with the answer. Standard library only.
package workspacestate

import (
	"context"
	"encoding/json"
	"time"
)

// Status is the workspace lifecycle status.
type Status string

const (
	Active          Status = "active"
	ReadOnly        Status = "read_only"
	Suspended       Status = "suspended"
	PendingDeletion Status = "pending_deletion"
)

// TTL bounds how long a cached state can be stale if invalidation fails.
const TTL = 60 * time.Second

const keyPrefix = "mul:plat:ws:"

// Key is the cache key of a workspace's state.
func Key(workspaceID string) string { return keyPrefix + workspaceID }

// Record is the state the guard acts on. A zero DeletionScheduledAt means no
// deletion is scheduled.
type Record struct {
	Status              Status
	DeletionScheduledAt time.Time
}

// Store reads workspace state. found == false means no row, which is Active.
type Store interface {
	WorkspaceState(ctx context.Context, workspaceID string) (rec Record, found bool, err error)
}

// Cache is a byte cache with TTL.
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Del(ctx context.Context, key string) error
}

// Guard is a cache-aside reader of workspace state.
type Guard struct {
	store Store
	cache Cache
}

// New returns a Guard over store and cache.
func New(store Store, cache Cache) *Guard {
	return &Guard{store: store, cache: cache}
}

type cached struct {
	Found               bool      `json:"found"`
	Status              Status    `json:"status,omitempty"`
	DeletionScheduledAt time.Time `json:"deletion_scheduled_at,omitempty"`
}

func activeRecord() Record { return Record{Status: Active} }

// State returns the workspace's state. Cache failures are ignored; store
// failures are returned.
func (g *Guard) State(ctx context.Context, workspaceID string) (Record, error) {
	k := Key(workspaceID)
	if b, ok, err := g.cache.Get(ctx, k); err == nil && ok {
		var c cached
		if json.Unmarshal(b, &c) == nil {
			if !c.Found {
				return activeRecord(), nil
			}
			return Record{Status: c.Status, DeletionScheduledAt: c.DeletionScheduledAt}, nil
		}
	}

	rec, found, err := g.store.WorkspaceState(ctx, workspaceID)
	if err != nil {
		return Record{}, err
	}
	if !found {
		rec = activeRecord()
	}
	c := cached{Found: found}
	if found {
		c.Status = rec.Status
		c.DeletionScheduledAt = rec.DeletionScheduledAt
	}
	if b, err := json.Marshal(c); err == nil {
		_ = g.cache.Set(ctx, k, b, TTL)
	}
	return rec, nil
}

// Invalidate drops the cached state. The caller logs the error and does not
// fail the write; the stale window is at most TTL.
func (g *Guard) Invalidate(ctx context.Context, workspaceID string) error {
	return g.cache.Del(ctx, Key(workspaceID))
}
