package platformauth

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCodes keeps the sign-in codes in Redis: one hash per email hash, with
// the HMAC of the code and an attempt counter, expiring with the code.
type RedisCodes struct{ rdb redis.Cmdable }

// NewRedisCodes returns a CodeStore on rdb.
func NewRedisCodes(rdb redis.Cmdable) *RedisCodes { return &RedisCodes{rdb: rdb} }

func codeKey(emailHash string) string { return "mpa:code:" + emailHash }

// attemptScript counts an attempt and returns the stored value, in one step so
// that parallel guesses each get their own attempt number.
var attemptScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 then
    return false
end
local n = redis.call('HINCRBY', KEYS[1], 'n', 1)
return {redis.call('HGET', KEYS[1], 'h'), n}
`)

func (c *RedisCodes) Save(ctx context.Context, emailHash string, mac []byte, ttl time.Duration) error {
	key := codeKey(emailHash)
	pipe := c.rdb.TxPipeline()
	pipe.Del(ctx, key)
	pipe.HSet(ctx, key, "h", hex.EncodeToString(mac), "n", 0)
	pipe.Expire(ctx, key, ttl)
	_, err := pipe.Exec(ctx)
	return err
}

func (c *RedisCodes) Attempt(ctx context.Context, emailHash string) ([]byte, int, bool, error) {
	res, err := attemptScript.Run(ctx, c.rdb, []string{codeKey(emailHash)}).Slice()
	if errors.Is(err, redis.Nil) {
		return nil, 0, false, nil
	}
	if err != nil {
		return nil, 0, false, err
	}
	if len(res) != 2 {
		return nil, 0, false, fmt.Errorf("unexpected script result of length %d", len(res))
	}
	encoded, _ := res[0].(string)
	n, _ := res[1].(int64)
	mac, err := hex.DecodeString(encoded)
	if err != nil {
		return nil, 0, false, errors.New("stored code is not hex")
	}
	return mac, int(n), true, nil
}

func (c *RedisCodes) Delete(ctx context.Context, emailHash string) error {
	return c.rdb.Del(ctx, codeKey(emailHash)).Err()
}

// RedisLimiter is a fixed-window counter in Redis.
type RedisLimiter struct{ rdb redis.Cmdable }

// NewRedisLimiter returns a Limiter on rdb.
func NewRedisLimiter(rdb redis.Cmdable) *RedisLimiter { return &RedisLimiter{rdb: rdb} }

// windowScript sets the expiry together with the first increment, so a crash
// between the two cannot leave a counter that never expires.
var windowScript = redis.NewScript(`
local n = redis.call('INCR', KEYS[1])
if n == 1 then
    redis.call('EXPIRE', KEYS[1], ARGV[1])
end
return n
`)

func (l *RedisLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	n, err := windowScript.Run(ctx, l.rdb, []string{key}, int(window.Seconds())).Int64()
	if err != nil {
		return false, err
	}
	return n <= int64(limit), nil
}

// RedisAccountCache is the accountguard cache on Redis. It uses the same keys
// as the Multica server, so a write on either side invalidates both.
type RedisAccountCache struct{ rdb redis.Cmdable }

// NewRedisAccountCache returns an accountguard.Cache on rdb.
func NewRedisAccountCache(rdb redis.Cmdable) *RedisAccountCache { return &RedisAccountCache{rdb: rdb} }

func (c *RedisAccountCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	b, err := c.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return b, true, nil
}

func (c *RedisAccountCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return c.rdb.Set(ctx, key, value, ttl).Err()
}

func (c *RedisAccountCache) Del(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, key).Err()
}
