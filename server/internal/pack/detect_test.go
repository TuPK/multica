package pack

import (
	"errors"
	"testing"
)

func TestDetect_OrderAndAmbiguity(t *testing.T) {
	for _, tc := range []struct {
		dir  string
		want Format
	}{
		{"native_bundle", FormatNativeBundle},
		{"multica_pack", FormatMulticaPack},
		{"claude_agents", FormatClaudeAgents},
		{"skills_only", FormatSkillsOnly},
	} {
		got, err := Detect(loadDir(t, tc.dir))
		if err != nil || got != tc.want {
			t.Errorf("%s: format %q, err %v; want %q", tc.dir, got, err, tc.want)
		}
	}

	// The order is fixed: a bundle that also has skills/ is read by its first sign.
	if got, _ := Detect(loadDir(t, "native_bundle")); got != FormatNativeBundle {
		t.Errorf("a native bundle with skills/ was read as %q", got)
	}
	if got, _ := Detect(loadDir(t, "claude_agents")); got != FormatClaudeAgents {
		t.Errorf("a folder of agents with skills/ was read as %q", got)
	}

	refused := map[string]*bundleCase{
		"two signs at the root: squad-bundle.json and pack.json": {dir: "broken/ambiguous_root"},
		"nothing a pack is made of":                              {dir: "broken/nothing"},
		"squad-bundle.json and agents/*.md": {dir: "native_bundle", set: map[string]string{
			"agents/extra.md": "---\nname: extra\ndescription: An agent next to a native bundle.\n---\n\nText.\n",
		}},
		"pack.json and agents/*.md": {dir: "multica_pack", set: map[string]string{
			"agents/extra.md": "---\nname: extra\ndescription: An agent next to a pack.\n---\n\nText.\n",
		}},
		"squad-bundle.json and pack.json, one more": {dir: "native_bundle", set: map[string]string{"pack.json": "{}"}},
		"a SKILL.md deeper than three folders": {dir: "broken/nothing", set: map[string]string{
			"a/b/c/d/SKILL.md": "---\nname: deep\ndescription: Too deep.\n---\n",
		}},
		"agents/*.md without a frontmatter block": {dir: "broken/nothing", set: map[string]string{
			"agents/notes.md": "# Notes\n\nNot an agent.\n",
		}},
	}
	for name, c := range refused {
		format, err := Detect(c.bundle(t))
		if !errors.Is(err, ErrInvalid) || format != "" {
			t.Errorf("%s: format %q, err %v; want pack_invalid", name, format, err)
		}
	}

	// agents/ files that are not agents do not hide the skills next to them.
	notes := &bundleCase{dir: "skills_only", set: map[string]string{"agents/notes.md": "# Notes\n"}}
	if got, err := Detect(notes.bundle(t)); err != nil || got != FormatSkillsOnly {
		t.Errorf("skills next to a notes file: %q, %v; want skills_only", got, err)
	}
	// A SKILL.md at the root is a one-skill bundle.
	root := &bundleCase{dir: "broken/nothing", set: map[string]string{"SKILL.md": "---\nname: solo\ndescription: One skill.\n---\n"}}
	if got, err := Detect(root.bundle(t)); err != nil || got != FormatSkillsOnly {
		t.Errorf("a root SKILL.md: %q, %v; want skills_only", got, err)
	}
	// .claude-plugin/plugin.json does not change the format.
	plugin := &bundleCase{dir: "claude_agents", set: map[string]string{".claude-plugin/plugin.json": `{"name":"desk","version":"1.0.0"}`}}
	if got, err := Detect(plugin.bundle(t)); err != nil || got != FormatClaudeAgents {
		t.Errorf("agents with a plugin manifest: %q, %v; want claude_agents", got, err)
	}
}
