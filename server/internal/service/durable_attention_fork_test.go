package service

import "testing"

func TestEstimateMinutesTable(t *testing.T) {
	cases := []struct {
		name         string
		kind         string
		words        int
		irreversible bool
		conf         float64
		want         float64
	}{
		{"proposal with empty body", "proposal", 0, false, 0, 3},
		{"proposal 399 words adds nothing", "proposal", 399, false, 0, 3},
		{"proposal 400 words adds one minute", "proposal", 400, false, 0, 4},
		{"proposal 1000 words adds two minutes", "proposal", 1000, false, 0, 5},
		{"org_change", "org_change", 0, false, 0, 6},
		{"effect_intent reversible", "effect_intent", 0, false, 0, 1},
		{"effect_intent irreversible", "effect_intent", 0, true, 0, 2},
		{"outcome_check manual", "outcome_check", 0, false, 0, 4},
		{"app_action_request", "app_action_request", 0, false, 0, 1},
		{"inquiry", "inquiry", 0, false, 0, 2},
		{"unknown kind is zero even when irreversible and long", "bogus", 900, true, 0.9, 0},
		{"confidence 0.8 halves", "org_change", 0, false, 0.8, 3},
		{"confidence 0.79 leaves it unchanged", "org_change", 0, false, 0.79, 6},
		{"confidence 0.8 halves proposal with words", "proposal", 400, false, 0.8, 2},
		{"confidence 0.9 halves irreversible intent", "effect_intent", 0, true, 0.9, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EstimateMinutes(tc.kind, tc.words, tc.irreversible, tc.conf, 1)
			if got != tc.want {
				t.Fatalf("EstimateMinutes(%q, %d, %v, %v, 1) = %v, want %v",
					tc.kind, tc.words, tc.irreversible, tc.conf, got, tc.want)
			}
		})
	}
}

func TestEstimateMinutesBatchOfSameKind(t *testing.T) {
	cases := []struct {
		name  string
		kind  string
		conf  float64
		batch int
		want  float64
	}{
		{"three proposals with empty body", "proposal", 0, 3, 4},
		{"three org changes", "org_change", 0, 3, 7},
		{"two org changes at confidence 0.9", "org_change", 0.9, 2, 3.5},
		{"unknown kind stays zero in a batch", "bogus", 0, 3, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EstimateMinutes(tc.kind, 0, false, tc.conf, tc.batch)
			if got != tc.want {
				t.Fatalf("EstimateMinutes(%q, conf=%v, batch=%d) = %v, want %v",
					tc.kind, tc.conf, tc.batch, got, tc.want)
			}
		})
	}
}

func TestBatchMinutes(t *testing.T) {
	cases := []struct {
		name   string
		perAsk []float64
		want   float64
	}{
		{"empty batch", nil, 0},
		{"single ask", []float64{5}, 5},
		{"largest plus half a minute per extra ask", []float64{3, 6, 1}, 7},
		{"equal asks", []float64{2, 2, 2, 2}, 3.5},
		{"fractional asks", []float64{0.5, 4}, 4.5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := BatchMinutes(tc.perAsk); got != tc.want {
				t.Fatalf("BatchMinutes(%v) = %v, want %v", tc.perAsk, got, tc.want)
			}
		})
	}
}
