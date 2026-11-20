package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/multica-ai/multica/platform-management/accountdeletion"
	"github.com/multica-ai/multica/server/internal/legalhold"
)

// JobNameAccountDeletion is the canonical name used in audit rows. Stable
// across releases.
const JobNameAccountDeletion = "account_deletion"

// AccountDeleter finds and carries out due account deletions. The handler
// package implements it, because the deletion reuses the workspace teardown and
// the member removal that live there.
type AccountDeleter interface {
	// DueAccountDeletions lists up to limit user ids that are pending and due at now.
	DueAccountDeletions(ctx context.Context, now time.Time, limit int) ([]string, error)
	// DeleteAccount deletes one account. It returns false, with no error, when
	// the account is no longer pending or not due.
	DeleteAccount(ctx context.Context, userID string, now time.Time) (bool, error)
}

// AccountDeletionJob runs daily. Each run processes at most
// accountdeletion.BatchSize accounts, each in its own transaction. A failure is
// logged and leaves that account pending for the next run, and a second run
// finds nothing left to do for an account that is already deleted.
func AccountDeletionJob(deleter AccountDeleter) JobSpec {
	return JobSpec{
		Name:              JobNameAccountDeletion,
		Cadence:           24 * time.Hour,
		CatchUpMode:       CatchUpLatestOnly,
		CatchUpWindow:     24 * time.Hour,
		RunTimeout:        30 * time.Minute,
		StaleTimeout:      35 * time.Minute,
		HeartbeatInterval: 30 * time.Second,
		AllowStaleReentry: true,
		MaxAttempts:       3,
		RetryBackoff:      []time.Duration{time.Minute, 5 * time.Minute},
		Scopes:            StaticScopes(ScopeGlobal),
		Handler: func(ctx context.Context, _ HandlerInput) (HandlerResult, error) {
			deleted, err := runAccountDeletions(ctx, deleter, time.Now())
			if err != nil {
				return HandlerResult{}, err
			}
			return HandlerResult{RowsAffected: int64(deleted)}, nil
		},
	}
}

// runAccountDeletions processes one batch and returns how many accounts it
// deleted. A failed account is logged and skipped, so one bad account cannot
// hold back the others.
func runAccountDeletions(ctx context.Context, deleter AccountDeleter, now time.Time) (int, error) {
	ids, err := deleter.DueAccountDeletions(ctx, now, accountdeletion.BatchSize)
	if err != nil {
		return 0, err
	}
	deleted, failed := 0, 0
	for _, id := range ids {
		if ctx.Err() != nil {
			break
		}
		done, err := deleter.DeleteAccount(ctx, id, now)
		switch {
		case errors.Is(err, legalhold.ErrHeld):
			slog.Info("account deletion skipped: legal hold", "user_id", id)
		case err != nil:
			failed++
			slog.Error("account deletion failed; the account stays pending and the next run retries", "user_id", id, "error", err)
		case done:
			deleted++
		}
	}
	slog.Info("account deletion", "due", len(ids), "deleted", deleted, "failed", failed)
	return deleted, nil
}
