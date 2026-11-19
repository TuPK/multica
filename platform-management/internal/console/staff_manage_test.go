package console

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/staff"
)

// The role and tenant of a staff row go together (spec R6 section 3.7): admin
// needs exactly one tenant, every other role has none.
func TestValidateStaffTarget(t *testing.T) {
	cases := []struct {
		name     string
		role     staff.Role
		tenantID string
		wantErr  string // the error code, "" for none
		wantID   string // the canonical tenant id returned
	}{
		{"admin without tenant", staff.RoleAdmin, "", "tenant_required", ""},
		{"admin with tenant", staff.RoleAdmin, tenantID, "", tenantID},
		{"admin with upper case tenant", staff.RoleAdmin, "EEEEEEEE-0000-0000-0000-000000000001", "", tenantID},
		{"admin with a malformed tenant", staff.RoleAdmin, "not-a-uuid", "tenant_invalid", ""},
		{"system_admin with tenant", staff.RoleSystemAdmin, tenantID, "tenant_not_allowed", ""},
		{"support with tenant", staff.RoleSupport, tenantID, "tenant_not_allowed", ""},
		{"billing with tenant", staff.RoleBilling, tenantID, "tenant_not_allowed", ""},
		{"support without tenant", staff.RoleSupport, "", "", ""},
		{"system_admin without tenant", staff.RoleSystemAdmin, "", "", ""},
		{"unknown role", staff.Role("root"), "", "invalid_role", ""},
		{"unknown role with tenant", staff.Role("root"), tenantID, "invalid_role", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validateStaffTarget(string(tc.role), tc.tenantID)
			if tc.wantErr == "" {
				if err != nil || got.Role != tc.role || got.TenantID != tc.wantID {
					t.Fatalf("got %+v, %v; want role %s tenant %q", got, err, tc.role, tc.wantID)
				}
				return
			}
			wantAPIError(t, err, 400, tc.wantErr)
		})
	}
}

func TestPutStaffRefusesABodyThatIsNotAnInput(t *testing.T) {
	e := newEnv(t)
	for _, body := range []string{``, `{}`, `{"role":"root"}`, `{"role":"support","extra":1}`, `{"role":"admin"}`, `{"role":"support","tenant_id":"` + tenantID + `"}`} {
		rec := e.do(adminID, "PUT", "/api/platform/staff/"+suppID, body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("PUT %q = %d %s, want 400", body, rec.Code, rec.Body.String())
		}
	}
	if len(e.store.audits) != 0 {
		t.Fatal("a refused request wrote an audit row")
	}
}

func TestPutStaffOnUnknownOrInactiveTargetIsNotFound(t *testing.T) {
	e := newEnv(t)
	wantCode(t, e.do(adminID, "PUT", "/api/platform/staff/"+custID, `{"role":"support"}`), 404, "not_found")
	wantCode(t, e.do(adminID, "PUT", "/api/platform/staff/nope", `{"role":"support"}`), 404, "not_found")
}

func TestPutStaffChangesRoleAndTenantAndAuditsIt(t *testing.T) {
	e := newEnv(t)
	rec := e.do(adminID, "PUT", "/api/platform/staff/"+suppID, `{"role":"admin","tenant_id":"`+tenantID+`"}`)
	wantStatus(t, rec, 200)
	if m := decode(t, rec); m["user_id"] != suppID || m["role"] != "admin" || m["tenant_id"] != tenantID {
		t.Fatalf("response = %v", m)
	}
	if r, _ := e.store.activeRole(suppID); r != "admin" || e.store.activeTenant(suppID) != tenantID {
		t.Fatalf("row = %q %q", r, e.store.activeTenant(suppID))
	}
	if len(e.store.audits) != 2 {
		t.Fatalf("audit rows = %+v, want role_changed and tenant_assigned", e.store.audits)
	}
	byAction := map[string]audit.Entry{}
	for _, a := range e.store.audits {
		byAction[a.Action] = a
		wantStaffActor(t, a, adminID, adminMail)
		if a.TargetID != suppID || a.TargetType != targetUser || a.TenantID != tenantID {
			t.Errorf("entry = %+v, want target %s and tenant %s", a, suppID, tenantID)
		}
		if a.Detail["old_role"] != "support" || a.Detail["new_role"] != "admin" || a.Detail["old_tenant_id"] != "" || a.Detail["new_tenant_id"] != tenantID {
			t.Errorf("detail = %v", a.Detail)
		}
	}
	if _, ok := byAction[audit.ActionStaffRoleChanged]; !ok {
		t.Error("no staff.role_changed row")
	}
	if _, ok := byAction[audit.ActionStaffTenantAssigned]; !ok {
		t.Error("no staff.tenant_assigned row")
	}

	// Moving an admin to another tenant is a tenant change only.
	e.store.audits = nil
	wantStatus(t, e.do(adminID, "PUT", "/api/platform/staff/"+suppID, `{"role":"admin","tenant_id":"`+otherTenantID+`"}`), 200)
	if len(e.store.audits) != 1 || e.store.audits[0].Action != audit.ActionStaffTenantAssigned || e.store.audits[0].TenantID != otherTenantID {
		t.Fatalf("audits = %+v", e.store.audits)
	}

	// Leaving admin clears the tenant; the row keeps the tenant it had.
	e.store.audits = nil
	wantStatus(t, e.do(adminID, "PUT", "/api/platform/staff/"+suppID, `{"role":"billing"}`), 200)
	if e.store.activeTenant(suppID) != "" {
		t.Errorf("tenant = %q, want cleared", e.store.activeTenant(suppID))
	}
	if len(e.store.audits) != 1 || e.store.audits[0].Action != audit.ActionStaffRoleChanged || e.store.audits[0].TenantID != otherTenantID ||
		e.store.audits[0].Detail["old_tenant_id"] != otherTenantID || e.store.audits[0].Detail["new_tenant_id"] != "" {
		t.Fatalf("audits = %+v", e.store.audits)
	}

	// A PUT that changes nothing writes nothing.
	e.store.audits = nil
	wantStatus(t, e.do(adminID, "PUT", "/api/platform/staff/"+suppID, `{"role":"billing"}`), 200)
	if len(e.store.audits) != 0 {
		t.Fatalf("a no-op PUT wrote %+v", e.store.audits)
	}
}

func TestPutStaffRefusesDemotingTheLastSystemAdmin(t *testing.T) {
	e := newEnv(t)
	wantCode(t, e.do(adminID, "PUT", "/api/platform/staff/"+adminID, `{"role":"support"}`), 400, "last_system_admin")
	wantCode(t, e.do(adminID, "PUT", "/api/platform/staff/"+adminID, `{"role":"admin","tenant_id":"`+tenantID+`"}`), 400, "last_system_admin")
	if r, _ := e.store.activeRole(adminID); r != "system_admin" || len(e.store.audits) != 0 {
		t.Fatal("the last system admin was demoted")
	}
	// With a second one, one can step down; then the other cannot.
	wantStatus(t, e.do(adminID, "PUT", "/api/platform/staff/"+suppID, `{"role":"system_admin"}`), 200)
	e.dir.roles[suppID] = staff.RoleSystemAdmin // the fake directory is the middleware's view of the row
	wantStatus(t, e.do(adminID, "PUT", "/api/platform/staff/"+adminID, `{"role":"support"}`), 200)
	wantCode(t, e.do(suppID, "PUT", "/api/platform/staff/"+suppID, `{"role":"billing"}`), 400, "last_system_admin")
}

func TestAddStaffTenantRules(t *testing.T) {
	e := newEnv(t)
	post := func(body string) *httptest.ResponseRecorder {
		return e.do(adminID, "POST", "/api/platform/staff", body)
	}
	wantCode(t, post(`{"email":"`+custMail+`","role":"admin"}`), 400, "tenant_required")
	for _, role := range []string{"system_admin", "support", "billing"} {
		wantCode(t, post(`{"email":"`+custMail+`","role":"`+role+`","tenant_id":"`+tenantID+`"}`), 400, "tenant_not_allowed")
	}
	wantCode(t, post(`{"email":"`+custMail+`","role":"admin","tenant_id":"nope"}`), 400, "tenant_invalid")
	if len(e.store.audits) != 0 || len(e.store.staffRows) != defaultStaffRows {
		t.Fatal("a refused request changed staff")
	}

	rec := post(`{"email":"` + custMail + `","role":"admin","tenant_id":"` + tenantID + `"}`)
	wantStatus(t, rec, 201)
	if m := decode(t, rec); m["role"] != "admin" || m["tenant_id"] != tenantID {
		t.Fatalf("response = %v", m)
	}
	if r, _ := e.store.activeRole(custID); r != "admin" || e.store.activeTenant(custID) != tenantID {
		t.Fatalf("row = %q %q", r, e.store.activeTenant(custID))
	}
	if a := onlyAudit(t, e); a.Action != audit.ActionStaffAdded || a.TenantID != tenantID || a.Detail["tenant_id"] != tenantID {
		t.Fatalf("entry = %+v", a)
	}
}

func TestListStaffShowsTenantID(t *testing.T) {
	e := newEnv(t)
	e.reader.staff = []StaffRow{
		{UserID: tadminID, Email: tadminMail, Role: "admin", TenantID: tenantID, CreatedAt: t0},
		{UserID: suppID, Email: suppMail, Role: "support", CreatedAt: t0},
	}
	rows, _ := decode(t, e.do(adminID, "GET", "/api/platform/staff", ""))["staff"].([]any)
	first, _ := rows[0].(map[string]any)
	second, _ := rows[1].(map[string]any)
	if first["tenant_id"] != tenantID || second["tenant_id"] != nil {
		t.Fatalf("staff = %v", rows)
	}
	if _, ok := second["tenant_id"]; !ok {
		t.Fatal("tenant_id is missing for a row with no tenant; it must be null")
	}
}

func TestRemoveStaffAuditCarriesTheTenantOfTheRemovedRow(t *testing.T) {
	e := newEnv(t)
	wantStatus(t, e.do(adminID, "DELETE", "/api/platform/staff/"+tadminID, ""), 200)
	if a := onlyAudit(t, e); a.Action != audit.ActionStaffRemoved || a.TenantID != tenantID {
		t.Fatalf("entry = %+v", a)
	}
}
