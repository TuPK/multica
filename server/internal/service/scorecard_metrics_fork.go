package service

import "time"

// Durable scorecard metrics (durable spec 2.2, 2.9; A5). They are computed from
// issue_status_transition and work_outcome, and they are present only when
// durable_outcomes is on. ComputeScorecard calls durableScorecardMetrics, which
// returns at once when ScorecardInputs.Durable is nil, so with the flag off the
// stored metrics are the Phase 4 keys and nothing else.

// Metric keys added by the durable platform.
const (
	MetricVerifiedPassRate = "verified_pass_rate"
	MetricVerifiedCoverage = "verified_coverage"
	MetricReworkRate       = "rework_rate"
	MetricHistoryPartial   = "history_partial"
)

// ScorecardDurableInputs is what the durable readers give one scorecard period.
type ScorecardDurableInputs struct {
	// CycleDays holds, per issue with a start transition, the days from its first
	// transition into started to its first transition into done in the period.
	CycleDays []float64
	// DoneEntries counts transitions into done in the period; Reopens counts
	// transitions out of done into a status that is neither done nor closed.
	DoneEntries int64
	Reopens     int64
	Outcomes    ScorecardOutcomeInputs
	// Unhistoried counts done issues with no done transition at all: they were done
	// before transitions were recorded, so the period is history_partial.
	Unhistoried int64
}

// ScorecardOutcomeInputs counts the work outcomes whose done entry is in the period.
type ScorecardOutcomeInputs struct {
	// Population is every outcome in the period, pending ones included.
	Population int64
	// Verified outcomes have a settled check verdict (verifiedOutcome).
	Verified int64
	// Pass counts the verified outcomes whose verdict is pass.
	Pass int64
}

// durableScorecardMetrics adds the durable keys to res. The rates are fractions,
// like the Phase 4 rates, and history_partial is 1 or 0.
func durableScorecardMetrics(in ScorecardInputs, res *ScorecardResult) {
	d := in.Durable
	if d == nil {
		return
	}
	// Replaces the Phase 4 cycle time (created to done) with the started-to-done one.
	cycle := ScorecardMetric{Sample: int64(len(d.CycleDays))}
	if len(d.CycleDays) > 0 {
		var sum float64
		for _, c := range d.CycleDays {
			sum += c
		}
		cycle.Value = f(round4(sum / float64(len(d.CycleDays))))
	}
	res.Metrics[MetricCycleTimeDays] = cycle

	res.Metrics[MetricReworkRate] = ratio(d.Reopens, d.DoneEntries)
	res.Metrics[MetricVerifiedPassRate] = ratio(d.Outcomes.Pass, d.Outcomes.Verified)
	res.Metrics[MetricVerifiedCoverage] = ratio(d.Outcomes.Verified, d.Outcomes.Population)

	var partial float64
	if d.Unhistoried > 0 {
		partial = 1
	}
	res.Metrics[MetricHistoryPartial] = ScorecardMetric{Value: f(partial), Sample: int64(len(in.Done))}
}

// verifiedOutcome reports whether a settled outcome has a check verdict: it was
// sampled, and its verdict is pass, partial or fail from its criteria. A reopen or
// a revert settles as fail with no check behind it, so it is not verified.
func verifiedOutcome(verdict, failCause string, sampled, settled bool) bool {
	if !sampled || !settled {
		return false
	}
	switch verdict {
	case "pass", "partial", "fail":
	default:
		return false
	}
	return failCause != "reopened" && failCause != "reverted"
}

// durableTransition is one issue_status_transition row with its lifecycle
// categories resolved (LifecycleCategoryOf). FromCategory is "" for a row with no
// from status.
type durableTransition struct {
	IssueID      string
	FromCategory string
	ToCategory   string
	At           time.Time
}

// durableHistory is what one issue's transitions say about a period.
type durableHistory struct {
	// FirstStarted is the first transition into started, at any time.
	FirstStarted time.Time
	HasStarted   bool
	// FirstDone is the first transition into done in the period.
	FirstDone time.Time
	HasDone   bool
	// DoneEntries and Reopens count the period's transitions into done and out of done.
	DoneEntries int64
	Reopens     int64
}

// durableHistories folds transitions, in any order, into one history per issue for
// the half-open period [start, end). Transitions outside the period still set the
// start, since a cycle time may begin before the period.
func durableHistories(transitions []durableTransition, start, end time.Time) map[string]*durableHistory {
	out := map[string]*durableHistory{}
	for _, tr := range transitions {
		h := out[tr.IssueID]
		if h == nil {
			h = &durableHistory{}
			out[tr.IssueID] = h
		}
		inPeriod := !tr.At.Before(start) && tr.At.Before(end)
		if tr.ToCategory == "started" && (!h.HasStarted || tr.At.Before(h.FirstStarted)) {
			h.FirstStarted, h.HasStarted = tr.At, true
		}
		if !inPeriod {
			continue
		}
		switch {
		case tr.ToCategory == "done":
			h.DoneEntries++
			if !h.HasDone || tr.At.Before(h.FirstDone) {
				h.FirstDone, h.HasDone = tr.At, true
			}
		case tr.FromCategory == "done" && tr.ToCategory != "closed":
			h.Reopens++
		}
	}
	return out
}

// durableCycleDays returns, for each issue with a start and a done in the period,
// the days from its first start to its first done in the period. A start after the
// done is not a cycle, so it is left out.
func durableCycleDays(histories map[string]*durableHistory) []float64 {
	var days []float64
	for _, h := range histories {
		if !h.HasDone || !h.HasStarted || h.FirstStarted.After(h.FirstDone) {
			continue
		}
		days = append(days, h.FirstDone.Sub(h.FirstStarted).Hours()/24)
	}
	return days
}
