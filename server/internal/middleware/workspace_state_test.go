package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/multica-ai/multica/platform-management/workspacestate"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

const (
	stateTestWorkspace = "22222222-2222-4222-8222-222222222222"
	stateTestUser      = "33333333-3333-4333-8333-333333333333"
)

// memberDB answers GetMemberByUserAndWorkspace with a member of role, a guest
// when guest is set.
type memberDB struct {
	db.DBTX
	role  string
	guest bool
}

func (m memberDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return memberRow{role: m.role, guest: m.guest}
}

type memberRow struct {
	pgx.Row
	role  string
	guest bool
}

func (r memberRow) Scan(dest ...any) error {
	// Column order of GetMemberByUserAndWorkspace: id, workspace_id, user_id,
	// role, created_at, is_guest, pinned_at.
	role, ok := dest[3].(*string)
	if !ok || len(dest) != 7 {
		return errors.New("unexpected member column layout")
	}
	guest, ok := dest[5].(*bool)
	if !ok {
		return errors.New("unexpected member column layout")
	}
	*role = r.role
	*guest = r.guest
	return nil
}

type fakeStateStore struct {
	rec   workspacestate.Record
	found bool
	err   error
	calls int
	ids   []string
}

func (s *fakeStateStore) WorkspaceState(_ context.Context, id string) (workspacestate.Record, bool, error) {
	s.calls++
	s.ids = append(s.ids, id)
	return s.rec, s.found, s.err
}

type noStateCache struct{}

func (noStateCache) Get(context.Context, string) ([]byte, bool, error)        { return nil, false, nil }
func (noStateCache) Set(context.Context, string, []byte, time.Duration) error { return nil }
func (noStateCache) Del(context.Context, string) error                        { return nil }

func stateStore(status workspacestate.Status) *fakeStateStore {
	if status == workspacestate.Active {
		return &fakeStateStore{}
	}
	return &fakeStateStore{rec: workspacestate.Record{Status: status}, found: true}
}

// stateRouter mounts the workspace middleware the way router.go does: a group
// whose middleware wraps a sub-router mount (so the middleware sees only the
// mount prefix), a role-checked group around a nested Route, a With() on a
// Route whose leaf is "/", and the subscription routes. A hit records the
// handler that served the request.
func stateRouter(store *fakeStateStore, role string, withGuard bool) (http.Handler, *[]string) {
	queries := db.New(memberDB{role: role})
	var served []string
	h := func(name string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			served = append(served, name)
			w.WriteHeader(http.StatusNoContent)
		}
	}
	r := chi.NewRouter()
	if withGuard {
		r.Use(WithWorkspaceState(workspacestate.New(store, noStateCache{})))
	}
	// Workspace-scoped group: the middleware wraps the Route mount.
	r.Group(func(r chi.Router) {
		r.Use(RequireWorkspaceMember(queries))
		r.Route("/api/issues", func(r chi.Router) {
			r.Post("/", h("create-issue"))
			r.Get("/", h("list-issues"))
			r.Post("/batch-delete", h("batch-delete"))
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", h("get-issue"))
				r.Delete("/", h("delete-issue"))
			})
		})
		r.Route("/api/agents/{id}", func(r chi.Router) {
			r.Post("/archive", h("archive-agent"))
		})
		r.Get("/api/company/export", h("export"))
	})
	r.Route("/api/workspaces", func(r chi.Router) {
		r.Route("/{id}", func(r chi.Router) {
			r.Group(func(r chi.Router) {
				r.Use(RequireWorkspaceMemberFromURL(queries, "id"))
				r.Get("/", h("get-workspace"))
				r.Post("/leave", h("leave"))
				r.Get("/members", h("list-members"))
			})
			r.Group(func(r chi.Router) {
				r.Use(RequireWorkspaceRoleFromURL(queries, "id", "owner", "admin"))
				r.Route("/members/{memberId}", func(r chi.Router) {
					r.Patch("/", h("update-member"))
					r.Delete("/", h("remove-member"))
				})
				r.Delete("/invitations/{invitationId}", h("revoke-invitation"))
			})
			r.With(RequireWorkspaceRoleFromURL(queries, "id", "owner")).Delete("/", h("delete-workspace"))
		})
	})
	r.Route("/api/cloud-subscriptions", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(RequireWorkspaceMember(queries))
			r.Get("/summary", h("sub-summary"))
		})
		r.Group(func(r chi.Router) {
			r.Use(RequireWorkspaceRole(queries, "owner", "admin"))
			r.Post("/checkout-sessions", h("sub-checkout"))
		})
	})
	// The daemon routes carry no workspace middleware.
	r.Route("/api/daemon", func(r chi.Router) {
		r.Post("/tasks/{taskId}/complete", h("daemon-complete"))
	})
	return r, &served
}

type stateCase struct {
	method, path string
	handler      string
}

func serveState(t *testing.T, handler http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("X-User-ID", stateTestUser)
	req.Header.Set("X-Workspace-ID", stateTestWorkspace)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func codeOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q is not JSON: %v", rec.Body.String(), err)
	}
	if body.Error == "" {
		t.Fatalf("body %q has no error message", rec.Body.String())
	}
	return body.Code
}

var wsPath = "/api/workspaces/" + stateTestWorkspace

func TestWorkspaceStateRequestPath(t *testing.T) {
	type want struct{ readOnly, suspended, pendingDeletion string } // "" = served; otherwise the refusal code
	const (
		ro  = "workspace_read_only"
		sus = "workspace_suspended"
		pd  = "workspace_pending_deletion"
	)
	cases := []struct {
		name         string
		method, path string
		role         string
		want         want
	}{
		{"read a list", "GET", "/api/issues/", "owner", want{"", sus, pd}},
		{"read an issue (mounted under a group)", "GET", "/api/issues/abc", "owner", want{"", sus, pd}},
		{"create an issue", "POST", "/api/issues/", "owner", want{ro, sus, pd}},
		{"delete an issue", "DELETE", "/api/issues/abc", "owner", want{"", sus, pd}},
		{"batch delete", "POST", "/api/issues/batch-delete", "owner", want{"", sus, pd}},
		{"archive an agent", "POST", "/api/agents/abc/archive", "owner", want{"", sus, pd}},
		{"export, owner", "GET", "/api/company/export", "owner", want{"", "", ""}},
		{"export, member", "GET", "/api/company/export", "member", want{"", sus, pd}},
		{"read the workspace", "GET", wsPath, "member", want{"", "", ""}},
		{"read the members", "GET", wsPath + "/members", "member", want{"", sus, pd}},
		{"leave", "POST", wsPath + "/leave", "member", want{"", "", ""}},
		{"update a member", "PATCH", wsPath + "/members/m1", "owner", want{ro, sus, pd}},
		{"remove a member", "DELETE", wsPath + "/members/m1", "owner", want{"", sus, pd}},
		{"revoke an invitation", "DELETE", wsPath + "/invitations/i1", "admin", want{"", sus, pd}},
		{"delete the workspace", "DELETE", wsPath, "owner", want{"", sus, pd}},
		{"subscription summary, owner", "GET", "/api/cloud-subscriptions/summary", "owner", want{"", "", ""}},
		{"subscription summary, member", "GET", "/api/cloud-subscriptions/summary", "member", want{"", sus, pd}},
		{"subscription checkout, admin", "POST", "/api/cloud-subscriptions/checkout-sessions", "admin", want{"", "", ""}},
	}
	for _, c := range cases {
		for _, status := range []struct {
			s    workspacestate.Status
			want string
		}{
			{workspacestate.ReadOnly, c.want.readOnly},
			{workspacestate.Suspended, c.want.suspended},
			{workspacestate.PendingDeletion, c.want.pendingDeletion},
		} {
			t.Run(c.name+"/"+string(status.s), func(t *testing.T) {
				h, served := stateRouter(stateStore(status.s), c.role, true)
				rec := serveState(t, h, c.method, c.path)
				if status.want == "" {
					if rec.Code != http.StatusNoContent || len(*served) != 1 {
						t.Fatalf("status %d served %v, want the handler to serve", rec.Code, *served)
					}
					return
				}
				if rec.Code != http.StatusForbidden || len(*served) != 0 {
					t.Fatalf("status %d served %v, want 403 and no handler", rec.Code, *served)
				}
				if got := codeOf(t, rec); got != status.want {
					t.Fatalf("code = %q, want %q", got, status.want)
				}
			})
		}
	}
}

func TestWorkspaceStateActiveServesEverything(t *testing.T) {
	for _, c := range []struct{ method, path string }{
		{"POST", "/api/issues/"}, {"GET", "/api/issues/"}, {"PATCH", wsPath + "/members/m1"},
		{"DELETE", wsPath}, {"GET", "/api/company/export"},
	} {
		store := stateStore(workspacestate.Active)
		h, served := stateRouter(store, "owner", true)
		if rec := serveState(t, h, c.method, c.path); rec.Code != http.StatusNoContent || len(*served) != 1 {
			t.Errorf("%s %s: status %d served %v", c.method, c.path, rec.Code, *served)
		}
		if len(store.ids) != 1 || store.ids[0] != stateTestWorkspace {
			t.Errorf("%s %s: state read for %v, want the workspace once", c.method, c.path, store.ids)
		}
	}
}

func TestWorkspaceStateNilGuardPasses(t *testing.T) {
	store := stateStore(workspacestate.Suspended)
	h, served := stateRouter(store, "owner", false)
	if rec := serveState(t, h, "POST", "/api/issues/"); rec.Code != http.StatusNoContent || len(*served) != 1 {
		t.Fatalf("status %d served %v, want served with no guard mounted", rec.Code, *served)
	}
	if store.calls != 0 {
		t.Fatalf("store read %d times with no guard mounted", store.calls)
	}
	var nilGuard *workspacestate.Guard
	r := chi.NewRouter()
	r.Use(WithWorkspaceState(nilGuard))
	r.Use(RequireWorkspaceMember(db.New(memberDB{role: "owner"})))
	r.Get("/x", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	if rec := serveState(t, r, "GET", "/x"); rec.Code != http.StatusNoContent {
		t.Fatalf("a nil guard refused: %d", rec.Code)
	}
}

func TestWorkspaceStateErrorIs503(t *testing.T) {
	store := &fakeStateStore{err: errors.New("db down")}
	h, served := stateRouter(store, "owner", true)
	for _, method := range []string{"GET", "POST"} {
		rec := serveState(t, h, method, "/api/issues/")
		if rec.Code != http.StatusServiceUnavailable || len(*served) != 0 {
			t.Fatalf("%s: status %d served %v, want 503 and no handler", method, rec.Code, *served)
		}
		if got := codeOf(t, rec); got != "workspace_state_unavailable" {
			t.Fatalf("%s: code = %q", method, got)
		}
	}
}

func TestWorkspaceStateChecksAfterMembership(t *testing.T) {
	// A non-member learns nothing about the workspace's state: the membership
	// check answers first.
	store := stateStore(workspacestate.Suspended)
	r := chi.NewRouter()
	r.Use(WithWorkspaceState(workspacestate.New(store, noStateCache{})))
	r.Use(RequireWorkspaceMember(db.New(noMemberDB{})))
	r.Get("/x", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	rec := serveState(t, r, "GET", "/x")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("non-member got %d, want 404", rec.Code)
	}
	if store.calls != 0 {
		t.Fatalf("state read %d times for a non-member", store.calls)
	}
}

type noMemberDB struct{ db.DBTX }

func (noMemberDB) QueryRow(context.Context, string, ...any) pgx.Row { return errRow{} }

type errRow struct{ pgx.Row }

func (errRow) Scan(...any) error { return pgx.ErrNoRows }

func TestWorkspaceStateUsesTheCanonicalWorkspaceID(t *testing.T) {
	// The cache key is the workspace's canonical text form, so a request that
	// spells the id in upper case must read the same key the platform clears.
	store := stateStore(workspacestate.Active)
	h, _ := stateRouter(store, "owner", true)
	req := httptest.NewRequest("GET", "/api/issues/", nil)
	req.Header.Set("X-User-ID", stateTestUser)
	req.Header.Set("X-Workspace-ID", strings.ToUpper(stateTestWorkspace))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("request was not served: %d", rec.Code)
	}
	if len(store.ids) != 1 || store.ids[0] != stateTestWorkspace {
		t.Fatalf("state read for %v, want %q", store.ids, stateTestWorkspace)
	}
}

func TestWorkspaceStateDaemonRouteIsNotGated(t *testing.T) {
	store := stateStore(workspacestate.Suspended)
	h, served := stateRouter(store, "owner", true)
	rec := serveState(t, h, "POST", "/api/daemon/tasks/t1/complete")
	if rec.Code != http.StatusNoContent || len(*served) != 1 || (*served)[0] != "daemon-complete" {
		t.Fatalf("status %d served %v, want the daemon handler to serve a suspended workspace", rec.Code, *served)
	}
	if store.calls != 0 {
		t.Fatalf("a daemon route read the workspace state %d times", store.calls)
	}
}

func TestWorkspaceStateUnknownStatusRefuses(t *testing.T) {
	h, served := stateRouter(stateStore("deleted"), "owner", true)
	rec := serveState(t, h, "GET", "/api/issues/")
	if rec.Code != http.StatusForbidden || len(*served) != 0 {
		t.Fatalf("status %d served %v", rec.Code, *served)
	}
	if got := codeOf(t, rec); got != "workspace_suspended" {
		t.Fatalf("code = %q", got)
	}
}
