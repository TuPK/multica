package workspaceguard

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"github.com/multica-ai/multica/platform-management/workspacestate"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// New returns the workspace-state guard backed by Postgres and, when rdb is
// non-nil, Redis under the key workspacestate.Key.
func New(q *db.Queries, rdb redis.UniversalClient) *workspacestate.Guard {
	var cache workspacestate.Cache = noCache{}
	if rdb != nil {
		cache = NewRedisCache(rdb)
	}
	return workspacestate.New(NewStore(q), cache)
}

// Store reads platform_workspace. It is the workspacestate.Store.
type Store struct{ q *db.Queries }

// NewStore returns a Store over q.
func NewStore(q *db.Queries) *Store { return &Store{q: q} }

// WorkspaceState implements workspacestate.Store. The platform writes the table
// and the server only reads it.
func (s *Store) WorkspaceState(ctx context.Context, workspaceID string) (workspacestate.Record, bool, error) {
	id, err := util.ParseUUID(workspaceID)
	if err != nil {
		return workspacestate.Record{}, false, err
	}
	row, err := s.q.WorkspaceState(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return workspacestate.Record{}, false, nil
	}
	if err != nil {
		return workspacestate.Record{}, false, err
	}
	rec := workspacestate.Record{Status: workspacestate.Status(row.Status)}
	if row.DeletionScheduledAt.Valid {
		rec.DeletionScheduledAt = row.DeletionScheduledAt.Time
	}
	return rec, true, nil
}

// RedisCache is the workspacestate.Cache over Redis.
type RedisCache struct{ rdb redis.UniversalClient }

// NewRedisCache returns a cache over rdb.
func NewRedisCache(rdb redis.UniversalClient) RedisCache { return RedisCache{rdb: rdb} }

func (c RedisCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	b, err := c.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return b, true, nil
}

func (c RedisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return c.rdb.Set(ctx, key, value, ttl).Err()
}

func (c RedisCache) Del(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, key).Err()
}

// noCache is used when Redis is not configured: every read goes to Postgres.
type noCache struct{}

func (noCache) Get(context.Context, string) ([]byte, bool, error)        { return nil, false, nil }
func (noCache) Set(context.Context, string, []byte, time.Duration) error { return nil }
func (noCache) Del(context.Context, string) error                        { return nil }
