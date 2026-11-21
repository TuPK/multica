package handler

import "testing"

// Spec 6.6: viewer is a valid member role for a member, an invitation and a
// share link. Anything else is refused.
func TestNormalizeMemberRoleAcceptsViewer(t *testing.T) {
	cases := map[string]struct {
		role string
		ok   bool
	}{
		"viewer":        {"viewer", true},
		" viewer ":      {"viewer", true},
		"member":        {"member", true},
		"":              {"member", true},
		"guest":         {"guest", true},
		"Viewer":        {"", false},
		"read-only":     {"", false},
		"superuser":     {"", false},
		"owner":         {"owner", true},
		"admin":         {"admin", true},
		"viewer viewer": {"", false},
	}
	for in, want := range cases {
		got, ok := normalizeMemberRole(in)
		if got != want.role || ok != want.ok {
			t.Errorf("normalizeMemberRole(%q) = (%q, %v), want (%q, %v)", in, got, ok, want.role, want.ok)
		}
	}
}

// Owners and admins assign viewer, member and admin; only an owner assigns
// owner. A member or a viewer assigns nothing (the route refuses them first).
func TestCanAssignMemberRole(t *testing.T) {
	cases := []struct {
		requester, role string
		want            bool
	}{
		{"owner", "owner", true},
		{"owner", "admin", true},
		{"owner", "member", true},
		{"owner", "viewer", true},
		{"admin", "owner", false},
		{"admin", "admin", true},
		{"admin", "member", true},
		{"admin", "viewer", true},
		{"member", "viewer", false},
		{"member", "member", false},
		{"viewer", "viewer", false},
		{"viewer", "member", false},
		{"", "member", false},
	}
	for _, c := range cases {
		if got := canAssignMemberRole(c.requester, c.role); got != c.want {
			t.Errorf("canAssignMemberRole(%q, %q) = %v, want %v", c.requester, c.role, got, c.want)
		}
	}
}
