package billing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/multica-ai/multica/platform-management/internal/db"
	"github.com/multica-ai/multica/platform-management/internal/paymentnotice"
)

// PgxReconcileStore is the ReconcileStore over a Postgres pool.
type PgxReconcileStore struct {
	notices *paymentnotice.PgxStore
	q       *db.Queries
}

var _ ReconcileStore = (*PgxReconcileStore)(nil)

func NewPgxReconcileStore(pool *pgxpool.Pool) *PgxReconcileStore {
	return &PgxReconcileStore{notices: paymentnotice.NewPgxStore(pool), q: db.New(pool)}
}

func (s *PgxReconcileStore) Notice(ctx context.Context, id string) (paymentnotice.Notice, error) {
	return s.notices.Get(ctx, id)
}

func (s *PgxReconcileStore) OrderByTransferCode(ctx context.Context, code string) (Order, bool, error) {
	row, err := s.q.GetBillingOrderByTransferCode(ctx, code)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, false, nil
	}
	if err != nil {
		return Order{}, false, err
	}
	return orderFromRow(row), true, nil
}

func (s *PgxReconcileStore) ReceivedBefore(ctx context.Context, cutoff time.Time, limit int) ([]string, error) {
	ids, err := s.q.ListReceivedPaymentNoticeIDsBefore(ctx, db.ListReceivedPaymentNoticeIDsBeforeParams{
		ReceivedAt: timestamptz(&cutoff), Limit: int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list received payment notices: %w", err)
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = uuidText(id)
	}
	return out, nil
}

// DecideNotice implements NoticeTx. The order id is empty for a decision that
// has no order.
func (t *pgxTx) DecideNotice(ctx context.Context, d NoticeDecision, orderID string) (bool, error) {
	id, err := parseUUID(d.Notice.ID)
	if err != nil {
		return false, err
	}
	decidedBy := d.ActorID
	if decidedBy == "" {
		decidedBy = d.ActorLabel
	}
	n, err := t.q.DecidePaymentNotice(ctx, db.DecidePaymentNoticeParams{
		ToState: string(d.To), Reason: d.Reason, OrderID: orderID, DecidedBy: decidedBy,
		ID: id, FromState: string(d.Notice.State),
	})
	return n > 0, err
}
