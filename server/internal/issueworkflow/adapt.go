package issueworkflow

// A request on a schema 1 workflow keeps the issue.flow of schema 1. Adapt reads
// it as a State for the schema 2 engine, and Unadapt writes a State back in the
// schema 1 form, so that a request has exactly one stored form (spec section 2.4).

// Stored version of a State read from a schema 1 state: the schema of the
// definition the request is pinned to.
const stateVersionV1 = 1

// Adapt reads a schema 1 state as a State. The maps are copied. A field schema 1
// has no equivalent for (seq, trail, activations) is left empty.
func Adapt(v1 FlowState) State {
	s := State{
		WorkflowID:  v1.WorkflowID,
		Version:     stateVersionV1,
		Step:        v1.Step,
		Loops:       map[string]int{LoopRework: v1.Rework},
		EnteredAt:   v1.EnteredAt,
		BlockedFrom: v1.BlockedFrom,
		PlanDone:    v1.PlanDone,
		RoundBase:   v1.RoundBase,
		ClosedAt:    v1.ClosedAt,
	}
	if v1.Switches != nil {
		s.Vars = make(map[string]any, len(v1.Switches))
		for k, v := range v1.Switches {
			s.Vars[k] = v
		}
	}
	if v1.Stop != StopNone {
		s.Stop = &StopRecord{Code: stopCodeOf(v1.Stop), From: v1.BlockedFrom}
	}
	return s
}

// Unadapt writes a State in the schema 1 form. A stop code schema 1 does not
// have reads as stalled, as an unknown stop does in FlowState.
func Unadapt(s State) FlowState {
	v1 := FlowState{
		WorkflowID:  s.WorkflowID,
		Step:        s.Step,
		Rework:      s.Loops[LoopRework],
		RoundBase:   s.RoundBase,
		EnteredAt:   s.EnteredAt,
		BlockedFrom: s.BlockedFrom,
		PlanDone:    s.PlanDone,
		ClosedAt:    s.ClosedAt,
	}
	if s.Vars != nil {
		v1.Switches = make(map[string]any, len(s.Vars))
		for k, v := range s.Vars {
			v1.Switches[k] = v
		}
	}
	if s.Stop != nil {
		v1.Stop = legacyStopOf(s.Stop.Code)
	}
	return v1
}

// stopCodeOf is the stop code of a schema 1 stop.
func stopCodeOf(s Stop) string {
	switch s {
	case Stop1:
		return StopGroundsMissing
	case Stop3:
		return StopLoopExhausted
	}
	return DocStopStalled
}

func legacyStopOf(code string) Stop {
	switch code {
	case StopGroundsMissing:
		return Stop1
	case StopLoopExhausted:
		return Stop3
	}
	return StopStalled
}
