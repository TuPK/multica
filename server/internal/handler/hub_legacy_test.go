package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/multica-ai/multica/server/internal/hub"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// hubLegacyClawHub serves a one-file ClawHub skill under clawHubAPIBase and counts
// the requests it receives. The fetchers reach it only through the Hub's safe client.
func hubLegacyClawHub(t *testing.T, slug string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	skillMD := "---\nname: " + slug + "\ndescription: legacy fetch test\n---\n\n# " + slug + "\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.URL.Path {
		case "/api/v1/skills/" + slug:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"skill": map[string]any{
					"slug":        slug,
					"displayName": slug,
					"summary":     "legacy fetch test",
					"tags":        map[string]string{"latest": "1.0.0"},
				},
			})
		case "/api/v1/skills/" + slug + "/versions/1.0.0":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"version": map[string]any{
					"version": "1.0.0",
					"files":   []map[string]any{{"path": "SKILL.md", "size": len(skillMD)}},
				},
			})
		case "/api/v1/skills/" + slug + "/file":
			_, _ = w.Write([]byte(skillMD))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	prev := clawHubAPIBase
	clawHubAPIBase = srv.URL + "/api/v1"
	t.Cleanup(func() { clawHubAPIBase = prev })
	return srv, &hits
}

// The ClawHub fetcher's requests go through the SafeFetcher: under production
// options a loopback ClawHub is refused before any request reaches it, and under
// the test options the same preview succeeds.
func TestHubLegacyClawHubFetchUsesSafeClient(t *testing.T) {
	setupHubImportTest(t)
	f := hubSyncWorkspace(t)
	owner := f.users[hubRoleOwner]
	slug := "hub-legacy-" + hubItemsSuffix()
	_, hits := hubLegacyClawHub(t, slug)
	sourceURL := "https://clawhub.ai/acme/" + slug

	t.Run("production options refuse the loopback origin", func(t *testing.T) {
		hubImportFetcher = hub.NewSafeFetcher(hub.FetcherOptions{})
		testutil.Call(t, testHandler.PreviewHubImport,
			hubTestRequest(http.MethodPost, "/api/hub/import/preview", map[string]any{
				"source": map[string]any{"type": "clawhub", "url": sourceURL},
			}, f.ws, owner)).Want(http.StatusBadRequest)
		if n := hits.Load(); n != 0 {
			t.Fatalf("the blocked origin received %d requests", n)
		}
	})

	t.Run("test options fetch the same skill", func(t *testing.T) {
		hubImportFetcher = hub.NewSafeFetcher(hub.FetcherOptions{AllowPrivate: true})
		var out hubImportPreviewBody
		testutil.Call(t, testHandler.PreviewHubImport,
			hubTestRequest(http.MethodPost, "/api/hub/import/preview", map[string]any{
				"source": map[string]any{"type": "clawhub", "url": sourceURL},
			}, f.ws, owner)).Want(http.StatusOK).JSON(&out)
		if out.Name != slug || out.Kind != "skill" {
			t.Fatalf("preview = %+v", out)
		}
	})
}
