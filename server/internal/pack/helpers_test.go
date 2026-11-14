package pack

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/hub"
)

// loadDir reads a folder of testdata as the bundle an archive would have given.
func loadDir(t *testing.T, name string) *hub.Bundle {
	t.Helper()
	root := filepath.Join("testdata", filepath.FromSlash(name))
	b := &hub.Bundle{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		content, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		b.Files = append(b.Files, hub.FileEntry{Path: filepath.ToSlash(rel), Size: int64(len(content)), Content: content})
		b.TotalSize += int64(len(content))
		return nil
	})
	if err != nil {
		t.Fatalf("load testdata/%s: %v", name, err)
	}
	if len(b.Files) == 0 {
		t.Fatalf("testdata/%s is empty", name)
	}
	sortFiles(b)
	return b
}

func sortFiles(b *hub.Bundle) {
	sort.Slice(b.Files, func(i, j int) bool { return b.Files[i].Path < b.Files[j].Path })
}

// edit returns a copy of the bundle with these files set (content "" removes none; use drop for that).
func edit(b *hub.Bundle, set map[string]string, drop ...string) *hub.Bundle {
	out := &hub.Bundle{}
	dropped := map[string]bool{}
	for _, d := range drop {
		dropped[d] = true
	}
	seen := map[string]bool{}
	for _, f := range b.Files {
		if dropped[f.Path] {
			continue
		}
		content := f.Content
		if s, ok := set[f.Path]; ok {
			content = []byte(s)
			seen[f.Path] = true
		}
		out.Files = append(out.Files, hub.FileEntry{Path: f.Path, Size: int64(len(content)), Content: content})
		out.TotalSize += int64(len(content))
	}
	for p, s := range set {
		if !seen[p] {
			out.Files = append(out.Files, hub.FileEntry{Path: p, Size: int64(len(s)), Content: []byte(s)})
			out.TotalSize += int64(len(s))
		}
	}
	sortFiles(out)
	return out
}

func fileText(t *testing.T, b *hub.Bundle, p string) string {
	t.Helper()
	for _, f := range b.Files {
		if f.Path == p {
			return string(f.Content)
		}
	}
	t.Fatalf("no file %s in the bundle", p)
	return ""
}

// frontmatterAndBody splits a markdown file made of a frontmatter block and a body.
func frontmatterAndBody(t *testing.T, text string) (front, body string) {
	t.Helper()
	rest, ok := strings.CutPrefix(text, "---\n")
	if !ok {
		t.Fatalf("no frontmatter: %.40q", text)
	}
	front, body, ok = strings.Cut(rest, "\n---\n")
	if !ok {
		t.Fatalf("frontmatter is not closed: %.40q", text)
	}
	return front, body
}

func mustNormalize(t *testing.T, b *hub.Bundle) *Pack {
	t.Helper()
	p, err := Normalize(b)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	return p
}

func agentByKey(t *testing.T, p *Pack, key string) Agent {
	t.Helper()
	for _, a := range p.Agents {
		if a.Key == key {
			return a
		}
	}
	t.Fatalf("no agent %q in %v", key, agentKeys(p))
	return Agent{}
}

func agentKeys(p *Pack) []string {
	keys := make([]string, len(p.Agents))
	for i, a := range p.Agents {
		keys[i] = a.Key
	}
	return keys
}

func skillByName(t *testing.T, p *Pack, name string) Skill {
	t.Helper()
	for _, s := range p.Skills {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("no skill %q", name)
	return Skill{}
}

func errorIssues(issues []Issue) []Issue {
	var out []Issue
	for _, i := range issues {
		if i.Severity == SeverityError {
			out = append(out, i)
		}
	}
	return out
}

// bundleCase is a testdata folder with some files added or replaced, and some dropped.
type bundleCase struct {
	dir  string
	set  map[string]string
	drop []string
}

func (c *bundleCase) bundle(t *testing.T) *hub.Bundle {
	t.Helper()
	b := loadDir(t, c.dir)
	if len(c.set) == 0 && len(c.drop) == 0 {
		return b
	}
	return edit(b, c.set, c.drop...)
}
