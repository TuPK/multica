// Command platform-backfill gives every workspace with no subscription row the
// free plan. Run it once per environment after the migrations and before the
// platform starts serving; it is safe to run again.
//
//	platform-backfill [--database-url URL] [--allow-production]
//
// The database is --database-url, or DATABASE_URL. A name that does not end in
// _test is refused unless --allow-production is given.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/multica-ai/multica/platform-management/internal/backfill"
	"github.com/multica-ai/multica/platform-management/internal/schemacheck"
)

const pingTimeout = 10 * time.Second

func main() {
	if err := run(os.Args[1:], os.Getenv); err != nil {
		slog.Error("platform-backfill stopped", "error", err)
		os.Exit(1)
	}
}

// guard returns why the backfill must not run against dsn, or nil. The reason
// never contains the DSN.
func guard(dsn string, allowProduction bool) error {
	if dsn == "" {
		return errors.New("no database: set --database-url or DATABASE_URL")
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return errors.New("the database URL cannot be parsed")
	}
	if strings.HasSuffix(cfg.Database, "_test") || allowProduction {
		return nil
	}
	return fmt.Errorf("database %q does not end in _test; pass --allow-production to run against it", cfg.Database)
}

func run(args []string, getenv func(string) string) error {
	fs := flag.NewFlagSet("platform-backfill", flag.ContinueOnError)
	dsnFlag := fs.String("database-url", "", "database to backfill (default: DATABASE_URL)")
	allowProduction := fs.Bool("allow-production", false, "allow a database whose name does not end in _test")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dsn := strings.TrimSpace(*dsnFlag)
	if dsn == "" {
		dsn = strings.TrimSpace(getenv("DATABASE_URL"))
	}
	if err := guard(dsn, *allowProduction); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer pool.Close()

	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	// The backfill writes the subscription and audit tables; refuse to run
	// before the migrations that create them.
	if err := schemacheck.Check(pingCtx, pool, schemacheck.Required); err != nil {
		return err
	}

	res, err := backfill.Run(ctx, backfill.NewPgxRunner(pool), backfill.BatchSize, slog.Default())
	if err != nil {
		return err
	}
	slog.Info("platform-backfill complete", "batches", res.Batches, "inserted", res.Inserted)
	return nil
}
