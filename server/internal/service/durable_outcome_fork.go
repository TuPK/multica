package service

// CriterionResult is one criterion check, as DecideVerdict reads it. Result is
// pass, partial, fail or unable; Severity is none, low, medium or high ("" is
// read as none).
type CriterionResult struct {
	Result   string
	Severity string
}

// VerdictInput is what settlement knows when it decides an outcome's verdict.
type VerdictInput struct {
	Sampled  bool
	Reopened bool
	Results  []CriterionResult
}

// VerdictResult is the settled verdict. FailCause is "reopened" or "criteria"
// when Verdict is fail, and "" otherwise.
type VerdictResult struct {
	Verdict   string
	Severity  string
	FailCause string
}

var severityRank = map[string]int{"none": 0, "low": 1, "medium": 2, "high": 3}

// DecideVerdict applies the table in spec 2.6; the first matching row wins.
// "unable" results are not counted, including toward severity.
func DecideVerdict(in VerdictInput) VerdictResult {
	var pass, partial, fail int
	severity := "none"
	for _, r := range in.Results {
		switch r.Result {
		case "pass":
			pass++
		case "partial":
			partial++
		case "fail":
			fail++
		default:
			continue
		}
		if severityRank[r.Severity] > severityRank[severity] {
			severity = r.Severity
		}
	}

	switch {
	case in.Reopened:
		return VerdictResult{Verdict: "fail", Severity: severity, FailCause: "reopened"}
	case !in.Sampled || pass+partial+fail == 0:
		return VerdictResult{Verdict: "unverified", Severity: severity}
	case fail > 0:
		return VerdictResult{Verdict: "fail", Severity: severity, FailCause: "criteria"}
	case partial > 0:
		return VerdictResult{Verdict: "partial", Severity: severity}
	default:
		return VerdictResult{Verdict: "pass", Severity: severity}
	}
}
