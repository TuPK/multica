package seat

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/internal/db"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
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
	if err := fn(&pgxTx{q: db.New(tx), subs: subscription.NewPgxReader(tx), plans: plan.NewPgxReader(tx)}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type pgxTx struct {
	q     *db.Queries
	subs  *subscription.PgxReader
	plans *plan.PgxReader
}

func parseUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil || !u.Valid {
		return pgtype.UUID{}, fmt.Errorf("invalid uuid %q", s)
	}
	return u, nil
}

func uuidText(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return u.String()
}

func timestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func fromRow(r db.PlatformSeatOperation) Op {
	op := Op{
		WorkspaceID: uuidText(r.WorkspaceID), Token: uuidText(r.OperationToken), Kind: r.Kind,
		SubjectID: uuidText(r.SubjectID), State: r.State, Reason: r.Reason,
	}
	if r.ExpiresAt.Valid {
		v := r.ExpiresAt.Time.UTC()
		op.ExpiresAt = &v
	}
	return op
}

func (t *pgxTx) Lock(ctx context.Context, workspaceID string) error {
	ns, key := LockKey(workspaceID)
	return t.q.LockSeatWorkspace(ctx, db.LockSeatWorkspaceParams{Column1: ns, Column2: key})
}

func (t *pgxTx) Subscription(ctx context.Context, ws string) (subscription.Subscription, bool, error) {
	return t.subs.Get(ctx, ws)
}

func (t *pgxTx) Plan(ctx context.Context, key string) (plan.Plan, bool, error) {
	return t.plans.Get(ctx, key)
}

func (t *pgxTx) Members(ctx context.Context, ws string) (int64, error) {
	id, err := parseUUID(ws)
	if err != nil {
		return 0, err
	}
	return t.q.CountWorkspaceMembers(ctx, id)
}

func (t *pgxTx) Held(ctx context.Context, ws string, now time.Time) (int64, error) {
	id, err := parseUUID(ws)
	if err != nil {
		return 0, err
	}
	return t.q.CountHeldSeatOperations(ctx, db.CountHeldSeatOperationsParams{
		WorkspaceID: id, ExpiresAt: pgtype.Timestamptz{Time: now, Valid: true},
	})
}

func (t *pgxTx) Operation(ctx context.Context, ws, token string) (Op, bool, error) {
	w, err := parseUUID(ws)
	if err != nil {
		return Op{}, false, err
	}
	tk, err := parseUUID(token)
	if err != nil {
		return Op{}, false, err
	}
	row, err := t.q.GetSeatOperation(ctx, db.GetSeatOperationParams{WorkspaceID: w, OperationToken: tk})
	if errors.Is(err, pgx.ErrNoRows) {
		return Op{}, false, nil
	}
	if err != nil {
		return Op{}, false, err
	}
	return fromRow(row), true, nil
}

func (t *pgxTx) Insert(ctx context.Context, op Op) (Op, bool, error) {
	w, err := parseUUID(op.WorkspaceID)
	if err != nil {
		return Op{}, false, err
	}
	tk, err := parseUUID(op.Token)
	if err != nil {
		return Op{}, false, err
	}
	subject, err := parseUUID(op.SubjectID)
	if err != nil {
		return Op{}, false, err
	}
	row, err := t.q.InsertSeatOperation(ctx, db.InsertSeatOperationParams{
		WorkspaceID: w, OperationToken: tk, Kind: op.Kind, SubjectID: subject, State: op.State,
		ExpiresAt: timestamptz(op.ExpiresAt),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Op{}, false, nil
	}
	if err != nil {
		return Op{}, false, err
	}
	return fromRow(row), true, nil
}

func (t *pgxTx) SetState(ctx context.Context, ws, token, state, reason string, expiresAt *time.Time) (Op, error) {
	w, err := parseUUID(ws)
	if err != nil {
		return Op{}, err
	}
	tk, err := parseUUID(token)
	if err != nil {
		return Op{}, err
	}
	row, err := t.q.SetSeatOperationState(ctx, db.SetSeatOperationStateParams{
		WorkspaceID: w, OperationToken: tk, State: state, Reason: reason, ExpiresAt: timestamptz(expiresAt),
	})
	if err != nil {
		return Op{}, err
	}
	return fromRow(row), nil
}
