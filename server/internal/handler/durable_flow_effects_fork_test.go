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
			old := testHandler.Intents
			testHandler.Intents = &service.IntentService{Queries: testHandler.Queries, TxStarter: testPool, Flags: testHandler.FeatureFlags, Tasks: testHandler.TaskService}
			t.Cleanup(func() { testHandler.Intents = old })
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
				testPool.Exec(context.Background(), `DELETE FROM effect_intent WHERE workspace_id=$1 AND target='flow/collect.docs' AND issue_id IN (SELECT id FROM issue WHERE parent_issue_id=$2)`, testWorkspaceID, request.ID)
			})
			rig.complete(request, nil)
			issue := rig.rows(request, "collect", 0)["docs"]
			if issue == "" {
				t.Fatal("external flow task was not created")
			}
			if !enabled {
				if n := queuedRuns(t, issue); n == 0 {
					t.Error("flag off: expected runnable task")
				}
				if n := dbfx.Count(t, `SELECT count(*) FROM effect_intent WHERE workspace_id=$1 AND issue_id=$2 AND target='flow/collect.docs'`, testWorkspaceID, issue); n != 0 {
					t.Errorf("flag off: got %d intents, want 0", n)
				}
				return
			}
			if n := queuedRuns(t, issue); n != 0 {
				t.Errorf("before approval: %d runnable runs, want 0", n)
			}
			var intent string
			err := testPool.QueryRow(context.Background(), `SELECT id::text FROM effect_intent WHERE workspace_id=$1 AND issue_id=$2 AND target='flow/collect.docs' AND state='pending'`, testWorkspaceID, issue).Scan(&intent)
			if err != nil {
				t.Fatalf("pending approval intent: %v", err)
			}
			if _, err := testHandler.Intents.Approve(context.Background(), parseUUID(testWorkspaceID), parseUUID(testUserID), parseUUID(intent), nil, "approved"); err != nil {
				t.Fatalf("approve: %v", err)
			}
			if n := queuedRuns(t, issue); n == 0 {
				t.Error("after approval: task is not runnable")
			}
		})
	}
}
