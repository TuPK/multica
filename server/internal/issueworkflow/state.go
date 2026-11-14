package issueworkflow

// OutcomeSuperseded is the outcome the engine writes on a task it cancels
// itself: a join that finished early (any, quorum), a request that reached a
// terminal, a resume that left the step. Only the engine writes it, and a join
// ignores such a task. A task a person cancelled does not carry it.
const OutcomeSuperseded = "superseded"

// State is what the schema 2 engine keeps in issue.flow for a request (spec
// section 2.4). It is plain data: the functions around it return new values and
// leave their arguments as they were.
type State struct {
	WorkflowID string `json:"workflow_id,omitempty"`
	// Version is the schema of the pinned definition.
	Version int    `json:"version"`
	Step    string `json:"step"`
	// Seq grows on every write; a write names the Seq it read.
	Seq int `json:"seq"`
	// Activations counts the entries into each step.
	Activations map[string]int `json:"activations,omitempty"`
	// Loops counts the back-edges followed in each loop.
	Loops map[string]int `json:"loops,omitempty"`
	// Vars are the variable values, set through the API.
	Vars      map[string]any `json:"vars,omitempty"`
	CameFrom  string         `json:"came_from,omitempty"`
	EnteredAt string         `json:"entered_at,omitempty"`
	// Stop is set while the request is parked; null otherwise.
	Stop        *StopRecord  `json:"stop"`
	BlockedFrom string       `json:"blocked_from,omitempty"`
	Checkpoints []Checkpoint `json:"checkpoints,omitempty"`
	// VarSets records which task set a variable at which entry into its step,
	// keyed "<step>.<task>".
	VarSets map[string]int `json:"var_sets,omitempty"`
	// Unreviewed is true from the moment a task of an agent produces work until
	// the next human checkpoint.
	Unreviewed bool `json:"unreviewed"`
	// Trail holds one entry per step left, oldest first, capped by the policy.
	Trail []TrailEntry `json:"trail,omitempty"`
	// Trial marks a request run as a rehearsal of a draft.
	Trial    bool `json:"trial"`
	PlanDone bool `json:"plan_done,omitempty"`
	// RoundBase is the schema 1 round base of a request on a compat v1 graph:
	// its tasks open in round RoundBase + Loops["rework"]. Schema 2 requests
	// use the activation of the step instead.
	RoundBase int `json:"round_base,omitempty"`
	// ClosedAt is left out until the request closes: the open-requests index
	// tests for the key.
	ClosedAt string `json:"closed_at,omitempty"`
}

// StopRecord says why and where a request is parked.
type StopRecord struct {
	Code string `json:"code"`
	From string `json:"from"`
	Edge string `json:"edge,omitempty"`
	At   string `json:"at,omitempty"`
	// Reason is why the engine stalled the request (a task that could not open, a
	// deadline); empty for any other stop. The insights of a version count it.
	Reason string `json:"reason,omitempty"`
}

// Checkpoint records that a person looked at the request: the step they were
// shown, the entry into it, who and when.
type Checkpoint struct {
	Step       string `json:"step"`
	Activation int    `json:"activation"`
	By         string `json:"by,omitempty"`
	At         string `json:"at,omitempty"`
}

// TrailEntry records one step the request left and the edge it left by.
type TrailEntry struct {
	Step       string `json:"step"`
	Activation int    `json:"activation"`
	Edge       string `json:"edge,omitempty"`
	At         string `json:"at,omitempty"`
}

// AppendTrail returns s with e added to its trail. The trail keeps the newest
// max entries: the oldest are dropped, since the default inputs of a task read
// the trail backwards from the end. A max of 0 or less keeps none. The trail of
// s is not touched.
func AppendTrail(s State, e TrailEntry, max int) State {
	if max <= 0 {
		s.Trail = nil
		return s
	}
	keep := s.Trail
	if len(keep) >= max {
		keep = keep[len(keep)-(max-1):]
	}
	trail := make([]TrailEntry, 0, len(keep)+1)
	trail = append(trail, keep...)
	s.Trail = append(trail, e)
	return s
}

// Round is the round the tasks of the current step open in on a compat v1
// graph: the schema 1 round base plus the rework count. A schema 2 request
// uses the activation of the step.
func (s State) Round() int { return s.RoundBase + s.Loops[LoopRework] }

// Clone returns a copy of the state that shares no map or slice with it, so a
// writer can change the copy and keep the state it read.
func (s State) Clone() State {
	out := s
	out.Activations = copyCounts(s.Activations)
	out.Loops = copyCounts(s.Loops)
	out.VarSets = copyCounts(s.VarSets)
	if s.Vars != nil {
		out.Vars = make(map[string]any, len(s.Vars))
		for k, v := range s.Vars {
			out.Vars[k] = v
		}
	}
	if s.Stop != nil {
		stop := *s.Stop
		out.Stop = &stop
	}
	out.Checkpoints = append([]Checkpoint(nil), s.Checkpoints...)
	out.Trail = append([]TrailEntry(nil), s.Trail...)
	return out
}
