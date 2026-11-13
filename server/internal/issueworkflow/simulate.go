package issueworkflow

import (
	"sort"
	"strconv"
	"strings"
)

// Simulation and static analysis of a graph (spec 4.10). Simulate runs a request
// through the graph with DecideGraph and BuildGraphFacts on made-up task rows;
// there is no database, no clock and no agent. Analyze reads the document alone.

// MaxSimulateHops is the most steps a simulation follows.
const MaxSimulateHops = 120

// The reserved keys of a script entry. Any other key names a task row, or a task
// (which then applies to all its instances).
const (
	// ScriptLead is the verdict the lead reports on a lead step.
	ScriptLead = "@lead"
	// ScriptExit is the edge id a person takes on a manual step.
	ScriptExit = "@exit"
	// ScriptVarPrefix starts a variable a task of the entry sets, for example
	// "@var.topics": a,b,c (a list is comma separated).
	ScriptVarPrefix = "@var."
)

// The verdicts a script can give a task besides the keys of its verdict set.
const (
	// ScriptOpen leaves the row open.
	ScriptOpen = "open"
	// ScriptCancelled is a task a person cancelled.
	ScriptCancelled = "cancelled"
)

// Outcomes of a simulation.
const (
	TraceTerminal  = "terminal"
	TraceStop      = "stop"
	TraceStalled   = "stalled"
	TraceWaiting   = "waiting"
	TraceTruncated = "truncated"
)

// TraceStep is one entry into a step and how it ended.
type TraceStep struct {
	Step string `json:"step"`
	// Activation is the number of this entry into the step, from 1.
	Activation int      `json:"activation"`
	Opened     []string `json:"opened,omitempty"`
	// Classes is the class of each closed row of the entry when it ended.
	Classes    map[string]string `json:"classes,omitempty"`
	JoinResult string            `json:"join_result,omitempty"`
	Edge       string            `json:"edge,omitempty"`
	// To is the step or terminal the request moved to, or the stop code.
	To        string         `json:"to,omitempty"`
	Cancelled []string       `json:"cancelled,omitempty"`
	Loops     map[string]int `json:"loops,omitempty"`
}

// Trace is the result of a simulation.
type Trace struct {
	Steps []TraceStep `json:"steps"`
	// Outcome is terminal, stop, stalled, waiting or truncated; Code is the
	// terminal key, the stop code, or the reason it stalled.
	Outcome   string `json:"outcome"`
	Code      string `json:"code,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
	Final     State  `json:"-"`
}

// Visits is how many times the trace entered a step.
func (t Trace) Visits(step string) int {
	n := 0
	for _, s := range t.Steps {
		if s.Step == step {
			n++
		}
	}
	return n
}

// Path lists the steps the trace entered, in order.
func (t Trace) Path() []string {
	out := make([]string, len(t.Steps))
	for i, s := range t.Steps {
		out[i] = s.Step
	}
	return out
}

// Simulate runs a request through the graph. vars overrides the defaults of the
// variables. script gives, for each step, one entry per entry into the step:
// the verdict of each task row ("docs", or "docs~2" for one instance), plus the
// reserved keys above. A task not named gets the default: done for work, the
// first passing verdict for a gate. maxHops is capped at MaxSimulateHops.
func Simulate(g Graph, vars map[string]any, script map[string][]map[string]string, maxHops int) Trace {
	if maxHops <= 0 || maxHops > MaxSimulateHops {
		maxHops = MaxSimulateHops
	}
	meta := ApplyMeta{Now: "simulated", By: "simulation"}
	st := InitialState(g, vars, meta.Now)
	var rows []TaskState
	var trace Trace
	cur := TraceStep{Step: st.Step, Activation: 1}
	varsApplied := map[string]bool{}
	hops := 0

	finish := func(outcome, code string) Trace {
		trace.Outcome, trace.Code, trace.Final = outcome, code, st
		return trace
	}

	for guard := 0; guard < 100000; guard++ {
		entry := entryScript(script, st)
		fs := BuildGraphFacts(g, st, rows, RequestFacts{})
		step, _ := g.Step(st.Step)
		d := DecideGraph(g, st, rows, fs)

		if d.Kind == DecisionOpen {
			round := entryRound(g, st)
			for _, o := range d.Open {
				ts := TaskState{Step: st.Step, Key: o.Key, Kind: o.Task.Kind, Round: round}
				closeRow(g.Doc, &ts, o.Task, scriptVerdict(entry, o))
				rows = append(rows, ts)
				cur.Opened = append(cur.Opened, o.Key)
			}
			st = ApplyOpened(g, st, d.Open)
			continue
		}

		// The first time the entry has nothing to open, the tasks have set their
		// variables; decide again with them.
		if key := st.Step + "#" + strconv.Itoa(st.Activations[st.Step]); !varsApplied[key] {
			varsApplied[key] = true
			if next, changed := applyScriptVars(g, st, entry); changed {
				st = next
				continue
			}
		}

		if d.Kind == DecisionWait {
			switch step.Kind {
			case StepKindLead:
				verdict := entry[ScriptLead]
				if verdict == "" {
					verdict = firstPass(g.Doc, KindWork, leadSet(step))
				}
				d = RouteLead(g, st, verdict, fs)
			case StepKindManual:
				exit := entry[ScriptExit]
				if exit == "" {
					for _, e := range step.Edges {
						if e.Trigger == EdgeTriggerManual {
							exit = e.ID
							break
						}
					}
				}
				d = RouteManual(g, st, rows, exit, fs)
			}
			if d.Kind != DecisionRoute {
				return finish(TraceWaiting, d.Reason)
			}
		}

		// A route.
		cur.Edge, cur.JoinResult, cur.Cancelled = d.TransitionID, d.JoinResult, d.Cancel
		cur.To = d.To
		if d.StopCode != "" {
			cur.To = d.StopCode
		}
		cur.Classes = entryClasses(g.Doc, step, st.Step, entryRound(g, st), rows)
		cur.Loops = copyCounts(st.Loops)
		for i := range rows {
			if inStrings(d.Cancel, RowRef(rows[i].Step, rows[i].Key)) {
				rows[i].Closed, rows[i].Cancelled, rows[i].Outcome = true, true, OutcomeSuperseded
			}
		}
		trace.Steps = append(trace.Steps, cur)
		st = ApplyDecision(g, st, d, meta)
		hops++
		switch {
		case d.StopCode == DocStopStalled:
			code := d.Reason
			if code == "" {
				code = DocStopStalled
			}
			return finish(TraceStalled, code)
		case d.StopCode != "":
			return finish(TraceStop, d.StopCode)
		case st.ClosedAt != "" || isTerminal(g, st.Step):
			return finish(TraceTerminal, st.Step)
		}
		if hops >= maxHops {
			trace.Truncated = true
			return finish(TraceTruncated, "")
		}
		cur = TraceStep{Step: st.Step, Activation: st.Activations[st.Step]}
	}
	trace.Truncated = true
	return finish(TraceTruncated, "")
}

func entryScript(script map[string][]map[string]string, st State) map[string]string {
	list := script[st.Step]
	if i := st.Activations[st.Step] - 1; i >= 0 && i < len(list) {
		return list[i]
	}
	return nil
}

func leadSet(step DocStep) string {
	if step.Lead != nil && step.Lead.Verdicts != "" {
		return step.Lead.Verdicts
	}
	return VerdictSetPlan
}

// firstPass is the first verdict of the set that passes.
func firstPass(doc FlowDoc, kind, set string) string {
	for _, v := range verdictsOf(doc, kind, set) {
		if v.Class == ClassPass {
			return v.Key
		}
	}
	return OutcomeDone
}

// scriptVerdict is the verdict the script gives a row: the row key, then the
// task key (which covers all instances), then the default.
func scriptVerdict(entry map[string]string, o OpenTask) string {
	if v, ok := entry[o.Key]; ok {
		return v
	}
	return entry[o.Task.Key]
}

// closeRow gives a new row the verdict of the script, or the default.
func closeRow(doc FlowDoc, ts *TaskState, task DocTask, verdict string) {
	if verdict == "" {
		verdict = firstPass(doc, task.Kind, task.Verdicts)
	}
	switch verdict {
	case ScriptOpen:
	case ScriptCancelled:
		ts.Closed, ts.Cancelled = true, true
	default:
		ts.Closed, ts.Outcome = true, verdict
	}
}

func entryClasses(doc FlowDoc, step DocStep, stepKey string, round int, rows []TaskState) map[string]string {
	out := map[string]string{}
	for _, ts := range rows {
		if ts.Step != stepKey || ts.Round != round {
			continue
		}
		if class, closed, counted := docTaskClass(doc, step, ts); counted && closed {
			out[ts.Key] = class
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// applyScriptVars sets the variables an entry's script names, converting the
// text to the variable's type. Unknown variables and bad values are skipped.
func applyScriptVars(g Graph, st State, entry map[string]string) (State, bool) {
	changed := false
	vars := map[string]any{}
	for k, v := range st.Vars {
		vars[k] = v
	}
	keys := make([]string, 0, len(entry))
	for k := range entry {
		if strings.HasPrefix(k, ScriptVarPrefix) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		name := strings.TrimPrefix(k, ScriptVarPrefix)
		for _, v := range g.Doc.Vars {
			if v.Key != name {
				continue
			}
			if value, ok := scriptValue(v, entry[k]); ok {
				vars[name] = value
				changed = true
			}
		}
	}
	st.Vars = vars
	return st, changed
}

func scriptValue(v Var, text string) (any, bool) {
	switch v.Type {
	case VarTypeBool:
		b, err := strconv.ParseBool(text)
		return b, err == nil
	case VarTypeInt:
		n, err := strconv.Atoi(text)
		return n, err == nil
	case VarTypeListString:
		list := []string{}
		for _, item := range strings.Split(text, ",") {
			if item = strings.TrimSpace(item); item != "" {
				list = append(list, item)
			}
		}
		return list, true
	}
	return text, true
}

// Analysis is what the document alone says about its cost and its exits.
type Analysis struct {
	// WorstCaseTaskIssues bounds the task issues one request can open.
	WorstCaseTaskIssues int `json:"worst_case_task_issues"`
	// WorstCaseActivations bounds the entries into steps of one request.
	WorstCaseActivations int         `json:"worst_case_activations"`
	Steps                []StepBound `json:"steps"`
	Loops                []LoopInfo  `json:"loops"`
	Stops                []StopInfo  `json:"stops"`
	// ReleasePathsWithoutHuman are the ways work of an agent can leave the
	// workspace without a person looking at it (spec 4.8, Y01).
	ReleasePathsWithoutHuman []ReleasePath `json:"release_paths_without_human"`
}

// StepBound is the most a step can be entered and the most tasks one entry opens.
type StepBound struct {
	Step       string `json:"step"`
	MaxEntries int    `json:"max_entries"`
	MaxTasks   int    `json:"max_tasks"`
}

// LoopInfo is a loop of the document.
type LoopInfo struct {
	Key    string   `json:"key"`
	Header string   `json:"header"`
	Max    int      `json:"max"`
	Body   []string `json:"body"`
}

// StopInfo is a stop and the edges that lead to it.
type StopInfo struct {
	Code   string   `json:"code"`
	Class  string   `json:"class,omitempty"`
	Status string   `json:"status"`
	Edges  []string `json:"edges"`
}

// ReleasePath is a way to a terminal that leaves the workspace.
type ReleasePath struct {
	// From is where the path starts: the start step, or a step with agent work.
	From     string   `json:"from"`
	Terminal string   `json:"terminal"`
	Path     []string `json:"path"`
}

// Analyze computes the bounds of spec 4.5 and the static facts of spec 4.10.
// The bounds are upper bounds. The entries into a step are at most the product,
// over every loop that multiplies the step, of the loop's bound plus one (see
// loopMultipliers). A loop with no header (the one a lifted flow with differing
// rework targets has) bounds every step. The tasks of an entry are at most the
// largest number of instances of each task, ignoring when.
func Analyze(doc FlowDoc) Analysis {
	g := NewGraph(doc)
	var a Analysis

	bodies := map[string]map[string]bool{}
	for _, l := range doc.Loops {
		body := map[string]bool{}
		if l.Header == "" {
			for _, s := range doc.Steps {
				body[s.Key] = true
			}
		} else {
			for _, key := range g.LoopBody(l.Key) {
				body[key] = true
			}
		}
		bodies[l.Key] = body
		a.Loops = append(a.Loops, LoopInfo{Key: l.Key, Header: l.Header, Max: l.Max.Limit(), Body: g.LoopBody(l.Key)})
	}
	multipliers := loopMultipliers(doc, bodies)
	for _, s := range doc.Steps {
		entries := 1
		for _, l := range doc.Loops {
			if multipliers[s.Key][l.Key] {
				entries *= l.Max.Limit() + 1
			}
		}
		tasks := 0
		for _, t := range s.Tasks {
			tasks += maxInstances(t)
		}
		a.Steps = append(a.Steps, StepBound{Step: s.Key, MaxEntries: entries, MaxTasks: tasks})
		a.WorstCaseActivations += entries
		a.WorstCaseTaskIssues += entries * tasks
	}

	for _, s := range doc.Stops {
		a.Stops = append(a.Stops, StopInfo{Code: s.Code, Class: s.Class, Status: stopStatus(s), Edges: stopEdges(doc, s.Code)})
	}
	if edges := stopEdges(doc, DocStopStalled); len(edges) > 0 {
		a.Stops = append(a.Stops, StopInfo{Code: DocStopStalled, Status: DefaultStopStatus, Edges: edges})
	}
	a.ReleasePathsWithoutHuman = releasePaths(g)
	return a
}

// loopMultipliers says, for each step, which loops multiply its entries.
//
// Spec 4.5 takes the loops whose body holds the step. That is not enough when two
// loops overlap without nesting: a loop whose counter is reset by a forward entry
// into its header gets a fresh bound for each such entry, and a second loop can
// make those entries repeat although the step lies outside the second loop's
// body. Example: loop l1 runs s1 to s2 and back, loop l2 runs s2 to s4 and back.
// Every return of l1 enters s2 forward and resets l2, so s3 (in the body of l2
// only) can be entered (l1 + 1) times (l2 + 1). So a loop that multiplies a step
// also hands on the loops that multiply its header, when it resets on entry.
// The result is still an upper bound, and equals the formula of the spec when the
// loops are separate or nested.
func loopMultipliers(doc FlowDoc, bodies map[string]map[string]bool) map[string]map[string]bool {
	mult := map[string]map[string]bool{}
	for _, s := range doc.Steps {
		mult[s.Key] = map[string]bool{}
		for _, l := range doc.Loops {
			if bodies[l.Key][s.Key] {
				mult[s.Key][l.Key] = true
			}
		}
	}
	for changed := true; changed; {
		changed = false
		for _, s := range doc.Steps {
			for _, l := range doc.Loops {
				if !mult[s.Key][l.Key] || l.Header == "" || l.EffectiveReset() != LoopResetOnEntry {
					continue
				}
				for inherited := range mult[l.Header] {
					if !mult[s.Key][inherited] {
						mult[s.Key][inherited] = true
						changed = true
					}
				}
			}
		}
	}
	return mult
}

func stopStatus(s StopKind) string {
	if s.Status != "" {
		return s.Status
	}
	return DefaultStopStatus
}

func stopEdges(doc FlowDoc, code string) []string {
	var out []string
	for _, s := range doc.Steps {
		for _, e := range s.Edges {
			if e.Stop == code {
				out = append(out, s.Key+"."+e.ID)
			}
		}
	}
	return out
}

// maxInstances is the most rows a task opens in one entry.
func maxInstances(t DocTask) int {
	switch {
	case t.Instances == nil:
		return 1
	case t.Instances.FromVar != "":
		if t.Instances.Max < 1 || t.Instances.Max > DocMaxInstances {
			return DocMaxInstances
		}
		return t.Instances.Max
	case t.Instances.Count > DocMaxInstances:
		return DocMaxInstances
	case t.Instances.Count < 1:
		return 1
	}
	return t.Instances.Count
}

// isCheckpoint reports whether the edge is a human checkpoint as the validator
// defines it (spec Y01): an edge of a manual step, or an edge of a tasks step that
// requires join.result = pass when the step judges a gate given to a person.
func isStaticCheckpoint(g Graph, step DocStep, e DocEdge) bool {
	if step.Kind == StepKindManual {
		return e.Trigger == EdgeTriggerManual
	}
	if step.Kind != StepKindTasks || e.When == nil || !requiresPass(*e.When) {
		return false
	}
	return g.isCheckpointEdge(step, JoinPass)
}

// requiresPass reports whether the condition holds only with join.result = pass
// (a leaf, or an all group that has one).
func requiresPass(c Condition) bool {
	switch {
	case c.All != nil:
		for _, child := range c.All {
			if requiresPass(child) {
				return true
			}
		}
		return false
	case c.Any != nil, c.Not != nil:
		return false
	}
	return c.Field == "join.result" && c.Op == OpEq && c.Value == JoinPass
}

// releasePaths finds, with the checkpoint edges taken out, the terminals that
// leave the workspace and are still reachable from the start step or from a step
// with work for an agent.
func releasePaths(g Graph) []ReleasePath {
	origins := []string{g.Doc.Start}
	for _, s := range g.Doc.Steps {
		if s.Key == g.Doc.Start {
			continue
		}
		for _, t := range s.Tasks {
			if t.Kind == KindWork && g.RoleKind(t.Role) == RoleKindAgent {
				origins = append(origins, s.Key)
				break
			}
		}
	}
	var out []ReleasePath
	for _, from := range origins {
		prev := map[string]string{from: ""}
		queue := []string{from}
		var reached []string
		for len(queue) > 0 {
			key := queue[0]
			queue = queue[1:]
			step, ok := g.Step(key)
			if !ok {
				continue
			}
			for _, e := range step.Edges {
				if e.To == "" || isStaticCheckpoint(g, step, e) {
					continue
				}
				if _, seen := prev[e.To]; seen {
					continue
				}
				prev[e.To] = key
				if t, ok := g.Terminal(e.To); ok {
					if t.LeavesWorkspace {
						reached = append(reached, e.To)
					}
					continue
				}
				queue = append(queue, e.To)
			}
		}
		for _, terminal := range reached {
			var path []string
			for at := terminal; at != ""; at = prev[at] {
				path = append([]string{at}, path...)
			}
			out = append(out, ReleasePath{From: from, Terminal: terminal, Path: path})
		}
	}
	return out
}
