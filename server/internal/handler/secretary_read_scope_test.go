package handler

import (
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// A Secretary run in a chat turn reads its request tree only: its desk, its
// requests, and the children of those requests. Anything else on the board is
// refused, and the refusal is the platform's, not the instructions'.
func TestSecretaryChatRun_ReadsItsRequestTreeOnly(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := seedDirectiveRun(t, "Is the spring launch on track?")
	desk := dbfx.Project(t, "Secretary desk", testutil.Cols{"workspace_id": testWorkspaceID})
	board := dbfx.Project(t, "Board "+t.Name(), testutil.Cols{"workspace_id": testWorkspaceID})
	boardIssue := dbfx.Issue(t, "Board issue outside the Secretary", testutil.Cols{"workspace_id": testWorkspaceID, "project_id": board})
	request := dbfx.Issue(t, "Desk request", testutil.Cols{"workspace_id": testWorkspaceID, "project_id": desk, "assignee_type": "agent", "assignee_id": f.secretaryID})
	child := dbfx.Issue(t, "Leader child of the request", testutil.Cols{"workspace_id": testWorkspaceID, "project_id": board, "parent_issue_id": request})

	get := func(issueID string) *testutil.Response {
		req := newRequest("GET", "/api/issues/"+issueID, nil)
		req.Header.Set("X-Agent-ID", f.secretaryID)
		req.Header.Set("X-Task-ID", f.taskID)
		req = withURLParam(req, "id", issueID)
		return testutil.Call(t, testHandler.GetIssue, req)
	}
	get(boardIssue).Want(http.StatusForbidden)
	get(request).Want(http.StatusOK)
	get(child).Want(http.StatusOK)

	list := func(query string) *testutil.Response {
		req := newRequest("GET", "/api/issues?workspace_id="+testWorkspaceID+query, nil)
		req.Header.Set("X-Agent-ID", f.secretaryID)
		req.Header.Set("X-Task-ID", f.taskID)
		return testutil.Call(t, testHandler.ListIssues, req)
	}
	list("").Want(http.StatusForbidden)
	list("&project_id=" + board).Want(http.StatusForbidden)
	list("&project_id=" + desk).Want(http.StatusOK)
}
