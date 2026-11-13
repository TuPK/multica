package issueworkflow

import (
	"sort"
	"strconv"
	"strings"
)

// TaskLinks says which earlier tasks a task reads and which rejections it must
// answer (spec 4.11). It is pure: it names task rows and reads none of their
// content.
//
// A TaskInput the function returns names a row. TaskInput.Identifier is the
// RowRef of the row ("step/key"), because this layer holds no issue numbers; the
// engine replaces it with the issue identifier. Key is the row key (key~i for an
// instance). Title carries the input's slot when the task declared one.
//
// The task is the one being opened at st.Step, in the current entry.
func TaskLinks(g Graph, st State, tasks []TaskState, task DocTask) (inputs, feedback []TaskInput) {
	if g.Compat == CompatV1 {
		return compatLinks(g, st, tasks)
	}
	if len(task.Inputs) > 0 {
		inputs = explicitInputs(task.Inputs, tasks)
	} else {
		inputs = defaultInputs(g, st, tasks, task)
	}
	return inputs, loopFeedback(g, st, tasks)
}

func linkRef(ts TaskState, slot string) TaskInput {
	return TaskInput{Identifier: RowRef(ts.Step, ts.Key), Key: ts.Key, Title: slot, Round: ts.Round}
}

func instanceIndex(key string) int {
	_, suffix, found := strings.Cut(key, "~")
	if !found {
		return 0
	}
	n, err := strconv.Atoi(suffix)
	if err != nil {
		return 0
	}
	return n
}

// rowsOf are the rows of the task in the latest entry of the step, instances in
// order. Superseded rows are not inputs.
func rowsOf(tasks []TaskState, step, taskKey string) []TaskState {
	latest, any := 0, false
	for _, ts := range tasks {
		if ts.Step == step && (!any || ts.Round > latest) {
			latest, any = ts.Round, true
		}
	}
	var out []TaskState
	for _, ts := range tasks {
		if ts.Step == step && ts.Round == latest && instanceBaseKey(ts.Key) == taskKey && ts.Outcome != OutcomeSuperseded {
			out = append(out, ts)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return instanceIndex(out[i].Key) < instanceIndex(out[j].Key) })
	return out
}

// explicitInputs follows the inputs the task declared. A source that never ran
// is skipped; whether that is allowed is up to required (see DecideGraph).
func explicitInputs(declared []Input, tasks []TaskState) []TaskInput {
	var out []TaskInput
	seen := map[string]bool{}
	for _, in := range declared {
		step, key, ok := strings.Cut(in.From, ".")
		if !ok {
			continue
		}
		for _, ts := range rowsOf(tasks, step, key) {
			if ref := RowRef(ts.Step, ts.Key); !seen[ref] {
				seen[ref] = true
				out = append(out, linkRef(ts, in.Slot))
			}
		}
	}
	return out
}

// defaultInputs: walk the trail back from the step the request came from and
// take the work tasks that closed with a pass in the nearest step that has any;
// then add the tasks this task comes after in its own step. The walk follows the
// path the request took, not the order the steps are declared in.
func defaultInputs(g Graph, st State, tasks []TaskState, task DocTask) []TaskInput {
	var out []TaskInput
	seen := map[string]bool{}
	add := func(ts TaskState) {
		if ref := RowRef(ts.Step, ts.Key); !seen[ref] {
			seen[ref] = true
			out = append(out, linkRef(ts, ""))
		}
	}
	for i := len(st.Trail) - 1; i >= 0 && len(out) == 0; i-- {
		entry := st.Trail[i]
		step, ok := g.Step(entry.Step)
		if !ok {
			continue
		}
		for _, ts := range tasks {
			if ts.Step != entry.Step || ts.Round != entry.Activation-1 {
				continue
			}
			class, closed, counted := docTaskClass(g.Doc, step, ts)
			if counted && closed && class == ClassPass && taskKindOf(step, ts) == KindWork {
				add(ts)
			}
		}
	}
	round := entryRound(g, st)
	for _, dep := range task.After {
		for _, ts := range tasks {
			if ts.Step == st.Step && ts.Round == round && instanceBaseKey(ts.Key) == dep && ts.Outcome != OutcomeSuperseded {
				add(ts)
			}
		}
	}
	return out
}

func taskKindOf(step DocStep, ts TaskState) string {
	base := instanceBaseKey(ts.Key)
	for _, t := range step.Tasks {
		if t.Key == base {
			return t.Kind
		}
	}
	return ts.Kind
}

// loopFeedback: when the request entered the current step by a back-edge, the
// tasks of the step it left that failed or blocked in that entry are what the
// rework must answer.
func loopFeedback(g Graph, st State, tasks []TaskState) []TaskInput {
	if len(st.Trail) == 0 {
		return nil
	}
	last := st.Trail[len(st.Trail)-1]
	edge, ok := g.Edge(last.Step, last.Edge)
	if !ok || !edge.Back || edge.Edge.To != st.Step {
		return nil
	}
	step, ok := g.Step(last.Step)
	if !ok {
		return nil
	}
	var out []TaskInput
	for _, ts := range tasks {
		if ts.Step != last.Step || ts.Round != last.Activation-1 {
			continue
		}
		class, _, counted := docTaskClass(g.Doc, step, ts)
		if counted && (class == ClassFail || class == ClassBlock) {
			out = append(out, linkRef(ts, ""))
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return instanceIndex(out[i].Key) < instanceIndex(out[j].Key) })
	return out
}

// compatLinks are the links of a graph lifted from schema 1, by position as
// flowTaskLinks does: the tasks of the nearest earlier step that has rows in
// this round are the inputs, and on a later round the gates of the previous
// round that did not approve are the rejections.
func compatLinks(g Graph, st State, tasks []TaskState) (inputs, feedback []TaskInput) {
	round := st.Round()
	index := -1
	for i, s := range g.Doc.Steps {
		if s.Key == st.Step {
			index = i
		}
	}
	for i := index - 1; i >= 0 && len(inputs) == 0; i-- {
		for _, ts := range tasks {
			if ts.Step == g.Doc.Steps[i].Key && ts.Round == round {
				inputs = append(inputs, linkRef(ts, ""))
			}
		}
	}
	if round > 0 {
		for _, ts := range tasks {
			if ts.Round == round-1 && ts.Kind == KindGate && (ts.Cancelled || ts.Outcome != OutcomeApproved) {
				feedback = append(feedback, linkRef(ts, ""))
			}
		}
	}
	return inputs, feedback
}
