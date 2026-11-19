package console

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/staff"
)

// scopeWorld is two tenants with one workspace each, the people in them, and the
// staff that look at them.
type scopeWorld struct {
	*dbFixture
	t1, t2       string
	ws1, ws2     string
	owner1, mem1 string
	owner2, mem2 string
	admin1       string // admin of t1
	admin2       string // admin of t2
	system       string
	support      string
	billing      string
}

func newScopeWorld(t *testing.T) *scopeWorld {
	t.Helper()
	f := newDBFixture(t)
	w := &scopeWorld{dbFixture: f}
	w.t1, w.t2 = f.namedTenant("Scope one "+randomHex(t)[:8]), f.namedTenant("Scope two "+randomHex(t)[:8])
	w.owner1, _ = f.user("owner")
	w.mem1, _ = f.user("member")
	w.owner2, _ = f.user("owner")
	w.mem2, _ = f.user("member")
	w.ws1 = f.workspaceIn(w.t1, w.owner1, w.mem1)
	w.ws2 = f.workspaceIn(w.t2, w.owner2, w.mem2)
	w.admin1, _ = f.staffUserInTenant(staff.RoleAdmin, w.t1)
	w.admin2, _ = f.staffUserInTenant(staff.RoleAdmin, w.t2)
	w.system, _ = f.staffUser(staff.RoleSystemAdmin)
	w.support, _ = f.staffUser(staff.RoleSupport)
	w.billing, _ = f.staffUser(staff.RoleBilling)
	return w
}

func (w *scopeWorld) order(ws string) string {
	w.t.Helper()
	var id string
	if err := w.pool.QueryRow(context.Background(), `
		INSERT INTO billing_order (id, workspace_id, kind, plan_key, "interval", amount, currency, status, provider, transfer_code, expires_at)
		VALUES (gen_random_uuid(), $1, 'subscription', 'free', 'month', 0, 'VND', 'pending', 'bank_transfer', 'scope-' || gen_random_uuid()::text, now() + interval '1 day')
		RETURNING id::text`, ws).Scan(&id); err != nil {
		w.t.Fatalf("create order: %v", err)
	}
	w.t.Cleanup(func() { w.pool.Exec(context.Background(), `DELETE FROM billing_order WHERE id = $1`, id) })
	return id
}

func TestAdminSeesOnlyOwnTenantInList(t *testing.T) {
	w := newScopeWorld(t)
	got := tenantIDsOf(w.getJSON(w.admin1, "/api/platform/tenants?limit=100"))
	if len(got) != 1 || got[w.t1] == nil {
		t.Fatalf("admin of t1 sees %v, want only %s", keys(got), w.t1)
	}
	// A search does not reach past the tenant.
	if got := tenantIDsOf(w.getJSON(w.admin1, "/api/platform/tenants?limit=100&q=Scope%20two")); len(got) != 0 {
		t.Errorf("admin of t1 found %v by searching for tenant two", keys(got))
	}
	// Staff with access to every tenant see both.
	for name, who := range map[string]string{"system_admin": w.system, "support": w.support, "billing": w.billing} {
		got := tenantIDsOf(w.getJSON(who, "/api/platform/tenants?limit=100"))
		if got[w.t1] == nil || got[w.t2] == nil {
			t.Errorf("%s does not see both tenants", name)
		}
	}
}

// An object outside the admin's tenant answers exactly as one that does not
// exist, so the admin cannot tell "not there" from "not yours" (rule 1).
func TestAdminGetsSameNotFoundForForeignAndMissingObjects(t *testing.T) {
	w := newScopeWorld(t)
	foreignOrder, missingOrder := w.order(w.ws2), "99999999-0000-4000-8000-000000000001"
	const missing = "99999999-0000-4000-8000-000000000002"
	checks := []struct{ method, foreign, missing, body string }{
		{"GET", "/tenants/" + w.t2, "/tenants/" + missing, ""},
		{"GET", "/workspaces/" + w.ws2, "/workspaces/" + missing, ""},
		{"GET", "/workspaces/" + w.ws2 + "/usage", "/workspaces/" + missing + "/usage", ""},
		{"GET", "/workspaces/" + w.ws2 + "/notes", "/workspaces/" + missing + "/notes", ""},
		{"POST", "/workspaces/" + w.ws2 + "/notes", "/workspaces/" + missing + "/notes", `{"body":"x"}`},
		{"GET", "/workspaces/" + w.ws2 + "/audit-events", "/workspaces/" + missing + "/audit-events", ""},
		{"POST", "/workspaces/" + w.ws2 + "/status", "/workspaces/" + missing + "/status", `{"status":"suspended","reason":"r"}`},
		{"GET", "/users/" + w.owner2, "/users/" + missing, ""},
		{"GET", "/orders/" + foreignOrder + "/einvoices", "/orders/" + missingOrder + "/einvoices", ""},
	}
	for _, c := range checks {
		a := w.do(w.admin1, c.method, "/api/platform"+c.foreign, c.body)
		b := w.do(w.admin1, c.method, "/api/platform"+c.missing, c.body)
		if a.Code != 404 || b.Code != 404 || a.Body.String() != b.Body.String() {
			t.Errorf("%s %s: foreign = %d %q, missing = %d %q; want the same 404 body", c.method, c.foreign, a.Code, a.Body.String(), b.Code, b.Body.String())
		}
	}
	if n := w.count(`SELECT count(*) FROM platform_workspace WHERE workspace_id = $1`, w.ws2); n != 0 {
		t.Errorf("a refused status change wrote %d rows", n)
	}
	if n := w.count(`SELECT count(*) FROM platform_note WHERE workspace_id = $1`, w.ws2); n != 0 {
		t.Errorf("a refused note wrote %d rows", n)
	}
	// system_admin reads the same objects.
	if rec := w.do(w.system, "GET", "/api/platform/workspaces/"+w.ws2, ""); rec.Code != 200 {
		t.Errorf("system_admin reading ws2 = %d", rec.Code)
	}
}

// The scope is the tenant's workspace links, read at each request: a workspace
// that joins the tenant is in scope at once, with no grant to the staff member
// (AC93).
func TestAdminScopeFollowsTenantWorkspaceLinks(t *testing.T) {
	w := newScopeWorld(t)
	extra := w.workspaceIn(w.t2, w.owner2, w.mem2)
	staffBefore := w.count(`SELECT count(*) FROM platform_staff WHERE user_id = $1 AND tenant_id = $2`, w.admin1, w.t1)

	if rec := w.do(w.admin1, "GET", "/api/platform/workspaces/"+extra, ""); rec.Code != 404 {
		t.Fatalf("before the link: %d, want 404", rec.Code)
	}
	w.exec(`DELETE FROM tenant_workspace WHERE workspace_id = $1`, extra)
	w.exec(`INSERT INTO tenant_workspace (workspace_id, tenant_id) VALUES ($1, $2)`, extra, w.t1)
	if rec := w.do(w.admin1, "GET", "/api/platform/workspaces/"+extra, ""); rec.Code != 200 {
		t.Fatalf("after the link: %d %s, want 200", rec.Code, rec.Body.String())
	}
	if rec := w.do(w.admin1, "GET", "/api/platform/users/"+w.owner2, ""); rec.Code != 200 {
		t.Errorf("a member of the new workspace is not in scope: %d", rec.Code)
	}
	if n := w.count(`SELECT count(*) FROM platform_staff WHERE user_id = $1 AND tenant_id = $2`, w.admin1, w.t1); n != staffBefore {
		t.Errorf("the staff row changed")
	}
}

func TestAdminOfDeletedTenantHasEmptyScope(t *testing.T) {
	w := newScopeWorld(t)
	gone := w.newTenantID()
	goneAdmin, _ := w.staffUserInTenant(staff.RoleAdmin, gone)
	w.exec(`DELETE FROM tenant WHERE id = $1`, gone)

	if got := tenantIDsOf(w.getJSON(goneAdmin, "/api/platform/tenants?limit=100")); len(got) != 0 {
		t.Errorf("an admin of a deleted tenant sees %v", keys(got))
	}
	for _, path := range []string{"/tenants/" + gone, "/tenants/" + w.t1, "/workspaces/" + w.ws1, "/users/" + w.owner1} {
		if rec := w.do(goneAdmin, "GET", "/api/platform"+path, ""); rec.Code != 404 {
			t.Errorf("GET %s = %d, want 404", path, rec.Code)
		}
	}
	if role, tenant, active := w.staffRow(goneAdmin); role != "admin" || tenant != gone || !active {
		t.Errorf("the staff row changed: %q %q %v", role, tenant, active)
	}
}

func TestAdminLockUnlockAndScheduleDeletionInOwnTenantOnly(t *testing.T) {
	w := newScopeWorld(t)
	set := func(who, ws, status string) *httptest.ResponseRecorder {
		return w.do(who, "POST", "/api/platform/workspaces/"+ws+"/status", `{"status":"`+status+`","reason":"scope test"}`)
	}
	for _, status := range []string{"suspended", "read_only", "active", "pending_deletion"} {
		if rec := set(w.admin1, w.ws1, status); rec.Code != 200 {
			t.Errorf("admin locking its own workspace to %s = %d %s", status, rec.Code, rec.Body.String())
		}
	}
	if n := w.count(`SELECT count(*) FROM platform_workspace WHERE workspace_id = $1 AND status = 'pending_deletion'`, w.ws1); n != 1 {
		t.Errorf("own workspace rows = %d, want pending_deletion", n)
	}
	for _, status := range []string{"suspended", "pending_deletion"} {
		if rec := set(w.admin1, w.ws2, status); rec.Code != 404 {
			t.Errorf("admin setting %s on a foreign workspace = %d, want 404", status, rec.Code)
		}
	}
	if n := w.count(`SELECT count(*) FROM platform_workspace WHERE workspace_id = $1`, w.ws2); n != 0 {
		t.Errorf("the foreign workspace got %d rows", n)
	}
	// The audit row names the admin and the workspace.
	if n := w.count(`SELECT count(*) FROM audit_event WHERE workspace_id = $1 AND action = 'workspace.status_changed' AND actor_id = $2`, w.ws1, w.admin1); n < 1 {
		t.Errorf("no audit row for the admin's lock")
	}
	// support locks and unlocks anywhere and cannot schedule a deletion; billing does none.
	if rec := set(w.support, w.ws2, "suspended"); rec.Code != 200 {
		t.Errorf("support lock = %d", rec.Code)
	}
	if rec := set(w.support, w.ws2, "pending_deletion"); rec.Code != 403 {
		t.Errorf("support scheduling a deletion = %d, want 403", rec.Code)
	}
	if rec := set(w.billing, w.ws2, "active"); rec.Code != 404 || rec.Body.String() != "404 page not found\n" {
		t.Errorf("billing lock = %d %q, want the flat 404", rec.Code, rec.Body.String())
	}
}

func TestAdminOrdersAreReadOnlyAndScoped(t *testing.T) {
	w := newScopeWorld(t)
	own, foreign := w.order(w.ws1), w.order(w.ws2)
	ids := func(who string) map[string]bool {
		out := map[string]bool{}
		rows, _ := w.getJSON(who, "/api/platform/orders")["orders"].([]any)
		for _, r := range rows {
			out[r.(map[string]any)["id"].(string)] = true
		}
		return out
	}
	if got := ids(w.admin1); !got[own] || got[foreign] {
		t.Errorf("admin of t1 sees orders %v, want its own %s and not %s", got, own, foreign)
	}
	if got := ids(w.billing); !got[own] || !got[foreign] {
		t.Errorf("billing does not see both orders")
	}
	for _, path := range []string{"/orders/" + own + "/confirm", "/orders/" + own + "/cancel"} {
		if rec := w.do(w.admin1, "POST", "/api/platform"+path, `{"reason":"r","transfer_reference":"x"}`); rec.Code != 404 || rec.Body.String() != "404 page not found\n" {
			t.Errorf("admin POST %s = %d %q, want the flat 404", path, rec.Code, rec.Body.String())
		}
	}
	if n := w.count(`SELECT count(*) FROM billing_order WHERE id = $1 AND status = 'pending'`, own); n != 1 {
		t.Errorf("the admin changed an order")
	}
}

func TestAdminHasNoAccessToPaymentRoutesAccountsOverviewAndUserActions(t *testing.T) {
	w := newScopeWorld(t)
	for _, c := range []struct{ method, path, body string }{
		{"GET", "/payment-notices", ""}, {"GET", "/einvoices", ""}, {"GET", "/accounts", ""}, {"POST", "/accounts", `{}`},
		{"GET", "/overview", ""}, {"GET", "/staff", ""},
		{"POST", "/users/" + w.owner1 + "/status", `{"status":"suspended","reason":"r"}`},
		{"POST", "/users/" + w.owner1 + "/revoke-sessions", `{"reason":"r"}`},
		{"POST", "/users/" + w.owner1 + "/mfa-reset", ""},
		{"PUT", "/workspaces/" + w.ws1 + "/subscription", `{}`},
		{"GET", "/plans", ""},
	} {
		rec := w.do(w.admin1, c.method, "/api/platform"+c.path, c.body)
		if rec.Code != 404 || rec.Body.String() != "404 page not found\n" {
			t.Errorf("admin %s %s = %d %q, want the flat 404", c.method, c.path, rec.Code, rec.Body.String())
		}
	}
	if n := w.count(`SELECT count(*) FROM platform_account WHERE user_id = $1`, w.owner1); n != 0 {
		t.Errorf("a refused user action wrote an account row")
	}
}

func TestUsersListScopedToTenantMembersForAdmin(t *testing.T) {
	w := newScopeWorld(t)
	rows, _ := w.getJSON(w.admin1, "/api/platform/users?limit=100")["users"].([]any)
	got := map[string]bool{}
	for _, r := range rows {
		got[r.(map[string]any)["id"].(string)] = true
	}
	if !got[w.owner1] || !got[w.mem1] || got[w.owner2] || got[w.mem2] || len(got) != 2 {
		t.Errorf("admin of t1 sees users %v, want exactly the 2 members of its workspace", got)
	}
	if rec := w.do(w.admin1, "GET", "/api/platform/users/"+w.mem1, ""); rec.Code != 200 {
		t.Errorf("a member of the tenant = %d, want 200", rec.Code)
	}
	if rec := w.do(w.admin1, "GET", "/api/platform/users/"+w.mem2, ""); rec.Code != 404 {
		t.Errorf("a member of another tenant = %d, want 404", rec.Code)
	}
}

func TestAuditListScopedForAdminAndFullForOthers(t *testing.T) {
	w := newScopeWorld(t)
	marker, _ := w.user("actor")
	insert := func(ws, tenant any) {
		w.exec(`INSERT INTO audit_event (workspace_id, tenant_id, actor_type, actor_id, action) VALUES ($1, $2, 'staff', $3, 'note.added')`, ws, tenant, marker)
	}
	insert(w.ws1, nil) // a row of the admin's workspace
	insert(nil, w.t1)  // a row that kept only the tenant, as after the workspace was deleted
	insert(w.ws2, nil) // another tenant's workspace
	insert(nil, w.t2)  // another tenant's row with no workspace
	insert(nil, nil)   // an account-level row
	path := "/api/platform/audit-events?limit=100&actor_id=" + marker

	rows := func(who string) int {
		list, _ := w.getJSON(who, path)["events"].([]any)
		return len(list)
	}
	if n := rows(w.admin1); n != 2 {
		t.Errorf("admin of t1 reads %d rows, want its workspace's row and its tenant's row", n)
	}
	for name, who := range map[string]string{"system_admin": w.system, "support": w.support, "billing": w.billing} {
		if n := rows(who); n != 5 {
			t.Errorf("%s reads %d rows, want all 5", name, n)
		}
	}
	// The workspace's own log, for a workspace in scope.
	if n := w.count(`SELECT count(*) FROM audit_event WHERE actor_id = $1`, marker); n != 5 {
		t.Fatalf("fixture rows = %d", n)
	}
	list, _ := w.getJSON(w.admin1, "/api/platform/workspaces/"+w.ws1+"/audit-events?actor_id="+marker)["events"].([]any)
	if len(list) != 1 {
		t.Errorf("workspace log of ws1 = %d rows, want 1", len(list))
	}
}

func TestStaffTenantMustExist(t *testing.T) {
	w := newScopeWorld(t)
	_, email := w.user("hire")
	missing := "99999999-0000-4000-8000-0000000000aa"
	for _, c := range []struct{ method, path, body string }{
		{"POST", "/staff", `{"email":"` + email + `","role":"admin","tenant_id":"` + missing + `"}`},
		{"PUT", "/staff/" + w.support, `{"role":"admin","tenant_id":"` + missing + `"}`},
	} {
		rec := w.do(w.system, c.method, "/api/platform"+c.path, c.body)
		if rec.Code != 400 || !strings.Contains(rec.Body.String(), `"code":"tenant_invalid"`) {
			t.Errorf("%s %s = %d %s, want 400 tenant_invalid", c.method, c.path, rec.Code, rec.Body.String())
		}
	}
	if role, _, _ := w.staffRow(w.support); role != "support" {
		t.Errorf("the support row changed to %q", role)
	}
}

// The handlers check the scope themselves: reached without the route's role
// list, they still read only the admin's tenant and refuse a role with no access.
func TestHandlersApplyScopeWhenCalledDirectlyWithoutMiddleware(t *testing.T) {
	w := newScopeWorld(t)
	direct := func(who, method, path, body string, pattern string, fn http.HandlerFunc) *httptest.ResponseRecorder {
		mux := http.NewServeMux()
		mux.Handle(method+" "+pattern, w.auth.RequirePlatformStaff()(fn))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, w.request(who, method, path, body))
		return rec
	}
	c := w.console

	rec := direct(w.admin1, "GET", "/api/platform/tenants?limit=100", "", "/api/platform/tenants", c.listTenants)
	var list map[string]any
	if err := jsonUnmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if got := tenantIDsOf(list); len(got) != 1 || got[w.t1] == nil {
		t.Errorf("listTenants as admin sees %v, want only its tenant", keys(got))
	}
	if rec := direct(w.admin1, "GET", "/api/platform/tenants/"+w.t2, "", "/api/platform/tenants/{id}", c.getTenant); rec.Code != 404 {
		t.Errorf("getTenant of a foreign tenant = %d, want 404", rec.Code)
	}
	if rec := direct(w.admin1, "GET", "/api/platform/workspaces/"+w.ws2, "", "/api/platform/workspaces/{id}", c.getWorkspace); rec.Code != 404 {
		t.Errorf("getWorkspace of a foreign workspace = %d, want 404", rec.Code)
	}
	if rec := direct(w.admin1, "POST", "/api/platform/workspaces/"+w.ws2+"/notes", `{"body":"x"}`, "/api/platform/workspaces/{id}/notes", c.addNote); rec.Code != 404 {
		t.Errorf("addNote on a foreign workspace = %d, want 404", rec.Code)
	}
	// billing has no access to the lock action: the flat 404, and nothing written.
	if rec := direct(w.billing, "POST", "/api/platform/workspaces/"+w.ws1+"/status", `{"status":"suspended","reason":"r"}`,
		"/api/platform/workspaces/{id}/status", c.setWorkspaceStatus); rec.Code != 404 || rec.Body.String() != "404 page not found\n" {
		t.Errorf("setWorkspaceStatus as billing = %d %q, want the flat 404", rec.Code, rec.Body.String())
	}
	if n := w.count(`SELECT count(*) FROM platform_workspace WHERE workspace_id = $1`, w.ws1); n != 0 {
		t.Errorf("a refused call wrote %d rows", n)
	}
	if n := w.count(`SELECT count(*) FROM platform_note WHERE workspace_id = $1`, w.ws2); n != 0 {
		t.Errorf("a refused note was written")
	}
}

// What support and billing see and do does not change with the tenant model:
// every route they had before answers them as it did, on any tenant's objects.
func TestSupportBillingScopeUnchanged(t *testing.T) {
	w := newScopeWorld(t)
	own, foreign := w.order(w.ws1), w.order(w.ws2)
	for _, ws := range []string{w.ws1, w.ws2} {
		for _, path := range []string{"", "/usage", "/notes", "/audit-events"} {
			for name, who := range map[string]string{"support": w.support, "billing": w.billing} {
				if rec := w.do(who, "GET", "/api/platform/workspaces/"+ws+path, ""); rec.Code != 200 {
					t.Errorf("%s GET /workspaces/%s%s = %d, want 200", name, ws[:8], path, rec.Code)
				}
			}
		}
	}
	for _, u := range []string{w.owner1, w.owner2} {
		for name, who := range map[string]string{"support": w.support, "billing": w.billing} {
			if rec := w.do(who, "GET", "/api/platform/users/"+u, ""); rec.Code != 200 {
				t.Errorf("%s GET /users/%s = %d, want 200", name, u[:8], rec.Code)
			}
		}
	}
	if rec := w.do(w.support, "POST", "/api/platform/workspaces/"+w.ws2+"/notes", `{"body":"support note"}`); rec.Code != 201 {
		t.Errorf("support adding a note = %d", rec.Code)
	}
	for _, path := range []string{"/orders", "/orders/" + own + "/einvoices", "/orders/" + foreign + "/einvoices"} {
		if rec := w.do(w.billing, "GET", "/api/platform"+path, ""); rec.Code != 200 {
			t.Errorf("billing GET %s = %d, want 200", path, rec.Code)
		}
		if rec := w.do(w.support, "GET", "/api/platform"+path, ""); rec.Code != 404 || rec.Body.String() != "404 page not found\n" {
			t.Errorf("support GET %s = %d, want the flat 404", path, rec.Code)
		}
	}
	for _, who := range []string{w.support, w.billing} {
		if rec := w.do(who, "GET", "/api/platform/staff", ""); rec.Code != 404 {
			t.Errorf("GET /staff = %d, want 404", rec.Code)
		}
	}
}

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }
