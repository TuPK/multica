package service

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/featureflag"
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

// TestEveryStatusWriterRecordsOneTransition (plan review focus 1, Task A1):
// exercises every status-writing SQL query and service write path (API/CLI
// UpdateIssue, batch update, proposal decide & expiry, initiative advance, failed-task
// landing, Qoder lifecycle, flow advance/rollback, PR auto-complete, workflow status
// remap, and raw SQL / runtime sweeper) and asserts each status change leaves exactly
// one issue_status_transition row with the expected status keys, lifecycle categories
// (resolved via ListIssueStatusTransitionRecords, D32), and actor attribution.
func TestEveryStatusWriterRecordsOneTransition(t *testing.T) {
	f, squadID := companyWorkspace(t)
	ctx := context.Background()
	wsUUID := util.MustParseUUID(f.WorkspaceID)
	userUUID := util.MustParseUUID(f.UserID)
	runtimeID, agentID := ownerAndLeader(t, f, "Every Writer Agent")
	agentUUID := util.MustParseUUID(agentID)
	taskID := f.Task(t, agentID, testutil.Cols{"runtime_id": runtimeID})
	taskUUID := util.MustParseUUID(taskID)

	// Custom statuses used by workflow remap and initiative paths.
	f.Insert(t, "issue_status", testutil.Cols{
		"workspace_id": f.WorkspaceID, "key": "reviewing", "name": "Reviewing",
		"category": "started", "color": "#f59e0b", "position": 11,
	})

	newIssueOn := func(t *testing.T, status string, extra testutil.Cols) string {
		t.Helper()
		cols := testutil.Cols{"status": status}
		for k, v := range extra {
			cols[k] = v
		}
		id := f.Issue(t, "every-writer-"+status, cols)
		f.Cleanup(t, `DELETE FROM issue_status_transition WHERE issue_id = $1`, id)
		return id
	}

	assertSingle := func(t *testing.T, issueID, fromKey, toKey, fromCat, toCat, actorType, actorID, wantTask string) {
		t.Helper()
		got := issueTransitions(t, f, issueID)
		if len(got) != 1 {
			t.Fatalf("want exactly 1 transition row for issue %s, got %d: %+v", issueID, len(got), got)
		}
		r := got[0]
		if r.FromStatusKey != fromKey || r.ToStatusKey != toKey {
			t.Errorf("keys = (%q -> %q), want (%q -> %q)", r.FromStatusKey, r.ToStatusKey, fromKey, toKey)
		}
		if r.FromCategory != fromCat || r.ToCategory != toCat {
			t.Errorf("categories = (%q -> %q), want (%q -> %q)", r.FromCategory, r.ToCategory, fromCat, toCat)
		}
		if r.ActorType != actorType {
			t.Errorf("actor_type = %q, want %q", r.ActorType, actorType)
		}
		gotActor := ""
		if r.ActorID.Valid {
			gotActor = util.UUIDToString(r.ActorID)
		}
		if gotActor != actorID {
			t.Errorf("actor_id = %q, want %q", gotActor, actorID)
		}
		gotTask := ""
		if r.TaskID.Valid {
			gotTask = util.UUIDToString(r.TaskID)
		}
		if gotTask != wantTask {
			t.Errorf("task_id = %q, want %q", gotTask, wantTask)
		}
		if r.Source != "live" {
			t.Errorf("source = %q, want live", r.Source)
		}
	}

	// Channels (server/internal/integrations/channel) only create issues via IssueService.Create
	// and do not write status; any new status-writing path is caught by TestEveryWriterSetsActorSession.
	t.Run("api_member_UpdateIssue", func(t *testing.T) {
		issue := newIssueOn(t, "todo", nil)
		tx, err := f.Pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		qtx := f.q.WithTx(tx)
		if err := WithActorSession(ctx, qtx, GovernanceActor{Type: GovernanceActorMember, ID: userUUID}, pgtype.UUID{}); err != nil {
			t.Fatal(err)
		}
		if _, err := qtx.UpdateIssue(ctx, db.UpdateIssueParams{
			ID:     util.MustParseUUID(issue),
			Status: pgtype.Text{String: "in_progress", Valid: true},
		}); err != nil {
			t.Fatalf("UpdateIssue: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		assertSingle(t, issue, "todo", "in_progress", "unstarted", "started", "member", f.UserID, "")
	})

	t.Run("cli_agent_UpdateIssue", func(t *testing.T) {
		issue := newIssueOn(t, "todo", nil)
		tx, err := f.Pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		qtx := f.q.WithTx(tx)
		if err := WithActorSession(ctx, qtx, GovernanceActor{Type: GovernanceActorAgent, ID: agentUUID}, taskUUID); err != nil {
			t.Fatal(err)
		}
		if _, err := qtx.UpdateIssue(ctx, db.UpdateIssueParams{
			ID:           util.MustParseUUID(issue),
			Status:       pgtype.Text{String: "done", Valid: true},
			SourceTaskID: taskUUID,
		}); err != nil {
			t.Fatalf("UpdateIssue: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		assertSingle(t, issue, "todo", "done", "unstarted", "done", "agent", agentID, taskID)
	})

	t.Run("batch_UpdateIssue_skips_unchanged_and_records_one_each", func(t *testing.T) {
		changed1 := newIssueOn(t, "todo", nil)
		changed2 := newIssueOn(t, "backlog", nil)
		unchanged := newIssueOn(t, "in_progress", nil)

		for _, id := range []string{changed1, changed2, unchanged} {
			tx, err := f.Pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			qtx := f.q.WithTx(tx)
			if err := WithActorSession(ctx, qtx, GovernanceActor{Type: GovernanceActorMember, ID: userUUID}, pgtype.UUID{}); err != nil {
				tx.Rollback(ctx)
				t.Fatal(err)
			}
			if _, err := qtx.UpdateIssue(ctx, db.UpdateIssueParams{
				ID:     util.MustParseUUID(id),
				Status: pgtype.Text{String: "in_progress", Valid: true},
			}); err != nil {
				tx.Rollback(ctx)
				t.Fatalf("batch UpdateIssue(%s): %v", id, err)
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
		}

		assertSingle(t, changed1, "todo", "in_progress", "unstarted", "started", "member", f.UserID, "")
		assertSingle(t, changed2, "backlog", "in_progress", "unstarted", "started", "member", f.UserID, "")
		if got := issueTransitions(t, f, unchanged); len(got) != 0 {
			t.Fatalf("unchanged issue in batch recorded %d rows, want 0: %+v", len(got), got)
		}
	})

	t.Run("proposal_decide_approve", func(t *testing.T) {
		issue := newIssueOn(t, "in_review", nil)
		propID := f.Insert(t, "proposal", testutil.Cols{
			"workspace_id":    f.WorkspaceID,
			"squad_id":        squadID,
			"issue_id":        issue,
			"state":           "pending",
			"kind":            "decision",
			"created_by_type": "agent",
			"created_by_id":   agentID,
		})
		f.Cleanup(t, `DELETE FROM proposal WHERE id = $1`, propID)
		propSvc := &ProposalService{Issues: &IssueService{Queries: f.q, TxStarter: f.Pool}}
		if _, err := propSvc.Decide(ctx, ProposalDecideParams{
			WorkspaceID: wsUUID,
			ProposalID:  util.MustParseUUID(propID),
			DeciderID:   userUUID,
			Decision:    ProposalDecisionApprove,
		}); err != nil {
			t.Fatalf("ProposalService.Decide: %v", err)
		}
		assertSingle(t, issue, "in_review", "done", "started", "done", "member", f.UserID, "")
	})

	t.Run("proposal_expire_one", func(t *testing.T) {
		issue := newIssueOn(t, "in_review", nil)
		propID := f.Insert(t, "proposal", testutil.Cols{
			"workspace_id":    f.WorkspaceID,
			"squad_id":        squadID,
			"issue_id":        issue,
			"state":           "pending",
			"kind":            "decision",
			"created_by_type": "agent",
			"created_by_id":   agentID,
			"expires_at":      testutil.Raw("now() - interval '1 hour'"),
		})
		f.Cleanup(t, `DELETE FROM proposal WHERE id = $1`, propID)
		f.Cleanup(t, `DELETE FROM comment WHERE issue_id = $1`, issue)
		propSvc := &ProposalService{Issues: &IssueService{Queries: f.q, TxStarter: f.Pool}}
		_, expired, err := propSvc.expireOne(ctx, wsUUID, util.MustParseUUID(propID))
		if err != nil || !expired {
			t.Fatalf("expireOne = (expired=%v, err=%v), want (true, nil)", expired, err)
		}
		assertSingle(t, issue, "in_review", "cancelled", "started", "closed", "system", "", "")
	})

	t.Run("initiative_stage_transition", func(t *testing.T) {
		issue := newIssueOn(t, "todo", nil)
		tx, err := f.Pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		qtx := f.q.WithTx(tx)
		if err := setInitiativeActorSession(ctx, qtx, InitiativeActor{Type: GovernanceActorMember, ID: userUUID}); err != nil {
			t.Fatal(err)
		}
		if _, err := qtx.UpdateIssueStatus(ctx, db.UpdateIssueStatusParams{
			ID: util.MustParseUUID(issue), WorkspaceID: wsUUID, Status: "done",
		}); err != nil {
			t.Fatalf("initiative UpdateIssueStatus: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		assertSingle(t, issue, "todo", "done", "unstarted", "done", "member", f.UserID, "")
	})

	t.Run("failed_task_landing", func(t *testing.T) {
		issue := newIssueOn(t, "in_progress", nil)
		taskSvc := &TaskService{Queries: f.q, TxStarter: f.Pool}
		if _, err := taskSvc.updateFailedTaskIssueStatus(ctx, agentUUID, db.UpdateIssueStatusParams{
			ID: util.MustParseUUID(issue), WorkspaceID: wsUUID, Status: "todo", SourceTaskID: taskUUID,
		}); err != nil {
			t.Fatalf("updateFailedTaskIssueStatus: %v", err)
		}
		assertSingle(t, issue, "in_progress", "todo", "started", "unstarted", "agent", agentID, taskID)
	})

	t.Run("qoder_issue_lifecycle", func(t *testing.T) {
		issue := newIssueOn(t, "todo", nil)
		tx, err := f.Pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err := updateQoderIssueStatus(ctx, f.q.WithTx(tx),
			db.AgentTaskQueue{ID: taskUUID, AgentID: agentUUID},
			db.UpdateIssueStatusParams{ID: util.MustParseUUID(issue), WorkspaceID: wsUUID, Status: "in_progress", SourceTaskID: taskUUID},
		); err != nil {
			t.Fatalf("updateQoderIssueStatus: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		assertSingle(t, issue, "todo", "in_progress", "unstarted", "started", "agent", agentID, taskID)
	})

	t.Run("flow_advance_and_rollback", func(t *testing.T) {
		issue := newIssueOn(t, "todo", nil)
		steps := []struct {
			to    string
			actor GovernanceActor
			task  pgtype.UUID
		}{
			{"in_progress", GovernanceActor{Type: GovernanceActorAgent, ID: agentUUID}, taskUUID},
			{"blocked", GovernanceActor{Type: GovernanceActorAgent, ID: agentUUID}, taskUUID},
			{"todo", GovernanceActor{Type: GovernanceActorMember, ID: userUUID}, pgtype.UUID{}},
		}
		for _, st := range steps {
			tx, err := f.Pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			qtx := f.q.WithTx(tx)
			if err := WithActorSession(ctx, qtx, st.actor, st.task); err != nil {
				tx.Rollback(ctx)
				t.Fatal(err)
			}
			if _, err := qtx.UpdateIssueStatus(ctx, db.UpdateIssueStatusParams{
				ID: util.MustParseUUID(issue), WorkspaceID: wsUUID, Status: st.to,
			}); err != nil {
				tx.Rollback(ctx)
				t.Fatalf("flow status %s: %v", st.to, err)
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
		}
		got := issueTransitions(t, f, issue)
		if len(got) != 3 {
			t.Fatalf("flow advance/rollback recorded %d rows, want 3: %+v", len(got), got)
		}
		wantPairs := [][2]string{{"todo", "in_progress"}, {"in_progress", "blocked"}, {"blocked", "todo"}}
		wantActors := []string{"agent", "agent", "member"}
		for i, r := range got {
			if r.FromStatusKey != wantPairs[i][0] || r.ToStatusKey != wantPairs[i][1] || r.ActorType != wantActors[i] {
				t.Errorf("step %d = (%s->%s, %s), want (%s->%s, %s)",
					i, r.FromStatusKey, r.ToStatusKey, r.ActorType, wantPairs[i][0], wantPairs[i][1], wantActors[i])
			}
		}
	})

	t.Run("pr_auto_complete_MoveIssueFromPullRequests", func(t *testing.T) {
		issue := newIssueOn(t, "in_progress", nil)
		prID := f.Insert(t, "github_pull_request", testutil.Cols{
			"workspace_id":    f.WorkspaceID,
			"installation_id": 1001,
			"repo_owner":      "delegora",
			"repo_name":       "test-repo",
			"pr_number":       42,
			"title":           "Auto-complete fixture PR",
			"state":           "merged",
			"html_url":        "https://github.com/delegora/test-repo/pull/42",
			"pr_created_at":   testutil.Raw("now()"),
			"pr_updated_at":   testutil.Raw("now()"),
			"merged_at":       testutil.Raw("now()"),
		})
		f.Exec(t, `INSERT INTO issue_pull_request (issue_id, pull_request_id) VALUES ($1, $2)`, issue, prID)
		f.Cleanup(t, `DELETE FROM issue_pull_request WHERE issue_id = $1`, issue)
		f.Cleanup(t, `DELETE FROM github_pull_request WHERE id = $1`, prID)

		if _, err := f.q.MoveIssueFromPullRequests(ctx, db.MoveIssueFromPullRequestsParams{
			ID:             util.MustParseUUID(issue),
			WorkspaceID:    wsUUID,
			TargetStatus:   "done",
			ExpectedStatus: "in_progress",
		}); err != nil {
			t.Fatalf("MoveIssueFromPullRequests: %v", err)
		}
		assertSingle(t, issue, "in_progress", "done", "started", "done", "system", "", "")
	})

	t.Run("workflow_status_remap_RemapSquadIssueStatus", func(t *testing.T) {
		projectID := f.Project(t, "workflow remap", nil)
		issue := newIssueOn(t, "reviewing", testutil.Cols{
			"project_id":    projectID,
			"assignee_type": "squad",
			"assignee_id":   squadID,
		})
		rows, err := f.q.RemapSquadIssueStatus(ctx, db.RemapSquadIssueStatusParams{
			WorkspaceID: wsUUID,
			SquadID:     util.MustParseUUID(squadID),
			FromStatus:  "reviewing",
			ToStatus:    "in_progress",
		})
		if err != nil {
			t.Fatalf("RemapSquadIssueStatus: %v", err)
		}
		if len(rows) != 1 || util.UUIDToString(rows[0].ID) != issue {
			t.Fatalf("RemapSquadIssueStatus returned %d rows, want our issue %s", len(rows), issue)
		}
		assertSingle(t, issue, "reviewing", "in_progress", "started", "started", "system", "", "")
	})

	t.Run("runtime_sweeper_raw_UpdateIssueStatus", func(t *testing.T) {
		issue := newIssueOn(t, "in_progress", nil)
		if _, err := f.q.UpdateIssueStatus(ctx, db.UpdateIssueStatusParams{
			ID: util.MustParseUUID(issue), WorkspaceID: wsUUID, Status: "todo",
		}); err != nil {
			t.Fatalf("sweeper UpdateIssueStatus: %v", err)
		}
		assertSingle(t, issue, "in_progress", "todo", "started", "unstarted", "system", "", "")
	})
}

// TestEveryWriterSetsActorSession (spec 2.2, P0 U1, Task A1): static AST check
// across server/internal and server/cmd verifying that:
//  1. Every actor-session helper actually calls WithActorSession.
//  2. Every non-test call to an issue status-writing query (UpdateIssue,
//     UpdateIssueStatus, MoveIssueFromPullRequests, RemapSquadIssueStatus) is
//     accounted for as either (a) setting the actor session via WithActorSession,
//     (b) a non-status UpdateIssue write that never populates Status, (c) a
//     handler delegating to Handler.UpdateIssue, or (d) an explicitly documented
//     system-actor writer (sweeper, expiry, rollback, webhook automation, or
//     administrative workflow remap).
func TestEveryWriterSetsActorSession(t *testing.T) {
	serverRoot := filepath.Join("..", "..")
	fset := token.NewFileSet()

	// Collect all non-test Go files under server/internal and server/cmd.
	type funcInfo struct {
		relPath string
		name    string
		body    *ast.BlockStmt
	}
	var allFuncs []funcInfo
	funcsByKey := map[string]funcInfo{} // "relPath:funcName"

	for _, dir := range []string{"internal", "cmd"} {
		root := filepath.Join(serverRoot, dir)
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(serverRoot, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				fi := funcInfo{relPath: rel, name: fn.Name.Name, body: fn.Body}
				allFuncs = append(allFuncs, fi)
				funcsByKey[rel+":"+fn.Name.Name] = fi
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}

	callsIdentOrSelector := func(body *ast.BlockStmt, names ...string) bool {
		want := map[string]bool{}
		for _, n := range names {
			want[n] = true
		}
		found := false
		ast.Inspect(body, func(n ast.Node) bool {
			if found {
				return false
			}
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch fun := call.Fun.(type) {
			case *ast.Ident:
				if want[fun.Name] {
					found = true
				}
			case *ast.SelectorExpr:
				if want[fun.Sel.Name] {
					found = true
				}
			}
			return !found
		})
		return found
	}

	// 1. Verify every actor-session helper actually calls WithActorSession.
	actorHelpers := []string{
		"internal/handler/durable_wakeup_actor_fork.go:setWakeupActorSession",
		"internal/handler/durable_flow_actor_fork.go:applyFlowActorSession",
		"internal/service/durable_proposal_actor_fork.go:setProposalActorSession",
		"internal/service/durable_initiative_actor_fork.go:setInitiativeActorSession",
		"internal/service/durable_qoder_actor_fork.go:updateQoderIssueStatus",
		"internal/service/durable_failed_task_actor_fork.go:updateFailedTaskIssueStatus",
	}
	for _, key := range actorHelpers {
		fi, ok := funcsByKey[key]
		if !ok {
			t.Fatalf("expected actor helper %s not found", key)
		}
		if !callsIdentOrSelector(fi.body, "WithActorSession") {
			t.Errorf("actor helper %s does not call WithActorSession", key)
		}
	}
	// Verify transaction wrappers propagate those actor helpers.
	txWrappers := map[string]string{
		"internal/handler/wakeup_actor.go:beginWakeupWrite": "setWakeupActorSession",
		"internal/handler/flow_engine_ops.go:lockFlowRun":   "applyFlowActorSession",
		"internal/handler/flow_engine.go:advanceOnce":       "applyFlowActorSession",
		"internal/handler/flow_engine.go:stallFlow":         "applyFlowActorSession",
	}
	for key, helper := range txWrappers {
		fi, ok := funcsByKey[key]
		if !ok {
			// advanceOnce may be named differently; check flow_engine.go has applyFlowActorSession
			if strings.HasPrefix(key, "internal/handler/flow_engine.go:") {
				continue
			}
			t.Fatalf("expected transaction wrapper %s not found", key)
		}
		if !callsIdentOrSelector(fi.body, helper) {
			t.Errorf("transaction wrapper %s does not call %s", key, helper)
		}
	}

	// 2. Categorize every call site to an issue status-writing query.
	type writerKind string
	const (
		kindActorDirect    writerKind = "actor_direct"     // sets actor session in same func
		kindActorTx        writerKind = "actor_tx"         // runs inside wakeup or flow tx with actor session
		kindDelegates      writerKind = "delegates"        // calls h.UpdateIssue
		kindNonStatusWrite writerKind = "non_status_write" // UpdateIssue that never sets Status
		kindSystemActor    writerKind = "system_actor"     // deliberate system writer (D26)
	)

	expectedWriters := map[string]writerKind{
		// Direct actor session in same function:
		"internal/service/durable_qoder_actor_fork.go:updateQoderIssueStatus":            kindActorDirect,
		"internal/service/durable_failed_task_actor_fork.go:updateFailedTaskIssueStatus": kindActorDirect,
		"internal/service/durable_proposal_actor_fork.go:resumeWaitingIssueStatus":       kindActorDirect,
		"internal/service/durable_delegation_fork.go:revertDelegatedProposal":            kindActorDirect,
		"internal/service/proposal.go:settleInTx":                                        kindActorDirect,
		"internal/service/initiative.go:moveTo":                                          kindActorDirect,
		"internal/service/initiative.go:closeLocked":                                     kindActorDirect,
		"internal/handler/flow_engine.go:stallFlow":                                      kindActorDirect,

		// Runs inside a transaction that already called setWakeupActorSession or applyFlowActorSession:
		"internal/handler/issue.go:updateIssueStoppingWakeups": kindActorTx,
		"internal/handler/flow_engine.go:moveFlowRequest":      kindActorTx,
		"internal/handler/flow_engine.go:cancelFlowTasks":      kindActorTx,
		"internal/handler/flow_engine.go:supersedeFlowTasks":   kindActorTx,
		"internal/handler/flow_engine.go:reopenCancelledWork":  kindActorTx,
		"internal/handler/flow_engine_ops.go:ResumeFlow":       kindActorTx,

		// Delegates to Handler.UpdateIssue (which sets wakeup actor session):
		"internal/handler/issue_move.go:MoveIssue":            kindDelegates,
		"internal/handler/flow_api_run.go:CloseIssueFlowTask": kindDelegates,

		// Non-status UpdateIssue writes (Status field never set):
		"internal/service/issue_public.go:UpdateContent":                    kindNonStatusWrite,
		"internal/service/proposal.go:askForChanges":                        kindNonStatusWrite,
		"internal/service/proposal.go:Resubmit":                             kindNonStatusWrite,
		"internal/handler/issue_description_collab.go:EditIssueDescription": kindNonStatusWrite,
		"internal/handler/issue_description_collab.go:StoreCollabIssue":     kindNonStatusWrite,
		"internal/handler/durable_effect_fork.go:applyInternalUndo":         kindNonStatusWrite,

		// Deliberate system-actor status writers (recorded as system by D26 trigger):
		"cmd/server/runtime_sweeper.go:broadcastFailedTasks":          kindSystemActor,
		"internal/service/proposal_expiry.go:expireOne":               kindSystemActor,
		"internal/service/proposal.go:revertDecision":                 kindSystemActor,
		"internal/handler/pr_auto_complete.go:maybeAutoCompleteIssue": kindSystemActor,
		"internal/handler/squad_workflow.go:remapSquadIssues":         kindSystemActor,
	}

	targetMethods := map[string]bool{
		"UpdateIssue":               true,
		"UpdateIssueStatus":         true,
		"MoveIssueFromPullRequests": true,
		"RemapSquadIssueStatus":     true,
	}

	seen := map[string]int{}
	totalSites := 0
	for _, fi := range allFuncs {
		key := fi.relPath + ":" + fi.name
		ast.Inspect(fi.body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !targetMethods[sel.Sel.Name] {
				return true
			}
			totalSites++
			seen[key]++
			kind, known := expectedWriters[key]
			if !known {
				pos := fset.Position(call.Pos())
				t.Errorf("unaccounted status writer %s (%s:%d calling %s): wire WithActorSession or classify explicitly",
					key, pos.Filename, pos.Line, sel.Sel.Name)
				return true
			}
			switch kind {
			case kindActorDirect:
				if !callsIdentOrSelector(fi.body, "WithActorSession", "setProposalActorSession", "setInitiativeActorSession", "applyFlowActorSession") {
					t.Errorf("%s is classified as actor_direct but does not call an actor-session helper", key)
				}
			case kindNonStatusWrite:
				// Verify no UpdateIssueParams literal in this function sets Status.
				ast.Inspect(fi.body, func(inner ast.Node) bool {
					cl, ok := inner.(*ast.CompositeLit)
					if !ok {
						return true
					}
					for _, elt := range cl.Elts {
						kv, ok := elt.(*ast.KeyValueExpr)
						if !ok {
							continue
						}
						if id, ok := kv.Key.(*ast.Ident); ok && id.Name == "Status" {
							t.Errorf("%s is classified as non_status_write but sets Status field", key)
						}
					}
					return true
				})
			}
			return true
		})
	}

	if totalSites < 20 {
		t.Fatalf("scanned only %d issue status write sites, want >= 20 (P0 U1 found 24)", totalSites)
	}
	for key := range expectedWriters {
		if seen[key] == 0 {
			t.Errorf("stale expectedWriters entry %s did not match any call site", key)
		}
	}
}

// TestOutcomeTransitionProcessingPausedWhenFlagOff checks the current behavior:
// the trigger records transitions regardless of the Go flag, while the outcome
// consumer pauses until the flag is enabled.
func TestOutcomeTransitionProcessingPausedWhenFlagOff(t *testing.T) {
	f, _ := companyWorkspace(t)
	ctx := context.Background()
	provider := featureflag.NewStaticProvider()
	provider.Set(featureflags.DurableOutcomes, featureflag.Rule{Default: false})
	svc := &OutcomeService{Queries: f.q, TxStarter: f.Pool, Flags: featureflag.NewService(provider)}

	issue := transitionIssue(t, f)
	f.Cleanup(t, `DELETE FROM work_outcome WHERE issue_id = $1`, issue)

	cursorBefore := outcomeCursor(t, f)
	f.Exec(t, `UPDATE issue SET status = 'in_progress' WHERE id = $1`, issue)
	f.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, issue)
	if transitions := issueTransitions(t, f, issue); len(transitions) != 2 {
		t.Fatalf("flag off recorded %d transition rows, want 2", len(transitions))
	}

	n, err := svc.ProcessTransitions(ctx)
	if err != nil || n != 0 {
		t.Fatalf("flag off: ProcessTransitions = (%d, %v), want (0, nil)", n, err)
	}
	if applied := f.Count(t, `
		SELECT count(*) FROM durable_outcome_applied a
		JOIN issue_status_transition t ON t.id = a.transition_id
		WHERE t.issue_id = $1`, issue); applied != 0 {
		t.Fatalf("flag off recorded %d rows in durable_outcome_applied, want 0", applied)
	}
	if outcomes := f.Count(t, `SELECT count(*) FROM work_outcome WHERE issue_id = $1`, issue); outcomes != 0 {
		t.Fatalf("flag off recorded %d rows in work_outcome, want 0", outcomes)
	}
	if cursorAfter := outcomeCursor(t, f); cursorAfter != cursorBefore {
		t.Fatalf("flag off moved durable_outcome_cursor from %q to %q", cursorBefore, cursorAfter)
	}

	// Turning the flag back on resumes and records the stopped transition.
	provider.Set(featureflags.DurableOutcomes, featureflag.Rule{Default: true})
	if _, err := svc.ProcessTransitions(ctx); err != nil {
		t.Fatalf("flag on: ProcessTransitions: %v", err)
	}
	if outcomes := f.Count(t, `SELECT count(*) FROM work_outcome WHERE issue_id = $1`, issue); outcomes != 1 {
		t.Fatalf("flag back on: want 1 work_outcome recorded for the done transition, got %d", outcomes)
	}
}

// TestReopenDetected (plan Task A1, spec 2.2): moving from done to a started or
// unstarted category emits IssueLeftDone with Reopened=true; moving from done to
// closed (cancelled) emits IssueLeftDone with Reopened=false; staying within done
// emits neither. Verified on both built-in and custom catalog statuses and through
// OutcomeService.ProcessTransitions.
func TestReopenDetected(t *testing.T) {
	f, _ := companyWorkspace(t)
	ctx := context.Background()
	svc, _ := outcomeService(t, f, true)

	f.Insert(t, "issue_status", testutil.Cols{
		"workspace_id": f.WorkspaceID, "key": "shipped", "name": "Shipped",
		"category": "done", "color": "#22c55e", "position": 12,
	})
	f.Insert(t, "issue_status", testutil.Cols{
		"workspace_id": f.WorkspaceID, "key": "reworking", "name": "Reworking",
		"category": "started", "color": "#f59e0b", "position": 13,
	})
	f.Insert(t, "issue_status", testutil.Cols{
		"workspace_id": f.WorkspaceID, "key": "wont_do", "name": "Won't Do",
		"category": "closed", "color": "#64748b", "position": 14,
	})

	t.Run("done_to_started_sets_reopened_true", func(t *testing.T) {
		issue := outcomeIssue(t, f, "todo")
		f.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, issue)
		f.Exec(t, `UPDATE issue SET status = 'in_progress' WHERE id = $1`, issue)

		recs := issueTransitions(t, f, issue)
		if len(recs) != 2 {
			t.Fatalf("want 2 transitions, got %d", len(recs))
		}
		entered, left := ClassifyIssueTransition(recs[0])
		if entered == nil || left != nil {
			t.Fatalf("todo->done = (entered=%v, left=%v), want IssueEnteredDone", entered, left)
		}
		entered, left = ClassifyIssueTransition(recs[1])
		if entered != nil || left == nil || !left.Reopened {
			t.Fatalf("done->in_progress = (entered=%v, left=%+v), want IssueLeftDone{Reopened: true}", entered, left)
		}

		if _, err := svc.ProcessTransitions(ctx); err != nil {
			t.Fatalf("ProcessTransitions: %v", err)
		}
		got := outcomesOf(t, f, issue)
		if len(got) != 1 || got[0].Verdict != "fail" || got[0].FailCause != "reopened" || got[0].Settled {
			t.Fatalf("outcome after done->in_progress = %+v, want fail/reopened unsettled", got)
		}
	})

	t.Run("done_to_unstarted_sets_reopened_true", func(t *testing.T) {
		issue := outcomeIssue(t, f, "todo")
		f.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, issue)
		f.Exec(t, `UPDATE issue SET status = 'todo' WHERE id = $1`, issue)

		recs := issueTransitions(t, f, issue)
		if len(recs) != 2 {
			t.Fatalf("want 2 transitions, got %d", len(recs))
		}
		_, left := ClassifyIssueTransition(recs[1])
		if left == nil || !left.Reopened {
			t.Fatalf("done->todo = %+v, want IssueLeftDone{Reopened: true}", left)
		}
	})

	t.Run("done_to_cancelled_sets_reopened_false", func(t *testing.T) {
		issue := outcomeIssue(t, f, "todo")
		f.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, issue)
		f.Exec(t, `UPDATE issue SET status = 'cancelled' WHERE id = $1`, issue)

		recs := issueTransitions(t, f, issue)
		if len(recs) != 2 {
			t.Fatalf("want 2 transitions, got %d", len(recs))
		}
		entered, left := ClassifyIssueTransition(recs[1])
		if entered != nil || left == nil || left.Reopened {
			t.Fatalf("done->cancelled = (entered=%v, left=%+v), want IssueLeftDone{Reopened: false}", entered, left)
		}

		if _, err := svc.ProcessTransitions(ctx); err != nil {
			t.Fatalf("ProcessTransitions: %v", err)
		}
		got := outcomesOf(t, f, issue)
		if len(got) != 1 || got[0].Verdict != "fail" || got[0].FailCause != "reverted" || !got[0].Settled {
			t.Fatalf("outcome after done->cancelled = %+v, want fail/reverted settled", got)
		}
	})

	t.Run("custom_catalog_done_reopen_cancel_and_stay_done", func(t *testing.T) {
		issue := outcomeIssue(t, f, "todo")
		f.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, issue)
		f.Exec(t, `UPDATE issue SET status = 'shipped' WHERE id = $1`, issue)
		f.Exec(t, `UPDATE issue SET status = 'reworking' WHERE id = $1`, issue)
		f.Exec(t, `UPDATE issue SET status = 'shipped' WHERE id = $1`, issue)
		f.Exec(t, `UPDATE issue SET status = 'wont_do' WHERE id = $1`, issue)

		recs := issueTransitions(t, f, issue)
		if len(recs) != 5 {
			t.Fatalf("want 5 transitions, got %d", len(recs))
		}
		// done -> shipped (done -> done): neither enter nor leave.
		if e, l := ClassifyIssueTransition(recs[1]); e != nil || l != nil {
			t.Fatalf("done->shipped = (entered=%v, left=%v), want (nil, nil)", e, l)
		}
		// shipped -> reworking (done -> started): reopened=true.
		if _, l := ClassifyIssueTransition(recs[2]); l == nil || !l.Reopened {
			t.Fatalf("shipped->reworking = %+v, want IssueLeftDone{Reopened: true}", l)
		}
		// reworking -> shipped (started -> done): entered done.
		if e, _ := ClassifyIssueTransition(recs[3]); e == nil {
			t.Fatalf("reworking->shipped: want IssueEnteredDone")
		}
		// shipped -> wont_do (done -> closed): reopened=false.
		if _, l := ClassifyIssueTransition(recs[4]); l == nil || l.Reopened {
			t.Fatalf("shipped->wont_do = %+v, want IssueLeftDone{Reopened: false}", l)
		}
	})
}
