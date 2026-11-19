package staff

import "testing"

func TestParseRoleAcceptsFourRoles(t *testing.T) {
	for _, s := range []string{"system_admin", "admin", "support", "billing"} {
		r, ok := ParseRole(s)
		if !ok || string(r) != s {
			t.Errorf("ParseRole(%q) = %q, %v", s, r, ok)
		}
	}
	for _, s := range []string{"", "Admin", "SYSTEM_ADMIN", "root", "owner", "system-admin"} {
		if _, ok := ParseRole(s); ok {
			t.Errorf("ParseRole(%q) accepted", s)
		}
	}
}

func TestPermitsReadsTheMatrix(t *testing.T) {
	cases := []struct {
		role   Role
		action string
		want   bool
	}{
		{RoleSystemAdmin, ActionStaffManage, true},
		{RoleAdmin, ActionStaffManage, false},
		{RoleSupport, ActionStaffManage, false},
		{RoleAdmin, ActionTenantsRead, true}, // Own is still permitted
		{RoleSupport, ActionWorkspacesLock, true},
		{RoleBilling, ActionWorkspacesLock, false},
	}
	for _, tc := range cases {
		if got := tc.role.Permits(tc.action); got != tc.want {
			t.Errorf("%s.Permits(%q) = %v, want %v", tc.role, tc.action, got, tc.want)
		}
	}
}

func TestPermitsRefusesAnUnknownActionAndAnUnknownRole(t *testing.T) {
	for _, role := range []Role{RoleSystemAdmin, RoleAdmin, RoleSupport, RoleBilling} {
		if role.Permits("no.such.action") {
			t.Errorf("%s permits an unknown action", role)
		}
	}
	for _, role := range []Role{"", "owner", "ADMIN"} {
		if role.Permits(ActionTenantsRead) {
			t.Errorf("role %q permits tenants.read", role)
		}
	}
}
