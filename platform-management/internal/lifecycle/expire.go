package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
)

const (
	// OrderExpiryJobName is the order expiry job's name in internal/jobs. It
	// is a job of its own, and not a step of the lifecycle job, because that
	// job is registered only when an email provider is configured and an order
	// expires with or without one.
	OrderExpiryJobName = "billing_order_expiry"
	// OrderExpiryInterval is how often the order expiry job runs.
	OrderExpiryInterval = time.Hour

	orderExpiryLabel = "system: " + OrderExpiryJobName
	targetOrder      = "billing_order"
)

// OrderExpirer moves each pending order whose expires_at has passed to
// expired, and writes billing.order_expired in the same transaction (spec
// 5.14). The caller passes the time; nothing here reads the clock.
type OrderExpirer struct {
	store Store
	log   *slog.Logger
}

// NewOrderExpirer returns an OrderExpirer over store.
func NewOrderExpirer(store Store, log *slog.Logger) (*OrderExpirer, error) {
	if store == nil {
		return nil, errors.New("lifecycle: Store is required")
	}
	if log == nil {
		log = slog.Default()
	}
	return &OrderExpirer{store: store, log: log}, nil
}

// Run expires the orders that are due at now and returns how many it expired.
// An order is due at expires_at itself, not after it. It continues past a
// failing order and returns the failures joined. A second run changes nothing:
// the update applies only to a pending order, so an order that is expired, paid
// or canceled is not touched and gets no second audit row.
func (x *OrderExpirer) Run(ctx context.Context, now time.Time) (int, error) {
	due, err := x.store.DueOrders(ctx, now, BatchSize)
	if err != nil {
		return 0, fmt.Errorf("lifecycle: list expired orders: %w", err)
	}
	expired := 0
	var errs []error
	for _, o := range due {
		if err := ctx.Err(); err != nil {
			return expired, errors.Join(append(errs, err)...)
		}
		var done bool
		err := x.store.InTx(ctx, func(tx Tx) error {
			done = false
			after, ok, err := tx.ExpireOrder(ctx, o.ID, now)
			if err != nil || !ok {
				return err
			}
			done = true
			return audit.Record(ctx, tx, audit.Entry{
				WorkspaceID: after.WorkspaceID,
				ActorType:   audit.ActorSystem, ActorLabel: orderExpiryLabel,
				Action:     audit.ActionBillingOrderExpired,
				TargetType: targetOrder, TargetID: after.ID,
				Detail: map[string]any{
					"kind": string(after.Kind), "plan": after.PlanKey, "interval": after.Interval, "seats": after.Seats,
					"amount": after.Amount, "vat_amount": after.VATAmount, "currency": after.Currency,
					"transfer_code": after.TransferCode, "expires_at": after.ExpiresAt.UTC().Format(time.RFC3339),
				},
			})
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("lifecycle: expire order %s: %w", o.ID, err))
			continue
		}
		if done {
			expired++
		}
	}
	return expired, errors.Join(errs...)
}

// Job returns the function internal/jobs runs every OrderExpiryInterval. now
// is the clock, read once per run.
func (x *OrderExpirer) Job(now func() time.Time) func(context.Context) error {
	return func(ctx context.Context) error {
		n, err := x.Run(ctx, now())
		if n > 0 {
			x.log.Info("lifecycle: orders expired", "expired", n)
		}
		return err
	}
}
