package issueworkflow

import (
	"reflect"
	"testing"
)

func refs(in []TaskInput) []string {
	var out []string
	for _, i := range in {
		out = append(out, i.Identifier)
	}
	return out
}

func TestTaskLinks_ExplicitInputsLatestActivation(t *testing.T) {
	g := loadGraph(t, "crossreview.json")
	step, _ := g.Step("review")
	st := at(g, "review", 2, nil)
	rows := []TaskState{
		// The first build, then the rebuild after the review sent it back.
		closedTask("build", "write_a", KindWork, 0, "done"), closedTask("build", "write_b", KindWork, 0, "done"),
		closedTask("build", "write_a", KindWork, 1, "done"), closedTask("build", "write_b", KindWork, 1, "done"),
		closedTask("review", "ana_checks_ben", KindGate, 0, "rejected"),
	}
	inputs, feedback := TaskLinks(g, st, rows, step.Tasks[0]) // Ana checks Ben's part
	if want := []string{"build/write_b"}; !reflect.DeepEqual(refs(inputs), want) {
		t.Fatalf("inputs = %v, want %v", refs(inputs), want)
	}
	if inputs[0].Key != "write_b" {
		t.Fatalf("input = %+v", inputs[0])
	}
	if len(feedback) != 0 {
		t.Fatalf("the request did not come back by an edge: feedback %v", refs(feedback))
	}
	inputs, _ = TaskLinks(g, st, rows, step.Tasks[1])
	if want := []string{"build/write_a"}; !reflect.DeepEqual(refs(inputs), want) {
		t.Fatalf("inputs = %v, want %v", refs(inputs), want)
	}

	t.Run("every instance of the source and the slot", func(t *testing.T) {
		m := loadGraph(t, "market.json")
		verify, _ := m.Step("verify")
		task := verify.Tasks[0]
		task.Inputs = []Input{{From: "collect.collect_source", Slot: "report"}}
		rows := []TaskState{
			closedTask("collect", "collect_source~2", KindWork, 0, "done"),
			closedTask("collect", "collect_source~1", KindWork, 0, "done"),
			{Step: "collect", Key: "collect_source~3", Kind: KindWork, Round: 0, Closed: true, Cancelled: true, Outcome: OutcomeSuperseded},
		}
		inputs, _ := TaskLinks(m, at(m, "verify", 1, nil), rows, task)
		if want := []string{"collect/collect_source~1", "collect/collect_source~2"}; !reflect.DeepEqual(refs(inputs), want) {
			t.Fatalf("inputs = %v, want the instances in order without the superseded one", refs(inputs))
		}
		if inputs[0].Title != "report" {
			t.Fatalf("slot = %q", inputs[0].Title)
		}
	})
	t.Run("a source that never ran is left out", func(t *testing.T) {
		inputs, _ := TaskLinks(g, st, nil, step.Tasks[0])
		if len(inputs) != 0 {
			t.Fatalf("inputs = %v", refs(inputs))
		}
	})
}

func TestTaskLinks_DefaultFollowsTrailNotDeclarationOrder(t *testing.T) {
	// The steps are declared x, y, z but the request went plan, y, x, z.
	doc := FlowDoc{Version: 2, Start: "plan", Terminals: []Terminal{{Key: "done", Status: "done"}},
		Roles: []Role{{Key: "w", Kind: RoleKindAgent}, {Key: "c", Kind: RoleKindAgent}},
		Steps: []DocStep{
			{Key: "plan", Kind: StepKindLead, Edges: []DocEdge{{ID: "p", To: "y"}}},
			{Key: "x", Kind: StepKindTasks, Tasks: []DocTask{{Key: "work_x", Kind: KindWork, Role: "w"}}, Edges: []DocEdge{{ID: "x1", To: "z"}}},
			{Key: "y", Kind: StepKindTasks, Tasks: []DocTask{{Key: "work_y", Kind: KindWork, Role: "w"}}, Edges: []DocEdge{{ID: "y1", To: "x"}}},
			{Key: "z", Kind: StepKindTasks, Tasks: []DocTask{
				{Key: "gate_z", Kind: KindGate, Role: "c"},
				{Key: "side", Kind: KindWork, Role: "w", After: []string{"gate_z"}},
			}, Edges: []DocEdge{{ID: "z1", To: "done"}}},
		}}
	g := NewGraph(doc)
	st := at(g, "z", 1, nil)
	st.Trail = []TrailEntry{{Step: "plan", Activation: 1, Edge: "p"}, {Step: "y", Activation: 1, Edge: "y1"}, {Step: "x", Activation: 1, Edge: "x1"}}
	rows := []TaskState{
		closedTask("y", "work_y", KindWork, 0, "done"),
		closedTask("x", "work_x", KindWork, 0, "done"),
	}
	step, _ := g.Step("z")
	inputs, _ := TaskLinks(g, st, rows, step.Tasks[0])
	if want := []string{"x/work_x"}; !reflect.DeepEqual(refs(inputs), want) {
		t.Fatalf("inputs = %v, want %v (the nearest step on the trail, not the one declared before)", refs(inputs), want)
	}

	t.Run("a step on the trail without passing work is skipped", func(t *testing.T) {
		bad := append([]TaskState(nil), rows...)
		bad[1] = closedTask("x", "work_x", KindWork, 0, "blocked")
		inputs, _ := TaskLinks(g, st, bad, step.Tasks[0])
		if want := []string{"y/work_y"}; !reflect.DeepEqual(refs(inputs), want) {
			t.Fatalf("inputs = %v, want %v", refs(inputs), want)
		}
	})
	t.Run("a gate is not an input", func(t *testing.T) {
		withGate := append(append([]TaskState(nil), rows...), closedTask("x", "gate_x", KindGate, 0, "approved"))
		inputs, _ := TaskLinks(g, st, withGate, step.Tasks[0])
		if want := []string{"x/work_x"}; !reflect.DeepEqual(refs(inputs), want) {
			t.Fatalf("inputs = %v", refs(inputs))
		}
	})
	t.Run("a task in the same step reads the tasks it comes after", func(t *testing.T) {
		withGate := append(append([]TaskState(nil), rows...), closedTask("z", "gate_z", KindGate, 0, "approved"))
		inputs, _ := TaskLinks(g, st, withGate, step.Tasks[1])
		if want := []string{"x/work_x", "z/gate_z"}; !reflect.DeepEqual(refs(inputs), want) {
			t.Fatalf("inputs = %v, want the trail input then the task it follows", refs(inputs))
		}
	})
	t.Run("the request that took another path reads that path", func(t *testing.T) {
		other := st
		other.Trail = []TrailEntry{{Step: "plan", Activation: 1}, {Step: "x", Activation: 1}, {Step: "y", Activation: 1}}
		inputs, _ := TaskLinks(g, other, rows, step.Tasks[0])
		if want := []string{"y/work_y"}; !reflect.DeepEqual(refs(inputs), want) {
			t.Fatalf("inputs = %v, want %v", refs(inputs), want)
		}
	})
	t.Run("an entry reads the work of its own activation", func(t *testing.T) {
		again := st
		again.Trail = []TrailEntry{{Step: "x", Activation: 2}}
		old := closedTask("x", "work_x", KindWork, 0, "done")
		fresh := closedTask("x", "work_x", KindWork, 1, "done")
		inputs, _ := TaskLinks(g, again, []TaskState{old}, step.Tasks[0])
		if len(inputs) != 0 {
			t.Fatalf("activation 2 has no round 1 work: %v", refs(inputs))
		}
		inputs, _ = TaskLinks(g, again, []TaskState{old, fresh}, step.Tasks[0])
		if len(inputs) != 1 || inputs[0].Identifier != "x/work_x" {
			t.Fatalf("inputs = %v", refs(inputs))
		}
	})
}

func TestTaskLinks_FeedbackAfterBackEdge(t *testing.T) {
	g := loadGraph(t, "trade.json")
	draft, _ := g.Step("draft")
	st := at(g, "draft", 2, nil)
	st.Trail = []TrailEntry{{Step: "plan", Activation: 1, Edge: "plan_ok"}, {Step: "collect", Activation: 1, Edge: "collect_ok"},
		{Step: "draft", Activation: 1, Edge: "draft_ok"}, {Step: "gate", Activation: 1, Edge: "gate_fix"}}
	rows := []TaskState{
		closedTask("collect", "docs", KindWork, 0, "done"),
		closedTask("draft", "crosscheck", KindWork, 0, "done"),
		closedTask("gate", "docs_gate", KindGate, 0, "approved"),
		closedTask("gate", "customs_gate", KindGate, 0, "rejected"),
	}
	_, feedback := TaskLinks(g, st, rows, draft.Tasks[0])
	if want := []string{"gate/customs_gate"}; !reflect.DeepEqual(refs(feedback), want) {
		t.Fatalf("feedback = %v, want the gate that rejected %v", refs(feedback), want)
	}

	// A blocked task is feedback too; an approving one is not.
	rows[2] = closedTask("gate", "docs_gate", KindGate, 0, "blocked")
	_, feedback = TaskLinks(g, st, rows, draft.Tasks[0])
	if want := []string{"gate/docs_gate", "gate/customs_gate"}; !reflect.DeepEqual(refs(feedback), want) {
		t.Fatalf("feedback = %v, want %v", refs(feedback), want)
	}

	t.Run("an entry by a forward edge has none", func(t *testing.T) {
		fwd := st
		fwd.Trail = []TrailEntry{{Step: "collect", Activation: 1, Edge: "collect_ok"}}
		if _, feedback := TaskLinks(g, fwd, rows, draft.Tasks[0]); len(feedback) != 0 {
			t.Fatalf("feedback = %v", refs(feedback))
		}
	})
	t.Run("only the entry the request left counts", func(t *testing.T) {
		later := append(append([]TaskState(nil), rows...), closedTask("gate", "docs_gate", KindGate, 1, "rejected"))
		_, feedback := TaskLinks(g, st, later, draft.Tasks[0])
		if len(feedback) != 2 {
			t.Fatalf("feedback = %v, want the two tasks of the entry the request left", refs(feedback))
		}
	})
	t.Run("a graph lifted from schema 1 reads by position", func(t *testing.T) {
		def := tradeFlow()
		v1 := mustLift(t, def, LiftOpts{Runtime: true})
		state := Adapt(FlowState{Step: "gate", Rework: 1})
		rows := []TaskState{
			closedTask("draft", "crosscheck", KindWork, 1, "done"),
			closedTask("collect", "docs", KindWork, 1, "done"),
			closedTask("gate", "docs_gate", KindGate, 0, "rejected"),
			closedTask("gate", "customs_gate", KindGate, 0, "approved"),
			closedTask("elsewhere", "other", KindGate, 0, "approved"),
		}
		gateStep, _ := v1.Step("gate")
		inputs, feedback := TaskLinks(v1, state, rows, gateStep.Tasks[0])
		if want := []string{"draft/crosscheck"}; !reflect.DeepEqual(refs(inputs), want) {
			t.Fatalf("inputs = %v, want %v (the nearest earlier step with rows in round 1)", refs(inputs), want)
		}
		if want := []string{"gate/docs_gate"}; !reflect.DeepEqual(refs(feedback), want) {
			t.Fatalf("feedback = %v, want the gate that did not approve in round 0", refs(feedback))
		}
	})
}

func TestTaskLinks_RequiredInputMissingStalls(t *testing.T) {
	g := loadGraph(t, "crossreview.json")
	st := at(g, "review", 1, nil)
	// build never ran: the gates read it, so the step cannot open them.
	wantStalled(t, decideAt(g, st, nil), ReasonInputMissing)
	built := []TaskState{closedTask("build", "write_a", KindWork, 0, "done")}
	wantStalled(t, decideAt(g, st, built), ReasonInputMissing) // write_b is missing
	built = append(built, closedTask("build", "write_b", KindWork, 0, "done"))
	if d := decideAt(g, st, built); d.Kind != DecisionOpen || len(d.Open) != 2 {
		t.Fatalf("with both sources: %+v", d)
	}

	t.Run("an input that is not required is skipped", func(t *testing.T) {
		doc := modify(loadFlowDoc(t, "crossreview.json"), func(d *FlowDoc) {
			ins := stepIdx(d, "review").Tasks
			for i := range ins {
				ins[i].Inputs = []Input{{From: ins[i].Inputs[0].From}}
			}
		})
		g := NewGraph(doc)
		if d := decideAt(g, st, nil); d.Kind != DecisionOpen {
			t.Fatalf("%+v", d)
		}
	})
	t.Run("a superseded source does not count", func(t *testing.T) {
		rows := []TaskState{
			{Step: "build", Key: "write_a", Kind: KindWork, Round: 0, Closed: true, Cancelled: true, Outcome: OutcomeSuperseded},
			closedTask("build", "write_b", KindWork, 0, "done"),
		}
		wantStalled(t, decideAt(g, st, rows), ReasonInputMissing)
	})
	t.Run("a graph lifted from schema 1 has no required inputs", func(t *testing.T) {
		v1 := mustLift(t, tradeFlow(), LiftOpts{Runtime: true})
		if d := decideAt(v1, Adapt(FlowState{Step: "gate"}), nil); d.Kind != DecisionOpen {
			t.Fatalf("%+v", d)
		}
	})
}

func TestAssign_StickyAcrossActivations(t *testing.T) {
	pool := []string{"ann", "bo", "cy"}
	// Second instance, no history: the second person.
	if got, ok := AssignPool(AssignRequest{Pool: pool, Instance: 2, Distinct: true}); !ok || got != "bo" {
		t.Fatalf("instance 2 = %q, %v", got, ok)
	}
	// A task without instances takes the first person; a fourth instance wraps.
	if got, _ := AssignPool(AssignRequest{Pool: pool}); got != "ann" {
		t.Fatalf("no instances = %q", got)
	}
	if got, _ := AssignPool(AssignRequest{Pool: pool, Instance: 4}); got != "ann" {
		t.Fatalf("instance 4 = %q, want it to wrap to the first", got)
	}
	// The person who held the row before keeps it.
	if got, _ := AssignPool(AssignRequest{Pool: pool, Instance: 2, Prior: "cy", Distinct: true}); got != "cy" {
		t.Fatalf("with a prior holder = %q, want cy", got)
	}
	// ... unless they cannot take it any more: then the next free person.
	got, ok := AssignPool(AssignRequest{Pool: pool, Instance: 2, Prior: "cy", Distinct: true, Unavailable: map[string]bool{"cy": true}})
	if !ok || got != "bo" {
		t.Fatalf("prior unavailable = %q, %v, want bo", got, ok)
	}
	// A prior holder who has left the pool does not keep the row.
	if got, _ := AssignPool(AssignRequest{Pool: pool, Instance: 3, Prior: "dee"}); got != "cy" {
		t.Fatalf("prior outside the pool = %q", got)
	}
	// The next person is the one after the first choice, who no other row holds.
	got, _ = AssignPool(AssignRequest{Pool: pool, Instance: 1, Distinct: true, Taken: map[string]bool{"ann": true}})
	if got != "bo" {
		t.Fatalf("first choice taken = %q, want bo", got)
	}
	got, _ = AssignPool(AssignRequest{Pool: pool, Instance: 1, Distinct: true, Taken: map[string]bool{"ann": true, "bo": true}})
	if got != "cy" {
		t.Fatalf("two taken = %q, want cy", got)
	}
}

func TestAssign_SkipsWorkerForGate(t *testing.T) {
	g := loadGraph(t, "trade.json")
	gate, _ := g.Step("gate")
	st := at(g, "gate", 1, nil)
	st.Trail = []TrailEntry{{Step: "plan", Activation: 1}, {Step: "collect", Activation: 1}, {Step: "draft", Activation: 1}}
	rows := []TaskState{
		closedTask("collect", "docs", KindWork, 0, "done"),
		closedTask("draft", "crosscheck", KindWork, 0, "done"),
	}
	holders := map[string]string{"draft/crosscheck": "ann", "collect/docs": "cy"}
	assignee := func(step, key string) string { return holders[RowRef(step, key)] }

	forbidden := JudgedWorkers(g, st, rows, gate.Tasks[0], assignee)
	if !reflect.DeepEqual(forbidden, map[string]bool{"ann": true}) {
		t.Fatalf("forbidden = %v, want only the person who drafted what the gate judges", forbidden)
	}
	got, ok := AssignPool(AssignRequest{Pool: []string{"ann", "bo"}, Forbidden: forbidden, Distinct: true})
	if !ok || got != "bo" {
		t.Fatalf("assigned %q, %v, want bo (ann drafted it)", got, ok)
	}
	// A prior holder who is forbidden does not keep the row.
	if got, _ := AssignPool(AssignRequest{Pool: []string{"ann", "bo"}, Prior: "ann", Forbidden: forbidden}); got != "bo" {
		t.Fatalf("sticky holder who now breaks separation kept the row: %q", got)
	}
	// Work is not judged: a work task has no forbidden people.
	draft, _ := g.Step("draft")
	if got := JudgedWorkers(g, st, rows, draft.Tasks[0], assignee); got != nil {
		t.Fatalf("a work task judges no one: %v", got)
	}
	// A pool with only the worker leaves no one for the gate.
	if _, ok := AssignPool(AssignRequest{Pool: []string{"ann"}, Forbidden: forbidden}); ok {
		t.Fatal("the only person in the pool did the work")
	}
}

func TestAssign_PoolExhaustedStalls(t *testing.T) {
	for name, req := range map[string]AssignRequest{
		"an empty pool":              {},
		"everyone unavailable":       {Pool: []string{"ann", "bo"}, Unavailable: map[string]bool{"ann": true, "bo": true}},
		"everyone forbidden":         {Pool: []string{"ann"}, Forbidden: map[string]bool{"ann": true}},
		"more instances than people": {Pool: []string{"ann", "bo"}, Instance: 3, Distinct: true, Taken: map[string]bool{"ann": true, "bo": true}},
	} {
		t.Run(name, func(t *testing.T) {
			if got, ok := AssignPool(req); ok {
				t.Fatalf("assigned %q, want the request to stall with %s", got, ReasonPoolTooSmall)
			}
		})
	}
	// Without distinct instances people are shared.
	if got, ok := AssignPool(AssignRequest{Pool: []string{"ann", "bo"}, Instance: 3, Taken: map[string]bool{"ann": true, "bo": true}}); !ok || got != "ann" {
		t.Fatalf("shared pool = %q, %v", got, ok)
	}
}
