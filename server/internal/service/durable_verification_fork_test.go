package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// verificationFixture is a workspace with a doer agent and a verifier agent, the
// OutcomeService that opens and settles outcomes, and the VerificationService whose
// checks these tests drive. Every row goes through the fixture, so cleanup is
// registered as the rows are made.
type verificationFixture struct {
	principalFixture
	ws       pgtype.UUID
	member   GovernanceActor
	runtime  string
	doer     string
	verifier string
	outcomes *OutcomeService
	checks   *VerificationService
}

func newVerificationFixture(t *testing.T) verificationFixture {
	t.Helper()
	f, _ := companyWorkspace(t)
	runtime := f.Runtime(t, "verification runtime")
	vf := verificationFixture{
		principalFixture: f,
		ws:               util.MustParseUUID(f.WorkspaceID),
		member:           GovernanceActor{Type: GovernanceActorMember, ID: util.MustParseUUID(f.UserID)},
		runtime:          runtime,
		doer:             f.Agent(t, "doer", runtime),
		verifier:         f.Agent(t, "verifier", runtime),
	}
	outcomes, _ := outcomeService(t, f, true)
	vf.outcomes = outcomes
	vf.checks = &VerificationService{
		Queries:   f.q,
		TxStarter: f.Pool,
		Tasks:     &TaskService{Queries: f.q, TxStarter: f.Pool, Bus: events.New()},
	}
	outcomes.Verification = vf.checks
	return vf
}

// verificationCleanup removes the rows the services write for an issue. They have no
// foreign keys, so the fixture does not reach them. It is registered after
// outcomeIssue, so it runs before the outcome and transition rows are deleted.
func verificationCleanup(t *testing.T, f principalFixture, issue string) {
	t.Helper()
	f.Cleanup(t, `DELETE FROM outcome_verification WHERE outcome_id IN (SELECT id FROM work_outcome WHERE issue_id = $1)`, issue)
	f.Cleanup(t, `DELETE FROM outcome_check_task WHERE outcome_id IN (SELECT id FROM work_outcome WHERE issue_id = $1)`, issue)
	f.Cleanup(t, `DELETE FROM acceptance_criterion WHERE issue_id = $1`, issue)
	f.Cleanup(t, `DELETE FROM comment WHERE issue_id = $1`, issue)
	f.Cleanup(t, `DELETE FROM agent_task_queue WHERE issue_id = $1`, issue)
}

// doneIssue makes an issue assigned to agent and moves it to done, then runs the
// outcome job so the issue has its current outcome.
func (vf verificationFixture) doneIssue(t *testing.T, agent string) string {
	t.Helper()
	issue := outcomeIssue(t, vf.principalFixture, "todo")
	verificationCleanup(t, vf.principalFixture, issue)
	vf.Exec(t, `UPDATE issue SET assignee_type = 'agent', assignee_id = $2 WHERE id = $1`, issue, agent)
	vf.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, issue)
	runOutcomeJob(t, vf.outcomes)
	return issue
}

// addCriterion writes one criterion as a member.
func (vf verificationFixture) addCriterion(t *testing.T, issue, text, kind string, spec map[string]any) db.AcceptanceCriterion {
	t.Helper()
	verificationCleanup(t, vf.principalFixture, issue)
	c, err := vf.checks.AddCriterion(context.Background(), vf.ws, util.MustParseUUID(issue), vf.member, CriterionInput{
		Text: text, CheckKind: kind, CheckSpec: spec,
	})
	if err != nil {
		t.Fatalf("add criterion %q: %v", text, err)
	}
	return c
}

// currentOutcome returns the issue's current outcome row.
func (vf verificationFixture) currentOutcome(t *testing.T, issue string) db.WorkOutcome {
	t.Helper()
	o, err := vf.checks.Queries.GetCurrentWorkOutcomeForIssue(context.Background(), db.GetCurrentWorkOutcomeForIssueParams{
		WorkspaceID: vf.ws, IssueID: util.MustParseUUID(issue),
	})
	if err != nil {
		t.Fatalf("current outcome of %s: %v", issue, err)
	}
	return o
}

// verificationsOf reads the checks of one outcome, oldest first.
func verificationsOf(t *testing.T, f principalFixture, outcome string) []verificationRow {
	t.Helper()
	rows, err := f.Pool.Query(context.Background(), `
		SELECT verifier_type, COALESCE(criterion_id::text, ''), result, note
		FROM outcome_verification WHERE outcome_id = $1 ORDER BY created_at, id`, outcome)
	if err != nil {
		t.Fatalf("list verifications: %v", err)
	}
	defer rows.Close()
	var out []verificationRow
	for rows.Next() {
		var r verificationRow
		if err := rows.Scan(&r.VerifierType, &r.CriterionID, &r.Result, &r.Note); err != nil {
			t.Fatalf("scan verification: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate verifications: %v", err)
	}
	return out
}

type verificationRow struct {
	VerifierType, CriterionID, Result, Note string
}

// setVerifierSettings writes the workspace's outcome verifier settings.
func (vf verificationFixture) setVerifierSettings(t *testing.T, settings map[string]any) {
	t.Helper()
	raw, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	vf.Exec(t, `UPDATE workspace SET settings = $2::jsonb WHERE id = $1`, vf.WorkspaceID, string(raw))
}

// TestVerifierNeverDoer: whichever step of the order picks the verifier, the doer is
// never the one picked, and a workspace whose only agent is the doer gets no verifier.
func TestVerifierNeverDoer(t *testing.T) {
	vf := newVerificationFixture(t)
	ctx := context.Background()
	doer := util.MustParseUUID(vf.doer)
	other := util.MustParseUUID(vf.verifier)

	// The setting names the doer; the pool holds the doer first and then the other.
	vf.setVerifierSettings(t, map[string]any{
		"outcome_verifier_agent_id": vf.doer,
		"outcome_verifier_pool":     []string{vf.doer, vf.verifier},
	})
	got, ok := vf.checks.PickVerifier(ctx, vf.ws, doer)
	if !ok || got != other {
		t.Fatalf("PickVerifier with the doer named first = %v, %v; want the other agent", util.UUIDToString(got), ok)
	}

	// A secretary who is the doer is skipped the same way.
	vf.Exec(t, `UPDATE workspace SET secretary_agent_id = $2, settings = '{}'::jsonb WHERE id = $1`, vf.WorkspaceID, vf.doer)
	got, ok = vf.checks.PickVerifier(ctx, vf.ws, doer)
	if ok && got == doer {
		t.Fatalf("PickVerifier returned the doer through the secretary")
	}
}

// TestNoEligibleVerifierYieldsUnable: a workspace with one agent, which did the
// work. The agent check has no verifier, so it is recorded as unable, and the outcome
// settles as unverified rather than failing on a check nobody could make.
func TestNoEligibleVerifierYieldsUnable(t *testing.T) {
	f, _ := companyWorkspace(t)
	runtime := f.Runtime(t, "solo runtime")
	solo := f.Agent(t, "solo doer", runtime)
	ws := util.MustParseUUID(f.WorkspaceID)
	checks := &VerificationService{Queries: f.q, TxStarter: f.Pool, Tasks: &TaskService{Queries: f.q, TxStarter: f.Pool, Bus: events.New()}}
	outcomes, _ := outcomeService(t, f, true)
	outcomes.Verification = checks

	issue := outcomeIssue(t, f, "todo")
	verificationCleanup(t, f, issue)
	f.Exec(t, `UPDATE issue SET assignee_type = 'agent', assignee_id = $2 WHERE id = $1`, issue, solo)
	f.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, issue)
	runOutcomeJob(t, outcomes)
	if _, err := checks.AddCriterion(context.Background(), ws, util.MustParseUUID(issue),
		GovernanceActor{Type: GovernanceActorMember, ID: util.MustParseUUID(f.UserID)},
		CriterionInput{Text: "The page loads.", CheckKind: "agent_judgment", CheckSpec: map[string]any{"rubric": "Does it load?"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := checks.ScheduleChecks(context.Background()); err != nil {
		t.Fatalf("schedule checks: %v", err)
	}

	o := (verificationFixture{principalFixture: f, ws: ws, checks: checks}).currentOutcome(t, issue)
	rows := verificationsOf(t, f, util.UUIDToString(o.ID))
	if len(rows) != 1 || rows[0].Result != "unable" || rows[0].VerifierType != "system" {
		t.Fatalf("verifications = %+v, want one system unable check", rows)
	}
	if n := f.Count(t, `SELECT count(*) FROM outcome_check_task WHERE outcome_id = $1`, util.UUIDToString(o.ID)); n != 0 {
		t.Fatalf("a check with no verifier was assigned to a task")
	}

	f.Exec(t, `UPDATE work_outcome SET settle_at = now() - interval '1 minute' WHERE id = $1`, util.UUIDToString(o.ID))
	if _, err := outcomes.Settle(context.Background(), time.Now()); err != nil {
		t.Fatalf("settle: %v", err)
	}
	var verdict string
	f.Pool.QueryRow(context.Background(), `SELECT verdict FROM work_outcome WHERE id = $1`, util.UUIDToString(o.ID)).Scan(&verdict)
	if verdict != "unverified" {
		t.Fatalf("verdict = %q, want unverified: an unable check does not count", verdict)
	}
}

// TestImplicitCriterionWhenNone: a sampled outcome whose issue has no criteria gets one
// implicit agent_judgment criterion, so the outcome is never checked by nothing.
func TestImplicitCriterionWhenNone(t *testing.T) {
	vf := newVerificationFixture(t)
	issue := vf.doneIssue(t, vf.doer)
	if _, err := vf.checks.ScheduleChecks(context.Background()); err != nil {
		t.Fatalf("schedule checks: %v", err)
	}
	criteria, err := vf.checks.ListCriteria(context.Background(), vf.ws, util.MustParseUUID(issue))
	if err != nil {
		t.Fatal(err)
	}
	if len(criteria) != 1 || criteria[0].CheckKind != "agent_judgment" || criteria[0].Source != "template" {
		t.Fatalf("criteria = %+v, want one implicit agent_judgment criterion", criteria)
	}
	if !strings.Contains(string(criteria[0].CheckSpec), "rubric") {
		t.Fatalf("implicit criterion has no rubric: %s", criteria[0].CheckSpec)
	}
	// A second run adds nothing: the criterion now exists.
	if _, err := vf.checks.ScheduleChecks(context.Background()); err != nil {
		t.Fatal(err)
	}
	again, _ := vf.checks.ListCriteria(context.Background(), vf.ws, util.MustParseUUID(issue))
	if len(again) != 1 {
		t.Fatalf("a second run made %d criteria, want 1", len(again))
	}
}

// TestArtifactCriterionPassesOnPublish: the artifact check waits until a document of
// its kind is published from the issue, then passes at once.
func TestArtifactCriterionPassesOnPublish(t *testing.T) {
	vf := newVerificationFixture(t)
	issue := vf.doneIssue(t, vf.doer)
	vf.addCriterion(t, issue, "A spec is published.", "artifact", map[string]any{"kind": "spec"})
	ctx := context.Background()

	if _, err := vf.checks.ScheduleChecks(ctx); err != nil {
		t.Fatal(err)
	}
	o := vf.currentOutcome(t, issue)
	if rows := verificationsOf(t, vf.principalFixture, util.UUIDToString(o.ID)); len(rows) != 0 {
		t.Fatalf("an artifact with nothing published has checks: %+v", rows)
	}

	vf.Insert(t, "knowledge_link", testutil.Cols{
		"workspace_id": vf.WorkspaceID, "knowledge_base_id": uuid.NewString(),
		"weknora_knowledge_id": "wk-" + issue, "source_type": "issue", "source_id": issue,
		"kind": "spec", "idempotency_key": "spec-" + issue, "state": "published",
		"created_by_type": "member", "created_by_id": vf.UserID, "published_at": testutil.Raw("now()"),
	})
	if _, err := vf.checks.ScheduleChecks(ctx); err != nil {
		t.Fatal(err)
	}
	rows := verificationsOf(t, vf.principalFixture, util.UUIDToString(o.ID))
	if len(rows) != 1 || rows[0].Result != "pass" || rows[0].VerifierType != "system" {
		t.Fatalf("after publish verifications = %+v, want one system pass", rows)
	}
}

// TestMetricCriterionFailsAfterWindow: a metric criterion whose window has passed
// without a reading that reaches the value fails, with the metric as its verifier. A
// criterion with a reading in the window passes.
func TestMetricCriterionFailsAfterWindow(t *testing.T) {
	vf := newVerificationFixture(t)
	ctx := context.Background()
	spec := func(key string) map[string]any {
		return map[string]any{"metric_key": key, "scope": "company", "op": ">=", "value": 10, "within_days": 1}
	}

	late := vf.doneIssue(t, vf.doer)
	lateC := vf.addCriterion(t, late, "Reach 10 signups.", "metric", spec("signups-late"))
	vf.Exec(t, `UPDATE acceptance_criterion SET created_at = now() - interval '2 days' WHERE id = $1`, util.UUIDToString(lateC.ID))

	met := vf.doneIssue(t, vf.doer)
	metC := vf.addCriterion(t, met, "Reach 10 signups.", "metric", spec("signups-met"))
	vf.Insert(t, "metric_reading", testutil.Cols{
		"workspace_id": vf.WorkspaceID, "metric_key": "signups-met", "scope_type": "company",
		"value": 12, "observed_at": testutil.Raw("now()"), "actor_type": "system",
	})

	if _, err := vf.checks.CheckMetrics(ctx, time.Now()); err != nil {
		t.Fatalf("check metrics: %v", err)
	}
	late1 := verificationsOf(t, vf.principalFixture, util.UUIDToString(vf.currentOutcome(t, late).ID))
	if len(late1) != 1 || late1[0].Result != "fail" || late1[0].VerifierType != "metric" {
		t.Fatalf("expired metric verifications = %+v, want one metric fail", late1)
	}
	met1 := verificationsOf(t, vf.principalFixture, util.UUIDToString(vf.currentOutcome(t, met).ID))
	if len(met1) != 1 || met1[0].Result != "pass" || met1[0].CriterionID != util.UUIDToString(metC.ID) {
		t.Fatalf("met metric verifications = %+v, want one metric pass", met1)
	}
	// Each criterion is decided once.
	if _, err := vf.checks.CheckMetrics(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if n := vf.Count(t, `SELECT count(*) FROM outcome_verification WHERE criterion_id = $1`, util.UUIDToString(lateC.ID)); n != 1 {
		t.Fatalf("a decided metric criterion was checked again (%d rows)", n)
	}
}

// TestVerificationRunClassHeldByBudget: a hold budget with no room holds the
// verification and shadow classes, and admits a human run.
func TestVerificationRunClassHeldByBudget(t *testing.T) {
	f, _ := companyWorkspace(t)
	ws := util.MustParseUUID(f.WorkspaceID)
	budgets := &BudgetService{Queries: f.q, TxStarter: f.Pool}
	if _, err := budgets.Create(context.Background(), ws, GovernanceActor{Type: GovernanceActorMember, ID: util.MustParseUUID(f.UserID)}, BudgetInput{
		ScopeType: BudgetScopeCompany, Period: BudgetPeriodMonth, AmountMicros: 0, SoftPercent: 80, Mode: BudgetModeHold,
	}); err != nil {
		t.Fatalf("create budget: %v", err)
	}
	gate := NewBudgetGate(events.New())
	scope := func(context.Context) (budgetScope, error) { return budgetScope{}, nil }
	now := time.Now()
	admit := func(class RunClass) bool {
		t.Helper()
		a, err := gate.Admit(context.Background(), f.q, ws, pgtype.UUID{}, class, scope, now)
		if err != nil {
			t.Fatalf("admit %s: %v", class, err)
		}
		return a.Admit
	}
	if admit(RunClassVerification) {
		t.Fatal("a verification task was admitted past a hard budget")
	}
	if admit(RunClassShadow) {
		t.Fatal("a shadow task was admitted past a hard budget")
	}
	if !admit(RunClassHuman) {
		t.Fatal("a human run was held by a budget")
	}
}

// TestBudgetHoldsVerificationWhenItListsRoutine: a budget made before the verification and
// shadow classes existed lists routine only, and still holds them; a budget that lists neither
// holds neither, and human work is never held.
func TestBudgetHoldsVerificationWhenItListsRoutine(t *testing.T) {
	legacy := []string{string(RunClassRoutine), string(RunClassDelegated)}
	if !budgetHoldsClass(legacy, RunClassVerification) || !budgetHoldsClass(legacy, RunClassShadow) {
		t.Fatal("a budget that lists routine must hold verification and shadow")
	}
	if budgetHoldsClass([]string{string(RunClassDelegated)}, RunClassVerification) {
		t.Fatal("a budget without routine held a verification task")
	}
	if budgetHoldsClass(legacy, RunClassHuman) {
		t.Fatal("a budget held a human run")
	}
}

// TestVerifierBriefingHidesOtherVerdicts: the verification briefing carries the issue,
// the criteria and the command to record a check. Another verifier's verdict on the
// same outcome is not in it.
func TestVerifierBriefingHidesOtherVerdicts(t *testing.T) {
	vf := newVerificationFixture(t)
	issue := vf.doneIssue(t, vf.doer)
	c := vf.addCriterion(t, issue, "The export has a header row.", "agent_judgment", map[string]any{"rubric": "Check the header."})
	o := vf.currentOutcome(t, issue)
	vf.Insert(t, "outcome_verification", testutil.Cols{
		"workspace_id": vf.WorkspaceID, "outcome_id": util.UUIDToString(o.ID), "criterion_id": util.UUIDToString(c.ID),
		"verifier_type": "agent", "verifier_id": vf.doer, "result": "fail", "note": "OTHER-VERDICT-TEXT",
	})
	criteria, err := vf.checks.ListCriteria(context.Background(), vf.ws, util.MustParseUUID(issue))
	if err != nil {
		t.Fatal(err)
	}
	brief := VerificationBrief(o, "Export the report", criteria)
	if strings.Contains(brief, "OTHER-VERDICT-TEXT") {
		t.Fatalf("the briefing shows another verifier's verdict:\n%s", brief)
	}
	for _, want := range []string{"The export has a header row.", "multica outcome verify", util.UUIDToString(o.ID)} {
		if !strings.Contains(brief, want) {
			t.Fatalf("briefing lacks %q:\n%s", want, brief)
		}
	}
}

// TestSettledFailPostsSystemComment: a settled fail leaves one system comment on the
// issue that names the doer; the issue stays done.
func TestSettledFailPostsSystemComment(t *testing.T) {
	vf := newVerificationFixture(t)
	issue := vf.doneIssue(t, vf.doer)
	c := vf.addCriterion(t, issue, "The page loads.", "manual", map[string]any{})
	o := vf.currentOutcome(t, issue)
	vf.Insert(t, "outcome_verification", testutil.Cols{
		"workspace_id": vf.WorkspaceID, "outcome_id": util.UUIDToString(o.ID), "criterion_id": util.UUIDToString(c.ID),
		"verifier_type": "member", "verifier_id": vf.UserID, "result": "fail", "severity": "high", "note": "500 on load",
	})
	vf.Exec(t, `UPDATE work_outcome SET settle_at = now() - interval '1 minute' WHERE id = $1`, util.UUIDToString(o.ID))
	if _, err := vf.outcomes.Settle(context.Background(), time.Now()); err != nil {
		t.Fatalf("settle: %v", err)
	}
	settled := vf.currentOutcome(t, issue)
	if settled.Verdict != "fail" || settled.FailCause != "criteria" {
		t.Fatalf("settled = %s/%s, want fail/criteria", settled.Verdict, settled.FailCause)
	}
	var count int
	var content, status string
	vf.Pool.QueryRow(context.Background(), `
		SELECT count(*), COALESCE(max(content), '') FROM comment
		WHERE issue_id = $1 AND author_type = 'system' AND type = 'system'`, issue).Scan(&count, &content)
	if count != 1 || !strings.Contains(content, "fail") || !strings.Contains(content, "mention://agent/"+vf.doer) {
		t.Fatalf("system comments = %d %q, want one naming the doer and the fail", count, content)
	}
	vf.Pool.QueryRow(context.Background(), `SELECT status FROM issue WHERE id = $1`, issue).Scan(&status)
	if status != "done" {
		t.Fatalf("the settled fail reopened the issue: status %q", status)
	}
}

// TestSettleUsesLatestCheckPerCriterion: a later check replaces an earlier one, and the
// earlier row stays in the log.
func TestSettleUsesLatestCheckPerCriterion(t *testing.T) {
	vf := newVerificationFixture(t)
	issue := vf.doneIssue(t, vf.doer)
	c := vf.addCriterion(t, issue, "The page loads.", "manual", map[string]any{})
	o := vf.currentOutcome(t, issue)
	outcome := util.UUIDToString(o.ID)
	vf.Insert(t, "outcome_verification", testutil.Cols{
		"workspace_id": vf.WorkspaceID, "outcome_id": outcome, "criterion_id": util.UUIDToString(c.ID),
		"verifier_type": "member", "verifier_id": vf.UserID, "result": "fail", "severity": "low", "created_at": testutil.Raw("now() - interval '1 hour'"),
	})
	vf.Insert(t, "outcome_verification", testutil.Cols{
		"workspace_id": vf.WorkspaceID, "outcome_id": outcome, "criterion_id": util.UUIDToString(c.ID),
		"verifier_type": "member", "verifier_id": vf.UserID, "result": "pass",
	})
	vf.Exec(t, `UPDATE work_outcome SET settle_at = now() - interval '1 minute' WHERE id = $1`, outcome)
	if _, err := vf.outcomes.Settle(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if settled := vf.currentOutcome(t, issue); settled.Verdict != "pass" {
		t.Fatalf("verdict = %s, want pass from the latest check", settled.Verdict)
	}
	if n := len(verificationsOf(t, vf.principalFixture, outcome)); n != 2 {
		t.Fatalf("the log has %d checks, want both kept", n)
	}
}

// TestCriterionLimitAndSettledEdits: twenty open criteria per issue, and no edit once the
// outcome has settled.
func TestCriterionLimitAndSettledEdits(t *testing.T) {
	vf := newVerificationFixture(t)
	ctx := context.Background()
	issue := outcomeIssue(t, vf.principalFixture, "todo")
	verificationCleanup(t, vf.principalFixture, issue)
	ws := vf.ws
	issueID := util.MustParseUUID(issue)
	for i := 0; i < MaxOpenCriteriaPerIssue; i++ {
		vf.addCriterion(t, issue, fmt.Sprintf("criterion %d", i), "manual", map[string]any{})
	}
	if _, err := vf.checks.AddCriterion(ctx, ws, issueID, vf.member, CriterionInput{Text: "one too many", CheckKind: "manual", CheckSpec: map[string]any{}}); !errors.Is(err, ErrCriterionLimit) {
		t.Fatalf("the 21st open criterion: err = %v, want ErrCriterionLimit", err)
	}

	// Settle the issue's outcome, then try to edit a criterion of it.
	vf.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, issue)
	runOutcomeJob(t, vf.outcomes)
	first, _ := vf.checks.ListCriteria(ctx, ws, issueID)
	vf.Exec(t, `UPDATE work_outcome SET settle_at = now() - interval '1 minute' WHERE issue_id = $1`, issue)
	if _, err := vf.outcomes.Settle(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	text := "edited"
	if _, err := vf.checks.UpdateCriterion(ctx, ws, issueID, first[0].ID, vf.member, CriterionPatch{Text: &text}); !errors.Is(err, ErrOutcomeSettled) {
		t.Fatalf("editing after settle: err = %v, want ErrOutcomeSettled", err)
	}
}

// TestVerifierRecordsOnlyItsOwnTask: an agent records a check only through the task it was
// given for that criterion. The doer may not record one.
func TestVerifierRecordsOnlyItsOwnTask(t *testing.T) {
	vf := newVerificationFixture(t)
	ctx := context.Background()
	issue := vf.doneIssue(t, vf.doer)
	c := vf.addCriterion(t, issue, "The export has a header.", "agent_judgment", map[string]any{"rubric": "Check the header."})
	o := vf.currentOutcome(t, issue)
	outcome := util.UUIDToString(o.ID)
	ws := vf.ws
	verifierAgent := util.MustParseUUID(vf.verifier)

	own := vf.Task(t, vf.verifier, testutil.Cols{"runtime_id": vf.runtime, "issue_id": issue,
		"context": fmt.Sprintf(`{"durable_kind":"verification","outcome_id":"%s"}`, outcome)})
	vf.Exec(t, `INSERT INTO outcome_check_task (workspace_id, outcome_id, criterion_id, task_id) VALUES ($1, $2, $3, $4)`,
		vf.WorkspaceID, outcome, util.UUIDToString(c.ID), own)
	other := vf.Task(t, vf.verifier, testutil.Cols{"runtime_id": vf.runtime, "issue_id": issue, "status": "completed"})

	agent := GovernanceActor{Type: GovernanceActorAgent, ID: verifierAgent}
	in := VerificationInput{CriterionID: c.ID, Result: "pass", TaskID: util.MustParseUUID(other)}
	if _, err := vf.checks.Record(ctx, ws, agent, o.ID, in); !errors.Is(err, ErrNotVerificationTask) {
		t.Fatalf("agent through another task: err = %v, want ErrNotVerificationTask", err)
	}
	in.TaskID = util.MustParseUUID(own)
	if _, err := vf.checks.Record(ctx, ws, agent, o.ID, in); err != nil {
		t.Fatalf("agent through its own task: %v", err)
	}

	// The doer is refused even with its own task, which is not the verifier's check.
	doerTask := vf.Task(t, vf.doer, testutil.Cols{"runtime_id": vf.runtime, "issue_id": issue, "status": "completed"})
	doer := GovernanceActor{Type: GovernanceActorAgent, ID: util.MustParseUUID(vf.doer)}
	in.TaskID = util.MustParseUUID(doerTask)
	if _, err := vf.checks.Record(ctx, ws, doer, o.ID, in); !errors.Is(err, ErrVerifierIsDoer) {
		t.Fatalf("the doer's check: err = %v, want ErrVerifierIsDoer", err)
	}
}

// TestSquadTemplateCriteriaApplied: the criteria a squad template declares are written
// to the issue the squad receives, once.
func TestSquadTemplateCriteriaApplied(t *testing.T) {
	vf := newVerificationFixture(t)
	ctx := context.Background()
	issue := outcomeIssue(t, vf.principalFixture, "todo")
	verificationCleanup(t, vf.principalFixture, issue)
	templates := []TemplateCriterion{
		{Role: "drafter", Text: "The draft follows the form.", CheckKind: "agent_judgment", CheckSpec: map[string]any{"rubric": "Check the form."}},
		{Role: "gate", Text: "The figures match their sources.", CheckKind: "agent_judgment", CheckSpec: map[string]any{"rubric": "Check the figures."}},
	}
	if n, err := vf.checks.ApplyTemplateCriteria(ctx, vf.ws, util.MustParseUUID(issue), templates); err != nil || n != 2 {
		t.Fatalf("apply = %d, %v; want 2 written", n, err)
	}
	if n, err := vf.checks.ApplyTemplateCriteria(ctx, vf.ws, util.MustParseUUID(issue), templates); err != nil || n != 0 {
		t.Fatalf("second apply = %d, %v; want none written", n, err)
	}
	criteria, _ := vf.checks.ListCriteria(ctx, vf.ws, util.MustParseUUID(issue))
	if len(criteria) != 2 || criteria[0].Source != "template" {
		t.Fatalf("criteria = %+v, want two template criteria", criteria)
	}
	var spec map[string]any
	if err := json.Unmarshal(criteria[0].CheckSpec, &spec); err != nil || spec["role"] != "drafter" {
		t.Fatalf("first criterion spec = %s (%v), want the role drafter", criteria[0].CheckSpec, err)
	}
}

// TestOverrideWithinWindowWritesGovernanceEvent: a person changes a settled conclusion
// within the window, and the change is a governance event.
func TestOverrideWithinWindowWritesGovernanceEvent(t *testing.T) {
	vf := newVerificationFixture(t)
	ctx := context.Background()
	issue := vf.doneIssue(t, vf.doer)
	o := vf.currentOutcome(t, issue)
	vf.Exec(t, `UPDATE work_outcome SET settle_at = now() - interval '1 minute' WHERE id = $1`, util.UUIDToString(o.ID))
	if _, err := vf.outcomes.Settle(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	updated, err := vf.checks.Override(ctx, vf.ws, vf.member, o.ID, VerificationInput{Result: "fail", Note: "wrong call"})
	if err != nil {
		t.Fatalf("override: %v", err)
	}
	if updated.Verdict != "fail" || updated.FailCause != "member" {
		t.Fatalf("overridden = %s/%s, want fail/member", updated.Verdict, updated.FailCause)
	}
	if n := vf.Count(t, `SELECT count(*) FROM governance_event WHERE workspace_id = $1 AND kind = 'outcome_overridden' AND subject_id = $2`, vf.WorkspaceID, util.UUIDToString(o.ID)); n != 1 {
		t.Fatalf("governance events = %d, want one outcome_overridden", n)
	}
}

// TestOverrideRefusedAfterWindow: a conclusion settled more than 30 days ago stays.
func TestOverrideRefusedAfterWindow(t *testing.T) {
	vf := newVerificationFixture(t)
	ctx := context.Background()
	issue := vf.doneIssue(t, vf.doer)
	o := vf.currentOutcome(t, issue)
	vf.Exec(t, `UPDATE work_outcome SET settle_at = now() - interval '40 days', settled_at = now() - interval '31 days' WHERE id = $1`, util.UUIDToString(o.ID))
	if _, err := vf.checks.Override(ctx, vf.ws, vf.member, o.ID, VerificationInput{Result: "pass"}); !errors.Is(err, ErrOverrideWindowClosed) {
		t.Fatalf("override after 30 days: err = %v, want ErrOverrideWindowClosed", err)
	}
}

// TestSquadTemplateCriteriaAppliedOnReceipt: a squad receives an issue; the criteria its
// template declares are written, and a criterion the person archived is not written back.
func TestSquadTemplateCriteriaAppliedOnReceipt(t *testing.T) {
	vf := newVerificationFixture(t)
	ctx := context.Background()
	issue := outcomeIssue(t, vf.principalFixture, "todo")
	verificationCleanup(t, vf.principalFixture, issue)
	templateKey := "demo_squad_" + uuid.NewString()[:8]
	criteria := SquadCriteriaByTemplate{templateKey: {
		{Role: "drafter", Text: "The draft follows the form.", CheckKind: CheckKindAgentJudgment, CheckSpec: map[string]any{"rubric": "Check the form."}},
	}}
	templateRef := pgtype.Text{String: templateKey, Valid: true}
	issueRow := db.Issue{ID: util.MustParseUUID(issue), WorkspaceID: vf.ws}

	if _, err := vf.checks.ApplySquadOutcomeCriteria(ctx, issueRow, templateRef, criteria); err != nil {
		t.Fatalf("apply on receipt: %v", err)
	}
	got, _ := vf.checks.ListCriteria(ctx, vf.ws, util.MustParseUUID(issue))
	if len(got) != 1 {
		t.Fatalf("receipt wrote %d criteria, want 1", len(got))
	}
	// The person archives it; a second receipt must not write it again.
	archive := true
	if _, err := vf.checks.UpdateCriterion(ctx, vf.ws, util.MustParseUUID(issue), got[0].ID, vf.member, CriterionPatch{Archive: archive}); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if n, err := vf.checks.ApplySquadOutcomeCriteria(ctx, issueRow, templateRef, criteria); err != nil || n != 0 {
		t.Fatalf("second receipt = %d, %v; want nothing written", n, err)
	}
	// A squad with no template, or a template with no criteria, writes nothing.
	if n, _ := vf.checks.ApplySquadOutcomeCriteria(ctx, issueRow, pgtype.Text{}, criteria); n != 0 {
		t.Fatal("a squad without a template wrote criteria")
	}
	if n, _ := vf.checks.ApplySquadOutcomeCriteria(ctx, issueRow, pgtype.Text{String: "no-such", Valid: true}, criteria); n != 0 {
		t.Fatal("a template without criteria wrote criteria")
	}
}

// TestParseSquadOutcomeCriteria reads the file the catalog build writes.
func TestParseSquadOutcomeCriteria(t *testing.T) {
	raw := []byte(`{"kv_demo":[{"role":"lead","text":"The plan names every step.","check_kind":"agent_judgment","check_spec":{"rubric":"Is it named?"}}]}`)
	got, err := ParseSquadOutcomeCriteria(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got["kv_demo"]) != 1 || got["kv_demo"][0].Role != "lead" || got["kv_demo"][0].CheckSpec["rubric"] != "Is it named?" {
		t.Fatalf("parsed = %+v", got)
	}
	if _, err := ParseSquadOutcomeCriteria([]byte(`{"kv_demo":`)); err == nil {
		t.Fatal("a broken file must be an error, not an empty catalog")
	}
}

// TestLegacyBudgetRowHoldsVerificationAndNewBudgetListsAllFour, on real rows: a budget
// written before the verification and shadow classes existed (routine and delegated only)
// still holds both, and a budget created now stores all four classes. No migration updates
// the old rows: a four-digit migration cannot touch budget (see migrations_order_test.go).
func TestLegacyBudgetRowHoldsVerificationAndNewBudgetListsAllFour(t *testing.T) {
	f, _ := companyWorkspace(t)
	ws := util.MustParseUUID(f.WorkspaceID)
	f.Insert(t, "budget", testutil.Cols{
		"workspace_id": f.WorkspaceID, "scope_type": "company", "period": "month",
		"amount_micros": 0, "soft_percent": 80, "mode": "hold",
		"classes": "{routine,delegated}", "status": "active",
	})
	gate := NewBudgetGate(events.New())
	scope := func(context.Context) (budgetScope, error) { return budgetScope{}, nil }
	admit := func(class RunClass) bool {
		t.Helper()
		a, err := gate.Admit(context.Background(), f.q, ws, pgtype.UUID{}, class, scope, time.Now())
		if err != nil {
			t.Fatalf("admit %s: %v", class, err)
		}
		return a.Admit
	}
	if admit(RunClassVerification) || admit(RunClassShadow) {
		t.Fatal("a legacy budget row that lists routine let a verification or shadow task through")
	}
	if !admit(RunClassHuman) {
		t.Fatal("a legacy budget row held a human run")
	}

	// A budget created now lists all four classes, stored as such.
	other, _ := companyWorkspace(t)
	created, err := (&BudgetService{Queries: other.q, TxStarter: other.Pool}).Create(context.Background(),
		util.MustParseUUID(other.WorkspaceID), GovernanceActor{Type: GovernanceActorMember, ID: util.MustParseUUID(other.UserID)},
		BudgetInput{ScopeType: BudgetScopeCompany, Period: BudgetPeriodMonth, AmountMicros: 1, SoftPercent: 80, Mode: BudgetModeWarn})
	if err != nil {
		t.Fatalf("create budget: %v", err)
	}
	got := created.Classes
	slices.Sort(got)
	if want := []string{"delegated", "routine", "shadow", "verification"}; !slices.Equal(got, want) {
		t.Fatalf("new budget classes = %v, want %v", got, want)
	}
}
