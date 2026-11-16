package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/auth/authtest"
	"github.com/multica-ai/multica/server/internal/middleware"
)

type createdPAT struct {
	ID          string  `json:"id"`
	Token       string  `json:"token"`
	WorkspaceID *string `json:"workspace_id"`
	ReadOnly    bool    `json:"read_only"`
}

// createPAT calls the create route as the shared test user and returns the
// status and decoded body. The row is removed at test end.
func createPAT(t *testing.T, body map[string]any) (int, createdPAT, string) {
	t.Helper()
	w := httptest.NewRecorder()
	testHandler.CreatePersonalAccessToken(w, newRequest("POST", "/api/tokens", body))
	var out createdPAT
	if w.Code == http.StatusCreated {
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode create response: %v (%s)", err, w.Body.String())
		}
		t.Cleanup(func() {
			testPool.Exec(context.Background(), `DELETE FROM personal_access_token WHERE id = $1`, parseUUID(out.ID))
		})
	}
	return w.Code, out, w.Body.String()
}

func TestPATRestriction_CreateStoresBothOptions(t *testing.T) {
	code, got, raw := createPAT(t, map[string]any{"name": "bound", "workspace_id": testWorkspaceID, "read_only": true})
	if code != http.StatusCreated {
		t.Fatalf("status = %d: %s", code, raw)
	}
	if got.WorkspaceID == nil || *got.WorkspaceID != testWorkspaceID || !got.ReadOnly {
		t.Fatalf("response = %+v, want workspace %s and read_only", got, testWorkspaceID)
	}
	var ws *string
	var ro bool
	if err := testPool.QueryRow(context.Background(),
		`SELECT workspace_id::text, read_only FROM personal_access_token WHERE id = $1`, parseUUID(got.ID)).Scan(&ws, &ro); err != nil {
		t.Fatalf("read row: %v", err)
	}
	if ws == nil || *ws != testWorkspaceID || !ro {
		t.Fatalf("stored workspace_id=%v read_only=%v", ws, ro)
	}
}

// A create call that names no option makes the unrestricted token existing
// clients expect, and the response shape only gains fields.
func TestPATRestriction_CreateDefaultsAreUnrestricted(t *testing.T) {
	code, got, raw := createPAT(t, map[string]any{"name": "plain"})
	if code != http.StatusCreated {
		t.Fatalf("status = %d: %s", code, raw)
	}
	if got.WorkspaceID != nil || got.ReadOnly || got.Token == "" {
		t.Fatalf("response = %+v, want no restriction and a token", got)
	}
	var generic map[string]any
	_ = json.Unmarshal([]byte(raw), &generic)
	for _, k := range []string{"id", "name", "token_prefix", "expires_at", "last_used_at", "created_at", "token", "workspace_id", "read_only"} {
		if _, ok := generic[k]; !ok {
			t.Errorf("response is missing %q: %s", k, raw)
		}
	}
}

func TestPATRestriction_CreateReadOnlyWithoutWorkspace(t *testing.T) {
	code, got, raw := createPAT(t, map[string]any{"name": "ro", "read_only": true})
	if code != http.StatusCreated || got.WorkspaceID != nil || !got.ReadOnly {
		t.Fatalf("status=%d response=%+v (%s)", code, got, raw)
	}
}

func TestPATRestriction_CreateRefusesInvalidInput(t *testing.T) {
	foreign := dbfx.Workspace(t, "PAT foreign", "pat-foreign-"+uuid.NewString()[:8])
	cases := []struct {
		name string
		body map[string]any
		want int
	}{
		{"workspace_id not a uuid", map[string]any{"name": "x", "workspace_id": "not-a-uuid"}, http.StatusBadRequest},
		{"workspace_id empty string", map[string]any{"name": "x", "workspace_id": ""}, http.StatusBadRequest},
		{"read_only not a bool", map[string]any{"name": "x", "read_only": "yes"}, http.StatusBadRequest},
		{"workspace the caller is not a member of", map[string]any{"name": "x", "workspace_id": foreign}, http.StatusNotFound},
		{"workspace that does not exist", map[string]any{"name": "x", "workspace_id": uuid.NewString()}, http.StatusNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var before int
			testPool.QueryRow(context.Background(), `SELECT count(*) FROM personal_access_token WHERE user_id = $1`, parseUUID(testUserID)).Scan(&before)
			code, _, raw := createPAT(t, c.body)
			if code != c.want {
				t.Fatalf("status = %d, want %d: %s", code, c.want, raw)
			}
			var after int
			testPool.QueryRow(context.Background(), `SELECT count(*) FROM personal_access_token WHERE user_id = $1`, parseUUID(testUserID)).Scan(&after)
			if after != before {
				t.Fatalf("a refused create left a token behind (%d -> %d)", before, after)
			}
		})
	}
}

func TestPATRestriction_ListShowsRestrictions(t *testing.T) {
	_, got, _ := createPAT(t, map[string]any{"name": "listed", "workspace_id": testWorkspaceID, "read_only": true})
	w := httptest.NewRecorder()
	testHandler.ListPersonalAccessTokens(w, newRequest("GET", "/api/tokens", nil))
	var list []createdPAT
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	for _, p := range list {
		if p.ID == got.ID {
			if p.WorkspaceID == nil || *p.WorkspaceID != testWorkspaceID || !p.ReadOnly {
				t.Fatalf("listed token = %+v", p)
			}
			return
		}
	}
	t.Fatal("created token is not in the list")
}

// patChain is the production order for a workspace route: Auth, then the
// workspace middleware. It uses the real queries and a fake cache.
func patChain(cache *auth.PATCache) http.Handler {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	return middleware.Auth(testHandler.Queries, cache, nil, nil, nil)(
		middleware.RequireWorkspaceMember(testHandler.Queries)(ok))
}

func patRequestFor(method, raw string, headers map[string]string) *http.Request {
	r := httptest.NewRequest(method, "/api/issues", nil)
	r.Header.Set("Authorization", "Bearer "+raw)
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

type patRefusal struct {
	Code string `json:"code"`
}

func wantPATRefusal(t *testing.T, w *httptest.ResponseRecorder, code string) {
	t.Helper()
	var got patRefusal
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if w.Code != http.StatusForbidden || got.Code != code {
		t.Fatalf("status=%d body=%s; want 403 %s", w.Code, w.Body.String(), code)
	}
}

// otherWorkspace creates a second workspace the test user belongs to.
func otherWorkspace(t *testing.T) (id, slug string) {
	t.Helper()
	slug = "pat-other-" + uuid.NewString()[:8]
	id = dbfx.Workspace(t, "PAT other", slug)
	dbfx.Member(t, id, testUserID, "owner")
	return id, slug
}

// AC18: a PAT bound to workspace A works in A and is refused in B, by id and by
// slug, through the database path and again on a cache hit.
func TestPATRestriction_BoundTokenWorksInItsWorkspaceAndIsRefusedInAnother(t *testing.T) {
	other, otherSlug := otherWorkspace(t)
	_, pat, _ := createPAT(t, map[string]any{"name": "ac18", "workspace_id": testWorkspaceID})
	cache := auth.NewPATCache(authtest.NewFakeRedis())
	chain := patChain(cache)

	for pass := 1; pass <= 2; pass++ { // pass 2 is served from the cache
		w := httptest.NewRecorder()
		chain.ServeHTTP(w, patRequestFor("GET", pat.Token, map[string]string{"X-Workspace-ID": testWorkspaceID}))
		if w.Code != http.StatusOK {
			t.Fatalf("pass %d: own workspace: status=%d %s", pass, w.Code, w.Body.String())
		}
		w = httptest.NewRecorder()
		chain.ServeHTTP(w, patRequestFor("GET", pat.Token, map[string]string{"X-Workspace-ID": other}))
		wantPATRefusal(t, w, "token_workspace_mismatch")
		w = httptest.NewRecorder()
		chain.ServeHTTP(w, patRequestFor("GET", pat.Token, map[string]string{"X-Workspace-Slug": otherSlug}))
		wantPATRefusal(t, w, "token_workspace_mismatch")
	}
	if _, ok := cache.GetEntry(context.Background(), auth.HashToken(pat.Token)); !ok {
		t.Fatal("the restriction was not cached")
	}
}

func TestPATRestriction_ReadOnlyTokenReadsButDoesNotWrite(t *testing.T) {
	_, pat, _ := createPAT(t, map[string]any{"name": "ro", "read_only": true})
	cache := auth.NewPATCache(authtest.NewFakeRedis())
	chain := patChain(cache)
	hdr := map[string]string{"X-Workspace-ID": testWorkspaceID}

	for pass := 1; pass <= 2; pass++ { // pass 2 is served from the cache
		w := httptest.NewRecorder()
		chain.ServeHTTP(w, patRequestFor("GET", pat.Token, hdr))
		if w.Code != http.StatusOK {
			t.Fatalf("pass %d: GET status=%d %s", pass, w.Code, w.Body.String())
		}
		for _, m := range []string{"POST", "PUT", "PATCH", "DELETE"} {
			w = httptest.NewRecorder()
			chain.ServeHTTP(w, patRequestFor(m, pat.Token, hdr))
			wantPATRefusal(t, w, "token_read_only")
		}
	}
}

// A token created before the migration has the default column values and keeps
// full access in every workspace of its owner.
func TestPATRestriction_TokenWithoutRestrictionsKeepsFullAccess(t *testing.T) {
	other, _ := otherWorkspace(t)
	raw, patID := insertTestPAT(t, time.Time{}) // CreatePersonalAccessToken params without the new fields
	var ws *string
	var ro bool
	testPool.QueryRow(context.Background(), `SELECT workspace_id::text, read_only FROM personal_access_token WHERE id = $1`, parseUUID(patID)).Scan(&ws, &ro)
	if ws != nil || ro {
		t.Fatalf("defaults are workspace_id=%v read_only=%v, want NULL and false", ws, ro)
	}
	chain := patChain(auth.NewPATCache(authtest.NewFakeRedis()))
	for _, ws := range []string{testWorkspaceID, other} {
		for _, m := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
			w := httptest.NewRecorder()
			chain.ServeHTTP(w, patRequestFor(m, raw, map[string]string{"X-Workspace-ID": ws}))
			if w.Code != http.StatusOK {
				t.Fatalf("%s in %s: status=%d %s", m, ws, w.Code, w.Body.String())
			}
		}
	}
}

// An entry that another process wrote under the old prefix must not stand in
// for the row: the restriction of a bound token still applies.
func TestPATRestriction_OldPrefixCacheEntryDoesNotBypassRestriction(t *testing.T) {
	other, _ := otherWorkspace(t)
	_, pat, _ := createPAT(t, map[string]any{"name": "old-prefix", "workspace_id": testWorkspaceID, "read_only": true})
	fake := authtest.NewFakeRedis()
	fake.Put("mul:auth:pat:"+auth.HashToken(pat.Token), testUserID)
	chain := patChain(auth.NewPATCache(fake))

	w := httptest.NewRecorder()
	chain.ServeHTTP(w, patRequestFor("GET", pat.Token, map[string]string{"X-Workspace-ID": other}))
	wantPATRefusal(t, w, "token_workspace_mismatch")
	w = httptest.NewRecorder()
	chain.ServeHTTP(w, patRequestFor("POST", pat.Token, map[string]string{"X-Workspace-ID": testWorkspaceID}))
	wantPATRefusal(t, w, "token_read_only")
}
