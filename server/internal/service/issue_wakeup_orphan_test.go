package service

import (
	"context"
	"testing"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// A workspace delete that bypasses DeleteWorkspace leaves its wakeups behind:
// issue_wakeup has no foreign keys, so the cascade from workspace to issue and
// agent does not reach it. Dispatch must retire such a rule and its receipts.
// Before the fix, the workspace lock returned pgx.ErrNoRows, which the pass
// reported as a failed dispatch on every scheduler tick, forever.
func TestDispatchRetiresWakeupWhoseWorkspaceIsGone(t *testing.T) {
	ctx := context.Background()

	t.Run("system rule", func(t *testing.T) {
		f, s, issue, _ := wakeFixture(t)
		parent, err := f.q.GetIssue(ctx, issue)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.ensureRule(ctx, parent); err != nil {
			t.Fatal(err)
		}
		rule, err := f.q.GetSystemWakeup(ctx, db.GetSystemWakeupParams{IssueID: issue, SystemRule: systemRuleText(SystemRuleChildDone)})
		if err != nil {
			t.Fatal(err)
		}
		f.Exec(t, `INSERT INTO issue_wakeup_receipt(id,wakeup_id,revision,event_key,event_type,payload)
			VALUES (gen_random_uuid(), $1, $2, 'k', 'issue.status_changed', '{}')`, rule.ID, rule.Revision)
		f.Exec(t, "DELETE FROM workspace WHERE id=$1", f.WorkspaceID)

		if err = s.dispatchSystem(ctx, rule); err != nil {
			t.Fatalf("dispatchSystem for a rule whose workspace is gone: %v", err)
		}
		if n := f.Count(t, "SELECT count(*) FROM issue_wakeup WHERE id=$1", rule.ID); n != 0 {
			t.Fatalf("orphaned system rule still exists: %d rows", n)
		}
		if n := f.Count(t, "SELECT count(*) FROM issue_wakeup_receipt WHERE wakeup_id=$1", rule.ID); n != 0 {
			t.Fatalf("orphaned system rule kept %d receipts", n)
		}
	})

	t.Run("agent rule", func(t *testing.T) {
		f, s, issue, agent := wakeFixture(t)
		w := wakeCreate(t, f, s, issue, WakeupInput{AgentID: agent, Kind: "event", Mode: "continuous",
			EventTypes: []string{"comment.created"}, Instruction: "Review it"})
		f.Exec(t, "DELETE FROM workspace WHERE id=$1", f.WorkspaceID)

		if err := s.dispatch(ctx, w); err != nil {
			t.Fatalf("dispatch for a rule whose workspace is gone: %v", err)
		}
		if n := f.Count(t, "SELECT count(*) FROM issue_wakeup WHERE id=$1", w.ID); n != 0 {
			t.Fatalf("orphaned agent rule still exists: %d rows", n)
		}
	})
}
