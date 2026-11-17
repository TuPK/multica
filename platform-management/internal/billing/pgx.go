package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/internal/db"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
	"github.com/multica-ai/multica/platform-management/internal/workspacestatus"
)

const (
	pgUniqueViolation = "23505"
	transferIndex     = "billing_order_transfer_uidx"
)

// Beginner starts a transaction. *pgxpool.Pool is one.
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// PgxRunner is the Runner over a Postgres pool.
type PgxRunner struct{ pool Beginner }

// NewPgxRunner returns a Runner that opens one transaction per call.
func NewPgxRunner(pool Beginner) *PgxRunner { return &PgxRunner{pool: pool} }

// InTx implements Runner. It returns fn's error as it is, so a caller can
// still recognise a sentinel.
func (r *PgxRunner) InTx(ctx context.Context, fn func(Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // a no-op once committed
	if err := fn(newPgxTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type pgxTx struct {
	workspacestatus.Tx
	q *db.Queries
}

func newPgxTx(tx pgx.Tx) *pgxTx {
	return &pgxTx{Tx: workspacestatus.NewTx(tx), q: db.New(tx)}
}

// NewTx returns the Tx over a transaction the caller began, for a writer that
// puts an order and a subscription in the same transaction as its own rows.
func NewTx(tx pgx.Tx) Tx { return newPgxTx(tx) }

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

func optionalUUID(s string) (pgtype.UUID, error) {
	if s == "" {
		return pgtype.UUID{}, nil
	}
	return parseUUID(s)
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

func orderFromRow(r db.BillingOrder) Order {
	return Order{
		ID: uuidText(r.ID), WorkspaceID: uuidText(r.WorkspaceID), Kind: Kind(r.Kind), PlanKey: r.PlanKey,
		Interval: r.Interval, Seats: int(r.Seats), Amount: r.Amount, VATAmount: r.VatAmount, Currency: r.Currency,
		Status: Status(r.Status), Provider: r.Provider, ProviderRef: r.ProviderRef, TransferCode: r.TransferCode,
		IdempotencyKey: r.IdempotencyKey, Buyer: json.RawMessage(r.Buyer),
		PeriodStart: timePtr(r.PeriodStart), PeriodEnd: timePtr(r.PeriodEnd), EInvoiceRef: r.EinvoiceRef,
		CreatedBy: uuidText(r.CreatedBy), ConfirmedBy: uuidText(r.ConfirmedBy), PaidAt: timePtr(r.PaidAt),
		ExpiresAt: r.ExpiresAt.Time.UTC(), CreatedAt: r.CreatedAt.Time.UTC(),
	}
}

func subscriptionFromRow(r db.WorkspaceSubscription) (subscription.Subscription, error) {
	return subscription.FromRow(r)
}

func (t *pgxTx) InsertOrder(ctx context.Context, n NewOrder) (Order, bool, error) {
	ws, err := parseUUID(n.WorkspaceID)
	if err != nil {
		return Order{}, false, err
	}
	by, err := optionalUUID(n.CreatedBy)
	if err != nil {
		return Order{}, false, err
	}
	row, err := t.q.CreateBillingOrder(ctx, db.CreateBillingOrderParams{
		WorkspaceID: ws, Kind: string(n.Kind), PlanKey: n.PlanKey, Interval: n.Interval, Seats: int32(n.Seats),
		Amount: n.Amount, VatAmount: n.VATAmount, Currency: n.Currency, Provider: n.Provider,
		TransferCode: n.TransferCode, IdempotencyKey: n.IdempotencyKey, Buyer: n.Buyer,
		PeriodStart: timestamptz(n.PeriodStart), PeriodEnd: timestamptz(n.PeriodEnd), CreatedBy: by,
		ExpiresAt: timestamptz(&n.ExpiresAt), CreatedAt: timestamptz(&n.CreatedAt),
	})
	if errors.Is(err, pgx.ErrNoRows) { // ON CONFLICT DO NOTHING returned no row
		return Order{}, false, nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation && pgErr.ConstraintName == transferIndex {
		return Order{}, false, ErrTransferCodeTaken
	}
	if err != nil {
		return Order{}, false, err
	}
	return orderFromRow(row), true, nil
}

func (t *pgxTx) OrderByIdempotencyKey(ctx context.Context, workspaceID, key string) (Order, bool, error) {
	ws, err := parseUUID(workspaceID)
	if err != nil {
		return Order{}, false, err
	}
	row, err := t.q.GetBillingOrderByIdempotencyKey(ctx, db.GetBillingOrderByIdempotencyKeyParams{WorkspaceID: ws, IdempotencyKey: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, false, nil
	}
	if err != nil {
		return Order{}, false, err
	}
	return orderFromRow(row), true, nil
}

func (t *pgxTx) LockOrder(ctx context.Context, id string) (Order, bool, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return Order{}, false, err
	}
	row, err := t.q.GetBillingOrderForUpdate(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, false, nil
	}
	if err != nil {
		return Order{}, false, err
	}
	return orderFromRow(row), true, nil
}

func (t *pgxTx) MarkPaid(ctx context.Context, id, confirmedBy string, paidAt time.Time) (Order, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return Order{}, err
	}
	by, err := parseUUID(confirmedBy)
	if err != nil {
		return Order{}, err
	}
	row, err := t.q.MarkBillingOrderPaid(ctx, db.MarkBillingOrderPaidParams{ID: uid, PaidAt: timestamptz(&paidAt), ConfirmedBy: by})
	if err != nil { // pgx.ErrNoRows here means the locked order was not pending
		return Order{}, err
	}
	return orderFromRow(row), nil
}

func (t *pgxTx) LockSubscription(ctx context.Context, workspaceID string) (subscription.Subscription, bool, error) {
	ws, err := parseUUID(workspaceID)
	if err != nil {
		return subscription.Subscription{}, false, err
	}
	row, err := t.q.GetWorkspaceSubscriptionForUpdate(ctx, ws)
	if errors.Is(err, pgx.ErrNoRows) {
		return subscription.Subscription{}, false, nil
	}
	if err != nil {
		return subscription.Subscription{}, false, err
	}
	s, err := subscriptionFromRow(row)
	return s, err == nil, err
}

func (t *pgxTx) InsertSubscription(ctx context.Context, s subscription.Subscription) (subscription.Subscription, bool, error) {
	ws, err := parseUUID(s.WorkspaceID)
	if err != nil {
		return subscription.Subscription{}, false, err
	}
	overrides, err := json.Marshal(s.LimitOverrides)
	if err != nil {
		return subscription.Subscription{}, false, err
	}
	row, err := t.q.InsertWorkspaceSubscription(ctx, db.InsertWorkspaceSubscriptionParams{
		WorkspaceID: ws, PlanKey: s.PlanKey, Status: s.Status, Seats: int32(s.Seats), Interval: s.Interval,
		CurrentPeriodStart: timestamptz(s.CurrentPeriodStart), CurrentPeriodEnd: timestamptz(s.CurrentPeriodEnd),
		TrialEnd: timestamptz(s.TrialEnd), GraceUntil: timestamptz(s.GraceUntil),
		CancelAtPeriodEnd: s.CancelAtPeriodEnd, LimitOverrides: overrides,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return subscription.Subscription{}, false, nil
	}
	if err != nil {
		return subscription.Subscription{}, false, err
	}
	out, err := subscriptionFromRow(row)
	return out, err == nil, err
}

func (t *pgxTx) UpdateSubscription(ctx context.Context, s subscription.Subscription) (subscription.Subscription, error) {
	ws, err := parseUUID(s.WorkspaceID)
	if err != nil {
		return subscription.Subscription{}, err
	}
	var pending []byte // NULL when there is no pending change
	if s.PendingChange != nil {
		if pending, err = json.Marshal(s.PendingChange); err != nil {
			return subscription.Subscription{}, err
		}
	}
	var unitPrice pgtype.Int8
	if s.UnitPrice != nil {
		unitPrice = pgtype.Int8{Int64: *s.UnitPrice, Valid: true}
	}
	row, err := t.q.ApplyPaymentToWorkspaceSubscription(ctx, db.ApplyPaymentToWorkspaceSubscriptionParams{
		WorkspaceID: ws, PlanKey: s.PlanKey, Seats: int32(s.Seats), Interval: s.Interval,
		CurrentPeriodStart: timestamptz(s.CurrentPeriodStart), CurrentPeriodEnd: timestamptz(s.CurrentPeriodEnd),
		Status: s.Status, GraceUntil: timestamptz(s.GraceUntil),
		PendingChange: pending, UnitPrice: unitPrice,
	})
	if err != nil {
		return subscription.Subscription{}, err
	}
	return subscriptionFromRow(row)
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

func (t *pgxTx) PendingOrder(ctx context.Context, workspaceID string) (Order, bool, error) {
	ws, err := parseUUID(workspaceID)
	if err != nil {
		return Order{}, false, err
	}
	row, err := t.q.GetPendingBillingOrderForWorkspace(ctx, ws)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, false, nil
	}
	if err != nil {
		return Order{}, false, err
	}
	return orderFromRow(row), true, nil
}

func (t *pgxTx) ReadOnlyForBilling(ctx context.Context, workspaceID string) (bool, error) {
	ws, err := parseUUID(workspaceID)
	if err != nil {
		return false, err
	}
	row, err := t.q.GetPlatformWorkspaceForUpdate(ctx, ws)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return row.Status == "read_only" && row.Reason == ReasonBilling, nil
}

func (t *pgxTx) UnpaidUsageOrder(ctx context.Context, workspaceID string) (Order, bool, error) {
	ws, err := parseUUID(workspaceID)
	if err != nil {
		return Order{}, false, err
	}
	row, err := t.q.GetUnpaidUsageOrderForWorkspace(ctx, ws)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, false, nil
	}
	if err != nil {
		return Order{}, false, err
	}
	return orderFromRow(row), true, nil
}

func (t *pgxTx) CancelOrder(ctx context.Context, id string) (Order, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return Order{}, err
	}
	row, err := t.q.CancelBillingOrder(ctx, uid)
	if err != nil { // pgx.ErrNoRows here means the locked order was not pending
		return Order{}, err
	}
	return orderFromRow(row), nil
}

func (t *pgxTx) UsageLines(ctx context.Context, orderID string) (int64, error) {
	uid, err := parseUUID(orderID)
	if err != nil {
		return 0, err
	}
	return t.q.CountUsageChargesOfOrder(ctx, uid)
}
