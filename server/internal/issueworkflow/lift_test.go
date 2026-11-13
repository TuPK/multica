package issueworkflow

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func edgeIDs(edges []GraphEdge) []string {
	var ids []string
	for _, e := range edges {
		ids = append(ids, e.Edge.ID)
	}
	return ids
}

func mustLift(t *testing.T, def Definition, opts LiftOpts) Graph {
	t.Helper()
	g, err := Lift(def, opts)
	if err != nil {
		t.Fatalf("Lift: %v", err)
	}
	return g
}

func joinIs(v string) *Condition { return cond("join.result", OpEq, v) }

func TestLift_StopSplit(t *testing.T) {
	g := mustLift(t, tradeFlow(), LiftOpts{Runtime: true})
	blockCond := Condition{Field: "join.result", Op: OpEq, Value: "block"}

	type want struct {
		id, to, stop, loop string
		when               *Condition
	}
	tests := map[string]struct {
		step  string
		edges []want
	}{
		"a gate that has a block edge and a default to blocked": {"gate", []want{
			{id: "blocked", stop: StopGroundsMissing, when: joinIs("block")},
			{id: "pass", to: "handover", when: joinIs("pass")},
			{id: "redo", to: "draft", loop: LoopRework, when: &Condition{All: []Condition{*joinIs("fail"), *cond("loop.rework.count", OpLt, 2)}}},
			{id: "default_block", stop: StopGroundsMissing, when: &blockCond},
			{id: "default", stop: StopLoopExhausted},
		}},
		"the handover has no block edge": {"handover", []want{
			{id: "pass", to: "in_review", when: joinIs("pass")},
			{id: "default_block", stop: StopGroundsMissing, when: &blockCond},
			{id: "default", stop: StopLoopExhausted},
		}},
		"a step whose only edge to blocked already implies block is not split": {"draft", []want{
			{id: "blocked", stop: StopGroundsMissing, when: joinIs("block")},
			{id: "default", to: "gate"},
		}},
		"the todo step": {"todo", []want{
			{id: "blocked", stop: StopGroundsMissing, when: cond("lead.verdict", OpEq, OutcomeBlocked)},
			{id: "default", to: "collect"},
		}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			edges := g.EdgesFrom(tt.step)
			if len(edges) != len(tt.edges) {
				t.Fatalf("edges = %v, want %d", edgeIDs(edges), len(tt.edges))
			}
			for i, w := range tt.edges {
				e := edges[i].Edge
				if e.ID != w.id || e.To != w.to || e.Stop != w.stop || e.Loop != w.loop || !reflect.DeepEqual(e.When, w.when) {
					t.Errorf("edge %d = %+v (when %+v), want %+v (when %+v)", i, e, e.When, w, w.when)
				}
				if (w.loop != "") != edges[i].Back {
					t.Errorf("edge %s: Back = %v", w.id, edges[i].Back)
				}
			}
		})
	}

	t.Run("the split edge keeps the label and sits right before the original", func(t *testing.T) {
		edges := g.EdgesFrom("handover")
		if edges[1].Edge.Label != edges[2].Edge.Label || edges[1].Edge.Label != "default" {
			t.Fatalf("labels = %q, %q", edges[1].Edge.Label, edges[2].Edge.Label)
		}
	})

	t.Run("the condition of the original edge is carried into the split", func(t *testing.T) {
		def := Definition{Name: "x", Flow: &FlowConfig{Version: 1}, Steps: []Step{
			{StatusKey: "todo", Transitions: []Transition{tr("default", nil, "work", false)}},
			{StatusKey: "work", Tasks: []Task{{Key: "t", Title: "T", Kind: KindGate, Handler: agentHandler(agentDocs), Requires: []string{RequireOutcome}}},
				Transitions: []Transition{
					tr("a", tasksIs(TasksAnyRejected), "blocked", false),
					tr("b", &Condition{All: []Condition{*tasksIs(TasksAnyRejected), *reworkBelow(3)}}, "blocked", false),
					tr("c", &Condition{Any: []Condition{*tasksIs(TasksAnyBlocked), *tasksIs(TasksAnyRejected)}}, "blocked", false),
					tr("d", &Condition{Any: []Condition{*tasksIs(TasksAnyBlocked), *tasksIs(TasksAnyBlocked)}}, "blocked", false),
					tr("e", &Condition{All: []Condition{*tasksIs(TasksAnyRejected), *tasksIs(TasksAnyBlocked)}}, "blocked", false),
					tr("f", &Condition{Field: FieldTasksOutcome, Op: OpIn, Value: []any{TasksAnyBlocked}}, "blocked", false),
					tr("g", &Condition{Not: tasksIs(TasksAnyBlocked)}, "blocked", false),
				}},
			{StatusKey: "blocked"}, {StatusKey: "done"},
		}}
		g := mustLift(t, def, LiftOpts{})
		var ids []string
		stops := map[string]string{}
		for _, e := range g.EdgesFrom("work") {
			ids = append(ids, e.Edge.ID)
			stops[e.Edge.ID] = e.Edge.Stop
		}
		wantIDs := []string{"a_block", "a", "b_block", "b", "c_block", "c", "d", "e", "f", "g_block", "g"}
		if !reflect.DeepEqual(ids, wantIDs) {
			t.Fatalf("edges = %v, want %v", ids, wantIDs)
		}
		for _, id := range []string{"a_block", "b_block", "c_block", "d", "e", "f", "g_block"} {
			if stops[id] != StopGroundsMissing {
				t.Errorf("%s stops with %q, want grounds_missing", id, stops[id])
			}
		}
		for _, id := range []string{"a", "b", "c", "g"} {
			if stops[id] != StopLoopExhausted {
				t.Errorf("%s stops with %q, want loop_exhausted", id, stops[id])
			}
		}
		// all: the block test joins the group instead of nesting it.
		b, _ := g.Edge("work", "b_block")
		if len(b.Edge.When.All) != 3 || !reflect.DeepEqual(b.Edge.When.All[2], blockCond) {
			t.Errorf("b_block when = %+v, want the all group with join.result block appended", b.Edge.When)
		}
		a, _ := g.Edge("work", "a_block")
		if len(a.Edge.When.All) != 2 || !reflect.DeepEqual(a.Edge.When.All[1], blockCond) {
			t.Errorf("a_block when = %+v", a.Edge.When)
		}
	})

	t.Run("a split id never replaces an id schema 1 gave", func(t *testing.T) {
		def := Definition{Name: "x", Flow: &FlowConfig{Version: 1}, Steps: []Step{
			{StatusKey: "todo", Transitions: []Transition{tr("default", nil, "work", false)}},
			{StatusKey: "work", Tasks: []Task{{Key: "t", Title: "T", Kind: KindWork, Handler: agentHandler(agentDocs)}},
				Transitions: []Transition{
					tr("end", tasksIs(TasksNone), "blocked", false),
					tr("end_block", nil, "done", false),
				}},
			{StatusKey: "blocked"}, {StatusKey: "done"},
		}}
		g := mustLift(t, def, LiftOpts{})
		if got, want := edgeIDs(g.EdgesFrom("work")), []string{"end_block_2", "end", "end_block"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("edges = %v, want %v", got, want)
		}
	})
}

func TestLift_ConditionRewrite(t *testing.T) {
	cfg := &FlowConfig{Version: 1, Switches: []Switch{
		{Key: "flag", Type: SwitchBool, Default: false},
		{Key: "kind", Type: SwitchEnum, Values: []string{"a", "b"}, Default: "a"},
	}}
	rewrite := func(step string, c Condition) (Condition, error) { return rewriteCondition(c, step, cfg) }

	tests := map[string]struct {
		step string
		in   Condition
		want Condition
	}{
		"all_approved": {"gate", *tasksIs(TasksAllApproved), *joinIs("pass")},
		"any_rejected": {"gate", *tasksIs(TasksAnyRejected), *joinIs("fail")},
		"any_blocked":  {"gate", *tasksIs(TasksAnyBlocked), *joinIs("block")},
		"none":         {"gate", *tasksIs(TasksNone), *joinIs("none")},
		"tasks_outcome in": {"gate", Condition{Field: FieldTasksOutcome, Op: OpIn, Value: []any{TasksAllApproved, TasksNone}},
			Condition{Field: "join.result", Op: OpIn, Value: []any{"pass", "none"}}},
		"rework_count lt":  {"gate", *reworkBelow(2), *cond("loop.rework.count", OpLt, 2)},
		"rework_count gte": {"gate", *cond(FieldReworkCount, OpGte, 1), *cond("loop.rework.count", OpGte, 1)},
		"rework_count in": {"gate", *cond(FieldReworkCount, OpIn, []any{0.0, 2.0}),
			Condition{Any: []Condition{*cond("loop.rework.count", OpEq, 0.0), *cond("loop.rework.count", OpEq, 2.0)}}},
		"outcome at todo":    {"todo", *cond(FieldOutcome, OpEq, OutcomeBlocked), *cond("lead.verdict", OpEq, OutcomeBlocked)},
		"outcome in at todo": {"todo", *cond(FieldOutcome, OpIn, []any{"blocked"}), *cond("lead.verdict", OpIn, []any{"blocked"})},
		"switch bool eq":     {"collect", *cond(SwitchField("flag"), OpEq, true), *cond("var.flag", OpEq, true)},
		"switch bool neq":    {"collect", *cond(SwitchField("flag"), OpNeq, true), *cond("var.flag", OpNeq, true)},
		"switch enum in":     {"collect", *cond(SwitchField("kind"), OpIn, []any{"a", "b"}), *cond("var.kind", OpIn, []any{"a", "b"})},
		"switch bool in":     {"collect", *cond(SwitchField("flag"), OpIn, []any{true}), Condition{Any: []Condition{*cond("var.flag", OpEq, true)}}},
		"groups are rewritten": {"gate", Condition{All: []Condition{*tasksIs(TasksAnyRejected), {Not: cond(SwitchField("flag"), OpEq, true)}}},
			Condition{All: []Condition{*joinIs("fail"), {Not: cond("var.flag", OpEq, true)}}}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			before, _ := json.Marshal(tt.in)
			got, err := rewrite(tt.step, tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v\nwant %+v", got, tt.want)
			}
			if after, _ := json.Marshal(tt.in); string(before) != string(after) {
				t.Fatalf("the input was changed: %s -> %s", before, after)
			}
		})
	}

	// A schema 1 field with no schema 2 fact is an error, not a guess.
	for name, tt := range map[string]struct {
		step string
		in   Condition
	}{
		"outcome away from todo": {"gate", *cond(FieldOutcome, OpEq, OutcomeBlocked)},
		"outcome exists":         {"todo", Condition{Field: FieldOutcome, Op: OpExists}},
		"undeclared switch":      {"collect", *cond(SwitchField("nope"), OpEq, true)},
		"priority":               {"collect", *cond(FieldPriority, OpEq, "high")},
		"assignee type":          {"collect", *cond(FieldAssigneeType, OpEq, "agent")},
		"tasks_outcome neq":      {"gate", Condition{Field: FieldTasksOutcome, Op: OpNeq, Value: TasksNone}},
		"unknown outcome value":  {"gate", *tasksIs("sideways")},
		"bad nested field":       {"gate", Condition{Not: cond(FieldPriority, OpEq, "high")}},
	} {
		t.Run("error/"+name, func(t *testing.T) {
			if got, err := rewrite(tt.step, tt.in); err == nil {
				t.Fatalf("got %+v, want an error", got)
			}
		})
	}

	t.Run("a lift that cannot rewrite a condition fails", func(t *testing.T) {
		def := tradeFlow()
		def.Steps[1].Tasks[1].When = cond(FieldPriority, OpEq, "high")
		if _, err := Lift(def, LiftOpts{}); err == nil || !strings.Contains(err.Error(), "customs") {
			t.Fatalf("err = %v, want one naming the task", err)
		}
	})

	t.Run("every condition of every catalog flow is evaluable", func(t *testing.T) {
		for id, f := range loadCatalogFlows(t) {
			def, _, _ := f.definition()
			g := mustLift(t, def, LiftOpts{Runtime: true})
			for _, s := range g.Doc.Steps {
				fs := BuildFacts(g.Doc, State{Step: s.Key, Activations: map[string]int{s.Key: 1}}, nil, RequestFacts{})
				var conds []*Condition
				for _, task := range s.Tasks {
					conds = append(conds, task.When)
				}
				for _, e := range s.Edges {
					conds = append(conds, e.When)
				}
				for _, c := range conds {
					if c == nil {
						continue
					}
					if _, err := EvaluateWith(*c, fs); err != nil {
						t.Errorf("%s/%s: %+v: %v", id, s.Key, *c, err)
					}
				}
			}
		}
	})
}

func lifted(t *testing.T, def Definition) Loop {
	t.Helper()
	g := mustLift(t, def, LiftOpts{})
	if len(g.Doc.Loops) != 1 {
		t.Fatalf("loops = %+v, want one", g.Doc.Loops)
	}
	return g.Doc.Loops[0]
}

// gateFlow is a flow whose gate step has rework edges to the targets given,
// each with its own limit.
func gateFlow(edges ...Transition) Definition {
	agent := func(key, id string) Task {
		return Task{Key: key, Title: key, Kind: KindWork, Handler: agentHandler(id)}
	}
	gate := Task{Key: "g", Title: "g", Kind: KindGate, Handler: agentHandler(agentDocsGate), Requires: []string{RequireOutcome}}
	return Definition{Name: "x", Flow: &FlowConfig{Version: 1}, Steps: []Step{
		{StatusKey: "todo", Transitions: []Transition{tr("default", nil, "one", false)}},
		{StatusKey: "one", Tasks: []Task{agent("a", agentDocs)}, Transitions: []Transition{tr("default", nil, "two", false)}},
		{StatusKey: "two", Tasks: []Task{agent("b", agentCustoms)}, Transitions: []Transition{tr("default", nil, "gate", false)}},
		{StatusKey: "gate", Tasks: []Task{gate}, Transitions: append(edges, tr("default", nil, "done", false))},
		{StatusKey: "done"},
	}}
}

func TestLift_ReworkLoopMaxIsLargestN(t *testing.T) {
	redo := func(id, to string, n int) Transition {
		return tr(id, &Condition{All: []Condition{*tasksIs(TasksAnyRejected), *reworkBelow(n)}}, to, true)
	}
	tests := map[string]struct {
		def  Definition
		want int
	}{
		"one edge":       {gateFlow(redo("r", "two", 2)), 2},
		"the largest":    {gateFlow(redo("r1", "two", 2), redo("r2", "two", 4), redo("r3", "two", 3)), 4},
		"at the maximum": {gateFlow(redo("r", "two", MaxReworkLimit)), MaxReworkLimit},
		"capped":         {gateFlow(redo("r", "two", 9)), MaxReworkLimit},
		"alone":          {gateFlow(tr("r", reworkBelow(1), "one", true)), 1},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			l := lifted(t, tt.def)
			if l.Key != LoopRework || l.Max != (LoopMax{N: tt.want}) {
				t.Fatalf("loop = %+v, want max %d", l, tt.want)
			}
		})
	}

	t.Run("no rework edge, no loop", func(t *testing.T) {
		if g := mustLift(t, gateFlow(), LiftOpts{}); len(g.Doc.Loops) != 0 {
			t.Fatalf("loops = %+v", g.Doc.Loops)
		}
	})
	t.Run("a rework edge without a limit cannot be lifted", func(t *testing.T) {
		if _, err := Lift(gateFlow(tr("r", tasksIs(TasksAnyRejected), "two", true)), LiftOpts{}); err == nil {
			t.Fatal("no error")
		}
	})
	t.Run("the trade flow", func(t *testing.T) {
		l := lifted(t, tradeFlow())
		if l.Header != "draft" || l.Max.N != 2 || l.EffectiveReset() != LoopResetOnEntry {
			t.Fatalf("loop = %+v, want header draft, max 2", l)
		}
	})
}

func TestLift_SingleCounterWithTwoReworkEdges(t *testing.T) {
	redo := func(id, to string, n int) Transition {
		return tr(id, &Condition{All: []Condition{*tasksIs(TasksAnyRejected), *reworkBelow(n)}}, to, true)
	}

	t.Run("two edges to one target share the header", func(t *testing.T) {
		g := mustLift(t, gateFlow(redo("r1", "two", 2), redo("r2", "two", 3)), LiftOpts{})
		if len(g.Doc.Loops) != 1 {
			t.Fatalf("loops = %+v, want the one rework counter", g.Doc.Loops)
		}
		l := g.Doc.Loops[0]
		if l.Header != "two" || l.Max.N != 3 || l.EffectiveReset() != LoopResetOnEntry {
			t.Fatalf("loop = %+v", l)
		}
		if got := len(g.BackEdgesOf(LoopRework)); got != 2 {
			t.Fatalf("back-edges = %d, want 2", got)
		}
	})

	t.Run("two edges to different targets have no header and never reset", func(t *testing.T) {
		g := mustLift(t, gateFlow(redo("r1", "one", 2), redo("r2", "two", 3)), LiftOpts{})
		if len(g.Doc.Loops) != 1 {
			t.Fatalf("loops = %+v, want one", g.Doc.Loops)
		}
		l := g.Doc.Loops[0]
		if l.Header != "" || l.Reset != LoopResetNever || l.Max.N != 3 {
			t.Fatalf("loop = %+v, want no header, reset never, max 3", l)
		}
		// Each edge keeps its own limit, so r1 still stops at 2 rounds.
		r1, _ := g.Edge("gate", "r1")
		r2, _ := g.Edge("gate", "r2")
		if !reflect.DeepEqual(r1.Edge.When.All[1], *cond("loop.rework.count", OpLt, 2)) || !reflect.DeepEqual(r2.Edge.When.All[1], *cond("loop.rework.count", OpLt, 3)) {
			t.Fatalf("limits: r1 %+v, r2 %+v", r1.Edge.When, r2.Edge.When)
		}
		if r1.Edge.Loop != LoopRework || r2.Edge.Loop != LoopRework {
			t.Fatal("the edges are not on the rework loop")
		}
		if body := g.LoopBody(LoopRework); len(body) != 0 {
			t.Fatalf("a loop without a header has body %v", body)
		}
	})
}

func TestLift_RunsRoundsLikeV1(t *testing.T) {
	runtime := mustLift(t, tradeFlow(), LiftOpts{Runtime: true})
	if runtime.Compat != CompatV1 {
		t.Fatalf("Compat = %q, want v1", runtime.Compat)
	}
	if fork := mustLift(t, tradeFlow(), LiftOpts{}); fork.Compat != CompatNone {
		t.Fatalf("a fork has Compat %q", fork.Compat)
	}
	for _, v1 := range []FlowState{
		{Step: "gate"},
		{Step: "gate", Rework: 2},
		{Step: "gate", RoundBase: 3},
		{Step: "draft", Rework: 1, RoundBase: 3},
	} {
		if got, want := Adapt(v1).Round(), v1.Round(); got != want {
			t.Errorf("Adapt(%+v).Round() = %d, want the schema 1 round %d", v1, got, want)
		}
	}
	// The rework counter the lifted loop reads is the one schema 1 kept.
	s := Adapt(FlowState{Step: "gate", Rework: 2})
	fs := BuildFacts(runtime.Doc, s, nil, RequestFacts{})
	if ok, err := EvaluateWith(*cond("loop.rework.count", OpEq, 2), fs); err != nil || !ok {
		t.Fatalf("loop.rework.count = 2: %v, %v", ok, err)
	}
}

func TestLift_InputsByPositionLikeV1(t *testing.T) {
	// Schema 1 has no inputs, sets or skills: a task reads the step before it,
	// which a compat v1 graph keeps by position. The lift adds none of the new
	// fields, in either mode.
	flows := loadCatalogFlows(t)
	if len(flows) == 0 {
		t.Skip("the catalog is not built")
	}
	for id, f := range flows {
		def, _, _ := f.definition()
		for _, runtime := range []bool{true, false} {
			g := mustLift(t, def, LiftOpts{Runtime: runtime})
			for _, s := range g.Doc.Steps {
				for _, task := range s.Tasks {
					if len(task.Inputs) > 0 || len(task.Sets) > 0 || len(task.Skills) > 0 || len(task.After) > 0 || task.Instances != nil || task.Deadline != nil {
						t.Errorf("%s/%s/%s carries a schema 2 field: %+v", id, s.Key, task.Key, task)
					}
				}
			}
			if (g.Compat == CompatV1) != runtime {
				t.Errorf("%s: Compat %q with Runtime %v", id, g.Compat, runtime)
			}
		}
	}
}

// manifestRoleKeys rebuilds what catalogFlow.definition did: the agent id given
// to each manifest role, in the order the roles first appear.
func manifestRoleKeys(f catalogFlow) (keys map[string]string, order []string) {
	keys = map[string]string{}
	seen := map[string]bool{}
	for _, s := range f.Steps {
		for _, task := range s.Tasks {
			if seen[task.Role] {
				continue
			}
			seen[task.Role] = true
			order = append(order, task.Role)
			keys[fmt.Sprintf("00000000-0000-4000-8000-%012d", len(order))] = task.Role
		}
	}
	return keys, order
}

func TestLift_ManifestKeepsRoleKeys(t *testing.T) {
	flows := loadCatalogFlows(t)
	if len(flows) == 0 {
		t.Skip("the catalog is not built")
	}
	for id, f := range flows {
		t.Run(id, func(t *testing.T) {
			def, _, _ := f.definition()
			roleKeys, order := manifestRoleKeys(f)
			g := mustLift(t, def, LiftOpts{RoleKeys: roleKeys})
			var got []string
			for _, r := range g.Doc.Roles {
				got = append(got, r.Key)
				if r.Kind != RoleKindAgent || r.Bind == nil || len(r.Bind.AgentIDs) != 1 {
					t.Errorf("role %+v is not one bound agent", r)
				}
			}
			if !reflect.DeepEqual(got, order) {
				t.Fatalf("roles = %v, want the manifest roles %v", got, order)
			}
			for _, s := range f.Steps {
				step, _ := g.Step(s.StatusKey)
				for i, task := range s.Tasks {
					if step.Tasks[i].Role != task.Role || step.Tasks[i].Key != task.Key {
						t.Errorf("%s: task %s has role %q, want %q", s.StatusKey, task.Key, step.Tasks[i].Role, task.Role)
					}
				}
			}
		})
	}

	t.Run("without a manifest the key is built from the agent id", func(t *testing.T) {
		g := mustLift(t, tradeFlow(), LiftOpts{})
		var got []string
		for _, r := range g.Doc.Roles {
			got = append(got, r.Key)
		}
		// The test agents all start with 00000000, so the keys are numbered.
		want := []string{"a_00000000", "a_00000000_2", "a_00000000_3", "a_00000000_4", "a_00000000_5", "a_00000000_6"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("roles = %v, want %v", got, want)
		}
		def := tradeFlow()
		def.Steps[1].Tasks[0].Handler.ID = "9f8e7d6c-1111-4222-8333-444455556666"
		g = mustLift(t, def, LiftOpts{})
		if g.Doc.Roles[0].Key != "a_9f8e7d6c" || g.Doc.Roles[0].Bind.AgentIDs[0] != "9f8e7d6c-1111-4222-8333-444455556666" {
			t.Fatalf("role = %+v", g.Doc.Roles[0])
		}
	})
	t.Run("one agent on two tasks is one role", func(t *testing.T) {
		g := mustLift(t, tradeFlow(), LiftOpts{})
		draft, _ := g.Step("draft")
		exp, _ := g.Step("in_review")
		if draft.Tasks[0].Role != exp.Tasks[0].Role {
			t.Fatalf("roles differ: %q, %q (the drafter also exports)", draft.Tasks[0].Role, exp.Tasks[0].Role)
		}
		if len(g.Doc.Roles) != 6 {
			t.Fatalf("roles = %d, want 6", len(g.Doc.Roles))
		}
	})
}

func TestLift_StructureOfTheTradeFlow(t *testing.T) {
	g := mustLift(t, tradeFlow(), LiftOpts{Runtime: true})
	doc := g.Doc
	if doc.Version != 2 || doc.Start != "todo" {
		t.Fatalf("doc = version %d start %q", doc.Version, doc.Start)
	}
	var steps []string
	for _, s := range doc.Steps {
		steps = append(steps, s.Key)
	}
	if want := []string{"todo", "collect", "draft", "gate", "handover", "in_review"}; !reflect.DeepEqual(steps, want) {
		t.Fatalf("steps = %v, want %v (blocked becomes a stop, done and cancelled terminals)", steps, want)
	}
	var terms []string
	for _, tm := range doc.Terminals {
		terms = append(terms, fmt.Sprintf("%s:%v", tm.Key, tm.LeavesWorkspace))
	}
	if want := []string{"done:true", "cancelled:false"}; !reflect.DeepEqual(terms, want) {
		t.Fatalf("terminals = %v, want %v", terms, want)
	}

	todo, _ := g.Step("todo")
	if todo.Kind != StepKindLead || todo.Lead == nil || todo.Lead.Verdicts != "plan" ||
		!reflect.DeepEqual(todo.Lead.Sets, []string{"route", "customs_lookup", "export"}) {
		t.Fatalf("todo = %+v lead %+v", todo, todo.Lead)
	}
	review, _ := g.Step("in_review")
	if review.Kind != StepKindManual || review.ExitRole != "managers" || review.Notice != "stop2" || len(review.Tasks) != 1 {
		t.Fatalf("in_review = %+v", review)
	}
	for _, s := range doc.Steps {
		if s.Status.Key != s.Key {
			t.Errorf("step %s shows status %+v", s.Key, s.Status)
		}
		if s.Kind == StepKindTasks && s.Join != nil {
			t.Errorf("step %s has a non-default join %+v", s.Key, s.Join)
		}
	}

	var vars []string
	for _, v := range doc.Vars {
		vars = append(vars, fmt.Sprintf("%s:%s:%v:%v:%v", v.Key, v.Type, v.Default, v.SetBy, v.SetAt))
	}
	if want := []string{
		"route:enum:shipment_dossier:[lead]:[todo]",
		"customs_lookup:bool:false:[lead]:[todo]",
		"export:bool:false:[lead]:[todo]",
	}; !reflect.DeepEqual(vars, want) {
		t.Fatalf("vars = %v, want %v", vars, want)
	}
	if doc.Vars[0].Values[0] != "shipment_dossier" {
		t.Fatalf("enum values = %v", doc.Vars[0].Values)
	}

	if len(doc.Stops) != 2 || doc.Stops[0].Code != StopGroundsMissing || doc.Stops[0].LegacyNo != 1 ||
		doc.Stops[1].Code != StopLoopExhausted || doc.Stops[1].LegacyNo != 3 {
		t.Fatalf("stops = %+v", doc.Stops)
	}

	draft, _ := g.Step("draft")
	cross := draft.Tasks[0]
	if cross.Requires == nil || cross.Requires.Verdict != nil || !reflect.DeepEqual(cross.Requires.Files, []FileReq{{Slot: "output", Min: 1}}) {
		t.Fatalf("crosscheck requires = %+v", cross.Requires)
	}
	gate, _ := g.Step("gate")
	docsGate := gate.Tasks[0]
	if docsGate.Requires == nil || docsGate.Requires.Verdict == nil || !*docsGate.Requires.Verdict || len(docsGate.Requires.Files) != 0 || docsGate.Verdicts != "gate" {
		t.Fatalf("docs_gate = %+v requires %+v", docsGate, docsGate.Requires)
	}
	collect, _ := g.Step("collect")
	if collect.Tasks[1].When == nil || !reflect.DeepEqual(*collect.Tasks[1].When, *cond("var.customs_lookup", OpEq, true)) {
		t.Fatalf("customs when = %+v", collect.Tasks[1].When)
	}
	if collect.Tasks[0].Requires != nil {
		t.Fatalf("docs requires = %+v, want none", collect.Tasks[0].Requires)
	}

	t.Run("messages and timeouts come from flow_config", func(t *testing.T) {
		def := tradeFlow()
		def.Flow.Messages = FlowMessages{Stop1: "S1\nbody", Human: "H", Stalled: "  "}
		def.Flow.TaskTimeoutMinutes = 30
		g := mustLift(t, def, LiftOpts{})
		if want := map[string]string{"stop1": "S1\nbody", "human": "H"}; !reflect.DeepEqual(g.Doc.Messages, want) {
			t.Fatalf("messages = %v, want %v", g.Doc.Messages, want)
		}
		if p := g.Doc.Policy; p.TaskIdleMinutes != 30 || p.TaskHardCapMinutes != 240 || p.TaskMaxAttempts != 2 {
			t.Fatalf("policy = %+v", p)
		}
	})
	t.Run("a layout is carried over", func(t *testing.T) {
		def := tradeFlow()
		def.Steps[1].Layout = &Layout{X: 10, Y: 20}
		g := mustLift(t, def, LiftOpts{})
		if s, _ := g.Step("collect"); s.Layout == nil || *s.Layout != (StepLayout{X: 10, Y: 20}) {
			t.Fatalf("layout = %+v", s.Layout)
		}
	})
	t.Run("a human step that is not in_review posts the human notice", func(t *testing.T) {
		def := tradeFlow()
		def.Steps[5].StatusKey = "approval"
		def.Steps[4].Transitions[0].ToStatusKey = "approval"
		g := mustLift(t, def, LiftOpts{})
		if s, _ := g.Step("approval"); s.Notice != "human" {
			t.Fatalf("notice = %q", s.Notice)
		}
	})
	t.Run("a workflow that is not a flow cannot be lifted", func(t *testing.T) {
		for name, def := range map[string]Definition{
			"no flow_config": {Steps: tradeFlow().Steps},
			"no plan step":   {Flow: tradeFlow().Flow, Steps: tradeFlow().Steps[1:]},
			"a task without an agent": func() Definition {
				d := tradeFlow()
				d.Steps[1].Tasks[0].Handler = Handler{Type: HandlerNone}
				return d
			}(),
		} {
			if _, err := Lift(def, LiftOpts{}); err == nil {
				t.Errorf("%s: no error", name)
			}
		}
	})
}

func TestLift_DoesNotChangeItsInputAndIsDeterministic(t *testing.T) {
	for id, f := range loadCatalogFlows(t) {
		def, _, _ := f.definition()
		before, err := json.Marshal(def)
		if err != nil {
			t.Fatal(err)
		}
		first := mustLift(t, def, LiftOpts{Runtime: true})
		second := mustLift(t, def, LiftOpts{Runtime: true})
		after, _ := json.Marshal(def)
		if string(before) != string(after) {
			t.Fatalf("%s: Lift changed the definition", id)
		}
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("%s: Lift is not deterministic", id)
		}
	}
	// The conditions of the lifted graph do not share memory with the input.
	def := tradeFlow()
	g := mustLift(t, def, LiftOpts{})
	collect, _ := g.Step("collect")
	collect.Tasks[1].When.Value = "changed"
	if def.Steps[1].Tasks[1].When.Value != true {
		t.Fatal("the lifted condition shares memory with the definition")
	}
}

func TestLift_ForkDocIsCleanV2(t *testing.T) {
	flows := loadCatalogFlows(t)
	if len(flows) == 0 {
		t.Skip("the catalog is not built")
	}
	ids := make([]string, 0, len(flows))
	for id := range flows {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	warnings := map[string]int{}
	exits := map[string]int{}
	t.Cleanup(func() {
		t.Logf("warnings of the fork documents, by code: %v", warnings)
		t.Logf("manual exits added, by flow: %v", exits)
	})
	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			def, ctx := forkInputs(t, flows[id])
			g := mustLift(t, def, LiftOpts{RoleKeys: ctx.Roster.RoleKeys})
			for _, s := range g.Doc.Steps {
				if s.Kind == StepKindManual {
					exits[id] += len(s.Edges)
				}
			}
			if g.Compat != CompatNone {
				t.Fatalf("Compat = %q, want none", g.Compat)
			}
			raw, err := json.Marshal(g.Doc)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), "compat") || strings.Contains(string(raw), "activation") {
				t.Fatalf("the fork document mentions compat or activation: %s", raw)
			}
			parsed, issues := ParseFlowDoc(raw)
			if len(issues) > 0 {
				t.Fatalf("the fork document does not parse: %+v", issues)
			}
			again, _ := json.Marshal(parsed)
			if string(again) != string(raw) {
				t.Fatalf("serialising the parsed document changed it\n%s\n%s", raw, again)
			}
			if parsed.Policy.AllowUnattendedRelease || parsed.Policy.LeadMayWork {
				t.Fatalf("policy = %+v", parsed.Policy)
			}
			// The fork is a draft: it passes every rule of spec section 3 with the
			// roster of the squad. Warnings are allowed.
			issues = ValidateFlowDoc(parsed, ctx)
			for _, i := range issues {
				if i.Severity == SeverityWarning {
					warnings[i.Code]++
				}
			}
			if HasErrors(issues) {
				t.Fatalf("the fork document of %s is not valid:%s", id, describeIssues(issues))
			}
		})
	}
}

func TestLift_ManualStepsGetExits(t *testing.T) {
	// The trade flow: in_review is human-only and is followed in the list by
	// blocked, done and cancelled, so it has the done button only.
	for _, runtime := range []bool{false, true} {
		g := mustLift(t, tradeFlow(), LiftOpts{Runtime: runtime})
		review, _ := g.Step("in_review")
		if review.Kind != StepKindManual || review.Notice != "stop2" || review.ExitRole != "managers" {
			t.Fatalf("runtime %v: in_review = %+v", runtime, review)
		}
		want := []DocEdge{{ID: "in_review_done", Label: "Done", To: "done", Trigger: EdgeTriggerManual}}
		if !reflect.DeepEqual(review.Edges, want) {
			t.Fatalf("runtime %v: edges = %+v, want %+v", runtime, review.Edges, want)
		}
	}

	// A human step in the middle has a button to the step after it and one to done.
	def := tradeFlow()
	approval := def.Steps[2] // draft becomes the human step "approval" between collect and draft
	approval.StatusKey, approval.HumanOnly = "approval", true
	approval.Tasks, approval.Transitions = nil, nil
	steps := append([]Step{}, def.Steps[:2]...)
	steps = append(steps, approval)
	steps = append(steps, tradeFlow().Steps[2:]...)
	def.Steps = steps
	def.Steps[1].Transitions[1].ToStatusKey = "approval"
	g := mustLift(t, def, LiftOpts{})
	step, _ := g.Step("approval")
	if step.Notice != "human" {
		t.Fatalf("notice = %q", step.Notice)
	}
	want := []DocEdge{
		{ID: "approval_next", Label: "Next step", To: "draft", Trigger: EdgeTriggerManual},
		{ID: "approval_done", Label: "Done", To: "done", Trigger: EdgeTriggerManual},
	}
	if !reflect.DeepEqual(step.Edges, want) {
		t.Fatalf("edges = %+v, want %+v", step.Edges, want)
	}

	t.Run("never an edge to a step that does not exist", func(t *testing.T) {
		d := tradeFlow()
		kept := d.Steps[:0:0]
		for _, s := range d.Steps {
			if s.StatusKey != "done" {
				kept = append(kept, s)
			}
		}
		d.Steps = kept
		g := mustLift(t, d, LiftOpts{})
		review, _ := g.Step("in_review")
		if len(review.Edges) != 0 {
			t.Fatalf("no done terminal, yet edges = %+v", review.Edges)
		}
		for _, s := range g.Doc.Steps {
			for _, e := range s.Edges {
				if e.To == "" {
					continue
				}
				_, isStep := g.Step(e.To)
				_, isTerminal := g.Terminal(e.To)
				if !isStep && !isTerminal {
					t.Errorf("%s.%s leads to %q, which is nothing", s.Key, e.ID, e.To)
				}
			}
		}
	})
	t.Run("an id that schema 1 used is kept", func(t *testing.T) {
		d := tradeFlow()
		d.Steps[5].Transitions = []Transition{tr("in_review_done", nil, "done", false)}
		g := mustLift(t, d, LiftOpts{})
		review, _ := g.Step("in_review")
		ids := []string{}
		for _, e := range review.Edges {
			ids = append(ids, e.ID)
		}
		if !reflect.DeepEqual(ids, []string{"in_review_done", "in_review_done_2"}) {
			t.Fatalf("ids = %v", ids)
		}
	})
	t.Run("the buttons are never taken by the decision", func(t *testing.T) {
		g := mustLift(t, tradeFlow(), LiftOpts{Runtime: true})
		st := Adapt(FlowState{Step: "in_review", Switches: tradeFlow().Flow.DefaultSwitches()})
		if d := decideAt(g, st, []TaskState{closedTask("in_review", "export", KindWork, 0, "done")}); d.Kind != DecisionWait {
			t.Fatalf("%+v", d)
		}
		// A person takes one, and it is a checkpoint.
		fs := BuildGraphFacts(g, st, nil, RequestFacts{})
		if d := RouteManual(g, st, nil, "in_review_done", fs); d.Kind != DecisionRoute || d.To != "done" || !d.Checkpoint {
			t.Fatalf("%+v", d)
		}
	})
}

func TestLift_RuntimeGraphIsExemptFromNewRules(t *testing.T) {
	flows := loadCatalogFlows(t)
	if len(flows) == 0 {
		t.Skip("the catalog is not built")
	}
	for id, f := range flows {
		def, _, _ := f.definition()
		g := mustLift(t, def, LiftOpts{Runtime: true})
		if g.Compat != CompatV1 {
			t.Errorf("%s: Compat = %q", id, g.Compat)
		}
		if !g.Doc.Policy.AllowUnattendedRelease {
			t.Errorf("%s: allow_unattended_release is off, so a request that leaves the workspace without a checkpoint would stall", id)
		}
		if g.Doc.Policy.LeadMayWork {
			t.Errorf("%s: lead_may_work is on", id)
		}
		// The runtime graph is still a document the parser reads.
		raw, _ := json.Marshal(g.Doc)
		if _, issues := ParseFlowDoc(raw); len(issues) > 0 {
			t.Errorf("%s: %+v", id, issues)
		}
	}
}

func TestLift_ForkDocGetsNoExemption(t *testing.T) {
	flows := loadCatalogFlows(t)
	if len(flows) == 0 {
		t.Skip("the catalog is not built")
	}
	for id, f := range flows {
		def, _, _ := f.definition()
		fork := mustLift(t, def, LiftOpts{})
		runtime := mustLift(t, def, LiftOpts{Runtime: true})
		if fork.Doc.Policy.AllowUnattendedRelease {
			t.Errorf("%s: the fork document is exempt from the release rule", id)
		}
		// The graphs differ in the exemption and nothing else.
		runtime.Doc.Policy.AllowUnattendedRelease = false
		runtime.Compat = CompatNone
		if !reflect.DeepEqual(fork, runtime) {
			t.Errorf("%s: the fork and the runtime graph differ beyond the exemption", id)
		}
	}
}
