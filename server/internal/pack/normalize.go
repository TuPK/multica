package pack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/multica-ai/multica/server/internal/hub"
)

const (
	// nativeFormat is the value of "format" in squad-bundle.json.
	nativeFormat = "multica.squad-bundle/v1"
	// maxReferences bounds the doc files that preview lists, and maxReferenceBytes the size of each file listed
	// (spec 16.1). The instructions of an agent and a SKILL.md are never cut: they may hold the procedure.
	maxReferences     = 20
	maxReferenceBytes = 200 << 10
)

// settingKeys are the settings of an agent or a skill that a bundle may carry and Multica does not apply
// (spec 16.1). They are listed, in this order, so that the person who imports configures the agent.
// Any other frontmatter key is dropped without a word.
var settingKeys = []string{"tools", "model", "allowed-tools", "env", "mcpServers"}

// Normalize reads a bundle into a Pack, whatever its format. It refuses with ErrInvalid a bundle whose
// format cannot be told, or whose manifest cannot be read. What is wrong with the contents (a missing
// description, a duplicate key, a skill that is not there) is left in the Pack for Validate to report: a
// bundle with such a fault can still be shown to the person who imports it. Normalize never reads the
// text of an instruction or a skill to guess anything.
func Normalize(b *hub.Bundle) (*Pack, error) {
	format, err := Detect(b)
	if err != nil {
		return nil, err
	}
	p := &Pack{Format: format}
	switch format {
	case FormatNativeBundle:
		err = normalizeNative(b, p)
	case FormatMulticaPack:
		err = normalizeMulticaPack(b, p)
	case FormatClaudeAgents:
		normalizeClaudeAgents(b, p)
	case FormatSkillsOnly:
		for _, f := range skillFiles(b, maxSkillDepth) {
			p.Skills = append(p.Skills, readSkill(b, f, p))
		}
	}
	if err != nil {
		return nil, err
	}
	sort.SliceStable(p.Skills, func(i, j int) bool {
		if p.Skills[i].Name != p.Skills[j].Name {
			return p.Skills[i].Name < p.Skills[j].Name
		}
		return p.Skills[i].Dir < p.Skills[j].Dir
	})
	p.References, p.ReferencesOmitted = capDocReferences(listReferences(b, p))
	return p, nil
}

// AgentKey makes the key of an agent from its name (spec 16.1): lower case, every other character an
// underscore. The result may not be a valid key; Validate says so.
func AgentKey(name string) string {
	var sb strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			sb.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			sb.WriteRune(r + 'a' - 'A')
		default:
			sb.WriteByte('_')
		}
	}
	return sb.String()
}

// normalizeClaudeAgents reads agents/*.md and skills/*/SKILL.md.
func normalizeClaudeAgents(b *hub.Bundle, p *Pack) {
	for _, f := range b.Files {
		dir, file := path.Split(f.Path)
		if dir != "agents/" || !strings.HasSuffix(file, ".md") {
			continue
		}
		fm := parseFrontmatter(f.Content)
		if !fm.has {
			continue
		}
		a := Agent{
			Name:         text(fm.values["name"]),
			Description:  text(fm.values["description"]),
			Instructions: strings.TrimSpace(fm.body),
			Path:         f.Path,
		}
		a.Key = AgentKey(a.Name)
		a.Skills, a.SkillsDeclared = nameList(fm.values["skills"])
		p.Ignored = append(p.Ignored, ignoredSettings(fm.values, func(key string) Ignored { return Ignored{Agent: a.Key, Key: key} })...)
		p.Agents = append(p.Agents, a)
	}
	readSkillsFolder(b, p)
	if m, ok := fileAt(b, ".claude-plugin/plugin.json"); ok {
		var plugin struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}
		if json.Unmarshal(m.Content, &plugin) == nil {
			p.ID, p.Name, p.Version = plugin.Name, plugin.Name, plugin.Version
		}
	}
}

// readSkillsFolder reads skills/<name>/SKILL.md, the skills of a bundle that has agents.
func readSkillsFolder(b *hub.Bundle, p *Pack) {
	for _, f := range skillFiles(b, 2) {
		if strings.HasPrefix(f.Path, "skills/") && strings.Count(f.Path, "/") == 2 {
			p.Skills = append(p.Skills, readSkill(b, f, p))
		}
	}
}

// nativeManifestFile is squad-bundle.json (spec appendix E.1).
type nativeManifestFile struct {
	Format  string `json:"format"`
	ID      string `json:"id"`
	Version string `json:"version"`
	Name    string `json:"name"`
	Leader  string `json:"leader"`
	Agents  []struct {
		Key                string    `json:"key"`
		Name               string    `json:"name"`
		RoleLabel          string    `json:"role_label"`
		Description        string    `json:"description"`
		Instructions       string    `json:"instructions"`
		Skills             *[]string `json:"skills"`
		MaxConcurrentTasks int       `json:"max_concurrent_tasks"`
	} `json:"agents"`
	Flow string `json:"flow"`
}

func normalizeNative(b *hub.Bundle, p *Pack) error {
	var m nativeManifestFile
	if err := readJSON(b, nativeManifest, &m); err != nil {
		return err
	}
	if m.Format != nativeFormat {
		return fmt.Errorf("%w: %s says format %q, want %q", ErrInvalid, nativeManifest, m.Format, nativeFormat)
	}
	p.ID, p.Version, p.Name, p.Leader, p.Flow = m.ID, m.Version, m.Name, m.Leader, m.Flow
	for _, e := range m.Agents {
		a := Agent{
			Key: e.Key, Name: e.Name, Description: e.Description, RoleLabel: e.RoleLabel,
			Path: e.Instructions, MaxConcurrentTasks: e.MaxConcurrentTasks,
		}
		if e.Skills != nil {
			a.Skills, a.SkillsDeclared = *e.Skills, true
		}
		a.Instructions = readInstructions(b, e.Instructions)
		p.Agents = append(p.Agents, a)
	}
	readSkillsFolder(b, p)
	return nil
}

// packFile is pack.json of an agent pack of Multica (catalog/packs/<id>/).
type packFile struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Agent   struct {
		Name               string `json:"name"`
		Description        string `json:"description"`
		InstructionsFile   string `json:"instructions_file"`
		RuntimeProvider    string `json:"runtime_provider"`
		MaxConcurrentTasks int    `json:"max_concurrent_tasks"`
	} `json:"agent"`
	Skills []string `json:"skills"`
}

func normalizeMulticaPack(b *hub.Bundle, p *Pack) error {
	var m packFile
	if err := readJSON(b, packManifest, &m); err != nil {
		return err
	}
	// template.json of the catalog names the pack; pack.json may say so itself.
	var template struct {
		ID      string `json:"id"`
		Version string `json:"version"`
	}
	if f, ok := fileAt(b, "template.json"); ok {
		_ = json.Unmarshal(f.Content, &template)
	}
	p.ID, p.Version = firstNonEmpty(m.ID, template.ID), firstNonEmpty(m.Version, template.Version)
	p.Name = m.Agent.Name
	file := firstNonEmpty(m.Agent.InstructionsFile, "instructions.md")
	a := Agent{
		Key: AgentKey(m.Agent.Name), Name: m.Agent.Name, Description: m.Agent.Description,
		Path: file, Instructions: readInstructions(b, file), MaxConcurrentTasks: m.Agent.MaxConcurrentTasks,
		Skills: m.Skills, SkillsDeclared: true,
	}
	if m.Agent.RuntimeProvider != "" {
		p.Ignored = append(p.Ignored, Ignored{Agent: a.Key, Key: "runtime_provider"})
	}
	p.Agents = append(p.Agents, a)
	for _, name := range m.Skills {
		if f, ok := fileAt(b, "skills/"+name+"/SKILL.md"); ok && safePath(name) {
			p.Skills = append(p.Skills, readSkill(b, f, p))
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// readJSON reads a manifest of the bundle. A manifest that is not there or is not JSON refuses the bundle.
func readJSON(b *hub.Bundle, name string, into any) error {
	f, ok := fileAt(b, name)
	if !ok {
		return fmt.Errorf("%w: %s is missing", ErrInvalid, name)
	}
	if err := json.NewDecoder(bytes.NewReader(f.Content)).Decode(into); err != nil {
		return fmt.Errorf("%w: %s is not valid: %v", ErrInvalid, name, err)
	}
	return nil
}

// readInstructions is the text of an instructions file, or empty when its path is unsafe or the file is
// not there; Validate reports both.
func readInstructions(b *hub.Bundle, p string) string {
	if !safePath(p) {
		return ""
	}
	f, ok := fileAt(b, p)
	if !ok {
		return ""
	}
	return strings.TrimSpace(string(f.Content))
}

// readSkill reads the SKILL.md f and lists the files of its folder.
func readSkill(b *hub.Bundle, f hub.FileEntry, p *Pack) Skill {
	fm := parseFrontmatter(f.Content)
	dir := path.Dir(f.Path)
	if dir == "." {
		dir = ""
	}
	s := Skill{Name: text(fm.values["name"]), Description: text(fm.values["description"]), Dir: dir}
	for _, other := range b.Files {
		if dir == "" || strings.HasPrefix(other.Path, dir+"/") {
			s.Files = append(s.Files, other.Path)
		}
	}
	p.Ignored = append(p.Ignored, ignoredSettings(fm.values, func(key string) Ignored { return Ignored{Skill: s.Name, Key: key} })...)
	return s
}

func ignoredSettings(values map[string]any, make func(key string) Ignored) []Ignored {
	var out []Ignored
	for _, key := range settingKeys {
		if _, ok := values[key]; ok {
			out = append(out, make(key))
		}
	}
	return out
}

// nameList reads the skills of an agent: a list, or one line of names separated by commas.
func nameList(v any) (names []string, declared bool) {
	var parts []string
	switch x := v.(type) {
	case []any:
		for _, item := range x {
			if s, ok := item.(string); ok {
				parts = append(parts, s)
			}
		}
	case string:
		parts = strings.Split(x, ",")
	default:
		return nil, false
	}
	for _, s := range parts {
		if s = strings.TrimSpace(s); s != "" {
			names = append(names, s)
		}
	}
	return names, true
}

// safePath reports whether a path stays inside a bundle: relative, with no "..", no backslash and no
// control character (the rules hub.ExtractZip applies to an archive).
func safePath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.ContainsAny(p, "\\:") {
		return false
	}
	for _, r := range p {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

// capDocReferences keeps every agent and skill reference and the first maxReferences doc references, and says
// how many doc references it left out. The list is in the order of listReferences, so the docs come last.
func capDocReferences(refs []Reference) ([]Reference, int) {
	out := make([]Reference, 0, len(refs))
	docs := 0
	for _, r := range refs {
		if r.Kind == "doc" {
			if docs++; docs > maxReferences {
				continue
			}
		}
		out = append(out, r)
	}
	return out, max(docs-maxReferences, 0)
}

// listReferences lists, in the order agent, skill, doc and then by path, the markdown files that may hold a
// procedure (spec 16.1): the instructions of each agent, each SKILL.md, README.md, and the .md files of
// docs/ and workflows/. It gives path and size only. Files over 200 KiB are left out. The docs are not cut at
// 20 here; capDocReferences does that.
func listReferences(b *hub.Bundle, p *Pack) []Reference {
	var agents, skills, docs []Reference
	add := func(into *[]Reference, f hub.FileEntry, kind string) {
		if len(f.Content) <= maxReferenceBytes && safePath(f.Path) {
			*into = append(*into, Reference{Path: f.Path, Bytes: len(f.Content), Kind: kind})
		}
	}
	listed := map[string]bool{}
	for _, a := range p.Agents {
		if f, ok := fileAt(b, a.Path); ok && !listed[f.Path] {
			listed[f.Path] = true
			add(&agents, f, "agent")
		}
	}
	for _, f := range b.Files {
		base := path.Base(f.Path)
		switch {
		case strings.EqualFold(base, "SKILL.md") && isSkillOf(p, f.Path):
			add(&skills, f, "skill")
		case !strings.HasSuffix(f.Path, ".md") || listed[f.Path]:
		case f.Path == "README.md", strings.HasPrefix(f.Path, "docs/"), strings.HasPrefix(f.Path, "workflows/"):
			add(&docs, f, "doc")
		}
	}
	for _, list := range [][]Reference{agents, skills, docs} {
		sort.Slice(list, func(i, j int) bool { return list[i].Path < list[j].Path })
	}
	return append(append(agents, skills...), docs...)
}

func isSkillOf(p *Pack, filePath string) bool {
	for _, s := range p.Skills {
		if path.Dir(filePath) == s.Dir || (s.Dir == "" && path.Dir(filePath) == ".") {
			return true
		}
	}
	return false
}
