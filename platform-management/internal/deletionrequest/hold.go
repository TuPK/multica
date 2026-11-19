package deletionrequest

import (
	"context"
	"errors"

	"github.com/multica-ai/multica/platform-management/audit"
)

// holdError is the 409 that a hold answers, with the hold's id for the caller to
// show to a system_admin (spec R6 section 3c.7, rule 6).
func holdError(holdID string) *Error {
	return &Error{Status: 409, Code: CodeLegalHold, Message: "a legal hold covers this and no deletion can start",
		Extra: map[string]any{"legal_hold_id": holdID}}
}

// checkWorkspaceHold refuses when a live hold covers the workspace or its tenant.
// It runs inside the deletion transaction, before any write.
func checkWorkspaceHold(ctx context.Context, tx Tx, ws Workspace) error {
	id, held, err := tx.ActiveLegalHold(ctx, ws.ID, ws.TenantID)
	if err != nil {
		return err
	}
	if held {
		return holdError(id)
	}
	return nil
}

// isHoldRefusal reports whether err is a hold's refusal.
func isHoldRefusal(err error) (*Error, bool) {
	var de *Error
	if errors.As(err, &de) && de.Code == CodeLegalHold {
		return de, true
	}
	return nil, false
}

// recordBlocked writes deletion.blocked_by_hold in its own transaction: the
// refused request rolled back and wrote nothing (spec R6 section 3c.7, rule 3).
func (s *Service) recordBlocked(ctx context.Context, by Actor, kind, targetID, tenantID, workspaceID string, refusal *Error) {
	holdID, _ := refusal.Extra["legal_hold_id"].(string)
	_ = s.Runner.InTx(ctx, func(tx Tx) error {
		return audit.Record(ctx, tx, by.entry(audit.Entry{
			WorkspaceID: workspaceID, TenantID: tenantID, Action: audit.ActionDeletionBlockedByHold,
			TargetType: kind, TargetID: targetID,
			Detail: map[string]any{"legal_hold_id": holdID, "kind": kind},
		}))
	})
}
