package accountrequest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
)

// WelcomeMailer sends the welcome email (spec 7b.5, rule 9). *mailer.Mailer is
// one. to must be a plain address; appURL is the only link of the message.
type WelcomeMailer interface {
	SendWelcome(ctx context.Context, to, workspaceName, planName, appURL string) error
}

const (
	// maxWelcomeAttempts is how many sends the job tries for one request.
	maxWelcomeAttempts = 3
	// maxResendsPerDay is how many times staff may send one request's welcome
	// again in resendWindow (spec 7b.5, rule 9).
	maxResendsPerDay = 3
	resendWindow     = 24 * time.Hour
)

var errNoMailer = errors.New("no email provider is configured")

func resendKey(requestID string) string { return "mpa:welcome_resend:" + requestID }

// resendable reports whether staff may send the welcome of r again: the person
// has an account, and the request is completed or failed at step 3, where the
// account stays for staff to finish.
func resendable(r Request) bool {
	return r.UserID != "" && (r.State == StateCompleted || (r.State == StateFailed && r.ErrorCode == ErrorCodeFinalizeFailed))
}

func notResendable(r Request) *Error {
	return &Error{
		Status: 409, Code: CodeNotResendable, State: r.State,
		Message: "the welcome can be sent again only for a request that has an account and is completed or failed at finalize",
	}
}

// sendDue sends the welcome of up to 20 completed requests that have none yet and
// fewer than 3 failed sends.
func (f *Finalizer) sendDue(ctx context.Context) error {
	ids, err := f.Store.ListWelcomeDue(ctx, batchSize, maxWelcomeAttempts)
	if err != nil {
		return err
	}
	var errs []error
	for _, id := range ids {
		err := f.Store.InTx(ctx, func(tx FinalizeTx) error {
			// The row stays locked while the email goes out, so a parallel run skips it.
			r, found, err := tx.LockWelcomeDue(ctx, id, maxWelcomeAttempts)
			if err != nil || !found {
				return err
			}
			_, _, err = f.deliver(ctx, tx, r, nil)
			return err
		})
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// deliver sends the welcome of r and records the outcome in tx: welcome_sent_at
// and an account.welcome_sent (account.welcome_resent for a resend) row, or an account.welcome_failed row. A failed
// send is the returned sendErr, not an error of the transaction, so the caller
// commits the record. by is the staff member who asked for a resend; nil is the
// job, whose failed sends are counted in welcome_attempts.
func (f *Finalizer) deliver(ctx context.Context, tx FinalizeTx, r Request, by *Actor) (out Request, sendErr, err error) {
	sendErr = errNoMailer
	if f.Mailer != nil {
		sendErr = f.Mailer.SendWelcome(ctx, r.Email, r.WorkspaceName, f.planName(ctx, r.PlanKey), f.AppURL)
	}
	if sendErr != nil && ctx.Err() != nil {
		return r, nil, ctx.Err() // stopping: roll back, count nothing
	}
	e := audit.Entry{TargetType: targetAccountRequest, TargetID: r.ID, TargetLabel: r.Email}
	if by != nil {
		e.ActorType, e.ActorID, e.ActorLabel = audit.ActorStaff, by.ID, by.Label
	} else {
		e.ActorType = audit.ActorSystem
	}
	resend := by != nil
	if sendErr != nil {
		f.Log.Warn("account request: welcome not sent", "request_id", r.ID, "resend", resend)
		out = r
		if !resend {
			if out, err = tx.MarkWelcomeFailed(ctx, r.ID); err != nil {
				return r, nil, err
			}
		}
		e.Action = audit.ActionAccountWelcomeFailed
		e.Detail = map[string]any{"resend": resend, "welcome_attempts": out.WelcomeAttempts}
		return out, sendErr, audit.Record(ctx, tx, e)
	}
	if out, err = tx.MarkWelcomeSent(ctx, r.ID, f.Now().UTC().Truncate(time.Microsecond)); err != nil {
		return r, nil, err
	}
	e.Action = audit.ActionAccountWelcomeSent
	if resend {
		e.Action = audit.ActionAccountWelcomeResent
	}
	e.Detail = map[string]any{"resend": resend}
	return out, nil, audit.Record(ctx, tx, e)
}

// planName is the plan's name for the welcome, or its key when the plan cannot
// be read: the welcome is worth sending without the name.
func (f *Finalizer) planName(ctx context.Context, key string) string {
	if p, found, err := f.Plans.Get(ctx, key); err == nil && found && p.Name != "" {
		return p.Name
	}
	return key
}

// resend sends the welcome of the request again, in one transaction that holds
// the request's lock, so it cannot overlap the job's send. A failed send is
// recorded and answered with 502 welcome_send_failed.
func (f *Finalizer) resend(ctx context.Context, by Actor, id string) (Request, error) {
	var out Request
	var sendErr error
	err := f.Store.InTx(ctx, func(tx FinalizeTx) error {
		out, sendErr = Request{}, nil
		r, found, err := tx.LockRequest(ctx, id)
		if err != nil {
			return err
		}
		if !found {
			return ErrNotFound
		}
		if !resendable(r) {
			return notResendable(r)
		}
		out, sendErr, err = f.deliver(ctx, tx, r, &by)
		return err
	})
	if err != nil {
		return Request{}, err
	}
	if sendErr != nil {
		return Request{}, &Error{Status: 502, Code: CodeWelcomeSendFailed, Message: "the welcome email could not be sent"}
	}
	return out, nil
}

// ResendWelcome sends the welcome of a request again (spec 7b.5, rule 9), at most
// three times a day for each request. It returns the request with its new
// welcome_sent_at. A request that cannot be resent is a 409
// account_request_not_resendable and uses none of the day's count; past the limit
// it is a 429 welcome_resend_rate_limited.
func (s *Service) ResendWelcome(ctx context.Context, by Actor, id string) (Request, error) {
	if s.Welcome == nil {
		return Request{}, errors.New("accountrequest: the welcome is not configured")
	}
	cur, found, err := s.Reader.Get(ctx, id)
	if err != nil {
		return Request{}, err
	}
	if !found {
		return Request{}, ErrNotFound
	}
	if !resendable(cur) {
		return Request{}, notResendable(cur)
	}
	ok, err := s.Limiter.Allow(ctx, resendKey(id), maxResendsPerDay, resendWindow)
	if err != nil {
		return Request{}, fmt.Errorf("count welcome resends: %w", err)
	}
	if !ok {
		return Request{}, &Error{Status: 429, Code: CodeWelcomeRateLimited, Message: "the welcome was sent again too many times today", RetryAfter: resendWindow}
	}
	return s.Welcome.resend(ctx, by, id)
}
