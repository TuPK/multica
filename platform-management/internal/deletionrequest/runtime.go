package deletionrequest

import (
	"context"
	"fmt"

	"github.com/multica-ai/multica/platform-management/audit"
)

// targetRuntime is the audit target type of a runtime.
const targetRuntime = "runtime"

// RequestRuntime asks for a runtime to be unregistered (spec R6 section 3b.5, rule 5).
// The confirmation is the runtime's name. The Multica staff_deletion job runs
// DeleteAgentRuntime; this package deletes nothing. It returns the live request
// and created == false when one exists.
func (s *Service) RequestRuntime(ctx context.Context, by Actor, runtimeID string, in Input) (Request, bool, error) {
	reason, rerr := CleanReason(in.Reason)
	if rerr != nil {
		return Request{}, false, rerr
	}
	var out Request
	created := false
	err := s.Runner.InTx(ctx, func(tx Tx) error {
		out, created = Request{}, false
		rt, found, err := tx.Runtime(ctx, runtimeID)
		if err != nil {
			return err
		}
		if !found {
			return ErrNotFound
		}
		if !ConfirmMatches(in.Confirm, rt.Name) {
			return &Error{Status: 400, Code: CodeConfirmationMismatch, Message: "the confirmation does not match the runtime name"}
		}
		if live, ok, err := tx.Live(ctx, KindRuntime, rt.ID); err != nil {
			return err
		} else if ok {
			out = live
			return nil
		}
		row, inserted, err := tx.Insert(ctx, NewRequest{
			Kind: KindRuntime, TargetID: rt.ID, TargetLabel: rt.Name, Reason: reason, TenantID: rt.TenantID, By: by,
		})
		if err != nil {
			return err
		}
		if !inserted {
			winner, ok, err := tx.Live(ctx, KindRuntime, rt.ID)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("deletion request for runtime %s conflicted but no live request is visible", rt.ID)
			}
			out = winner
			return nil
		}
		out, created = row, true
		return audit.Record(ctx, tx, by.entry(audit.Entry{
			WorkspaceID: rt.WorkspaceID, TenantID: rt.TenantID, Action: audit.ActionRuntimeUnregisterRequested,
			TargetType: targetRuntime, TargetID: rt.ID, TargetLabel: rt.Name,
			Detail: map[string]any{"reason": reason, "request_id": row.ID},
		}))
	})
	if err != nil {
		return Request{}, false, err
	}
	return out, created, nil
}
