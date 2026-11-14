package pack

import (
	"fmt"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/hub"
)

// overLimit builds a skills-only bundle that is valid except for its size.
func overLimit(files int, each int) *hub.Bundle {
	b := &hub.Bundle{}
	add := func(p string, content []byte) {
		b.Files = append(b.Files, hub.FileEntry{Path: p, Size: int64(len(content)), Content: content})
		b.TotalSize += int64(len(content))
	}
	add("only/SKILL.md", []byte("---\nname: only\ndescription: One skill.\n---\n"))
	for i := 0; i < files; i++ {
		add(fmt.Sprintf("only/data/f%04d.bin", i), make([]byte, each))
	}
	sortFiles(b)
	return b
}

func TestValidate_BrokenPacks(t *testing.T) {
	skills := &bundleCase{dir: "skills_only"}
	for _, tc := range []struct {
		name   string
		bundle *hub.Bundle
		code   string
		path   string
		params map[string]any
	}{
		{name: "a skill without a description", bundle: (&bundleCase{dir: "broken/skill_no_description"}).bundle(t),
			code: "pack_field_missing", path: "skills[only].description"},
		{name: "a skill without a name", bundle: (&bundleCase{dir: "broken/skill_no_name"}).bundle(t),
			code: "pack_field_missing", path: "skills[nameless].name"},
		{name: "a skill name that is not a name", bundle: (&bundleCase{dir: "broken/skill_bad_name"}).bundle(t),
			code: "pack_skill_name_invalid", path: "skills[Bad_Name]"},
		{name: "an agent without a description", bundle: (&bundleCase{dir: "broken/agent_no_description"}).bundle(t),
			code: "pack_field_missing", path: "agents[bad_agent].description"},
		{name: "an agent without a name", bundle: (&bundleCase{dir: "broken/agent_no_description", set: map[string]string{
			"agents/bad.md": "---\ndescription: Has no name.\n---\n\nText.\n",
		}}).bundle(t), code: "pack_field_missing", path: "agents[bad].name"},
		{name: "a skill an agent names that the bundle does not have", bundle: (&bundleCase{dir: "broken/agent_skill_missing"}).bundle(t),
			code: "pack_skill_missing", path: "agents[reader].skills[ghost]"},
		{name: "a skill a pack names that the bundle does not have", bundle: (&bundleCase{dir: "broken/missing_skill"}).bundle(t),
			code: "pack_skill_missing", path: "agents[reader].skills[ghost]"},
		{name: "two agents with one key", bundle: (&bundleCase{dir: "broken/duplicate_agent_key"}).bundle(t),
			code: "pack_agent_key_duplicate", path: "agents[a_b]"},
		{name: "a name that cannot be a key", bundle: (&bundleCase{dir: "broken/bad_agent_key"}).bundle(t),
			code: "pack_agent_key_invalid", path: "agents[9lives]"},
		{name: "a leader that points to no agent", bundle: (&bundleCase{dir: "broken/leader_missing"}).bundle(t),
			code: "pack_leader_invalid", path: "leader"},
		{name: "a leader that is empty", bundle: (&bundleCase{dir: "native_bundle", set: map[string]string{
			"squad-bundle.json": strings.Replace(fileOf(t, "native_bundle", "squad-bundle.json"), `"leader": "planner"`, `"leader": ""`, 1),
		}}).bundle(t), code: "pack_leader_invalid", path: "leader"},
		{name: "a native bundle with no agent besides its lead", bundle: (&bundleCase{dir: "native_bundle", set: map[string]string{
			"squad-bundle.json": `{"format":"multica.squad-bundle/v1","id":"x","version":"1","name":"X","leader":"planner","agents":[{"key":"planner","name":"Planner","description":"d","instructions":"agents/planner/instructions.md"}]}`,
		}}).bundle(t), code: "pack_no_member_agent", path: "agents"},
		{name: "an instructions path that leaves the bundle", bundle: (&bundleCase{dir: "broken/path_escape"}).bundle(t),
			code: "pack_path_unsafe", path: "agents[beta].instructions"},
		{name: "a flow path that leaves the bundle", bundle: (&bundleCase{dir: "broken/path_escape"}).bundle(t),
			code: "pack_path_unsafe", path: "flow"},
		{name: "a flow file that is not there", bundle: (&bundleCase{dir: "native_bundle", drop: []string{"flow.json"}}).bundle(t),
			code: "pack_flow_missing", path: "flow"},
		{name: "an instructions file that is not there", bundle: (&bundleCase{dir: "native_bundle", drop: []string{"agents/writer/instructions.md"}}).bundle(t),
			code: "pack_instructions_missing", path: "agents[writer].instructions"},
		{name: "a file whose path leaves the bundle", bundle: (&bundleCase{dir: "skills_only", set: map[string]string{"../outside.txt": "x"}}).bundle(t),
			code: "pack_path_unsafe", path: "files[../outside.txt]"},
		{name: "an absolute path", bundle: (&bundleCase{dir: "skills_only", set: map[string]string{"/etc/passwd": "x"}}).bundle(t),
			code: "pack_path_unsafe", path: "files[/etc/passwd]"},
		{name: "a path with a backslash", bundle: skills.withFile(t, `a\b/SKILL.md`, "x"),
			code: "pack_path_unsafe", path: `files[a\b/SKILL.md]`},
		{name: "more files than the limit", bundle: overLimit(hub.DefaultLimits.MaxFiles, 1),
			code: "pack_too_large", path: "files", params: map[string]any{"limit": hub.DefaultLimits.MaxFiles}},
		{name: "a file over the limit", bundle: overLimit(1, int(hub.DefaultLimits.MaxFileBytes)+1),
			code: "pack_too_large", path: "files[only/data/f0000.bin]", params: map[string]any{"limit": hub.DefaultLimits.MaxFileBytes}},
		{name: "a bundle over the total limit", bundle: overLimit(9, int(hub.DefaultLimits.MaxFileBytes)),
			code: "pack_too_large", path: "size", params: map[string]any{"limit": hub.DefaultLimits.MaxTotalBytes}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A bundle the format cannot be read from at all is refused by Normalize; Validate is for the rest.
			p, err := Normalize(tc.bundle)
			if err != nil {
				t.Fatalf("Normalize: %v", err)
			}
			issues := Validate(tc.bundle, p)
			var got *Issue
			for i := range issues {
				if issues[i].Code == tc.code && issues[i].Path == tc.path {
					got = &issues[i]
				}
			}
			if got == nil {
				t.Fatalf("no %s at %s in %+v", tc.code, tc.path, issues)
			}
			if got.Severity != SeverityError || got.Message == "" {
				t.Errorf("issue %+v: want an error with a message", *got)
			}
			for k, want := range tc.params {
				if fmt.Sprint(got.Params[k]) != fmt.Sprint(want) {
					t.Errorf("params[%s] = %v, want %v", k, got.Params[k], want)
				}
			}
		})
	}

	t.Run("a bundle exactly at the limits is not too large", func(t *testing.T) {
		b := overLimit(hub.DefaultLimits.MaxFiles-1, 1)
		p := mustNormalize(t, b)
		for _, i := range Validate(b, p) {
			if i.Code == "pack_too_large" {
				t.Errorf("%+v", i)
			}
		}
	})

	t.Run("the four good bundles have no error", func(t *testing.T) {
		for _, dir := range []string{"native_bundle", "multica_pack", "claude_agents", "skills_only"} {
			b := loadDir(t, dir)
			if issues := errorIssues(Validate(b, mustNormalize(t, b))); len(issues) != 0 {
				t.Errorf("%s: %+v", dir, issues)
			}
		}
	})

	t.Run("every issue is of the shape of the flow validator's", func(t *testing.T) {
		b := loadDir(t, "broken/agent_skill_missing")
		for _, i := range Validate(b, mustNormalize(t, b)) {
			if i.Code == "" || i.Severity == "" || i.Message == "" {
				t.Errorf("issue %+v lacks a code, a severity or a message", i)
			}
		}
	})

	t.Run("more references than are listed is said, not an error", func(t *testing.T) {
		b := loadDir(t, "claude_agents")
		var capped *Issue
		for _, i := range Validate(b, mustNormalize(t, b)) {
			if i.Code == "pack_references_capped" {
				capped = &i
			}
		}
		if capped == nil || capped.Severity != SeverityInfo {
			t.Errorf("issue %+v", capped)
		}
	})
}

func fileOf(t *testing.T, dir, p string) string {
	t.Helper()
	return fileText(t, loadDir(t, dir), p)
}

// withFile is the bundle with one more file.
func (c *bundleCase) withFile(t *testing.T, p, content string) *hub.Bundle {
	t.Helper()
	return edit(c.bundle(t), map[string]string{p: content})
}
