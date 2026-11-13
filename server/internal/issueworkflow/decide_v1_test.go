package issueworkflow

import (
	"testing"

	"github.com/multica-ai/multica/server/internal/issuestatus"
)

// graphDecide decides a schema 1 request the way the engine does: it lifts the
// definition, reads the state as a State and asks DecideGraph. The result has the
// schema 1 shape: a stop is a route to its board status, the tasks to open are the
// schema 1 tasks, and TasksOutcome is the verdict the join result stands for.
func graphDecide(t *testing.T, def Definition, state FlowState, tasks []TaskState) Decision {
	t.Helper()
	g, err := Lift(def, LiftOpts{Runtime: true})
	if err != nil {
		t.Fatalf("Lift: %v", err)
	}
	st := Adapt(state)
	d := DecideGraph(g, st, tasks, BuildGraphFacts(g, st, tasks, RequestFacts{}))
	return legacyDecision(def, state, d)
}

// graphRoutePlan routes the plan step once the lead has planned, as PlanFlow does.
func graphRoutePlan(t *testing.T, def Definition, state FlowState, blocked bool) Decision {
	t.Helper()
	g, err := Lift(def, LiftOpts{Runtime: true})
	if err != nil {
		t.Fatalf("Lift: %v", err)
	}
	verdict := "planned"
	if blocked {
		verdict = OutcomeBlocked
	}
	st := Adapt(state)
	return legacyDecision(def, state, RouteLead(g, st, verdict, BuildGraphFacts(g, st, nil, RequestFacts{})))
}

func legacyDecision(def Definition, state FlowState, d GraphDecision) Decision {
	out := d.Decision
	switch out.Kind {
	case DecisionOpen:
		for _, o := range d.Open {
			if task, ok := def.V1Task(state.Step, o.Key); ok {
				out.Missing = append(out.Missing, task)
			}
		}
	case DecisionRoute:
		if d.StopCode != "" {
			out.To = d.Status
		}
		for outcome, result := range joinResultOf {
			if result == d.JoinResult && d.JoinResult != "" {
				out.TasksOutcome = outcome
			}
		}
	}
	return out
}

// The named inputs of the oracle table decide the same through the graph. The
// differential test covers every state of the catalog flows; this keeps the
// cases the schema 1 tests were written for.
func TestGraphDecidesTheOracleCases(t *testing.T) {
	g := map[string]Graph{}
	for name, c := range oracleDecideCases() {
		t.Run("Decide/"+name, func(t *testing.T) {
			graph, ok := g["trade"]
			if !ok {
				var err error
				if graph, err = Lift(c.def, LiftOpts{Runtime: true}); err != nil {
					t.Fatal(err)
				}
				g["trade"] = graph
			}
			want := oracleDecide(c.def, c.state, c.tasks)
			st := Adapt(c.state)
			got := DecideGraph(graph, st, c.tasks, BuildGraphFacts(graph, st, c.tasks, RequestFacts{}))
			if diff := compareDecisions(want, got, splitEdgeIDs(c.def, graph), c.state.Step != StartStatus); diff != "" {
				t.Fatalf("the graph decides differently from the oracle: %s\n  oracle: %+v\n  graph:  %+v", diff, want, got)
			}
		})
	}

	def := tradeFlow()
	graph, err := Lift(def, LiftOpts{Runtime: true})
	if err != nil {
		t.Fatal(err)
	}
	planStates := map[string]FlowState{
		"plan step":   {Step: "todo", Switches: def.Flow.DefaultSwitches()},
		"later round": {Step: "todo", Rework: 1, RoundBase: 2, Switches: def.Flow.DefaultSwitches()},
	}
	for name, state := range planStates {
		for _, blocked := range []bool{false, true} {
			t.Run("RoutePlan/"+name, func(t *testing.T) {
				want := oracleRoutePlan(def, state, blocked)
				verdict := "planned"
				if blocked {
					verdict = OutcomeBlocked
				}
				st := Adapt(state)
				got := RouteLead(graph, st, verdict, BuildGraphFacts(graph, st, nil, RequestFacts{}))
				if diff := compareDecisions(want, got, splitEdgeIDs(def, graph), false); diff != "" {
					t.Fatalf("RouteLead(blocked=%v) differs from RoutePlan: %s", blocked, diff)
				}
			})
		}
	}
	// An unknown step has no plan to route.
	if got := oracleRoutePlan(def, FlowState{Step: "nowhere"}, false); got.To != issuestatus.Blocked || got.Stop != StopStalled {
		t.Fatalf("the oracle routes an unknown step to %+v", got)
	}
}

// A schema 1 request never resets its rework count by entering a step: the
// loop header of the lifted graph is the step the rework edges return to, and
// a forward entry into it (a resume) must keep the count.
func TestApplyResume_CompatV1KeepsTheReworkCount(t *testing.T) {
	def := tradeFlow()
	g, err := Lift(def, LiftOpts{Runtime: true})
	if err != nil {
		t.Fatal(err)
	}
	loop, ok := g.Loop(LoopRework)
	if !ok || loop.Header == "" {
		t.Fatalf("the trade flow has no rework loop with a header: %+v", loop)
	}
	st := Adapt(FlowState{Step: "gate", Rework: 2, Switches: def.Flow.DefaultSwitches()})
	got := ApplyResume(g, st, loop.Header, ApplyMeta{Now: "2026-11-13T00:00:00Z"})
	if got.Loops[LoopRework] != 2 {
		t.Fatalf("rework count after entering the loop header = %d, want 2", got.Loops[LoopRework])
	}

	g.Compat = CompatNone
	if got := ApplyResume(g, st, loop.Header, ApplyMeta{Now: "2026-11-13T00:00:00Z"}); got.Loops[LoopRework] != 0 {
		t.Fatalf("a schema 2 graph keeps the count on entering the header: %d, want 0", got.Loops[LoopRework])
	}
}

// A decision records the facts the edges it tried read (spec 4.13), so a reader
// can tell why the request went this way.
func TestDecideGraph_RecordsTheFactsTheEdgesRead(t *testing.T) {
	def := tradeFlow()
	g, err := Lift(def, LiftOpts{Runtime: true})
	if err != nil {
		t.Fatal(err)
	}
	state := FlowState{Step: "gate", Rework: 1, Switches: def.Flow.DefaultSwitches()}
	tasks := []TaskState{
		closedTask("gate", "docs_gate", KindGate, 1, "approved"),
		closedTask("gate", "customs_gate", KindGate, 1, "rejected"),
	}
	st := Adapt(state)
	d := DecideGraph(g, st, tasks, BuildGraphFacts(g, st, tasks, RequestFacts{}))
	if d.Kind != DecisionRoute || !d.Rework {
		t.Fatalf("decision = %+v, want a rework route", d)
	}
	if got := d.Facts["join.result"]; got != JoinFail {
		t.Fatalf("join.result = %v, want %q; facts %v", got, JoinFail, d.Facts)
	}
	if got := d.Facts["loop.rework.count"]; got != 1 {
		t.Fatalf("loop.rework.count = %v, want 1; facts %v", got, d.Facts)
	}
}

func TestStalledDecision(t *testing.T) {
	d := StalledDecision("agent_unavailable")
	if d.Kind != DecisionRoute || d.Stop != StopStalled || d.StopCode != DocStopStalled || d.Status != DefaultStopStatus || d.Reason != "agent_unavailable" {
		t.Fatalf("StalledDecision = %+v", d)
	}
}
