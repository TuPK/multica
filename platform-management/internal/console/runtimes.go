package console

import (
	"net/http"
	"strings"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/deletionrequest"
	"github.com/multica-ai/multica/platform-management/internal/staff"
)

// Runtimes of the Delegora daemons for staff (spec R6 section 3b.5, QT5). The
// list and detail show status and versions, never the device details, metadata
// or addresses (rule 2). Disable and enable write platform_runtime; unregister
// is a deletion request that Delegora's staff_deletion job runs (rule 5).

// targetRuntime is the audit target type of a runtime.
const targetRuntime = "runtime"

// runtimeStatuses are the agent_runtime states a list may filter by.
var runtimeStatuses = []string{"online", "offline"}

// osOf reduces device_info to the operating system: its first word, lower-cased.
func osOf(deviceInfo string) string {
	fields := strings.Fields(deviceInfo)
	if len(fields) == 0 {
		return ""
	}
	return strings.ToLower(fields[0])
}

func runtimeJSON(r RuntimeRow) map[string]any {
	out := map[string]any{
		"id": r.ID, "name": r.Name, "workspace_id": r.WorkspaceID, "tenant_id": r.TenantID, "status": r.Status,
		"os": r.OS, "daemon_version": r.DaemonVersion, "agent_version": r.AgentVersion, "owner_id": r.OwnerID,
		"disabled": r.Disabled, "running_tasks": r.RunningTasks, "created_at": ts(r.CreatedAt),
	}
	if r.LastSeenAt != nil {
		out["last_seen_at"] = ts(*r.LastSeenAt)
	} else {
		out["last_seen_at"] = nil
	}
	return out
}

// runtimeVisible reports whether the scope may see a runtime of the given tenant.
func runtimeVisible(scope Scope, tenantID string) bool {
	return scope.All || (scope.TenantID != "" && scope.TenantID == tenantID)
}

// listRuntimes returns one page of runtimes, newest first (M41). An admin sees
// the runtimes of its tenant's workspaces only.
func (h *Handler) listRuntimes(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionRuntimesRead)
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
	status := q.Get("status")
	if status != "" && !containsString(runtimeStatuses, status) {
		writeError(w, badRequest("invalid_status", "status must be online or offline"))
		return
	}
	for _, key := range []string{"tenant_id", "workspace_id"} {
		if v := q.Get(key); v != "" {
			if _, err := parseUUID(v); err != nil {
				writeError(w, badRequest("invalid_"+key, key+" must be an id"))
				return
			}
		}
	}
	rows, err := h.Reader.SearchRuntimes(r.Context(), RuntimeSearch{
		Pattern: pattern, Status: status, TenantFilter: q.Get("tenant_id"), WorkspaceFilter: q.Get("workspace_id"),
		DisabledOnly: q.Get("disabled") == "true", Cursor: p.cursor, Limit: p.limit, Scope: scope,
	})
	if err != nil {
		h.fail(w, r, "search runtimes", err)
		return
	}
	rows, next := nextCursor(rows, p.limit, func(x RuntimeRow) (time.Time, string) { return x.CreatedAt, x.ID })
	out := make([]map[string]any, len(rows))
	for i, x := range rows {
		out[i] = runtimeJSON(x)
	}
	writeJSON(w, http.StatusOK, map[string]any{"runtimes": out, "next_cursor": next})
}

// visibleRuntime reads one runtime the scope may see; anything else is the flat 404.
func (h *Handler) visibleRuntime(w http.ResponseWriter, r *http.Request, scope Scope, id string) (RuntimeRow, bool) {
	row, found, err := h.Reader.RuntimeDetail(r.Context(), id)
	if err != nil {
		h.fail(w, r, "get runtime", err)
		return RuntimeRow{}, false
	}
	if !found || !runtimeVisible(scope, row.TenantID) {
		writeError(w, notFound())
		return RuntimeRow{}, false
	}
	return row, true
}

// getRuntime returns one runtime with its newest tasks and seven-day failures (M41).
func (h *Handler) getRuntime(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionRuntimesRead)
	if !ok {
		return
	}
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	row, ok := h.visibleRuntime(w, r, scope, id)
	if !ok {
		return
	}
	tasks, failed, err := h.Reader.RuntimeTasks(r.Context(), id)
	if err != nil {
		h.fail(w, r, "list runtime recent runs", err)
		return
	}
	out := make([]map[string]any, len(tasks))
	for i, t := range tasks {
		item := map[string]any{"id": t.ID, "status": t.Status, "has_error": t.HasError, "created_at": ts(t.CreatedAt)}
		if t.CompletedAt != nil {
			item["completed_at"] = ts(*t.CompletedAt)
		} else {
			item["completed_at"] = nil
		}
		out[i] = item
	}
	body := runtimeJSON(row)
	body["tasks"] = out
	body["failed_tasks_7d"] = failed
	writeJSON(w, http.StatusOK, body)
}

// setRuntimeStatus disables or enables a runtime (M42). The write and its audit
// row commit together; a state that is already in place changes nothing.
func (h *Handler) setRuntimeStatus(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionRuntimeStatus)
	if !ok {
		return
	}
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	var body struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	}
	if perr := decodeBody(w, r, &body); perr != nil {
		writeError(w, perr)
		return
	}
	if body.Status != "disabled" && body.Status != "enabled" {
		writeError(w, badRequest("invalid_status", "status must be disabled or enabled"))
		return
	}
	reason, perr := cleanReason(body.Reason)
	if perr != nil {
		writeError(w, perr)
		return
	}
	row, ok := h.visibleRuntime(w, r, scope, id)
	if !ok {
		return
	}
	a, err := h.currentActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	disable := body.Status == "disabled"
	changed := false
	err = h.Runner.InTx(r.Context(), func(tx Tx) error {
		changed = false
		c, err := tx.SetRuntimeDisabled(r.Context(), id, disable, reason, a.ID)
		if err != nil {
			return err
		}
		if !c {
			return nil
		}
		changed = true
		action := audit.ActionRuntimeEnabled
		if disable {
			action = audit.ActionRuntimeDisabled
		}
		return audit.Record(r.Context(), tx, a.staffEntry(audit.Entry{
			WorkspaceID: row.WorkspaceID, TenantID: row.TenantID, Action: action,
			TargetType: targetRuntime, TargetID: id, TargetLabel: row.Name,
			Detail: map[string]any{"reason": reason},
		}))
	})
	if err != nil {
		h.fail(w, r, "set runtime status", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"changed": changed, "disabled": disable})
}

// unregisterRuntime asks for a runtime to be unregistered (M43). The confirmation
// is the runtime's name. It answers 202 for a new request and 200 for a live one.
func (h *Handler) unregisterRuntime(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionRuntimeUnregister)
	if !ok {
		return
	}
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	if _, ok := h.visibleRuntime(w, r, scope, id); !ok {
		return
	}
	var in deletionrequest.Input
	if perr := decodeBody(w, r, &in); perr != nil {
		writeError(w, perr)
		return
	}
	by, err := h.deletionActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	req, created, err := h.Deletions.RequestRuntime(r.Context(), by, id, in)
	if err != nil {
		h.failDeletion(w, r, "request runtime unregister", err)
		return
	}
	writeJSON(w, deleteStatus(created), deleteAnswer{RequestID: req.ID, State: req.State})
}
