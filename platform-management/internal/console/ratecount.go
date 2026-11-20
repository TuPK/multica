package console

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RateCounter reads the API requests a workspace made in the minute that holds
// now (spec R6 section 3b.4). The counter lives in Redis and keeps no history.
type RateCounter interface {
	RateCurrent(ctx context.Context, workspaceID string, now time.Time) (int64, error)
}

// RedisRateCounter reads the minute counters the Multica API increments.
type RedisRateCounter struct{ RDB redis.UniversalClient }

// RateCurrent returns the requests of the workspace this minute; a minute with no
// requests has no key, and its count is zero.
func (c RedisRateCounter) RateCurrent(ctx context.Context, workspaceID string, now time.Time) (int64, error) {
	n, err := c.RDB.Get(ctx, rateKey(workspaceID, now)).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read api rate: %w", err)
	}
	return n, nil
}

// rateKey is the key of the workspace's counter for the minute of now. It must
// match the key the Multica API increments.
func rateKey(workspaceID string, now time.Time) string {
	return fmt.Sprintf("mul:rl:ws:%s:%d", workspaceID, now.Unix()/60)
}
