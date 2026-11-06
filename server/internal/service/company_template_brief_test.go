package service

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

// secretaryBriefSections is the company brief's section order as the server
// accepts it (handler/brief.go briefSections). The Secretary's brief skill must
// number every one of them, so the boss's brief has the Goals, Budget and People
// sections the phase 4 digest feeds (spec 4.1, 4.2, 4.6).
var secretaryBriefSections = []string{
	"Headline", "Needs your decision", "Progress", "Pipeline", "Goals", "Budget",
	"Blockers and risks", "Watch", "People", "FYI",
}

// TestSecretaryBriefSkillNumbersEveryServerSection checks that the Secretary's
// brief skill lists each section the server accepts, in the server's order, and
// that the FYI line names the auto-approvals (spec 4.4). The procedure lives in
// the built-in skill, not in the instructions.
func TestSecretaryBriefSkillNumbersEveryServerSection(t *testing.T) {
	var data companyData
	if err := json.Unmarshal(companyTemplateJSON, &data); err != nil {
		t.Fatalf("decode template data: %v", err)
	}
	var brief string
	for _, sk := range data.StarterSkills {
		if sk.Name == StarterSkillSecretaryBrief {
			brief = sk.Content
			break
		}
	}
	if brief == "" {
		t.Fatalf("the %s skill is not in the template data", StarterSkillSecretaryBrief)
	}

	numbered := regexp.MustCompile(`(?m)^(\d+)\. \*\*([^*]+)\*\*`)
	var got []string
	for _, m := range numbered.FindAllStringSubmatch(brief, -1) {
		got = append(got, m[2])
	}
	if strings.Join(got, "|") != strings.Join(secretaryBriefSections, "|") {
		t.Fatalf("the Secretary's brief sections are %v, want %v", got, secretaryBriefSections)
	}
	if !strings.Contains(brief, "Auto-approved by policy") {
		t.Error("the Secretary's FYI does not name the auto-approvals")
	}
}
