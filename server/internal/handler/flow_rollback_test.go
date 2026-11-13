package handler

import (
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// Rollback (S27, docs/engineering/kitviet-runbook.md section "Flow"): each way back from the flow
// engine leaves ordinary issues and no half state. The migrations' own down path runs in
// server/cmd/migrate (TestFlowMigrations_UpDown, in a scratch schema).

func flowGet(t *testing.T, issueID string) *testutil.Response {
	t.Helper()
	req := withURLParam(newRequest("GET", "/api/issues/"+issueID+"/flow", nil), "id", issueID)
	return testutil.Call(t, testHandler.GetIssueFlow, req)
}

func TestRollback_FlagOffTurnsFlowWorkflowIntoPlain(t *testing.T) {
	f, request := autoFixture(t)
	id := uuidToString(request.ID)
	docs := tasksAt(t, request, "collect", 0)["docs"]
	withFeatureFlag(t, testHandler, "flow_engine", false)

	// The API steps aside, the guards step aside, the runner has nothing to do.
	flowGet(t, id).Want(http.StatusNotFound)
	updateIssueForTest(t, docs, map[string]any{"status": "done"}).Want(http.StatusOK)
	agentSetStatus(t, id, f.Agents.Docs, "collect", "in_progress").Want(http.StatusOK)
	if n := sweep(t); n != 0 {
		t.Fatalf("the sweep changed %d requests with the engine off", n)
	}
	// The stored state is kept, so turning the engine back on resumes where it was.
	if _, ok := flowRequestState(loadIssueRow(t, id)); !ok {
		t.Fatal("turning the engine off erased the flow state")
	}
}

func TestRollback_DetachAllThenMigrationDown(t *testing.T) {
	f, first := autoFixture(t)
	second := f.newRequest(t)
	f.plan(t, second, nil)
	for _, request := range []string{uuidToString(first.ID), uuidToString(second.ID)} {
		req := withURLParam(newRequest("POST", "/api/issues/"+request+"/flow/detach", map[string]any{"cancel_tasks": false}), "id", request)
		testutil.Call(t, testHandler.DetachIssueFlow, req).Want(http.StatusOK)
	}

	// Nothing of the flow is left on any issue of the workspace, so dropping the columns loses no flow state.
	if n := dbfx.Count(t, `SELECT count(*) FROM issue WHERE workspace_id = $1 AND (flow IS NOT NULL OR flow_task IS NOT NULL OR flow_step IS NOT NULL OR flow_round IS NOT NULL)`, testWorkspaceID); n != 0 {
		t.Fatalf("%d issues still carry flow columns after detaching every request", n)
	}
	// The tasks are ordinary sub-issues: one closes with no outcome and no attachment.
	if n := countTasks(t, first); n != 0 {
		t.Fatalf("%d issues still count as flow tasks", n)
	}
	var child string
	dbfx.QueryRow(t, `SELECT id::text FROM issue WHERE parent_issue_id = $1 LIMIT 1`, first.ID).Scan(&child)
	if child == "" {
		t.Fatal("the detached request lost its task sub-issues")
	}
	updateIssueForTest(t, child, map[string]any{"status": "done"}).Want(http.StatusOK)
}

func TestRollback_LeadTextFallsBackWhenNotManaged(t *testing.T) {
	seedTestCatalog(t)
	plain := dbfx.Issue(t, "plain issue "+workflowTestSuffix())

	// The lead's first command, `multica issue flow show`, gets a 404 with this code on an issue no
	// flow runs, and the lead then follows its old procedure.
	body := flowGet(t, plain).Want(http.StatusNotFound).Map()
	if body["code"] != "flow_not_managed" {
		t.Fatalf("code = %v, want flow_not_managed", body["code"])
	}
}
