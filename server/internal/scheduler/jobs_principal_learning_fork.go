package scheduler

import (
	"context"
	"time"
)

// PrincipalLearner posts the profile update batches of the principal profiles (durable spec 6.3).
type PrincipalLearner interface {
	LearnBatches(ctx context.Context, now time.Time) (int, error)
}

// PrincipalLearningJob runs every six hours. Each person gets at most one batch per window, so a run
// that finds the window's batch already posted writes nothing.
func PrincipalLearningJob(learner PrincipalLearner) JobSpec {
	return JobSpec{
		Name:              "principal_learning",
		Cadence:           6 * time.Hour,
		CatchUpMode:       CatchUpLatestOnly,
		CatchUpWindow:     6 * time.Hour,
		RunTimeout:        10 * time.Minute,
		StaleTimeout:      15 * time.Minute,
		HeartbeatInterval: 30 * time.Second,
		AllowStaleReentry: true,
		MaxAttempts:       3,
		RetryBackoff:      []time.Duration{time.Minute, 5 * time.Minute},
		Scopes:            StaticScopes(ScopeGlobal),
		Handler: func(ctx context.Context, _ HandlerInput) (HandlerResult, error) {
			n, err := learner.LearnBatches(ctx, time.Now())
			return HandlerResult{RowsAffected: int64(n)}, err
		},
	}
}
