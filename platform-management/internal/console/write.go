package console

import (
	"context"
	"net/http"
	"time"

	"github.com/multica-ai/multica/platform-management/accountguard"
	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/accountcache"
	"github.com/multica-ai/multica/platform-management/internal/staff"
	"github.com/multica-ai/multica/platform-management/internal/workspacestatus"
	"github.com/multica-ai/multica/platform-management/workspacestate"
)

const afterChangeTimeout = 5 * time.Second

const (
	targetWorkspace = "workspace"
	targetUser      = "user"
)

type statusBody struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

// setWorkspaceStatus moves a workspace to a state through workspacestatus.Set,
// which writes the platform row and its audit row in one transaction and drops
// the cache key the Multica server reads.
func (h *Handler) setWorkspaceStatus(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionWorkspacesLock)
	if !ok {
		return
	}
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	var body statusBody
	if perr := decodeBody(w, r, &body); perr != nil {
		writeError(w, perr)
		return
	}
	status, perr := parseTenantStatus(body.Status)
	if perr != nil {
		writeError(w, perr)
		return
	}
	reason, perr := cleanReason(body.Reason)
	if perr != nil {
		writeError(w, perr)
		return
	}
	a, err := h.currentActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	// The route admits support for lock and unlock. Scheduling a deletion is not
	// support's (spec 5.2).
	if status == workspacestate.PendingDeletion && staff.Allowed(a.Role, staff.ActionWorkspacesDeletion) == staff.None {
		writeError(w, forbidden())
		return
	}
	if visible, err := h.workspaceVisible(r.Context(), scope, id); err != nil {
		h.fail(w, r, "check workspace scope", err)
		return
	} else if !visible {
		writeError(w, notFound())
		return
	}
	// Set does not check that the workspace exists; without this a typo in the
	// id would create a state row for nothing.
	current, found, err := h.Reader.GetWorkspace(r.Context(), id)
	if err != nil {
		h.fail(w, r, "get tenant", err)
		return
	}
	if !found {
		writeError(w, notFound())
		return
	}
	// Leaving pending_deletion cancels the deletion, which is admin's as well.
	if workspacestate.Status(current.Status) == workspacestate.PendingDeletion && status != workspacestate.PendingDeletion &&
		staff.Allowed(a.Role, staff.ActionWorkspacesDeletion) == staff.None {
		writeError(w, forbidden())
		return
	}
	changed, err := h.Workspaces.Set(r.Context(), workspacestatus.Change{
		WorkspaceID: id, Status: status, Reason: reason, ChangedBy: a.ID, ActorLabel: a.Email,
	})
	if err != nil {
		h.fail(w, r, "set workspace state", err)
		return
	}
	d, found, err := h.Reader.GetWorkspace(r.Context(), id)
	if err != nil {
		h.fail(w, r, "get tenant", err)
		return
	}
	if !found {
		writeError(w, notFound())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"changed": changed, "state": toStateJSON(d)})
}

type noteBody struct {
	Body string `json:"body"`
}

// addNote stores an internal note. The audit row names the note but does not
// carry its text: the workspace's owners read the workspace audit log, and
// notes are never shown to them.
func (h *Handler) addNote(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionNotesWrite)
	if !ok {
		return
	}
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	var in noteBody
	if perr := decodeBody(w, r, &in); perr != nil {
		writeError(w, perr)
		return
	}
	text, perr := cleanNoteBody(in.Body)
	if perr != nil {
		writeError(w, perr)
		return
	}
	a, err := h.currentActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	if visible, err := h.workspaceVisible(r.Context(), scope, id); err != nil {
		h.fail(w, r, "check workspace scope", err)
		return
	} else if !visible {
		writeError(w, notFound())
		return
	}
	var note Note
	err = h.Runner.InTx(r.Context(), func(tx Tx) error {
		name, found, err := tx.WorkspaceName(r.Context(), id)
		if err != nil {
			return err
		}
		if !found {
			return notFound()
		}
		noteID, at, err := tx.InsertNote(r.Context(), id, a.ID, text)
		if err != nil {
			return err
		}
		note = Note{ID: noteID, WorkspaceID: id, AuthorID: a.ID, AuthorEmail: a.Email, Body: text, CreatedAt: at}
		return audit.Record(r.Context(), tx, a.staffEntry(audit.Entry{
			WorkspaceID: id,
			Action:      audit.ActionNoteAdded,
			TargetType:  targetWorkspace,
			TargetID:    id,
			TargetLabel: name,
			Detail:      map[string]any{"note_id": noteID},
		}))
	})
	if err != nil {
		h.fail(w, r, "add note", err)
		return
	}
	writeJSON(w, http.StatusCreated, toNoteJSON(note))
}

// afterAccountChange drops the guard cache and tells the Multica server to
// close the user's sockets. The change is committed by now, so a failure is
// logged and the request still succeeds: the cache entry expires within
// accountguard.TTL, and a socket is closed by the next change notice or the
// server's periodic status recheck.
func (h *Handler) afterAccountChange(ctx context.Context, userID, kind string) {
	// The change is committed; a client that hangs up now must not stop the
	// notice.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), afterChangeTimeout)
	defer cancel()
	if err := h.Cache.Invalidate(ctx, userID); err != nil {
		h.Log.Warn("console: account cache not invalidated; the cached state stays until it expires",
			"user_id", userID, "ttl", accountguard.TTL, "error", err)
	}
	if err := h.Notifier.Publish(ctx, userID, kind); err != nil {
		h.Log.Error("console: account change not published; open sockets stay until they reconnect",
			"user_id", userID, "kind", kind, "error", err)
	}
}

// setUserStatus suspends or re-activates a Multica account. An account that is
// pending deletion or deleted is not the console's to change.
func (h *Handler) setUserStatus(w http.ResponseWriter, r *http.Request) {
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	var body statusBody
	if perr := decodeBody(w, r, &body); perr != nil {
		writeError(w, perr)
		return
	}
	to, perr := parseAccountStatus(body.Status)
	if perr != nil {
		writeError(w, perr)
		return
	}
	reason, perr := cleanReason(body.Reason)
	if perr != nil {
		writeError(w, perr)
		return
	}
	a, err := h.currentActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	if id == a.ID {
		writeError(w, badRequest("self_target", "staff cannot change their own account"))
		return
	}

	changed := false
	err = h.Runner.InTx(r.Context(), func(tx Tx) error {
		email, found, err := tx.UserEmail(r.Context(), id)
		if err != nil {
			return err
		}
		if !found {
			return notFound()
		}
		acct, found, err := tx.Account(r.Context(), id)
		if err != nil {
			return err
		}
		from := accountguard.Active
		if found {
			from = accountguard.State(acct.Status)
		}
		if from != accountguard.Active && from != accountguard.Suspended {
			return &apiError{Status: http.StatusConflict, Code: "account_locked",
				Message: "the account is pending deletion or deleted"}
		}
		if from == to {
			return nil
		}
		if err := tx.SetAccountStatus(r.Context(), id, string(to), reason, a.ID); err != nil {
			return err
		}
		changed = true
		return audit.Record(r.Context(), tx, a.staffEntry(audit.Entry{
			Action:      audit.ActionAccountStatusChanged,
			TargetType:  targetUser,
			TargetID:    id,
			TargetLabel: email,
			Detail:      map[string]any{"from": string(from), "to": string(to), "reason": reason},
		}))
	})
	if err != nil {
		h.fail(w, r, "set account status", err)
		return
	}
	if changed {
		h.afterAccountChange(r.Context(), id, accountcache.KindStatus)
	}
	writeJSON(w, http.StatusOK, map[string]any{"changed": changed, "status": string(to)})
}

type reasonBody struct {
	Reason string `json:"reason"`
}

// revokeSessions signs a user out everywhere: every session issued before now
// stops working. The mark is in whole seconds, as the servers compare it.
func (h *Handler) revokeSessions(w http.ResponseWriter, r *http.Request) {
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	var body reasonBody
	if perr := decodeBody(w, r, &body); perr != nil {
		writeError(w, perr)
		return
	}
	reason, perr := cleanReason(body.Reason)
	if perr != nil {
		writeError(w, perr)
		return
	}
	a, err := h.currentActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}

	at := h.Now().UTC().Truncate(time.Second)
	err = h.Runner.InTx(r.Context(), func(tx Tx) error {
		email, found, err := tx.UserEmail(r.Context(), id)
		if err != nil {
			return err
		}
		if !found {
			return notFound()
		}
		if err := tx.SetSessionsValidAfter(r.Context(), id, a.ID, at); err != nil {
			return err
		}
		return audit.Record(r.Context(), tx, a.staffEntry(audit.Entry{
			Action:      audit.ActionAccountSessionsRevoked,
			TargetType:  targetUser,
			TargetID:    id,
			TargetLabel: email,
			Detail:      map[string]any{"reason": reason, "sessions_valid_after": at.Format(time.RFC3339)},
		}))
	})
	if err != nil {
		h.fail(w, r, "revoke sessions", err)
		return
	}
	h.afterAccountChange(r.Context(), id, accountcache.KindSessions)
	writeJSON(w, http.StatusOK, map[string]any{"sessions_valid_after": ts(at)})
}

// resetUserMFA deletes a user's second factor so they can sign in with the
// email code alone until they enrol again. It does not sign them out. A staff
// member cannot reset their own: the console would then let them skip the check
// it enforces on them. The audit row is in the transaction of the delete, and
// the user is emailed after it commits.
func (h *Handler) resetUserMFA(w http.ResponseWriter, r *http.Request) {
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	a, err := h.currentActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	if id == a.ID {
		writeError(w, &apiError{Status: http.StatusForbidden, Code: "self_reset_refused",
			Message: "staff cannot reset their own two-step sign-in"})
		return
	}

	var email string
	reset := false
	err = h.Runner.InTx(r.Context(), func(tx Tx) error {
		var found bool
		var err error
		if email, found, err = tx.UserEmail(r.Context(), id); err != nil {
			return err
		}
		if !found {
			return notFound()
		}
		if reset, err = tx.DeleteUserMFA(r.Context(), id); err != nil || !reset {
			return err
		}
		return audit.Record(r.Context(), tx, a.staffEntry(audit.Entry{
			Action:      audit.ActionUserMFAReset,
			TargetType:  targetUser,
			TargetID:    id,
			TargetLabel: email,
		}))
	})
	if err != nil {
		h.fail(w, r, "reset two-step sign-in", err)
		return
	}
	if reset {
		h.mailMFAReset(r.Context(), id, email)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"reset": reset})
}

// mailMFAReset emails the user after the reset is committed. A failure is
// logged and the request still succeeds.
func (h *Handler) mailMFAReset(ctx context.Context, userID, email string) {
	if h.Mailer == nil {
		h.Log.Warn("console: two-step sign-in reset; user notice not sent: no email provider", "user_id", userID)
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), afterChangeTimeout)
	defer cancel()
	if err := h.Mailer.SendMFAReset(ctx, email); err != nil {
		h.Log.Warn("console: two-step sign-in reset; user notice failed", "user_id", userID, "error", err)
	}
}

type addStaffBody struct {
	Email    string `json:"email"`
	Role     string `json:"role"`
	TenantID string `json:"tenant_id"`
}

// addStaff gives an existing Multica account a staff role, and re-activates
// the account's revoked row when it has one.
func (h *Handler) addStaff(w http.ResponseWriter, r *http.Request) {
	if !h.requireStaffManager(w, r) {
		return
	}
	var body addStaffBody
	if perr := decodeBody(w, r, &body); perr != nil {
		writeError(w, perr)
		return
	}
	email, perr := parseEmail(body.Email)
	if perr != nil {
		writeError(w, perr)
		return
	}
	target, perr := validateStaffTarget(body.Role, body.TenantID)
	if perr != nil {
		writeError(w, perr)
		return
	}
	a, err := h.currentActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}

	var userID string
	err = h.Runner.InTx(r.Context(), func(tx Tx) error {
		if err := tenantMustExist(r.Context(), tx, target.TenantID); err != nil {
			return err
		}
		id, found, err := tx.UserIDByEmail(r.Context(), email)
		if err != nil {
			return err
		}
		if !found {
			return &apiError{Status: http.StatusNotFound, Code: "no_such_account", Message: "no Multica account has this email"}
		}
		userID = id
		if _, _, active, err := tx.ActiveStaffRole(r.Context(), id); err != nil {
			return err
		} else if active {
			return alreadyStaff()
		}
		reactivated, err := tx.ReactivateStaff(r.Context(), id, string(target.Role), target.TenantID)
		if err != nil {
			return err
		}
		if !reactivated {
			inserted, err := tx.InsertStaff(r.Context(), id, string(target.Role), target.TenantID, a.ID)
			if err != nil {
				return err
			}
			if !inserted {
				return alreadyStaff()
			}
		}
		return audit.Record(r.Context(), tx, a.staffEntry(audit.Entry{
			Action:      audit.ActionStaffAdded,
			TenantID:    target.TenantID,
			TargetType:  targetUser,
			TargetID:    id,
			TargetLabel: email,
			Detail:      map[string]any{"role": string(target.Role), "tenant_id": target.TenantID, "reactivated": reactivated},
		}))
	})
	if err != nil {
		h.fail(w, r, "add staff", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"user_id": userID, "email": email, "role": string(target.Role), "tenant_id": tenantOrNull(target.TenantID),
	})
}

func alreadyStaff() *apiError {
	return &apiError{Status: http.StatusConflict, Code: "already_staff", Message: "the account already has an active staff role"}
}

// removeStaff revokes a staff row. The last active system administrator cannot
// be removed.
func (h *Handler) removeStaff(w http.ResponseWriter, r *http.Request) {
	if !h.requireStaffManager(w, r) {
		return
	}
	id, perr := pathID(r, "user_id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	a, err := h.currentActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}

	var removedRole string
	err = h.Runner.InTx(r.Context(), func(tx Tx) error {
		// Locking the system administrators first makes two removals queue up,
		// so each one counts the system administrators that remain.
		admins, err := tx.ActiveSystemAdmins(r.Context())
		if err != nil {
			return err
		}
		role, tenant, found, err := tx.ActiveStaffRole(r.Context(), id)
		if err != nil {
			return err
		}
		if !found {
			return notFound()
		}
		if role == string(staff.RoleSystemAdmin) && !otherThan(admins, id) {
			return lastSystemAdmin()
		}
		email, _, err := tx.UserEmail(r.Context(), id)
		if err != nil {
			return err
		}
		revoked, err := tx.RevokeStaff(r.Context(), id)
		if err != nil {
			return err
		}
		if !revoked {
			return notFound()
		}
		removedRole = role
		return audit.Record(r.Context(), tx, a.staffEntry(audit.Entry{
			Action:      audit.ActionStaffRemoved,
			TenantID:    tenant,
			TargetType:  targetUser,
			TargetID:    id,
			TargetLabel: email,
			Detail:      map[string]any{"role": role, "tenant_id": tenant},
		}))
	})
	if err != nil {
		h.fail(w, r, "remove staff", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"user_id": id, "role": removedRole})
}
