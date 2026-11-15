package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/multica-ai/multica/platform-management/internal/testdb"
)

// Database test: skipped unless PLATFORM_TEST_DATABASE_URL names a *_test
// database and DATABASE_URL equals it. The Redis half also needs
// PLATFORM_TEST_REDIS_URL, a Redis that tests may use.
func TestHealthzAgainstRealDependencies(t *testing.T) {
	pool := testdb.Open(t)
	redisURL := os.Getenv("PLATFORM_TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("PLATFORM_TEST_REDIS_URL is not set")
	}
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatalf("PLATFORM_TEST_REDIS_URL cannot be parsed: %v", err)
	}
	rdb := redis.NewClient(opts)
	t.Cleanup(func() { rdb.Close() })

	redisPing := func(ctx context.Context) error { return rdb.Ping(ctx).Err() }
	serve := func(checks ...Check) (int, string) {
		srv := httptest.NewServer(NewAPIHandler(HealthHandler(checks...), http.NotFoundHandler()))
		defer srv.Close()
		return get(t, srv.URL, "/healthz")
	}

	code, body := serve(Check{Name: "database", Ping: pool.Ping}, Check{Name: "redis", Ping: redisPing})
	if code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", code, body)
	}

	// A Redis nobody listens on: the database is still reported healthy.
	dead := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	t.Cleanup(func() { dead.Close() })
	code, body = serve(
		Check{Name: "database", Ping: pool.Ping},
		Check{Name: "redis", Ping: func(ctx context.Context) error { return dead.Ping(ctx).Err() }},
	)
	hb := decodeHealth(t, body)
	if code != http.StatusServiceUnavailable || strings.Join(hb.Failed, ",") != "redis" {
		t.Fatalf("status = %d, body = %s", code, body)
	}

	// A closed pool: the database is reported, Redis is still healthy.
	pool.Close()
	code, body = serve(Check{Name: "database", Ping: pool.Ping}, Check{Name: "redis", Ping: redisPing})
	hb = decodeHealth(t, body)
	if code != http.StatusServiceUnavailable || strings.Join(hb.Failed, ",") != "database" {
		t.Fatalf("status = %d, body = %s", code, body)
	}
}
