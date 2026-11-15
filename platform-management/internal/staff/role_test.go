package staff

import "testing"

// The route names below are the groups of spec 5.2 and 5.4. They name what a
// route does, not its path, because the routes arrive in later slices.
func TestPermits(t *testing.T) {
	const (
		yes = true
		no  = false
	)
	tests := []struct {
		route                   string
		support, billing, admin bool
	}{
		// Read: every staff role.
		{"tenants.read", yes, yes, yes},
		{"users.read", yes, yes, yes},
		{"audit.read", yes, yes, yes},
		{"notes.read", yes, yes, yes},
		{"overview.read", yes, yes, yes},
		// Support actions: support, and so billing and admin.
		{"accounts.lock", yes, yes, yes},
		{"workspaces.suspend", yes, yes, yes},
		{"sessions.revoke", yes, yes, yes},
		{"notes.write", yes, yes, yes},
		// Billing actions: billing and admin.
		{"orders.read", no, yes, yes},
		{"orders.confirm", no, yes, yes},
		{"orders.cancel", no, yes, yes},
		{"subscriptions.change", no, yes, yes},
		{"subscriptions.renew", no, yes, yes},
		// Admin only.
		{"staff.manage", no, no, yes},
		{"plans.manage", no, no, yes},
		{"workspaces.schedule_deletion", no, no, yes},
		{"workspaces.cancel_deletion", no, no, yes},
	}
	for _, tc := range tests {
		for role, want := range map[Role]bool{RoleSupport: tc.support, RoleBilling: tc.billing, RoleAdmin: tc.admin} {
			if got := role.Permits(tc.route); got != want {
				t.Errorf("%s.Permits(%q) = %v, want %v", role, tc.route, got, want)
			}
		}
	}
}

func TestPermitsRefusesAnUnknownRouteAndAnUnknownRole(t *testing.T) {
	for _, role := range []Role{RoleSupport, RoleBilling, RoleAdmin} {
		if role.Permits("no.such.route") {
			t.Errorf("%s permits an unknown route", role)
		}
	}
	for _, role := range []Role{"", "owner", "ADMIN"} {
		if role.Permits("tenants.read") {
			t.Errorf("role %q permits tenants.read", role)
		}
	}
}

func TestParseRole(t *testing.T) {
	for _, s := range []string{"support", "billing", "admin"} {
		r, ok := ParseRole(s)
		if !ok || string(r) != s {
			t.Errorf("ParseRole(%q) = %q, %v", s, r, ok)
		}
	}
	for _, s := range []string{"", "Admin", "root"} {
		if _, ok := ParseRole(s); ok {
			t.Errorf("ParseRole(%q) accepted", s)
		}
	}
}
