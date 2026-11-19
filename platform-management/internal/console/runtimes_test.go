package console

import (
	"net/http"
	"testing"
)

// Spec R6 section 3b.5 (QT5): runtimes for staff. Lists show no device details
// or addresses; disable writes one audit row and clears on enable; admin sees
// its tenant only; a runtime request needs the runtime's name as confirmation.

const (
	rtMine  = "7b000000-0000-0000-0000-000000000001"
	rtOther = "7b000000-0000-0000-0000-000000000002"
)

func runtimeEnv(t *testing.T) *env {
	t.Helper()
	e := newEnv(t)
	seen := t0
	e.reader.runtimes = []RuntimeRow{
		{ID: rtMine, Name: "laptop", WorkspaceID: wsID, TenantID: tenantID, Status: "online", OS: "linux", DaemonVersion: "0.9.1", LastSeenAt: &seen, CreatedAt: t0},
		{ID: rtOther, Name: "far", WorkspaceID: foreignWSID, TenantID: otherTenantID, Status: "offline", OS: "darwin", CreatedAt: t0},
	}
	e.reader.runtimeTasks = map[string][]RuntimeTask{rtMine: {{ID: "t1", Status: "failed", HasError: true, CreatedAt: t0}}}
	e.reader.runtimeFailed = 2
	return e
}

func TestRuntimeListIsScopedAndHidesDeviceDetails(t *testing.T) {
	e := runtimeEnv(t)
	rec := e.do(tadminID, "GET", "/api/platform/runtimes", "")
	wantStatus(t, rec, http.StatusOK)
	rows, _ := decode(t, rec)["runtimes"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["id"] != rtMine {
		t.Fatalf("admin saw %v, want its tenant's runtime only", rows)
	}
	for _, key := range []string{"device_info", "metadata", "ip"} {
		if _, ok := rows[0].(map[string]any)[key]; ok {
			t.Fatalf("list row carries %q", key)
		}
	}
	rec = e.do(adminID, "GET", "/api/platform/runtimes", "")
	if rows, _ := decode(t, rec)["runtimes"].([]any); len(rows) != 2 {
		t.Fatalf("system_admin saw %d runtimes, want 2", len(rows))
	}
}

func TestRuntimeBillingIsNotFound(t *testing.T) {
	e := runtimeEnv(t)
	wantUnknownRouteOrNotFound(t, e.do(billID, "GET", "/api/platform/runtimes", ""))
}

func TestRuntimeDetailHasRecentTasksWithoutContent(t *testing.T) {
	e := runtimeEnv(t)
	rec := e.do(tadminID, "GET", "/api/platform/runtimes/"+rtMine, "")
	wantStatus(t, rec, http.StatusOK)
	body := decode(t, rec)
	tasks, _ := body["tasks"].([]any)
	if len(tasks) != 1 || tasks[0].(map[string]any)["has_error"] != true || body["failed_tasks_7d"] != float64(2) {
		t.Fatalf("detail = %s", rec.Body.String())
	}
	if _, ok := tasks[0].(map[string]any)["error"]; ok {
		t.Fatal("a recent run carries its error text")
	}
	wantUnknownRouteOrNotFound(t, e.do(tadminID, "GET", "/api/platform/runtimes/"+rtOther, ""))
}

func TestRuntimeDisableIsIdempotentAndAuditedOnce(t *testing.T) {
	e := runtimeEnv(t)
	path := "/api/platform/runtimes/" + rtMine + "/status"
	rec := e.do(tadminID, "POST", path, `{"status":"disabled","reason":"abuse report"}`)
	wantStatus(t, rec, http.StatusOK)
	if decode(t, rec)["changed"] != true || !e.store.runtimeDisabled[rtMine] {
		t.Fatalf("first disable: %s", rec.Body.String())
	}
	rec = e.do(tadminID, "POST", path, `{"status":"disabled","reason":"abuse report"}`)
	if decode(t, rec)["changed"] != false {
		t.Fatalf("second disable must change nothing: %s", rec.Body.String())
	}
	n := 0
	for _, a := range e.store.audits {
		if a.Action == "runtime.disabled" && a.TenantID == tenantID && a.WorkspaceID == wsID {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("runtime.disabled audit rows = %d, want 1", n)
	}
	rec = e.do(tadminID, "POST", path, `{"status":"enabled","reason":"resolved"}`)
	if decode(t, rec)["changed"] != true || e.store.runtimeDisabled[rtMine] {
		t.Fatalf("enable must clear the disabled state: %s", rec.Body.String())
	}
}

func TestRuntimeStatusOutsideTenantIsNotFound(t *testing.T) {
	e := runtimeEnv(t)
	wantUnknownRouteOrNotFound(t, e.do(tadminID, "POST", "/api/platform/runtimes/"+rtOther+"/status", `{"status":"disabled","reason":"r"}`))
	if e.store.runtimeDisabled[rtOther] {
		t.Fatal("a runtime outside the tenant was disabled")
	}
}

func TestRuntimeUnregisterGoesThroughDeletionRequest(t *testing.T) {
	e := runtimeEnv(t)
	rec := e.do(tadminID, "POST", "/api/platform/runtimes/"+rtMine+"/unregister", `{"reason":"retired machine","confirm":"laptop"}`)
	wantStatus(t, rec, http.StatusAccepted)
	if len(e.delete.runtimes) != 1 || e.delete.runtimes[0].id != rtMine || e.delete.runtimes[0].in.Confirm != "laptop" {
		t.Fatalf("deletion requests = %+v", e.delete.runtimes)
	}
	wantUnknownRouteOrNotFound(t, e.do(billID, "POST", "/api/platform/runtimes/"+rtMine+"/unregister", `{"reason":"r","confirm":"laptop"}`))
}
