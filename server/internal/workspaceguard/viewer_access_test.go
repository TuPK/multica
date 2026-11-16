package workspaceguard

import (
	"net/http"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/workspacepolicy"
)

// Spec 6.6: a viewer reads what a member reads and writes nothing, except the
// personal state on the allow-list. Each case names the chi route template the
// request is served by, as the middleware resolves it.
func TestCheckWorkspaceAccessRefusesAViewerWrite(t *testing.T) {
	cases := []struct {
		name    string
		method  string
		pattern string
		allowed bool
	}{
		{"read a list", http.MethodGet, "/api/issues/", true},
		{"read one issue", http.MethodGet, "/api/issues/{id}", true},
		{"head", http.MethodHead, "/api/issues/", true},
		{"create an issue", http.MethodPost, "/api/issues/", false},
		{"edit an issue", http.MethodPatch, "/api/issues/{id}", false},
		{"delete an issue", http.MethodDelete, "/api/issues/{id}", false},
		{"invite a member", http.MethodPost, "/api/workspaces/{id}/members", false},
		{"change a member role", http.MethodPatch, "/api/workspaces/{id}/members/{memberId}", false},
		{"unknown route", http.MethodPost, "", false},

		// The allow-list: personal state only.
		{"mark one inbox item read", http.MethodPost, "/api/inbox/{id}/read", true},
		{"mark one issue's inbox read", http.MethodPost, "/api/inbox/issues/{issueId}/read", true},
		{"mark the whole inbox read", http.MethodPost, "/api/inbox/mark-all-read", true},
		{"patch notification preferences", http.MethodPatch, "/api/notification-preferences/", true},
		{"replace notification preferences", http.MethodPut, "/api/notification-preferences/", true},
		{"pin an item", http.MethodPost, "/api/pins/", true},
		{"reorder pins", http.MethodPut, "/api/pins/reorder", true},
		{"unpin an item", http.MethodDelete, "/api/pins/{itemType}/{itemId}", true},
		{"leave the workspace", http.MethodPost, "/api/workspaces/{id}/leave", true},

		// Not on the allow-list: a write that only looks like personal state.
		{"archive an inbox item", http.MethodPost, "/api/inbox/{id}/archive", false},
		{"delete the workspace", http.MethodDelete, "/api/workspaces/{id}", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for _, machine := range []bool{false, true} {
				s := Session{Role: "viewer", Method: c.method, RoutePattern: c.pattern, Machine: machine}
				got := CheckWorkspaceAccess(workspacepolicy.Default(), s, time.Now())
				want := CodeViewerReadOnly
				if c.allowed {
					want = ""
				}
				if got != want {
					t.Errorf("machine=%v %s %s: CheckWorkspaceAccess = %q, want %q", machine, c.method, c.pattern, got, want)
				}
			}
		})
	}
}

// A member, an admin or an owner is never refused by the viewer rule, whatever
// the method, so the rule cannot leak into the roles that write.
func TestCheckWorkspaceAccessDoesNotRefuseWritersForTheViewerRule(t *testing.T) {
	for _, role := range []string{"owner", "admin", "member", ""} {
		s := Session{Role: role, Method: http.MethodPost, RoutePattern: "/api/issues/"}
		if got := CheckWorkspaceAccess(workspacepolicy.Default(), s, time.Now()); got != "" {
			t.Errorf("role %q: CheckWorkspaceAccess = %q, want no refusal", role, got)
		}
	}
}

func TestViewerWriteRefusalMessage(t *testing.T) {
	if got := Message(CodeViewerReadOnly); got != "viewers can only read this workspace" {
		t.Errorf("Message(%q) = %q", CodeViewerReadOnly, got)
	}
}
