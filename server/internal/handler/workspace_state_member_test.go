package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/multica-ai/multica/platform-management/workspacestate"
	"github.com/multica-ai/multica/server/internal/middleware"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// NOT RUN where this was written: this package's TestMain needs a database.
// The test itself uses fakes only. It checks that requireWorkspaceMember
// enforces the workspace state the way the workspace middleware does.

const (
	wsStateTestWorkspace = "22222222-2222-4222-8222-222222222222"
	wsStateTestUser      = "33333333-3333-4333-8333-333333333333"
)

type wsStateMemberDB struct {
	db.DBTX
	role string
	none bool
}

func (m wsStateMemberDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return wsStateMemberRow{role: m.role, none: m.none}
}

type wsStateMemberRow struct {
	pgx.Row
	role string
	none bool
}

func (r wsStateMemberRow) Scan(dest ...any) error {
	if r.none {
		return pgx.ErrNoRows
	}
	// GetMemberByUserAndWorkspace columns: id, workspace_id, user_id, role,
	// created_at, is_guest, pinned_at.
	role, ok := dest[3].(*string)
	if !ok || len(dest) != 7 {
		return errors.New("unexpected member column layout")
	}
	*role = r.role
	return nil
}

type wsStateStore struct {
	rec   workspacestate.Record
	found bool
	err   error
	calls int
}

func (s *wsStateStore) WorkspaceState(context.Context, string) (workspacestate.Record, bool, error) {
	s.calls++
	return s.rec, s.found, s.err
}

type wsStateNoCache struct{}

func (wsStateNoCache) Get(context.Context, string) ([]byte, bool, error)        { return nil, false, nil }
func (wsStateNoCache) Set(context.Context, string, []byte, time.Duration) error { return nil }
func (wsStateNoCache) Del(context.Context, string) error                        { return nil }

func wsStateStoreFor(status workspacestate.Status) *wsStateStore {
	if status == workspacestate.Active {
		return &wsStateStore{}
	}
	return &wsStateStore{rec: workspacestate.Record{Status: status}, found: true}
}

// wsStateRouter mounts routes whose handlers authorize through
// requireWorkspaceMember, as the handlers outside the workspace middleware do.
// The same routes also run behind the middleware, so the two bodies compare.
func wsStateRouter(store *wsStateStore, role string, mountGuard bool) http.Handler {
	h := &Handler{Queries: db.New(wsStateMemberDB{role: role})}
	member := func(w http.ResponseWriter, r *http.Request) {
		if _, ok := h.requireWorkspaceMember(w, r, wsStateTestWorkspace, "workspace not found"); !ok {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
	r := chi.NewRouter()
	if mountGuard {
		r.Use(middleware.WithWorkspaceState(workspacestate.New(store, wsStateNoCache{})))
	}
	r.Get("/self/issues", member)
	r.Post("/self/issues", member)
	r.Delete("/self/issues/{id}", member)
	r.Get("/api/company/export", member)
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireWorkspaceMember(h.Queries))
		r.Post("/mw/issues", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	})
	return r
}

func wsStateServe(handler http.Handler, method, path string, daemon bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("X-User-ID", wsStateTestUser)
	req.Header.Set("X-Workspace-ID", wsStateTestWorkspace)
	if daemon {
		req = req.WithContext(middleware.WithDaemonContext(req.Context(), wsStateTestWorkspace, "daemon-1"))
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestRequireWorkspaceMemberRefusesAWriteInAReadOnlyWorkspace(t *testing.T) {
	rec := wsStateServe(wsStateRouter(wsStateStoreFor(workspacestate.ReadOnly), "owner", true), "POST", "/self/issues", false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403", rec.Code)
	}
	want := `{"code":"workspace_read_only","error":"workspace is read-only"}` + "\n"
	if rec.Body.String() != want {
		t.Fatalf("body %q, want %q", rec.Body.String(), want)
	}
}

func TestRequireWorkspaceMemberBodyMatchesTheMiddleware(t *testing.T) {
	for _, status := range []workspacestate.Status{workspacestate.ReadOnly, workspacestate.Suspended, workspacestate.PendingDeletion} {
		router := wsStateRouter(wsStateStoreFor(status), "owner", true)
		viaHandler := wsStateServe(router, "POST", "/self/issues", false)
		viaMiddleware := wsStateServe(router, "POST", "/mw/issues", false)
		if viaHandler.Code != viaMiddleware.Code || viaHandler.Body.String() != viaMiddleware.Body.String() {
			t.Errorf("%s: handler %d %q, middleware %d %q", status,
				viaHandler.Code, viaHandler.Body.String(), viaMiddleware.Code, viaMiddleware.Body.String())
		}
	}
	errRouter := wsStateRouter(&wsStateStore{err: errors.New("db down")}, "owner", true)
	a, b := wsStateServe(errRouter, "POST", "/self/issues", false), wsStateServe(errRouter, "POST", "/mw/issues", false)
	if a.Code != http.StatusServiceUnavailable || a.Code != b.Code || a.Body.String() != b.Body.String() {
		t.Errorf("state error: handler %d %q, middleware %d %q", a.Code, a.Body.String(), b.Code, b.Body.String())
	}
}

func TestRequireWorkspaceMemberAppliesTheDecisionTable(t *testing.T) {
	cases := []struct {
		name   string
		status workspacestate.Status
		role   string
		method string
		path   string
		want   int // 204 served, 403 refused
		code   string
	}{
		{"read in read_only", workspacestate.ReadOnly, "member", "GET", "/self/issues", 204, ""},
		{"delete in read_only", workspacestate.ReadOnly, "member", "DELETE", "/self/issues/abc", 204, ""},
		{"read in suspended", workspacestate.Suspended, "owner", "GET", "/self/issues", 403, "workspace_suspended"},
		{"write in pending_deletion", workspacestate.PendingDeletion, "owner", "POST", "/self/issues", 403, "workspace_pending_deletion"},
		{"export, owner, suspended", workspacestate.Suspended, "owner", "GET", "/api/company/export", 204, ""},
		{"export, member, suspended", workspacestate.Suspended, "member", "GET", "/api/company/export", 403, "workspace_suspended"},
		{"write in active", workspacestate.Active, "member", "POST", "/self/issues", 204, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := wsStateServe(wsStateRouter(wsStateStoreFor(c.status), c.role, true), c.method, c.path, false)
			if rec.Code != c.want {
				t.Fatalf("status %d, want %d (%s)", rec.Code, c.want, rec.Body.String())
			}
			if c.code != "" && !strings.Contains(rec.Body.String(), `"code":"`+c.code+`"`) {
				t.Fatalf("body %q lacks code %q", rec.Body.String(), c.code)
			}
		})
	}
}

func TestRequireWorkspaceMemberWithoutAGuardPasses(t *testing.T) {
	store := wsStateStoreFor(workspacestate.Suspended)
	rec := wsStateServe(wsStateRouter(store, "owner", false), "POST", "/self/issues", false)
	if rec.Code != http.StatusNoContent || store.calls != 0 {
		t.Fatalf("status %d, state reads %d; want served with no state read", rec.Code, store.calls)
	}
}

func TestRequireWorkspaceMemberChecksMembershipFirst(t *testing.T) {
	store := wsStateStoreFor(workspacestate.Suspended)
	h := &Handler{Queries: db.New(wsStateMemberDB{none: true})}
	r := chi.NewRouter()
	r.Use(middleware.WithWorkspaceState(workspacestate.New(store, wsStateNoCache{})))
	r.Get("/x", func(w http.ResponseWriter, r *http.Request) {
		h.requireWorkspaceMember(w, r, wsStateTestWorkspace, "workspace not found")
	})
	rec := wsStateServe(r, "GET", "/x", false)
	if rec.Code != http.StatusNotFound || store.calls != 0 {
		t.Fatalf("status %d, state reads %d; want 404 and no state read for a non-member", rec.Code, store.calls)
	}
}

// The daemon routes (daemon.go: requireDaemonWorkspaceAccess and the register
// path, reached with a daemon token or a PAT/JWT fallback) call
// requireWorkspaceMember. A daemon request must pass in every state, without
// reading the state, so a state outage cannot stop a running task reporting.
func TestRequireWorkspaceMemberLetsDaemonRequestsThroughInEveryState(t *testing.T) {
	for _, status := range []workspacestate.Status{workspacestate.ReadOnly, workspacestate.Suspended, workspacestate.PendingDeletion} {
		for _, method := range []string{"GET", "POST"} {
			store := wsStateStoreFor(status)
			rec := wsStateServe(wsStateRouter(store, "member", true), method, "/self/issues", true)
			if rec.Code != http.StatusNoContent || store.calls != 0 {
				t.Errorf("%s %s daemon: status %d, state reads %d; want served with no state read", status, method, rec.Code, store.calls)
			}
		}
	}
	store := &wsStateStore{err: errors.New("db down")}
	if rec := wsStateServe(wsStateRouter(store, "member", true), "POST", "/self/issues", true); rec.Code != http.StatusNoContent {
		t.Errorf("a state outage stopped a daemon request: %d", rec.Code)
	}
}
