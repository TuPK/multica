package paymentnotice

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/multica-ai/multica/platform-management/internal/db"
)

type PgxStore struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

var _ Store = (*PgxStore)(nil)

func NewPgxStore(pool *pgxpool.Pool) *PgxStore {
	return &PgxStore{pool: pool, q: db.New(pool)}
}

func (s *PgxStore) Insert(ctx context.Context, n NewNotice) (Notice, bool, error) {
	arg := db.InsertPaymentNoticeParams{
		Provider:        n.Provider,
		ProviderEventID: n.ProviderEventID,
		TransferCode:    n.TransferCode,
		Amount:          n.Amount,
		Currency:        n.Currency,
		BodySha256:      n.BodySHA256,
	}
	if !n.ReceivedAt.IsZero() {
		arg.ReceivedAt = pgtype.Timestamptz{Time: n.ReceivedAt, Valid: true}
	}
	row, err := s.q.InsertPaymentNotice(ctx, arg)
	if err == nil {
		return fromRow(row), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Notice{}, false, fmt.Errorf("insert payment notice: %w", err)
	}
	existing, err := s.q.GetPaymentNoticeByEvent(ctx, db.GetPaymentNoticeByEventParams{
		Provider:        n.Provider,
		ProviderEventID: n.ProviderEventID,
	})
	if err != nil {
		return Notice{}, false, fmt.Errorf("read existing payment notice: %w", err)
	}
	return fromRow(existing), false, nil
}

func (s *PgxStore) Get(ctx context.Context, id string) (Notice, error) {
	uid, err := parseID(id)
	if err != nil {
		return Notice{}, err
	}
	row, err := s.q.GetPaymentNotice(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return Notice{}, ErrNotFound
	}
	if err != nil {
		return Notice{}, fmt.Errorf("get payment notice: %w", err)
	}
	return fromRow(row), nil
}

func (s *PgxStore) Transition(ctx context.Context, c Change) error {
	if !c.From.CanMoveTo(c.To) {
		return ErrInvalidTransition
	}
	uid, err := parseID(c.ID)
	if err != nil {
		return err
	}
	n, err := s.q.TransitionPaymentNotice(ctx, db.TransitionPaymentNoticeParams{
		ToState:        string(c.To),
		ReviewReason:   c.ReviewReason,
		DecidedBy:      c.DecidedBy,
		DecisionReason: c.DecisionReason,
		ID:             uid,
		FromState:      string(c.From),
	})
	if err != nil {
		return fmt.Errorf("transition payment notice: %w", err)
	}
	if n > 0 {
		return nil
	}
	_, err = s.q.GetPaymentNotice(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("transition payment notice: %w", err)
	}
	return ErrStateConflict
}

func (s *PgxStore) ListNeedsReview(ctx context.Context, limit int) ([]Notice, error) {
	rows, err := s.q.ListPaymentNoticesNeedingReview(ctx, int32(limit))
	if err != nil {
		return nil, fmt.Errorf("list payment notices: %w", err)
	}
	out := make([]Notice, 0, len(rows))
	for _, r := range rows {
		out = append(out, fromRow(r))
	}
	return out, nil
}

// parseID treats a malformed id as not found: no row can carry it.
func parseID(id string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(id); err != nil {
		return u, ErrNotFound
	}
	return u, nil
}

func fromRow(r db.PaymentNotice) Notice {
	n := Notice{
		ID:              uuidString(r.ID),
		Provider:        r.Provider,
		ProviderEventID: r.ProviderEventID,
		State:           State(r.State),
		ReviewReason:    r.ReviewReason,
		TransferCode:    r.TransferCode,
		Amount:          r.Amount,
		Currency:        r.Currency,
		OrderID:         r.OrderID,
		BodySHA256:      r.BodySha256,
		ReceivedAt:      r.ReceivedAt.Time,
		DecidedBy:       r.DecidedBy,
		DecisionReason:  r.DecisionReason,
	}
	if r.DecidedAt.Valid {
		n.DecidedAt = r.DecidedAt.Time
	}
	return n
}

func uuidString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
