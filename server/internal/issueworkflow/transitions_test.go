package issueworkflow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/issuestatus"
)

func cond(field, op string, value any) *Condition {
	return &Condition{Field: field, Op: op, Value: value}
}

func TestEvaluateMatrix(t *testing.T) {
	outcomeApproved := Facts{Outcome: OutcomeApproved, Priority: "high", Labels: []string{"bug", "api"}, ProjectID: "p1", ReworkCount: 2, CreatorType: "member", AssigneeType: "agent"}
	noOutcome := Facts{Priority: "", ReworkCount: 0}
	tests := map[string]struct {
		c    Condition
		f    Facts
		want bool
	}{
		"outcome eq matches":                   {c: *cond(FieldOutcome, OpEq, OutcomeApproved), f: outcomeApproved, want: true},
		"outcome eq other value":               {c: *cond(FieldOutcome, OpEq, OutcomeRejected), f: outcomeApproved, want: false},
		"outcome neq other value":              {c: *cond(FieldOutcome, OpNeq, OutcomeRejected), f: outcomeApproved, want: true},
		"outcome in list":                      {c: *cond(FieldOutcome, OpIn, []any{OutcomeRejected, OutcomeApproved}), f: outcomeApproved, want: true},
		"outcome exists when reported":         {c: *cond(FieldOutcome, OpExists, nil), f: outcomeApproved, want: true},
		"outcome absent matches no eq":         {c: *cond(FieldOutcome, OpEq, OutcomeApproved), f: noOutcome, want: false},
		"outcome absent matches no neq":        {c: *cond(FieldOutcome, OpNeq, OutcomeRejected), f: noOutcome, want: false},
		"outcome absent does not exist":        {c: *cond(FieldOutcome, OpExists, nil), f: noOutcome, want: false},
		"negated absent outcome is true":       {c: Condition{Not: cond(FieldOutcome, OpEq, OutcomeApproved)}, f: noOutcome, want: true},
		"priority eq":                          {c: *cond(FieldPriority, OpEq, "high"), f: outcomeApproved, want: true},
		"priority empty counts as none":        {c: *cond(FieldPriority, OpEq, "none"), f: noOutcome, want: true},
		"priority gte matches equal rank":      {c: *cond(FieldPriority, OpGte, "high"), f: outcomeApproved, want: true},
		"priority gt excludes equal rank":      {c: *cond(FieldPriority, OpGt, "high"), f: outcomeApproved, want: false},
		"priority lt above rank":               {c: *cond(FieldPriority, OpLt, "urgent"), f: outcomeApproved, want: true},
		"priority lte below rank":              {c: *cond(FieldPriority, OpLte, "medium"), f: outcomeApproved, want: false},
		"priority in list":                     {c: *cond(FieldPriority, OpIn, []any{"low", "high"}), f: outcomeApproved, want: true},
		"labels eq contains":                   {c: *cond(FieldLabels, OpEq, "bug"), f: outcomeApproved, want: true},
		"labels eq missing":                    {c: *cond(FieldLabels, OpEq, "ui"), f: outcomeApproved, want: false},
		"labels neq missing":                   {c: *cond(FieldLabels, OpNeq, "ui"), f: outcomeApproved, want: true},
		"labels in any present":                {c: *cond(FieldLabels, OpIn, []any{"ui", "api"}), f: outcomeApproved, want: true},
		"labels exists with labels":            {c: *cond(FieldLabels, OpExists, nil), f: outcomeApproved, want: true},
		"labels absent matches nothing":        {c: *cond(FieldLabels, OpNeq, "ui"), f: noOutcome, want: false},
		"rework count gte threshold":           {c: *cond(FieldReworkCount, OpGte, 2), f: outcomeApproved, want: true},
		"rework count eq zero":                 {c: *cond(FieldReworkCount, OpEq, 0), f: noOutcome, want: true},
		"rework count lt threshold":            {c: *cond(FieldReworkCount, OpLt, 2), f: outcomeApproved, want: false},
		"rework count from json float":         {c: *cond(FieldReworkCount, OpGt, float64(1)), f: outcomeApproved, want: true},
		"rework count in list":                 {c: *cond(FieldReworkCount, OpIn, []any{float64(2), float64(5)}), f: outcomeApproved, want: true},
		"project eq":                           {c: *cond(FieldProjectID, OpEq, "p1"), f: outcomeApproved, want: true},
		"project absent matches no eq":         {c: *cond(FieldProjectID, OpEq, "p1"), f: noOutcome, want: false},
		"creator type eq":                      {c: *cond(FieldCreatorType, OpEq, "member"), f: outcomeApproved, want: true},
		"assignee type neq":                    {c: *cond(FieldAssigneeType, OpNeq, "agent"), f: outcomeApproved, want: false},
		"absent assignee does not exist":       {c: *cond(FieldAssigneeType, OpExists, nil), f: noOutcome, want: false},
		"all true":                             {c: Condition{All: []Condition{*cond(FieldOutcome, OpEq, OutcomeApproved), *cond(FieldPriority, OpEq, "high")}}, f: outcomeApproved, want: true},
		"all with one false":                   {c: Condition{All: []Condition{*cond(FieldOutcome, OpEq, OutcomeApproved), *cond(FieldPriority, OpEq, "low")}}, f: outcomeApproved, want: false},
		"any with one true":                    {c: Condition{Any: []Condition{*cond(FieldOutcome, OpEq, OutcomeRejected), *cond(FieldLabels, OpEq, "bug")}}, f: outcomeApproved, want: true},
		"any all false":                        {c: Condition{Any: []Condition{*cond(FieldOutcome, OpEq, OutcomeRejected), *cond(FieldLabels, OpEq, "ui")}}, f: outcomeApproved, want: false},
		"not inverts":                          {c: Condition{Not: cond(FieldReworkCount, OpGte, 5)}, f: outcomeApproved, want: true},
		"nested group":                         {c: Condition{All: []Condition{{Not: cond(FieldOutcome, OpEq, OutcomeRejected)}, {Any: []Condition{*cond(FieldPriority, OpGte, "high"), *cond(FieldLabels, OpEq, "ui")}}}}, f: outcomeApproved, want: true},
		"empty group matches nothing":          {c: Condition{All: []Condition{}}, f: outcomeApproved, want: false},
		"unknown field matches nothing":        {c: *cond("nope", OpEq, "x"), f: outcomeApproved, want: false},
		"non-string eq value matches nothing":  {c: *cond(FieldOutcome, OpEq, 3), f: outcomeApproved, want: false},
		"non-number rework value is rejected":  {c: *cond(FieldReworkCount, OpEq, "2"), f: outcomeApproved, want: false},
		"priority ordering on unknown value":   {c: *cond(FieldPriority, OpGte, "critical"), f: outcomeApproved, want: false},
		"int value in code is a number":        {c: *cond(FieldReworkCount, OpEq, 2), f: outcomeApproved, want: true},
		"string list in code is a list":        {c: *cond(FieldLabels, OpIn, []string{"ui", "api"}), f: outcomeApproved, want: true},
		"empty priority counts as none in any": {c: Condition{Any: []Condition{*cond(FieldPriority, OpEq, "none")}}, f: Facts{}, want: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := Evaluate(tt.c, tt.f); got != tt.want {
				t.Fatalf("Evaluate = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRouteFirstMatchWinsAndDefaultIsFallback(t *testing.T) {
	step := Step{StatusKey: "in_review", Transitions: []Transition{
		{ID: "urgent", Label: "Urgent", When: cond(FieldPriority, OpGte, "urgent"), ToStatusKey: "blocked"},
		{ID: "rework", Label: "Rework", When: &Condition{All: []Condition{
			*cond(FieldOutcome, OpEq, OutcomeRejected), *cond(FieldReworkCount, OpGte, 3)}}, ToStatusKey: "blocked"},
		{ID: "rejected", Label: "Rejected", When: cond(FieldOutcome, OpEq, OutcomeRejected), ToStatusKey: "in_progress"},
		{ID: "default", Label: "Default", ToStatusKey: "qa"},
	}}
	tests := map[string]struct {
		f      Facts
		target string
	}{
		"urgent beats rejection":      {f: Facts{Priority: "urgent", Outcome: OutcomeRejected}, target: "blocked"},
		"third rejection blocks":      {f: Facts{Outcome: OutcomeRejected, ReworkCount: 3}, target: "blocked"},
		"plain rejection goes back":   {f: Facts{Outcome: OutcomeRejected, ReworkCount: 1}, target: "in_progress"},
		"approval falls to default":   {f: Facts{Outcome: OutcomeApproved}, target: "qa"},
		"no outcome falls to default": {f: Facts{}, target: "qa"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok := step.Route(tt.f)
			if !ok || got.ToStatusKey != tt.target {
				t.Fatalf("Route = %q (ok %v), want %q", got.ToStatusKey, ok, tt.target)
			}
		})
	}
}

func TestRouteWithoutDefaultMayMatchNothing(t *testing.T) {
	step := Step{StatusKey: "qa", Transitions: []Transition{
		{ID: "approved", Label: "Approved", When: cond(FieldOutcome, OpEq, OutcomeApproved), ToStatusKey: "done"},
	}}
	if got, ok := step.Route(Facts{Outcome: OutcomeBlocked}); ok {
		t.Fatalf("Route matched %q, want no match", got.ToStatusKey)
	}
}

func TestValidateConditionMatrix(t *testing.T) {
	deep := func(levels int) Condition {
		c := *cond(FieldOutcome, OpEq, OutcomeApproved)
		for i := 1; i < levels; i++ {
			inner := c
			c = Condition{Not: &inner}
		}
		return c
	}
	tests := map[string]struct {
		c       Condition
		wantErr string
	}{
		"valid leaf":                  {c: *cond(FieldPriority, OpIn, []any{"low", "high"})},
		"valid exists":                {c: *cond(FieldLabels, OpExists, nil)},
		"depth four is allowed":       {c: deep(4)},
		"depth five is refused":       {c: deep(5), wantErr: "nest at most 4"},
		"empty group":                 {c: Condition{All: []Condition{}}, wantErr: "at least one condition"},
		"two forms at once":           {c: Condition{Not: cond(FieldOutcome, OpEq, OutcomeApproved), Field: FieldOutcome, Op: OpEq, Value: OutcomeApproved}, wantErr: "is a group"},
		"no form at all":              {c: Condition{}, wantErr: "is a group"},
		"unknown field":               {c: *cond("color", OpEq, "red"), wantErr: "unknown condition field"},
		"operator not for field":      {c: *cond(FieldOutcome, OpGt, OutcomeApproved), wantErr: "does not apply"},
		"gt on rework count is fine":  {c: *cond(FieldReworkCount, OpGt, 1)},
		"exists on priority refused":  {c: *cond(FieldPriority, OpExists, nil), wantErr: "does not apply"},
		"exists with a value refused": {c: *cond(FieldLabels, OpExists, "bug"), wantErr: "takes no value"},
		"in with empty list":          {c: *cond(FieldPriority, OpIn, []any{}), wantErr: "non-empty list"},
		"enum value not allowed":      {c: *cond(FieldOutcome, OpEq, "maybe"), wantErr: "not one of the field's values"},
		"priority not in enum":        {c: *cond(FieldPriority, OpEq, "critical"), wantErr: "not one of the field's values"},
		"rework count negative":       {c: *cond(FieldReworkCount, OpGte, -1), wantErr: "whole number"},
		"rework count fractional":     {c: *cond(FieldReworkCount, OpGte, 1.5), wantErr: "whole number"},
		"rework count as text":        {c: *cond(FieldReworkCount, OpEq, "2"), wantErr: "whole number"},
		"label empty string refused":  {c: *cond(FieldLabels, OpEq, ""), wantErr: "non-empty string"},
		"assignee type squad allowed": {c: *cond(FieldAssigneeType, OpEq, "squad")},
		"creator type squad refused":  {c: *cond(FieldCreatorType, OpEq, "squad"), wantErr: "not one of the field's values"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := ValidateCondition(tt.c)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateCondition: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateCondition = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func transitionWorkflow() Definition {
	return Definition{Steps: []Step{
		{StatusKey: "in_progress", Handler: Handler{Type: HandlerNone}},
		{StatusKey: "in_review", Handler: Handler{Type: HandlerNone}},
		{StatusKey: "qa", Handler: Handler{Type: HandlerNone}},
		{StatusKey: issuestatus.Blocked, Handler: Handler{Type: HandlerNone}},
		{StatusKey: issuestatus.Done, Handler: Handler{Type: HandlerNone}},
	}}
}

func TestValidateTransitions(t *testing.T) {
	def := transitionWorkflow()
	with := func(ts ...Transition) Step {
		return Step{StatusKey: "in_review", Handler: Handler{Type: HandlerNone}, Transitions: ts}
	}
	ok := Transition{ID: "a", Label: "A", When: cond(FieldOutcome, OpEq, OutcomeApproved), ToStatusKey: "qa"}
	def2 := Transition{ID: "d", Label: "D", ToStatusKey: "done"}
	tests := map[string]struct {
		step    Step
		wantErr string
	}{
		"ordered branches and default":      {step: with(ok, def2)},
		"no transitions":                    {step: with()},
		"unknown target":                    {step: with(Transition{ID: "x", Label: "X", ToStatusKey: "nowhere"}), wantErr: "not another status"},
		"target is the step itself":         {step: with(Transition{ID: "x", Label: "X", ToStatusKey: "in_review"}), wantErr: "not another status"},
		"default not last":                  {step: with(def2, ok), wantErr: "must come last"},
		"two defaults":                      {step: with(Transition{ID: "d1", Label: "D1", ToStatusKey: "done"}, Transition{ID: "d2", Label: "D2", ToStatusKey: "qa"}), wantErr: "must come last"},
		"duplicate id":                      {step: with(ok, Transition{ID: "a", Label: "B", ToStatusKey: "done"}), wantErr: "more than once"},
		"empty label":                       {step: with(Transition{ID: "x", Label: "", ToStatusKey: "qa"}), wantErr: "labels must be"},
		"label too long":                    {step: with(Transition{ID: "x", Label: strings.Repeat("l", MaxTransitionLabelRunes+1), ToStatusKey: "qa"}), wantErr: "labels must be"},
		"invalid condition":                 {step: with(Transition{ID: "x", Label: "X", When: cond(FieldOutcome, OpGt, "x"), ToStatusKey: "qa"}), wantErr: "does not apply"},
		"handler override squad refused":    {step: with(Transition{ID: "x", Label: "X", ToStatusKey: "qa", Handler: &Handler{Type: HandlerSquad, ID: "s"}}), wantErr: "hand off to an agent"},
		"handler override agent without id": {step: with(Transition{ID: "x", Label: "X", ToStatusKey: "qa", Handler: &Handler{Type: HandlerAgent}}), wantErr: "choose a agent"},
		"too many branches":                 {step: with(manyTransitions(MaxTransitions + 1)...), wantErr: "at most 10 branches"},
		"ten branches allowed":              {step: with(manyTransitions(MaxTransitions)...)},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := validateTransitions(tt.step, def)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateTransitions: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateTransitions = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

// manyTransitions builds n conditional branches to qa, with no default.
func manyTransitions(n int) []Transition {
	out := make([]Transition, n)
	for i := range out {
		out[i] = Transition{ID: "t" + strings.Repeat("x", i+1), Label: "L", When: cond(FieldReworkCount, OpEq, i), ToStatusKey: "qa"}
	}
	return out
}

func TestLegacyTransitionsMatchTheMigrationRule(t *testing.T) {
	tests := map[string]struct {
		next, back string
		want       []string // ids in order
	}{
		"next and back":  {next: "done", back: "in_progress", want: []string{"approved", "rejected", "default"}},
		"next only":      {next: "done", want: []string{"approved", "default"}},
		"back only":      {back: "in_progress", want: []string{"rejected"}},
		"neither routes": {want: nil},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := LegacyTransitions(tt.next, tt.back)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d transitions, want %d: %+v", len(got), len(tt.want), got)
			}
			for i, id := range tt.want {
				if got[i].ID != id {
					t.Fatalf("transition %d id = %q, want %q", i, got[i].ID, id)
				}
			}
			step := Step{StatusKey: "review", Transitions: got}
			if step.NextStatusKey() != tt.next || step.BackStatusKey() != tt.back {
				t.Fatalf("derived next/back = %q/%q, want %q/%q", step.NextStatusKey(), step.BackStatusKey(), tt.next, tt.back)
			}
		})
	}
}

func TestDecodeLegacyRowsAndEncodeTransitionsOnly(t *testing.T) {
	legacy := []byte(`[{"status_key":"in_review","handler":{"type":"agent","id":"a1"},"instructions":"review","next_status_key":"done","back_status_key":"in_progress"},{"status_key":"todo"}]`)
	steps, err := DecodeSteps(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if steps[0].NextStatusKey() != "done" || steps[0].BackStatusKey() != "in_progress" {
		t.Fatalf("legacy row did not decode to transitions: %+v", steps[0].Transitions)
	}

	raw, err := EncodeSteps(steps)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "next_status_key") || strings.Contains(string(raw), "back_status_key") {
		t.Fatalf("encoded steps still carry legacy keys: %s", raw)
	}
	again, err := DecodeSteps(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(again[0].Transitions) != len(steps[0].Transitions) {
		t.Fatalf("round trip changed transitions: %+v vs %+v", again[0].Transitions, steps[0].Transitions)
	}
}

func TestDecodeKeepsStoredTransitions(t *testing.T) {
	stored := []byte(`[{"status_key":"qa","handler":{"type":"none"},"transitions":[
		{"id":"t1","label":"Blocked","when":{"field":"priority","op":"gte","value":"urgent"},"to_status_key":"blocked"},
		{"id":"d","label":"Default","when":null,"to_status_key":"done"}]}]`)
	steps, err := DecodeSteps(stored)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps[0].Transitions) != 2 || steps[0].Transitions[0].When == nil {
		t.Fatalf("stored transitions lost: %+v", steps[0].Transitions)
	}
	if steps[0].NextStatusKey() != "done" {
		t.Fatalf("default target = %q, want done", steps[0].NextStatusKey())
	}
}

func TestNormalizeTransitionsAssignsIDsOnce(t *testing.T) {
	in := []Transition{{Label: "  New  ", ToStatusKey: " qa "}, {ID: "keep", Label: "Kept", ToStatusKey: "done"}}
	out := NormalizeTransitions(in)
	if out[0].ID == "" || !strings.HasPrefix(out[0].ID, "t") {
		t.Fatalf("new transition has no id: %+v", out[0])
	}
	if out[0].Label != "New" || out[0].ToStatusKey != "qa" {
		t.Fatalf("text not trimmed: %+v", out[0])
	}
	if out[1].ID != "keep" {
		t.Fatalf("existing id changed: %q", out[1].ID)
	}
	again := NormalizeTransitions(out)
	if again[0].ID != out[0].ID {
		t.Fatalf("id changed on second normalize: %q -> %q", out[0].ID, again[0].ID)
	}
}

func TestNextStatusFallsBackWhenOnlyBranchesAreConditional(t *testing.T) {
	def := Definition{Steps: []Step{
		{StatusKey: "in_progress", Handler: Handler{Type: HandlerNone}},
		{StatusKey: "in_review", Handler: Handler{Type: HandlerNone}, Transitions: []Transition{
			{ID: "r", Label: "Rejected", When: cond(FieldOutcome, OpEq, OutcomeRejected), ToStatusKey: "in_progress"},
		}},
		{StatusKey: "done", Handler: Handler{Type: HandlerNone}},
	}}
	if got := def.NextStatus("in_review"); got != "done" {
		t.Fatalf("NextStatus = %q, want the board's next status done", got)
	}
	if got := def.Steps[1].BackStatusKey(); got != "in_progress" {
		t.Fatalf("BackStatusKey = %q, want in_progress", got)
	}
}

func TestOutcomeRouteErrorNamesTheRoute(t *testing.T) {
	err := &OutcomeRouteError{Outcome: OutcomeRejected, StatusKey: "in_review", Expected: "in_progress", Requested: "done"}
	msg := err.Error()
	for _, want := range []string{`"rejected"`, `"in_review"`, `"in_progress"`, `"done"`} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message %q lacks %s", msg, want)
		}
	}
}

func TestConditionJSONRoundTrip(t *testing.T) {
	raw := []byte(`{"all":[{"field":"rework_count","op":"gte","value":3},{"not":{"field":"labels","op":"exists"}}]}`)
	var c Condition
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCondition(c); err != nil {
		t.Fatalf("decoded condition does not validate: %v", err)
	}
	if !Evaluate(c, Facts{ReworkCount: 3}) {
		t.Fatal("rework 3 with no labels should match")
	}
	if Evaluate(c, Facts{ReworkCount: 3, Labels: []string{"x"}}) {
		t.Fatal("a labelled issue should not match the negated label check")
	}
}
