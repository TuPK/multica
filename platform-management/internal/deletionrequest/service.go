package deletionrequest

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/workspacestatus"
	"github.com/multica-ai/multica/platform-management/workspacestate"
)

// Deps are the Service's dependencies.
type Deps struct {
	Runner Runner
	Reader Reader
	Cache  Invalidator
	// Accounts serves account requests (spec R6 section 7). Nil skips the
	// after-commit notices, as in tests of workspace and tenant requests.
	Accounts AccountNotifier
	// Owners and Mail send the notice of an immediate deletion to each owner of a
	// workspace after the commit (spec R6 section 3c.9, rule 4). Nil skips them.
	Owners OwnerLookup
	Mail   DeletionMailer
	Log    *slog.Logger
	// Now returns the time of a request. Nil uses time.Now.
	Now func() time.Time
}

// Service requests, cancels and retries immediate deletions.
type Service struct{ Deps }

// New builds the Service.
func New(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Service{d}
}

const (
	targetRequest   = "deletion_request"
	targetWorkspace = "workspace"
	targetTenant    = "tenant"
	defaultLimit    = 50
)

// RequestWorkspace asks for a workspace to be deleted now. It returns the live
// request and created == false when one already exists for the workspace. It
// returns ErrNotFound for an unknown workspace and a 400 confirmation_mismatch
// when in.Confirm is not the workspace's slug.
func (s *Service) RequestWorkspace(ctx context.Context, by Actor, workspaceID string, in Input) (Request, bool, error) {
	reason, rerr := CleanReason(in.Reason)
	if rerr != nil {
		return Request{}, false, rerr
	}
	var out Request
	created := false
	var changed []string
	var notices []Workspace
	err := s.Runner.InTx(ctx, func(tx Tx) error {
		out, created, changed = Request{}, false, nil
		ws, found, err := tx.Workspace(ctx, workspaceID)
		if err != nil {
			return err
		}
		if !found {
			return ErrNotFound
		}
		if !ConfirmMatches(in.Confirm, ws.Slug) {
			return &Error{Status: 400, Code: CodeConfirmationMismatch, Message: "the confirmation does not match the workspace slug"}
		}
		if live, ok, err := tx.Live(ctx, KindWorkspace, ws.ID); err != nil {
			return err
		} else if ok {
			out = live
			return nil
		}
		row, inserted, err := tx.Insert(ctx, NewRequest{
			Kind: KindWorkspace, TargetID: ws.ID, TargetLabel: ws.Slug, Reason: reason, TenantID: ws.TenantID,
			ExportFirst: in.ExportFirst, By: by,
		})
		if err != nil {
			return err
		}
		if !inserted {
			winner, ok, err := tx.Live(ctx, KindWorkspace, ws.ID)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("deletion request for workspace %s conflicted but no live request is visible", ws.ID)
			}
			out = winner
			return nil
		}
		out, created = row, true
		moved, err := s.setPending(ctx, tx, by, ws, row, reason)
		if err != nil {
			return err
		}
		if moved {
			changed = append(changed, ws.ID)
		}
		if in.ExportFirst {
			if _, err := tx.InsertExport(ctx, ws.ID, by.ID); err != nil {
				return err
			}
		}
		notices = []Workspace{ws}
		return nil
	})
	if err != nil {
		return Request{}, false, err
	}
	s.invalidate(ctx, changed)
	s.noticeOwners(ctx, notices, out, in.ExportFirst)
	return out, created, nil
}

// RequestTenant asks for a tenant and every workspace in it to be deleted now.
// The tenant's gate lock is held for the whole transaction, so a workspace
// creation either lands in the list or is refused (spec R6 section 6.4).
func (s *Service) RequestTenant(ctx context.Context, by Actor, tenantID string, in Input) (Request, bool, error) {
	reason, rerr := CleanReason(in.Reason)
	if rerr != nil {
		return Request{}, false, rerr
	}
	var out Request
	created := false
	var changed []string
	var notices []Workspace
	err := s.Runner.InTx(ctx, func(tx Tx) error {
		out, created, changed, notices = Request{}, false, nil, nil
		tn, found, err := tx.Tenant(ctx, tenantID)
		if err != nil {
			return err
		}
		if !found {
			return ErrNotFound
		}
		if err := tx.LockTenant(ctx, tn.ID); err != nil {
			return err
		}
		if !ConfirmMatches(in.Confirm, tn.Name) {
			return &Error{Status: 400, Code: CodeConfirmationMismatch, Message: "the confirmation does not match the tenant name"}
		}
		if live, ok, err := tx.Live(ctx, KindTenant, tn.ID); err != nil {
			return err
		} else if ok {
			out = live
			return nil
		}
		row, inserted, err := tx.Insert(ctx, NewRequest{
			Kind: KindTenant, TargetID: tn.ID, TargetLabel: tn.Name, Reason: reason, TenantID: tn.ID,
			ExportFirst: in.ExportFirst, By: by,
		})
		if err != nil {
			return err
		}
		if !inserted {
			winner, ok, err := tx.Live(ctx, KindTenant, tn.ID)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("deletion request for tenant %s conflicted but no live request is visible", tn.ID)
			}
			out = winner
			return nil
		}
		out, created = row, true
		ids, err := tx.TenantWorkspaces(ctx, tn.ID)
		if err != nil {
			return err
		}
		for _, id := range ids {
			ws, ok, err := tx.Workspace(ctx, id)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			moved, err := s.setPending(ctx, tx, by, ws, row, reason)
			if err != nil {
				return err
			}
			if moved {
				changed = append(changed, ws.ID)
			}
			if in.ExportFirst {
				if _, err := tx.InsertExport(ctx, ws.ID, by.ID); err != nil {
					return err
				}
			}
			notices = append(notices, ws)
		}
		return audit.Record(ctx, tx, by.entry(audit.Entry{
			TenantID: tn.ID, Action: audit.ActionTenantImmediateDeletionRequested, TargetType: targetTenant, TargetID: tn.ID, TargetLabel: tn.Name,
			Detail: map[string]any{"reason": reason, "request_id": row.ID, "workspace_ids": ids},
		}))
	})
	if err != nil {
		return Request{}, false, err
	}
	s.invalidate(ctx, changed)
	s.noticeOwners(ctx, notices, out, in.ExportFirst)
	return out, created, nil
}

// noticeOwners sends the scheduled-deletion notice to every owner of each
// workspace, after the commit (spec R6 section 3c.9, rule 4). A failure to send
// is audited as deletion.notice_failed and does not stop the deletion.
func (s *Service) noticeOwners(ctx context.Context, wss []Workspace, req Request, exportFirst bool) {
	if s.Owners == nil || s.Mail == nil {
		return
	}
	at := s.Now().UTC()
	for _, ws := range wss {
		emails, err := s.Owners.OwnerEmails(ctx, ws.ID)
		if err != nil {
			s.Log.Warn("deletionrequest: owners not read; no notice sent", "workspace_id", ws.ID, "error", err)
			continue
		}
		for _, to := range emails {
			action := audit.ActionDeletionNoticeSent
			if err := s.Mail.SendDeletionNotice(ctx, to, ws.Name, at, exportFirst); err != nil {
				action = audit.ActionDeletionNoticeFailed
				s.Log.Warn("deletionrequest: deletion notice not sent", "workspace_id", ws.ID, "error", err)
			}
			_ = s.Runner.InTx(ctx, func(tx Tx) error {
				return audit.Record(ctx, tx, audit.Entry{
					WorkspaceID: ws.ID, TenantID: ws.TenantID, ActorType: audit.ActorSystem,
					Action: action, TargetType: targetWorkspace, TargetID: ws.ID, TargetLabel: ws.Slug,
					Detail: map[string]any{"request_id": req.ID},
				})
			})
		}
	}
}

// setPending moves one workspace to pending_deletion with the date now, and writes
// its request audit row. It reports whether the status row changed.
func (s *Service) setPending(ctx context.Context, tx Tx, by Actor, ws Workspace, row Request, reason string) (bool, error) {
	moved, err := workspacestatus.SetInTx(ctx, tx, workspacestatus.Change{
		WorkspaceID:         ws.ID,
		Status:              workspacestate.PendingDeletion,
		Reason:              ReasonImmediate,
		ChangedBy:           by.ID,
		ActorLabel:          by.Label,
		DeletionScheduledAt: s.Now().UTC(),
	})
	if err != nil {
		return false, err
	}
	return moved, audit.Record(ctx, tx, by.entry(audit.Entry{
		WorkspaceID: ws.ID, TenantID: ws.TenantID, Action: audit.ActionWorkspaceImmediateDeletionRequested,
		TargetType: targetWorkspace, TargetID: ws.ID, TargetLabel: ws.Slug,
		Detail: map[string]any{"reason": reason, "request_id": row.ID},
	}))
}

// Cancel moves a pending or failed request to canceled. Its workspaces return to
// active, through workspacestatus with its audit row. It returns ErrNotFound for
// an unknown id and a 409 deletion_request_not_cancelable for any other state.
func (s *Service) Cancel(ctx context.Context, by Actor, id string) (Request, error) {
	var out Request
	var changed []string
	err := s.Runner.InTx(ctx, func(tx Tx) error {
		out, changed = Request{}, nil
		cur, found, err := tx.Lock(ctx, id)
		if err != nil {
			return err
		}
		if !found {
			return ErrNotFound
		}
		if cur.State != StatePending && cur.State != StateFailed {
			return &Error{Status: 409, Code: CodeNotCancelable, Message: "only a pending or failed request can be canceled", State: cur.State}
		}
		ids := []string{cur.TargetID}
		if cur.Kind == KindTenant {
			if ids, err = tx.TenantWorkspaces(ctx, cur.TargetID); err != nil {
				return err
			}
		}
		for _, wsID := range ids {
			ws, ok, err := tx.Workspace(ctx, wsID)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			moved, err := workspacestatus.SetInTx(ctx, tx, workspacestatus.Change{
				WorkspaceID: ws.ID, Status: workspacestate.Active, Reason: ReasonCanceled,
				ChangedBy: by.ID, ActorLabel: by.Label,
			})
			if err != nil {
				return err
			}
			if moved {
				changed = append(changed, ws.ID)
			}
		}
		if out, err = tx.MarkCanceled(ctx, id); err != nil {
			return err
		}
		return audit.Record(ctx, tx, by.entry(audit.Entry{
			WorkspaceID: workspaceFor(cur), TenantID: cur.TenantID, Action: audit.ActionDeletionRequestCanceled,
			TargetType: targetRequest, TargetID: id, TargetLabel: cur.TargetLabel,
			Detail: map[string]any{"kind": cur.Kind, "from": cur.State},
		}))
	})
	if err != nil {
		return Request{}, err
	}
	s.invalidate(ctx, changed)
	return out, nil
}

// Retry sets a failed request back to pending with attempts = 0. Any other
// state is a 409 deletion_request_not_retryable.
func (s *Service) Retry(ctx context.Context, by Actor, id string) (Request, error) {
	var out Request
	err := s.Runner.InTx(ctx, func(tx Tx) error {
		cur, found, err := tx.Lock(ctx, id)
		if err != nil {
			return err
		}
		if !found {
			return ErrNotFound
		}
		if cur.State != StateFailed {
			return &Error{Status: 409, Code: CodeNotRetryable, Message: "only a failed request can be retried", State: cur.State}
		}
		if out, err = tx.Retry(ctx, id); err != nil {
			return err
		}
		return audit.Record(ctx, tx, by.entry(audit.Entry{
			WorkspaceID: workspaceFor(cur), TenantID: cur.TenantID, Action: audit.ActionDeletionRequestRetried,
			TargetType: targetRequest, TargetID: id, TargetLabel: cur.TargetLabel,
			Detail: map[string]any{"kind": cur.Kind},
		}))
	})
	if err != nil {
		return Request{}, err
	}
	return out, nil
}

// List returns the newest requests that match f.
func (s *Service) List(ctx context.Context, f Filter) ([]Request, error) {
	if f.Limit <= 0 {
		f.Limit = defaultLimit
	}
	return s.Reader.List(ctx, f)
}

// Get returns found == false for an unknown id.
func (s *Service) Get(ctx context.Context, id string) (Request, bool, error) {
	return s.Reader.Get(ctx, id)
}

func (s *Service) invalidate(ctx context.Context, ids []string) {
	if s.Cache == nil {
		return
	}
	for _, id := range ids {
		if err := s.Cache.Invalidate(ctx, id); err != nil {
			s.Log.Warn("deletionrequest: cache key not deleted; the cached state stays until it expires",
				"workspace_id", id, "ttl", workspacestate.TTL, "error", err)
		}
	}
}

// workspaceFor is the workspace an audit row of a request belongs to: the target
// of a workspace request, nothing for a tenant request.
func workspaceFor(r Request) string {
	if r.Kind == KindWorkspace {
		return r.TargetID
	}
	return ""
}

func (a Actor) entry(e audit.Entry) audit.Entry {
	e.ActorType, e.ActorID, e.ActorLabel = audit.ActorStaff, a.ID, a.Label
	return e
}
