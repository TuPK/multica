package service

import (
	"context"
	"errors"
	"testing"

	"github.com/multica-ai/multica/server/internal/util"
)

// A link that fails leaves no pair behind, cancels the shadow task it queued, and a later claim
// of the same primary records the pair with its link. The primary's own claim is not undone.
func TestShadowLinkFailureLeavesNoPairAndCancelsTheShadowTask(t *testing.T) {
	sf := newShadowFixture(t, true)
	sf.start(t)
	task := sf.primaryTask(t)
	ctx := context.Background()
	linkErr := errors.New("link step timed out")
	sf.svc.beforeLink = func() error { return linkErr }

	if err := sf.svc.OnPrimaryClaimed(ctx, task); !errors.Is(err, linkErr) {
		t.Fatalf("OnPrimaryClaimed = %v, want the link error", err)
	}
	if n := sf.Count(t, `SELECT COUNT(*) FROM shadow_pair WHERE workspace_id = $1 AND shadow_task_id IS NULL`, sf.WorkspaceID); n != 0 {
		t.Errorf("pairs with no shadow task = %d, want 0", n)
	}
	if n := sf.Count(t, `SELECT COUNT(*) FROM shadow_pair WHERE primary_task_id = $1`, util.UUIDToString(task.ID)); n != 0 {
		t.Errorf("pairs for the primary = %d, want 0 after the link failed", n)
	}
	if n := sf.Count(t, `
		SELECT COUNT(*) FROM agent_task_queue
		WHERE issue_id = $1 AND status NOT IN ('cancelled', 'completed')
		  AND agent_id IN (SELECT id FROM agent WHERE shadow_of_agent_id = $2)`,
		util.UUIDToString(task.IssueID), sf.seat); n != 0 {
		t.Errorf("live shadow tasks after the link failed = %d, want 0", n)
	}
	if n := sf.Count(t, `SELECT COUNT(*) FROM agent_task_queue WHERE id = $1 AND status = 'completed'`, util.UUIDToString(task.ID)); n != 0 {
		t.Errorf("the primary task was completed by the shadow rollback")
	}

	sf.svc.beforeLink = nil
	if err := sf.svc.OnPrimaryClaimed(ctx, task); err != nil {
		t.Fatalf("OnPrimaryClaimed after the link recovers: %v", err)
	}
	if n := sf.Count(t, `SELECT COUNT(*) FROM shadow_pair WHERE primary_task_id = $1 AND shadow_task_id IS NOT NULL`, util.UUIDToString(task.ID)); n != 1 {
		t.Errorf("linked pairs for the primary = %d, want 1", n)
	}
}
