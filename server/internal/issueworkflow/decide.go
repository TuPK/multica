package issueworkflow

import (
	"fmt"
	"strings"
)

// The decision of the schema 2 engine (spec section 4.2 to 4.8). DecideGraph is
// a pure function of the graph, the stored state, the task rows and the facts:
// it reads no clock, no database and no text. Another function applies a
// decision to a state (ApplyDecision), so a simulation and the engine move a
// request the same way.
//
// For a graph lifted from schema 1 it decides what the schema 1 Decide decides;
// lift_differential_test.go compares the two on every state of the 21 catalog flows.

// Reasons a request stalls (the stalled stop code with a reason, spec 4.6).
const (
	ReasonStepMissing              = "step_missing"
	ReasonNoEdge                   = "no_edge"
	ReasonConditionInvalid         = "condition_invalid"
	ReasonInstancesTooFew          = "instances_too_few"
	ReasonFlowCapReached           = "flow_cap_reached"
	ReasonReleaseWithoutCheckpoint = "release_without_checkpoint"
	ReasonInputMissing             = "input_missing"
	ReasonLeadVerdictInvalid       = "lead_verdict_invalid"
	ReasonStopUnknown              = "stop_unknown"
	// ReasonEdgeUnavailable: a person chose an exit that cannot be taken now.
	ReasonEdgeUnavailable = "edge_unavailable"
	// ReasonAgentUnavailable: the agent of a task cannot take it (archived, left the squad).
	ReasonAgentUnavailable = "agent_unavailable"
	// ReasonTaskStuck: a task or the lead used all its attempts without closing.
	ReasonTaskStuck = "task_stuck"
)

// The values of join.result.
const (
	JoinPass  = "pass"
	JoinFail  = "fail"
	JoinBlock = "block"
	JoinNone  = "none"
)

// DefaultStopStatus is the status of a request parked at a stop that names none.
const DefaultStopStatus = "blocked"

// OpenTask is a task row the engine opens: a task, or one instance of it.
type OpenTask struct {
	// Key is the row key: the task key, or key~i for the i-th instance.
	Key  string
	Task DocTask
	// Index is the 1-based instance, 0 for a task without instances.
	Index int
	// Count is the number of instances, 0 for a task without instances.
	Count int
	// Item is the element of the list variable this instance works on. It goes
	// into the description as quoted data, never into a fact (spec 2.5).
	Item string
}

// GraphDecision is the result of DecideGraph. It extends Decision, whose fields
// keep their meaning for a graph lifted from schema 1:
//
//   - Kind is wait, open or route.
//   - To is the key of the step or terminal the request moves to; empty when it
//     stops. Status is the board status the request gets, also for a stop.
//   - Rework is true when the edge closes a loop; TransitionID is the edge id.
//   - Stop is the schema 1 stop (1, 3, stalled) of a stop code that has a legacy
//     number, and stalled for the implicit stalled stop.
//   - HumanStop is true when the target step is manual.
type GraphDecision struct {
	Decision
	// Open lists the task rows to open (Kind open). Missing is not used.
	Open []OpenTask
	// StopCode is the stop the request parks in; "stalled" for the implicit one.
	StopCode string
	// Reason says why a request stalled, or why a wait is a refusal.
	Reason string
	Status string
	Loop   string
	// JoinResult is the result of the join the edge was chosen on.
	JoinResult string
	// Cancel lists the open task rows (as "step/key") the engine supersedes: the
	// rest of a join that finished early, and every open task at a terminal.
	Cancel []string
	// Checkpoint: the request passes a human checkpoint with this move.
	Checkpoint bool
	// Notice is the message key of the target step.
	Notice string
	// Facts are the values of the facts the edges tried read, the chosen edge
	// and the ones before it: why the request went this way (spec 4.13). A fact
	// that has no value is left out.
	Facts map[string]any
}

// RowRef names a task row of a request: the step and the row key.
func RowRef(step, key string) string { return step + "/" + key }

// entryRound is the flow_round of the tasks of the current entry into the
// step: the activation, or the schema 1 round on a compat v1 graph.
func entryRound(g Graph, st State) int {
	if g.Compat == CompatV1 {
		return st.Round()
	}
	return currentRound(st)
}

// StalledDecision is the decision to park a request in the implicit stalled
// stop, for a reason the engine found (a task that cannot open, a deadline).
func StalledDecision(reason string) GraphDecision { return stalledGraph(reason) }

func stalledGraph(reason string) GraphDecision {
	return GraphDecision{
		Decision: Decision{Kind: DecisionRoute, Stop: StopStalled},
		StopCode: DocStopStalled, Status: DefaultStopStatus, Reason: reason,
	}
}

func waitGraph(reason string) GraphDecision {
	return GraphDecision{Decision: Decision{Kind: DecisionWait}, Reason: reason}
}

// isStopStatus reports whether the key is the status of a stop. A request parked
// at a stop of a lifted graph keeps the schema 1 step of that status (blocked),
// which is not a step of the graph.
func (g Graph) isStopStatus(key string) bool {
	if key == DefaultStopStatus {
		return true
	}
	for _, s := range g.Doc.Stops {
		if s.Status != "" && s.Status == key {
			return true
		}
	}
	return false
}

// instance is a task row that the step expects.
type instance struct {
	task  DocTask
	key   string
	index int
	count int
	item  string
}

// DecideGraph says what to do next for a request at st.Step. tasks may hold the
// rows of every step and entry; only those of the current entry count. fs is the
// fact set of the request (BuildGraphFacts); the function does not change it.
func DecideGraph(g Graph, st State, tasks []TaskState, fs FactSet) GraphDecision {
	if st.ClosedAt != "" || st.Stop != nil {
		return waitGraph("")
	}
	if _, ok := g.Terminal(st.Step); ok {
		return waitGraph("")
	}
	step, ok := g.Step(st.Step)
	if !ok {
		if g.isStopStatus(st.Step) {
			return waitGraph("")
		}
		return stalledGraph(ReasonStepMissing)
	}
	if step.Kind == StepKindLead {
		return waitGraph("")
	}
	manual := step.Kind == StepKindManual
	if len(step.Tasks) == 0 && !manual && len(step.Edges) == 0 {
		return waitGraph("")
	}

	round := entryRound(g, st)
	rows, order := entryRows(st.Step, round, tasks)
	expected, reason := expectedInstances(g, st, step, fs)
	if reason != "" {
		return stalledGraph(reason)
	}
	join := step.EffectiveJoin()
	stats := analyzeJoin(g.Doc, step, join, expected, rows, order)
	ready := readyInstances(g.Doc, step, expected, rows)

	if manual {
		// A manual step never routes by itself: it opens its tasks and waits for
		// a person to choose an exit (RouteManual).
		return g.openOrWait(st, tasks, step, ready)
	}

	complete, result, cancelOpen := stats.complete(join, len(ready))
	if !complete {
		if len(ready) > 0 {
			return g.openOrWait(st, tasks, step, ready)
		}
		return waitGraph("")
	}

	fs2 := fs.Clone()
	stats.setFacts(&fs2, result)
	d, ok := g.route(st, tasks, step, fs2, "", result)
	if cancelOpen && ok {
		for _, ref := range openRefs(step.Key, rows, order) {
			if !inStrings(d.Cancel, ref) {
				d.Cancel = append(d.Cancel, ref)
			}
		}
	}
	return d
}

// openOrWait opens the ready tasks, or waits when there are none. It refuses to
// open tasks past the cap on task issues or without a required input.
func (g Graph) openOrWait(st State, tasks []TaskState, step DocStep, ready []instance) GraphDecision {
	if len(ready) == 0 {
		return waitGraph("")
	}
	if len(tasks)+len(ready) > g.Doc.Policy.Effective().MaxTaskIssues {
		return stalledGraph(ReasonFlowCapReached)
	}
	opened := make([]OpenTask, 0, len(ready))
	for _, in := range ready {
		if g.Compat != CompatV1 && missingRequiredInput(in.task, tasks) {
			return stalledGraph(ReasonInputMissing)
		}
		opened = append(opened, OpenTask{Key: in.key, Task: in.task, Index: in.index, Count: in.count, Item: in.item})
	}
	return GraphDecision{Decision: Decision{Kind: DecisionOpen}, Open: opened}
}

// missingRequiredInput reports whether a required input of the task has no row
// to read yet (spec 4.11, E25).
func missingRequiredInput(task DocTask, tasks []TaskState) bool {
	for _, in := range task.Inputs {
		if !in.Required {
			continue
		}
		step, key, ok := strings.Cut(in.From, ".")
		if !ok {
			return true
		}
		found := false
		for _, ts := range tasks {
			if ts.Step == step && instanceBaseKey(ts.Key) == key && ts.Outcome != OutcomeSuperseded {
				found = true
				break
			}
		}
		if !found {
			return true
		}
	}
	return false
}

// entryRows indexes the rows of the current entry by row key. When a key repeats,
// the last row wins, as the schema 1 index of current tasks does.
func entryRows(step string, round int, tasks []TaskState) (map[string]TaskState, []string) {
	rows := map[string]TaskState{}
	var order []string
	for _, ts := range tasks {
		if ts.Step != step || ts.Round != round {
			continue
		}
		if _, seen := rows[ts.Key]; !seen {
			order = append(order, ts.Key)
		}
		rows[ts.Key] = ts
	}
	return rows, order
}

// expectedInstances lists the rows the step expects at this moment, in task
// order: the tasks whose when holds, each expanded into its instances. The
// condition is evaluated again on every decision, so a variable that changes
// opens more tasks on the next advance (spec 4.2a).
func expectedInstances(g Graph, st State, step DocStep, fs FactSet) ([]instance, string) {
	var out []instance
	for _, t := range step.Tasks {
		if t.When != nil {
			ok, err := EvaluateWith(*t.When, fs)
			if err != nil {
				return nil, ReasonConditionInvalid
			}
			if !ok {
				continue
			}
		}
		if t.Instances == nil {
			out = append(out, instance{task: t, key: t.Key})
			continue
		}
		n, items, reason := instanceList(g, st, *t.Instances)
		if reason != "" {
			return nil, reason
		}
		for i := 1; i <= n; i++ {
			in := instance{task: t, key: fmt.Sprintf("%s~%d", t.Key, i), index: i, count: n}
			if items != nil {
				in.item = items[i-1]
			}
			out = append(out, in)
		}
	}
	return out, ""
}

// instanceList is the number of instances of a task and, for from_var, the list
// element of each. A list shorter than min stalls the request.
func instanceList(g Graph, st State, in Instances) (int, []string, string) {
	if in.FromVar == "" {
		n := in.Count
		if n > DocMaxInstances {
			n = DocMaxInstances
		}
		if n < 1 {
			return 0, nil, ReasonInstancesTooFew
		}
		return n, nil, ""
	}
	list := listVar(g, st, in.FromVar)
	min := in.Min
	if min < 1 {
		min = 1
	}
	if len(list) < min {
		return 0, nil, ReasonInstancesTooFew
	}
	max := in.Max
	if max < 1 || max > DocMaxInstances {
		max = DocMaxInstances
	}
	n := len(list)
	if n > max {
		n = max
	}
	return n, list[:n], ""
}

// varValue is the value of a variable: the stored one, or the default of the
// document.
func varValue(g Graph, st State, key string) (any, bool) {
	if v, ok := st.Vars[key]; ok {
		return v, true
	}
	for _, v := range g.Doc.Vars {
		if v.Key == key {
			return v.Default, true
		}
	}
	return nil, false
}

func listVar(g Graph, st State, key string) []string {
	raw, ok := varValue(g, st, key)
	if !ok {
		return nil
	}
	fv, ok := varFactValue(Var{Type: VarTypeListString}, raw)
	if !ok {
		return nil
	}
	return fv.List
}

// readyInstances are the expected rows that have no row yet and whose after
// tasks are all closed without a block. An after task that is not expected (its
// when is false) does not hold the task back.
func readyInstances(doc FlowDoc, step DocStep, expected []instance, rows map[string]TaskState) []instance {
	byTask := map[string][]instance{}
	for _, in := range expected {
		byTask[in.task.Key] = append(byTask[in.task.Key], in)
	}
	var ready []instance
	for _, in := range expected {
		if _, opened := rows[in.key]; opened {
			continue
		}
		satisfied := true
		for _, dep := range in.task.After {
			for _, d := range byTask[dep] {
				row, has := rows[d.key]
				if !has {
					satisfied = false
					break
				}
				class, closed, counted := docTaskClass(doc, step, row)
				if counted && (!closed || class == ClassBlock) {
					satisfied = false
					break
				}
			}
			if !satisfied {
				break
			}
		}
		if satisfied {
			ready = append(ready, in)
		}
	}
	return ready
}

// joinStats are the counts of the rows of the current entry.
type joinStats struct {
	all classCounts
	// over are the rows the join judges.
	pass, fail, neutral int
	openOver            int
	// anyBlock: some row of the entry, over the join or not, reported blocked.
	anyBlock bool
	// open counts the rows that are not closed and did not report blocked.
	open         int
	unopened     int
	unopenedOver int
}

func inOver(over JoinOver, kind, base string) bool {
	switch over.Kind {
	case "", JoinOverGates:
		return kind == KindGate
	case JoinOverAll:
		return true
	}
	return inStrings(over.Tasks, base)
}

func analyzeJoin(doc FlowDoc, step DocStep, join Join, expected []instance, rows map[string]TaskState, order []string) joinStats {
	var j joinStats
	for _, key := range order {
		ts := rows[key]
		class, closed, counted := docTaskClass(doc, step, ts)
		if !counted {
			continue
		}
		j.all.add(class, closed, counted)
		base := instanceBaseKey(ts.Key)
		kind := ts.Kind
		for _, t := range step.Tasks {
			if t.Key == base {
				kind = t.Kind
			}
		}
		over := inOver(join.Over, kind, base)
		switch {
		case class == ClassBlock:
			j.anyBlock = true
		case !closed:
			j.open++
			if over {
				j.openOver++
			}
		case over:
			switch class {
			case ClassPass:
				j.pass++
			case ClassFail:
				j.fail++
			case ClassNeutral:
				j.neutral++
			}
		}
	}
	for _, in := range expected {
		if _, opened := rows[in.key]; opened {
			continue
		}
		j.unopened++
		if inOver(join.Over, in.task.Kind, in.task.Key) {
			j.unopenedOver++
		}
	}
	return j
}

// complete says whether the join has finished, with what result, and whether the
// open rows are superseded (spec 4.3). ready is the number of rows that can open.
func (j joinStats) complete(join Join, ready int) (done bool, result string, cancelOpen bool) {
	atClose := join.Block == JoinBlockAtClose
	if j.anyBlock && !atClose {
		return true, JoinBlock, false
	}
	fullyClosed := j.open == 0 && j.unopened == 0
	// Nothing can open and nothing is running, but a task is held back by a task
	// that blocked: the step cannot go on and finishes with what it has.
	stuck := j.open == 0 && ready == 0 && j.unopened > 0 && j.anyBlock
	finished := fullyClosed || stuck

	switch join.Mode {
	case JoinModeAny:
		if j.pass > 0 {
			return true, JoinPass, true
		}
		if finished {
			switch {
			case j.anyBlock:
				return true, JoinBlock, false
			case j.fail > 0:
				return true, JoinFail, false
			}
			return true, JoinNone, false
		}
	case JoinModeQuorum:
		k := join.K
		if k < 1 {
			k = 1
		}
		if j.pass >= k {
			return true, JoinPass, true
		}
		if j.pass+j.openOver+j.unopenedOver < k && !(atClose && j.anyBlock) {
			return true, JoinFail, true
		}
		if finished {
			if j.anyBlock {
				return true, JoinBlock, false
			}
			return true, JoinFail, false
		}
	default:
		if finished {
			switch {
			case j.anyBlock:
				return true, JoinBlock, false
			case j.fail > 0:
				return true, JoinFail, false
			case j.pass > 0:
				return true, JoinPass, false
			}
			return true, JoinNone, false
		}
	}
	return false, "", false
}

// setFacts gives the join facts their values for the edges.
func (j joinStats) setFacts(fs *FactSet, result string) {
	_ = fs.Set("join.result", StringValue(result))
	_ = fs.Set("join.pass", IntValue(j.all.pass))
	_ = fs.Set("join.fail", IntValue(j.all.fail))
	_ = fs.Set("join.block", IntValue(j.all.block))
	_ = fs.Set("join.neutral", IntValue(j.all.neutral))
	_ = fs.Set("join.open", IntValue(j.all.open))
	_ = fs.Set("join.total", IntValue(j.all.total))
}

func openRefs(step string, rows map[string]TaskState, order []string) []string {
	var out []string
	for _, key := range order {
		ts := rows[key]
		if !ts.Closed && ts.Outcome != OutcomeSuperseded && ts.Outcome != OutcomeBlocked {
			out = append(out, RowRef(step, key))
		}
	}
	return out
}

// RouteLead routes a lead step once the lead has reported its verdict: a key of
// the step's verdict set (planned in the built-in plan set) or blocked.
func RouteLead(g Graph, st State, verdict string, fs FactSet) GraphDecision {
	step, ok := g.Step(st.Step)
	if !ok || step.Kind != StepKindLead {
		return stalledGraph(ReasonStepMissing)
	}
	set := VerdictSetPlan
	if step.Lead != nil && step.Lead.Verdicts != "" {
		set = step.Lead.Verdicts
	}
	valid := verdict == OutcomeBlocked
	for _, v := range verdictsOf(g.Doc, KindWork, set) {
		if v.Key == verdict {
			valid = true
		}
	}
	if !valid {
		return stalledGraph(ReasonLeadVerdictInvalid)
	}
	fs2 := fs.Clone()
	_ = fs2.Set("lead.verdict", StringValue(verdict))
	d, _ := g.route(st, nil, step, fs2, "", "")
	return d
}

// RouteManual takes the exit a person chose on a manual step. The request is
// not moved when the exit cannot be taken: the result is a wait with
// ReasonEdgeUnavailable. The exit is a human checkpoint.
func RouteManual(g Graph, st State, tasks []TaskState, edgeID string, fs FactSet) GraphDecision {
	step, ok := g.Step(st.Step)
	if !ok || step.Kind != StepKindManual {
		return waitGraph(ReasonEdgeUnavailable)
	}
	edge, ok := g.Edge(step.Key, edgeID)
	if !ok || edge.Edge.Trigger != EdgeTriggerManual {
		return waitGraph(ReasonEdgeUnavailable)
	}
	d, ok := g.route(st, tasks, step, fs, edgeID, "")
	if !ok {
		return waitGraph(ReasonEdgeUnavailable)
	}
	return d
}

// route picks the edge and builds the decision. manualID names the exit a person
// chose; empty for automatic routing. The bool is false when no decision was
// made for an exit that was asked for.
func (g Graph) route(st State, tasks []TaskState, step DocStep, fs FactSet, manualID, result string) (GraphDecision, bool) {
	var chosen *GraphEdge
	var read []string
	for _, e := range g.EdgesFrom(step.Key) {
		if manualID != "" {
			if e.Edge.ID != manualID {
				continue
			}
		} else if e.Edge.Trigger == EdgeTriggerManual {
			continue
		}
		if e.Edge.When != nil {
			walkCondition(*e.Edge.When, func(c Condition) { read = append(read, c.Field) })
			ok, err := EvaluateWith(*e.Edge.When, fs)
			if err != nil {
				return stalledGraph(ReasonConditionInvalid), true
			}
			if !ok {
				continue
			}
		}
		if e.Back && !g.loopHasRoom(st, e.Edge.Loop) {
			continue
		}
		e := e
		chosen = &e
		break
	}
	if chosen == nil {
		if manualID != "" {
			return GraphDecision{}, false
		}
		d := stalledGraph(ReasonNoEdge)
		d.Facts = factValues(fs, read)
		return d, true
	}
	d := g.decisionFor(st, tasks, step, *chosen, result)
	d.Facts = factValues(fs, read)
	return d, true
}

// factValues reads the values of the named facts, each once. A fact without a
// value is left out.
func factValues(fs FactSet, keys []string) map[string]any {
	out := map[string]any{}
	for _, key := range keys {
		spec, ok := fs.Spec(key)
		if !ok {
			continue
		}
		v, present := fs.Value(key)
		if !present {
			continue
		}
		switch spec.Type {
		case VarTypeBool:
			out[key] = v.Bool
		case VarTypeInt:
			out[key] = v.Int
		case VarTypeListString:
			out[key] = v.List
		default:
			out[key] = v.Str
		}
	}
	return out
}

// loopHasRoom reports whether the loop may be followed once more.
func (g Graph) loopHasRoom(st State, loop string) bool {
	l, ok := g.Loop(loop)
	if !ok {
		return false
	}
	return st.Loops[loop] < g.loopLimit(st, l)
}

// loopLimit is the bound of a loop now: the number, or the variable clamped to
// its cap (spec 4.5).
func (g Graph) loopLimit(st State, l Loop) int {
	if !l.Max.FromVar() {
		return l.Max.N
	}
	raw, _ := varValue(g, st, l.Max.Var)
	n := 0
	if fv, ok := varFactValue(Var{Type: VarTypeInt}, raw); ok {
		n = fv.Int
	}
	if n > l.Max.Cap {
		n = l.Max.Cap
	}
	if n < 0 {
		n = 0
	}
	return n
}

// decisionFor builds the decision of following an edge.
func (g Graph) decisionFor(st State, tasks []TaskState, step DocStep, ge GraphEdge, result string) GraphDecision {
	e := ge.Edge
	d := GraphDecision{
		Decision:   Decision{Kind: DecisionRoute, Rework: ge.Back, TransitionID: e.ID},
		Loop:       e.Loop,
		JoinResult: result,
	}
	if e.Stop != "" {
		code := e.Stop
		d.StopCode = code
		d.Status = DefaultStopStatus
		switch kind, declared := g.Stop(code); {
		case code == DocStopStalled:
			d.Stop = StopStalled
		case declared:
			if kind.Status != "" {
				d.Status = kind.Status
			}
			switch kind.LegacyNo {
			case 1:
				d.Stop = Stop1
			case 3:
				d.Stop = Stop3
			}
		default:
			bad := stalledGraph(ReasonStopUnknown)
			bad.TransitionID = e.ID
			return bad
		}
		return d
	}

	checkpoint := step.Kind == StepKindManual || g.isCheckpointEdge(step, result)
	if target, ok := g.Terminal(e.To); ok {
		unreviewed := st.Unreviewed && !checkpoint
		if target.LeavesWorkspace && unreviewed && !g.Doc.Policy.AllowUnattendedRelease {
			bad := stalledGraph(ReasonReleaseWithoutCheckpoint)
			bad.TransitionID = e.ID
			return bad
		}
		d.To, d.Status, d.Checkpoint = target.Key, target.Status, checkpoint
		if g.Compat != CompatV1 {
			d.Cancel = openTaskRefs(tasks)
		}
		return d
	}
	next, ok := g.Step(e.To)
	if !ok {
		bad := stalledGraph(ReasonStepMissing)
		bad.TransitionID = e.ID
		return bad
	}
	total := 0
	for _, n := range st.Activations {
		total += n
	}
	if total+1 > g.Doc.Policy.Effective().MaxActivations {
		bad := stalledGraph(ReasonFlowCapReached)
		bad.TransitionID = e.ID
		return bad
	}
	d.To, d.Status, d.Checkpoint, d.Notice = next.Key, next.Status.Key, checkpoint, next.Notice
	d.HumanStop = next.Kind == StepKindManual
	return d
}

func openTaskRefs(tasks []TaskState) []string {
	var out []string
	for _, ts := range tasks {
		if !ts.Closed && ts.Outcome != OutcomeSuperseded && ts.Outcome != OutcomeBlocked {
			out = append(out, RowRef(ts.Step, ts.Key))
		}
	}
	return out
}

// isCheckpointEdge reports whether leaving the tasks step with this join result
// is a human checkpoint: the step judges, with mode all, a gate given to a
// person, and the result is pass (spec 4.8, Y01). The static rule of the
// validator and this runtime rule use the same definition of a human gate.
func (g Graph) isCheckpointEdge(step DocStep, result string) bool {
	if result != JoinPass || step.Kind != StepKindTasks {
		return false
	}
	join := step.EffectiveJoin()
	if join.Mode != JoinModeAll {
		return false
	}
	for _, t := range step.Tasks {
		if t.Kind != KindGate || t.When != nil || !inOver(join.Over, t.Kind, t.Key) {
			continue
		}
		switch g.RoleKind(t.Role) {
		case RoleKindMember, RoleKindManagers, RoleKindRequester:
			return true
		}
	}
	return false
}

// ApplyMeta carries what a state change records and the engine knows: when, and
// who for a checkpoint.
type ApplyMeta struct {
	Now string
	By  string
}

// InitialState is the state of a new request: at the start step, its first
// entry, the variables at their defaults except the ones given.
func InitialState(g Graph, vars map[string]any, now string) State {
	st := State{
		Version:     g.Doc.Version,
		Step:        g.Doc.Start,
		Activations: map[string]int{g.Doc.Start: 1},
		Loops:       map[string]int{},
		Vars:        map[string]any{},
		EnteredAt:   now,
	}
	for _, v := range g.Doc.Vars {
		st.Vars[v.Key] = v.Default
	}
	for k, v := range vars {
		st.Vars[k] = v
	}
	return st
}

func copyCounts(m map[string]int) map[string]int {
	out := make(map[string]int, len(m)+1)
	for k, v := range m {
		out[k] = v
	}
	return out
}

// ApplyDecision returns the state after a route decision: the trail grows, the
// counters move (spec 4.5), a stop parks the request, a terminal closes it. Any
// other decision leaves the state as it is. The argument is not changed.
func ApplyDecision(g Graph, st State, d GraphDecision, meta ApplyMeta) State {
	if d.Kind != DecisionRoute {
		return st
	}
	entry := st.Activations[st.Step]
	out := st
	out.Seq++
	out.Activations = copyCounts(st.Activations)
	out.Loops = copyCounts(st.Loops)
	out = AppendTrail(out, TrailEntry{Step: st.Step, Activation: entry, Edge: d.TransitionID, At: meta.Now}, g.Doc.Policy.Effective().MaxActivations)
	if d.Checkpoint {
		out.Checkpoints = append(append([]Checkpoint(nil), st.Checkpoints...), Checkpoint{Step: st.Step, Activation: entry, By: meta.By, At: meta.Now})
		out.Unreviewed = false
	}
	switch {
	case d.StopCode != "":
		out.Stop = &StopRecord{Code: d.StopCode, From: st.Step, Edge: d.TransitionID, At: meta.Now}
		out.BlockedFrom = st.Step
	case isTerminal(g, d.To):
		out.CameFrom = st.Step
		out.Step = d.To
		out.ClosedAt = meta.Now
	default:
		out = enterStep(g, out, d.To, d.Loop, meta)
		out.CameFrom = st.Step
	}
	return out
}

func isTerminal(g Graph, key string) bool {
	_, ok := g.Terminal(key)
	return ok
}

// enterStep moves the request into a step. A back-edge counts in its loop; a
// forward entry resets the loops that start at the step (spec 4.5). The state's
// maps must already be the caller's own.
func enterStep(g Graph, st State, to, backLoop string, meta ApplyMeta) State {
	st.Step = to
	st.EnteredAt = meta.Now
	st.Activations[to]++
	if backLoop != "" {
		st.Loops[backLoop]++
		return st
	}
	if g.Compat == CompatV1 {
		// The rework count of a schema 1 request is never reset by entering a step.
		return st
	}
	for _, l := range g.Doc.Loops {
		if l.Header == to && l.EffectiveReset() == LoopResetOnEntry {
			st.Loops[l.Key] = 0
		}
	}
	return st
}

// ApplyResume returns the state after a manager resumes a parked request into a
// step. It counts as a forward entry (spec 4.5, 4.7): the step's activation
// grows and the loops that start there are reset.
func ApplyResume(g Graph, st State, to string, meta ApplyMeta) State {
	out := st
	out.Seq++
	out.Activations = copyCounts(st.Activations)
	out.Loops = copyCounts(st.Loops)
	out.Stop = nil
	out.BlockedFrom = ""
	out.CameFrom = st.Step
	return enterStep(g, out, to, "", meta)
}

// ApplyOpened returns the state after the engine opened task rows: the request
// is unreviewed from the moment a task of an agent produces work (spec 4.8). A
// gate does not make work.
func ApplyOpened(g Graph, st State, opened []OpenTask) State {
	for _, o := range opened {
		if o.Task.Kind == KindWork && g.RoleKind(o.Task.Role) == RoleKindAgent {
			st.Unreviewed = true
			break
		}
	}
	return st
}
