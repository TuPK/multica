package subscription

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/trust"
)

const (
	maxReasonChars = 1000
	maxSeats       = 1_000_000

	targetWorkspace = "workspace"
)

// Error is a refusal the caller sees: an HTTP status, a stable code and a
// message. Anything else a Service returns is an internal failure.
type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func badRequest(code, msg string) *Error {
	return &Error{Status: http.StatusBadRequest, Code: code, Message: msg}
}

// NotFound is the refusal for an unknown workspace, plan or route.
func NotFound() *Error {
	return &Error{Status: http.StatusNotFound, Code: "not_found", Message: "not found"}
}

// Change is a staff member's replacement of a workspace's subscription fields.
// Every field is set by the caller: an omitted time is none, not "unchanged".
type Change struct {
	WorkspaceID        string
	PlanKey            string
	Seats              int
	Interval           string
	CurrentPeriodStart *time.Time
	CurrentPeriodEnd   *time.Time
	TrialEnd           *time.Time
	GraceUntil         *time.Time
	CancelAtPeriodEnd  bool
	LimitOverrides     plan.Limits
	Reason             string

	ActorID    string
	ActorLabel string
}

// Result is the row after a Change. Changed is false when the request matched
// what was already there: nothing was written.
type Result struct {
	Subscription Subscription
	Changed      bool
}

// Service applies staff changes to subscriptions.
type Service struct {
	runner Runner
	cache  Invalidator
}

// NewService returns a Service. cache is told about each committed change.
func NewService(r Runner, cache Invalidator) *Service { return &Service{runner: r, cache: cache} }

func utcMicro(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := t.UTC().Truncate(time.Microsecond)
	return &v
}

// normalize checks the fields that need no database and returns the change with
// its times in UTC at the precision Postgres keeps.
func (c Change) normalize() (Change, *Error) {
	c.Reason = strings.TrimSpace(c.Reason)
	if c.Reason == "" {
		return c, badRequest("reason_required", "a reason is required")
	}
	if strings.ContainsRune(c.Reason, 0) {
		return c, badRequest("invalid_body", "reason contains a NUL character")
	}
	if utf8.RuneCountInString(c.Reason) > maxReasonChars {
		return c, badRequest("reason_too_long", fmt.Sprintf("reason is longer than %d characters", maxReasonChars))
	}
	id, ok := trust.CanonicalUUID(c.WorkspaceID)
	if !ok {
		return c, badRequest("invalid_subscription", "workspace id is not a UUID")
	}
	c.WorkspaceID = id
	if err := plan.ValidateKey(c.PlanKey); err != nil {
		return c, badRequest("invalid_subscription", "plan_key: "+err.Error())
	}
	if c.Seats < 0 || c.Seats > maxSeats {
		return c, badRequest("invalid_subscription", "seats must be from 0 to 1000000")
	}
	if c.Interval != IntervalMonth && c.Interval != IntervalYear {
		return c, badRequest("invalid_subscription", "interval must be month or year")
	}
	c.CurrentPeriodStart, c.CurrentPeriodEnd = utcMicro(c.CurrentPeriodStart), utcMicro(c.CurrentPeriodEnd)
	c.TrialEnd, c.GraceUntil = utcMicro(c.TrialEnd), utcMicro(c.GraceUntil)
	if c.CurrentPeriodStart != nil && c.CurrentPeriodEnd != nil && !c.CurrentPeriodStart.Before(*c.CurrentPeriodEnd) {
		return c, badRequest("invalid_subscription", "current_period_start must be before current_period_end")
	}
	if err := plan.ValidateLimits(c.LimitOverrides); err != nil {
		return c, badRequest("invalid_subscription", "limit_overrides: "+err.Error())
	}
	if c.LimitOverrides == nil {
		c.LimitOverrides = plan.Limits{}
	}
	return c, nil
}

// baseline is what a workspace with no row is: the free plan, active, version 0.
func baseline(workspaceID string) Subscription {
	return Subscription{
		WorkspaceID: workspaceID, PlanKey: plan.FreeKey, Status: StatusActive,
		Interval: IntervalMonth, LimitOverrides: plan.Limits{},
	}
}

// Apply replaces the workspace's subscription fields with c. In one transaction
// it writes the row, moves its version up by one and writes the audit row; it
// clears the policy cache entry after the commit. A request that changes
// nothing writes nothing.
func (s *Service) Apply(ctx context.Context, c Change) (Result, error) {
	c, perr := c.normalize()
	if perr != nil {
		return Result{}, perr
	}

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
		current, exists, err := tx.Get(ctx, c.WorkspaceID)
		if err != nil {
			return err
		}
		p, found, err := tx.Plan(ctx, c.PlanKey)
		if err != nil {
			return err
		}
		if !found {
			return badRequest("unknown_plan", "no plan has this key")
		}
		// Subscribers stay on a plan that was archived; nobody new joins it.
		if p.ArchivedAt != nil && !(exists && current.PlanKey == c.PlanKey) {
			return badRequest("plan_archived", "the plan is archived and takes no new subscribers")
		}
		if p.MinSeats != nil && c.Seats < *p.MinSeats {
			return badRequest("seats_below_minimum", fmt.Sprintf("the plan needs at least %d seats", *p.MinSeats))
		}

		before := current
		if !exists {
			before = baseline(c.WorkspaceID)
		}
		next := before
		next.PlanKey, next.Seats, next.Interval = c.PlanKey, c.Seats, c.Interval
		next.CurrentPeriodStart, next.CurrentPeriodEnd, next.TrialEnd, next.GraceUntil =
			c.CurrentPeriodStart, c.CurrentPeriodEnd, c.TrialEnd, c.GraceUntil
		next.CancelAtPeriodEnd, next.LimitOverrides = c.CancelAtPeriodEnd, c.LimitOverrides

		changed := diff(before, next)
		if len(changed) == 0 {
			res = Result{Subscription: before}
			return nil
		}

		var written Subscription
		if exists {
			written, err = tx.Update(ctx, next)
			if err != nil {
				return err
			}
		} else {
			var ok bool
			written, ok, err = tx.Insert(ctx, next)
			if err != nil {
				return err
			}
			if !ok {
				return &Error{Status: http.StatusConflict, Code: "concurrent_change",
					Message: "the subscription was changed by someone else; read it again and retry"}
			}
		}

		if err := audit.Record(ctx, tx, audit.Entry{
			WorkspaceID: c.WorkspaceID,
			ActorType:   audit.ActorStaff,
			ActorID:     c.ActorID,
			ActorLabel:  c.ActorLabel,
			Action:      audit.ActionSubscriptionChanged,
			TargetType:  targetWorkspace,
			TargetID:    c.WorkspaceID,
			TargetLabel: name,
			Detail: map[string]any{
				"reason":  c.Reason,
				"version": written.Version,
				"created": !exists,
				"changes": changed,
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

// diff returns {field: {from, to}} for each changeable field that differs. The
// names are the audit detail's: the plan is "plan", not "plan_key", because
// audit.Redact hides the value of any key that contains "key".
func diff(a, b Subscription) map[string]any {
	out := map[string]any{}
	set := func(name string, from, to any) { out[name] = map[string]any{"from": from, "to": to} }

	if a.PlanKey != b.PlanKey {
		set("plan", a.PlanKey, b.PlanKey)
	}
	if a.Seats != b.Seats {
		set("seats", a.Seats, b.Seats)
	}
	if a.Interval != b.Interval {
		set("interval", a.Interval, b.Interval)
	}
	for _, f := range []struct {
		name     string
		from, to *time.Time
	}{
		{"current_period_start", a.CurrentPeriodStart, b.CurrentPeriodStart},
		{"current_period_end", a.CurrentPeriodEnd, b.CurrentPeriodEnd},
		{"trial_end", a.TrialEnd, b.TrialEnd},
		{"grace_until", a.GraceUntil, b.GraceUntil},
	} {
		if !sameInstant(f.from, f.to) {
			set(f.name, timeText(f.from), timeText(f.to))
		}
	}
	if a.CancelAtPeriodEnd != b.CancelAtPeriodEnd {
		set("cancel_at_period_end", a.CancelAtPeriodEnd, b.CancelAtPeriodEnd)
	}
	if !reflect.DeepEqual(orEmpty(a.LimitOverrides), orEmpty(b.LimitOverrides)) {
		set("limit_overrides", limitsText(a.LimitOverrides), limitsText(b.LimitOverrides))
	}
	return out
}

func orEmpty(l plan.Limits) plan.Limits {
	if l == nil {
		return plan.Limits{}
	}
	return l
}

func sameInstant(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

func timeText(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// limitsText is the audit form of limits: a null stays a null, because it is an
// override that lifts the plan's limit.
func limitsText(l plan.Limits) map[string]any {
	out := make(map[string]any, len(l))
	for gate, v := range l {
		if v == nil {
			out[gate] = nil
			continue
		}
		out[gate] = *v
	}
	return out
}
