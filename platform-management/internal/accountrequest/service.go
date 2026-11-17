package accountrequest

import (
	"context"
	"fmt"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/plan"
)

const (
	targetAccountRequest = "account_request"
	rateWindow           = time.Hour
	defaultListLimit     = 50
)

// Limiter counts requests in a window. platformauth.RedisLimiter is one.
type Limiter interface {
	// Allow counts one request under key and reports whether it is within limit.
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

// Deps are the Service's dependencies. Now defaults to time.Now.
type Deps struct {
	Runner  Runner
	Reader  Reader
	Plans   plan.Reader
	Limiter Limiter
	// CreatePerHour is PLATFORM_ACCOUNT_CREATE_PER_HOUR.
	CreatePerHour int
	// IndividualMaxSeats is PLATFORM_INDIVIDUAL_MAX_SEATS.
	IndividualMaxSeats int
	// Welcome sends the welcome email. Nil means ResendWelcome is unavailable.
	Welcome *Finalizer
	Now     func() time.Time
}

// Service creates, cancels and reads account requests.
type Service struct{ Deps }

// New builds the Service.
func New(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Service{d}
}

func rateKey(staffID string) string { return "mpa:account_create:" + staffID }

// Create checks in, then stores a pending request and its audit row in one
// transaction. created is false when a live request already holds the email:
// the answer is then that request and nothing is written (spec 7b.5, rule 5).
func (s *Service) Create(ctx context.Context, by Actor, in Input) (r Request, created bool, err error) {
	in, verr := ValidateInput(in)
	if verr != nil {
		return Request{}, false, verr
	}
	p, found, err := s.Plans.Get(ctx, in.PlanKey)
	if err != nil {
		return Request{}, false, fmt.Errorf("read plan: %w", err)
	}
	if !found {
		return Request{}, false, invalid("the plan does not exist", "plan_key")
	}
	if in, verr = CheckPlan(in, p, s.IndividualMaxSeats); verr != nil {
		return Request{}, false, verr
	}

	err = s.Runner.InTx(ctx, func(tx Tx) error {
		r, created = Request{}, false
		if _, exists, err := tx.UserIDByEmail(ctx, in.Email); err != nil {
			return err
		} else if exists {
			return &Error{Status: 409, Code: CodeEmailExists, Message: "an account with this email already exists"}
		}
		if live, ok, err := tx.LiveByEmail(ctx, in.Email); err != nil {
			return err
		} else if ok {
			r = live
			return nil
		}
		// A request that only repeats a live one does not use the staff member's count.
		ok, err := s.Limiter.Allow(ctx, rateKey(by.ID), s.CreatePerHour, rateWindow)
		if err != nil {
			return fmt.Errorf("count account requests: %w", err)
		}
		if !ok {
			return &Error{Status: 429, Code: CodeRateLimited, Message: "too many account requests this hour", RetryAfter: rateWindow}
		}
		row, inserted, err := tx.Insert(ctx, in, by.ID)
		if err != nil {
			return err
		}
		if !inserted {
			// The unique index on the email: another request committed first.
			winner, ok, err := tx.LiveByEmail(ctx, in.Email)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("account request for %s conflicted but no live request is visible", in.Email)
			}
			r = winner
			return nil
		}
		r, created = row, true
		return audit.Record(ctx, tx, by.entry(audit.Entry{
			Action: audit.ActionAccountProvisionRequested, TargetType: targetAccountRequest, TargetID: row.ID, TargetLabel: row.Email,
			Detail: map[string]any{"kind": row.Kind, "plan": row.PlanKey, "seats": row.Seats, "interval": row.Interval, "trial": row.Trial},
		}))
	})
	if err != nil {
		return Request{}, false, err
	}
	return r, created, nil
}

// Cancel moves a pending request to canceled, with its audit row (spec 7b.5,
// rule 12). It returns ErrNotFound for an unknown id and a 409
// account_request_not_cancelable for any other state.
func (s *Service) Cancel(ctx context.Context, by Actor, id string) (Request, error) {
	var out Request
	err := s.Runner.InTx(ctx, func(tx Tx) error {
		cur, found, err := tx.Lock(ctx, id)
		if err != nil {
			return err
		}
		if !found {
			return ErrNotFound
		}
		if cur.State != StatePending {
			return &Error{Status: 409, Code: CodeNotCancelable, Message: "only a pending request can be canceled", State: cur.State}
		}
		if out, err = tx.Cancel(ctx, id); err != nil {
			return err
		}
		return audit.Record(ctx, tx, by.entry(audit.Entry{
			Action: audit.ActionAccountProvisionCanceled, TargetType: targetAccountRequest, TargetID: id, TargetLabel: cur.Email,
		}))
	})
	if err != nil {
		return Request{}, err
	}
	return out, nil
}

// List returns the newest requests, up to the limit (50 when it is 0).
func (s *Service) List(ctx context.Context, q ListQuery) ([]Request, error) {
	if q.Limit <= 0 {
		q.Limit = defaultListLimit
	}
	return s.Reader.List(ctx, q)
}

// Get returns found == false for an unknown id.
func (s *Service) Get(ctx context.Context, id string) (Request, bool, error) {
	return s.Reader.Get(ctx, id)
}

func (a Actor) entry(e audit.Entry) audit.Entry {
	e.ActorType, e.ActorID, e.ActorLabel = audit.ActorStaff, a.ID, a.Label
	return e
}
