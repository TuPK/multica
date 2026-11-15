// Package accountcache is the platform's Redis side of account changes: the
// cache behind accountguard.Guard, and the message that tells the Multica
// server to close a user's open sockets.
package accountcache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Channel is the pub/sub channel the Multica server subscribes to. The server
// declares the same name in server/internal/realtime/account_changes.go; the
// two modules cannot share a constant because this package is internal.
const Channel = "mul:acct:changed"

// Message kinds.
const (
	// KindStatus: the account's state changed.
	KindStatus = "status"
	// KindSessions: the account's sessions were revoked.
	KindSessions = "sessions"
)

// Message is the payload on Channel.
type Message struct {
	UserID string `json:"user_id"`
	Kind   string `json:"kind"`
}

// Commands is the part of a Redis client that Cache uses. redis.Cmdable
// satisfies it.
type Commands interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
}

// Cache is the accountguard.Cache on Redis. It uses the keys the Multica
// server reads, so a write on either side invalidates both.
type Cache struct{ rdb Commands }

// NewCache returns a Cache on rdb.
func NewCache(rdb Commands) *Cache { return &Cache{rdb: rdb} }

func (c *Cache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	b, err := c.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return b, true, nil
}

func (c *Cache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return c.rdb.Set(ctx, key, value, ttl).Err()
}

func (c *Cache) Del(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, key).Err()
}

// Client is the part of a Redis client that Publisher uses. redis.Cmdable
// satisfies it.
type Client interface {
	Publish(ctx context.Context, channel string, message any) *redis.IntCmd
}

// Publisher writes account-change messages.
type Publisher struct{ rdb Client }

// NewPublisher returns a Publisher on rdb.
func NewPublisher(rdb Client) *Publisher { return &Publisher{rdb: rdb} }

// Publish tells every Multica server node that the user's account changed. It
// returns an error for an empty user id or an unknown kind, and when Redis
// refuses the message.
func (p *Publisher) Publish(ctx context.Context, userID, kind string) error {
	if userID == "" {
		return errors.New("accountcache: user id is required")
	}
	if kind != KindStatus && kind != KindSessions {
		return fmt.Errorf("accountcache: unknown kind %q", kind)
	}
	payload, err := json.Marshal(Message{UserID: userID, Kind: kind})
	if err != nil {
		return err
	}
	return p.rdb.Publish(ctx, Channel, payload).Err()
}
