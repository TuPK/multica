package handler

// Workflow routes and restricted projects (spec 7.1 rules 2, 3, 5). The
// outsider is a plain member who is not granted the restricted project; to make
// a refusal come from visibility and not from a role check, the outsider is
// the lead of that project and the owner of the workflow it uses.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// newWorkflowRestrictedFixture puts a restricted project on a new workflow.
// The outsider leads the project and owns the workflow; the grantee is granted
// the project.
func newWorkflowRestrictedFixture(t *testing.T) (restrictedFixture, IssueWorkflowResponse) {
	t.Helper()
	seedTestCatalog(t)
	f := newRestrictedFixture(t)
	wf := createTestWorkflow(t, "Restricted "+workflowTestSuffix(), "in_progress")
	dbfx.Exec(t, `UPDATE project SET lead_type = 'member', lead_id = $1 WHERE id = $2`,
		f.outsiderID, f.restrictedProjectID)
	dbfx.Exec(t, `UPDATE issue_workflow SET owner_id = $1 WHERE id = $2`, f.outsiderID, wf.ID)
	return f, wf
}

// answersMissing fails unless the missing id answers 404 and the hidden one
// answers the same, ids apart.
func answersMissing(t *testing.T, name string, hidden, missing *httptest.ResponseRecorder, hiddenID, missingID string) {
	t.Helper()
	if missing.Code != http.StatusNotFound {
		t.Fatalf("%s on a missing id: want 404, got %d %s", name, missing.Code, missing.Body.String())
	}
	sameAnswer(t, name, hidden, missing, hiddenID, missingID)
}

func serveWorkflowRoute(h http.HandlerFunc, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h(w, req)
	return w
}

// TestRestrictedWorkflowRoutesAnswerMissing: every workflow route that names a
// restricted project, or a workflow a restricted project uses, answers an
// outsider like a missing project or workflow, and changes nothing.
func TestRestrictedWorkflowRoutesAnswerMissing(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	// Each route gets its own fixture: a route that wrongly succeeds must not
	// change what the next one sees.
	type routeFixture struct {
		restrictedFixture
		wf        IssueWorkflowResponse
		issueID   string
		missingID string
	}
	setup := func(t *testing.T) routeFixture {
		f, wf := newWorkflowRestrictedFixture(t)
		return routeFixture{
			restrictedFixture: f,
			wf:                wf,
			issueID:           dbfx.Issue(t, "stays on in_progress", testutil.Cols{"project_id": f.restrictedProjectID, "status": "in_progress"}),
			missingID:         uuid.NewString(),
		}
	}

	t.Run("PreviewIssueWorkflowBrief", func(t *testing.T) {
		x := setup(t)
		preview := func(projectID string) *httptest.ResponseRecorder {
			req := withURLParam(requestAs(x.outsiderID, "POST", "/api/issue-workflows/"+x.wf.ID+"/brief?workspace_id="+testWorkspaceID,
				map[string]any{"status_key": "in_progress", "project_id": projectID}), "id", x.wf.ID)
			return serveWorkflowRoute(testHandler.PreviewIssueWorkflowBrief, req)
		}
		hidden, missing := preview(x.restrictedProjectID), preview(x.missingID)
		if hidden.Code != http.StatusOK || hidden.Code != missing.Code || hidden.Body.String() != missing.Body.String() {
			t.Errorf("a hidden project is named in the brief:\nhidden:  %d %s\nmissing: %d %s",
				hidden.Code, hidden.Body.String(), missing.Code, missing.Body.String())
		}
	})

	t.Run("PreviewWorkflowHandoff", func(t *testing.T) {
		x := setup(t)
		openIssue := dbfx.Issue(t, "visible issue", testutil.Cols{"project_id": x.workspaceProjectID, "status": "todo"})
		preview := func(projectID string) *httptest.ResponseRecorder {
			req := withURLParam(requestAs(x.outsiderID, "GET", "/api/issues/"+openIssue+"/workflow-handoff?status=in_progress&project_id="+projectID+"&workspace_id="+testWorkspaceID, nil),
				"id", openIssue)
			return serveWorkflowRoute(testHandler.PreviewWorkflowHandoff, req)
		}
		answersMissing(t, "PreviewWorkflowHandoff", preview(x.restrictedProjectID), preview(x.missingID), x.restrictedProjectID, x.missingID)
	})
}

// flowWorkflowWithRequests creates a flow workflow owned by the outsider and
// two open requests pinned to it: one in the restricted project, one in the
// workspace project. It returns the workflow id and the two request ids.
func flowWorkflowWithRequests(t *testing.T, f restrictedFixture) (workflowID, hiddenRequest, visibleRequest string) {
	t.Helper()
	seedTestCatalog(t)
	created := createFlowWorkflowForTest(t, flowWorkflowBody("Hidden requests "+workflowTestSuffix(), seededReadyAgentID(t), nil))
	dbfx.Exec(t, `UPDATE issue_workflow SET owner_id = $1 WHERE id = $2`, f.outsiderID, created.ID)
	request := func(title, projectID string) string {
		return dbfx.Issue(t, title, testutil.Cols{
			"project_id": projectID,
			"flow":       testutil.Raw(fmt.Sprintf(`'{"workflow_id":"%s","step":"todo","rework":0}'::jsonb`, created.ID)),
		})
	}
	return created.ID, request("hidden flow request", f.restrictedProjectID), request("visible flow request", f.workspaceProjectID)
}

// TestRestrictedWorkflowOpenRequestCountsOmitHidden: open_flow_requests on a
// workflow counts only the requests the caller sees.
func TestRestrictedWorkflowOpenRequestCountsOmitHidden(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	workflowID, _, _ := flowWorkflowWithRequests(t, f)

	openRequests := func(userID string) int {
		t.Helper()
		var out struct {
			Workflows []IssueWorkflowResponse `json:"workflows"`
		}
		req := workspaceRequest(userID, testWorkspaceID, "GET", "/api/issue-workflows", nil)
		testutil.Call(t, testHandler.ListIssueWorkflows, req).Want(http.StatusOK).JSON(&out)
		for _, wf := range out.Workflows {
			if wf.ID == workflowID {
				return wf.OpenFlowRequests
			}
		}
		t.Fatalf("workflow %s missing from the list as %s", workflowID, userID)
		return 0
	}
	if got := openRequests(f.outsiderID); got != 1 {
		t.Errorf("outsider open_flow_requests = %d, want 1", got)
	}
	if got := openRequests(testUserID); got != 2 {
		t.Errorf("owner open_flow_requests = %d, want 2", got)
	}
}
