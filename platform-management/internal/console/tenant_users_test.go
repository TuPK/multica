package console

import (
	"net/http"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/staff"
)

// Spec R6 section 3b.3 (QT3): tenant users, and removal or role change as
// requests. Owner is immutable through staff; admin cannot change its own role;
// staff users are hidden from admin.

const (
	tuWS    = "7a000000-0000-0000-0000-000000000001"
	tuOwner = "7a000000-0000-0000-0000-000000000002"
	tuMem   = "7a000000-0000-0000-0000-000000000003"
	tuStaff = "7a000000-0000-0000-0000-000000000004"
)

func tenantUserEnv(t *testing.T) *env {
	t.Helper()
	e := newEnv(t)
	e.reader.wsTenant[tuWS] = tenantID
	e.reader.memberRoles = map[string]string{
		tuWS + "/" + tuOwner:  "owner",
		tuWS + "/" + tuMem:    "member",
		tuWS + "/" + tadminID: "admin",
		tuWS + "/" + tuStaff:  "member",
	}
	e.reader.memberMail = map[string]string{tuOwner: "owner@example.test", tuMem: "member@example.test", tadminID: tadminMail, tuStaff: "staff@example.test"}
	e.reader.staffUsers = map[string]bool{tuStaff: true}
	e.reader.tenantUsers = []TenantUser{
		{ID: tuMem, Email: "member@example.test", Name: "M", AccountStatus: "active", Workspaces: []TenantUserWorkspace{{ID: tuWS, Name: "Acme", Role: "member"}}},
		{ID: tuStaff, Email: "staff@example.test", Name: "S", AccountStatus: "active", IsStaff: true, Workspaces: []TenantUserWorkspace{{ID: tuWS, Name: "Acme", Role: "member"}}},
	}
	return e
}

func TestRoleChangeRules(t *testing.T) {
	cases := []struct {
		name      string
		viewer    staff.Role
		viewerID  string
		target    string
		current   string
		next      string
		wantCode  string
		wantError bool
	}{
		{name: "member to viewer is allowed", viewer: staff.RoleSystemAdmin, viewerID: adminID, target: tuMem, current: "member", next: "viewer"},
		{name: "owner as source is refused", viewer: staff.RoleSystemAdmin, viewerID: adminID, target: tuOwner, current: "owner", next: "admin", wantCode: "owner_role_immutable", wantError: true},
		{name: "owner as target is refused", viewer: staff.RoleSystemAdmin, viewerID: adminID, target: tuMem, current: "member", next: "owner", wantCode: "owner_role_immutable", wantError: true},
		{name: "admin cannot change its own role", viewer: staff.RoleAdmin, viewerID: tadminID, target: tadminID, current: "admin", next: "member", wantCode: "cannot_change_own_role", wantError: true},
		{name: "unknown role is 400", viewer: staff.RoleSystemAdmin, viewerID: adminID, target: tuMem, current: "member", next: "guest", wantCode: "member_role_invalid", wantError: true},
	}
	for _, c := range cases {
		err := checkRoleChange(c.viewer, c.viewerID, c.target, c.current, c.next)
		if !c.wantError {
			if err != nil {
				t.Errorf("%s: %v", c.name, err)
			}
			continue
		}
		if err == nil || err.Code != c.wantCode {
			t.Errorf("%s: got %+v, want %s", c.name, err, c.wantCode)
		}
	}
}

func TestTenantUsersHidesStaffFromAdmin(t *testing.T) {
	e := tenantUserEnv(t)
	rec := e.do(tadminID, "GET", "/api/platform/tenants/"+tenantID+"/users", "")
	wantStatus(t, rec, http.StatusOK)
	users, _ := decode(t, rec)["users"].([]any)
	if len(users) != 1 {
		t.Fatalf("admin saw %d users, want 1 (staff hidden)", len(users))
	}
	rec = e.do(adminID, "GET", "/api/platform/tenants/"+tenantID+"/users", "")
	users, _ = decode(t, rec)["users"].([]any)
	if len(users) != 2 {
		t.Fatalf("system_admin saw %d users, want 2", len(users))
	}
}

func TestRemoveRefusesOwnerAndStaffTarget(t *testing.T) {
	e := tenantUserEnv(t)
	wantCode(t, e.do(adminID, "POST", "/api/platform/workspaces/"+tuWS+"/members/"+tuOwner+"/remove", ""), 409, "owner_role_immutable")
	wantCode(t, e.do(adminID, "POST", "/api/platform/workspaces/"+tuWS+"/members/"+tuStaff+"/remove", ""), 409, "user_is_staff")
	wantUnknownRouteOrNotFound(t, e.do(tadminID, "POST", "/api/platform/workspaces/"+tuWS+"/members/"+tuStaff+"/remove", ""))
	if len(e.store.memberChanges) != 0 {
		t.Fatalf("refused removals wrote requests: %+v", e.store.memberChanges)
	}
}

func TestChangeRoleWritesRequestAndAudit(t *testing.T) {
	e := tenantUserEnv(t)
	rec := e.do(adminID, "POST", "/api/platform/workspaces/"+tuWS+"/members/"+tuMem+"/role", `{"role":"viewer"}`)
	wantStatus(t, rec, http.StatusAccepted)
	if len(e.store.memberChanges) != 1 {
		t.Fatalf("requests = %+v", e.store.memberChanges)
	}
	got := e.store.memberChanges[0]
	if got.Kind != "change_role" || got.Role != "viewer" || got.PreviousRole != "member" || got.TargetUserID != tuMem {
		t.Fatalf("request = %+v", got)
	}
	found := false
	for _, a := range e.store.audits {
		if a.Action == "member.staff_role_change_requested" && a.ActorType == "staff" && a.TenantID == tenantID {
			found = true
		}
	}
	if !found {
		t.Fatal("no staff role-change audit row with the tenant id")
	}
}

func TestAdminCannotChangeItsOwnRole(t *testing.T) {
	e := tenantUserEnv(t)
	wantCode(t, e.do(tadminID, "POST", "/api/platform/workspaces/"+tuWS+"/members/"+tadminID+"/role", `{"role":"member"}`), 409, "cannot_change_own_role")
}
