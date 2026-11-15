package workspaceguard

import "testing"

func TestDecideForMemberEqualsDecideForAHumanWithNoRoleLimit(t *testing.T) {
	statuses := []string{"active", "read_only", "suspended", "pending_deletion", "", "deleted"}
	for _, status := range statuses {
		for _, rt := range corpus {
			for _, method := range allMethods {
				want := Decide(status, method, rt.pattern)
				if len(want.Roles) > 0 {
					continue // role-limited entries are covered below
				}
				got := DecideForMember(status, method, rt.pattern, "member", false)
				if got.Allowed != want.Allowed || got.Code != want.Code {
					t.Errorf("%q %s %q: DecideForMember = %+v, Decide = %+v", status, method, rt.pattern, got, want)
				}
			}
		}
	}
}

func TestDecideForMemberDaemonPathAllowsEverything(t *testing.T) {
	for _, status := range []string{"active", "read_only", "suspended", "pending_deletion", "", "deleted"} {
		for _, method := range allMethods {
			for _, pattern := range []string{routeIssues, routeIssue, "", "/api/daemon/tasks/{taskId}/complete"} {
				for _, role := range []string{"owner", "member", ""} {
					d := DecideForMember(status, method, pattern, role, true)
					if !d.Allowed || d.Code != "" {
						t.Errorf("daemon %q %s %q role %q = %+v, want a plain allow", status, method, pattern, role, d)
					}
				}
			}
		}
	}
}

func TestDecideForMemberEnforcesTheRolesOfAnAllowedDecision(t *testing.T) {
	cases := []struct {
		status, method, pattern, role string
		allowed                       bool
		code                          string
	}{
		{"suspended", "GET", routeCompanyExport, "owner", true, ""},
		{"suspended", "GET", routeCompanyExport, "admin", false, "workspace_suspended"},
		{"pending_deletion", "GET", routeHubExport, "member", false, "workspace_pending_deletion"},
		{"pending_deletion", "GET", routeHubExport, "owner", true, ""},
		{"suspended", "GET", routeSubSummary, "admin", true, ""},
		{"suspended", "GET", routeSubSummary, "owner", true, ""},
		{"suspended", "GET", routeSubSummary, "member", false, "workspace_suspended"},
		{"pending_deletion", "POST", routeSubCheckout, "", false, "workspace_pending_deletion"},
		// No role limit: any member passes.
		{"suspended", "GET", routeWorkspace, "member", true, ""},
		// A route that is not allowed keeps Decide's refusal, whatever the role.
		{"read_only", "POST", routeIssues, "owner", false, "workspace_read_only"},
		// read_only has no role limit on the subscription routes.
		{"read_only", "POST", routeSubCheckout, "member", true, ""},
		// An active workspace ignores the role.
		{"active", "POST", routeIssues, "", true, ""},
	}
	for _, c := range cases {
		d := DecideForMember(c.status, c.method, c.pattern, c.role, false)
		if d.Allowed != c.allowed || d.Code != c.code {
			t.Errorf("%s %s %s role %q = %+v, want allowed=%v code=%q", c.status, c.method, c.pattern, c.role, d, c.allowed, c.code)
		}
	}
}
