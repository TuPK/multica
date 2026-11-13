package issueworkflow

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/multica-ai/multica/server/internal/issuestatus"
)

// The named validator of schema 2 (docs/engineering/flow-engine-generalization-spec.md
// section 3). ValidateFlowDoc reads a parsed document and the roster it will run
// with and returns every problem at once; nothing stops at the first. It is
// pure: no database, no clock. A caller maps the codes to HTTP errors and to
// translated text; Message is English for logs and the CLI.
//
// Each issue has a Code, a Severity, a Path made of keys and not positions
// (steps[gate].tasks[docs_gate].role), and Params with stable identifiers.
// Params["rule"] holds the code of the spec table (S01 to Y05). The Code is the
// snake_case name the table gives the rule, or the name chosen here where the
// table has none.

// Roster is who the squad has. Plain data: the caller reads it from the database.
type Roster struct {
	// Lead is the id of the squad's lead. The lead plans and does not take tasks.
	Lead string
	// Agents are the ids of the agent members of the squad.
	Agents []string
	// Archived are agent ids that can no longer take work.
	Archived map[string]bool
	// AgentSkills lists the skills each agent has, attached directly or through
	// the squad.
	AgentSkills map[string][]string
	// RoleKeys maps an agent id to the key of the role a schema 1 manifest gave
	// it. Only ValidateSchema1ForSquad reads it, to keep the manifest's keys.
	RoleKeys map[string]string
}

// StatusInfo is a board status of the workspace.
type StatusInfo struct {
	// Category is unstarted, started, done or closed.
	Category string
	Archived bool
}

// StatusCatalog maps a status key to its status.
type StatusCatalog map[string]StatusInfo

// DefaultStatusCatalog is the seven canonical statuses every workspace has.
func DefaultStatusCatalog() StatusCatalog {
	return StatusCatalog{
		issuestatus.Backlog:    {Category: issuestatus.CategoryUnstarted},
		issuestatus.Todo:       {Category: issuestatus.CategoryUnstarted},
		issuestatus.InProgress: {Category: issuestatus.CategoryStarted},
		issuestatus.InReview:   {Category: issuestatus.CategoryStarted},
		issuestatus.Blocked:    {Category: issuestatus.CategoryStarted},
		issuestatus.Done:       {Category: issuestatus.CategoryDone},
		issuestatus.Cancelled:  {Category: issuestatus.CategoryClosed},
	}
}

// ValidateContext is what a document is checked against besides itself.
type ValidateContext struct {
	Roster Roster
	// Statuses is the board's status catalog. When nil the checks that need it
	// (S05, S08) are skipped.
	Statuses StatusCatalog
	// WorkspaceMembers are the member ids of the workspace; a member bound to a
	// role must be one. When nil the member check is skipped.
	WorkspaceMembers map[string]bool
	// CanCreateStatuses: the actor is an admin and may create a status by name.
	CanCreateStatuses bool
	// Publish adds the checks that only matter when a draft is published: every
	// role is bound (R02) and its agents have the skills (R09).
	Publish bool
}

// The ack kinds a Policy can carry.
const (
	AckLeadMayWork         = "lead_may_work"
	AckUnattendedRelease   = "unattended_release"
	AckNoGate              = "no_gate"
	ackParamRule           = "rule"
	messagePlaceholderList = "requester, request, step, rework, tasks, missing, reason, loop, stop"
)

var messagePlaceholders = map[string]bool{
	"requester": true, "request": true, "step": true, "rework": true, "tasks": true,
	"missing": true, "reason": true, "loop": true, "stop": true,
}

var (
	placeholderPattern = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	skillPattern       = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	extPattern         = regexp.MustCompile(`^\.[a-z0-9]+$`)
)

// Text limits of the document (spec 2.3).
const (
	docMaxLabelRunes       = 80
	docMaxStepTitleRunes   = 64
	docMaxTaskTitleRunes   = 80
	docMaxEdgeLabelRunes   = 64
	docMaxVarStringRunes   = 200
	docMaxVarListItems     = 20
	docMaxEnumValues       = 16
	docMaxVerdictsPerSet   = 8
	docMaxFileMin          = 10
	docMaxExtensions       = 8
	docMaxDeadlineMinutes  = 60 * 24 * 30
	docMaxDeadlineAttempts = 10
)

// ValidateFlowDocJSON parses a document strictly and, when it parses, validates
// it. The issues of the parse carry the rule they belong to (S01 to S03).
func ValidateFlowDocJSON(raw []byte, ctx ValidateContext) []Issue {
	doc, issues := ParseFlowDoc(raw)
	if len(issues) > 0 {
		for i := range issues {
			rule := "S01"
			switch issues[i].Code {
			case CodeFlowDocTooLarge:
				rule = "S02"
			case CodeKeyInvalid:
				rule = "S03"
			case CodeVarSetInvalid:
				rule = "T08"
			}
			p := map[string]any{ackParamRule: rule}
			for k, v := range issues[i].Params {
				p[k] = v
			}
			issues[i].Params = p
		}
		return issues
	}
	return ValidateFlowDoc(doc, ctx)
}

// ValidateFlowDoc checks a parsed document against the rules of spec section 3.
// The order of the issues is the order of the groups S, G, R, T, J, F, P, Y and,
// inside a group, the order of the document.
func ValidateFlowDoc(doc FlowDoc, ctx ValidateContext) []Issue {
	v := newValidator(doc, ctx)
	v.checkStructure()
	v.checkGraph()
	v.checkRoles()
	v.checkTasks()
	v.checkJoins()
	v.checkFacts()
	v.checkMessagesAndPolicy()
	v.checkRelease()
	return v.issues
}

// HasErrors reports whether any issue is an error.
func HasErrors(issues []Issue) bool {
	for _, i := range issues {
		if i.Severity == SeverityError {
			return true
		}
	}
	return false
}

type validator struct {
	doc    FlowDoc
	ctx    ValidateContext
	g      Graph
	issues []Issue

	steps     map[string]DocStep
	terminals map[string]Terminal
	roles     map[string]Role
	vars      map[string]Var
	sets      map[string]VerdictSet
	loops     map[string]Loop
	stops     map[string]StopKind
	// tasks maps "step.task" to the task; taskStep maps it to its step key.
	tasks map[string]DocTask
	// succ are the steps each step leads to by any edge; fwd the ones without a back edge.
	succ map[string][]string
	fwd  map[string][]string
}

func newValidator(doc FlowDoc, ctx ValidateContext) *validator {
	v := &validator{
		doc: doc, ctx: ctx, g: NewGraph(doc),
		steps: map[string]DocStep{}, terminals: map[string]Terminal{}, roles: map[string]Role{},
		vars: map[string]Var{}, sets: map[string]VerdictSet{}, loops: map[string]Loop{}, stops: map[string]StopKind{},
		tasks: map[string]DocTask{}, succ: map[string][]string{}, fwd: map[string][]string{},
	}
	for _, s := range doc.Steps {
		if _, dup := v.steps[s.Key]; !dup {
			v.steps[s.Key] = s
		}
		for _, t := range s.Tasks {
			if _, dup := v.tasks[s.Key+"."+t.Key]; !dup {
				v.tasks[s.Key+"."+t.Key] = t
			}
		}
		for _, e := range s.Edges {
			if _, ok := v.stepByKey(doc, e.To); e.To != "" && ok {
				v.succ[s.Key] = append(v.succ[s.Key], e.To)
				if !e.IsBack() {
					v.fwd[s.Key] = append(v.fwd[s.Key], e.To)
				}
			}
		}
	}
	for _, t := range doc.Terminals {
		if _, dup := v.terminals[t.Key]; !dup {
			v.terminals[t.Key] = t
		}
	}
	for _, r := range doc.Roles {
		if _, dup := v.roles[r.Key]; !dup {
			v.roles[r.Key] = r
		}
	}
	for _, x := range doc.Vars {
		if _, dup := v.vars[x.Key]; !dup {
			v.vars[x.Key] = x
		}
	}
	for _, x := range doc.VerdictSets {
		if _, dup := v.sets[x.Key]; !dup {
			v.sets[x.Key] = x
		}
	}
	for _, x := range doc.Loops {
		if _, dup := v.loops[x.Key]; !dup {
			v.loops[x.Key] = x
		}
	}
	for _, x := range doc.Stops {
		if _, dup := v.stops[x.Code]; !dup {
			v.stops[x.Code] = x
		}
	}
	return v
}

func (v *validator) stepByKey(doc FlowDoc, key string) (DocStep, bool) {
	for _, s := range doc.Steps {
		if s.Key == key {
			return s, true
		}
	}
	return DocStep{}, false
}

// add records an issue. params may be nil.
func (v *validator) add(rule, code, severity, path, message string, params map[string]any) *Issue {
	p := map[string]any{ackParamRule: rule}
	for k, x := range params {
		p[k] = x
	}
	v.issues = append(v.issues, Issue{Code: code, Severity: severity, Path: path, Message: message, Params: p})
	return &v.issues[len(v.issues)-1]
}

func (v *validator) err(rule, code, path, format string, args ...any) {
	v.add(rule, code, SeverityError, path, fmt.Sprintf(format, args...), nil)
}

func (v *validator) errp(rule, code, path string, params map[string]any, format string, args ...any) {
	v.add(rule, code, SeverityError, path, fmt.Sprintf(format, args...), params)
}

func (v *validator) warn(rule, code, path string, params map[string]any, format string, args ...any) *Issue {
	return v.add(rule, code, SeverityWarning, path, fmt.Sprintf(format, args...), params)
}

// label names a list element by its key, or by its position when it has none.
func label(key string, index int) string {
	if key != "" {
		return key
	}
	return fmt.Sprintf("#%d", index)
}

func stepPath(key string) string        { return "steps[" + key + "]" }
func taskPath(step, task string) string { return stepPath(step) + ".tasks[" + task + "]" }
func edgePath(step, id string) string   { return stepPath(step) + ".edges[" + id + "]" }
func rolePath(key string) string        { return "roles[" + key + "]" }
func varPath(key string) string         { return "vars[" + key + "]" }
func loopPath(key string) string        { return "loops[" + key + "]" }
func stopPath(code string) string       { return "stops[" + code + "]" }
func terminalPath(key string) string    { return "terminals[" + key + "]" }
func setPath(key string) string         { return "verdicts[" + key + "]" }
func messagePath(key string) string     { return "messages[" + key + "]" }
func (v *validator) isBuiltinRole(k string) bool {
	return k == RoleKindLead || k == RoleKindRequester || k == RoleKindManagers
}

// roleKind is the kind of a role: a declared one or a built-in one.
func (v *validator) roleKind(key string) string {
	if r, ok := v.roles[key]; ok {
		return r.Kind
	}
	if v.isBuiltinRole(key) {
		return key
	}
	return ""
}

func isHumanKind(kind string) bool {
	return kind == RoleKindMember || kind == RoleKindManagers || kind == RoleKindRequester
}

// statusKeyOf is the board status key a step shows: its key, or the key made
// from the name of a new status.
func statusKeyOf(s StepStatus) string {
	if s.Key != "" {
		return s.Key
	}
	if s.Name == "" {
		return ""
	}
	var b strings.Builder
	lastUnderscore := true
	for _, r := range strings.ToLower(s.Name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastUnderscore = false
		} else if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

func runes(s string) int { return utf8.RuneCountInString(s) }

// ---------------------------------------------------------------- S: structure

func (v *validator) checkStructure() {
	d := v.doc
	if d.Version > FlowDocVersion {
		v.err("S01", CodeFlowSchemaUnsupported, "version", "version %d is newer than this server understands (%d)", d.Version, FlowDocVersion)
	} else if d.Version != FlowDocVersion {
		v.err("S01", CodeFlowDocInvalid, "version", "version must be %d", FlowDocVersion)
	}

	// S02: counts and caps.
	limit := func(path, what string, n, max int) {
		if n > max {
			v.errp("S02", "limit_exceeded", path, map[string]any{"limit": what, "max": max, "actual": n}, "%s: %d is more than the limit of %d", what, n, max)
		}
	}
	limit("steps", "steps", len(d.Steps), DocMaxSteps)
	limit("roles", "roles", len(d.Roles), DocMaxRoles)
	limit("vars", "vars", len(d.Vars), DocMaxVars)
	limit("verdicts", "verdict sets", len(d.VerdictSets), DocMaxVerdictSets)
	limit("loops", "loops", len(d.Loops), DocMaxLoops)
	for _, set := range d.VerdictSets {
		limit(setPath(set.Key), "verdicts in a set", len(set.Verdicts), docMaxVerdictsPerSet)
	}
	for _, l := range d.Loops {
		if max := l.Max.Limit(); max < 1 || max > DocMaxLoopMax {
			v.errp("S02", "limit_exceeded", loopPath(l.Key)+".max", map[string]any{"limit": "loop max", "max": DocMaxLoopMax, "actual": max}, "loop %q: max must be 1 to %d", l.Key, DocMaxLoopMax)
		}
	}
	for _, s := range d.Steps {
		limit(stepPath(s.Key)+".tasks", "tasks in a step", len(s.Tasks), DocMaxTasksPerStep)
		limit(stepPath(s.Key)+".edges", "edges in a step", len(s.Edges), DocMaxEdgesPerStep)
		for _, t := range s.Tasks {
			p := taskPath(s.Key, t.Key)
			limit(p+".inputs", "inputs of a task", len(t.Inputs), DocMaxInputsPerTask)
			limit(p+".sets", "variables a task sets", len(t.Sets), DocMaxSetsPerTask)
			limit(p+".skills", "skills of a task", len(t.Skills), DocMaxSkillsPerTask)
			if in := t.Instances; in != nil {
				if n := maxInstances(t); n > DocMaxInstances || (in.Count > DocMaxInstances) || in.Max > DocMaxInstances {
					v.errp("S02", "limit_exceeded", p+".instances", map[string]any{"limit": "instances", "max": DocMaxInstances, "actual": max(in.Count, in.Max)}, "instances: at most %d", DocMaxInstances)
				}
			}
			if t.When != nil {
				if depth := conditionDepth(*t.When); depth > DocMaxExprDepth {
					v.errp("S02", "limit_exceeded", p+".when", map[string]any{"limit": "expression depth", "max": DocMaxExprDepth, "actual": depth}, "conditions nest at most %d levels", DocMaxExprDepth)
				}
			}
		}
		for _, e := range s.Edges {
			if e.When != nil {
				if depth := conditionDepth(*e.When); depth > DocMaxExprDepth {
					v.errp("S02", "limit_exceeded", edgePath(s.Key, e.ID)+".when", map[string]any{"limit": "expression depth", "max": DocMaxExprDepth, "actual": depth}, "conditions nest at most %d levels", DocMaxExprDepth)
				}
			}
		}
	}
	if p := d.Policy; p.MaxTaskIssues > DocMaxTaskIssuesHard {
		v.errp("S02", "limit_exceeded", "policy.max_task_issues", map[string]any{"limit": "task issues", "max": DocMaxTaskIssuesHard, "actual": p.MaxTaskIssues}, "max_task_issues is at most %d", DocMaxTaskIssuesHard)
	}
	for name, n := range map[string]int{"task_idle_minutes": d.Policy.TaskIdleMinutes, "task_hard_cap_minutes": d.Policy.TaskHardCapMinutes,
		"task_max_attempts": d.Policy.TaskMaxAttempts, "human_remind_minutes": d.Policy.HumanRemindMinutes, "human_stall_minutes": d.Policy.HumanStallMinutes,
		"max_activations": d.Policy.MaxActivations, "max_task_issues": d.Policy.MaxTaskIssues} {
		if n < 0 {
			v.errp("S02", "limit_exceeded", "policy."+name, map[string]any{"limit": name, "max": 0, "actual": n}, "%s cannot be negative", name)
		}
	}

	v.checkKeys()
	v.checkStart()
	v.checkStatuses()
	v.checkTexts()
}

// conditionDepth is the number of levels of a condition.
func conditionDepth(c Condition) int {
	deepest := 0
	switch {
	case c.All != nil:
		for _, child := range c.All {
			deepest = max(deepest, conditionDepth(child))
		}
	case c.Any != nil:
		for _, child := range c.Any {
			deepest = max(deepest, conditionDepth(child))
		}
	case c.Not != nil:
		deepest = conditionDepth(*c.Not)
	}
	return deepest + 1
}

// checkKeys is S03: keys match the pattern and are unique in their set.
func (v *validator) checkKeys() {
	d := v.doc
	check := func(path, kind, key string, seen map[string]bool) {
		if !docKeyPattern.MatchString(key) {
			v.errp("S03", CodeKeyInvalid, path, map[string]any{"key": key, "pattern": docKeyPattern.String()}, "%s %q must match %s", kind, key, docKeyPattern.String())
			return
		}
		if seen[key] {
			v.errp("S03", "key_duplicate", path, map[string]any{"key": key, "kind": kind}, "%s %q is used more than once", kind, key)
		}
		seen[key] = true
	}
	seen := map[string]bool{}
	for i, r := range d.Roles {
		check(rolePath(label(r.Key, i)), "role key", r.Key, seen)
	}
	seen = map[string]bool{}
	for i, x := range d.Vars {
		check(varPath(label(x.Key, i)), "var key", x.Key, seen)
	}
	seen = map[string]bool{}
	for i, set := range d.VerdictSets {
		check(setPath(label(set.Key, i)), "verdict set key", set.Key, seen)
		inner := map[string]bool{}
		for j, vd := range set.Verdicts {
			check(setPath(label(set.Key, i))+".verdicts["+label(vd.Key, j)+"]", "verdict key", vd.Key, inner)
		}
	}
	seen = map[string]bool{}
	for i, l := range d.Loops {
		check(loopPath(label(l.Key, i)), "loop key", l.Key, seen)
	}
	seen = map[string]bool{}
	for i, s := range d.Stops {
		check(stopPath(label(s.Code, i)), "stop code", s.Code, seen)
	}
	// Steps and terminals share one namespace: an edge names either.
	seen = map[string]bool{}
	for i, t := range d.Terminals {
		check(terminalPath(label(t.Key, i)), "terminal key", t.Key, seen)
	}
	for i, s := range d.Steps {
		check(stepPath(label(s.Key, i)), "step key", s.Key, seen)
		inner := map[string]bool{}
		for j, t := range s.Tasks {
			check(taskPath(label(s.Key, i), label(t.Key, j)), "task key", t.Key, inner)
		}
		ids := map[string]bool{}
		for j, e := range s.Edges {
			check(edgePath(label(s.Key, i), label(e.ID, j)), "edge id", e.ID, ids)
		}
	}
	names := make([]string, 0, len(d.Messages))
	for k := range d.Messages {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		if !docKeyPattern.MatchString(k) {
			v.errp("S03", CodeKeyInvalid, messagePath(k), map[string]any{"key": k, "pattern": docKeyPattern.String()}, "message key %q must match %s", k, docKeyPattern.String())
		}
	}
}

// checkStart is S04.
func (v *validator) checkStart() {
	s, ok := v.steps[v.doc.Start]
	switch {
	case v.doc.Start == "" || !ok:
		v.errp("S04", "start_invalid", "start", map[string]any{"start": v.doc.Start}, "start %q is not a step", v.doc.Start)
	case s.Kind != StepKindLead && s.Kind != StepKindTasks:
		v.errp("S04", "start_invalid", "start", map[string]any{"start": v.doc.Start, "kind": s.Kind}, "the start step must be a lead or tasks step, not %q", s.Kind)
	}
	if len(v.doc.Terminals) == 0 {
		v.err("S04", "terminal_missing", "terminals", "a flow needs at least one terminal")
	}
}

// checkStatuses is S05 to S08.
func (v *validator) checkStatuses() {
	d := v.doc
	byStatus := map[string][]string{}
	var order []string
	for i, s := range d.Steps {
		p := stepPath(label(s.Key, i)) + ".status"
		key := statusKeyOf(s.Status)
		hasKey, hasName := s.Status.Key != "", s.Status.Name != "" || s.Status.Category != ""
		switch {
		case hasKey == hasName:
			v.err("S05", "status_invalid", p, "a step shows a status by key or by name and category, not both and not neither")
		case hasKey:
			if v.ctx.Statuses != nil {
				if info, ok := v.ctx.Statuses[s.Status.Key]; !ok || info.Archived {
					v.errp("S05", "status_invalid", p, map[string]any{"status": s.Status.Key}, "status %q does not exist or is archived", s.Status.Key)
				}
			}
		default:
			if s.Status.Name == "" || !validCategory(s.Status.Category) {
				v.err("S05", "status_invalid", p, "a new status needs a name and a category (unstarted, started, done, closed)")
			} else if !v.ctx.CanCreateStatuses {
				if _, exists := v.ctx.Statuses[key]; !exists {
					v.errp("S05", "status_create_forbidden", p, map[string]any{"status": s.Status.Name}, "creating the status %q needs an admin", s.Status.Name)
				}
			}
		}
		if key != "" {
			if _, seen := byStatus[key]; !seen {
				order = append(order, key)
			}
			byStatus[key] = append(byStatus[key], label(s.Key, i))
		}
	}
	if len(order) > DocMaxStatusNames {
		v.errp("S06", "too_many_statuses", "steps", map[string]any{"max": DocMaxStatusNames, "actual": len(order)}, "the steps show %d different statuses; the limit is %d", len(order), DocMaxStatusNames)
	}
	for _, key := range order {
		if steps := byStatus[key]; len(steps) > 1 {
			v.warn("S07", "status_shared", stepPath(steps[1])+".status", map[string]any{"status": key, "steps": steps},
				"steps %s show the status %q: dragging a card into it is ambiguous", strings.Join(steps, ", "), key)
		}
	}

	if v.ctx.Statuses == nil {
		return
	}
	for i, t := range d.Terminals {
		info, ok := v.ctx.Statuses[t.Status]
		if !ok || (info.Category != issuestatus.CategoryDone && info.Category != issuestatus.CategoryClosed) {
			v.errp("S08", "terminal_status_invalid", terminalPath(label(t.Key, i))+".status", map[string]any{"status": t.Status}, "terminal %q: status %q must be a done or closed status", t.Key, t.Status)
		}
	}
	for i, s := range d.Stops {
		if s.Status == "" {
			continue
		}
		info, ok := v.ctx.Statuses[s.Status]
		if !ok || (s.Status != issuestatus.Blocked && info.Category != issuestatus.CategoryStarted) {
			v.errp("S08", "stop_status_invalid", stopPath(label(s.Code, i))+".status", map[string]any{"status": s.Status}, "stop %q: status %q must be blocked or a started status", s.Code, s.Status)
		}
	}
}

func validCategory(c string) bool {
	switch c {
	case issuestatus.CategoryUnstarted, issuestatus.CategoryStarted, issuestatus.CategoryDone, issuestatus.CategoryClosed:
		return true
	}
	return false
}

// checkTexts is S09: titles, labels, instructions and messages by length.
func (v *validator) checkTexts() {
	d := v.doc
	long := func(path, what, text string, max int) {
		if n := runes(text); n > max {
			v.errp("S09", "text_too_long", path, map[string]any{"field": what, "max": max, "actual": n}, "%s is %d characters; the limit is %d", what, n, max)
		}
	}
	for i, r := range d.Roles {
		long(rolePath(label(r.Key, i))+".label", "role label", r.Label, docMaxLabelRunes)
	}
	for i, s := range d.Steps {
		sp := stepPath(label(s.Key, i))
		long(sp+".title", "step title", s.Title, docMaxStepTitleRunes)
		for j, t := range s.Tasks {
			tp := taskPath(label(s.Key, i), label(t.Key, j))
			if strings.TrimSpace(t.Title) == "" {
				v.errp("S09", "text_missing", tp+".title", map[string]any{"field": "task title"}, "task %q needs a title", t.Key)
			}
			long(tp+".title", "task title", t.Title, docMaxTaskTitleRunes)
			long(tp+".instructions", "instructions", t.Instructions, DocMaxInstructionsRunes)
		}
		for j, e := range s.Edges {
			ep := edgePath(label(s.Key, i), label(e.ID, j))
			if n := runes(e.Label); n < 1 || n > docMaxEdgeLabelRunes {
				v.errp("S09", "text_too_long", ep+".label", map[string]any{"field": "edge label", "max": docMaxEdgeLabelRunes, "actual": n}, "an edge label is 1 to %d characters", docMaxEdgeLabelRunes)
			}
		}
	}
	names := make([]string, 0, len(d.Messages))
	for k := range d.Messages {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		long(messagePath(k), "message", d.Messages[k], DocMaxMessageRunes)
	}
}

// ---------------------------------------------------------------- G: graph

// reachableFrom is the set of steps reachable from start over the given
// successors, start included, without passing through skip.
func reachableFrom(start string, next map[string][]string, skip string) map[string]bool {
	seen := map[string]bool{}
	if start == skip {
		return seen
	}
	seen[start] = true
	queue := []string{start}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, n := range next[cur] {
			if n != skip && !seen[n] {
				seen[n] = true
				queue = append(queue, n)
			}
		}
	}
	return seen
}

// dominates reports whether every path from the start step to target passes
// through step.
func (v *validator) dominates(step, target string) bool {
	if step == target || step == v.doc.Start {
		return true
	}
	return !reachableFrom(v.doc.Start, v.succ, step)[target]
}

func (v *validator) checkGraph() {
	d := v.doc
	declaredStop := func(code string) bool {
		_, ok := v.stops[code]
		return code == DocStopStalled || ok
	}
	for i, s := range d.Steps {
		sk := label(s.Key, i)
		for j, e := range s.Edges {
			ep := edgePath(sk, label(e.ID, j))
			// G01
			switch {
			case (e.To == "") == (e.Stop == ""):
				v.err("G01", "edge_target_invalid", ep, "an edge has exactly one of to and stop")
			case e.To != "":
				_, isStep := v.steps[e.To]
				_, isTerminal := v.terminals[e.To]
				if !isStep && !isTerminal {
					v.errp("G01", "edge_target_unknown", ep+".to", map[string]any{"to": e.To}, "edge %q leads to %q, which is no step or terminal", e.ID, e.To)
				}
			default:
				if !declaredStop(e.Stop) {
					v.errp("G01", "stop_unknown", ep+".stop", map[string]any{"stop": e.Stop}, "edge %q stops with %q, which is not declared", e.ID, e.Stop)
				}
			}
			// G09
			switch {
			case e.Trigger != "" && e.Trigger != EdgeTriggerAuto && e.Trigger != EdgeTriggerManual:
				v.errp("G09", "edge_trigger_invalid", ep+".trigger", map[string]any{"trigger": e.Trigger}, "trigger must be auto or manual")
			case e.Trigger == EdgeTriggerManual && s.Kind != StepKindManual:
				v.errp("G09", "edge_trigger_invalid", ep+".trigger", map[string]any{"trigger": e.Trigger}, "a manual edge belongs to a manual step")
			case s.Kind == StepKindManual && e.Trigger != EdgeTriggerManual:
				v.errp("G09", "edge_trigger_invalid", ep+".trigger", map[string]any{"trigger": e.Trigger}, "every edge of a manual step is a manual edge")
			}
		}
	}

	v.checkReachability()
	v.checkDeadSteps()
	v.checkCycles()
	v.checkLoops()
	v.checkLeadSteps()
	v.checkCost()
}

// checkReachability is G02 and G08.
func (v *validator) checkReachability() {
	if _, ok := v.steps[v.doc.Start]; !ok {
		return
	}
	reach := reachableFrom(v.doc.Start, v.succ, "")
	for i, s := range v.doc.Steps {
		if !reach[s.Key] {
			v.errp("G02", "step_unreachable", stepPath(label(s.Key, i)), map[string]any{"step": s.Key}, "step %q cannot be reached from %q", s.Key, v.doc.Start)
		}
	}
	live := false
	for key := range reach {
		for _, e := range v.steps[key].Edges {
			if _, ok := v.terminals[e.To]; ok && e.To != "" {
				live = true
			}
		}
	}
	if !live {
		v.err("G08", "no_live_path", "steps", "no path leads from the start to a terminal without a stop")
	}
}

// resultsCovered reports whether every value is matched by an edge: an edge
// with no condition, or one whose condition is a check of the field for it.
func resultsCovered(edges []DocEdge, field string, values []string) []string {
	var missing []string
	for _, val := range values {
		covered := false
		for _, e := range edges {
			if e.Trigger == EdgeTriggerManual {
				continue
			}
			if e.When == nil || conditionCovers(*e.When, field, val) {
				covered = true
				break
			}
		}
		if !covered {
			missing = append(missing, val)
		}
	}
	return missing
}

// conditionCovers reports whether the condition holds whenever field has value.
func conditionCovers(c Condition, field, value string) bool {
	switch {
	case c.All != nil:
		for _, child := range c.All {
			if !conditionCovers(child, field, value) {
				return false
			}
		}
		return len(c.All) > 0
	case c.Any != nil:
		for _, child := range c.Any {
			if conditionCovers(child, field, value) {
				return true
			}
		}
		return false
	case c.Not != nil:
		return false
	}
	if c.Field != field {
		return false
	}
	switch c.Op {
	case OpEq:
		return c.Value == value
	case OpIn:
		for _, item := range asList(c.Value) {
			if item == value {
				return true
			}
		}
	}
	return false
}

// checkDeadSteps is G03: every step can be left.
func (v *validator) checkDeadSteps() {
	for i, s := range v.doc.Steps {
		sp := stepPath(label(s.Key, i))
		if s.Kind == StepKindManual {
			manual := false
			for _, e := range s.Edges {
				manual = manual || e.Trigger == EdgeTriggerManual
			}
			if !manual {
				v.errp("G03", "dead_step", sp, map[string]any{"step": s.Key}, "manual step %q has no manual edge: a request would wait there forever", s.Key)
			}
			continue
		}
		defaults := 0
		for j, e := range s.Edges {
			if e.When == nil {
				defaults++
				if j != len(s.Edges)-1 {
					v.errp("G03", "default_edge_not_last", edgePath(label(s.Key, i), label(e.ID, j)), map[string]any{"edge": e.ID}, "the edge without a condition must come last")
				}
			}
		}
		if defaults > 1 {
			v.errp("G03", "default_edge_not_last", sp+".edges", map[string]any{"step": s.Key}, "a step has at most one edge without a condition")
		}
		if defaults > 0 {
			continue
		}
		var domain []string
		field := "join.result"
		if s.Kind == StepKindLead {
			field = "lead.verdict"
			domain = v.leadVerdicts(s)
		} else {
			domain = v.joinDomain(s)
		}
		if missing := resultsCovered(s.Edges, field, domain); len(missing) > 0 || len(s.Edges) == 0 {
			v.errp("G03", "dead_step", sp, map[string]any{"step": s.Key, "unrouted": missing}, "step %q has no edge for %s: a request could be stuck there", s.Key, strings.Join(missing, ", "))
		}
	}
}

// leadVerdicts are the verdicts a lead step can report.
func (v *validator) leadVerdicts(s DocStep) []string {
	set := VerdictSetPlan
	if s.Lead != nil && s.Lead.Verdicts != "" {
		set = s.Lead.Verdicts
	}
	out := []string{OutcomeBlocked}
	for _, vd := range v.verdictsOf(set) {
		out = append(out, vd.Key)
	}
	return out
}

// verdictsOf is the verdicts of a declared or built-in set.
func (v *validator) verdictsOf(key string) []Verdict {
	if set, ok := v.sets[key]; ok {
		return set.Verdicts
	}
	return builtinVerdicts[key]
}

func (v *validator) setKnown(key string) bool {
	if _, ok := v.sets[key]; ok {
		return true
	}
	_, ok := builtinVerdicts[key]
	return ok
}

// taskVerdicts is the verdict set a task reports in.
func (v *validator) taskVerdicts(t DocTask) []Verdict {
	key := t.Verdicts
	if key == "" {
		if t.Kind == KindGate {
			key = VerdictSetGate
		} else {
			key = VerdictSetWork
		}
	}
	return v.verdictsOf(key)
}

// overTasks are the tasks a step's join judges.
func (v *validator) overTasks(s DocStep) []DocTask {
	join := s.EffectiveJoin()
	var out []DocTask
	for _, t := range s.Tasks {
		if inOver(join.Over, t.Kind, t.Key) {
			out = append(out, t)
		}
	}
	return out
}

// joinDomain are the results the join of a tasks step can give (J02): block
// always; none when nothing is judged or an abstention is possible; pass and
// fail when a verdict of that class is possible.
func (v *validator) joinDomain(s DocStep) []string {
	over := v.overTasks(s)
	domain := map[string]bool{JoinBlock: true}
	if len(over) == 0 {
		domain[JoinNone] = true
	}
	allConditional := len(over) > 0
	for _, t := range over {
		if t.When == nil {
			allConditional = false
		}
		for _, vd := range v.taskVerdicts(t) {
			switch vd.Class {
			case ClassPass:
				domain[JoinPass] = true
			case ClassFail:
				domain[JoinFail] = true
			case ClassNeutral:
				domain[JoinNone] = true
			}
		}
	}
	if allConditional {
		domain[JoinNone] = true
	}
	var out []string
	for _, r := range []string{JoinPass, JoinFail, JoinBlock, JoinNone} {
		if domain[r] {
			out = append(out, r)
		}
	}
	return out
}

// checkCycles is G04: without its back edges the graph has no cycle.
func (v *validator) checkCycles() {
	state := map[string]int{}
	var onCycle []string
	var visit func(key string) bool
	visit = func(key string) bool {
		switch state[key] {
		case 1:
			onCycle = append(onCycle, key)
			return true
		case 2:
			return false
		}
		state[key] = 1
		for _, next := range v.fwd[key] {
			if visit(next) {
				return true
			}
		}
		state[key] = 2
		return false
	}
	for _, s := range v.doc.Steps {
		if visit(s.Key) {
			v.errp("G04", "cycle_without_loop", stepPath(onCycle[0]), map[string]any{"step": onCycle[0]}, "step %q is on a cycle that has no loop edge", onCycle[0])
			return
		}
	}
}

// checkLoops is G05 to G07.
func (v *validator) checkLoops() {
	for i, l := range v.doc.Loops {
		lp := loopPath(label(l.Key, i))
		_, headerOK := v.steps[l.Header]
		if !headerOK {
			v.errp("G06", "loop_invalid", lp+".header", map[string]any{"loop": l.Key, "header": l.Header}, "loop %q: the header %q is not a step", l.Key, l.Header)
		}
		backs := 0
		forwardIn := l.Header == v.doc.Start
		for _, s := range v.doc.Steps {
			for _, e := range s.Edges {
				if e.Loop == l.Key {
					backs++
				}
				if headerOK && !e.IsBack() && e.To == l.Header {
					forwardIn = true
				}
			}
		}
		if backs == 0 {
			v.errp("G06", "loop_invalid", lp, map[string]any{"loop": l.Key}, "loop %q has no edge that closes it", l.Key)
		}
		if headerOK && !forwardIn {
			v.errp("G06", "loop_invalid", lp, map[string]any{"loop": l.Key, "header": l.Header}, "loop %q: no edge enters its header %q forward", l.Key, l.Header)
		}
	}
	for i, s := range v.doc.Steps {
		lastBack := -1
		for j, e := range s.Edges {
			if !e.IsBack() {
				continue
			}
			lastBack = j
			ep := edgePath(label(s.Key, i), label(e.ID, j))
			l, ok := v.loops[e.Loop]
			switch {
			case !ok:
				v.errp("G05", "loop_edge_invalid", ep+".loop", map[string]any{"loop": e.Loop}, "edge %q names the loop %q, which is not declared", e.ID, e.Loop)
			case e.To != l.Header:
				v.errp("G05", "loop_edge_invalid", ep+".to", map[string]any{"loop": e.Loop, "header": l.Header}, "edge %q closes loop %q and must lead to its header %q", e.ID, e.Loop, l.Header)
			case !reachableFrom(l.Header, v.fwd, "")[s.Key]:
				v.errp("G05", "loop_edge_invalid", ep, map[string]any{"loop": e.Loop, "header": l.Header}, "edge %q closes loop %q but %q cannot be reached from its header", e.ID, e.Loop, s.Key)
			}
		}
		if lastBack >= 0 {
			exit := false
			for _, e := range s.Edges[lastBack+1:] {
				exit = exit || e.When == nil || e.Stop != ""
			}
			if !exit {
				v.errp("G07", "loop_exit_missing", stepPath(label(s.Key, i)), map[string]any{"step": s.Key}, "step %q goes back but has no later edge for when the loop is used up", s.Key)
			}
		}
	}
}

// checkLeadSteps is G10.
func (v *validator) checkLeadSteps() {
	for i, s := range v.doc.Steps {
		sp := stepPath(label(s.Key, i))
		if s.Kind != StepKindLead {
			if s.Lead != nil {
				v.err("G10", "lead_step_invalid", sp+".lead", "only a lead step has a lead block")
			}
			continue
		}
		if len(s.Tasks) > 0 {
			v.err("G10", "lead_step_invalid", sp+".tasks", "a lead step has no tasks")
		}
		if s.Lead == nil {
			continue
		}
		for _, name := range s.Lead.Sets {
			x, ok := v.vars[name]
			if !ok {
				v.errp("G10", "lead_step_invalid", sp+".lead.sets", map[string]any{"var": name}, "the lead sets %q, which is not a variable", name)
				continue
			}
			if !inStrings(x.SetBy, RoleKindLead) || !(inStrings(x.SetAt, s.Key) || inStrings(x.SetAt, "*")) {
				v.errp("G10", "lead_step_invalid", sp+".lead.sets", map[string]any{"var": name}, "variable %q is not settable by the lead at step %q", name, s.Key)
			}
		}
		if s.Lead.Verdicts != "" && !v.setKnown(s.Lead.Verdicts) {
			v.errp("G10", "lead_step_invalid", sp+".lead.verdicts", map[string]any{"set": s.Lead.Verdicts}, "verdict set %q is not declared", s.Lead.Verdicts)
		}
	}
}

// checkCost is G11: the worst case of spec 4.5 against the caps.
func (v *validator) checkCost() {
	a := Analyze(v.doc)
	p := v.doc.Policy.Effective()
	if a.WorstCaseTaskIssues > p.MaxTaskIssues || a.WorstCaseActivations > p.MaxActivations {
		v.errp("G11", "flow_too_expensive", "steps", map[string]any{
			"worst_case_task_issues": a.WorstCaseTaskIssues, "max_task_issues": p.MaxTaskIssues,
			"worst_case_activations": a.WorstCaseActivations, "max_activations": p.MaxActivations,
		}, "the worst case is %d task issues (limit %d) and %d entries into steps (limit %d)",
			a.WorstCaseTaskIssues, p.MaxTaskIssues, a.WorstCaseActivations, p.MaxActivations)
	}
}
