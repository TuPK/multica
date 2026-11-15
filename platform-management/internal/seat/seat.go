// Package seat serves the seat protocol that Multica's seat client
// (server/internal/seatcapacity) calls: reserve, claim, consume, confirm and
// release of a member seat, counted against the seats a workspace bought
// (spec 5.6, Appendix B).
//
// The state of a workspace's seats is the rows of platform_seat_operation
// plus the shared member table. Every call runs in one transaction that first
// takes the workspace lock (see LockKey), so two calls for one workspace never
// interleave.
package seat

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
)

// Operation kinds, as the client sends them.
const (
	KindInvitation = "invitation"
	KindShareJoin  = "share_join"
)

// Operation states, as the recovery worker reads them
// (server/internal/seatcapacity/worker.go).
//
//	reserved   an invitation holds a seat until it expires
//	consuming  an accept or a share-link join is in flight and holds a seat
//	used       the member exists; the member table now counts the seat
//	released   the seat is free
const (
	StateReserved  = "reserved"
	StateConsuming = "consuming"
	StateUsed      = "used"
	StateReleased  = "released"
)

// Reasons the handler and the worker compare against.
const (
	ReasonCapacityFull = "capacity_full"
	ReasonReleased     = "released"
)

// Error codes of the JSON error body.
const (
	CodeOvercommitted = "capacity_overcommitted"
	CodeNotFound      = "operation_not_found"
	CodeInvalid       = "invalid_request"
	CodeInternal      = "internal"
)

// ErrOvercommitted means the workspace has more members than purchased seats.
var ErrOvercommitted = errors.New("workspace members exceed purchased seats")

// ErrNotFound means GetOperation named no operation.
var ErrNotFound = errors.New("seat operation not found")

// Op is one row of platform_seat_operation.
type Op struct {
	WorkspaceID string
	Token       string
	Kind        string
	SubjectID   string
	State       string
	Reason      string
	ExpiresAt   *time.Time
}

// effectiveState reports a reserved seat past its expiry as released (rule 5).
func (o Op) effectiveState(now time.Time) string {
	if o.State == StateReserved && o.ExpiresAt != nil && !o.ExpiresAt.After(now) {
		return StateReleased
	}
	return o.State
}

// Tx is one transaction. Lock is called before anything else.
type Tx interface {
	// Lock takes the workspace's seat lock for the rest of the transaction.
	Lock(ctx context.Context, workspaceID string) error
	Subscription(ctx context.Context, workspaceID string) (subscription.Subscription, bool, error)
	Plan(ctx context.Context, key string) (plan.Plan, bool, error)
	// Members counts the rows of the shared member table.
	Members(ctx context.Context, workspaceID string) (int64, error)
	// Held counts the operations that hold a seat at now.
	Held(ctx context.Context, workspaceID string, now time.Time) (int64, error)
	Operation(ctx context.Context, workspaceID, token string) (Op, bool, error)
	// Insert returns inserted == false when the token already exists.
	Insert(ctx context.Context, op Op) (inserted Op, ok bool, err error)
	SetState(ctx context.Context, workspaceID, token, state, reason string, expiresAt *time.Time) (Op, error)
}

// Runner runs fn in one transaction.
type Runner interface {
	InTx(ctx context.Context, fn func(Tx) error) error
}

// Capacity is the JSON the client reads as seatcapacity.Capacity.
type Capacity struct {
	PurchasedSeats int64 `json:"purchased_seats"`
	UsedSeats      int64 `json:"used_seats"`
	ReservedSeats  int64 `json:"reserved_seats"`
	Version        int64 `json:"version"`
}

// Operation is the JSON the client reads as seatcapacity.Operation.
type Operation struct {
	Token       string  `json:"token"`
	WorkspaceID string  `json:"workspace_id"`
	Kind        string  `json:"kind"`
	SubjectID   string  `json:"subject_id"`
	State       string  `json:"state"`
	ExpiresAt   *string `json:"expires_at,omitempty"`
}

// Decision is the JSON the client reads as seatcapacity.Decision.
type Decision struct {
	Managed   bool       `json:"managed"`
	Allowed   bool       `json:"allowed"`
	Reason    string     `json:"reason,omitempty"`
	Operation *Operation `json:"operation,omitempty"`
	Capacity  *Capacity  `json:"capacity,omitempty"`
}

// Service is the seat protocol.
type Service struct {
	runner Runner
	now    func() time.Time
}

// NewService returns a Service. now is time.Now in production.
func NewService(runner Runner, now func() time.Time) *Service {
	return &Service{runner: runner, now: now}
}

// view is a workspace's seat capacity, or managed == false.
type view struct {
	managed bool
	cap     Capacity
}

// load reads the workspace's capacity inside the locked transaction. A
// workspace with no subscription row, or a plan with no seat limit, is not
// managed.
func (s *Service) load(ctx context.Context, tx Tx, ws string) (view, error) {
	sub, found, err := tx.Subscription(ctx, ws)
	if err != nil {
		return view{}, err
	}
	if !found {
		return view{}, nil
	}
	p, found, err := tx.Plan(ctx, sub.PlanKey)
	if err != nil {
		return view{}, err
	}
	if !found {
		return view{}, fmt.Errorf("plan %q does not exist", sub.PlanKey)
	}
	purchased, ok := subscription.PurchasedSeats(p, &sub)
	if !ok {
		return view{}, nil
	}
	used, err := tx.Members(ctx, ws)
	if err != nil {
		return view{}, err
	}
	held, err := tx.Held(ctx, ws, s.now())
	if err != nil {
		return view{}, err
	}
	return view{managed: true, cap: Capacity{
		PurchasedSeats: purchased, UsedSeats: used, ReservedSeats: held, Version: sub.Version,
	}}, nil
}

// run takes the lock, loads the capacity and runs fn.
func (s *Service) run(ctx context.Context, ws string, fn func(tx Tx, v view) (Decision, error)) (Decision, error) {
	var out Decision
	err := s.runner.InTx(ctx, func(tx Tx) error {
		if err := tx.Lock(ctx, ws); err != nil {
			return err
		}
		v, err := s.load(ctx, tx, ws)
		if err != nil {
			return err
		}
		out, err = fn(tx, v)
		return err
	})
	if err != nil {
		return Decision{}, err
	}
	return out, nil
}

func (s *Service) operationJSON(o Op) *Operation {
	out := &Operation{
		Token: o.Token, WorkspaceID: o.WorkspaceID, Kind: o.Kind, SubjectID: o.SubjectID,
		State: o.effectiveState(s.now()),
	}
	if o.ExpiresAt != nil {
		v := o.ExpiresAt.UTC().Format(time.RFC3339Nano)
		out.ExpiresAt = &v
	}
	return out
}

// answer builds the decision for an operation. A released operation, and a
// reserved one past its expiry, is not allowed.
func (s *Service) answer(v view, o Op) Decision {
	d := Decision{Managed: v.managed, Operation: s.operationJSON(o)}
	if v.managed {
		c := v.cap
		d.Capacity = &c
	}
	if d.Operation.State == StateReleased {
		d.Reason = ReasonReleased
		return d
	}
	d.Allowed = true
	return d
}

// refuse is a decision with no operation. A workspace that is not managed has
// nothing to refuse: the caller takes its unmanaged path.
func refuse(v view, reason string) Decision {
	if !v.managed {
		return Decision{}
	}
	return Decision{Managed: true, Reason: reason, Capacity: capacityOf(v)}
}

// Reserve holds a seat for an invitation until expiresAt (rule 5).
func (s *Service) Reserve(ctx context.Context, ws, token, subject string, expiresAt time.Time) (Decision, error) {
	return s.admit(ctx, ws, Op{
		WorkspaceID: ws, Token: token, Kind: KindInvitation, SubjectID: subject,
		State: StateReserved, ExpiresAt: &expiresAt,
	})
}

// Claim holds a seat for a share-link join that is in flight. It has no expiry:
// the recovery worker retires an abandoned claim with Release.
func (s *Service) Claim(ctx context.Context, ws, token, subject string) (Decision, error) {
	return s.admit(ctx, ws, Op{
		WorkspaceID: ws, Token: token, Kind: KindShareJoin, SubjectID: subject, State: StateConsuming,
	})
}

// admit inserts op when the workspace has a free seat. A token that already
// exists returns its own state and reserves nothing (rule 1).
func (s *Service) admit(ctx context.Context, ws string, op Op) (Decision, error) {
	return s.run(ctx, ws, func(tx Tx, v view) (Decision, error) {
		if !v.managed {
			return Decision{}, nil
		}
		if existing, found, err := tx.Operation(ctx, ws, op.Token); err != nil {
			return Decision{}, err
		} else if found {
			return s.answer(v, existing), nil
		}
		c := v.cap
		if c.UsedSeats > c.PurchasedSeats {
			return Decision{}, ErrOvercommitted
		}
		if c.PurchasedSeats-c.UsedSeats-c.ReservedSeats <= 0 {
			return refuse(v, ReasonCapacityFull), nil
		}
		inserted, ok, err := tx.Insert(ctx, op)
		if err != nil {
			return Decision{}, err
		}
		if !ok {
			// The workspace lock makes this unreachable unless a row was written
			// outside it; fail rather than guess.
			return Decision{}, errors.New("seat operation appeared under the workspace lock")
		}
		v.cap.ReservedSeats++
		return s.answer(v, inserted), nil
	})
}

// Consume moves a reserved seat to consuming when the invitation is accepted.
// It is allowed only while the held seat still exists.
func (s *Service) Consume(ctx context.Context, ws, token string) (Decision, error) {
	return s.run(ctx, ws, func(tx Tx, v view) (Decision, error) {
		op, found, err := tx.Operation(ctx, ws, token)
		if err != nil {
			return Decision{}, err
		}
		if !found {
			return refuse(v, ReasonReleased), nil
		}
		if op.effectiveState(s.now()) == StateReserved {
			// The accept is in flight: the seat no longer expires with the invitation.
			if op, err = tx.SetState(ctx, ws, token, StateConsuming, "", nil); err != nil {
				return Decision{}, err
			}
		}
		return s.answer(v, op), nil
	})
}

// Confirm records that the member exists. The member table already counts the
// seat, so the operation leaves the held count; the call succeeds whatever the
// operation's state, because the product transaction has committed.
func (s *Service) Confirm(ctx context.Context, ws, token string) (Decision, error) {
	return s.run(ctx, ws, func(tx Tx, v view) (Decision, error) {
		op, found, err := tx.Operation(ctx, ws, token)
		if err != nil {
			return Decision{}, err
		}
		if !found {
			return Decision{Managed: v.managed, Allowed: true, Capacity: capacityOf(v)}, nil
		}
		if op.State == StateReserved || op.State == StateConsuming {
			if op, err = tx.SetState(ctx, ws, token, StateUsed, "", nil); err != nil {
				return Decision{}, err
			}
			if v.managed && v.cap.ReservedSeats > 0 {
				v.cap.ReservedSeats--
			}
		}
		d := s.answer(v, op)
		d.Allowed, d.Reason = true, ""
		return d, nil
	})
}

// Release frees the seat of a token. A missing or already released operation
// is not allowed with reason "released" and is not an error (rule 2).
func (s *Service) Release(ctx context.Context, ws, token string) (Decision, error) {
	return s.run(ctx, ws, func(tx Tx, v view) (Decision, error) {
		op, found, err := tx.Operation(ctx, ws, token)
		if err != nil {
			return Decision{}, err
		}
		if !found || op.effectiveState(s.now()) == StateReleased {
			if found {
				return s.answer(v, op), nil
			}
			return refuse(v, ReasonReleased), nil
		}
		wasHeld := op.State == StateReserved || op.State == StateConsuming
		if op, err = tx.SetState(ctx, ws, token, StateReleased, "", nil); err != nil {
			return Decision{}, err
		}
		if wasHeld && v.managed && v.cap.ReservedSeats > 0 {
			v.cap.ReservedSeats--
		}
		d := Decision{Managed: v.managed, Allowed: true, Operation: s.operationJSON(op), Capacity: capacityOf(v)}
		return d, nil
	})
}

// ReleaseMember frees the seat of a removed member. The member table is the
// count of used seats, so the removal already freed it; there is no row to
// change, and the answer is the same every time (rule 2).
func (s *Service) ReleaseMember(ctx context.Context, ws string) (Decision, error) {
	return s.run(ctx, ws, func(_ Tx, v view) (Decision, error) {
		return refuse(v, ReasonReleased), nil
	})
}

// Get returns the current state of an operation, or ErrNotFound.
func (s *Service) Get(ctx context.Context, ws, token string) (Decision, error) {
	return s.run(ctx, ws, func(tx Tx, v view) (Decision, error) {
		op, found, err := tx.Operation(ctx, ws, token)
		if err != nil {
			return Decision{}, err
		}
		if !found {
			if !v.managed {
				return Decision{}, nil
			}
			return Decision{}, ErrNotFound
		}
		d := s.answer(v, op)
		// A read reports the state; it does not grant anything.
		d.Allowed, d.Reason = false, ""
		return d, nil
	})
}

func capacityOf(v view) *Capacity {
	if !v.managed {
		return nil
	}
	c := v.cap
	return &c
}
