package scheduler

import (
	"context"
	"errors"
	"time"
)

// OutcomeRunner is the work the durable outcome job does each minute. Both
// methods return without writing while durable_outcomes is off (durable spec 2.4).
type OutcomeRunner interface {
	ProcessTransitions(ctx context.Context) (int, error)
	ScheduleChecks(ctx context.Context) (int, error)
	Settle(ctx context.Context, now time.Time) (int, error)
}

// MetricChecker decides the metric criteria of outcomes (durable spec 2.5).
type MetricChecker interface {
	CheckMetrics(ctx context.Context, now time.Time) (int, error)
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
			_, processErr := runner.ProcessTransitions(ctx)
			_, scheduleErr := runner.ScheduleChecks(ctx)
			_, settleErr := runner.Settle(ctx, time.Now())
			return HandlerResult{}, errors.Join(processErr, scheduleErr, settleErr)
		},
	}
}

// DurableMetricJob checks the metric criteria of current outcomes once an hour (spec 2.5).
// A criterion is decided once, so a run that finds nothing new writes nothing.
func DurableMetricJob(checker MetricChecker) JobSpec {
	return JobSpec{
		Name: "durable_metric", Cadence: time.Hour, CatchUpMode: CatchUpLatestOnly, CatchUpWindow: 24 * time.Hour,
		RunTimeout: 50 * time.Second, StaleTimeout: 2 * time.Minute, HeartbeatInterval: 10 * time.Second,
		AllowStaleReentry: true, MaxAttempts: 1, Scopes: StaticScopes(ScopeGlobal),
		Handler: func(ctx context.Context, _ HandlerInput) (HandlerResult, error) {
			_, err := checker.CheckMetrics(ctx, time.Now())
			return HandlerResult{}, err
		},
	}
}
