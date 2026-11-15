// Package testdb opens the platform's test database. The platform module
// cannot import the server's testutil, so it has its own guarded helper.
package testdb

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// guard returns why the test database must not be used, or "" when it may be.
// The reason never contains the DSN.
func guard(testDSN, databaseURL string) string {
	if testDSN == "" {
		return "PLATFORM_TEST_DATABASE_URL is not set"
	}
	cfg, err := pgx.ParseConfig(testDSN)
	if err != nil {
		return "PLATFORM_TEST_DATABASE_URL cannot be parsed"
	}
	if name := cfg.Database; !strings.HasSuffix(name, "_test") || name == "multica" {
		return "PLATFORM_TEST_DATABASE_URL must name a database ending in _test"
	}
	if databaseURL != testDSN {
		return "DATABASE_URL must equal PLATFORM_TEST_DATABASE_URL"
	}
	return ""
}

// Open returns a pool on PLATFORM_TEST_DATABASE_URL. It skips the test unless
// that database's name ends in _test and DATABASE_URL equals the same DSN.
func Open(t testing.TB) *pgxpool.Pool {
	t.Helper()
	if reason := guard(os.Getenv("PLATFORM_TEST_DATABASE_URL"), os.Getenv("DATABASE_URL")); reason != "" {
		t.Skip(reason)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, os.Getenv("PLATFORM_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping test database: %v", err)
	}
	return pool
}
