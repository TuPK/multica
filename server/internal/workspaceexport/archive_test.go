package workspaceexport

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

const (
	wsA = "11111111-1111-1111-1111-111111111111"
	wsB = "22222222-2222-2222-2222-222222222222"
)

// memSource serves rows from memory, including rows of other workspaces, the
// way a careless query would.
type memSource struct{ rows map[string][]Row }

func (m memSource) Rows(_ context.Context, table, _ string, emit func(Row) error) error {
	for _, r := range m.rows[table] {
		if err := emit(r); err != nil {
			return err
		}
	}
	return nil
}

// Planted values that must never reach the archive.
var secrets = []string{"SECRET-custom-env-value", "SECRET-mcp-config-value", "SECRET-token-value", "SECRET-settings-value"}

func fixture() memSource {
	leak := func(r Row) Row {
		r["custom_env"] = secrets[0]
		r["mcp_config"] = secrets[1]
		r["token"] = secrets[2]
		r["settings"] = secrets[3]
		return r
	}
	return memSource{rows: map[string][]Row{
		"workspace": {
			leak(Row{"id": wsA, "name": "Alpha", "slug": "alpha"}),
			leak(Row{"id": wsB, "name": "Beta", "slug": "beta"}),
		},
		"members": {
			leak(Row{"id": "m1", "workspace_id": wsA, "user_id": "u1", "role": "owner", "email": "a@example.com"}),
			leak(Row{"id": "m2", "workspace_id": wsB, "user_id": "u2", "role": "owner", "email": "OTHER-WORKSPACE-MEMBER"}),
		},
		"projects": {leak(Row{"id": "p1", "workspace_id": wsA, "title": "P"})},
		"issues": {
			leak(Row{"id": "i1", "workspace_id": wsA, "title": "one", "description": "line1\nline2"}),
			leak(Row{"id": "i2", "workspace_id": wsA, "title": "two"}),
			leak(Row{"id": "i3", "workspace_id": wsB, "title": "OTHER-WORKSPACE-ISSUE"}),
		},
		"comments":    {leak(Row{"id": "c1", "workspace_id": wsA, "issue_id": "i1", "content": "hello"})},
		"labels":      {leak(Row{"id": "l1", "workspace_id": wsA, "name": "bug"})},
		"statuses":    {leak(Row{"id": "s1", "workspace_id": wsA, "key": "todo", "name": "Todo"})},
		"attachments": {leak(Row{"id": "a1", "workspace_id": wsA, "issue_id": "i1", "name": "f.png", "size_bytes": 12, "content_type": "image/png", "url": "https://bucket/secret-path"})},
	}}
}

func readZip(t *testing.T, data []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("not a zip: %v", err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		out[f.Name] = string(b)
	}
	return out
}

func build(t *testing.T) (map[string]string, Manifest) {
	t.Helper()
	data, manifest, err := BuildArchive(context.Background(), fixture(), wsA, time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return readZip(t, data), manifest
}

func TestArchiveHasEveryFileAndEachLineIsAJSONObject(t *testing.T) {
	files, _ := build(t)
	for _, name := range []string{"workspace.ndjson", "members.ndjson", "projects.ndjson", "issues.ndjson", "comments.ndjson", "labels.ndjson", "statuses.ndjson", "attachments.ndjson", "manifest.json"} {
		if _, ok := files[name]; !ok {
			t.Errorf("archive has no %s", name)
		}
	}
	if len(files) != 9 {
		t.Errorf("archive has %d files, want 9", len(files))
	}
	for name, body := range files {
		if !strings.HasSuffix(name, ".ndjson") {
			continue
		}
		sc := bufio.NewScanner(strings.NewReader(body))
		for sc.Scan() {
			var obj map[string]any
			if err := json.Unmarshal(sc.Bytes(), &obj); err != nil {
				t.Errorf("%s: line %q is not a JSON object: %v", name, sc.Text(), err)
			}
		}
	}
	// A description with a newline stays on one line.
	if lines := strings.Count(strings.TrimSpace(files["issues.ndjson"]), "\n") + 1; lines != 2 {
		t.Errorf("issues.ndjson has %d lines, want 2", lines)
	}
}

func TestArchiveContainsNoPlantedSecret(t *testing.T) {
	files, _ := build(t)
	for name, body := range files {
		for _, secret := range secrets {
			if strings.Contains(body, secret) {
				t.Errorf("%s contains the planted secret %q", name, secret)
			}
		}
		for _, key := range []string{"custom_env", "mcp_config", "\"token\"", "secret-path"} {
			if strings.Contains(body, key) {
				t.Errorf("%s contains %q, which is not on an allowlist", name, key)
			}
		}
	}
}

func TestArchiveSkipsRowsOfAnotherWorkspace(t *testing.T) {
	files, _ := build(t)
	for name, body := range files {
		for _, other := range []string{wsB, "OTHER-WORKSPACE", "Beta"} {
			if strings.Contains(body, other) {
				t.Errorf("%s contains %q from another workspace", name, other)
			}
		}
	}
}

func TestManifestCarriesTheRowCountsAndTheExportTime(t *testing.T) {
	files, manifest := build(t)
	want := map[string]int{"workspace": 1, "members": 1, "projects": 1, "issues": 2, "comments": 1, "labels": 1, "statuses": 1, "attachments": 1}
	for table, n := range want {
		if manifest.Counts[table] != n {
			t.Errorf("Counts[%s] = %d, want %d", table, manifest.Counts[table], n)
		}
	}
	var fromFile Manifest
	if err := json.Unmarshal([]byte(files["manifest.json"]), &fromFile); err != nil {
		t.Fatal(err)
	}
	if fromFile.WorkspaceID != wsA {
		t.Errorf("manifest workspace_id = %q", fromFile.WorkspaceID)
	}
	if !fromFile.ExportedAt.Equal(time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("manifest exported_at = %v", fromFile.ExportedAt)
	}
	for table, n := range want {
		if fromFile.Counts[table] != n {
			t.Errorf("manifest.json counts[%s] = %d, want %d", table, fromFile.Counts[table], n)
		}
		lines := 0
		if body := strings.TrimSpace(files[table+".ndjson"]); body != "" {
			lines = strings.Count(body, "\n") + 1
		}
		if lines != n {
			t.Errorf("%s.ndjson has %d lines, manifest says %d", table, lines, n)
		}
	}
}

func TestArchiveOfAnEmptyTableHasAnEmptyFile(t *testing.T) {
	src := fixture()
	src.rows["comments"] = nil
	data, manifest, err := BuildArchive(context.Background(), src, wsA, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if body, ok := readZip(t, data)["comments.ndjson"]; !ok || body != "" {
		t.Errorf("comments.ndjson = %q, present=%v; want present and empty", body, ok)
	}
	if manifest.Counts["comments"] != 0 {
		t.Errorf("Counts[comments] = %d", manifest.Counts["comments"])
	}
}
