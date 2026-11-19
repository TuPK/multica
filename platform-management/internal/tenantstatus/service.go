package tenantstatus

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/db"
	"github.com/multica-ai/multica/platform-management/internal/workspacestatus"
	"github.com/multica-ai/multica/platform-management/workspacestate"
)

// Beginner opens a transaction; *pgxpool.Pool is one.
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// ErrNotFound is returned for an unknown tenant.
var ErrNotFound = errors.New("tenantstatus: not found")

// Actor is the staff member who changes the status, for the audit rows.
type Actor struct {
	ID    string
	Label string
}

// Service locks and unlocks tenants. Cache drops each changed workspace's cached state after the commit.
type Service struct {
	pool  Beginner
	cache workspacestatus.Invalidator
}

// New returns a Service over pool.
func New(pool Beginner, cache workspacestatus.Invalidator) *Service {
	return &Service{pool: pool, cache: cache}
}

// SetStatus sets the tenant's status and the status of its workspaces in one
// transaction (spec R6 section 3c.9, rule 1), with one audit row for the tenant
// and the usual row for each workspace that changes. It returns the workspaces
// changed. The status is checked by the caller.
func (s *Service) SetStatus(ctx context.Context, by Actor, tenantID, status, reason string) ([]string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)
	tid, err := parseUUID(tenantID)
	if err != nil {
		return nil, ErrNotFound
	}
	if _, err := q.LockTenantForStatus(ctx, tid); errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("lock tenant: %w", err)
	}
	statusRows, err := q.TenantWorkspaceStatuses(ctx, tid)
	if err != nil {
		return nil, fmt.Errorf("read workspace statuses: %w", err)
	}
	rows := make([]WorkspaceStatus, len(statusRows))
	for i, r := range statusRows {
		rows[i] = WorkspaceStatus{ID: uuidText(r.WorkspaceID), Status: r.Status, Reason: r.Reason}
	}
	ids := Targets(rows, status)
	wsTx := workspacestatus.NewTx(tx)
	nextReason := ReasonTenant
	if status == StatusActive {
		nextReason = ""
	}
	var changed []string
	for _, id := range ids {
		moved, err := workspacestatus.SetInTx(ctx, wsTx, workspacestatus.Change{
			WorkspaceID: id, Status: workspacestate.Status(status), Reason: nextReason,
			ChangedBy: by.ID, ActorLabel: by.Label,
		})
		if err != nil {
			return nil, err
		}
		if moved {
			changed = append(changed, id)
		}
	}
	if err := q.SetTenantStatus(ctx, db.SetTenantStatusParams{
		ID: tid, Status: status, StatusReason: reason, StatusChangedBy: uuidOf(by.ID),
	}); err != nil {
		return nil, fmt.Errorf("set tenant status: %w", err)
	}
	if err := audit.Record(ctx, wsTx, audit.Entry{
		TenantID: tenantID, ActorType: audit.ActorStaff, ActorID: by.ID, ActorLabel: by.Label,
		Action: audit.ActionTenantStatusChanged, TargetType: "tenant", TargetID: tenantID,
		Detail: map[string]any{"status": status, "reason": reason, "workspace_ids": ids},
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	if s.cache != nil {
		for _, id := range changed {
			_ = s.cache.Invalidate(ctx, id)
		}
	}
	return changed, nil
}

func parseUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil || !u.Valid {
		return pgtype.UUID{}, fmt.Errorf("invalid uuid %q", s)
	}
	return u, nil
}

func uuidOf(s string) pgtype.UUID {
	u, err := parseUUID(s)
	if err != nil {
		return pgtype.UUID{}
	}
	return u
}

func uuidText(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
