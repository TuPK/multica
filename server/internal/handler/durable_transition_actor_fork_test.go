package handler

import (
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

func TestStatusTransitionRecordsMemberRESTActor(t *testing.T) {
	seedTestCatalog(t)
	issue := dbfx.Issue(t, "transition actor", testutil.Cols{"status": "todo"})
	dbfx.Cleanup(t, `DELETE FROM issue_status_transition WHERE issue_id = $1`, issue)
	updateIssueForTest(t, issue, map[string]any{"status": "in_progress"}).Want(200)
	var kind string
	var actor *string
	dbfx.QueryRow(t, `SELECT actor_type, actor_id::text FROM issue_status_transition WHERE issue_id = $1`, issue).Scan(&kind, &actor)
	if kind != "member" || actor == nil || *actor != testUserID {
		t.Fatalf("actor = (%s, %v), want member %s", kind, actor, testUserID)
	}
}

func TestStatusTransitionRecordsTaskRESTActor(t *testing.T) {
	seedTestCatalog(t)
	agent := seededReadyAgentID(t)
	issue := dbfx.Issue(t, "task transition actor", testutil.Cols{"status": "todo", "assignee_type": "agent", "assignee_id": agent})
	dbfx.Cleanup(t, `DELETE FROM issue_status_transition WHERE issue_id = $1`, issue)
	task := goldenRun(t, issue, agent, "")
	agentUpdateIssue(t, issue, agent, task, map[string]any{"status": "in_progress"}).Want(200)
	var kind string
	var actor, gotTask *string
	dbfx.QueryRow(t, `SELECT actor_type, actor_id::text, task_id::text FROM issue_status_transition WHERE issue_id = $1`, issue).Scan(&kind, &actor, &gotTask)
	if kind != "agent" || actor == nil || *actor != agent || gotTask == nil || *gotTask != task {
		t.Fatalf("actor = (%s, %v), task = %v, want agent %s, task %s", kind, actor, gotTask, agent, task)
	}
}
