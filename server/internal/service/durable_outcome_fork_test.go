package service

import "testing"

// TestDecideVerdictTable walks the rows of spec 2.6 in table order: the first
// matching row wins. "unable" results are ignored everywhere, including in
// severity.
func TestDecideVerdictTable(t *testing.T) {
	pass := CriterionResult{Result: "pass"}
	partial := CriterionResult{Result: "partial"}
	fail := CriterionResult{Result: "fail"}
	unable := CriterionResult{Result: "unable", Severity: "high"}

	cases := []struct {
		name string
		in   VerdictInput
		want VerdictResult
	}{
		{
			name: "reopened is fail/reopened with no results",
			in:   VerdictInput{Sampled: true, Reopened: true},
			want: VerdictResult{Verdict: "fail", Severity: "none", FailCause: "reopened"},
		},
		{
			name: "reopened beats unsampled",
			in:   VerdictInput{Sampled: false, Reopened: true},
			want: VerdictResult{Verdict: "fail", Severity: "none", FailCause: "reopened"},
		},
		{
			name: "reopened beats passing results",
			in:   VerdictInput{Sampled: true, Reopened: true, Results: []CriterionResult{pass}},
			want: VerdictResult{Verdict: "fail", Severity: "none", FailCause: "reopened"},
		},
		{
			name: "not sampled and not reopened is unverified",
			in:   VerdictInput{Sampled: false, Reopened: false},
			want: VerdictResult{Verdict: "unverified", Severity: "none"},
		},
		{
			// Table order: the sampled=false row comes before the fail row, so
			// no result can settle the verdict when nothing was sampled.
			name: "not sampled stays unverified even with a fail result",
			in:   VerdictInput{Sampled: false, Results: []CriterionResult{fail}},
			want: VerdictResult{Verdict: "unverified", Severity: "none"},
		},
		{
			name: "sampled with no results is unverified",
			in:   VerdictInput{Sampled: true},
			want: VerdictResult{Verdict: "unverified", Severity: "none"},
		},
		{
			name: "only unable results is unverified",
			in:   VerdictInput{Sampled: true, Results: []CriterionResult{unable, unable}},
			want: VerdictResult{Verdict: "unverified", Severity: "none"},
		},
		{
			name: "all pass is pass",
			in:   VerdictInput{Sampled: true, Results: []CriterionResult{pass, pass}},
			want: VerdictResult{Verdict: "pass", Severity: "none"},
		},
		{
			name: "unable is ignored, so pass stays pass and severity stays none",
			in:   VerdictInput{Sampled: true, Results: []CriterionResult{pass, unable}},
			want: VerdictResult{Verdict: "pass", Severity: "none"},
		},
		{
			name: "any fail is fail/criteria",
			in:   VerdictInput{Sampled: true, Results: []CriterionResult{pass, fail}},
			want: VerdictResult{Verdict: "fail", Severity: "none", FailCause: "criteria"},
		},
		{
			name: "fail beats partial",
			in:   VerdictInput{Sampled: true, Results: []CriterionResult{partial, fail}},
			want: VerdictResult{Verdict: "fail", Severity: "none", FailCause: "criteria"},
		},
		{
			name: "partial without fail is partial",
			in:   VerdictInput{Sampled: true, Results: []CriterionResult{pass, partial}},
			want: VerdictResult{Verdict: "partial", Severity: "none"},
		},
		{
			name: "severity is the highest across results",
			in: VerdictInput{Sampled: true, Results: []CriterionResult{
				{Result: "pass", Severity: "low"},
				{Result: "partial", Severity: "medium"},
				{Result: "pass", Severity: "none"},
			}},
			want: VerdictResult{Verdict: "partial", Severity: "medium"},
		},
		{
			name: "severity of a fail counts toward the max",
			in: VerdictInput{Sampled: true, Results: []CriterionResult{
				{Result: "pass", Severity: "low"},
				{Result: "fail", Severity: "high"},
			}},
			want: VerdictResult{Verdict: "fail", Severity: "high", FailCause: "criteria"},
		},
		{
			name: "severity of an unable result is not counted",
			in: VerdictInput{Sampled: true, Results: []CriterionResult{
				{Result: "pass", Severity: "low"},
				unable,
			}},
			want: VerdictResult{Verdict: "pass", Severity: "low"},
		},
		{
			name: "reopened still reports the max severity of its results",
			in: VerdictInput{Sampled: true, Reopened: true, Results: []CriterionResult{
				{Result: "partial", Severity: "medium"},
			}},
			want: VerdictResult{Verdict: "fail", Severity: "medium", FailCause: "reopened"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DecideVerdict(tc.in); got != tc.want {
				t.Fatalf("DecideVerdict(%+v) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}
