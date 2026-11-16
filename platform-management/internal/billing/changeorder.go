package billing

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
)

// ChangeOrder asks for a plan change that is charged now (spec 7b.1, rule 5).
// AcceptedAmount is the amount before VAT the client was quoted. ExpectedVersion
// 0 skips the version check. VATRate prices the VAT of the order.
type ChangeOrder struct {
	WorkspaceID     string
	Request         subscription.Request
	AcceptedAmount  int64
	ExpectedVersion int64
	VATRate         Rate
	IdempotencyKey  string
	CreatedBy       string
}

// OrderForKey returns the workspace's order for an idempotency key, found ==
// false when there is none. A repeated charged change is answered from it before
// the change is quoted again, since the quote no longer applies once it is paid.
func (s *Service) OrderForKey(ctx context.Context, workspaceID, key string) (Order, bool, error) {
	ws, err := canonicalUUID(workspaceID)
	if err != nil {
		return Order{}, false, fmt.Errorf("billing: workspace id: %w", err)
	}
	if key == "" {
		return Order{}, false, nil
	}
	var o Order
	var found bool
	err = s.d.Runner.InTx(ctx, func(tx Tx) error {
		var err error
		o, found, err = tx.OrderByIdempotencyKey(ctx, ws, key)
		return err
	})
	return o, found, err
}

// CreateChangeOrder writes the order of a charged plan change in one
// transaction, after checking it against the subscription as it is then. A
// repeated idempotency key returns the first order and writes nothing. Otherwise
// the workspace must have no pending order (rule 9), the subscription row is
// locked, the change is quoted again from that row, and the quote must still
// equal AcceptedAmount and be a charge, not a scheduled change. A change that
// is not charged is never an order; the caller schedules it instead.
func (s *Service) CreateChangeOrder(ctx context.Context, c ChangeOrder) (out Order, created bool, err error) {
	ws, err := canonicalUUID(c.WorkspaceID)
	if err != nil {
		return Order{}, false, fmt.Errorf("billing: workspace id: %w", err)
	}
	for attempt := 1; ; attempt++ {
		created = false
		code, err := s.d.NewCode()
		if err != nil {
			return Order{}, false, err
		}
		now := s.d.Now().UTC()
		err = s.d.Runner.InTx(ctx, func(tx Tx) error {
			created = false
			if c.IdempotencyKey != "" {
				existing, found, err := tx.OrderByIdempotencyKey(ctx, ws, c.IdempotencyKey)
				if err != nil {
					return err
				}
				if found {
					out = existing
					return nil
				}
			}
			pending, found, err := tx.PendingOrder(ctx, ws)
			if err != nil {
				return err
			}
			if found {
				return subscription.OrderPending(pending.ID)
			}
			cur, exists, err := tx.LockSubscription(ctx, ws)
			if err != nil {
				return err
			}
			if exists && c.ExpectedVersion != 0 && cur.Version != c.ExpectedVersion {
				return subscription.QuoteChanged()
			}
			target, found, err := tx.Plan(ctx, c.Request.PlanKey)
			if err != nil {
				return err
			}
			if !found {
				return &subscription.Error{Status: http.StatusBadRequest, Code: "unknown_plan", Message: "no plan has this key"}
			}
			var current plan.Plan
			if exists {
				if current, _, err = tx.Plan(ctx, cur.PlanKey); err != nil {
					return err
				}
			}
			q, refusal := subscription.QuotePlanChange(subscription.QuoteInput{
				Current: cur, HasRow: exists, CurrentPlan: current, Target: target, Request: c.Request, Now: now,
			})
			if refusal != nil {
				return refusal
			}
			if q.Mode == subscription.ModeSchedule || q.Amount != c.AcceptedAmount {
				return subscription.QuoteChanged()
			}
			d := Draft{
				WorkspaceID: ws, Kind: KindSubscribe, PlanKey: target.Key, Interval: c.Request.Interval,
				Seats: q.Seats, Amount: q.Amount, Currency: target.Currency,
				IdempotencyKey: c.IdempotencyKey, CreatedBy: c.CreatedBy,
			}
			if q.Mode == subscription.ModeCharge {
				d.Kind = KindChangePlan
				d.PeriodStart, d.PeriodEnd = cur.CurrentPeriodStart, cur.CurrentPeriodEnd
			} else {
				end, err := PeriodEnd(now, c.Request.Interval)
				if err != nil {
					return err
				}
				start := now
				d.PeriodStart, d.PeriodEnd = &start, &end
			}
			if d.VATAmount, _, err = WithVAT(q.Amount, c.VATRate); err != nil {
				return err
			}
			if d, err = d.validate(); err != nil {
				return err
			}
			o, inserted, err := tx.InsertOrder(ctx, NewOrder{
				Draft: d, Provider: s.d.Provider, TransferCode: code, CreatedAt: now, ExpiresAt: now.Add(OrderLifetime),
			})
			if err != nil {
				return err
			}
			if !inserted {
				// A concurrent request with the same key won the insert.
				existing, found, err := tx.OrderByIdempotencyKey(ctx, ws, c.IdempotencyKey)
				if err != nil {
					return err
				}
				if !found {
					return errors.New("order for the idempotency key vanished")
				}
				out = existing
				return nil
			}
			created = true
			out = o
			return audit.Record(ctx, tx, orderCreatedEntry(o))
		})
		if errors.Is(err, ErrTransferCodeTaken) && attempt < createAttempts {
			continue
		}
		if err != nil {
			return Order{}, false, err
		}
		return out, created, nil
	}
}

// RenewalDraft prices the renewal of a subscription from its pending change
// (spec 7b.1, rule 6). p is the plan the renewal terms name, and start begins
// the new period. The VAT is not set here: the caller adds it, as it does for
// every order.
func RenewalDraft(cur subscription.Subscription, p plan.Plan, start time.Time) (Draft, error) {
	planKey, interval, seats := subscription.RenewalTerms(cur)
	if p.Key != planKey {
		return Draft{}, fmt.Errorf("billing: plan %q is not the renewal's plan %q", p.Key, planKey)
	}
	amount, err := Price(p, interval, seats)
	if err != nil {
		return Draft{}, err
	}
	end, err := PeriodEnd(start, interval)
	if err != nil {
		return Draft{}, err
	}
	return Draft{
		WorkspaceID: cur.WorkspaceID, Kind: KindRenew, PlanKey: planKey, Interval: interval, Seats: seats,
		Amount: amount, Currency: p.Currency, PeriodStart: &start, PeriodEnd: &end,
	}, nil
}
