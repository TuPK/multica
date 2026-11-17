package accountrequest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/money"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
)

// FinalizeTx is the writes of one step-3 operation, all inside one transaction
// with its audit rows: the finalize of a request, the record of its failure, or
// one welcome email.
type FinalizeTx interface {
	// LockProvisioned reads a provisioned request that has a user and a
	// workspace and locks it. found is false when the request is in another
	// state or another transaction holds it (SKIP LOCKED).
	LockProvisioned(ctx context.Context, id string) (r Request, found bool, err error)
	// LockWelcomeDue reads a completed request whose welcome is unsent and has
	// attempts left, and locks it. found is false as for LockProvisioned.
	LockWelcomeDue(ctx context.Context, id string, maxAttempts int) (r Request, found bool, err error)
	// LockRequest reads a request and waits for its lock.
	LockRequest(ctx context.Context, id string) (r Request, found bool, err error)
	// StaffEmail returns the email of a user, empty when there is none.
	StaffEmail(ctx context.Context, userID string) (string, error)
	// SaveProfile inserts the workspace's billing profile or replaces it.
	SaveProfile(ctx context.Context, p subscription.BillingProfile) error
	// LockSubscription, InsertSubscription and UpdateSubscription are the
	// writes of subscription.Tx: the first row has version 1, an update adds one.
	LockSubscription(ctx context.Context, workspaceID string) (s subscription.Subscription, found bool, err error)
	InsertSubscription(ctx context.Context, s subscription.Subscription) (subscription.Subscription, bool, error)
	UpdateSubscription(ctx context.Context, s subscription.Subscription) (subscription.Subscription, error)
	// InsertOrder and MarkOrderPaid are the order writes of billing.Tx. The order
	// is inserted pending, as billing writes it, then marked paid.
	InsertOrder(ctx context.Context, n billing.NewOrder) (o billing.Order, inserted bool, err error)
	MarkOrderPaid(ctx context.Context, orderID, confirmedBy string, paidAt time.Time) (billing.Order, error)
	// Complete moves a provisioned request to completed and clears its copy of
	// the billing profile.
	Complete(ctx context.Context, id string, at time.Time) (Request, error)
	// RecordFinalizeFailure counts one failed attempt. The request becomes
	// failed with finalize_failed when permanent is true or the attempt was the
	// last; otherwise it stays provisioned. found is false when the request is no
	// longer provisioned.
	RecordFinalizeFailure(ctx context.Context, id string, permanent bool, maxAttempts int) (r Request, found bool, err error)
	// MarkWelcomeSent sets welcome_sent_at.
	MarkWelcomeSent(ctx context.Context, id string, at time.Time) (Request, error)
	// MarkWelcomeFailed counts one failed send in welcome_attempts.
	MarkWelcomeFailed(ctx context.Context, id string) (Request, error)

	audit.Writer
}

// FinalizeStore runs the transactions of step 3 and lists the requests it is due
// for.
type FinalizeStore interface {
	// InTx runs fn in one transaction: it commits when fn returns nil and rolls
	// back otherwise. It returns fn's error as it is.
	InTx(ctx context.Context, fn func(FinalizeTx) error) error
	// ListProvisioned returns up to limit ids of provisioned requests, oldest first.
	ListProvisioned(ctx context.Context, limit int) ([]string, error)
	// ListWelcomeDue returns up to limit ids of completed requests whose welcome
	// is unsent and has fewer than maxAttempts attempts, oldest first.
	ListWelcomeDue(ctx context.Context, limit, maxAttempts int) ([]string, error)
}

// Invalidator drops the cached entitlement policy of a workspace.
// *entitlement.Service is one.
type Invalidator interface {
	Invalidate(workspaceID string)
}

// FinalizerDeps are the Finalizer's dependencies. Mailer may be nil: no welcome
// is sent then and a completed request keeps an unsent welcome. Now defaults to
// time.Now and Log to slog.Default.
type FinalizerDeps struct {
	Store  FinalizeStore
	Plans  plan.Reader
	Cache  Invalidator
	Mailer WelcomeMailer
	// AppURL is PLATFORM_APP_URL, the only link of the welcome.
	AppURL string
	Now    func() time.Time
	Log    *slog.Logger
}

// Finalizer is step 3 of spec 7b.5, rule 7, and the welcome of rule 9.
type Finalizer struct{ FinalizerDeps }

const (
	// maxFinalizeAttempts is how many times step 3 is tried before the request
	// fails (spec 7b.5, rule 7). The job runs once a minute, so the tries are at
	// least a minute apart.
	maxFinalizeAttempts = 3
	// batchSize is how many requests one run takes for each of its two tasks.
	batchSize = 20

	// grantProvider is the provider of the zero-amount order that records a grant.
	grantProvider = "admin_grant"
	// grantKeyPrefix starts the idempotency key of that order: one request, one order.
	grantKeyPrefix = "account_request:"
	grantReason    = "account created by staff"
)

// NewFinalizer returns a Finalizer. Store, Plans and Cache are required.
func NewFinalizer(d FinalizerDeps) (*Finalizer, error) {
	if d.Store == nil || d.Plans == nil || d.Cache == nil {
		return nil, errors.New("accountrequest: Store, Plans and Cache are required")
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	return &Finalizer{d}, nil
}

// permanentError marks an error no retry can fix.
type permanentError struct{ err error }

func (e *permanentError) Error() string { return e.err.Error() }
func (e *permanentError) Unwrap() error { return e.err }

// Permanent marks err as one that a retry cannot fix: the request fails at once
// instead of after the third attempt. Every error not marked is temporary.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &permanentError{err}
}

// IsPermanent reports whether err, or an error it wraps, was marked Permanent.
func IsPermanent(err error) bool {
	var p *permanentError
	return errors.As(err, &p)
}

// RunOnce is one run of the job: it finishes up to 20 provisioned requests, then
// sends up to 20 welcomes that are due. A request that fails does not stop the
// others. The error is the joined failures that could not be recorded.
func (f *Finalizer) RunOnce(ctx context.Context) error {
	var errs []error
	ids, err := f.Store.ListProvisioned(ctx, batchSize)
	if err != nil {
		errs = append(errs, err)
	}
	for _, id := range ids {
		if err := f.finalizeOne(ctx, id); err != nil {
			errs = append(errs, err)
		}
	}
	if f.Mailer != nil {
		if err := f.sendDue(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// finalizeOne runs step 3 for one request in one transaction. A run that finds
// the request locked or no longer provisioned does nothing. A failure rolls the
// transaction back and is counted in a second one.
func (f *Finalizer) finalizeOne(ctx context.Context, id string) error {
	var done bool
	var workspaceID string
	err := f.Store.InTx(ctx, func(tx FinalizeTx) error {
		done = false
		r, found, err := tx.LockProvisioned(ctx, id)
		if err != nil || !found {
			return err
		}
		if err := f.finalize(ctx, tx, r); err != nil {
			return err
		}
		done, workspaceID = true, r.WorkspaceID
		return nil
	})
	if err == nil {
		if done {
			f.Cache.Invalidate(workspaceID)
			f.Log.Info("account request finalized", "request_id", id, "workspace_id", workspaceID)
		}
		return nil
	}
	if ctx.Err() != nil {
		return err // the service is stopping; that is not a failed attempt
	}
	return f.recordFailure(ctx, id, err)
}

func (f *Finalizer) recordFailure(ctx context.Context, id string, cause error) error {
	permanent := IsPermanent(cause)
	f.Log.Warn("account request: finalize failed", "request_id", id, "permanent", permanent, "error", cause)
	return f.Store.InTx(ctx, func(tx FinalizeTx) error {
		r, found, err := tx.RecordFinalizeFailure(ctx, id, permanent, maxFinalizeAttempts)
		if err != nil || !found || r.State != StateFailed {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorType: audit.ActorSystem, Action: audit.ActionAccountProvisionFailed,
			TargetType: targetAccountRequest, TargetID: r.ID, TargetLabel: r.Email,
			Detail: map[string]any{"error_code": r.ErrorCode, "attempts": r.Attempts, "permanent": permanent, "kind": r.Kind, "plan": r.PlanKey},
		})
	})
}

// finalize writes everything of step 3 for the locked request r.
func (f *Finalizer) finalize(ctx context.Context, tx FinalizeTx, r Request) error {
	now := f.Now().UTC().Truncate(time.Microsecond)
	p, found, err := f.Plans.Get(ctx, r.PlanKey)
	if err != nil {
		return fmt.Errorf("read plan: %w", err)
	}
	if !found {
		return Permanent(fmt.Errorf("plan %q does not exist", r.PlanKey))
	}
	grant, err := grantFor(r, p, now)
	if err != nil {
		return err
	}
	staffEmail, err := tx.StaffEmail(ctx, r.CreatedBy)
	if err != nil {
		return err
	}
	staff := audit.Entry{ActorType: audit.ActorStaff, ActorID: r.CreatedBy, ActorLabel: staffEmail, WorkspaceID: r.WorkspaceID}

	if r.Kind == KindBusiness {
		if err := f.saveBusinessProfile(ctx, tx, r, staff, now); err != nil {
			return err
		}
	}
	if grant != nil {
		if err := f.writeGrant(ctx, tx, r, p, *grant, staff, now); err != nil {
			return err
		}
	}

	e := staff
	e.Action, e.TargetType, e.TargetID, e.TargetLabel = audit.ActionAccountProvisioned, targetAccountRequest, r.ID, r.Email
	e.Detail = map[string]any{"kind": r.Kind, "plan": r.PlanKey, "created_by": r.CreatedBy}
	if err := audit.Record(ctx, tx, e); err != nil {
		return err
	}
	_, err = tx.Complete(ctx, r.ID, now)
	return err
}

// grantFor returns the subscription the request's activation asks for, or nil
// for a plan with nothing to grant (spec 7b.5, rule 6). Its WorkspaceID is set
// by the caller.
func grantFor(r Request, p plan.Plan, now time.Time) (*subscription.Subscription, error) {
	switch {
	case p.Key == plan.FreeKey:
		return nil, nil
	case p.BillingModel == plan.BillingUsage:
		start, end := subscription.UsagePeriod(now)
		return &subscription.Subscription{
			PlanKey: p.Key, Status: subscription.StatusActive, Seats: 0, Interval: subscription.IntervalMonth,
			CurrentPeriodStart: &start, CurrentPeriodEnd: &end,
		}, nil
	case r.Trial:
		if p.TrialDays <= 0 {
			return nil, Permanent(fmt.Errorf("plan %q has no trial", p.Key))
		}
		end := now.AddDate(0, 0, p.TrialDays)
		return &subscription.Subscription{
			PlanKey: p.Key, Status: subscription.StatusTrialing, Seats: r.Seats, Interval: r.Interval,
			CurrentPeriodStart: &now, CurrentPeriodEnd: &end, TrialEnd: &end,
		}, nil
	case r.CompReason != "":
		end, err := billing.PeriodEnd(now, r.Interval)
		if err != nil {
			return nil, Permanent(err)
		}
		return &subscription.Subscription{
			PlanKey: p.Key, Status: subscription.StatusActive, Seats: r.Seats, Interval: r.Interval,
			CurrentPeriodStart: &now, CurrentPeriodEnd: &end,
		}, nil
	}
	return nil, Permanent(fmt.Errorf("plan %q is paid and the request has neither a trial nor a comp reason", p.Key))
}

// saveBusinessProfile writes the workspace's billing profile from the copy the
// request holds, verified now by the staff member who made the request.
func (f *Finalizer) saveBusinessProfile(ctx context.Context, tx FinalizeTx, r Request, staff audit.Entry, now time.Time) error {
	bp := r.BillingProfile
	if bp == nil {
		return Permanent(errors.New("a business request holds no billing profile"))
	}
	err := tx.SaveProfile(ctx, subscription.BillingProfile{
		WorkspaceID: r.WorkspaceID, AccountKind: subscription.AccountBusiness, LegalName: bp.LegalName, TaxCode: bp.TaxCode,
		Address: bp.Address, InvoiceEmail: bp.InvoiceEmail, VerifiedAt: &now, VerifiedBy: r.CreatedBy, UpdatedBy: r.CreatedBy,
	})
	if err != nil {
		return err
	}
	e := staff
	e.Action, e.TargetType, e.TargetID, e.TargetLabel = audit.ActionWorkspaceAccountKindChanged, "workspace", r.WorkspaceID, r.WorkspaceName
	e.Detail = map[string]any{"changes": map[string]any{"account_kind": map[string]any{"from": subscription.AccountIndividual, "to": subscription.AccountBusiness}}}
	return audit.Record(ctx, tx, e)
}

// writeGrant writes the subscription and the zero-amount, already paid order
// that records the grant, with the audit rows each of them has elsewhere. A
// workspace that already has a row (the backfill gives each the free plan) is
// moved on and its version goes up by one; otherwise the row is new, at version 1.
func (f *Finalizer) writeGrant(ctx context.Context, tx FinalizeTx, r Request, p plan.Plan, next subscription.Subscription, staff audit.Entry, now time.Time) error {
	next.WorkspaceID = r.WorkspaceID
	currency := strings.ToLower(p.Currency)
	if currency == "" {
		currency = money.Default
	}
	buyer := json.RawMessage(`{}`)
	if bp := r.BillingProfile; bp != nil {
		b, err := json.Marshal(map[string]string{
			"legal_name": bp.LegalName, "tax_code": bp.TaxCode, "address": bp.Address, "invoice_email": bp.InvoiceEmail,
		})
		if err != nil {
			return err
		}
		buyer = b
	}
	code, err := billing.NewTransferCode()
	if err != nil {
		return err
	}
	o, inserted, err := tx.InsertOrder(ctx, billing.NewOrder{
		Draft: billing.Draft{
			WorkspaceID: r.WorkspaceID, Kind: billing.KindSubscribe, PlanKey: next.PlanKey, Interval: next.Interval,
			Seats: next.Seats, Amount: 0, VATAmount: 0, Currency: currency, Buyer: buyer,
			PeriodStart: next.CurrentPeriodStart, PeriodEnd: next.CurrentPeriodEnd,
			IdempotencyKey: grantKeyPrefix + r.ID, CreatedBy: r.CreatedBy,
		},
		Provider: grantProvider, TransferCode: code, CreatedAt: now, ExpiresAt: now,
	})
	if err != nil {
		return err
	}
	if !inserted {
		return errors.New("the grant order of this request already exists")
	}
	created := staff
	created.Action, created.TargetType, created.TargetID = audit.ActionBillingOrderCreated, "billing_order", o.ID
	created.Detail = orderDetail(o)
	if err := audit.Record(ctx, tx, created); err != nil {
		return err
	}

	cur, exists, err := tx.LockSubscription(ctx, r.WorkspaceID)
	if err != nil {
		return err
	}
	if exists {
		next.LimitOverrides = cur.LimitOverrides // staff's overrides stay with the workspace
		if next, err = tx.UpdateSubscription(ctx, next); err != nil {
			return err
		}
	} else {
		var ok bool
		if next, ok, err = tx.InsertSubscription(ctx, next); err != nil {
			return err
		} else if !ok {
			return errors.New("the subscription was written by another transaction")
		}
	}

	if o, err = tx.MarkOrderPaid(ctx, o.ID, r.CreatedBy, now); err != nil {
		return err
	}
	paid := staff
	paid.Action, paid.TargetType, paid.TargetID = audit.ActionBillingOrderPaid, "billing_order", o.ID
	paid.Detail = orderDetail(o)
	paid.Detail["subscription_version"] = next.Version
	if err := audit.Record(ctx, tx, paid); err != nil {
		return err
	}

	changed := staff
	changed.Action, changed.TargetType, changed.TargetID, changed.TargetLabel = audit.ActionSubscriptionChanged, "workspace", r.WorkspaceID, r.WorkspaceName
	changed.Detail = map[string]any{
		"reason": grantReason, "version": next.Version, "created": !exists, "plan": next.PlanKey, "status": next.Status,
		"seats": next.Seats, "interval": next.Interval, "order_id": o.ID,
	}
	return audit.Record(ctx, tx, changed)
}

// orderDetail names the fields of an order for the audit log, as the billing
// service does, avoiding key names the redaction filter hides.
func orderDetail(o billing.Order) map[string]any {
	return map[string]any{
		"kind": string(o.Kind), "plan": o.PlanKey, "interval": o.Interval, "seats": o.Seats,
		"amount": o.Amount, "vat_amount": o.VATAmount, "currency": o.Currency,
		"provider": o.Provider, "transfer_code": o.TransferCode,
	}
}
