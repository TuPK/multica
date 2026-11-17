package handler

// Workflow routes and restricted projects (spec 7.1 rules 2, 3, 5). The
// outsider is a plain member who is not granted the restricted project; to make
// a refusal come from visibility and not from a role check, the outsider is
// the lead of that project and the owner of the workflow it uses.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/projectaccess"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// newWorkflowRestrictedFixture puts a restricted project on a new workflow.
// The outsider leads the project and owns the workflow; the grantee is granted
// the project.
func newWorkflowRestrictedFixture(t *testing.T) (restrictedFixture, IssueWorkflowResponse) {
	t.Helper()
	seedTestCatalog(t)
	f := newRestrictedFixture(t)
	wf := createTestWorkflow(t, "Restricted "+workflowTestSuffix(), "in_progress")
	dbfx.Exec(t, `UPDATE project SET workflow_id = $1, lead_type = 'member', lead_id = $2 WHERE id = $3`,
		wf.ID, f.outsiderID, f.restrictedProjectID)
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
	workflowOf := func(t *testing.T, projectID string) string {
		var id *string
		dbfx.QueryRow(t, `SELECT workflow_id::text FROM project WHERE id = $1`, projectID).Scan(&id)
		if id == nil {
			return ""
		}
		return *id
	}

	t.Run("SetProjectWorkflow", func(t *testing.T) {
		x := setup(t)
		other := createTestWorkflow(t, "Other "+workflowTestSuffix(), "in_progress")
		set := func(projectID string) *httptest.ResponseRecorder {
			req := withURLParam(requestAs(x.outsiderID, "POST", "/api/projects/"+projectID+"/workflow?workspace_id="+testWorkspaceID,
				map[string]any{"workflow_id": other.ID}), "id", projectID)
			return serveWorkflowRoute(testHandler.SetProjectWorkflow, req)
		}
		answersMissing(t, "SetProjectWorkflow", set(x.restrictedProjectID), set(x.missingID), x.restrictedProjectID, x.missingID)
		if got := workflowOf(t, x.restrictedProjectID); got != x.wf.ID {
			t.Errorf("the restricted project's workflow changed to %s", got)
		}
	})

	t.Run("UpdateIssueWorkflow", func(t *testing.T) {
		x := setup(t)
		patch := func(id string, body map[string]any) *httptest.ResponseRecorder {
			req := withURLParam(requestAs(x.outsiderID, "PATCH", "/api/issue-workflows/"+id+"?workspace_id="+testWorkspaceID, body), "id", id)
			return serveWorkflowRoute(testHandler.UpdateIssueWorkflow, req)
		}
		drop := map[string]any{"steps": statusSteps("backlog", "blocked"), "status_mapping": map[string]string{"in_progress": "todo"}}
		answersMissing(t, "UpdateIssueWorkflow drop step", patch(x.wf.ID, drop), patch(x.missingID, drop), x.wf.ID, x.missingID)
		if status, _, _ := issueStatusAndAssignee(t, x.issueID); status != "in_progress" {
			t.Errorf("a hidden issue was remapped to %q", status)
		}
		rename := map[string]any{"name": "Taken " + workflowTestSuffix()}
		answersMissing(t, "UpdateIssueWorkflow rename", patch(x.wf.ID, rename), patch(x.missingID, rename), x.wf.ID, x.missingID)
		var name string
		dbfx.QueryRow(t, `SELECT name FROM issue_workflow WHERE id = $1`, x.wf.ID).Scan(&name)
		if name != x.wf.Name {
			t.Errorf("the workflow was renamed to %q", name)
		}
	})

	t.Run("DeleteIssueWorkflow", func(t *testing.T) {
		x := setup(t)
		del := func(id string) *httptest.ResponseRecorder {
			req := withURLParam(requestAs(x.outsiderID, "DELETE", "/api/issue-workflows/"+id+"?workspace_id="+testWorkspaceID, nil), "id", id)
			return serveWorkflowRoute(testHandler.DeleteIssueWorkflow, req)
		}
		answersMissing(t, "DeleteIssueWorkflow", del(x.wf.ID), del(x.missingID), x.wf.ID, x.missingID)
		var n int
		dbfx.QueryRow(t, `SELECT count(*) FROM issue_workflow WHERE id = $1`, x.wf.ID).Scan(&n)
		if n != 1 {
			t.Errorf("the workflow was deleted")
		}
	})

	t.Run("SetIssueWorkflowHandoff copy", func(t *testing.T) {
		x := setup(t)
		agentID := seededReadyAgentID(t)
		copyFor := func(projectID string) *httptest.ResponseRecorder {
			body := handToAgent(agentID, "Only here")
			body["copy_for_project_id"] = projectID
			req := withURLParams(requestAs(x.outsiderID, "PUT", "/api/issue-workflows/"+x.wf.ID+"/handoffs/in_progress?workspace_id="+testWorkspaceID, body),
				"id", x.wf.ID, "status", "in_progress")
			return serveWorkflowRoute(testHandler.SetIssueWorkflowHandoff, req)
		}
		answersMissing(t, "SetIssueWorkflowHandoff copy", copyFor(x.restrictedProjectID), copyFor(x.missingID), x.restrictedProjectID, x.missingID)
		if got := workflowOf(t, x.restrictedProjectID); got != x.wf.ID {
			t.Errorf("the restricted project moved to the copy %s", got)
		}
	})

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

// TestRestrictedWorkflowRemapSkipsNothingVisibleToOwner: the owner's remap of a
// restricted project's issues moves every one of them, through a project
// switch and through a workflow edit, and a grantee who leads the project gets
// the plan the owner gets.
func TestRestrictedWorkflowRemapSkipsNothingVisibleToOwner(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f, wf := newWorkflowRestrictedFixture(t)
	dbfx.Exec(t, `UPDATE project SET lead_id = $1 WHERE id = $2`, f.granteeID, f.restrictedProjectID)
	without := createTestWorkflow(t, "Without "+workflowTestSuffix())
	var issues []string
	for _, title := range []string{"first", "second"} {
		issues = append(issues, dbfx.Issue(t, title, testutil.Cols{"project_id": f.restrictedProjectID, "status": "in_progress"}))
	}
	assertStatuses := func(want string) {
		t.Helper()
		for _, id := range issues {
			if status, _, _ := issueStatusAndAssignee(t, id); status != want {
				t.Errorf("issue %s is on %q, want %q", id, status, want)
			}
		}
	}
	switchAs := func(userID string, body map[string]any) *httptest.ResponseRecorder {
		req := withURLParam(requestAs(userID, "POST", "/api/projects/"+f.restrictedProjectID+"/workflow?workspace_id="+testWorkspaceID, body), "id", f.restrictedProjectID)
		return serveWorkflowRoute(testHandler.SetProjectWorkflow, req)
	}

	dry := map[string]any{"workflow_id": without.ID, "dry_run": true}
	ownerDry, granteeDry := switchAs(testUserID, dry), switchAs(f.granteeID, dry)
	if ownerDry.Code != http.StatusOK || granteeDry.Code != http.StatusOK || ownerDry.Body.String() != granteeDry.Body.String() {
		t.Fatalf("dry run differs:\nowner:   %d %s\ngrantee: %d %s", ownerDry.Code, ownerDry.Body.String(), granteeDry.Code, granteeDry.Body.String())
	}
	if !strings.Contains(ownerDry.Body.String(), `"issue_count":2`) {
		t.Fatalf("the plan does not count both issues: %s", ownerDry.Body.String())
	}

	if w := switchAs(testUserID, map[string]any{"workflow_id": without.ID, "status_mapping": map[string]string{"in_progress": "todo"}}); w.Code != http.StatusOK {
		t.Fatalf("owner switch: %d %s", w.Code, w.Body.String())
	} else if !strings.Contains(w.Body.String(), `"updated_issues":2`) {
		t.Errorf("owner switch did not move both issues: %s", w.Body.String())
	}
	assertStatuses("todo")

	// A workflow edit remaps the issues of every project on it.
	dbfx.Exec(t, `UPDATE project SET workflow_id = $1 WHERE id = $2`, wf.ID, f.restrictedProjectID)
	dbfx.Exec(t, `UPDATE issue SET status = 'in_progress' WHERE id = ANY($1::uuid[])`, issues)
	edit := map[string]any{"steps": statusSteps("backlog", "blocked"), "status_mapping": map[string]string{"in_progress": "todo"}}
	req := withURLParam(requestAs(testUserID, "PATCH", "/api/issue-workflows/"+wf.ID+"?workspace_id="+testWorkspaceID, edit), "id", wf.ID)
	if w := serveWorkflowRoute(testHandler.UpdateIssueWorkflow, req); w.Code != http.StatusOK {
		t.Fatalf("owner edit: %d %s", w.Code, w.Body.String())
	}
	assertStatuses("todo")
}

// TestRestrictedWorkflowListsOmitHidden: the workflow lists and the queries
// behind them leave a restricted project and its issues out for an outsider.
func TestRestrictedWorkflowListsOmitHidden(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f, wf := newWorkflowRestrictedFixture(t)
	dbfx.Issue(t, "hidden in_progress", testutil.Cols{"project_id": f.restrictedProjectID, "status": "in_progress"})
	dbfx.Issue(t, "open in_progress", testutil.Cols{"project_id": f.workspaceProjectID, "status": "in_progress"})
	dbfx.Exec(t, `UPDATE project SET workflow_id = $1 WHERE id = $2`, wf.ID, f.workspaceProjectID)

	projectIDs := func(userID string) []string {
		t.Helper()
		var list struct {
			Workflows []IssueWorkflowResponse `json:"workflows"`
		}
		w := serveWorkflowRoute(testHandler.ListIssueWorkflows, requestAs(userID, "GET", "/api/issue-workflows?workspace_id="+testWorkspaceID, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("ListIssueWorkflows as %s: %d %s", userID, w.Code, w.Body.String())
		}
		if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
			t.Fatalf("decode ListIssueWorkflows: %v", err)
		}
		for _, item := range list.Workflows {
			if item.ID == wf.ID {
				return item.ProjectIDs
			}
		}
		t.Fatalf("workflow %s missing from the list", wf.ID)
		return nil
	}
	contains := func(ids []string, id string) bool {
		for _, got := range ids {
			if got == id {
				return true
			}
		}
		return false
	}
	if contains(projectIDs(f.outsiderID), f.restrictedProjectID) {
		t.Errorf("ListIssueWorkflows lists the restricted project for the outsider")
	}
	if !contains(projectIDs(f.outsiderID), f.workspaceProjectID) {
		t.Errorf("ListIssueWorkflows omits the workspace project for the outsider")
	}
	for name, userID := range map[string]string{"grantee": f.granteeID, "owner": testUserID} {
		if !contains(projectIDs(userID), f.restrictedProjectID) {
			t.Errorf("ListIssueWorkflows omits the restricted project for the %s", name)
		}
	}

	usage := func(userID string) string {
		w := serveWorkflowRoute(testHandler.ListIssueWorkflowUsage, requestAs(userID, "GET", "/api/issue-workflows/usage?workspace_id="+testWorkspaceID, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("ListIssueWorkflowUsage as %s: %d %s", userID, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	if strings.Contains(usage(f.outsiderID), f.restrictedProjectID) {
		t.Errorf("ListIssueWorkflowUsage lists the restricted project for the outsider")
	}
	if !strings.Contains(usage(testUserID), f.restrictedProjectID) {
		t.Errorf("ListIssueWorkflowUsage omits the restricted project for the owner")
	}
}

// TestRestrictedWorkflowQueriesApplyScope: the queries workflow management
// reads give an outsider no restricted project and no hidden issue, and the
// owner everything.
func TestRestrictedWorkflowQueriesApplyScope(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f, wf := newWorkflowRestrictedFixture(t)
	dbfx.Issue(t, "hidden in_progress", testutil.Cols{"project_id": f.restrictedProjectID, "status": "in_progress"})
	dbfx.Issue(t, "open in_progress", testutil.Cols{"project_id": f.workspaceProjectID, "status": "in_progress"})
	dbfx.Exec(t, `UPDATE project SET workflow_id = $1 WHERE id = $2`, wf.ID, f.workspaceProjectID)
	ctx := context.Background()
	ws := parseUUID(testWorkspaceID)

	scopeOf := func(userID string) projectaccess.Scope {
		t.Helper()
		scope, err := projectaccess.ForViewer(ctx, testHandler.Queries, ws, parseUUID(userID))
		if err != nil {
			t.Fatalf("scope for %s: %v", userID, err)
		}
		return scope
	}
	viewers := map[string]projectaccess.Scope{"owner": scopeOf(testUserID), "grantee": scopeOf(f.granteeID), "outsider": scopeOf(f.outsiderID)}
	wantRestricted := map[string]bool{"owner": true, "grantee": true, "outsider": false}
	hasProject := func(projects []db.Project, id string) bool {
		for _, p := range projects {
			if uuidToString(p.ID) == id {
				return true
			}
		}
		return false
	}
	for name, scope := range viewers {
		onWorkflow, err := testHandler.Queries.ListProjectsUsingIssueWorkflow(ctx, db.ListProjectsUsingIssueWorkflowParams{
			WorkspaceID: ws, WorkflowID: parseUUID(wf.ID), ScopeMode: scope.Mode(), ScopeUserID: scope.UserID(),
		})
		if err != nil {
			t.Fatalf("ListProjectsUsingIssueWorkflow as %s: %v", name, err)
		}
		if got := hasProject(onWorkflow, f.restrictedProjectID); got != wantRestricted[name] {
			t.Errorf("ListProjectsUsingIssueWorkflow as %s: restricted project listed = %v, want %v", name, got, wantRestricted[name])
		}
		if !hasProject(onWorkflow, f.workspaceProjectID) {
			t.Errorf("ListProjectsUsingIssueWorkflow as %s omits the workspace project", name)
		}

		counts, err := testHandler.Queries.CountProjectIssuesByStatus(ctx, db.CountProjectIssuesByStatusParams{
			WorkspaceID: ws, ProjectIds: []pgtype.UUID{parseUUID(f.restrictedProjectID), parseUUID(f.workspaceProjectID)},
			ScopeMode: scope.Mode(), ScopeUserID: scope.UserID(),
		})
		if err != nil {
			t.Fatalf("CountProjectIssuesByStatus as %s: %v", name, err)
		}
		want := int64(1)
		if wantRestricted[name] {
			want = 2
		}
		var got int64
		for _, c := range counts {
			if c.Status == "in_progress" {
				got = c.IssueCount
			}
		}
		if got != want {
			t.Errorf("CountProjectIssuesByStatus as %s: in_progress = %d, want %d", name, got, want)
		}
	}

	// A project on Default is listed by the Default query under the same rule.
	dbfx.Exec(t, `UPDATE project SET workflow_id = NULL WHERE id = $1`, f.restrictedProjectID)
	for name, scope := range viewers {
		onDefault, err := testHandler.Queries.ListProjectsUsingDefaultIssueWorkflow(ctx, db.ListProjectsUsingDefaultIssueWorkflowParams{
			WorkspaceID: ws, ScopeMode: scope.Mode(), ScopeUserID: scope.UserID(),
		})
		if err != nil {
			t.Fatalf("ListProjectsUsingDefaultIssueWorkflow as %s: %v", name, err)
		}
		if got := hasProject(onDefault, f.restrictedProjectID); got != wantRestricted[name] {
			t.Errorf("ListProjectsUsingDefaultIssueWorkflow as %s: restricted project listed = %v, want %v", name, got, wantRestricted[name])
		}
	}
}
