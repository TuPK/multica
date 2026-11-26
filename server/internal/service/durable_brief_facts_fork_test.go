package service

import (
	"strings"
	"testing"
)

// TestBriefFactsMarkdownNamesEachNumber: the outcome line carries every count, the reopened
// fails are named inside the fails, and each mode change and the envelope count are listed.
func TestBriefFactsMarkdownNamesEachNumber(t *testing.T) {
	got := BriefFactsMarkdown(BriefOutcomeFacts{
		Settled: 25, Pass: 24, Fail: 1, Reopened: 1,
		ModeChanges:   []BriefModeChange{{AgentName: "Worker", WorkClass: "general", Mode: "sampled"}},
		EnvelopeMoves: 0,
	})
	for _, want := range []string{
		"**Outcomes this period:** 25 settled: 24 pass, 0 partial, 1 fail (1 reopened), 0 unverified.",
		"- Verification mode of Worker (general) is now sampled.",
		"- Autonomy envelope: 0 squad level change(s) on evidence.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("facts missing %q\n%s", want, got)
		}
	}
}

// TestBriefFactsMarkdownQuietPeriodReadsAsZero: a period with no outcome still shows the
// outcome line, so the boss sees zero rather than a missing number.
func TestBriefFactsMarkdownQuietPeriodReadsAsZero(t *testing.T) {
	got := BriefFactsMarkdown(BriefOutcomeFacts{})
	if !strings.Contains(got, "**Outcomes this period:** 0 settled") {
		t.Fatalf("quiet period lost the outcome line:\n%s", got)
	}
}

// TestBriefWithFactsGoesUnderProgress: the facts land before the heading that follows
// Progress, and the other sections keep their order.
func TestBriefWithFactsGoesUnderProgress(t *testing.T) {
	body := strings.Join([]string{
		"## Headline",
		"Quiet week.",
		"",
		"## Progress",
		"Squad Alpha delivered.",
		"",
		"## Blockers and risks",
		"None.",
	}, "\n")
	got := BriefWithFacts(body, "**Outcomes this period:** 25 settled.\n")
	progress := strings.Index(got, "## Progress")
	facts := strings.Index(got, "**Outcomes this period:**")
	blockers := strings.Index(got, "## Blockers and risks")
	if !(progress < facts && facts < blockers) {
		t.Fatalf("facts out of place (progress=%d facts=%d blockers=%d):\n%s", progress, facts, blockers, got)
	}
	if !strings.Contains(got, "Squad Alpha delivered.") || !strings.Contains(got, "## Headline\nQuiet week.") {
		t.Fatalf("existing text lost:\n%s", got)
	}
}

// TestBriefWithFactsIgnoresHeadingsInFences: a "## " line inside a code fence does not end
// the Progress section.
func TestBriefWithFactsIgnoresHeadingsInFences(t *testing.T) {
	body := strings.Join([]string{
		"## Progress",
		"```",
		"## not a heading",
		"```",
		"## FYI",
		"None.",
	}, "\n")
	got := BriefWithFacts(body, "**Outcomes this period:** 0 settled.\n")
	fence := strings.Index(got, "```\n## not a heading\n```")
	facts := strings.Index(got, "**Outcomes this period:**")
	fyi := strings.Index(got, "## FYI")
	if !(fence < facts && facts < fyi) {
		t.Fatalf("facts placed inside the fence or after FYI:\n%s", got)
	}
}

// TestBriefWithFactsWithoutProgressLeavesBodyAlone: a body with no Progress section is
// returned unchanged, and so is a body when there are no facts.
func TestBriefWithFactsWithoutProgressLeavesBodyAlone(t *testing.T) {
	body := "## Headline\nNothing.\n"
	if got := BriefWithFacts(body, "**Outcomes this period:** 0 settled.\n"); got != body {
		t.Fatalf("body without Progress changed:\n%s", got)
	}
	withProgress := "## Progress\nNews.\n"
	if got := BriefWithFacts(withProgress, ""); got != withProgress {
		t.Fatalf("empty facts changed the body:\n%s", got)
	}
}
