package backfill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/db"
)

// Beginner starts a transaction. *pgxpool.Pool is one.
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// PgxRunner is the Runner over a Postgres pool.
type PgxRunner struct{ pool Beginner }

// NewPgxRunner returns a Runner that opens one transaction per batch.
func NewPgxRunner(pool Beginner) *PgxRunner { return &PgxRunner{pool: pool} }

// InTx implements Runner.
func (r *PgxRunner) InTx(ctx context.Context, fn func(Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // a no-op once committed
	if err := fn(&pgxTx{q: db.New(tx)}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type pgxTx struct{ q *db.Queries }

func (t *pgxTx) Unsubscribed(ctx context.Context, limit int) ([]Candidate, error) {
	rows, err := t.q.ListWorkspacesWithoutSubscription(ctx, int32(limit))
	if err != nil {
		return nil, err
	}
	out := make([]Candidate, len(rows))
	for i, r := range rows {
		out[i] = Candidate{ID: r.ID.String(), Name: r.Name}
	}
	return out, nil
}

func (t *pgxTx) InsertFree(ctx context.Context, workspaceID string) (bool, error) {
	var uid pgtype.UUID
	if err := uid.Scan(workspaceID); err != nil || !uid.Valid {
		return false, fmt.Errorf("invalid workspace id %q", workspaceID)
	}
	// The insert sets version 1; the other columns are the free plan's defaults
	// with no period, trial or grace date.
	_, err := t.q.InsertWorkspaceSubscription(ctx, db.InsertWorkspaceSubscriptionParams{
		WorkspaceID: uid, PlanKey: Plan, Status: "active", Seats: 0, Interval: "month",
		LimitOverrides: []byte("{}"),
	})
	if errors.Is(err, pgx.ErrNoRows) { // ON CONFLICT DO NOTHING
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// InsertAuditEvent implements audit.Writer inside the transaction.
func (t *pgxTx) InsertAuditEvent(ctx context.Context, e audit.Entry) error {
	ws, err := parseOptionalUUID(e.WorkspaceID)
	if err != nil {
		return err
	}
	raw, err := marshalDetail(e.Detail)
	if err != nil {
		return err
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
	})
}

func parseOptionalUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if s == "" {
		return u, nil
	}
	if err := u.Scan(s); err != nil || !u.Valid {
		return pgtype.UUID{}, fmt.Errorf("invalid uuid %q", s)
	}
	return u, nil
}

func marshalDetail(detail map[string]any) ([]byte, error) {
	if detail == nil {
		detail = map[string]any{}
	}
	return json.Marshal(detail)
}
