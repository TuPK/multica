package issueworkflow

import (
	"fmt"
	"math"
)

// Expressions of schema 2. The tree is the Condition of transitions.go; what
// changes is the fact it reads and how it fails. Evaluate (schema 1) treats a
// field it does not know as false. EvaluateWith treats it as an error, so a typo
// in a document cannot route a request silently.

// ExprError is a condition that cannot be evaluated: a fact outside the fact
// set, an operator the fact's type does not take, or a value of the wrong type.
type ExprError struct {
	Code    string
	Field   string
	Message string
}

func (e *ExprError) Error() string {
	if e.Field == "" {
		return e.Code + ": " + e.Message
	}
	return e.Code + ": " + e.Field + ": " + e.Message
}

func exprErr(code, field, format string, args ...any) *ExprError {
	return &ExprError{Code: code, Field: field, Message: fmt.Sprintf(format, args...)}
}

// truth is a three-valued result. A comparison on a fact that has no value yet
// is unknown, and unknown stays unknown through not, so that negating a
// comparison on a fact that has not happened does not make it true.
type truth int

const (
	truthFalse truth = iota
	truthTrue
	truthUnknown
)

func (t truth) not() truth {
	switch t {
	case truthTrue:
		return truthFalse
	case truthFalse:
		return truthTrue
	}
	return truthUnknown
}

// EvaluateWith reports whether the condition holds for the facts. Only a result
// that is known to be true holds.
//
// A field outside the fact set is an error, never false. A fact the set declares
// but has no value for yet (a task that did not open, a project the request is
// not in) answers exists with false, answers every other comparison with
// false, and counts as 0 when it is a counter; not over such a comparison is
// false as well, not true.
func EvaluateWith(c Condition, fs FactSet) (bool, error) {
	t, err := evalTruth(c, fs, 1)
	if err != nil {
		return false, err
	}
	return t == truthTrue, nil
}

func evalTruth(c Condition, fs FactSet, depth int) (truth, error) {
	if depth > DocMaxExprDepth {
		return truthFalse, exprErr(CodeConditionInvalid, "", "conditions can nest at most %d levels", DocMaxExprDepth)
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
		return truthFalse, exprErr(CodeConditionInvalid, "", "a condition is a group (all or any), a negation (not), or a field check (field, op, value)")
	}
	switch {
	case c.All != nil:
		return evalGroup("all", c.All, fs, depth, truthTrue, truthFalse)
	case c.Any != nil:
		return evalGroup("any", c.Any, fs, depth, truthFalse, truthTrue)
	case c.Not != nil:
		t, err := evalTruth(*c.Not, fs, depth+1)
		return t.not(), err
	}
	return evalFact(c, fs)
}

// evalGroup combines the children of an all or any group. identity is the
// result of an empty run of children that all agree, decisive the result that
// settles the group at once. Every child is evaluated, so a bad field is an
// error wherever it sits.
func evalGroup(name string, children []Condition, fs FactSet, depth int, identity, decisive truth) (truth, error) {
	if len(children) == 0 {
		return truthFalse, exprErr(CodeConditionInvalid, "", "a %q group needs at least one condition", name)
	}
	result := identity
	for _, child := range children {
		t, err := evalTruth(child, fs, depth+1)
		if err != nil {
			return truthFalse, err
		}
		switch {
		case t == decisive:
			result = decisive
		case t == truthUnknown && result != decisive:
			result = truthUnknown
		}
	}
	return result, nil
}

func evalFact(c Condition, fs FactSet) (truth, error) {
	spec, ok := fs.Spec(c.Field)
	if !ok {
		if isLegacyFact(c.Field) {
			return truthFalse, exprErr(CodeLegacyFieldInV2, c.Field, "a schema 1 field; schema 2 reads the facts of the fact registry")
		}
		return truthFalse, exprErr(CodeUnknownFact, c.Field, "not a fact of this flow")
	}
	if !hasOp(spec.Ops, c.Op) {
		return truthFalse, exprErr(CodeOpNotAllowed, c.Field, "operator %q does not apply to a %s fact", c.Op, spec.Type)
	}
	value, present := fs.Value(c.Field)
	if c.Op == OpExists {
		if c.Value != nil {
			return truthFalse, exprErr(CodeConditionInvalid, c.Field, "operator %q takes no value", c.Op)
		}
		return fromBool(present && factPresent(spec, value)), nil
	}
	if err := checkOperand(spec, c); err != nil {
		return truthFalse, err
	}
	if !present {
		if !isCounter(spec) {
			return truthUnknown, nil
		}
		value = FactValue{}
	}
	return fromBool(compareFact(spec, c, value)), nil
}

func fromBool(b bool) truth {
	if b {
		return truthTrue
	}
	return truthFalse
}

func hasOp(ops []string, op string) bool { return inStrings(ops, op) }

// isCounter reports whether a fact that has no value reads as 0: the counts
// the engine keeps.
func isCounter(spec FactSpec) bool {
	return spec.Type == VarTypeInt && spec.Source == FactSourceEngine
}

// factPresent is what exists means for a fact that has a value: an empty string
// or an empty list is as good as none, as in schema 1.
func factPresent(spec FactSpec, v FactValue) bool {
	switch spec.Type {
	case VarTypeString, VarTypeEnum:
		return v.Str != ""
	case VarTypeListString:
		return len(v.List) > 0
	}
	return true
}

// checkOperand validates the value of a condition against the fact's type, so
// that a wrong value is an error even when the fact has no value yet.
func checkOperand(spec FactSpec, c Condition) *ExprError {
	operands := []any{c.Value}
	if c.Op == OpIn {
		operands = asList(c.Value)
		if len(operands) == 0 {
			return exprErr(CodeConditionInvalid, c.Field, "operator %q needs a non-empty list", c.Op)
		}
	}
	for _, operand := range operands {
		switch spec.Type {
		case VarTypeBool:
			if _, ok := operand.(bool); !ok {
				return exprErr(CodeConditionInvalid, c.Field, "value must be true or false")
			}
		case VarTypeInt:
			n, ok := asNumber(operand)
			if !ok || n != math.Trunc(n) {
				return exprErr(CodeConditionInvalid, c.Field, "value must be a whole number")
			}
		case VarTypeEnum:
			s, ok := operand.(string)
			if !ok || (len(spec.Values) > 0 && !inStrings(spec.Values, s)) {
				return exprErr(CodeConditionInvalid, c.Field, "value %v is not one of the fact's values", operand)
			}
		case VarTypeString, VarTypeListString:
			if s, ok := operand.(string); !ok || s == "" {
				return exprErr(CodeConditionInvalid, c.Field, "value must be a non-empty string")
			}
		}
	}
	return nil
}

// compareFact applies the operator of a valid condition to the fact's value.
func compareFact(spec FactSpec, c Condition, v FactValue) bool {
	switch spec.Type {
	case VarTypeBool:
		want, _ := c.Value.(bool)
		switch c.Op {
		case OpEq:
			return v.Bool == want
		case OpNeq:
			return v.Bool != want
		}
	case VarTypeInt:
		return compareNumber(c.Op, c.Value, float64(v.Int))
	case VarTypeEnum:
		if spec.Key == "request.priority" {
			return compareRanked(c.Op, c.Value, priorityRank[v.Str], v.Str)
		}
		return compareString(c.Op, c.Value, v.Str)
	case VarTypeString:
		if v.Str == "" {
			return false
		}
		return compareString(c.Op, c.Value, v.Str)
	case VarTypeListString:
		if c.Op != OpContains {
			return false
		}
		want, _ := c.Value.(string)
		for _, item := range v.List {
			if item == want {
				return true
			}
		}
	}
	return false
}

func inStrings(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}
