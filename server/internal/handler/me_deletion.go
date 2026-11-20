package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/platform-management/accountdeletion"
	"github.com/multica-ai/multica/platform-management/accountguard"
	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/server/internal/auditstore"
	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/logger"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// deletedUserLabel is the label the job puts on the audit rows it writes for
// the user it is deleting, so no email lands in a row the two label renames do
// not reach (those only match target_type 'user').
const deletedUserLabel = "deleted user"

// accountDeletionWorkspace is a workspace named in the preview.
type accountDeletionWorkspace struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// accountDeletionBlocker is a workspace that blocks the request because the
// user owns it and other members remain.
type accountDeletionBlocker struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	MemberCount int    `json:"member_count"`
}

// AccountDeletionPreviewResponse is the body of POST /api/me/deletion?preview=1.
// ScheduledAt is always null: a preview schedules nothing.
type AccountDeletionPreviewResponse struct {
	SoleWorkspaces []accountDeletionWorkspace `json:"sole_workspaces"`
	ScheduledAt    *string                    `json:"scheduled_at"`
}

// AccountDeletionScheduledResponse is the body of a successful POST /api/me/deletion.
type AccountDeletionScheduledResponse struct {
	ScheduledAt string `json:"scheduled_at"`
}

// AccountDeletionCanceledResponse is the body of a successful DELETE /api/me/deletion.
type AccountDeletionCanceledResponse struct {
	Status string `json:"status"`
}

type soleOwnerResponse struct {
	Error      string                   `json:"error"`
	Code       string                   `json:"code"`
	Workspaces []accountDeletionBlocker `json:"workspaces"`
}

// sessionAuthTime returns the `auth_time` claim of the request's session token.
// present is false for a personal access token, a machine token, an expired or
// forged token, and a session token minted without the claim. There is no
// fallback to `iat`: a token that cannot prove when the user logged in is not
// a recent login.
func sessionAuthTime(r *http.Request) (authTime int64, present bool) {
	tok, _ := sessionTokenFromRequest(r)
	if tok == "" {
		return 0, false
	}
	claims, err := auth.ParseSessionToken(tok)
	if err != nil {
		return 0, false
	}
	switch v := claims[auth.AuthTimeClaim].(type) {
	case float64:
		return int64(v), true
	case int64:
		return v, true
	default:
		return 0, false
	}
}

// requireRecentLogin answers 403 recent_login_required unless the caller is a
// human with a session whose login is within accountdeletion.RecentLoginWindow.
func requireRecentLogin(w http.ResponseWriter, r *http.Request, now time.Time) bool {
	if isMachineCredentialActor(r) {
		writeError(w, http.StatusForbidden, "this endpoint is only available to human actors")
		return false
	}
	authTime, present := sessionAuthTime(r)
	if !accountdeletion.RecentLogin(authTime, present, now) {
		writeErrorCode(w, http.StatusForbidden, "recent_login_required", "sign in again to confirm this action")
		return false
	}
	return true
}

func membershipsFromRows(rows []db.ListUserWorkspaceCountsRow) []accountdeletion.Membership {
	out := make([]accountdeletion.Membership, len(rows))
	for i, r := range rows {
		out[i] = accountdeletion.Membership{
			WorkspaceID: uuidToString(r.WorkspaceID),
			Name:        r.WorkspaceName,
			Role:        r.Role,
			MemberCount: int(r.MemberCount),
		}
	}
	return out
}

func formatDeletionTime(t pgtype.Timestamptz) string {
	return t.Time.UTC().Format(time.RFC3339)
}

// RequestAccountDeletion serves POST /api/me/deletion. With ?preview=1 it
// lists the workspaces the job would delete and changes nothing. Otherwise it
// schedules the deletion, ends the caller's sessions and sends a confirmation
// email after the commit.
//
// guard is the account guard whose cached record for this user is dropped once
// the state changes; a nil guard skips the drop.
func (h *Handler) RequestAccountDeletion(guard *accountguard.Guard) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := requireUserID(w, r)
		if !ok {
			return
		}
		uid, ok := parseUUIDOrBadRequest(w, userID, "user id")
		if !ok {
			return
		}
		ctx := r.Context()
		now := time.Unix(time.Now().Unix(), 0)
		if !requireRecentLogin(w, r, now) {
			return
		}
		preview := r.URL.Query().Get("preview") == "1"

		if !preview {
			state, err := h.Queries.GetAccountDeletionSchedule(ctx, uid)
			if err == nil && state.Status == accountdeletion.StatusPending && state.DeletionScheduledAt.Valid {
				writeJSON(w, http.StatusOK, AccountDeletionScheduledResponse{ScheduledAt: formatDeletionTime(state.DeletionScheduledAt)})
				return
			}
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusInternalServerError, "failed to load account state")
				return
			}
		}

		rows, err := h.Queries.ListUserWorkspaceCounts(ctx, uid)
		if err != nil {
			slog.Warn("account deletion: list workspaces failed", append(logger.RequestAttrs(r), "error", err, "user_id", userID)...)
			writeError(w, http.StatusInternalServerError, "failed to load workspaces")
			return
		}
		plan := accountdeletion.Classify(membershipsFromRows(rows))
		if len(plan.Blocking) > 0 {
			blockers := make([]accountDeletionBlocker, len(plan.Blocking))
			for i, b := range plan.Blocking {
				blockers[i] = accountDeletionBlocker{ID: b.WorkspaceID, Name: b.Name, MemberCount: b.MemberCount}
			}
			writeJSON(w, http.StatusConflict, soleOwnerResponse{
				Error:      "transfer ownership or delete these workspaces before deleting your account",
				Code:       "sole_owner",
				Workspaces: blockers,
			})
			return
		}
		if preview {
			sole := make([]accountDeletionWorkspace, len(plan.Sole))
			for i, s := range plan.Sole {
				sole[i] = accountDeletionWorkspace{ID: s.WorkspaceID, Name: s.Name}
			}
			writeJSON(w, http.StatusOK, AccountDeletionPreviewResponse{SoleWorkspaces: sole})
			return
		}

		user, err := h.Queries.GetUser(ctx, uid)
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, "user not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load user")
			return
		}

		scheduled := accountdeletion.ScheduledAt(now)
		tx, err := h.TxStarter.Begin(ctx)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to schedule deletion")
			return
		}
		defer tx.Rollback(ctx)
		scheduledAt, err := h.Queries.WithTx(tx).RequestAccountDeletion(ctx, db.RequestAccountDeletionParams{
			UserID:              uid,
			SessionsValidAfter:  pgtype.Timestamptz{Time: now, Valid: true},
			DeletionScheduledAt: pgtype.Timestamptz{Time: scheduled, Valid: true},
		})
		if errors.Is(err, pgx.ErrNoRows) {
			// A concurrent request won, or the account is not active.
			h.writeExistingDeletionSchedule(w, r, uid)
			return
		}
		if err != nil {
			slog.Warn("account deletion: schedule failed", append(logger.RequestAttrs(r), "error", err, "user_id", userID)...)
			writeError(w, http.StatusInternalServerError, "failed to schedule deletion")
			return
		}
		if err := h.recordAudit(r, tx, audit.Entry{
			Action:      audit.ActionAccountDeletionRequest,
			TargetType:  "user",
			TargetID:    userID,
			TargetLabel: user.Email,
		}); err != nil {
			slog.Warn("account deletion: audit failed", append(logger.RequestAttrs(r), "error", err, "user_id", userID)...)
			writeError(w, http.StatusInternalServerError, "failed to schedule deletion")
			return
		}
		if err := tx.Commit(ctx); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to schedule deletion")
			return
		}

		h.afterAccountStateChange(r, guard, userID)
		when := formatDeletionTime(scheduledAt)
		if h.EmailService != nil {
			email := user.Email
			go func() {
				if err := h.EmailService.SendAccountDeletionScheduledEmail(email, when); err != nil {
					slog.Warn("account deletion: confirmation email failed", "user_id", userID, "error", err)
				}
			}()
		}
		slog.Info("account deletion scheduled", append(logger.RequestAttrs(r), "user_id", userID, "scheduled_at", when)...)
		h.Hub.DisconnectUser(userID)
		writeJSON(w, http.StatusAccepted, AccountDeletionScheduledResponse{ScheduledAt: when})
	}
}

// writeExistingDeletionSchedule answers a request that lost the race to an
// earlier one: the existing date with 200, or 409 when the account is not
// pending at all.
func (h *Handler) writeExistingDeletionSchedule(w http.ResponseWriter, r *http.Request, uid pgtype.UUID) {
	state, err := h.Queries.GetAccountDeletionSchedule(r.Context(), uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load account state")
		return
	}
	if state.Status != accountdeletion.StatusPending || !state.DeletionScheduledAt.Valid {
		writeError(w, http.StatusConflict, "account is not active")
		return
	}
	writeJSON(w, http.StatusOK, AccountDeletionScheduledResponse{ScheduledAt: formatDeletionTime(state.DeletionScheduledAt)})
}

// afterAccountStateChange drops the cached account record so the guard sees the
// new state at once. A failure only leaves the old record readable for at most
// accountguard.TTL.
func (h *Handler) afterAccountStateChange(r *http.Request, guard *accountguard.Guard, userID string) {
	if guard == nil {
		return
	}
	if err := guard.Invalidate(r.Context(), userID); err != nil {
		slog.Warn("account deletion: account cache invalidation failed; stale for up to the cache TTL",
			append(logger.RequestAttrs(r), "error", err, "user_id", userID)...)
	}
}

// CancelAccountDeletion serves DELETE /api/me/deletion. It needs no recent
// login: the caller has just signed in to cancel, and the guard lets a pending
// account reach only this route, GET /api/me and sign-out.
func (h *Handler) CancelAccountDeletion(guard *accountguard.Guard) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := requireUserID(w, r)
		if !ok {
			return
		}
		uid, ok := parseUUIDOrBadRequest(w, userID, "user id")
		if !ok {
			return
		}
		if isMachineCredentialActor(r) {
			writeError(w, http.StatusForbidden, "this endpoint is only available to human actors")
			return
		}
		ctx := r.Context()
		user, err := h.Queries.GetUser(ctx, uid)
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, "user not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load user")
			return
		}
		tx, err := h.TxStarter.Begin(ctx)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to cancel deletion")
			return
		}
		defer tx.Rollback(ctx)
		if _, err := h.Queries.WithTx(tx).CancelAccountDeletion(ctx, uid); errors.Is(err, pgx.ErrNoRows) {
			writeErrorCode(w, http.StatusConflict, "not_pending_deletion", "the account is not scheduled for deletion")
			return
		} else if err != nil {
			slog.Warn("account deletion: cancel failed", append(logger.RequestAttrs(r), "error", err, "user_id", userID)...)
			writeError(w, http.StatusInternalServerError, "failed to cancel deletion")
			return
		}
		if err := h.recordAudit(r, tx, audit.Entry{
			Action:      audit.ActionAccountDeletionCanceled,
			TargetType:  "user",
			TargetID:    userID,
			TargetLabel: user.Email,
		}); err != nil {
			slog.Warn("account deletion: audit failed", append(logger.RequestAttrs(r), "error", err, "user_id", userID)...)
			writeError(w, http.StatusInternalServerError, "failed to cancel deletion")
			return
		}
		if err := tx.Commit(ctx); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to cancel deletion")
			return
		}
		h.afterAccountStateChange(r, guard, userID)
		slog.Info("account deletion canceled", append(logger.RequestAttrs(r), "user_id", userID)...)
		writeJSON(w, http.StatusOK, AccountDeletionCanceledResponse{Status: "active"})
	}
}

// DueAccountDeletions lists the accounts the job should process at now.
func (h *Handler) DueAccountDeletions(ctx context.Context, now time.Time, limit int) ([]string, error) {
	ids, err := h.Queries.ListDueAccountDeletions(ctx, db.ListDueAccountDeletionsParams{
		Now:      pgtype.Timestamptz{Time: now, Valid: true},
		RowLimit: int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = uuidToString(id)
	}
	return out, nil
}

// DeleteAccount carries out one due deletion. It returns false, with no error,
// when the account is no longer pending or not yet due, so a second run finds
// nothing to do.
//
// The account's platform_account row is locked for the whole run. A cancel that
// arrives meanwhile waits, then finds the account deleted. Each workspace is
// removed in its own transaction, because revokeAndRemoveMember and the
// workspace teardown commit their own; a failure part-way leaves the account
// pending, and the next run finishes the rest. The personal rows, the audit
// labels, the user row and the status change commit together.
func (h *Handler) DeleteAccount(ctx context.Context, userID string, now time.Time) (bool, error) {
	return h.DeleteAccountForOrigin(ctx, userID, now, DeletionOrigin{Kind: DeletionOriginSelf})
}

// DeleteAccountForOrigin is DeleteAccount for a caller other than the account's
// own scheduled deletion. The pipeline is the same; origin only sets the actor
// of the audit rows it writes (spec R6 section 7.4, phase 2 step 4).
func (h *Handler) DeleteAccountForOrigin(ctx context.Context, userID string, now time.Time, origin DeletionOrigin) (bool, error) {
	uid, err := util.ParseUUID(userID)
	if err != nil {
		return false, fmt.Errorf("parse user id: %w", err)
	}
	if err := h.checkAccountHeld(ctx, uid); err != nil {
		return false, err
	}
	user, err := h.Queries.GetUser(ctx, uid)
	if err != nil {
		return false, fmt.Errorf("load user: %w", err)
	}

	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)
	if _, err := qtx.LockDueAccountDeletion(ctx, db.LockDueAccountDeletionParams{
		UserID: uid,
		Now:    pgtype.Timestamptz{Time: now, Valid: true},
	}); errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("lock account: %w", err)
	}

	rows, err := h.Queries.ListUserWorkspaceCounts(ctx, uid)
	if err != nil {
		return false, fmt.Errorf("list workspaces: %w", err)
	}
	for _, row := range rows {
		if row.MemberCount <= 1 {
			if err := h.deleteSoleMemberWorkspace(ctx, row.WorkspaceID, user, origin); err != nil {
				return false, fmt.Errorf("delete workspace %s: %w", uuidToString(row.WorkspaceID), err)
			}
			continue
		}
		if err := h.removeMemberForAccountDeletion(ctx, row, user, origin); err != nil {
			return false, fmt.Errorf("remove membership in %s: %w", uuidToString(row.WorkspaceID), err)
		}
	}

	patHashes, err := qtx.DeleteUserPersonalAccessTokens(ctx, uid)
	if err != nil {
		return false, fmt.Errorf("delete personal access tokens: %w", err)
	}
	daemonHashes, err := qtx.DeleteUserDaemonTokens(ctx, uid)
	if err != nil {
		return false, fmt.Errorf("delete daemon tokens: %w", err)
	}
	steps := []struct {
		name string
		run  func() error
	}{
		{"task tokens", func() error { return qtx.DeleteUserTaskTokens(ctx, uid) }},
		{"composio connections", func() error { return qtx.DeleteUserComposioConnections(ctx, uid) }},
		{"channel bindings", func() error { return qtx.DeleteUserChannelBindings(ctx, uid) }},
		{"lark bindings", func() error { return qtx.DeleteUserLarkBindings(ctx, uid) }},
		{"knowledge owner accounts", func() error { return qtx.DeleteUserKnowledgeOwnerAccounts(ctx, uid) }},
		{"consents", func() error { return qtx.DeleteUserConsents(ctx, uid) }},
		{"second factor", func() error { _, err := qtx.DeleteUserMFA(ctx, uid); return err }},
		{"feedback", func() error { return qtx.DeleteUserFeedback(ctx, uid) }},
		{"client usage", func() error { return qtx.DeleteUserClientUsage(ctx, uid) }},
		{"notification preferences", func() error { return qtx.DeleteUserNotificationPreferences(ctx, uid) }},
		{"pinned items", func() error { return qtx.DeleteUserPinnedItems(ctx, uid) }},
		{"chat pinned agents", func() error { return qtx.DeleteUserChatPinnedAgents(ctx, uid) }},
		{"issue unread", func() error { return qtx.DeleteUserIssueUnread(ctx, uid) }},
		{"delegation days", func() error { return qtx.DeleteUserDelegationDays(ctx, uid) }},
		{"autopilot collaborators", func() error { return qtx.DeleteUserAutopilotCollaborators(ctx, uid) }},
		{"project members", func() error { return qtx.DeleteUserProjectMembers(ctx, uid) }},
		{"invitations", func() error {
			return qtx.DeleteUserInvitations(ctx, db.DeleteUserInvitationsParams{UserID: uid, Email: user.Email})
		}},
		{"anonymize account requests", func() error { return qtx.AnonymizeAccountRequestsForUser(ctx, uid) }},
		{"anonymize member requests", func() error { return qtx.AnonymizeMemberRequestsForUser(ctx, uid) }},
		{"anonymize user", func() error { return qtx.AnonymizeDeletedUser(ctx, uid) }},
		{"anonymize audit actor", func() error {
			return qtx.AnonymizeAuditActor(ctx, pgtype.Text{String: userID, Valid: true})
		}},
		{"anonymize audit target", func() error { return qtx.AnonymizeAuditTarget(ctx, userID) }},
		{"mark deleted", func() error { return qtx.MarkAccountDeleted(ctx, uid) }},
	}
	for _, s := range steps {
		if err := s.run(); err != nil {
			return false, fmt.Errorf("%s: %w", s.name, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}

	for _, hash := range patHashes {
		h.PATCache.Invalidate(ctx, hash)
	}
	for _, hash := range daemonHashes {
		h.DaemonTokenCache.Invalidate(ctx, hash)
	}
	h.Hub.DisconnectUser(userID)
	slog.Info("account deleted", "user_id", userID)
	return true, nil
}

// removeMemberForAccountDeletion removes the user from a workspace that goes on
// without them, through the same path as leaving it.
func (h *Handler) removeMemberForAccountDeletion(ctx context.Context, row db.ListUserWorkspaceCountsRow, user db.User, origin DeletionOrigin) error {
	workspaceID := uuidToString(row.WorkspaceID)
	if row.Role == "owner" {
		members, err := h.Queries.ListMembers(ctx, row.WorkspaceID)
		if err != nil {
			return err
		}
		// Leaving does not leave a workspace without an owner, and neither does this.
		if countOwners(members) <= 1 {
			return errors.New("the user is the only owner of a workspace that has other members")
		}
	}
	actor := origin.accountActor(uuidToString(user.ID))
	result, err := h.revokeAndRemoveMember(ctx, row.WorkspaceID, user.ID, row.MemberID, user.ID, func(tx pgx.Tx) error {
		return audit.Record(ctx, auditstore.NewTx(h.Queries, tx), audit.Entry{
			WorkspaceID: workspaceID,
			ActorType:   actor.Type,
			ActorID:     actor.ID,
			ActorLabel:  actor.Label,
			Action:      audit.ActionMemberLeft,
			TargetType:  "member",
			TargetID:    uuidToString(user.ID),
			TargetLabel: deletedUserLabel,
			Detail:      actor.Detail,
		})
	})
	if err != nil {
		return err
	}
	userID := uuidToString(user.ID)
	h.Hub.DisconnectMember(workspaceID, userID, uuidToString(row.MemberID))
	h.MembershipCache.Invalidate(ctx, userID, workspaceID)
	logRevocation(result, workspaceID, userID)
	h.publishRevocation(ctx, result, workspaceID, "member", userID)
	h.publish(protocol.EventMemberRemoved, workspaceID, "member", userID, map[string]any{
		"member_id":    uuidToString(row.MemberID),
		"workspace_id": workspaceID,
		"user_id":      userID,
	})
	h.notifyDaemonWorkspacesChanged(userID)
	return nil
}

// deleteSoleMemberWorkspace deletes a workspace whose only member is the user,
// through the same teardown DeleteWorkspace runs.
func (h *Handler) deleteSoleMemberWorkspace(ctx context.Context, wsID pgtype.UUID, user db.User, origin DeletionOrigin) error {
	workspaceID := uuidToString(wsID)
	userID := uuidToString(user.ID)
	if err := h.prepareQoderWorkspaceDeletion(ctx, workspaceID); err != nil {
		return fmt.Errorf("qoder cleanup is pending: %w", err)
	}
	h.MembershipCache.Invalidate(ctx, userID, workspaceID)

	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	teardown, err := h.deleteWorkspaceData(ctx, tx, wsID)
	if err != nil {
		return err
	}
	actor := origin.accountActor(userID)
	if err := audit.Record(ctx, auditstore.NewTx(h.Queries, tx), audit.Entry{
		WorkspaceID: workspaceID,
		ActorType:   actor.Type,
		ActorID:     actor.ID,
		ActorLabel:  actor.Label,
		Action:      audit.ActionWorkspaceDeleted,
		TargetType:  "workspace",
		TargetID:    workspaceID,
		TargetLabel: teardown.Name,
		Detail:      actor.Detail,
	}); err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	for _, runtimeID := range teardown.RuntimeIDs {
		h.NotifyRuntimeGone(uuidToString(runtimeID))
	}
	h.deleteS3Objects(ctx, teardown.ObjectURLs)
	h.deleteHubObjects(ctx, workspaceID)
	h.publish(protocol.EventWorkspaceDeleted, workspaceID, "member", userID, map[string]any{
		"workspace_id": workspaceID,
	})
	h.notifyDaemonWorkspacesChanged(userID)
	return nil
}
