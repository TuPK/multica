package pack

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestNormalize_ClaudeAgents(t *testing.T) {
	b := loadDir(t, "claude_agents")
	p := mustNormalize(t, b)

	if p.Format != FormatClaudeAgents || p.Leader != "" || p.Flow != "" {
		t.Fatalf("format %q leader %q flow %q", p.Format, p.Leader, p.Flow)
	}
	// One agent per file, one skill per SKILL.md.
	if len(p.Agents) != 7 || len(p.Skills) != 18 {
		t.Fatalf("%d agents and %d skills, want 7 and 18", len(p.Agents), len(p.Skills))
	}
	gom := agentByKey(t, p, "gom_du_lieu_shop")
	if gom.Name != "gom-du-lieu-shop" || !strings.HasPrefix(gom.Description, "Thu thập dữ liệu nội bộ của shop") {
		t.Errorf("name %q description %q", gom.Name, gom.Description)
	}
	// The body of the file is the instructions, and only the body.
	_, body := frontmatterAndBody(t, fileText(t, b, "agents/gom-du-lieu-shop.md"))
	if gom.Instructions != strings.TrimSpace(body) || !strings.HasPrefix(gom.Instructions, "# Gom dữ liệu shop") {
		t.Errorf("instructions are not the body of the file: %.60q", gom.Instructions)
	}
	if gom.Path != "agents/gom-du-lieu-shop.md" {
		t.Errorf("path %q", gom.Path)
	}
	// None of them says which skills it wants, so the person who imports chooses.
	for _, a := range p.Agents {
		if a.SkillsDeclared || len(a.Skills) != 0 {
			t.Errorf("%s: skills %v declared %v, want none declared", a.Key, a.Skills, a.SkillsDeclared)
		}
	}
	// Settings are listed, not applied: tools and model of every agent.
	var ignored []string
	for _, i := range p.Ignored {
		if i.Agent == "gom_du_lieu_shop" {
			ignored = append(ignored, i.Key)
		}
	}
	if !reflect.DeepEqual(ignored, []string{"tools", "model"}) {
		t.Errorf("ignored for gom_du_lieu_shop = %v, want [tools model]", ignored)
	}
	if len(p.Ignored) != 14 {
		t.Errorf("%d ignored settings, want tools and model of 7 agents", len(p.Ignored))
	}
	// A skill keeps the list of its files, references included.
	gen := skillByName(t, p, "viet-giong-gen-z")
	if gen.Dir != "skills/viet-giong-gen-z" || !reflect.DeepEqual(gen.Files, []string{"skills/viet-giong-gen-z/SKILL.md", "skills/viet-giong-gen-z/references/tu-dien-gen-z.md"}) {
		t.Errorf("skill %q: dir %q files %v", gen.Name, gen.Dir, gen.Files)
	}
	if len(skillByName(t, p, "day-chuyen-len-san-pham-moi").Files) != 1 {
		t.Errorf("a skill with one file lists %v", skillByName(t, p, "day-chuyen-len-san-pham-moi").Files)
	}
	if issues := errorIssues(Validate(b, p)); len(issues) != 0 {
		t.Errorf("the real bundle has errors: %v", issues)
	}

	t.Run("an agent that names its skills is given only those", func(t *testing.T) {
		front, body := frontmatterAndBody(t, fileText(t, b, "agents/soan-listing-san-pham.md"))
		edited := edit(b, map[string]string{
			"agents/soan-listing-san-pham.md": "---\n" + front + "\nskills: [tinh-gia-ban-san, mo-ta-san-pham-shop]\n---\n" + body,
		})
		p := mustNormalize(t, edited)
		a := agentByKey(t, p, "soan_listing_san_pham")
		if !a.SkillsDeclared || !reflect.DeepEqual(a.Skills, []string{"tinh-gia-ban-san", "mo-ta-san-pham-shop"}) {
			t.Errorf("skills %v declared %v", a.Skills, a.SkillsDeclared)
		}
		if other := agentByKey(t, p, "gom_du_lieu_shop"); other.SkillsDeclared {
			t.Error("an agent that names no skills was given some")
		}
		// The skills are still all of the bundle's: the choice belongs to the person who imports.
		if len(p.Skills) != 18 {
			t.Errorf("%d skills", len(p.Skills))
		}
		if issues := errorIssues(Validate(edited, p)); len(issues) != 0 {
			t.Errorf("errors: %v", issues)
		}
	})

	t.Run("skills may be given as a comma separated line", func(t *testing.T) {
		front, body := frontmatterAndBody(t, fileText(t, b, "agents/soan-listing-san-pham.md"))
		edited := edit(b, map[string]string{
			"agents/soan-listing-san-pham.md": "---\n" + front + "\nskills: tinh-gia-ban-san, mo-ta-san-pham-shop\n---\n" + body,
		})
		a := agentByKey(t, mustNormalize(t, edited), "soan_listing_san_pham")
		if !reflect.DeepEqual(a.Skills, []string{"tinh-gia-ban-san", "mo-ta-san-pham-shop"}) {
			t.Errorf("skills %v", a.Skills)
		}
	})

	t.Run("the plugin manifest names the bundle", func(t *testing.T) {
		edited := edit(b, map[string]string{".claude-plugin/plugin.json": `{"name":"ban-hang","version":"1.5.0","description":"x"}`})
		p := mustNormalize(t, edited)
		if p.ID != "ban-hang" || p.Name != "ban-hang" || p.Version != "1.5.0" {
			t.Errorf("id %q name %q version %q", p.ID, p.Name, p.Version)
		}
	})
}

func TestNormalize_SkillsOnlyHasNoAgents(t *testing.T) {
	b := loadDir(t, "skills_only")
	p := mustNormalize(t, b)
	if p.Format != FormatSkillsOnly || len(p.Agents) != 0 || p.Leader != "" || p.Flow != "" {
		t.Fatalf("format %q agents %v leader %q", p.Format, agentKeys(p), p.Leader)
	}
	names := make([]string, len(p.Skills))
	for i, s := range p.Skills {
		names[i] = s.Name
	}
	if !reflect.DeepEqual(names, []string{"clear-brief", "data-check", "outline-first"}) {
		t.Fatalf("skills %v", names)
	}
	// A folder up to three levels down is a skill, and its description comes from the file.
	deep := skillByName(t, p, "data-check")
	if deep.Dir != "tools/analysis/data-check" || deep.Description != "Check a table for gaps and duplicated rows before anyone relies on it." {
		t.Errorf("dir %q description %q", deep.Dir, deep.Description)
	}
	if got := skillByName(t, p, "outline-first").Files; !reflect.DeepEqual(got, []string{"outline-first/SKILL.md", "outline-first/references/example.md"}) {
		t.Errorf("files %v", got)
	}
	// allowed-tools is a setting of the skill, and is not applied.
	if !reflect.DeepEqual(p.Ignored, []Ignored{{Skill: "data-check", Key: "allowed-tools"}}) {
		t.Errorf("ignored %v", p.Ignored)
	}
	if issues := errorIssues(Validate(b, p)); len(issues) != 0 {
		t.Errorf("errors: %v", issues)
	}
}

func TestNormalize_MulticaPack(t *testing.T) {
	b := loadDir(t, "multica_pack")
	p := mustNormalize(t, b)
	if p.Format != FormatMulticaPack || p.ID != "field-researcher" || p.Version != "1.2.0" || p.Name != "Field Researcher" || p.Leader != "" {
		t.Fatalf("format %q id %q version %q name %q leader %q", p.Format, p.ID, p.Version, p.Name, p.Leader)
	}
	if len(p.Agents) != 1 {
		t.Fatalf("agents %v, want one", agentKeys(p))
	}
	a := p.Agents[0]
	if a.Key != "field_researcher" || a.Name != "Field Researcher" || a.MaxConcurrentTasks != 2 || a.Path != "instructions.md" {
		t.Errorf("agent %+v", a)
	}
	if !strings.HasPrefix(a.Instructions, "# Field Researcher") || strings.HasSuffix(a.Instructions, "\n") {
		t.Errorf("instructions %q", a.Instructions)
	}
	// The agent has the skills the pack names, and they are all declared.
	if !a.SkillsDeclared || !reflect.DeepEqual(a.Skills, []string{"evidence-notes", "source-ranking"}) {
		t.Errorf("skills %v declared %v", a.Skills, a.SkillsDeclared)
	}
	if len(p.Skills) != 2 || skillByName(t, p, "evidence-notes").Dir != "skills/evidence-notes" {
		t.Errorf("skills %+v", p.Skills)
	}
	// The runtime is the importer's choice.
	if !reflect.DeepEqual(p.Ignored, []Ignored{{Agent: "field_researcher", Key: "runtime_provider"}}) {
		t.Errorf("ignored %v", p.Ignored)
	}
	if issues := errorIssues(Validate(b, p)); len(issues) != 0 {
		t.Errorf("errors: %v", issues)
	}

	t.Run("a pack without template.json says nothing of its id", func(t *testing.T) {
		p := mustNormalize(t, edit(b, nil, "template.json"))
		if p.ID != "" || p.Version != "" {
			t.Errorf("id %q version %q", p.ID, p.Version)
		}
	})
	t.Run("pack.json may carry the id and version itself", func(t *testing.T) {
		raw := fileText(t, b, "pack.json")
		edited := edit(b, map[string]string{"pack.json": strings.Replace(raw, "{\n", "{\n  \"id\": \"own-id\",\n  \"version\": \"3.0.0\",\n", 1)}, "template.json")
		p := mustNormalize(t, edited)
		if p.ID != "own-id" || p.Version != "3.0.0" {
			t.Errorf("id %q version %q", p.ID, p.Version)
		}
	})
}

func TestNormalize_NativeBundleKeepsLeaderAndFlow(t *testing.T) {
	b := loadDir(t, "native_bundle")
	p := mustNormalize(t, b)
	if p.Format != FormatNativeBundle || p.ID != "newsletter-desk" || p.Version != "1.0.0" || p.Name != "Newsletter Desk" {
		t.Fatalf("format %q id %q version %q name %q", p.Format, p.ID, p.Version, p.Name)
	}
	if p.Leader != "planner" || p.Flow != "flow.json" {
		t.Errorf("leader %q flow %q", p.Leader, p.Flow)
	}
	if !reflect.DeepEqual(agentKeys(p), []string{"planner", "writer"}) {
		t.Fatalf("agents %v", agentKeys(p))
	}
	w := agentByKey(t, p, "writer")
	if w.Name != "Writer" || w.RoleLabel != "Writes the issue" || w.Path != "agents/writer/instructions.md" || w.MaxConcurrentTasks != 1 {
		t.Errorf("writer %+v", w)
	}
	if !strings.HasPrefix(w.Instructions, "# Writer") || !w.SkillsDeclared || !reflect.DeepEqual(w.Skills, []string{"outline", "tone-check"}) {
		t.Errorf("writer instructions %q skills %v", w.Instructions, w.Skills)
	}
	if len(p.Skills) != 2 || len(p.Ignored) != 0 {
		t.Errorf("skills %d ignored %v", len(p.Skills), p.Ignored)
	}
	// What may hold a procedure: the instructions of each agent, each SKILL.md, README.md and docs/.
	want := []Reference{
		{Path: "agents/planner/instructions.md", Bytes: len(fileText(t, b, "agents/planner/instructions.md")), Kind: "agent"},
		{Path: "agents/writer/instructions.md", Bytes: len(fileText(t, b, "agents/writer/instructions.md")), Kind: "agent"},
		{Path: "skills/outline/SKILL.md", Bytes: len(fileText(t, b, "skills/outline/SKILL.md")), Kind: "skill"},
		{Path: "skills/tone-check/SKILL.md", Bytes: len(fileText(t, b, "skills/tone-check/SKILL.md")), Kind: "skill"},
		{Path: "README.md", Bytes: len(fileText(t, b, "README.md")), Kind: "doc"},
		{Path: "docs/process.md", Bytes: len(fileText(t, b, "docs/process.md")), Kind: "doc"},
	}
	if !reflect.DeepEqual(p.References, want) {
		t.Errorf("references\n got %+v\nwant %+v", p.References, want)
	}
	if issues := errorIssues(Validate(b, p)); len(issues) != 0 {
		t.Errorf("errors: %v", issues)
	}

	t.Run("a bundle without a flow says so", func(t *testing.T) {
		edited := edit(b, map[string]string{"squad-bundle.json": strings.Replace(fileText(t, b, "squad-bundle.json"), `,
  "flow": "flow.json"`, "", 1)}, "flow.json")
		if p := mustNormalize(t, edited); p.Flow != "" {
			t.Errorf("flow %q", p.Flow)
		}
	})
	t.Run("a manifest that cannot be read is a refused bundle", func(t *testing.T) {
		for name, content := range map[string]string{
			"not json":       `{"format": "multica.squad-bundle/v1", "agents": [`,
			"another format": `{"format": "something/else", "leader": "a", "agents": []}`,
		} {
			_, err := Normalize(edit(b, map[string]string{"squad-bundle.json": content}))
			if !errors.Is(err, ErrInvalid) {
				t.Errorf("%s: err %v, want pack_invalid", name, err)
			}
		}
		if _, err := Normalize(loadDir(t, "broken/bad_manifest")); !errors.Is(err, ErrInvalid) {
			t.Errorf("a cut manifest: %v", err)
		}
	})
}

func TestNormalize_AgentKeyFromName(t *testing.T) {
	agentFile := func(name string) map[string]string {
		return map[string]string{"agents/one.md": "---\nname: \"" + name + "\"\ndescription: An agent.\n---\n\nText.\n"}
	}
	for name, want := range map[string]string{
		"gom-du-lieu-shop": "gom_du_lieu_shop",
		"Research Lead":    "research_lead",
		"a-b.c":            "a_b_c",
		"Soát-Từ.Cấm":      "so_t_t__c_m",
		"already_ok_2":     "already_ok_2",
		"9lives":           "9lives",
	} {
		p := mustNormalize(t, edit(loadDir(t, "broken/nothing"), agentFile(name)))
		if len(p.Agents) != 1 || p.Agents[0].Key != want {
			t.Errorf("%q: key %v, want %q", name, agentKeys(p), want)
		}
	}

	// The key is made from the name alone: two names that give one key are an error, and say which.
	p := mustNormalize(t, loadDir(t, "broken/duplicate_agent_key"))
	if !reflect.DeepEqual(agentKeys(p), []string{"a_b", "a_b"}) {
		t.Fatalf("keys %v", agentKeys(p))
	}
	var duplicate *Issue
	for _, i := range Validate(loadDir(t, "broken/duplicate_agent_key"), p) {
		if i.Code == "pack_agent_key_duplicate" {
			duplicate = &i
		}
	}
	if duplicate == nil || duplicate.Severity != SeverityError || duplicate.Path != "agents[a_b]" {
		t.Errorf("duplicate issue %+v", duplicate)
	}
	// A key that cannot be a key is an error too.
	bad := mustNormalize(t, loadDir(t, "broken/bad_agent_key"))
	var found bool
	for _, i := range Validate(loadDir(t, "broken/bad_agent_key"), bad) {
		found = found || (i.Code == "pack_agent_key_invalid" && i.Path == "agents[9lives]")
	}
	if !found {
		t.Error("no pack_agent_key_invalid for 9lives")
	}
}

func TestReferences_ListedNotParsed(t *testing.T) {
	for _, dir := range []string{"claude_agents", "native_bundle", "multica_pack", "skills_only"} {
		b := loadDir(t, dir)
		before := mustNormalize(t, b)

		// Every markdown file gets another body: text that reads like a procedure, a lead and a flow.
		changed := map[string]string{}
		for _, f := range b.Files {
			if !strings.HasSuffix(f.Path, ".md") {
				continue
			}
			text := string(f.Content)
			replacement := "Step 1: the lead is Zed. Then agent Omega reviews. Loop back to step 1 twice. Leader: omega.\n"
			if strings.HasPrefix(text, "---\n") {
				front, _ := frontmatterAndBody(t, text)
				replacement = "---\n" + front + "\n---\n\n" + replacement
			}
			changed[f.Path] = replacement
		}
		after := mustNormalize(t, edit(b, changed))

		// Only the instructions, and the size of each listed file, may follow the text.
		for _, p := range []*Pack{before, after} {
			for i := range p.Agents {
				p.Agents[i].Instructions = ""
			}
			for i := range p.References {
				p.References[i].Bytes = 0
			}
		}
		if !reflect.DeepEqual(before, after) {
			t.Errorf("%s: the pack changed with the text of its markdown files\nbefore %+v\nafter  %+v", dir, before, after)
		}
	}

	// A file in docs/ or workflows/, or a README.md, is listed with its path and size and nothing else.
	b := edit(loadDir(t, "skills_only"), map[string]string{
		"README.md":              "# R\n",
		"docs/a/b.md":            "# B\n",
		"workflows/launch.md":    "# Launch\n",
		"notes/elsewhere.md":     "# Not listed\n",
		"docs/picture.png":       "not markdown",
		"workflows/data.json":    "{}",
		"outline-first/NOTE.txt": "not markdown",
	})
	p := mustNormalize(t, b)
	got := map[string]string{}
	for _, r := range p.References {
		got[r.Path] = r.Kind
	}
	for _, path := range []string{"README.md", "docs/a/b.md", "workflows/launch.md"} {
		if got[path] != "doc" {
			t.Errorf("%s listed as %q, want doc", path, got[path])
		}
	}
	for _, path := range []string{"notes/elsewhere.md", "docs/picture.png", "workflows/data.json", "outline-first/NOTE.txt", "outline-first/references/example.md"} {
		if _, ok := got[path]; ok {
			t.Errorf("%s is listed, and is not a file that may hold a procedure", path)
		}
	}
	if got["writing/clear-brief/SKILL.md"] != "skill" {
		t.Errorf("a SKILL.md is not listed: %v", got)
	}

	t.Run("at most twenty files of at most 200 KiB", func(t *testing.T) {
		set := map[string]string{"docs/a-huge.md": strings.Repeat("x", 200<<10+1)}
		for i := 0; i < 25; i++ {
			set["docs/d"+string(rune('a'+i))+".md"] = "# Doc\n"
		}
		p := mustNormalize(t, edit(loadDir(t, "skills_only"), set))
		if len(p.References) != 20 {
			t.Fatalf("%d references, want 20", len(p.References))
		}
		for _, r := range p.References {
			if r.Path == "docs/a-huge.md" {
				t.Error("a file over 200 KiB is listed")
			}
		}
		// The three SKILL.md files come before the docs, as the kinds are ordered: agent, skill, doc.
		if p.References[0].Kind != "skill" || p.References[2].Kind != "skill" || p.References[3].Kind != "doc" {
			t.Errorf("order %+v", p.References[:4])
		}
	})
}

func TestUnknownFrontmatterKeysIgnored(t *testing.T) {
	b := loadDir(t, "claude_agents")
	before := mustNormalize(t, b)

	extra := "color: red\nx-weird:\n  nested: [1, 2]\ntriggers: [a, b]\n"
	set := map[string]string{}
	for _, f := range b.Files {
		if !strings.HasSuffix(f.Path, ".md") || !strings.HasPrefix(string(f.Content), "---\n") {
			continue
		}
		front, body := frontmatterAndBody(t, string(f.Content))
		set[f.Path] = "---\n" + front + "\n" + extra + "---\n" + body
	}
	edited := edit(b, set)
	after := mustNormalize(t, edited)

	// The unknown keys leave no trace: not in the pack, not among the ignored settings, not an issue.
	for _, p := range []*Pack{before, after} {
		for i := range p.References {
			p.References[i].Bytes = 0
		}
	}
	if !reflect.DeepEqual(before, after) {
		t.Errorf("unknown keys changed the pack\nbefore %+v\nafter  %+v", before.Ignored, after.Ignored)
	}
	if issues := Validate(edited, mustNormalize(t, edited)); len(errorIssues(issues)) != 0 {
		t.Errorf("unknown keys are not errors: %v", issues)
	}
	// A bundle of skills too.
	skills := loadDir(t, "skills_only")
	withKeys := edit(skills, map[string]string{
		"outline-first/SKILL.md": "---\nname: outline-first\ndescription: Write the outline of a document and get it approved before the full text.\nversion: 4\nlicense: MIT\n---\n\n# Outline first\n",
	})
	if got, want := skillByName(t, mustNormalize(t, withKeys), "outline-first"), skillByName(t, mustNormalize(t, skills), "outline-first"); !reflect.DeepEqual(got, want) {
		t.Errorf("skill %+v, want %+v", got, want)
	}
}
