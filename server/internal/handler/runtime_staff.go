package handler

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/server/internal/auditstore"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/util"
)

// StaffRuntimeError refuses a runtime unregister requested by platform staff.
// Code is the error_code the request records. The codes are the ones the owner's
// delete answers with 409 (spec R6 section 3b.5, D98).
type StaffRuntimeError struct{ Code string }

func (e *StaffRuntimeError) Error() string { return "staff runtime unregister refused: " + e.Code }

// Error codes of a staff runtime unregister.
const (
	StaffRuntimeErrManagedByConnection = "runtime_managed_by_connection"
	StaffRuntimeErrProfileInstanceLive = "profile_instance_live"
	StaffRuntimeErrHasActiveAgents     = "runtime_has_active_agents"
	StaffRuntimeErrWorkspaceMismatch   = "runtime_workspace_mismatch"
)

// DeleteRuntimeForStaff unregisters a runtime for a platform request. It runs
// the same teardown the owner's delete runs (DeleteAgentRuntime) and refuses
// what the owner's delete refuses. Staff cannot confirm a cascade, so a runtime
// with active agents is refused: its agents are reassigned first. A runtime that
// is already gone is done: it returns false with no error. A runtime still
// draining its tasks returns a plain error, which the job retries.
func (h *Handler) DeleteRuntimeForStaff(ctx context.Context, runtimeID string, origin DeletionOrigin) (bool, error) {
	id, err := util.ParseUUID(runtimeID)
	if err != nil {
		return false, fmt.Errorf("parse runtime id: %w", err)
	}
	rt, err := h.Queries.GetAgentRuntime(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load runtime: %w", err)
	}
	if managed, err := h.qoderManagesRuntime(ctx, rt); err != nil {
		return false, fmt.Errorf("check runtime connection: %w", err)
	} else if managed {
		return false, &StaffRuntimeError{Code: StaffRuntimeErrManagedByConnection}
	}
	if _, live, err := h.runtimeLiveProfile(ctx, rt); err != nil {
		return false, fmt.Errorf("check runtime profile: %w", err)
	} else if live {
		return false, &StaffRuntimeError{Code: StaffRuntimeErrProfileInstanceLive}
	}

	wsID := uuidToString(rt.WorkspaceID)
	tenantID := ""
	if t, err := h.Queries.GetTenantIDByWorkspace(ctx, rt.WorkspaceID); err == nil {
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
	if _, err := qtx.LockAgentRuntime(ctx, rt.ID); err != nil {
		return false, fmt.Errorf("lock runtime: %w", err)
	}
	if _, err := qtx.ListUserAgentsByRuntimeForUpdate(ctx, rt.ID); err != nil {
		return false, fmt.Errorf("lock runtime dependencies: %w", err)
	}
	active, err := qtx.ListActiveAgentsByRuntimeForUpdate(ctx, rt.ID)
	if err != nil {
		return false, fmt.Errorf("check runtime dependencies: %w", err)
	}
	if len(active) > 0 {
		return false, &StaffRuntimeError{Code: StaffRuntimeErrHasActiveAgents}
	}

	teardown, err := service.TeardownRuntime(ctx, qtx, rt.ID, service.RuntimeTeardownOptions{CancelNonTerminalTasks: true, SeatFlags: h.FeatureFlags})
	switch {
	case errors.Is(err, service.ErrRuntimeWorkspaceMismatch):
		return false, &StaffRuntimeError{Code: StaffRuntimeErrWorkspaceMismatch}
	case err != nil:
		// ErrRuntimeNotDrained and any database error: retried by the job.
		return false, fmt.Errorf("teardown runtime: %w", err)
	}
	if err := qtx.DeleteAgentRuntime(ctx, rt.ID); err != nil {
		return false, fmt.Errorf("delete runtime: %w", err)
	}

	// The owner's runtime.deleted row, with the staff actor and the tenant the
	// workspace belonged to.
	entry := runtimeDeletedEntry(rt, teardown)
	entry.TenantID = tenantID
	entry.ActorType = audit.ActorStaff
	entry.ActorID = origin.StaffID
	entry.ActorLabel = origin.StaffLabel
	entry.Detail["reason"] = "staff_request"
	entry.Detail["origin"] = string(DeletionOriginStaff)
	entry.Detail["request_id"] = origin.RequestID
	if err := audit.Record(ctx, auditstore.NewTx(h.Queries, tx), entry); err != nil {
		return false, fmt.Errorf("audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}

	h.NotifyRuntimeGone(uuidToString(rt.ID))
	h.PublishRuntimeTeardown(ctx, teardown, wsID, "system", "", "delete", true)
	return true, nil
}
