package service

import (
	"context"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestFailedTaskLandingTransitionAttributesAgentAndTask(t *testing.T) {
	f, _ := companyWorkspace(t)
	ctx := context.Background()
	runtime := f.Runtime(t, "failed transition")
	agent := f.Agent(t, "failed transition", runtime)
	issue := f.Issue(t, "failed transition", testutil.Cols{"status": "in_progress", "assignee_type": "agent", "assignee_id": agent})
	f.Cleanup(t, `DELETE FROM issue_status_transition WHERE issue_id = $1`, issue)
	taskID := f.Task(t, agent, testutil.Cols{"runtime_id": runtime, "issue_id": issue, "status": "failed", "failure_reason": "agent_error", "attempt": 1, "max_attempts": 1})
	task, err := f.q.GetAgentTask(ctx, util.MustParseUUID(taskID))
	if err != nil {
		t.Fatalf("load failed task: %v", err)
	}

	f.svc.TaskSvc.HandleFailedTasks(ctx, []db.AgentTaskQueue{task})

	var kind, actor, gotTask string
	f.QueryRow(t, `SELECT actor_type, COALESCE(actor_id::text, ''), COALESCE(task_id::text, '') FROM issue_status_transition WHERE issue_id = $1 AND to_status_key = 'blocked'`, issue).Scan(&kind, &actor, &gotTask)
	if kind != "agent" || actor != agent || gotTask != taskID {
		t.Fatalf("transition actor = (%q, %q), task = %q; want agent %s and task %s", kind, actor, gotTask, agent, taskID)
	}
}
