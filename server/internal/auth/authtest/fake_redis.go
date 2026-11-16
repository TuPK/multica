// Package authtest holds test doubles for the auth package's Redis-backed
// caches, so their tests run without a Redis server.
package authtest

import (
	"context"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// FakeRedis is an in-memory redis.UniversalClient that implements only the
// commands the auth caches use (GET, SET, DEL). Any other command panics on
// the nil embedded client, which flags a cache that grew a new dependency.
// TTLs are recorded but never expire an entry.
type FakeRedis struct {
	redis.UniversalClient

	mu   sync.Mutex
	data map[string]string
	ttl  map[string]time.Duration
}

// NewFakeRedis returns an empty fake.
func NewFakeRedis() *FakeRedis {
	return &FakeRedis{data: map[string]string{}, ttl: map[string]time.Duration{}}
}

// Get implements redis.UniversalClient.
func (f *FakeRedis) Get(_ context.Context, key string) *redis.StringCmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.data[key]
	if !ok {
		return redis.NewStringResult("", redis.Nil)
	}
	return redis.NewStringResult(v, nil)
}

// Set implements redis.UniversalClient. Values are stored as strings.
func (f *FakeRedis) Set(_ context.Context, key string, value any, ttl time.Duration) *redis.StatusCmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch v := value.(type) {
	case string:
		f.data[key] = v
	case []byte:
		f.data[key] = string(v)
	default:
		panic("authtest.FakeRedis: unsupported value type")
	}
	f.ttl[key] = ttl
	return redis.NewStatusResult("OK", nil)
}

// Del implements redis.UniversalClient.
func (f *FakeRedis) Del(_ context.Context, keys ...string) *redis.IntCmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	var n int64
	for _, k := range keys {
		if _, ok := f.data[k]; ok {
			n++
		}
		delete(f.data, k)
		delete(f.ttl, k)
	}
	return redis.NewIntResult(n, nil)
}

// Raw returns the stored string for key.
func (f *FakeRedis) Raw(key string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.data[key]
	return v, ok
}

// Put stores a raw value under key, as another process would have.
func (f *FakeRedis) Put(key, value string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[key] = value
}

// StoredKeys returns every stored key.
func (f *FakeRedis) StoredKeys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.data))
	for k := range f.data {
		out = append(out, k)
	}
	return out
}
