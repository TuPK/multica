package scheduler

import (
	"context"
	"time"
)

// SeatDossierCleaner files the monthly cleanup issues of the seat dossiers (durable spec 9.2).
type SeatDossierCleaner interface {
	CleanupMonthly(ctx context.Context) (int, error)
}

// DurableSeatCleanupJob runs once a day. The cleaner files each seat's cleanup issue once a
// month, so a run that finds the month's issues filed writes nothing.
func DurableSeatCleanupJob(cleaner SeatDossierCleaner) JobSpec {
	return JobSpec{
		Name:              "durable_seat_cleanup",
		Cadence:           24 * time.Hour,
		CatchUpMode:       CatchUpLatestOnly,
		CatchUpWindow:     24 * time.Hour,
		RunTimeout:        10 * time.Minute,
		StaleTimeout:      15 * time.Minute,
		HeartbeatInterval: 30 * time.Second,
		AllowStaleReentry: true,
		MaxAttempts:       3,
		RetryBackoff:      []time.Duration{time.Minute, 5 * time.Minute},
		Scopes:            StaticScopes(ScopeGlobal),
		Handler: func(ctx context.Context, _ HandlerInput) (HandlerResult, error) {
			n, err := cleaner.CleanupMonthly(ctx)
			return HandlerResult{RowsAffected: int64(n)}, err
		},
	}
}
