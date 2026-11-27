package scheduler

import (
	"context"
	"time"
)

// ShadowReconciler settles completed shadow pairs that have no comparison and no juror
// task yet (for example when durable_seats was off at completion time).
type ShadowReconciler interface {
	ReconcileUnjudgedPairs(ctx context.Context) (int, error)
}

// DurableShadowReconcileJob settles unjudged completed shadow pairs every minute. It is a
// no-op while durable_seats is off, and idempotent across runs.
func DurableShadowReconcileJob(reconciler ShadowReconciler) JobSpec {
	return JobSpec{
		Name:              "durable_shadow_reconcile",
		Cadence:           time.Minute,
		CatchUpMode:       CatchUpLatestOnly,
		CatchUpWindow:     time.Hour,
		RunTimeout:        50 * time.Second,
		StaleTimeout:      2 * time.Minute,
		HeartbeatInterval: 10 * time.Second,
		AllowStaleReentry: true,
		MaxAttempts:       1,
		Scopes:            StaticScopes(ScopeGlobal),
		Handler: func(ctx context.Context, _ HandlerInput) (HandlerResult, error) {
			n, err := reconciler.ReconcileUnjudgedPairs(ctx)
			return HandlerResult{RowsAffected: int64(n)}, err
		},
	}
}
