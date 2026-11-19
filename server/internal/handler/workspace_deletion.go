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

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/server/internal/auditstore"
	"github.com/multica-ai/multica/server/internal/logger"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/util"
	"github.com/multica-ai/multica/server/internal/workspacedeletion"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// WorkspaceDeletionScheduledResponse is the 202 body of DeleteWorkspace when
// the deletion waits out a grace period.
type WorkspaceDeletionScheduledResponse struct {
	Status              string `json:"status"`
	DeletionScheduledAt string `json:"deletion_scheduled_at"`
}

// scheduleWorkspaceDeletion records a deletion that runs at scheduleAt instead
// of deleting the workspace now. The caller has checked that the requester is
// the owner. It writes the response.
func (h *Handler) scheduleWorkspaceDeletion(w http.ResponseWriter, r *http.Request, requester db.Member, scheduleAt time.Time) {
	workspaceID := uuidToString(requester.WorkspaceID)

	// Stop remote work before the workspace is marked; a pending cleanup leaves
	// the workspace untouched, so the owner can retry.
	if err := h.prepareQoderWorkspaceDeletion(r.Context(), workspaceID); err != nil {
		writeError(w, http.StatusConflict, "Qoder cleanup is pending; retry workspace deletion shortly")
		return
	}

	var scheduled pgtype.Timestamptz
	err := h.auditTx(r, func(qtx *db.Queries) (audit.Entry, error) {
		var err error
		scheduled, err = qtx.ScheduleWorkspaceDeletion(r.Context(), db.ScheduleWorkspaceDeletionParams{
			WorkspaceID: requester.WorkspaceID,
			Reason:      workspacedeletion.Reason,
			ChangedBy:   requester.UserID,
			ScheduledAt: pgtype.Timestamptz{Time: scheduleAt, Valid: true},
		})
		if err != nil {
			return audit.Entry{}, err
		}
		return audit.Entry{
			WorkspaceID: workspaceID,
			Action:      audit.ActionWorkspaceDeletionScheduled,
			TargetType:  "workspace",
			TargetID:    workspaceID,
			Detail:      map[string]any{"deletion_scheduled_at": scheduled.Time.UTC().Format(time.RFC3339)},
		}, nil
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Already pending: report the schedule that stands.
		existing, stateErr := h.Queries.WorkspaceState(r.Context(), requester.WorkspaceID)
		if stateErr != nil || !existing.DeletionScheduledAt.Valid {
			slog.Warn("workspace deletion: read existing schedule failed", append(logger.RequestAttrs(r), "error", stateErr, "workspace_id", workspaceID)...)
			writeError(w, http.StatusInternalServerError, "failed to schedule workspace deletion")
			return
		}
		scheduled = existing.DeletionScheduledAt
	case err != nil:
		slog.Warn("workspace deletion: schedule failed", append(logger.RequestAttrs(r), "error", err, "workspace_id", workspaceID)...)
		writeError(w, http.StatusInternalServerError, "failed to schedule workspace deletion")
		return
	}

	h.invalidateWorkspaceState(r, workspaceID)
	slog.Info("workspace deletion scheduled", append(logger.RequestAttrs(r), "workspace_id", workspaceID, "deletion_scheduled_at", scheduled.Time)...)
	writeJSON(w, http.StatusAccepted, WorkspaceDeletionScheduledResponse{
		Status:              workspacedeletion.StatusPending,
		DeletionScheduledAt: scheduled.Time.UTC().Format(time.RFC3339),
	})
}

// CancelWorkspaceDeletion serves DELETE /api/workspaces/{id}/deletion. Only the
// owner may cancel, and only while the deletion is pending.
func (h *Handler) CancelWorkspaceDeletion(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	requester, ok := h.workspaceMember(w, r, workspaceID)
	if !ok {
		return
	}
	if requester.Role != "owner" {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	workspaceUUID := uuidToString(requester.WorkspaceID)
	err := h.auditTx(r, func(qtx *db.Queries) (audit.Entry, error) {
		if _, err := qtx.CancelWorkspaceDeletion(r.Context(), db.CancelWorkspaceDeletionParams{
			WorkspaceID: requester.WorkspaceID,
			ChangedBy:   requester.UserID,
		}); err != nil {
			return audit.Entry{}, err
		}
		return audit.Entry{
			WorkspaceID: workspaceUUID,
			Action:      audit.ActionWorkspaceDeletionCancelled,
			TargetType:  "workspace",
			TargetID:    workspaceUUID,
		}, nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeErrorCode(w, http.StatusNotFound, "no_pending_deletion", "the workspace is not scheduled for deletion")
		return
	}
	if err != nil {
		slog.Warn("workspace deletion: cancel failed", append(logger.RequestAttrs(r), "error", err, "workspace_id", workspaceID)...)
		writeError(w, http.StatusInternalServerError, "failed to cancel workspace deletion")
		return
	}

	h.invalidateWorkspaceState(r, uuidToString(requester.WorkspaceID))
	slog.Info("workspace deletion canceled", append(logger.RequestAttrs(r), "workspace_id", workspaceID)...)
	w.WriteHeader(http.StatusNoContent)
}

// invalidateWorkspaceState drops the cached workspace state. A failure is
// logged and not returned: the stale state lives at most workspacestate.TTL.
func (h *Handler) invalidateWorkspaceState(r *http.Request, workspaceID string) {
	guard := middleware.WorkspaceStateFromContext(r.Context())
	if guard == nil {
		return
	}
	if err := guard.Invalidate(r.Context(), workspaceID); err != nil {
		slog.Warn("workspace deletion: state cache invalidation failed; stale for up to the cache TTL",
			append(logger.RequestAttrs(r), "error", err, "workspace_id", workspaceID)...)
	}
}

// DueWorkspaceDeletions lists up to limit workspaces that are pending deletion
// and due at now, oldest first.
func (h *Handler) DueWorkspaceDeletions(ctx context.Context, now time.Time, limit int) ([]string, error) {
	ids, err := h.Queries.ListDueWorkspaceDeletions(ctx, db.ListDueWorkspaceDeletionsParams{
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

// DeleteDueWorkspace deletes one workspace whose grace period has ended,
// through the teardown DeleteWorkspace runs. It returns false, with no error,
// when the workspace is no longer pending or not yet due: it was cancelled, a
// parallel run deleted it, or the schedule moved.
//
// The platform_workspace row is locked for the whole transaction. A cancel or a
// parallel run waits for it, then finds the row changed or gone, so a workspace
// is deleted once and never after a cancel that committed first. The row is
// removed with the workspace; nothing cascades to it.
func (h *Handler) DeleteDueWorkspace(ctx context.Context, workspaceID string, now time.Time) (bool, error) {
	wsID, err := util.ParseUUID(workspaceID)
	if err != nil {
		return false, fmt.Errorf("parse workspace id: %w", err)
	}

	// Look before the Qoder cleanup so a cancelled workspace is not touched.
	state, err := h.Queries.WorkspaceState(ctx, wsID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read state: %w", err)
	}
	if state.Status != workspacedeletion.StatusPending || !state.DeletionScheduledAt.Valid ||
		!workspacedeletion.Due(now, state.DeletionScheduledAt.Time) {
		return false, nil
	}

	return h.deleteWorkspaceForOrigin(ctx, wsID, DeletionOrigin{Kind: DeletionOriginPeriodEnded}, func(qtx *db.Queries) (bool, error) {
		scheduledAt, err := qtx.LockPendingWorkspaceDeletion(ctx, wsID)
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("lock schedule: %w", err)
		}
		return scheduledAt.Valid && workspacedeletion.Due(now, scheduledAt.Time), nil
	}, nil)
}

// DeleteWorkspaceForStaff runs phase 2 of an immediate deletion for one
// workspace (spec R6 section 6.3). The platform request is running, so the
// workspace's schedule is not consulted. It returns false, with no error, when
// the workspace is already gone. inject is a test hook: a non-nil error from
// stage "audit" fails the transaction, which then keeps every row of the workspace.
func (h *Handler) DeleteWorkspaceForStaff(ctx context.Context, workspaceID string, origin DeletionOrigin, inject func(stage string) error) (bool, error) {
	wsID, err := util.ParseUUID(workspaceID)
	if err != nil {
		return false, fmt.Errorf("parse workspace id: %w", err)
	}
	return h.deleteWorkspaceForOrigin(ctx, wsID, origin, nil, inject)
}

// deleteWorkspaceForOrigin is the teardown shared by the period-ended and the
// staff deletion. admit, when set, runs in the transaction before the teardown
// and reports whether the deletion still stands. The transaction holds the
// whole teardown, its audit row and the commit; the side effects follow the commit.
func (h *Handler) deleteWorkspaceForOrigin(ctx context.Context, wsID pgtype.UUID, origin DeletionOrigin, admit func(qtx *db.Queries) (bool, error), inject func(stage string) error) (bool, error) {
	workspaceID := uuidToString(wsID)
	if err := h.prepareQoderWorkspaceDeletion(ctx, workspaceID); err != nil {
		return false, fmt.Errorf("qoder cleanup is pending: %w", err)
	}
	var affectedUserIDs []string
	if members, err := h.Queries.ListMembers(ctx, wsID); err == nil {
		affectedUserIDs = make([]string, 0, len(members))
		for _, m := range members {
			userID := uuidToString(m.UserID)
			h.MembershipCache.Invalidate(ctx, userID, workspaceID)
			affectedUserIDs = append(affectedUserIDs, userID)
		}
	}

	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)
	if admit != nil {
		ok, err := admit(qtx)
		if err != nil {
			return false, fmt.Errorf("lock schedule: %w", err)
		}
		if !ok {
			return false, nil
		}
	}

	// The tenant is read before the teardown removes its link. It goes on the
	// audit row, which keeps it once the workspace is gone.
	var tenantID string
	if origin.Kind == DeletionOriginStaff {
		tid, err := qtx.GetTenantIDByWorkspace(ctx, wsID)
		switch {
		case err == nil:
			tenantID = uuidToString(tid)
		case !errors.Is(err, pgx.ErrNoRows):
			return false, fmt.Errorf("read tenant: %w", err)
		}
	}

	teardown, err := h.deleteWorkspaceData(ctx, tx, wsID)
	if errors.Is(err, pgx.ErrNoRows) {
		// The workspace row is gone: a parallel run deleted it.
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if inject != nil {
		if err := inject("audit"); err != nil {
			return false, err
		}
	}
	if err := audit.Record(ctx, auditstore.NewTx(h.Queries, tx), origin.workspaceDeletedEntry(workspaceID, tenantID, teardown.Name)); err != nil {
		return false, fmt.Errorf("audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}

	for _, runtimeID := range teardown.RuntimeIDs {
		h.NotifyRuntimeGone(uuidToString(runtimeID))
	}
	h.deleteS3Objects(ctx, teardown.ObjectURLs)
	h.deleteHubObjects(ctx, workspaceID)
	h.publish(protocol.EventWorkspaceDeleted, workspaceID, "system", "", map[string]any{
		"workspace_id": workspaceID,
	})
	h.notifyDaemonWorkspacesChanged(affectedUserIDs...)
	return true, nil
}
