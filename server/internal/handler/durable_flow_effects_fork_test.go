package handler

import (
	"context"
	"testing"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/service"
)

func TestFlowExternalTaskWaitsForApproval(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		name := "flag_off"
		if enabled {
			name = "flag_on"
		}
		t.Run(name, func(t *testing.T) {
			withFeatureFlag(t, testHandler, featureflags.DurableEffects, enabled)
			old := testHandler.durable.Intents
			testHandler.durable.Intents = &service.IntentService{Queries: testHandler.Queries, TxStarter: testPool, Flags: testHandler.FeatureFlags, Tasks: testHandler.TaskService}
			t.Cleanup(func() { testHandler.durable.Intents = old })
			rig := newGraphRig(t, "trade", graphRigOpts{Mutate: func(doc map[string]any) {
				for _, step := range asMaps(doc["steps"]) {
					if step["key"] == "collect" {
						for _, task := range asMaps(step["tasks"]) {
							if task["key"] == "docs" {
								task["effects"] = "external"
							}
						}
					}
				}
			}})
			request := rig.newRequest()
			t.Cleanup(func() {
				testPool.Exec(context.Background(), `DELETE FROM effect_intent WHERE workspace_id=$1 AND target LIKE 'flow/collect.docs#%' AND issue_id IN (SELECT id FROM issue WHERE parent_issue_id=$2)`, testWorkspaceID, request.ID)
			})
			rig.complete(request, nil)
			issue := rig.rows(request, "collect", 0)["docs"]
			if enabled {
				// The task is held before the engine creates it: no issue until its intent is approved.
				if issue != "" {
					t.Fatalf("before approval: external task %s was created", issue)
				}
			} else if issue == "" {
				t.Fatal("flag off: external flow task was not created")
			}
			if !enabled {
				if n := queuedRuns(t, issue); n == 0 {
					t.Error("flag off: expected runnable task")
				}
				if n := dbfx.Count(t, `SELECT count(*) FROM effect_intent WHERE workspace_id=$1 AND issue_id=$2 AND target LIKE 'flow/collect.docs#%'`, testWorkspaceID, issue); n != 0 {
					t.Errorf("flag off: got %d intents, want 0", n)
				}
				return
			}
			var intent string
			err := testPool.QueryRow(context.Background(), `SELECT id::text FROM effect_intent WHERE workspace_id=$1 AND target LIKE 'flow/collect.docs#%' AND state='pending' AND issue_id=$2`, testWorkspaceID, request.ID).Scan(&intent)
			if err != nil {
				t.Fatalf("pending approval intent: %v", err)
			}
			if _, err := testHandler.durable.Intents.Approve(context.Background(), parseUUID(testWorkspaceID), parseUUID(testUserID), parseUUID(intent), nil, "approved"); err != nil {
				t.Fatalf("approve: %v", err)
			}
			// The request's next advance opens the approved task (spec 5.4; approval runs nothing itself).
			current, err := testHandler.Queries.GetIssue(context.Background(), request.ID)
			if err != nil {
				t.Fatalf("load request: %v", err)
			}
			if _, err := testHandler.AdvanceFlow(context.Background(), current); err != nil {
				t.Fatalf("advance: %v", err)
			}
			issue = rig.rows(request, "collect", 0)["docs"]
			if issue == "" {
				t.Fatal("after approval: the next advance did not open the task")
			}
			if n := queuedRuns(t, issue); n == 0 {
				t.Error("after approval: task is not runnable")
			}
		})
	}
}
