package workspacestate

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

func (f *fakeStore) WorkspaceState(context.Context, string) (Record, bool, error) {
	f.calls++
	return f.rec, f.found, f.err
}

type fakeCache struct {
	m      map[string][]byte
	getErr error
	setErr error
	delErr error
	ttl    time.Duration
	dels   []string
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
	c.dels = append(c.dels, k)
	if c.delErr != nil {
		return c.delErr
	}
	delete(c.m, k)
	return nil
}

func TestMissReadsStoreThenHitsCache(t *testing.T) {
	when := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	s := &fakeStore{rec: Record{Status: PendingDeletion, DeletionScheduledAt: when}, found: true}
	c := newCache()
	g := New(s, c)
	for i := 0; i < 2; i++ {
		rec, err := g.State(context.Background(), "w1")
		if err != nil || rec.Status != PendingDeletion || !rec.DeletionScheduledAt.Equal(when) {
			t.Fatalf("got %+v %v", rec, err)
		}
	}
	if s.calls != 1 {
		t.Fatalf("store calls = %d, want 1", s.calls)
	}
	if _, ok := c.m["mul:plat:ws:w1"]; !ok || c.ttl != 60*time.Second {
		t.Fatalf("cache key/ttl wrong: %v %v", c.m, c.ttl)
	}
}

func TestNoRowIsActiveAndNegativelyCached(t *testing.T) {
	s := &fakeStore{}
	g := New(s, newCache())
	for i := 0; i < 2; i++ {
		rec, err := g.State(context.Background(), "w1")
		if err != nil || rec.Status != Active || !rec.DeletionScheduledAt.IsZero() {
			t.Fatalf("got %+v %v", rec, err)
		}
	}
	if s.calls != 1 {
		t.Fatalf("store calls = %d, want 1 (no-row result must be cached)", s.calls)
	}
}

func TestEachStatusRoundTripsThroughCache(t *testing.T) {
	for _, st := range []Status{Active, ReadOnly, Suspended, PendingDeletion} {
		s := &fakeStore{rec: Record{Status: st}, found: true}
		g := New(s, newCache())
		for i := 0; i < 2; i++ {
			rec, err := g.State(context.Background(), "w1")
			if err != nil || rec.Status != st {
				t.Fatalf("%s: got %+v %v", st, rec, err)
			}
		}
	}
}

func TestCacheErrorFallsBackToStore(t *testing.T) {
	s := &fakeStore{rec: Record{Status: ReadOnly}, found: true}
	c := newCache()
	c.getErr = errors.New("redis down")
	c.setErr = errors.New("redis down")
	g := New(s, c)
	rec, err := g.State(context.Background(), "w1")
	if err != nil || rec.Status != ReadOnly {
		t.Fatalf("got %+v %v", rec, err)
	}
	if s.calls != 1 {
		t.Fatalf("store calls = %d, want 1", s.calls)
	}
}

func TestCorruptCacheEntryFallsBackToStore(t *testing.T) {
	s := &fakeStore{rec: Record{Status: Suspended}, found: true}
	c := newCache()
	c.m["mul:plat:ws:w1"] = []byte("not json")
	rec, err := New(s, c).State(context.Background(), "w1")
	if err != nil || rec.Status != Suspended || s.calls != 1 {
		t.Fatalf("got %+v %v calls=%d", rec, err, s.calls)
	}
}

func TestStoreErrorIsReturnedAndNotCached(t *testing.T) {
	boom := errors.New("db down")
	s := &fakeStore{err: boom}
	c := newCache()
	rec, err := New(s, c).State(context.Background(), "w1")
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if rec.Status == Active {
		t.Fatalf("a store error must not return an active record: %+v", rec)
	}
	if len(c.m) != 0 {
		t.Fatalf("store error was cached: %v", c.m)
	}
}

func TestInvalidateDeletesKey(t *testing.T) {
	s := &fakeStore{rec: Record{Status: ReadOnly}, found: true}
	c := newCache()
	g := New(s, c)
	if _, err := g.State(context.Background(), "w1"); err != nil {
		t.Fatal(err)
	}
	if err := g.Invalidate(context.Background(), "w1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.m["mul:plat:ws:w1"]; ok {
		t.Fatal("key still cached")
	}
	if len(c.dels) != 1 || c.dels[0] != "mul:plat:ws:w1" {
		t.Fatalf("dels = %v", c.dels)
	}
	s.rec = Record{Status: Active}
	rec, _ := g.State(context.Background(), "w1")
	if rec.Status != Active || s.calls != 2 {
		t.Fatalf("after invalidate: %+v calls=%d", rec, s.calls)
	}
}

func TestInvalidateReturnsCacheError(t *testing.T) {
	boom := errors.New("redis down")
	c := newCache()
	c.delErr = boom
	if err := New(&fakeStore{}, c).Invalidate(context.Background(), "w1"); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

func TestKeyIsExported(t *testing.T) {
	if got := Key("abc"); got != "mul:plat:ws:abc" {
		t.Fatalf("Key = %q", got)
	}
}
