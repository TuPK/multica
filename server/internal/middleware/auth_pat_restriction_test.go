package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/auth/authtest"
)

const (
	patTestToken = "mul_restriction_test_token"
	patTestWsA   = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	patTestWsB   = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
)

type patRequest struct {
	method, path string
	header       map[string]string
}

// servePAT runs one request through mw with a cached PAT entry and reports the
// response and whether the wrapped handler ran. Queries are nil: the entry is
// served from the cache, so a request that reaches the database fails.
func servePAT(t *testing.T, build func(*auth.PATCache) func(http.Handler) http.Handler, entry auth.PATEntry, req patRequest) (*httptest.ResponseRecorder, bool) {
	t.Helper()
	cache := auth.NewPATCache(authtest.NewFakeRedis())
	cache.SetEntry(context.Background(), auth.HashToken(patTestToken), entry, auth.AuthCacheTTL)
	return servePATWith(t, cache, build, req)
}

func servePATWith(t *testing.T, cache *auth.PATCache, build func(*auth.PATCache) func(http.Handler) http.Handler, req patRequest) (*httptest.ResponseRecorder, bool) {
	t.Helper()
	called := false
	h := build(cache)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	r := httptest.NewRequest(req.method, req.path, nil)
	r.Header.Set("Authorization", "Bearer "+patTestToken)
	for k, v := range req.header {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w, called
}

func buildAuth(cache *auth.PATCache) func(http.Handler) http.Handler {
	return Auth(nil, cache, nil, nil, nil)
}

func buildDaemonAuth(cache *auth.PATCache) func(http.Handler) http.Handler {
	return DaemonAuth(nil, cache, nil, nil, nil)
}

func wantCoded(t *testing.T, w *httptest.ResponseRecorder, called bool, code string) {
	t.Helper()
	if called {
		t.Fatalf("handler ran; want refusal %s", code)
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (%s)", w.Code, w.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not the coded JSON shape: %v (%q)", err, w.Body.String())
	}
	if body["code"] != code || body["error"] == "" {
		t.Fatalf("body = %v, want code %q and an error message", body, code)
	}
}

func wantAllowed(t *testing.T, w *httptest.ResponseRecorder, called bool) {
	t.Helper()
	if !called || w.Code != http.StatusOK {
		t.Fatalf("called=%v status=%d body=%s; want the request to pass", called, w.Code, w.Body.String())
	}
}

func TestAuth_ReadOnlyPAT_SafeMethodsPass_WritesRefused(t *testing.T) {
	entry := auth.PATEntry{UserID: "user-1", ReadOnly: true}
	hdr := map[string]string{"X-Workspace-ID": patTestWsA}
	for _, m := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		t.Run(m, func(t *testing.T) {
			w, called := servePAT(t, buildAuth, entry, patRequest{m, "/api/issues", hdr})
			wantAllowed(t, w, called)
		})
	}
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(m, func(t *testing.T) {
			w, called := servePAT(t, buildAuth, entry, patRequest{m, "/api/issues", hdr})
			wantCoded(t, w, called, "token_read_only")
		})
	}
}

func TestAuth_BoundPAT_WorkspaceScope(t *testing.T) {
	entry := auth.PATEntry{UserID: "user-1", WorkspaceID: patTestWsA}
	cases := []struct {
		name string
		req  patRequest
		want string // "" = allowed
	}{
		{"own workspace by header", patRequest{"GET", "/api/issues", map[string]string{"X-Workspace-ID": patTestWsA}}, ""},
		{"own workspace by query", patRequest{"GET", "/api/issues?workspace_id=" + patTestWsA, nil}, ""},
		{"own workspace by url, write", patRequest{"POST", "/api/workspaces/" + patTestWsA + "/members", nil}, ""},
		{"other workspace by header", patRequest{"GET", "/api/issues", map[string]string{"X-Workspace-ID": patTestWsB}}, "token_workspace_mismatch"},
		{"other workspace by query", patRequest{"GET", "/api/issues?workspace_id=" + patTestWsB, nil}, "token_workspace_mismatch"},
		{"other workspace by url", patRequest{"GET", "/api/workspaces/" + patTestWsB + "/members", nil}, "token_workspace_mismatch"},
		{"own header, other query", patRequest{"GET", "/api/issues?workspace_id=" + patTestWsB, map[string]string{"X-Workspace-ID": patTestWsA}}, "token_workspace_mismatch"},
		{"other header, own query", patRequest{"GET", "/api/issues?workspace_id=" + patTestWsA, map[string]string{"X-Workspace-ID": patTestWsB}}, "token_workspace_mismatch"},
		{"not a uuid", patRequest{"GET", "/api/issues", map[string]string{"X-Workspace-ID": "nope"}}, "token_workspace_mismatch"},
		{"slug that cannot be resolved", patRequest{"GET", "/api/issues", map[string]string{"X-Workspace-Slug": "some-slug"}}, "token_workspace_mismatch"},
		{"GET /api/me without workspace", patRequest{"GET", "/api/me", nil}, ""},
		{"GET /api/me with other workspace", patRequest{"GET", "/api/me", map[string]string{"X-Workspace-ID": patTestWsB}}, "token_workspace_mismatch"},
		{"POST /api/me without workspace", patRequest{"POST", "/api/me", nil}, "token_workspace_mismatch"},
		{"PATCH /api/me without workspace", patRequest{"PATCH", "/api/me", nil}, "token_workspace_mismatch"},
		{"other route without workspace", patRequest{"GET", "/api/workspaces", nil}, "token_workspace_mismatch"},
		{"token routes without workspace", patRequest{"POST", "/api/tokens", nil}, "token_workspace_mismatch"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w, called := servePAT(t, buildAuth, entry, c.req)
			if c.want == "" {
				wantAllowed(t, w, called)
			} else {
				wantCoded(t, w, called, c.want)
			}
		})
	}
}

// A token created before the change has no restriction and keeps full access:
// any workspace, any method, no workspace context.
func TestAuth_UnrestrictedPAT_KeepsFullAccess(t *testing.T) {
	entry := auth.PATEntry{UserID: "user-1"}
	for _, req := range []patRequest{
		{"POST", "/api/issues", map[string]string{"X-Workspace-ID": patTestWsA}},
		{"DELETE", "/api/issues/x", map[string]string{"X-Workspace-ID": patTestWsB}},
		{"POST", "/api/tokens", nil},
		{"GET", "/api/workspaces", nil},
	} {
		w, called := servePAT(t, buildAuth, entry, req)
		wantAllowed(t, w, called)
	}
}

func TestAuth_BoundReadOnlyPAT_AppliesBoth(t *testing.T) {
	entry := auth.PATEntry{UserID: "user-1", WorkspaceID: patTestWsA, ReadOnly: true}
	w, called := servePAT(t, buildAuth, entry, patRequest{"GET", "/api/issues", map[string]string{"X-Workspace-ID": patTestWsA}})
	wantAllowed(t, w, called)
	w, called = servePAT(t, buildAuth, entry, patRequest{"POST", "/api/issues", map[string]string{"X-Workspace-ID": patTestWsA}})
	wantCoded(t, w, called, "token_read_only")
	w, called = servePAT(t, buildAuth, entry, patRequest{"GET", "/api/issues", map[string]string{"X-Workspace-ID": patTestWsB}})
	wantCoded(t, w, called, "token_workspace_mismatch")
}

// An entry under the old prefix carries no restrictions. It must not be
// trusted: the middleware goes to the database instead (nil here, so 401).
func TestAuth_OldPrefixCacheEntryIsIgnored(t *testing.T) {
	fake := authtest.NewFakeRedis()
	fake.Put("mul:auth:pat:"+auth.HashToken(patTestToken), "user-1")
	cache := auth.NewPATCache(fake)
	w, called := servePATWith(t, cache, buildAuth, patRequest{"POST", "/api/issues", map[string]string{"X-Workspace-ID": patTestWsB}})
	if called || w.Code != http.StatusUnauthorized {
		t.Fatalf("called=%v status=%d; an old-prefix entry must not authenticate", called, w.Code)
	}
}

func TestDaemonAuth_PATRestrictions(t *testing.T) {
	ro := auth.PATEntry{UserID: "user-1", ReadOnly: true}
	w, called := servePAT(t, buildDaemonAuth, ro, patRequest{"POST", "/api/daemon/heartbeat", nil})
	wantCoded(t, w, called, "token_read_only")
	w, called = servePAT(t, buildDaemonAuth, ro, patRequest{"GET", "/api/daemon/runtimes", nil})
	wantAllowed(t, w, called)

	bound := auth.PATEntry{UserID: "user-1", WorkspaceID: patTestWsA}
	w, called = servePAT(t, buildDaemonAuth, bound, patRequest{"POST", "/api/daemon/heartbeat", map[string]string{"X-Workspace-ID": patTestWsB}})
	wantCoded(t, w, called, "token_workspace_mismatch")
	w, called = servePAT(t, buildDaemonAuth, bound, patRequest{"POST", "/api/daemon/heartbeat", map[string]string{"X-Workspace-ID": patTestWsA}})
	wantAllowed(t, w, called)

	w, called = servePAT(t, buildDaemonAuth, auth.PATEntry{UserID: "user-1"}, patRequest{"POST", "/api/daemon/heartbeat", map[string]string{"X-Workspace-ID": patTestWsB}})
	wantAllowed(t, w, called)
}

// The workspace middleware resolves the workspace for routes that name it in
// the URL or in a slug, so it checks the binding again against the workspace it
// finally resolved.
func TestRequireWorkspaceMember_RefusesBoundPATOfOtherWorkspace(t *testing.T) {
	mw := RequireWorkspaceMember(nil)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not run")
	}))
	r := httptest.NewRequest("GET", "/api/issues", nil)
	r.Header.Set("X-User-ID", "11111111-1111-1111-1111-111111111111")
	r.Header.Set("X-Workspace-ID", patTestWsB)
	r = r.WithContext(WithPATEntry(r.Context(), auth.PATEntry{UserID: "u", WorkspaceID: patTestWsA}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	wantCoded(t, w, false, "token_workspace_mismatch")
}
