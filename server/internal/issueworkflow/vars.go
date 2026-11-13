package issueworkflow

import (
	"fmt"
	"strings"
)

// Var returns the variable of the document with the key.
func (g Graph) Var(key string) (Var, bool) {
	for _, v := range g.Doc.Vars {
		if v.Key == key {
			return v, true
		}
	}
	return Var{}, false
}

// Role returns the role of the document with the key.
func (g Graph) Role(key string) (Role, bool) {
	r, ok := g.roles[key]
	return r, ok
}

// TaskVerdicts is the verdict set a task reports in: the one it names, or the
// built-in set of its kind. blocked is always allowed besides these.
func TaskVerdicts(doc FlowDoc, task DocTask) []Verdict {
	return verdictsOf(doc, task.Kind, task.Verdicts)
}

// CheckVarValue checks a value against the declaration of a variable (type,
// enum values, range, length, items) and returns it in the form the state stores.
// The error says what is wrong in words a caller can show.
func CheckVarValue(v Var, raw any) (any, error) {
	switch v.Type {
	case VarTypeBool:
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("%s takes true or false", v.Key)
		}
		return b, nil
	case VarTypeEnum:
		s, ok := raw.(string)
		if !ok || !inStrings(v.Values, s) {
			return nil, fmt.Errorf("%s takes one of: %s", v.Key, strings.Join(v.Values, ", "))
		}
		return s, nil
	case VarTypeInt:
		n, ok := asNumber(raw)
		if !ok || n != float64(int(n)) {
			return nil, fmt.Errorf("%s takes a whole number", v.Key)
		}
		i := int(n)
		if v.Min != nil && i < *v.Min {
			return nil, fmt.Errorf("%s is at least %d", v.Key, *v.Min)
		}
		if v.Max != nil && i > *v.Max {
			return nil, fmt.Errorf("%s is at most %d", v.Key, *v.Max)
		}
		return i, nil
	case VarTypeString:
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("%s takes text", v.Key)
		}
		if v.MaxLen > 0 && len([]rune(s)) > v.MaxLen {
			return nil, fmt.Errorf("%s is at most %d characters", v.Key, v.MaxLen)
		}
		return s, nil
	case VarTypeListString:
		fv, ok := varFactValue(v, raw)
		if !ok {
			return nil, fmt.Errorf("%s takes a list of texts", v.Key)
		}
		if v.MaxItems > 0 && len(fv.List) > v.MaxItems {
			return nil, fmt.Errorf("%s has at most %d items", v.Key, v.MaxItems)
		}
		for _, item := range fv.List {
			if v.MaxLen > 0 && len([]rune(item)) > v.MaxLen {
				return nil, fmt.Errorf("an item of %s is at most %d characters", v.Key, v.MaxLen)
			}
		}
		if fv.List == nil {
			return []string{}, nil
		}
		return fv.List, nil
	}
	return nil, fmt.Errorf("%s has an unknown type", v.Key)
}

// VarAllowedAt reports whether the variable may be set at the step by a person
// or role the declaration names (set_at).
func VarAllowedAt(v Var, step string) bool {
	return inStrings(v.SetAt, "*") || inStrings(v.SetAt, step)
}
