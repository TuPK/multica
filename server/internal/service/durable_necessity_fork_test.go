package service

import "testing"

func runOf(c Comparison, n int) []Comparison {
	out := make([]Comparison, n)
	for i := range out {
		out[i] = c
	}
	return out
}

func joinRuns(runs ...[]Comparison) []Comparison {
	var out []Comparison
	for _, r := range runs {
		out = append(out, r...)
	}
	return out
}

func TestNecessityVerdictTable(t *testing.T) {
	const minPairs = 20
	cases := []struct {
		name        string
		comparisons []Comparison
		margin      float64
		want        string
	}{
		{
			// Spec 10.5 item 2 asks for not_needed here. At margin 0.05 that is not reachable:
			// the 95% upper bound for d at n=40 is 0.0876, so the verdict is inconclusive at 2×minPairs.
			name:        "40 ties at default margin 0.05 is inconclusive",
			comparisons: runOf(ComparisonTie, 40),
			margin:      0.05,
			want:        "inconclusive",
		},
		{
			name:        "40 ties with margin 0.10 is not_needed",
			comparisons: runOf(ComparisonTie, 40),
			margin:      0.10,
			want:        "not_needed",
		},
		{
			name:        "80 ties at margin 0.05 is not_needed",
			comparisons: runOf(ComparisonTie, 80),
			margin:      0.05,
			want:        "not_needed",
		},
		{
			name:        "40 pairs with 30% primary wins is needed",
			comparisons: joinRuns(runOf(ComparisonPrimary, 12), runOf(ComparisonTie, 28)),
			margin:      0.05,
			want:        "needed",
		},
		{
			name:        "exactly minPairs decisive pairs is needed",
			comparisons: joinRuns(runOf(ComparisonPrimary, 12), runOf(ComparisonTie, 8)),
			margin:      0.05,
			want:        "needed",
		},
		{
			name:        "19 ties is below minPairs, running",
			comparisons: runOf(ComparisonTie, 19),
			margin:      0.05,
			want:        "running",
		},
		{
			name:        "25 undecided pairs between minPairs and 2×minPairs is running",
			comparisons: joinRuns(runOf(ComparisonPrimary, 2), runOf(ComparisonShadow, 2), runOf(ComparisonTie, 21)),
			margin:      0.05,
			want:        "running",
		},
		{
			name:        "40 undecided pairs at 2×minPairs is inconclusive",
			comparisons: joinRuns(runOf(ComparisonPrimary, 4), runOf(ComparisonShadow, 3), runOf(ComparisonTie, 33)),
			margin:      0.05,
			want:        "inconclusive",
		},
		{
			// Unable pairs must not count toward n. Counted, n would be 50 and the verdict inconclusive.
			name:        "unable pairs are excluded from n: 30 ties + 20 unable is running",
			comparisons: joinRuns(runOf(ComparisonTie, 30), runOf(ComparisonUnable, 20)),
			margin:      0.05,
			want:        "running",
		},
		{
			// Counted, the 12 primary wins among 40 pairs would make this needed.
			name:        "unable pairs are excluded from n: 12 primary + 28 unable is running",
			comparisons: joinRuns(runOf(ComparisonPrimary, 12), runOf(ComparisonUnable, 28)),
			margin:      0.05,
			want:        "running",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NecessityVerdict(tc.comparisons, tc.margin, minPairs); got != tc.want {
				t.Errorf("NecessityVerdict = %q, want %q", got, tc.want)
			}
		})
	}
}
