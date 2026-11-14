package issueworkflow

import (
	"fmt"
	"reflect"
	"testing"
	"time"
)

// The operating numbers of a version (plan P26, spec 16.6), computed from hand-built trails.

var insightsBase = time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)

// at is the time the given number of minutes after the start of the day, as a trail writes it.
func atMin(minutes int) string {
	return insightsBase.Add(time.Duration(minutes) * time.Minute).Format(time.RFC3339)
}

func insightTrail(entries ...TrailEntry) []TrailEntry { return entries }

func insightLeave(step string, activation int, edge string, minutes int) TrailEntry {
	return TrailEntry{Step: step, Activation: activation, Edge: edge, At: atMin(minutes)}
}

func intPtr(n int) *int { return &n }

func TestInsights_EndsEdgesLoopsDurations(t *testing.T) {
	states := []State{
		// S1: straight through, closed at a terminal.
		{Step: "done", ClosedAt: atMin(30), Trail: insightTrail(insightLeave("plan", 1, "plan_ok", 0), insightLeave("gate", 1, "gate_ok", 30))},
		// S2: one round of the fix loop, closed at a terminal.
		{Step: "done", ClosedAt: atMin(110), Loops: map[string]int{"fix": 1}, Trail: insightTrail(
			insightLeave("plan", 1, "plan_ok", 0), insightLeave("gate", 1, "gate_fix", 40), insightLeave("fix", 1, "fix_back", 50), insightLeave("gate", 2, "gate_ok", 110))},
		// S3: parked at a declared stop after the loop.
		{Step: "gate", Loops: map[string]int{"fix": 1}, Stop: &StopRecord{Code: "loop_exhausted", From: "gate", Edge: "gate_stop"}, Trail: insightTrail(
			insightLeave("plan", 1, "plan_ok", 0), insightLeave("gate", 1, "gate_fix", 20), insightLeave("fix", 1, "fix_back", 25), insightLeave("gate", 2, "gate_stop", 50))},
		// S4: stalled with a reason.
		{Step: "draft", Stop: &StopRecord{Code: "stalled", From: "draft", Reason: "flow_agent_unavailable"}, Trail: insightTrail(
			insightLeave("plan", 1, "plan_ok", 0), insightLeave("draft", 1, "draft_stall", 15))},
		// S5: still running at the gate; the visit in progress has no duration yet.
		{Step: "gate", Trail: insightTrail(insightLeave("plan", 1, "plan_ok", 0))},
		// S6: stalled, from before the reason was kept.
		{Step: "draft", Stop: &StopRecord{Code: "stalled", From: "draft"}, Trail: insightTrail(
			insightLeave("plan", 1, "plan_ok", 0), insightLeave("draft", 1, "draft_stall", 25))},
	}
	closes := []ManualClose{{Step: "gate", Task: "sign"}, {Step: "gate", Task: "sign"}, {}}

	got := Insights(states, closes)
	want := Report{
		Requests: RequestCounts{Total: 6, Open: 1, ByEnd: map[string]int{"terminal:done": 2, "stop:loop_exhausted": 1, "stalled": 2}},
		Steps: []StepInsight{
			{Key: "draft", Entered: 2, MedianMinutes: intPtr(15), P90Minutes: intPtr(25)},
			{Key: "fix", Entered: 2, MedianMinutes: intPtr(5), P90Minutes: intPtr(10)},
			{Key: "gate", Entered: 6, MedianMinutes: intPtr(30), P90Minutes: intPtr(60)},
			{Key: "plan", Entered: 6},
		},
		Edges: []EdgeInsight{
			{Step: "draft", Edge: "draft_stall", Taken: 2},
			{Step: "fix", Edge: "fix_back", Taken: 2},
			{Step: "gate", Edge: "gate_fix", Taken: 2},
			{Step: "gate", Edge: "gate_ok", Taken: 2},
			{Step: "gate", Edge: "gate_stop", Taken: 1},
			{Step: "plan", Edge: "plan_ok", Taken: 6},
		},
		Loops:        []LoopInsight{{Key: "fix", Histogram: []int{4, 2}}},
		Stalls:       []StallInsight{{Reason: "flow_agent_unavailable", Count: 1}, {Reason: "unknown", Count: 1}},
		ManualCloses: 2,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("report differs.\n got: %s\nwant: %s", dumpReport(got), dumpReport(want))
	}
}

func TestInsights_Percentiles(t *testing.T) {
	// Ten visits of 1..10 minutes: the nearest rank gives 5 and 9.
	var states []State
	for minutes := 1; minutes <= 10; minutes++ {
		states = append(states, State{Step: "done", ClosedAt: atMin(minutes), Trail: insightTrail(insightLeave("a", 1, "a_b", 0), insightLeave("b", 1, "b_done", minutes))})
	}
	got := Insights(states, nil)
	var b StepInsight
	for _, s := range got.Steps {
		if s.Key == "b" {
			b = s
		}
	}
	if b.Entered != 10 || b.MedianMinutes == nil || *b.MedianMinutes != 5 || b.P90Minutes == nil || *b.P90Minutes != 9 {
		t.Fatalf("step b = %+v", b)
	}
}

func TestInsights_Empty(t *testing.T) {
	got := Insights(nil, nil)
	if got.Requests.Total != 0 || got.Requests.ByEnd == nil || got.Steps == nil || got.Edges == nil || got.Loops == nil || got.Stalls == nil {
		t.Fatalf("an empty report has no lists to marshal as null: %+v", got)
	}
}

func TestInsights_IgnoresMalformedTrail(t *testing.T) {
	states := []State{{
		Step: "e",
		Trail: insightTrail(
			insightLeave("a", 1, "e1", 0),
			// No step: nothing to count, and the entry after it has no start to measure from.
			TrailEntry{Step: "", Activation: 1, Edge: "x", At: atMin(5)},
			// A time that is not a time: the visit is counted, its length is not known.
			TrailEntry{Step: "b", Activation: 1, Edge: "e2", At: "not a time"},
			// Follows an entry with no usable time.
			insightLeave("c", 1, "e3", 20),
			// Earlier than the entry before it: a negative visit is not a visit length.
			insightLeave("d", 1, "e4", 10),
			// No edge: counted as entered, no edge taken. Its length counts, since the entry before it is sound.
			TrailEntry{Step: "f", Activation: 1, At: atMin(15)},
		),
	}, {
		// A state with nothing in it.
	}}
	got := Insights(states, []ManualClose{{Step: "gate"}})

	steps := map[string]StepInsight{}
	for _, s := range got.Steps {
		steps[s.Key] = s
	}
	for _, key := range []string{"a", "b", "c", "d", "e"} {
		s, ok := steps[key]
		if !ok || s.Entered != 1 || s.MedianMinutes != nil || s.P90Minutes != nil {
			t.Errorf("step %q = %+v (present %v), want one entry and no duration", key, s, ok)
		}
	}
	if f := steps["f"]; f.Entered != 1 || f.MedianMinutes == nil || *f.MedianMinutes != 5 {
		t.Errorf("step f = %+v, want one entry of 5 minutes", f)
	}
	if _, ok := steps[""]; ok || len(got.Steps) != 6 {
		t.Errorf("steps = %+v, want the six that have a key", got.Steps)
	}
	edges := map[string]int{}
	for _, e := range got.Edges {
		edges[e.Step+"/"+e.Edge] = e.Taken
	}
	if !reflect.DeepEqual(edges, map[string]int{"a/e1": 1, "b/e2": 1, "c/e3": 1, "d/e4": 1}) {
		t.Errorf("edges = %v", edges)
	}
	if got.Requests.Total != 2 || got.ManualCloses != 0 {
		t.Errorf("requests %+v, manual closes %d", got.Requests, got.ManualCloses)
	}
}

func TestInsights_LoopLimitsFromDefinition(t *testing.T) {
	states := []State{
		{Step: "done", ClosedAt: atMin(1), Loops: map[string]int{"fix": 2}},
		{Step: "done", ClosedAt: atMin(1), Loops: map[string]int{"fix": 1}},
		{Step: "done", ClosedAt: atMin(1)},
	}
	report := Insights(states, nil).WithLoopLimits([]Loop{
		{Key: "fix", Header: "gate", Max: LoopMax{N: 3}},
		{Key: "never_seen", Header: "gate", Max: LoopMax{Var: "rounds", Cap: 2}},
	})
	want := []LoopInsight{
		{Key: "fix", Max: 3, Histogram: []int{1, 1, 1, 0}},
		{Key: "never_seen", Max: 2, Histogram: []int{3, 0, 0}},
	}
	if !reflect.DeepEqual(report.Loops, want) {
		t.Fatalf("loops = %+v, want %+v", report.Loops, want)
	}
}

func dumpReport(r Report) string {
	out := ""
	add := func(label string, v any) { out += "\n  " + label + ": " + dumpValue(v) }
	add("requests", r.Requests)
	for _, s := range r.Steps {
		add("step "+s.Key, []any{s.Entered, derefInt(s.MedianMinutes), derefInt(s.P90Minutes)})
	}
	add("edges", r.Edges)
	add("loops", r.Loops)
	add("stalls", r.Stalls)
	add("manual", r.ManualCloses)
	return out
}

func derefInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func dumpValue(v any) string { return fmt.Sprintf("%+v", v) }

// The stall reason that the report counts is kept in the state when the engine stalls a request.
func TestApplyDecision_StallKeepsItsReason(t *testing.T) {
	g := loadGraph(t, "patterns/p01_sequential.json")
	st := InitialState(g, nil, atMin(0))
	next := ApplyDecision(g, st, StalledDecision("flow_agent_unavailable"), ApplyMeta{Now: atMin(5)})
	if next.Stop == nil || next.Stop.Code != DocStopStalled || next.Stop.Reason != "flow_agent_unavailable" {
		t.Fatalf("stop = %+v", next.Stop)
	}
	if st.Stop != nil {
		t.Fatal("the state that was read changed")
	}
}
