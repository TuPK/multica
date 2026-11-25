package issueworkflow

import "testing"

// TestValidate_EffectsOnlyExternal: a task's effects is unset or external (durable spec
// 5.4). Any other value is refused under T09, and external is accepted.
func TestValidate_EffectsOnlyExternal(t *testing.T) {
	bad := tradeDoc(t, func(d *FlowDoc) { stepIdx(d, "collect").Tasks[0].Effects = "maybe" })
	issues := issuesOfRule(ValidateFlowDoc(bad, vctx()), "T09")
	if len(issues) != 1 || issues[0].Code != "effects_invalid" {
		t.Fatalf("effects \"maybe\": %s, want one T09 effects_invalid", describeIssues(ValidateFlowDoc(bad, vctx())))
	}

	good := tradeDoc(t, func(d *FlowDoc) { stepIdx(d, "collect").Tasks[0].Effects = EffectsExternal })
	if got := issuesOfRule(ValidateFlowDoc(good, vctx()), "T09"); len(got) != 0 {
		t.Fatalf("effects external: %s, want no T09", describeIssues(got))
	}
}
