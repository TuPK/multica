package scheduler

import (
	"context"
	"time"
)

// TrackRecordRunner recomputes the track records of agent pairs (durable spec 3.2).
type TrackRecordRunner interface {
	RecomputeAll(ctx context.Context) (int, error)
}

// DurableTrackRecordJob recomputes every pair with an outcome settled in the last 90
// days, once a day. A recompute that finds nothing new writes nothing, so a retry is
// safe.
func DurableTrackRecordJob(runner TrackRecordRunner) JobSpec {
	return JobSpec{
		Name: "durable_track_record", Cadence: 24 * time.Hour, CatchUpMode: CatchUpLatestOnly, CatchUpWindow: 24 * time.Hour,
		RunTimeout: 10 * time.Minute, StaleTimeout: 15 * time.Minute, HeartbeatInterval: 30 * time.Second,
		AllowStaleReentry: true, MaxAttempts: 3, RetryBackoff: []time.Duration{time.Minute, 5 * time.Minute},
		Scopes: StaticScopes(ScopeGlobal),
		Handler: func(ctx context.Context, _ HandlerInput) (HandlerResult, error) {
			n, err := runner.RecomputeAll(ctx)
			return HandlerResult{RowsAffected: int64(n)}, err
		},
	}
}
