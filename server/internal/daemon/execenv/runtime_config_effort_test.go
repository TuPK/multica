package execenv

import (
	"strings"
	"testing"
)

func TestBriefCarriesAgentThinkingLevelForClaude(t *testing.T) {
	t.Parallel()
	out := buildMetaSkillContent("claude", TaskContextForEnv{IssueID: "issue-1", AgentID: "a-1", AgentName: "Eve", AgentThinkingLevel: "high"})
	for _, want := range []string{
		"## Reasoning Effort",
		"This agent's thinking level is `high`.",
		"effort: \"high\"",
		"as its level argument",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("claude brief missing %q", want)
		}
	}
}

func TestBriefOmitsEffortPolicyWithoutClaudeOrLevel(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		provider string
		level    string
	}{
		"other provider": {provider: "codex", level: "high"},
		"no level":       {provider: "claude", level: ""},
	} {
		out := buildMetaSkillContent(tc.provider, TaskContextForEnv{IssueID: "issue-1", AgentID: "a-1", AgentName: "Eve", AgentThinkingLevel: tc.level})
		if strings.Contains(out, "## Reasoning Effort") {
			t.Errorf("%s: brief unexpectedly carries the effort policy", name)
		}
	}
}
