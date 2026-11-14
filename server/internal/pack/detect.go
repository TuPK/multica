package pack

import (
	"fmt"
	"path"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/multica-ai/multica/server/internal/hub"
)

const (
	nativeManifest = "squad-bundle.json"
	packManifest   = "pack.json"
	// maxSkillDepth is how many folders down a SKILL.md may sit in a bundle of skills only.
	maxSkillDepth = 3
)

// Detect says which format a bundle is, by the fixed order of spec 16.1: squad-bundle.json, pack.json,
// agents/*.md with frontmatter, then folders with a SKILL.md. A bundle with two of the first three signs
// at its root, or with none of the four, is refused with ErrInvalid. hub.Detect is not used: it only knows
// a skill or a plugin.
func Detect(b *hub.Bundle) (Format, error) {
	var signs []Format
	if hasFile(b, nativeManifest) {
		signs = append(signs, FormatNativeBundle)
	}
	if hasFile(b, packManifest) {
		signs = append(signs, FormatMulticaPack)
	}
	if len(agentFiles(b)) > 0 {
		signs = append(signs, FormatClaudeAgents)
	}
	switch {
	case len(signs) > 1:
		return "", fmt.Errorf("%w: the root has more than one sign of a format (%s)", ErrInvalid, joinFormats(signs))
	case len(signs) == 1:
		return signs[0], nil
	case len(skillFiles(b, maxSkillDepth)) > 0:
		return FormatSkillsOnly, nil
	}
	return "", fmt.Errorf("%w: no %s, %s, agents/*.md with a name and a description, or SKILL.md up to %d folders down", ErrInvalid, nativeManifest, packManifest, maxSkillDepth)
}

func joinFormats(fs []Format) string {
	names := make([]string, len(fs))
	for i, f := range fs {
		names[i] = string(f)
	}
	return strings.Join(names, ", ")
}

func hasFile(b *hub.Bundle, p string) bool {
	_, ok := fileAt(b, p)
	return ok
}

func fileAt(b *hub.Bundle, p string) (hub.FileEntry, bool) {
	for _, f := range b.Files {
		if f.Path == p {
			return f, true
		}
	}
	return hub.FileEntry{}, false
}

// agentFiles are the files agents/<name>.md that are agents: they have a frontmatter block. Detect asks for
// one whose block has a name and a description; a file without any block is a note, not an agent.
func agentFiles(b *hub.Bundle) []hub.FileEntry {
	var out []hub.FileEntry
	valid := false
	for _, f := range b.Files {
		dir, file := path.Split(f.Path)
		if dir != "agents/" || !strings.HasSuffix(file, ".md") {
			continue
		}
		fm := parseFrontmatter(f.Content)
		if !fm.has {
			continue
		}
		out = append(out, f)
		if text(fm.values["name"]) != "" && text(fm.values["description"]) != "" {
			valid = true
		}
	}
	if !valid {
		return nil
	}
	return out
}

// skillFiles are the SKILL.md files at most depth folders down.
func skillFiles(b *hub.Bundle, depth int) []hub.FileEntry {
	var out []hub.FileEntry
	for _, f := range b.Files {
		if strings.EqualFold(path.Base(f.Path), "SKILL.md") && strings.Count(f.Path, "/") <= depth {
			out = append(out, f)
		}
	}
	return out
}

// frontmatter is the block of YAML at the head of a markdown file, and what follows it.
type frontmatter struct {
	// has is true when the file starts with a block, even one that cannot be read.
	has    bool
	values map[string]any
	body   string
}

func parseFrontmatter(content []byte) frontmatter {
	s := strings.ReplaceAll(strings.TrimPrefix(string(content), "\ufeff"), "\r\n", "\n")
	rest, ok := strings.CutPrefix(s, "---\n")
	if !ok {
		return frontmatter{body: s}
	}
	block, body, closed := cutBlock(rest)
	if !closed {
		return frontmatter{body: s}
	}
	var values map[string]any
	if err := yaml.Unmarshal([]byte(block), &values); err != nil {
		values = nil
	}
	return frontmatter{has: true, values: values, body: body}
}

// cutBlock splits at the first line that is only "---".
func cutBlock(s string) (block, body string, closed bool) {
	if strings.HasPrefix(s, "---\n") || s == "---" {
		return "", strings.TrimPrefix(s, "---\n"), true
	}
	if i := strings.Index(s, "\n---\n"); i >= 0 {
		return s[:i], s[i+len("\n---\n"):], true
	}
	if strings.HasSuffix(s, "\n---") {
		return strings.TrimSuffix(s, "\n---"), "", true
	}
	return "", "", false
}

// text is a frontmatter value read as a one-line label; anything but a string is empty.
func text(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}
