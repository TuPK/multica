package accountguard

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeStore struct {
	rec   Record
	found bool
	err   error
	calls int
}

func (f *fakeStore) AccountState(context.Context, string) (Record, bool, error) {
	f.calls++
	return f.rec, f.found, f.err
}

type fakeCache struct {
	m      map[string][]byte
	getErr error
	setErr error
	delErr error
	ttl    time.Duration
}

func newCache() *fakeCache { return &fakeCache{m: map[string][]byte{}} }

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
	if c.delErr != nil {
		return c.delErr
	}
	delete(c.m, k)
	return nil
}

func TestMissReadsStoreThenHitsCache(t *testing.T) {
	mark := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	s := &fakeStore{rec: Record{Status: Suspended, SessionsValidAfter: mark}, found: true}
	c := newCache()
	g := New(s, c)
	for i := 0; i < 2; i++ {
		rec, err := g.State(context.Background(), "u1")
		if err != nil || rec.Status != Suspended || !rec.SessionsValidAfter.Equal(mark) {
			t.Fatalf("got %+v %v", rec, err)
		}
	}
	if s.calls != 1 {
		t.Fatalf("store calls = %d, want 1", s.calls)
	}
	if _, ok := c.m["mul:auth:acct:u1"]; !ok || c.ttl != 60*time.Second {
		t.Fatalf("cache key/ttl wrong: %v %v", c.m, c.ttl)
	}
}

func TestNoRowIsActiveAndNegativelyCached(t *testing.T) {
	s := &fakeStore{}
	g := New(s, newCache())
	for i := 0; i < 2; i++ {
		rec, err := g.State(context.Background(), "u1")
		if err != nil || rec.Status != Active || !rec.SessionsValidAfter.IsZero() {
			t.Fatalf("got %+v %v", rec, err)
		}
	}
	if s.calls != 1 {
		t.Fatalf("store calls = %d, want 1", s.calls)
	}
}

func TestCacheFailuresDoNotFail(t *testing.T) {
	s := &fakeStore{rec: Record{Status: Deleted}, found: true}
	c := newCache()
	c.getErr, c.setErr = errors.New("get"), errors.New("set")
	rec, err := New(s, c).State(context.Background(), "u1")
	if err != nil || rec.Status != Deleted {
		t.Fatalf("got %+v %v", rec, err)
	}
}

func TestCorruptCacheFallsBackToStore(t *testing.T) {
	s := &fakeStore{rec: Record{Status: PendingDeletion}, found: true}
	c := newCache()
	c.m["mul:auth:acct:u1"] = []byte("not json")
	rec, err := New(s, c).State(context.Background(), "u1")
	if err != nil || rec.Status != PendingDeletion || s.calls != 1 {
		t.Fatalf("got %+v %v calls=%d", rec, err, s.calls)
	}
}

func TestStoreErrorReturned(t *testing.T) {
	boom := errors.New("db down")
	_, err := New(&fakeStore{err: boom}, newCache()).State(context.Background(), "u1")
	if !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
}

func TestInvalidate(t *testing.T) {
	s := &fakeStore{rec: Record{Status: Suspended}, found: true}
	c := newCache()
	g := New(s, c)
	_, _ = g.State(context.Background(), "u1")
	if err := g.Invalidate(context.Background(), "u1"); err != nil {
		t.Fatal(err)
	}
	if len(c.m) != 0 {
		t.Fatal("key not deleted")
	}
	_, _ = g.State(context.Background(), "u1")
	if s.calls != 2 {
		t.Fatalf("store calls = %d, want 2", s.calls)
	}
	boom := errors.New("del")
	c.delErr = boom
	if err := g.Invalidate(context.Background(), "u1"); !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
}

// Pins the spec bound (AC1): a state change is seen within 60 seconds.
func TestCacheTTLIsAtMostSixtySeconds(t *testing.T) {
	if TTL <= 0 || TTL > 60*time.Second {
		t.Fatalf("TTL = %s, want > 0 and <= 60s", TTL)
	}
}
