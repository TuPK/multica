package handler

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/server/internal/auditstore"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// StaffMemberError refuses a member change requested by platform staff. Code is
// the error_code the request records (spec R6 section 3b.3, D94).
type StaffMemberError struct{ Code string }

func (e *StaffMemberError) Error() string { return "staff member change refused: " + e.Code }

// Error codes of staff member changes.
const (
	StaffMemberErrSoleOwner          = "sole_owner"
	StaffMemberErrOwnerRoleImmutable = "owner_role_immutable"
	StaffMemberErrRoleChanged        = "role_changed_meanwhile"
)

// RemoveMemberForStaff removes the user from the workspace for a platform
// remove request. It goes through revokeAndRemoveMember, as DeleteMember does,
// so the runtimes, agents, tasks and seat are settled the same way. A user who
// is no longer a member is already removed: it returns false with no error.
func (h *Handler) RemoveMemberForStaff(ctx context.Context, workspaceID, userID string, origin DeletionOrigin) (bool, error) {
	wsID, err := util.ParseUUID(workspaceID)
	if err != nil {
		return false, fmt.Errorf("parse workspace id: %w", err)
	}
	uid, err := util.ParseUUID(userID)
	if err != nil {
		return false, fmt.Errorf("parse user id: %w", err)
	}
	member, err := h.Queries.GetMemberByUserAndWorkspace(ctx, db.GetMemberByUserAndWorkspaceParams{UserID: uid, WorkspaceID: wsID})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read membership: %w", err)
	}
	if member.Role == "owner" {
		members, err := h.Queries.ListMembers(ctx, wsID)
		if err != nil {
			return false, fmt.Errorf("list members: %w", err)
		}
		if countOwners(members) <= 1 {
			return false, &StaffMemberError{Code: StaffMemberErrSoleOwner}
		}
		return false, &StaffMemberError{Code: StaffMemberErrOwnerRoleImmutable}
	}
	target, err := h.Queries.GetUser(ctx, uid)
	if err != nil {
		return false, fmt.Errorf("load user: %w", err)
	}
	tenantID := ""
	if t, err := h.Queries.GetTenantIDByWorkspace(ctx, wsID); err == nil {
		tenantID = uuidToString(t)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("read tenant: %w", err)
	}

	result, err := h.revokeAndRemoveMember(ctx, wsID, uid, member.ID, pgtype.UUID{}, func(tx pgx.Tx) error {
		return audit.Record(ctx, auditstore.NewTx(h.Queries, tx), audit.Entry{
			WorkspaceID: workspaceID,
			TenantID:    tenantID,
			ActorType:   audit.ActorStaff,
			ActorID:     origin.StaffID,
			ActorLabel:  origin.StaffLabel,
			Action:      audit.ActionMemberRemoved,
			TargetType:  "member",
			TargetID:    userID,
			TargetLabel: target.Email,
			Detail:      map[string]any{"reason": "staff_request", "origin": string(DeletionOriginStaff), "request_id": origin.RequestID},
		})
	})
	if err != nil {
		return false, err
	}
	h.Hub.DisconnectMember(workspaceID, userID, uuidToString(member.ID))
	h.settleMemberCapacityRelease(ctx, uuid.UUID(wsID.Bytes), uuid.UUID(member.ID.Bytes))
	h.MembershipCache.Invalidate(ctx, userID, workspaceID)
	logRevocation(result, workspaceID, userID)
	h.publishRevocation(ctx, result, workspaceID, "system", "")
	h.publish(protocol.EventMemberRemoved, workspaceID, "system", "", map[string]any{
		"member_id":    uuidToString(member.ID),
		"workspace_id": workspaceID,
		"user_id":      userID,
	})
	h.notifyDaemonWorkspacesChanged(userID)
	return true, nil
}

// ChangeMemberRoleForStaff sets a member's role for a platform change_role
// request. Owner is immutable on either side. previousRole must still be the
// member's role, so a change made after the platform read the role is refused.
// A change to the role the member already has changes nothing and returns false.
func (h *Handler) ChangeMemberRoleForStaff(ctx context.Context, workspaceID, userID, previousRole, role string, origin DeletionOrigin) (bool, error) {
	wsID, err := util.ParseUUID(workspaceID)
	if err != nil {
		return false, fmt.Errorf("parse workspace id: %w", err)
	}
	uid, err := util.ParseUUID(userID)
	if err != nil {
		return false, fmt.Errorf("parse user id: %w", err)
	}
	member, err := h.Queries.GetMemberByUserAndWorkspace(ctx, db.GetMemberByUserAndWorkspaceParams{UserID: uid, WorkspaceID: wsID})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, &StaffMemberError{Code: "member_not_found"}
	}
	if err != nil {
		return false, fmt.Errorf("read membership: %w", err)
	}
	if member.Role == "owner" || role == "owner" {
		return false, &StaffMemberError{Code: StaffMemberErrOwnerRoleImmutable}
	}
	if member.Role != previousRole {
		return false, &StaffMemberError{Code: StaffMemberErrRoleChanged}
	}
	if member.Role == role {
		return false, nil
	}
	target, err := h.Queries.GetUser(ctx, uid)
	if err != nil {
		return false, fmt.Errorf("load user: %w", err)
	}
	tenantID := ""
	if t, err := h.Queries.GetTenantIDByWorkspace(ctx, wsID); err == nil {
		tenantID = uuidToString(t)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("read tenant: %w", err)
	}

	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)
	updated, err := qtx.UpdateMemberRole(ctx, db.UpdateMemberRoleParams{ID: member.ID, Role: role})
	if err != nil {
		return false, fmt.Errorf("update role: %w", err)
	}
	if err := audit.Record(ctx, auditstore.NewTx(h.Queries, tx), audit.Entry{
		WorkspaceID: workspaceID,
		TenantID:    tenantID,
		ActorType:   audit.ActorStaff,
		ActorID:     origin.StaffID,
		ActorLabel:  origin.StaffLabel,
		Action:      audit.ActionMemberRoleChanged,
		TargetType:  "member",
		TargetID:    userID,
		TargetLabel: target.Email,
		Detail:      map[string]any{"from": previousRole, "to": role, "origin": string(DeletionOriginStaff), "request_id": origin.RequestID},
	}); err != nil {
		return false, fmt.Errorf("audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	h.MembershipCache.Invalidate(ctx, userID, workspaceID)
	h.publish(protocol.EventMemberUpdated, workspaceID, "system", "", map[string]any{
		"member_id":    uuidToString(updated.ID),
		"workspace_id": workspaceID,
		"user_id":      userID,
		"role":         role,
	})
	h.notifyDaemonWorkspacesChanged(userID)
	return true, nil
}
