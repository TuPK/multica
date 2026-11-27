package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/handler"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// durableOpenFlags turns durable_open on or off for a handler under test.
func durableOpenFlags(on bool) *featureflag.Service {
	p := featureflag.NewStaticProvider()
	p.Set(featureflags.DurableOpen, featureflag.Rule{Default: on})
	return featureflag.NewService(p)
}

// TestVerifyRateLimited: verification is open to anyone, so the verify route sits behind the
// per-IP limiter. The attempts past the limit are refused with 429 (durable spec 13.2). The
// limiter needs Redis, so the test runs only when REDIS_TEST_URL names a server.
func TestVerifyRateLimited(t *testing.T) {
	url := os.Getenv("REDIS_TEST_URL")
	if url == "" {
		t.Skip("REDIS_TEST_URL not set")
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		t.Fatalf("parse REDIS_TEST_URL: %v", err)
	}
	rdb := redis.NewClient(opts)
	defer rdb.Close()

	const limit = 2
	r := chi.NewRouter()
	registerForkPublicRoutes(r, &handler.Handler{FeatureFlags: durableOpenFlags(true)}, nil, middleware.RateLimit(rdb, limit, time.Minute, nil))

	// A client address no other test uses, so earlier runs do not count against this one.
	remote := fmt.Sprintf("198.51.100.%d:4000", time.Now().UnixNano()%250+1)
	statuses := make([]int, 0, limit+1)
	for i := 0; i <= limit; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/attestations/verify", strings.NewReader("not json"))
		req.RemoteAddr = remote
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		statuses = append(statuses, rec.Code)
	}
	for i, code := range statuses[:limit] {
		if code != http.StatusOK {
			t.Fatalf("attempt %d: status %d, want 200 within the limit (all: %v)", i+1, code, statuses)
		}
	}
	if statuses[limit] != http.StatusTooManyRequests {
		t.Fatalf("attempt %d: status %d, want 429 (all: %v)", limit+1, statuses[limit], statuses)
	}
}

// The well-known keys are public and need no session; the document carries no seed. With
// durable_open off the route is not there (spec 14.1).
func TestWellKnownKeysArePublic(t *testing.T) {
	keysRequest := func(on bool) *httptest.ResponseRecorder {
		r := chi.NewRouter()
		registerForkPublicRoutes(r, &handler.Handler{FeatureFlags: durableOpenFlags(on)}, nil, func(next http.Handler) http.Handler { return next })
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.well-known/multica-attestation-keys.json", nil))
		return rec
	}
	rec := keysRequest(true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200 with no signing key configured", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"keys"`) {
		t.Fatalf("body has no keys list: %s", rec.Body.String())
	}
	if rec := keysRequest(false); rec.Code != http.StatusNotFound {
		t.Fatalf("flag off: status %d, want 404", rec.Code)
	}
}
