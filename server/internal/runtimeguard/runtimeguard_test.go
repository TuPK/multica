package runtimeguard

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type fakeQuerier struct {
	disabled bool
	err      error
	calls    int
}

func (f *fakeQuerier) GetPlatformRuntimeDisabled(context.Context, pgtype.UUID) (bool, error) {
	f.calls++
	return f.disabled, f.err
}

type fakeCache struct {
	m      map[string][]byte
	ttl    time.Duration
	getErr error
	setErr error
}

func newFakeCache() *fakeCache { return &fakeCache{m: map[string][]byte{}} }

func (c *fakeCache) Get(_ context.Context, k string) ([]byte, bool, error) {
	if c.getErr != nil {
		return nil, false, c.getErr
	}
	v, ok := c.m[k]
	return v, ok, nil
}

func (c *fakeCache) Set(_ context.Context, k string, v []byte, ttl time.Duration) error {
	if c.setErr != nil {
		return c.setErr
	}
	c.m[k], c.ttl = v, ttl
	return nil
}

func (c *fakeCache) Del(_ context.Context, k string) error {
	delete(c.m, k)
	return nil
}

var testID = pgtype.UUID{Bytes: [16]byte{1, 2, 3}, Valid: true}

func TestDisabledReportsStaffState(t *testing.T) {
	ctx := context.Background()
	if got, err := newGuard(&fakeQuerier{disabled: true}, noCache{}).Disabled(ctx, testID); err != nil || !got {
		t.Fatalf("Disabled = %v, %v; want true, nil", got, err)
	}
	if got, err := newGuard(&fakeQuerier{}, noCache{}).Disabled(ctx, testID); err != nil || got {
		t.Fatalf("Disabled = %v, %v; want false, nil", got, err)
	}
}

// A read error is returned, never read as enabled: the caller must fail closed.
func TestDisabledReturnsReadErrorInsteadOfGuessing(t *testing.T) {
	boom := errors.New("connection reset")
	got, err := newGuard(&fakeQuerier{err: boom}, noCache{}).Disabled(context.Background(), testID)
	if !errors.Is(err, boom) || got {
		t.Fatalf("Disabled = %v, %v; want false with the read error", got, err)
	}
}

// A hit answers from the cache, for both states, so a repeated daemon request
// does not read Postgres.
func TestDisabledCachesBothStatesForTTL(t *testing.T) {
	ctx := context.Background()
	for _, disabled := range []bool{true, false} {
		q := &fakeQuerier{disabled: disabled}
		c := newFakeCache()
		g := newGuard(q, c)
		for i := 0; i < 3; i++ {
			got, err := g.Disabled(ctx, testID)
			if err != nil || got != disabled {
				t.Fatalf("disabled=%v round %d: got %v, %v", disabled, i, got, err)
			}
		}
		if q.calls != 1 {
			t.Fatalf("disabled=%v: store calls = %d, want 1", disabled, q.calls)
		}
		if _, ok := c.m[Key(testID)]; !ok || c.ttl != TTL {
			t.Fatalf("disabled=%v: key or ttl wrong: %v %v", disabled, c.m, c.ttl)
		}
	}
}

// A cached answer is replaced after the platform deletes the key, so a disable
// or enable takes effect on the next request.
func TestDisableTakesEffectAfterInvalidation(t *testing.T) {
	ctx := context.Background()
	q := &fakeQuerier{}
	c := newFakeCache()
	g := newGuard(q, c)
	if got, _ := g.Disabled(ctx, testID); got {
		t.Fatal("enabled runtime reported disabled")
	}
	q.disabled = true
	if got, _ := g.Disabled(ctx, testID); got {
		t.Fatal("within the TTL the cached enabled state should answer")
	}
	c.Del(ctx, Key(testID))
	if got, err := g.Disabled(ctx, testID); err != nil || !got {
		t.Fatalf("after invalidation: got %v, %v; want disabled", got, err)
	}
}

// A broken cache is ignored: the store answers.
func TestCacheErrorsFallBackToStore(t *testing.T) {
	q := &fakeQuerier{disabled: true}
	c := newFakeCache()
	c.getErr = errors.New("redis down")
	c.setErr = errors.New("redis down")
	got, err := newGuard(q, c).Disabled(context.Background(), testID)
	if err != nil || !got {
		t.Fatalf("got %v, %v; want disabled, nil", got, err)
	}
	if q.calls != 1 {
		t.Fatalf("store calls = %d, want 1", q.calls)
	}
}

// A failed read is not cached: the next request asks the store again.
func TestStoreErrorIsNotCached(t *testing.T) {
	q := &fakeQuerier{err: errors.New("boom")}
	c := newFakeCache()
	g := newGuard(q, c)
	g.Disabled(context.Background(), testID)
	if _, ok := c.m[Key(testID)]; ok {
		t.Fatal("a failed read was cached")
	}
}
