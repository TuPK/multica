package deletionrequest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/multica-ai/multica/platform-management/accountguard"
	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/accountcache"
)

// Account deletion, phase 1 (spec R6 section 7). A system_admin asks for a user's
// account to be deleted now. The platform checks the rules of section 7.3, writes
// the account request with its audit row and the pending_deletion status in one
// transaction, and requests the immediate deletion of the workspaces the staff
// member chose to delete. Multica's staff_deletion job (phase 2) does the rest.

// Resolution actions of spec R6 section 7.3.
const (
	ActionTransfer = "transfer"
	ActionDelete   = "delete"
)

// Audit target type of an account.
const targetUser = "user"

// accountStatusDeleted is the platform status of a deleted account; such an account is not found.
const accountStatusDeleted = "deleted"

// AccountUser is the target row of an account request.
type AccountUser struct {
	ID     string
	Email  string
	Status string
	// Staff is true when the user has an active platform_staff row.
	Staff bool
}

// AccountWorkspace is a workspace the account belongs to, with the counts the
// sole-owner rule needs.
type AccountWorkspace struct {
	ID          string `json:"workspace_id"`
	Slug        string `json:"slug"`
	Role        string `json:"role"`
	OwnerCount  int    `json:"owner_count"`
	MemberCount int    `json:"member_count"`
}

// NeedChoice reports whether the account is the only owner of a workspace that
// has other members: such a workspace needs a choice (spec R6 section 7.3).
func (w AccountWorkspace) NeedChoice() bool {
	return w.Role == "owner" && w.OwnerCount == 1 && w.MemberCount > 1
}

// Resolution is the choice for one workspace that needs one. SuccessorUserID is
// set for a transfer only.
type Resolution struct {
	WorkspaceID     string `json:"workspace_id"`
	Action          string `json:"action"`
	SuccessorUserID string `json:"successor_user_id,omitempty"`
}

// AccountInput is what staff submit to delete an account now.
type AccountInput struct {
	Reason      string       `json:"reason"`
	Confirm     string       `json:"confirm"`
	Resolutions []Resolution `json:"resolutions"`
}

// AccountPreview is what the console shows before the form is sent (spec R6 section 7.5).
type AccountPreview struct {
	Email string `json:"email"`
	// NeedChoice lists the workspaces the staff member must resolve.
	NeedChoice []AccountWorkspace `json:"need_choice"`
	// DeletedWith lists the workspaces that go with the account because it is their only member.
	DeletedWith []AccountWorkspace `json:"deleted_with"`
}

// EmailMatches reports whether the typed email equals the account's email. The
// match ignores case and surrounding spaces (spec R6 GĐ10).
func EmailMatches(typed, expected string) bool {
	return strings.EqualFold(strings.TrimSpace(typed), strings.TrimSpace(expected))
}

// successorRole reports whether a workspace role may receive ownership (spec R6 GĐ11).
func successorRole(role string) bool { return role == "admin" || role == "member" }

// checkResolutions applies the input rules of spec R6 section 7.3 against the
// workspaces that need a choice. An unknown, duplicated or invalid choice is a
// 400; a missing choice is a 409 that lists the workspaces. successorOK reports
// whether a user may take ownership of a workspace.
func checkResolutions(required []AccountWorkspace, res []Resolution, successorOK func(workspaceID, userID string) bool) error {
	need := make(map[string]bool, len(required))
	for _, w := range required {
		need[w.ID] = true
	}
	seen := make(map[string]bool, len(res))
	for _, r := range res {
		if !need[r.WorkspaceID] || seen[r.WorkspaceID] {
			return &Error{Status: 400, Code: CodeSuccessorInvalid, Message: "a choice must name one workspace that needs one",
				Extra: map[string]any{"workspace_id": r.WorkspaceID}}
		}
		seen[r.WorkspaceID] = true
		switch r.Action {
		case ActionTransfer:
			if r.SuccessorUserID == "" || !successorOK(r.WorkspaceID, r.SuccessorUserID) {
				return &Error{Status: 400, Code: CodeSuccessorInvalid, Message: "the successor must be an active admin or member of the workspace",
					Extra: map[string]any{"workspace_id": r.WorkspaceID}}
			}
		case ActionDelete:
			if r.SuccessorUserID != "" {
				return &Error{Status: 400, Code: CodeResolutionInvalid, Message: "a delete choice takes no successor",
					Extra: map[string]any{"workspace_id": r.WorkspaceID}}
			}
		default:
			return &Error{Status: 400, Code: CodeResolutionInvalid, Message: "action must be transfer or delete",
				Extra: map[string]any{"workspace_id": r.WorkspaceID}}
		}
	}
	var missing []string
	for _, w := range required {
		if !seen[w.ID] {
			missing = append(missing, w.ID)
		}
	}
	if len(missing) > 0 {
		return &Error{Status: 409, Code: CodeSoleOwnerUnresolved, Message: "the account is the only owner of a workspace with other members; choose a successor or delete the workspace",
			Extra: map[string]any{"workspace_ids": missing}}
	}
	return nil
}

// refuseAccount applies the checks that come before any write: the account must
// exist, the staff member may not delete themself, and the target must not be an
// active staff member (spec R6 section 7.2, rule 3).
func refuseAccount(by Actor, acc AccountUser) error {
	if acc.ID == by.ID {
		return &Error{Status: 409, Code: CodeCannotDeleteSelf, Message: "a staff member cannot delete their own account"}
	}
	if acc.Staff {
		return &Error{Status: 409, Code: CodeUserStaff, Message: "an active staff member cannot be deleted now"}
	}
	return nil
}

// needChoiceOf returns the workspaces of wss that need a choice.
func needChoiceOf(wss []AccountWorkspace) []AccountWorkspace {
	var out []AccountWorkspace
	for _, w := range wss {
		if w.NeedChoice() {
			out = append(out, w)
		}
	}
	return out
}

// PreviewAccount returns what a deletion of the account would need, and writes nothing.
func (s *Service) PreviewAccount(ctx context.Context, by Actor, userID string) (AccountPreview, error) {
	var out AccountPreview
	err := s.Runner.InTx(ctx, func(tx Tx) error {
		out = AccountPreview{}
		acc, found, err := tx.AccountUser(ctx, userID)
		if err != nil {
			return err
		}
		if !found {
			return ErrNotFound
		}
		if err := refuseAccount(by, acc); err != nil {
			return err
		}
		wss, err := tx.AccountWorkspaces(ctx, acc.ID)
		if err != nil {
			return err
		}
		out.Email = acc.Email
		out.NeedChoice = needChoiceOf(wss)
		for _, w := range wss {
			if w.MemberCount == 1 {
				out.DeletedWith = append(out.DeletedWith, w)
			}
		}
		return nil
	})
	if err != nil {
		return AccountPreview{}, err
	}
	return out, nil
}

// RequestAccount asks for a user's account to be deleted now. It returns the
// live request and created == false when one exists. It returns ErrNotFound for
// an unknown or deleted user, and the rejections of spec R6 section 7.5 otherwise.
func (s *Service) RequestAccount(ctx context.Context, by Actor, userID string, in AccountInput) (Request, bool, error) {
	reason, rerr := CleanReason(in.Reason)
	if rerr != nil {
		return Request{}, false, rerr
	}
	var out Request
	created := false
	var changed []string
	err := s.Runner.InTx(ctx, func(tx Tx) error {
		out, created, changed = Request{}, false, nil
		acc, found, err := tx.AccountUser(ctx, userID)
		if err != nil {
			return err
		}
		if !found {
			return ErrNotFound
		}
		if err := refuseAccount(by, acc); err != nil {
			return err
		}
		if !EmailMatches(in.Confirm, acc.Email) {
			return &Error{Status: 400, Code: CodeConfirmationMismatch, Message: "the confirmation does not match the account email"}
		}
		if live, ok, err := tx.Live(ctx, KindAccount, acc.ID); err != nil {
			return err
		} else if ok {
			out = live
			return nil
		}
		wss, err := tx.AccountWorkspaces(ctx, acc.ID)
		if err != nil {
			return err
		}
		required := needChoiceOf(wss)
		// The successor check reads the member row of each transfer choice; the
		// result is collected first because checkResolutions takes no error.
		okSuccessor := map[string]bool{}
		for _, r := range in.Resolutions {
			if r.Action != ActionTransfer || r.SuccessorUserID == "" || r.SuccessorUserID == acc.ID {
				continue
			}
			role, status, found, err := tx.WorkspaceMember(ctx, r.WorkspaceID, r.SuccessorUserID)
			if err != nil {
				return err
			}
			okSuccessor[r.WorkspaceID+"/"+r.SuccessorUserID] = found && successorRole(role) && status == "active"
		}
		if err := checkResolutions(required, in.Resolutions, func(wsID, uid string) bool {
			return uid != acc.ID && okSuccessor[wsID+"/"+uid]
		}); err != nil {
			return err
		}
		resJSON, err := json.Marshal(in.Resolutions)
		if err != nil {
			return fmt.Errorf("encode resolutions: %w", err)
		}
		row, inserted, err := tx.Insert(ctx, NewRequest{
			Kind: KindAccount, TargetID: acc.ID, TargetLabel: acc.Email, Reason: reason, Resolutions: resJSON, By: by,
		})
		if err != nil {
			return err
		}
		if !inserted {
			winner, ok, err := tx.Live(ctx, KindAccount, acc.ID)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("deletion request for account %s conflicted but no live request is visible", acc.ID)
			}
			out = winner
			return nil
		}
		out, created = row, true
		if err := audit.Record(ctx, tx, by.entry(audit.Entry{
			Action: audit.ActionAccountImmediateDeletionRequested, TargetType: targetUser, TargetID: acc.ID, TargetLabel: acc.Email,
			Detail: map[string]any{"reason": reason, "request_id": row.ID, "resolutions": resolutionDetail(in.Resolutions)},
		})); err != nil {
			return err
		}
		for _, r := range in.Resolutions {
			if r.Action != ActionDelete {
				continue
			}
			moved, err := s.requestChildWorkspace(ctx, tx, by, r.WorkspaceID, row, reason)
			if err != nil {
				return err
			}
			if moved {
				changed = append(changed, r.WorkspaceID)
			}
		}
		return tx.ScheduleAccount(ctx, acc.ID, reason, by, s.Now().UTC())
	})
	if err != nil {
		return Request{}, false, err
	}
	s.invalidate(ctx, changed)
	if created {
		s.notifyAccount(ctx, userID)
	}
	return out, created, nil
}

// requestChildWorkspace writes the workspace request of a delete choice (spec R6
// section 7.4, phase 1) as a child of the account request, and moves the
// workspace to pending_deletion. A live request of its own makes it a 409.
func (s *Service) requestChildWorkspace(ctx context.Context, tx Tx, by Actor, workspaceID string, parent Request, reason string) (bool, error) {
	ws, found, err := tx.Workspace(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	if !found {
		return false, &Error{Status: 400, Code: CodeSuccessorInvalid, Message: "a choice must name one workspace that needs one",
			Extra: map[string]any{"workspace_id": workspaceID}}
	}
	if _, live, err := tx.Live(ctx, KindWorkspace, ws.ID); err != nil {
		return false, err
	} else if live {
		return false, &Error{Status: 409, Code: CodeWorkspaceRequestLive, Message: "the workspace already has a live deletion request",
			Extra: map[string]any{"workspace_id": ws.ID}}
	}
	child, inserted, err := tx.Insert(ctx, NewRequest{
		Kind: KindWorkspace, TargetID: ws.ID, TargetLabel: ws.Slug, Reason: reason, TenantID: ws.TenantID, ParentID: parent.ID, By: by,
	})
	if err != nil {
		return false, err
	}
	if !inserted {
		return false, &Error{Status: 409, Code: CodeWorkspaceRequestLive, Message: "the workspace already has a live deletion request",
			Extra: map[string]any{"workspace_id": ws.ID}}
	}
	return s.setPending(ctx, tx, by, ws, child, reason)
}

// resolutionDetail is the audit form of the choices: ids and actions, no emails.
func resolutionDetail(res []Resolution) []map[string]string {
	out := make([]map[string]string, 0, len(res))
	for _, r := range res {
		item := map[string]string{"workspace_id": r.WorkspaceID, "action": r.Action}
		if r.SuccessorUserID != "" {
			item["successor_user_id"] = r.SuccessorUserID
		}
		out = append(out, item)
	}
	return out
}

// notifyAccount drops the account's cached state and tells the Multica server
// the account changed, after the commit. A failure is logged; the cache entry
// expires within its TTL.
func (s *Service) notifyAccount(ctx context.Context, userID string) {
	if s.Accounts == nil {
		return
	}
	if err := s.Accounts.Invalidate(ctx, userID); err != nil {
		s.Log.Warn("deletionrequest: account cache key not deleted; the cached state stays until it expires",
			"user_id", userID, "ttl", accountguard.TTL, "error", err)
	}
	if err := s.Accounts.Publish(ctx, userID, accountcache.KindStatus); err != nil {
		s.Log.Error("deletionrequest: account change not published; open sockets stay until they reconnect",
			"user_id", userID, "error", err)
	}
}
