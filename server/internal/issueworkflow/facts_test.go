package issueworkflow

import (
	"reflect"
	"sort"
	"testing"
)

// Every fact of spec section 2.6 and where it comes from. A fact added to the
// registry without a line here fails the test, and so does a line without a fact.
func TestFactRegistry_EverySourceIsDeclared(t *testing.T) {
	want := map[string]string{
		"join.result":            FactSourceEngine,
		"join.pass":              FactSourceEngine,
		"join.fail":              FactSourceEngine,
		"join.block":             FactSourceEngine,
		"join.neutral":           FactSourceEngine,
		"join.open":              FactSourceEngine,
		"join.total":             FactSourceEngine,
		"task.<key>.verdict":     FactSourceEngine,
		"task.<key>.pass":        FactSourceEngine,
		"task.<key>.fail":        FactSourceEngine,
		"task.<key>.block":       FactSourceEngine,
		"task.<key>.neutral":     FactSourceEngine,
		"lead.verdict":           FactSourceEngine,
		"loop.<key>.count":       FactSourceEngine,
		"step.<key>.activations": FactSourceEngine,
		"flow.activations":       FactSourceEngine,
		"request.priority":       FactSourceRequest,
		"request.labels":         FactSourceRequest,
		"request.creator_type":   FactSourceRequest,
		"request.project_id":     FactSourceRequest,
		"var.<key>":              FactSourceVar,
	}
	got := map[string]string{}
	for _, spec := range FactRegistry {
		if _, dup := got[spec.Key]; dup {
			t.Fatalf("fact %q is listed twice", spec.Key)
		}
		got[spec.Key] = spec.Source
		switch spec.Source {
		case FactSourceEngine, FactSourceRequest, FactSourceVar:
		default:
			t.Errorf("fact %q has the source %q; the sources are engine, request and var", spec.Key, spec.Source)
		}
		if spec.Type == "" {
			t.Errorf("fact %q has no type", spec.Key)
		}
		if spec.Source != FactSourceVar && len(spec.Ops) == 0 {
			t.Errorf("fact %q has no operators", spec.Key)
		}
		if spec.Type == VarTypeEnum && len(spec.Values) == 0 {
			t.Errorf("enum fact %q lists no values", spec.Key)
		}
	}
	if !reflect.DeepEqual(got, want) {
		var missing, extra []string
		for k := range want {
			if _, ok := got[k]; !ok {
				missing = append(missing, k)
			}
		}
		for k := range got {
			if _, ok := want[k]; !ok {
				extra = append(extra, k)
			}
		}
		sort.Strings(missing)
		sort.Strings(extra)
		t.Fatalf("the registry and the spec differ; missing %v, extra %v, or a source differs", missing, extra)
	}

	t.Run("every fact a document expands to has a declared source", func(t *testing.T) {
		doc := parseSample(t)
		expanded := ExpandFacts(doc)
		seen := map[string]bool{}
		for _, spec := range expanded {
			if seen[spec.Key] {
				t.Fatalf("expanded fact %q twice", spec.Key)
			}
			seen[spec.Key] = true
			if spec.Source != FactSourceEngine && spec.Source != FactSourceRequest && spec.Source != FactSourceVar {
				t.Errorf("%q has the source %q", spec.Key, spec.Source)
			}
		}
		for _, key := range []string{"task.collect.pass", "task.verify.verdict", "loop.fix.count", "step.check.activations", "var.rounds", "var.sources", "join.result", "flow.activations"} {
			if !seen[key] {
				t.Errorf("the sample document does not expand to %q", key)
			}
		}
		if seen["var.nothing"] || seen["task.<key>.pass"] {
			t.Fatal("a placeholder or an undeclared variable was expanded")
		}
	})
}

func TestFactRegistry_RejectsLegacyNamesInV2(t *testing.T) {
	doc := parseSample(t)
	legacy := []string{"tasks_outcome", "rework_count", "outcome", "switch.customs_lookup", "switch.route"}
	for _, field := range legacy {
		_, issue := LookupFact(field, doc.Vars)
		if issue == nil || issue.Code != CodeLegacyFieldInV2 {
			t.Errorf("LookupFact(%q) = %+v, want %s", field, issue, CodeLegacyFieldInV2)
		}
		fs := BuildFacts(doc, State{Step: "plan"}, nil, RequestFacts{})
		_, err := EvaluateWith(Condition{Field: field, Op: OpEq, Value: "x"}, fs)
		if e, ok := err.(*ExprError); !ok || e.Code != CodeLegacyFieldInV2 {
			t.Errorf("EvaluateWith(%q) error = %v, want %s", field, err, CodeLegacyFieldInV2)
		}
	}

	for _, field := range []string{"join.result", "task.collect.pass", "loop.fix.count", "var.rounds", "request.labels", "lead.verdict"} {
		spec, issue := LookupFact(field, doc.Vars)
		if issue != nil || spec.Key != field {
			t.Errorf("LookupFact(%q) = %+v, %+v", field, spec, issue)
		}
	}
	if spec, _ := LookupFact("var.rounds", doc.Vars); spec.Type != VarTypeInt || !hasOp(spec.Ops, OpGte) {
		t.Errorf("var.rounds = %+v, want an int fact with ordering operators", spec)
	}
	if spec, _ := LookupFact("var.sources", doc.Vars); spec.Type != VarTypeListString || !hasOp(spec.Ops, OpContains) {
		t.Errorf("var.sources = %+v, want a list fact with contains", spec)
	}
	unknown := []string{"priority", "labels", "project_id", "creator_type", "assignee_type", "join", "join.winner", "task.docs", "task.Docs.pass",
		"task.docs.pass.extra", "var.nothing", "var.", "request", "loop.fix", "bogus", ""}
	for _, field := range unknown {
		if _, issue := LookupFact(field, doc.Vars); issue == nil || issue.Code != CodeUnknownFact {
			t.Errorf("LookupFact(%q) issue = %+v, want %s", field, issue, CodeUnknownFact)
		}
	}
}

func rowOf(step, key, kind string, round int, closed bool, outcome string) TaskState {
	return TaskState{Step: step, Key: key, Kind: kind, Round: round, Closed: closed, Outcome: outcome}
}

func factInt(t *testing.T, fs FactSet, key string, want int) {
	t.Helper()
	v, ok := fs.Value(key)
	if !ok || v.Int != want {
		t.Errorf("%s = %+v (has a value: %v), want %d", key, v, ok, want)
	}
}

func factStr(t *testing.T, fs FactSet, key, want string) {
	t.Helper()
	v, ok := fs.Value(key)
	if !ok || v.Str != want {
		t.Errorf("%s = %+v (has a value: %v), want %q", key, v, ok, want)
	}
}

func factNone(t *testing.T, fs FactSet, key string) {
	t.Helper()
	if _, declared := fs.Spec(key); !declared {
		t.Errorf("%s is not declared", key)
	}
	if v, ok := fs.Value(key); ok {
		t.Errorf("%s has the value %+v, want none", key, v)
	}
}

func TestBuildFacts_FromServerStateOnly(t *testing.T) {
	doc := parseSample(t)
	state := State{
		Version: 2, Step: "check", Seq: 9,
		Activations: map[string]int{"plan": 1, "research": 1, "draft": 2, "check": 2},
		Loops:       map[string]int{"fix": 1},
		Vars:        map[string]any{"urgent": true, "rounds": float64(3), "sources": []any{"a", "b"}},
	}
	tasks := []TaskState{
		// The first entry into check: ignored, the request is in its second.
		rowOf("check", "verify~1", KindGate, 0, true, "fix"),
		rowOf("check", "verify~2", KindGate, 0, true, "ok"),
		// The current entry (activations 2 means round 1).
		rowOf("check", "verify~1", KindGate, 1, true, "ok"),
		rowOf("check", "verify~2", KindGate, 1, true, "fix"),
		rowOf("check", "second", KindGate, 1, false, ""),
		// Earlier steps: the latest entry counts.
		rowOf("draft", "write", KindWork, 0, true, "done"),
		rowOf("draft", "write", KindWork, 1, true, "done"),
		rowOf("research", "collect", KindWork, 0, true, "done"),
	}
	req := RequestFacts{Priority: "high", Labels: []string{"vip", "eu"}, CreatorType: "member", ProjectID: "p1"}
	fs := BuildFacts(doc, state, tasks, req)

	// engine facts
	factInt(t, fs, "join.pass", 1)
	factInt(t, fs, "join.fail", 1)
	factInt(t, fs, "join.block", 0)
	factInt(t, fs, "join.neutral", 0)
	factInt(t, fs, "join.open", 1)
	factInt(t, fs, "join.total", 3)
	factInt(t, fs, "task.verify.pass", 1)
	factInt(t, fs, "task.verify.fail", 1)
	factNone(t, fs, "task.verify.verdict") // two instances: the verdict is not one value
	factInt(t, fs, "task.second.pass", 0)
	factNone(t, fs, "task.second.verdict") // still open
	factInt(t, fs, "task.write.pass", 1)
	factStr(t, fs, "task.write.verdict", "done")
	factStr(t, fs, "task.collect.verdict", "done")
	factInt(t, fs, "loop.fix.count", 1)
	factInt(t, fs, "loop.retry.count", 0)
	factInt(t, fs, "step.draft.activations", 2)
	factInt(t, fs, "step.release.activations", 0)
	factInt(t, fs, "flow.activations", 6)
	factNone(t, fs, "join.result") // the join algorithm sets it
	factNone(t, fs, "lead.verdict")

	// request facts
	factStr(t, fs, "request.priority", "high")
	factStr(t, fs, "request.creator_type", "member")
	factStr(t, fs, "request.project_id", "p1")
	if v, _ := fs.Value("request.labels"); !reflect.DeepEqual(v.List, []string{"vip", "eu"}) {
		t.Errorf("request.labels = %v", v.List)
	}

	// var facts: the stored value, or the default of the document
	if v, _ := fs.Value("var.urgent"); !v.Bool {
		t.Error("var.urgent should be true")
	}
	factInt(t, fs, "var.rounds", 3)
	factStr(t, fs, "var.route", "fast")
	if v, _ := fs.Value("var.sources"); !reflect.DeepEqual(v.List, []string{"a", "b"}) {
		t.Errorf("var.sources = %v", v.List)
	}
	if spec, _ := fs.Spec("var.rounds"); spec.Source != FactSourceVar || spec.Type != VarTypeInt {
		t.Errorf("var.rounds spec = %+v", spec)
	}

	t.Run("a task that never opened has no value and counts as zero", func(t *testing.T) {
		fs := BuildFacts(doc, State{Version: 2, Step: "plan", Activations: map[string]int{"plan": 1}}, nil, RequestFacts{})
		factNone(t, fs, "task.write.verdict")
		factNone(t, fs, "task.write.pass")
		factNone(t, fs, "request.project_id")
		factStr(t, fs, "request.priority", "none")
		factInt(t, fs, "join.total", 0)
	})

	t.Run("a stored value of the wrong type gives no fact", func(t *testing.T) {
		fs := BuildFacts(doc, State{Step: "plan", Vars: map[string]any{"rounds": "three", "urgent": 1.0}}, nil, RequestFacts{})
		factNone(t, fs, "var.rounds")
		factNone(t, fs, "var.urgent")
	})

	t.Run("the state, the rows and the request are not changed", func(t *testing.T) {
		before := state
		beforeVars := map[string]any{"urgent": true, "rounds": float64(3), "sources": []any{"a", "b"}}
		_ = BuildFacts(doc, state, tasks, req)
		if !reflect.DeepEqual(state.Vars, beforeVars) || !reflect.DeepEqual(state.Activations, before.Activations) {
			t.Fatalf("BuildFacts changed the state: %+v", state)
		}
		again := BuildFacts(doc, state, tasks, req)
		if !reflect.DeepEqual(again, fs) {
			t.Fatal("BuildFacts is not a pure function of its arguments")
		}
	})

	t.Run("it takes no text", func(t *testing.T) {
		typ := reflect.TypeOf(BuildFacts)
		for i := 0; i < typ.NumIn(); i++ {
			switch typ.In(i).Kind() {
			case reflect.String, reflect.Interface, reflect.Map:
				t.Errorf("parameter %d of BuildFacts is a %s: facts come from structured server state", i, typ.In(i))
			}
		}
		for _, typ := range []reflect.Type{reflect.TypeOf(TaskState{}), reflect.TypeOf(RequestFacts{})} {
			for i := 0; i < typ.NumField(); i++ {
				if n := typ.Field(i).Name; n == "Comment" || n == "Description" || n == "Content" || n == "Title" || n == "Text" {
					t.Errorf("%s.%s would carry text into a fact", typ.Name(), n)
				}
			}
		}
	})
}

func TestBuildFacts_TaskClasses(t *testing.T) {
	doc := parseSample(t)
	step, _ := NewGraph(doc).Step("check")
	tests := map[string]struct {
		row     TaskState
		class   string
		closed  bool
		counted bool
	}{
		"open":                         {TaskState{Key: "verify~1", Kind: KindGate}, "", false, true},
		"pass verdict":                 {TaskState{Key: "verify~1", Kind: KindGate, Closed: true, Outcome: "ok"}, ClassPass, true, true},
		"fail verdict":                 {TaskState{Key: "verify~1", Kind: KindGate, Closed: true, Outcome: "fix"}, ClassFail, true, true},
		"neutral verdict":              {TaskState{Key: "verify~1", Kind: KindGate, Closed: true, Outcome: "skip"}, ClassNeutral, true, true},
		"blocked":                      {TaskState{Key: "verify~1", Kind: KindGate, Closed: true, Outcome: "blocked"}, ClassBlock, true, true},
		"a verdict outside the set":    {TaskState{Key: "verify~1", Kind: KindGate, Closed: true, Outcome: "approved"}, ClassFail, true, true},
		"gate closed with no verdict":  {TaskState{Key: "verify~1", Kind: KindGate, Closed: true}, ClassFail, true, true},
		"gate cancelled by a person":   {TaskState{Key: "verify~1", Kind: KindGate, Closed: true, Cancelled: true}, ClassFail, true, true},
		"superseded is ignored":        {TaskState{Key: "verify~1", Kind: KindGate, Closed: true, Cancelled: true, Outcome: OutcomeSuperseded}, "", false, false},
		"an open task superseded":      {TaskState{Key: "verify~1", Kind: KindGate, Outcome: OutcomeSuperseded}, "", false, false},
		"work cancelled adds no class": {TaskState{Key: "write", Kind: KindWork, Closed: true, Cancelled: true}, "", true, false},
		"work done":                    {TaskState{Key: "write", Kind: KindWork, Closed: true, Outcome: "done"}, ClassPass, true, true},
		"work done with no outcome":    {TaskState{Key: "write", Kind: KindWork, Closed: true}, ClassPass, true, true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			class, closed, counted := docTaskClass(doc, step, tt.row)
			if class != tt.class || closed != tt.closed || counted != tt.counted {
				t.Fatalf("class %q closed %v counted %v, want %q %v %v", class, closed, counted, tt.class, tt.closed, tt.counted)
			}
		})
	}

	t.Run("a superseded task does not count in the join", func(t *testing.T) {
		state := State{Step: "check", Activations: map[string]int{"check": 1}}
		fs := BuildFacts(doc, state, []TaskState{
			rowOf("check", "verify~1", KindGate, 0, true, "ok"),
			{Step: "check", Key: "verify~2", Kind: KindGate, Round: 0, Closed: true, Cancelled: true, Outcome: OutcomeSuperseded},
		}, RequestFacts{})
		factInt(t, fs, "join.total", 1)
		factInt(t, fs, "join.pass", 1)
		factInt(t, fs, "task.verify.pass", 1)
		factStr(t, fs, "task.verify.verdict", "ok")
	})
}
