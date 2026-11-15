// Command platform-server runs the platform management service: the API
// listener and the console listener, in one process.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/multica-ai/multica/platform-management/internal/config"
	"github.com/multica-ai/multica/platform-management/internal/schemacheck"
	"github.com/multica-ai/multica/platform-management/internal/server"
)

const (
	// consoleDistDir is relative to the working directory; the image's
	// WORKDIR holds console/dist next to the binary.
	consoleDistDir = "console/dist"

	startupTimeout = 15 * time.Second
	shutdownGrace  = 10 * time.Second
)

func main() {
	if err := run(); err != nil {
		slog.Error("platform-server stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return fmt.Errorf("configuration: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer pool.Close()

	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return &config.Error{Variable: "REDIS_URL", Reason: "cannot be parsed as a Redis URL"}
	}
	rdb := redis.NewClient(redisOpts)
	defer rdb.Close()

	startCtx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	if err := pool.Ping(startCtx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	if err := schemacheck.Check(startCtx, pool, schemacheck.Required); err != nil {
		return err
	}

	apiLn, err := net.Listen("tcp", ":"+cfg.APIPort)
	if err != nil {
		return fmt.Errorf("listen on PLATFORM_API_PORT: %w", err)
	}
	consoleLn, err := net.Listen("tcp", ":"+cfg.ConsolePort)
	if err != nil {
		apiLn.Close()
		return fmt.Errorf("listen on PLATFORM_CONSOLE_PORT: %w", err)
	}

	health := server.HealthHandler(
		server.Check{Name: "database", Ping: pool.Ping},
		server.Check{Name: "redis", Ping: func(ctx context.Context) error { return rdb.Ping(ctx).Err() }},
	)
	// The console reaches the API through the API's own socket, on loopback.
	apiURL := &url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.1", cfg.APIPort)}

	slog.Info("platform-server starting", "api_port", cfg.APIPort, "console_port", cfg.ConsolePort)
	return server.Serve(ctx, shutdownGrace,
		server.Listener{Name: "api", Listener: apiLn, Handler: server.NewAPIHandler(health)},
		server.Listener{Name: "console", Listener: consoleLn, Handler: server.NewConsoleHandler(health, apiURL, consoleDistDir)},
	)
}
