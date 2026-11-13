package issueworkflow

import (
	"errors"
	"testing"
)

func fcond(field, op string, value any) Condition {
	return Condition{Field: field, Op: op, Value: value}
}

// exprFacts is the fact set of the sample document in its check step.
func exprFacts(t *testing.T) FactSet {
	t.Helper()
	doc := parseSample(t)
	state := State{
		Version: 2, Step: "check",
		Activations: map[string]int{"plan": 1, "research": 1, "draft": 2, "check": 2},
		Loops:       map[string]int{"fix": 1},
		Vars:        map[string]any{"urgent": true, "rounds": 3.0, "note": "rush job", "sources": []any{"a", "b"}},
	}
	tasks := []TaskState{
		rowOf("draft", "write", KindWork, 1, true, "done"),
		rowOf("check", "verify~1", KindGate, 1, true, "ok"),
		rowOf("check", "verify~2", KindGate, 1, true, "fix"),
	}
	fs := BuildFacts(doc, state, tasks, RequestFacts{Priority: "high", Labels: []string{"vip", "eu"}, CreatorType: "agent", ProjectID: "p1"})
	if err := fs.Set("join.result", StringValue("fail")); err != nil {
		t.Fatal(err)
	}
	return fs
}

func TestExpr_TypedOperators(t *testing.T) {
	fs := exprFacts(t)
	tests := map[string]struct {
		c    Condition
		want bool
	}{
		// bool
		"bool eq true":   {fcond("var.urgent", OpEq, true), true},
		"bool eq false":  {fcond("var.urgent", OpEq, false), false},
		"bool neq false": {fcond("var.urgent", OpNeq, false), true},
		// enum
		"enum eq":         {fcond("var.route", OpEq, "fast"), true},
		"enum neq":        {fcond("var.route", OpNeq, "fast"), false},
		"enum in":         {fcond("var.route", OpIn, []any{"full", "fast"}), true},
		"enum in strings": {fcond("var.route", OpIn, []string{"full"}), false},
		"join result eq":  {fcond("join.result", OpEq, "fail"), true},
		"join result in":  {fcond("join.result", OpIn, []any{"pass", "none"}), false},
		// int
		"int eq":           {fcond("var.rounds", OpEq, 3), true},
		"int eq float":     {fcond("var.rounds", OpEq, 3.0), true},
		"int neq":          {fcond("var.rounds", OpNeq, 3), false},
		"int gt":           {fcond("var.rounds", OpGt, 2), true},
		"int gt equal":     {fcond("var.rounds", OpGt, 3), false},
		"int gte":          {fcond("var.rounds", OpGte, 3), true},
		"int lt":           {fcond("var.rounds", OpLt, 3), false},
		"int lte":          {fcond("var.rounds", OpLte, 3), true},
		"int in":           {fcond("var.rounds", OpIn, []any{1.0, 3.0}), true},
		"counter eq":       {fcond("loop.fix.count", OpEq, 1), true},
		"counter lt":       {fcond("loop.fix.count", OpLt, 1), false},
		"join counter":     {fcond("join.pass", OpGte, 1), true},
		"task counter":     {fcond("task.verify.fail", OpEq, 1), true},
		"step activations": {fcond("step.draft.activations", OpEq, 2), true},
		"flow activations": {fcond("flow.activations", OpLte, 6), true},
		// string
		"string eq":           {fcond("var.note", OpEq, "rush job"), true},
		"string neq":          {fcond("var.note", OpNeq, "rush job"), false},
		"string in":           {fcond("var.note", OpIn, []any{"x", "rush job"}), true},
		"string exists":       {Condition{Field: "var.note", Op: OpExists}, true},
		"project eq":          {fcond("request.project_id", OpEq, "p1"), true},
		"project exists":      {Condition{Field: "request.project_id", Op: OpExists}, true},
		"task verdict eq":     {fcond("task.write.verdict", OpEq, "done"), true},
		"task verdict in":     {fcond("task.write.verdict", OpIn, []any{"x", "done"}), true},
		"task verdict exists": {Condition{Field: "task.write.verdict", Op: OpExists}, true},
		// list_string
		"list contains":     {fcond("var.sources", OpContains, "b"), true},
		"list contains not": {fcond("var.sources", OpContains, "c"), false},
		"list exists":       {Condition{Field: "var.sources", Op: OpExists}, true},
		"labels contains":   {fcond("request.labels", OpContains, "vip"), true},
		"labels exists":     {Condition{Field: "request.labels", Op: OpExists}, true},
		// priority is ordered
		"priority eq":  {fcond("request.priority", OpEq, "high"), true},
		"priority gt":  {fcond("request.priority", OpGt, "medium"), true},
		"priority gte": {fcond("request.priority", OpGte, "urgent"), false},
		"priority lt":  {fcond("request.priority", OpLt, "urgent"), true},
		"priority lte": {fcond("request.priority", OpLte, "low"), false},
		"priority in":  {fcond("request.priority", OpIn, []any{"high", "urgent"}), true},
		"creator type": {fcond("request.creator_type", OpEq, "agent"), true},
		// groups
		"all":              {Condition{All: []Condition{fcond("var.urgent", OpEq, true), fcond("var.rounds", OpGte, 3)}}, true},
		"all with a false": {Condition{All: []Condition{fcond("var.urgent", OpEq, true), fcond("var.rounds", OpGt, 3)}}, false},
		"any":              {Condition{Any: []Condition{fcond("var.urgent", OpEq, false), fcond("var.rounds", OpGte, 3)}}, true},
		"any all false":    {Condition{Any: []Condition{fcond("var.urgent", OpEq, false), fcond("var.rounds", OpGt, 3)}}, false},
		"not":              {Condition{Not: &Condition{Field: "var.urgent", Op: OpEq, Value: false}}, true},
		"nested":           {Condition{All: []Condition{{Any: []Condition{fcond("var.urgent", OpEq, false), {Not: &Condition{Field: "var.rounds", Op: OpLt, Value: 3}}}}}}, true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := EvaluateWith(tt.c, fs)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("EvaluateWith(%+v) = %v, want %v", tt.c, got, tt.want)
			}
		})
	}

	// An operator or value that does not fit the type is an error, not false.
	bad := map[string]struct {
		c    Condition
		code string
	}{
		"gt on a bool":          {fcond("var.urgent", OpGt, true), CodeOpNotAllowed},
		"in on a bool":          {fcond("var.urgent", OpIn, []any{true}), CodeOpNotAllowed},
		"contains on a string":  {fcond("var.note", OpContains, "x"), CodeOpNotAllowed},
		"gt on a string":        {fcond("var.note", OpGt, "a"), CodeOpNotAllowed},
		"gt on an enum":         {fcond("var.route", OpGt, "fast"), CodeOpNotAllowed},
		"eq on a list":          {fcond("var.sources", OpEq, "a"), CodeOpNotAllowed},
		"contains on an int":    {fcond("var.rounds", OpContains, 1), CodeOpNotAllowed},
		"exists on a bool":      {Condition{Field: "var.urgent", Op: OpExists}, CodeOpNotAllowed},
		"gt on a verdict":       {fcond("task.write.verdict", OpGt, "a"), CodeOpNotAllowed},
		"a string for an int":   {fcond("var.rounds", OpEq, "3"), CodeConditionInvalid},
		"a fraction for an int": {fcond("var.rounds", OpEq, 2.5), CodeConditionInvalid},
		"an int for a bool":     {fcond("var.urgent", OpEq, 1), CodeConditionInvalid},
		"a value outside enum":  {fcond("var.route", OpEq, "slow"), CodeConditionInvalid},
		"a result outside enum": {fcond("join.result", OpEq, "maybe"), CodeConditionInvalid},
		"a bad priority":        {fcond("request.priority", OpGt, "huge"), CodeConditionInvalid},
		"an empty in list":      {fcond("var.rounds", OpIn, []any{}), CodeConditionInvalid},
		"a bad in item":         {fcond("var.route", OpIn, []any{"fast", 1}), CodeConditionInvalid},
		"exists with a value":   {Condition{Field: "var.note", Op: OpExists, Value: true}, CodeConditionInvalid},
		"contains an int":       {fcond("var.sources", OpContains, 1), CodeConditionInvalid},
		"contains nothing":      {fcond("var.sources", OpContains, ""), CodeConditionInvalid},
		"an empty group":        {Condition{All: []Condition{}}, CodeConditionInvalid},
		"an empty any":          {Condition{Any: []Condition{}}, CodeConditionInvalid},
		"two forms":             {Condition{Field: "var.urgent", Op: OpEq, Value: true, Not: &Condition{Field: "var.urgent", Op: OpEq, Value: true}}, CodeConditionInvalid},
		"no form":               {Condition{}, CodeConditionInvalid},
		"a field without an op": {Condition{Field: "var.urgent"}, CodeOpNotAllowed},
	}
	for name, tt := range bad {
		t.Run("error/"+name, func(t *testing.T) {
			got, err := EvaluateWith(tt.c, fs)
			var ee *ExprError
			if !errors.As(err, &ee) || ee.Code != tt.code {
				t.Fatalf("EvaluateWith(%+v) = %v, %v; want an error %s", tt.c, got, err, tt.code)
			}
			if got {
				t.Fatal("an error must not hold")
			}
		})
	}

	t.Run("depth", func(t *testing.T) {
		leaf := fcond("var.urgent", OpEq, true)
		deep := Condition{Not: &Condition{Not: &Condition{Not: &Condition{Not: &leaf}}}} // 5 levels
		if _, err := EvaluateWith(deep, fs); err == nil {
			t.Fatal("a condition nested 5 levels was evaluated")
		}
		ok := Condition{Not: &Condition{Not: &Condition{Not: &leaf}}} // 4 levels
		if got, err := EvaluateWith(ok, fs); err != nil || got {
			t.Fatalf("a condition nested 4 levels = %v, %v; want false, nil", got, err)
		}
	})
}

func TestExpr_UnknownFactIsAnErrorNotFalse(t *testing.T) {
	fs := exprFacts(t)
	fields := []string{
		"nothing", "var.nothing", "task.ghost.pass", "task.write.colour", "loop.ghost.count", "step.ghost.activations",
		"join.winner", "request.assignee", "priority", "labels",
	}
	for _, field := range fields {
		t.Run(field, func(t *testing.T) {
			got, err := EvaluateWith(fcond(field, OpEq, "x"), fs)
			var ee *ExprError
			if !errors.As(err, &ee) || ee.Code != CodeUnknownFact || ee.Field != field {
				t.Fatalf("EvaluateWith(%q) = %v, %v; want an unknown_fact error", field, got, err)
			}
			if got {
				t.Fatal("an unknown field must not hold")
			}
		})
	}

	t.Run("it is an error wherever it sits", func(t *testing.T) {
		unknown := fcond("nothing", OpEq, "x")
		cases := map[string]Condition{
			"after a false child in all": {All: []Condition{fcond("var.urgent", OpEq, false), unknown}},
			"after a true child in any":  {Any: []Condition{fcond("var.urgent", OpEq, true), unknown}},
			"under not":                  {Not: &unknown},
			"inside a group of a group":  {All: []Condition{{Any: []Condition{unknown}}}},
		}
		for name, c := range cases {
			if _, err := EvaluateWith(c, fs); err == nil {
				t.Errorf("%s: no error", name)
			}
		}
	})

	t.Run("a fact set that declares nothing knows no field", func(t *testing.T) {
		if _, err := EvaluateWith(fcond("flow.activations", OpEq, 0), FactSet{}); err == nil {
			t.Fatal("an empty fact set evaluated a field")
		}
	})

	t.Run("Set refuses an undeclared fact", func(t *testing.T) {
		fs := exprFacts(t)
		if err := fs.Set("nothing", IntValue(1)); err == nil {
			t.Fatal("Set accepted an undeclared fact")
		}
	})
}

// A comparison on a fact that has no value yet is false, and so is its
// negation. Without that, "not (task.docs.verdict eq rejected)" would pass a
// request whose docs task never opened.
func TestExpr_NotOverMissingFactDoesNotLeak(t *testing.T) {
	// The request is in plan: no task has opened, no project, join.result unset.
	doc := parseSample(t)
	fs := BuildFacts(doc, State{Version: 2, Step: "plan", Activations: map[string]int{"plan": 1}}, nil, RequestFacts{})

	holds := func(c Condition) bool {
		t.Helper()
		got, err := EvaluateWith(c, fs)
		if err != nil {
			t.Fatalf("EvaluateWith(%+v): %v", c, err)
		}
		return got
	}
	not := func(c Condition) Condition { return Condition{Not: &c} }

	missing := []Condition{
		fcond("task.verify.verdict", OpEq, "ok"),
		fcond("task.verify.verdict", OpNeq, "ok"),
		fcond("task.verify.verdict", OpIn, []any{"ok", "fix"}),
		fcond("request.project_id", OpEq, "p1"),
		fcond("request.project_id", OpNeq, "p1"),
		fcond("join.result", OpEq, "fail"),
		fcond("join.result", OpNeq, "fail"),
		fcond("lead.verdict", OpEq, "blocked"),
		fcond("request.creator_type", OpEq, "agent"),
		fcond("request.creator_type", OpNeq, "agent"),
	}
	for _, c := range missing {
		if holds(c) {
			t.Errorf("%s %s on a missing fact holds", c.Field, c.Op)
		}
		if holds(not(c)) {
			t.Errorf("not (%s %s) on a missing fact holds: the negation leaks", c.Field, c.Op)
		}
	}

	t.Run("through groups", func(t *testing.T) {
		m := fcond("task.verify.verdict", OpEq, "ok")
		yes := fcond("flow.activations", OpGte, 1)
		no := fcond("flow.activations", OpGte, 99)
		tests := map[string]struct {
			c    Condition
			want bool
		}{
			"not (all missing, true)":  {not(Condition{All: []Condition{m, yes}}), false},
			"not (all missing, false)": {not(Condition{All: []Condition{m, no}}), true}, // false is known
			"not (any missing, false)": {not(Condition{Any: []Condition{m, no}}), false},
			"not (any missing, true)":  {not(Condition{Any: []Condition{m, yes}}), false},
			"any missing, true":        {Condition{Any: []Condition{m, yes}}, true},
			"all missing, true":        {Condition{All: []Condition{m, yes}}, false},
			"not not missing":          {not(not(m)), false},
			"missing and not missing":  {Condition{Any: []Condition{m, not(m)}}, false},
		}
		for name, tt := range tests {
			if got := holds(tt.c); got != tt.want {
				t.Errorf("%s = %v, want %v", name, got, tt.want)
			}
		}
	})

	t.Run("exists is a known answer, so its negation holds", func(t *testing.T) {
		exists := Condition{Field: "task.verify.verdict", Op: OpExists}
		if holds(exists) {
			t.Error("exists on a missing fact holds")
		}
		if !holds(not(exists)) {
			t.Error("not exists on a missing fact should hold: it is known not to exist")
		}
	})
}

func TestExpr_DeclaredButAbsentFact(t *testing.T) {
	doc := parseSample(t)
	// The request has not been to check at all: nothing of that step exists.
	fs := BuildFacts(doc, State{Version: 2, Step: "plan", Activations: map[string]int{"plan": 1}}, nil, RequestFacts{})

	holds := func(c Condition) (bool, error) { return EvaluateWith(c, fs) }

	t.Run("exists is false", func(t *testing.T) {
		for _, field := range []string{"task.verify.verdict", "request.project_id", "request.labels"} {
			got, err := holds(Condition{Field: field, Op: OpExists})
			if err != nil || got {
				t.Errorf("exists %s = %v, %v; want false", field, got, err)
			}
		}
	})
	t.Run("comparisons are false", func(t *testing.T) {
		for _, c := range []Condition{
			fcond("task.verify.verdict", OpEq, "ok"),
			fcond("request.project_id", OpIn, []any{"p1"}),
			fcond("join.result", OpEq, "pass"),
		} {
			if got, err := holds(c); err != nil || got {
				t.Errorf("%s %s = %v, %v; want false", c.Field, c.Op, got, err)
			}
		}
	})
	t.Run("counters are zero", func(t *testing.T) {
		tests := map[string]struct {
			c    Condition
			want bool
		}{
			"task pass eq 0":     {fcond("task.verify.pass", OpEq, 0), true},
			"task fail eq 0":     {fcond("task.verify.fail", OpEq, 0), true},
			"task block eq 0":    {fcond("task.verify.block", OpEq, 0), true},
			"task neutral lte 0": {fcond("task.verify.neutral", OpLte, 0), true},
			"task pass gt 0":     {fcond("task.verify.pass", OpGt, 0), false},
			"task pass gte 1":    {fcond("task.verify.pass", OpGte, 1), false},
			"task pass neq 0":    {fcond("task.verify.pass", OpNeq, 0), false},
			"not task pass gt 0": {Condition{Not: &Condition{Field: "task.verify.pass", Op: OpGt, Value: 0}}, true},
			"join pass eq 0":     {fcond("join.pass", OpEq, 0), true},
			"join total lt 1":    {fcond("join.total", OpLt, 1), true},
			"step never entered": {fcond("step.check.activations", OpEq, 0), true},
		}
		for name, tt := range tests {
			got, err := holds(tt.c)
			if err != nil || got != tt.want {
				t.Errorf("%s = %v, %v; want %v", name, got, err, tt.want)
			}
		}
	})
	t.Run("a declared fact is not an unknown field", func(t *testing.T) {
		if _, err := holds(fcond("task.verify.verdict", OpEq, "ok")); err != nil {
			t.Fatalf("a declared fact without a value is an error: %v", err)
		}
		if _, err := holds(fcond("task.ghost.verdict", OpEq, "ok")); err == nil {
			t.Fatal("a task the document does not have is not a declared fact")
		}
	})
	t.Run("a wrong operand is still an error when the fact is absent", func(t *testing.T) {
		if _, err := holds(fcond("task.verify.pass", OpEq, "zero")); err == nil {
			t.Fatal("a string operand for a counter was accepted")
		}
		if _, err := holds(fcond("join.result", OpEq, "maybe")); err == nil {
			t.Fatal("a value outside the enum was accepted")
		}
	})
}

func TestExpr_SchemaOneEvaluateUnchanged(t *testing.T) {
	// Evaluate keeps treating an unknown field as false: it serves schema 1 and
	// the ordinary workflows.
	if Evaluate(Condition{Field: "nothing", Op: OpEq, Value: "x"}, Facts{}) {
		t.Fatal("Evaluate held for an unknown field")
	}
	if !Evaluate(Condition{Not: &Condition{Field: "nothing", Op: OpEq, Value: "x"}}, Facts{}) {
		t.Fatal("Evaluate: not over a false check should hold in schema 1")
	}
}
