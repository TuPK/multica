package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestSettleLocksAndRereadsReopenedOutcome(t *testing.T) {
	f, _ := companyWorkspace(t)
	svc, _ := outcomeService(t, f, true)
	ctx := context.Background()

	issue := outcomeIssue(t, f, "todo")
	f.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, issue)
	runOutcomeJob(t, svc)

	got := outcomesOf(t, f, issue)
	if len(got) != 1 {
		t.Fatalf("outcomes = %d, want 1", len(got))
	}
	outcomeID := util.MustParseUUID(got[0].ID)
	wsID := util.MustParseUUID(f.WorkspaceID)
	issueID := util.MustParseUUID(issue)

	f.Exec(t, `UPDATE work_outcome SET settle_at = now() - interval '1 minute' WHERE id = $1`, got[0].ID)
	stale, err := f.q.GetWorkOutcomeInWorkspace(ctx, db.GetWorkOutcomeInWorkspaceParams{ID: outcomeID, WorkspaceID: wsID})
	if err != nil {
		t.Fatalf("read stale outcome: %v", err)
	}

	// Reopen commits between ListDueWorkOutcomes and settleOne's transaction.
	n, err := f.q.ReopenCurrentWorkOutcome(ctx, db.ReopenCurrentWorkOutcomeParams{WorkspaceID: wsID, IssueID: issueID})
	if err != nil || n != 1 {
		t.Fatalf("reopen outcome: n=%d err=%v", n, err)
	}

	if _, err := svc.settleOne(ctx, stale, pgtype.Timestamptz{Time: time.Now(), Valid: true}); err != nil {
		t.Fatalf("settleOne: %v", err)
	}

	after := outcomesOf(t, f, issue)
	if len(after) != 1 || after[0].Verdict != "fail" || after[0].FailCause != "reopened" {
		t.Fatalf("after settleOne with stale read: verdict=%q fail_cause=%q, want fail/reopened", after[0].Verdict, after[0].FailCause)
	}
}

func TestLateCommitPastOverlapWindowIsStillApplied(t *testing.T) {
	f, _ := companyWorkspace(t)
	svc, _ := outcomeService(t, f, true)
	ctx := context.Background()

	late := outcomeIssue(t, f, "todo")
	other := outcomeIssue(t, f, "todo")

	tx, err := f.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE issue SET status = 'done' WHERE id = $1`, late); err != nil {
		t.Fatalf("open tx update: %v", err)
	}
	// Backdate the uncommitted transition's created_at by 10 minutes to simulate a
	// long-running transaction whose created_at is beyond the old 5-minute overlap.
	if _, err := tx.Exec(ctx, `UPDATE issue_status_transition SET created_at = now() - interval '10 minutes' WHERE issue_id = $1`, late); err != nil {
		t.Fatalf("backdate open tx transition: %v", err)
	}

	// Commit another transition with current timestamp and advance the cursor past it.
	f.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, other)
	runOutcomeJob(t, svc)
	if got := outcomesOf(t, f, other); len(got) != 1 {
		t.Fatalf("other outcomes = %d, want 1", len(got))
	}
	if got := outcomesOf(t, f, late); len(got) != 0 {
		t.Fatalf("uncommitted late transition produced %d outcomes, want 0", len(got))
	}

	// Now commit the late transaction (whose created_at is >5m before the cursor).
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit late tx: %v", err)
	}
	runOutcomeJob(t, svc)

	if got := outcomesOf(t, f, late); len(got) != 1 {
		t.Fatalf("late-committing transition past overlap: outcomes = %d, want 1", len(got))
	}
}

func TestFailingTransitionIsolatedAndRetried(t *testing.T) {
	fA, _ := companyWorkspace(t)
	fB, _ := companyWorkspace(t)
	svc, _ := outcomeService(t, fA, true)
	ctx := context.Background()

	issueA := outcomeIssue(t, fA, "todo")
	issueB := outcomeIssue(t, fB, "todo")
	fA.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, issueA)
	fB.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, issueB)
	transA := latestTransitionTo(t, fA, issueA, "done")

	attemptsA := 0
	failA := true
	svc.testApplyErr = func(r db.ListIssueTransitionsAfterCursorRow) error {
		if util.UUIDToString(r.ID) == transA {
			attemptsA++
			if failA {
				return errors.New("simulated transition failure")
			}
		}
		return nil
	}

	// Run 1: workspace A transition fails, workspace B transition must still succeed.
	if _, err := svc.ProcessTransitions(ctx); err != nil {
		t.Fatalf("run 1 returned error instead of isolating failing transition: %v", err)
	}
	if got := outcomesOf(t, fB, issueB); len(got) != 1 {
		t.Fatalf("workspace B outcomes = %d, want 1 despite workspace A failure", len(got))
	}
	if got := outcomesOf(t, fA, issueA); len(got) != 0 {
		t.Fatalf("failing workspace A outcomes = %d, want 0", len(got))
	}

	// Verify failure was recorded.
	var status, lastErr string
	var recordedAttempts int
	if err := fA.Pool.QueryRow(ctx, `SELECT status, attempts, last_error FROM durable_outcome_applied WHERE transition_id = $1`, transA).Scan(&status, &recordedAttempts, &lastErr); err != nil {
		t.Fatalf("read recorded failure: %v", err)
	}
	if status != "failed" || recordedAttempts != 1 || lastErr == "" {
		t.Fatalf("recorded failure = (%q, %d, %q), want (failed, 1, non-empty)", status, recordedAttempts, lastErr)
	}

	// Run 2: failing transition is retried and succeeds when error clears.
	failA = false
	if _, err := svc.ProcessTransitions(ctx); err != nil {
		t.Fatalf("run 2 retry: %v", err)
	}
	if attemptsA != 2 {
		t.Fatalf("attempts for transition A = %d, want 2", attemptsA)
	}
	if got := outcomesOf(t, fA, issueA); len(got) != 1 {
		t.Fatalf("workspace A outcomes after retry = %d, want 1", len(got))
	}
}

func TestFailingTransitionDeadLettersAfterLimitedRetries(t *testing.T) {
	f, _ := companyWorkspace(t)
	svc, _ := outcomeService(t, f, true)
	issue := outcomeIssue(t, f, "todo")
	f.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, issue)
	transition := latestTransitionTo(t, f, issue, "done")
	attempts := 0
	svc.testApplyErr = func(r db.ListIssueTransitionsAfterCursorRow) error {
		if util.UUIDToString(r.ID) == transition {
			attempts++
			return errors.New("permanent failure")
		}
		return nil
	}
	for i := 0; i < 5; i++ {
		if _, err := svc.ProcessTransitions(context.Background()); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	var status, lastErr string
	var recorded int
	if err := f.Pool.QueryRow(context.Background(), `SELECT status, attempts, last_error FROM durable_outcome_applied WHERE transition_id = $1`, transition).Scan(&status, &recorded, &lastErr); err != nil {
		t.Fatal(err)
	}
	if status != "dead_letter" || recorded != 3 || attempts != 3 || lastErr == "" {
		t.Fatalf("status=%q recorded=%d attempts=%d error=%q; want dead_letter and 3 attempts", status, recorded, attempts, lastErr)
	}
}

func TestOutcomeWorkspaceSettleDaysAndDisabledProject(t *testing.T) {
	f, _ := companyWorkspace(t)
	svc, _ := outcomeService(t, f, true)
	now := time.Now().UTC().Truncate(time.Second)
	svc.Now = func() time.Time { return now }
	f.Exec(t, `UPDATE workspace SET settings = jsonb_set(COALESCE(settings, '{}'::jsonb), '{settle_days}', '14'::jsonb) WHERE id = $1`, f.WorkspaceID)
	project := f.Project(t, "disabled outcomes", testutil.Cols{"outcomes_disabled": true})
	skipped := outcomeIssue(t, f, "todo")
	kept := outcomeIssue(t, f, "todo")
	f.Exec(t, `UPDATE issue SET project_id = $1 WHERE id = $2`, project, skipped)
	f.Exec(t, `UPDATE issue SET status = 'done' WHERE id IN ($1, $2)`, skipped, kept)
	runOutcomeJob(t, svc)
	if got := outcomesOf(t, f, skipped); len(got) != 0 {
		t.Fatalf("disabled project produced %d outcomes", len(got))
	}
	got := outcomesOf(t, f, kept)
	if len(got) != 1 || !got[0].SettleAt.Equal(now.Add(14*24*time.Hour)) {
		t.Fatalf("settle outcome = %+v, want 14 days", got)
	}
}

func TestOutcomeUsesTransitionAssigneeAndLabels(t *testing.T) {
	f, _ := companyWorkspace(t)
	svc, _ := outcomeService(t, f, true)
	issue := labelIssue(t, f, "todo", "wc:skip")
	f.Exec(t, `UPDATE issue SET assignee_type = 'member', assignee_id = $1, status = 'done' WHERE id = $2`, f.UserID, issue)
	f.Exec(t, `DELETE FROM issue_to_label WHERE issue_id = $1`, issue)
	runOutcomeJob(t, svc)
	if got := outcomesOf(t, f, issue); len(got) != 0 {
		t.Fatalf("skip label at transition produced %d outcomes", len(got))
	}

	issue2 := labelIssue(t, f, "todo", "wc:research")
	f.Exec(t, `UPDATE issue SET assignee_type = 'member', assignee_id = $1, status = 'done' WHERE id = $2`, f.UserID, issue2)
	f.Exec(t, `UPDATE issue SET assignee_id = NULL, assignee_type = NULL WHERE id = $1`, issue2)
	f.Exec(t, `DELETE FROM issue_to_label WHERE issue_id = $1`, issue2)
	runOutcomeJob(t, svc)
	if got := outcomesOf(t, f, issue2); len(got) != 1 || got[0].WorkClass != "label:research" {
		t.Fatalf("snapshot work class = %+v", got)
	}
	var member string
	if err := f.Pool.QueryRow(context.Background(), `SELECT member_id::text FROM work_outcome WHERE issue_id = $1`, issue2).Scan(&member); err != nil {
		t.Fatal(err)
	}
	if member != f.UserID {
		t.Fatalf("outcome member = %q, want %q", member, f.UserID)
	}
}

func TestAgentOutcomeUsesHomeSquad(t *testing.T) {
	f, _ := companyWorkspace(t)
	svc, _ := outcomeService(t, f, true)
	agent := f.Agent(t, "outcome doer", "")
	squad := f.Squad(t, "home squad", agent)
	f.Exec(t, `UPDATE agent SET home_squad_id = $1 WHERE id = $2`, squad, agent)
	issue := outcomeIssue(t, f, "todo")
	f.Exec(t, `UPDATE issue SET assignee_type = 'agent', assignee_id = $1, status = 'done' WHERE id = $2`, agent, issue)
	runOutcomeJob(t, svc)
	if _, err := svc.Settle(context.Background(), time.Now().Add(8*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	var got string
	if err := f.Pool.QueryRow(context.Background(), `SELECT squad_id::text FROM work_outcome WHERE issue_id = $1 AND settled_at IS NOT NULL`, issue).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != squad {
		t.Fatalf("outcome squad = %q, want %q", got, squad)
	}
}

// Regression test: an unsettled fail outcome whose issue was deleted before Settle runs must not
// poison the global Settle sweep with "post settled note: no rows in result set".
func TestSettleIgnoresDeletedIssueForSettledNote(t *testing.T) {
	f, _ := companyWorkspace(t)
	svc, _ := outcomeService(t, f, true)
	ctx := context.Background()

	orphanIssue := outcomeIssue(t, f, "todo")
	f.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, orphanIssue)
	runOutcomeJob(t, svc)
	// Reopen puts the outcome into fail/reopened, which triggers postSettledNote on settle.
	f.Exec(t, `UPDATE issue SET status = 'in_progress' WHERE id = $1`, orphanIssue)
	runOutcomeJob(t, svc)
	// Delete the issue before settlement (no FK cascade).
	f.Exec(t, `DELETE FROM issue WHERE id = $1`, orphanIssue)

	// A healthy issue in the same workspace must still settle cleanly.
	healthyIssue := outcomeIssue(t, f, "todo")
	f.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, healthyIssue)
	runOutcomeJob(t, svc)

	if _, err := svc.Settle(ctx, time.Now().Add(8*24*time.Hour)); err != nil {
		t.Fatalf("Settle failed due to deleted issue: %v", err)
	}
	if n := f.Count(t, `SELECT COUNT(*) FROM work_outcome WHERE issue_id = $1 AND settled_at IS NOT NULL`, healthyIssue); n != 1 {
		t.Fatalf("healthy issue settled outcomes = %d, want 1", n)
	}
}
