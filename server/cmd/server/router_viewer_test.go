package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/multica-ai/multica/server/internal/analytics"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/realtime"
)

// viewerAllowList is the writes a viewer may make, copied from the viewer rule
// (workspaceguard.viewerWrites). Each entry is personal state, not workspace
// content. The test refuses to run if one is no longer a registered route.
var viewerAllowList = map[string]string{
	"POST /api/inbox/{id}/read":             "marks the viewer's own inbox item read",
	"POST /api/inbox/issues/{issueId}/read": "marks the viewer's own inbox items read",
	"POST /api/inbox/mark-all-read":         "marks the viewer's own inbox read",
	"PATCH /api/notification-preferences":   "the viewer's own notification settings",
	"PUT /api/notification-preferences":     "the viewer's own notification settings",
	"POST /api/pins":                        "the viewer's own pins",
	"PUT /api/pins/reorder":                 "the viewer's own pins",
	"DELETE /api/pins/{itemType}/{itemId}":  "the viewer's own pins",
	"POST /api/workspaces/{id}/leave":       "a viewer may always leave",
}

// minViewerWriteRoutes guards against the walk silently seeing a stub router.
const minViewerWriteRoutes = 100

// isWorkspaceMiddleware reports whether mw is one of the middlewares built by
// the workspace member/role/permission constructors.
func isWorkspaceMiddleware(mw func(http.Handler) http.Handler) bool {
	fn := runtime.FuncForPC(reflect.ValueOf(mw).Pointer())
	return fn != nil && strings.Contains(fn.Name(), "middleware.buildMiddleware")
}

// AC19 through the real router and the real workspace middleware: a viewer is
// refused every workspace write route except the allow-list, and cannot
// register a runtime.
//
// Database-backed. TestMain connects through DATABASE_URL, so this test runs
// only when PLATFORM_TEST_DATABASE_URL is set to the same dedicated *_test DSN.
func TestViewerIsRefusedEveryWriteRouteOfTheRealRouter(t *testing.T) {
	dsn := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if dsn == "" || dsn != os.Getenv("DATABASE_URL") {
		t.Skip("PLATFORM_TEST_DATABASE_URL not set or differs from DATABASE_URL")
	}
	ctx := context.Background()

	const slug = "integration-tests-viewer-walk"
	_, _ = testPool.Exec(ctx, `DELETE FROM workspace WHERE slug = $1`, slug)
	var wsID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO workspace (name, slug, description)
		VALUES ($1, $2, $3)
		RETURNING id
	`, "Integration Tests Viewer Walk", slug, "viewer route walk").Scan(&wsID); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = testPool.Exec(bg, `DELETE FROM member WHERE workspace_id = $1`, wsID)
		_, _ = testPool.Exec(bg, `DELETE FROM workspace WHERE id = $1`, wsID)
	})
	if _, err := testPool.Exec(ctx, `INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'viewer')`, wsID, testUserID); err != nil {
		t.Fatalf("create viewer: %v", err)
	}

	call := func(method, path, body string) (int, string) {
		t.Helper()
		req, err := http.NewRequest(method, testServer.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+testToken)
		req.Header.Set("X-Workspace-ID", wsID)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer resp.Body.Close()
		var out struct {
			Code string `json:"code"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out.Code
	}

	router := NewRouter(testPool, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil)
	registered := map[string]bool{}
	type route struct{ key, path string }
	var writes []route
	err := chi.Walk(router, func(method, pattern string, _ http.Handler, mws ...func(http.Handler) http.Handler) error {
		pattern = strings.TrimSuffix(strings.ReplaceAll(pattern, "/*/", "/"), "/")
		if pattern == "" {
			pattern = "/"
		}
		registered[method+" "+pattern] = true
		switch method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		default:
			return nil
		}
		if !strings.HasPrefix(pattern, "/api/") {
			return nil
		}
		workspaceRoute := false
		for _, mw := range mws {
			if isWorkspaceMiddleware(mw) {
				workspaceRoute = true
				break
			}
		}
		if workspaceRoute {
			writes = append(writes, route{method + " " + pattern, pattern})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk router: %v", err)
	}

	for key := range viewerAllowList {
		if !registered[key] {
			t.Errorf("allow-list entry %q is not a registered route", key)
		}
	}
	if len(writes) < minViewerWriteRoutes {
		t.Fatalf("walked %d workspace write routes, want at least %d", len(writes), minViewerWriteRoutes)
	}
	sort.Slice(writes, func(i, j int) bool { return writes[i].key < writes[j].key })

	checked, byViewerRule := 0, 0
	for _, rt := range writes {
		if _, ok := viewerAllowList[rt.key]; ok {
			continue
		}
		method, pattern, _ := strings.Cut(rt.key, " ")
		path := strings.NewReplacer("{id}", wsID).Replace(pattern)
		if !strings.HasPrefix(pattern, "/api/workspaces/{id}") {
			path = strings.ReplaceAll(pattern, "{id}", "00000000-0000-4000-8000-000000000001")
		}
		for strings.Contains(path, "{") {
			start := strings.Index(path, "{")
			end := strings.Index(path[start:], "}") + start
			path = path[:start] + "00000000-0000-4000-8000-000000000001" + path[end+1:]
		}
		status, code := call(method, path, "{}")
		// A route that also needs a role or a permission refuses a viewer
		// earlier with its own 403; every other route reaches the viewer rule.
		if status != http.StatusForbidden {
			t.Errorf("viewer %s: %d %q, want 403", rt.key, status, code)
		}
		if code == "viewer_read_only" {
			byViewerRule++
		}
		checked++
	}
	if byViewerRule == 0 {
		t.Errorf("no route was refused by the viewer rule (viewer_read_only)")
	}
	t.Logf("walked %d workspace write routes, %d allow-listed, %d refused as viewer (%d by the viewer rule)", len(writes), len(writes)-checked, checked, byViewerRule)

	t.Run("a viewer cannot register a runtime", func(t *testing.T) {
		const key = "POST /api/daemon/register"
		if !registered[key] {
			t.Fatalf("%s is not a registered route", key)
		}
		body := `{"daemon_id":"viewer-walk-daemon","workspace_id":"` + wsID + `","runtimes":[{"name":"x","type":"claude","status":"online"}]}`
		status, code := call(http.MethodPost, "/api/daemon/register", body)
		if status != http.StatusForbidden || code != "viewer_read_only" {
			t.Errorf("viewer %s: %d %q, want 403 viewer_read_only", key, status, code)
		}
	})
}
