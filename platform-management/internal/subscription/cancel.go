package subscription

import (
	"context"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
)

// cancelableStatus is the set of statuses that can be cancelled and resumed
// (spec 7b.1, rule 8).
func cancelableStatus(status string) bool {
	return status == StatusActive || status == StatusTrialing
}

// CanCancel reports whether a subscription in this status may be cancelled.
func CanCancel(s Subscription) bool { return cancelableStatus(s.Status) }

// CanResume reports whether a subscription that has cancel_at_period_end set can
// be resumed at now: its status is one that can be cancelled, and its period
// (or, when trialing, its trial) has not ended.
func CanResume(s Subscription, now time.Time) bool {
	if !s.CancelAtPeriodEnd || !cancelableStatus(s.Status) {
		return false
	}
	end := s.CurrentPeriodEnd
	if s.Status == StatusTrialing {
		end = s.TrialEnd
	}
	return end != nil && now.Before(*end)
}

// SetCancel sets cancel_at_period_end: true to cancel, false to resume (spec
// 7b.1, rule 8). A cancel outside the table is refused. A request that changes
// nothing writes nothing. Cancelling keeps any pending change; the change stays
// unused until the subscription resumes.
func (s *PlanChanges) SetCancel(ctx context.Context, workspaceID string, cancel bool, actor Actor) (Result, error) {
	var res Result
	err := s.runner.InTx(ctx, func(tx Tx) error {
		res = Result{}
		name, found, err := tx.WorkspaceName(ctx, workspaceID)
		if err != nil {
			return err
		}
		if !found {
			return NotFound()
		}
		cur, exists, err := tx.Get(ctx, workspaceID)
		if err != nil {
			return err
		}
		now := s.now().UTC()
		action := audit.ActionSubscriptionCancelRequested
		if cancel {
			if !exists || !CanCancel(cur) {
				return conflict("subscription_not_cancelable", "only an active or trialing subscription can be cancelled")
			}
			if cur.CancelAtPeriodEnd {
				res = Result{Subscription: cur}
				return nil
			}
		} else {
			if !exists || !cur.CancelAtPeriodEnd {
				res = Result{Subscription: cur}
				return nil
			}
			if !CanResume(cur, now) {
				return conflict("subscription_not_resumable", "the subscription cannot be resumed: its period or trial has ended")
			}
			action = audit.ActionSubscriptionCancelWithdrawn
		}
		row := cur
		row.CancelAtPeriodEnd = cancel
		written, err := tx.Update(ctx, row)
		if err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{
			WorkspaceID: workspaceID, ActorType: audit.ActorMember, ActorID: actor.ID, ActorLabel: actor.Label,
			Action: action, TargetType: targetWorkspace, TargetID: workspaceID, TargetLabel: name,
			Detail: map[string]any{"period_end": timeText(cur.CurrentPeriodEnd), "version": written.Version},
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
		s.cache.Invalidate(workspaceID)
	}
	return res, nil
}
