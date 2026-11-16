package subscription

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/db"
	"github.com/multica-ai/multica/platform-management/internal/plan"
)

// Beginner starts a transaction. *pgxpool.Pool is one.
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// PgxRunner is the Runner over a Postgres pool.
type PgxRunner struct{ pool Beginner }

// NewPgxRunner returns a Runner that opens one transaction per call.
func NewPgxRunner(pool Beginner) *PgxRunner { return &PgxRunner{pool: pool} }

// InTx implements Runner. It returns fn's error as it is, so a caller can still
// recognise an *Error.
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

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func fromRow(r db.WorkspaceSubscription) (Subscription, error) {
	overrides := plan.Limits{}
	if err := json.Unmarshal(r.LimitOverrides, &overrides); err != nil {
		return Subscription{}, fmt.Errorf("subscription %s: limit_overrides: %w", uuidText(r.WorkspaceID), err)
	}
	if overrides == nil {
		overrides = plan.Limits{}
	}
	pending, err := pendingFromJSON(r.PendingChange)
	if err != nil {
		return Subscription{}, fmt.Errorf("subscription %s: pending_change: %w", uuidText(r.WorkspaceID), err)
	}
	var unitPrice *int64
	if r.UnitPrice.Valid {
		v := r.UnitPrice.Int64
		unitPrice = &v
	}
	return Subscription{
		WorkspaceID: uuidText(r.WorkspaceID), PlanKey: r.PlanKey, Status: r.Status, Seats: int(r.Seats),
		Interval: r.Interval, CurrentPeriodStart: timePtr(r.CurrentPeriodStart), CurrentPeriodEnd: timePtr(r.CurrentPeriodEnd),
		TrialEnd: timePtr(r.TrialEnd), GraceUntil: timePtr(r.GraceUntil), CancelAtPeriodEnd: r.CancelAtPeriodEnd,
		LimitOverrides: overrides, Version: r.Version, UpdatedAt: r.UpdatedAt.Time.UTC(),
		PendingChange: pending, UnitPrice: unitPrice,
	}, nil
}

// pendingFromJSON reads the pending_change column. NULL, a JSON null and an
// empty object all mean no pending change.
func pendingFromJSON(raw []byte) (*PendingChange, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var p PendingChange
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	if p.PlanKey == "" {
		return nil, nil
	}
	return &p, nil
}

// pendingToJSON is the column value for pending_change: NULL when there is none.
func pendingToJSON(p *PendingChange) ([]byte, error) {
	if p == nil {
		return nil, nil
	}
	return json.Marshal(p)
}

func int8Ptr(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

type pgxTx struct{ q *db.Queries }

func (t *pgxTx) WorkspaceName(ctx context.Context, id string) (string, bool, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return "", false, err
	}
	name, err := t.q.GetWorkspaceName(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return name, err == nil, err
}

func (t *pgxTx) Get(ctx context.Context, workspaceID string) (Subscription, bool, error) {
	uid, err := parseUUID(workspaceID)
	if err != nil {
		return Subscription{}, false, err
	}
	row, err := t.q.GetWorkspaceSubscriptionForUpdate(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return Subscription{}, false, nil
	}
	if err != nil {
		return Subscription{}, false, err
	}
	s, err := fromRow(row)
	return s, err == nil, err
}

func (t *pgxTx) Insert(ctx context.Context, s Subscription) (Subscription, bool, error) {
	uid, err := parseUUID(s.WorkspaceID)
	if err != nil {
		return Subscription{}, false, err
	}
	overrides, err := json.Marshal(orEmpty(s.LimitOverrides))
	if err != nil {
		return Subscription{}, false, err
	}
	pending, err := pendingToJSON(s.PendingChange)
	if err != nil {
		return Subscription{}, false, err
	}
	row, err := t.q.InsertWorkspaceSubscription(ctx, db.InsertWorkspaceSubscriptionParams{
		WorkspaceID: uid, PlanKey: s.PlanKey, Status: s.Status, Seats: int32(s.Seats), Interval: s.Interval,
		CurrentPeriodStart: timestamptz(s.CurrentPeriodStart), CurrentPeriodEnd: timestamptz(s.CurrentPeriodEnd),
		TrialEnd: timestamptz(s.TrialEnd), GraceUntil: timestamptz(s.GraceUntil),
		CancelAtPeriodEnd: s.CancelAtPeriodEnd, LimitOverrides: overrides,
		PendingChange: pending, UnitPrice: int8Ptr(s.UnitPrice),
	})
	if errors.Is(err, pgx.ErrNoRows) { // ON CONFLICT DO NOTHING returned no row
		return Subscription{}, false, nil
	}
	if err != nil {
		return Subscription{}, false, err
	}
	out, err := fromRow(row)
	return out, err == nil, err
}

func (t *pgxTx) Update(ctx context.Context, s Subscription) (Subscription, error) {
	uid, err := parseUUID(s.WorkspaceID)
	if err != nil {
		return Subscription{}, err
	}
	overrides, err := json.Marshal(orEmpty(s.LimitOverrides))
	if err != nil {
		return Subscription{}, err
	}
	pending, err := pendingToJSON(s.PendingChange)
	if err != nil {
		return Subscription{}, err
	}
	row, err := t.q.UpdateWorkspaceSubscription(ctx, db.UpdateWorkspaceSubscriptionParams{
		WorkspaceID: uid, PlanKey: s.PlanKey, Seats: int32(s.Seats), Interval: s.Interval,
		CurrentPeriodStart: timestamptz(s.CurrentPeriodStart), CurrentPeriodEnd: timestamptz(s.CurrentPeriodEnd),
		TrialEnd: timestamptz(s.TrialEnd), GraceUntil: timestamptz(s.GraceUntil),
		CancelAtPeriodEnd: s.CancelAtPeriodEnd, LimitOverrides: overrides,
		PendingChange: pending, UnitPrice: int8Ptr(s.UnitPrice),
	})
	if err != nil {
		return Subscription{}, err
	}
	return fromRow(row)
}

func (t *pgxTx) Plan(ctx context.Context, key string) (plan.Plan, bool, error) {
	row, err := t.q.GetPlan(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return plan.Plan{}, false, nil
	}
	if err != nil {
		return plan.Plan{}, false, err
	}
	p, err := plan.FromRow(row)
	return p, err == nil, err
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

// PgxReader is the Reader over a Postgres pool.
type PgxReader struct{ q *db.Queries }

// NewPgxReader returns a Reader over pool.
func NewPgxReader(pool db.DBTX) *PgxReader { return &PgxReader{q: db.New(pool)} }

// Get implements Reader.
func (r *PgxReader) Get(ctx context.Context, workspaceID string) (Subscription, bool, error) {
	uid, err := parseUUID(workspaceID)
	if err != nil {
		return Subscription{}, false, err
	}
	row, err := r.q.GetWorkspaceSubscription(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return Subscription{}, false, nil
	}
	if err != nil {
		return Subscription{}, false, err
	}
	s, err := fromRow(row)
	return s, err == nil, err
}

// PgxMembers is the MemberReader over the shared member table.
type PgxMembers struct{ q *db.Queries }

// NewPgxMembers returns a MemberReader over pool.
func NewPgxMembers(pool db.DBTX) *PgxMembers { return &PgxMembers{q: db.New(pool)} }

// Role implements MemberReader.
func (m *PgxMembers) Role(ctx context.Context, workspaceID, userID string) (string, bool, error) {
	ws, err := parseUUID(workspaceID)
	if err != nil {
		return "", false, err
	}
	user, err := parseUUID(userID)
	if err != nil {
		return "", false, err
	}
	role, err := m.q.GetMemberRole(ctx, db.GetMemberRoleParams{WorkspaceID: ws, UserID: user})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return role, err == nil, err
}

// Count implements MemberReader.
func (m *PgxMembers) Count(ctx context.Context, workspaceID string) (int64, error) {
	ws, err := parseUUID(workspaceID)
	if err != nil {
		return 0, err
	}
	return m.q.CountWorkspaceMembers(ctx, ws)
}

// Reserved implements MemberReader: the operations that hold a seat and have
// not expired.
func (m *PgxMembers) Reserved(ctx context.Context, workspaceID string) (int64, error) {
	ws, err := parseUUID(workspaceID)
	if err != nil {
		return 0, err
	}
	return m.q.CountHeldSeatOperations(ctx, db.CountHeldSeatOperationsParams{
		WorkspaceID: ws, ExpiresAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
}

// PgxPending is the PendingReader over billing_order.
type PgxPending struct{ q *db.Queries }

// NewPgxPending returns a PendingReader over pool.
func NewPgxPending(pool db.DBTX) *PgxPending { return &PgxPending{q: db.New(pool)} }

// PendingSeats implements PendingReader.
func (p *PgxPending) PendingSeats(ctx context.Context, workspaceID string, now time.Time) (PendingPurchase, bool, error) {
	ws, err := parseUUID(workspaceID)
	if err != nil {
		return PendingPurchase{}, false, err
	}
	row, err := p.q.GetPendingAddSeatsOrder(ctx, db.GetPendingAddSeatsOrderParams{
		WorkspaceID: ws, ExpiresAt: pgtype.Timestamptz{Time: now, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return PendingPurchase{}, false, nil
	}
	if err != nil {
		return PendingPurchase{}, false, err
	}
	return PendingPurchase{OrderID: uuidText(row.ID), Seats: int64(row.Seats), ExpiresAt: row.ExpiresAt.Time.UTC()}, true, nil
}

// PgxPendingOrders is the PendingOrders over billing_order.
type PgxPendingOrders struct{ q *db.Queries }

// NewPgxPendingOrders returns a PendingOrders over pool.
func NewPgxPendingOrders(pool db.DBTX) *PgxPendingOrders { return &PgxPendingOrders{q: db.New(pool)} }

// UnpaidUsageOrder returns the workspace's oldest unpaid usage invoice, found ==
// false when it has none (spec 7b.3, rule 13).
func (p *PgxPendingOrders) UnpaidUsageOrder(ctx context.Context, workspaceID string) (string, bool, error) {
	ws, err := parseUUID(workspaceID)
	if err != nil {
		return "", false, err
	}
	row, err := p.q.GetUnpaidUsageOrderForWorkspace(ctx, ws)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return uuidText(row.ID), true, nil
}

// PendingOrder implements PendingOrders.
func (p *PgxPendingOrders) PendingOrder(ctx context.Context, workspaceID string) (string, bool, error) {
	ws, err := parseUUID(workspaceID)
	if err != nil {
		return "", false, err
	}
	row, err := p.q.GetPendingBillingOrderForWorkspace(ctx, ws)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return uuidText(row.ID), true, nil
}
