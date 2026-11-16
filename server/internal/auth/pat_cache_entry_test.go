package auth_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/auth/authtest"
)

func TestPATCacheEntry_NilSafe(t *testing.T) {
	var c *auth.PATCache
	ctx := context.Background()
	if _, ok := c.GetEntry(ctx, "h"); ok {
		t.Fatal("nil cache must miss")
	}
	c.SetEntry(ctx, "h", auth.PATEntry{UserID: "u"}, auth.AuthCacheTTL) // no panic
}

// The cached value carries the token's restrictions under the v2 prefix.
func TestPATCacheEntry_RoundTripUnderV2Prefix(t *testing.T) {
	fake := authtest.NewFakeRedis()
	c := auth.NewPATCache(fake)
	ctx := context.Background()

	want := auth.PATEntry{UserID: "user-1", WorkspaceID: "ws-1", ReadOnly: true}
	c.SetEntry(ctx, "hash-1", want, auth.AuthCacheTTL)

	got, ok := c.GetEntry(ctx, "hash-1")
	if !ok || got != want {
		t.Fatalf("GetEntry = (%+v, %v), want (%+v, true)", got, ok, want)
	}

	raw, ok := fake.Raw("mul:auth:pat:v2:hash-1")
	if !ok {
		t.Fatalf("entry not stored under mul:auth:pat:v2:, keys = %v", fake.StoredKeys())
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("stored value is not JSON: %v (%q)", err, raw)
	}
	if decoded["user_id"] != "user-1" || decoded["workspace_id"] != "ws-1" || decoded["read_only"] != true {
		t.Fatalf("stored JSON = %v", decoded)
	}
}

// A token with no restriction round-trips as unrestricted.
func TestPATCacheEntry_UnrestrictedRoundTrip(t *testing.T) {
	c := auth.NewPATCache(authtest.NewFakeRedis())
	ctx := context.Background()
	c.SetEntry(ctx, "h", auth.PATEntry{UserID: "u"}, auth.AuthCacheTTL)
	got, ok := c.GetEntry(ctx, "h")
	if !ok || got.WorkspaceID != "" || got.ReadOnly || got.UserID != "u" {
		t.Fatalf("GetEntry = (%+v, %v)", got, ok)
	}
}

// An entry under the old prefix is a bare user id with no restriction data. It
// must read as a miss, so a bound or read-only token is looked up again.
func TestPATCacheEntry_OldPrefixEntryIsIgnored(t *testing.T) {
	fake := authtest.NewFakeRedis()
	c := auth.NewPATCache(fake)
	fake.Put("mul:auth:pat:hash-old", "user-1")
	if e, ok := c.GetEntry(context.Background(), "hash-old"); ok {
		t.Fatalf("old-prefix entry must be ignored, got %+v", e)
	}
}

func TestPATCacheEntry_MalformedOrEmptyUserIsMiss(t *testing.T) {
	fake := authtest.NewFakeRedis()
	c := auth.NewPATCache(fake)
	fake.Put("mul:auth:pat:v2:bad", "not json")
	fake.Put("mul:auth:pat:v2:nouser", `{"workspace_id":"ws"}`)
	for _, h := range []string{"bad", "nouser"} {
		if e, ok := c.GetEntry(context.Background(), h); ok {
			t.Fatalf("%s: must be a miss, got %+v", h, e)
		}
	}
}

// The legacy Set, still used by the WebSocket resolver, cannot plant an entry
// the middleware would read: it carries no restrictions.
func TestPATCacheEntry_LegacySetIsNotVisibleToEntryReads(t *testing.T) {
	c := auth.NewPATCache(authtest.NewFakeRedis())
	ctx := context.Background()
	c.Set(ctx, "h", "user-1", auth.AuthCacheTTL)
	if e, ok := c.GetEntry(ctx, "h"); ok {
		t.Fatalf("legacy Set must not satisfy GetEntry, got %+v", e)
	}
}

func TestPATCacheEntry_InvalidateRemovesBothKeys(t *testing.T) {
	fake := authtest.NewFakeRedis()
	c := auth.NewPATCache(fake)
	ctx := context.Background()
	c.SetEntry(ctx, "h", auth.PATEntry{UserID: "u"}, auth.AuthCacheTTL)
	c.Set(ctx, "h", "u", auth.AuthCacheTTL)
	c.Invalidate(ctx, "h")
	for _, k := range fake.StoredKeys() {
		if strings.HasSuffix(k, ":h") {
			t.Fatalf("key %q survived Invalidate", k)
		}
	}
}
