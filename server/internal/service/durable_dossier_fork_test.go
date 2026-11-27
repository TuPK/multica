package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// dossierEntry makes an active entry for the pure selection tests.
func dossierEntry(kind, text, workClass, status string, created time.Time, id byte) db.SeatDossierEntry {
	return db.SeatDossierEntry{
		ID:        pgtype.UUID{Bytes: [16]byte{id}, Valid: true},
		Kind:      kind,
		Text:      text,
		WorkClass: workClass,
		Status:    status,
		CreatedAt: pgtype.Timestamptz{Time: created, Valid: true},
	}
}

func TestSelectDossierLimitAndOrder(t *testing.T) {
	base := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	at := func(h int) time.Time { return base.Add(time.Duration(h) * time.Hour) }
	const wc = "label:billing"
	entries := []db.SeatDossierEntry{
		dossierEntry("lesson", "older lesson of the class", wc, "active", at(0), 1),
		dossierEntry("pitfall", "newer pitfall of the class", wc, "active", at(1), 2),
		dossierEntry("preference", "general preference", "", "active", at(2), 3),
		dossierEntry("pitfall", "general pitfall", "", "active", at(3), 4),
		dossierEntry("lesson", "lesson of another class", "label:other", "active", at(4), 5),
		dossierEntry("pitfall", "retired pitfall of the class", wc, "retired", at(5), 6),
	}

	t.Run("class pitfalls and lessons first, then general, newest first", func(t *testing.T) {
		got := SelectDossier(entries, wc, DossierCharLimit)
		want := []string{"newer pitfall of the class", "older lesson of the class", "general pitfall", "general preference"}
		if len(got) != len(want) {
			t.Fatalf("got %d entries, want %d: %v", len(got), len(want), got)
		}
		for i, e := range got {
			if e.Text != want[i] {
				t.Errorf("entry %d = %q, want %q", i, e.Text, want[i])
			}
		}
	})

	t.Run("an issue with no class gets only the general entries", func(t *testing.T) {
		got := SelectDossier(entries, "", DossierCharLimit)
		for _, e := range got {
			if e.WorkClass != "" {
				t.Errorf("entry %q of class %q was selected for an issue with no class", e.Text, e.WorkClass)
			}
		}
		if len(got) != 2 {
			t.Errorf("got %d entries, want the 2 general ones", len(got))
		}
	})

	t.Run("the block stays within the character limit and stops at the first entry that does not fit", func(t *testing.T) {
		long := make([]db.SeatDossierEntry, 0, 8)
		for i := 0; i < 8; i++ {
			long = append(long, dossierEntry("pitfall", strings.Repeat("x", 800), "", "active", at(i), byte(10+i)))
		}
		got := SelectDossier(long, "", DossierCharLimit)
		used := 0
		for _, e := range got {
			used += utf8.RuneCountInString(dossierLine(e))
		}
		if used > DossierCharLimit {
			t.Fatalf("selected %d characters, limit is %d", used, DossierCharLimit)
		}
		next := utf8.RuneCountInString(dossierLine(long[len(got)]))
		if used+next <= DossierCharLimit {
			t.Fatalf("stopped at %d characters though the next entry fits", used)
		}
		if len(got) != 4 {
			t.Errorf("got %d entries of 820 characters, want 4 within 4000", len(got))
		}
	})
}

// dossierFixture is a workspace with one seat and the dossier service. The flag is set
// as the test needs it.
type dossierFixture struct {
	principalFixture
	ws      pgtype.UUID
	runtime string
	seat    string
	svc     *DossierService
	flags   *featureflag.Service
	owner   GovernanceActor
}

func newDossierFixture(t *testing.T, flagOn bool) dossierFixture {
	t.Helper()
	f, _ := companyWorkspace(t)
	runtime := f.Runtime(t, "dossier runtime", testutil.Cols{"provider": "anthropic"})
	provider := featureflag.NewStaticProvider()
	provider.Set(featureflags.DurableSeats, featureflag.Rule{Default: flagOn})
	flags := featureflag.NewService(provider)
	bus := events.New()
	tasks := &TaskService{Queries: f.q, TxStarter: f.Pool, Bus: bus}
	ws := util.MustParseUUID(f.WorkspaceID)
	df := dossierFixture{
		principalFixture: f,
		ws:               ws,
		runtime:          runtime,
		seat:             f.Agent(t, "dossier seat", runtime, testutil.Cols{"model": "model-a"}),
		flags:            flags,
		owner:            GovernanceActor{Type: GovernanceActorMember, ID: util.MustParseUUID(f.UserID)},
	}
	df.svc = &DossierService{
		Queries: f.q, TxStarter: f.Pool, Flags: flags,
		Issues: NewIssueService(f.q, f.Pool, bus, nil, tasks),
	}
	f.Cleanup(t, `DELETE FROM seat_dossier_entry WHERE workspace_id = $1`, f.WorkspaceID)
	f.Cleanup(t, `DELETE FROM governance_event WHERE workspace_id = $1`, f.WorkspaceID)
	return df
}

// cleanLessonIssues removes the lesson and cleanup issues the services file, with
// the task rows they queued, so the fixture's cleanup can run.
func (df dossierFixture) cleanLessonIssues(t *testing.T) {
	t.Helper()
	df.Cleanup(t, `DELETE FROM agent_task_queue WHERE issue_id IN (SELECT id FROM issue WHERE workspace_id = $1 AND title LIKE 'Write lessons%')`, df.WorkspaceID)
	df.Cleanup(t, `DELETE FROM issue WHERE workspace_id = $1 AND title LIKE 'Write lessons%'`, df.WorkspaceID)
}

// lessonIssues counts the lesson issues filed on the seat.
func (df dossierFixture) lessonIssues(t *testing.T) int {
	t.Helper()
	return df.Count(t, `SELECT COUNT(*) FROM issue WHERE workspace_id = $1 AND assignee_id = $2 AND title LIKE 'Write lessons%'`,
		df.WorkspaceID, df.seat)
}

func dossierInput(kind, text string) DossierEntryInput {
	return DossierEntryInput{
		Kind: kind, Text: text,
		Evidence: []DossierEvidence{{Type: "outcome", Ref: "00000000-0000-0000-0000-000000000001"}},
	}
}

func TestLessonTaskOnFailOnly(t *testing.T) {
	df := newDossierFixture(t, true)
	df.cleanLessonIssues(t)
	outcomes, _ := outcomeService(t, df.principalFixture, true)

	for _, verdict := range []string{"pass", "partial", "fail", "unable"} {
		before := df.lessonIssues(t)
		issue := outcomeIssue(t, df.principalFixture, "todo")
		verificationCleanup(t, df.principalFixture, issue)
		df.Exec(t, `UPDATE issue SET assignee_type = 'agent', assignee_id = $2 WHERE id = $1`, issue, df.seat)
		df.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, issue)
		runOutcomeJob(t, outcomes)
		o, err := df.q.GetCurrentWorkOutcomeForIssue(context.Background(), db.GetCurrentWorkOutcomeForIssueParams{
			WorkspaceID: df.ws, IssueID: util.MustParseUUID(issue),
		})
		if err != nil {
			t.Fatalf("current outcome: %v", err)
		}
		o.Verdict = verdict

		if err := df.svc.OnOutcomeSettled(context.Background(), o); err != nil {
			t.Fatalf("OnOutcomeSettled(%s): %v", verdict, err)
		}
		want := 0
		if verdict == "fail" || verdict == "partial" {
			want = 1
		}
		if got := df.lessonIssues(t) - before; got != want {
			t.Errorf("verdict %s filed %d lesson tasks, want %d", verdict, got, want)
		}
	}
}

func TestMax100Active(t *testing.T) {
	df := newDossierFixture(t, true)
	ctx := context.Background()
	for i := 0; i < dossierMaxActive; i++ {
		if _, err := df.svc.Add(ctx, df.ws, util.MustParseUUID(df.seat), df.owner,
			[]DossierEntryInput{dossierInput("lesson", "entry")}); err != nil {
			t.Fatalf("entry %d: %v", i+1, err)
		}
	}
	_, err := df.svc.Add(ctx, df.ws, util.MustParseUUID(df.seat), df.owner, []DossierEntryInput{dossierInput("lesson", "one too many")})
	if !errors.Is(err, ErrDossierFull) {
		t.Fatalf("entry 101 err = %v, want ErrDossierFull", err)
	}

	// Retiring one frees a slot.
	list, err := df.svc.List(ctx, df.ws, util.MustParseUUID(df.seat), "active", 200)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if _, err := df.svc.Retire(ctx, df.ws, util.MustParseUUID(df.seat), list[0].ID, df.owner, "duplicate"); err != nil {
		t.Fatalf("retire: %v", err)
	}
	if _, err := df.svc.Add(ctx, df.ws, util.MustParseUUID(df.seat), df.owner, []DossierEntryInput{dossierInput("lesson", "fits now")}); err != nil {
		t.Fatalf("entry after a retire: %v", err)
	}
}

func TestMemberEntryOnlyMemberRetires(t *testing.T) {
	df := newDossierFixture(t, true)
	ctx := context.Background()
	seat := util.MustParseUUID(df.seat)
	agent := GovernanceActor{Type: GovernanceActorAgent, ID: seat}

	memberEntry, err := df.svc.Add(ctx, df.ws, seat, df.owner, []DossierEntryInput{dossierInput("preference", "members like short notes")})
	if err != nil {
		t.Fatalf("member adds: %v", err)
	}
	id := memberEntry[0].ID
	if _, err := df.svc.Retire(ctx, df.ws, seat, id, agent, "outdated"); !errors.Is(err, ErrDossierMemberOnly) {
		t.Fatalf("agent retires a member entry: err = %v, want ErrDossierMemberOnly", err)
	}
	if got := df.Count(t, `SELECT COUNT(*) FROM seat_dossier_entry WHERE id = $1 AND status = 'active'`, util.UUIDToString(id)); got != 1 {
		t.Fatalf("member entry is no longer active after the agent's refused retire")
	}
	if _, err := df.svc.Retire(ctx, df.ws, seat, id, df.owner, "member decided"); err != nil {
		t.Fatalf("member retires own entry: %v", err)
	}

	own, err := df.svc.Add(ctx, df.ws, seat, agent, []DossierEntryInput{dossierInput("pitfall", "the export drops headers")})
	if err != nil {
		t.Fatalf("agent adds to its own seat: %v", err)
	}
	if _, err := df.svc.Retire(ctx, df.ws, seat, own[0].ID, agent, "covered by a newer lesson"); err != nil {
		t.Fatalf("agent retires its own entry: %v", err)
	}

	other := util.MustParseUUID(df.Agent(t, "other seat", df.runtime))
	if _, err := df.svc.Add(ctx, df.ws, other, agent, []DossierEntryInput{dossierInput("lesson", "not mine")}); !errors.Is(err, ErrDossierForbidden) {
		t.Fatalf("agent writes to another seat: err = %v, want ErrDossierForbidden", err)
	}
}

func TestModelChangeLogsEvent(t *testing.T) {
	df := newDossierFixture(t, true)
	ctx := context.Background()
	seat := util.MustParseUUID(df.seat)
	before, err := df.q.GetAgent(ctx, seat)
	if err != nil {
		t.Fatalf("load seat: %v", err)
	}
	openai := df.Runtime(t, "openai runtime", testutil.Cols{"provider": "openai"})
	df.Exec(t, `UPDATE agent SET runtime_id = $2, model = 'model-b' WHERE id = $1`, df.seat, openai)
	after, err := df.q.GetAgent(ctx, seat)
	if err != nil {
		t.Fatalf("reload seat: %v", err)
	}

	if err := RecordSeatModelChange(ctx, df.q, df.flags, df.owner, before, after); err != nil {
		t.Fatalf("record: %v", err)
	}
	var raw []byte
	df.QueryRow(t, `SELECT details FROM governance_event WHERE workspace_id = $1 AND kind = 'seat_model_changed' AND subject_id = $2`,
		df.WorkspaceID, df.seat).Scan(&raw)
	var details map[string]any
	if err := json.Unmarshal(raw, &details); err != nil {
		t.Fatalf("details: %v", err)
	}
	want := map[string]any{"from_provider": "anthropic", "from_model": "model-a", "to_provider": "openai", "to_model": "model-b"}
	for k, v := range want {
		if details[k] != v {
			t.Errorf("details[%s] = %v, want %v", k, details[k], v)
		}
	}

	// A save that moves neither runtime nor model logs nothing.
	if err := RecordSeatModelChange(ctx, df.q, df.flags, df.owner, after, after); err != nil {
		t.Fatalf("record unchanged: %v", err)
	}
	if got := df.Count(t, `SELECT COUNT(*) FROM governance_event WHERE workspace_id = $1 AND kind = 'seat_model_changed'`, df.WorkspaceID); got != 1 {
		t.Errorf("events = %d after an unchanged save, want 1", got)
	}
}

func TestModelChangeSilentWhenFlagOff(t *testing.T) {
	df := newDossierFixture(t, false)
	ctx := context.Background()
	seat := util.MustParseUUID(df.seat)
	before, _ := df.q.GetAgent(ctx, seat)
	after := before
	after.Model = pgtype.Text{String: "model-b", Valid: true}
	if err := RecordSeatModelChange(ctx, df.q, df.flags, df.owner, before, after); err != nil {
		t.Fatalf("record: %v", err)
	}
	if got := df.Count(t, `SELECT COUNT(*) FROM governance_event WHERE workspace_id = $1 AND kind = 'seat_model_changed'`, df.WorkspaceID); got != 0 {
		t.Fatalf("flag off wrote %d events, want none", got)
	}
}

func TestDossierInBriefingOnlyWhenFlagOn(t *testing.T) {
	ctx := context.Background()
	for _, flagOn := range []bool{true, false} {
		df := newDossierFixture(t, flagOn)
		if _, err := df.svc.Add(ctx, df.ws, util.MustParseUUID(df.seat), df.owner,
			[]DossierEntryInput{dossierInput("pitfall", "the export drops headers")}); err != nil {
			t.Fatalf("add: %v", err)
		}
		issue := df.Issue(t, "briefing dossier")
		task := df.Task(t, df.seat, testutil.Cols{"issue_id": issue, "runtime_id": df.runtime})
		row, err := df.q.GetAgentTask(ctx, util.MustParseUUID(task))
		if err != nil {
			t.Fatalf("task: %v", err)
		}
		blocks := DurableBriefingBlocks(ctx, df.q, df.flags, row)
		found := false
		for _, b := range blocks {
			if b.Title == briefingDossierTitle {
				found = true
				if !strings.Contains(b.Body, "the export drops headers") {
					t.Errorf("dossier block lacks the entry: %q", b.Body)
				}
			}
		}
		if found != flagOn {
			t.Errorf("flag %v: dossier block present = %v", flagOn, found)
		}
	}
}

// TestCleanupMonthlyOncePerSeat: the monthly cleanup files one issue for a seat that holds two
// or more active entries, waits until the second day of the month, and files the seat's issue
// once per month.
func TestCleanupMonthlyOncePerSeat(t *testing.T) {
	df := newDossierFixture(t, true)
	ctx := context.Background()
	cleanup := func() {
		df.Cleanup(t, `DELETE FROM agent_task_queue WHERE issue_id IN (SELECT id FROM issue WHERE workspace_id = $1 AND title LIKE 'Seat dossier cleanup%')`, df.WorkspaceID)
		df.Cleanup(t, `DELETE FROM issue WHERE workspace_id = $1 AND title LIKE 'Seat dossier cleanup%'`, df.WorkspaceID)
	}
	cleanup()
	cleanupIssues := func() int {
		return df.Count(t, `SELECT COUNT(*) FROM issue WHERE workspace_id = $1 AND assignee_id = $2 AND title LIKE 'Seat dossier cleanup%'`,
			df.WorkspaceID, df.seat)
	}
	seat := util.MustParseUUID(df.seat)
	if _, err := df.svc.Add(ctx, df.ws, seat, df.owner,
		[]DossierEntryInput{dossierInput("lesson", "first"), dossierInput("lesson", "second")}); err != nil {
		t.Fatalf("add: %v", err)
	}
	at := func(day int) func() time.Time {
		return func() time.Time { return time.Date(2026, 11, day, 9, 0, 0, 0, time.UTC) }
	}

	df.svc.Now = at(1)
	if _, err := df.svc.CleanupMonthly(ctx); err != nil {
		t.Fatalf("cleanup on the first: %v", err)
	}
	if got := cleanupIssues(); got != 0 {
		t.Fatalf("cleanup on the first day filed %d issues, want 0", got)
	}

	df.svc.Now = at(2)
	if _, err := df.svc.CleanupMonthly(ctx); err != nil {
		t.Fatalf("cleanup on the second: %v", err)
	}
	if got := cleanupIssues(); got != 1 {
		t.Fatalf("cleanup on the second day filed %d issues, want 1", got)
	}
	if _, err := df.svc.CleanupMonthly(ctx); err != nil {
		t.Fatalf("cleanup again: %v", err)
	}
	if got := cleanupIssues(); got != 1 {
		t.Errorf("a second run in the month filed a second issue: %d issues, want 1", got)
	}
}

// TestTeardownUnbindLogsSeatChange: a runtime teardown takes its seats off the runtime, and
// each unbound seat writes seat_model_changed with the runtime it left as the from side.
func TestTeardownUnbindLogsSeatChange(t *testing.T) {
	df := newDossierFixture(t, true)
	if _, err := TeardownRuntime(context.Background(), df.q, util.MustParseUUID(df.runtime),
		RuntimeTeardownOptions{SeatFlags: df.flags}); err != nil {
		t.Fatalf("teardown: %v", err)
	}
	if got := df.Count(t, `SELECT COUNT(*) FROM governance_event
		WHERE workspace_id = $1 AND kind = 'seat_model_changed' AND subject_id = $2`, df.WorkspaceID, df.seat); got != 1 {
		t.Fatalf("seat_model_changed events = %d, want 1", got)
	}
	var from, to string
	df.QueryRow(t, `SELECT details->>'from_provider', details->>'to_provider' FROM governance_event
		WHERE workspace_id = $1 AND kind = 'seat_model_changed' AND subject_id = $2`, df.WorkspaceID, df.seat).Scan(&from, &to)
	if from != "anthropic" || to != "" {
		t.Errorf("from_provider=%q to_provider=%q, want anthropic and empty", from, to)
	}
}

// TestBriefingCountsDossierUses: each briefing that carries an entry counts it once.
func TestBriefingCountsDossierUses(t *testing.T) {
	df := newDossierFixture(t, true)
	ctx := context.Background()
	seat := util.MustParseUUID(df.seat)
	if _, err := df.svc.Add(ctx, df.ws, seat, df.owner,
		[]DossierEntryInput{dossierInput("lesson", "first"), dossierInput("lesson", "second")}); err != nil {
		t.Fatalf("add: %v", err)
	}
	task := db.AgentTaskQueue{AgentID: seat}
	for i := 0; i < 2; i++ {
		if blocks := dossierBriefing(ctx, df.q, df.flags, task); len(blocks) != 1 {
			t.Fatalf("briefing %d: %d blocks, want 1", i+1, len(blocks))
		}
	}
	if got := df.Count(t, `SELECT COALESCE(SUM(uses), 0)::int FROM seat_dossier_entry WHERE workspace_id = $1 AND agent_id = $2`,
		df.WorkspaceID, df.seat); got != 4 {
		t.Errorf("total uses = %d, want 4 (two entries, two briefings)", got)
	}
}

// TestShadowTaskGetsSeatDossierInBriefing: a shadow task runs on a shadow agent linked to its
// seat via shadow_of_agent_id. Its briefing resolves the seat and carries the seat's dossier,
// while the normal non-shadow path is unchanged (durable spec 9.4, V1.6).
func TestShadowTaskGetsSeatDossierInBriefing(t *testing.T) {
	df := newDossierFixture(t, true)
	ctx := context.Background()
	seatID := util.MustParseUUID(df.seat)
	if _, err := df.svc.Add(ctx, df.ws, seatID, df.owner,
		[]DossierEntryInput{dossierInput("pitfall", "always verify invoice totals")}); err != nil {
		t.Fatalf("add seat dossier entry: %v", err)
	}
	candRuntime := df.Runtime(t, "shadow candidate runtime", testutil.Cols{"provider": "openai"})
	shadowAgent, err := df.q.CreateShadowAgent(ctx, db.CreateShadowAgentParams{
		Name: "dossier seat (shadow test)", RuntimeID: util.MustParseUUID(candRuntime),
		Model: pgtype.Text{String: "model-b", Valid: true}, SeatID: seatID, WorkspaceID: df.ws,
	})
	if err != nil {
		t.Fatalf("create shadow agent: %v", err)
	}
	issue := df.Issue(t, "shadow dossier briefing")

	// Shadow task assigned to the shadow agent receives the seat's dossier.
	shadowTaskID := df.Task(t, util.UUIDToString(shadowAgent.ID), testutil.Cols{
		"issue_id": issue, "runtime_id": candRuntime,
		"context": testutil.Raw(`'{"durable_kind":"shadow","shadow_purpose":"model_trial"}'::jsonb`),
	})
	shadowTask, err := df.q.GetAgentTask(ctx, util.MustParseUUID(shadowTaskID))
	if err != nil {
		t.Fatalf("load shadow task: %v", err)
	}
	shadowBlocks := DurableBriefingBlocks(ctx, df.q, df.flags, shadowTask)
	foundShadow := false
	for _, b := range shadowBlocks {
		if b.Title == briefingDossierTitle && strings.Contains(b.Body, "always verify invoice totals") {
			foundShadow = true
		}
	}
	if !foundShadow {
		t.Fatalf("shadow task briefing did not include the seat's dossier; blocks = %+v", shadowBlocks)
	}

	// Normal (non-shadow) task assigned directly to the seat still gets the seat's dossier.
	normalTaskID := df.Task(t, df.seat, testutil.Cols{"issue_id": issue, "runtime_id": df.runtime})
	normalTask, err := df.q.GetAgentTask(ctx, util.MustParseUUID(normalTaskID))
	if err != nil {
		t.Fatalf("load normal task: %v", err)
	}
	normalBlocks := DurableBriefingBlocks(ctx, df.q, df.flags, normalTask)
	foundNormal := false
	for _, b := range normalBlocks {
		if b.Title == briefingDossierTitle && strings.Contains(b.Body, "always verify invoice totals") {
			foundNormal = true
		}
	}
	if !foundNormal {
		t.Fatalf("normal task briefing did not include the seat's dossier; blocks = %+v", normalBlocks)
	}
}
