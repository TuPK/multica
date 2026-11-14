package pack

import (
	"path"
	"regexp"
	"strings"

	"github.com/multica-ai/multica/server/internal/hub"
)

var (
	// agentKeyPattern and skillNamePattern are the rules of spec 16.1.
	agentKeyPattern  = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)
	skillNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
)

// Validate checks a bundle and the Pack read from it, and returns every problem it finds, not only the
// first. An error is a reason not to import the bundle as it is. Paths name elements by key, not by
// position (agents[researcher].skills[foo]). It does not open the text of any instruction or skill.
func Validate(b *hub.Bundle, p *Pack) []Issue {
	v := &validator{b: b, p: p}
	v.bundle()
	v.agents()
	v.skills()
	if p.Format == FormatNativeBundle {
		v.native()
	}
	v.references()
	return v.issues
}

type validator struct {
	b      *hub.Bundle
	p      *Pack
	issues []Issue
}

func (v *validator) add(severity, code, at, message string, params map[string]any) {
	v.issues = append(v.issues, Issue{Code: code, Severity: severity, Path: at, Message: message, Params: params})
}

func (v *validator) err(code, at, message string, params map[string]any) {
	v.add(SeverityError, code, at, message, params)
}

// bundle checks the size limits of hub.DefaultLimits and that every path stays inside the bundle.
func (v *validator) bundle() {
	lim := hub.DefaultLimits
	if len(v.b.Files) > lim.MaxFiles {
		v.err("pack_too_large", "files", "the bundle has more files than the limit",
			map[string]any{"files": len(v.b.Files), "limit": lim.MaxFiles})
	}
	var total int64
	for _, f := range v.b.Files {
		at := "files[" + f.Path + "]"
		if !safePath(f.Path) {
			v.err("pack_path_unsafe", at, "the path leaves the bundle or is not a plain relative path", nil)
		}
		total += int64(len(f.Content))
		if int64(len(f.Content)) > lim.MaxFileBytes {
			v.err("pack_too_large", at, "the file is larger than the limit",
				map[string]any{"bytes": len(f.Content), "limit": lim.MaxFileBytes})
		}
	}
	if total > lim.MaxTotalBytes {
		v.err("pack_too_large", "size", "the bundle is larger than the limit",
			map[string]any{"bytes": total, "limit": lim.MaxTotalBytes})
	}
}

// agentLabel names an agent in a path: its key, or the file it came from when it has none.
func agentLabel(a Agent) string {
	if a.Key != "" {
		return a.Key
	}
	return strings.TrimSuffix(path.Base(a.Path), path.Ext(a.Path))
}

func (v *validator) agents() {
	skills := map[string]bool{}
	for _, s := range v.p.Skills {
		skills[s.Name] = true
	}
	count := map[string]int{}
	for _, a := range v.p.Agents {
		count[a.Key]++
	}
	reported := map[string]bool{}
	for _, a := range v.p.Agents {
		label := "agents[" + agentLabel(a) + "]"
		if a.Name == "" {
			v.err("pack_field_missing", label+".name", "the agent has no name", map[string]any{"field": "name"})
		}
		if a.Description == "" {
			v.err("pack_field_missing", label+".description", "the agent has no description", map[string]any{"field": "description"})
		}
		if a.Key != "" && !agentKeyPattern.MatchString(a.Key) {
			v.err("pack_agent_key_invalid", label, "the key is not 2 to 32 characters of a-z, 0-9 and _, starting with a letter", map[string]any{"key": a.Key})
		}
		if a.Key != "" && count[a.Key] > 1 && !reported[a.Key] {
			reported[a.Key] = true
			v.err("pack_agent_key_duplicate", label, "two agents have this key", map[string]any{"key": a.Key})
		}
		v.instructions(a, label)
		for _, name := range a.Skills {
			if !skills[name] {
				v.err("pack_skill_missing", label+".skills["+name+"]", "the agent asks for a skill that the bundle does not have", map[string]any{"skill": name})
			}
		}
	}
}

// instructions checks where an agent's instructions came from. A folder of agents reads them from the
// agent's own file, which is there by construction; the other formats name a file.
func (v *validator) instructions(a Agent, label string) {
	if v.p.Format == FormatClaudeAgents {
		return
	}
	switch {
	case a.Path == "":
		v.err("pack_instructions_missing", label+".instructions", "the agent names no instructions file", nil)
	case !safePath(a.Path):
		v.err("pack_path_unsafe", label+".instructions", "the instructions path leaves the bundle", map[string]any{"path": a.Path})
	case !hasFile(v.b, a.Path):
		v.err("pack_instructions_missing", label+".instructions", "the instructions file is not in the bundle", map[string]any{"path": a.Path})
	}
}

func (v *validator) skills() {
	count := map[string]int{}
	for _, s := range v.p.Skills {
		count[s.Name]++
	}
	reported := map[string]bool{}
	for _, s := range v.p.Skills {
		label := s.Name
		if label == "" {
			label = path.Base(s.Dir)
		}
		at := "skills[" + label + "]"
		if s.Name == "" {
			v.err("pack_field_missing", at+".name", "the skill has no name", map[string]any{"field": "name"})
		} else if !skillNamePattern.MatchString(s.Name) {
			v.err("pack_skill_name_invalid", at, "a skill name is lower case letters, digits, . _ -, starting with a letter or a digit", map[string]any{"name": s.Name})
		}
		if s.Description == "" {
			v.err("pack_field_missing", at+".description", "the skill has no description", map[string]any{"field": "description"})
		}
		if s.Name != "" && count[s.Name] > 1 && !reported[s.Name] {
			reported[s.Name] = true
			v.err("pack_skill_duplicate", at, "two skills have this name", map[string]any{"name": s.Name})
		}
	}
}

// native checks what only a native bundle declares: its lead, a member besides the lead, and its flow.
func (v *validator) native() {
	leader := v.p.Leader
	known := false
	others := 0
	for _, a := range v.p.Agents {
		if a.Key == leader && leader != "" {
			known = true
		} else {
			others++
		}
	}
	if !known {
		v.err("pack_leader_invalid", "leader", "the leader is empty or is not the key of an agent of the bundle", map[string]any{"leader": leader})
	}
	if others == 0 {
		v.err("pack_no_member_agent", "agents", "the bundle needs at least one agent besides its leader", nil)
	}
	switch flow := v.p.Flow; {
	case flow == "":
	case !safePath(flow):
		v.err("pack_path_unsafe", "flow", "the flow path leaves the bundle", map[string]any{"path": flow})
	case !hasFile(v.b, flow):
		v.err("pack_flow_missing", "flow", "the flow file is not in the bundle", map[string]any{"path": flow})
	}
}

// references says when more files may hold a procedure than preview lists.
func (v *validator) references() {
	if n := len(listReferences(v.b, v.p)); n > maxReferences {
		v.add(SeverityInfo, "pack_references_capped", "references", "only the first files that may hold a procedure are listed",
			map[string]any{"files": n, "limit": maxReferences})
	}
}
