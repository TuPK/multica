// Command platform-server runs the platform management service: the API
// listener and the console listener, in one process.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/multica-ai/multica/platform-management/accountguard"
	"github.com/multica-ai/multica/platform-management/internal/accountcache"
	"github.com/multica-ai/multica/platform-management/internal/accountrequest"
	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/billingapi"
	"github.com/multica-ai/multica/platform-management/internal/config"
	"github.com/multica-ai/multica/platform-management/internal/console"
	"github.com/multica-ai/multica/platform-management/internal/db"
	"github.com/multica-ai/multica/platform-management/internal/entitlement"
	"github.com/multica-ai/multica/platform-management/internal/jobs"
	"github.com/multica-ai/multica/platform-management/internal/lifecycle"
	"github.com/multica-ai/multica/platform-management/internal/mailer"
	"github.com/multica-ai/multica/platform-management/internal/mfa"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/platformauth"
	"github.com/multica-ai/multica/platform-management/internal/pricechange"
	"github.com/multica-ai/multica/platform-management/internal/schemacheck"
	"github.com/multica-ai/multica/platform-management/internal/seat"
	"github.com/multica-ai/multica/platform-management/internal/server"
	"github.com/multica-ai/multica/platform-management/internal/staff"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
	"github.com/multica-ai/multica/platform-management/internal/trust"
	"github.com/multica-ai/multica/platform-management/internal/workspacestatus"
	"github.com/multica-ai/multica/platform-management/workspacestate"
)

const (
	// consoleDistDir is relative to the working directory; the image's
	// WORKDIR holds console/dist next to the binary.
	consoleDistDir = "console/dist"

	startupTimeout  = 15 * time.Second
	shutdownGrace   = 10 * time.Second
	mailHTTPTimeout = 20 * time.Second
)

func main() {
	if err := run(); err != nil {
		slog.Error("platform-server stopped", "error", err)
		os.Exit(1)
	}
}

// warnEmptyBank logs one warning naming the BILLING_BANK_* variables that are
// empty. The manual provider then tells customers an empty account.
func warnEmptyBank(log *slog.Logger, b config.Billing) {
	if empty := b.EmptyBankVariables(); len(empty) > 0 {
		log.Warn("billing: bank details are empty; manual payment instructions will show empty fields", "empty", strings.Join(empty, ","))
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

	// An empty plan table gets the embedded catalogue; a table with rows is
	// staff's and is left alone.
	planRunner := plan.NewPgxRunner(pool)
	if _, err := plan.Seed(startCtx, planRunner, slog.Default()); err != nil {
		return fmt.Errorf("load the plan catalogue: %w", err)
	}

	queries := db.New(pool)
	store := staff.NewStore(queries)

	// The key must open what Multica sealed, or no staff member can sign in.
	mfaCrypto, err := mfa.NewCrypto(cfg.MFASecretKey)
	if err != nil {
		return &config.Error{Variable: mfa.EnvKey, Reason: "is not a usable key"}
	}
	mfaStore := staff.NewMFAStore(queries)
	if err := mfa.CheckKey(startCtx, mfaCrypto, mfaStore); err != nil {
		return err
	}

	seeder := staff.NewSeeder(queries, cfg.AdminEmails, slog.Default())
	// An administrator without a Multica account is logged and skipped; only a
	// database error stops the start.
	if err := seeder.SeedAll(startCtx); err != nil {
		return err
	}

	warnEmptyBank(slog.Default(), cfg.Billing)

	var smtpDialer mailer.SMTPDialer
	if cfg.Email.Provider == config.ProviderSMTP {
		smtpDialer = mailer.NewSMTPDialer(cfg.Email)
	}
	// With no provider (allowed only with a dev code) the sign-in handler gets
	// a nil Mailer and sends nothing, and the console sends no reset notice.
	var signInMail platformauth.Mailer
	var consoleMail console.Mailer
	if cfg.Email.Provider != "" {
		m, err := mailer.New(cfg.Email, &http.Client{Timeout: mailHTTPTimeout}, smtpDialer, slog.Default())
		if err != nil {
			return err
		}
		signInMail = m
		consoleMail = m
	}
	accountCache := accountcache.NewCache(rdb)
	guard := accountguard.New(store, accountCache)
	auth := platformauth.New(platformauth.Deps{
		Config: platformauth.Config{
			SessionSecret: cfg.SessionSecret,
			ConsoleOrigin: cfg.ConsoleOrigin,
			SecureCookies: cfg.SecureCookies,
			AdminCIDRs:    cfg.AdminCIDRs,
			AdminEmails:   cfg.AdminEmails,

			DevVerificationCode: cfg.DevVerificationCode,
		},
		Directory: store,
		Seeder:    seeder,
		Guard:     guard,
		Codes:     platformauth.NewRedisCodes(rdb),
		Limiter:   platformauth.NewRedisLimiter(rdb),
		Mailer:    signInMail,
		MFA:       mfaStore,
		Crypto:    mfaCrypto,
		Log:       slog.Default(),
	})

	// The entitlement policy, and the staff writes that clear its cache.
	policy := entitlement.NewService(entitlement.NewPgxLoader(pool), time.Now, slog.Default())
	billingAPI := subscription.NewConsole(subscription.ConsoleDeps{
		RequireStaff:  auth.RequirePlatformStaff,
		RequireOrigin: auth.RequireOrigin,
		Plans:         plan.NewPgxReader(pool),
		PlanRunner:    planRunner,
		Subscriptions: subscription.NewService(subscription.NewPgxRunner(pool), policy),
		Emails:        store,
		Cache:         policy,
		Log:           slog.Default(),
	})

	// The routes Multica's clients call. The billing routes carry X-User-ID and
	// no credential, so they run behind the trusted-peer rule.
	peers := trust.New(cfg.TrustedCIDRs)
	billingReads := subscription.NewAPI(subscription.APIDeps{
		Subscriptions: subscription.NewPgxReader(pool),
		Plans:         plan.NewPgxReader(pool),
		Members:       subscription.NewPgxMembers(pool),
		Pending:       subscription.NewPgxPending(pool),
		Log:           slog.Default(),
	}).Handler(peers.RequireUser)
	routes := []server.Route{{Pattern: entitlement.PolicyPattern, Handler: policy.Handler()}}
	for _, pattern := range subscription.APIPatterns {
		routes = append(routes, server.Route{Pattern: pattern, Handler: billingReads})
	}
	billingProfile := subscription.NewProfileAPI(subscription.ProfileAPIDeps{
		Service: subscription.NewProfileService(subscription.NewPgxProfileRunner(pool)),
		Members: subscription.NewPgxMembers(pool),
		Log:     slog.Default(),
	}).Handler(peers.RequireUser)
	routes = append(routes, server.Route{Pattern: subscription.ProfilePattern, Handler: billingProfile})
	// Orders are created here and confirmed by staff in the console; the manual
	// provider is the only one in this release.
	orders, err := billing.New(billing.Deps{
		Runner: billing.NewPgxRunner(pool), Provider: billing.ProviderManual,
		Workspaces: workspacestate.New(nil, accountCache), Entitlements: policy, Log: slog.Default(),
	})
	if err != nil {
		return err
	}

	// The console confirms orders through the same service that creates them.
	consoleAPI := console.New(console.Deps{
		RequireStaff:  auth.RequirePlatformStaff,
		RequireOrigin: auth.RequireOrigin,
		Reader:        console.NewPgxReader(pool),
		Runner:        console.NewPgxRunner(pool),
		Workspaces: workspacestatus.New(workspacestatus.NewPgxRunner(pool),
			workspacestate.New(nil, accountCache), slog.Default()),
		Cache:    guard,
		Notifier: accountcache.NewPublisher(rdb),
		Payments: orders,
		Accounts: accountrequest.New(accountrequest.Deps{
			Runner: accountrequest.NewPgxRunner(pool), Reader: accountrequest.NewPgxReader(pool),
			Plans: plan.NewPgxReader(pool), Limiter: platformauth.NewRedisLimiter(rdb),
			CreatePerHour: cfg.AccountCreatePerHour, IndividualMaxSeats: cfg.IndividualMaxSeats,
		}),
		Emails: store,
		Mailer: consoleMail,
		Log:    slog.Default(),
	})

	billingWrites, err := billingapi.New(billingapi.Deps{
		Subscriptions: subscription.NewPgxReader(pool),
		Plans:         plan.NewPgxReader(pool),
		Members:       subscription.NewPgxMembers(pool),
		Orders:        orders,
		Workspaces:    billingapi.NewPgxWorkspaces(pool),
		VATRate:       cfg.Billing.VATRate,
		AppURL:        cfg.AppURL,
		Log:           slog.Default(),
	})
	if err != nil {
		return err
	}
	billingWritesHandler := billingWrites.Handler(peers.RequireUser)
	for _, pattern := range billingapi.Patterns {
		routes = append(routes, server.Route{Pattern: pattern, Handler: billingWritesHandler})
	}
	// The seat protocol of Multica's seat client. It takes no credential, like
	// the entitlement policy route.
	seatAPI := seat.NewHandler(seat.NewService(seat.NewPgxRunner(pool), time.Now), slog.Default()).Mux()
	for _, pattern := range seat.Patterns {
		routes = append(routes, server.Route{Pattern: pattern, Handler: seatAPI})
	}

	// The background jobs. Each takes an advisory lock by name, so several
	// platform instances share one schedule.
	runner := jobs.New(jobs.NewPoolLocker(pool))
	orderExpiry, err := lifecycle.NewOrderExpirer(lifecycle.NewPgxStore(pool), slog.Default())
	if err != nil {
		return err
	}
	if err := runner.Register(lifecycle.OrderExpiryJobName, lifecycle.OrderExpiryInterval, orderExpiry.Job(time.Now)); err != nil {
		return err
	}
	// The price-change notice uses the lifecycle mail sender; nil sends no notice.
	var noticeSender lifecycle.Sender
	if cfg.Email.Provider == "" {
		slog.Warn("lifecycle notices are off: no email provider is configured")
	} else {
		noticeMail, err := lifecycle.NewMailSender(cfg.Email, &http.Client{Timeout: mailHTTPTimeout}, smtpDialer, time.Now, slog.Default())
		if err != nil {
			return err
		}
		noticeSender = noticeMail
		lifecycleJob, err := lifecycle.New(lifecycle.Deps{
			Store: lifecycle.NewPgxStore(pool), Mail: noticeMail,
			Workspaces: workspacestate.New(nil, accountCache), Entitlements: policy,
			AppURL: cfg.AppURL, Log: slog.Default(),
		})
		if err != nil {
			return err
		}
		if err := runner.Register(lifecycle.JobName, lifecycle.Interval, lifecycleJob.Job(time.Now)); err != nil {
			return err
		}
	}

	// The plan price-change schedule (spec 7b.4): the apply job and the staff routes.
	priceSvc, err := pricechange.New(pricechange.Deps{
		Store: pricechange.NewPgxStore(pool), Mail: noticeSender, Cache: policy,
		AppURL: cfg.AppURL, Log: slog.Default(),
	})
	if err != nil {
		return err
	}
	if err := runner.Register(pricechange.JobName, pricechange.Interval, priceSvc.Job(time.Now)); err != nil {
		return err
	}
	priceChanges := pricechange.NewHandler(pricechange.HandlerDeps{
		RequireStaff: auth.RequirePlatformStaff, RequireOrigin: auth.RequireOrigin,
		Service: priceSvc, Emails: store, Log: slog.Default(),
	})
	platformHandler := newPlatformMux(priceChanges, server.NewPlatformHandler(auth, consoleAPI, billingAPI))

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
	jobsCtx, stopJobs := context.WithCancel(ctx)
	jobsDone := make(chan struct{})
	go func() {
		defer close(jobsDone)
		_ = runner.Run(jobsCtx)
	}()
	err = server.Serve(ctx, shutdownGrace,
		server.Listener{Name: "api", Listener: apiLn, Handler: server.NewAPIHandler(peers, health, platformHandler, routes...)},
		server.Listener{Name: "console", Listener: consoleLn, Handler: server.NewConsoleHandler(health, apiURL, consoleDistDir)},
	)
	stopJobs()
	<-jobsDone
	// Let a code that was just requested finish sending.
	auth.Wait()
	return err
}

// newPlatformMux routes the price-change staff routes to priceChanges. A PUT to
// /api/platform/plans/{key} that carries a price field is refused with
// price_field_via_schedule before the console sees it (spec 7b.4 rule 2). The
// refusal sits here, not in the console, because the console's package cannot
// import pricechange without a cycle. Every other path goes to rest.
func newPlatformMux(priceChanges, rest http.Handler) http.Handler {
	mux := http.NewServeMux()
	for _, pattern := range pricechange.Patterns {
		mux.Handle(pattern, priceChanges)
	}
	mux.Handle("/api/platform/plans/{key}", pricechange.RefusePriceFields(rest))
	mux.Handle("/", rest)
	return mux
}
