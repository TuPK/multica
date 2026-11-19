package deletionrequest

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
)

// Two-person approval (spec R6 section 3c.6, QT10, D103). With an action named
// in ApprovalActions, its request waits for a second system_admin: the request
// row is written awaiting_approval and nothing else happens. The approval runs
// phase 1 through the same functions, with the stored choices.

// Actions the approval list may name (spec rule 1).
const (
	ActionTenantDelete  = "tenant.delete_now"
	ActionAccountDelete = "account.delete_now"
)

// ApprovalTTL is how long an awaiting request waits before it is canceled (rule 4).
const ApprovalTTL = 24 * time.Hour

// Error codes of spec R6 section 3c.6.
const (
	CodeApproverMustDiffer   = "approver_must_differ"
	CodeApproverUnavailable  = "approver_unavailable"
	CodeApprovalNotPending   = "approval_not_pending"
	stateAwaitingApproval    = "awaiting_approval"
	errorCodeApprovalExpired = "approval_expired"
)

type resumeKey struct{}

// resumeID is the request an approval is running phase 1 for, or "".
func resumeID(ctx context.Context) string {
	id, _ := ctx.Value(resumeKey{}).(string)
	return id
}

func withResume(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, resumeKey{}, id)
}

// needsApproval reports whether the action of a kind waits for a second approver.
func (s *Service) needsApproval(kind string) bool {
	switch kind {
	case KindTenant:
		return s.ApprovalActions[ActionTenantDelete]
	case KindAccount:
		return s.ApprovalActions[ActionAccountDelete]
	}
	return false
}

// requireSecondAdmin refuses a request that no second system_admin could approve (rule 5).
func (s *Service) requireSecondAdmin(ctx context.Context, tx Tx) error {
	n, err := tx.CountSystemAdmins(ctx)
	if err != nil {
		return err
	}
	if n < 2 {
		return &Error{Status: http.StatusConflict, Code: CodeApproverUnavailable,
			Message: "an approval needs two active system administrators"}
	}
	return nil
}

// holdForApproval moves a fresh pending request to awaiting_approval and audits it.
func (s *Service) holdForApproval(ctx context.Context, tx Tx, by Actor, row Request, kind, targetID, tenantID string) (Request, error) {
	awaiting, err := tx.AwaitApproval(ctx, row.ID)
	if err != nil {
		return Request{}, err
	}
	if err := audit.Record(ctx, tx, by.entry(audit.Entry{
		TenantID: tenantID, Action: audit.ActionDeletionApprovalRequested,
		TargetType: kind, TargetID: targetID, TargetLabel: row.TargetLabel,
		Detail: map[string]any{"request_id": row.ID, "kind": kind},
	})); err != nil {
		return Request{}, err
	}
	return awaiting, nil
}

// loadForApproval reads a request awaiting approval and checks the approver may act on it.
func loadForApproval(ctx context.Context, tx Tx, id string, by Actor) (Request, error) {
	cur, found, err := tx.Lock(ctx, id)
	if err != nil {
		return Request{}, err
	}
	if !found {
		return Request{}, ErrNotFound
	}
	if cur.State != stateAwaitingApproval {
		return Request{}, &Error{Status: http.StatusConflict, Code: CodeApprovalNotPending, Message: "the request is not awaiting approval", State: cur.State}
	}
	if cur.RequestedBy == by.ID {
		return Request{}, &Error{Status: http.StatusConflict, Code: CodeApproverMustDiffer, Message: "the approver must differ from the requester"}
	}
	return cur, nil
}

// Approve approves an awaiting request and runs its phase 1 (spec rule 3). If
// phase 1 fails, the request goes back to awaiting_approval.
func (s *Service) Approve(ctx context.Context, by Actor, id string) (Request, error) {
	var cur Request
	err := s.Runner.InTx(ctx, func(tx Tx) error {
		c, err := loadForApproval(ctx, tx, id, by)
		if err != nil {
			return err
		}
		cur = c
		approved, ok, err := tx.ApproveRequest(ctx, id, by.ID)
		if err != nil {
			return err
		}
		if !ok {
			return &Error{Status: http.StatusConflict, Code: CodeApprovalNotPending, Message: "the request is not awaiting approval"}
		}
		cur = approved
		return audit.Record(ctx, tx, by.entry(audit.Entry{
			TenantID: "", Action: audit.ActionDeletionApproved, TargetType: "deletion_request", TargetID: id,
			TargetLabel: cur.TargetLabel, Detail: map[string]any{"kind": cur.Kind, "request_id": id},
		}))
	})
	if err != nil {
		return Request{}, err
	}
	rctx := withResume(ctx, id)
	var out Request
	switch cur.Kind {
	case KindTenant:
		out, _, err = s.RequestTenant(rctx, by, cur.TargetID, Input{Reason: cur.Reason, Confirm: cur.TargetLabel, ExportFirst: cur.ExportFirst})
	case KindAccount:
		var res []Resolution
		if len(cur.Resolutions) > 0 {
			if uerr := json.Unmarshal(cur.Resolutions, &res); uerr != nil {
				err = uerr
			}
		}
		if err == nil {
			out, _, err = s.RequestAccount(rctx, by, cur.TargetID, AccountInput{Reason: cur.Reason, Confirm: cur.TargetLabel, Resolutions: res})
		}
	default:
		err = &Error{Status: http.StatusConflict, Code: CodeApprovalNotPending, Message: "this request kind takes no approval"}
	}
	if err != nil {
		_ = s.Runner.InTx(ctx, func(tx Tx) error { return tx.RevertApproval(ctx, id) })
		return Request{}, err
	}
	return out, nil
}

// Reject cancels an awaiting request (spec rule 3).
func (s *Service) Reject(ctx context.Context, by Actor, id string) (Request, error) {
	var out Request
	err := s.Runner.InTx(ctx, func(tx Tx) error {
		cur, err := loadForApproval(ctx, tx, id, by)
		if err != nil {
			return err
		}
		row, ok, err := tx.RejectApproval(ctx, id)
		if err != nil {
			return err
		}
		if !ok {
			return &Error{Status: http.StatusConflict, Code: CodeApprovalNotPending, Message: "the request is not awaiting approval", State: cur.State}
		}
		out = row
		return audit.Record(ctx, tx, by.entry(audit.Entry{
			Action: audit.ActionDeletionRejected, TargetType: "deletion_request", TargetID: id,
			TargetLabel: cur.TargetLabel, Detail: map[string]any{"kind": cur.Kind, "request_id": id},
		}))
	})
	if err != nil {
		return Request{}, err
	}
	return out, nil
}

// ExpireApprovals cancels every awaiting request older than ApprovalTTL, with an
// audit row each (spec rule 4). It returns how many it canceled. The job runs it
// every minute; a second run finds nothing left to cancel.
func (s *Service) ExpireApprovals(ctx context.Context) (int, error) {
	cutoff := s.Now().Add(-ApprovalTTL)
	var n int
	err := s.Runner.InTx(ctx, func(tx Tx) error {
		n = 0
		rows, err := tx.ExpireApprovals(ctx, cutoff)
		if err != nil {
			return err
		}
		for _, r := range rows {
			n++
			if err := audit.Record(ctx, tx, audit.Entry{
				ActorType: audit.ActorSystem, Action: audit.ActionDeletionApprovalExpired,
				TargetType: "deletion_request", TargetID: r.ID, TargetLabel: r.TargetLabel,
				Detail: map[string]any{"kind": r.Kind, "request_id": r.ID, "error_code": errorCodeApprovalExpired},
			}); err != nil {
				return err
			}
		}
		return nil
	})
	return n, err
}
