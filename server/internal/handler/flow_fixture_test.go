package handler

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// A flow request with its workflow and tasks, built straight in the database so
// the guard tests do not depend on the engine that opens them in production.
//
// The workflow is: plan (todo) -> in_progress (work task "draft", needs an
// attachment) -> in_review (gate task "check", needs an outcome) -> done, with a
// rework back to in_progress twice and blocked for the stops. It uses built-in
// statuses only, so a test creates no catalog rows.

func flowGuardWorkflowBody(name, workAgent, gateAgent string) map[string]any {
	return map[string]any{
		"name": name,
		"flow_config": map[string]any{
			"version":  1,
			"switches": []map[string]any{{"key": "export", "type": "bool", "default": false}},
		},
		"steps": []map[string]any{
			{"status_key": "todo", "transitions": []map[string]any{
				{"id": "default", "label": "Default", "when": nil, "to_status_key": "in_progress"},
			}},
			{"status_key": "in_progress",
				"tasks": []map[string]any{{
					"key": "draft", "title": "Draft", "kind": "work", "instructions": "Draft it.",
					"handler":  map[string]any{"type": "agent", "id": workAgent},
					"requires": []string{"attachment"},
				}},
				"transitions": []map[string]any{
					{"id": "blocked", "label": "Blocked", "when": map[string]any{"field": "tasks_outcome", "op": "eq", "value": "any_blocked"}, "to_status_key": "blocked"},
					{"id": "default", "label": "Default", "when": nil, "to_status_key": "in_review"},
				}},
			{"status_key": "in_review",
				"tasks": []map[string]any{{
					"key": "check", "title": "Check", "kind": "gate", "instructions": "Check it.",
					"handler":  map[string]any{"type": "agent", "id": gateAgent},
					"requires": []string{"outcome"},
				}},
				"transitions": []map[string]any{
					{"id": "blocked", "label": "Blocked", "when": map[string]any{"field": "tasks_outcome", "op": "eq", "value": "any_blocked"}, "to_status_key": "blocked"},
					{"id": "pass", "label": "Pass", "when": map[string]any{"field": "tasks_outcome", "op": "eq", "value": "all_approved"}, "to_status_key": "done"},
					{"id": "redo", "label": "Redo", "rework": true, "to_status_key": "in_progress", "when": map[string]any{"all": []map[string]any{
						{"field": "tasks_outcome", "op": "eq", "value": "any_rejected"},
						{"field": "rework_count", "op": "lt", "value": 2},
					}}},
					{"id": "default", "label": "Default", "when": nil, "to_status_key": "blocked"},
				}},
			{"status_key": "blocked"},
		},
	}
}

type flowFixture struct {
	Workflow  IssueWorkflowResponse
	Request   string
	Work      string // the "draft" task issue, in step in_progress
	Gate      string // the "check" task issue, in step in_review
	WorkAgent string
	GateAgent string
}

// cleanupFlowRows removes what a flow test leaves in the shared workspace: the requests and tasks
// the engine made (also the ones a detach turned into plain issues), and the runs of its agents queued
// since the test began. A run left queued on the seeded runtime is claimed by the next test that claims a task
// there, for example the squad briefing test.
func cleanupFlowRows(t *testing.T) {
	t.Helper()
	started := time.Now().Add(-time.Second)
	remove := func() {
		ctx := context.Background()
		// Only the runs of this workspace's agents: the database is shared with real runs.
		testPool.Exec(ctx, `DELETE FROM agent_task_queue WHERE created_at >= $1 AND
			agent_id IN (SELECT id FROM agent WHERE workspace_id = $2)`, started, testWorkspaceID)
		testPool.Exec(ctx, `DELETE FROM issue WHERE workspace_id = $1 AND created_at >= $2 AND
			(flow IS NOT NULL OR flow_task IS NOT NULL OR parent_issue_id IN
				(SELECT id FROM issue WHERE workspace_id = $1 AND created_at >= $2 AND title LIKE 'Trade request %'))`, testWorkspaceID, started)
	}
	t.Cleanup(remove)
}

func newFlowFixture(t *testing.T) flowFixture {
	t.Helper()
	cleanupFlowRows(t)
	seedTestCatalog(t)
	workAgent := seededReadyAgentID(t)
	gateAgent := goldenLeadAgent(t, workAgent)
	wf := createFlowWorkflowForTest(t, flowGuardWorkflowBody("Flow guard "+workflowTestSuffix(), workAgent, gateAgent))
	request := dbfx.Issue(t, "flow request "+workflowTestSuffix(), testutil.Cols{
		"status": "in_progress",
		"flow":   testutil.Raw(fmt.Sprintf(`'{"workflow_id":"%s","step":"in_progress","rework":0}'::jsonb`, wf.ID)),
	})
	task := func(title, step, key, agent string) string {
		return dbfx.Issue(t, title+" "+workflowTestSuffix(), testutil.Cols{
			"status": "in_progress", "parent_issue_id": request,
			"flow_step": step, "flow_task": key, "flow_round": 0,
			"assignee_type": "agent", "assignee_id": agent,
		})
	}
	return flowFixture{
		Workflow:  wf,
		Request:   request,
		Work:      task("flow draft task", "in_progress", "draft", workAgent),
		Gate:      task("flow check task", "in_review", "check", gateAgent),
		WorkAgent: workAgent,
		GateAgent: gateAgent,
	}
}

// issueOutcome reads the stored outcome of an issue, or "" when none.
func issueOutcome(t *testing.T, issueID string) string {
	t.Helper()
	var outcome *string
	dbfx.QueryRow(t, `SELECT outcome FROM issue WHERE id = $1`, issueID).Scan(&outcome)
	if outcome == nil {
		return ""
	}
	return *outcome
}

// attachAs records a file an agent attached to an issue.
func attachAs(t *testing.T, issueID, agentID string) string {
	t.Helper()
	return dbfx.Insert(t, "attachment", testutil.Cols{
		"workspace_id": testWorkspaceID, "issue_id": issueID,
		"uploader_type": "agent", "uploader_id": agentID,
		"filename": "draft.md", "url": "https://files.example.test/draft.md",
		"content_type": "text/markdown", "size_bytes": 12,
	})
}

func wantFlowRejection(t *testing.T, res *testutil.Response, status int, code string) map[string]any {
	t.Helper()
	body := res.Want(status).Map()
	if body["code"] != code {
		t.Fatalf("code = %v, want %s (body %v)", body["code"], code, body)
	}
	return body
}

func closeIssue(t *testing.T, issueID string, extra map[string]any) *testutil.Response {
	t.Helper()
	body := map[string]any{"status": "done"}
	for k, v := range extra {
		body[k] = v
	}
	return updateIssueForTest(t, issueID, body)
}
