package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// viewerRouter mounts one write and one read of each kind behind the workspace
// middleware, so the refusal is decided by the route the request is served by.
func viewerRouter(role string) http.Handler {
	queries := db.New(memberDB{role: role})
	ok := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(RequireWorkspaceMember(queries))
		r.Get("/api/issues/", ok)
		r.Post("/api/issues/", ok)
		r.Post("/api/inbox/{id}/read", ok)
		r.Post("/api/inbox/{id}/archive", ok)
		r.Patch("/api/notification-preferences/", ok)
		r.Post("/api/pins/", ok)
		r.Post("/api/workspaces/{id}/leave", ok)
	})
	return r
}

func serveViewer(h http.Handler, method, path string, machine bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("X-User-ID", stateTestUser)
	req.Header.Set("X-Workspace-ID", stateTestWorkspace)
	if !machine {
		req = req.WithContext(WithSession(req.Context(), []string{"otp"}, 0, false))
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestViewerIsRefusedEveryWriteButTheAllowList(t *testing.T) {
	cases := []struct {
		method, path string
		want         int
		code         string
	}{
		{http.MethodGet, "/api/issues/", http.StatusNoContent, ""},
		{http.MethodPost, "/api/issues/", http.StatusForbidden, "viewer_read_only"},
		{http.MethodPost, "/api/inbox/abc/read", http.StatusNoContent, ""},
		{http.MethodPost, "/api/inbox/abc/archive", http.StatusForbidden, "viewer_read_only"},
		{http.MethodPatch, "/api/notification-preferences/", http.StatusNoContent, ""},
		{http.MethodPost, "/api/pins/", http.StatusNoContent, ""},
		{http.MethodPost, "/api/workspaces/abc/leave", http.StatusNoContent, ""},
	}
	for _, machine := range []bool{false, true} {
		h := viewerRouter("viewer")
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

// The same routes served to a member: the viewer rule does not reach them.
func TestMemberIsNotRefusedByTheViewerRule(t *testing.T) {
	h := viewerRouter("member")
	for _, path := range []string{"/api/issues/", "/api/inbox/abc/archive"} {
		rec := serveViewer(h, http.MethodPost, path, false)
		if rec.Code != http.StatusNoContent {
			t.Errorf("member POST %s: status %d, want 204 (body %q)", path, rec.Code, rec.Body.String())
		}
	}
}
