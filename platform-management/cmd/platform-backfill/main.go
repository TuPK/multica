// Command platform-backfill gives every workspace with no subscription row the
// free plan. Run it once per environment after the migrations and before the
// platform starts serving; it is safe to run again.
//
//	platform-backfill [subscriptions] [--database-url URL] [--allow-production]
//	platform-backfill tenants [--database-url URL] [--allow-production]
//
// The tenants command gives every workspace with no tenant a tenant of its own
// (spec R6 section 5.4); run it after the tenant migrations and the deploy of the
// new server, to repair workspaces an old server created in between.
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

// The commands. Without a name the command is the subscription backfill.
const (
	commandSubscriptions = "subscriptions"
	commandTenants       = "tenants"
)

type options struct {
	command         string
	databaseURL     string
	allowProduction bool
}

// parseArgs reads [command] [flags]. The command is the first argument when it
// does not start with a dash.
func parseArgs(args []string) (options, error) {
	opts := options{command: commandSubscriptions}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		opts.command, args = args[0], args[1:]
		if opts.command != commandSubscriptions && opts.command != commandTenants {
			return options{}, fmt.Errorf("unknown command %q; use subscriptions or tenants", opts.command)
		}
	}
	fs := flag.NewFlagSet("platform-backfill", flag.ContinueOnError)
	fs.StringVar(&opts.databaseURL, "database-url", "", "database to backfill (default: DATABASE_URL)")
	fs.BoolVar(&opts.allowProduction, "allow-production", false, "allow a database whose name does not end in _test")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() > 0 {
		return options{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	opts.databaseURL = strings.TrimSpace(opts.databaseURL)
	return opts, nil
}

func run(args []string, getenv func(string) string) error {
	opts, err := parseArgs(args)
	if err != nil {
		return err
	}
	dsn := opts.databaseURL
	if dsn == "" {
		dsn = strings.TrimSpace(getenv("DATABASE_URL"))
	}
	if err := guard(dsn, opts.allowProduction); err != nil {
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

	if opts.command == commandTenants {
		res, err := backfill.RunTenants(ctx, backfill.NewPgxTenantRunner(pool), backfill.BatchSize, slog.Default())
		if err != nil {
			return err
		}
		slog.Info("platform-backfill tenants complete", "batches", res.Batches, "tenants", res.Tenants, "linked", res.Linked)
		return nil
	}
	res, err := backfill.Run(ctx, backfill.NewPgxRunner(pool), backfill.BatchSize, slog.Default())
	if err != nil {
		return err
	}
	slog.Info("platform-backfill complete", "batches", res.Batches, "inserted", res.Inserted)
	return nil
}
