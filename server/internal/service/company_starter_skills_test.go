package service

import (
	"strings"
	"testing"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestStarterSkillsForRole(t *testing.T) {
	t.Parallel()
	card := companySquad{Key: "market_research", Leader: "research_lead"}

	cases := []struct {
		name string
		card companySquad
		role companyRole
		want []string
	}{
		{
			name: "the Secretary gets its whole built-in bundle, router first",
			card: companySquad{Key: "executive_office", Leader: "secretary"},
			role: companyRole{Key: "secretary", Runtime: "secretary"},
			want: SecretarySkills,
		},
		{
			name: "a squad leader gets the reporting and directive skills",
			card: card,
			role: companyRole{Key: "research_lead", Runtime: "worker"},
			want: []string{StarterSkillSquadReporting, StarterSkillDirectiveExecution},
		},
		{
			name: "a member gets none",
			card: card,
			role: companyRole{Key: "scout", Runtime: "worker"},
			want: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := starterSkillsForRole(tc.card, tc.role)
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("starterSkillsForRole() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsStarterSkillConfig(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		`{"source":"starter_company"}`:      true,
		`{"source":"someone_else"}`:         false,
		`{}`:                                false,
		`null`:                              false,
		``:                                  false,
		`not json`:                          false,
		string(starterSkillConfig("skill")): true,
	}
	for in, want := range cases {
		if got := isStarterSkillConfig([]byte(in)); got != want {
			t.Errorf("isStarterSkillConfig(%q) = %v, want %v", in, got, want)
		}
	}
}

// The embedded data carries every starter skill, each with a SKILL.md whose
// frontmatter name matches its folder, a description, a body, and reference files.
func TestStarterSkillsEmbeddedInCompanyData(t *testing.T) {
	t.Parallel()
	data, err := loadCompanyData()
	if err != nil {
		t.Fatalf("loadCompanyData: %v", err)
	}
	byName := map[string]companyStarterSkill{}
	for _, sk := range data.StarterSkills {
		byName[sk.Name] = sk
	}
	for _, name := range append([]string{StarterSkillSquadReporting, StarterSkillDirectiveExecution}, SecretarySkills...) {
		sk, ok := byName[name]
		if !ok {
			t.Fatalf("starter skill %q is missing from the generated company data (pnpm catalog:build)", name)
		}
		if sk.Description == "" {
			t.Errorf("%s: empty description", name)
		}
		if !strings.HasPrefix(sk.Content, "---\nname: "+name+"\n") {
			t.Errorf("%s: SKILL.md does not start with its frontmatter name", name)
		}
		if len(sk.Files) == 0 || sk.Files[0].Content == "" {
			t.Errorf("%s: no reference file", name)
		}
	}
}

func TestSecretaryDeskTitleIsStable(t *testing.T) {
	t.Parallel()
	// Setup finds the desk by this title, so the title is part of the contract.
	if SecretaryDeskProjectTitle != "Secretary desk" {
		t.Fatalf("SecretaryDeskProjectTitle = %q", SecretaryDeskProjectTitle)
	}
}

func TestSecretarySkillsAreTheBuiltInBundle(t *testing.T) {
	t.Parallel()
	want := []string{
		"secretary-router",
		"secretary-facts",
		"secretary-ask",
		"secretary-analysis",
		"secretary-brief",
		"secretary-directives",
		"secretary-leader-questions",
	}
	if strings.Join(SecretarySkills, ",") != strings.Join(want, ",") {
		t.Fatalf("SecretarySkills = %v, want %v", SecretarySkills, want)
	}
}

func TestStarterSkillUpdateDue(t *testing.T) {
	t.Parallel()
	const installed = "---\nname: secretary-ask\n---\nold procedure"
	const bundle = "---\nname: secretary-ask\n---\nnew procedure"

	skill := func(content string, config []byte) db.Skill {
		return db.Skill{Content: content, Config: config}
	}
	next := companyStarterSkill{Name: "secretary-ask", Content: bundle}

	cases := map[string]struct {
		existing db.Skill
		want     bool
	}{
		"an unedited skill takes the newer bundle": {
			existing: skill(installed, starterSkillConfig(installed)),
			want:     true,
		},
		"a skill the boss edited keeps the boss's content": {
			existing: skill("---\nname: secretary-ask\n---\nthe boss's own", starterSkillConfig(installed)),
			want:     false,
		},
		"a skill from before fingerprints is treated as unedited": {
			existing: skill(installed, []byte(`{"source":"starter_company"}`)),
			want:     true,
		},
		"a skill the boss made under the same name is never touched": {
			existing: skill(installed, []byte(`{"source":"someone_else"}`)),
			want:     false,
		},
		"a skill already at the bundle's content needs no update": {
			existing: skill(bundle, starterSkillConfig(bundle)),
			want:     false,
		},
	}
	for name, tc := range cases {
		if got := starterSkillUpdateDue(tc.existing, next); got != tc.want {
			t.Errorf("%s: starterSkillUpdateDue() = %v, want %v", name, got, tc.want)
		}
	}
}
