package scheduler

import (
	"context"
	"time"
)

// FlowAdvancer moves the requests a flow runs. *handler.Handler implements it.
type FlowAdvancer interface {
	SweepFlowRequests(ctx context.Context) (int, error)
}

// FlowSweepJob is the safety net of the flow runner: it starts the requests that missed the engine,
// advances the ones whose event was missed, and acts on tasks nobody works on. It does nothing
// while FF_FLOW_RUNNER is off. Advancing is idempotent, so a pass that is cut short or taken over
// after a restart (AllowStaleReentry) leaves nothing half done.
func FlowSweepJob(advancer FlowAdvancer) JobSpec {
	return JobSpec{
		Name: "flow_sweep", Cadence: 30 * time.Second, CatchUpMode: CatchUpLatestOnly, CatchUpWindow: time.Hour,
		RunTimeout: 45 * time.Second, StaleTimeout: time.Minute, HeartbeatInterval: 10 * time.Second,
		AllowStaleReentry: true, MaxAttempts: 1, Scopes: StaticScopes(ScopeGlobal),
		Handler: func(ctx context.Context, _ HandlerInput) (HandlerResult, error) {
			_, err := advancer.SweepFlowRequests(ctx)
			return HandlerResult{}, err
		},
	}
}
