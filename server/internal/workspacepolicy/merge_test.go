package workspacepolicy

import (
	"reflect"
	"testing"
)

// TestMergeTable is the rule of spec R6 D101: a workspace may only tighten the
// tenant floor, so each field takes the stricter value.
func TestMergeTable(t *testing.T) {
	base := func(p Policy) Policy {
		if p.AllowedInviteDomains == nil {
			p.AllowedInviteDomains = []string{}
		}
		return p
	}
	cases := []struct {
		name      string
		tenant    Policy
		workspace Policy
		want      Policy
	}{
		{
			name:      "both empty keeps defaults",
			tenant:    Default(),
			workspace: Default(),
			want:      Default(),
		},
		{
			name:      "tenant sets no domain list, workspace sets one",
			tenant:    Default(),
			workspace: Policy{AllowedInviteDomains: []string{"a.test"}, ShareLinksEnabled: true},
			want:      Policy{AllowedInviteDomains: []string{"a.test"}, ShareLinksEnabled: true},
		},
		{
			name:      "tenant domains apply when the workspace has none",
			tenant:    Policy{AllowedInviteDomains: []string{"a.test", "b.test"}, ShareLinksEnabled: true},
			workspace: Default(),
			want:      Policy{AllowedInviteDomains: []string{"a.test", "b.test"}, ShareLinksEnabled: true},
		},
		{
			name:      "domains intersect",
			tenant:    Policy{AllowedInviteDomains: []string{"a.test", "b.test"}, ShareLinksEnabled: true},
			workspace: Policy{AllowedInviteDomains: []string{"b.test", "c.test"}, ShareLinksEnabled: true},
			want:      Policy{AllowedInviteDomains: []string{"b.test"}, ShareLinksEnabled: true},
		},
		{
			name:      "require_sso and require_mfa are the union",
			tenant:    Policy{ShareLinksEnabled: true, RequireSSO: true},
			workspace: Policy{ShareLinksEnabled: true, RequireMFA: true},
			want:      Policy{ShareLinksEnabled: true, RequireSSO: true, RequireMFA: true},
		},
		{
			name:      "share links are on only when both allow them",
			tenant:    Policy{ShareLinksEnabled: false},
			workspace: Policy{ShareLinksEnabled: true},
			want:      Policy{ShareLinksEnabled: false},
		},
		{
			name:      "zero session age is no limit, so the other side applies",
			tenant:    Policy{ShareLinksEnabled: true, SessionMaxAgeHours: 0},
			workspace: Policy{ShareLinksEnabled: true, SessionMaxAgeHours: 24},
			want:      Policy{ShareLinksEnabled: true, SessionMaxAgeHours: 24},
		},
		{
			name:      "the shorter session age wins",
			tenant:    Policy{ShareLinksEnabled: true, SessionMaxAgeHours: 24},
			workspace: Policy{ShareLinksEnabled: true, SessionMaxAgeHours: 8},
			want:      Policy{ShareLinksEnabled: true, SessionMaxAgeHours: 8},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Merge(c.tenant, c.workspace)
			if !reflect.DeepEqual(base(got), base(c.want)) {
				t.Errorf("Merge = %+v, want %+v", got, c.want)
			}
		})
	}
}

// Disjoint domain lists leave no address allowed, not every address.
func TestMergeDisjointDomainsAllowNothing(t *testing.T) {
	got := Merge(
		Policy{AllowedInviteDomains: []string{"a.test"}, ShareLinksEnabled: true},
		Policy{AllowedInviteDomains: []string{"b.test"}, ShareLinksEnabled: true},
	)
	if DomainAllowed(got.AllowedInviteDomains, "user@b.test") || DomainAllowed(got.AllowedInviteDomains, "user@a.test") {
		t.Errorf("disjoint lists allow an address: %v", got.AllowedInviteDomains)
	}
}
