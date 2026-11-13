package issueworkflow

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"
)

// The enforced squad workflow engine (docs/engineering/kitviet-workflow-spec.md).
//
// A workflow with a flow_config is a flow. A request given to a squad that uses
// one runs on it: each step opens one sub-issue per task, the step closes when
// every task has, and the transitions of the step route the request on the
// verdicts. This file holds the pure part: the model, its validation and the
// decision. The server reads and writes the database around it.

// Task kinds and what a task must have to close.
const (
	KindWork = "work"
	KindGate = "gate"

	RequireOutcome    = "outcome"
	RequireAttachment = "attachment"
)

// Switch types.
const (
	SwitchBool = "bool"
	SwitchEnum = "enum"
)

// Limits of a flow.
const (
	MaxFlowSteps      = 12
	MaxTasksPerStep   = 8
	MaxSwitches       = 8
	MaxTaskTitleRunes = 80
	MaxReworkLimit    = 5
	maxEnumValues     = 16
	maxMessageRunes   = 4000
)

// Defaults of a flow_config that leaves a timeout out.
const (
	defaultTaskTimeoutMinutes = 120
	defaultTaskHardCapMinutes = 240
	defaultTaskMaxAttempts    = 2
)

var taskKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

// Task is one unit of work the engine opens as a sub-issue when a request
// enters the step. A gate must report an outcome to close.
type Task struct {
	Key          string     `json:"key"`
	Title        string     `json:"title"`
	Kind         string     `json:"kind"`
	Handler      Handler    `json:"handler"`
	Instructions string     `json:"instructions,omitempty"`
	When         *Condition `json:"when,omitempty"`
	Requires     []string   `json:"requires,omitempty"`
}

// normalizeTasks trims the free text of tasks and their handlers. It returns
// a copy, so the caller's steps are not changed.
func normalizeTasks(tasks []Task) []Task {
	if len(tasks) == 0 {
		return tasks
	}
	out := make([]Task, len(tasks))
	for i, t := range tasks {
		t.Key = strings.TrimSpace(t.Key)
		t.Title = strings.TrimSpace(t.Title)
		t.Instructions = strings.TrimSpace(t.Instructions)
		t.Handler = NormalizeHandler(t.Handler)
		out[i] = t
	}
	return out
}

// Needs reports whether the task must have the thing before it closes.
func (t Task) Needs(requirement string) bool {
	for _, r := range t.Requires {
		if r == requirement {
			return true
		}
	}
	return false
}

// Switch is a choice the lead makes at the plan step. A bool switch is on or
// off; an enum switch takes one of its values.
type Switch struct {
	Key     string   `json:"key"`
	Type    string   `json:"type"`
	Values  []string `json:"values,omitempty"`
	Default any      `json:"default"`
}

// FlowMessages are the templates of the comments the engine posts at a stop.
// The first line of a message is its title and the rest its body. The engine
// replaces {requester}, {request}, {step}, {rework} and {tasks}.
type FlowMessages struct {
	Stop1   string `json:"stop1,omitempty"`
	Stop2   string `json:"stop2,omitempty"`
	Stop3   string `json:"stop3,omitempty"`
	Stalled string `json:"stalled,omitempty"`
	// Human is the comment posted when the request enters a human-only step that is not in_review
	// (a step where a person approves something before the work goes on).
	Human string `json:"human,omitempty"`
}

// FlowConfig is the workflow-level configuration of a flow.
type FlowConfig struct {
	Version            int          `json:"version"`
	TaskTimeoutMinutes int          `json:"task_timeout_minutes,omitempty"`
	TaskHardCapMinutes int          `json:"task_hard_cap_minutes,omitempty"`
	TaskMaxAttempts    int          `json:"task_max_attempts,omitempty"`
	Switches           []Switch     `json:"switches,omitempty"`
	Messages           FlowMessages `json:"messages,omitempty"`
}

// ParseFlowConfig decodes a stored flow_config. A missing or null value is not
// a flow and returns nil.
func ParseFlowConfig(raw []byte) (*FlowConfig, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var cfg FlowConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("decode flow_config: %w", err)
	}
	return &cfg, nil
}

// TaskTimeout is the minutes an open task may sit without a run.
func (c *FlowConfig) TaskTimeout() int {
	if c == nil || c.TaskTimeoutMinutes <= 0 {
		return defaultTaskTimeoutMinutes
	}
	return c.TaskTimeoutMinutes
}

// TaskHardCap is the minutes a run on a task may last.
func (c *FlowConfig) TaskHardCap() int {
	if c == nil || c.TaskHardCapMinutes <= 0 {
		return defaultTaskHardCapMinutes
	}
	return c.TaskHardCapMinutes
}

// TaskAttempts is the number of runs a task gets before the request stalls.
func (c *FlowConfig) TaskAttempts() int {
	if c == nil || c.TaskMaxAttempts <= 0 {
		return defaultTaskMaxAttempts
	}
	return c.TaskMaxAttempts
}

// Switch returns the declared switch with the key.
func (c *FlowConfig) Switch(key string) (Switch, bool) {
	if c == nil {
		return Switch{}, false
	}
	for _, s := range c.Switches {
		if s.Key == key {
			return s, true
		}
	}
	return Switch{}, false
}

// DefaultSwitches is the value of every switch before the lead chooses. The map
// is new on each call.
func (c *FlowConfig) DefaultSwitches() map[string]any {
	out := map[string]any{}
	if c == nil {
		return out
	}
	for _, s := range c.Switches {
		switch s.Type {
		case SwitchEnum:
			if v, ok := s.Default.(string); ok && v != "" {
				out[s.Key] = v
			} else if len(s.Values) > 0 {
				out[s.Key] = s.Values[0]
			}
		default:
			v, _ := s.Default.(bool)
			out[s.Key] = v
		}
	}
	return out
}

// CheckValue reports whether value is a legal value of the switch.
func (s Switch) CheckValue(value any) error {
	switch s.Type {
	case SwitchBool:
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("switch %q takes true or false", s.Key)
		}
	case SwitchEnum:
		v, ok := value.(string)
		if ok {
			for _, allowed := range s.Values {
				if allowed == v {
					return nil
				}
			}
		}
		return fmt.Errorf("switch %q takes one of: %s", s.Key, strings.Join(s.Values, ", "))
	default:
		return fmt.Errorf("switch %q has an unknown type %q", s.Key, s.Type)
	}
	return nil
}

// ExpectedTasks lists the tasks a step opens for a request with these switch
// values: the tasks without a condition and those whose condition holds.
func ExpectedTasks(step Step, switches map[string]any) []Task {
	var out []Task
	facts := Facts{Switches: switches}
	for _, task := range step.Tasks {
		if task.When == nil || Evaluate(*task.When, facts) {
			out = append(out, task)
		}
	}
	return out
}

// Stop is why a request waits for a person: 1 is missing grounds, 3 is the end
// of the rework rounds, "stalled" is a task that stopped moving. Stop 2, the
// hand-over to a person, is the human-only step itself and is not stored.
type Stop string

const (
	StopNone    Stop = ""
	Stop1       Stop = "1"
	Stop3       Stop = "3"
	StopStalled Stop = "stalled"
)

func (s Stop) MarshalJSON() ([]byte, error) {
	switch s {
	case StopNone:
		return []byte("null"), nil
	case Stop1:
		return []byte("1"), nil
	case Stop3:
		return []byte("3"), nil
	}
	return []byte(`"stalled"`), nil
}

// UnmarshalJSON reads null, 1, 3 and "stalled"; any other value is a stop the
// reader does not know and counts as stalled.
func (s *Stop) UnmarshalJSON(raw []byte) error {
	switch strings.TrimSpace(string(raw)) {
	case "null":
		*s = StopNone
	case "1":
		*s = Stop1
	case "3":
		*s = Stop3
	default:
		*s = StopStalled
	}
	return nil
}

// FlowState is what the engine keeps in issue.flow for a request.
type FlowState struct {
	WorkflowID string `json:"workflow_id,omitempty"`
	Step       string `json:"step"`
	// Rework counts the rework transitions followed; the limit is read from it.
	Rework int `json:"rework"`
	// RoundBase is added to Rework to give the round of the tasks opened now.
	// It is 0 until a person resumes a request into a step that already ran in
	// the current round: the tasks open in a new round, which the unique task
	// index would otherwise refuse.
	RoundBase   int            `json:"round_base,omitempty"`
	Switches    map[string]any `json:"switches,omitempty"`
	EnteredAt   string         `json:"entered_at,omitempty"`
	Stop        Stop           `json:"stop,omitempty"`
	BlockedFrom string         `json:"blocked_from,omitempty"`
	PlanDone    bool           `json:"plan_done,omitempty"`
	// ClosedAt is left out until the request closes: the open-requests index
	// tests for the key.
	ClosedAt string `json:"closed_at,omitempty"`
	// Seq grows on every write; a write names the Seq it read. A state stored
	// before Seq existed reads as 0.
	Seq int `json:"seq,omitempty"`
	// Unreviewed and Trail are the fields of the schema 2 state (spec 2.4) that a
	// schema 1 request carries too.
	Unreviewed bool         `json:"unreviewed,omitempty"`
	Trail      []TrailEntry `json:"trail,omitempty"`
}

// Round is the round the tasks of the current step open in (and carry in
// issue.flow_round).
func (s FlowState) Round() int { return s.RoundBase + s.Rework }

// TaskState is a task of a request as the engine saw it: the issue's identity
// columns, whether it is closed, and the outcome it reported.
type TaskState struct {
	Step  string
	Key   string
	Kind  string
	Round int
	// Closed: the task issue is done or cancelled and has no run left.
	Closed bool
	// Cancelled: the task was cancelled instead of done.
	Cancelled bool
	// Outcome is issue.outcome; empty when none was reported.
	Outcome string
}

// DecisionKind is what the engine does next with a request.
type DecisionKind string

const (
	// DecisionWait: nothing to do until a task closes or a person acts.
	DecisionWait DecisionKind = "wait"
	// DecisionOpen: open the missing tasks of the step.
	DecisionOpen DecisionKind = "open"
	// DecisionRoute: leave the step by a transition.
	DecisionRoute DecisionKind = "route"
)

// Decision is what the engine does next with a request. DecideGraph returns it
// inside a GraphDecision; the fields keep their schema 1 meaning for a graph lifted
// from a schema 1 workflow.
type Decision struct {
	Kind DecisionKind
	// Missing are the tasks to open (DecisionOpen).
	Missing []Task
	// To is the status the request moves to (DecisionRoute).
	To string
	// Rework: the transition sends the request back for another round.
	Rework       bool
	TransitionID string
	// TasksOutcome is the verdict of the step's tasks that the transition was
	// chosen on.
	TasksOutcome string
	// Stop is set when To is blocked.
	Stop Stop
	// HumanStop: To is a human-only step (stop 2).
	HumanStop bool
}

// ValidateFlow checks the flow-specific rules of a workflow (spec section 3.7).
// A workflow without a flow_config may not carry tasks, human-only steps or
// rework transitions.
func ValidateFlow(def Definition) error {
	if def.Flow == nil {
		for _, s := range def.Steps {
			if len(s.Tasks) > 0 || s.HumanOnly {
				return invalid("step %q has tasks or is human-only: that needs a flow_config", s.StatusKey)
			}
			for _, t := range s.Transitions {
				if t.Rework {
					return invalid("step %q has a rework transition: that needs a flow_config", s.StatusKey)
				}
			}
		}
		return nil
	}
	if len(def.Steps) > MaxFlowSteps {
		return invalid("a flow workflow can have at most %d statuses", MaxFlowSteps)
	}
	if err := validateFlowConfig(def.Flow); err != nil {
		return err
	}
	withTasks := 0
	for _, s := range def.Steps {
		if len(s.Tasks) > 0 {
			withTasks++
		}
	}
	if withTasks == 0 {
		return invalid("a flow workflow needs at least one step with tasks")
	}
	plan, ok := def.Step(StartStatus)
	if !ok {
		return invalid("a flow workflow needs the %q plan step", StartStatus)
	}
	if len(plan.Tasks) > 0 {
		return invalid("the plan step %q cannot have tasks", plan.StatusKey)
	}
	if plan.NextStatusKey() == "" {
		return invalid("the plan step %q needs a default transition", plan.StatusKey)
	}
	for i, s := range def.Steps {
		if err := validateFlowStep(def, i, s); err != nil {
			return err
		}
	}
	return validateFlowGraph(def)
}

func validateFlowConfig(cfg *FlowConfig) error {
	if cfg.Version != 1 {
		return invalid("flow_config version must be 1")
	}
	if len(cfg.Switches) > MaxSwitches {
		return invalid("a flow can have at most %d switches", MaxSwitches)
	}
	seen := map[string]bool{}
	for _, s := range cfg.Switches {
		if !IsSwitchKey(s.Key) {
			return invalid("switch key %q must match %s", s.Key, switchKeyPattern.String())
		}
		if seen[s.Key] {
			return invalid("switch %q is declared more than once", s.Key)
		}
		seen[s.Key] = true
		switch s.Type {
		case SwitchBool:
			if _, ok := s.Default.(bool); !ok {
				return invalid("switch %q: default must be true or false", s.Key)
			}
		case SwitchEnum:
			if len(s.Values) == 0 || len(s.Values) > maxEnumValues {
				return invalid("switch %q needs 1-%d values", s.Key, maxEnumValues)
			}
			valueSeen := map[string]bool{}
			for _, v := range s.Values {
				if v == "" || valueSeen[v] {
					return invalid("switch %q has an empty or repeated value", s.Key)
				}
				valueSeen[v] = true
			}
			if d, ok := s.Default.(string); !ok || !valueSeen[d] {
				return invalid("switch %q: default must be one of its values", s.Key)
			}
		default:
			return invalid("switch %q: type must be bool or enum", s.Key)
		}
	}
	for _, msg := range []string{cfg.Messages.Stop1, cfg.Messages.Stop2, cfg.Messages.Stop3, cfg.Messages.Stalled, cfg.Messages.Human} {
		if utf8.RuneCountInString(msg) > maxMessageRunes {
			return invalid("a flow message must be at most %d characters", maxMessageRunes)
		}
	}
	return nil
}

func validateFlowStep(def Definition, index int, s Step) error {
	if len(s.Tasks) > MaxTasksPerStep {
		return invalid("step %q can have at most %d tasks", s.StatusKey, MaxTasksPerStep)
	}
	keys := map[string]bool{}
	var gateAgents = map[string]string{}
	for _, task := range s.Tasks {
		if !taskKeyPattern.MatchString(task.Key) {
			return invalid("task key %q must match %s", task.Key, taskKeyPattern.String())
		}
		if keys[task.Key] {
			return invalid("task %q appears more than once in step %q", task.Key, s.StatusKey)
		}
		keys[task.Key] = true
		if title := strings.TrimSpace(task.Title); title == "" || utf8.RuneCountInString(title) > MaxTaskTitleRunes {
			return invalid("task %q: title must be 1-%d characters", task.Key, MaxTaskTitleRunes)
		}
		if utf8.RuneCountInString(task.Instructions) > MaxInstructionsRunes {
			return invalid("task %q: instructions must be at most %d characters", task.Key, MaxInstructionsRunes)
		}
		if task.Kind != KindWork && task.Kind != KindGate {
			return invalid("task %q: kind must be work or gate", task.Key)
		}
		for _, r := range task.Requires {
			if r != RequireOutcome && r != RequireAttachment {
				return invalid("task %q requires %q: use outcome or attachment", task.Key, r)
			}
		}
		if task.Kind == KindGate && !task.Needs(RequireOutcome) {
			return invalid("gate task %q must require an outcome", task.Key)
		}
		if task.Handler.Type != HandlerAgent || task.Handler.ID == "" {
			return invalid("task %q must be handed to an agent", task.Key)
		}
		if task.When != nil {
			if err := ValidateFlowCondition(*task.When); err != nil {
				return err
			}
		}
		if task.Kind == KindGate {
			if other, dup := gateAgents[task.Handler.ID]; dup {
				return invalid("gates %q and %q of step %q use the same agent", other, task.Key, s.StatusKey)
			}
			gateAgents[task.Handler.ID] = task.Key
		}
	}
	if index > 0 {
		for _, task := range s.Tasks {
			if task.Kind != KindGate {
				continue
			}
			for _, before := range def.Steps[index-1].Tasks {
				if before.Kind == KindWork && before.Handler.ID == task.Handler.ID {
					return invalid("gate %q uses the same agent as work task %q of the previous step", task.Key, before.Key)
				}
			}
		}
	}
	if s.HumanOnly {
		for _, task := range s.Tasks {
			if task.Key != "export" || task.When == nil {
				return invalid("step %q is human-only: its only agent task is the optional export", s.StatusKey)
			}
		}
	}
	if len(s.Tasks) > 0 && !s.HumanOnly {
		if len(s.Transitions) == 0 {
			return invalid("step %q has tasks and needs transitions", s.StatusKey)
		}
		if s.NextStatusKey() == "" {
			return invalid("step %q needs a default transition", s.StatusKey)
		}
		if hasGate(s) && !routesOn(s, FieldTasksOutcome, TasksAllApproved) {
			return invalid("step %q has gates and needs a transition for tasks_outcome all_approved", s.StatusKey)
		}
	}
	return validateTransitions(s, def)
}

func hasGate(s Step) bool {
	for _, task := range s.Tasks {
		if task.Kind == KindGate {
			return true
		}
	}
	return false
}

// routesOn reports whether some transition of the step tests field against value.
func routesOn(s Step, field, value string) bool {
	found := false
	for _, t := range s.Transitions {
		if t.When == nil {
			continue
		}
		walkCondition(*t.When, func(c Condition) {
			if c.Field != field {
				return
			}
			if v, ok := c.Value.(string); ok && v == value {
				found = true
			}
			for _, item := range asList(c.Value) {
				if v, ok := item.(string); ok && v == value {
					found = true
				}
			}
		})
	}
	return found
}

// walkCondition calls fn for every field check in the condition.
func walkCondition(c Condition, fn func(Condition)) {
	switch {
	case c.All != nil:
		for _, child := range c.All {
			walkCondition(child, fn)
		}
	case c.Any != nil:
		for _, child := range c.Any {
			walkCondition(child, fn)
		}
	case c.Not != nil:
		walkCondition(*c.Not, fn)
	default:
		fn(c)
	}
}

// validateFlowGraph checks what spans steps: declared switches, rework
// transitions and cycles.
func validateFlowGraph(def Definition) error {
	index := map[string]int{}
	for i, s := range def.Steps {
		index[s.StatusKey] = i
	}
	var problem error
	check := func(c *Condition) {
		if c == nil || problem != nil {
			return
		}
		walkCondition(*c, func(f Condition) {
			if problem != nil {
				return
			}
			key, ok := strings.CutPrefix(f.Field, FieldSwitchPrefix)
			if !ok {
				return
			}
			sw, declared := def.Flow.Switch(key)
			if !declared {
				problem = invalid("switch %q is not declared in flow_config", key)
				return
			}
			values := []any{f.Value}
			if f.Op == OpIn {
				values = asList(f.Value)
			}
			for _, v := range values {
				switch sw.Type {
				case SwitchBool:
					if _, ok := v.(bool); !ok {
						problem = invalid("switch %q is a bool: compare it with true or false", key)
					}
				case SwitchEnum:
					if err := sw.CheckValue(v); err != nil {
						problem = invalid("switch %q: value %v is not one of its values", key, v)
					}
				}
			}
		})
	}
	for _, s := range def.Steps {
		for _, task := range s.Tasks {
			check(task.When)
		}
		for _, t := range s.Transitions {
			check(t.When)
		}
	}
	if problem != nil {
		return problem
	}
	edges := map[string][]string{}
	for i, s := range def.Steps {
		for _, t := range s.Transitions {
			if !t.Rework {
				edges[s.StatusKey] = append(edges[s.StatusKey], t.ToStatusKey)
				continue
			}
			if target, ok := index[t.ToStatusKey]; !ok || target >= i {
				return invalid("rework transition %q of %q must go back to an earlier step", t.ID, s.StatusKey)
			}
			limit, ok := reworkLimit(t.When)
			if !ok {
				return invalid("rework transition %q of %q needs a condition rework_count lt N", t.ID, s.StatusKey)
			}
			if limit < 1 || limit > MaxReworkLimit {
				return invalid("rework transition %q of %q can allow at most %d rounds", t.ID, s.StatusKey, MaxReworkLimit)
			}
		}
	}
	const (
		visiting = 1
		done     = 2
	)
	state := map[string]int{}
	var visit func(key string) bool
	visit = func(key string) bool {
		switch state[key] {
		case visiting:
			return true
		case done:
			return false
		}
		state[key] = visiting
		for _, next := range edges[key] {
			if visit(next) {
				return true
			}
		}
		state[key] = done
		return false
	}
	for _, s := range def.Steps {
		if visit(s.StatusKey) {
			return invalid("the transitions of this workflow form a cycle without a rework transition")
		}
	}
	return nil
}

// reworkLimit reads N from a condition that requires rework_count lt N, either
// alone or as one member of an all group.
func reworkLimit(c *Condition) (int, bool) {
	if c == nil {
		return 0, false
	}
	if c.Field == FieldReworkCount && c.Op == OpLt {
		n, ok := asNumber(c.Value)
		return int(n), ok
	}
	for _, child := range c.All {
		if n, ok := reworkLimit(&child); ok {
			return n, true
		}
	}
	return 0, false
}

// Roles of an issue on a flow.
const (
	RoleRequest = "request"
	RoleTask    = "task"
)

// FlowRef is the flow part of an issue response: where a request is, or which
// task of which step and round an issue is.
type FlowRef struct {
	Role       string `json:"role"`
	Step       string `json:"step"`
	Task       string `json:"task,omitempty"`
	Round      *int   `json:"round,omitempty"`
	Stop       Stop   `json:"stop,omitempty"`
	WorkflowID string `json:"workflow_id,omitempty"`
	Rework     *int   `json:"rework,omitempty"`
}

// IssueFlowRef reads the flow columns of an issue row. It returns nil for an
// issue that is neither a request on a flow nor a task the engine opened, and
// for a stored state it cannot read.
func IssueFlowRef(flow []byte, flowStep, flowTask pgtype.Text, flowRound pgtype.Int4) *FlowRef {
	if flowTask.Valid {
		ref := &FlowRef{Role: RoleTask, Step: flowStep.String, Task: flowTask.String}
		if flowRound.Valid {
			round := int(flowRound.Int32)
			ref.Round = &round
		}
		return ref
	}
	if len(flow) == 0 {
		return nil
	}
	var state FlowState
	if err := json.Unmarshal(flow, &state); err != nil || state.Step == "" {
		return nil
	}
	rework := state.Rework
	return &FlowRef{Role: RoleRequest, Step: state.Step, Stop: state.Stop, WorkflowID: state.WorkflowID, Rework: &rework}
}

// SplitMessage splits a stop message into its title (the first line) and body.
func SplitMessage(message string) (title, body string) {
	message = strings.TrimSpace(message)
	title, body, _ = strings.Cut(message, "\n")
	return strings.TrimSpace(title), strings.TrimSpace(body)
}

// RenderMessage replaces {name} in a message template with vars[name]. A name
// without a value is left as it is, so a template typo shows instead of vanishing.
func RenderMessage(template string, vars map[string]string) string {
	pairs := make([]string, 0, len(vars)*2)
	for name, value := range vars {
		pairs = append(pairs, "{"+name+"}", value)
	}
	return strings.NewReplacer(pairs...).Replace(template)
}

// TaskInput is a task of an earlier step whose output the new task reads.
type TaskInput struct {
	Identifier string
	Key        string
	Title      string
	// Round is the flow_round of the row; a row key repeats across entries.
	Round int
}

// TaskDescriptionInput is what the description of a task issue says. The
// description is fixed text: the lead does not write it, and it never carries
// the reasoning of the agent who did the earlier work, so a gate judges the
// files and nothing else.
type TaskDescriptionInput struct {
	Task              Task
	RequestIdentifier string
	// Round is the round the task opens in, 0 for the first.
	Round int
	// Inputs are the tasks of the step before whose attached files this task reads.
	Inputs []TaskInput
	// Rejected are the gates of the previous round that rejected the draft,
	// listed on a rework round.
	Rejected []TaskInput
}

// TaskDescription renders the description of a task issue (spec section 5.5).
func TaskDescription(in TaskDescriptionInput) string {
	var b strings.Builder
	if goal := strings.TrimSpace(in.Task.Instructions); goal != "" {
		b.WriteString(goal)
		b.WriteString("\n\n")
	}
	fmt.Fprintf(&b, "This issue is the task `%s` of the request %s. Read the request first; do only this task.\n", in.Task.Key, in.RequestIdentifier)
	if len(in.Inputs) > 0 {
		b.WriteString("\n## Inputs\n")
		for _, input := range in.Inputs {
			fmt.Fprintf(&b, "- %s · %s: the files attached to it (download one with `multica attachment download <attachment-id>`).\n", input.Identifier, input.Key)
		}
		b.WriteString("Judge the attached files only. Do not rely on the comments of whoever wrote them.\n")
	}
	if in.Round > 0 && len(in.Rejected) > 0 {
		fmt.Fprintf(&b, "\n## Rework (round %d)\nThe previous round was rejected. Read what these tasks found and fix it:\n", in.Round+1)
		for _, input := range in.Rejected {
			fmt.Fprintf(&b, "- %s · %s\n", input.Identifier, input.Key)
		}
	}
	b.WriteString("\n## How to finish\n")
	b.WriteString("`<id>` is the identifier of this issue.\n")
	if in.Task.Needs(RequireAttachment) {
		b.WriteString("1. Attach your output as a file: `multica issue comment add <id> --attachment <file> --content \"...\"`.\n")
	}
	switch {
	case in.Task.Kind == KindGate || in.Task.Needs(RequireOutcome):
		b.WriteString("- Pass: `multica issue status <id> done --outcome approved`.\n")
		b.WriteString("- Fail: `multica issue status <id> done --outcome rejected`, after a comment that lists each finding.\n")
	default:
		b.WriteString("- Done: `multica issue status <id> done`.\n")
	}
	b.WriteString("- You cannot go on (missing input or grounds): `multica issue status <id> blocked --outcome blocked`, after a comment that says what is missing.\n")
	b.WriteString("\n## What the server refuses\n")
	if in.Task.Kind == KindGate || in.Task.Needs(RequireOutcome) {
		b.WriteString("- Closing this issue without `--outcome approved` or `--outcome rejected`.\n")
	}
	if in.Task.Needs(RequireAttachment) {
		b.WriteString("- Closing this issue before you attached a file to it.\n")
	}
	b.WriteString("- `--outcome blocked` with a done status, and any outcome other than `blocked` with a blocked status.\n")
	b.WriteString("- Moving the request itself or opening sub-issues under it: the request is run by the workflow.\n")
	return b.String()
}

// FlowTaskBriefInput is what the brief of a task run says.
type FlowTaskBriefInput struct {
	WorkflowName      string
	RequestIdentifier string
	TaskIdentifier    string
	StepName          string
	Task              Task
	// Round is the round the task opened in, 0 for the first.
	Round int
	// Inputs are the tasks of the step before; Rejected the gates of the
	// previous round that rejected the draft.
	Inputs   []TaskInput
	Rejected []TaskInput
}

// FlowTaskBrief renders what an agent reads about the task it works on: what the
// workflow expects of it, where its input is, the commands that end it with the
// outcome the server requires, and what the server refuses. It is the claim-time
// counterpart of the task description, with this issue's own identifier in the
// commands.
func FlowTaskBrief(in FlowTaskBriefInput) string {
	var b strings.Builder
	id := in.TaskIdentifier
	fmt.Fprintf(&b, "This issue is the task %q (%s) of the request %s, run by the workflow %s at the step %s. A server drives the workflow: do this task and nothing else.\n",
		in.Task.Key, in.Task.Title, in.RequestIdentifier, briefLine(in.WorkflowName), briefLine(in.StepName))
	if len(in.Inputs) > 0 {
		b.WriteString("\nYour input is the files attached to these tasks (download one with `multica attachment download <attachment-id>`):\n")
		for _, input := range in.Inputs {
			fmt.Fprintf(&b, "- %s · %s\n", input.Identifier, input.Key)
		}
		b.WriteString("Work from the attached files. Do not rely on the comments of whoever wrote them.\n")
	}
	if in.Round > 0 && len(in.Rejected) > 0 {
		fmt.Fprintf(&b, "\nThis is round %d: the previous round was rejected by these gates. Read what they found:\n", in.Round+1)
		for _, input := range in.Rejected {
			fmt.Fprintf(&b, "- %s · %s\n", input.Identifier, input.Key)
		}
	}
	b.WriteString("\nYour moves:\n")
	if in.Task.Needs(RequireAttachment) {
		fmt.Fprintf(&b, "Attach your output  → multica issue comment add %s --attachment <file> --content \"...\"\n", id)
	}
	if in.Task.Kind == KindGate || in.Task.Needs(RequireOutcome) {
		fmt.Fprintf(&b, "Pass                → multica issue status %s done --outcome approved\n", id)
		fmt.Fprintf(&b, "Fail                → comment each finding, then multica issue status %s done --outcome rejected\n", id)
	} else {
		fmt.Fprintf(&b, "Done                → multica issue status %s done\n", id)
	}
	fmt.Fprintf(&b, "Can't go on         → comment what is missing, then multica issue status %s blocked --outcome blocked\n", id)
	b.WriteString("\nThe server refuses:\n")
	if in.Task.Kind == KindGate || in.Task.Needs(RequireOutcome) {
		b.WriteString("- closing this issue without --outcome approved or --outcome rejected;\n")
	}
	if in.Task.Needs(RequireAttachment) {
		b.WriteString("- closing this issue before you attached a file to it;\n")
	}
	b.WriteString("- --outcome blocked with a done status, and any other outcome with a blocked status;\n")
	b.WriteString("- any change of the status of the request, and sub-issues created under it.\n")
	b.WriteString("Do not run `multica issue flow`: the lead of the squad does.\n")
	return b.String()
}

// FlowLeadBriefInput is what the brief of a run on the request itself says.
type FlowLeadBriefInput struct {
	WorkflowName      string
	RequestIdentifier string
	Step              string
	StepName          string
	// PlanStep: the request waits for the lead's plan.
	PlanStep bool
	Stop     Stop
	// Switches and Values describe the choices the lead makes when it plans.
	Switches []Switch
	Values   map[string]any
}

// FlowLeadBrief renders what the lead of the squad reads about a request a flow
// runs: at the plan step it plans; afterwards the server moves the request and
// the lead only asks it to advance.
func FlowLeadBrief(in FlowLeadBriefInput) string {
	var b strings.Builder
	id := in.RequestIdentifier
	fmt.Fprintf(&b, "%s is run by the workflow %s and is at the step %s. A server moves it: you do not split the work and you do not hand it out.\n",
		id, briefLine(in.WorkflowName), briefLine(in.StepName))
	switch {
	case in.Stop != StopNone:
		b.WriteString("\nThe request has stopped and waits for a person (a member who manages the squad resumes it). There is nothing for you to run now.\n")
	case in.PlanStep:
		fmt.Fprintf(&b, "\nPlan it:\n1. Read the request and its attachments: `multica issue get %s`.\n", id)
		fmt.Fprintf(&b, "2. Run `multica issue flow show %s` to see the state.\n", id)
		fmt.Fprintf(&b, "3. Choose the switches and run `multica issue flow plan %s`, for example `--switch customs_lookup=true`.\n", id)
		if len(in.Switches) > 0 {
			b.WriteString("   The switches of this workflow:\n")
			for _, s := range in.Switches {
				current := ""
				if v, ok := in.Values[s.Key]; ok {
					current = fmt.Sprintf(" (now %v)", v)
				}
				switch s.Type {
				case SwitchEnum:
					fmt.Fprintf(&b, "   - %s: one of %s%s\n", s.Key, strings.Join(s.Values, ", "), current)
				default:
					fmt.Fprintf(&b, "   - %s: true or false%s\n", s.Key, current)
				}
			}
		}
		fmt.Fprintf(&b, "4. If the request lacks what the work needs, report it instead: `multica issue flow plan %s --blocked --missing \"<what>\" --reason \"<why>\"`.\n", id)
	default:
		fmt.Fprintf(&b, "\nWhen you are woken, run `multica issue flow advance %s`. It opens the tasks of the step, or moves the request on when they are all closed, and tells you what it waits for. It is safe to run again.\n", id)
	}
	b.WriteString("\nDo not create sub-issues under this request and do not set the status of the request: the server refuses both.\n")
	b.WriteString("The server sets the status of the request when it stops. It posts the stop comment (the headers \"BLOCKED · stop 1\", \"BLOCKED · stop 3\" and \"IN REVIEW · stop 2\") unless you already posted one for that stop. You may post the stop comment yourself, as your squad rules say; the request then gets exactly one.\n")
	return b.String()
}
