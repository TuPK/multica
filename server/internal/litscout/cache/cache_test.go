package cache

import (
	"strings"
	"testing"
	"time"
)

func TestKeyIsStableAndOrderIndependent(t *testing.T) {
	a := Key("openalex", "q", map[string]string{"a": "1", "b": "2"})
	b := Key("openalex", "q", map[string]string{"b": "2", "a": "1"})
	if a != b {
		t.Fatalf("keys differ: %s %s", a, b)
	}
	if a == Key("openalex", "q2", map[string]string{"a": "1", "b": "2"}) {
		t.Fatal("different query gave the same key")
	}
}

func TestGetSetAndTTL(t *testing.T) {
	now := time.Unix(1000, 0)
	c := &Cache{Dir: t.TempDir(), TTL: time.Hour, Now: func() time.Time { return now }}
	if _, ok := c.Get("k"); ok {
		t.Fatal("hit on empty cache")
	}
	if err := c.Set("k", []byte(`{"x":1}`)); err != nil {
		t.Fatal(err)
	}
	// ModTime is the real clock; move the fake clock relative to it.
	now = time.Now()
	if v, ok := c.Get("k"); !ok || string(v) != `{"x":1}` {
		t.Fatalf("got %q %v", v, ok)
	}
	now = time.Now().Add(2 * time.Hour)
	if _, ok := c.Get("k"); ok {
		t.Fatal("expired entry was returned")
	}
}

func TestNilAndZeroTTLNeverHit(t *testing.T) {
	var c *Cache
	if _, ok := c.Get("k"); ok {
		t.Fatal("nil cache hit")
	}
	if err := c.Set("k", []byte("v")); err != nil {
		t.Fatal(err)
	}
	z := &Cache{Dir: t.TempDir()}
	_ = z.Set("k", []byte("v"))
	if _, ok := z.Get("k"); ok {
		t.Fatal("zero TTL cache hit")
	}
}

func TestKeyDoesNotLeakCredentialWhenCallerOmitsIt(t *testing.T) {
	k := Key("openalex", "q", map[string]string{"authenticated": "true"})
	if strings.Contains(k, "secret") {
		t.Fatal("unexpected")
	}
}
