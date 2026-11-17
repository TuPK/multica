package middleware

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// guestRouter mounts routes behind the workspace member guard for a member with
// role. The guest flag is set on the member row the guard reads.
func guestRouter(role string) http.Handler {
	queries := db.New(memberDB{role: role, guest: true})
	ok := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(RequireWorkspaceMember(queries))
		r.Get("/api/issues/", ok)
		r.Post("/api/issues/", ok)
		r.Get("/api/issues/{id}", ok)
		r.Delete("/api/issues/{id}", ok)
		r.Post("/api/inbox/{id}/read", ok)
		r.Post("/api/agents/", ok)
		r.Get("/api/workspaces/{id}/members", ok)
	})
	return r
}

// Spec 7.1 rule 9: a guest reaches only the routes of the guest allow-list. A
// route outside it is refused with 403 guest_forbidden, whatever its method, and
// the refusal comes before any role check.
func TestGuestIsRefusedOutsideTheAllowList(t *testing.T) {
	cases := []struct {
		method, path string
		want         int
		code         string
	}{
		{http.MethodGet, "/api/issues/", http.StatusNoContent, ""},
		{http.MethodPost, "/api/issues/", http.StatusNoContent, ""},
		{http.MethodGet, "/api/issues/abc", http.StatusNoContent, ""},
		{http.MethodPost, "/api/inbox/abc/read", http.StatusNoContent, ""},
		{http.MethodDelete, "/api/issues/abc", http.StatusForbidden, "guest_forbidden"},
		{http.MethodPost, "/api/agents/", http.StatusForbidden, "guest_forbidden"},
		{http.MethodGet, "/api/workspaces/abc/members", http.StatusForbidden, "guest_forbidden"},
	}
	for _, machine := range []bool{false, true} {
		h := guestRouter("member")
		for _, c := range cases {
			rec := serveViewer(h, c.method, c.path, machine)
			if rec.Code != c.want {
				t.Errorf("machine=%v %s %s: status %d, want %d (body %q)", machine, c.method, c.path, rec.Code, c.want, rec.Body.String())
				continue
			}
			if c.code != "" && codeOf(t, rec) != c.code {
				t.Errorf("machine=%v %s %s: code %q, want %q", machine, c.method, c.path, codeOf(t, rec), c.code)
			}
		}
	}
}

// Spec 7.1 rule 9: a guest's writes follow the role the guest holds. A guest
// viewer reads what the allow-list admits and writes nothing; a guest member
// writes the allow-listed routes.
func TestGuestWriteFollowsRole(t *testing.T) {
	viewer := guestRouter("viewer")
	if rec := serveViewer(viewer, http.MethodGet, "/api/issues/", false); rec.Code != http.StatusNoContent {
		t.Errorf("guest viewer GET /api/issues/: status %d, want 204", rec.Code)
	}
	rec := serveViewer(viewer, http.MethodPost, "/api/issues/", false)
	if rec.Code != http.StatusForbidden || codeOf(t, rec) != "viewer_read_only" {
		t.Errorf("guest viewer POST /api/issues/: status %d code %q, want 403 viewer_read_only", rec.Code, codeOf(t, rec))
	}

	member := guestRouter("member")
	if rec := serveViewer(member, http.MethodPost, "/api/issues/", false); rec.Code != http.StatusNoContent {
		t.Errorf("guest member POST /api/issues/: status %d, want 204 (body %q)", rec.Code, rec.Body.String())
	}
}
