package console

import (
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/usage"
)

func i64(n int64) *int64 { return &n }

// The old workspace routes under /tenants/{id} no longer exist (spec R6 section
// 5.8, D77): not for any role, and without an alias.
func TestOldTenantSubroutesAreNotFound(t *testing.T) {
	for _, who := range []string{adminID, tadminID, suppID, billID} {
		e := newEnv(t)
		for _, c := range []struct{ method, path, body string }{
			{"POST", "/api/platform/tenants/" + wsID + "/status", `{"status":"read_only","reason":"r"}`},
			{"GET", "/api/platform/tenants/" + wsID + "/usage", ""},
			{"GET", "/api/platform/tenants/" + wsID + "/audit-events", ""},
			{"GET", "/api/platform/tenants/" + wsID + "/notes", ""},
			{"POST", "/api/platform/tenants/" + wsID + "/notes", `{"body":"hello"}`},
			{"PUT", "/api/platform/tenants/" + wsID + "/subscription", `{}`},
			{"GET", "/api/platform/tenants/" + wsID + "/billing-profile", ""},
		} {
			wantUnknownRoute(t, e.do(who, c.method, c.path, c.body))
		}
		if len(e.ws.calls) != 0 || e.store.txs != 0 {
			t.Errorf("%s: an old route reached the store", who)
		}
	}
}

func TestTenantListJSONHasWorkspaceCountAndMaxWorkspaces(t *testing.T) {
	e := newEnv(t)
	e.reader.tenantRows = []TenantRow{
		{ID: "bbbbbbbb-0000-0000-0000-000000000003", Name: "C", Status: "active", WorkspaceCount: 3, MaxWorkspaces: i64(10), BestPlanKey: "business",
			StatusSummary: map[string]int64{"active": 2, "suspended": 1}, CreatedAt: t0.Add(2 * time.Hour), LastActivityAt: t0},
		{ID: "bbbbbbbb-0000-0000-0000-000000000002", Name: "B", Status: "active", WorkspaceCount: 1, BestPlanKey: "enterprise",
			StatusSummary: map[string]int64{"active": 1}, CreatedAt: t0.Add(time.Hour), LastActivityAt: t0},
		{ID: "bbbbbbbb-0000-0000-0000-000000000001", Name: "A", Status: "active", WorkspaceCount: 1, MaxWorkspaces: i64(1), BestPlanKey: "free", CreatedAt: t0, LastActivityAt: t0},
	}
	rec := e.do(suppID, "GET", "/api/platform/tenants?q=50%25_off&limit=2", "")
	wantStatus(t, rec, 200)
	q := e.reader.lastTenants
	if q.Limit != 2 || q.Pattern == nil || *q.Pattern != `%50\%\_off%` || !q.Scope.All {
		t.Fatalf("search = %+v", q)
	}
	m := decode(t, rec)
	rows, _ := m["tenants"].([]any)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want the page of 2", len(rows))
	}
	first, _ := rows[0].(map[string]any)
	second, _ := rows[1].(map[string]any)
	summary, _ := first["status_summary"].(map[string]any)
	if first["name"] != "C" || first["workspace_count"] != float64(3) || first["max_workspaces"] != float64(10) ||
		first["best_plan_key"] != "business" || summary["suspended"] != float64(1) || first["last_activity_at"] == nil {
		t.Fatalf("row = %v", first)
	}
	// An empty limit is no limit: null, not 0.
	if v, ok := second["max_workspaces"]; !ok || v != nil {
		t.Errorf("max_workspaces = %v (present %v), want null", v, ok)
	}
	next, _ := m["next_cursor"].(string)
	cur, err := decodeCursor(next)
	if err != nil || cur.ID != "bbbbbbbb-0000-0000-0000-000000000002" || !cur.CreatedAt.Equal(t0.Add(time.Hour)) {
		t.Fatalf("next cursor = %q %+v %v", next, cur, err)
	}

	// The cursor goes back to the reader; a page that fits has no next cursor.
	e.reader.tenantRows = e.reader.tenantRows[2:]
	rec = e.do(adminID, "GET", "/api/platform/tenants?limit=2&cursor="+next, "")
	wantStatus(t, rec, 200)
	if e.reader.lastTenants.Cursor == nil || e.reader.lastTenants.Cursor.ID != cur.ID {
		t.Fatalf("cursor was not passed: %+v", e.reader.lastTenants)
	}
	if decode(t, rec)["next_cursor"] != nil {
		t.Fatal("the last page has a next cursor")
	}
	wantCode(t, e.do(adminID, "GET", "/api/platform/tenants?limit=0", ""), 400, "invalid_filter")
	wantCode(t, e.do(adminID, "GET", "/api/platform/tenants?cursor=zzz", ""), 400, "invalid_cursor")
}

func TestTenantDetailListsWorkspacesWithTheirSummary(t *testing.T) {
	e := newEnv(t)
	d := e.reader.tenantDetails[tenantID]
	d.MaxWorkspaces = i64(3)
	d.CreatedBy = adminID
	d.Workspaces = []TenantWorkspace{
		{ID: wsID, Name: wsName, Slug: "acme", Status: "active", PlanKey: "pro", MemberCount: 4, OwnerID: custID, OwnerEmail: custMail, CreatedAt: t0,
			Usage: usage.Snapshot{Day: dayPtr("2026-11-14"), Members: 3, TasksRun: 9}},
		{ID: "bbbbbbbb-0000-0000-0000-000000000002", Name: "Second", Slug: "second", Status: "suspended", PlanKey: "free", MemberCount: 1, CreatedAt: t0},
	}
	e.reader.tenantDetails[tenantID] = d

	rec := e.do(suppID, "GET", "/api/platform/tenants/"+tenantID, "")
	wantStatus(t, rec, 200)
	m := decode(t, rec)
	if m["id"] != tenantID || m["name"] != "Acme tenant" || m["max_workspaces"] != float64(3) {
		t.Fatalf("tenant = %v", m)
	}
	ws, _ := m["workspaces"].([]any)
	if len(ws) != 2 {
		t.Fatalf("workspaces = %v", m["workspaces"])
	}
	first, _ := ws[0].(map[string]any)
	owner, _ := first["owner"].(map[string]any)
	if first["id"] != wsID || first["plan_key"] != "pro" || first["status"] != "active" || first["member_count"] != float64(4) ||
		owner["email"] != custMail || first["usage_members"] != float64(3) || first["usage_day"] != "2026-11-14" {
		t.Fatalf("workspace = %v", first)
	}
	second, _ := ws[1].(map[string]any)
	if v, ok := second["owner"]; !ok || v != nil {
		t.Errorf("a workspace with no owner has owner %v (present %v), want null", v, ok)
	}
	// Only these fields leave the console for a workspace (isolation rule I6).
	wantKeys := map[string]bool{
		"id": true, "name": true, "slug": true, "status": true, "plan_key": true, "member_count": true, "owner": true, "created_at": true,
		"usage_day": true, "usage_members": true, "usage_agents": true, "usage_runtimes": true, "usage_issues_total": true,
		"usage_issues_created": true, "usage_tasks_run": true, "usage_storage_bytes": true,
	}
	for key := range first {
		if !wantKeys[key] {
			t.Errorf("the workspace summary has an unexpected field %q", key)
		}
	}
	wantCode(t, e.do(suppID, "GET", "/api/platform/tenants/"+otherTenantID, ""), 404, "not_found")
	wantCode(t, e.do(suppID, "GET", "/api/platform/tenants/nope", ""), 404, "not_found")
}

// --- admin scope with fakes: the handler passes the scope and refuses what is
// outside it. The queries themselves are checked on the database.

func TestAdminSeesOnlyOwnTenantInTheListRequest(t *testing.T) {
	e := newEnv(t)
	e.reader.tenantRows = []TenantRow{
		{ID: tenantID, Name: "Mine", WorkspaceCount: 1, CreatedAt: t0, LastActivityAt: t0},
		{ID: otherTenantID, Name: "Other", WorkspaceCount: 1, CreatedAt: t0, LastActivityAt: t0},
	}
	rec := e.do(tadminID, "GET", "/api/platform/tenants", "")
	wantStatus(t, rec, 200)
	if got := e.reader.lastTenants.Scope; got.All || got.TenantID != tenantID {
		t.Fatalf("scope = %+v, want the admin's tenant", got)
	}
	rows, _ := decode(t, rec)["tenants"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["id"] != tenantID {
		t.Fatalf("rows = %v, want only the admin's tenant", rows)
	}
	// support sees both.
	e.do(suppID, "GET", "/api/platform/tenants", "")
	if !e.reader.lastTenants.Scope.All {
		t.Fatalf("scope for support = %+v, want all", e.reader.lastTenants.Scope)
	}
}

func TestAdminOutOfScopeObjectsAnswerTheSame404AsMissingOnes(t *testing.T) {
	e := newEnv(t)
	const foreignWS = "bbbbbbbb-0000-0000-0000-0000000000f1"
	const missingWS = "bbbbbbbb-0000-0000-0000-0000000000f2"
	e.reader.wsTenant[foreignWS] = otherTenantID
	e.reader.details[foreignWS] = WorkspaceDetail{WorkspaceRow: WorkspaceRow{ID: foreignWS, Name: "Foreign", Slug: "f", Status: "active", CreatedAt: t0, LastActivityAt: t0}}
	e.reader.tenantDetails[otherTenantID] = TenantDetail{TenantRow: TenantRow{ID: otherTenantID, Name: "Other tenant"}}

	for _, c := range []struct{ method, foreign, missing, body string }{
		{"GET", "/api/platform/tenants/" + otherTenantID, "/api/platform/tenants/eeeeeeee-0000-0000-0000-0000000000ff", ""},
		{"GET", "/api/platform/workspaces/" + foreignWS, "/api/platform/workspaces/" + missingWS, ""},
		{"GET", "/api/platform/workspaces/" + foreignWS + "/usage", "/api/platform/workspaces/" + missingWS + "/usage", ""},
		{"GET", "/api/platform/workspaces/" + foreignWS + "/notes", "/api/platform/workspaces/" + missingWS + "/notes", ""},
		{"POST", "/api/platform/workspaces/" + foreignWS + "/notes", "/api/platform/workspaces/" + missingWS + "/notes", `{"body":"x"}`},
		{"POST", "/api/platform/workspaces/" + foreignWS + "/status", "/api/platform/workspaces/" + missingWS + "/status", `{"status":"suspended","reason":"r"}`},
		{"GET", "/api/platform/workspaces/" + foreignWS + "/audit-events", "/api/platform/workspaces/" + missingWS + "/audit-events", ""},
		{"GET", "/api/platform/users/" + suppID, "/api/platform/users/" + noOneID, ""},
	} {
		a := e.do(tadminID, c.method, c.foreign, c.body)
		b := e.do(tadminID, c.method, c.missing, c.body)
		if a.Code != 404 || b.Code != 404 || a.Body.String() != b.Body.String() {
			t.Errorf("%s %s: foreign = %d %q, missing = %d %q; want the same 404", c.method, c.foreign, a.Code, a.Body.String(), b.Code, b.Body.String())
		}
	}
	if len(e.store.audits) != 0 || len(e.ws.calls) != 0 || len(e.store.notes) != 0 {
		t.Errorf("a refused request wrote something: %d audits, %d state changes, %d notes", len(e.store.audits), len(e.ws.calls), len(e.store.notes))
	}
}

func TestAdminLocksUnlocksAndSchedulesDeletionInOwnTenantOnly(t *testing.T) {
	e := newEnv(t)
	path := "/api/platform/workspaces/" + wsID + "/status"
	for _, status := range []string{"suspended", "read_only", "active", "pending_deletion"} {
		wantStatus(t, e.do(tadminID, "POST", path, `{"status":"`+status+`","reason":"r"}`), 200)
	}
	if len(e.ws.calls) != 4 {
		t.Fatalf("Set calls = %d, want 4", len(e.ws.calls))
	}
	// support locks and unlocks, and cannot schedule a deletion; billing does none of it.
	e = newEnv(t)
	wantCode(t, e.do(suppID, "POST", path, `{"status":"pending_deletion","reason":"r"}`), 403, "forbidden")
	wantStatus(t, e.do(suppID, "POST", path, `{"status":"suspended","reason":"r"}`), 200)
	wantUnknownRoute(t, e.do(billID, "POST", path, `{"status":"suspended","reason":"r"}`))
}

func TestAdminOrderRequestsCarryTheScopeAndWritesAreRefused(t *testing.T) {
	e := newEnv(t)
	wantStatus(t, e.do(tadminID, "GET", "/api/platform/orders", ""), 200)
	if got := e.reader.lastOrderScope; got.All || got.TenantID != tenantID {
		t.Fatalf("order scope = %+v, want the admin's tenant", got)
	}
	e.do(billID, "GET", "/api/platform/orders", "")
	if !e.reader.lastOrderScope.All {
		t.Fatalf("order scope for billing = %+v, want all", e.reader.lastOrderScope)
	}
	// No write route of orders, and no payment route, is open to the admin.
	seedOrder(e, "77777777-0000-0000-0000-000000000001", billing.StatusPending, t0)
	for _, c := range []struct{ method, path, body string }{
		{"POST", "/api/platform/orders/77777777-0000-0000-0000-000000000001/confirm", `{"transfer_reference":"x"}`},
		{"POST", "/api/platform/orders/77777777-0000-0000-0000-000000000001/cancel", `{"reason":"r"}`},
		{"GET", "/api/platform/payment-notices", ""},
		{"GET", "/api/platform/einvoices", ""},
		{"GET", "/api/platform/accounts", ""},
		{"POST", "/api/platform/accounts", `{}`},
		{"GET", "/api/platform/overview", ""},
		{"POST", "/api/platform/users/" + custID + "/status", `{"status":"suspended","reason":"r"}`},
		{"POST", "/api/platform/users/" + custID + "/revoke-sessions", `{"reason":"r"}`},
		{"POST", "/api/platform/users/" + custID + "/mfa-reset", ""},
		{"GET", "/api/platform/staff", ""},
	} {
		wantUnknownRoute(t, e.do(tadminID, c.method, c.path, c.body))
	}
	if e.store.txs != 0 {
		t.Error("a refused request opened a transaction")
	}
}

func TestUsersAndAuditRequestsCarryTheAdminsScope(t *testing.T) {
	e := newEnv(t)
	wantStatus(t, e.do(tadminID, "GET", "/api/platform/users", ""), 200)
	if got := e.reader.lastUsers.Scope; got.All || got.TenantID != tenantID {
		t.Fatalf("users scope = %+v", got)
	}
	wantStatus(t, e.do(tadminID, "GET", "/api/platform/audit-events", ""), 200)
	if got := e.reader.lastAudit.Scope; got.All || got.TenantID != tenantID {
		t.Fatalf("audit scope = %+v", got)
	}
	for _, who := range []string{suppID, billID, adminID} {
		e.do(who, "GET", "/api/platform/audit-events", "")
		if !e.reader.lastAudit.Scope.All {
			t.Errorf("%s: audit scope = %+v, want all", who, e.reader.lastAudit.Scope)
		}
		e.do(who, "GET", "/api/platform/users", "")
		if !e.reader.lastUsers.Scope.All {
			t.Errorf("%s: users scope = %+v, want all", who, e.reader.lastUsers.Scope)
		}
	}
	// An admin whose user is not a member of the tenant's workspaces gets 404 for them.
	wantCode(t, e.do(tadminID, "GET", "/api/platform/users/"+suppID, ""), 404, "not_found")
}

func TestAddAndUpdateStaffRequireAnExistingTenant(t *testing.T) {
	e := newEnv(t)
	e.store.tenants = map[string]bool{tenantID: true}
	wantCode(t, e.do(adminID, "POST", "/api/platform/staff", `{"email":"`+custMail+`","role":"admin","tenant_id":"`+otherTenantID+`"}`), 400, "tenant_invalid")
	wantCode(t, e.do(adminID, "PUT", "/api/platform/staff/"+suppID, `{"role":"admin","tenant_id":"`+otherTenantID+`"}`), 400, "tenant_invalid")
	if len(e.store.audits) != 0 || len(e.store.staffRows) != defaultStaffRows {
		t.Fatal("a refused request changed staff")
	}
	wantStatus(t, e.do(adminID, "PUT", "/api/platform/staff/"+suppID, `{"role":"admin","tenant_id":"`+tenantID+`"}`), 200)
}
