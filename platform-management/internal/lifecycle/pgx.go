package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/db"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
	"github.com/multica-ai/multica/platform-management/internal/workspacestatus"
	"github.com/multica-ai/multica/platform-management/workspacestate"
)

// Pool is the part of *pgxpool.Pool the store uses.
type Pool interface {
	db.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// PgxStore is the Store over Postgres.
type PgxStore struct {
	pool Pool
	q    *db.Queries
}

// NewPgxStore returns a Store over pool.
func NewPgxStore(pool Pool) *PgxStore { return &PgxStore{pool: pool, q: db.New(pool)} }

func parseUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil || !u.Valid {
		return pgtype.UUID{}, fmt.Errorf("invalid uuid %q", s)
	}
	return u, nil
}

func timestamptz(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func timestamptzPtr(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return timestamptz(*t)
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func fromRow(r db.WorkspaceSubscription) (subscription.Subscription, error) {
	overrides := plan.Limits{}
	if err := json.Unmarshal(r.LimitOverrides, &overrides); err != nil {
		return subscription.Subscription{}, fmt.Errorf("subscription %s: limit_overrides: %w", r.WorkspaceID.String(), err)
	}
	if overrides == nil {
		overrides = plan.Limits{}
	}
	return subscription.Subscription{
		WorkspaceID: r.WorkspaceID.String(), PlanKey: r.PlanKey, Status: r.Status, Seats: int(r.Seats),
		Interval: r.Interval, CurrentPeriodStart: timePtr(r.CurrentPeriodStart), CurrentPeriodEnd: timePtr(r.CurrentPeriodEnd),
		TrialEnd: timePtr(r.TrialEnd), GraceUntil: timePtr(r.GraceUntil), CancelAtPeriodEnd: r.CancelAtPeriodEnd,
		LimitOverrides: overrides, Version: r.Version, UpdatedAt: r.UpdatedAt.Time.UTC(),
	}, nil
}

func fromRows(rows []db.WorkspaceSubscription, err error) ([]subscription.Subscription, error) {
	if err != nil {
		return nil, err
	}
	out := make([]subscription.Subscription, 0, len(rows))
	for _, r := range rows {
		s, err := fromRow(r)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// Due implements Store. Each edge has its own query, written to the rule of
// Edge.Due.
func (s *PgxStore) Due(ctx context.Context, e Edge, now time.Time, limit int) ([]subscription.Subscription, error) {
	at, n := timestamptz(now), int32(limit)
	switch e {
	case EdgeTrialEnded:
		return fromRows(s.q.ListSubscriptionsTrialEnded(ctx, db.ListSubscriptionsTrialEndedParams{TrialEnd: at, Limit: n}))
	case EdgeCanceledLapsed:
		return fromRows(s.q.ListSubscriptionsCanceledLapsed(ctx, db.ListSubscriptionsCanceledLapsedParams{CurrentPeriodEnd: at, Limit: n}))
	case EdgePeriodLapsed:
		return fromRows(s.q.ListSubscriptionsPeriodLapsed(ctx, db.ListSubscriptionsPeriodLapsedParams{CurrentPeriodEnd: at, Limit: n}))
	case EdgeGraceEnded:
		return fromRows(s.q.ListSubscriptionsGraceEnded(ctx, db.ListSubscriptionsGraceEndedParams{GraceUntil: at, Limit: n}))
	case EdgeReadOnlyEnded:
		return fromRows(s.q.ListSubscriptionsReadOnlyEnded(ctx, db.ListSubscriptionsReadOnlyEndedParams{
			GraceUntil: at, Limit: n, Column3: ReadOnlyDays,
		}))
	}
	return nil, fmt.Errorf("lifecycle: unknown edge %q", e)
}

// UsageUnpaid implements Store.
func (s *PgxStore) UsageUnpaid(ctx context.Context, now time.Time, dueDays, limit int) ([]subscription.Subscription, error) {
	rows, err := s.q.ListSubscriptionsUsageUnpaid(ctx, db.ListSubscriptionsUsageUnpaidParams{
		DueDays: int32(dueDays), GraceDays: GraceDays, Now: timestamptz(now), RowLimit: int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]subscription.Subscription, 0, len(rows))
	for _, r := range rows {
		sub, err := fromRow(r.WorkspaceSubscription)
		if err != nil {
			return nil, err
		}
		sub.GraceUntil = timePtr(r.UsageGraceUntil)
		out = append(out, sub)
	}
	return out, nil
}

// Reminders implements Store.
func (s *PgxStore) Reminders(ctx context.Context, now, until time.Time, limit int) ([]subscription.Subscription, error) {
	return fromRows(s.q.ListSubscriptionsForReminder(ctx, db.ListSubscriptionsForReminderParams{
		CurrentPeriodEnd: timestamptz(now), CurrentPeriodEnd_2: timestamptz(until), Limit: int32(limit),
	}))
}

// InTx implements Store.
func (s *PgxStore) InTx(ctx context.Context, fn func(Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // a no-op once committed
	if err := fn(&pgxTx{Tx: workspacestatus.NewTx(tx), q: db.New(tx)}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Contact implements Store: the workspace's name and slug, and the addresses
// of its owners and admins. A workspace that is gone has no contact. An
// address that is not a plain email address is left out.
func (s *PgxStore) Contact(ctx context.Context, workspaceID string) (Contact, error) {
	id, err := parseUUID(workspaceID)
	if err != nil {
		return Contact{}, err
	}
	w, err := s.q.GetWorkspaceNameAndSlug(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Contact{}, nil
	}
	if err != nil {
		return Contact{}, err
	}
	emails, err := s.q.ListWorkspaceNoticeEmails(ctx, id)
	if err != nil {
		return Contact{}, err
	}
	c := Contact{Name: w.Name, Slug: w.Slug}
	seen := map[string]bool{}
	for _, e := range emails {
		if a, err := mail.ParseAddress(e); err != nil || a.Address != e || a.Name != "" || seen[e] {
			continue
		}
		seen[e] = true
		c.Emails = append(c.Emails, e)
	}
	return c, nil
}

// InsertNotice implements Store.
func (s *PgxStore) InsertNotice(ctx context.Context, n Notice) (bool, error) {
	id, err := parseUUID(n.WorkspaceID)
	if err != nil {
		return false, err
	}
	rows, err := s.q.InsertBillingNotice(ctx, db.InsertBillingNoticeParams{WorkspaceID: id, NoticeKey: n.Key, Kind: n.Kind})
	return rows > 0, err
}

// DeleteNotice implements Store.
func (s *PgxStore) DeleteNotice(ctx context.Context, workspaceID, key string) error {
	id, err := parseUUID(workspaceID)
	if err != nil {
		return err
	}
	return s.q.DeleteBillingNotice(ctx, db.DeleteBillingNoticeParams{WorkspaceID: id, NoticeKey: key})
}

// pgxTx is Tx over one transaction. The embedded workspacestatus.Tx is the
// write path of the workspace's state and its audit row.
type pgxTx struct {
	workspacestatus.Tx
	q *db.Queries
}

// Move implements Tx.
func (t *pgxTx) Move(ctx context.Context, from, to subscription.Subscription) (subscription.Subscription, bool, error) {
	id, err := parseUUID(from.WorkspaceID)
	if err != nil {
		return subscription.Subscription{}, false, err
	}
	row, err := t.q.MoveWorkspaceSubscription(ctx, db.MoveWorkspaceSubscriptionParams{
		WorkspaceID: id, Status: from.Status, Version: from.Version,
		PlanKey: to.PlanKey, Status_2: to.Status, Seats: int32(to.Seats),
		CurrentPeriodStart: timestamptzPtr(to.CurrentPeriodStart), CurrentPeriodEnd: timestamptzPtr(to.CurrentPeriodEnd),
		GraceUntil: timestamptzPtr(to.GraceUntil), CancelAtPeriodEnd: to.CancelAtPeriodEnd,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return subscription.Subscription{}, false, nil
	}
	if err != nil {
		return subscription.Subscription{}, false, err
	}
	after, err := fromRow(row)
	return after, err == nil, err
}

// WorkspaceState implements Tx.
func (t *pgxTx) WorkspaceState(ctx context.Context, workspaceID string) (workspacestate.Status, string, bool, error) {
	id, err := parseUUID(workspaceID)
	if err != nil {
		return "", "", false, err
	}
	row, err := t.q.GetPlatformWorkspaceForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return workspacestate.Status(row.Status), row.Reason, true, nil
}

// DueOrders implements Store.
func (s *PgxStore) DueOrders(ctx context.Context, now time.Time, limit int) ([]billing.Order, error) {
	rows, err := s.q.ListExpiredPendingBillingOrders(ctx, db.ListExpiredPendingBillingOrdersParams{
		ExpiresAt: timestamptz(now), Limit: int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]billing.Order, 0, len(rows))
	for _, r := range rows {
		out = append(out, orderFromRow(r))
	}
	return out, nil
}

// ExpireOrder implements Tx.
func (t *pgxTx) ExpireOrder(ctx context.Context, id string, now time.Time) (billing.Order, bool, error) {
	oid, err := parseUUID(id)
	if err != nil {
		return billing.Order{}, false, err
	}
	row, err := t.q.ExpireBillingOrder(ctx, db.ExpireBillingOrderParams{ID: oid, ExpiresAt: timestamptz(now)})
	if errors.Is(err, pgx.ErrNoRows) {
		return billing.Order{}, false, nil
	}
	if err != nil {
		return billing.Order{}, false, err
	}
	return orderFromRow(row), true, nil
}

// orderFromRow reads the fields the expiry step and its audit row use.
func orderFromRow(r db.BillingOrder) billing.Order {
	return billing.Order{
		ID: r.ID.String(), WorkspaceID: r.WorkspaceID.String(), Kind: billing.Kind(r.Kind), PlanKey: r.PlanKey,
		Interval: r.Interval, Seats: int(r.Seats), Amount: r.Amount, VATAmount: r.VatAmount, Currency: r.Currency,
		Status: billing.Status(r.Status), TransferCode: r.TransferCode, ExpiresAt: r.ExpiresAt.Time.UTC(),
		CreatedAt: r.CreatedAt.Time.UTC(),
	}
}
