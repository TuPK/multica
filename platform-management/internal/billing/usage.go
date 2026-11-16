package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
)

// UsageActivation is a workspace's move onto a pay as you go plan (spec 7b.3,
// rule 6). The caller has checked the billing profile and the plan.
type UsageActivation struct {
	WorkspaceID    string
	PlanKey        string
	CreatedBy      string
	IdempotencyKey string
	// Buyer is a JSON object snapshot of the billing profile. Empty means {}.
	Buyer json.RawMessage
}

// ActivateUsage makes the workspace's subscription active on a usage plan for
// the current UTC calendar month, and writes a subscribe order for 0 VND that is
// already paid, so the activation has a row in the order history. It runs in
// one transaction with its audit rows. A repeated idempotency key returns the
// first order with created == false and writes nothing.
func (s *Service) ActivateUsage(ctx context.Context, a UsageActivation) (o Order, created bool, err error) {
	ws, err := canonicalUUID(a.WorkspaceID)
	if err != nil {
		return Order{}, false, fmt.Errorf("billing: workspace id: %w", err)
	}
	by, err := canonicalUUID(a.CreatedBy)
	if err != nil {
		return Order{}, false, fmt.Errorf("billing: created by: %w", err)
	}
	if a.PlanKey == "" || a.IdempotencyKey == "" {
		return Order{}, false, errors.New("billing: plan key and idempotency key are required")
	}
	buyer := a.Buyer
	if len(buyer) == 0 {
		buyer = json.RawMessage(`{}`)
	}

	now := s.d.Now().UTC()
	start, end := subscription.UsagePeriod(now)
	code, err := s.d.NewCode()
	if err != nil {
		return Order{}, false, err
	}
	n := NewOrder{
		Draft: Draft{
			WorkspaceID: ws, Kind: KindSubscribe, PlanKey: a.PlanKey, Interval: IntervalMonth,
			Seats: 0, Amount: 0, VATAmount: 0, Currency: "vnd", Buyer: buyer,
			PeriodStart: &start, PeriodEnd: &end, IdempotencyKey: a.IdempotencyKey, CreatedBy: by,
		},
		Provider: s.d.Provider, TransferCode: code, CreatedAt: now, ExpiresAt: now.Add(OrderLifetime),
	}

	err = s.d.Runner.InTx(ctx, func(tx Tx) error {
		created = false
		var inserted bool
		o, inserted, err = tx.InsertOrder(ctx, n)
		if err != nil {
			return err
		}
		if !inserted {
			var found bool
			if o, found, err = tx.OrderByIdempotencyKey(ctx, ws, a.IdempotencyKey); err != nil {
				return err
			}
			if !found {
				return errors.New("order for the idempotency key vanished")
			}
			return nil
		}
		created = true
		if err := audit.Record(ctx, tx, orderCreatedEntry(o)); err != nil {
			return err
		}

		cur, exists, err := tx.LockSubscription(ctx, ws)
		if err != nil {
			return err
		}
		// Staff's limit overrides stay with the workspace across a plan change.
		overrides := plan.Limits{}
		if exists && cur.LimitOverrides != nil {
			overrides = cur.LimitOverrides
		}
		next := subscription.Subscription{
			WorkspaceID: ws, PlanKey: a.PlanKey, Status: subscription.StatusActive, Seats: 0,
			Interval: subscription.IntervalMonth, CurrentPeriodStart: &start, CurrentPeriodEnd: &end,
			LimitOverrides: overrides,
		}
		var written subscription.Subscription
		if exists {
			written, err = tx.UpdateSubscription(ctx, next)
		} else {
			var ok bool
			written, ok, err = tx.InsertSubscription(ctx, next)
			if err == nil && !ok {
				return errors.New("the subscription was written by another transaction; retry")
			}
		}
		if err != nil {
			return err
		}

		o, err = tx.MarkPaid(ctx, o.ID, by, now)
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			WorkspaceID: ws, ActorType: audit.ActorMember, ActorID: by,
			Action: audit.ActionSubscriptionChanged, TargetType: "workspace", TargetID: ws,
			Detail: map[string]any{
				"plan": a.PlanKey, "billing_model": "usage", "version": written.Version,
				"created": !exists, "order_id": o.ID,
			},
		})
	})
	if err != nil {
		return Order{}, false, fmt.Errorf("billing: activate usage plan: %w", err)
	}
	if created {
		s.d.Entitlements.Invalidate(ws)
	}
	return o, created, nil
}
