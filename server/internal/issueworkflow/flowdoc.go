package issueworkflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// FlowDoc is the schema 2 flow definition (docs/engineering/flow-engine-generalization-spec.md
// section 2.3): roles, variables, verdict sets, loops, stop kinds, terminals and
// steps joined by edges. This file holds the data model and its strict parse;
// the rules that need more than one field live in the validator.

// FlowDocVersion is the schema version this server writes and reads.
const FlowDocVersion = 2

// Limits of a FlowDoc (spec section 2.7). The server owns them; ParseFlowDoc
// enforces the document size and the validator enforces the counts.
const (
	DocMaxBytes             = 512 << 10
	DocMaxSteps             = 40
	DocMaxStatusNames       = 12
	DocMaxTasksPerStep      = 12
	DocMaxEdgesPerStep      = 10
	DocMaxRoles             = 16
	DocMaxVars              = 16
	DocMaxVerdictSets       = 8
	DocMaxLoops             = 8
	DocMaxLoopMax           = 20
	DocMaxInstances         = 20
	DocMaxExprDepth         = 4
	DocMaxAfterDepth        = 6
	DocMaxMessageRunes      = 4000
	DocMaxInstructionsRunes = 8000
	DocMaxInputsPerTask     = 12
	DocMaxSetsPerTask       = 8
	DocMaxSkillsPerTask     = 8
	DocMaxTaskIssuesHard    = 500
)

// Defaults of a Policy that leaves a value out (spec section 2.3).
const (
	DefaultTaskIdleMinutes    = 120
	DefaultTaskHardCapMinutes = 240
	DefaultTaskMaxAttempts    = 2
	DefaultHumanRemindMinutes = 1440
	DefaultHumanStallMinutes  = 10080
	DefaultMaxActivations     = 100
	DefaultMaxTaskIssues      = 300
)

// Kinds and classes a FlowDoc names.
const (
	RoleKindAgent     = "agent"
	RoleKindMember    = "member"
	RoleKindLead      = "lead"
	RoleKindRequester = "requester"
	RoleKindManagers  = "managers"

	VarTypeBool       = "bool"
	VarTypeEnum       = "enum"
	VarTypeInt        = "int"
	VarTypeString     = "string"
	VarTypeListString = "list_string"

	ClassPass    = "pass"
	ClassFail    = "fail"
	ClassNeutral = "neutral"
	// ClassBlock is the class of the outcome blocked. No verdict set lists it.
	ClassBlock = "block"

	LoopResetOnEntry = "on_entry"
	LoopResetNever   = "never"

	StepKindTasks  = "tasks"
	StepKindLead   = "lead"
	StepKindManual = "manual"

	JoinOverGates  = "gates"
	JoinOverAll    = "all"
	JoinModeAll    = "all"
	JoinModeAny    = "any"
	JoinModeQuorum = "quorum"

	JoinBlockImmediate = "immediate"
	JoinBlockAtClose   = "at_close"

	EdgeTriggerAuto   = "auto"
	EdgeTriggerManual = "manual"

	// DocStopStalled is the stop code that every FlowDoc has without declaring it.
	DocStopStalled = "stalled"
)

// FlowDoc is a schema 2 flow definition.
type FlowDoc struct {
	Version     int               `json:"version"`
	Start       string            `json:"start"`
	Roles       []Role            `json:"roles,omitempty"`
	Vars        []Var             `json:"vars,omitempty"`
	VerdictSets []VerdictSet      `json:"verdicts,omitempty"`
	Loops       []Loop            `json:"loops,omitempty"`
	Stops       []StopKind        `json:"stops,omitempty"`
	Terminals   []Terminal        `json:"terminals"`
	Steps       []DocStep         `json:"steps"`
	Messages    map[string]string `json:"messages,omitempty"`
	Policy      Policy            `json:"policy"`
}

// Role is a seat that tasks name. lead, requester and managers are built in.
type Role struct {
	Key   string `json:"key"`
	Label string `json:"label,omitempty"`
	Kind  string `json:"kind"`
	// Pool is the number of agents or members the role needs bound; 0 means 1.
	Pool int `json:"pool,omitempty"`
	// Bind exists only in a compiled document.
	Bind *RoleBind `json:"bind,omitempty"`
	// Source is stored and ignored by the engine.
	Source *RoleSource `json:"source,omitempty"`
}

// RoleBind is who fills a role in a compiled document.
type RoleBind struct {
	AgentIDs  []string `json:"agent_ids,omitempty"`
	MemberIDs []string `json:"member_ids,omitempty"`
}

// RoleSource says where a role came from (the skill/agent bundle importer).
type RoleSource struct {
	Bundle   string `json:"bundle,omitempty"`
	AgentKey string `json:"agent_key,omitempty"`
}

// Var is a typed variable that people or tasks set through the API.
type Var struct {
	Key      string   `json:"key"`
	Type     string   `json:"type"`
	Values   []string `json:"values,omitempty"`
	Min      *int     `json:"min,omitempty"`
	Max      *int     `json:"max,omitempty"`
	MaxLen   int      `json:"max_len,omitempty"`
	MaxItems int      `json:"max_items,omitempty"`
	Default  any      `json:"default"`
	SetBy    []string `json:"set_by,omitempty"`
	// SetAt lists step keys, or ["*"] for any step.
	SetAt []string `json:"set_at,omitempty"`
}

// VerdictSet is the outcomes a task may report and the class of each.
type VerdictSet struct {
	Key      string    `json:"key"`
	Verdicts []Verdict `json:"verdicts"`
}

// Verdict is one outcome of a set.
type Verdict struct {
	Key   string `json:"key"`
	Class string `json:"class"`
	Label string `json:"label,omitempty"`
}

// Loop bounds how many times a back-edge may be followed.
type Loop struct {
	Key    string  `json:"key"`
	Header string  `json:"header"`
	Max    LoopMax `json:"max"`
	// Reset is on_entry (the default) or never.
	Reset string `json:"reset,omitempty"`
}

// EffectiveReset is the reset rule with the default applied.
func (l Loop) EffectiveReset() string {
	if l.Reset == "" {
		return LoopResetOnEntry
	}
	return l.Reset
}

// LoopMax is a loop's bound: a number, or a variable clamped to a cap. In JSON
// it is an int or {"var": key, "cap": n}.
type LoopMax struct {
	// N is the bound when Var is empty.
	N   int
	Var string
	Cap int
}

// FromVar reports whether the bound is read from a variable.
func (m LoopMax) FromVar() bool { return m.Var != "" }

// Limit is the largest value the bound can take: N, or Cap.
func (m LoopMax) Limit() int {
	if m.FromVar() {
		return m.Cap
	}
	return m.N
}

func (m LoopMax) MarshalJSON() ([]byte, error) {
	if m.FromVar() {
		return json.Marshal(struct {
			Var string `json:"var"`
			Cap int    `json:"cap"`
		}{m.Var, m.Cap})
	}
	return json.Marshal(m.N)
}

func (m *LoopMax) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if bytes.Equal(raw, []byte("null")) {
		return nil
	}
	if len(raw) > 0 && raw[0] == '{' {
		var obj struct {
			Var string `json:"var"`
			Cap int    `json:"cap"`
		}
		if err := decodeStrict(raw, &obj); err != nil {
			return fmt.Errorf("loop max: %w", err)
		}
		if obj.Var == "" {
			return fmt.Errorf("loop max: an object needs a var")
		}
		*m = LoopMax{Var: obj.Var, Cap: obj.Cap}
		return nil
	}
	var n int
	if err := json.Unmarshal(raw, &n); err != nil {
		return fmt.Errorf("loop max: want a whole number or {var, cap}: %w", err)
	}
	*m = LoopMax{N: n}
	return nil
}

// StopKind is a named way for a request to park and wait for a person.
type StopKind struct {
	Code  string `json:"code"`
	Class string `json:"class"`
	Label string `json:"label,omitempty"`
	// Message is a key of FlowDoc.Messages.
	Message  string `json:"message,omitempty"`
	Status   string `json:"status,omitempty"`
	ResumeBy string `json:"resume_by,omitempty"`
	// LegacyNo is the stop number schema 1 gave this stop (1 or 3).
	LegacyNo int `json:"legacy_no,omitempty"`
}

// Terminal ends a request.
type Terminal struct {
	Key             string `json:"key"`
	Status          string `json:"status"`
	LeavesWorkspace bool   `json:"leaves_workspace,omitempty"`
}

// StepStatus is the board status a step shows: an existing status by key, or a
// new one by name and category.
type StepStatus struct {
	Key      string `json:"key,omitempty"`
	Name     string `json:"name,omitempty"`
	Category string `json:"category,omitempty"`
}

// StepLead configures a step of kind lead.
type StepLead struct {
	Sets []string `json:"sets,omitempty"`
	// Verdicts is a verdict set key; the built-in plan set when empty.
	Verdicts string    `json:"verdicts,omitempty"`
	Requires *Requires `json:"requires,omitempty"`
}

// StepLayout is where the designer draws a step.
type StepLayout struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// DocStep is a node of the flow: it opens tasks, waits for the lead, or waits
// for a person, then leaves by an edge.
type DocStep struct {
	Key      string     `json:"key"`
	Title    string     `json:"title,omitempty"`
	Status   StepStatus `json:"status"`
	Kind     string     `json:"kind"`
	Tasks    []DocTask  `json:"tasks,omitempty"`
	Join     *Join      `json:"join,omitempty"`
	Lead     *StepLead  `json:"lead,omitempty"`
	ExitRole string     `json:"exit_role,omitempty"`
	// Notice is a key of FlowDoc.Messages posted when the request enters the step.
	Notice string      `json:"notice,omitempty"`
	Edges  []DocEdge   `json:"edges"`
	Layout *StepLayout `json:"layout,omitempty"`
}

// EffectiveJoin is the step's join with the defaults applied.
func (s DocStep) EffectiveJoin() Join {
	j := Join{}
	if s.Join != nil {
		j = *s.Join
	}
	if j.Over.Kind == "" {
		j.Over = JoinOver{Kind: JoinOverGates}
	}
	if j.Mode == "" {
		j.Mode = JoinModeAll
	}
	if j.Block == "" {
		j.Block = JoinBlockImmediate
	}
	return j
}

// DocTask is one unit of work or judgement a step opens.
type DocTask struct {
	Key          string        `json:"key"`
	Title        string        `json:"title"`
	Kind         string        `json:"kind"`
	Role         string        `json:"role"`
	Instances    *Instances    `json:"instances,omitempty"`
	After        []string      `json:"after,omitempty"`
	When         *Condition    `json:"when,omitempty"`
	Verdicts     string        `json:"verdicts,omitempty"`
	Requires     *Requires     `json:"requires,omitempty"`
	Inputs       []Input       `json:"inputs,omitempty"`
	Sets         []string      `json:"sets,omitempty"`
	Skills       []string      `json:"skills,omitempty"`
	Deadline     *TaskDeadline `json:"deadline,omitempty"`
	Instructions string        `json:"instructions,omitempty"`
}

// Instances opens a task several times: a fixed count, or one per element of a
// list variable between Min and Max.
type Instances struct {
	Count   int    `json:"count,omitempty"`
	FromVar string `json:"from_var,omitempty"`
	Min     int    `json:"min,omitempty"`
	Max     int    `json:"max,omitempty"`
	// Distinct asks for a different person per instance; true when absent.
	Distinct *bool `json:"distinct,omitempty"`
}

// IsDistinct is Distinct with its default of true.
func (i Instances) IsDistinct() bool { return i.Distinct == nil || *i.Distinct }

// Requires is what a task must have before it can close.
type Requires struct {
	Files []FileReq `json:"files,omitempty"`
	// Verdict is tri-state: absent means true for a gate and false for work.
	Verdict *bool    `json:"verdict,omitempty"`
	Vars    []string `json:"vars,omitempty"`
}

// FileReq is an attachment a task must carry.
type FileReq struct {
	Slot         string   `json:"slot"`
	Min          int      `json:"min"`
	Max          int      `json:"max,omitempty"`
	Ext          []string `json:"ext,omitempty"`
	ContentTypes []string `json:"content_types,omitempty"`
}

// Input names the output of an earlier task that this task reads. From is
// "<stepKey>.<taskKey>".
type Input struct {
	From     string `json:"from"`
	Slot     string `json:"slot,omitempty"`
	Required bool   `json:"required,omitempty"`
}

// TaskDeadline overrides the policy timeouts of one task. The first three apply to
// agents, the last two to people.
type TaskDeadline struct {
	IdleMinutes        int `json:"idle_minutes,omitempty"`
	HardCapMinutes     int `json:"hard_cap_minutes,omitempty"`
	MaxAttempts        int `json:"max_attempts,omitempty"`
	RemindAfterMinutes int `json:"remind_after_minutes,omitempty"`
	StallAfterMinutes  int `json:"stall_after_minutes,omitempty"`
}

// Join says when a step's tasks are done and what they decided.
type Join struct {
	Over  JoinOver `json:"over"`
	Mode  string   `json:"mode,omitempty"`
	K     int      `json:"k,omitempty"`
	Block string   `json:"block,omitempty"`
}

// JoinOver is the tasks a join judges: the gates, every task, or a list of task
// keys. In JSON it is "gates", "all" or an array of keys.
type JoinOver struct {
	// Kind is JoinOverGates, JoinOverAll or "tasks"; empty is the default, gates.
	Kind  string
	Tasks []string
}

// JoinOverTasks is the Kind of a join over an explicit list of tasks.
const JoinOverTasks = "tasks"

func (o JoinOver) MarshalJSON() ([]byte, error) {
	switch o.Kind {
	case "", JoinOverGates:
		return json.Marshal(JoinOverGates)
	case JoinOverAll:
		return json.Marshal(JoinOverAll)
	}
	tasks := o.Tasks
	if tasks == nil {
		tasks = []string{}
	}
	return json.Marshal(tasks)
}

func (o *JoinOver) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if bytes.Equal(raw, []byte("null")) {
		return nil
	}
	if len(raw) > 0 && raw[0] == '[' {
		var keys []string
		if err := json.Unmarshal(raw, &keys); err != nil {
			return fmt.Errorf("join over: a list holds task keys: %w", err)
		}
		if keys == nil {
			keys = []string{}
		}
		*o = JoinOver{Kind: JoinOverTasks, Tasks: keys}
		return nil
	}
	var word string
	if err := json.Unmarshal(raw, &word); err != nil {
		return fmt.Errorf("join over: want %q, %q or a list of task keys: %w", JoinOverGates, JoinOverAll, err)
	}
	switch word {
	case JoinOverGates, JoinOverAll:
		*o = JoinOver{Kind: word}
		return nil
	}
	return fmt.Errorf("join over: %q is not %q, %q or a list of task keys", word, JoinOverGates, JoinOverAll)
}

// DocEdge leaves a step: to another step or terminal, or into a stop.
type DocEdge struct {
	ID      string     `json:"id"`
	Label   string     `json:"label,omitempty"`
	To      string     `json:"to,omitempty"`
	When    *Condition `json:"when,omitempty"`
	Loop    string     `json:"loop,omitempty"`
	Stop    string     `json:"stop,omitempty"`
	Trigger string     `json:"trigger,omitempty"`
}

// IsBack reports whether the edge closes a loop.
func (e DocEdge) IsBack() bool { return e.Loop != "" }

// Policy holds the timeouts, caps and acknowledged exceptions of a flow.
type Policy struct {
	TaskIdleMinutes        int   `json:"task_idle_minutes,omitempty"`
	TaskHardCapMinutes     int   `json:"task_hard_cap_minutes,omitempty"`
	TaskMaxAttempts        int   `json:"task_max_attempts,omitempty"`
	HumanRemindMinutes     int   `json:"human_remind_minutes,omitempty"`
	HumanStallMinutes      int   `json:"human_stall_minutes,omitempty"`
	MaxActivations         int   `json:"max_activations,omitempty"`
	MaxTaskIssues          int   `json:"max_task_issues,omitempty"`
	LeadMayWork            bool  `json:"lead_may_work,omitempty"`
	AllowUnattendedRelease bool  `json:"allow_unattended_release,omitempty"`
	Acks                   []Ack `json:"acks,omitempty"`
}

// Ack is an admin's acknowledgement of a warning, recorded at publish.
type Ack struct {
	Kind string `json:"kind"`
	By   string `json:"by,omitempty"`
	At   string `json:"at,omitempty"`
}

// Effective returns the policy with the defaults of spec section 2.3 for every
// value left out. The result never exceeds the hard cap on task issues.
func (p Policy) Effective() Policy {
	fill := func(v *int, def int) {
		if *v <= 0 {
			*v = def
		}
	}
	fill(&p.TaskIdleMinutes, DefaultTaskIdleMinutes)
	fill(&p.TaskHardCapMinutes, DefaultTaskHardCapMinutes)
	fill(&p.TaskMaxAttempts, DefaultTaskMaxAttempts)
	fill(&p.HumanRemindMinutes, DefaultHumanRemindMinutes)
	fill(&p.HumanStallMinutes, DefaultHumanStallMinutes)
	fill(&p.MaxActivations, DefaultMaxActivations)
	fill(&p.MaxTaskIssues, DefaultMaxTaskIssues)
	if p.MaxTaskIssues > DocMaxTaskIssuesHard {
		p.MaxTaskIssues = DocMaxTaskIssuesHard
	}
	return p
}

// Severities of an Issue.
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
	SeverityInfo    = "info"
)

// Issue is one problem found in a document. Path names elements by key, not
// by position, so it survives a reorder.
type Issue struct {
	Code     string         `json:"code"`
	Severity string         `json:"severity"`
	Path     string         `json:"path,omitempty"`
	Message  string         `json:"message"`
	Params   map[string]any `json:"params,omitempty"`
	// Ack names the acknowledgement an admin must give for a warning to be
	// accepted when the document is published; empty when none is needed.
	Ack string `json:"ack,omitempty"`
}

func (i Issue) Error() string {
	if i.Path == "" {
		return i.Code + ": " + i.Message
	}
	return i.Code + " at " + i.Path + ": " + i.Message
}

func docIssue(code, path, format string, args ...any) Issue {
	return Issue{Code: code, Severity: SeverityError, Path: path, Message: fmt.Sprintf(format, args...)}
}

// Codes ParseFlowDoc reports.
const (
	CodeFlowSchemaUnsupported = "flow_schema_unsupported"
	CodeFlowDocInvalid        = "flow_doc_invalid"
	CodeFlowDocTooLarge       = "flow_doc_too_large"
	CodeKeyInvalid            = "key_invalid"
	CodeVarSetInvalid         = "var_set_invalid"
)

// ParseFlowDoc reads a stored or submitted schema 2 document. It is strict: a
// key the server does not know at any level, a key that does not match the key
// pattern, a version above 2 and a document over 512 KiB are all errors, because
// a server must not guess what a newer key means. It reports every problem it
// finds. The returned document is only meaningful when there is no issue.
func ParseFlowDoc(raw []byte) (FlowDoc, []Issue) {
	var doc FlowDoc
	if len(raw) > DocMaxBytes {
		return doc, []Issue{docIssue(CodeFlowDocTooLarge, "", "the document is %d bytes; the limit is %d", len(raw), DocMaxBytes)}
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return doc, []Issue{docIssue(CodeFlowDocInvalid, "", "not valid JSON: %v", err)}
	}
	obj, ok := generic.(map[string]any)
	if !ok {
		return doc, []Issue{docIssue(CodeFlowDocInvalid, "", "a flow document is a JSON object")}
	}
	if v, present := obj["version"]; present {
		if n, isNum := v.(float64); isNum && n > FlowDocVersion {
			return doc, []Issue{docIssue(CodeFlowSchemaUnsupported, "version", "version %v is newer than this server understands (%d)", v, FlowDocVersion)}
		}
	}
	var issues []Issue
	walkUnknownKeys("", generic, reflect.TypeOf(doc), &issues)
	if len(issues) > 0 {
		return doc, issues
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return FlowDoc{}, []Issue{docIssue(CodeFlowDocInvalid, "", "%v", err)}
	}
	if doc.Version != FlowDocVersion {
		return doc, []Issue{docIssue(CodeFlowDocInvalid, "version", "version must be %d", FlowDocVersion)}
	}
	issues = append(issues, checkDocKeys(doc)...)
	issues = append(issues, checkTaskVars(doc)...)
	return doc, issues
}

// decodeStrict decodes JSON into v and refuses keys v has no field for.
func decodeStrict(raw []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

var jsonUnmarshalerType = reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()

// walkUnknownKeys reports every object key that the Go type behind it has no
// JSON field for. Types with their own UnmarshalJSON check themselves.
func walkUnknownKeys(path string, value any, t reflect.Type, issues *[]Issue) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if reflect.PointerTo(t).Implements(jsonUnmarshalerType) {
		return
	}
	switch t.Kind() {
	case reflect.Struct:
		obj, ok := value.(map[string]any)
		if !ok {
			return
		}
		fields := map[string]reflect.Type{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if name == "-" || !f.IsExported() {
				continue
			}
			if name == "" {
				name = f.Name
			}
			fields[name] = f.Type
		}
		names := make([]string, 0, len(obj))
		for name := range obj {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			ft, known := fields[name]
			child := name
			if path != "" {
				child = path + "." + name
			}
			if !known {
				*issues = append(*issues, docIssue(CodeFlowDocInvalid, child, "unknown key %q", name))
				continue
			}
			walkUnknownKeys(child, obj[name], ft, issues)
		}
	case reflect.Slice:
		items, ok := value.([]any)
		if !ok {
			return
		}
		for i, item := range items {
			walkUnknownKeys(path+"["+elementLabel(item, i)+"]", item, t.Elem(), issues)
		}
	case reflect.Map:
		obj, ok := value.(map[string]any)
		if !ok {
			return
		}
		for name, item := range obj {
			walkUnknownKeys(path+"["+name+"]", item, t.Elem(), issues)
		}
	}
}

// elementLabel names a list element by its key, id or code when it has one.
func elementLabel(item any, index int) string {
	if obj, ok := item.(map[string]any); ok {
		for _, field := range []string{"key", "id", "code"} {
			if s, ok := obj[field].(string); ok && docKeyPattern.MatchString(s) {
				return s
			}
		}
	}
	return fmt.Sprint(index)
}

// docKeyPattern is the pattern of every key of a FlowDoc; it is the same as
// the task key pattern of schema 1.
var docKeyPattern = taskKeyPattern

// checkDocKeys checks that every key the document defines matches the key
// pattern. References (a task's role, an edge's target) are not keys they
// define, so a dangling one is the validator's finding.
func checkDocKeys(doc FlowDoc) []Issue {
	var issues []Issue
	check := func(path, kind, key string) {
		if !docKeyPattern.MatchString(key) {
			issues = append(issues, Issue{
				Code: CodeKeyInvalid, Severity: SeverityError, Path: path,
				Message: fmt.Sprintf("%s %q must match %s", kind, key, docKeyPattern.String()),
				Params:  map[string]any{"key": key, "pattern": docKeyPattern.String()},
			})
		}
	}
	label := func(key string, index int) string {
		if docKeyPattern.MatchString(key) {
			return key
		}
		return fmt.Sprint(index)
	}
	for i, r := range doc.Roles {
		check("roles["+label(r.Key, i)+"]", "role key", r.Key)
	}
	for i, v := range doc.Vars {
		check("vars["+label(v.Key, i)+"]", "var key", v.Key)
	}
	for i, set := range doc.VerdictSets {
		path := "verdicts[" + label(set.Key, i) + "]"
		check(path, "verdict set key", set.Key)
		for j, v := range set.Verdicts {
			check(path+".verdicts["+label(v.Key, j)+"]", "verdict key", v.Key)
		}
	}
	for i, l := range doc.Loops {
		check("loops["+label(l.Key, i)+"]", "loop key", l.Key)
	}
	for i, s := range doc.Stops {
		check("stops["+label(s.Code, i)+"]", "stop code", s.Code)
	}
	for i, t := range doc.Terminals {
		check("terminals["+label(t.Key, i)+"]", "terminal key", t.Key)
	}
	names := make([]string, 0, len(doc.Messages))
	for name := range doc.Messages {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		check("messages["+name+"]", "message key", name)
	}
	for i, s := range doc.Steps {
		path := "steps[" + label(s.Key, i) + "]"
		check(path, "step key", s.Key)
		for j, t := range s.Tasks {
			taskPath := path + ".tasks[" + label(t.Key, j) + "]"
			check(taskPath, "task key", t.Key)
			if t.Requires != nil {
				for k, f := range t.Requires.Files {
					check(taskPath+".requires.files["+fmt.Sprint(k)+"]", "file slot", f.Slot)
				}
			}
			for k, in := range t.Inputs {
				if in.Slot != "" {
					check(taskPath+".inputs["+fmt.Sprint(k)+"]", "input slot", in.Slot)
				}
			}
		}
		for j, e := range s.Edges {
			check(path+".edges["+label(e.ID, j)+"]", "edge id", e.ID)
		}
	}
	return issues
}

// checkTaskVars checks what one task says about its own variables: a task can
// only require a variable that it is allowed to set.
func checkTaskVars(doc FlowDoc) []Issue {
	var issues []Issue
	for _, s := range doc.Steps {
		for _, t := range s.Tasks {
			if t.Requires == nil {
				continue
			}
			for _, v := range t.Requires.Vars {
				if !inStrings(t.Sets, v) {
					issues = append(issues, Issue{
						Code: CodeVarSetInvalid, Severity: SeverityError,
						Path:    "steps[" + s.Key + "].tasks[" + t.Key + "].requires.vars",
						Message: fmt.Sprintf("task %q requires the variable %q but does not list it in sets", t.Key, v),
						Params:  map[string]any{"task": t.Key, "var": v},
					})
				}
			}
		}
	}
	return issues
}
