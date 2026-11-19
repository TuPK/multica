package billing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/paymentnotice"
	"github.com/multica-ai/multica/platform-management/internal/staff"
)

var (
	// ErrNoticeConflict is returned when a payment notice is no longer in the
	// state the caller read, so another decision won.
	ErrNoticeConflict = errors.New("billing: the payment notice is not in the expected state")
	// ErrForbidden is returned when the staff role may not decide notices (O-R4-8).
	ErrForbidden = errors.New("billing: the staff role may not decide payment notices")
	// ErrReasonRequired is returned when staff give no reason for a decision.
	ErrReasonRequired = errors.New("billing: a reason is required")
	// ErrOrderNotPending is returned when staff resolve a notice to an order
	// that is paid, canceled or expired.
	ErrOrderNotPending = errors.New("billing: the order is not pending")
	// ErrAutomaticReactivation is returned by an automatic ApplyPayment on a
	// workspace that is read_only for billing: staff confirm that payment.
	ErrAutomaticReactivation = errors.New("billing: the workspace is read_only for billing; staff must confirm the payment")
)

// NoticeClosedError is returned when a notice is not in needs_review, so it
// cannot be resolved or dismissed. The console maps it to 409
// payment_notice_closed with State.
type NoticeClosedError struct{ State paymentnotice.State }

func (e *NoticeClosedError) Error() string {
	return fmt.Sprintf("billing: the payment notice is %s", e.State)
}

// NoticeDecision moves a payment notice out of Notice.State. ApplyPayment sets
// ActorID and ActorLabel from the confirmation.
type NoticeDecision struct {
	// Notice is the notice as read; its State is the state the move requires.
	Notice paymentnotice.Notice
	To     paymentnotice.State
	// Reason is the review reason for needs_review and the staff reason otherwise.
	Reason string
	// ActorID is the staff member; empty means the system.
	ActorID    string
	ActorLabel string
}

// decideNotice moves the notice and writes its audit row in tx. o is the order
// the notice is decided against, nil when it has none. It returns
// ErrNoticeConflict when the notice is no longer in d.Notice.State.
func decideNotice(ctx context.Context, tx Tx, d NoticeDecision, o *Order) error {
	if !d.Notice.State.CanMoveTo(d.To) {
		return paymentnotice.ErrInvalidTransition
	}
	nt, ok := tx.(NoticeTx)
	if !ok {
		return errors.New("billing: the transaction cannot write payment notices")
	}
	orderID := ""
	if o != nil {
		orderID = o.ID
	}
	moved, err := nt.DecideNotice(ctx, d, orderID)
	if err != nil {
		return err
	}
	if !moved {
		return ErrNoticeConflict
	}
	if err := audit.Record(ctx, tx, noticeEntry(d, o)); err != nil {
		return fmt.Errorf("record audit event: %w", err)
	}
	return nil
}

var noticeActions = map[paymentnotice.State]string{
	paymentnotice.StateApplied:     audit.ActionPaymentNoticeApplied,
	paymentnotice.StateNeedsReview: audit.ActionPaymentNoticeNeedsReview,
	paymentnotice.StateResolved:    audit.ActionPaymentNoticeResolved,
	paymentnotice.StateDismissed:   audit.ActionPaymentNoticeDismissed,
}

// noticeEntry never holds a body, signature or secret: the notice has none.
func noticeEntry(d NoticeDecision, o *Order) audit.Entry {
	n := d.Notice
	detail := map[string]any{
		"provider": n.Provider, "event_id": n.ProviderEventID,
		"notice_amount": n.Amount, "currency": n.Currency,
	}
	if d.Reason != "" {
		detail["reason"] = d.Reason
	}
	e := audit.Entry{
		ActorType: audit.ActorSystem, ActorLabel: d.ActorLabel,
		Action: noticeActions[d.To], TargetType: "payment_notice", TargetID: n.ID, Detail: detail,
	}
	if d.ActorID != "" {
		e.ActorType, e.ActorID = audit.ActorStaff, d.ActorID
	}
	if o != nil {
		e.WorkspaceID = o.WorkspaceID
		detail["order_id"], detail["order_total"] = o.ID, o.Total()
	}
	return e
}

// ReconcileStore is the reads of the reconciler, outside any transaction.
type ReconcileStore interface {
	// Notice returns paymentnotice.ErrNotFound for an unknown id.
	Notice(ctx context.Context, id string) (paymentnotice.Notice, error)
	OrderByTransferCode(ctx context.Context, code string) (o Order, found bool, err error)
	// ReceivedBefore returns the ids of notices still received that arrived at
	// or before cutoff, oldest first.
	ReceivedBefore(ctx context.Context, cutoff time.Time, limit int) ([]string, error)
}

// Redrive defaults of the payment_notice_redrive job (spec 7.2).
const (
	RedriveAge   = 2 * time.Minute
	RedriveLimit = 100
)

// Actor is the staff member deciding a notice.
type Actor struct {
	ID    string
	Label string // the staff member's email
	Role  staff.Role
}

// allowed reports whether the actor may decide payment notices: row M16 of the
// matrix of spec R6 section 3.6.
func (a Actor) allowed() bool { return staff.Allowed(a.Role, staff.ActionPaymentsManage) == staff.All }

// Reconciler matches payment notices to orders (spec 7.2). A notice that
// matches exactly pays its order through ApplyPayment; any other leaves every
// order untouched and goes to the staff queue.
type Reconciler struct {
	svc   *Service
	store ReconcileStore
}

func NewReconciler(svc *Service, store ReconcileStore) *Reconciler {
	return &Reconciler{svc: svc, store: store}
}

// Reconcile decides one notice. A notice that is not received is returned
// unchanged. A transient error leaves the notice received, for Redrive.
func (r *Reconciler) Reconcile(ctx context.Context, noticeID string) (paymentnotice.Notice, error) {
	n, err := r.store.Notice(ctx, noticeID)
	if err != nil || n.State != paymentnotice.StateReceived {
		return n, err
	}
	var o Order
	found := false
	if n.TransferCode != "" {
		if o, found, err = r.store.OrderByTransferCode(ctx, n.TransferCode); err != nil {
			return n, fmt.Errorf("billing: reconcile: %w", err)
		}
	}
	if reason := matchNotice(n, o, found, r.svc.d.Now().UTC()); reason != "" {
		return r.review(ctx, n, reason)
	}
	applied, err := r.svc.ApplyPayment(ctx, Confirmation{
		OrderID: o.ID, Provider: n.Provider,
		Notice: &NoticeDecision{Notice: n, To: paymentnotice.StateApplied},
	})
	switch {
	case errors.Is(err, ErrNoticeConflict):
		// Another worker decided the notice first; its result stands.
	case errors.Is(err, ErrOrderExpired), errors.Is(err, ErrInvalidTransition):
		return r.review(ctx, n, ReasonOrderNotPending)
	case errors.Is(err, ErrAutomaticReactivation), errors.Is(err, ErrNoSubscription), errors.Is(err, ErrPlanMismatch):
		return r.review(ctx, n, ReasonApplyFailed)
	case err != nil:
		return n, fmt.Errorf("billing: reconcile: %w", err)
	case applied.AlreadyPaid:
		return r.review(ctx, n, ReasonOrderNotPending)
	}
	return r.store.Notice(ctx, noticeID)
}

// review moves a received notice to needs_review, unless another worker has
// decided it already, in which case it returns the notice as it now is.
func (r *Reconciler) review(ctx context.Context, n paymentnotice.Notice, reason string) (paymentnotice.Notice, error) {
	err := r.svc.d.Runner.InTx(ctx, func(tx Tx) error {
		return decideNotice(ctx, tx, NoticeDecision{
			Notice: n, To: paymentnotice.StateNeedsReview, Reason: reason, ActorLabel: "provider:" + n.Provider,
		}, nil)
	})
	if err != nil && !errors.Is(err, ErrNoticeConflict) {
		return n, fmt.Errorf("billing: reconcile: %w", err)
	}
	return r.store.Notice(ctx, n.ID)
}

// Redrive reconciles the notices still received after olderThan, at most limit,
// and returns how many it decided without error. It is the body of the
// payment_notice_redrive job.
func (r *Reconciler) Redrive(ctx context.Context, olderThan time.Duration, limit int) (int, error) {
	ids, err := r.store.ReceivedBefore(ctx, r.svc.d.Now().UTC().Add(-olderThan), limit)
	if err != nil {
		return 0, fmt.Errorf("billing: redrive: %w", err)
	}
	done := 0
	var errs []error
	for _, id := range ids {
		if _, err := r.Reconcile(ctx, id); err != nil {
			errs = append(errs, err)
			continue
		}
		done++
	}
	return done, errors.Join(errs...)
}

// Resolve applies the notice to an order chosen by staff, an amount mismatch
// included, and marks the notice resolved, in one transaction. The audit row
// records the notice amount and the order total. Resolving a resolved notice
// to the same order again changes nothing.
func (r *Reconciler) Resolve(ctx context.Context, actor Actor, noticeID, orderID, reason string) (paymentnotice.Notice, error) {
	if !actor.allowed() {
		return paymentnotice.Notice{}, ErrForbidden
	}
	if strings.TrimSpace(reason) == "" {
		return paymentnotice.Notice{}, ErrReasonRequired
	}
	orderID, err := canonicalUUID(orderID)
	if err != nil {
		return paymentnotice.Notice{}, fmt.Errorf("billing: order id: %w", err)
	}
	n, err := r.store.Notice(ctx, noticeID)
	if err != nil {
		return n, err
	}
	if n.State == paymentnotice.StateResolved && n.OrderID == orderID {
		return n, nil
	}
	if n.State != paymentnotice.StateNeedsReview {
		return n, &NoticeClosedError{State: n.State}
	}
	applied, err := r.svc.ApplyPayment(ctx, Confirmation{
		OrderID: orderID, ConfirmedBy: actor.ID, ActorLabel: actor.Label,
		Notice: &NoticeDecision{Notice: n, To: paymentnotice.StateResolved, Reason: reason},
	})
	switch {
	case errors.Is(err, ErrNoticeConflict):
		return r.settled(ctx, noticeID, orderID)
	case errors.Is(err, ErrOrderExpired), errors.Is(err, ErrInvalidTransition):
		return n, ErrOrderNotPending
	case err != nil:
		return n, err
	case applied.AlreadyPaid:
		return n, ErrOrderNotPending
	}
	return r.store.Notice(ctx, noticeID)
}

// settled reports the outcome of a resolve that lost a race: a repeat of the
// winning resolve succeeds, anything else finds the notice closed.
func (r *Reconciler) settled(ctx context.Context, noticeID, orderID string) (paymentnotice.Notice, error) {
	n, err := r.store.Notice(ctx, noticeID)
	if err != nil {
		return n, err
	}
	if n.State == paymentnotice.StateResolved && n.OrderID == orderID {
		return n, nil
	}
	return n, &NoticeClosedError{State: n.State}
}

// Dismiss closes a needs_review notice that no order should take, such as money
// refunded outside the system, and writes payment.notice_dismissed.
func (r *Reconciler) Dismiss(ctx context.Context, actor Actor, noticeID, reason string) (paymentnotice.Notice, error) {
	if !actor.allowed() {
		return paymentnotice.Notice{}, ErrForbidden
	}
	if strings.TrimSpace(reason) == "" {
		return paymentnotice.Notice{}, ErrReasonRequired
	}
	staffID, err := canonicalUUID(actor.ID)
	if err != nil {
		return paymentnotice.Notice{}, fmt.Errorf("billing: staff id: %w", err)
	}
	n, err := r.store.Notice(ctx, noticeID)
	if err != nil {
		return n, err
	}
	if n.State != paymentnotice.StateNeedsReview {
		return n, &NoticeClosedError{State: n.State}
	}
	err = r.svc.d.Runner.InTx(ctx, func(tx Tx) error {
		return decideNotice(ctx, tx, NoticeDecision{
			Notice: n, To: paymentnotice.StateDismissed, Reason: reason, ActorID: staffID, ActorLabel: actor.Label,
		}, nil)
	})
	if errors.Is(err, ErrNoticeConflict) {
		current, readErr := r.store.Notice(ctx, noticeID)
		if readErr != nil {
			return current, readErr
		}
		return current, &NoticeClosedError{State: current.State}
	}
	if err != nil {
		return n, fmt.Errorf("billing: dismiss notice: %w", err)
	}
	return r.store.Notice(ctx, noticeID)
}
