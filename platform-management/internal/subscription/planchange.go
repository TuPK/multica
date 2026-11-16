package subscription

import (
	"context"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
)

// PendingOrders reports whether a workspace has an order that is still pending.
// The check runs after the subscription row is locked, so an order created
// concurrently is seen.
type PendingOrders interface {
	// PendingOrder returns found == false when the workspace has no pending order.
	PendingOrder(ctx context.Context, workspaceID string) (orderID string, found bool, err error)
}

// Actor is the member who makes a self-service change; it is written to the audit row.
type Actor struct {
	ID    string
	Label string
}

// ScheduleChange is a downgrade, seat reduction or interval change to store for
// the next renewal (spec 7b.1, rule 6). ExpectedVersion 0 skips the version check.
type ScheduleChange struct {
	WorkspaceID     string
	PlanKey         string
	Interval        string
	Seats           int
	ExpectedVersion int64
}

// PlanChanges stores and withdraws scheduled changes, and sets the cancel and
// resume flags of a subscription, each in one transaction with its audit row.
type PlanChanges struct {
	runner Runner
	cache  Invalidator
	orders PendingOrders
	now    func() time.Time
}

// NewPlanChanges returns a PlanChanges. now is the clock; nil means time.Now.
func NewPlanChanges(r Runner, cache Invalidator, orders PendingOrders, now func() time.Time) *PlanChanges {
	if now == nil {
		now = time.Now
	}
	return &PlanChanges{runner: r, cache: cache, orders: orders, now: now}
}

// Schedule stores the change as pending_change. It refuses while the workspace
// has a pending order (rule 9) and a stale version (quote_changed). Asking for
// the change that is already stored writes nothing, so a retry is safe.
func (s *PlanChanges) Schedule(ctx context.Context, c ScheduleChange, actor Actor) (Result, error) {
	var res Result
	err := s.runner.InTx(ctx, func(tx Tx) error {
		res = Result{}
		name, found, err := tx.WorkspaceName(ctx, c.WorkspaceID)
		if err != nil {
			return err
		}
		if !found {
			return NotFound()
		}
		cur, exists, err := tx.Get(ctx, c.WorkspaceID)
		if err != nil {
			return err
		}
		if !exists {
			return conflict("subscription_not_changeable", "the workspace has no paid subscription to change")
		}
		if c.ExpectedVersion != 0 && cur.Version != c.ExpectedVersion {
			return QuoteChanged()
		}
		next := PendingChange{PlanKey: c.PlanKey, Seats: c.Seats, Interval: c.Interval, RequestedBy: actor.ID, RequestedAt: s.now().UTC()}
		if cur.PendingChange != nil && sameTarget(*cur.PendingChange, next) {
			res = Result{Subscription: cur}
			return nil
		}
		orderID, pending, err := s.orders.PendingOrder(ctx, c.WorkspaceID)
		if err != nil {
			return err
		}
		if pending {
			return OrderPending(orderID)
		}
		row := cur
		row.PendingChange = &next
		written, err := tx.Update(ctx, row)
		if err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{
			WorkspaceID: c.WorkspaceID, ActorType: audit.ActorMember, ActorID: actor.ID, ActorLabel: actor.Label,
			Action: audit.ActionSubscriptionChangeScheduled, TargetType: targetWorkspace, TargetID: c.WorkspaceID, TargetLabel: name,
			Detail: map[string]any{
				"plan": c.PlanKey, "seats": c.Seats, "interval": c.Interval,
				"effective": "next_renewal", "version": written.Version,
			},
		}); err != nil {
			return err
		}
		res = Result{Subscription: written, Changed: true}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	if res.Changed {
		s.cache.Invalidate(c.WorkspaceID)
	}
	return res, nil
}

// CancelScheduled removes the pending change and keeps the rest of the row. With
// nothing pending it writes nothing.
func (s *PlanChanges) CancelScheduled(ctx context.Context, workspaceID string, actor Actor) (Result, error) {
	var res Result
	err := s.runner.InTx(ctx, func(tx Tx) error {
		res = Result{}
		name, found, err := tx.WorkspaceName(ctx, workspaceID)
		if err != nil {
			return err
		}
		if !found {
			return NotFound()
		}
		cur, exists, err := tx.Get(ctx, workspaceID)
		if err != nil {
			return err
		}
		if !exists || cur.PendingChange == nil {
			res = Result{Subscription: cur}
			return nil
		}
		prev := *cur.PendingChange
		row := cur
		row.PendingChange = nil
		written, err := tx.Update(ctx, row)
		if err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{
			WorkspaceID: workspaceID, ActorType: audit.ActorMember, ActorID: actor.ID, ActorLabel: actor.Label,
			Action: audit.ActionSubscriptionChangeCanceled, TargetType: targetWorkspace, TargetID: workspaceID, TargetLabel: name,
			Detail: map[string]any{
				"plan": prev.PlanKey, "seats": prev.Seats, "interval": prev.Interval, "version": written.Version,
			},
		}); err != nil {
			return err
		}
		res = Result{Subscription: written, Changed: true}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	if res.Changed {
		s.cache.Invalidate(workspaceID)
	}
	return res, nil
}

func sameTarget(a, b PendingChange) bool {
	return a.PlanKey == b.PlanKey && a.Seats == b.Seats && a.Interval == b.Interval
}

// QuoteChanged is the refusal for a quote that no longer matches the subscription.
func QuoteChanged() *Error {
	return conflict("quote_changed", "the subscription changed since the quote; ask for a new one")
}

// OrderPending refuses a change while the workspace has a pending order. The
// order's id is in the body, so the client can show the order to pay.
func OrderPending(orderID string) *Error {
	e := conflict("order_pending", "the workspace has a pending order; pay it or let it expire first")
	e.Detail = map[string]any{"order_id": orderID}
	return e
}

// RenewalTerms returns the plan, interval and seats the next renewal is priced
// for: the pending change when one is stored (rule 6), else the current terms.
func RenewalTerms(s Subscription) (planKey, interval string, seats int) {
	if s.PendingChange != nil {
		p := s.PendingChange
		return p.PlanKey, p.Interval, p.Seats
	}
	return s.PlanKey, s.Interval, s.Seats
}
