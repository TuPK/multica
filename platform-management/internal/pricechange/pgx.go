package pricechange

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/db"
	"github.com/multica-ai/multica/platform-management/internal/lifecycle"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
)

// uniqueViolation is the SQLSTATE of a unique index refusing a row.
const uniqueViolation = "23505"

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

// optionalUUID is NULL for an empty string.
func optionalUUID(s string) (pgtype.UUID, error) {
	if s == "" {
		return pgtype.UUID{}, nil
	}
	return parseUUID(s)
}

func timestamptz(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func uuidString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return u.String()
}

func changeFromRow(r db.PlanPriceChange) (Change, error) {
	c := Change{
		ID: r.ID.String(), PlanKey: r.PlanKey, State: r.State, EffectiveFrom: r.EffectiveFrom.Time.UTC(),
		Reason: r.Reason, NoticeSentAt: timePtr(r.NoticeSentAt), CreatedBy: uuidString(r.CreatedBy),
		CreatedAt: r.CreatedAt.Time.UTC(), AppliedAt: timePtr(r.AppliedAt),
		CanceledBy: uuidString(r.CanceledBy), CanceledAt: timePtr(r.CanceledAt),
	}
	if err := json.Unmarshal(r.Old, &c.Old); err != nil {
		return Change{}, fmt.Errorf("price change %s: old: %w", c.ID, err)
	}
	if err := json.Unmarshal(r.New, &c.New); err != nil {
		return Change{}, fmt.Errorf("price change %s: new: %w", c.ID, err)
	}
	return c, nil
}

func changesFromRows(rows []db.PlanPriceChange, err error) ([]Change, error) {
	if err != nil {
		return nil, err
	}
	out := make([]Change, 0, len(rows))
	for _, r := range rows {
		c, err := changeFromRow(r)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// InTx implements Store. It returns fn's error as it is, so a caller can still
// recognise its own error types.
func (s *PgxStore) InTx(ctx context.Context, fn func(Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // a no-op once committed
	if err := fn(&pgxTx{q: db.New(tx)}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// PaidWorkspaces implements Store.
func (s *PgxStore) PaidWorkspaces(ctx context.Context, planKey string) ([]Workspace, error) {
	rows, err := s.q.ListPaidWorkspacesOnPlan(ctx, planKey)
	if err != nil {
		return nil, err
	}
	out := make([]Workspace, len(rows))
	for i, r := range rows {
		out[i] = Workspace{ID: r.WorkspaceID.String(), PendingDeletion: r.PendingDeletion}
	}
	return out, nil
}

// History implements Store.
func (s *PgxStore) History(ctx context.Context, planKey string) ([]Change, error) {
	return changesFromRows(s.q.ListPlanPriceChanges(ctx, planKey))
}

// Due implements Store.
func (s *PgxStore) Due(ctx context.Context, now time.Time, limit int) ([]Change, error) {
	return changesFromRows(s.q.ListDuePlanPriceChanges(ctx, db.ListDuePlanPriceChangesParams{
		EffectiveFrom: timestamptz(now), Limit: int32(limit),
	}))
}

// NoticeDue implements Store.
func (s *PgxStore) NoticeDue(ctx context.Context, limit int) ([]Change, error) {
	return changesFromRows(s.q.ListPlanPriceChangesToNotify(ctx, int32(limit)))
}

// MarkNoticeSent implements Store.
func (s *PgxStore) MarkNoticeSent(ctx context.Context, id string, at time.Time) error {
	u, err := parseUUID(id)
	if err != nil {
		return err
	}
	return s.q.MarkPlanPriceChangeNoticeSent(ctx, db.MarkPlanPriceChangeNoticeSentParams{ID: u, NoticeSentAt: timestamptz(at)})
}

// Contact implements Store: the workspace's name and slug, and the addresses of
// its owners. A workspace that is gone has no contact. An address that is not a
// plain email address is left out.
func (s *PgxStore) Contact(ctx context.Context, workspaceID string) (lifecycle.Contact, error) {
	id, err := parseUUID(workspaceID)
	if err != nil {
		return lifecycle.Contact{}, err
	}
	w, err := s.q.GetWorkspaceNameAndSlug(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return lifecycle.Contact{}, nil
	}
	if err != nil {
		return lifecycle.Contact{}, err
	}
	emails, err := s.q.ListWorkspaceOwnerEmails(ctx, id)
	if err != nil {
		return lifecycle.Contact{}, err
	}
	c := lifecycle.Contact{Name: w.Name, Slug: w.Slug}
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
func (s *PgxStore) InsertNotice(ctx context.Context, n lifecycle.Notice) (bool, error) {
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

type pgxTx struct{ q *db.Queries }

// LockPlan implements Tx.
func (t *pgxTx) LockPlan(ctx context.Context, key string) (PlanRow, bool, error) {
	row, err := t.q.GetPlanForUpdate(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return PlanRow{}, false, nil
	}
	if err != nil {
		return PlanRow{}, false, err
	}
	seat := func(v pgtype.Int8) json.RawMessage {
		if !v.Valid {
			return json.RawMessage("null")
		}
		return json.RawMessage(fmt.Sprint(v.Int64))
	}
	return PlanRow{
		Key: row.Key, Name: row.Name, BillingModel: row.BillingModel, Version: row.Version,
		Prices: Prices{
			FieldSeatPriceMonth: seat(row.SeatPriceMonth), FieldSeatPriceYear: seat(row.SeatPriceYear),
			FieldUsageRates: row.UsageRates, FieldUsageTerms: row.UsageTerms,
		},
	}, true, nil
}

// Scheduled implements Tx.
func (t *pgxTx) Scheduled(ctx context.Context, key string) (Change, bool, error) {
	row, err := t.q.GetScheduledPlanPriceChange(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return Change{}, false, nil
	}
	if err != nil {
		return Change{}, false, err
	}
	c, err := changeFromRow(row)
	return c, err == nil, err
}

// Insert implements Tx.
func (t *pgxTx) Insert(ctx context.Context, c Change) (Change, error) {
	by, err := optionalUUID(c.CreatedBy)
	if err != nil {
		return Change{}, err
	}
	old, err := json.Marshal(c.Old)
	if err != nil {
		return Change{}, err
	}
	next, err := json.Marshal(c.New)
	if err != nil {
		return Change{}, err
	}
	row, err := t.q.InsertPlanPriceChange(ctx, db.InsertPlanPriceChangeParams{
		PlanKey: c.PlanKey, EffectiveFrom: timestamptz(c.EffectiveFrom), Old: old, New: next, Reason: c.Reason, CreatedBy: by,
	})
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == uniqueViolation {
		return Change{}, ErrPending
	}
	if err != nil {
		return Change{}, err
	}
	return changeFromRow(row)
}

// MarkApplied implements Tx.
func (t *pgxTx) MarkApplied(ctx context.Context, id string, now time.Time) (Change, bool, error) {
	u, err := parseUUID(id)
	if err != nil {
		return Change{}, false, err
	}
	row, err := t.q.MarkPlanPriceChangeApplied(ctx, db.MarkPlanPriceChangeAppliedParams{ID: u, AppliedAt: timestamptz(now)})
	if errors.Is(err, pgx.ErrNoRows) {
		return Change{}, false, nil
	}
	if err != nil {
		return Change{}, false, err
	}
	c, err := changeFromRow(row)
	return c, err == nil, err
}

// SetPrices implements Tx.
func (t *pgxTx) SetPrices(ctx context.Context, key string, p Prices) error {
	arg := db.SetPlanPricesParams{Key: key}
	for f, raw := range p {
		switch f {
		case FieldSeatPriceMonth, FieldSeatPriceYear:
			n, ok := seatPrice(raw)
			if !ok {
				return fmt.Errorf("pricechange: %s is not a whole number", f)
			}
			v := pgtype.Int8{Int64: n, Valid: true}
			if f == FieldSeatPriceMonth {
				arg.SeatPriceMonth = v
			} else {
				arg.SeatPriceYear = v
			}
		case FieldUsageRates:
			arg.UsageRates = raw
		case FieldUsageTerms:
			arg.UsageTerms = raw
		default:
			return fmt.Errorf("pricechange: %s is not a price field", f)
		}
	}
	return t.q.SetPlanPrices(ctx, arg)
}

// Cancel implements Tx.
func (t *pgxTx) Cancel(ctx context.Context, key, by string, now time.Time) (Change, bool, error) {
	u, err := optionalUUID(by)
	if err != nil {
		return Change{}, false, err
	}
	row, err := t.q.CancelScheduledPlanPriceChange(ctx, db.CancelScheduledPlanPriceChangeParams{
		PlanKey: key, CanceledBy: u, CanceledAt: timestamptz(now),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Change{}, false, nil
	}
	if err != nil {
		return Change{}, false, err
	}
	c, err := changeFromRow(row)
	return c, err == nil, err
}

// InsertAuditEvent implements audit.Writer inside the transaction.
func (t *pgxTx) InsertAuditEvent(ctx context.Context, e audit.Entry) error {
	var ws pgtype.UUID
	if e.WorkspaceID != "" {
		if err := ws.Scan(e.WorkspaceID); err != nil {
			return fmt.Errorf("invalid workspace id %q: %w", e.WorkspaceID, err)
		}
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

// ScheduledPriceChange implements subscription.PriceChangeReader.
func (s *PgxStore) ScheduledPriceChange(ctx context.Context, planKey string) (subscription.PriceChange, bool, error) {
	row, err := s.q.GetScheduledPlanPriceChange(ctx, planKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return subscription.PriceChange{}, false, nil
	}
	if err != nil {
		return subscription.PriceChange{}, false, err
	}
	c, err := changeFromRow(row)
	if err != nil {
		return subscription.PriceChange{}, false, err
	}
	month, err := priceUpdate(c.New, FieldSeatPriceMonth)
	if err != nil {
		return subscription.PriceChange{}, false, err
	}
	year, err := priceUpdate(c.New, FieldSeatPriceYear)
	if err != nil {
		return subscription.PriceChange{}, false, err
	}
	return subscription.PriceChange{
		PlanKey: c.PlanKey, EffectiveFrom: c.EffectiveFrom, NoticeSentAt: c.NoticeSentAt,
		SeatPriceMonth: month, SeatPriceYear: year,
	}, true, nil
}

// priceUpdate reads a seat price field of a change: unset when the change omits
// it, nil when it sets none.
func priceUpdate(p Prices, field string) (subscription.PriceUpdate, error) {
	raw, ok := p[field]
	if !ok {
		return subscription.PriceUpdate{}, nil
	}
	var v *int64
	if err := json.Unmarshal(raw, &v); err != nil {
		return subscription.PriceUpdate{}, fmt.Errorf("price change: %s: %w", field, err)
	}
	return subscription.PriceUpdate{Set: true, Value: v}, nil
}
