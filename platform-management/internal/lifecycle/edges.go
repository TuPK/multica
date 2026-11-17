package lifecycle

import (
	"time"

	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
)

// Edge is one step of the state diagram.
type Edge string

const (
	// EdgeTrialEnded: trialing to free when trial_end has passed.
	EdgeTrialEnded Edge = "trial_ended"
	// EdgeUsageSwitch: active on a seat plan to active on a usage plan when the
	// paid period is over and pending_change is that usage plan (spec 7b.1,
	// rule 7). There is no renewal order: a usage plan has no seats to renew.
	EdgeUsageSwitch Edge = "usage_switch"
	// EdgeCanceledLapsed: active with cancel_at_period_end to free when the period is over.
	EdgeCanceledLapsed Edge = "canceled_lapsed"
	// EdgePeriodLapsed: active to past_due when the period is over.
	EdgePeriodLapsed Edge = "period_lapsed"
	// EdgeUsageUnpaid: active to past_due when a usage invoice has stayed unpaid
	// for PLATFORM_USAGE_DUE_DAYS (spec 7b.3, rule 13).
	EdgeUsageUnpaid Edge = "usage_unpaid"
	// EdgeGraceEnded: past_due to read_only when grace_until has passed.
	EdgeGraceEnded Edge = "grace_ended"
	// EdgeReadOnlyEnded: read_only to free thirty days after grace_until.
	EdgeReadOnlyEnded Edge = "read_only_ended"
)

// Edges lists the edges in the order a run takes them. A subscription that is
// overdue by more than one step, after a stop of the job, moves through all of
// them in one run. EdgeUsageSwitch comes before EdgePeriodLapsed, so a period
// that ends with a usage plan pending never lapses.
var Edges = []Edge{EdgeTrialEnded, EdgeUsageSwitch, EdgeCanceledLapsed, EdgePeriodLapsed, EdgeUsageUnpaid, EdgeGraceEnded, EdgeReadOnlyEnded}

// From is the status the edge leaves.
func (e Edge) From() string {
	switch e {
	case EdgeTrialEnded:
		return StatusTrialing
	case EdgeUsageSwitch, EdgeCanceledLapsed, EdgePeriodLapsed, EdgeUsageUnpaid:
		return subscription.StatusActive
	case EdgeGraceEnded:
		return StatusPastDue
	case EdgeReadOnlyEnded:
		return StatusReadOnly
	}
	return ""
}

// To is the status the edge enters.
func (e Edge) To() string {
	switch e {
	case EdgeTrialEnded, EdgeCanceledLapsed, EdgeReadOnlyEnded:
		return StatusFree
	case EdgeUsageSwitch:
		return subscription.StatusActive
	case EdgePeriodLapsed, EdgeUsageUnpaid:
		return StatusPastDue
	case EdgeGraceEnded:
		return StatusReadOnly
	}
	return ""
}

// reached reports whether now is at or after t. Every edge fires at its
// boundary, not after it.
func reached(now time.Time, t *time.Time) bool { return t != nil && !now.Before(*t) }

// Due reports whether s takes the edge at now.
func (e Edge) Due(s subscription.Subscription, now time.Time) bool {
	if e.From() == "" || s.Status != e.From() {
		return false
	}
	switch e {
	case EdgeTrialEnded:
		return reached(now, s.TrialEnd)
	case EdgeUsageSwitch:
		// A usage plan has no seats, and a seat plan always has at least one, so
		// a pending change with none is the usage plan. The store checks the plan.
		return !s.CancelAtPeriodEnd && s.PendingChange != nil && s.PendingChange.Seats == 0 && reached(now, s.CurrentPeriodEnd)
	case EdgeCanceledLapsed:
		return s.CancelAtPeriodEnd && reached(now, s.CurrentPeriodEnd)
	case EdgePeriodLapsed:
		return !s.CancelAtPeriodEnd && reached(now, s.CurrentPeriodEnd)
	case EdgeUsageUnpaid:
		// The store has set GraceUntil to the end of the grace the debt earns;
		// it lists only subscriptions whose invoice is past its due day.
		return s.GraceUntil != nil
	case EdgeGraceEnded:
		return reached(now, s.GraceUntil)
	case EdgeReadOnlyEnded:
		// The read-only period is counted from grace_until, which read_only keeps.
		if s.GraceUntil == nil {
			return false
		}
		end := s.GraceUntil.Add(ReadOnlyPeriod)
		return reached(now, &end)
	}
	return false
}

// Apply returns s as the edge leaves it, before the version moves.
//
// The free subscription is the free plan with no seats, no period, no grace
// and no cancel flag. trial_end stays: it is what tells that the workspace
// has used its one trial. limit_overrides stay, because staff set them.
func (e Edge) Apply(s subscription.Subscription) subscription.Subscription {
	s.Status = e.To()
	switch e {
	case EdgeUsageSwitch:
		// The usage period starts when the paid seat period ends and runs to the
		// end of that UTC month (spec 7b.3, rule 6). Rating starts at its first day.
		start := *s.CurrentPeriodEnd
		_, end := subscription.UsagePeriod(start)
		s.PlanKey, s.Seats, s.Interval = s.PendingChange.PlanKey, 0, subscription.IntervalMonth
		s.CurrentPeriodStart, s.CurrentPeriodEnd, s.GraceUntil = &start, &end, nil
		s.PendingChange, s.UnitPrice = nil, nil
	case EdgePeriodLapsed:
		s.GraceUntil = nil
		if s.CurrentPeriodEnd != nil {
			g := s.CurrentPeriodEnd.Add(GracePeriod)
			s.GraceUntil = &g
		}
	case EdgeUsageUnpaid:
		// grace_until is the one the store computed.
	case EdgeGraceEnded:
		// grace_until stays: the read-only period is counted from it.
	default:
		s.PlanKey = plan.FreeKey
		s.Seats = 0
		s.CurrentPeriodStart, s.CurrentPeriodEnd, s.GraceUntil = nil, nil, nil
		s.CancelAtPeriodEnd = false
	}
	return s
}
