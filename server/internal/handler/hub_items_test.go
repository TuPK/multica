package handler

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/hub"
	"github.com/multica-ai/multica/server/internal/storage"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// Each test names its rows with a random suffix. Skills are unique per workspace,
// and the suite shares one database, so a fixed name would collide across runs.

func hubItemsSuffix() string {
	return uuid.NewString()[:8]
}

// insertHubItemsSkill inserts a skill with its origin config and returns its id.
func insertHubItemsSkill(t *testing.T, workspaceID, name, content, config string) string {
	t.Helper()
	return dbfx.Insert(t, "skill", testutil.Cols{
		"workspace_id": workspaceID,
		"name":         name,
		"description":  "hub items fixture",
		"content":      content,
		"config":       testutil.Raw(config),
		"created_by":   testUserID,
	})
}

// insertHubItemsPlugin inserts one published plugin version with two package files.
// The manifest is stored on the version row, as publish stores it.
func insertHubItemsPlugin(t *testing.T, workspaceID, pluginKey, version string) string {
	t.Helper()
	pkgID := dbfx.Insert(t, "plugin_package", testutil.Cols{
		"workspace_id": workspaceID,
		"plugin_key":   pluginKey,
		"name":         "Hub items plugin",
		"created_by":   testUserID,
	})
	manifest := `{"key":"` + pluginKey + `","version":"` + version + `"}`
	versionID := dbfx.Insert(t, "plugin_package_version", testutil.Cols{
		"package_id":   pkgID,
		"workspace_id": workspaceID,
		"version":      version,
		"manifest":     testutil.Raw("'" + manifest + "'::jsonb"),
		"digest":       strings.Repeat("a", 64),
		"size_bytes":   int64(len("console.log('hub items');\n")),
		"published_by": testUserID,
	})
	for _, f := range []struct{ path, body string }{{"dist/index.js", "console.log('hub items');\n"}} {
		dbfx.Insert(t, "plugin_package_file", testutil.Cols{
			"version_id": versionID,
			"path":       f.path,
			"content":    []byte(f.body),
			"size_bytes": int64(len(f.body)),
			"sha256":     strings.Repeat("b", 64),
		})
	}
	dbfx.Cleanup(t, "DELETE FROM hub_item WHERE workspace_id = $1 AND ref_id = $2", workspaceID, pkgID)
	return pkgID
}

// registerHubItemCleanup removes the hub rows and version rows of the given refs.
// It is registered after the fixtures, so it runs first.
func registerHubItemCleanup(t *testing.T, workspaceID string, refs ...string) {
	t.Helper()
	for _, ref := range refs {
		dbfx.Cleanup(t, "DELETE FROM hub_item_version_blob WHERE workspace_id = $1 AND item_id IN (SELECT id FROM hub_item WHERE workspace_id = $1 AND ref_id = $2)", workspaceID, ref)
		dbfx.Cleanup(t, "DELETE FROM hub_item WHERE workspace_id = $1 AND ref_id = $2", workspaceID, ref)
	}
}

func listHubItemsForTest(t *testing.T, workspaceID, userID string) []HubItemResponse {
	t.Helper()
	var out []HubItemResponse
	testutil.Call(t, testHandler.ListHubItems,
		hubTestRequest(http.MethodGet, "/api/hub/items", nil, workspaceID, userID)).
		Want(http.StatusOK).JSON(&out)
	return out
}

func findHubItemResponse(items []HubItemResponse, refID string) (HubItemResponse, bool) {
	for _, item := range items {
		if item.RefID == refID {
			return item, true
		}
	}
	return HubItemResponse{}, false
}

func TestHubItemsListCreatesRowsOnceAndReadsOrigin(t *testing.T) {
	ws := testWorkspaceID
	suffix := hubItemsSuffix()
	src := createHubSourceForTest(t, ws, hubTestLabel(t, "items-source"), "github", "https://github.com/acme/hub-items-"+suffix)

	manual := insertHubItemsSkill(t, ws, "hubitems-manual-"+suffix, "# Manual\n", `'{}'::jsonb`)
	tracked := insertHubItemsSkill(t, ws, "hubitems-tracked-"+suffix, "# Tracked\n",
		`'{"origin":{"type":"github","source_url":"https://github.com/acme/hub-items-`+suffix+`"}}'::jsonb`)
	orphan := insertHubItemsSkill(t, ws, "hubitems-orphan-"+suffix, "# Orphan\n",
		`'{"origin":{"type":"github","source_url":"https://github.com/acme/no-source-`+suffix+`"}}'::jsonb`)
	uploaded := insertHubItemsSkill(t, ws, "hubitems-upload-"+suffix, "# Uploaded\n",
		`'{"origin":{"type":"upload","source_url":""}}'::jsonb`)
	pkg := insertHubItemsPlugin(t, ws, "com.example.hubitems."+suffix, "1.2.0")
	registerHubItemCleanup(t, ws, manual, tracked, orphan, uploaded, pkg)

	first := listHubItemsForTest(t, ws, testUserID)
	check := func(label, ref, wantKind, wantTracking, wantSource string) HubItemResponse {
		t.Helper()
		item, ok := findHubItemResponse(first, ref)
		if !ok {
			t.Fatalf("%s: %s missing from the list", label, ref)
		}
		if item.Kind != wantKind || item.Tracking != wantTracking {
			t.Fatalf("%s: kind=%s tracking=%s, want %s/%s", label, item.Kind, item.Tracking, wantKind, wantTracking)
		}
		gotSource := ""
		if item.SourceID != nil {
			gotSource = *item.SourceID
		}
		if gotSource != wantSource {
			t.Fatalf("%s: source_id=%q, want %q", label, gotSource, wantSource)
		}
		return item
	}
	check("manual skill", manual, "skill", "manual", "")
	check("skill with a workspace source", tracked, "skill", "tracking", src.ID)
	check("skill with an unknown source", orphan, "skill", "tracking", "")
	check("upload-origin skill", uploaded, "skill", "manual", "")
	pluginItem := check("plugin", pkg, "plugin", "manual", "")
	if pluginItem.LatestVersion != "1.2.0" || pluginItem.FileCount != 2 || pluginItem.SizeBytes <= 0 {
		t.Fatalf("plugin row = version %q files %d bytes %d, want 1.2.0, 2 files (manifest + one), positive size",
			pluginItem.LatestVersion, pluginItem.FileCount, pluginItem.SizeBytes)
	}
	if pluginItem.UpdateAvailable || pluginItem.LocalModified || pluginItem.CheckedAt != nil || pluginItem.RemoteDigest != "" {
		t.Fatalf("update-state fields = %+v, want the defaults Phase 5 has not written", pluginItem)
	}

	// Listing builds no bundle and writes no version row.
	for _, ref := range []string{manual, tracked, orphan, uploaded, pkg} {
		if n := dbfx.Count(t, "SELECT count(*) FROM hub_item WHERE workspace_id = $1 AND ref_id = $2 AND bundle_key = ''", ws, ref); n != 1 {
			t.Fatalf("ref %s: rows without a bundle = %d, want 1 (the list must not build bundles)", ref, n)
		}
	}
	for _, ref := range []string{manual, tracked, orphan, uploaded, pkg} {
		if n := dbfx.Count(t, "SELECT count(*) FROM hub_item_version_blob WHERE workspace_id = $1 AND item_id IN (SELECT id FROM hub_item WHERE workspace_id = $1 AND ref_id = $2)", ws, ref); n != 0 {
			t.Fatalf("ref %s: version rows after list = %d, want 0", ref, n)
		}
	}

	// A second list changes nothing: same rows, one per item.
	second := listHubItemsForTest(t, ws, testUserID)
	for _, ref := range []string{manual, tracked, orphan, uploaded, pkg} {
		a, _ := findHubItemResponse(first, ref)
		b, ok := findHubItemResponse(second, ref)
		if !ok || a.ID != b.ID {
			t.Fatalf("ref %s: hub row changed between lists (%q -> %q)", ref, a.ID, b.ID)
		}
		if n := dbfx.Count(t, "SELECT count(*) FROM hub_item WHERE workspace_id = $1 AND ref_id = $2", ws, ref); n != 1 {
			t.Fatalf("ref %s: hub rows = %d after two lists, want 1", ref, n)
		}
	}
}

func TestHubItemsListIsScopedToTheWorkspace(t *testing.T) {
	suffix := hubItemsSuffix()
	here := testWorkspaceID
	other := dbfx.Workspace(t, "Hub items other "+suffix, "hub-items-other-"+suffix)
	dbfx.Member(t, other, testUserID, "member")

	mine := insertHubItemsSkill(t, here, "hubitems-here-"+suffix, "# Here\n", `'{}'::jsonb`)
	theirs := insertHubItemsSkill(t, other, "hubitems-there-"+suffix, "# There\n", `'{}'::jsonb`)
	registerHubItemCleanup(t, here, mine)
	registerHubItemCleanup(t, other, theirs)

	inHere := listHubItemsForTest(t, here, testUserID)
	if _, ok := findHubItemResponse(inHere, theirs); ok {
		t.Fatal("the first workspace's list contains the second workspace's skill")
	}
	if _, ok := findHubItemResponse(inHere, mine); !ok {
		t.Fatal("the first workspace's list is missing its own skill")
	}
	inOther := listHubItemsForTest(t, other, testUserID)
	if _, ok := findHubItemResponse(inOther, mine); ok {
		t.Fatal("the second workspace's list contains the first workspace's skill")
	}
}

func TestHubItemsListRequiresMembership(t *testing.T) {
	outsider := dbfx.User(t, "Hub items outsider", "hub-items-outsider-"+hubItemsSuffix()+"@multica.ai")
	testutil.Call(t, testHandler.ListHubItems,
		hubTestRequest(http.MethodGet, "/api/hub/items", nil, testWorkspaceID, outsider)).
		Want(http.StatusNotFound)
}

// hubBundleBody decodes GET .../bundle.
func getHubItemBundleForTest(t *testing.T, workspaceID, userID, kind, refID string) HubItemBundleResponse {
	t.Helper()
	var out HubItemBundleResponse
	req := withURLParams(hubTestRequest(http.MethodGet, "/api/hub/items/"+kind+"/"+refID+"/bundle", nil, workspaceID, userID), "kind", kind, "id", refID)
	testutil.Call(t, testHandler.GetHubItemBundle, req).Want(http.StatusOK).JSON(&out)
	return out
}

// zipEntryNames returns the entry names of a zip archive.
func zipEntryNames(t *testing.T, data []byte) []string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("bundle is not a zip: %v", err)
	}
	names := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	return names
}

func TestHubBundleBuildsOnFirstRequestAndDownloadsVerifiably(t *testing.T) {
	setupHubImportTest(t)
	ws := testWorkspaceID
	suffix := hubItemsSuffix()
	skill := insertHubItemsSkill(t, ws, "hubitems-bundle-"+suffix, "---\nname: hubitems-bundle\ndescription: fixture\n---\n# Bundle\n", `'{}'::jsonb`)
	dbfx.Insert(t, "skill_file", testutil.Cols{"skill_id": skill, "path": "references/notes.md", "content": "notes\n"})
	registerHubItemCleanup(t, ws, skill)

	before := dbfx.Count(t, "SELECT count(*) FROM hub_item WHERE workspace_id = $1 AND ref_id = $2 AND bundle_key <> ''", ws, skill)
	if before != 0 {
		t.Fatalf("bundle recorded before any request: %d", before)
	}

	got := getHubItemBundleForTest(t, ws, testUserID, "skill", skill)
	if !strings.HasPrefix(got.URL, "/api/hub/items/skill/"+skill+"/bundle/download") {
		t.Fatalf("url = %q, want the authenticated download route (the local store cannot presign)", got.URL)
	}
	if got.ExpiresIn != 900 || len(got.Digest) != 64 || got.SizeBytes <= 0 || got.Version != got.Digest {
		t.Fatalf("bundle response = %+v, want a 64-char digest used as the version and a 900s expiry", got)
	}
	if n := dbfx.Count(t, "SELECT count(*) FROM hub_item WHERE workspace_id = $1 AND ref_id = $2 AND bundle_key LIKE 'hub/%/skill/%/bundle.zip' AND bundle_digest = $3", ws, skill, got.Digest); n != 1 {
		t.Fatalf("hub row does not record the bundle: %d matching rows", n)
	}
	if n := dbfx.Count(t, "SELECT count(*) FROM hub_item_version_blob WHERE workspace_id = $1 AND item_id IN (SELECT id FROM hub_item WHERE workspace_id = $1 AND ref_id = $2) AND digest = $3", ws, skill, got.Digest); n != 1 {
		t.Fatalf("version rows for the digest = %d, want 1", n)
	}

	// A second request serves the same bundle and writes no second version row.
	again := getHubItemBundleForTest(t, ws, testUserID, "skill", skill)
	if again.Digest != got.Digest {
		t.Fatalf("second digest = %s, want %s", again.Digest, got.Digest)
	}
	if n := dbfx.Count(t, "SELECT count(*) FROM hub_item_version_blob WHERE workspace_id = $1 AND item_id IN (SELECT id FROM hub_item WHERE workspace_id = $1 AND ref_id = $2)", ws, skill); n != 1 {
		t.Fatalf("version rows after a repeat request = %d, want 1", n)
	}

	rec := testutil.Call(t, testHandler.DownloadHubItemBundle,
		withURLParams(hubTestRequest(http.MethodGet, "/api/hub/items/skill/"+skill+"/bundle/download", nil, ws, testUserID), "kind", "skill", "id", skill)).
		Want(http.StatusOK)
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("content type = %q, want application/zip", ct)
	}
	data := rec.Body.Bytes()
	names := zipEntryNames(t, data)
	if strings.Join(names, ",") != "SKILL.md,references/notes.md" {
		t.Fatalf("bundle entries = %v, want SKILL.md and references/notes.md", names)
	}
	bundle, err := hub.ExtractZip(bytes.NewReader(data), int64(len(data)), hub.DefaultLimits)
	if err != nil {
		t.Fatalf("extract served bundle: %v", err)
	}
	if digest := hub.Digest(bundle); digest != got.Digest {
		t.Fatalf("served bundle digest = %s, want the digest the link reported %s", digest, got.Digest)
	}
}

// presigningHubStore makes the Hub store answer presign requests, as S3 does.
type presigningHubStore struct {
	storage.Storage
	url string
}

func (s presigningHubStore) PresignGet(_ context.Context, key string, _ time.Duration) (string, error) {
	return s.url + "/" + key, nil
}

func TestHubBundleReturnsPresignedLinkWhenTheStoreCanSign(t *testing.T) {
	setupHubImportTest(t)
	ws := testWorkspaceID
	skill := insertHubItemsSkill(t, ws, "hubitems-presign-"+hubItemsSuffix(), "# Presign\n", `'{}'::jsonb`)
	registerHubItemCleanup(t, ws, skill)

	prev := testHandler.HubStorage
	testHandler.HubStorage = presigningHubStore{Storage: prev, url: "https://files.example.test/hub"}
	t.Cleanup(func() { testHandler.HubStorage = prev })

	got := getHubItemBundleForTest(t, ws, testUserID, "skill", skill)
	if !strings.HasPrefix(got.URL, "https://files.example.test/hub/hub/") || !strings.HasSuffix(got.URL, "/bundle.zip") {
		t.Fatalf("url = %q, want the presigned link", got.URL)
	}
}

func TestHubBundlePluginUsesThePublishedVersion(t *testing.T) {
	setupHubImportTest(t)
	ws := testWorkspaceID
	pkg := insertHubItemsPlugin(t, ws, "com.example.hubbundle."+hubItemsSuffix(), "2.0.1")

	got := getHubItemBundleForTest(t, ws, testUserID, "plugin", pkg)
	if got.Version != "2.0.1" {
		t.Fatalf("version = %q, want the published 2.0.1", got.Version)
	}
	rec := testutil.Call(t, testHandler.DownloadHubItemBundle,
		withURLParams(hubTestRequest(http.MethodGet, "/api/hub/items/plugin/"+pkg+"/bundle/download", nil, ws, testUserID), "kind", "plugin", "id", pkg)).
		Want(http.StatusOK)
	names := zipEntryNames(t, rec.Body.Bytes())
	if strings.Join(names, ",") != "dist/index.js,multica.plugin.json" {
		t.Fatalf("plugin bundle entries = %v, want the package file and the manifest", names)
	}
	if n := dbfx.Count(t, "SELECT count(*) FROM hub_item_version_blob WHERE workspace_id = $1 AND version = '2.0.1' AND item_id IN (SELECT id FROM hub_item WHERE workspace_id = $1 AND ref_id = $2)", ws, pkg); n != 1 {
		t.Fatalf("version row for 2.0.1 = %d, want 1", n)
	}
}

func TestHubBundleRejectsUnknownItemsAndBadParams(t *testing.T) {
	setupHubImportTest(t)
	suffix := hubItemsSuffix()
	here := testWorkspaceID
	other := dbfx.Workspace(t, "Hub bundle other "+suffix, "hub-bundle-other-"+suffix)
	dbfx.Member(t, other, testUserID, "member")
	skill := insertHubItemsSkill(t, here, "hubitems-scope-"+suffix, "# Scope\n", `'{}'::jsonb`)
	registerHubItemCleanup(t, here, skill)

	// The skill exists, but not in the workspace the request names.
	req := withURLParams(hubTestRequest(http.MethodGet, "/api/hub/items/skill/"+skill+"/bundle", nil, other, testUserID), "kind", "skill", "id", skill)
	testutil.Call(t, testHandler.GetHubItemBundle, req).Want(http.StatusNotFound)
	// A skill that does not exist.
	missing := uuid.NewString()
	req = withURLParams(hubTestRequest(http.MethodGet, "/api/hub/items/skill/"+missing+"/bundle", nil, here, testUserID), "kind", "skill", "id", missing)
	testutil.Call(t, testHandler.GetHubItemBundle, req).Want(http.StatusNotFound)
	// A kind the hub does not hold.
	req = withURLParams(hubTestRequest(http.MethodGet, "/api/hub/items/widget/"+skill+"/bundle", nil, here, testUserID), "kind", "widget", "id", skill)
	testutil.Call(t, testHandler.GetHubItemBundle, req).Want(http.StatusBadRequest)
	// An id that is not a UUID.
	req = withURLParams(hubTestRequest(http.MethodGet, "/api/hub/items/skill/nope/bundle", nil, here, testUserID), "kind", "skill", "id", "nope")
	testutil.Call(t, testHandler.GetHubItemBundle, req).Want(http.StatusBadRequest)
	// A user who is not a member of the workspace.
	outsider := dbfx.User(t, "Hub bundle outsider", "hub-bundle-outsider-"+suffix+"@multica.ai")
	req = withURLParams(hubTestRequest(http.MethodGet, "/api/hub/items/skill/"+skill+"/bundle", nil, here, outsider), "kind", "skill", "id", skill)
	testutil.Call(t, testHandler.GetHubItemBundle, req).Want(http.StatusNotFound)
}

func TestHubImportCommitWritesVersionRow(t *testing.T) {
	setupHubImportTest(t)
	name := hubImportSkillName(t, "blob")
	srvURL := hubImportServe(t, map[string][]byte{"/blob.zip": hubImportZip(t, hubImportSkillFiles(name, nil))}).URL + "/blob.zip"
	out := previewHubImportForTest(t, "url_zip", srvURL)

	rec := commitHubImportForTest(t, testWorkspaceID, testUserID, map[string]any{"preview_id": out.PreviewID})
	body := decodeHubImportCommit(t, rec)
	if rec.Code != http.StatusOK || body.HubItemID == "" {
		t.Fatalf("commit = %d %+v", rec.Code, body)
	}
	dbfx.Cleanup(t, "DELETE FROM hub_item_version_blob WHERE item_id = $1", body.HubItemID)
	dbfx.Cleanup(t, "DELETE FROM hub_item WHERE id = $1", body.HubItemID)
	dbfx.Cleanup(t, "DELETE FROM skill WHERE id = $1", body.ID)
	dbfx.Cleanup(t, "DELETE FROM hub_source WHERE workspace_id = $1 AND type = 'url_zip' AND url = $2", testWorkspaceID, srvURL)

	var digest, bundleKey string
	dbfx.QueryRow(t, "SELECT bundle_digest, bundle_key FROM hub_item WHERE id = $1", body.HubItemID).Scan(&digest, &bundleKey)
	if n := dbfx.Count(t, "SELECT count(*) FROM hub_item_version_blob WHERE item_id = $1 AND version = $2 AND digest = $2 AND bundle_key = $3", body.HubItemID, digest, bundleKey); n != 1 {
		t.Fatalf("version row for the committed bundle = %d, want 1", n)
	}
}

func TestHubBundleAndListDropAnItemWhoseSkillWasDeleted(t *testing.T) {
	setupHubImportTest(t)
	ws := testWorkspaceID
	skill := insertHubItemsSkill(t, ws, "hubitems-gone-"+hubItemsSuffix(), "# Gone\n", `'{}'::jsonb`)
	registerHubItemCleanup(t, ws, skill)

	getHubItemBundleForTest(t, ws, testUserID, "skill", skill)
	dbfx.Exec(t, "DELETE FROM skill WHERE id = $1", skill)

	req := withURLParams(hubTestRequest(http.MethodGet, "/api/hub/items/skill/"+skill+"/bundle", nil, ws, testUserID), "kind", "skill", "id", skill)
	testutil.Call(t, testHandler.GetHubItemBundle, req).Want(http.StatusNotFound)
	if _, ok := findHubItemResponse(listHubItemsForTest(t, ws, testUserID), skill); ok {
		t.Fatal("the list still shows a skill that was deleted")
	}
}
