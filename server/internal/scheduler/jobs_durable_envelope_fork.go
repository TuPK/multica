package scheduler

import (
	"context"
	"time"
)

// EnvelopeRunner moves squads inside their autonomy envelopes (durable spec 3.4).
type EnvelopeRunner interface {
	Nightly(ctx context.Context) (int, error)
}

// DurableEnvelopeJob moves every squad with an envelope by the evidence rules, once a day.
// A run that finds nothing to move writes nothing, so a retry is safe.
func DurableEnvelopeJob(runner EnvelopeRunner) JobSpec {
	return JobSpec{
		Name: "durable_envelope", Cadence: 24 * time.Hour, CatchUpMode: CatchUpLatestOnly, CatchUpWindow: 24 * time.Hour,
		RunTimeout: 10 * time.Minute, StaleTimeout: 15 * time.Minute, HeartbeatInterval: 30 * time.Second,
		AllowStaleReentry: true, MaxAttempts: 3, RetryBackoff: []time.Duration{time.Minute, 5 * time.Minute},
		Scopes: StaticScopes(ScopeGlobal),
		Handler: func(ctx context.Context, _ HandlerInput) (HandlerResult, error) {
			n, err := runner.Nightly(ctx)
			return HandlerResult{RowsAffected: int64(n)}, err
		},
	}
}
