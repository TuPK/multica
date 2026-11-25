package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// outcomeSettleWindow is the default settle window of spec 2.4 (settle_days = 7).
const outcomeSettleWindow = 7 * 24 * time.Hour

// outcomeService returns an OutcomeService over the fixture's pool and the flag
// provider that decides durable_outcomes. The provider stays with the test so it
// can turn the flag on or off mid-test.
func outcomeService(t *testing.T, f principalFixture, flagOn bool) (*OutcomeService, *featureflag.StaticProvider) {
	t.Helper()
	provider := featureflag.NewStaticProvider()
	provider.Set(featureflags.DurableOutcomes, featureflag.Rule{Default: flagOn})
	return &OutcomeService{Queries: f.q, TxStarter: f.Pool, Flags: featureflag.NewService(provider)}, provider
}

// outcomeIssue creates an issue on status and removes the rows the job writes for
// it when the test ends. Cleanups run in reverse, so the outcomes go before the
// transitions they came from, and both before the issue.
func outcomeIssue(t *testing.T, f principalFixture, status string) string {
	t.Helper()
	issue := f.Issue(t, "outcome lifecycle", testutil.Cols{"status": status})
	f.Cleanup(t, `DELETE FROM issue_status_transition WHERE issue_id = $1`, issue)
	f.Cleanup(t, `DELETE FROM work_outcome WHERE issue_id = $1`, issue)
	return issue
}

// labelIssue creates an issue on status that carries one label of the given name.
func labelIssue(t *testing.T, f principalFixture, status, label string) string {
	t.Helper()
	issue := outcomeIssue(t, f, status)
	labelID := f.Insert(t, "issue_label", testutil.Cols{"workspace_id": f.WorkspaceID, "name": label, "color": "#888888"})
	f.Exec(t, `INSERT INTO issue_to_label (issue_id, label_id) VALUES ($1, $2)`, issue, labelID)
	f.Cleanup(t, `DELETE FROM issue_to_label WHERE issue_id = $1`, issue)
	return issue
}

// outcome is one work_outcome row as the tests read it. An absent superseded_by
// reads as "".
type outcome struct {
	ID, DoneTransitionID, SupersededBy              string
	Verdict, FailCause, WorkClass, VerificationMode string
	Sampled, Settled                                bool
	SettleAt                                        time.Time
}

// outcomesOf returns the outcomes of one issue, oldest first. It filters by the
// fixture's issue id, never by a count over the table: other packages write to the
// same database.
func outcomesOf(t *testing.T, f principalFixture, issue string) []outcome {
	t.Helper()
	rows, err := f.Pool.Query(context.Background(), `
		SELECT id::text, done_transition_id::text, COALESCE(superseded_by::text, ''),
		       verdict, fail_cause, work_class, verification_mode, sampled,
		       settle_at, settled_at IS NOT NULL
		FROM work_outcome WHERE issue_id = $1 ORDER BY created_at, id`, issue)
	if err != nil {
		t.Fatalf("list outcomes: %v", err)
	}
	defer rows.Close()
	var out []outcome
	for rows.Next() {
		var o outcome
		if err := rows.Scan(&o.ID, &o.DoneTransitionID, &o.SupersededBy, &o.Verdict, &o.FailCause,
			&o.WorkClass, &o.VerificationMode, &o.Sampled, &o.SettleAt, &o.Settled); err != nil {
			t.Fatalf("scan outcome: %v", err)
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate outcomes: %v", err)
	}
	return out
}

// latestTransitionTo returns the id of the newest transition of issue into key.
func latestTransitionTo(t *testing.T, f principalFixture, issue, key string) string {
	t.Helper()
	var id string
	if err := f.Pool.QueryRow(context.Background(), `
		SELECT id::text FROM issue_status_transition
		WHERE issue_id = $1 AND to_status_key = $2
		ORDER BY created_at DESC, id DESC LIMIT 1`, issue, key).Scan(&id); err != nil {
		t.Fatalf("latest transition to %s: %v", key, err)
	}
	return id
}

// outcomeCursor returns the stored job cursor, or "" when none is stored.
func outcomeCursor(t *testing.T, f principalFixture) string {
	t.Helper()
	var s string
	if err := f.Pool.QueryRow(context.Background(), `
		SELECT COALESCE((SELECT created_at::text || ' ' || transition_id::text
		FROM durable_outcome_cursor WHERE name = 'outcome'), '')`).Scan(&s); err != nil {
		t.Fatalf("read outcome cursor: %v", err)
	}
	return s
}

func runOutcomeJob(t *testing.T, svc *OutcomeService) {
	t.Helper()
	if _, err := svc.ProcessTransitions(context.Background()); err != nil {
		t.Fatalf("process transitions: %v", err)
	}
}

// TestWorkOutcomeCreatedOnceForDone: an issue that enters done gets one pending
// outcome with the default settle window, and a second run of the job adds no row.
func TestWorkOutcomeCreatedOnceForDone(t *testing.T) {
	f, _ := companyWorkspace(t)
	svc, _ := outcomeService(t, f, true)
	issue := outcomeIssue(t, f, "todo")
	f.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, issue)
	doneID := latestTransitionTo(t, f, issue, "done")

	before := time.Now()
	runOutcomeJob(t, svc)
	runOutcomeJob(t, svc)

	got := outcomesOf(t, f, issue)
	if len(got) != 1 {
		t.Fatalf("want one outcome for the done transition, got %d: %+v", len(got), got)
	}
	o := got[0]
	if o.DoneTransitionID != doneID {
		t.Fatalf("done_transition_id = %s, want %s", o.DoneTransitionID, doneID)
	}
	if o.Verdict != "pending" || o.Settled || o.SupersededBy != "" {
		t.Fatalf("new outcome = %+v, want pending, unsettled and current", o)
	}
	if o.WorkClass != "general" {
		t.Fatalf("work_class = %q, want general (no flow, label or squad)", o.WorkClass)
	}
	// TODO(A6): TestOutcomeSampledFollowsTrackRecord replaces this fixed mode. Until
	// then every outcome is verified in full.
	if o.VerificationMode != "full" || !o.Sampled {
		t.Fatalf("verification = %s sampled=%v, want full and sampled", o.VerificationMode, o.Sampled)
	}
	if d := o.SettleAt.Sub(before); d < outcomeSettleWindow || d > outcomeSettleWindow+time.Minute {
		t.Fatalf("settle_at is %s after the job ran, want about %s", d, outcomeSettleWindow)
	}
}

// TestReopenMakesFailThenNewOutcome: done, then started (the outcome becomes
// fail/reopened), then done again (a new outcome is created and supersedes the old).
func TestReopenMakesFailThenNewOutcome(t *testing.T) {
	f, _ := companyWorkspace(t)
	svc, _ := outcomeService(t, f, true)
	issue := outcomeIssue(t, f, "todo")

	f.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, issue)
	runOutcomeJob(t, svc)
	first := outcomesOf(t, f, issue)
	if len(first) != 1 {
		t.Fatalf("after done: want one outcome, got %d", len(first))
	}

	f.Exec(t, `UPDATE issue SET status = 'in_progress' WHERE id = $1`, issue)
	runOutcomeJob(t, svc)
	reopened := outcomesOf(t, f, issue)
	if len(reopened) != 1 || reopened[0].Verdict != "fail" || reopened[0].FailCause != "reopened" || reopened[0].Settled {
		t.Fatalf("after reopen: outcome = %+v, want fail/reopened and unsettled", reopened)
	}

	f.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, issue)
	runOutcomeJob(t, svc)
	got := outcomesOf(t, f, issue)
	if len(got) != 2 {
		t.Fatalf("after second done: want two outcomes, got %d: %+v", len(got), got)
	}
	old, next := got[0], got[1]
	if old.ID != first[0].ID || old.Verdict != "fail" || old.FailCause != "reopened" {
		t.Fatalf("old outcome = %+v, want the first one, fail/reopened", old)
	}
	if old.SupersededBy != next.ID {
		t.Fatalf("old outcome superseded_by = %q, want the new outcome %s", old.SupersededBy, next.ID)
	}
	if next.Verdict != "pending" || next.SupersededBy != "" || next.FailCause != "" {
		t.Fatalf("new outcome = %+v, want pending and current", next)
	}
}

// TestCancelFromDoneFailsReverted: done → cancelled settles the current outcome as
// fail/reverted (D31: this is the only revert signal).
func TestCancelFromDoneFailsReverted(t *testing.T) {
	f, _ := companyWorkspace(t)
	svc, _ := outcomeService(t, f, true)
	issue := outcomeIssue(t, f, "todo")

	f.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, issue)
	runOutcomeJob(t, svc)
	f.Exec(t, `UPDATE issue SET status = 'cancelled' WHERE id = $1`, issue)
	runOutcomeJob(t, svc)

	got := outcomesOf(t, f, issue)
	if len(got) != 1 {
		t.Fatalf("want one outcome, got %d", len(got))
	}
	if got[0].Verdict != "fail" || got[0].FailCause != "reverted" || !got[0].Settled {
		t.Fatalf("outcome = %+v, want fail/reverted and settled", got[0])
	}
}

// TestWipSkipLabelNoOutcome: an issue labelled wc:skip writes no outcome, and a
// labelled wc:research issue gets its work class from the label.
func TestWipSkipLabelNoOutcome(t *testing.T) {
	f, _ := companyWorkspace(t)
	svc, _ := outcomeService(t, f, true)
	skipped := labelIssue(t, f, "todo", "wc:skip")
	research := labelIssue(t, f, "todo", "wc:research")
	f.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, skipped)
	f.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, research)

	runOutcomeJob(t, svc)

	if got := outcomesOf(t, f, skipped); len(got) != 0 {
		t.Fatalf("wc:skip issue wrote %d outcomes, want none", len(got))
	}
	got := outcomesOf(t, f, research)
	if len(got) != 1 || got[0].WorkClass != "label:research" {
		t.Fatalf("wc:research outcome = %+v, want one with work_class label:research", got)
	}
}

// TestSettleJobSettlesOnlyDue: only the outcome whose settle_at has passed is
// settled; the other stays pending.
func TestSettleJobSettlesOnlyDue(t *testing.T) {
	f, _ := companyWorkspace(t)
	svc, _ := outcomeService(t, f, true)
	due := outcomeIssue(t, f, "todo")
	notDue := outcomeIssue(t, f, "todo")
	f.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, due)
	f.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, notDue)
	runOutcomeJob(t, svc)
	f.Exec(t, `UPDATE work_outcome SET settle_at = now() - interval '1 hour' WHERE issue_id = $1`, due)

	if _, err := svc.Settle(context.Background(), time.Now()); err != nil {
		t.Fatalf("settle: %v", err)
	}

	got := outcomesOf(t, f, due)
	if len(got) != 1 || !got[0].Settled || got[0].Verdict != "unverified" {
		t.Fatalf("due outcome = %+v, want settled as unverified (sampled, no criterion results)", got)
	}
	got = outcomesOf(t, f, notDue)
	if len(got) != 1 || got[0].Settled || got[0].Verdict != "pending" {
		t.Fatalf("not-due outcome = %+v, want still pending and unsettled", got)
	}
}

// TestUnsampledSettlesUnverified: an outcome that was not chosen for checking
// settles as unverified (spec 2.6), not pass.
func TestUnsampledSettlesUnverified(t *testing.T) {
	f, _ := companyWorkspace(t)
	svc, _ := outcomeService(t, f, true)
	issue := outcomeIssue(t, f, "todo")
	f.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, issue)
	runOutcomeJob(t, svc)
	f.Exec(t, `UPDATE work_outcome SET sampled = false, settle_at = now() - interval '1 hour' WHERE issue_id = $1`, issue)

	if _, err := svc.Settle(context.Background(), time.Now()); err != nil {
		t.Fatalf("settle: %v", err)
	}

	got := outcomesOf(t, f, issue)
	if len(got) != 1 || got[0].Verdict != "unverified" || !got[0].Settled || got[0].FailCause != "" {
		t.Fatalf("unsampled outcome = %+v, want unverified, settled, no fail cause", got)
	}
}

// TestOutcomeFlagOffStopsWrites: with durable_outcomes off, neither the transition
// job nor settlement writes, and the cursor does not move. Turning the flag on
// resumes from the stopped cursor.
func TestOutcomeFlagOffStopsWrites(t *testing.T) {
	f, _ := companyWorkspace(t)
	svc, provider := outcomeService(t, f, false)
	ctx := context.Background()
	issue := outcomeIssue(t, f, "todo")
	f.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, issue)

	// A due outcome, planted directly, lets the settle half of the job be tested too.
	planted := f.Insert(t, "work_outcome", testutil.Cols{
		"workspace_id": f.WorkspaceID, "issue_id": issue, "done_transition_id": uuid.NewString(),
		"work_class": "general", "verification_mode": "full", "sampled": true,
		"settle_at": testutil.Raw("now() - interval '1 hour'"),
	})

	cursorBefore := outcomeCursor(t, f)
	if n, err := svc.ProcessTransitions(ctx); err != nil || n != 0 {
		t.Fatalf("flag off: ProcessTransitions = %d, %v; want 0, nil", n, err)
	}
	if _, err := svc.Settle(ctx, time.Now()); err != nil {
		t.Fatalf("flag off: settle: %v", err)
	}
	// The planted row shares the issue id, so only rows other than it count as writes.
	for _, o := range outcomesOf(t, f, issue) {
		if o.ID != planted {
			t.Fatalf("flag off wrote an outcome from the transition: %+v", o)
		}
	}
	if f.Count(t, `SELECT count(*) FROM work_outcome WHERE id = $1 AND settled_at IS NOT NULL`, planted) != 0 {
		t.Fatalf("flag off settled an outcome")
	}
	if after := outcomeCursor(t, f); after != cursorBefore {
		t.Fatalf("flag off moved the cursor from %q to %q", cursorBefore, after)
	}

	provider.Set(featureflags.DurableOutcomes, featureflag.Rule{Default: true})
	runOutcomeJob(t, svc)
	if _, err := svc.Settle(ctx, time.Now()); err != nil {
		t.Fatalf("flag on: settle: %v", err)
	}
	fromTransition := 0
	for _, o := range outcomesOf(t, f, issue) {
		if o.ID != planted {
			fromTransition++
		}
	}
	if fromTransition != 1 {
		t.Fatalf("flag back on: want the stopped transition read into one outcome, got %d", fromTransition)
	}
	if f.Count(t, `SELECT count(*) FROM work_outcome WHERE id = $1 AND settled_at IS NOT NULL`, planted) != 1 {
		t.Fatalf("flag back on: the due outcome was not settled")
	}
}

// TestLateCommitIsPickedUpInOverlap: a transaction that is still open when the job
// reads another transaction's commit can commit later. The overlap read must still
// apply its transition, and only once.
func TestLateCommitIsPickedUpInOverlap(t *testing.T) {
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
		t.Fatalf("open transaction update: %v", err)
	}

	// Committed while the first transaction is still open, so its created_at is later.
	f.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, other)
	runOutcomeJob(t, svc)
	if got := outcomesOf(t, f, late); len(got) != 0 {
		t.Fatalf("uncommitted transition was read: %+v", got)
	}
	if got := outcomesOf(t, f, other); len(got) != 1 {
		t.Fatalf("committed transition: want one outcome, got %d", len(got))
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit late transaction: %v", err)
	}
	lateDone := latestTransitionTo(t, f, late, "done")
	runOutcomeJob(t, svc)
	runOutcomeJob(t, svc)

	if got := outcomesOf(t, f, late); len(got) != 1 {
		t.Fatalf("late commit: want one outcome, got %d: %+v", len(got), got)
	}
	if n := f.Count(t, `SELECT count(*) FROM durable_outcome_applied WHERE transition_id = $1`, lateDone); n != 1 {
		t.Fatalf("late commit: applied %d times, want once", n)
	}
}

// TestReplayedOverlapAppliesOnce: rereading transitions the job already applied, as
// a restarted job does after it loses its position, creates no further outcome.
func TestReplayedOverlapAppliesOnce(t *testing.T) {
	f, _ := companyWorkspace(t)
	svc, _ := outcomeService(t, f, true)
	issue := outcomeIssue(t, f, "todo")
	f.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, issue)
	doneID := latestTransitionTo(t, f, issue, "done")
	runOutcomeJob(t, svc)

	// Rewind the cursor to just before the transition; the next run rereads it.
	f.Exec(t, `UPDATE durable_outcome_cursor
		SET created_at = (SELECT created_at - interval '1 second' FROM issue_status_transition WHERE id = $1),
		    transition_id = '00000000-0000-0000-0000-000000000000'
		WHERE name = 'outcome'`, doneID)
	runOutcomeJob(t, svc)
	runOutcomeJob(t, svc)

	if got := outcomesOf(t, f, issue); len(got) != 1 {
		t.Fatalf("replayed overlap: want one outcome, got %d: %+v", len(got), got)
	}
	if n := f.Count(t, `SELECT count(*) FROM durable_outcome_applied WHERE transition_id = $1`, doneID); n != 1 {
		t.Fatalf("replayed overlap: applied %d times, want once", n)
	}
}
