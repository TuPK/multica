package service

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// issueTransitions returns the transitions of one issue, oldest first, read
// through the Go read path so categories are resolved. It filters by the
// fixture's issue id, never by a count over the whole table, because other
// packages write to the same database.
func issueTransitions(t *testing.T, f principalFixture, issueID string) []IssueStatusTransitionRecord {
	t.Helper()
	got, err := ListIssueStatusTransitionRecords(context.Background(), f.q,
		util.MustParseUUID(f.WorkspaceID), util.MustParseUUID(issueID))
	if err != nil {
		t.Fatalf("list transitions: %v", err)
	}
	return got
}

// transitionIssue creates an issue on status todo and removes its transition
// rows when the test ends, so the shared database keeps no rows of this test.
func transitionIssue(t *testing.T, f principalFixture) string {
	t.Helper()
	issue := f.Issue(t, "status transition", testutil.Cols{"status": "todo"})
	f.Cleanup(t, `DELETE FROM issue_status_transition WHERE issue_id = $1`, issue)
	return issue
}

// TestStatusTriggerRecordsOneRow: one status change writes exactly one row with
// the old and new key, their lifecycle categories, and the system actor, because
// no session was set.
func TestStatusTriggerRecordsOneRow(t *testing.T) {
	f, _ := companyWorkspace(t)
	issue := transitionIssue(t, f)

	f.Exec(t, `UPDATE issue SET status = 'in_progress' WHERE id = $1`, issue)

	got := issueTransitions(t, f, issue)
	if len(got) != 1 {
		t.Fatalf("want exactly one transition row, got %d: %+v", len(got), got)
	}
	r := got[0]
	if r.FromStatusKey != "todo" || r.ToStatusKey != "in_progress" {
		t.Fatalf("status pair = (%s, %s), want (todo, in_progress)", r.FromStatusKey, r.ToStatusKey)
	}
	if r.FromCategory != "unstarted" || r.ToCategory != "started" {
		t.Fatalf("category pair = (%s, %s), want (unstarted, started)", r.FromCategory, r.ToCategory)
	}
	if r.ActorType != "system" || r.ActorID.Valid || r.TaskID.Valid {
		t.Fatalf("actor = (%s, %v, %v), want (system, NULL, NULL)", r.ActorType, r.ActorID, r.TaskID)
	}
	if r.Source != "live" {
		t.Fatalf("source = %s, want live", r.Source)
	}
}

// TestStatusTriggerCatchesRawSQL: a status write that never goes through the
// application (plain SQL, as a sweeper would issue it) still leaves its row.
func TestStatusTriggerCatchesRawSQL(t *testing.T) {
	f, _ := companyWorkspace(t)
	issue := transitionIssue(t, f)

	f.Exec(t, `UPDATE issue SET status = 'cancelled' WHERE id = $1`, issue)

	got := issueTransitions(t, f, issue)
	if len(got) != 1 {
		t.Fatalf("raw SQL update: want one transition row, got %d", len(got))
	}
	if got[0].ToCategory != "closed" || got[0].ActorType != "system" {
		t.Fatalf("raw SQL row = (to_category %s, actor %s), want (closed, system)", got[0].ToCategory, got[0].ActorType)
	}
}

// TestStatusTriggerIgnoresUnchangedStatus: a write that leaves status alone
// (here, a title change and a status set to its current value) writes no row.
func TestStatusTriggerIgnoresUnchangedStatus(t *testing.T) {
	f, _ := companyWorkspace(t)
	issue := transitionIssue(t, f)

	f.Exec(t, `UPDATE issue SET title = 'renamed' WHERE id = $1`, issue)
	f.Exec(t, `UPDATE issue SET status = status WHERE id = $1`, issue)

	if got := issueTransitions(t, f, issue); len(got) != 0 {
		t.Fatalf("unchanged status wrote %d rows: %+v", len(got), got)
	}
}

// TestActorSessionFlowsIntoRow: a status change inside a transaction that set
// the actor session records that actor, and the task id when one is given.
func TestActorSessionFlowsIntoRow(t *testing.T) {
	f, _ := companyWorkspace(t)
	ctx := context.Background()
	runtimeID, agentID := ownerAndLeader(t, f, "Transition Actor")
	taskID := f.Task(t, agentID, testutil.Cols{"runtime_id": runtimeID})

	cases := []struct {
		name      string
		actor     GovernanceActor
		task      pgtype.UUID
		wantType  string
		wantActor string
		wantTask  string
	}{
		{
			name:      "member",
			actor:     GovernanceActor{Type: GovernanceActorMember, ID: util.MustParseUUID(f.UserID)},
			wantType:  "member",
			wantActor: f.UserID,
		},
		{
			name:      "agent with task",
			actor:     GovernanceActor{Type: GovernanceActorAgent, ID: util.MustParseUUID(agentID)},
			task:      util.MustParseUUID(taskID),
			wantType:  "agent",
			wantActor: agentID,
			wantTask:  taskID,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			issue := transitionIssue(t, f)
			tx, err := f.Pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)

			if err := WithActorSession(ctx, f.q.WithTx(tx), c.actor, c.task); err != nil {
				t.Fatalf("WithActorSession: %v", err)
			}
			if _, err := tx.Exec(ctx, `UPDATE issue SET status = 'in_progress' WHERE id = $1`, issue); err != nil {
				t.Fatalf("update status: %v", err)
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatalf("commit: %v", err)
			}

			got := issueTransitions(t, f, issue)
			if len(got) != 1 {
				t.Fatalf("want one transition row, got %d", len(got))
			}
			if got[0].ActorType != c.wantType {
				t.Fatalf("actor_type = %s, want %s", got[0].ActorType, c.wantType)
			}
			if gotActor := util.UUIDToString(got[0].ActorID); gotActor != c.wantActor {
				t.Fatalf("actor_id = %s, want %s", gotActor, c.wantActor)
			}
			gotTask := ""
			if got[0].TaskID.Valid {
				gotTask = util.UUIDToString(got[0].TaskID)
			}
			if gotTask != c.wantTask {
				t.Fatalf("task_id = %q, want %q", gotTask, c.wantTask)
			}
		})
	}
}

// TestActorSessionOutsideTransactionRejected: WithActorSession on a connection
// that is not inside a transaction must fail. SET LOCAL there would vanish at
// once and the row would be attributed to system without anyone noticing.
func TestActorSessionOutsideTransactionRejected(t *testing.T) {
	f, _ := companyWorkspace(t)
	actor := GovernanceActor{Type: GovernanceActorMember, ID: util.MustParseUUID(f.UserID)}

	if err := WithActorSession(context.Background(), f.q, actor, pgtype.UUID{}); err == nil {
		t.Fatal("WithActorSession outside a transaction returned nil, want an error")
	}
}

// TestStatusTriggerInvalidActorUUIDFallsBackToSystem: a malformed actor id in
// the session must not break the status write. The row is written as system
// with no actor id.
func TestStatusTriggerInvalidActorUUIDFallsBackToSystem(t *testing.T) {
	f, _ := companyWorkspace(t)
	ctx := context.Background()
	issue := transitionIssue(t, f)

	tx, err := f.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT set_config('multica.actor_type', 'agent', true), set_config('multica.actor_id', 'not-a-uuid', true)`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE issue SET status = 'in_progress' WHERE id = $1`, issue); err != nil {
		t.Fatalf("status update with a malformed actor id must succeed: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	got := issueTransitions(t, f, issue)
	if len(got) != 1 {
		t.Fatalf("want one transition row, got %d", len(got))
	}
	if got[0].ActorType != "system" || got[0].ActorID.Valid {
		t.Fatalf("actor = (%s, %v), want (system, NULL)", got[0].ActorType, got[0].ActorID)
	}
}

// TestTransitionRowsIndependentOfFlag: durable_outcomes is off by default. The
// flag is read by the Go services that score outcomes, never by the trigger, so
// a status change writes its row with the flag off too.
func TestTransitionRowsIndependentOfFlag(t *testing.T) {
	f, _ := companyWorkspace(t)
	issue := transitionIssue(t, f)

	f.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, issue)

	if got := issueTransitions(t, f, issue); len(got) != 1 || got[0].ToCategory != "done" {
		t.Fatalf("flag-off status change: want one done row, got %+v", got)
	}
}

// TestListIssueStatusTransitionsReturnsRowsOfOneIssue checks the raw read query
// the later durable tasks build on: only the requested issue's rows, oldest first.
func TestListIssueStatusTransitionsReturnsRowsOfOneIssue(t *testing.T) {
	f, _ := companyWorkspace(t)
	ctx := context.Background()
	issue := transitionIssue(t, f)
	other := transitionIssue(t, f)

	f.Exec(t, `UPDATE issue SET status = 'in_progress' WHERE id = $1`, issue)
	f.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, issue)
	f.Exec(t, `UPDATE issue SET status = 'in_progress' WHERE id = $1`, other)

	rows, err := f.q.ListIssueStatusTransitions(ctx, db.ListIssueStatusTransitionsParams{
		WorkspaceID: util.MustParseUUID(f.WorkspaceID),
		IssueID:     util.MustParseUUID(issue),
	})
	if err != nil {
		t.Fatalf("ListIssueStatusTransitions: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want two rows for the issue, got %d", len(rows))
	}
	if rows[0].ToStatusKey != "in_progress" || rows[1].ToStatusKey != "done" {
		t.Fatalf("order = (%s, %s), want (in_progress, done)", rows[0].ToStatusKey, rows[1].ToStatusKey)
	}
}

// TestLatestDoneTransitionFindsTheLastDoneRow checks LatestDoneTransition, which
// the outcome task reads to find the time an issue last entered done.
func TestLatestDoneTransitionFindsTheLastDoneRow(t *testing.T) {
	f, _ := companyWorkspace(t)
	ctx := context.Background()
	issue := transitionIssue(t, f)

	if _, err := f.q.LatestDoneTransition(ctx, util.MustParseUUID(issue)); err == nil {
		t.Fatal("no done transition yet: want an error (no rows), got nil")
	}
	f.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, issue)
	f.Exec(t, `UPDATE issue SET status = 'in_progress' WHERE id = $1`, issue)
	f.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, issue)

	row, err := f.q.LatestDoneTransition(ctx, util.MustParseUUID(issue))
	if err != nil {
		t.Fatalf("LatestDoneTransition: %v", err)
	}
	if row.ToStatusKey != "done" || row.FromStatusKey.String != "in_progress" {
		t.Fatalf("latest done = (from %v, to %s), want (from in_progress, to done)", row.FromStatusKey, row.ToStatusKey)
	}
}

// TestLatestDoneTransitionCountsCustomDoneStatus: a custom status whose catalog
// category is done counts as done, and one in another category does not. This
// is the catalog join that a built-in key fallback cannot give.
func TestLatestDoneTransitionCountsCustomDoneStatus(t *testing.T) {
	f, _ := companyWorkspace(t)
	ctx := context.Background()
	issue := transitionIssue(t, f)
	f.Insert(t, "issue_status", testutil.Cols{
		"workspace_id": f.WorkspaceID, "key": "shipped", "name": "Shipped",
		"category": "done", "color": "#22c55e", "position": 9,
	})
	f.Insert(t, "issue_status", testutil.Cols{
		"workspace_id": f.WorkspaceID, "key": "reviewing", "name": "Reviewing",
		"category": "started", "color": "#f59e0b", "position": 10,
	})

	f.Exec(t, `UPDATE issue SET status = 'reviewing' WHERE id = $1`, issue)
	if _, err := f.q.LatestDoneTransition(ctx, util.MustParseUUID(issue)); err == nil {
		t.Fatal("a started custom status is not done: want no rows, got one")
	}
	f.Exec(t, `UPDATE issue SET status = 'shipped' WHERE id = $1`, issue)

	row, err := f.q.LatestDoneTransition(ctx, util.MustParseUUID(issue))
	if err != nil {
		t.Fatalf("LatestDoneTransition: %v", err)
	}
	if row.ToStatusKey != "shipped" {
		t.Fatalf("latest done = %s, want shipped", row.ToStatusKey)
	}
}

// TestLifecycleCategoryOf: a catalog row decides; without one, built-in keys map
// as migration 469 does, and any other key is unstarted.
func TestLifecycleCategoryOf(t *testing.T) {
	cases := []struct {
		name    string
		catalog pgtype.Text
		key     string
		want    string
	}{
		{"catalog row wins", pgtype.Text{String: "closed", Valid: true}, "done", "closed"},
		{"built-in todo without row", pgtype.Text{}, "todo", "unstarted"},
		{"built-in in_review without row", pgtype.Text{}, "in_review", "started"},
		{"built-in done without row", pgtype.Text{}, "done", "done"},
		{"built-in cancelled without row", pgtype.Text{}, "cancelled", "closed"},
		{"unknown key without row", pgtype.Text{}, "mystery", "unstarted"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := LifecycleCategoryOf(c.catalog, c.key); got != c.want {
				t.Fatalf("LifecycleCategoryOf(%v, %q) = %s, want %s", c.catalog, c.key, got, c.want)
			}
		})
	}
}
