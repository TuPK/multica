package scheduler

import (
	"context"
	"time"
)

// OutcomeRunner is the work the durable outcome job does each minute. Both
// methods return without writing while durable_outcomes is off (durable spec 2.4).
type OutcomeRunner interface {
	ProcessTransitions(ctx context.Context) (int, error)
	Settle(ctx context.Context, now time.Time) (int, error)
}

// DurableOutcomeJob reads new status transitions into work outcomes and settles the
// outcomes that are due. It runs every minute; a transition is read once, by cursor,
// so a run that is retried or overlaps another does no extra work.
func DurableOutcomeJob(runner OutcomeRunner) JobSpec {
	return JobSpec{
		Name: "durable_outcome", Cadence: time.Minute, CatchUpMode: CatchUpLatestOnly, CatchUpWindow: time.Hour,
		RunTimeout: 50 * time.Second, StaleTimeout: 2 * time.Minute, HeartbeatInterval: 10 * time.Second,
		AllowStaleReentry: true, MaxAttempts: 1, Scopes: StaticScopes(ScopeGlobal),
		Handler: func(ctx context.Context, _ HandlerInput) (HandlerResult, error) {
			if _, err := runner.ProcessTransitions(ctx); err != nil {
				return HandlerResult{}, err
			}
			_, err := runner.Settle(ctx, time.Now())
			return HandlerResult{}, err
		},
	}
}
