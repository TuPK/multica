package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/multica-ai/multica/server/internal/hub"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// hubGitHubCodeload serves repository zips the way codeload.github.com does, by
// /{owner}/{repo}/zip/{ref}. A ref that is not in routes answers 404, and the test
// can change a served zip between syncs. It records every request path in order.
type hubGitHubCodeload struct {
	srv    *httptest.Server
	mu     sync.Mutex
	routes map[string][]byte
	seen   []string
}

func newHubGitHubCodeload(t *testing.T) *hubGitHubCodeload {
	t.Helper()
	c := &hubGitHubCodeload{routes: map[string][]byte{}}
	c.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		c.seen = append(c.seen, r.URL.Path)
		body, ok := c.routes[r.URL.Path]
		c.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(c.srv.Close)
	prev := hub.GitHubCodeloadBase
	hub.GitHubCodeloadBase = c.srv.URL
	t.Cleanup(func() { hub.GitHubCodeloadBase = prev })
	return c
}

func (c *hubGitHubCodeload) set(path string, body []byte) {
	c.mu.Lock()
	c.routes[path] = body
	c.mu.Unlock()
}

func (c *hubGitHubCodeload) requests() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.seen...)
}

// hubGitHubRepoZip is a repository archive: every entry sits under one top folder,
// as GitHub's zips do, and the folder is stripped on extraction.
func hubGitHubRepoZip(t *testing.T, repo, body string) []byte {
	t.Helper()
	return hubImportZip(t, map[string]string{
		repo + "-master/SKILL.md":            "---\nname: " + repo + "\ndescription: github test\n---\n\n" + body + "\n",
		repo + "-master/references/notes.md": "Notes.\n",
	})
}

func TestHubGitHubPreviewFallsBackFromHEADToMaster(t *testing.T) {
	setupHubImportTest(t)
	f := hubSyncWorkspace(t)
	owner := f.users[hubRoleOwner]
	repo := "gh-preview-" + hubItemsSuffix()
	codeload := newHubGitHubCodeload(t)
	// HEAD and main are missing, so only master can be fetched.
	codeload.set("/acme/"+repo+"/zip/refs/heads/master", hubGitHubRepoZip(t, repo, "# preview"))

	var out hubImportPreviewBody
	testutil.Call(t, testHandler.PreviewHubImport,
		hubTestRequest(http.MethodPost, "/api/hub/import/preview", map[string]any{
			"source": map[string]any{"type": "github", "url": "https://github.com/acme/" + repo},
		}, f.ws, owner)).Want(http.StatusOK).JSON(&out)
	if out.Name != repo || out.Kind != "skill" {
		t.Fatalf("preview = %+v", out)
	}
	want := []string{"/acme/" + repo + "/zip/HEAD", "/acme/" + repo + "/zip/refs/heads/main", "/acme/" + repo + "/zip/refs/heads/master"}
	if got := codeload.requests(); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("codeload requests = %v, want %v", got, want)
	}
}

func TestHubGitHubPreviewWithExplicitRefDoesNotFallBack(t *testing.T) {
	setupHubImportTest(t)
	f := hubSyncWorkspace(t)
	owner := f.users[hubRoleOwner]
	repo := "gh-ref-" + hubItemsSuffix()
	codeload := newHubGitHubCodeload(t)
	codeload.set("/acme/"+repo+"/zip/v1.2.0", hubGitHubRepoZip(t, repo, "# tagged"))

	testutil.Call(t, testHandler.PreviewHubImport,
		hubTestRequest(http.MethodPost, "/api/hub/import/preview", map[string]any{
			"source": map[string]any{"type": "github", "url": "https://github.com/acme/" + repo, "ref": "v1.2.0"},
		}, f.ws, owner)).Want(http.StatusOK)
	if got := codeload.requests(); len(got) != 1 || got[0] != "/acme/"+repo+"/zip/v1.2.0" {
		t.Fatalf("codeload requests = %v, want only the explicit ref", got)
	}
}

func TestHubGitHubPreviewMissingRepositoryIs404Upstream(t *testing.T) {
	setupHubImportTest(t)
	f := hubSyncWorkspace(t)
	owner := f.users[hubRoleOwner]
	newHubGitHubCodeload(t)

	testutil.Call(t, testHandler.PreviewHubImport,
		hubTestRequest(http.MethodPost, "/api/hub/import/preview", map[string]any{
			"source": map[string]any{"type": "github", "url": "https://github.com/acme/no-such-" + hubItemsSuffix()},
		}, f.ws, owner)).Want(http.StatusBadGateway)
}

func TestHubGitHubCommitAndSyncDetectsChangedTree(t *testing.T) {
	setupHubImportTest(t)
	f := hubSyncWorkspace(t)
	owner := f.users[hubRoleOwner]
	repo := "gh-sync-" + hubItemsSuffix()
	codeload := newHubGitHubCodeload(t)
	masterPath := "/acme/" + repo + "/zip/refs/heads/master"
	codeload.set(masterPath, hubGitHubRepoZip(t, repo, "# v1"))
	sourceURL := "https://github.com/acme/" + repo

	var preview hubImportPreviewBody
	testutil.Call(t, testHandler.PreviewHubImport,
		hubTestRequest(http.MethodPost, "/api/hub/import/preview", map[string]any{
			"source": map[string]any{"type": "github", "url": sourceURL},
		}, f.ws, owner)).Want(http.StatusOK).JSON(&preview)
	rec := commitHubImportForTest(t, f.ws, owner, map[string]any{"preview_id": preview.PreviewID})
	committed := decodeHubImportCommit(t, rec)
	if rec.Code != http.StatusOK || committed.HubItemID == "" {
		t.Fatalf("github commit = %d %+v", rec.Code, committed)
	}
	var sourceRef string
	dbfx.QueryRow(t, `SELECT source_ref FROM hub_item WHERE id = $1`, committed.HubItemID).Scan(&sourceRef)
	if sourceRef != "refs/heads/master" {
		t.Fatalf("hub item source_ref = %q, want refs/heads/master", sourceRef)
	}
	var sourceID string
	dbfx.QueryRow(t, `SELECT id::text FROM hub_source WHERE workspace_id = $1 AND type = 'github' AND url = $2`, f.ws, sourceURL).Scan(&sourceID)

	var unchanged HubSourceSyncResponse
	syncHubSourceForTest(t, f.ws, owner, sourceID).Want(http.StatusOK).JSON(&unchanged)
	if unchanged.Status != "ok" || unchanged.Unchanged != 1 || unchanged.UpdatesAvailable != 0 {
		t.Fatalf("sync of an unchanged tree = %+v", unchanged)
	}

	// The repository moves on. The sync fetches the same ref it recorded, not the fallback chain.
	codeload.set(masterPath, hubGitHubRepoZip(t, repo, "# v2"))
	var changed HubSourceSyncResponse
	syncHubSourceForTest(t, f.ws, owner, sourceID).Want(http.StatusOK).JSON(&changed)
	if changed.UpdatesAvailable != 1 || len(changed.Items) != 1 || changed.Items[0].Change != hubChangeUpdate {
		t.Fatalf("sync of a changed tree = %+v", changed)
	}
	if _, update, _, _ := hubItemStateForTest(t, committed.HubItemID); !update {
		t.Fatal("update_available not set after the tree changed")
	}
}
