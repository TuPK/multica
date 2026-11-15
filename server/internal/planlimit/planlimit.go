package planlimit

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/entitlement"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Code is the machine-readable code of a refused creation. Clients translate by
// code, never by message.
const Code = "plan_limit_reached"

// Locker takes the transaction-scoped advisory lock of one quota. It must be a
// store bound to the creating transaction: the lock is released when that
// transaction ends, and the count is only exact if the insert happens before
// then. *db.Queries satisfies it.
type Locker interface {
	LockPlanGate(ctx context.Context, arg db.LockPlanGateParams) error
}

// CountFunc returns how much of the resource the workspace uses now. It runs
// after the lock is held.
type CountFunc func(ctx context.Context) (int64, error)

// Policy is one gate's instruction for one workspace, reduced to what an
// enforcement point needs.
type Policy struct {
	Gate entitlement.GateName
	// Enforced is true only for an `enforce` gate. An absent gate, `off`, and
	// the `observe` that a stale policy degrades to all leave it false, so the
	// creation proceeds.
	Enforced bool
	Limit    int64
}

// Resolve reads the gate from provider. Do it before opening the creating
// transaction: a policy fetch is a network call and must not hold a connection
// or a lock. A nil provider (no MULTICA_CLOUD_URL) is off.
//
// Unlike the issue gate, a limit of 0 is enforced: a plan feature that is not
// included is served as `enforce` with limit 0, and a resource gate of 0 means
// none may be created.
func Resolve(ctx context.Context, provider entitlement.Provider, workspaceID pgtype.UUID, gate entitlement.GateName) Policy {
	off := Policy{Gate: gate}
	if provider == nil || !workspaceID.Valid || workspaceID.Bytes == uuid.Nil {
		return off
	}
	decision := provider.Gate(ctx, uuid.UUID(workspaceID.Bytes), gate)
	if decision.Gate.Action != entitlement.ActionEnforce || decision.Gate.Limit == nil || *decision.Gate.Limit < 0 {
		return off
	}
	return Policy{Gate: gate, Enforced: true, Limit: int64(*decision.Gate.Limit)}
}

// ReachedError reports a creation that the plan's quota refuses.
type ReachedError struct {
	Gate  entitlement.GateName
	Limit int64
	// Used is what the workspace used before this creation.
	Used int64
}

func (e *ReachedError) Error() string {
	return fmt.Sprintf("plan limit reached: %s used %d of %d", e.Gate, e.Used, e.Limit)
}

// Body is the 402 response (spec 10). The numbers are JSON numbers.
type Body struct {
	Error string `json:"error"`
	Code  string `json:"code"`
	Gate  string `json:"gate"`
	Limit int64  `json:"limit"`
	Used  int64  `json:"used"`
}

// Body returns the response to send with 402 Payment Required.
func (e *ReachedError) Body() Body {
	return Body{
		Error: "this workspace has reached its plan limit for " + string(e.Gate),
		Code:  Code,
		Gate:  string(e.Gate),
		Limit: e.Limit,
		Used:  e.Used,
	}
}

// Admit decides whether the workspace may take `requested` more of the
// resource. It returns nil when the gate is not enforced, without a lock or a
// query. Otherwise it takes pg_advisory_xact_lock on (workspace, gate), counts
// with count, and returns *ReachedError when used + requested > limit.
//
// l must be bound to the transaction that creates the resource, and the
// creating statement must run on it before it commits. Two creations at the
// last slot then queue on the lock, and the second counts what the first
// committed.
func (p Policy) Admit(ctx context.Context, l Locker, workspaceID pgtype.UUID, requested int64, count CountFunc) error {
	if !p.Enforced {
		return nil
	}
	if err := l.LockPlanGate(ctx, db.LockPlanGateParams{
		Scope: uuid.UUID(workspaceID.Bytes).String(), Gate: string(p.Gate),
	}); err != nil {
		return fmt.Errorf("lock %s quota: %w", p.Gate, err)
	}
	return p.decide(ctx, requested, count)
}

// Precheck applies the same rule without the lock. A path that knows the size
// of what it will store calls it before storing, so a refused upload stores
// nothing. It is not a reservation: the locked Admit in the creating
// transaction stays the authority.
func (p Policy) Precheck(ctx context.Context, _ pgtype.UUID, requested int64, count CountFunc) error {
	if !p.Enforced {
		return nil
	}
	return p.decide(ctx, requested, count)
}

func (p Policy) decide(ctx context.Context, requested int64, count CountFunc) error {
	used, err := count(ctx)
	if err != nil {
		return fmt.Errorf("count %s usage: %w", p.Gate, err)
	}
	if used+requested > p.Limit {
		return &ReachedError{Gate: p.Gate, Limit: p.Limit, Used: used}
	}
	return nil
}

// AgentCounter, RuntimeCounter and AttachmentCounter are the count queries.
type (
	AgentCounter interface {
		CountPlanLimitAgents(ctx context.Context, workspaceID pgtype.UUID) (int64, error)
	}
	RuntimeCounter interface {
		CountPlanLimitRuntimes(ctx context.Context, workspaceID pgtype.UUID) (int64, error)
	}
	AttachmentCounter interface {
		SumPlanLimitAttachmentBytes(ctx context.Context, workspaceID pgtype.UUID) (int64, error)
	}
)

// CountAgents counts the workspace's agents that are not archived and are user
// agents. System agents (the agent builder's carriers) are not the customer's.
func CountAgents(q AgentCounter, workspaceID pgtype.UUID) CountFunc {
	return func(ctx context.Context) (int64, error) { return q.CountPlanLimitAgents(ctx, workspaceID) }
}

// CountRuntimes counts the workspace's runtimes, including the placeholder a
// failed custom profile leaves: it is a row the workspace shows and deletes.
func CountRuntimes(q RuntimeCounter, workspaceID pgtype.UUID) CountFunc {
	return func(ctx context.Context) (int64, error) { return q.CountPlanLimitRuntimes(ctx, workspaceID) }
}

// CountRuntimesBeforeInsert counts the runtimes without the one the caller's
// transaction has just inserted, so a refusal reports what the workspace used
// before it.
func CountRuntimesBeforeInsert(q RuntimeCounter, workspaceID pgtype.UUID) CountFunc {
	return func(ctx context.Context) (int64, error) {
		n, err := q.CountPlanLimitRuntimes(ctx, workspaceID)
		return n - 1, err
	}
}

// SumAttachmentBytes sums the sizes of the workspace's attachments.
func SumAttachmentBytes(q AttachmentCounter, workspaceID pgtype.UUID) CountFunc {
	return func(ctx context.Context) (int64, error) { return q.SumPlanLimitAttachmentBytes(ctx, workspaceID) }
}

// AdmitEach admits a batch item by item, for a path that must not fail as a
// whole when one item does not fit (a chat message's media: the message
// already exists). It takes the lock and counts once, then walks sizes in
// order: an item that fits is admitted and takes its room, one that does not
// is refused and takes none, so a smaller item after it can still fit. The
// result has one entry per size: nil for admitted, the refusal otherwise.
// A gate that is not enforced admits everything and touches nothing.
func (p Policy) AdmitEach(ctx context.Context, l Locker, workspaceID pgtype.UUID, sizes []int64, count CountFunc) ([]*ReachedError, error) {
	refusals := make([]*ReachedError, len(sizes))
	if !p.Enforced || len(sizes) == 0 {
		return refusals, nil
	}
	if err := l.LockPlanGate(ctx, db.LockPlanGateParams{
		Scope: uuid.UUID(workspaceID.Bytes).String(), Gate: string(p.Gate),
	}); err != nil {
		return nil, fmt.Errorf("lock %s quota: %w", p.Gate, err)
	}
	used, err := count(ctx)
	if err != nil {
		return nil, fmt.Errorf("count %s usage: %w", p.Gate, err)
	}
	for i, size := range sizes {
		if used+size > p.Limit {
			refusals[i] = &ReachedError{Gate: p.Gate, Limit: p.Limit, Used: used}
			continue
		}
		used += size
	}
	return refusals, nil
}
