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

// TestLift_CarriesEffectsMark: a task of a schema 1 flow marked external lifts to a graph
// task marked external, so the graph validator and the engine read the same mark.
func TestLift_CarriesEffectsMark(t *testing.T) {
	def := tradeFlow()
	def.Steps[1].Tasks[0].Effects = EffectsExternal
	g := mustLift(t, def, LiftOpts{Runtime: true})
	step, ok := g.Step("collect")
	if !ok || len(step.Tasks) == 0 {
		t.Fatalf("lifted graph has no collect step with tasks")
	}
	if got := step.Tasks[0].Effects; got != EffectsExternal {
		t.Fatalf("lifted task effects = %q, want %q", got, EffectsExternal)
	}
}

// TestValidateSchema1RefusesOtherEffects: a schema 1 task takes only external or no effects mark.
func TestValidateSchema1RefusesOtherEffects(t *testing.T) {
	def := tradeFlow()
	def.Steps[1].Tasks[0].Effects = "maybe"
	if err := validateFlowStep(def, 1, def.Steps[1]); err == nil {
		t.Fatal("effects \"maybe\" passed schema 1 validation, want a refusal")
	}
	def.Steps[1].Tasks[0].Effects = EffectsExternal
	if err := validateFlowStep(def, 1, def.Steps[1]); err != nil {
		t.Fatalf("effects external: %v", err)
	}
}
