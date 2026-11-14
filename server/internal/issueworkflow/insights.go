package issueworkflow

import (
	"math"
	"sort"
	"time"
)

// The operating numbers of one version of a flow (spec 16.6): where its requests end, how
// long they stay in each step, which edges they take, how many rounds each loop runs, and why
// they stall. Insights is pure; the handler reads the states and the log and attaches the
// limits of the definition. Nothing here recommends a change.

// Report is the answer of flow-insights.
type Report struct {
	Requests RequestCounts  `json:"requests"`
	Steps    []StepInsight  `json:"steps"`
	Edges    []EdgeInsight  `json:"edges"`
	Loops    []LoopInsight  `json:"loops"`
	Stalls   []StallInsight `json:"stalls"`
	// ManualCloses counts the closes a member made for the assignee of a task.
	ManualCloses int `json:"manual_closes"`
	// Truncated is set by the caller when it read fewer requests than matched.
	Truncated bool `json:"truncated"`
}

// RequestCounts says how many requests there are and where the ones that are not running are.
// An end is "terminal:<key>", "stop:<code>" (a declared stop the request is parked at) or
// "stalled".
type RequestCounts struct {
	Total int            `json:"total"`
	Open  int            `json:"open"`
	ByEnd map[string]int `json:"by_end"`
}

// StepInsight is how often a step was entered and how long a visit that ended took. The
// minutes are null while no visit has a usable length.
type StepInsight struct {
	Key           string `json:"key"`
	Entered       int    `json:"entered"`
	MedianMinutes *int   `json:"median_minutes"`
	P90Minutes    *int   `json:"p90_minutes"`
}

// EdgeInsight is how often an edge was taken.
type EdgeInsight struct {
	Step  string `json:"step"`
	Edge  string `json:"edge"`
	Taken int    `json:"taken"`
}

// LoopInsight is how many requests used 0, 1, 2 … rounds of a loop. Max is the bound of the
// loop in the definition, which only the caller knows (WithLoopLimits).
type LoopInsight struct {
	Key       string `json:"key"`
	Max       int    `json:"max"`
	Histogram []int  `json:"histogram"`
}

// StallInsight is how many requests are stalled for a reason; "unknown" is a stall recorded
// before the reason was kept.
type StallInsight struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

// ManualClose is one close a member made for the assignee of a task, read from the activity
// log. A row with no task is a malformed row and is not counted.
type ManualClose struct {
	Step string
	Task string
}

// stallReasonUnknown names a stall whose reason was not recorded.
const stallReasonUnknown = "unknown"

// Insights computes the report of the states of the requests of one version. A trail entry
// that cannot be read is skipped, not an error: an entry with no step is dropped, one with
// no usable time counts as a visit of unknown length, and a visit that would be negative has
// no length.
//
// A visit starts when the entry before it was written, so the first entry of a trail has no
// length, and a step entered by a resume after a stop includes the time the request was
// parked.
func Insights(states []State, closes []ManualClose) Report {
	type edgeKey struct{ step, edge string }
	entered := map[string]int{}
	minutes := map[string][]time.Duration{}
	taken := map[edgeKey]int{}
	loopCounts := map[string]map[int]int{}
	stalls := map[string]int{}
	report := Report{Requests: RequestCounts{ByEnd: map[string]int{}}}

	for _, st := range states {
		report.Requests.Total++
		var prevAt time.Time
		prevOK := false
		for _, e := range st.Trail {
			at, err := time.Parse(time.RFC3339, e.At)
			timeOK := err == nil
			if e.Step == "" {
				prevAt, prevOK = time.Time{}, false
				continue
			}
			entered[e.Step]++
			if e.Edge != "" {
				taken[edgeKey{e.Step, e.Edge}]++
			}
			if timeOK && prevOK {
				if d := at.Sub(prevAt); d >= 0 {
					minutes[e.Step] = append(minutes[e.Step], d)
				}
			}
			prevAt, prevOK = at, timeOK
		}
		switch {
		case st.Stop != nil:
			if st.Stop.Code == DocStopStalled {
				report.Requests.ByEnd["stalled"]++
				reason := st.Stop.Reason
				if reason == "" {
					reason = stallReasonUnknown
				}
				stalls[reason]++
			} else {
				report.Requests.ByEnd["stop:"+st.Stop.Code]++
			}
		case st.ClosedAt != "":
			report.Requests.ByEnd["terminal:"+st.Step]++
		default:
			report.Requests.Open++
			// A visit in progress: the trail holds only the steps left.
			if st.Step != "" {
				entered[st.Step]++
			}
		}
		for key, n := range st.Loops {
			if n < 0 {
				continue
			}
			if loopCounts[key] == nil {
				loopCounts[key] = map[int]int{}
			}
			loopCounts[key][n]++
		}
	}

	report.Steps = make([]StepInsight, 0, len(entered))
	for key, n := range entered {
		step := StepInsight{Key: key, Entered: n}
		if ds := minutes[key]; len(ds) > 0 {
			sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
			median, p90 := roundMinutes(nearestRank(ds, 0.5)), roundMinutes(nearestRank(ds, 0.9))
			step.MedianMinutes, step.P90Minutes = &median, &p90
		}
		report.Steps = append(report.Steps, step)
	}
	sort.Slice(report.Steps, func(i, j int) bool { return report.Steps[i].Key < report.Steps[j].Key })

	report.Edges = make([]EdgeInsight, 0, len(taken))
	for k, n := range taken {
		report.Edges = append(report.Edges, EdgeInsight{Step: k.step, Edge: k.edge, Taken: n})
	}
	sort.Slice(report.Edges, func(i, j int) bool {
		a, b := report.Edges[i], report.Edges[j]
		if a.Step != b.Step {
			return a.Step < b.Step
		}
		return a.Edge < b.Edge
	})

	report.Loops = make([]LoopInsight, 0, len(loopCounts))
	for key, counts := range loopCounts {
		highest := 0
		for n := range counts {
			highest = max(highest, n)
		}
		histogram := make([]int, highest+1)
		withCount := 0
		for n, requests := range counts {
			histogram[n] = requests
			withCount += requests
		}
		// A request that never touched the loop used no rounds of it.
		histogram[0] += report.Requests.Total - withCount
		report.Loops = append(report.Loops, LoopInsight{Key: key, Histogram: histogram})
	}
	sort.Slice(report.Loops, func(i, j int) bool { return report.Loops[i].Key < report.Loops[j].Key })

	report.Stalls = make([]StallInsight, 0, len(stalls))
	for reason, n := range stalls {
		report.Stalls = append(report.Stalls, StallInsight{Reason: reason, Count: n})
	}
	sort.Slice(report.Stalls, func(i, j int) bool {
		a, b := report.Stalls[i], report.Stalls[j]
		if a.Count != b.Count {
			return a.Count > b.Count
		}
		return a.Reason < b.Reason
	})

	for _, c := range closes {
		if c.Task != "" {
			report.ManualCloses++
		}
	}
	return report
}

// WithLoopLimits sets the bound of each loop from the definition and gives every loop the
// definition declares a histogram long enough for its bound, including the loops no request
// has run yet. A loop of the report that the definition does not declare keeps a bound of 0.
func (r Report) WithLoopLimits(loops []Loop) Report {
	byKey := map[string]LoopInsight{}
	for _, l := range r.Loops {
		byKey[l.Key] = l
	}
	for _, l := range loops {
		insight, seen := byKey[l.Key]
		insight.Key = l.Key
		insight.Max = l.Max.Limit()
		if !seen {
			insight.Histogram = []int{r.Requests.Total}
		}
		for len(insight.Histogram) < insight.Max+1 {
			insight.Histogram = append(insight.Histogram, 0)
		}
		byKey[l.Key] = insight
	}
	out := make([]LoopInsight, 0, len(byKey))
	for _, l := range byKey {
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	r.Loops = out
	return r
}

// nearestRank is the p-th percentile of sorted values by the nearest-rank method.
func nearestRank(sorted []time.Duration, p float64) time.Duration {
	rank := int(math.Ceil(p * float64(len(sorted))))
	rank = min(max(rank, 1), len(sorted))
	return sorted[rank-1]
}

func roundMinutes(d time.Duration) int { return int(math.Round(d.Minutes())) }
