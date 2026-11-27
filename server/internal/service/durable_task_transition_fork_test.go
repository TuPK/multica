package service

import (
	"context"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
)

func TestQoderTaskStatusTransitionRecordsActorAndTask(t *testing.T) {
	f, _ := companyWorkspace(t)
	runtime := f.Runtime(t, "qoder transition", testutil.Cols{"provider": "qoder_cloud"})
	agent := f.Agent(t, "qoder transition", runtime)
	issue := f.Issue(t, "qoder transition", testutil.Cols{"status": "todo", "assignee_type": "agent", "assignee_id": agent})
	f.Cleanup(t, `DELETE FROM issue_status_transition WHERE issue_id = $1`, issue)
	task := f.Task(t, agent, testutil.Cols{"runtime_id": runtime, "issue_id": issue, "status": "dispatched"})
	if _, err := f.svc.TaskSvc.StartTask(context.Background(), util.MustParseUUID(task)); err != nil {
		t.Fatalf("start task: %v", err)
	}
	var kind string
	var actor, gotTask *string
	f.QueryRow(t, `SELECT actor_type, actor_id::text, task_id::text FROM issue_status_transition WHERE issue_id = $1`, issue).Scan(&kind, &actor, &gotTask)
	if kind != "agent" || actor == nil || *actor != agent || gotTask == nil || *gotTask != task {
		t.Fatalf("actor = (%s, %v), task = %v, want agent %s, task %s", kind, actor, gotTask, agent, task)
	}
}
