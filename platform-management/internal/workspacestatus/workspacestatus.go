// Package workspacestatus is the platform's write path for a workspace's
// state. Set writes the platform_workspace row and its audit row in one
// transaction, then deletes the cache key that the Multica server reads, so the
// server sees the change at once instead of after the cache TTL.
package workspacestatus

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/workspacestate"
)

// AuditAction is the audit action of a state change (spec 4.3 catalog).
const AuditAction = audit.ActionWorkspaceStatusChanged

const auditTargetType = "workspace"

// Change is one request to set a workspace's state.
type Change struct {
	WorkspaceID string
	Status      workspacestate.Status
	Reason      string
	// ChangedBy is the staff user who made the change. It is the audit actor.
	ChangedBy string
	// ActorLabel is the staff member's email. It is copied to the audit row's
	// actor_label, which is how the row reads without a join to the user table.
	ActorLabel string
	// DeletionScheduledAt is when a pending deletion is carried out. It is only
	// valid with the pending_deletion status; zero means none.
	DeletionScheduledAt time.Time
}

// Tx is the reads and writes of one Set, all inside one transaction.
type Tx interface {
	// Current reads the workspace's row and locks it. found is false when the
	// workspace has no row, which is active.
	Current(ctx context.Context, workspaceID string) (rec workspacestate.Record, found bool, err error)
	// Write upserts the row, and reports whether it wrote one. The upsert writes
	// only when the status differs from the stored one.
	Write(ctx context.Context, c Change) (written bool, err error)
	audit.Writer
}

// Runner runs fn in one transaction: it commits when fn returns nil and rolls
// back otherwise.
type Runner interface {
	InTx(ctx context.Context, fn func(Tx) error) error
}

// Invalidator drops the cached state of a workspace. *workspacestate.Guard is one.
type Invalidator interface {
	Invalidate(ctx context.Context, workspaceID string) error
}

// Service sets workspace states.
type Service struct {
	runner Runner
	cache  Invalidator
	log    *slog.Logger
}

// New returns a Service. A nil log uses slog.Default().
func New(runner Runner, cache Invalidator, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{runner: runner, cache: cache, log: log}
}

// ValidStatus reports whether s is one of the four workspace states. The
// database does not check it; this is the check.
func ValidStatus(s workspacestate.Status) bool {
	switch s {
	case workspacestate.Active, workspacestate.ReadOnly, workspacestate.Suspended, workspacestate.PendingDeletion:
		return true
	}
	return false
}

// canonicalUUID returns the lower-case text form of a UUID. The cache key is
// built from it, so a differently spelled id would clear the wrong key.
func canonicalUUID(s string) (string, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil || !u.Valid {
		return "", fmt.Errorf("invalid uuid %q", s)
	}
	b := u.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// Set moves a workspace to c.Status and reports whether the state changed. Setting
// the state it already has writes nothing: no row, no audit entry, no cache
// deletion. A cache deletion failure is logged and not returned; the cached
// state then goes stale for at most workspacestate.TTL.
func (s *Service) Set(ctx context.Context, c Change) (changed bool, err error) {
	if !ValidStatus(c.Status) {
		return false, fmt.Errorf("workspacestatus: unknown status %q", c.Status)
	}
	if c.WorkspaceID, err = canonicalUUID(c.WorkspaceID); err != nil {
		return false, fmt.Errorf("workspacestatus: workspace id: %w", err)
	}
	if c.ChangedBy, err = canonicalUUID(c.ChangedBy); err != nil {
		return false, fmt.Errorf("workspacestatus: changed by: %w", err)
	}
	if !c.DeletionScheduledAt.IsZero() && c.Status != workspacestate.PendingDeletion {
		return false, errors.New("workspacestatus: a deletion date needs the pending_deletion status")
	}

	err = s.runner.InTx(ctx, func(tx Tx) error {
		cur, found, err := tx.Current(ctx, c.WorkspaceID)
		if err != nil {
			return fmt.Errorf("read current state: %w", err)
		}
		from := workspacestate.Active
		if found {
			from = cur.Status
		}
		if from == c.Status {
			return nil
		}
		written, err := tx.Write(ctx, c)
		if err != nil {
			return fmt.Errorf("write state: %w", err)
		}
		if !written {
			return nil
		}
		if err := audit.Record(ctx, tx, auditEntry(c, from)); err != nil {
			return fmt.Errorf("record audit event: %w", err)
		}
		changed = true
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("workspacestatus: %w", err)
	}
	if !changed {
		return false, nil
	}
	if err := s.cache.Invalidate(ctx, c.WorkspaceID); err != nil {
		s.log.Warn("workspacestatus: cache key not deleted; the cached state stays until it expires",
			"workspace_id", c.WorkspaceID, "ttl", workspacestate.TTL, "error", err)
	}
	return true, nil
}

func auditEntry(c Change, from workspacestate.Status) audit.Entry {
	detail := map[string]any{
		"from":   string(from),
		"to":     string(c.Status),
		"reason": c.Reason,
	}
	if !c.DeletionScheduledAt.IsZero() {
		detail["deletion_scheduled_at"] = c.DeletionScheduledAt.UTC().Format(time.RFC3339)
	}
	return audit.Entry{
		WorkspaceID: c.WorkspaceID,
		ActorType:   audit.ActorStaff,
		ActorID:     c.ChangedBy,
		ActorLabel:  c.ActorLabel,
		Action:      AuditAction,
		TargetType:  auditTargetType,
		TargetID:    c.WorkspaceID,
		Detail:      detail,
	}
}
