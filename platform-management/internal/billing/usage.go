package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
	"github.com/multica-ai/multica/platform-management/internal/workspacestatus"
	"github.com/multica-ai/multica/platform-management/workspacestate"
)

// writeOffReason is the platform_workspace reason of the move back to active
// when staff write off the last unpaid usage invoice.
const writeOffReason = "usage debt written off"

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

// Subscription statuses of an overdue workspace (spec 5.9) that a cleared usage
// debt ends.
const (
	statusPastDue  = "past_due"
	statusReadOnly = "read_only"
)

// applyUsagePayment marks a usage invoice paid. It leaves the plan, seats and
// period alone and, when no other usage invoice is unpaid, ends the overdue
// state the debt caused (spec 7b.3, rule 13).
func (s *Service) applyUsagePayment(ctx context.Context, tx Tx, o Order, confirmedBy, label string, now time.Time, out *Applied) error {
	cur, exists, err := tx.LockSubscription(ctx, o.WorkspaceID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrNoSubscription
	}
	if o, err = tx.MarkPaid(ctx, o.ID, confirmedBy, now); err != nil {
		return err
	}
	sub, reactivated, err := clearUsageDebt(ctx, tx, cur, confirmedBy, label, paymentReason)
	if err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, orderPaidEntry(o, sub, confirmedBy, label)); err != nil {
		return fmt.Errorf("record audit event: %w", err)
	}
	*out = Applied{Order: o, Subscription: sub, WorkspaceReactivated: reactivated}
	return nil
}

// clearUsageDebt returns a past_due or read_only subscription to active, and a
// workspace made read_only for billing back to active, once the workspace has
// no unpaid usage invoice. A free or active subscription is left as it is. cur
// is the locked subscription.
func clearUsageDebt(ctx context.Context, tx Tx, cur subscription.Subscription, staffID, label, reason string) (subscription.Subscription, bool, error) {
	if _, unpaid, err := tx.UnpaidUsageOrder(ctx, cur.WorkspaceID); err != nil || unpaid {
		return cur, false, err
	}
	var err error
	if cur.Status == statusPastDue || cur.Status == statusReadOnly {
		cur.Status, cur.GraceUntil = subscription.StatusActive, nil
		if cur, err = tx.UpdateSubscription(ctx, cur); err != nil {
			return cur, false, err
		}
	}
	overdue, err := tx.ReadOnlyForBilling(ctx, cur.WorkspaceID)
	if err != nil || !overdue {
		return cur, false, err
	}
	reactivated, err := workspacestatus.SetInTx(ctx, tx, workspacestatus.Change{
		WorkspaceID: cur.WorkspaceID, Status: workspacestate.Active, Reason: reason,
		ChangedBy: staffID, ActorLabel: label,
	})
	return cur, reactivated, err
}

// WriteOff is staff's decision to write off a usage invoice (spec 7b.3, rule 14).
type WriteOff struct {
	OrderID    string
	Reason     string
	StaffID    string
	ActorLabel string
}

// ErrNotUsageOrder is returned when WriteOffUsage is given an order of another kind.
var ErrNotUsageOrder = errors.New("billing: the order is not a usage invoice")

// WriteOffUsage cancels a pending usage invoice and writes
// billing.usage_written_off in place of billing.order_canceled, in one
// transaction. The invoice's usage_charge rows keep their order_id, so no later
// invoice takes them. When no unpaid usage invoice is left, the workspace and
// its subscription return to active.
func (s *Service) WriteOffUsage(ctx context.Context, w WriteOff) (Order, error) {
	orderID, err := canonicalUUID(w.OrderID)
	if err != nil {
		return Order{}, fmt.Errorf("billing: order id: %w", err)
	}
	staffID, err := canonicalUUID(w.StaffID)
	if err != nil {
		return Order{}, fmt.Errorf("billing: staff id: %w", err)
	}
	var out Order
	var reactivated bool
	err = s.d.Runner.InTx(ctx, func(tx Tx) error {
		o, found, err := tx.LockOrder(ctx, orderID)
		if err != nil {
			return err
		}
		if !found {
			return ErrOrderNotFound
		}
		if o.Kind != KindUsage {
			return ErrNotUsageOrder
		}
		if err := Transition(o.Status, StatusCanceled, o.ExpiresAt, s.d.Now().UTC()); err != nil {
			return err
		}
		if out, err = tx.CancelOrder(ctx, orderID); err != nil {
			return err
		}
		lines, err := tx.UsageLines(ctx, orderID)
		if err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{
			WorkspaceID: out.WorkspaceID, ActorType: audit.ActorStaff, ActorID: staffID, ActorLabel: w.ActorLabel,
			Action: audit.ActionBillingUsageWrittenOff, TargetType: auditOrderTarget, TargetID: out.ID,
			Detail: map[string]any{
				"reason": w.Reason, "order_id": out.ID, "amount": out.Amount, "vat_amount": out.VATAmount, "lines": lines,
			},
		}); err != nil {
			return err
		}
		cur, exists, err := tx.LockSubscription(ctx, out.WorkspaceID)
		if err != nil || !exists {
			return err
		}
		_, reactivated, err = clearUsageDebt(ctx, tx, cur, staffID, w.ActorLabel, writeOffReason)
		return err
	})
	if err != nil {
		return Order{}, fmt.Errorf("billing: write off usage invoice: %w", err)
	}
	s.d.Entitlements.Invalidate(out.WorkspaceID)
	if reactivated {
		if err := s.d.Workspaces.Invalidate(ctx, out.WorkspaceID); err != nil {
			s.d.Log.Warn("billing: workspace state cache key not deleted; the cached state stays until it expires",
				"workspace_id", out.WorkspaceID, "ttl", workspacestate.TTL, "error", err)
		}
	}
	return out, nil
}
