// Package pack recognises, reads and checks a bundle of skills and agents that a
// person brings to Multica (spec 16.1). It is pure: it reads a hub.Bundle that is
// already unpacked and knows nothing of handlers or storage. It never reads the
// text of an instruction or a skill to guess a role, a step or a lead; it only
// lists the markdown files that may hold a procedure (spec 16.1, references).
package pack

import (
	"errors"
)

// Format is how a bundle was recognised.
type Format string

const (
	// FormatNativeBundle is squad-bundle.json at the root (spec appendix E.1).
	FormatNativeBundle Format = "native_bundle"
	// FormatMulticaPack is pack.json at the root: an agent pack of Multica.
	FormatMulticaPack Format = "multica_pack"
	// FormatClaudeAgents is an agents/ folder of markdown files with frontmatter.
	FormatClaudeAgents Format = "claude_agents"
	// FormatSkillsOnly is one or more folders that hold a SKILL.md.
	FormatSkillsOnly Format = "skills_only"
)

// ErrInvalid is wrapped by every refusal of a whole bundle (spec code pack_invalid).
var ErrInvalid = errors.New("pack_invalid")

// Pack is a bundle in the one shape the rest of the import works on.
type Pack struct {
	// ID and Version are what the bundle says of itself; empty when its format has no such field.
	ID      string
	Name    string
	Version string
	Format  Format
	Agents  []Agent
	Skills  []Skill
	// Leader is the key of the lead agent. Only a native bundle declares one.
	Leader string
	// Flow is the bundle path of flow.json, empty when the bundle has none.
	Flow       string
	References []Reference
	// ReferencesOmitted is how many doc references the cap left out of References (spec 16.1).
	ReferencesOmitted int
	Ignored           []Ignored
}

// Agent is one agent of a bundle.
type Agent struct {
	// Key is made from the name (spec 16.1); a native bundle declares it.
	Key         string
	Name        string
	Description string
	// RoleLabel is the label of the agent's place in a squad (native bundle only).
	RoleLabel    string
	Instructions string
	// Path is the bundle path the instructions were read from.
	Path string
	// Skills are the skill names the agent asks for. SkillsDeclared tells "asks for none" from "does not say":
	// when it does not, the person who imports the bundle chooses.
	Skills             []string
	SkillsDeclared     bool
	MaxConcurrentTasks int
}

// Skill is one skill of a bundle: the folder that holds its SKILL.md.
type Skill struct {
	Name        string
	Description string
	// Dir is the bundle folder of the skill; empty when SKILL.md is at the root.
	Dir string
	// Files are the bundle paths of every file of the skill, SKILL.md included. Their content stays in the bundle.
	Files []string
}

// Reference is a markdown file that may hold a procedure. Only its path and size are kept.
type Reference struct {
	Path  string
	Bytes int
	// Kind is "agent", "skill" or "doc".
	Kind string
}

// Ignored is a setting of the bundle that is not applied: the person who imports it configures the agent.
type Ignored struct {
	Agent string `json:"agent,omitempty"`
	Skill string `json:"skill,omitempty"`
	Key   string `json:"key"`
}

// Severities of an Issue, as in the flow validator.
const (
	SeverityError = "error"
	SeverityInfo  = "info"
)

// Issue is one problem found in a bundle. It has the shape of the issues of the flow validator.
type Issue struct {
	Code     string         `json:"code"`
	Severity string         `json:"severity"`
	Path     string         `json:"path,omitempty"`
	Message  string         `json:"message"`
	Params   map[string]any `json:"params,omitempty"`
}
