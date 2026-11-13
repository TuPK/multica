package issueworkflow

import (
	"github.com/multica-ai/multica/server/internal/issuestatus"
)

// The schema 1 decision functions as they were when the flow engine was
// generalised (flow.go: Decide, route, tasksOutcomeOf, RoutePlan, currentTasks,
// stalledRoute), copied verbatim under an oracle prefix. They are the reference
// the lifted schema 2 engine is compared with, and the only copy left: the engine
// decides by the graph. Do not edit them to make a test pass: when the graph and
// the oracle differ, one of them changed behavior.

// oracleCase is one input of the oracle comparison.
type oracleCase struct {
	def   Definition
	state FlowState
	tasks []TaskState
}

// oracleDecideCases are the inputs of the TestDecide table (flow_test.go).
func oracleDecideCases() map[string]oracleCase {
	def := tradeFlow()
	gateState := func(rework int) FlowState {
		return FlowState{Step: "gate", Rework: rework, Switches: def.Flow.DefaultSwitches()}
	}
	bothGates := func(round int, docs, customs string) []TaskState {
		return []TaskState{
			closedTask("gate", "docs_gate", KindGate, round, docs),
			closedTask("gate", "customs_gate", KindGate, round, customs),
		}
	}
	cases := map[string]oracleCase{
		"both gates approve":                               {state: gateState(0), tasks: bothGates(0, "approved", "approved")},
		"both gates approve after two reworks":             {state: gateState(2), tasks: bothGates(2, "approved", "approved")},
		"one gate rejects, no rework yet":                  {state: gateState(0), tasks: bothGates(0, "approved", "rejected")},
		"one gate rejects, one rework used":                {state: gateState(1), tasks: bothGates(1, "rejected", "approved")},
		"both gates reject at the limit":                   {state: gateState(2), tasks: bothGates(2, "rejected", "rejected")},
		"a gate closed with no outcome counts as rejected": {state: gateState(0), tasks: bothGates(0, "approved", "")},
		"a cancelled gate counts as rejected": {state: gateState(0), tasks: []TaskState{
			closedTask("gate", "docs_gate", KindGate, 0, "approved"),
			{Step: "gate", Key: "customs_gate", Kind: KindGate, Round: 0, Closed: true, Cancelled: true}}},
		"an outcome the gate may not report counts as rejected": {state: gateState(0), tasks: bothGates(0, "approved", "done")},
		"blocked beats rejected":                                {state: gateState(0), tasks: bothGates(0, "rejected", "blocked")},
		"blocked stops at once while the other gate is open": {state: gateState(0), tasks: []TaskState{
			{Step: "gate", Key: "docs_gate", Kind: KindGate, Round: 0, Outcome: "blocked"},
			openTask("gate", "customs_gate", KindGate, 0)}},
		"a gate is still open": {state: gateState(0), tasks: []TaskState{
			closedTask("gate", "docs_gate", KindGate, 0, "approved"),
			openTask("gate", "customs_gate", KindGate, 0)}},
		"a gate has not been opened yet": {state: gateState(0), tasks: []TaskState{
			closedTask("gate", "docs_gate", KindGate, 0, "approved")}},
		"the step has no tasks yet":             {state: gateState(0)},
		"tasks of an earlier round are ignored": {state: gateState(1), tasks: bothGates(0, "approved", "approved")},
		"tasks of another step are ignored": {state: gateState(0), tasks: []TaskState{
			closedTask("draft", "crosscheck", KindWork, 0, "done")}},
		"collect with the optional task off": {
			state: FlowState{Step: "collect", Switches: map[string]any{"customs_lookup": false}},
			tasks: []TaskState{closedTask("collect", "docs", KindWork, 0, "done")}},
		"collect with the optional task on, only docs opened": {
			state: FlowState{Step: "collect", Switches: map[string]any{"customs_lookup": true}},
			tasks: []TaskState{closedTask("collect", "docs", KindWork, 0, "done")}},
		"collect with the optional task on, both done": {
			state: FlowState{Step: "collect", Switches: map[string]any{"customs_lookup": true}},
			tasks: []TaskState{closedTask("collect", "docs", KindWork, 0, "done"), closedTask("collect", "customs", KindWork, 0, "done")}},
		"collect where docs reports blocked while customs runs": {
			state: FlowState{Step: "collect", Switches: map[string]any{"customs_lookup": true}},
			tasks: []TaskState{{Step: "collect", Key: "docs", Kind: KindWork, Outcome: "blocked"}, openTask("collect", "customs", KindWork, 0)}},
		"draft done": {state: FlowState{Step: "draft", Rework: 1, Switches: def.Flow.DefaultSwitches()},
			tasks: []TaskState{closedTask("draft", "crosscheck", KindWork, 1, "done")}},
		"handover approved enters the human step": {state: FlowState{Step: "handover", Rework: 1, Switches: def.Flow.DefaultSwitches()},
			tasks: []TaskState{closedTask("handover", "handover", KindGate, 1, "approved")}},
		"handover rejected stops for the third time": {state: FlowState{Step: "handover", Switches: def.Flow.DefaultSwitches()},
			tasks: []TaskState{closedTask("handover", "handover", KindGate, 0, "rejected")}},
		"the human step never routes":              {state: FlowState{Step: "in_review", Switches: def.Flow.DefaultSwitches()}},
		"the human step opens the optional export": {state: FlowState{Step: "in_review", Switches: map[string]any{"export": true}}},
		"the human step with a finished export still waits": {
			state: FlowState{Step: "in_review", Switches: map[string]any{"export": true}},
			tasks: []TaskState{closedTask("in_review", "export", KindWork, 0, "done")}},
		"a step the workflow does not have": {state: FlowState{Step: "nowhere"}},
		"the plan step waits":               {state: FlowState{Step: StartStatus, Switches: def.Flow.DefaultSwitches()}},
		"round base shifts the round": {state: FlowState{Step: "gate", RoundBase: 3, Switches: def.Flow.DefaultSwitches()},
			tasks: bothGates(3, "approved", "approved")},
	}
	for name, c := range cases {
		c.def = def
		cases[name] = c
	}
	return cases
}

// oracleDecide says what to do next for a request at state.Step. tasks may hold
// the tasks of every step and round; only those of the current step and round
// count. It is a pure function of its arguments.
func oracleDecide(def Definition, state FlowState, tasks []TaskState) Decision {
	step, ok := def.Step(state.Step)
	if !ok {
		return oracleStalledRoute()
	}
	// The plan step waits for the lead's plan; a step with nothing to open and
	// nowhere to go (blocked, done) waits for a person.
	if state.Step == StartStatus || (len(step.Tasks) == 0 && !step.HumanOnly && len(step.Transitions) == 0) {
		return Decision{Kind: DecisionWait}
	}
	expected := ExpectedTasks(step, state.Switches)
	current := oracleCurrentTasks(state, tasks)
	var missing []Task
	for _, task := range expected {
		if _, has := current[task.Key]; !has {
			missing = append(missing, task)
		}
	}
	if step.HumanOnly {
		if len(missing) > 0 {
			return Decision{Kind: DecisionOpen, Missing: missing}
		}
		return Decision{Kind: DecisionWait}
	}
	outcome := oracleTasksOutcomeOf(step, current)
	if outcome != TasksAnyBlocked {
		if len(missing) > 0 {
			return Decision{Kind: DecisionOpen, Missing: missing}
		}
		for _, ts := range current {
			if !ts.Closed {
				return Decision{Kind: DecisionWait}
			}
		}
	}
	return oracleRoute(def, step, state, Facts{
		ReworkCount:  state.Rework,
		TasksOutcome: outcome,
		Switches:     state.Switches,
	})
}

// oracleRoutePlan routes the plan step once the lead has planned. blocked is the
// lead's report that the grounds are missing.
func oracleRoutePlan(def Definition, state FlowState, blocked bool) Decision {
	step, ok := def.Step(state.Step)
	if !ok {
		return oracleStalledRoute()
	}
	facts := Facts{ReworkCount: state.Rework, TasksOutcome: TasksNone, Switches: state.Switches}
	if blocked {
		facts.Outcome = OutcomeBlocked
	}
	return oracleRoute(def, step, state, facts)
}

func oracleStalledRoute() Decision {
	return Decision{Kind: DecisionRoute, To: issuestatus.Blocked, Stop: StopStalled}
}

func oracleRoute(def Definition, step Step, state FlowState, facts Facts) Decision {
	picked, ok := step.Route(facts)
	if !ok {
		return oracleStalledRoute()
	}
	out := Decision{
		Kind:         DecisionRoute,
		To:           picked.ToStatusKey,
		Rework:       picked.Rework,
		TransitionID: picked.ID,
		TasksOutcome: facts.TasksOutcome,
	}
	if out.To == issuestatus.Blocked {
		// A request goes to blocked for lack of grounds when a task or the lead
		// reported it; otherwise the rework rounds ran out.
		// The plan step can block a request on what the lead chose (a squad that refuses records that
		// are not anonymised): that is a lack of grounds too.
		if facts.TasksOutcome == TasksAnyBlocked || facts.Outcome == OutcomeBlocked || step.StatusKey == StartStatus {
			out.Stop = Stop1
		} else {
			out.Stop = Stop3
		}
	}
	if target, ok := def.Step(out.To); ok && target.HumanOnly {
		out.HumanStop = true
	}
	return out
}

// oracleCurrentTasks indexes the tasks of the request's current step and round.
func oracleCurrentTasks(state FlowState, tasks []TaskState) map[string]TaskState {
	out := map[string]TaskState{}
	for _, ts := range tasks {
		if ts.Step == state.Step && ts.Round == state.Round() {
			out[ts.Key] = ts
		}
	}
	return out
}

// oracleTasksOutcomeOf gathers the verdicts of a step's tasks. A blocked report from
// any task wins; a gate that did not approve (rejected, cancelled, closed with
// no outcome) counts as rejected; a step without gates has no verdict.
func oracleTasksOutcomeOf(step Step, current map[string]TaskState) string {
	gates, rejected := 0, false
	for _, ts := range current {
		if ts.Outcome == OutcomeBlocked {
			return TasksAnyBlocked
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
		if kind != KindGate {
			continue
		}
		gates++
		if ts.Cancelled || ts.Outcome != OutcomeApproved {
			rejected = true
		}
	}
	switch {
	case rejected:
		return TasksAnyRejected
	case gates > 0:
		return TasksAllApproved
	}
	return TasksNone
}
