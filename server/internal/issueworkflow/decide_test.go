package issueworkflow

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// loadFlowDoc reads a FlowDoc of testdata/flows and requires it to parse.
func loadFlowDoc(t *testing.T, name string) FlowDoc {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "flows", name))
	if err != nil {
		t.Fatal(err)
	}
	doc, issues := ParseFlowDoc(raw)
	if len(issues) > 0 {
		t.Fatalf("%s does not parse: %+v", name, issues)
	}
	return doc
}

func loadGraph(t *testing.T, name string) Graph { return NewGraph(loadFlowDoc(t, name)) }

// modify returns the document with the edit applied to a copy of its steps.
func modify(doc FlowDoc, edit func(d *FlowDoc)) FlowDoc {
	steps := make([]DocStep, len(doc.Steps))
	for i, s := range doc.Steps {
		s.Tasks = append([]DocTask(nil), s.Tasks...)
		s.Edges = append([]DocEdge(nil), s.Edges...)
		steps[i] = s
	}
	doc.Steps = steps
	doc.Loops = append([]Loop(nil), doc.Loops...)
	doc.Terminals = append([]Terminal(nil), doc.Terminals...)
	doc.Stops = append([]StopKind(nil), doc.Stops...)
	edit(&doc)
	return doc
}

func stepIdx(d *FlowDoc, key string) *DocStep {
	for i := range d.Steps {
		if d.Steps[i].Key == key {
			return &d.Steps[i]
		}
	}
	panic("no step " + key)
}

// at is the state of a request in its n-th entry into the step.
func at(g Graph, step string, entry int, vars map[string]any) State {
	st := InitialState(g, vars, "")
	st.Step = step
	st.Activations[step] = entry
	return st
}

func decideAt(g Graph, st State, rows []TaskState) GraphDecision {
	return DecideGraph(g, st, rows, BuildGraphFacts(g, st, rows, RequestFacts{}))
}

func openKeys(d GraphDecision) []string {
	var keys []string
	for _, o := range d.Open {
		keys = append(keys, o.Key)
	}
	return keys
}

func wantRoute(t *testing.T, d GraphDecision, edge, to, result string) {
	t.Helper()
	if d.Kind != DecisionRoute || d.TransitionID != edge || d.To != to || d.JoinResult != result {
		t.Fatalf("decision = kind %q edge %q to %q join %q stop %q reason %q open %v, want route by %q to %q on %q",
			d.Kind, d.TransitionID, d.To, d.JoinResult, d.StopCode, d.Reason, openKeys(d), edge, to, result)
	}
}

func wantStop(t *testing.T, d GraphDecision, edge, code string) {
	t.Helper()
	if d.Kind != DecisionRoute || d.TransitionID != edge || d.StopCode != code || d.To != "" {
		t.Fatalf("decision = kind %q edge %q to %q stop %q reason %q, want stop %q by %q", d.Kind, d.TransitionID, d.To, d.StopCode, d.Reason, code, edge)
	}
}

func wantStalled(t *testing.T, d GraphDecision, reason string) {
	t.Helper()
	if d.Kind != DecisionRoute || d.StopCode != DocStopStalled || d.Stop != StopStalled || d.Reason != reason || d.Status != "blocked" {
		t.Fatalf("decision = kind %q stop %q reason %q, want stalled for %q", d.Kind, d.StopCode, d.Reason, reason)
	}
}

func TestDecide_JoinAll_MatchesLiftedV1(t *testing.T) {
	def := tradeFlow()
	g := mustLift(t, def, LiftOpts{Runtime: true})
	st := Adapt(FlowState{Step: "gate", Switches: def.Flow.DefaultSwitches()})
	rows := func(docs, customs string) []TaskState {
		return []TaskState{closedTask("gate", "docs_gate", KindGate, 0, docs), closedTask("gate", "customs_gate", KindGate, 0, customs)}
	}
	tests := map[string]struct {
		rows   []TaskState
		edge   string
		result string
	}{
		"both approve": {rows("approved", "approved"), "pass", "pass"},
		"one rejects":  {rows("approved", "rejected"), "redo", "fail"},
		"one blocks":   {rows("rejected", "blocked"), "blocked", "block"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := decideAt(g, st, tt.rows)
			old := oracleDecide(def, FlowState{Step: "gate", Switches: def.Flow.DefaultSwitches()}, tt.rows)
			if got.TransitionID != tt.edge || got.JoinResult != tt.result || got.TransitionID != old.TransitionID {
				t.Fatalf("graph %+v, oracle %+v, want edge %q on %q", got, old, tt.edge, tt.result)
			}
		})
	}
}

func TestDecide_JoinAnyCancelsTheRest(t *testing.T) {
	g := loadGraph(t, "patterns/p07_discriminator_any.json")
	st := at(g, "race", 1, nil)
	rows := []TaskState{openTask("race", "g1", KindGate, 0), closedTask("race", "g2", KindGate, 0, "approved"), openTask("race", "g3", KindGate, 0)}
	d := decideAt(g, st, rows)
	wantRoute(t, d, "race_ok", "done", JoinPass)
	if want := []string{"race/g1", "race/g3"}; !reflect.DeepEqual(d.Cancel, want) {
		t.Fatalf("cancel = %v, want the rest of the race %v", d.Cancel, want)
	}
	// Nobody passes: the join waits for every racer, then fails.
	rows = []TaskState{closedTask("race", "g1", KindGate, 0, "rejected"), openTask("race", "g2", KindGate, 0), closedTask("race", "g3", KindGate, 0, "rejected")}
	if d := decideAt(g, st, rows); d.Kind != DecisionWait {
		t.Fatalf("with a racer still open: %+v", d)
	}
	rows[1] = closedTask("race", "g2", KindGate, 0, "rejected")
	wantStop(t, decideAt(g, st, rows), "race_stop", "loop_exhausted")
	if d := decideAt(g, st, rows); d.JoinResult != JoinFail {
		t.Fatalf("join = %q, want fail", d.JoinResult)
	}
}

func TestDecide_QuorumPasses(t *testing.T) {
	g := loadGraph(t, "patterns/p08_n_of_m_quorum.json")
	st := at(g, "vote", 1, nil)
	rows := []TaskState{closedTask("vote", "v1", KindGate, 0, "approved"), closedTask("vote", "v2", KindGate, 0, "approved"), openTask("vote", "v3", KindGate, 0)}
	d := decideAt(g, st, rows)
	wantRoute(t, d, "vote_ok", "done", JoinPass)
	if want := []string{"vote/v3"}; !reflect.DeepEqual(d.Cancel, want) {
		t.Fatalf("cancel = %v, want %v", d.Cancel, want)
	}
	// One approval is not yet two.
	rows = []TaskState{closedTask("vote", "v1", KindGate, 0, "approved"), openTask("vote", "v2", KindGate, 0), openTask("vote", "v3", KindGate, 0)}
	if d := decideAt(g, st, rows); d.Kind != DecisionWait {
		t.Fatalf("one of two approvals: %+v", d)
	}
}

func TestDecide_QuorumFailsEarly(t *testing.T) {
	g := loadGraph(t, "patterns/p08_n_of_m_quorum.json")
	st := at(g, "vote", 1, nil)
	// pass 0 + open 1 < 2: the last voter cannot make a majority.
	rows := []TaskState{closedTask("vote", "v1", KindGate, 0, "rejected"), closedTask("vote", "v2", KindGate, 0, "rejected"), openTask("vote", "v3", KindGate, 0)}
	d := decideAt(g, st, rows)
	wantStop(t, d, "vote_stop", "loop_exhausted")
	if d.JoinResult != JoinFail || !reflect.DeepEqual(d.Cancel, []string{"vote/v3"}) {
		t.Fatalf("join %q cancel %v, want fail and the open voter cancelled", d.JoinResult, d.Cancel)
	}
	// One rejection leaves a majority possible.
	rows = []TaskState{closedTask("vote", "v1", KindGate, 0, "rejected"), openTask("vote", "v2", KindGate, 0), openTask("vote", "v3", KindGate, 0)}
	if d := decideAt(g, st, rows); d.Kind != DecisionWait {
		t.Fatalf("one rejection: %+v", d)
	}
}

func TestDecide_NeutralIsNotCounted(t *testing.T) {
	g := loadGraph(t, "legal.json")
	st := at(g, "review", 1, nil)
	review := func(outcomes ...string) []TaskState {
		var rows []TaskState
		for i, o := range outcomes {
			key := "reviewer~" + string(rune('1'+i))
			if o == "open" {
				rows = append(rows, openTask("review", key, KindGate, 0))
			} else {
				rows = append(rows, closedTask("review", key, KindGate, 0, o))
			}
		}
		return rows
	}
	// 3 approve and 2 abstain: the quorum of 3 is met by the approvals alone.
	wantRoute(t, decideAt(g, st, review("approve", "approve", "abstain", "approve", "abstain")), "review_ok", "counsel_signoff", JoinPass)
	// 2 approve and 3 abstain: abstentions do not count, so the quorum fails.
	d := decideAt(g, st, review("approve", "approve", "abstain", "abstain", "abstain"))
	if d.JoinResult != JoinFail {
		t.Fatalf("2 approvals and 3 abstentions: join %q, want fail", d.JoinResult)
	}
	// 2 abstain, 2 open: 1 approval + 2 open still reaches 3, so wait.
	if d := decideAt(g, st, review("approve", "abstain", "abstain", "open", "open")); d.Kind != DecisionWait {
		t.Fatalf("a quorum still possible: %+v", d)
	}
	// 3 abstain, 1 approval, 1 open: 1 + 1 < 3, fail now.
	if d := decideAt(g, st, review("approve", "abstain", "abstain", "abstain", "open")); d.JoinResult != JoinFail {
		t.Fatalf("a quorum no longer possible: %+v", d)
	}
}

func TestDecide_AllNeutralIsNoneNotPass(t *testing.T) {
	doc := modify(loadFlowDoc(t, "legal.json"), func(d *FlowDoc) {
		stepIdx(d, "review").Join = &Join{Over: JoinOver{Kind: JoinOverGates}, Mode: JoinModeAll}
	})
	g := NewGraph(doc)
	st := at(g, "review", 1, nil)
	var rows []TaskState
	for _, k := range []string{"reviewer~1", "reviewer~2", "reviewer~3", "reviewer~4", "reviewer~5"} {
		rows = append(rows, closedTask("review", k, KindGate, 0, "abstain"))
	}
	d := decideAt(g, st, rows)
	if d.JoinResult != JoinNone || d.TransitionID == "review_ok" {
		t.Fatalf("everyone abstained: join %q by %q, want none and not the pass edge", d.JoinResult, d.TransitionID)
	}
	rows[0] = closedTask("review", "reviewer~1", KindGate, 0, "approve")
	wantRoute(t, decideAt(g, st, rows), "review_ok", "counsel_signoff", JoinPass)
}

func TestDecide_BlockIsImmediate(t *testing.T) {
	g := loadGraph(t, "patterns/p02_parallel_join_all.json")
	st := at(g, "work", 1, nil)
	rows := []TaskState{
		closedTask("work", "ta", KindWork, 0, "done"),
		{Step: "work", Key: "tb", Kind: KindWork, Round: 0, Outcome: OutcomeBlocked}, // blocked, issue still open
		openTask("work", "tc", KindWork, 0),
	}
	d := decideAt(g, st, rows)
	if d.Kind != DecisionRoute || d.JoinResult != JoinBlock {
		t.Fatalf("a task reported blocked while another runs: %+v", d)
	}
	if len(d.Cancel) != 0 {
		t.Fatalf("a block must not cancel the open task: %v", d.Cancel)
	}
}

func TestDecide_BlockAtCloseWaits(t *testing.T) {
	doc := modify(loadFlowDoc(t, "patterns/p02_parallel_join_all.json"), func(d *FlowDoc) {
		stepIdx(d, "work").Join.Block = JoinBlockAtClose
	})
	g := NewGraph(doc)
	st := at(g, "work", 1, nil)
	rows := []TaskState{
		closedTask("work", "ta", KindWork, 0, "blocked"),
		openTask("work", "tb", KindWork, 0),
		closedTask("work", "tc", KindWork, 0, "done"),
	}
	if d := decideAt(g, st, rows); d.Kind != DecisionWait {
		t.Fatalf("a block at_close must wait for the open task: %+v", d)
	}
	rows[1] = closedTask("work", "tb", KindWork, 0, "done")
	if d := decideAt(g, st, rows); d.Kind != DecisionRoute || d.JoinResult != JoinBlock {
		t.Fatalf("all closed: %+v, want a block result", d)
	}
}

func TestDecide_SupersededIgnoredByJoin(t *testing.T) {
	g := loadGraph(t, "patterns/p02_parallel_join_all.json")
	st := at(g, "work", 1, nil)
	rows := []TaskState{
		closedTask("work", "ta", KindWork, 0, "done"),
		{Step: "work", Key: "tb", Kind: KindWork, Round: 0, Closed: true, Cancelled: true, Outcome: OutcomeSuperseded},
		closedTask("work", "tc", KindWork, 0, "done"),
	}
	wantRoute(t, decideAt(g, st, rows), "work_ok", "done", JoinPass)

	q := loadGraph(t, "patterns/p08_n_of_m_quorum.json")
	rows = []TaskState{
		closedTask("vote", "v1", KindGate, 0, "approved"),
		{Step: "vote", Key: "v2", Kind: KindGate, Round: 0, Closed: true, Cancelled: true, Outcome: OutcomeSuperseded},
		closedTask("vote", "v3", KindGate, 0, "approved"),
	}
	wantRoute(t, decideAt(q, at(q, "vote", 1, nil), rows), "vote_ok", "done", JoinPass)
	// A cancelled voter a person cancelled is a rejection; a superseded one is nothing.
	rows[1] = TaskState{Step: "vote", Key: "v2", Kind: KindGate, Round: 0, Closed: true, Cancelled: true}
	rows[2] = closedTask("vote", "v3", KindGate, 0, "rejected")
	if d := decideAt(q, at(q, "vote", 1, nil), rows); d.JoinResult != JoinFail {
		t.Fatalf("a gate a person cancelled counts as a rejection: %+v", d)
	}
}

func TestDecide_GateClosedWithoutVerdictFails(t *testing.T) {
	g := loadGraph(t, "crossreview.json")
	st := at(g, "review", 1, nil)
	rows := []TaskState{
		closedTask("build", "write_a", KindWork, 0, "done"), closedTask("build", "write_b", KindWork, 0, "done"),
		closedTask("review", "ana_checks_ben", KindGate, 0, ""),
		closedTask("review", "ben_checks_ana", KindGate, 0, "approved"),
	}
	d := decideAt(g, st, rows)
	if d.JoinResult != JoinFail || d.TransitionID != "review_redo" {
		t.Fatalf("a gate closed with no verdict: %+v, want a fail that goes back", d)
	}
}

func TestDecide_QuorumUnreachableFromStartFails(t *testing.T) {
	doc := modify(loadFlowDoc(t, "legal.json"), func(d *FlowDoc) {
		stepIdx(d, "review").Tasks[0].Instances = &Instances{Count: 2}
	})
	g := NewGraph(doc)
	d := decideAt(g, at(g, "review", 1, nil), nil)
	if d.Kind != DecisionRoute || d.JoinResult != JoinFail || len(d.Open) != 0 {
		t.Fatalf("2 reviewers cannot make a quorum of 3: %+v, want a fail with nothing opened", d)
	}
}

func TestDecide_AfterOpensInWaves(t *testing.T) {
	g := loadGraph(t, "patterns/p01_sequential.json")
	st := at(g, "first", 1, nil)
	d := decideAt(g, st, nil)
	if d.Kind != DecisionOpen || !reflect.DeepEqual(openKeys(d), []string{"one"}) {
		t.Fatalf("first wave: %+v", d)
	}
	if d := decideAt(g, st, []TaskState{openTask("first", "one", KindWork, 0)}); d.Kind != DecisionWait {
		t.Fatalf("while the first task runs: %+v", d)
	}
	d = decideAt(g, st, []TaskState{closedTask("first", "one", KindWork, 0, "done")})
	if d.Kind != DecisionOpen || !reflect.DeepEqual(openKeys(d), []string{"two"}) {
		t.Fatalf("second wave: %+v", d)
	}
	wantRoute(t, decideAt(g, st, []TaskState{closedTask("first", "one", KindWork, 0, "done"), closedTask("first", "two", KindWork, 0, "done")}), "first_next", "second", JoinNone)

	t.Run("a task it comes after that does not open does not hold it back", func(t *testing.T) {
		doc := modify(loadFlowDoc(t, "patterns/p04_multi_choice.json"), func(d *FlowDoc) {
			stepIdx(d, "work").Tasks[2].After = []string{"task_a"}
		})
		g := NewGraph(doc)
		d := decideAt(g, at(g, "work", 1, nil), nil)
		if want := []string{"task_c"}; !reflect.DeepEqual(openKeys(d), want) {
			t.Fatalf("opened %v, want %v (task_a is off)", openKeys(d), want)
		}
	})
	t.Run("a blocked task it comes after does not release it", func(t *testing.T) {
		d := decideAt(g, st, []TaskState{closedTask("first", "one", KindWork, 0, "blocked")})
		if d.Kind != DecisionRoute || d.JoinResult != JoinBlock {
			t.Fatalf("a blocked first task: %+v", d)
		}
	})
}

func TestDecide_InstancesFromListVar(t *testing.T) {
	g := loadGraph(t, "patterns/p06_instances_from_var.json")
	st := at(g, "work", 1, map[string]any{"items": []any{"red", "green", "blue"}})
	d := decideAt(g, st, nil)
	if !reflect.DeepEqual(openKeys(d), []string{"item~1", "item~2", "item~3"}) {
		t.Fatalf("opened %v", openKeys(d))
	}
	for i, o := range d.Open {
		if o.Index != i+1 || o.Count != 3 || o.Item != []string{"red", "green", "blue"}[i] || o.Task.Key != "item" {
			t.Errorf("instance %d = %+v", i, o)
		}
	}
	// The default of the variable applies when the request has not set it.
	st.Vars = nil
	if got := openKeys(decideAt(g, st, nil)); !reflect.DeepEqual(got, []string{"item~1"}) {
		t.Fatalf("default list: %v", got)
	}
}

func TestDecide_InstancesRespectMax(t *testing.T) {
	g := loadGraph(t, "patterns/p06_instances_from_var.json")
	st := at(g, "work", 1, map[string]any{"items": []any{"1", "2", "3", "4", "5", "6"}})
	d := decideAt(g, st, nil)
	if len(d.Open) != 4 || d.Open[3].Key != "item~4" || d.Open[0].Count != 4 {
		t.Fatalf("six items with max 4: opened %v", openKeys(d))
	}
}

func TestDecide_InstancesTooFewStalls(t *testing.T) {
	g := loadGraph(t, "patterns/p06_instances_from_var.json")
	st := at(g, "work", 1, map[string]any{"items": []any{}})
	wantStalled(t, decideAt(g, st, nil), ReasonInstancesTooFew)

	doc := modify(loadFlowDoc(t, "patterns/p06_instances_from_var.json"), func(d *FlowDoc) {
		stepIdx(d, "work").Tasks[0].Instances.Min = 3
	})
	g = NewGraph(doc)
	wantStalled(t, decideAt(g, at(g, "work", 1, map[string]any{"items": []any{"a", "b"}}), nil), ReasonInstancesTooFew)
	if d := decideAt(g, at(g, "work", 1, map[string]any{"items": []any{"a", "b", "c"}}), nil); d.Kind != DecisionOpen {
		t.Fatalf("exactly min items: %+v", d)
	}
}

// A task that is open stays part of the entry when a variable changes, and a
// variable that changes can open one more (spec 4.2a). Pattern 15, adding
// instances while a step runs, is this state test and not a separate document:
// it is "one part", and the list of instances itself is frozen.
func TestDecide_OpenTaskSurvivesVarChange(t *testing.T) {
	g := loadGraph(t, "patterns/p04_multi_choice.json")
	rows := []TaskState{openTask("work", "task_a", KindWork, 0), closedTask("work", "task_c", KindWork, 0, "done")}

	// want_a was true when task_a opened; now it is false.
	st := at(g, "work", 1, map[string]any{"want_a": false})
	if d := decideAt(g, st, rows); d.Kind != DecisionWait || len(d.Cancel) != 0 {
		t.Fatalf("task_a is open though its condition is now false: %+v, want it to be waited for, not cancelled", d)
	}
	rows[0] = closedTask("work", "task_a", KindWork, 0, "done")
	wantRoute(t, decideAt(g, st, rows), "work_next", "done", JoinNone)

	// A variable that turns on adds a task on the next advance.
	st = at(g, "work", 1, map[string]any{"want_a": false, "want_b": true})
	d := decideAt(g, st, rows)
	if d.Kind != DecisionOpen || !reflect.DeepEqual(openKeys(d), []string{"task_b"}) {
		t.Fatalf("want_b turned on: %+v, want task_b opened", d)
	}
}

func TestPatterns_Pattern15_LateTaskOpensWhenVariableChanges(t *testing.T) {
	g := loadGraph(t, "patterns/p04_multi_choice.json")
	st := at(g, "work", 1, map[string]any{"want_a": true, "want_b": false})
	d := decideAt(g, st, nil)
	if want := []string{"task_a", "task_c"}; !reflect.DeepEqual(openKeys(d), want) {
		t.Fatalf("first advance opened %v, want %v", openKeys(d), want)
	}
	rows := []TaskState{openTask("work", "task_a", KindWork, 0), openTask("work", "task_c", KindWork, 0)}
	st.Vars["want_b"] = true
	d = decideAt(g, st, rows)
	if want := []string{"task_b"}; d.Kind != DecisionOpen || !reflect.DeepEqual(openKeys(d), want) {
		t.Fatalf("after want_b: %+v, want %v opened while the others run", d, want)
	}
}

func TestDecide_LoopCountsAndResetsOnForwardEntry(t *testing.T) {
	g := loadGraph(t, "trade.json")
	meta := ApplyMeta{Now: "t1"}
	st := at(g, "gate", 1, map[string]any{"customs_lookup": false})
	st.Activations["draft"] = 1
	rows := []TaskState{closedTask("gate", "docs_gate", KindGate, 0, "rejected"), closedTask("gate", "customs_gate", KindGate, 0, "approved")}
	d := decideAt(g, st, rows)
	wantRoute(t, d, "gate_fix", "draft", JoinFail)
	if !d.Rework || d.Loop != "fix" {
		t.Fatalf("the edge closes the loop fix: %+v", d)
	}
	next := ApplyDecision(g, st, d, meta)
	if next.Loops["fix"] != 1 || next.Activations["draft"] != 2 || next.Step != "draft" || next.CameFrom != "gate" || next.Seq != 1 {
		t.Fatalf("after the back-edge: %+v", next)
	}
	if st.Loops["fix"] != 0 || st.Activations["draft"] != 1 || st.Step != "gate" {
		t.Fatalf("ApplyDecision changed its argument: %+v", st)
	}
	if len(next.Trail) != 1 || next.Trail[0] != (TrailEntry{Step: "gate", Activation: 1, Edge: "gate_fix", At: "t1"}) {
		t.Fatalf("trail = %+v", next.Trail)
	}

	// Entering the header by a forward edge resets the loop.
	back := next
	back.Step = "collect"
	back.Activations = map[string]int{"collect": 1}
	back.Loops = map[string]int{"fix": 2}
	forward := ApplyDecision(g, back, GraphDecision{Decision: Decision{Kind: DecisionRoute, TransitionID: "collect_ok", To: "draft"}}, meta)
	if forward.Loops["fix"] != 0 || forward.Activations["draft"] != 1 {
		t.Fatalf("a forward entry into the header: loops %v activations %v, want fix reset", forward.Loops, forward.Activations)
	}
	if back.Loops["fix"] != 2 {
		t.Fatal("ApplyDecision changed the loops of its argument")
	}
}

func TestDecide_NestedLoopResets(t *testing.T) {
	g := loadGraph(t, "patterns/p09_nested_cycles.json")
	script := map[string][]map[string]string{
		"c": {{"gate_c": "rejected"}, {"gate_c": "rejected"}, {"gate_c": "rejected"}, {"gate_c": "rejected"}, {"gate_c": "rejected"}, {"gate_c": "rejected"}},
		"d": {{"gate_d": "rejected"}, {"gate_d": "rejected"}},
	}
	tr := Simulate(g, nil, script, 0)
	if tr.Outcome != TraceStop || tr.Code != "loop_exhausted" {
		t.Fatalf("outcome %s %s", tr.Outcome, tr.Code)
	}
	// The inner loop runs 3 times in each of the 2 passes of the outer loop,
	// because going back to a resets the inner count: forward entry into b.
	for step, want := range map[string]int{"a": 2, "b": 6, "c": 6, "d": 2} {
		if got := tr.Visits(step); got != want {
			t.Errorf("visits of %s = %d, want %d (path %v)", step, got, want, tr.Path())
		}
	}
}

func TestDecide_LoopMaxFromVarIsCapped(t *testing.T) {
	g := loadGraph(t, "research.json")
	l, _ := g.Loop("rounds")
	for _, tt := range []struct {
		vars map[string]any
		want int
	}{
		{map[string]any{"max_rounds": 2}, 2},
		{map[string]any{"max_rounds": 5}, 5},
		{map[string]any{"max_rounds": 9}, 5},
		{map[string]any{"max_rounds": float64(4)}, 4},
		{map[string]any{"max_rounds": -3}, 0},
		{map[string]any{"max_rounds": "many"}, 0},
		{nil, 3},
	} {
		st := at(g, "decide", 1, tt.vars)
		if tt.vars == nil {
			st.Vars = nil
		}
		if got := g.loopLimit(st, l); got != tt.want {
			t.Errorf("vars %v: limit %d, want %d", tt.vars, got, tt.want)
		}
	}
	fixed, _ := g.Loop("fixes")
	if got := g.loopLimit(at(g, "audit", 1, nil), fixed); got != 2 {
		t.Errorf("fixed loop limit = %d", got)
	}
}

func TestDecide_ExhaustedBackEdgeFallsThrough(t *testing.T) {
	g := loadGraph(t, "trade.json")
	rows := []TaskState{closedTask("gate", "docs_gate", KindGate, 2, "rejected"), closedTask("gate", "customs_gate", KindGate, 2, "approved")}
	st := at(g, "gate", 3, map[string]any{"customs_lookup": false})
	st.Loops["fix"] = 1
	wantRoute(t, decideAt(g, st, rows), "gate_fix", "draft", JoinFail)
	st.Loops["fix"] = 2
	d := decideAt(g, st, rows)
	wantStop(t, d, "gate_default", "loop_exhausted")
	if d.Stop != Stop3 || d.Status != "blocked" {
		t.Fatalf("stop %q status %q, want stop 3 at blocked", d.Stop, d.Status)
	}
}

func TestDecide_SameHeaderLoopsAreIndependent(t *testing.T) {
	doc := FlowDoc{Version: 2, Start: "b", Terminals: []Terminal{{Key: "done", Status: "done"}},
		Loops: []Loop{{Key: "short", Header: "b", Max: LoopMax{N: 3}}, {Key: "long", Header: "b", Max: LoopMax{N: 3}}},
		Steps: []DocStep{
			{Key: "b", Kind: StepKindTasks, Edges: []DocEdge{{ID: "b1", To: "c"}}},
			{Key: "c", Kind: StepKindTasks, Edges: []DocEdge{{ID: "c_back", To: "b", Loop: "short"}}},
			{Key: "d", Kind: StepKindTasks, Edges: []DocEdge{{ID: "d_back", To: "b", Loop: "long"}}},
		}}
	g := NewGraph(doc)
	meta := ApplyMeta{}
	st := InitialState(g, nil, "")
	st.Step, st.Activations = "c", map[string]int{"c": 1}
	st.Loops = map[string]int{"short": 0, "long": 2}
	st = ApplyDecision(g, st, GraphDecision{Decision: Decision{Kind: DecisionRoute, TransitionID: "c_back", To: "b", Rework: true}, Loop: "short"}, meta)
	if st.Loops["short"] != 1 || st.Loops["long"] != 2 {
		t.Fatalf("the back-edge of one loop moved the other: %v", st.Loops)
	}
	st.Step = "c"
	st = ApplyDecision(g, st, GraphDecision{Decision: Decision{Kind: DecisionRoute, TransitionID: "c_back", To: "b", Rework: true}, Loop: "short"}, meta)
	if st.Loops["short"] != 2 || st.Loops["long"] != 2 {
		t.Fatalf("loops = %v", st.Loops)
	}
	st.Step = "d"
	st = ApplyDecision(g, st, GraphDecision{Decision: Decision{Kind: DecisionRoute, TransitionID: "d_back", To: "b", Rework: true}, Loop: "long"}, meta)
	if st.Loops["short"] != 2 || st.Loops["long"] != 3 {
		t.Fatalf("loops = %v", st.Loops)
	}
	// Only a forward entry resets either.
	st.Step = "d"
	st = ApplyDecision(g, st, GraphDecision{Decision: Decision{Kind: DecisionRoute, TransitionID: "x", To: "b"}}, meta)
	if st.Loops["short"] != 0 || st.Loops["long"] != 0 {
		t.Fatalf("a forward entry into the shared header: %v", st.Loops)
	}
}

func TestDecide_ResumeCountsAsForwardEntry(t *testing.T) {
	g := loadGraph(t, "trade.json")
	st := at(g, "gate", 3, nil)
	st.Activations["draft"] = 3
	st.Loops["fix"] = 2
	st.Stop = &StopRecord{Code: "loop_exhausted", From: "gate", Edge: "gate_default"}
	st.BlockedFrom = "gate"
	out := ApplyResume(g, st, "draft", ApplyMeta{Now: "t"})
	if out.Step != "draft" || out.Activations["draft"] != 4 || out.Loops["fix"] != 0 || out.Stop != nil || out.BlockedFrom != "" {
		t.Fatalf("resume into the header: %+v", out)
	}
	if st.Stop == nil || st.Loops["fix"] != 2 {
		t.Fatal("ApplyResume changed its argument")
	}
	// Resuming into a step that is not a header leaves the counters.
	out = ApplyResume(g, st, "gate", ApplyMeta{})
	if out.Loops["fix"] != 2 || out.Activations["gate"] != 4 {
		t.Fatalf("resume into gate: loops %v activations %v", out.Loops, out.Activations)
	}
}

func TestDecide_NoEdgeStalls(t *testing.T) {
	doc := modify(loadFlowDoc(t, "patterns/p04_multi_choice.json"), func(d *FlowDoc) {
		stepIdx(d, "work").Edges = []DocEdge{{ID: "never", To: "done", When: &Condition{Field: "var.want_a", Op: OpEq, Value: true}}}
	})
	g := NewGraph(doc)
	rows := []TaskState{closedTask("work", "task_c", KindWork, 0, "done")}
	wantStalled(t, decideAt(g, at(g, "work", 1, map[string]any{"want_a": false}), rows), ReasonNoEdge)

	t.Run("a step that does not exist", func(t *testing.T) {
		wantStalled(t, decideAt(g, at(g, "nowhere", 1, nil), nil), ReasonStepMissing)
	})
	t.Run("a condition that cannot be evaluated", func(t *testing.T) {
		bad := modify(doc, func(d *FlowDoc) {
			stepIdx(d, "work").Edges[0].When = &Condition{Field: "var.ghost", Op: OpEq, Value: true}
		})
		bg := NewGraph(bad)
		wantStalled(t, decideAt(bg, at(bg, "work", 1, nil), rows), ReasonConditionInvalid)
	})
}

func TestDecide_ManualStepWaitsForExit(t *testing.T) {
	g := loadGraph(t, "patterns/p10_deferred_choice.json")
	st := at(g, "choose", 1, nil)
	if d := decideAt(g, st, nil); d.Kind != DecisionWait {
		t.Fatalf("a manual step decides nothing by itself: %+v", d)
	}
	fs := BuildGraphFacts(g, st, nil, RequestFacts{})
	d := RouteManual(g, st, nil, "choose_cancel", fs)
	wantRoute(t, d, "choose_cancel", "cancelled", "")
	if !d.Checkpoint {
		t.Fatal("a person's exit is a checkpoint")
	}
	if d := RouteManual(g, st, nil, "choose_accept", fs); d.To != "done" {
		t.Fatalf("the other exit: %+v", d)
	}
	for _, bad := range []string{"nope", "", "plan_ok"} {
		if d := RouteManual(g, st, nil, bad, fs); d.Kind != DecisionWait || d.Reason != ReasonEdgeUnavailable {
			t.Fatalf("exit %q: %+v, want a refusal", bad, d)
		}
	}
	// A manual exit is not taken by automatic routing, and an automatic edge is not a person's exit.
	if d := RouteManual(g, at(g, "work", 1, nil), nil, "work_next", fs); d.Reason != ReasonEdgeUnavailable {
		t.Fatalf("exit on a tasks step: %+v", d)
	}
	// Its tasks open first, and the step still waits afterwards.
	h := loadGraph(t, "patterns/p11_cancel.json")
	hst := at(h, "hold", 1, nil)
	if d := decideAt(h, hst, nil); d.Kind != DecisionOpen || !reflect.DeepEqual(openKeys(d), []string{"export"}) || d.Open[0].Task.Kind != KindWork {
		t.Fatalf("manual step opens its task: %+v", d)
	}
	if d := decideAt(h, hst, []TaskState{closedTask("hold", "export", KindWork, 0, "done")}); d.Kind != DecisionWait {
		t.Fatalf("manual step with its task done: %+v", d)
	}
	// Reaching a manual step is a human stop.
	prev := loadGraph(t, "patterns/p10_deferred_choice.json")
	d = decideAt(prev, at(prev, "work", 1, nil), []TaskState{closedTask("work", "prepare", KindWork, 0, "done")})
	if !d.HumanStop || d.To != "choose" || d.Notice != "note" {
		t.Fatalf("work to choose: %+v", d)
	}
}

func TestDecide_StopEdgeParksWithCode(t *testing.T) {
	g := loadGraph(t, "trade.json")
	st := at(g, "gate", 1, map[string]any{"customs_lookup": false})
	rows := []TaskState{closedTask("gate", "docs_gate", KindGate, 0, "approved"), closedTask("gate", "customs_gate", KindGate, 0, "blocked")}
	d := decideAt(g, st, rows)
	wantStop(t, d, "gate_blocked", "grounds_missing")
	if d.Stop != Stop1 || d.Status != "blocked" || d.JoinResult != JoinBlock {
		t.Fatalf("stop 1 expected: %+v", d)
	}
	parked := ApplyDecision(g, st, d, ApplyMeta{Now: "t"})
	if parked.Stop == nil || *parked.Stop != (StopRecord{Code: "grounds_missing", From: "gate", Edge: "gate_blocked", At: "t"}) || parked.BlockedFrom != "gate" || parked.Step != "gate" {
		t.Fatalf("parked state = %+v", parked)
	}
	if d := decideAt(g, parked, rows); d.Kind != DecisionWait {
		t.Fatalf("a parked request waits for a manager: %+v", d)
	}

	t.Run("a stop can park the request at another status", func(t *testing.T) {
		doc := modify(loadFlowDoc(t, "trade.json"), func(d *FlowDoc) {
			for i := range d.Stops {
				if d.Stops[i].Code == "grounds_missing" {
					d.Stops[i].Status = "in_review"
				}
			}
		})
		g := NewGraph(doc)
		d := decideAt(g, st, rows)
		if d.StopCode != "grounds_missing" || d.Status != "in_review" {
			t.Fatalf("stop status = %q", d.Status)
		}
	})
	t.Run("a stop that is not declared stalls", func(t *testing.T) {
		doc := modify(loadFlowDoc(t, "trade.json"), func(d *FlowDoc) { stepIdx(d, "gate").Edges[0].Stop = "ghost" })
		g := NewGraph(doc)
		d := decideAt(g, st, rows)
		wantStalled(t, d, ReasonStopUnknown)
	})
	t.Run("the implicit stalled stop", func(t *testing.T) {
		doc := modify(loadFlowDoc(t, "trade.json"), func(d *FlowDoc) { stepIdx(d, "gate").Edges[0].Stop = "stalled" })
		g := NewGraph(doc)
		d := decideAt(g, st, rows)
		if d.StopCode != "stalled" || d.Stop != StopStalled || d.TransitionID != "gate_blocked" {
			t.Fatalf("decision = %+v", d)
		}
	})
}

// releaseDoc is: plan, agent work, then an automatic edge into a terminal that
// leaves the workspace.
func releaseDoc(allow bool, gateRole string) FlowDoc {
	doc := FlowDoc{Version: 2, Start: "plan",
		Roles:     []Role{{Key: "worker", Kind: RoleKindAgent}, {Key: "person", Kind: RoleKindMember}, {Key: "checker", Kind: RoleKindAgent}},
		Terminals: []Terminal{{Key: "done", Status: "done", LeavesWorkspace: true}},
		Policy:    Policy{AllowUnattendedRelease: allow},
		Steps: []DocStep{
			{Key: "plan", Kind: StepKindLead, Edges: []DocEdge{{ID: "plan_ok", To: "work"}}},
			{Key: "work", Kind: StepKindTasks, Tasks: []DocTask{{Key: "make", Title: "Make", Kind: KindWork, Role: "worker"}},
				Edges: []DocEdge{{ID: "work_next", To: "check"}}},
			{Key: "check", Kind: StepKindTasks, Tasks: []DocTask{{Key: "judge", Title: "Judge", Kind: KindGate, Role: gateRole}},
				Edges: []DocEdge{{ID: "check_ok", To: "done", When: &Condition{Field: "join.result", Op: OpEq, Value: "pass"}}, {ID: "check_stop", Stop: "stalled"}}},
		}}
	return doc
}

func TestDecide_TerminalWithoutCheckpointStalls(t *testing.T) {
	g := NewGraph(releaseDoc(false, "checker"))
	st := at(g, "work", 1, nil)
	opened := decideAt(g, st, nil)
	if opened.Kind != DecisionOpen {
		t.Fatalf("%+v", opened)
	}
	st = ApplyOpened(g, st, opened.Open)
	if !st.Unreviewed {
		t.Fatal("a task of an agent that works leaves the request unreviewed")
	}
	st.Step, st.Activations["check"] = "check", 1
	rows := []TaskState{closedTask("check", "judge", KindGate, 0, "approved")}
	d := decideAt(g, st, rows)
	wantStalled(t, d, ReasonReleaseWithoutCheckpoint)
	if d.TransitionID != "check_ok" {
		t.Fatalf("the stalled decision names the edge it refused: %q", d.TransitionID)
	}

	t.Run("an acknowledged exception releases", func(t *testing.T) {
		g := NewGraph(releaseDoc(true, "checker"))
		wantRoute(t, decideAt(g, st, rows), "check_ok", "done", JoinPass)
	})
	t.Run("a rehearsal ignores the exception", func(t *testing.T) {
		g := NewGraph(releaseDoc(true, "checker"))
		trial := st
		trial.Trial = true
		wantStalled(t, decideAt(g, trial, rows), ReasonReleaseWithoutCheckpoint)
		trial.Unreviewed = false
		wantRoute(t, decideAt(g, trial, rows), "check_ok", "done", JoinPass)
	})
	t.Run("a request nobody worked on is not unreviewed", func(t *testing.T) {
		clean := st
		clean.Unreviewed = false
		wantRoute(t, decideAt(g, clean, rows), "check_ok", "done", JoinPass)
	})
	t.Run("a gate does not make work", func(t *testing.T) {
		gate := g
		got := ApplyOpened(gate, at(gate, "check", 1, nil), []OpenTask{{Key: "judge", Task: DocTask{Kind: KindGate, Role: "checker"}}})
		if got.Unreviewed {
			t.Fatal("a gate marked the request unreviewed")
		}
		human := ApplyOpened(gate, at(gate, "check", 1, nil), []OpenTask{{Key: "make", Task: DocTask{Kind: KindWork, Role: "person"}}})
		if human.Unreviewed {
			t.Fatal("work of a person marked the request unreviewed")
		}
	})
}

func TestDecide_ActivationCapStalls(t *testing.T) {
	doc := modify(loadFlowDoc(t, "patterns/p01_sequential.json"), func(d *FlowDoc) { d.Policy.MaxActivations = 3 })
	g := NewGraph(doc)
	rows := []TaskState{closedTask("first", "one", KindWork, 0, "done"), closedTask("first", "two", KindWork, 0, "done")}
	st := at(g, "first", 1, nil)
	st.Activations["plan"] = 1
	wantRoute(t, decideAt(g, st, rows), "first_next", "second", JoinNone)
	st.Activations["plan"] = 2
	d := decideAt(g, st, rows)
	wantStalled(t, d, ReasonFlowCapReached)

	t.Run("the task issue cap", func(t *testing.T) {
		doc := modify(loadFlowDoc(t, "patterns/p05_instances_count.json"), func(d *FlowDoc) { d.Policy.MaxTaskIssues = 4 })
		g := NewGraph(doc)
		st := at(g, "work", 1, nil)
		if d := decideAt(g, st, nil); d.Kind != DecisionOpen || len(d.Open) != 3 {
			t.Fatalf("3 tasks under a cap of 4: %+v", d)
		}
		prior := []TaskState{closedTask("earlier", "x", KindWork, 0, "done"), closedTask("earlier", "y", KindWork, 0, "done")}
		wantStalled(t, decideAt(g, st, prior), ReasonFlowCapReached)
	})
}

func TestDecide_UnreviewedBlocksRelease(t *testing.T) {
	// A person looked at the request once, but an agent produced new work after.
	g := NewGraph(releaseDoc(false, "person"))
	st := at(g, "check", 1, nil)
	st.Checkpoints = []Checkpoint{{Step: "earlier", Activation: 1, By: "u"}}
	st.Unreviewed = true
	rows := []TaskState{closedTask("check", "judge", KindGate, 0, "approved")}
	// The gate is a person's, so passing it is a checkpoint and lets the release through.
	d := decideAt(g, st, rows)
	if d.Kind != DecisionRoute || d.To != "done" || !d.Checkpoint {
		t.Fatalf("a human gate that passes is a checkpoint: %+v", d)
	}
	// An agent's gate is not.
	g = NewGraph(releaseDoc(false, "checker"))
	wantStalled(t, decideAt(g, st, rows), ReasonReleaseWithoutCheckpoint)
	st.Unreviewed = false
	wantRoute(t, decideAt(g, st, rows), "check_ok", "done", JoinPass)
}

func TestDecide_CheckpointClearsUnreviewed(t *testing.T) {
	g := loadGraph(t, "trade.json")
	st := at(g, "release", 1, map[string]any{"export": true})
	st.Unreviewed = true
	rows := []TaskState{closedTask("release", "export", KindWork, 0, "done")}
	fs := BuildGraphFacts(g, st, rows, RequestFacts{})
	d := RouteManual(g, st, rows, "release_done", fs)
	wantRoute(t, d, "release_done", "done", "")
	if !d.Checkpoint {
		t.Fatal("the exit is a checkpoint")
	}
	done := ApplyDecision(g, st, d, ApplyMeta{Now: "t", By: "manager"})
	if done.Unreviewed || done.ClosedAt != "t" || done.Step != "done" ||
		len(done.Checkpoints) != 1 || done.Checkpoints[0] != (Checkpoint{Step: "release", Activation: 1, By: "manager", At: "t"}) {
		t.Fatalf("closed state = %+v", done)
	}
	if !st.Unreviewed || len(st.Checkpoints) != 0 {
		t.Fatal("ApplyDecision changed its argument")
	}
	if d := decideAt(g, done, nil); d.Kind != DecisionWait {
		t.Fatalf("a closed request waits: %+v", d)
	}
}
