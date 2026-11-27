package handler

import (
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

func TestUpdateIssueStatusTransitionAttributesRequestingMember(t *testing.T) {
	seedTestCatalog(t)
	issue := dbfx.Issue(t, "member status transition", testutil.Cols{"status": "todo"})
	dbfx.Cleanup(t, `DELETE FROM issue_status_transition WHERE issue_id = $1`, issue)

	updateIssueForTest(t, issue, map[string]any{"status": "in_progress"}).Want(200)

	var kind, actor, task string
	dbfx.QueryRow(t, `SELECT actor_type, COALESCE(actor_id::text, ''), COALESCE(task_id::text, '') FROM issue_status_transition WHERE issue_id = $1`, issue).Scan(&kind, &actor, &task)
	if kind != "member" || actor != testUserID || task != "" {
		t.Fatalf("transition actor = (%q, %q), task = %q; want member %s and no task", kind, actor, task, testUserID)
	}
}
