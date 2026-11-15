package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// JobNameAuditRetention is the canonical name used in audit rows. Stable across
// releases.
const JobNameAuditRetention = "audit_retention"

const (
	// DefaultAuditRetentionDays applies when AUDIT_RETENTION_DAYS is unset.
	DefaultAuditRetentionDays = 365
	// MinAuditRetentionDays is the shortest retention the server accepts.
	MinAuditRetentionDays = 30
	// auditRetentionBatch bounds one run. Rows beyond it wait for the next run.
	auditRetentionBatch = 5000
)

// ParseAuditRetentionDays reads the AUDIT_RETENTION_DAYS value. An empty value
// means the default; anything that is not a whole number of at least the
// minimum is an error.
func ParseAuditRetentionDays(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultAuditRetentionDays, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("AUDIT_RETENTION_DAYS must be a whole number of days, got %q", raw)
	}
	if n < MinAuditRetentionDays {
		return 0, fmt.Errorf("AUDIT_RETENTION_DAYS must be at least %d, got %d", MinAuditRetentionDays, n)
	}
	return n, nil
}

// AuditEventDeleter deletes audit rows. *db.Queries satisfies it.
type AuditEventDeleter interface {
	DeleteAuditEventsBefore(ctx context.Context, arg db.DeleteAuditEventsBeforeParams) (int64, error)
}

// AuditRetentionJob runs daily and deletes at most one batch of audit rows
// older than retentionDays. A second run finds nothing more to delete once the
// backlog is gone, so a retry after a failure is safe.
func AuditRetentionJob(deleter AuditEventDeleter, retentionDays int) JobSpec {
	return JobSpec{
		Name:              JobNameAuditRetention,
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
			cutoff := time.Now().AddDate(0, 0, -retentionDays)
			n, err := deleter.DeleteAuditEventsBefore(ctx, db.DeleteAuditEventsBeforeParams{
				CreatedAt: pgtype.Timestamptz{Time: cutoff, Valid: true},
				Limit:     auditRetentionBatch,
			})
			if err != nil {
				return HandlerResult{}, fmt.Errorf("audit retention: %w", err)
			}
			slog.Info("audit retention", "deleted", n)
			return HandlerResult{RowsAffected: n}, nil
		},
	}
}
