package issueworkflow

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// The sources a fact can come from (spec section 2.6). There are no others: a
// condition never reads text, a comment or a file.
const (
	// FactSourceEngine: the engine counts it from the state and the task rows.
	FactSourceEngine = "engine"
	// FactSourceRequest: a column of the request issue.
	FactSourceRequest = "request"
	// FactSourceVar: a variable that a person or task set through the API.
	FactSourceVar = "var"
)

// FactTypeByVar is the Type of the var.<key> entry of the registry: the type of
// the fact is the Type of the Var it names.
const FactTypeByVar = "var"

// Operators beyond the ones transitions.go defines.
const OpContains = "contains"

// Legacy schema 1 field names. A schema 2 document that uses one is rejected
// with CodeLegacyFieldInV2: Lift rewrites them, authors do not write them.
const CodeLegacyFieldInV2 = "legacy_field_in_v2"

// Codes of a condition that cannot be evaluated.
const (
	CodeUnknownFact      = "unknown_fact"
	CodeOpNotAllowed     = "op_not_allowed"
	CodeConditionInvalid = "condition_invalid"
)

// FactSpec is one fact an expression may read.
type FactSpec struct {
	// Key is the fact name. <key> stands for a key of the document: a task, a
	// loop, a step or a variable.
	Key    string
	Type   string
	Source string
	Ops    []string
	// Values lists the allowed values of an enum fact.
	Values []string
}

var (
	opsEquality = []string{OpEq, OpNeq, OpIn}
	opsCounter  = []string{OpEq, OpNeq, OpGt, OpGte, OpLt, OpLte}
	opsString   = []string{OpEq, OpNeq, OpIn, OpExists}
	opsList     = []string{OpContains, OpExists}

	joinResults = []string{"pass", "fail", "block", "none"}
	priorities  = []string{"none", "low", "medium", "high", "urgent"}
)

// FactRegistry is every fact an expression can read, with its type, operators
// and source (spec section 2.6). An expression that names anything else is an
// error.
var FactRegistry = []FactSpec{
	{Key: "join.result", Type: VarTypeEnum, Source: FactSourceEngine, Ops: opsEquality, Values: joinResults},
	{Key: "join.pass", Type: VarTypeInt, Source: FactSourceEngine, Ops: opsCounter},
	{Key: "join.fail", Type: VarTypeInt, Source: FactSourceEngine, Ops: opsCounter},
	{Key: "join.block", Type: VarTypeInt, Source: FactSourceEngine, Ops: opsCounter},
	{Key: "join.neutral", Type: VarTypeInt, Source: FactSourceEngine, Ops: opsCounter},
	{Key: "join.open", Type: VarTypeInt, Source: FactSourceEngine, Ops: opsCounter},
	{Key: "join.total", Type: VarTypeInt, Source: FactSourceEngine, Ops: opsCounter},
	{Key: "task.<key>.verdict", Type: VarTypeString, Source: FactSourceEngine, Ops: opsString},
	{Key: "task.<key>.pass", Type: VarTypeInt, Source: FactSourceEngine, Ops: opsCounter},
	{Key: "task.<key>.fail", Type: VarTypeInt, Source: FactSourceEngine, Ops: opsCounter},
	{Key: "task.<key>.block", Type: VarTypeInt, Source: FactSourceEngine, Ops: opsCounter},
	{Key: "task.<key>.neutral", Type: VarTypeInt, Source: FactSourceEngine, Ops: opsCounter},
	{Key: "lead.verdict", Type: VarTypeString, Source: FactSourceEngine, Ops: opsEquality},
	{Key: "loop.<key>.count", Type: VarTypeInt, Source: FactSourceEngine, Ops: opsCounter},
	{Key: "step.<key>.activations", Type: VarTypeInt, Source: FactSourceEngine, Ops: opsCounter},
	{Key: "flow.activations", Type: VarTypeInt, Source: FactSourceEngine, Ops: opsCounter},
	{Key: "request.priority", Type: VarTypeEnum, Source: FactSourceRequest,
		Ops: []string{OpEq, OpNeq, OpIn, OpGt, OpGte, OpLt, OpLte}, Values: priorities},
	{Key: "request.labels", Type: VarTypeListString, Source: FactSourceRequest, Ops: opsList},
	{Key: "request.creator_type", Type: VarTypeEnum, Source: FactSourceRequest, Ops: opsEquality, Values: []string{"member", "agent"}},
	{Key: "request.project_id", Type: VarTypeString, Source: FactSourceRequest, Ops: opsString},
	{Key: "var.<key>", Type: FactTypeByVar, Source: FactSourceVar},
}

// opsOfVar are the operators of a variable of the type (spec section 2.6).
func opsOfVar(t string) []string {
	switch t {
	case VarTypeBool:
		return []string{OpEq, OpNeq}
	case VarTypeEnum:
		return opsEquality
	case VarTypeInt:
		return []string{OpEq, OpNeq, OpGt, OpGte, OpLt, OpLte, OpIn}
	case VarTypeString:
		return opsString
	case VarTypeListString:
		return opsList
	}
	return nil
}

// specOfVar is the fact for a declared variable.
func specOfVar(v Var) FactSpec {
	return FactSpec{Key: "var." + v.Key, Type: v.Type, Source: FactSourceVar, Ops: opsOfVar(v.Type), Values: v.Values}
}

func isLegacyFact(field string) bool {
	switch field {
	case FieldTasksOutcome, FieldReworkCount, FieldOutcome:
		return true
	}
	return strings.HasPrefix(field, FieldSwitchPrefix)
}

// LookupFact resolves a field name of an expression to its spec. vars are the
// document's variables, which decide the type of a var.<key> fact. A schema 1
// name gets CodeLegacyFieldInV2 and any other name outside the registry gets
// CodeUnknownFact.
func LookupFact(field string, vars []Var) (FactSpec, *Issue) {
	if isLegacyFact(field) {
		return FactSpec{}, &Issue{Code: CodeLegacyFieldInV2, Severity: SeverityError, Message: fmt.Sprintf("%q is a schema 1 field; schema 2 reads the facts of the fact registry", field), Params: map[string]any{"field": field}}
	}
	parts := strings.Split(field, ".")
	for _, spec := range FactRegistry {
		pattern := strings.Split(spec.Key, ".")
		if len(pattern) != len(parts) {
			continue
		}
		var key string
		match := true
		for i := range pattern {
			switch {
			case pattern[i] == "<key>":
				if !docKeyPattern.MatchString(parts[i]) {
					match = false
				}
				key = parts[i]
			case pattern[i] != parts[i]:
				match = false
			}
		}
		if !match {
			continue
		}
		if spec.Source != FactSourceVar {
			spec.Key = field
			return spec, nil
		}
		for _, v := range vars {
			if v.Key == key {
				return specOfVar(v), nil
			}
		}
		break
	}
	return FactSpec{}, &Issue{Code: CodeUnknownFact, Severity: SeverityError, Message: fmt.Sprintf("%q is not a fact", field), Params: map[string]any{"field": field}}
}

// ExpandFacts lists the concrete facts a document can have: the fixed ones, and
// the task, loop, step and variable facts for each key the document defines.
func ExpandFacts(doc FlowDoc) []FactSpec {
	var out []FactSpec
	for _, spec := range FactRegistry {
		switch {
		case spec.Source == FactSourceVar:
			for _, v := range doc.Vars {
				out = append(out, specOfVar(v))
			}
		case strings.Contains(spec.Key, "<key>"):
			var keys []string
			switch {
			case strings.HasPrefix(spec.Key, "task."):
				seen := map[string]bool{}
				for _, s := range doc.Steps {
					for _, t := range s.Tasks {
						if !seen[t.Key] {
							seen[t.Key] = true
							keys = append(keys, t.Key)
						}
					}
				}
			case strings.HasPrefix(spec.Key, "loop."):
				for _, l := range doc.Loops {
					keys = append(keys, l.Key)
				}
			case strings.HasPrefix(spec.Key, "step."):
				for _, s := range doc.Steps {
					keys = append(keys, s.Key)
				}
			}
			for _, key := range keys {
				concrete := spec
				concrete.Key = strings.Replace(spec.Key, "<key>", key, 1)
				out = append(out, concrete)
			}
		default:
			out = append(out, spec)
		}
	}
	return out
}

// FactValue is the value of a fact. The fact's type says which field is read:
// Bool for bool, Int for int, Str for string and enum, List for list_string.
type FactValue struct {
	Bool bool
	Int  int
	Str  string
	List []string
}

// BoolValue, IntValue, StringValue and ListValue build the value of a fact.
func BoolValue(b bool) FactValue         { return FactValue{Bool: b} }
func IntValue(n int) FactValue           { return FactValue{Int: n} }
func StringValue(s string) FactValue     { return FactValue{Str: s} }
func ListValue(items []string) FactValue { return FactValue{List: items} }

// FactSet is the facts one decision can read: the facts that are declared, and
// the values the ones that have any. A declared fact without a value has not
// happened yet (a task that did not open), which is not the same as a field the
// document has no fact for.
type FactSet struct {
	specs  map[string]FactSpec
	values map[string]FactValue
	// shared: specs belongs to a Graph and is copied before it is written.
	shared bool
}

// Clone returns a copy that can be given values without changing the original.
func (fs FactSet) Clone() FactSet {
	out := FactSet{specs: fs.specs, shared: true, values: make(map[string]FactValue, len(fs.values)+8)}
	for k, v := range fs.values {
		out.values[k] = v
	}
	return out
}

// Declare adds a fact to the set. It has no value until Set gives it one.
func (fs *FactSet) Declare(spec FactSpec) {
	if fs.specs == nil || fs.shared {
		copied := make(map[string]FactSpec, len(fs.specs)+1)
		for k, v := range fs.specs {
			copied[k] = v
		}
		fs.specs, fs.shared = copied, false
	}
	fs.specs[spec.Key] = spec
}

// Set gives a declared fact its value.
func (fs *FactSet) Set(key string, v FactValue) error {
	if _, ok := fs.specs[key]; !ok {
		return &ExprError{Code: CodeUnknownFact, Field: key, Message: fmt.Sprintf("%q is not a declared fact", key)}
	}
	if fs.values == nil {
		fs.values = map[string]FactValue{}
	}
	fs.values[key] = v
	return nil
}

// Spec returns the declaration of a fact.
func (fs FactSet) Spec(key string) (FactSpec, bool) {
	spec, ok := fs.specs[key]
	return spec, ok
}

// Value returns the value of a fact that has one.
func (fs FactSet) Value(key string) (FactValue, bool) {
	v, ok := fs.values[key]
	return v, ok
}

// Keys lists the declared facts in order.
func (fs FactSet) Keys() []string {
	keys := make([]string, 0, len(fs.specs))
	for k := range fs.specs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// RequestFacts are the columns of the request issue that conditions can read.
type RequestFacts struct {
	Priority    string
	Labels      []string
	CreatorType string
	ProjectID   string
}

// BuildFacts builds the fact set of a request from what the server holds: the
// document, the stored state, the task rows and the request issue. It takes no
// text from a person or an agent. The facts that depend on the join algorithm
// (join.result) and on what the lead reported (lead.verdict) are declared and
// left without a value for the decision to set.
//
// The join.* counts cover the tasks of the entry into the current step. A
// task.<key>.* fact reads the latest entry into the step that holds the task.
func BuildFacts(doc FlowDoc, state State, tasks []TaskState, req RequestFacts) FactSet {
	var fs FactSet
	for _, spec := range ExpandFacts(doc) {
		fs.Declare(spec)
	}
	return buildFacts(doc, fs, state, tasks, req, currentRound(state))
}

// BuildGraphFacts is BuildFacts for a graph: the tasks of the current entry are
// the ones of the round the graph runs the step in (the activation, or the
// schema 1 round of a compat v1 graph). It reuses the fact declarations the graph
// computed once.
func BuildGraphFacts(g Graph, state State, tasks []TaskState, req RequestFacts) FactSet {
	fs := FactSet{specs: g.factSpecs, shared: true}
	return buildFacts(g.Doc, fs, state, tasks, req, entryRound(g, state))
}

func buildFacts(doc FlowDoc, fs FactSet, state State, tasks []TaskState, req RequestFacts, entry int) FactSet {
	set := func(key string, v FactValue) { _ = fs.Set(key, v) }

	total := 0
	for _, step := range doc.Steps {
		n := state.Activations[step.Key]
		total += n
		set("step."+step.Key+".activations", IntValue(n))
	}
	set("flow.activations", IntValue(total))
	for _, loop := range doc.Loops {
		set("loop."+loop.Key+".count", IntValue(state.Loops[loop.Key]))
	}

	for _, v := range doc.Vars {
		raw, ok := state.Vars[v.Key]
		if !ok {
			raw = v.Default
		}
		if value, ok := varFactValue(v, raw); ok {
			set("var."+v.Key, value)
		}
	}

	priority := req.Priority
	if priority == "" {
		priority = "none"
	}
	set("request.priority", StringValue(priority))
	set("request.labels", ListValue(req.Labels))
	if req.CreatorType != "" {
		set("request.creator_type", StringValue(req.CreatorType))
	}
	if req.ProjectID != "" {
		set("request.project_id", StringValue(req.ProjectID))
	}

	rows := map[string][]TaskState{}
	latest := map[string]int{}
	for _, ts := range tasks {
		rows[ts.Step] = append(rows[ts.Step], ts)
		if r, seen := latest[ts.Step]; !seen || ts.Round > r {
			latest[ts.Step] = ts.Round
		}
	}
	for _, step := range doc.Steps {
		round, ok := latest[step.Key]
		if step.Key == state.Step {
			round, ok = entry, true
		}
		if !ok {
			continue
		}
		var counts classCounts
		perTask := map[string]*classCounts{}
		for _, ts := range rows[step.Key] {
			if ts.Round != round {
				continue
			}
			class, closed, counted := docTaskClass(doc, step, ts)
			counts.add(class, closed, counted)
			key := instanceBaseKey(ts.Key)
			pt := perTask[key]
			if pt == nil {
				pt = &classCounts{}
				perTask[key] = pt
			}
			pt.add(class, closed, counted)
			if counted && ts.Outcome != "" {
				pt.verdict, pt.verdicts = ts.Outcome, pt.verdicts+1
			}
		}
		if step.Key == state.Step {
			set("join.pass", IntValue(counts.pass))
			set("join.fail", IntValue(counts.fail))
			set("join.block", IntValue(counts.block))
			set("join.neutral", IntValue(counts.neutral))
			set("join.open", IntValue(counts.open))
			set("join.total", IntValue(counts.total))
		}
		for _, task := range step.Tasks {
			pt, opened := perTask[task.Key]
			if !opened {
				continue
			}
			prefix := "task." + task.Key + "."
			set(prefix+"pass", IntValue(pt.pass))
			set(prefix+"fail", IntValue(pt.fail))
			set(prefix+"block", IntValue(pt.block))
			set(prefix+"neutral", IntValue(pt.neutral))
			if pt.total == 1 && pt.verdicts == 1 {
				set(prefix+"verdict", StringValue(pt.verdict))
			}
		}
	}
	return fs
}

// currentRound is the flow_round of the tasks of the current entry into the
// step: the number of entries before this one.
func currentRound(state State) int {
	if n := state.Activations[state.Step]; n > 0 {
		return n - 1
	}
	return 0
}

type classCounts struct {
	pass, fail, block, neutral, open, total int
	verdict                                 string
	verdicts                                int
}

func (c *classCounts) add(class string, closed, counted bool) {
	if !counted {
		return
	}
	c.total++
	if class == ClassBlock {
		c.block++
		return
	}
	if !closed {
		c.open++
		return
	}
	switch class {
	case ClassPass:
		c.pass++
	case ClassFail:
		c.fail++
	case ClassBlock:
		c.block++
	case ClassNeutral:
		c.neutral++
	}
}

// instanceBaseKey strips the instance suffix of a task key: docs~2 is the
// second instance of docs.
func instanceBaseKey(key string) string {
	base, suffix, found := strings.Cut(key, "~")
	if !found {
		return key
	}
	if _, err := strconv.Atoi(suffix); err != nil {
		return key
	}
	return base
}

// docTaskClass says how a task row counts toward a join (spec section 4.3).
// closed is false for a task that is still open; counted is false for a row the
// join ignores: a task the engine superseded, or work a person cancelled.
func docTaskClass(doc FlowDoc, step DocStep, ts TaskState) (class string, closed, counted bool) {
	if ts.Outcome == OutcomeSuperseded {
		return "", false, false
	}
	// A task that reported blocked has a verdict even while its issue is open.
	if ts.Outcome == OutcomeBlocked {
		return ClassBlock, ts.Closed, true
	}
	if !ts.Closed {
		return "", false, true
	}
	kind := ts.Kind
	var verdicts string
	base := instanceBaseKey(ts.Key)
	for _, t := range step.Tasks {
		if t.Key == base {
			kind, verdicts = t.Kind, t.Verdicts
		}
	}
	switch {
	case ts.Cancelled:
		if kind == KindGate {
			return ClassFail, true, true
		}
		return "", true, false
	case ts.Outcome == "":
		if kind == KindGate {
			return ClassFail, true, true
		}
		return ClassPass, true, true
	}
	for _, v := range verdictsOf(doc, kind, verdicts) {
		if v.Key == ts.Outcome {
			return v.Class, true, true
		}
	}
	return ClassFail, true, true
}

// The verdict sets every document has without declaring them.
const (
	VerdictSetWork = "work"
	VerdictSetGate = "gate"
	VerdictSetPlan = "plan"
)

// builtinVerdicts are the sets a document can name without declaring them.
var builtinVerdicts = map[string][]Verdict{
	VerdictSetWork: {{Key: OutcomeDone, Class: ClassPass}},
	VerdictSetGate: {{Key: OutcomeApproved, Class: ClassPass}, {Key: OutcomeRejected, Class: ClassFail}},
	VerdictSetPlan: {{Key: "planned", Class: ClassPass}},
}

// verdictsOf is the verdict set a task reports in: the one it names, or the
// built-in set of its kind. A declared set wins over a built-in of the same name.
func verdictsOf(doc FlowDoc, kind, key string) []Verdict {
	if key == "" {
		if kind == KindGate {
			key = VerdictSetGate
		} else {
			key = VerdictSetWork
		}
	}
	for _, set := range doc.VerdictSets {
		if set.Key == key {
			return set.Verdicts
		}
	}
	return builtinVerdicts[key]
}

// VerdictsOf is the verdict set a task reports in: the one it names, or the built-in set
// of its kind (spec 2.3).
func VerdictsOf(doc FlowDoc, task DocTask) []Verdict {
	return verdictsOf(doc, task.Kind, task.Verdicts)
}

// varFactValue turns the stored value of a variable into a fact value. A value
// of the wrong type for the variable has no fact.
func varFactValue(v Var, raw any) (FactValue, bool) {
	switch v.Type {
	case VarTypeBool:
		b, ok := raw.(bool)
		return BoolValue(b), ok
	case VarTypeEnum, VarTypeString:
		s, ok := raw.(string)
		return StringValue(s), ok
	case VarTypeInt:
		n, ok := asNumber(raw)
		if !ok || n != float64(int(n)) {
			return FactValue{}, false
		}
		return IntValue(int(n)), true
	case VarTypeListString:
		switch list := raw.(type) {
		case []string:
			return ListValue(append([]string(nil), list...)), true
		case []any:
			out := make([]string, 0, len(list))
			for _, item := range list {
				s, ok := item.(string)
				if !ok {
					return FactValue{}, false
				}
				out = append(out, s)
			}
			return ListValue(out), true
		case nil:
			return ListValue(nil), true
		}
	}
	return FactValue{}, false
}
