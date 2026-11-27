package scheduler

import (
	"context"
	"time"
)

// NecessityScheduler opens the weekly necessity tests of every workspace (durable spec 10.3).
type NecessityScheduler interface {
	Weekly(ctx context.Context) (int, error)
}

// FlowStageAnalyst counts the gate candidates of every workspace each month (durable spec 10.4).
type FlowStageAnalyst interface {
	MonthlyFlowStages(ctx context.Context) (int, error)
}

// DurableNecessityJob runs once a week. Each workspace's skills are checked against the schedule
// (at most two running tests per squad), so a run that finds the squads full starts nothing.
func DurableNecessityJob(scheduler NecessityScheduler) JobSpec {
	return JobSpec{
		Name:              "durable_necessity_weekly",
		Cadence:           7 * 24 * time.Hour,
		CatchUpMode:       CatchUpLatestOnly,
		CatchUpWindow:     24 * time.Hour,
		RunTimeout:        20 * time.Minute,
		StaleTimeout:      30 * time.Minute,
		HeartbeatInterval: 30 * time.Second,
		AllowStaleReentry: true,
		MaxAttempts:       3,
		RetryBackoff:      []time.Duration{time.Minute, 5 * time.Minute},
		Scopes:            StaticScopes(ScopeGlobal),
		Handler: func(ctx context.Context, _ HandlerInput) (HandlerResult, error) {
			n, err := scheduler.Weekly(ctx)
			return HandlerResult{RowsAffected: int64(n)}, err
		},
	}
}

// DurableFlowStageJob runs once a month and counts the flow gates that pass the merge-or-drop
// threshold (durable spec 10.4).
func DurableFlowStageJob(analyst FlowStageAnalyst) JobSpec {
	return JobSpec{
		Name:              "durable_flow_stage_monthly",
		Cadence:           30 * 24 * time.Hour,
		CatchUpMode:       CatchUpLatestOnly,
		CatchUpWindow:     24 * time.Hour,
		RunTimeout:        20 * time.Minute,
		StaleTimeout:      30 * time.Minute,
		HeartbeatInterval: 30 * time.Second,
		AllowStaleReentry: true,
		MaxAttempts:       3,
		RetryBackoff:      []time.Duration{time.Minute, 5 * time.Minute},
		Scopes:            StaticScopes(ScopeGlobal),
		Handler: func(ctx context.Context, _ HandlerInput) (HandlerResult, error) {
			n, err := analyst.MonthlyFlowStages(ctx)
			return HandlerResult{RowsAffected: int64(n)}, err
		},
	}
}
