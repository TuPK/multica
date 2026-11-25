package service

import (
	"context"
	"math"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// durableUUID parses a fixture id, failing the test when it is not one.
func durableUUID(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	parsed, err := util.ParseUUID(value)
	if err != nil {
		t.Fatalf("parse uuid %q: %v", value, err)
	}
	return parsed
}

// durableNow falls in the week of 25 Nov 2026, so the last completed ISO week is
// Mon 16 Nov (inclusive) to Mon 23 Nov (exclusive). Fixture times below are placed
// inside it unless a test says otherwise.
var durableNow = time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)

func durableAt(day, hour int) time.Time {
	return time.Date(2026, 11, day, hour, 0, 0, 0, time.UTC)
}

// scorecardService returns a ScorecardService whose durable_outcomes flag is on or
// off for the whole test.
func scorecardService(f principalFixture, flagOn bool) *ScorecardService {
	provider := featureflag.NewStaticProvider()
	provider.Set(featureflags.DurableOutcomes, featureflag.Rule{Default: flagOn})
	return &ScorecardService{
		Queries: f.q,
		Now:     func() time.Time { return durableNow },
		Flags:   featureflag.NewService(provider),
	}
}

// durableAgentScope is the agent scope of agentID.
func durableAgentScope(t *testing.T, agentID string) ScorecardScope {
	t.Helper()
	id := durableUUID(t, agentID)
	return ScorecardScope{SubjectType: ScorecardSubjectAgent, SubjectID: id, AgentID: id}
}

// durableIssue creates an issue assigned to agentID in status, with the stored done
// time updatedAt (the proxy the scorecard read before A5). Its transitions are
// inserted with durableTransition; the trigger is not used, so times are exact.
func durableIssue(t *testing.T, f principalFixture, agentID, status string, updatedAt time.Time) string {
	t.Helper()
	return f.Issue(t, "durable scorecard", testutil.Cols{
		"status":        status,
		"assignee_type": "agent",
		"assignee_id":   agentID,
		"created_at":    durableAt(10, 9),
		"updated_at":    updatedAt,
	})
}

// insertTransition inserts one issue_status_transition row. from is "" for a
// row with no from status.
func insertTransition(t *testing.T, f principalFixture, issue, from, to string, at time.Time) {
	t.Helper()
	cols := testutil.Cols{
		"workspace_id":  f.WorkspaceID,
		"issue_id":      issue,
		"to_status_key": to,
		"actor_type":    "system",
		"source":        "live",
		"created_at":    at,
	}
	if from != "" {
		cols["from_status_key"] = from
	}
	f.Insert(t, "issue_status_transition", cols)
}

// durableAgent returns a fresh agent in the fixture's workspace.
func durableAgent(t *testing.T, f principalFixture, name string) string {
	t.Helper()
	runtime := f.Runtime(t, name+"-runtime")
	return f.Agent(t, name, runtime)
}

// computeAgentWeek computes the agent's last completed week and decodes it.
func computeAgentWeek(t *testing.T, svc *ScorecardService, f principalFixture, agentID string) map[string]ScorecardMetric {
	t.Helper()
	sc, err := svc.Compute(context.Background(), f.q, durableUUID(t, f.WorkspaceID), durableAgentScope(t, agentID), ScorecardPeriodWeek)
	if err != nil {
		t.Fatalf("compute scorecard: %v", err)
	}
	doc, err := DecodeScorecardDocument(sc.Metrics)
	if err != nil {
		t.Fatalf("decode scorecard: %v", err)
	}
	return doc.Metrics
}

// requireValue fails unless the metric has the value want (to four places) and the sample.
func requireValue(t *testing.T, m map[string]ScorecardMetric, key string, want float64, sample int64) {
	t.Helper()
	got, ok := m[key]
	if !ok {
		t.Fatalf("metric %s missing; have %v", key, metricKeys(m))
	}
	if got.Value == nil {
		t.Fatalf("metric %s has no value, want %v", key, want)
	}
	if math.Abs(*got.Value-want) > 1e-4 {
		t.Fatalf("metric %s = %v, want %v", key, *got.Value, want)
	}
	if got.Sample != sample {
		t.Fatalf("metric %s sample = %d, want %d", key, got.Sample, sample)
	}
}

func metricKeys(m map[string]ScorecardMetric) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// TestScorecardUsesDoneTransition: the done time is the first transition into done
// in the period, not updated_at. Issue X entered done inside the week but was
// edited after it, so updated_at falls outside; Y's updated_at is inside the week
// but it entered done after it. Only X is in the week's done set.
func TestScorecardUsesDoneTransition(t *testing.T) {
	f, _ := newPrincipalFixture(t)
	agent := durableAgent(t, f, "done-transition")

	x := durableIssue(t, f, agent, "done", durableAt(27, 9))
	insertTransition(t, f, x, "todo", "in_progress", durableAt(16, 9))
	insertTransition(t, f, x, "in_progress", "done", durableAt(18, 10))

	y := durableIssue(t, f, agent, "done", durableAt(18, 9))
	insertTransition(t, f, y, "in_progress", "done", durableAt(24, 9))

	m := computeAgentWeek(t, scorecardService(f, true), f, agent)
	requireValue(t, m, MetricIssuesDone, 1, 1)
	// X: first started 16 Nov 09:00, done 18 Nov 10:00 → 2 days 1 hour.
	requireValue(t, m, MetricCycleTimeDays, 49.0/24, 1)
}

// TestReworkRateFromTransitions: rework is transitions out of done in the week over
// transitions into done in the week. R goes done, reopened, done again; S goes done
// once. Two reopen-free entries plus one reopen over three entries.
func TestReworkRateFromTransitions(t *testing.T) {
	f, _ := newPrincipalFixture(t)
	agent := durableAgent(t, f, "rework")

	r := durableIssue(t, f, agent, "done", durableAt(19, 9))
	insertTransition(t, f, r, "todo", "in_progress", durableAt(16, 8))
	insertTransition(t, f, r, "in_progress", "done", durableAt(17, 9))
	insertTransition(t, f, r, "done", "in_progress", durableAt(18, 9))
	insertTransition(t, f, r, "in_progress", "done", durableAt(19, 9))

	s := durableIssue(t, f, agent, "done", durableAt(20, 10))
	insertTransition(t, f, s, "todo", "in_progress", durableAt(16, 10))
	insertTransition(t, f, s, "in_progress", "done", durableAt(20, 10))

	m := computeAgentWeek(t, scorecardService(f, true), f, agent)
	// Reopens 1 over done entries 3. The reopen_count column is 0 here, so only the
	// transitions can produce this.
	requireValue(t, m, MetricReworkRate, 1.0/3, 3)
	requireValue(t, m, MetricIssuesDone, 2, 2)
}

// TestCycleTimeFromFirstStartedToFirstDone: cycle time runs from the first
// transition into started, which may be before the week, to the done entry in the
// week. An issue with no started transition has no cycle time and is left out of
// the sample.
func TestCycleTimeFromFirstStartedToFirstDone(t *testing.T) {
	f, _ := newPrincipalFixture(t)
	agent := durableAgent(t, f, "cycle")

	// Started in the week, blocked, done: first start 16 Nov 09:00, done 19 Nov 09:00.
	c := durableIssue(t, f, agent, "done", durableAt(19, 9))
	insertTransition(t, f, c, "todo", "in_progress", durableAt(16, 9))
	insertTransition(t, f, c, "in_progress", "blocked", durableAt(17, 9))
	insertTransition(t, f, c, "blocked", "done", durableAt(19, 9))

	// Started before the week: first start 10 Nov 09:00, done 19 Nov 09:00 → 9 days.
	e := durableIssue(t, f, agent, "done", durableAt(19, 9))
	insertTransition(t, f, e, "todo", "in_progress", durableAt(10, 9))
	insertTransition(t, f, e, "in_progress", "done", durableAt(19, 9))

	// Never started: done straight from todo. No cycle time.
	n := durableIssue(t, f, agent, "done", durableAt(18, 9))
	insertTransition(t, f, n, "todo", "done", durableAt(18, 9))

	m := computeAgentWeek(t, scorecardService(f, true), f, agent)
	requireValue(t, m, MetricCycleTimeDays, (3+9)/2.0, 2)
	requireValue(t, m, MetricIssuesDone, 3, 3)
}

// TestHistoryPartialForPreFlagWork: work done before the transitions were recorded
// has no done transition. It still counts, by its stored done time, and the period
// is marked history_partial. A period whose done work all has transitions is not.
func TestHistoryPartialForPreFlagWork(t *testing.T) {
	f, _ := newPrincipalFixture(t)
	preFlag := durableAgent(t, f, "pre-flag")
	withHistory := durableAgent(t, f, "with-history")

	// No transition at all: done before tracking.
	durableIssue(t, f, preFlag, "done", durableAt(18, 9))

	h := durableIssue(t, f, withHistory, "done", durableAt(18, 9))
	insertTransition(t, f, h, "in_progress", "done", durableAt(18, 9))

	svc := scorecardService(f, true)
	pre := computeAgentWeek(t, svc, f, preFlag)
	requireValue(t, pre, MetricIssuesDone, 1, 1)
	requireValue(t, pre, MetricHistoryPartial, 1, 1)

	clean := computeAgentWeek(t, svc, f, withHistory)
	requireValue(t, clean, MetricHistoryPartial, 0, 1)
}

// TestScorecardUnchangedWhenFlagOff: with durable_outcomes off the scorecard keeps
// exactly the Phase 4 keys, even with transitions and outcomes in the fixture, and
// cycle time is still the proxy (created to updated_at).
func TestScorecardUnchangedWhenFlagOff(t *testing.T) {
	f, _ := newPrincipalFixture(t)
	agent := durableAgent(t, f, "flag-off")

	issue := f.Issue(t, "flag off", testutil.Cols{
		"status":        "done",
		"assignee_type": "agent",
		"assignee_id":   agent,
		"created_at":    durableAt(10, 0),
		"updated_at":    durableAt(18, 0),
	})
	insertTransition(t, f, issue, "todo", "in_progress", durableAt(11, 0))
	insertTransition(t, f, issue, "in_progress", "done", durableAt(18, 0))

	m := computeAgentWeek(t, scorecardService(f, false), f, agent)

	// The Phase 4 keys for an agent scope (no routines, no brief).
	want := []string{
		MetricRuns, MetricRunsUncharged, MetricSuccessRate, MetricIssuesDone,
		MetricFirstPassRate, MetricCycleTimeDays, MetricCostPerDoneUSD, MetricUnpricedTokens,
		MetricHumanInterventionRate, MetricProposalAcceptance, MetricKnowledgePublished,
		MetricKnowledgeReviews,
	}
	sort.Strings(want)
	if got := metricKeys(m); !equalStrings(got, want) {
		t.Fatalf("flag off keys = %v, want the Phase 4 keys %v", got, want)
	}
	// The proxy: created 10 Nov 00:00 to updated_at 18 Nov 00:00 is 8 days.
	requireValue(t, m, MetricCycleTimeDays, 8, 1)
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// The tests below need no database. They pin the history arithmetic, the verified
// rule and the flag-off guard, so they run where the DB tests skip.

func pureTransition(issue, from, to string, at time.Time) durableTransition {
	return durableTransition{IssueID: issue, FromCategory: from, ToCategory: to, At: at}
}

func TestDurableHistoriesCountsPeriodTransitions(t *testing.T) {
	start, end := durableAt(16, 0), durableAt(23, 0)
	histories := durableHistories([]durableTransition{
		pureTransition("a", "unstarted", "started", durableAt(10, 9)), // before the period: still the first start
		pureTransition("a", "started", "done", durableAt(17, 9)),
		pureTransition("a", "done", "started", durableAt(18, 9)), // reopen
		pureTransition("a", "started", "done", durableAt(19, 9)),
		pureTransition("a", "done", "closed", durableAt(20, 9)), // cancelled from done: not a reopen
		pureTransition("b", "started", "done", durableAt(9, 9)), // before the period: not counted
	}, start, end)

	a := histories["a"]
	if !a.HasStarted || !a.FirstStarted.Equal(durableAt(10, 9)) {
		t.Fatalf("first start = %v (has %v), want 10 Nov 09:00", a.FirstStarted, a.HasStarted)
	}
	if a.DoneEntries != 2 || !a.FirstDone.Equal(durableAt(17, 9)) {
		t.Fatalf("done entries %d first done %v, want 2 and 17 Nov 09:00", a.DoneEntries, a.FirstDone)
	}
	if a.Reopens != 1 {
		t.Fatalf("reopens = %d, want 1 (the closed transition is not a reopen)", a.Reopens)
	}
	b := histories["b"]
	if b.HasDone || b.DoneEntries != 0 {
		t.Fatalf("done before the period was counted: %+v", b)
	}
}

func TestDurableCycleDaysNeedStartBeforeDone(t *testing.T) {
	histories := map[string]*durableHistory{
		"ok":       {HasStarted: true, FirstStarted: durableAt(10, 0), HasDone: true, FirstDone: durableAt(12, 0)},
		"no-start": {HasDone: true, FirstDone: durableAt(12, 0)},
		"after":    {HasStarted: true, FirstStarted: durableAt(13, 0), HasDone: true, FirstDone: durableAt(12, 0)},
	}
	days := durableCycleDays(histories)
	if len(days) != 1 || days[0] != 2 {
		t.Fatalf("cycle days = %v, want [2]", days)
	}
}

func TestVerifiedOutcomeRule(t *testing.T) {
	cases := []struct {
		verdict, cause   string
		sampled, settled bool
		want             bool
	}{
		{"pass", "", true, true, true},
		{"partial", "", true, true, true},
		{"fail", "criteria", true, true, true},
		{"fail", "reopened", true, true, false},
		{"fail", "reverted", true, true, false},
		{"pass", "", false, true, false},
		{"pass", "", true, false, false},
		{"unverified", "", true, true, false},
		{"pending", "", true, true, false},
	}
	for _, c := range cases {
		if got := verifiedOutcome(c.verdict, c.cause, c.sampled, c.settled); got != c.want {
			t.Errorf("verifiedOutcome(%s,%s,sampled=%v,settled=%v) = %v, want %v", c.verdict, c.cause, c.sampled, c.settled, got, c.want)
		}
	}
}

func TestDurableMetricsAddNothingWithoutDurableInputs(t *testing.T) {
	res := ComputeScorecard(ScorecardInputs{Runs: ScorecardRunInputs{Completed: 5}})
	before := metricKeys(res.Metrics)
	durableScorecardMetrics(ScorecardInputs{}, &res)
	if got := metricKeys(res.Metrics); !equalStrings(got, before) {
		t.Fatalf("keys changed with no durable inputs: %v -> %v", before, got)
	}
}

func TestDurableMetricsRates(t *testing.T) {
	res := ComputeScorecard(ScorecardInputs{Runs: ScorecardRunInputs{Completed: 5}})
	durableScorecardMetrics(ScorecardInputs{
		Done: make([]ScorecardIssueRow, 4),
		Durable: &ScorecardDurableInputs{
			CycleDays:   []float64{2, 4},
			DoneEntries: 4,
			Reopens:     1,
			Outcomes:    ScorecardOutcomeInputs{Population: 8, Verified: 4, Pass: 3},
			Unhistoried: 1,
		},
	}, &res)
	requireValue(t, res.Metrics, MetricCycleTimeDays, 3, 2)
	requireValue(t, res.Metrics, MetricReworkRate, 0.25, 4)
	requireValue(t, res.Metrics, MetricVerifiedPassRate, 0.75, 4)
	requireValue(t, res.Metrics, MetricVerifiedCoverage, 0.5, 8)
	requireValue(t, res.Metrics, MetricHistoryPartial, 1, 4)
}
