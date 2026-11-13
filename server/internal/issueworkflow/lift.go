package issueworkflow

import (
	"fmt"
	"strings"

	"github.com/multica-ai/multica/server/internal/issuestatus"
)

// Lift reads a schema 1 workflow as a schema 2 graph (spec section 9.2). It is
// a pure function: the Definition is not changed and the same input gives the
// same graph. The rules of the engine do not change with it; the lifted graph
// says what the schema 1 engine does, in the terms of the schema 2 engine.
//
// A human-only step (spec 9.2 item 3) becomes a manual step with exit_role
// managers and the notice stop2 (status in_review) or human. Schema 1 has no exit
// for it: a person drags the card. A schema 2 manual step needs at least one
// manual edge or it is a dead end, so Lift gives each one two buttons, in this
// order, and only when the target exists in the lifted graph:
//
//   - <step>_next, "Next step": to the step that follows it in the schema 1 list,
//     unless that is blocked, done or cancelled;
//   - <step>_done, "Done": to the done terminal, which leaves the workspace.
//
// Both are manual edges with no condition, stop or loop. A step that is last
// before the terminals gets only the done button. The buttons are never taken by
// DecideGraph: a manual step waits, as the schema 1 step does, until a person
// takes an exit, so the decisions of the lifted graph do not change.

// Names the lift gives to what schema 1 has no name for.
const (
	// StopGroundsMissing is stop 1 of schema 1: a task or the lead reported that
	// the grounds are missing.
	StopGroundsMissing = "grounds_missing"
	// StopLoopExhausted is stop 3 of schema 1: the rework rounds ran out.
	StopLoopExhausted = "loop_exhausted"
	// LoopRework is the one loop of a lifted flow: schema 1 has a single rework
	// counter.
	LoopRework = "rework"
	// SlotOutput is the file slot of a task that required an attachment.
	SlotOutput = "output"

	// The message keys schema 1 flow_config uses.
	MessageStop1   = "stop1"
	MessageStop2   = "stop2"
	MessageStop3   = "stop3"
	MessageStalled = "stalled"
	MessageHuman   = "human"
)

// LiftOpts says what the lifted graph is for.
type LiftOpts struct {
	// Runtime: the graph runs requests of the schema 1 workflow. It is marked
	// compat v1 and exempt from the rules that schema 1 never had (spec 9.2
	// item 12). Without it the result is a clean schema 2 document, as when a
	// schema 1 flow is forked into a draft.
	Runtime bool
	// RoleKeys maps an agent id to the role key a manifest gave its task. An
	// agent not in the map gets the key a_<first 8 characters of its id>.
	RoleKeys map[string]string
}

// Lift converts a schema 1 flow definition into a graph.
func Lift(def Definition, opts LiftOpts) (Graph, error) {
	if def.Flow == nil {
		return Graph{}, fmt.Errorf("lift: the workflow has no flow_config")
	}
	cfg := def.Flow
	if _, ok := def.Step(StartStatus); !ok {
		return Graph{}, fmt.Errorf("lift: the workflow has no %q step", StartStatus)
	}

	roles, roleOf, err := liftRoles(def, opts)
	if err != nil {
		return Graph{}, err
	}
	vars := liftVars(cfg)
	varKeys := make([]string, len(vars))
	for i, v := range vars {
		varKeys[i] = v.Key
	}

	doc := FlowDoc{
		Version: FlowDocVersion,
		Start:   StartStatus,
		Roles:   roles,
		Vars:    vars,
		Stops: []StopKind{
			{Code: StopGroundsMissing, Class: "missing_grounds", Label: "Grounds missing", Message: MessageStop1, ResumeBy: "managers", LegacyNo: 1},
			{Code: StopLoopExhausted, Class: "loop_exhausted", Label: "Rework rounds used", Message: MessageStop3, ResumeBy: "managers", LegacyNo: 3},
		},
		Messages: liftMessages(cfg.Messages),
		Policy: Policy{
			TaskIdleMinutes:        cfg.TaskTimeout(),
			TaskHardCapMinutes:     cfg.TaskHardCap(),
			TaskMaxAttempts:        cfg.TaskAttempts(),
			AllowUnattendedRelease: opts.Runtime,
		},
	}

	nextOf := map[string]string{}
	for i, s := range def.Steps {
		if i+1 < len(def.Steps) {
			nextOf[s.StatusKey] = def.Steps[i+1].StatusKey
		}
		switch s.StatusKey {
		case issuestatus.Blocked:
			continue
		case issuestatus.Done:
			doc.Terminals = append(doc.Terminals, Terminal{Key: issuestatus.Done, Status: issuestatus.Done, LeavesWorkspace: true})
			continue
		case issuestatus.Cancelled:
			doc.Terminals = append(doc.Terminals, Terminal{Key: issuestatus.Cancelled, Status: issuestatus.Cancelled})
			continue
		}
		step, err := liftStep(s, cfg, roleOf, varKeys)
		if err != nil {
			return Graph{}, err
		}
		doc.Steps = append(doc.Steps, step)
	}

	addManualExits(&doc, nextOf)

	loop, err := liftLoop(def)
	if err != nil {
		return Graph{}, err
	}
	if loop != nil {
		doc.Loops = []Loop{*loop}
	}

	g := NewGraph(doc)
	if opts.Runtime {
		g.Compat = CompatV1
	}
	return g, nil
}

// Edge ids and labels of the buttons of a lifted manual step.
const (
	manualNextSuffix = "_next"
	manualDoneSuffix = "_done"
)

// addManualExits gives each manual step its buttons (see Lift). nextOf maps a
// schema 1 step to the one that follows it in the list.
func addManualExits(doc *FlowDoc, nextOf map[string]string) {
	steps := map[string]bool{}
	for _, s := range doc.Steps {
		steps[s.Key] = true
	}
	done := false
	for _, t := range doc.Terminals {
		done = done || t.Key == issuestatus.Done
	}
	for i := range doc.Steps {
		s := &doc.Steps[i]
		if s.Kind != StepKindManual {
			continue
		}
		used := map[string]bool{}
		for _, e := range s.Edges {
			used[e.ID] = true
		}
		if next := nextOf[s.Key]; next != "" && next != s.Key && steps[next] {
			s.Edges = append(s.Edges, DocEdge{ID: uniqueID(s.Key+manualNextSuffix, used), Label: "Next step", To: next, Trigger: EdgeTriggerManual})
		}
		if done {
			s.Edges = append(s.Edges, DocEdge{ID: uniqueID(s.Key+manualDoneSuffix, used), Label: "Done", To: issuestatus.Done, Trigger: EdgeTriggerManual})
		}
	}
}

// liftRoles gives each distinct agent a role, in the order the agents first
// appear. It returns the roles and the role key of each agent id.
func liftRoles(def Definition, opts LiftOpts) ([]Role, map[string]string, error) {
	var roles []Role
	roleOf := map[string]string{}
	taken := map[string]bool{}
	for _, s := range def.Steps {
		for _, t := range s.Tasks {
			if t.Handler.Type != HandlerAgent || t.Handler.ID == "" {
				return nil, nil, fmt.Errorf("lift: task %q of %q is not handed to an agent", t.Key, s.StatusKey)
			}
			id := t.Handler.ID
			if _, seen := roleOf[id]; seen {
				continue
			}
			key := opts.RoleKeys[id]
			if key == "" {
				key = "a_" + strings.ToLower(strings.SplitN(id, "-", 2)[0])
				if len(key) > 10 {
					key = key[:10]
				}
			}
			unique := key
			for n := 2; taken[unique]; n++ {
				unique = fmt.Sprintf("%s_%d", key, n)
			}
			taken[unique] = true
			roleOf[id] = unique
			roles = append(roles, Role{Key: unique, Kind: RoleKindAgent, Pool: 1, Bind: &RoleBind{AgentIDs: []string{id}}})
		}
	}
	return roles, roleOf, nil
}

// liftVars turns the switches into variables the lead sets at the start step.
func liftVars(cfg *FlowConfig) []Var {
	defaults := cfg.DefaultSwitches()
	vars := make([]Var, 0, len(cfg.Switches))
	for _, sw := range cfg.Switches {
		v := Var{Key: sw.Key, Type: VarTypeBool, Default: defaults[sw.Key], SetBy: []string{RoleKindLead}, SetAt: []string{StartStatus}}
		if sw.Type == SwitchEnum {
			v.Type = VarTypeEnum
			v.Values = append([]string(nil), sw.Values...)
		}
		vars = append(vars, v)
	}
	return vars
}

func liftMessages(m FlowMessages) map[string]string {
	out := map[string]string{}
	for key, text := range map[string]string{
		MessageStop1: m.Stop1, MessageStop2: m.Stop2, MessageStop3: m.Stop3, MessageStalled: m.Stalled, MessageHuman: m.Human,
	} {
		if strings.TrimSpace(text) != "" {
			out[key] = text
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func liftStep(s Step, cfg *FlowConfig, roleOf map[string]string, varKeys []string) (DocStep, error) {
	step := DocStep{Key: s.StatusKey, Status: StepStatus{Key: s.StatusKey}, Kind: StepKindTasks}
	if s.Layout != nil {
		step.Layout = &StepLayout{X: s.Layout.X, Y: s.Layout.Y}
	}
	switch {
	case s.StatusKey == StartStatus:
		step.Kind = StepKindLead
		step.Lead = &StepLead{Sets: append([]string{}, varKeys...), Verdicts: VerdictSetPlan}
	case s.HumanOnly:
		step.Kind = StepKindManual
		step.ExitRole = RoleKindManagers
		step.Notice = MessageHuman
		if s.StatusKey == issuestatus.InReview {
			step.Notice = MessageStop2
		}
	}
	for _, t := range s.Tasks {
		task, err := liftTask(t, s.StatusKey, cfg, roleOf)
		if err != nil {
			return DocStep{}, err
		}
		step.Tasks = append(step.Tasks, task)
	}
	// The ids schema 1 gave stay as they are; an edge made by a split takes a free id.
	used := map[string]bool{}
	for _, t := range s.Transitions {
		used[t.ID] = true
	}
	for _, t := range s.Transitions {
		edges, err := liftEdge(t, s.StatusKey, cfg, used)
		if err != nil {
			return DocStep{}, err
		}
		step.Edges = append(step.Edges, edges...)
	}
	return step, nil
}

func liftTask(t Task, stepKey string, cfg *FlowConfig, roleOf map[string]string) (DocTask, error) {
	task := DocTask{Key: t.Key, Title: t.Title, Kind: t.Kind, Role: roleOf[t.Handler.ID], Instructions: t.Instructions}
	if t.When != nil {
		when, err := rewriteCondition(*t.When, stepKey, cfg)
		if err != nil {
			return DocTask{}, fmt.Errorf("lift: task %q of %q: %w", t.Key, stepKey, err)
		}
		task.When = &when
	}
	var req Requires
	if t.Needs(RequireOutcome) {
		yes := true
		req.Verdict = &yes
		task.Verdicts = VerdictSetGate
	}
	if t.Needs(RequireAttachment) {
		req.Files = []FileReq{{Slot: SlotOutput, Min: 1}}
	}
	if req.Verdict != nil || len(req.Files) > 0 {
		task.Requires = &req
	}
	return task, nil
}

// liftEdge converts a transition. An edge into blocked becomes a stop; away from
// the start step it may become two (spec 9.2 item 6), because schema 1 chooses
// stop 1 or stop 3 by what is true when the request leaves, and schema 2 states
// the choice in the edges. used holds the ids taken in the step, which a split edge avoids.
func liftEdge(t Transition, stepKey string, cfg *FlowConfig, used map[string]bool) ([]DocEdge, error) {
	edge := DocEdge{ID: t.ID, Label: t.Label}
	var when *Condition
	if t.When != nil {
		c, err := rewriteCondition(*t.When, stepKey, cfg)
		if err != nil {
			return nil, fmt.Errorf("lift: transition %q of %q: %w", t.ID, stepKey, err)
		}
		when = &c
	}
	edge.When = when
	if t.Rework {
		edge.To = t.ToStatusKey
		edge.Loop = LoopRework
		return []DocEdge{edge}, nil
	}
	if t.ToStatusKey != issuestatus.Blocked {
		edge.To = t.ToStatusKey
		return []DocEdge{edge}, nil
	}
	if stepKey == StartStatus || impliesBlock(t.When) {
		edge.Stop = StopGroundsMissing
		return []DocEdge{edge}, nil
	}
	blocked := Condition{Field: "join.result", Op: OpEq, Value: "block"}
	split := DocEdge{ID: uniqueID(t.ID+"_block", used), Label: t.Label, Stop: StopGroundsMissing}
	switch {
	case when == nil:
		split.When = &blocked
	case when.All != nil:
		split.When = &Condition{All: append(append([]Condition{}, when.All...), blocked)}
	default:
		split.When = &Condition{All: []Condition{*when, blocked}}
	}
	edge.Stop = StopLoopExhausted
	return []DocEdge{split, edge}, nil
}

func uniqueID(id string, used map[string]bool) string {
	unique := id
	for n := 2; used[unique]; n++ {
		unique = fmt.Sprintf("%s_%d", id, n)
	}
	used[unique] = true
	return unique
}

// impliesBlock reports whether the schema 1 condition holds only when some task
// of the step reported blocked.
func impliesBlock(c *Condition) bool {
	if c == nil {
		return false
	}
	switch {
	case c.All != nil:
		for _, child := range c.All {
			if impliesBlock(&child) {
				return true
			}
		}
		return false
	case c.Any != nil:
		for _, child := range c.Any {
			if !impliesBlock(&child) {
				return false
			}
		}
		return len(c.Any) > 0
	case c.Not != nil:
		return false
	}
	if c.Field != FieldTasksOutcome {
		return false
	}
	switch c.Op {
	case OpEq:
		return c.Value == TasksAnyBlocked
	case OpIn:
		items := asList(c.Value)
		for _, item := range items {
			if item != TasksAnyBlocked {
				return false
			}
		}
		return len(items) > 0
	}
	return false
}

// joinResultOf maps a tasks_outcome value to a join.result value.
var joinResultOf = map[string]string{
	TasksAllApproved: "pass",
	TasksAnyRejected: "fail",
	TasksAnyBlocked:  "block",
	TasksNone:        "none",
}

// rewriteCondition writes a schema 1 condition in the facts of schema 2. It
// builds a new tree and leaves c as it was. A field that schema 2 has no fact
// with the same meaning for is an error, not a guess.
func rewriteCondition(c Condition, stepKey string, cfg *FlowConfig) (Condition, error) {
	switch {
	case c.All != nil:
		children, err := rewriteChildren(c.All, stepKey, cfg)
		return Condition{All: children}, err
	case c.Any != nil:
		children, err := rewriteChildren(c.Any, stepKey, cfg)
		return Condition{Any: children}, err
	case c.Not != nil:
		inner, err := rewriteCondition(*c.Not, stepKey, cfg)
		return Condition{Not: &inner}, err
	}

	if key, ok := strings.CutPrefix(c.Field, FieldSwitchPrefix); ok {
		sw, declared := cfg.Switch(key)
		if !declared {
			return Condition{}, fmt.Errorf("switch %q is not declared", key)
		}
		field := "var." + key
		if c.Op == OpIn && sw.Type == SwitchBool {
			return inToAny(field, c.Value)
		}
		return Condition{Field: field, Op: c.Op, Value: c.Value}, nil
	}
	switch c.Field {
	case FieldTasksOutcome:
		switch c.Op {
		case OpEq:
			s, _ := c.Value.(string)
			mapped, ok := joinResultOf[s]
			if !ok {
				return Condition{}, fmt.Errorf("tasks_outcome value %v is not known", c.Value)
			}
			return Condition{Field: "join.result", Op: OpEq, Value: mapped}, nil
		case OpIn:
			var items []any
			for _, item := range asList(c.Value) {
				s, _ := item.(string)
				mapped, ok := joinResultOf[s]
				if !ok {
					return Condition{}, fmt.Errorf("tasks_outcome value %v is not known", item)
				}
				items = append(items, mapped)
			}
			return Condition{Field: "join.result", Op: OpIn, Value: items}, nil
		}
		return Condition{}, fmt.Errorf("operator %q does not apply to tasks_outcome", c.Op)
	case FieldReworkCount:
		field := "loop." + LoopRework + ".count"
		if c.Op == OpIn {
			return inToAny(field, c.Value)
		}
		return Condition{Field: field, Op: c.Op, Value: c.Value}, nil
	case FieldOutcome:
		// Only the plan step is routed with an outcome; at any other step the
		// field is never set, which has no schema 2 counterpart.
		if stepKey != StartStatus {
			return Condition{}, fmt.Errorf("the outcome field only has a value at the %q step", StartStatus)
		}
		if c.Op == OpExists {
			return Condition{}, fmt.Errorf("outcome exists has no schema 2 fact")
		}
		return Condition{Field: "lead.verdict", Op: c.Op, Value: c.Value}, nil
	}
	return Condition{}, fmt.Errorf("the field %q has no schema 2 fact", c.Field)
}

func rewriteChildren(children []Condition, stepKey string, cfg *FlowConfig) ([]Condition, error) {
	out := make([]Condition, 0, len(children))
	for _, child := range children {
		c, err := rewriteCondition(child, stepKey, cfg)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// inToAny writes "field in [a, b]" as "any of field eq a, field eq b", for a fact
// that has no in operator.
func inToAny(field string, value any) (Condition, error) {
	items := asList(value)
	if len(items) == 0 {
		return Condition{}, fmt.Errorf("operator %q needs a non-empty list", OpIn)
	}
	eqs := make([]Condition, 0, len(items))
	for _, item := range items {
		eqs = append(eqs, Condition{Field: field, Op: OpEq, Value: item})
	}
	return Condition{Any: eqs}, nil
}

// liftLoop builds the single loop of the rework transitions (spec 9.2 item 7).
// Schema 1 has one counter however many rework transitions a flow has, so there
// is one loop: its bound is the largest N of the conditions rework_count lt N,
// and each edge keeps its own condition and so its own limit. Its header is the
// shared target of the rework edges; when they differ it has none and the
// counter is never reset.
func liftLoop(def Definition) (*Loop, error) {
	header, max, count := "", 0, 0
	agree := true
	for _, s := range def.Steps {
		for _, t := range s.Transitions {
			if !t.Rework {
				continue
			}
			n, ok := reworkLimit(t.When)
			if !ok {
				return nil, fmt.Errorf("lift: rework transition %q of %q needs a condition rework_count lt N", t.ID, s.StatusKey)
			}
			if n > max {
				max = n
			}
			if count == 0 {
				header = t.ToStatusKey
			} else if header != t.ToStatusKey {
				agree = false
			}
			count++
		}
	}
	if count == 0 {
		return nil, nil
	}
	if max > MaxReworkLimit {
		max = MaxReworkLimit
	}
	if max < 1 {
		max = 1
	}
	loop := &Loop{Key: LoopRework, Header: header, Max: LoopMax{N: max}}
	if !agree {
		loop.Header = ""
		loop.Reset = LoopResetNever
	}
	return loop, nil
}
