package scheduler

import (
	"context"
	"time"
)

// PrincipalLearner enqueues the routine profile-update tasks for active principal profiles (durable spec 6.3).
type PrincipalLearner interface {
	LearnBatches(ctx context.Context, now time.Time) (int, error)
}

// PrincipalLearningJob runs every six hours. Decisions already picked up by a previous routine task
// are skipped, so re-running the job writes nothing new for the same decisions.
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
