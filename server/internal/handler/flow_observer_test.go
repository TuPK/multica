package handler

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/multica-ai/multica/server/internal/issuestatus"
	"github.com/multica-ai/multica/server/internal/issueworkflow"
)

// The oracle observer (spec 9.3 item 7, decision D24). The engine decides every
// advance by the graph of the workflow; in every test of this package the
// observer also asks the frozen schema 1 functions, copied below verbatim from
// before the engine was generalised, and panics when the two differ. A panic
// ends the test binary with the case in the message: a difference is a change
// of behavior of a flow that is running, not a flaky test.
//
// Set MULTICA_FLOW_OBSERVER_LOG=1 to print the running tally on stderr after
// every decision; the last line is the total of the run.

func init() { flowDecideObserver = observeFlowDecision }

var flowObserver struct {
	mu    sync.Mutex
	calls int
	flows map[string]bool
	kinds map[issueworkflow.DecisionKind]int
	diffs []string
}

func observedFlowCalls() int {
	flowObserver.mu.Lock()
	defer flowObserver.mu.Unlock()
	return flowObserver.calls
}

func observeFlowDecision(def issueworkflow.Definition, current issueworkflow.State, tasks []issueworkflow.TaskState, d issueworkflow.GraphDecision) {
	diff := ""
	state := issueworkflow.Unadapt(current)
	var oracle issueworkflow.Decision
	// The oracle knows schema 1 only; a schema 2 graph is checked by its own tests.
	if def.Graph.Compat == issueworkflow.CompatV1 {
		oracle = oracleDecide(def, state, tasks)
		diff = compareWithOracle(def, state, oracle, d)
	}
	if diff != "" {
		diff = fmt.Sprintf("flow %q, step %s, rework %d, round_base %d, switches %v, tasks %+v\n  oracle: %+v\n  graph:  %+v\n  diff: %s",
			def.Name, state.Step, state.Rework, state.RoundBase, state.Switches, tasks, oracle, d, diff)
	}
	flowObserver.mu.Lock()
	flowObserver.calls++
	if flowObserver.flows == nil {
		flowObserver.flows = map[string]bool{}
		flowObserver.kinds = map[issueworkflow.DecisionKind]int{}
	}
	flowObserver.flows[def.Name] = true
	flowObserver.kinds[d.Kind]++
	if diff != "" {
		flowObserver.diffs = append(flowObserver.diffs, diff)
	}
	if os.Getenv("MULTICA_FLOW_OBSERVER_LOG") != "" {
		fmt.Fprintf(os.Stderr, "flow observer: calls=%d flows=%d differences=%d wait=%d open=%d route=%d\n",
			flowObserver.calls, len(flowObserver.flows), len(flowObserver.diffs),
			flowObserver.kinds[issueworkflow.DecisionWait], flowObserver.kinds[issueworkflow.DecisionOpen], flowObserver.kinds[issueworkflow.DecisionRoute])
	}
	flowObserver.mu.Unlock()
	if diff != "" {
		panic("flow decide observer: the graph decides differently from the schema 1 oracle\n  " + diff)
	}
}

// joinResultOfTasksOutcome maps the schema 1 verdict of a step's tasks to the join result.
var joinResultOfTasksOutcome = map[string]string{
	issueworkflow.TasksAllApproved: issueworkflow.JoinPass,
	issueworkflow.TasksAnyRejected: issueworkflow.JoinFail,
	issueworkflow.TasksAnyBlocked:  issueworkflow.JoinBlock,
	issueworkflow.TasksNone:        issueworkflow.JoinNone,
}

// compareWithOracle returns what differs between the oracle and the graph decision.
func compareWithOracle(def issueworkflow.Definition, state issueworkflow.FlowState, o issueworkflow.Decision, d issueworkflow.GraphDecision) string {
	if o.Kind != d.Kind {
		return fmt.Sprintf("kind: oracle %q, graph %q (reason %q)", o.Kind, d.Kind, d.Reason)
	}
	var diffs []string
	switch o.Kind {
	case issueworkflow.DecisionOpen:
		var want, got []string
		for _, t := range o.Missing {
			want = append(want, t.Key)
		}
		for _, t := range d.Open {
			got = append(got, t.Key)
		}
		if strings.Join(want, ",") != strings.Join(got, ",") {
			diffs = append(diffs, fmt.Sprintf("tasks to open: oracle %v, graph %v", want, got))
		}
	case issueworkflow.DecisionRoute:
		target := d.To
		if d.StopCode != "" {
			target = d.Status
		}
		if o.To != target {
			diffs = append(diffs, fmt.Sprintf("target: oracle %q, graph %q (stop code %q)", o.To, target, d.StopCode))
		}
		if o.Rework != d.Rework {
			diffs = append(diffs, fmt.Sprintf("rework: oracle %v, graph %v", o.Rework, d.Rework))
		}
		if o.Stop != d.Stop {
			diffs = append(diffs, fmt.Sprintf("stop: oracle %q, graph %q (code %q)", o.Stop, d.Stop, d.StopCode))
		}
		if o.HumanStop != d.HumanStop {
			diffs = append(diffs, fmt.Sprintf("human stop: oracle %v, graph %v", o.HumanStop, d.HumanStop))
		}
		id := d.TransitionID
		if orig, split := splitTransitionIDs(def)[id]; split {
			id = orig
		}
		if o.TransitionID != id {
			diffs = append(diffs, fmt.Sprintf("edge: oracle %q, graph %q", o.TransitionID, d.TransitionID))
		}
		if o.TasksOutcome != "" && state.Step != issueworkflow.StartStatus {
			if want := joinResultOfTasksOutcome[o.TasksOutcome]; want != d.JoinResult {
				diffs = append(diffs, fmt.Sprintf("join result: oracle %q (%s), graph %q", want, o.TasksOutcome, d.JoinResult))
			}
		}
	}
	return strings.Join(diffs, "; ")
}

// splitTransitionIDs maps the id of an edge that Lift made by splitting a
// transition (the stop 1 edge before the edge that keeps the id) to the id of
// the schema 1 transition it comes from.
func splitTransitionIDs(def issueworkflow.Definition) map[string]string {
	out := map[string]string{}
	for _, s := range def.Steps {
		edges := def.Graph.EdgesFrom(s.StatusKey)
		k := 0
		for _, t := range s.Transitions {
			if k < len(edges) && edges[k].Edge.ID != t.ID {
				out[edges[k].Edge.ID] = t.ID
				k++
			}
			k++
		}
	}
	return out
}

// TestEngine_ObserverSeesEveryAdvance: the hook is called at each decision the
// engine makes, with the stored state and the task rows it decided from. A
// request that opens its tasks and then waits for them is two decisions.
func TestEngine_ObserverSeesEveryAdvance(t *testing.T) {
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)

	type seen struct {
		step string
		kind issueworkflow.DecisionKind
	}
	var calls []seen
	var mu sync.Mutex
	inner := flowDecideObserver
	flowDecideObserver = func(def issueworkflow.Definition, state issueworkflow.State, tasks []issueworkflow.TaskState, d issueworkflow.GraphDecision) {
		mu.Lock()
		calls = append(calls, seen{state.Step, d.Kind})
		mu.Unlock()
		inner(def, state, tasks, d)
	}
	t.Cleanup(func() { flowDecideObserver = inner })

	before := observedFlowCalls()
	f.plan(t, request, nil)
	got := make([]string, len(calls))
	for i, c := range calls {
		got[i] = fmt.Sprintf("%s:%s", c.step, c.kind)
	}
	want := []string{"collect:open", "collect:wait"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("decisions after the plan = %v, want %v", got, want)
	}
	if n := observedFlowCalls() - before; n != len(want) {
		t.Fatalf("the oracle observer saw %d decisions, want %d", n, len(want))
	}

	// Close the task: the next advance decides to route.
	calls = nil
	finishTask(t, tasksAt(t, request, "collect", 0)["docs"], f.Agents.Docs, "")
	advance(t, request)
	if len(calls) == 0 || calls[0].step != "collect" || calls[0].kind != issueworkflow.DecisionRoute {
		t.Fatalf("decisions after the task closed = %v, want collect:route first", calls)
	}

	flowObserver.mu.Lock()
	diffs := len(flowObserver.diffs)
	flowObserver.mu.Unlock()
	if diffs != 0 {
		t.Fatalf("the observer recorded %d differences", diffs)
	}
}

// The schema 1 decision functions as they were before the flow engine was
// generalised (issueworkflow/flow.go: Decide, route, tasksOutcomeOf,
// currentTasks, stalledRoute), copied verbatim under an oracle prefix, as in
// issueworkflow/flow_v1_oracle_test.go. Do not edit them to make a test pass.

func oracleDecide(def issueworkflow.Definition, state issueworkflow.FlowState, tasks []issueworkflow.TaskState) issueworkflow.Decision {
	step, ok := def.Step(state.Step)
	if !ok {
		return oracleStalledRoute()
	}
	// The plan step waits for the lead's plan; a step with nothing to open and
	// nowhere to go (blocked, done) waits for a person.
	if state.Step == issueworkflow.StartStatus || (len(step.Tasks) == 0 && !step.HumanOnly && len(step.Transitions) == 0) {
		return issueworkflow.Decision{Kind: issueworkflow.DecisionWait}
	}
	expected := issueworkflow.ExpectedTasks(step, state.Switches)
	current := oracleCurrentTasks(state, tasks)
	var missing []issueworkflow.Task
	for _, task := range expected {
		if _, has := current[task.Key]; !has {
			missing = append(missing, task)
		}
	}
	if step.HumanOnly {
		if len(missing) > 0 {
			return issueworkflow.Decision{Kind: issueworkflow.DecisionOpen, Missing: missing}
		}
		return issueworkflow.Decision{Kind: issueworkflow.DecisionWait}
	}
	outcome := oracleTasksOutcomeOf(step, current)
	if outcome != issueworkflow.TasksAnyBlocked {
		if len(missing) > 0 {
			return issueworkflow.Decision{Kind: issueworkflow.DecisionOpen, Missing: missing}
		}
		for _, ts := range current {
			if !ts.Closed {
				return issueworkflow.Decision{Kind: issueworkflow.DecisionWait}
			}
		}
	}
	return oracleRoute(def, step, issueworkflow.Facts{
		ReworkCount:  state.Rework,
		TasksOutcome: outcome,
		Switches:     state.Switches,
	})
}

func oracleStalledRoute() issueworkflow.Decision {
	return issueworkflow.Decision{Kind: issueworkflow.DecisionRoute, To: issuestatus.Blocked, Stop: issueworkflow.StopStalled}
}

func oracleRoute(def issueworkflow.Definition, step issueworkflow.Step, facts issueworkflow.Facts) issueworkflow.Decision {
	picked, ok := step.Route(facts)
	if !ok {
		return oracleStalledRoute()
	}
	out := issueworkflow.Decision{
		Kind:         issueworkflow.DecisionRoute,
		To:           picked.ToStatusKey,
		Rework:       picked.Rework,
		TransitionID: picked.ID,
		TasksOutcome: facts.TasksOutcome,
	}
	if out.To == issuestatus.Blocked {
		// A request goes to blocked for lack of grounds when a task or the lead
		// reported it; otherwise the rework rounds ran out.
		if facts.TasksOutcome == issueworkflow.TasksAnyBlocked || facts.Outcome == issueworkflow.OutcomeBlocked || step.StatusKey == issueworkflow.StartStatus {
			out.Stop = issueworkflow.Stop1
		} else {
			out.Stop = issueworkflow.Stop3
		}
	}
	if target, ok := def.Step(out.To); ok && target.HumanOnly {
		out.HumanStop = true
	}
	return out
}

func oracleCurrentTasks(state issueworkflow.FlowState, tasks []issueworkflow.TaskState) map[string]issueworkflow.TaskState {
	out := map[string]issueworkflow.TaskState{}
	for _, ts := range tasks {
		if ts.Step == state.Step && ts.Round == state.Round() {
			out[ts.Key] = ts
		}
	}
	return out
}

func oracleTasksOutcomeOf(step issueworkflow.Step, current map[string]issueworkflow.TaskState) string {
	gates, rejected := 0, false
	for _, ts := range current {
		if ts.Outcome == issueworkflow.OutcomeBlocked {
			return issueworkflow.TasksAnyBlocked
		}
	}
	for _, ts := range current {
		if !ts.Closed {
			// A gate still open has not judged yet.
			continue
		}
		kind := ts.Kind
		for _, task := range step.Tasks {
			if task.Key == ts.Key {
				kind = task.Kind
			}
		}
		if kind != issueworkflow.KindGate {
			continue
		}
		gates++
		if ts.Cancelled || ts.Outcome != issueworkflow.OutcomeApproved {
			rejected = true
		}
	}
	switch {
	case rejected:
		return issueworkflow.TasksAnyRejected
	case gates > 0:
		return issueworkflow.TasksAllApproved
	}
	return issueworkflow.TasksNone
}
