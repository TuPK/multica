package handler

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// readHubExportZip splits an export body into its top-level entries. Item bundles
// are themselves zips, so each one is opened and returned as a file listing.
func readHubExportZip(t *testing.T, body []byte) (hubExportManifest, map[string][]string) {
	t.Helper()
	outer, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("export is not a zip: %v", err)
	}
	var manifest hubExportManifest
	bundles := map[string][]string{}
	for _, f := range outer.File {
		data := readZipEntryForTest(t, f)
		if f.Name == "manifest.json" {
			if err := json.Unmarshal(data, &manifest); err != nil {
				t.Fatalf("manifest.json: %v", err)
			}
			continue
		}
		inner, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatalf("%s is not a zip: %v", f.Name, err)
		}
		var names []string
		for _, innerFile := range inner.File {
			names = append(names, innerFile.Name)
		}
		bundles[f.Name] = names
	}
	return manifest, bundles
}

func readZipEntryForTest(t *testing.T, f *zip.File) []byte {
	t.Helper()
	rc, err := f.Open()
	if err != nil {
		t.Fatalf("open %s: %v", f.Name, err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read %s: %v", f.Name, err)
	}
	return data
}

func exportHubItemsForTest(t *testing.T, ws, owner string) *testutil.Response {
	t.Helper()
	return testutil.Call(t, testHandler.ExportHubItems,
		hubTestRequest(http.MethodGet, "/api/hub/export", nil, ws, owner))
}

func TestHubExport_ZipHasManifestSkillsAndPlugins(t *testing.T) {
	ws := dbfx.Workspace(t, hubTestLabel(t, "workspace"), hubTestSlug(t))
	dbfx.Cleanup(t, "DELETE FROM hub_activity WHERE workspace_id = $1", ws)
	owner := dbfx.User(t, hubTestLabel(t, "owner"), hubTestEmail(t))
	dbfx.Member(t, ws, owner, hubRoleOwner)

	skillID := dbfx.Insert(t, "skill", testutil.Cols{
		"workspace_id": ws, "name": "Export Skill", "content": "# Export skill\n",
	})
	dbfx.Insert(t, "skill_file", testutil.Cols{
		"skill_id": skillID, "path": "notes/tips.md", "content": "tip one",
	})
	dbfx.Insert(t, "hub_item", testutil.Cols{
		"workspace_id": ws, "kind": "skill", "ref_id": skillID,
		"tracking": "manual", "latest_version": "3",
	})

	pkgID := dbfx.Insert(t, "plugin_package", testutil.Cols{
		"workspace_id": ws, "plugin_key": "hub-export-" + strings.ReplaceAll(hubTestUUID(t), "-", ""),
		"name": "Export Plugin",
	})
	versionID := dbfx.Insert(t, "plugin_package_version", testutil.Cols{
		"package_id": pkgID, "workspace_id": ws, "version": "1.2.0",
		"manifest": `{"name":"export-plugin","version":"1.2.0"}`,
		"digest":   strings.Repeat("a", 64), "size_bytes": 15,
	})
	dbfx.Insert(t, "plugin_package_file", testutil.Cols{
		"version_id": versionID, "path": "scripts/run.js", "content": []byte("console.log(1)\n"),
		"size_bytes": 15, "sha256": strings.Repeat("b", 64),
	})
	dbfx.Insert(t, "hub_item", testutil.Cols{
		"workspace_id": ws, "kind": "plugin", "ref_id": pkgID, "tracking": "manual",
	})

	resp := exportHubItemsForTest(t, ws, owner).Want(http.StatusOK)
	if ct := resp.Header().Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("Content-Type = %q, want application/zip", ct)
	}
	manifest, bundles := readHubExportZip(t, resp.Body.Bytes())

	if len(manifest.Items) != 2 {
		t.Fatalf("manifest items = %+v, want 2", manifest.Items)
	}
	byKind := map[string]hubExportItem{}
	for _, item := range manifest.Items {
		byKind[item.Kind] = item
	}
	skill, plugin := byKind["skill"], byKind["plugin"]
	if skill.Path != "skill/Export-Skill.zip" || skill.Name != "Export Skill" || skill.Version != "3" || skill.Digest == "" {
		t.Fatalf("skill manifest entry = %+v", skill)
	}
	if plugin.Path != "plugin/Export-Plugin.zip" || plugin.Version != "1.2.0" || plugin.Digest != strings.Repeat("a", 64) {
		t.Fatalf("plugin manifest entry = %+v", plugin)
	}

	if got := strings.Join(bundles[skill.Path], ","); got != "SKILL.md,notes/tips.md" {
		t.Fatalf("skill bundle files = %s", got)
	}
	// The virtual manifest is rendered into the plugin bundle next to its stored files.
	if got := strings.Join(bundles[plugin.Path], ","); got != "multica.plugin.json,scripts/run.js" {
		t.Fatalf("plugin bundle files = %s", got)
	}

	var count string
	if err := testPool.QueryRow(t.Context(),
		`SELECT detail->>'count' FROM hub_activity WHERE workspace_id = $1 AND action = 'item_exported'`, ws,
	).Scan(&count); err != nil {
		t.Fatalf("read export activity: %v", err)
	}
	if count != "2" {
		t.Fatalf("item_exported count = %s, want 2", count)
	}
}

func TestHubExport_SkipsItemOverTheCapAndStreamsTheRest(t *testing.T) {
	previous := hubExportBundleMaxBytes
	hubExportBundleMaxBytes = 20
	t.Cleanup(func() { hubExportBundleMaxBytes = previous })

	ws := dbfx.Workspace(t, hubTestLabel(t, "workspace"), hubTestSlug(t))
	dbfx.Cleanup(t, "DELETE FROM hub_activity WHERE workspace_id = $1", ws)
	owner := dbfx.User(t, hubTestLabel(t, "owner"), hubTestEmail(t))
	dbfx.Member(t, ws, owner, hubRoleOwner)

	for name, content := range map[string]string{
		"small-skill": "# s",
		"large-skill": strings.Repeat("x", 100),
	} {
		skillID := dbfx.Insert(t, "skill", testutil.Cols{"workspace_id": ws, "name": name, "content": content})
		dbfx.Insert(t, "hub_item", testutil.Cols{
			"workspace_id": ws, "kind": "skill", "ref_id": skillID, "tracking": "manual",
		})
	}

	resp := exportHubItemsForTest(t, ws, owner).Want(http.StatusOK)
	manifest, bundles := readHubExportZip(t, resp.Body.Bytes())
	if len(manifest.Items) != 1 || manifest.Items[0].Name != "small-skill" {
		t.Fatalf("manifest items = %+v, want only small-skill", manifest.Items)
	}
	if _, ok := bundles["skill/large-skill.zip"]; ok {
		t.Fatalf("the over-cap skill was written to the export")
	}
	if _, ok := bundles["skill/small-skill.zip"]; !ok {
		t.Fatalf("the under-cap skill is missing from the export")
	}
}

func TestHubExport_SafeNamesCannotEscapeTheirFolder(t *testing.T) {
	cases := map[string]string{
		"../../etc/passwd": "etc-passwd",
		`a/b\c`:            "a-b-c",
		".hidden":          "hidden",
		"Export Skill!":    "Export-Skill",
		"   ":              "item",
		"":                 "item",
		"日本語":              "item",
	}
	for in, want := range cases {
		got := hubExportSafeName(in)
		if got != want {
			t.Errorf("hubExportSafeName(%q) = %q, want %q", in, got, want)
		}
		if strings.ContainsAny(got, `/\`) || strings.Contains(got, "..") || strings.HasPrefix(got, ".") {
			t.Errorf("hubExportSafeName(%q) = %q is not a plain file name", in, got)
		}
	}
}

func TestHubExport_NamesAreUniqueWithinKind(t *testing.T) {
	names := newHubExportNames()
	first := names.assign("skill", "Same Name", "ref-1")
	second := names.assign("skill", "Same Name", "ref-2")
	other := names.assign("plugin", "Same Name", "ref-3")
	if first != "Same-Name" || second != "Same-Name-ref-2" || other != "Same-Name" {
		t.Fatalf("names = %q, %q, %q", first, second, other)
	}
}
