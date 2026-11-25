package scheduler

import (
	"context"
	"time"
)

// IntentExpirer expires the pending intents past their time (durable spec 5.2).
type IntentExpirer interface {
	Expire(ctx context.Context) (int, error)
}

// DurableIntentExpiryJob expires pending intents every minute. An intent is expired
// once, and its agent is woken once, so a run that finds nothing new writes nothing.
func DurableIntentExpiryJob(runner IntentExpirer) JobSpec {
	return JobSpec{
		Name: "durable_intent_expiry", Cadence: time.Minute, CatchUpMode: CatchUpLatestOnly, CatchUpWindow: time.Hour,
		RunTimeout: 50 * time.Second, StaleTimeout: 2 * time.Minute, HeartbeatInterval: 10 * time.Second,
		AllowStaleReentry: true, MaxAttempts: 1, Scopes: StaticScopes(ScopeGlobal),
		Handler: func(ctx context.Context, _ HandlerInput) (HandlerResult, error) {
			_, err := runner.Expire(ctx)
			return HandlerResult{}, err
		},
	}
}
