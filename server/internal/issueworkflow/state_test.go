package issueworkflow

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestState_TrailCappedAtMaxActivations(t *testing.T) {
	const max = 4
	s := State{Step: "a"}
	var snapshots []State
	for i := 1; i <= max+3; i++ {
		s = AppendTrail(s, TrailEntry{Step: fmt.Sprintf("s%d", i), Activation: i, Edge: "e"}, max)
		snapshots = append(snapshots, s)
		if want := min(i, max); len(s.Trail) != want {
			t.Fatalf("after %d appends the trail has %d entries, want %d", i, len(s.Trail), want)
		}
	}
	var steps []string
	for _, e := range s.Trail {
		steps = append(steps, e.Step)
	}
	if want := []string{"s4", "s5", "s6", "s7"}; !reflect.DeepEqual(steps, want) {
		t.Fatalf("the trail keeps %v, want the newest %v", steps, want)
	}

	t.Run("it is pure", func(t *testing.T) {
		// An earlier snapshot is not changed by later appends.
		if got := snapshots[1].Trail; len(got) != 2 || got[0].Step != "s1" || got[1].Step != "s2" {
			t.Fatalf("an earlier state was changed: %+v", got)
		}
		base := State{Step: "a", Trail: []TrailEntry{{Step: "x"}, {Step: "y"}}}
		full := AppendTrail(base, TrailEntry{Step: "z"}, 2)
		if len(base.Trail) != 2 || base.Trail[0].Step != "x" || base.Trail[1].Step != "y" {
			t.Fatalf("the argument was changed: %+v", base.Trail)
		}
		if len(full.Trail) != 2 || full.Trail[0].Step != "y" || full.Trail[1].Step != "z" {
			t.Fatalf("result = %+v, want y, z", full.Trail)
		}
		// The two trails do not share memory.
		full.Trail[0].Step = "changed"
		if base.Trail[1].Step != "y" {
			t.Fatal("the result shares its array with the argument")
		}
	})

	t.Run("a cap of one or none", func(t *testing.T) {
		one := AppendTrail(State{Trail: []TrailEntry{{Step: "x"}}}, TrailEntry{Step: "y"}, 1)
		if len(one.Trail) != 1 || one.Trail[0].Step != "y" {
			t.Fatalf("cap 1: %+v", one.Trail)
		}
		for _, m := range []int{0, -1} {
			if got := AppendTrail(State{Trail: []TrailEntry{{Step: "x"}}}, TrailEntry{Step: "y"}, m); len(got.Trail) != 0 {
				t.Fatalf("cap %d keeps %+v", m, got.Trail)
			}
		}
	})

	t.Run("the cap is the policy default", func(t *testing.T) {
		s := State{}
		for i := 0; i < DefaultMaxActivations+10; i++ {
			s = AppendTrail(s, TrailEntry{Step: "a", Activation: i}, Policy{}.Effective().MaxActivations)
		}
		if len(s.Trail) != DefaultMaxActivations || s.Trail[len(s.Trail)-1].Activation != DefaultMaxActivations+9 {
			t.Fatalf("trail = %d entries, last %+v", len(s.Trail), s.Trail[len(s.Trail)-1])
		}
	})
}

func TestState_JSON(t *testing.T) {
	raw := `{"workflow_id":"wf","version":2,"step":"gate","seq":17,
	  "activations":{"collect":1,"draft":2,"gate":2},"loops":{"fix":1},
	  "vars":{"route":"shipment_dossier","customs_lookup":true},
	  "came_from":"draft","entered_at":"2026-11-13T08:00:00Z",
	  "stop":{"code":"loop_exhausted","from":"gate","edge":"gate_default","at":"2026-11-13T09:00:00Z"},
	  "blocked_from":"gate",
	  "checkpoints":[{"step":"release","activation":1,"by":"u1","at":"2026-11-13T08:30:00Z"}],
	  "var_sets":{"plan_sources.list_sources":1},"unreviewed":false,
	  "trail":[{"step":"plan","activation":1,"edge":"plan_ok","at":"2026-11-13T08:01:00Z"}],
	  "trial":false,"plan_done":true}`
	var s State
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatal(err)
	}
	if s.Seq != 17 || s.Activations["draft"] != 2 || s.Loops["fix"] != 1 || s.Stop == nil || s.Stop.Code != "loop_exhausted" ||
		len(s.Checkpoints) != 1 || s.VarSets["plan_sources.list_sources"] != 1 || len(s.Trail) != 1 || !s.PlanDone {
		t.Fatalf("state = %+v", s)
	}
	again, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var want, got map[string]any
	_ = json.Unmarshal([]byte(raw), &want)
	_ = json.Unmarshal(again, &got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("re-encoded state differs\n got: %v\nwant: %v", got, want)
	}

	idle, _ := json.Marshal(State{Version: 2, Step: "plan"})
	if strings.Contains(string(idle), "closed_at") {
		t.Fatalf("an open state serializes closed_at: %s", idle)
	}
	if !strings.Contains(string(idle), `"stop":null`) {
		t.Fatalf("a state that is not parked writes stop null: %s", idle)
	}
	if OutcomeSuperseded != "superseded" {
		t.Fatalf("OutcomeSuperseded = %q", OutcomeSuperseded)
	}
	if IsOutcome(OutcomeSuperseded) {
		t.Fatal("superseded must not become an outcome a handler may report")
	}
}
