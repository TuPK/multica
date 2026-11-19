package staffdeletion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/server/internal/auditstore"
	"github.com/multica-ai/multica/server/internal/handler"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// ActionAccountDeleted is the audit action of an account the staff job deleted.
// The platform audit catalog does not list it yet.
const ActionAccountDeleted = "account.deleted"

const (
	errChildFailed         = "child_failed"
	errSoleOwnerUnresolved = "sole_owner_unresolved"

	// deletedUserLabel is the label of the account after its deletion, as the
	// self-service job writes it.
	deletedUserLabel = "deleted user"

	actionTransfer = "transfer"
	roleOwner      = "owner"
	statusActive   = "active"
)

// permanentError is a failure no retry can fix; Code goes to error_code.
type permanentError struct{ Code string }

func (e *permanentError) Error() string { return "staff deletion failed: " + e.Code }

// errWaitingForChildren releases a request whose workspace requests have not
// finished. The attempt is not counted.
var errWaitingForChildren = errors.New("waiting for child requests")

// resolution is one choice of an account request (spec R6 section 7.3).
type resolution struct {
	WorkspaceID     string `json:"workspace_id"`
	Action          string `json:"action"`
	SuccessorUserID string `json:"successor_user_id"`
}

// runAccount carries out an account request. It runs after its workspace
// requests complete (spec R6 section 7.4, phase 2 step 1), transfers ownership
// for each transfer choice, re-checks that no workspace is left without an
// owner, and deletes the account through the self-service pipeline.
func (r *Runner) runAccount(ctx context.Context, req db.PlatformDeletionRequest, origin handler.DeletionOrigin) error {
	states, err := r.d.Queries.ListStaffDeletionChildStates(ctx, req.ID)
	if err != nil {
		return fmt.Errorf("list child requests: %w", err)
	}
	for _, state := range states {
		switch state {
		case "failed":
			return &permanentError{Code: errChildFailed}
		case "pending", "running":
			return errWaitingForChildren
		}
	}

	var resolutions []resolution
	if len(req.Resolutions) > 0 {
		if err := json.Unmarshal(req.Resolutions, &resolutions); err != nil {
			return fmt.Errorf("decode resolutions: %w", err)
		}
	}
	userID := util.UUIDToString(req.TargetID)
	for _, res := range resolutions {
		if res.Action != actionTransfer {
			continue
		}
		if err := r.transfer(ctx, res, origin); err != nil {
			return err
		}
	}

	if err := r.checkNoSoleOwner(ctx, req.TargetID, userID); err != nil {
		return err
	}

	deleted, err := r.d.Eraser.DeleteAccountForOrigin(ctx, userID, time.Now(), origin)
	if err != nil {
		return fmt.Errorf("delete account: %w", err)
	}
	if !deleted {
		// No pending row: a previous attempt deleted the account.
		return nil
	}
	return audit.Record(ctx, auditstore.New(r.d.Queries), audit.Entry{
		ActorType:   audit.ActorStaff,
		ActorID:     origin.StaffID,
		ActorLabel:  origin.StaffLabel,
		Action:      ActionAccountDeleted,
		TargetType:  "user",
		TargetID:    userID,
		TargetLabel: deletedUserLabel,
		Detail:      map[string]any{"reason": "account_deletion", "origin": "staff", "request_id": origin.RequestID},
	})
}

// transfer makes the successor the owner of the workspace, in one transaction.
// The successor must still be an active admin or member of the workspace (GĐ11);
// otherwise the request stops with sole_owner_unresolved.
func (r *Runner) transfer(ctx context.Context, res resolution, origin handler.DeletionOrigin) error {
	wsID, err := util.ParseUUID(res.WorkspaceID)
	if err != nil {
		return fmt.Errorf("parse workspace id: %w", err)
	}
	successor, err := util.ParseUUID(res.SuccessorUserID)
	if err != nil {
		return fmt.Errorf("parse successor id: %w", err)
	}

	tx, err := r.d.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	qtx := r.d.Queries.WithTx(tx)

	member, err := qtx.GetMemberByUserAndWorkspace(ctx, db.GetMemberByUserAndWorkspaceParams{UserID: successor, WorkspaceID: wsID})
	if errors.Is(err, pgx.ErrNoRows) {
		return &permanentError{Code: errSoleOwnerUnresolved}
	}
	if err != nil {
		return fmt.Errorf("read successor membership: %w", err)
	}
	if member.Role != "admin" && member.Role != "member" {
		return &permanentError{Code: errSoleOwnerUnresolved}
	}
	schedule, err := qtx.GetAccountDeletionSchedule(ctx, successor)
	switch {
	case errors.Is(err, pgx.ErrNoRows): // no row means an active account
	case err != nil:
		return fmt.Errorf("read successor account: %w", err)
	case schedule.Status != statusActive:
		return &permanentError{Code: errSoleOwnerUnresolved}
	}

	if _, err := qtx.UpdateMemberRole(ctx, db.UpdateMemberRoleParams{ID: member.ID, Role: roleOwner}); err != nil {
		return fmt.Errorf("promote successor: %w", err)
	}
	if err := audit.Record(ctx, auditstore.NewTx(r.d.Queries, tx), audit.Entry{
		WorkspaceID: res.WorkspaceID,
		ActorType:   audit.ActorStaff,
		ActorID:     origin.StaffID,
		ActorLabel:  origin.StaffLabel,
		Action:      audit.ActionMemberRoleChanged,
		TargetType:  "member",
		TargetID:    res.SuccessorUserID,
		Detail:      map[string]any{"from": member.Role, "to": roleOwner, "reason": "account_deletion", "origin": "staff", "request_id": origin.RequestID},
	}); err != nil {
		return fmt.Errorf("audit role change: %w", err)
	}
	return tx.Commit(ctx)
}

// checkNoSoleOwner stops the request when a workspace that has other members is
// left without an owner once the account is gone. The account is not deleted
// then; DeleteAccount's own check stays in place behind this one.
func (r *Runner) checkNoSoleOwner(ctx context.Context, userID pgtype.UUID, userIDText string) error {
	rows, err := r.d.Queries.ListUserWorkspaceCounts(ctx, userID)
	if err != nil {
		return fmt.Errorf("list workspaces: %w", err)
	}
	for _, row := range rows {
		if row.Role != roleOwner || row.MemberCount <= 1 {
			continue
		}
		members, err := r.d.Queries.ListMembers(ctx, row.WorkspaceID)
		if err != nil {
			return fmt.Errorf("list members: %w", err)
		}
		others := 0
		for _, m := range members {
			if m.Role == roleOwner && util.UUIDToString(m.UserID) != userIDText {
				others++
			}
		}
		if others == 0 {
			return &permanentError{Code: errSoleOwnerUnresolved}
		}
	}
	return nil
}
