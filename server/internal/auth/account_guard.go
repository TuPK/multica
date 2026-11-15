package auth

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/platform-management/accountguard"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/redis/go-redis/v9"
)

// NewAccountGuard returns the account-state guard backed by Postgres and, when
// rdb is non-nil, Redis. It is not wired into any middleware yet.
func NewAccountGuard(q *db.Queries, rdb redis.UniversalClient) *accountguard.Guard {
	var cache accountguard.Cache = noCache{}
	if rdb != nil {
		cache = redisAccountCache{rdb: rdb}
	}
	return accountguard.New(accountStore{q: q}, cache)
}

type accountStore struct{ q *db.Queries }

func (s accountStore) AccountState(ctx context.Context, userID string) (accountguard.Record, bool, error) {
	uid, err := util.ParseUUID(userID)
	if err != nil {
		return accountguard.Record{}, false, err
	}
	row, err := s.q.AccountState(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return accountguard.Record{}, false, nil
	}
	if err != nil {
		return accountguard.Record{}, false, err
	}
	rec := accountguard.Record{Status: accountguard.State(row.Status)}
	if row.SessionsValidAfter.Valid {
		rec.SessionsValidAfter = row.SessionsValidAfter.Time
	}
	return rec, true, nil
}

type redisAccountCache struct{ rdb redis.UniversalClient }

func (c redisAccountCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	b, err := c.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return b, true, nil
}

func (c redisAccountCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return c.rdb.Set(ctx, key, value, ttl).Err()
}

func (c redisAccountCache) Del(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, key).Err()
}

// noCache is used when Redis is not configured: every read goes to Postgres.
type noCache struct{}

func (noCache) Get(context.Context, string) ([]byte, bool, error)        { return nil, false, nil }
func (noCache) Set(context.Context, string, []byte, time.Duration) error { return nil }
func (noCache) Del(context.Context, string) error                        { return nil }
