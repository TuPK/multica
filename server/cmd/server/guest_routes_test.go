package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/multica-ai/multica/server/internal/analytics"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/realtime"
	"github.com/multica-ai/multica/server/internal/workspaceguard"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Spec 7.1 rule 9 (D53): a guest reaches only the routes on the guest allow-list
// (workspaceguard.GuestAllows). chi.Walk hands each workspace route its own
// middleware chain; each route is mounted with that chain in front of a sentinel
// handler, so no real handler runs. A workspace route outside the allow-list must
// be refused with 403 guest_forbidden; an allow-listed route must reach its
// handler. Routes that do not take a workspace (the user's own account, sign-in,
// daemon, webhooks) carry no workspace guard and are outside this walk.
//
// Database-backed: it runs only when the integration fixture is up.
func TestGuestRouteTableDefaultsToForbidden(t *testing.T) {
	if testPool == nil {
		t.Skip("integration database not available")
	}
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, `UPDATE member SET role = 'member', is_guest = true WHERE workspace_id = $1 AND user_id = $2`, testWorkspaceID, testUserID); err != nil {
		t.Fatalf("make the test user a guest: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `UPDATE member SET role = 'owner', is_guest = false WHERE workspace_id = $1 AND user_id = $2`, testWorkspaceID, testUserID)
	})

	// The guard is identified by its code: every workspace guard is built by the
	// same function literal, so they share one code pointer.
	guard := reflect.ValueOf(middleware.RequireWorkspaceMember(db.New(testPool))).Pointer()

	router := NewRouter(testPool, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil)
	scoped, reachedAllowed := 0, 0
	err := chi.Walk(router, func(method, route string, _ http.Handler, mws ...func(http.Handler) http.Handler) error {
		if !hasWorkspaceGuard(mws, guard) {
			return nil
		}
		scoped++
		key := method + " " + route
		reached, status, code := serveAsGuest(method, route, mws)
		allowed := workspaceguard.GuestAllows(method, route)
		switch {
		case allowed && !reached:
			t.Errorf("%s: the guest allow-list admits a guest, but the request was refused with %d %q", key, status, code)
		case allowed:
			reachedAllowed++
		case reached:
			t.Errorf("%s: a guest reached the handler of a route outside the guest allow-list", key)
		case status != http.StatusForbidden || code != workspaceguard.CodeGuestForbidden:
			t.Errorf("%s: a guest was refused with %d %q, want 403 %s", key, status, code, workspaceguard.CodeGuestForbidden)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk routes: %v", err)
	}
	if scoped == 0 {
		t.Fatal("no workspace routes found; the walk is broken")
	}
	if reachedAllowed == 0 {
		t.Fatal("no allow-listed route reached a guest; the allow-list or the walk is broken")
	}
}

func hasWorkspaceGuard(mws []func(http.Handler) http.Handler, guard uintptr) bool {
	for _, mw := range mws {
		if reflect.ValueOf(mw).Pointer() == guard {
			return true
		}
	}
	return false
}

// serveAsGuest runs one route's middleware chain as the test user, a guest
// member of the test workspace. It reports whether the sentinel was reached, the
// status the chain wrote when it was not, and the refusal code it carried.
func serveAsGuest(method, route string, mws []func(http.Handler) http.Handler) (bool, int, string) {
	reached := false
	sentinel := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusNoContent)
	})
	mux := chi.NewRouter()
	mux.With(mws...).Method(method, route, sentinel)

	path := pathParam.ReplaceAllStringFunc(route, func(p string) string {
		if p == "{id}" {
			return testWorkspaceID
		}
		return "00000000-0000-4000-8000-000000000001"
	})
	// A wildcard route matches any tail; a concrete segment stands in for it.
	path = strings.ReplaceAll(path, "/*", "/x")
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("X-Workspace-ID", testWorkspaceID)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var body struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return reached, rec.Code, body.Code
}
