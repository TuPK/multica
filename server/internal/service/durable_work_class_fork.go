package service

import "strings"

// maxWorkClassLen is the spec 1.2 limit: char_length(work_class) <= 200.
const maxWorkClassLen = 200

const wcLabelPrefix = "wc:"

// WorkClassInput is the issue data WorkClassFor reads. It is plain data so the
// derivation stays pure until the issue and label tables exist.
type WorkClassInput struct {
	FlowID       string
	FlowStageKey string
	LabelNames   []string
	SquadID      string
}

// WorkClassFor derives the work class in the priority order of spec 1.2:
// flow, then a "wc:" label, then the squad, then "general".
func WorkClassFor(in WorkClassInput) string {
	var class string
	switch tail := wcLabelTail(in.LabelNames); {
	case in.FlowID != "" && in.FlowStageKey != "":
		class = "flow:" + in.FlowID + ":" + in.FlowStageKey
	case tail != "":
		class = "label:" + tail
	case in.SquadID != "":
		class = "squad:" + in.SquadID
	default:
		class = "general"
	}
	return cutWorkClass(class, maxWorkClassLen)
}

// wcLabelTail returns the part after "wc:" of the first such label by name, or
// "" when none has a non-empty tail.
func wcLabelTail(names []string) string {
	first := ""
	for _, n := range names {
		if len(n) > len(wcLabelPrefix) && strings.HasPrefix(n, wcLabelPrefix) && (first == "" || n < first) {
			first = n
		}
	}
	return strings.TrimPrefix(first, wcLabelPrefix)
}

// cutWorkClass cuts s to at most max characters. Characters, not bytes, match
// Postgres char_length, so a multi-byte value is never split mid-rune.
func cutWorkClass(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
