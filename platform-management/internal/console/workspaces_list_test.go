package console

import (
	"net/http"
	"testing"
)

// Spec R6 section 3b.2 (QT2): GET /workspaces lists every workspace for the
// staff roles that may see all, and only the admin's tenant for admin; export
// is one queued row, refused while one is running and while the workspace is
// pending deletion.

func TestWorkspaceListIsScopedForAdminAndFiltered(t *testing.T) {
	e := newEnv(t)
	e.reader.workspaceList = []WorkspaceListRow{
		{ID: wsID, Name: "Acme", Slug: "acme", TenantID: tenantID, Status: "active", PlanKey: "free", CreatedAt: t0},
		{ID: foreignWSID, Name: "Far", Slug: "far", TenantID: otherTenantID, Status: "active", PlanKey: "free", CreatedAt: t0},
	}
	rec := e.do(tadminID, "GET", "/api/platform/workspaces?status=active", "")
	wantStatus(t, rec, http.StatusOK)
	rows, _ := decode(t, rec)["workspaces"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["id"] != wsID {
		t.Fatalf("admin saw %v, want only its own tenant's workspace", rows)
	}
	if e.reader.lastWorkspaces.Scope.TenantID != tenantID || e.reader.lastWorkspaces.Status != "active" {
		t.Fatalf("search = %+v", e.reader.lastWorkspaces)
	}

	rec = e.do(adminID, "GET", "/api/platform/workspaces", "")
	wantStatus(t, rec, http.StatusOK)
	if rows, _ := decode(t, rec)["workspaces"].([]any); len(rows) != 2 {
		t.Fatalf("system_admin saw %d workspaces, want 2", len(rows))
	}
}

func TestWorkspaceListOtherTenantFilterIsEmptyForAdmin(t *testing.T) {
	e := newEnv(t)
	e.reader.workspaceList = []WorkspaceListRow{
		{ID: foreignWSID, Name: "Far", Slug: "far", TenantID: otherTenantID, Status: "active", CreatedAt: t0},
	}
	rec := e.do(tadminID, "GET", "/api/platform/workspaces?tenant_id="+otherTenantID, "")
	wantStatus(t, rec, http.StatusOK)
	if rows, _ := decode(t, rec)["workspaces"].([]any); len(rows) != 0 {
		t.Fatalf("admin saw another tenant's workspace: %v", rows)
	}
}

func TestWorkspaceExportIsQueuedOnceAndRefusedWhenRunningOrDeleting(t *testing.T) {
	e := newEnv(t)
	path := "/api/platform/workspaces/" + wsID + "/export"

	wantUnknownRouteOrNotFound(t, e.do(suppID, "POST", path, "{}"))
	wantUnknownRouteOrNotFound(t, e.do(billID, "POST", path, "{}"))

	rec := e.do(tadminID, "POST", path, "{}")
	wantStatus(t, rec, http.StatusAccepted)
	if len(e.store.exports) != 1 || e.store.exports[0].by != tadminID {
		t.Fatalf("export rows = %+v", e.store.exports)
	}
	n := 0
	for _, a := range e.store.audits {
		if a.Action == "workspace.export_requested" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("export audit rows = %d", n)
	}

	e.store.running[wsID] = true
	wantCode(t, e.do(tadminID, "POST", path, "{}"), 409, "export_in_progress")
	delete(e.store.running, wsID)

	e.reader.details[wsID] = WorkspaceDetail{WorkspaceRow: WorkspaceRow{ID: wsID, Name: wsName, Slug: "acme", Status: "pending_deletion", CreatedAt: t0, LastActivityAt: t0}}
	wantCode(t, e.do(tadminID, "POST", path, "{}"), 409, "workspace_state_conflict")
	if len(e.store.exports) != 1 {
		t.Fatalf("a refused export wrote a row")
	}
}
