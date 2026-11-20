// Package runtimeguard tells whether platform staff disabled a runtime (spec R6
// section 3b.5, D98). A disabled runtime cannot register, heartbeat or claim
// tasks. The answer is a cache-aside read of platform_runtime: a cached answer
// lasts at most TTL, and the platform deletes the key when it disables or
// enables a runtime, so a change takes effect on the next request.
package runtimeguard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"

	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// TTL bounds how long a cached answer can be stale if the platform's key delete
// fails (spec R6 AC127: the change applies within 60 seconds even when the cache
// is broken).
const TTL = 60 * time.Second

const keyPrefix = "mul:plat:rt:"

// Key is the cache key of a runtime's state. The platform deletes it on every
// disable and enable.
func Key(runtimeID pgtype.UUID) string { return keyPrefix + util.UUIDToString(runtimeID) }

// Querier is the one read the guard makes; *db.Queries satisfies it.
type Querier interface {
	GetPlatformRuntimeDisabled(ctx context.Context, runtimeID pgtype.UUID) (bool, error)
}

// Cache is a byte cache with TTL.
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Del(ctx context.Context, key string) error
}

// Guard answers whether a runtime is disabled.
type Guard struct {
	q     Querier
	cache Cache
}

// New returns a Guard over q and, when rdb is non-nil, Redis. Without Redis
// every answer is read from Postgres.
func New(q Querier, rdb redis.UniversalClient) *Guard {
	var cache Cache = noCache{}
	if rdb != nil {
		cache = redisCache{rdb: rdb}
	}
	return newGuard(q, cache)
}

func newGuard(q Querier, cache Cache) *Guard {
	return &Guard{q: q, cache: cache}
}

type cached struct {
	Disabled bool `json:"disabled"`
}

// Disabled reports whether staff disabled the runtime. A database error is
// returned as is: the guard fails closed, so the caller answers 503 and does
// not guess. Cache errors are ignored and fall back to the database.
func (g *Guard) Disabled(ctx context.Context, runtimeID pgtype.UUID) (bool, error) {
	k := Key(runtimeID)
	if b, ok, err := g.cache.Get(ctx, k); err == nil && ok {
		var c cached
		if json.Unmarshal(b, &c) == nil {
			return c.Disabled, nil
		}
	}
	disabled, err := g.q.GetPlatformRuntimeDisabled(ctx, runtimeID)
	if err != nil {
		return false, fmt.Errorf("read platform runtime state: %w", err)
	}
	if b, err := json.Marshal(cached{Disabled: disabled}); err == nil {
		_ = g.cache.Set(ctx, k, b, TTL)
	}
	return disabled, nil
}

// redisCache is the Cache over Redis.
type redisCache struct{ rdb redis.UniversalClient }

func (c redisCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	b, err := c.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return b, true, nil
}

func (c redisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return c.rdb.Set(ctx, key, value, ttl).Err()
}

func (c redisCache) Del(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, key).Err()
}

// noCache is used when Redis is not configured.
type noCache struct{}

func (noCache) Get(context.Context, string) ([]byte, bool, error)        { return nil, false, nil }
func (noCache) Set(context.Context, string, []byte, time.Duration) error { return nil }
func (noCache) Del(context.Context, string) error                        { return nil }

var _ Querier = (*db.Queries)(nil)
