package issueworkflow

// ReasonPoolTooSmall: no one in the role's pool can take a task row.
const ReasonPoolTooSmall = "pool_too_small"

// AssignRequest is one choice of a person from a role's pool (spec 2.5).
type AssignRequest struct {
	// Pool is the people bound to the role, in the order of the binding.
	Pool []string
	// Instance is the 1-based instance, 0 for a task without instances.
	Instance int
	// Prior is who held this row at an earlier entry into the step. The same
	// person keeps it when they still can: whoever fixes the work is the one who
	// did it.
	Prior string
	// Taken are the people who hold another row of the same task in this entry.
	Taken map[string]bool
	// Unavailable are people who cannot take work: archived, or gone from the squad.
	Unavailable map[string]bool
	// Forbidden are people who may not take this row: the ones whose work a gate
	// judges (E13).
	Forbidden map[string]bool
	// Distinct asks for a different person per instance.
	Distinct bool
}

// AssignPool picks the person for a task row. The i-th instance gets
// Pool[(i-1) mod n] and a task without instances gets Pool[0]. A person who is
// unavailable, forbidden, or (when distinct) already holding another instance
// is passed over for the next person in the pool who is free. It reports false
// when nobody is left: the request stalls with ReasonPoolTooSmall.
func AssignPool(req AssignRequest) (string, bool) {
	n := len(req.Pool)
	if n == 0 {
		return "", false
	}
	usable := func(p string) bool {
		if req.Unavailable[p] || req.Forbidden[p] {
			return false
		}
		return !(req.Distinct && req.Taken[p])
	}
	if req.Prior != "" && inStrings(req.Pool, req.Prior) && !req.Unavailable[req.Prior] && !req.Forbidden[req.Prior] {
		return req.Prior, true
	}
	start := 0
	if req.Instance > 1 {
		start = (req.Instance - 1) % n
	}
	for offset := 0; offset < n; offset++ {
		if p := req.Pool[(start+offset)%n]; usable(p) {
			return p, true
		}
	}
	return "", false
}

// JudgedWorkers are the people who did the work a gate judges: the assignees of
// the work tasks among its inputs (TaskLinks), which include the work tasks of
// its own step that it comes after. assignee names who holds a task row. A task
// that is not a gate judges nobody.
func JudgedWorkers(g Graph, st State, tasks []TaskState, task DocTask, assignee func(step, rowKey string) string) map[string]bool {
	if task.Kind != KindGate {
		return nil
	}
	inputs, _ := TaskLinks(g, st, tasks, task)
	out := map[string]bool{}
	for _, in := range inputs {
		for _, ts := range tasks {
			if RowRef(ts.Step, ts.Key) != in.Identifier {
				continue
			}
			step, ok := g.Step(ts.Step)
			if !ok || taskKindOf(step, ts) != KindWork {
				continue
			}
			if p := assignee(ts.Step, ts.Key); p != "" {
				out[p] = true
			}
		}
	}
	return out
}
