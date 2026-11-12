package issueworkflow

import (
	"crypto/rand"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

// Routing (docs/engineering/workflow-tab.md §3). A step's transitions say where
// an issue goes when the step is left. They are evaluated in order and the
// first match wins; a transition with a null condition is the default and must
// come last. A step without transitions keeps the board's next status.

// Outcomes a handler reports when it moves an issue out of a step.
const (
	OutcomeApproved = "approved"
	OutcomeRejected = "rejected"
	OutcomeBlocked  = "blocked"
	OutcomeDone     = "done"
)

// Fields a condition can test.
const (
	FieldOutcome      = "outcome"
	FieldPriority     = "priority"
	FieldLabels       = "labels"
	FieldProjectID    = "project_id"
	FieldReworkCount  = "rework_count"
	FieldCreatorType  = "creator_type"
	FieldAssigneeType = "assignee_type"
)

// Operators a condition can apply.
const (
	OpEq     = "eq"
	OpNeq    = "neq"
	OpIn     = "in"
	OpGt     = "gt"
	OpGte    = "gte"
	OpLt     = "lt"
	OpLte    = "lte"
	OpExists = "exists"
)

const (
	MaxTransitions          = 10
	MaxConditionDepth       = 4
	MaxTransitionLabelRunes = 64
)

// Condition is a routing condition: a group (all, any), a negation (not), or a
// field check (field, op, value). Exactly one form is set.
type Condition struct {
	All   []Condition `json:"all,omitempty"`
	Any   []Condition `json:"any,omitempty"`
	Not   *Condition  `json:"not,omitempty"`
	Field string      `json:"field,omitempty"`
	Op    string      `json:"op,omitempty"`
	Value any         `json:"value,omitempty"`
}

// Transition routes an issue out of its step. A nil When is the default branch.
type Transition struct {
	ID          string     `json:"id"`
	Label       string     `json:"label"`
	When        *Condition `json:"when"`
	ToStatusKey string     `json:"to_status_key"`
	// Handler, when set, replaces the target step's handler for this branch.
	Handler *Handler `json:"handler,omitempty"`
}

// Facts is what a condition can see about an issue at the moment it leaves a
// step. An empty Outcome means the handler reported none.
type Facts struct {
	Outcome      string
	Priority     string
	Labels       []string
	ProjectID    string
	ReworkCount  int
	CreatorType  string
	AssigneeType string
}

// IsOutcome reports whether s is an outcome a handler may report.
func IsOutcome(s string) bool {
	switch s {
	case OutcomeApproved, OutcomeRejected, OutcomeBlocked, OutcomeDone:
		return true
	}
	return false
}

// Route returns the first transition of the step that matches f.
func (s Step) Route(f Facts) (Transition, bool) {
	for _, t := range s.Transitions {
		if t.When == nil || Evaluate(*t.When, f) {
			return t, true
		}
	}
	return Transition{}, false
}

// NextStatusKey is the target of the step's default transition: where the
// issue goes when no other branch matches. Empty when the step has none.
func (s Step) NextStatusKey() string {
	for _, t := range s.Transitions {
		if t.When == nil {
			return t.ToStatusKey
		}
	}
	return ""
}

// BackStatusKey is the target of the step's rejected branch: where the issue
// goes when review or QA sends it back. Empty when the step has none.
func (s Step) BackStatusKey() string {
	for _, t := range s.Transitions {
		if isOutcomeEq(t.When, OutcomeRejected) {
			return t.ToStatusKey
		}
	}
	return ""
}

func isOutcomeEq(c *Condition, outcome string) bool {
	return c != nil && c.Field == FieldOutcome && c.Op == OpEq && c.Value == outcome
}

// LegacyTransitions converts the two-way routing of the pre-transition model
// (next and back statuses) into transitions. It is the rule the 977 migration
// applies: outcome approved goes to next, outcome rejected goes to back, and
// the default goes to next. The ids are fixed so the migration and this code
// agree.
func LegacyTransitions(next, back string) []Transition {
	var out []Transition
	if next != "" {
		out = append(out, Transition{ID: "approved", Label: "Approved", When: outcomeCondition(OutcomeApproved), ToStatusKey: next})
	}
	if back != "" {
		out = append(out, Transition{ID: "rejected", Label: "Rejected", When: outcomeCondition(OutcomeRejected), ToStatusKey: back})
	}
	if next != "" {
		out = append(out, Transition{ID: "default", Label: "Default", ToStatusKey: next})
	}
	return out
}

func outcomeCondition(outcome string) *Condition {
	return &Condition{Field: FieldOutcome, Op: OpEq, Value: outcome}
}

// NormalizeTransitions trims transition text and gives new transitions an id.
// Ids are kept once assigned, so a condition's identity survives an edit.
func NormalizeTransitions(transitions []Transition) []Transition {
	if len(transitions) == 0 {
		return nil
	}
	out := make([]Transition, len(transitions))
	for i, t := range transitions {
		t.ID = strings.TrimSpace(t.ID)
		if t.ID == "" {
			t.ID = "t" + rand.Text()
		}
		t.Label = strings.TrimSpace(t.Label)
		t.ToStatusKey = strings.TrimSpace(t.ToStatusKey)
		if t.Handler != nil {
			h := NormalizeHandler(*t.Handler)
			t.Handler = &h
		}
		out[i] = t
	}
	return out
}

// Evaluate reports whether the condition holds for f. The condition is
// assumed valid (see ValidateCondition); an invalid one matches nothing.
//
// A field the issue does not have matches no operator except exists: outcome
// is absent until a handler reports one, and project_id is empty outside a
// project. Negating such a check with not still matches, as usual.
func Evaluate(c Condition, f Facts) bool {
	switch {
	case c.All != nil:
		if len(c.All) == 0 {
			return false
		}
		for _, child := range c.All {
			if !Evaluate(child, f) {
				return false
			}
		}
		return true
	case c.Any != nil:
		for _, child := range c.Any {
			if Evaluate(child, f) {
				return true
			}
		}
		return false
	case c.Not != nil:
		return !Evaluate(*c.Not, f)
	}
	return evaluateField(c, f)
}

var priorityRank = map[string]int{"none": 0, "low": 1, "medium": 2, "high": 3, "urgent": 4}

func evaluateField(c Condition, f Facts) bool {
	if _, ok := fieldSpecs[c.Field]; !ok {
		return false
	}
	if c.Op == OpExists {
		return fieldPresent(c.Field, f)
	}
	switch c.Field {
	case FieldLabels:
		if len(f.Labels) == 0 {
			return false
		}
		return compareLabels(c.Op, c.Value, f.Labels)
	case FieldReworkCount:
		return compareNumber(c.Op, c.Value, float64(f.ReworkCount))
	case FieldPriority:
		return compareRanked(c.Op, c.Value, priorityRank[stringField(FieldPriority, f)], stringField(FieldPriority, f))
	}
	current := stringField(c.Field, f)
	if current == "" {
		return false
	}
	return compareString(c.Op, c.Value, current)
}

func fieldPresent(field string, f Facts) bool {
	switch field {
	case FieldLabels:
		return len(f.Labels) > 0
	case FieldPriority, FieldReworkCount:
		return true
	}
	return stringField(field, f) != ""
}

func stringField(field string, f Facts) string {
	switch field {
	case FieldOutcome:
		return f.Outcome
	case FieldProjectID:
		return f.ProjectID
	case FieldCreatorType:
		return f.CreatorType
	case FieldAssigneeType:
		return f.AssigneeType
	case FieldPriority:
		if f.Priority == "" {
			return "none"
		}
		return f.Priority
	}
	return ""
}

func compareString(op string, value any, current string) bool {
	switch op {
	case OpEq:
		s, ok := value.(string)
		return ok && current == s
	case OpNeq:
		s, ok := value.(string)
		return ok && current != s
	case OpIn:
		return inList(value, func(item any) bool {
			s, ok := item.(string)
			return ok && s == current
		})
	}
	return false
}

func compareLabels(op string, value any, labels []string) bool {
	has := func(name string) bool {
		for _, l := range labels {
			if l == name {
				return true
			}
		}
		return false
	}
	switch op {
	case OpEq:
		s, ok := value.(string)
		return ok && has(s)
	case OpNeq:
		s, ok := value.(string)
		return ok && !has(s)
	case OpIn:
		return inList(value, func(item any) bool {
			s, ok := item.(string)
			return ok && has(s)
		})
	}
	return false
}

func compareNumber(op string, value any, current float64) bool {
	if op == OpIn {
		return inList(value, func(item any) bool {
			n, ok := asNumber(item)
			return ok && n == current
		})
	}
	n, ok := asNumber(value)
	if !ok {
		return false
	}
	switch op {
	case OpEq:
		return current == n
	case OpNeq:
		return current != n
	case OpGt:
		return current > n
	case OpGte:
		return current >= n
	case OpLt:
		return current < n
	case OpLte:
		return current <= n
	}
	return false
}

// compareRanked compares an ordered enum (priority) by its rank for the
// ordering operators, and by its name for eq, neq and in.
func compareRanked(op string, value any, rank int, name string) bool {
	switch op {
	case OpGt, OpGte, OpLt, OpLte:
		s, ok := value.(string)
		if !ok {
			return false
		}
		target, ok := priorityRank[s]
		if !ok {
			return false
		}
		switch op {
		case OpGt:
			return rank > target
		case OpGte:
			return rank >= target
		case OpLt:
			return rank < target
		}
		return rank <= target
	}
	return compareString(op, value, name)
}

// inList reports whether any item of a list value satisfies match.
func inList(value any, match func(item any) bool) bool {
	for _, item := range asList(value) {
		if match(item) {
			return true
		}
	}
	return false
}

// asList accepts the list shapes a condition value takes, decoded from JSON
// or built in code.
func asList(value any) []any {
	switch v := value.(type) {
	case []any:
		return v
	case []string:
		out := make([]any, len(v))
		for i, s := range v {
			out[i] = s
		}
		return out
	}
	return nil
}

// asNumber accepts the numeric types a condition value takes: JSON decodes
// every number as float64, and code may use an int.
func asNumber(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	}
	return 0, false
}

// fieldSpec is the type and allowed operators of a condition field.
type fieldSpec struct {
	// kind is the value type the field compares against.
	kind fieldKind
	// values lists the accepted values of an enum field.
	values map[string]bool
	ops    map[string]bool
}

type fieldKind int

const (
	kindText fieldKind = iota
	kindEnum
	kindNumber
)

func set(items ...string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, item := range items {
		out[item] = true
	}
	return out
}

var fieldSpecs = map[string]fieldSpec{
	FieldOutcome: {kind: kindEnum,
		values: set(OutcomeApproved, OutcomeRejected, OutcomeBlocked, OutcomeDone),
		ops:    set(OpEq, OpNeq, OpIn, OpExists)},
	FieldPriority: {kind: kindEnum,
		values: set("none", "low", "medium", "high", "urgent"),
		ops:    set(OpEq, OpNeq, OpIn, OpGt, OpGte, OpLt, OpLte)},
	FieldLabels:      {kind: kindText, ops: set(OpEq, OpNeq, OpIn, OpExists)},
	FieldProjectID:   {kind: kindText, ops: set(OpEq, OpNeq, OpIn, OpExists)},
	FieldReworkCount: {kind: kindNumber, ops: set(OpEq, OpNeq, OpIn, OpGt, OpGte, OpLt, OpLte)},
	FieldCreatorType: {kind: kindEnum, values: set("member", "agent"),
		ops: set(OpEq, OpNeq, OpIn, OpExists)},
	FieldAssigneeType: {kind: kindEnum, values: set("member", "agent", "squad"),
		ops: set(OpEq, OpNeq, OpIn, OpExists)},
}

// ValidateCondition checks a condition's shape, depth, field, operator and value.
func ValidateCondition(c Condition) error {
	return validateCondition(c, 1)
}

func validateCondition(c Condition, depth int) error {
	if depth > MaxConditionDepth {
		return invalid("conditions can nest at most %d levels", MaxConditionDepth)
	}
	forms := 0
	if c.All != nil {
		forms++
	}
	if c.Any != nil {
		forms++
	}
	if c.Not != nil {
		forms++
	}
	if c.Field != "" || c.Op != "" {
		forms++
	}
	if forms != 1 {
		return invalid("a condition is a group (all or any), a negation (not), or a field check (field, op, value)")
	}
	switch {
	case c.All != nil:
		return validateChildren("all", c.All, depth)
	case c.Any != nil:
		return validateChildren("any", c.Any, depth)
	case c.Not != nil:
		return validateCondition(*c.Not, depth+1)
	}
	return validateField(c)
}

func validateChildren(group string, children []Condition, depth int) error {
	if len(children) == 0 {
		return invalid("a %q group needs at least one condition", group)
	}
	for _, child := range children {
		if err := validateCondition(child, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func validateField(c Condition) error {
	spec, ok := fieldSpecs[c.Field]
	if !ok {
		return invalid("unknown condition field %q", c.Field)
	}
	if !spec.ops[c.Op] {
		return invalid("operator %q does not apply to field %q", c.Op, c.Field)
	}
	if c.Op == OpExists {
		if c.Value != nil {
			return invalid("operator %q takes no value", c.Op)
		}
		return nil
	}
	if c.Op == OpIn {
		items := asList(c.Value)
		if len(items) == 0 {
			return invalid("operator %q needs a non-empty list", c.Op)
		}
		for _, item := range items {
			if err := checkScalar(spec, item); err != nil {
				return invalid("field %q: %s", c.Field, err.Error())
			}
		}
		return nil
	}
	if err := checkScalar(spec, c.Value); err != nil {
		return invalid("field %q: %s", c.Field, err.Error())
	}
	return nil
}

// checkScalar validates one value against a field. Ordering operators only
// reach fields that have an order (priority, rework_count), so the enum check
// covers them too.
func checkScalar(spec fieldSpec, value any) error {
	switch spec.kind {
	case kindNumber:
		n, ok := asNumber(value)
		if !ok || n < 0 || n != math.Trunc(n) {
			return fmt.Errorf("value must be a whole number")
		}
	case kindEnum:
		s, ok := value.(string)
		if !ok || !spec.values[s] {
			return fmt.Errorf("value %v is not one of the field's values", value)
		}
	case kindText:
		s, ok := value.(string)
		if !ok || s == "" {
			return fmt.Errorf("value must be a non-empty string")
		}
	}
	return nil
}

// OutcomeRouteError refuses a status change whose outcome routes the issue
// somewhere else. The step's transitions pick the target for an outcome, so a
// run that names another status would skip the routing it reported for.
type OutcomeRouteError struct {
	Outcome   string
	StatusKey string
	// Expected is the status the outcome routes to; Requested is the one written.
	Expected  string
	Requested string
}

func (e *OutcomeRouteError) Error() string {
	return fmt.Sprintf("outcome %q from status %q routes the issue to %q, not %q; set the status to %q",
		e.Outcome, e.StatusKey, e.Expected, e.Requested, e.Expected)
}

// validateTransitions checks a step's transitions against the workflow: every
// target is another status the workflow lists, ids and labels are usable, at
// most one default exists and it comes last, and conditions are well formed.
func validateTransitions(s Step, def Definition) error {
	if len(s.Transitions) > MaxTransitions {
		return invalid("a status can route to at most %d branches", MaxTransitions)
	}
	seen := make(map[string]bool, len(s.Transitions))
	defaults := 0
	for i, t := range s.Transitions {
		if seen[t.ID] {
			return invalid("transition %q appears more than once on %q", t.ID, s.StatusKey)
		}
		seen[t.ID] = true
		if t.Label == "" || utf8.RuneCountInString(t.Label) > MaxTransitionLabelRunes {
			return invalid("transition labels must be 1-%d characters", MaxTransitionLabelRunes)
		}
		if t.When == nil {
			defaults++
			if i != len(s.Transitions)-1 {
				return invalid("the default transition of %q must come last", s.StatusKey)
			}
		} else if err := ValidateCondition(*t.When); err != nil {
			return err
		}
		if t.ToStatusKey == s.StatusKey || !def.Has(t.ToStatusKey) {
			return invalid("%q is not another status of this workflow", t.ToStatusKey)
		}
		if t.Handler != nil {
			if err := validateHandler(*t.Handler); err != nil {
				return err
			}
		}
	}
	if defaults > 1 {
		return invalid("a status can have at most one default transition")
	}
	return nil
}
