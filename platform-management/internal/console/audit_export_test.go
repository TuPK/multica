package console

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// Spec R6 section 3c.8 (QT18): audit export by range, streamed; at most 31 days;
// one audit row per export; staff roles only.

func auditExportEnv(t *testing.T) *env {
	t.Helper()
	e := newEnv(t)
	e.reader.events = []AuditRow{
		{ID: "a1", WorkspaceID: wsID, ActorType: "staff", ActorID: adminID, ActorLabel: adminMail, Action: "workspace.status_changed", CreatedAt: t0, Detail: []byte(`{"reason":"r"}`)},
		{ID: "a2", WorkspaceID: wsID, ActorType: "user", ActorID: custID, ActorLabel: custMail, Action: "auth.login", CreatedAt: t0, Detail: []byte(`{}`)},
	}
	return e
}

func exportPath(from, to time.Time, extra string) string {
	return "/api/platform/audit-events/export?from=" + from.Format(time.RFC3339) + "&to=" + to.Format(time.RFC3339) + extra
}

func TestAuditExportRangeLimits(t *testing.T) {
	e := auditExportEnv(t)
	from := t0.Add(-40 * 24 * time.Hour)
	wantCode(t, e.do(adminID, "GET", exportPath(from, t0, ""), ""), 400, "export_range_too_large")
	wantCode(t, e.do(adminID, "GET", exportPath(t0, t0.Add(-time.Hour), ""), ""), 400, "export_range_invalid")
}

func TestAuditExportNDJSONIsAuditedWithCount(t *testing.T) {
	e := auditExportEnv(t)
	rec := e.do(adminID, "GET", exportPath(t0.Add(-time.Hour), t0.Add(time.Hour), ""), "")
	wantStatus(t, rec, http.StatusOK)
	lines := strings.Split(strings.TrimSpace(rec.Body.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines = %d, body %q", len(lines), rec.Body.String())
	}
	found := false
	for _, a := range e.store.audits {
		if a.Action == "audit.exported" && a.ActorType == "staff" && a.Detail["rows"] == 2 {
			found = true
		}
	}
	if !found {
		t.Fatal("no audit.exported row that counts the rows")
	}
}

func TestAuditExportCSVHasHeader(t *testing.T) {
	e := auditExportEnv(t)
	rec := e.do(adminID, "GET", exportPath(t0.Add(-time.Hour), t0.Add(time.Hour), "&format=csv"), "")
	wantStatus(t, rec, http.StatusOK)
	if !strings.HasPrefix(rec.Body.String(), "id,created_at,actor_type,") {
		t.Fatalf("csv body = %q", rec.Body.String())
	}
}

func TestAuditExportIsStaffOnly(t *testing.T) {
	e := auditExportEnv(t)
	for _, u := range []string{suppID, billID} {
		wantUnknownRouteOrNotFound(t, e.do(u, "GET", exportPath(t0.Add(-time.Hour), t0.Add(time.Hour), ""), ""))
	}
}
