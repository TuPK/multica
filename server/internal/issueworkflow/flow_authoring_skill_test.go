package issueworkflow

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// The skill that teaches an agent to draft a flow (plan P25, spec 16.4) carries a copy of the
// pattern documents. A copy that drifts, or one that does not validate, teaches a wrong document.

var flowAuthoringSkillDir = filepath.Join("..", "..", "..", "catalog", "skills", "builtin", "flow-authoring")

func TestFlowAuthoringSkill_PatternFilesValidate(t *testing.T) {
	copies, _ := filepath.Glob(filepath.Join(flowAuthoringSkillDir, "references", "patterns", "*.json"))
	sources, _ := filepath.Glob(filepath.Join("testdata", "flows", "patterns", "*.json"))
	if len(copies) == 0 || len(copies) != len(sources) {
		t.Fatalf("the skill has %d pattern files, the tests have %d", len(copies), len(sources))
	}
	for _, copied := range copies {
		name := filepath.Base(copied)
		t.Run(name, func(t *testing.T) {
			raw := readFile(t, copied)
			for _, issue := range ValidateFlowDocJSON(raw, vctx()) {
				if issue.Severity == SeverityError {
					t.Fatalf("the skill teaches a document with an error:%s", describeIssues(ValidateFlowDocJSON(raw, vctx())))
				}
			}
			source, err := os.ReadFile(filepath.Join("testdata", "flows", "patterns", name))
			if err != nil {
				t.Fatalf("no source for %s: %v", name, err)
			}
			if !bytes.Equal(raw, source) {
				t.Fatalf("%s differs from testdata/flows/patterns/%s: copy it again", name, name)
			}
		})
	}
}

// The skill's own files: it is a folder an agent installs as it is.
func TestFlowAuthoringSkill_FolderIsAnInstallableSkill(t *testing.T) {
	for _, name := range []string{"SKILL.md", "template.json"} {
		if _, err := os.Stat(filepath.Join(flowAuthoringSkillDir, name)); err != nil {
			t.Errorf("the skill has no %s: %v", name, err)
		}
	}
	skill := readFile(t, filepath.Join(flowAuthoringSkillDir, "SKILL.md"))
	if !bytes.HasPrefix(skill, []byte("---\nname: flow-authoring\n")) {
		t.Errorf("SKILL.md does not open with the front matter of the skill:\n%.120s", skill)
	}
}
