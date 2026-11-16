package scheduler

import (
	"context"
	"time"
)

// JobNameUsageSnapshot is the canonical name used in audit rows. Stable across
// releases.
const JobNameUsageSnapshot = "platform_usage_daily"

// usageSnapshotDelay is how long after UTC midnight the job becomes due. It
// must equal usagesnapshot.RunDelay; the day it computes is decided there.
const usageSnapshotDelay = time.Hour

// UsageSnapshotRunner computes the usage rows of the day before the given run
// time. internal/usagesnapshot implements it; it returns how many rows it wrote.
type UsageSnapshotRunner interface {
	RunUsageSnapshot(ctx context.Context, runAt time.Time) (int, error)
}

// UsageSnapshotJob runs once a day at 01:00 UTC. The run time handed to the
// runner is the plan time plus the delay, not the wall clock, so a retry or a
// late start still computes the day of its own plan, and the rows it writes
// replace the ones an earlier attempt wrote.
func UsageSnapshotJob(runner UsageSnapshotRunner) JobSpec {
	return JobSpec{
		Name:              JobNameUsageSnapshot,
		Cadence:           24 * time.Hour,
		ScheduleDelay:     usageSnapshotDelay,
		CatchUpMode:       CatchUpLatestOnly,
		CatchUpWindow:     48 * time.Hour,
		RunTimeout:        30 * time.Minute,
		StaleTimeout:      40 * time.Minute,
		HeartbeatInterval: 30 * time.Second,
		AllowStaleReentry: true,
		MaxAttempts:       3,
		RetryBackoff:      []time.Duration{5 * time.Minute, 15 * time.Minute},
		Scopes:            StaticScopes(ScopeGlobal),
		Handler: func(ctx context.Context, in HandlerInput) (HandlerResult, error) {
			n, err := runner.RunUsageSnapshot(ctx, in.PlanTime.Add(usageSnapshotDelay))
			return HandlerResult{RowsAffected: int64(n)}, err
		},
	}
}
