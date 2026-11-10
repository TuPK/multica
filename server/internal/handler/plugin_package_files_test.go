package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/testutil"
)

var pluginFilesKeySeq atomic.Int64

// seedPublishedVersion inserts one package with one published version holding
// the given files. Rows are removed by the fixture when the test finishes, in
// the reverse order they were created: files, version, package.
func seedPublishedVersion(t *testing.T, workspaceID string, files map[string][]byte) (packageID, versionID string) {
	t.Helper()
	key := fmt.Sprintf("com.example.files%d-%d", time.Now().UnixNano(), pluginFilesKeySeq.Add(1))
	packageID = dbfx.Insert(t, "plugin_package", testutil.Cols{
		"workspace_id": workspaceID,
		"plugin_key":   key,
		"name":         "Files Test",
	})
	versionID = dbfx.Insert(t, "plugin_package_version", testutil.Cols{
		"package_id":   packageID,
		"workspace_id": workspaceID,
		"version":      "1.0.0",
		"manifest":     []byte(`{"manifest_version":1}`),
		"digest":       strings.Repeat("a", 64),
		"size_bytes":   0,
	})
	for path, content := range files {
		sum := sha256.Sum256(content)
		dbfx.Insert(t, "plugin_package_file", testutil.Cols{
			"version_id": versionID,
			"path":       path,
			"content":    content,
			"size_bytes": len(content),
			"sha256":     hex.EncodeToString(sum[:]),
		})
	}
	return packageID, versionID
}

func pluginFilesPath(workspaceID, packageID, versionID, query string) string {
	path := fmt.Sprintf("/api/workspaces/%s/plugins/packages/%s/versions/%s/files", workspaceID, packageID, versionID)
	if query != "" {
		path += "/content?" + query
	}
	return path
}

func pluginPathQuery(path string) string {
	return "path=" + url.QueryEscape(path)
}

func pluginFilesParams(workspaceID, packageID, versionID string) map[string]string {
	return map[string]string{"id": workspaceID, "packageId": packageID, "versionId": versionID}
}

type pluginFileListResponse struct {
	Files []struct {
		Path      string `json:"path"`
		SizeBytes int64  `json:"size_bytes"`
		SHA256    string `json:"sha256"`
	} `json:"files"`
}

type pluginFileContentResponse struct {
	Path      string  `json:"path"`
	Content   *string `json:"content"`
	IsBinary  bool    `json:"is_binary"`
	SizeBytes int64   `json:"size_bytes"`
	SHA256    string  `json:"sha256"`
}

func TestPluginPackageVersionFilesListsSortedFiles(t *testing.T) {
	withPluginsV1Flag(t, testHandler, true)
	readme := []byte("# Hello\n")
	script := []byte("export const x = 1;\n")
	packageID, versionID := seedPublishedVersion(t, testWorkspaceID, map[string][]byte{
		"ui/main.js":        script,
		"README.md":         readme,
		"skills/a/SKILL.md": []byte("---\nname: a\n---\n"),
	})

	recorder := httptest.NewRecorder()
	testHandler.ListPluginPackageVersionFiles(recorder, pluginHandlerRequest(http.MethodGet,
		pluginFilesPath(testWorkspaceID, packageID, versionID, ""), nil, pluginFilesParams(testWorkspaceID, packageID, versionID)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var list pluginFileListResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	// The virtual manifest is listed too; byte order puts it between README.md and skills/.
	if len(list.Files) != 4 {
		t.Fatalf("files=%d want 4: %s", len(list.Files), recorder.Body.String())
	}
	wantOrder := []string{"README.md", "multica.plugin.json", "skills/a/SKILL.md", "ui/main.js"}
	for i, path := range wantOrder {
		if list.Files[i].Path != path {
			t.Fatalf("file %d path=%q want %q", i, list.Files[i].Path, path)
		}
	}
	if got := list.Files[3]; got.SizeBytes != int64(len(script)) || got.SHA256 != fmt.Sprintf("%x", sha256.Sum256(script)) {
		t.Fatalf("ui/main.js entry = %+v", got)
	}
}

// seededManifestText is what the seeded version's manifest `{"manifest_version":1}`
// renders to as multica.plugin.json: two-space indent and a trailing newline.
const seededManifestText = "{\n  \"manifest_version\": 1\n}\n"

func TestPluginPackageVersionFilesListsVirtualManifestForEmptyVersion(t *testing.T) {
	withPluginsV1Flag(t, testHandler, true)
	packageID, versionID := seedPublishedVersion(t, testWorkspaceID, nil)

	recorder := httptest.NewRecorder()
	testHandler.ListPluginPackageVersionFiles(recorder, pluginHandlerRequest(http.MethodGet,
		pluginFilesPath(testWorkspaceID, packageID, versionID, ""), nil, pluginFilesParams(testWorkspaceID, packageID, versionID)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var list pluginFileListResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list.Files) != 1 {
		t.Fatalf("files=%d want only the virtual manifest: %s", len(list.Files), recorder.Body.String())
	}
	got := list.Files[0]
	wantSum := sha256.Sum256([]byte(seededManifestText))
	if got.Path != "multica.plugin.json" || got.SizeBytes != int64(len(seededManifestText)) || got.SHA256 != fmt.Sprintf("%x", wantSum) {
		t.Fatalf("virtual manifest entry = %+v", got)
	}
}

func TestPluginPackageVersionFileContentServesVirtualManifest(t *testing.T) {
	withPluginsV1Flag(t, testHandler, true)
	packageID, versionID := seedPublishedVersion(t, testWorkspaceID, map[string][]byte{"ui/main.js": []byte("x")})

	recorder := httptest.NewRecorder()
	testHandler.GetPluginPackageVersionFileContent(recorder, pluginHandlerRequest(http.MethodGet,
		pluginFilesPath(testWorkspaceID, packageID, versionID, pluginPathQuery("multica.plugin.json")), nil,
		pluginFilesParams(testWorkspaceID, packageID, versionID)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("content status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var got pluginFileContentResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode content: %v", err)
	}
	if got.Content == nil || *got.Content != seededManifestText || got.IsBinary {
		t.Fatalf("virtual manifest content = %+v", got)
	}
	if got.SizeBytes != int64(len(seededManifestText)) || got.SHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(seededManifestText))) {
		t.Fatalf("virtual manifest metadata = %+v", got)
	}

	// The list and the content endpoint must describe the same bytes.
	listRecorder := httptest.NewRecorder()
	testHandler.ListPluginPackageVersionFiles(listRecorder, pluginHandlerRequest(http.MethodGet,
		pluginFilesPath(testWorkspaceID, packageID, versionID, ""), nil, pluginFilesParams(testWorkspaceID, packageID, versionID)))
	var list pluginFileListResponse
	if err := json.Unmarshal(listRecorder.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	for _, entry := range list.Files {
		if entry.Path == "multica.plugin.json" && (entry.SHA256 != got.SHA256 || entry.SizeBytes != got.SizeBytes) {
			t.Fatalf("list entry %+v disagrees with content %+v", entry, got)
		}
	}
}

func TestPluginPackageVersionRealManifestFileWinsOverVirtual(t *testing.T) {
	withPluginsV1Flag(t, testHandler, true)
	real := []byte("{\"real\":true}")
	packageID, versionID := seedPublishedVersion(t, testWorkspaceID, map[string][]byte{"multica.plugin.json": real})

	listRecorder := httptest.NewRecorder()
	testHandler.ListPluginPackageVersionFiles(listRecorder, pluginHandlerRequest(http.MethodGet,
		pluginFilesPath(testWorkspaceID, packageID, versionID, ""), nil, pluginFilesParams(testWorkspaceID, packageID, versionID)))
	var list pluginFileListResponse
	if err := json.Unmarshal(listRecorder.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list.Files) != 1 || list.Files[0].SizeBytes != int64(len(real)) || list.Files[0].SHA256 != fmt.Sprintf("%x", sha256.Sum256(real)) {
		t.Fatalf("list must show only the real row: %s", listRecorder.Body.String())
	}

	recorder := httptest.NewRecorder()
	testHandler.GetPluginPackageVersionFileContent(recorder, pluginHandlerRequest(http.MethodGet,
		pluginFilesPath(testWorkspaceID, packageID, versionID, pluginPathQuery("multica.plugin.json")), nil,
		pluginFilesParams(testWorkspaceID, packageID, versionID)))
	var got pluginFileContentResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode content: %v", err)
	}
	if got.Content == nil || *got.Content != string(real) {
		t.Fatalf("content must be the real row, got %s", recorder.Body.String())
	}
}

func TestPluginPackageVersionFileContentReturnsText(t *testing.T) {
	withPluginsV1Flag(t, testHandler, true)
	content := []byte("console.log('héllo');\n")
	packageID, versionID := seedPublishedVersion(t, testWorkspaceID, map[string][]byte{"ui/main.js": content})

	recorder := httptest.NewRecorder()
	testHandler.GetPluginPackageVersionFileContent(recorder, pluginHandlerRequest(http.MethodGet,
		pluginFilesPath(testWorkspaceID, packageID, versionID, pluginPathQuery("ui/main.js")), nil,
		pluginFilesParams(testWorkspaceID, packageID, versionID)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("content status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var got pluginFileContentResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode content: %v", err)
	}
	if got.Content == nil || *got.Content != string(content) || got.IsBinary {
		t.Fatalf("text file = %+v", got)
	}
	if got.Path != "ui/main.js" || got.SizeBytes != int64(len(content)) || got.SHA256 != fmt.Sprintf("%x", sha256.Sum256(content)) {
		t.Fatalf("metadata = %+v", got)
	}
}

func TestPluginPackageVersionFileContentWithholdsBinaryAndOversizedText(t *testing.T) {
	withPluginsV1Flag(t, testHandler, true)
	const limit = 1 << 20
	cases := map[string]struct {
		content    []byte
		wantBinary bool
	}{
		"NUL byte":           {content: []byte("PNG\x00\x01\x02"), wantBinary: true},
		"invalid UTF-8":      {content: []byte{0xff, 0xfe, 'a'}, wantBinary: true},
		"over 1 MiB of text": {content: []byte(strings.Repeat("a", limit+1)), wantBinary: true},
		"exactly 1 MiB text": {content: []byte(strings.Repeat("a", limit)), wantBinary: false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			path := "assets/file.bin"
			packageID, versionID := seedPublishedVersion(t, testWorkspaceID, map[string][]byte{path: tc.content})

			recorder := httptest.NewRecorder()
			testHandler.GetPluginPackageVersionFileContent(recorder, pluginHandlerRequest(http.MethodGet,
				pluginFilesPath(testWorkspaceID, packageID, versionID, pluginPathQuery(path)), nil,
				pluginFilesParams(testWorkspaceID, packageID, versionID)))
			if recorder.Code != http.StatusOK {
				t.Fatalf("content status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			// Decode into a raw map too: `"content": null` must be explicit, not
			// an omitted key, or the client cannot tell "binary" from "missing".
			var raw map[string]json.RawMessage
			if err := json.Unmarshal(recorder.Body.Bytes(), &raw); err != nil {
				t.Fatalf("decode raw: %v", err)
			}
			var got pluginFileContentResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode content: %v", err)
			}
			if got.IsBinary != tc.wantBinary {
				t.Fatalf("is_binary=%v want %v", got.IsBinary, tc.wantBinary)
			}
			if tc.wantBinary && (got.Content != nil || string(raw["content"]) != "null") {
				t.Fatalf("binary file must return content null, got %s", recorder.Body.String())
			}
			if !tc.wantBinary && (got.Content == nil || len(*got.Content) != len(tc.content)) {
				t.Fatalf("text file must return its content")
			}
			if got.SizeBytes != int64(len(tc.content)) {
				t.Fatalf("size_bytes=%d want real size %d", got.SizeBytes, len(tc.content))
			}
		})
	}
}

func TestPluginPackageVersionFilesDoNotLeakAcrossWorkspaces(t *testing.T) {
	withPluginsV1Flag(t, testHandler, true)
	otherWorkspaceID := dbfx.Workspace(t, "Plugin Files Other", fmt.Sprintf("plugin-files-other-%d", time.Now().UnixNano()))
	otherPackageID, otherVersionID := seedPublishedVersion(t, otherWorkspaceID, map[string][]byte{"ui/main.js": []byte("x")})
	ownPackageID, ownVersionID := seedPublishedVersion(t, testWorkspaceID, map[string][]byte{"ui/main.js": []byte("y")})

	// A package from another workspace, addressed through this workspace's URL.
	// The path names a real file, so only the ownership check stands between
	// the caller and the bytes.
	requests := map[string]struct {
		handler http.HandlerFunc
		path    string
		params  map[string]string
	}{
		"list foreign package": {
			handler: testHandler.ListPluginPackageVersionFiles,
			path:    pluginFilesPath(testWorkspaceID, otherPackageID, otherVersionID, ""),
			params:  pluginFilesParams(testWorkspaceID, otherPackageID, otherVersionID),
		},
		"content foreign package": {
			handler: testHandler.GetPluginPackageVersionFileContent,
			path:    pluginFilesPath(testWorkspaceID, otherPackageID, otherVersionID, pluginPathQuery("ui/main.js")),
			params:  pluginFilesParams(testWorkspaceID, otherPackageID, otherVersionID),
		},
		"version of another package in this workspace": {
			handler: testHandler.GetPluginPackageVersionFileContent,
			path:    pluginFilesPath(testWorkspaceID, ownPackageID, otherVersionID, pluginPathQuery("ui/main.js")),
			params:  pluginFilesParams(testWorkspaceID, ownPackageID, otherVersionID),
		},
	}
	for name, tc := range requests {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			tc.handler(recorder, pluginHandlerRequest(http.MethodGet, tc.path, nil, tc.params))
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("status=%d want 404 body=%s", recorder.Code, recorder.Body.String())
			}
			if strings.Contains(recorder.Body.String(), `"sha256"`) || strings.Contains(recorder.Body.String(), `"content"`) {
				t.Fatalf("404 body leaks file data: %s", recorder.Body.String())
			}
		})
	}

	// The reverse: this workspace's package addressed through the other
	// workspace's URL. The caller is not a member there, so membership refuses
	// it before the package is looked at.
	t.Run("own package through another workspace URL", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		testHandler.GetPluginPackageVersionFileContent(recorder, pluginHandlerRequest(http.MethodGet,
			pluginFilesPath(otherWorkspaceID, ownPackageID, ownVersionID, pluginPathQuery("ui/main.js")), nil,
			pluginFilesParams(otherWorkspaceID, ownPackageID, ownVersionID)))
		if recorder.Code == http.StatusOK {
			t.Fatalf("non-member read succeeded: %s", recorder.Body.String())
		}
		if strings.Contains(recorder.Body.String(), `"content"`) {
			t.Fatalf("non-member response leaks content: %s", recorder.Body.String())
		}
	})
}

func TestPluginPackageVersionFileContentRejectsBadPathAndMissingFile(t *testing.T) {
	withPluginsV1Flag(t, testHandler, true)
	packageID, versionID := seedPublishedVersion(t, testWorkspaceID, map[string][]byte{"ui/main.js": []byte("x")})
	params := pluginFilesParams(testWorkspaceID, packageID, versionID)

	badPaths := map[string]string{
		"missing":          "",
		"absolute":         "/ui/main.js",
		"parent traversal": "../secret",
		"inner traversal":  "ui/../../main.js",
		"empty segment":    "ui//main.js",
		"dot segment":      "./ui/main.js",
		"backslash":        `ui\main.js`,
	}
	for name, path := range badPaths {
		t.Run(name, func(t *testing.T) {
			query := ""
			if path != "" {
				query = pluginPathQuery(path)
			}
			recorder := httptest.NewRecorder()
			testHandler.GetPluginPackageVersionFileContent(recorder, pluginHandlerRequest(http.MethodGet,
				pluginFilesPath(testWorkspaceID, packageID, versionID, query), nil, params))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status=%d want 400 body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}

	t.Run("file not in version", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		testHandler.GetPluginPackageVersionFileContent(recorder, pluginHandlerRequest(http.MethodGet,
			pluginFilesPath(testWorkspaceID, packageID, versionID, pluginPathQuery("ui/absent.js")), nil, params))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("status=%d want 404 body=%s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("malformed version id", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		testHandler.ListPluginPackageVersionFiles(recorder, pluginHandlerRequest(http.MethodGet,
			pluginFilesPath(testWorkspaceID, packageID, "not-a-uuid", ""), nil,
			pluginFilesParams(testWorkspaceID, packageID, "not-a-uuid")))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status=%d want 400 body=%s", recorder.Code, recorder.Body.String())
		}
	})
}

func TestPluginPackageVersionFilesRequirePluginsV1(t *testing.T) {
	withPluginsV1Flag(t, testHandler, false)
	packageID, versionID := seedPublishedVersion(t, testWorkspaceID, map[string][]byte{"ui/main.js": []byte("x")})

	recorder := httptest.NewRecorder()
	testHandler.ListPluginPackageVersionFiles(recorder, pluginHandlerRequest(http.MethodGet,
		pluginFilesPath(testWorkspaceID, packageID, versionID, ""), nil, pluginFilesParams(testWorkspaceID, packageID, versionID)))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403 body=%s", recorder.Code, recorder.Body.String())
	}
}
