package accountguard

import (
	"context"
	"testing"
	"time"
)

// ttlCache is a cache whose entries expire by an injected clock, so the test
// can move time past the TTL the guard asked for.
type ttlCache struct {
	now     time.Time
	vals    map[string][]byte
	expires map[string]time.Time
}

func (c *ttlCache) Get(_ context.Context, k string) ([]byte, bool, error) {
	exp, ok := c.expires[k]
	if !ok || !c.now.Before(exp) {
		return nil, false, nil
	}
	return c.vals[k], true, nil
}

func (c *ttlCache) Set(_ context.Context, k string, v []byte, ttl time.Duration) error {
	c.vals[k] = v
	c.expires[k] = c.now.Add(ttl)
	return nil
}

func (c *ttlCache) Del(_ context.Context, k string) error {
	delete(c.vals, k)
	delete(c.expires, k)
	return nil
}

type countingStore struct {
	rec   Record
	reads int
}

func (s *countingStore) AccountState(context.Context, string) (Record, bool, error) {
	s.reads++
	return s.rec, true, nil
}

// socketRecheckInterval mirrors accountStatusRecheckInterval in
// server/internal/realtime. It is not shared across modules, so this test pins
// the platform side only: the server's constant must stay at or below this.
const socketRecheckInterval = 20 * time.Second

// Spec: a cached account state is stale for at most 30 seconds when
// invalidation fails, so a socket recheck every 20s closes a SQL-suspended
// account's socket within the 60-second bound.
func TestGuardCacheStalenessIsBoundedByTTL(t *testing.T) {
	if TTL > 30*time.Second {
		t.Fatalf("TTL = %s, want <= 30s", TTL)
	}
	if TTL+socketRecheckInterval > 60*time.Second {
		t.Fatalf("TTL + recheck interval = %s, spec bound is 60s", TTL+socketRecheckInterval)
	}
	ctx := context.Background()
	cache := &ttlCache{now: time.Unix(1_700_000_000, 0), vals: map[string][]byte{}, expires: map[string]time.Time{}}
	store := &countingStore{rec: Record{Status: Active}}
	g := New(store, cache)

	if rec, _ := g.State(ctx, "u1"); rec.Status != Active {
		t.Fatalf("first read = %q, want active", rec.Status)
	}
	// The account is suspended behind the cache's back (a failed invalidation).
	store.rec = Record{Status: Suspended}

	cache.now = cache.now.Add(TTL - time.Second)
	if rec, _ := g.State(ctx, "u1"); rec.Status != Active || store.reads != 1 {
		t.Fatalf("inside the TTL: status %q, store reads %d; want the cached active state", rec.Status, store.reads)
	}

	cache.now = cache.now.Add(time.Second)
	if rec, _ := g.State(ctx, "u1"); rec.Status != Suspended || store.reads != 2 {
		t.Fatalf("at the TTL: status %q, store reads %d; want a fresh suspended read", rec.Status, store.reads)
	}
}
