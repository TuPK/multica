package handler

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/hub"
	"github.com/multica-ai/multica/server/internal/storage"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// Each test names its rows after itself. Skills and plugins created through the
// commit path are removed by id or key; the staging objects live in a temp dir.

// setupHubImportTest points the handler at a temp-dir local store and lets the
// preview fetch from an httptest server on loopback. Both are restored afterward.
func setupHubImportTest(t *testing.T) {
	t.Helper()
	t.Setenv("LOCAL_UPLOAD_DIR", t.TempDir())
	store := storage.NewLocalStorageFromEnv()
	if store == nil {
		t.Fatal("local storage did not initialize")
	}
	prevStorage := testHandler.HubStorage
	testHandler.HubStorage = store
	prevFetcher := hubImportFetcher
	hubImportFetcher = hub.NewSafeFetcher(hub.FetcherOptions{AllowPrivate: true})
	prevTTL := hubPreviewTTL
	t.Cleanup(func() {
		testHandler.HubStorage = prevStorage
		hubImportFetcher = prevFetcher
		hubPreviewTTL = prevTTL
	})
}

func hubImportZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		entry, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func hubImportSkillFiles(name string, extra map[string]string) map[string]string {
	files := map[string]string{
		"SKILL.md":            "---\nname: " + name + "\ndescription: A hub import test skill\n---\n\n# " + name + "\n",
		"references/notes.md": "Supporting notes.\n",
	}
	for k, v := range extra {
		files[k] = v
	}
	return files
}

// hubImportPluginKey is the manifest key of the plugin fixture. The test that
// uses it removes its packages by this key.
const hubImportPluginKey = "com.example.hubimport"

func hubImportPluginZip(t *testing.T, version string) []byte {
	t.Helper()
	manifest := strings.Replace(packageManifest(version), "com.example.published", hubImportPluginKey, 1)
	return pluginBundleZip(t, manifest, "console.log('hub import');\n")
}

// hubImportServe serves the given bodies by path and returns the server.
func hubImportServe(t *testing.T, routes map[string][]byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := routes[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func hubImportPreviewRequest(t *testing.T, sourceType, rawURL string) *http.Request {
	t.Helper()
	return hubTestRequest(http.MethodPost, "/api/hub/import/preview", map[string]any{
		"source": map[string]any{"type": sourceType, "url": rawURL},
	}, testWorkspaceID, testUserID)
}

type hubImportPreviewBody struct {
	PreviewID string `json:"preview_id"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	Files     []struct {
		Path      string `json:"path"`
		SizeBytes int64  `json:"size_bytes"`
	} `json:"files"`
	Scan struct {
		Status   string `json:"status"`
		Findings []struct {
			Rule     string `json:"rule"`
			Severity string `json:"severity"`
		} `json:"findings"`
	} `json:"scan"`
	Conflict struct {
		Exists bool   `json:"exists"`
		ItemID string `json:"item_id"`
		Kind   string `json:"kind"`
	} `json:"conflict"`
	Registry *struct {
		Name  string `json:"name"`
		Items []struct {
			Kind string   `json:"kind"`
			Name string   `json:"name"`
			Tags []string `json:"tags"`
		} `json:"items"`
	} `json:"registry"`
}

func previewHubImportForTest(t *testing.T, sourceType, rawURL string) hubImportPreviewBody {
	t.Helper()
	var out hubImportPreviewBody
	testutil.Call(t, testHandler.PreviewHubImport, hubImportPreviewRequest(t, sourceType, rawURL)).
		Want(http.StatusOK).JSON(&out)
	return out
}

func commitHubImportForTest(t *testing.T, workspaceID, userID string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	return testutil.Call(t, testHandler.CommitHubImport,
		hubTestRequest(http.MethodPost, "/api/hub/import/commit", body, workspaceID, userID)).ResponseRecorder
}

type hubImportCommitBody struct {
	Kind      string `json:"kind"`
	ID        string `json:"id"`
	HubItemID string `json:"hub_item_id"`
	Skipped   bool   `json:"skipped"`
	Error     string `json:"error"`
	Code      string `json:"code"`
}

func decodeHubImportCommit(t *testing.T, rec *httptest.ResponseRecorder) hubImportCommitBody {
	t.Helper()
	var out hubImportCommitBody
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode commit body %q: %v", rec.Body.String(), err)
	}
	return out
}

// hubImportSkillName keeps skill names unique in the shared database.
func hubImportSkillName(t *testing.T, suffix string) string {
	return hubTestLabel(t, suffix)
}

func TestHubImportPreviewSkillFromURLZip(t *testing.T) {
	setupHubImportTest(t)
	name := hubImportSkillName(t, "skill")
	zipBytes := hubImportZip(t, hubImportSkillFiles(name, nil))
	srv := hubImportServe(t, map[string][]byte{"/skill.zip": zipBytes})

	out := previewHubImportForTest(t, "url_zip", srv.URL+"/skill.zip")
	if out.PreviewID == "" || out.Kind != "skill" || out.Name != name {
		t.Fatalf("preview = %+v", out)
	}
	if out.Scan.Status != hub.ScanClean || out.Conflict.Exists {
		t.Fatalf("scan/conflict = %+v / %+v", out.Scan, out.Conflict)
	}
	if len(out.Files) != 2 {
		t.Fatalf("files = %+v, want SKILL.md and references/notes.md", out.Files)
	}
}

func TestHubImportPreviewPluginFromURLZip(t *testing.T) {
	setupHubImportTest(t)
	zipBytes := hubImportPluginZip(t, "1.0.0")
	srv := hubImportServe(t, map[string][]byte{"/plugin.zip": zipBytes})

	out := previewHubImportForTest(t, "url_zip", srv.URL+"/plugin.zip")
	if out.Kind != "plugin" || out.Name != hubImportPluginKey || out.Version != "1.0.0" {
		t.Fatalf("preview = %+v", out)
	}
}

func TestHubImportPreviewUploadMultipart(t *testing.T) {
	setupHubImportTest(t)
	name := hubImportSkillName(t, "upload")
	zipBytes := hubImportZip(t, hubImportSkillFiles(name, nil))

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", "skill.zip")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	_, _ = part.Write(zipBytes)
	_ = form.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/hub/import/preview", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	req.Header.Set("X-Workspace-ID", testWorkspaceID)
	req.Header.Set("X-User-ID", testUserID)

	var out hubImportPreviewBody
	testutil.Call(t, testHandler.PreviewHubImport, req).Want(http.StatusOK).JSON(&out)
	if out.PreviewID == "" || out.Kind != "skill" || out.Name != name {
		t.Fatalf("upload preview = %+v", out)
	}
}

func TestHubImportPreviewRegistryListsItems(t *testing.T) {
	setupHubImportTest(t)
	index := map[string]any{
		"schema": 1,
		"name":   "Hub import test registry",
		"items": []map[string]any{
			{
				"kind": "skill", "name": "registry-skill", "description": "one",
				"version": "1.0.0", "url": "https://example.com/a.zip",
				"sha256": strings.Repeat("a", 64), "tags": []string{"docs"},
			},
			{"kind": "bogus", "name": "dropped", "url": "https://example.com/b.zip", "sha256": strings.Repeat("b", 64)},
		},
	}
	indexBytes, _ := json.Marshal(index)
	srv := hubImportServe(t, map[string][]byte{"/hub-index.json": indexBytes})

	out := previewHubImportForTest(t, "registry", srv.URL)
	if out.Kind != "registry" || out.PreviewID != "" || out.Registry == nil {
		t.Fatalf("registry preview = %+v", out)
	}
	if out.Registry.Name != "Hub import test registry" || len(out.Registry.Items) != 1 {
		t.Fatalf("registry = %+v", out.Registry)
	}
	if got := out.Registry.Items[0]; got.Name != "registry-skill" || got.Tags == nil {
		t.Fatalf("registry item = %+v", got)
	}
}

// Under the production fetcher, a loopback URL is refused at dial time.
func TestHubImportPreviewRefusesLoopbackUnderProductionOptions(t *testing.T) {
	setupHubImportTest(t)
	hubImportFetcher = hub.NewSafeFetcher(hub.FetcherOptions{})
	srv := hubImportServe(t, map[string][]byte{"/skill.zip": hubImportZip(t, hubImportSkillFiles("loopback", nil))})

	rec := testutil.Call(t, testHandler.PreviewHubImport, hubImportPreviewRequest(t, "url_zip", srv.URL+"/skill.zip")).
		Want(http.StatusBadRequest)
	if !strings.Contains(rec.Body.String(), "address is not allowed") {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "127.0.0.1") {
		t.Fatalf("the refusal leaked the resolved address: %s", rec.Body.String())
	}
}

func TestHubImportPreviewRejectsUnsupportedType(t *testing.T) {
	setupHubImportTest(t)
	testutil.Call(t, testHandler.PreviewHubImport, hubImportPreviewRequest(t, "upload", "")).Want(http.StatusBadRequest)
}

func TestHubImportCommitBlockedScanIs409(t *testing.T) {
	setupHubImportTest(t)
	name := hubImportSkillName(t, "blocked")
	elf := "\x7fELF" + strings.Repeat("\x00", 64)
	zipBytes := hubImportZip(t, hubImportSkillFiles(name, map[string]string{"bin/tool": elf}))
	srv := hubImportServe(t, map[string][]byte{"/blocked.zip": zipBytes})

	out := previewHubImportForTest(t, "url_zip", srv.URL+"/blocked.zip")
	if out.Scan.Status != hub.ScanBlocked {
		t.Fatalf("scan status = %q, want blocked", out.Scan.Status)
	}
	rec := commitHubImportForTest(t, testWorkspaceID, testUserID, map[string]any{
		"preview_id": out.PreviewID, "confirm_warnings": true,
	})
	body := decodeHubImportCommit(t, rec)
	if rec.Code != http.StatusConflict || body.Code != "scan_blocked" {
		t.Fatalf("commit = %d %+v, want 409 scan_blocked", rec.Code, body)
	}
	if n := dbfx.Count(t, "SELECT count(*) FROM skill WHERE workspace_id = $1 AND name = $2", testWorkspaceID, name); n != 0 {
		t.Fatalf("a blocked bundle created %d skills", n)
	}
}

func TestHubImportCommitWarningsNeedConfirm(t *testing.T) {
	setupHubImportTest(t)
	name := hubImportSkillName(t, "warned")
	notes := "api_key = \"abcdefghijklmnopqrstuvwx\"\n"
	zipBytes := hubImportZip(t, hubImportSkillFiles(name, map[string]string{"notes/keys.md": notes}))
	srv := hubImportServe(t, map[string][]byte{"/warned.zip": zipBytes})

	out := previewHubImportForTest(t, "url_zip", srv.URL+"/warned.zip")
	if out.Scan.Status != hub.ScanWarn {
		t.Fatalf("scan status = %q, want warn", out.Scan.Status)
	}
	rec := commitHubImportForTest(t, testWorkspaceID, testUserID, map[string]any{"preview_id": out.PreviewID})
	if body := decodeHubImportCommit(t, rec); rec.Code != http.StatusConflict || body.Code != "warnings_unconfirmed" {
		t.Fatalf("unconfirmed commit = %d %+v", rec.Code, body)
	}

	rec = commitHubImportForTest(t, testWorkspaceID, testUserID, map[string]any{
		"preview_id": out.PreviewID, "confirm_warnings": true,
	})
	body := decodeHubImportCommit(t, rec)
	if rec.Code != http.StatusOK || body.Kind != "skill" || body.ID == "" {
		t.Fatalf("confirmed commit = %d %+v", rec.Code, body)
	}
	dbfx.Cleanup(t, "DELETE FROM hub_item WHERE ref_id = $1", body.ID)
	dbfx.Cleanup(t, "DELETE FROM skill WHERE id = $1", body.ID)
}

// Conflicts: skip writes nothing, rename uses the given name, replace overwrites
// the creator's skill in place.
func TestHubImportCommitConflictStrategies(t *testing.T) {
	setupHubImportTest(t)
	name := hubImportSkillName(t, "conflict")
	existingID := dbfx.Insert(t, "skill", testutil.Cols{
		"workspace_id": testWorkspaceID,
		"name":         name,
		"created_by":   testUserID,
	})
	zipBytes := hubImportZip(t, hubImportSkillFiles(name, nil))
	srv := hubImportServe(t, map[string][]byte{"/conflict.zip": zipBytes})

	out := previewHubImportForTest(t, "url_zip", srv.URL+"/conflict.zip")
	if !out.Conflict.Exists || out.Conflict.ItemID != existingID || out.Conflict.Kind != "skill" {
		t.Fatalf("conflict = %+v, want existing %s", out.Conflict, existingID)
	}

	// Without a strategy, a conflict is refused.
	rec := commitHubImportForTest(t, testWorkspaceID, testUserID, map[string]any{"preview_id": out.PreviewID})
	if body := decodeHubImportCommit(t, rec); rec.Code != http.StatusConflict || body.Code != "name_conflict" {
		t.Fatalf("no-strategy commit = %d %+v", rec.Code, body)
	}

	// Skip: no write, no hub item.
	rec = commitHubImportForTest(t, testWorkspaceID, testUserID, map[string]any{
		"preview_id": out.PreviewID, "on_conflict": "skip",
	})
	body := decodeHubImportCommit(t, rec)
	if rec.Code != http.StatusOK || !body.Skipped || body.ID != existingID {
		t.Fatalf("skip = %d %+v", rec.Code, body)
	}
	if n := dbfx.Count(t, "SELECT count(*) FROM hub_item WHERE ref_id = $1", existingID); n != 0 {
		t.Fatalf("skip wrote %d hub items", n)
	}

	// Rename without a name is refused.
	rec = commitHubImportForTest(t, testWorkspaceID, testUserID, map[string]any{
		"preview_id": out.PreviewID, "on_conflict": "rename",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("rename without name = %d, want 400", rec.Code)
	}

	// Rename with a fresh name creates a second skill.
	renamed := name + " renamed"
	rec = commitHubImportForTest(t, testWorkspaceID, testUserID, map[string]any{
		"preview_id": out.PreviewID, "on_conflict": "rename", "name": renamed,
	})
	body = decodeHubImportCommit(t, rec)
	if rec.Code != http.StatusOK || body.Skipped || body.ID == "" || body.ID == existingID {
		t.Fatalf("rename = %d %+v", rec.Code, body)
	}
	dbfx.Cleanup(t, "DELETE FROM hub_item WHERE ref_id = $1", body.ID)
	dbfx.Cleanup(t, "DELETE FROM skill WHERE id = $1", body.ID)

	// Replace overwrites the existing skill and keeps its id. A successful commit
	// consumes its preview, so replace uses a fresh one.
	replacePreview := previewHubImportForTest(t, "url_zip", srv.URL+"/conflict.zip")
	rec = commitHubImportForTest(t, testWorkspaceID, testUserID, map[string]any{
		"preview_id": replacePreview.PreviewID, "on_conflict": "replace",
	})
	body = decodeHubImportCommit(t, rec)
	if rec.Code != http.StatusOK || body.ID != existingID || body.HubItemID == "" {
		t.Fatalf("replace = %d %+v", rec.Code, body)
	}
	dbfx.Cleanup(t, "DELETE FROM hub_item WHERE ref_id = $1", existingID)
	dbfx.Cleanup(t, "DELETE FROM skill WHERE id = $1", existingID)
}

func TestHubImportCommitExpiredPreviewIs410(t *testing.T) {
	setupHubImportTest(t)
	name := hubImportSkillName(t, "expired")
	srv := hubImportServe(t, map[string][]byte{"/expired.zip": hubImportZip(t, hubImportSkillFiles(name, nil))})

	hubPreviewTTL = -time.Minute
	out := previewHubImportForTest(t, "url_zip", srv.URL+"/expired.zip")
	hubPreviewTTL = 30 * time.Minute

	rec := commitHubImportForTest(t, testWorkspaceID, testUserID, map[string]any{"preview_id": out.PreviewID})
	body := decodeHubImportCommit(t, rec)
	if rec.Code != http.StatusGone || body.Error != "preview expired" {
		t.Fatalf("expired commit = %d %+v", rec.Code, body)
	}
}

// A preview belongs to its workspace and user; another workspace sees not found.
func TestHubImportCommitCrossWorkspacePreviewIs404(t *testing.T) {
	setupHubImportTest(t)
	name := hubImportSkillName(t, "crossws")
	srv := hubImportServe(t, map[string][]byte{"/cross.zip": hubImportZip(t, hubImportSkillFiles(name, nil))})
	out := previewHubImportForTest(t, "url_zip", srv.URL+"/cross.zip")

	slug := fmt.Sprintf("hub-import-%d", time.Now().UnixNano())
	otherWS := dbfx.Workspace(t, hubImportSkillName(t, "other workspace"), slug)
	dbfx.Member(t, otherWS, testUserID, "member")

	rec := commitHubImportForTest(t, otherWS, testUserID, map[string]any{"preview_id": out.PreviewID})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-workspace commit = %d %s, want 404", rec.Code, rec.Body.String())
	}
}

func TestHubImportCommitSkillCreatesHubRows(t *testing.T) {
	setupHubImportTest(t)
	name := hubImportSkillName(t, "rows")
	srvURL := hubImportServe(t, map[string][]byte{"/rows.zip": hubImportZip(t, hubImportSkillFiles(name, nil))}).URL + "/rows.zip"
	out := previewHubImportForTest(t, "url_zip", srvURL)

	rec := commitHubImportForTest(t, testWorkspaceID, testUserID, map[string]any{"preview_id": out.PreviewID})
	body := decodeHubImportCommit(t, rec)
	if rec.Code != http.StatusOK || body.Kind != "skill" || body.HubItemID == "" || body.Skipped {
		t.Fatalf("commit = %d %+v", rec.Code, body)
	}
	dbfx.Cleanup(t, "DELETE FROM hub_item WHERE ref_id = $1", body.ID)
	dbfx.Cleanup(t, "DELETE FROM skill WHERE id = $1", body.ID)

	var tracking, bundleKey, sourceType, sourceURL string
	dbfx.QueryRow(t, `SELECT hi.tracking, hi.bundle_key, hs.type, hs.url
		FROM hub_item hi JOIN hub_source hs ON hs.id = hi.source_id
		WHERE hi.id = $1`, body.HubItemID).Scan(&tracking, &bundleKey, &sourceType, &sourceURL)
	if tracking != "tracking" || sourceType != "url_zip" || sourceURL != srvURL {
		t.Fatalf("hub item tracking=%q source=%s %s", tracking, sourceType, sourceURL)
	}
	if !strings.Contains(bundleKey, "/skill/"+body.ID+"/") {
		t.Fatalf("bundle key %q does not name the skill", bundleKey)
	}
	dbfx.Cleanup(t, "DELETE FROM hub_source WHERE workspace_id = $1 AND type = 'url_zip' AND url = $2", testWorkspaceID, srvURL)
	if _, err := testHandler.HubStorage.GetReader(context.Background(), bundleKey); err != nil {
		t.Fatalf("bundle %q was not stored: %v", bundleKey, err)
	}
}

func TestHubImportCommitPluginPublishesAndReplayConflicts(t *testing.T) {
	setupHubImportTest(t)
	withPluginsV1Flag(t, testHandler, true)
	withHostCapabilities(t)
	dbfx.Cleanup(t, "DELETE FROM plugin_package_file WHERE version_id IN (SELECT id FROM plugin_package_version WHERE workspace_id = $1 AND package_id IN (SELECT id FROM plugin_package WHERE workspace_id = $1 AND plugin_key = $2))", testWorkspaceID, hubImportPluginKey)
	dbfx.Cleanup(t, "DELETE FROM plugin_package_version WHERE workspace_id = $1 AND package_id IN (SELECT id FROM plugin_package WHERE workspace_id = $1 AND plugin_key = $2)", testWorkspaceID, hubImportPluginKey)
	dbfx.Cleanup(t, "DELETE FROM plugin_package WHERE workspace_id = $1 AND plugin_key = $2", testWorkspaceID, hubImportPluginKey)

	srv := hubImportServe(t, map[string][]byte{"/plugin.zip": hubImportPluginZip(t, "1.0.0")})
	out := previewHubImportForTest(t, "url_zip", srv.URL+"/plugin.zip")

	rec := commitHubImportForTest(t, testWorkspaceID, testUserID, map[string]any{"preview_id": out.PreviewID})
	body := decodeHubImportCommit(t, rec)
	if rec.Code != http.StatusOK || body.Kind != "plugin" || body.ID == "" || body.HubItemID == "" {
		t.Fatalf("plugin commit = %d %+v", rec.Code, body)
	}
	if n := dbfx.Count(t, "SELECT count(*) FROM hub_item WHERE ref_id = $1 AND kind = 'plugin' AND latest_version = '1.0.0'", body.ID); n != 1 {
		t.Fatalf("hub item rows = %d, want 1", n)
	}

	// A new preview of the same version cannot be published again: published
	// versions are immutable, so the publish refuses it.
	again := previewHubImportForTest(t, "url_zip", srv.URL+"/plugin.zip")
	rec = commitHubImportForTest(t, testWorkspaceID, testUserID, map[string]any{
		"preview_id": again.PreviewID, "on_conflict": "replace",
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("replayed plugin commit = %d %s, want 409", rec.Code, rec.Body.String())
	}
}
