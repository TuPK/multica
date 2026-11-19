package console

import (
	"errors"
	"net/http"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/staff"
)

// Workspace list and export of spec R6 section 3b.2 (QT2).

// exportAllowedStatuses are the workspace states an export may start from. A
// workspace pending deletion is being removed, so it is refused (spec rule 4).
var exportAllowedStatuses = []string{"active", "read_only", "suspended"}

var errExportInProgress = errors.New("export in progress")

// listWorkspaces returns one page of the workspaces, newest first (M33). An
// admin sees its own tenant's workspaces only.
func (h *Handler) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionWorkspacesList)
	if !ok {
		return
	}
	q := r.URL.Query()
	p, perr := parseListParams(q)
	if perr != nil {
		writeError(w, perr)
		return
	}
	pattern, perr := likePattern(q.Get("q"))
	if perr != nil {
		writeError(w, perr)
		return
	}
	tenant := q.Get("tenant_id")
	if tenant != "" {
		if _, err := parseUUID(tenant); err != nil {
			writeError(w, badRequest("invalid_tenant", "tenant_id must be a tenant id"))
			return
		}
	}
	rows, err := h.Reader.SearchWorkspaces(r.Context(), WorkspaceSearch{
		Pattern: pattern, Status: q.Get("status"), PlanKey: q.Get("plan_key"), TenantFilter: tenant,
		Cursor: p.cursor, Limit: p.limit, Scope: scope,
	})
	if err != nil {
		h.fail(w, r, "search workspaces", err)
		return
	}
	rows, next := nextCursor(rows, p.limit, func(x WorkspaceListRow) (time.Time, string) { return x.CreatedAt, x.ID })
	out := make([]map[string]any, len(rows))
	for i, x := range rows {
		out[i] = map[string]any{
			"id": x.ID, "name": x.Name, "slug": x.Slug, "tenant_id": x.TenantID, "tenant_name": x.TenantName,
			"status": x.Status, "plan_key": x.PlanKey, "member_count": x.MemberCount, "created_at": ts(x.CreatedAt),
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspaces": out, "next_cursor": next})
}

// exportWorkspace queues one export of a workspace's data (M34, D91). It answers
// 202 with the queued row's id. A second request while one is queued or running
// is 409 export_in_progress; a workspace pending deletion is 409 workspace_state_conflict.
func (h *Handler) exportWorkspace(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionWorkspaceExport)
	if !ok {
		return
	}
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	if !h.visibleWorkspace(w, r, scope, id) {
		return
	}
	current, found, err := h.Reader.GetWorkspace(r.Context(), id)
	if err != nil {
		h.fail(w, r, "get workspace", err)
		return
	}
	if !found {
		writeError(w, notFound())
		return
	}
	if !containsString(exportAllowedStatuses, current.Status) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "the workspace state does not allow this action", "code": "workspace_state_conflict",
			"status": current.Status, "allowed": exportAllowedStatuses,
		})
		return
	}
	a, err := h.currentActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	tenant, err := h.Reader.WorkspaceTenant(r.Context(), id)
	if err != nil {
		h.fail(w, r, "workspace tenant", err)
		return
	}
	var exportID string
	err = h.Runner.InTx(r.Context(), func(tx Tx) error {
		busy, err := tx.RunningExport(r.Context(), id)
		if err != nil {
			return err
		}
		if busy {
			return errExportInProgress
		}
		exportID, err = tx.InsertExport(r.Context(), id, a.ID)
		if err != nil {
			return err
		}
		return audit.Record(r.Context(), tx, a.staffEntry(audit.Entry{
			WorkspaceID: id, TenantID: tenant, Action: audit.ActionWorkspaceExportRequested,
			TargetType: targetWorkspace, TargetID: id, TargetLabel: current.Slug,
			Detail: map[string]any{"export_id": exportID},
		}))
	})
	if errors.Is(err, errExportInProgress) {
		writeError(w, &apiError{Status: http.StatusConflict, Code: "export_in_progress", Message: "an export of this workspace is already queued or running"})
		return
	}
	if err != nil {
		h.fail(w, r, "request workspace export", err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"export_id": exportID, "status": "queued"})
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
