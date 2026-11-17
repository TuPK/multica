package workspaceguard

import (
	"net/http"
	"testing"
)

// Spec 7.1 rule 9 (D53): a guest reaches only the routes on the guest allow-list.
// The table is closed: a route that is not listed is refused, reads included.
func TestGuestAllowListMatrix(t *testing.T) {
	cases := []struct {
		name, method, pattern string
		allowed               bool
	}{
		// Issues and comments the guest is granted; the handler narrows them to the granted projects.
		{"list issues", http.MethodGet, "/api/issues/", true},
		{"read one issue", http.MethodGet, "/api/issues/{id}", true},
		{"create an issue", http.MethodPost, "/api/issues/", true},
		{"edit one issue", http.MethodPut, "/api/issues/{id}", true},
		{"list comments", http.MethodGet, "/api/issues/{id}/comments", true},
		{"comment on an issue", http.MethodPost, "/api/issues/{id}/comments", true},
		{"search issues", http.MethodGet, "/api/issues/search", true},

		// Personal state and the inbox.
		{"read the inbox", http.MethodGet, "/api/inbox", true},
		{"mark one inbox item read", http.MethodPost, "/api/inbox/{id}/read", true},
		{"list pins", http.MethodGet, "/api/pins", true},
		{"read issue view preferences", http.MethodGet, "/api/issue-view-preferences", true},
		{"leave the workspace", http.MethodPost, "/api/workspaces/{id}/leave", true},
		{"list members (names and avatars, no emails)", http.MethodGet, "/api/workspaces/{id}/members", true},

		// Closed by default: nothing in this list is granted to a guest.
		{"read the workspace", http.MethodGet, "/api/workspaces/{id}", false},
		{"invite a member", http.MethodPost, "/api/workspaces/{id}/members", false},
		{"change workspace settings", http.MethodPatch, "/api/workspaces/{id}", false},
		{"delete a member", http.MethodDelete, "/api/workspaces/{id}/members/{memberId}", false},
		{"list agents", http.MethodGet, "/api/agents", false},
		{"create an agent", http.MethodPost, "/api/agents", false},
		{"delete an issue", http.MethodDelete, "/api/issues/{id}", false},
		{"start an issue flow", http.MethodPost, "/api/issues/{id}/flow/start", false},
		{"create a label", http.MethodPost, "/api/labels", false},
		{"knowledge", http.MethodGet, "/api/knowledge/bases", false},
		{"unknown route", http.MethodPost, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := GuestAllows(c.method, c.pattern); got != c.allowed {
				t.Errorf("GuestAllows(%s, %q) = %v, want %v", c.method, c.pattern, got, c.allowed)
			}
		})
	}
}

// The refusal carries its own code and message, so clients branch on guest_forbidden.
func TestGuestRefusalCodeAndMessage(t *testing.T) {
	if CodeGuestForbidden != "guest_forbidden" {
		t.Errorf("CodeGuestForbidden = %q, want guest_forbidden", CodeGuestForbidden)
	}
	if got := Message(CodeGuestForbidden); got != "guests cannot use this route" {
		t.Errorf("Message(%q) = %q", CodeGuestForbidden, got)
	}
}
