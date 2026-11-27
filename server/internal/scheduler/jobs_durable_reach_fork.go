package scheduler

import (
	"context"
	"time"
)

// ReachRunner writes the nightly Reach Map of every workspace (durable spec 11.2).
type ReachRunner interface {
	Nightly(ctx context.Context) (int, error)
}

// DurableReachJob recomputes every workspace's Reach Map once a day and drops snapshots older
// than 400 days. A run that repeats a day overwrites that day's rows, so a retry is safe. It
// writes nothing while durable_open is off.
func DurableReachJob(runner ReachRunner) JobSpec {
	return JobSpec{
		Name: "durable_reach", Cadence: 24 * time.Hour, CatchUpMode: CatchUpLatestOnly, CatchUpWindow: 24 * time.Hour,
		RunTimeout: 10 * time.Minute, StaleTimeout: 15 * time.Minute, HeartbeatInterval: 30 * time.Second,
		AllowStaleReentry: true, MaxAttempts: 3, RetryBackoff: []time.Duration{time.Minute, 5 * time.Minute},
		Scopes: StaticScopes(ScopeGlobal),
		Handler: func(ctx context.Context, _ HandlerInput) (HandlerResult, error) {
			n, err := runner.Nightly(ctx)
			return HandlerResult{RowsAffected: int64(n)}, err
		},
	}
}
