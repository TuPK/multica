package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
	"github.com/multica-ai/multica/platform-management/internal/workspacestatus"
	"github.com/multica-ai/multica/platform-management/workspacestate"
)

var (
	// ErrOrderNotFound is returned when an order id matches no order.
	ErrOrderNotFound = errors.New("billing: order not found")
	// ErrNoSubscription is returned when add_seats is applied to a workspace with no subscription.
	ErrNoSubscription = errors.New("billing: the workspace has no subscription")
	// ErrPlanMismatch is returned when add_seats names a plan other than the subscription's.
	ErrPlanMismatch = errors.New("billing: the order's plan is not the subscription's plan")
	// ErrTransferCodeTaken is returned by Tx.InsertOrder when the transfer code is already in use.
	ErrTransferCodeTaken = errors.New("billing: transfer code already in use")
	// ErrSubscriptionRaced is returned when another transaction created the
	// workspace's first subscription row while the order was applied.
	ErrSubscriptionRaced = errors.New("billing: the subscription was created concurrently; retry")
)

const (
	auditOrderTarget = "billing_order"
	// paymentReason is the platform_workspace reason of the move back to active.
	paymentReason = "payment received"
	// createAttempts is how many transfer codes CreateOrder tries: the first,
	// and one retry after a collision.
	createAttempts = 2
	maxKeyLen      = 200
)

var currencyPattern = regexp.MustCompile(`^[A-Za-z]{3}$`)

// NewOrder is a pending order ready to insert.
type NewOrder struct {
	Draft
	Provider     string
	TransferCode string
	CreatedAt    time.Time
	ExpiresAt    time.Time
}

// Tx is the reads and writes of one billing transaction, audit rows included.
type Tx interface {
	// InsertOrder writes a pending order. It returns inserted == false, and
	// writes nothing, when the workspace already has an order with the same
	// non-empty idempotency key. It returns ErrTransferCodeTaken for a code
	// collision.
	InsertOrder(ctx context.Context, n NewOrder) (o Order, inserted bool, err error)
	OrderByIdempotencyKey(ctx context.Context, workspaceID, key string) (o Order, found bool, err error)
	// LockOrder reads the order and locks its row.
	LockOrder(ctx context.Context, id string) (o Order, found bool, err error)
	// MarkPaid moves a pending order to paid.
	MarkPaid(ctx context.Context, id, confirmedBy string, paidAt time.Time) (Order, error)
	// LockSubscription reads the workspace's subscription and locks its row.
	LockSubscription(ctx context.Context, workspaceID string) (s subscription.Subscription, found bool, err error)
	// InsertSubscription writes the first row, at version 1. It reports false
	// when another transaction wrote the row first.
	InsertSubscription(ctx context.Context, s subscription.Subscription) (subscription.Subscription, bool, error)
	// UpdateSubscription writes plan, seats, interval, period, status and
	// grace_until, and moves the version up by one.
	UpdateSubscription(ctx context.Context, s subscription.Subscription) (subscription.Subscription, error)
	// ReadOnlyForBilling reports whether the workspace is read_only with the
	// reason ReasonBilling.
	ReadOnlyForBilling(ctx context.Context, workspaceID string) (bool, error)
	// Plan reads a plan. found == false for an unknown key.
	Plan(ctx context.Context, key string) (plan.Plan, bool, error)
	// PendingOrder returns the workspace's pending order, found == false when it
	// has none. A usage invoice is a debt, not a pending change (spec 7b.1,
	// rule 9), so it is not one.
	PendingOrder(ctx context.Context, workspaceID string) (o Order, found bool, err error)
	// UnpaidUsageOrder returns the workspace's oldest pending usage invoice,
	// found == false when it has none.
	UnpaidUsageOrder(ctx context.Context, workspaceID string) (o Order, found bool, err error)
	// CancelOrder moves a pending order to canceled.
	CancelOrder(ctx context.Context, id string) (Order, error)
	// UsageLines counts the usage_charge rows of an invoice.
	UsageLines(ctx context.Context, orderID string) (int64, error)

	workspacestatus.Tx
}

// Runner runs fn in one transaction: it commits when fn returns nil and rolls
// back otherwise.
type Runner interface {
	InTx(ctx context.Context, fn func(Tx) error) error
}

// Deps are what a Service needs.
type Deps struct {
	Runner Runner
	// Provider is the name stored on new orders, ProviderManual in this release.
	Provider string
	// Workspaces drops the cached workspace state after a payment moves it.
	Workspaces workspacestatus.Invalidator
	// Entitlements drops the cached entitlement policy after a payment changes
	// a subscription. *entitlement.Service is one.
	Entitlements subscription.Invalidator
	// Now is the clock. Nil means time.Now.
	Now func() time.Time
	// NewCode makes a transfer code. Nil means NewTransferCode.
	NewCode func() (string, error)
	Log     *slog.Logger
}

// Service creates orders and applies confirmed payments.
type Service struct {
	d Deps
}

// New returns a Service. Runner, Provider, Workspaces and Entitlements are required.
func New(d Deps) (*Service, error) {
	if d.Runner == nil || d.Provider == "" || d.Workspaces == nil || d.Entitlements == nil {
		return nil, errors.New("billing: Runner, Provider, Workspaces and Entitlements are required")
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.NewCode == nil {
		d.NewCode = NewTransferCode
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	return &Service{d: d}, nil
}

// Draft is what a caller decides about an order. Amount is the price before
// VAT. For add_seats, Seats is the number of seats added and the period is the
// subscription's current one.
type Draft struct {
	WorkspaceID string
	Kind        Kind
	PlanKey     string
	Interval    string
	Seats       int
	Amount      int64
	VATAmount   int64
	Currency    string
	// Buyer is a JSON object snapshot of the billing profile. Empty means {}.
	Buyer       json.RawMessage
	PeriodStart *time.Time
	PeriodEnd   *time.Time
	// IdempotencyKey, when not empty, makes the create safe to repeat in a
	// workspace.
	IdempotencyKey string
	// CreatedBy is the user who placed the order; empty for the system.
	CreatedBy string
}

func (d Draft) validate() (Draft, error) {
	var err error
	if d.WorkspaceID, err = canonicalUUID(d.WorkspaceID); err != nil {
		return d, fmt.Errorf("billing: workspace id: %w", err)
	}
	if d.CreatedBy != "" {
		if d.CreatedBy, err = canonicalUUID(d.CreatedBy); err != nil {
			return d, fmt.Errorf("billing: created by: %w", err)
		}
	}
	usage := d.Kind == KindUsage
	switch {
	case !d.Kind.Valid():
		return d, fmt.Errorf("billing: unknown kind %q", d.Kind)
	case d.PlanKey == "":
		return d, errors.New("billing: plan key is required")
	case d.Interval != IntervalMonth && d.Interval != IntervalYear:
		return d, fmt.Errorf("billing: unknown interval %q", d.Interval)
	case usage && d.Seats != 0:
		return d, errors.New("billing: a usage order has no seats")
	case usage && (d.PeriodStart == nil || d.IdempotencyKey == ""):
		return d, errors.New("billing: a usage order needs a period and an idempotency key")
	case !usage && (d.Seats < 1 || d.Seats > MaxSeats):
		return d, fmt.Errorf("billing: seats must be from 1 to %d", MaxSeats)
	case d.Amount <= 0:
		return d, errors.New("billing: amount must be positive")
	case d.VATAmount < 0:
		return d, errors.New("billing: VAT amount must not be negative")
	case d.Amount > 1<<62 || d.VATAmount > 1<<62:
		return d, errors.New("billing: amount is too large")
	case !currencyPattern.MatchString(d.Currency):
		return d, errors.New("billing: currency must be a three-letter code")
	case len(d.IdempotencyKey) > maxKeyLen:
		return d, fmt.Errorf("billing: idempotency key is longer than %d bytes", maxKeyLen)
	}
	if (d.PeriodStart == nil) != (d.PeriodEnd == nil) {
		return d, errors.New("billing: period start and end go together")
	}
	if d.PeriodStart != nil && !d.PeriodEnd.After(*d.PeriodStart) {
		return d, errors.New("billing: the period must end after it starts")
	}
	if len(d.Buyer) == 0 {
		d.Buyer = json.RawMessage(`{}`)
	} else {
		var obj map[string]any
		if json.Unmarshal(d.Buyer, &obj) != nil || obj == nil {
			return d, errors.New("billing: buyer must be a JSON object")
		}
	}
	return d, nil
}

// ValidateDraft checks a draft and returns it with the defaults filled in. The
// usage invoicing, which writes its orders in its own transaction, uses it.
func ValidateDraft(d Draft) (Draft, error) { return d.validate() }

// CreateOrder writes a pending order, its transfer code and expires_at (seven
// days after creation), and the billing.order_created audit row, in one
// transaction. When the workspace already has an order with the same non-empty
// idempotency key, it returns that order with created == false and writes
// nothing.
func (s *Service) CreateOrder(ctx context.Context, d Draft) (o Order, created bool, err error) {
	if d, err = d.validate(); err != nil {
		return Order{}, false, err
	}
	for attempt := 1; ; attempt++ {
		created = false
		code, err := s.d.NewCode()
		if err != nil {
			return Order{}, false, err
		}
		now := s.d.Now().UTC()
		n := NewOrder{Draft: d, Provider: s.d.Provider, TransferCode: code, CreatedAt: now, ExpiresAt: now.Add(OrderLifetime)}
		err = s.d.Runner.InTx(ctx, func(tx Tx) error {
			var inserted bool
			var err error
			o, inserted, err = tx.InsertOrder(ctx, n)
			if err != nil {
				return err
			}
			if !inserted {
				var found bool
				if o, found, err = tx.OrderByIdempotencyKey(ctx, d.WorkspaceID, d.IdempotencyKey); err != nil {
					return err
				}
				if !found {
					return errors.New("order for the idempotency key vanished")
				}
				return nil
			}
			created = true
			return audit.Record(ctx, tx, orderCreatedEntry(o))
		})
		if errors.Is(err, ErrTransferCodeTaken) && attempt < createAttempts {
			continue
		}
		if err != nil {
			return Order{}, false, fmt.Errorf("billing: create order: %w", err)
		}
		return o, created, nil
	}
}

// Confirmation is a confirmed payment: the staff member who matched the
// transfer and the order they confirm.
type Confirmation struct {
	OrderID     string
	ConfirmedBy string
	// ActorLabel is the staff member's email, copied to the audit rows.
	ActorLabel string
}

// Applied is the result of ApplyPayment.
type Applied struct {
	Order Order
	// AlreadyPaid is true when the order was paid before this call; nothing was written.
	AlreadyPaid bool
	// Subscription is the subscription after the payment. It is the zero value
	// when AlreadyPaid.
	Subscription subscription.Subscription
	// WorkspaceReactivated is true when the payment moved a read_only
	// workspace back to active.
	WorkspaceReactivated bool
}

// ApplyPayment applies a confirmed payment in one transaction: it locks the
// order, checks it is pending and not expired, sets it paid, updates the
// workspace's subscription for the order's kind, moves a workspace made
// read_only by an overdue subscription back to active, and writes
// billing.order_paid. A failure anywhere, the audit write included, rolls the
// whole change back. On an order that is already paid it writes nothing and
// returns that order.
func (s *Service) ApplyPayment(ctx context.Context, c Confirmation) (Applied, error) {
	orderID, err := canonicalUUID(c.OrderID)
	if err != nil {
		return Applied{}, fmt.Errorf("billing: order id: %w", err)
	}
	confirmedBy, err := canonicalUUID(c.ConfirmedBy)
	if err != nil {
		return Applied{}, fmt.Errorf("billing: confirmed by: %w", err)
	}

	var out Applied
	err = s.d.Runner.InTx(ctx, func(tx Tx) error {
		o, found, err := tx.LockOrder(ctx, orderID)
		if err != nil {
			return err
		}
		if !found {
			return ErrOrderNotFound
		}
		if o.Status == StatusPaid {
			out = Applied{Order: o, AlreadyPaid: true}
			return nil
		}
		now := s.d.Now().UTC()
		if err := Transition(o.Status, StatusPaid, o.ExpiresAt, now); err != nil {
			return err
		}

		if o.Kind == KindUsage {
			return s.applyUsagePayment(ctx, tx, o, confirmedBy, c.ActorLabel, now, &out)
		}
		cur, exists, err := tx.LockSubscription(ctx, o.WorkspaceID)
		if err != nil {
			return err
		}
		next, err := SubscriptionAfter(o, cur, exists)
		if err != nil {
			return err
		}
		if o.Kind != KindAddSeats {
			// The unit price is the seat price of the plan and interval now paid
			// for (spec 7b.1, rule 5). add_seats buys seats at the price already set.
			p, found, err := tx.Plan(ctx, next.PlanKey)
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("billing: plan %q does not exist", next.PlanKey)
			}
			unit, ok := subscription.SeatUnitPrice(p, next.Interval)
			if !ok {
				return fmt.Errorf("billing: plan %q has no seat price for %s", next.PlanKey, next.Interval)
			}
			next.UnitPrice = &unit
		}
		if o, err = tx.MarkPaid(ctx, orderID, confirmedBy, now); err != nil {
			return err
		}
		if exists {
			if next, err = tx.UpdateSubscription(ctx, next); err != nil {
				return err
			}
		} else {
			var ok bool
			if next, ok, err = tx.InsertSubscription(ctx, next); err != nil {
				return err
			} else if !ok {
				return ErrSubscriptionRaced
			}
		}

		overdue, err := tx.ReadOnlyForBilling(ctx, o.WorkspaceID)
		if err != nil {
			return err
		}
		reactivated := false
		if overdue {
			reactivated, err = workspacestatus.SetInTx(ctx, tx, workspacestatus.Change{
				WorkspaceID: o.WorkspaceID, Status: workspacestate.Active, Reason: paymentReason,
				ChangedBy: confirmedBy, ActorLabel: c.ActorLabel,
			})
			if err != nil {
				return err
			}
		}
		if err := audit.Record(ctx, tx, orderPaidEntry(o, next, confirmedBy, c.ActorLabel)); err != nil {
			return fmt.Errorf("record audit event: %w", err)
		}
		out = Applied{Order: o, Subscription: next, WorkspaceReactivated: reactivated}
		return nil
	})
	if err != nil {
		return Applied{}, fmt.Errorf("billing: apply payment: %w", err)
	}
	if out.AlreadyPaid {
		return out, nil
	}
	s.d.Entitlements.Invalidate(out.Order.WorkspaceID)
	if out.WorkspaceReactivated {
		if err := s.d.Workspaces.Invalidate(ctx, out.Order.WorkspaceID); err != nil {
			s.d.Log.Warn("billing: workspace state cache key not deleted; the cached state stays until it expires",
				"workspace_id", out.Order.WorkspaceID, "ttl", workspacestate.TTL, "error", err)
		}
	}
	return out, nil
}

// SubscriptionAfter returns the subscription a paid order leaves behind. cur
// is the current row and exists says whether there is one.
//
// subscribe, renew and change_plan take the order's plan, interval, seats and
// period, make the subscription active and end any grace period. add_seats adds
// the order's seats to the current count and changes nothing else; it needs a
// subscription on the order's plan. Fields the order does not own, such as the
// limit overrides and cancel_at_period_end, are kept.
func SubscriptionAfter(o Order, cur subscription.Subscription, exists bool) (subscription.Subscription, error) {
	if o.Seats < 1 {
		return subscription.Subscription{}, errors.New("billing: the order has no seats")
	}
	switch o.Kind {
	case KindAddSeats:
		if !exists {
			return subscription.Subscription{}, ErrNoSubscription
		}
		if cur.PlanKey != o.PlanKey {
			return subscription.Subscription{}, ErrPlanMismatch
		}
		cur.Seats += o.Seats
		return cur, nil
	case KindSubscribe, KindRenew, KindChangePlan:
		if o.PeriodStart == nil || o.PeriodEnd == nil || !o.PeriodEnd.After(*o.PeriodStart) {
			return subscription.Subscription{}, errors.New("billing: the order has no valid period")
		}
		// A renewal is priced from the pending change (rule 6), so a paid renewal
		// must be the pending change; applying it clears the pending change.
		if o.Kind == KindRenew && exists && cur.PendingChange != nil {
			pc := cur.PendingChange
			if pc.PlanKey != o.PlanKey || pc.Seats != o.Seats || pc.Interval != o.Interval {
				return subscription.Subscription{}, errors.New("billing: the renewal order does not match the pending change")
			}
		}
		if !exists {
			cur = subscription.Subscription{WorkspaceID: o.WorkspaceID, LimitOverrides: plan.Limits{}}
		}
		start, end := *o.PeriodStart, *o.PeriodEnd
		cur.PlanKey, cur.Interval, cur.Seats = o.PlanKey, o.Interval, o.Seats
		cur.CurrentPeriodStart, cur.CurrentPeriodEnd = &start, &end
		cur.Status = subscription.StatusActive
		cur.GraceUntil = nil
		cur.PendingChange = nil
		return cur, nil
	}
	return subscription.Subscription{}, fmt.Errorf("billing: unknown kind %q", o.Kind)
}

func orderCreatedEntry(o Order) audit.Entry {
	e := audit.Entry{
		WorkspaceID: o.WorkspaceID, Action: audit.ActionBillingOrderCreated,
		TargetType: auditOrderTarget, TargetID: o.ID,
		Detail: orderDetail(o),
	}
	if o.CreatedBy != "" {
		e.ActorType, e.ActorID = audit.ActorMember, o.CreatedBy
	} else {
		e.ActorType = audit.ActorSystem
	}
	return e
}

func orderPaidEntry(o Order, sub subscription.Subscription, staffID, label string) audit.Entry {
	d := orderDetail(o)
	d["subscription_version"] = sub.Version
	return audit.Entry{
		WorkspaceID: o.WorkspaceID, ActorType: audit.ActorStaff, ActorID: staffID, ActorLabel: label,
		Action: audit.ActionBillingOrderPaid, TargetType: auditOrderTarget, TargetID: o.ID, Detail: d,
	}
}

// orderDetail avoids key names the audit redaction filter treats as secrets,
// such as plan_key.
func orderDetail(o Order) map[string]any {
	return map[string]any{
		"kind": string(o.Kind), "plan": o.PlanKey, "interval": o.Interval, "seats": o.Seats,
		"amount": o.Amount, "vat_amount": o.VATAmount, "currency": o.Currency,
		"provider": o.Provider, "transfer_code": o.TransferCode,
	}
}

func canonicalUUID(s string) (string, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil || !u.Valid {
		return "", fmt.Errorf("invalid uuid %q", s)
	}
	return u.String(), nil
}
