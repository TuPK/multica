package hub

import (
	"fmt"
	"path"
	"strings"

	"github.com/multica-ai/multica/server/internal/skill"
	"github.com/multica-ai/multica/server/pkg/plugincontract"
)

// Detect decides whether a bundle installs as a skill or a plugin.
//
// A skill has a SKILL.md (case-insensitive) at the bundle root, or the only
// one-level-deep SKILL.md when the bundle sits in a single directory. A plugin
// has the manifest named by plugincontract.ManifestFilename in the same places.
// A bundle with neither, or with both, is refused.
//
// Skill name and description come from SKILL.md frontmatter; the name falls back
// to the SKILL.md directory name. Plugin name is the manifest key, so it stays
// stable across display-name edits.
func Detect(b *Bundle) (Detected, error) {
	skillPath, hasSkill, err := findEntry(b, "SKILL.md", true)
	if err != nil {
		return Detected{}, err
	}
	manifestPath, hasPlugin, err := findEntry(b, plugincontract.ManifestFilename, false)
	if err != nil {
		return Detected{}, err
	}
	switch {
	case hasSkill && hasPlugin:
		return Detected{}, fmt.Errorf("%w: %s and %s both present", ErrAmbiguousKind, skillPath, manifestPath)
	case hasPlugin:
		return detectPlugin(b, manifestPath)
	case hasSkill:
		return detectSkill(b, skillPath)
	default:
		return Detected{}, ErrNotDetected
	}
}

func detectSkill(b *Bundle, p string) (Detected, error) {
	content, ok := b.fileContent(p)
	if !ok {
		return Detected{}, fmt.Errorf("%w: %s not readable", ErrInvalidSkill, p)
	}
	name, description := skill.ParseSkillFrontmatter(string(content))
	if name == "" {
		name = path.Base(path.Dir(p))
		if name == "." || name == "/" {
			return Detected{}, fmt.Errorf("%w: frontmatter name is required for a root SKILL.md", ErrInvalidSkill)
		}
	}
	return Detected{Kind: KindSkill, Name: name, Description: description, SkillMDPath: p}, nil
}

func detectPlugin(b *Bundle, p string) (Detected, error) {
	content, ok := b.fileContent(p)
	if !ok {
		return Detected{}, fmt.Errorf("%w: %s not readable", ErrInvalidManifest, p)
	}
	manifest, _, err := plugincontract.ParseManifest(content)
	if err != nil {
		return Detected{}, fmt.Errorf("%w: %v", ErrInvalidManifest, err)
	}
	return Detected{
		Kind:        KindPlugin,
		Name:        manifest.Key,
		Description: manifest.Description,
		Version:     manifest.Version,
	}, nil
}

// findEntry locates a file by base name at the root, or at one level below the
// root when no root file exists. Two matches at the same level are ambiguous.
// caseInsensitive selects EqualFold instead of exact matching.
func findEntry(b *Bundle, base string, caseInsensitive bool) (string, bool, error) {
	match := func(name string) bool {
		if caseInsensitive {
			return strings.EqualFold(name, base)
		}
		return name == base
	}

	var root, nested []string
	for _, f := range b.Files {
		dir, file := path.Split(f.Path)
		if !match(file) {
			continue
		}
		switch strings.Count(dir, "/") {
		case 0:
			root = append(root, f.Path)
		case 1:
			nested = append(nested, f.Path)
		}
	}
	level := root
	if len(level) == 0 {
		level = nested
	}
	switch len(level) {
	case 0:
		return "", false, nil
	case 1:
		return level[0], true, nil
	default:
		return "", false, fmt.Errorf("%w: more than one %s at the same level: %s", ErrInvalidArchive, base, strings.Join(level, ", "))
	}
}

func (b *Bundle) fileContent(p string) ([]byte, bool) {
	for _, f := range b.Files {
		if f.Path == p {
			return f.Content, true
		}
	}
	return nil, false
}
