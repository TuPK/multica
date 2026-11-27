package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

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

// attentionFixture is a workspace with one live squad. The flag provider stays with the
// test, so the test can turn durable_principal on or off.
type attentionFixture struct {
	principalFixture
	attention *AttentionService
	ws        pgtype.UUID
	squad     pgtype.UUID
}

func newAttentionFixture(t *testing.T, flagOn bool) attentionFixture {
	t.Helper()
	f, _ := newPrincipalFixture(t)
	provider := featureflag.NewStaticProvider()
	provider.Set(featureflags.DurablePrincipal, featureflag.Rule{Default: flagOn})
	f.Cleanup(t, `DELETE FROM attention_ledger WHERE workspace_id = $1`, f.WorkspaceID)
	runtime := f.Runtime(t, "attention runtime", testutil.Cols{"provider": "codex"})
	leader := f.Agent(t, "attention leader", runtime)
	squad := f.Squad(t, "attention squad", leader)
	return attentionFixture{
		principalFixture: f,
		attention:        &AttentionService{Queries: f.q, Flags: featureflag.NewService(provider)},
		ws:               attentionUUID(t, f.WorkspaceID),
		squad:            attentionUUID(t, squad),
	}
}

func attentionUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	u, err := util.ParseUUID(s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return u
}

func newAttentionSubject() pgtype.UUID { return pgtype.UUID{Bytes: uuid.New(), Valid: true} }

// outcomeAsk is a system question about a manual check: 4 minutes in the table.
func (f attentionFixture) outcomeAsk() AskInput {
	return AskInput{WorkspaceID: f.ws, SquadID: f.squad, SubjectType: "outcome_check", SubjectID: newAttentionSubject(), Reason: "verification"}
}

// squadMinutes is what the squad has spent this week, as the budget reads it.
func (f attentionFixture) squadMinutes(t *testing.T) float64 {
	t.Helper()
	minutes, err := f.q.SumSquadAttentionMinutes(context.Background(), db.SumSquadAttentionMinutesParams{
		WorkspaceID: f.ws, SquadID: f.squad, WeekStart: pgtype.Date{Time: attentionWeekStart(time.Now()), Valid: true},
	})
	if err != nil {
		t.Fatalf("sum squad minutes: %v", err)
	}
	return minutes
}

// ledgerRow reads one question's state and stored minutes.
func (f attentionFixture) ledgerRow(t *testing.T, subject pgtype.UUID) (state string, minutes float64) {
	t.Helper()
	err := f.Pool.QueryRow(context.Background(),
		`SELECT state, estimated_minutes::float8 FROM attention_ledger WHERE subject_id = $1`, subject).Scan(&state, &minutes)
	if err != nil {
		t.Fatalf("read ledger row: %v", err)
	}
	return state, minutes
}

// TestSquadShareExhaustedQueues: a squad past its share queues the next question. The
// question stays out of the inbox and the chat, and it is listed for the brief.
func TestSquadShareExhaustedQueues(t *testing.T) {
	f := newAttentionFixture(t, true)
	ctx := context.Background()
	// The squad's whole share is 7 minutes; a manual check costs 4.
	f.Exec(t, `UPDATE workspace SET attention_minutes_per_week = 7 WHERE id = $1`, f.ws)

	fits, over, again := f.outcomeAsk(), f.outcomeAsk(), f.outcomeAsk()
	for _, tc := range []struct {
		name string
		ask  AskInput
		want string
	}{
		{"first question fits the share", fits, AttentionDelivered},
		{"second question passes the share", over, AttentionQueued},
		{"a third question queues too", again, AttentionQueued},
	} {
		got, err := f.attention.Admit(ctx, tc.ask)
		if err != nil {
			t.Fatalf("%s: admit: %v", tc.name, err)
		}
		if got != tc.want {
			t.Fatalf("%s: state = %q, want %q", tc.name, got, tc.want)
		}
	}

	if spent := f.squadMinutes(t); spent != 4 {
		t.Fatalf("queued questions must not spend minutes: squad spent %v, want 4", spent)
	}
	queued, err := QueuedAttentionAsks(ctx, f.q, f.ws)
	if err != nil {
		t.Fatalf("queued asks: %v", err)
	}
	listed := map[string]bool{}
	for _, q := range queued {
		listed[q.SubjectID] = true
	}
	if len(queued) != 2 || !listed[attentionIDString(over.SubjectID)] || !listed[attentionIDString(again.SubjectID)] {
		t.Fatalf("brief should list the two queued questions, got %+v", queued)
	}
	if listed[attentionIDString(fits.SubjectID)] {
		t.Fatalf("a delivered question must not be listed as queued")
	}
	// A queued question reaches neither the inbox nor the chat: the fixture has no other inbox rows.
	var inbox int
	if err := f.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM inbox_item WHERE workspace_id = $1`, f.ws).Scan(&inbox); err != nil {
		t.Fatalf("count inbox items: %v", err)
	}
	if inbox != 0 {
		t.Fatalf("queued questions reached the inbox: %d items", inbox)
	}
}

// TestAttentionMinutesPerDone: the minutes the window spent over the issues it finished. No
// reading while the window has no question or no done issue.
func TestAttentionMinutesPerDone(t *testing.T) {
	f := newAttentionFixture(t, true)
	ctx := context.Background()
	now := time.Now()
	since, until := now.Add(-time.Hour), now.Add(time.Hour)

	if _, ok, err := AttentionMinutesPerDone(ctx, f.q, f.ws, since, until); err != nil || ok {
		t.Fatalf("no question in the window: ok=%v err=%v, want no reading", ok, err)
	}
	for i := 0; i < 2; i++ {
		if _, err := f.attention.Admit(ctx, f.outcomeAsk()); err != nil {
			t.Fatalf("admit: %v", err)
		}
	}
	if _, ok, err := AttentionMinutesPerDone(ctx, f.q, f.ws, since, until); err != nil || ok {
		t.Fatalf("no issue done in the window: ok=%v err=%v, want no reading", ok, err)
	}

	// Two manual checks of 4 minutes each, and one issue done in the window: 8 minutes per done.
	f.Issue(t, "attention done", testutil.Cols{
		"status": "done", "assignee_type": "squad", "assignee_id": attentionIDString(f.squad), "done_at": testutil.Raw("now()"),
	})
	perDone, ok, err := AttentionMinutesPerDone(ctx, f.q, f.ws, since, until)
	if err != nil || !ok || perDone != 8 {
		t.Fatalf("minutes per done = (%v, %v, %v), want (8, true, nil)", perDone, ok, err)
	}
}

// TestUrgentNotQueued: an urgent question is delivered even past the squad's share.
func TestUrgentNotQueued(t *testing.T) {
	f := newAttentionFixture(t, true)
	ctx := context.Background()
	f.Exec(t, `UPDATE workspace SET attention_minutes_per_week = 7 WHERE id = $1`, f.ws)
	if _, err := f.attention.Admit(ctx, f.outcomeAsk()); err != nil {
		t.Fatalf("admit: %v", err)
	}

	urgent := f.outcomeAsk()
	urgent.Urgent = true
	got, err := f.attention.Admit(ctx, urgent)
	if err != nil {
		t.Fatalf("admit urgent: %v", err)
	}
	if got != AttentionDelivered {
		t.Fatalf("urgent question state = %q, want delivered", got)
	}
	if state, _ := f.ledgerRow(t, urgent.SubjectID); state != AttentionDelivered {
		t.Fatalf("urgent ledger row state = %q, want delivered", state)
	}
}

// TestDelegatedCostsNothing: a question the envelope answers is delegated and spends no
// minutes, even with the share used up.
func TestDelegatedCostsNothing(t *testing.T) {
	f := newAttentionFixture(t, true)
	ctx := context.Background()
	f.Exec(t, `UPDATE workspace SET attention_minutes_per_week = 4 WHERE id = $1`, f.ws)
	if _, err := f.attention.Admit(ctx, f.outcomeAsk()); err != nil {
		t.Fatalf("admit: %v", err)
	}
	before := f.squadMinutes(t)

	delegated := f.outcomeAsk()
	delegated.Delegated = true
	got, err := f.attention.Admit(ctx, delegated)
	if err != nil {
		t.Fatalf("admit delegated: %v", err)
	}
	if got != AttentionDelegated {
		t.Fatalf("delegated question state = %q, want delegated", got)
	}
	state, minutes := f.ledgerRow(t, delegated.SubjectID)
	if state != AttentionDelegated || minutes != 0 {
		t.Fatalf("delegated row = (%q, %v minutes), want (delegated, 0)", state, minutes)
	}
	if after := f.squadMinutes(t); after != before {
		t.Fatalf("a delegated question changed the squad's spend: %v -> %v", before, after)
	}
}

// TestAttentionFlagOffNoLedger: with durable_principal off nothing is written and no
// agent-ask rule applies, so today's behaviour is unchanged.
func TestAttentionFlagOffNoLedger(t *testing.T) {
	f := newAttentionFixture(t, false)
	ctx := context.Background()
	f.Exec(t, `UPDATE workspace SET attention_minutes_per_week = 0 WHERE id = $1`, f.ws)

	ask := f.outcomeAsk()
	ask.ByAgent = true
	ask.Reason = ""
	got, err := f.attention.Admit(ctx, ask)
	if err != nil {
		t.Fatalf("admit with the flag off: %v", err)
	}
	if got != AttentionDelivered {
		t.Fatalf("state = %q, want delivered", got)
	}
	var rows int
	if err := f.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM attention_ledger WHERE workspace_id = $1`, f.ws).Scan(&rows); err != nil {
		t.Fatalf("count ledger rows: %v", err)
	}
	if rows != 0 {
		t.Fatalf("flag off wrote %d ledger rows, want 0", rows)
	}
}

// TestAgentAskRequiresReason: an agent's question needs reason and why_not_decide, and
// missing_info needs the sources it searched. A person's question needs none of them.
func TestAgentAskRequiresReason(t *testing.T) {
	f := newAttentionFixture(t, true)
	ctx := context.Background()
	agentAsk := func(reason, why string, searched ...string) AskInput {
		ask := f.outcomeAsk()
		ask.ByAgent = true
		ask.Reason, ask.WhyNotDecide, ask.Searched = reason, why, searched
		return ask
	}
	refused := []struct {
		name string
		ask  AskInput
		want error
	}{
		{"no reason", agentAsk("", "the boss owns this"), ErrAttentionReasonRequired},
		{"no why_not_decide", agentAsk("judgment", "  "), ErrAttentionReasonRequired},
		{"an unknown reason", agentAsk("bored", "why"), ErrAttentionReasonRequired},
		{"missing_info without searched", agentAsk("missing_info", "no spec"), ErrAttentionSearchedRequired},
		{"missing_info with an unknown source", agentAsk("missing_info", "no spec", "google"), ErrAttentionSearchedRequired},
		{"why_not_decide over 500 characters", agentAsk("judgment", strings.Repeat("w", 501)), ErrAttentionWhyTooLong},
	}
	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.attention.Admit(ctx, tc.ask)
			if !errors.Is(err, tc.want) {
				t.Fatalf("admit error = %v, want %v", err, tc.want)
			}
			if state, _ := f.ledgerRowOrNone(t, tc.ask.SubjectID); state != "" {
				t.Fatalf("a refused question wrote a ledger row in state %q", state)
			}
		})
	}

	ok := agentAsk("missing_info", "no spec", "knowledge")
	if got, err := f.attention.Admit(ctx, ok); err != nil || got != AttentionDelivered {
		t.Fatalf("agent missing_info with a searched source = (%q, %v), want (delivered, nil)", got, err)
	}
	person := f.outcomeAsk()
	if got, err := f.attention.Admit(ctx, person); err != nil || got != AttentionDelivered {
		t.Fatalf("a person's question needs no reason = (%q, %v), want (delivered, nil)", got, err)
	}
}

// ledgerRowOrNone is ledgerRow for a question that may have no row.
func (f attentionFixture) ledgerRowOrNone(t *testing.T, subject pgtype.UUID) (string, float64) {
	t.Helper()
	var state string
	var minutes float64
	err := f.Pool.QueryRow(context.Background(),
		`SELECT state, estimated_minutes::float8 FROM attention_ledger WHERE subject_id = $1`, subject).Scan(&state, &minutes)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0
	}
	if err != nil {
		t.Fatalf("read ledger row: %v", err)
	}
	return state, minutes
}

// TestAnsweredRecordsTheMeasuredTime: answering a question records its state and the
// measured seconds, cut at 30 minutes (spec 7.2).
func TestAnsweredRecordsTheMeasuredTime(t *testing.T) {
	f := newAttentionFixture(t, true)
	ctx := context.Background()
	ask := f.outcomeAsk()
	if _, err := f.attention.Admit(ctx, ask); err != nil {
		t.Fatalf("admit: %v", err)
	}
	long := 2000
	if err := f.attention.Answered(ctx, "outcome_check", ask.SubjectID, &long); err != nil {
		t.Fatalf("answered: %v", err)
	}
	var state string
	var seconds int32
	if err := f.Pool.QueryRow(ctx,
		`SELECT state, actual_seconds FROM attention_ledger WHERE subject_id = $1`, ask.SubjectID).Scan(&state, &seconds); err != nil {
		t.Fatalf("read answered row: %v", err)
	}
	if state != "answered" || seconds != attentionMaxSeconds {
		t.Fatalf("answered row = (%q, %d seconds), want (answered, %d)", state, seconds, attentionMaxSeconds)
	}
}

// TestAnswerAppRequestMarksAnswered: a person's decision on an app action request closes its
// row, and with the flag off the decision writes nothing to the ledger.
func TestAnswerAppRequestMarksAnswered(t *testing.T) {
	f := newAttentionFixture(t, true)
	ctx := context.Background()
	ask := f.outcomeAsk()
	ask.SubjectType, ask.Reason = "app_action_request", "policy"
	if _, err := f.attention.Admit(ctx, ask); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := f.attention.AnswerAppRequest(ctx, ask.SubjectID); err != nil {
		t.Fatalf("answer app request: %v", err)
	}
	var state string
	if err := f.Pool.QueryRow(ctx,
		`SELECT state FROM attention_ledger WHERE subject_id = $1`, ask.SubjectID).Scan(&state); err != nil {
		t.Fatalf("read row: %v", err)
	}
	if state != "answered" {
		t.Fatalf("state = %q, want answered", state)
	}

	provider := featureflag.NewStaticProvider()
	provider.Set(featureflags.DurablePrincipal, featureflag.Rule{Default: false})
	off := &AttentionService{Queries: f.q, Flags: featureflag.NewService(provider)}
	pending := f.outcomeAsk()
	pending.SubjectType, pending.Reason = "app_action_request", "policy"
	if _, err := f.attention.Admit(ctx, pending); err != nil {
		t.Fatalf("admit pending: %v", err)
	}
	if err := off.AnswerAppRequest(ctx, pending.SubjectID); err != nil {
		t.Fatalf("answer with the flag off: %v", err)
	}
	if err := f.Pool.QueryRow(ctx,
		`SELECT state FROM attention_ledger WHERE subject_id = $1`, pending.SubjectID).Scan(&state); err != nil {
		t.Fatalf("read pending row: %v", err)
	}
	if state == "answered" {
		t.Fatalf("flag off changed the ledger row to answered")
	}
}

// TestAttentionBudgetSetAndRead: the budget is saved and read back with the week's spend; a budget
// outside a week or a split that is neither equal nor goals is refused and saves nothing.
func TestAttentionBudgetSetAndRead(t *testing.T) {
	f := newAttentionFixture(t, true)
	ctx := context.Background()
	ask := f.outcomeAsk()
	if _, err := f.attention.Admit(ctx, ask); err != nil {
		t.Fatalf("admit: %v", err)
	}

	got, err := f.attention.SetBudget(ctx, f.ws, 90, attentionSplitGoals)
	if err != nil {
		t.Fatalf("set budget: %v", err)
	}
	if !got.Enabled || got.MinutesPerWeek != 90 || got.Split != attentionSplitGoals {
		t.Fatalf("budget = %+v, want enabled, 90 minutes, goals", got)
	}
	if got.SpentMinutes != 4 {
		t.Fatalf("spent = %v, want the outcome check's 4 minutes", got.SpentMinutes)
	}

	if _, err := f.attention.SetBudget(ctx, f.ws, attentionMaxMinutesPerWeek+1, attentionSplitEqual); !errors.Is(err, ErrAttentionBudgetRange) {
		t.Fatalf("over a week: err = %v, want ErrAttentionBudgetRange", err)
	}
	if _, err := f.attention.SetBudget(ctx, f.ws, 60, "bogus"); !errors.Is(err, ErrAttentionSplit) {
		t.Fatalf("bad split: err = %v, want ErrAttentionSplit", err)
	}
	after, err := f.attention.Budget(ctx, f.ws)
	if err != nil {
		t.Fatalf("read budget: %v", err)
	}
	if after.MinutesPerWeek != 90 || after.Split != attentionSplitGoals {
		t.Fatalf("refused writes changed the budget: %+v", after)
	}
}

// TestAttentionBudgetSplit: the squad's share of the week, equal or by active goals.
func TestAttentionBudgetSplit(t *testing.T) {
	squads := make([]db.ListLiveSquadAttentionWeightsRow, 3)
	for i := range squads {
		squads[i].SquadID = pgtype.UUID{Bytes: uuid.New(), Valid: true}
	}
	goals := []db.ListLiveSquadAttentionWeightsRow{
		{SquadID: squads[0].SquadID, ActiveGoals: 1},
		{SquadID: squads[1].SquadID, ActiveGoals: 3},
		{SquadID: squads[2].SquadID},
	}
	missing := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	cases := []struct {
		name   string
		split  string
		rows   []db.ListLiveSquadAttentionWeightsRow
		squad  pgtype.UUID
		want   float64
		wantOK bool
	}{
		{"equal split of 60 across three squads", "equal", squads, squads[0].SquadID, 20, true},
		{"goals split weighs active goals", "goals", goals, squads[1].SquadID, 45, true},
		{"goals split gives a squad with no goal nothing", "goals", goals, squads[2].SquadID, 0, true},
		{"goals split with no active goal falls back to equal", "goals", squads, squads[2].SquadID, 20, true},
		{"a squad that is not live has no budget", "equal", squads, missing, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := attentionBudget(60, tc.split, tc.rows, tc.squad)
			if ok != tc.wantOK || got != tc.want {
				t.Fatalf("attentionBudget = (%v, %v), want (%v, %v)", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func attentionIDString(id pgtype.UUID) string { return uuid.UUID(id.Bytes).String() }

// TestAttentionMinutesPerDoneReading: the nightly rollup writes attention.minutes_per_done for the
// company, the minutes of the day's questions over the issues done that day (spec 7.5). With
// durable_principal off it writes nothing, even for rows the flag wrote earlier.
func TestAttentionMinutesPerDoneReading(t *testing.T) {
	f := newAttentionFixture(t, true)
	f.Cleanup(t, `DELETE FROM metric_reading WHERE workspace_id = $1`, f.WorkspaceID)
	ctx := context.Background()
	now := time.Date(2026, 11, 27, 6, 0, 0, 0, time.UTC)
	yesterdayNoon := now.AddDate(0, 0, -1).Add(12 * time.Hour)

	// Two manual checks of 4 minutes each, moved into yesterday, and one issue done yesterday: 8 minutes per done.
	for i := 0; i < 2; i++ {
		if _, err := f.attention.Admit(ctx, f.outcomeAsk()); err != nil {
			t.Fatalf("admit: %v", err)
		}
	}
	f.Exec(t, `UPDATE attention_ledger SET created_at = $2 WHERE workspace_id = $1`, f.ws, yesterdayNoon)
	f.Issue(t, "attention reading done", testutil.Cols{
		"status": "done", "assignee_type": "squad", "assignee_id": attentionIDString(f.squad), "done_at": yesterdayNoon,
	})

	metrics := NewMetricService(f.q, sharedTestPool(t))
	metrics.Flags = f.attention.Flags
	if _, err := metrics.RollupBuiltinMetrics(ctx, f.ws, now); err != nil {
		t.Fatalf("rollup: %v", err)
	}
	readingOf := func() (float64, int) {
		t.Helper()
		var count int
		var value float64
		if err := sharedTestPool(t).QueryRow(ctx, `
			SELECT COUNT(*), COALESCE(MAX(value::float8), 0) FROM metric_reading
			WHERE workspace_id = $1 AND metric_key = 'attention.minutes_per_done'
			  AND scope_type = 'company' AND scope_id IS NULL AND actor_type = 'system'`, f.WorkspaceID).Scan(&count, &value); err != nil {
			t.Fatalf("read attention reading: %v", err)
		}
		return value, count
	}
	if value, count := readingOf(); count != 1 || value != 8 {
		t.Fatalf("flag on: attention.minutes_per_done = %v (%d readings), want 8 (1 reading)", value, count)
	}

	f.Exec(t, `DELETE FROM metric_reading WHERE workspace_id = $1 AND metric_key = 'attention.minutes_per_done'`, f.ws)
	metrics.Flags = featureflag.NewService(featureflag.NewStaticProvider())
	if _, err := metrics.RollupBuiltinMetrics(ctx, f.ws, now); err != nil {
		t.Fatalf("rollup with flag off: %v", err)
	}
	if _, count := readingOf(); count != 0 {
		t.Fatalf("flag off: %d attention readings written, want none", count)
	}
}

// TestMeasureAndParseAttentionSeconds: MeasureAttentionSeconds and ParseOpenedAtSeconds compute
// elapsed seconds clamped to 30 minutes and AnswerAppRequestWithDuration persists actual_seconds
// (durable spec 7.2, plan C1).
func TestMeasureAndParseAttentionSeconds(t *testing.T) {
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)

	if got := MeasureAttentionSeconds(time.Time{}, now); got != nil {
		t.Fatalf("zero openedAt = %v, want nil", *got)
	}
	if got := MeasureAttentionSeconds(now.Add(time.Minute), now); got != nil {
		t.Fatalf("future openedAt = %v, want nil", *got)
	}
	if got := MeasureAttentionSeconds(now.Add(-45*time.Second), now); got == nil || *got != 45 {
		t.Fatalf("45s elapsed = %v, want 45", got)
	}
	if got := MeasureAttentionSeconds(now.Add(-2*time.Hour), now); got == nil || *got != attentionMaxSeconds {
		t.Fatalf("2h elapsed = %v, want %d", got, attentionMaxSeconds)
	}

	if got := ParseOpenedAtSeconds("", now); got != nil {
		t.Fatalf("empty = %v, want nil", *got)
	}
	if got := ParseOpenedAtSeconds("not-a-time", now); got != nil {
		t.Fatalf("invalid = %v, want nil", *got)
	}
	if got := ParseOpenedAtSeconds("-5", now); got != nil {
		t.Fatalf("negative seconds = %v, want nil", *got)
	}
	if got := ParseOpenedAtSeconds("90", now); got == nil || *got != 90 {
		t.Fatalf("90s string = %v, want 90", got)
	}
	if got := ParseOpenedAtSeconds("9999", now); got == nil || *got != attentionMaxSeconds {
		t.Fatalf("9999s string = %v, want %d", got, attentionMaxSeconds)
	}
	if got := ParseOpenedAtSeconds(now.Add(-120*time.Second).Format(time.RFC3339), now); got == nil || *got != 120 {
		t.Fatalf("RFC3339 120s ago = %v, want 120", got)
	}

	// End-to-end check on attention_ledger via AnswerAppRequestWithDuration.
	f := newAttentionFixture(t, true)
	ctx := context.Background()
	ask := f.outcomeAsk()
	ask.SubjectType, ask.Reason = "app_action_request", "policy"
	if _, err := f.attention.Admit(ctx, ask); err != nil {
		t.Fatalf("admit: %v", err)
	}
	secs := ParseOpenedAtSeconds(now.Add(-75*time.Second).Format(time.RFC3339), now)
	if err := f.attention.AnswerAppRequestWithDuration(ctx, ask.SubjectID, secs); err != nil {
		t.Fatalf("AnswerAppRequestWithDuration: %v", err)
	}
	var state string
	var stored int32
	if err := f.Pool.QueryRow(ctx,
		`SELECT state, actual_seconds FROM attention_ledger WHERE subject_id = $1`, ask.SubjectID).Scan(&state, &stored); err != nil {
		t.Fatalf("read row: %v", err)
	}
	if state != "answered" || stored != 75 {
		t.Fatalf("ledger row = (%q, %d), want (answered, 75)", state, stored)
	}
}
