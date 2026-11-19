package workspacestatus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/db"
	"github.com/multica-ai/multica/platform-management/workspacestate"
)

// Beginner starts a transaction. *pgxpool.Pool is one.
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// PgxRunner is the Runner over a Postgres pool.
type PgxRunner struct{ pool Beginner }

// NewPgxRunner returns a Runner that opens one transaction per call.
func NewPgxRunner(pool Beginner) *PgxRunner { return &PgxRunner{pool: pool} }

// InTx implements Runner.
func (r *PgxRunner) InTx(ctx context.Context, fn func(Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // a no-op once committed
	if err := fn(NewTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type pgxTx struct{ q *db.Queries }

// NewTx returns the Tx over a transaction the caller began, for SetInTx.
func NewTx(dbtx db.DBTX) Tx { return &pgxTx{q: db.New(dbtx)} }

func parseUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid uuid %q: %w", s, err)
	}
	return u, nil
}

func (t *pgxTx) Current(ctx context.Context, workspaceID string) (workspacestate.Record, bool, error) {
	id, err := parseUUID(workspaceID)
	if err != nil {
		return workspacestate.Record{}, false, err
	}
	row, err := t.q.GetPlatformWorkspaceForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return workspacestate.Record{}, false, nil
	}
	if err != nil {
		return workspacestate.Record{}, false, err
	}
	rec := workspacestate.Record{Status: workspacestate.Status(row.Status)}
	if row.DeletionScheduledAt.Valid {
		rec.DeletionScheduledAt = row.DeletionScheduledAt.Time
	}
	return rec, true, nil
}

func (t *pgxTx) Write(ctx context.Context, c Change) (bool, error) {
	ws, err := parseUUID(c.WorkspaceID)
	if err != nil {
		return false, err
	}
	by, err := parseUUID(c.ChangedBy)
	if err != nil {
		return false, err
	}
	var scheduled pgtype.Timestamptz
	if !c.DeletionScheduledAt.IsZero() {
		scheduled = pgtype.Timestamptz{Time: c.DeletionScheduledAt, Valid: true}
	}
	n, err := t.q.UpsertPlatformWorkspaceStatus(ctx, db.UpsertPlatformWorkspaceStatusParams{
		WorkspaceID:         ws,
		Status:              string(c.Status),
		Reason:              c.Reason,
		ChangedBy:           by,
		DeletionScheduledAt: scheduled,
	})
	return n > 0, err
}

// InsertAuditEvent implements audit.Writer inside the transaction.
func (t *pgxTx) InsertAuditEvent(ctx context.Context, e audit.Entry) error {
	var ws pgtype.UUID
	if e.WorkspaceID != "" {
		u, err := parseUUID(e.WorkspaceID)
		if err != nil {
			return err
		}
		ws = u
	}
	detail := e.Detail
	if detail == nil {
		detail = map[string]any{}
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	var tenant pgtype.UUID
	if e.TenantID != "" {
		if err := tenant.Scan(e.TenantID); err != nil {
			return err
		}
	}
	return t.q.InsertAuditEvent(ctx, db.InsertAuditEventParams{
		WorkspaceID: ws,
		ActorType:   e.ActorType,
		ActorID:     pgtype.Text{String: e.ActorID, Valid: e.ActorID != ""},
		ActorLabel:  e.ActorLabel,
		Action:      e.Action,
		TargetType:  e.TargetType,
		TargetID:    e.TargetID,
		TargetLabel: e.TargetLabel,
		Detail:      raw,
		Ip:          e.IP,
		UserAgent:   e.UserAgent,
		RequestID:   e.RequestID,
		TenantID:    tenant,
	})
}
