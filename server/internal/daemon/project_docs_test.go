package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func writeDocFile(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCollectProjectDocsReadsOnlyProjectKnowledge(t *testing.T) {
	root := t.TempDir()
	writeDocFile(t, root, "README.md", "# Shop")
	writeDocFile(t, root, "docs/prd.md", "# PRD")
	writeDocFile(t, root, "docs/sdlc/plan.md", "# Plan")
	writeDocFile(t, root, "docs/notes.txt", "not markdown")
	writeDocFile(t, root, "docs/empty.md", "  \n")
	writeDocFile(t, root, "docs/.hidden/secret.md", "# hidden")
	writeDocFile(t, root, "docs/node_modules/x.md", "# dep")
	writeDocFile(t, root, ".agents/skills/a/SKILL.md", "# tooling")
	writeDocFile(t, root, "src/readme.md", "# source tree")
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("# outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "docs", "link.md")); err != nil {
		t.Fatal(err)
	}

	files, err := collectProjectDocs(root)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range files {
		got = append(got, f.Path)
		if len(f.SHA256) != 64 || f.Content == "" {
			t.Fatalf("incomplete file %+v", f)
		}
	}
	want := []string{"README.md", "docs/prd.md", "docs/sdlc/plan.md"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestCollectProjectDocsFailsWithoutMarkdown(t *testing.T) {
	if _, err := collectProjectDocs(t.TempDir()); err == nil {
		t.Fatal("an empty directory must be an error, so a failed scan never empties a base")
	}
	if _, err := collectProjectDocs(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("a missing directory must be an error")
	}
}

func TestOwnsProjectSourceMatchesCurrentAndLegacyIDs(t *testing.T) {
	d := &Daemon{cfg: Config{DaemonID: "new", LegacyDaemonIDs: []string{"old"}}}
	for id, want := range map[string]bool{"new": true, "old": true, "other": false, "": false} {
		if got := d.ownsProjectSource(projectDocSource{DaemonID: id, LocalPath: "/p"}); got != want {
			t.Fatalf("daemon id %q: got %v, want %v", id, got, want)
		}
	}
	if d.ownsProjectSource(projectDocSource{DaemonID: "new"}) {
		t.Fatal("a source without a path is not ours")
	}
}
