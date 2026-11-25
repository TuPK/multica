package service

import (
	"fmt"
	"math"
	"testing"
	"time"
)

func TestWilsonLowerKnownValues(t *testing.T) {
	cases := []struct {
		name                string
		pass, partial, fail int
		want                float64
	}{
		{"ten passes", 10, 0, 0, 0.7225},
		{"eight pass two fail", 8, 0, 2, 0.4902},
		{"no outcomes", 0, 0, 0, 0},
		{"forty-five pass five fail", 45, 0, 5, 0.7864},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := WilsonLower(tc.pass, tc.partial, tc.fail)
			if math.Round(got*10000) != math.Round(tc.want*10000) {
				t.Fatalf("WilsonLower(%d,%d,%d) = %.6f, want %.4f", tc.pass, tc.partial, tc.fail, got, tc.want)
			}
		})
	}
}

func TestModeTransitions(t *testing.T) {
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	daysAgo := func(d int) time.Time { return now.AddDate(0, 0, -d) }

	ok30 := TrackRow{N: 10, ScoreLower: 0.80}
	ok90 := TrackRow{N: 30, ScoreLower: 0.90}

	cases := []struct {
		name       string
		cur        string
		r30, r90   TrackRow
		lastChange time.Time
		want       string
	}{
		{
			name: "full rises one step to sampled when r30 meets the bar",
			cur:  "full", r30: ok30, lastChange: daysAgo(8), want: "sampled",
		},
		{
			name: "full stays when r30 has fewer than 10 outcomes",
			cur:  "full", r30: TrackRow{N: 9, ScoreLower: 0.95}, lastChange: daysAgo(8), want: "full",
		},
		{
			name: "full stays when r30 score is below 0.80",
			cur:  "full", r30: TrackRow{N: 10, ScoreLower: 0.79}, lastChange: daysAgo(8), want: "full",
		},
		{
			name: "full does not change again within 7 days of the last change",
			cur:  "full", r30: ok30, lastChange: daysAgo(3), want: "full",
		},
		{
			name: "full may change exactly 7 days after the last change",
			cur:  "full", r30: ok30, lastChange: daysAgo(7), want: "sampled",
		},
		{
			name: "full with no prior change may rise",
			cur:  "full", r30: ok30, lastChange: time.Time{}, want: "sampled",
		},
		{
			name: "full never skips to spot in one step",
			cur:  "full", r30: ok30, r90: ok90, lastChange: daysAgo(8), want: "sampled",
		},
		{
			name: "full stays when r30 has a high fail",
			cur:  "full", r30: TrackRow{N: 10, ScoreLower: 0.95, NHighFail: 1}, lastChange: daysAgo(8), want: "full",
		},
		{
			name: "spot drops to full at once on a high fail, ignoring the 7-day limit",
			cur:  "spot", r30: TrackRow{N: 10, ScoreLower: 0.95, NHighFail: 1}, r90: ok90, lastChange: daysAgo(1), want: "full",
		},
		{
			name: "sampled rises to spot when r90 meets the bar",
			cur:  "sampled", r30: ok30, r90: ok90, lastChange: daysAgo(8), want: "spot",
		},
		{
			name: "sampled stays when r90 has fewer than 30 outcomes",
			cur:  "sampled", r30: ok30, r90: TrackRow{N: 29, ScoreLower: 0.95}, lastChange: daysAgo(8), want: "sampled",
		},
		{
			name: "sampled stays when r90 score is below 0.90",
			cur:  "sampled", r30: ok30, r90: TrackRow{N: 30, ScoreLower: 0.89}, lastChange: daysAgo(8), want: "sampled",
		},
		{
			name: "sampled stays when r90 has a high fail",
			cur:  "sampled", r30: ok30, r90: TrackRow{N: 30, ScoreLower: 0.95, NHighFail: 1}, lastChange: daysAgo(8), want: "sampled",
		},
		{
			name: "sampled does not rise to spot within 7 days of the last change",
			cur:  "sampled", r30: ok30, r90: ok90, lastChange: daysAgo(2), want: "sampled",
		},
		{
			name: "sampled falls one step to full when r30 score drops below 0.80",
			cur:  "sampled", r30: TrackRow{N: 20, ScoreLower: 0.75}, r90: ok90, lastChange: daysAgo(1), want: "full",
		},
		{
			name: "sampled drops to full at once on an r30 high fail",
			cur:  "sampled", r30: TrackRow{N: 20, ScoreLower: 0.95, NHighFail: 1}, r90: ok90, lastChange: daysAgo(8), want: "full",
		},
		{
			name: "spot falls one step to sampled when r90 score drops below 0.90",
			cur:  "spot", r30: ok30, r90: TrackRow{N: 40, ScoreLower: 0.85}, lastChange: daysAgo(1), want: "sampled",
		},
		{
			name: "spot falls only one step even when both scores are low",
			cur:  "spot", r30: TrackRow{N: 10, ScoreLower: 0.50}, r90: TrackRow{N: 40, ScoreLower: 0.50}, lastChange: daysAgo(1), want: "sampled",
		},
		{
			name: "spot holds when r90 meets the bar",
			cur:  "spot", r30: ok30, r90: ok90, lastChange: daysAgo(1), want: "spot",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NextVerificationMode(tc.cur, tc.r30, tc.r90, tc.lastChange, now)
			if got != tc.want {
				t.Fatalf("NextVerificationMode(%q) = %q, want %q", tc.cur, got, tc.want)
			}
		})
	}
}

func TestSampledDeterministic(t *testing.T) {
	for _, mode := range []string{"full", "sampled", "spot"} {
		first := Sampled("out-42", mode)
		for i := 0; i < 5; i++ {
			if Sampled("out-42", mode) != first {
				t.Fatalf("Sampled(out-42, %q) changed between calls", mode)
			}
		}
	}

	if !Sampled("any-outcome", "full") {
		t.Fatal("full mode must select every outcome")
	}

	const total = 10000
	for _, tc := range []struct {
		mode string
		want float64
	}{
		{"sampled", 0.30},
		{"spot", 0.10},
	} {
		picked := 0
		for i := 0; i < total; i++ {
			if Sampled(fmt.Sprintf("out-%d", i), tc.mode) {
				picked++
			}
		}
		ratio := float64(picked) / total
		if math.Abs(ratio-tc.want) > 0.02 {
			t.Fatalf("mode %s: selected ratio %.4f, want %.2f ± 0.02", tc.mode, ratio, tc.want)
		}
	}
}
