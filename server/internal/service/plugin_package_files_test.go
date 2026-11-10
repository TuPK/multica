package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestClassifyPluginPackageFile(t *testing.T) {
	limit := pluginPackageFileTextLimit
	cases := map[string]struct {
		content    []byte
		wantBinary bool
	}{
		"plain text":        {content: []byte("export {};\n"), wantBinary: false},
		"empty file":        {content: []byte{}, wantBinary: false},
		"multi-byte UTF-8":  {content: []byte("名前 é"), wantBinary: false},
		"NUL byte":          {content: []byte("a\x00b"), wantBinary: true},
		"invalid UTF-8":     {content: []byte{0xc3, 0x28}, wantBinary: true},
		"one byte over MiB": {content: make([]byte, limit+1), wantBinary: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := classifyPluginPackageFile(db.PluginPackageFile{
				Path:      "f",
				Content:   tc.content,
				SizeBytes: int64(len(tc.content)),
				Sha256:    "s",
			})
			if got.IsBinary != tc.wantBinary {
				t.Fatalf("is_binary=%v want %v", got.IsBinary, tc.wantBinary)
			}
			if tc.wantBinary == (got.Content != nil) {
				t.Fatalf("content presence wrong for %s: %+v", name, got)
			}
			if got.SizeBytes != int64(len(tc.content)) {
				t.Fatalf("size_bytes=%d want %d", got.SizeBytes, len(tc.content))
			}
		})
	}
}

func TestValidatePluginPackageFilePath(t *testing.T) {
	for _, valid := range []string{"README.md", "ui/main.js", "skills/a/SKILL.md", "a.b/c-d_e"} {
		if err := validatePluginPackageFilePath(valid); err != nil {
			t.Errorf("%q rejected: %v", valid, err)
		}
	}
	for _, invalid := range []string{"", "/abs", "../up", "a/../b", "a//b", "./a", "a/.", `a\b`, "a\x00b", strings.Repeat("a", maxPluginPackageFilePathLength+1)} {
		err := validatePluginPackageFilePath(invalid)
		var pluginErr *PluginError
		if !errors.As(err, &pluginErr) || pluginErr.Kind != PluginErrorInvalid {
			t.Errorf("%q: err=%v want PluginErrorInvalid", invalid, err)
		}
	}
}

// TestPluginPackageFilesAreScopedToWorkspace seeds its own workspaces, so the
// rows it asserts on are its own even when other packages share the database.
func TestPluginPackageFilesAreScopedToWorkspace(t *testing.T) {
	pool := sharedTestPool(t)
	ctx := context.Background()
	suffix := time.Now().UnixNano()

	insertWorkspace := func(label string) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO workspace (name, slug) VALUES ($1, $2) RETURNING id`,
			"plugin files "+label, fmt.Sprintf("plugin-files-%s-%d", label, suffix)).Scan(&id); err != nil {
			t.Fatalf("seed workspace %s: %v", label, err)
		}
		t.Cleanup(func() { pool.Exec(context.Background(), `DELETE FROM workspace WHERE id = $1`, id) })
		return id
	}
	ownWS := insertWorkspace("own")
	otherWS := insertWorkspace("other")

	var packageID, versionID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO plugin_package (workspace_id, plugin_key, name) VALUES ($1, $2, 'Files') RETURNING id`,
		ownWS, fmt.Sprintf("com.example.svcfiles%d", suffix)).Scan(&packageID); err != nil {
		t.Fatalf("seed package: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM plugin_package_file WHERE version_id IN (SELECT id FROM plugin_package_version WHERE package_id = $1)`, packageID)
		pool.Exec(context.Background(), `DELETE FROM plugin_package_version WHERE package_id = $1`, packageID)
		pool.Exec(context.Background(), `DELETE FROM plugin_package WHERE id = $1`, packageID)
	})
	if err := pool.QueryRow(ctx,
		`INSERT INTO plugin_package_version (package_id, workspace_id, version, manifest, digest, size_bytes)
		 VALUES ($1, $2, '1.0.0', '{}', $3, 0) RETURNING id`,
		packageID, ownWS, strings.Repeat("b", 64)).Scan(&versionID); err != nil {
		t.Fatalf("seed version: %v", err)
	}
	files := map[string][]byte{"ui/main.js": []byte("ok"), "bin.dat": {0, 1, 2}}
	for path, content := range files {
		sum := sha256.Sum256(content)
		if _, err := pool.Exec(ctx,
			`INSERT INTO plugin_package_file (version_id, path, content, size_bytes, sha256) VALUES ($1, $2, $3, $4, $5)`,
			versionID, path, content, len(content), hex.EncodeToString(sum[:])); err != nil {
			t.Fatalf("seed file %s: %v", path, err)
		}
	}

	svc := &PluginService{Queries: db.New(pool)}
	own := pluginFilesTestUUID(t, ownWS)
	other := pluginFilesTestUUID(t, otherWS)

	entries, err := svc.ListPackageVersionFiles(ctx, own, packageID, versionID)
	if err != nil {
		t.Fatalf("list own: %v", err)
	}
	// The virtual manifest sorts between bin.dat and ui/main.js.
	wantPaths := []string{"bin.dat", "multica.plugin.json", "ui/main.js"}
	if len(entries) != len(wantPaths) {
		t.Fatalf("entries = %+v", entries)
	}
	for i, path := range wantPaths {
		if entries[i].Path != path {
			t.Fatalf("entry %d = %q want %q", i, entries[i].Path, path)
		}
	}

	bin, err := svc.PackageVersionFile(ctx, own, packageID, versionID, "bin.dat")
	if err != nil {
		t.Fatalf("read binary: %v", err)
	}
	if !bin.IsBinary || bin.Content != nil || bin.SizeBytes != 3 {
		t.Fatalf("binary file = %+v", bin)
	}

	// The version's manifest is `{}`, rendered as indented text with a newline.
	manifest, err := svc.PackageVersionFile(ctx, own, packageID, versionID, "multica.plugin.json")
	if err != nil {
		t.Fatalf("read virtual manifest: %v", err)
	}
	if manifest.Content == nil || *manifest.Content != "{}\n" || manifest.IsBinary || manifest.SizeBytes != 3 {
		t.Fatalf("virtual manifest = %+v", manifest)
	}
	if manifest.SHA256 != entries[1].SHA256 {
		t.Fatalf("virtual manifest digest %s disagrees with list %s", manifest.SHA256, entries[1].SHA256)
	}

	for name, call := range map[string]func() error{
		"list from another workspace": func() error {
			_, err := svc.ListPackageVersionFiles(ctx, other, packageID, versionID)
			return err
		},
		"read from another workspace": func() error {
			_, err := svc.PackageVersionFile(ctx, other, packageID, versionID, "ui/main.js")
			return err
		},
		"unknown package": func() error {
			_, err := svc.ListPackageVersionFiles(ctx, own, "00000000-0000-0000-0000-000000000000", versionID)
			return err
		},
		"malformed package id": func() error {
			_, err := svc.ListPackageVersionFiles(ctx, own, "nope", versionID)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := call()
			var pluginErr *PluginError
			if !errors.As(err, &pluginErr) || pluginErr.Kind != PluginErrorNotFound {
				t.Fatalf("err=%v want PluginErrorNotFound", err)
			}
		})
	}
}

func TestManifestVirtualFileRendersIndentedJSON(t *testing.T) {
	file, err := manifestVirtualFile(db.PluginPackageVersion{Manifest: []byte(`{"a":1}`)})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	const want = "{\n  \"a\": 1\n}\n"
	if file.Path != "multica.plugin.json" || string(file.Content) != want || file.SizeBytes != int64(len(want)) {
		t.Fatalf("virtual file = path %q content %q size %d", file.Path, file.Content, file.SizeBytes)
	}
	sum := sha256.Sum256([]byte(want))
	if file.Sha256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("digest %s does not match the rendered bytes", file.Sha256)
	}
}

func pluginFilesTestUUID(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	parsed, err := util.ParseUUID(value)
	if err != nil {
		t.Fatalf("parse uuid %q: %v", value, err)
	}
	return parsed
}
