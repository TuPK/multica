// Package lifecycle moves workspace subscriptions through their states on time
// and emails owners and admins about it (spec 5.9).
//
//	trialing --trial_end--> free
//	active --period over, not renewed--> past_due --grace over--> read_only --30 days--> free
//	active --period over, cancel_at_period_end--> free
//	active (usage plan) --usage invoice unpaid past its due days--> past_due, then as above
//
// A payment at any step returns the workspace to active; that is
// billing.Service.ApplyPayment, not this package.
//
// Every move is one conditional update on the row the job saw (same status,
// same version), so a second run, or a run in another process, cannot move a
// subscription twice. The caller passes the time; nothing here reads the clock.
package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
	"github.com/multica-ai/multica/platform-management/internal/workspacestatus"
	"github.com/multica-ai/multica/platform-management/workspacestate"
)

const (
	// JobName is the job's name in internal/jobs. The name is the lock key and
	// the key of the run history, so it does not change.
	JobName = "subscription_lifecycle"
	// Interval is how often the job runs.
	Interval = time.Hour

	// GraceDays is how long a past_due subscription keeps full rights, and
	// ReadOnlyDays how long a workspace stays read_only before the subscription
	// goes to free, counted from grace_until.
	GraceDays      = 7
	ReadOnlyDays   = 30
	GracePeriod    = GraceDays * 24 * time.Hour
	ReadOnlyPeriod = ReadOnlyDays * 24 * time.Hour

	// BatchSize bounds the subscriptions one run takes per edge. A run that
	// hits it leaves the rest to the next run.
	BatchSize = 500
)

// Subscription statuses the job writes. active is subscription.StatusActive.
const (
	StatusTrialing = "trialing"
	StatusPastDue  = "past_due"
	StatusReadOnly = "read_only"
	StatusFree     = "free"
)

// ReminderDays are the days before current_period_end at which an active
// subscription is reminded, largest first.
var ReminderDays = []int{14, 7, 1}

// systemUser is the changed_by of a state change the job makes: the nil UUID.
// platform_workspace.changed_by and the audit actor are filled from a staff
// user, and the job has none.
const (
	systemUser  = "00000000-0000-0000-0000-000000000000"
	systemLabel = "system: " + JobName

	// reasonFreePlan is the platform_workspace reason of the move back to
	// active when the read-only period ends.
	reasonFreePlan = "subscription ended: free plan"
)

// Notice kinds, the kind column of billing_notice.
const (
	KindReminder = "remind"
	KindState    = "state"
)

// Contact is who a workspace's notices go to.
type Contact struct {
	// Name is the workspace name, and Slug its address segment.
	Name, Slug string
	// Emails are the owners' and admins' addresses.
	Emails []string
}

// Notice is one row of billing_notice.
type Notice struct {
	WorkspaceID string
	Key         string
	Kind        string
}

// Message is one email.
type Message struct {
	To      []string
	Subject string
	// Body is plain text.
	Body string
}

// Sender sends an email.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

// Tx is the reads and writes of one lifecycle step, inside one transaction.
type Tx interface {
	// Move writes to's plan, status, seats, period, grace and cancel flag to
	// the row that still has from's status and version, and moves its version up
	// by one. It returns applied == false, and writes nothing, when the row has
	// changed since from was read.
	Move(ctx context.Context, from, to subscription.Subscription) (after subscription.Subscription, applied bool, err error)
	// WorkspaceState returns the workspace's platform_workspace status and
	// reason, and found == false when it has no row, which is active. It locks
	// the row.
	WorkspaceState(ctx context.Context, workspaceID string) (status workspacestate.Status, reason string, found bool, err error)
	// ExpireOrder moves the order to expired when it is still pending and its
	// expires_at is at or before now. It returns expired == false, and writes
	// nothing, for an order that is not pending or has not expired.
	ExpireOrder(ctx context.Context, id string, now time.Time) (order billing.Order, expired bool, err error)

	workspacestatus.Tx
}

// Store is everything the job reads and writes.
type Store interface {
	// Due returns up to limit subscriptions for which e.Due holds at now, the
	// longest-overdue first.
	Due(ctx context.Context, e Edge, now time.Time, limit int) ([]subscription.Subscription, error)
	// UsageUnpaid returns up to limit active subscriptions on a usage plan that
	// have a pending usage invoice created dueDays or more before now, the
	// oldest invoice first. Each has GraceUntil set to that invoice's created_at
	// plus dueDays and GracePeriod: the value the move writes.
	UsageUnpaid(ctx context.Context, now time.Time, dueDays, limit int) ([]subscription.Subscription, error)
	// Reminders returns up to limit active subscriptions that do not stop at
	// the end of the period and whose current_period_end is after now and not
	// after until.
	Reminders(ctx context.Context, now, until time.Time, limit int) ([]subscription.Subscription, error)
	// DueOrders returns up to limit pending orders whose expires_at is at or
	// before now, the longest-expired first.
	DueOrders(ctx context.Context, now time.Time, limit int) ([]billing.Order, error)
	// InTx runs fn in one transaction: it commits when fn returns nil and rolls
	// back otherwise.
	InTx(ctx context.Context, fn func(Tx) error) error
	// Contact returns the workspace's name, slug and notice addresses.
	Contact(ctx context.Context, workspaceID string) (Contact, error)
	// InsertNotice records the notice and returns inserted == false when its
	// key is already recorded for the workspace.
	InsertNotice(ctx context.Context, n Notice) (inserted bool, err error)
	// DeleteNotice removes the record, so that the notice can be sent again.
	DeleteNotice(ctx context.Context, workspaceID, key string) error
}

// EntitlementCache drops the cached entitlement policy of a workspace.
// *entitlement.Service is one.
type EntitlementCache interface {
	Invalidate(workspaceID string)
}

// Deps are what a Service needs.
type Deps struct {
	Store Store
	Mail  Sender
	// Workspaces drops the cached workspace state after a commit that changed
	// it. *workspacestate.Guard is one.
	Workspaces workspacestatus.Invalidator
	// Entitlements drops the cached policy after a subscription moves.
	Entitlements EntitlementCache
	// AppURL is the origin of the Multica web app. The emails link to the
	// workspace's settings page on it.
	AppURL string
	// UsageDueDays is PLATFORM_USAGE_DUE_DAYS: how long a usage invoice may stay
	// unpaid before its subscription goes past_due (spec 7b.3, rule 13).
	UsageDueDays int
	Log          *slog.Logger
}

// Service is the subscription_lifecycle job.
type Service struct {
	d Deps
}

// New returns a Service. Store, Mail, Workspaces, Entitlements and AppURL are
// required.
func New(d Deps) (*Service, error) {
	if d.Store == nil || d.Mail == nil || d.Workspaces == nil || d.Entitlements == nil || d.AppURL == "" || d.UsageDueDays < 1 {
		return nil, errors.New("lifecycle: Store, Mail, Workspaces, Entitlements, AppURL and UsageDueDays are required")
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	return &Service{d: d}, nil
}

// Report counts what one run did.
type Report struct {
	// Moved counts the subscriptions moved, by edge.
	Moved map[Edge]int
	// Skipped counts the moves another writer got to first.
	Skipped int
	// Reminders and StateEmails count the emails sent.
	Reminders, StateEmails int
	// NoRecipients counts the notices dropped because the workspace has no
	// owner or admin with an email.
	NoRecipients int
}

// Run takes the subscriptions that are due at now through their next step and
// sends the notices. It continues past a failing subscription and returns the
// failures joined. A run that ends early, because ctx ended, returns what it
// did so far.
func (s *Service) Run(ctx context.Context, now time.Time) (Report, error) {
	rep := Report{Moved: map[Edge]int{}}
	var errs []error
	for _, e := range Edges {
		if err := ctx.Err(); err != nil {
			return rep, errors.Join(append(errs, err)...)
		}
		var due []subscription.Subscription
		var err error
		if e == EdgeUsageUnpaid {
			due, err = s.d.Store.UsageUnpaid(ctx, now, s.d.UsageDueDays, BatchSize)
		} else {
			due, err = s.d.Store.Due(ctx, e, now, BatchSize)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("lifecycle: list %s: %w", e, err))
			continue
		}
		for _, sub := range due {
			if err := ctx.Err(); err != nil {
				return rep, errors.Join(append(errs, err)...)
			}
			// The store filters on the same rule; this keeps a row it should not
			// have returned from moving.
			if !e.Due(sub, now) {
				continue
			}
			if err := s.step(ctx, e, sub, &rep); err != nil {
				errs = append(errs, fmt.Errorf("lifecycle: %s for workspace %s: %w", e, sub.WorkspaceID, err))
			}
		}
	}
	if err := s.remind(ctx, now, &rep); err != nil {
		errs = append(errs, err)
	}
	return rep, errors.Join(errs...)
}

// step moves one subscription along e, then sends the state email. A move
// that another writer got to first is skipped and sends nothing.
func (s *Service) step(ctx context.Context, e Edge, before subscription.Subscription, rep *Report) error {
	var after subscription.Subscription
	var applied, workspaceChanged bool
	err := s.d.Store.InTx(ctx, func(tx Tx) error {
		after, applied, workspaceChanged = subscription.Subscription{}, false, false
		moved, ok, err := tx.Move(ctx, before, e.Apply(before))
		if err != nil || !ok {
			return err
		}
		after, applied = moved, true
		workspaceChanged, err = s.moveWorkspace(ctx, tx, e, before.WorkspaceID)
		return err
	})
	if err != nil {
		return err
	}
	if !applied {
		rep.Skipped++
		return nil
	}
	rep.Moved[e]++
	// After the commit, so that nothing reads the old state back into the cache.
	s.d.Entitlements.Invalidate(after.WorkspaceID)
	if workspaceChanged {
		if err := s.d.Workspaces.Invalidate(ctx, after.WorkspaceID); err != nil {
			s.d.Log.Warn("lifecycle: workspace state cache key not deleted; the cached state stays until it expires",
				"workspace_id", after.WorkspaceID, "ttl", workspacestate.TTL, "error", err)
		}
	}
	return s.notifyState(ctx, e, before, after, rep)
}

// moveWorkspace changes the workspace's state with the edge, inside the
// subscription's transaction, and reports whether it changed.
//
// Staff decisions stand: a workspace that staff suspended or marked for
// deletion is not made read_only, and a workspace that is read_only for
// another reason than billing is not made active again.
func (s *Service) moveWorkspace(ctx context.Context, tx Tx, e Edge, workspaceID string) (bool, error) {
	var to workspacestate.Status
	var reason string
	switch e {
	case EdgeGraceEnded:
		status, _, found, err := tx.WorkspaceState(ctx, workspaceID)
		if err != nil {
			return false, err
		}
		if found && status != workspacestate.Active {
			return false, nil
		}
		to, reason = workspacestate.ReadOnly, billing.ReasonBilling
	case EdgeReadOnlyEnded:
		status, why, found, err := tx.WorkspaceState(ctx, workspaceID)
		if err != nil {
			return false, err
		}
		if !found || status != workspacestate.ReadOnly || why != billing.ReasonBilling {
			return false, nil
		}
		to, reason = workspacestate.Active, reasonFreePlan
	default:
		return false, nil
	}
	return workspacestatus.SetInTx(ctx, tx, workspacestatus.Change{
		WorkspaceID: workspaceID, Status: to, Reason: reason,
		ChangedBy: systemUser, ActorLabel: systemLabel,
	})
}

// causeOf is why the edge ends in free.
func causeOf(e Edge) string {
	switch e {
	case EdgeTrialEnded:
		return causeTrial
	case EdgeCanceledLapsed:
		return causeCanceled
	}
	return causeLapsed
}

// notifyState emails the owners and admins about a transition. The key holds
// the new status and the version after the move, so each transition has one.
func (s *Service) notifyState(ctx context.Context, e Edge, before, after subscription.Subscription, rep *Report) error {
	n := Notice{
		WorkspaceID: after.WorkspaceID, Kind: KindState,
		Key: fmt.Sprintf("state:%s:%d", after.Status, after.Version),
	}
	out, err := s.notify(ctx, n, func(c Contact) (string, string, error) {
		// The free subscription has lost its plan and period; the email speaks
		// of the ones it had.
		about := after
		if after.Status == StatusFree {
			about = before
		}
		d := s.data(c, about)
		d.Cause = causeOf(e)
		return renderState(after.Status, d)
	})
	switch out {
	case outcomeSent:
		rep.StateEmails++
	case outcomeNoRecipients:
		rep.NoRecipients++
	}
	return err
}

// reminderWindow returns the reminder an active subscription is in at now: the
// smallest of ReminderDays that is not more than the days left. A run that
// starts late sends the reminder for its window and not the earlier ones.
func reminderWindow(end, now time.Time) (days int, ok bool) {
	if !now.Before(end) {
		return 0, false
	}
	for i := len(ReminderDays) - 1; i >= 0; i-- {
		if !now.Before(end.Add(-time.Duration(ReminderDays[i]) * 24 * time.Hour)) {
			return ReminderDays[i], true
		}
	}
	return 0, false
}

// remind sends the reminder each active, renewing subscription is due.
func (s *Service) remind(ctx context.Context, now time.Time, rep *Report) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	until := now.Add(time.Duration(ReminderDays[0]) * 24 * time.Hour)
	subs, err := s.d.Store.Reminders(ctx, now, until, BatchSize)
	if err != nil {
		return fmt.Errorf("lifecycle: list reminders: %w", err)
	}
	var errs []error
	for _, sub := range subs {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		if sub.Status != subscription.StatusActive || sub.CancelAtPeriodEnd || sub.CurrentPeriodEnd == nil {
			continue
		}
		days, ok := reminderWindow(*sub.CurrentPeriodEnd, now)
		if !ok {
			continue
		}
		n := Notice{
			WorkspaceID: sub.WorkspaceID, Kind: KindReminder,
			Key: fmt.Sprintf("remind:%d:%d", sub.CurrentPeriodEnd.Unix(), days),
		}
		out, err := s.notify(ctx, n, func(c Contact) (string, string, error) {
			d := s.data(c, sub)
			d.DaysLeft = days
			return renderReminder(d)
		})
		switch out {
		case outcomeSent:
			rep.Reminders++
		case outcomeNoRecipients:
			rep.NoRecipients++
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("lifecycle: reminder for workspace %s: %w", sub.WorkspaceID, err))
		}
	}
	return errors.Join(errs...)
}

// Job returns the function internal/jobs runs every Interval. now is the clock,
// read once per run.
func (s *Service) Job(now func() time.Time) func(context.Context) error {
	return func(ctx context.Context) error {
		rep, err := s.Run(ctx, now())
		if rep.total() > 0 || rep.Reminders > 0 || rep.StateEmails > 0 || rep.Skipped > 0 {
			s.d.Log.Info("lifecycle: run finished", "moved", rep.total(), "skipped", rep.Skipped,
				"reminders", rep.Reminders, "state_emails", rep.StateEmails, "no_recipients", rep.NoRecipients)
		}
		return err
	}
}

func (r Report) total() int {
	n := 0
	for _, c := range r.Moved {
		n += c
	}
	return n
}
