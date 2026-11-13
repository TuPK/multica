package issueworkflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAdapt_RoundTrip(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "v1_states", "*.json"))
	if err != nil || len(paths) != 8 {
		t.Fatalf("samples = %d, %v", len(paths), err)
	}
	for _, path := range paths {
		t.Run(strings.TrimSuffix(filepath.Base(path), ".json"), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var v1 FlowState
			if err := json.Unmarshal(raw, &v1); err != nil {
				t.Fatal(err)
			}
			back := Unadapt(Adapt(v1))
			if !reflect.DeepEqual(back, v1) {
				t.Fatalf("Unadapt(Adapt(s)) = %+v, want %+v", back, v1)
			}
			again, err := json.Marshal(back)
			if err != nil {
				t.Fatal(err)
			}
			var want, got map[string]any
			_ = json.Unmarshal(raw, &want)
			_ = json.Unmarshal(again, &got)
			for key := range want {
				if _, ok := got[key]; !ok {
					t.Errorf("key %q was lost", key)
				}
			}
			for key := range got {
				if _, ok := want[key]; !ok {
					t.Errorf("key %q was added", key)
				}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("round trip changed the state\n got: %v\nwant: %v", got, want)
			}

			// The adapted state reads like the original.
			s := Adapt(v1)
			if s.Step != v1.Step || s.Loops[LoopRework] != v1.Rework || s.RoundBase != v1.RoundBase || s.WorkflowID != v1.WorkflowID ||
				s.ClosedAt != v1.ClosedAt || s.BlockedFrom != v1.BlockedFrom || s.PlanDone != v1.PlanDone || !reflect.DeepEqual(s.Vars, v1.Switches) {
				t.Fatalf("Adapt(%+v) = %+v", v1, s)
			}
			if s.Version != 1 || s.Seq != 0 {
				t.Fatalf("version %d seq %d, want a schema 1 state without a seq", s.Version, s.Seq)
			}
		})
	}

	t.Run("the maps are copied", func(t *testing.T) {
		v1 := FlowState{Step: "gate", Switches: map[string]any{"export": true}}
		s := Adapt(v1)
		s.Vars["export"] = false
		if v1.Switches["export"] != true {
			t.Fatal("Adapt shares the switches map")
		}
		back := Unadapt(s)
		back.Switches["export"] = true
		if s.Vars["export"] != false {
			t.Fatal("Unadapt shares the vars map")
		}
	})
	t.Run("a state with no switches stays without switches", func(t *testing.T) {
		if got := Unadapt(Adapt(FlowState{Step: "todo"})); got.Switches != nil || got.Stop != StopNone {
			t.Fatalf("got %+v", got)
		}
	})
}

func TestAdapt_StopCodes(t *testing.T) {
	tests := map[string]struct {
		stop Stop
		code string
	}{
		"stop 1":  {Stop1, "grounds_missing"},
		"stop 3":  {Stop3, "loop_exhausted"},
		"stalled": {StopStalled, "stalled"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			s := Adapt(FlowState{Step: "blocked", BlockedFrom: "gate", Stop: tt.stop})
			if s.Stop == nil || s.Stop.Code != tt.code || s.Stop.From != "gate" {
				t.Fatalf("Adapt stop = %+v, want %s from gate", s.Stop, tt.code)
			}
			if got := Unadapt(s).Stop; got != tt.stop {
				t.Fatalf("Unadapt stop = %q, want %q", got, tt.stop)
			}
		})
	}
	t.Run("no stop", func(t *testing.T) {
		if s := Adapt(FlowState{Step: "gate"}); s.Stop != nil {
			t.Fatalf("stop = %+v", s.Stop)
		}
		if got := Unadapt(State{Step: "gate"}).Stop; got != StopNone {
			t.Fatalf("stop = %q", got)
		}
	})
	t.Run("a code schema 1 does not have is stalled", func(t *testing.T) {
		if got := Unadapt(State{Step: "blocked", Stop: &StopRecord{Code: "custom_code"}}).Stop; got != StopStalled {
			t.Fatalf("stop = %q, want stalled", got)
		}
	})
	t.Run("the stop codes are the ones the lifted stops declare", func(t *testing.T) {
		g := mustLift(t, tradeFlow(), LiftOpts{})
		legacy := map[int]string{}
		for _, s := range g.Doc.Stops {
			legacy[s.LegacyNo] = s.Code
		}
		if legacy[1] != stopCodeOf(Stop1) || legacy[3] != stopCodeOf(Stop3) {
			t.Fatalf("legacy numbers %v vs adapt codes %q, %q", legacy, stopCodeOf(Stop1), stopCodeOf(Stop3))
		}
	})
}
