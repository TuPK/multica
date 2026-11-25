package scheduler

import (
	"context"
	"time"
)

// JuryTimer times out the jury panels that have waited a day (durable spec 8.3).
type JuryTimer interface {
	Timeout(ctx context.Context) error
}

// DurableJuryTimeoutJob times out open jury panels once an hour. A panel is timed out
// once, so a run that finds none writes nothing.
func DurableJuryTimeoutJob(timer JuryTimer) JobSpec {
	return JobSpec{
		Name: "durable_jury_timeout", Cadence: time.Hour, CatchUpMode: CatchUpLatestOnly, CatchUpWindow: 24 * time.Hour,
		RunTimeout: 50 * time.Second, StaleTimeout: 2 * time.Minute, HeartbeatInterval: 10 * time.Second,
		AllowStaleReentry: true, MaxAttempts: 1, Scopes: StaticScopes(ScopeGlobal),
		Handler: func(ctx context.Context, _ HandlerInput) (HandlerResult, error) {
			return HandlerResult{}, timer.Timeout(ctx)
		},
	}
}
