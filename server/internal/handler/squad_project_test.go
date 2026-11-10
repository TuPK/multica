package handler

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// assignSquadProject posts the assignment as the workspace admin.
func assignSquadProject(t *testing.T, fx iniWorkspace, squadID, projectID string, body map[string]any) *testutil.Response {
	t.Helper()
	if body == nil {
		body = map[string]any{}
	}
	body["project_id"] = projectID
	req := newRequestAs(fx.AdminID, "POST", "/api/squads/"+squadID+"/project", body)
	req = withURLParams(req, "workspaceId", fx.ID, "id", squadID)
	return testutil.Call(t, testHandler.AssignSquadProject, req)
}

// TestAssignSquadProjectSetsSDLCWorkflowAndSquad: the project takes the squad's
// SDLC Auto workflow and shows on the squad's room.
func TestAssignSquadProjectSetsSDLCWorkflowAndSquad(t *testing.T) {
	fx := newIniWorkspace(t)
	projectID := dbfxProject(t, fx, "Mua Ngon Ecommerce")

	var out struct {
		Project struct {
			ID      string  `json:"id"`
			SquadID *string `json:"squad_id"`
		} `json:"project"`
	}
	assignSquadProject(t, fx, fx.Build, projectID, nil).Want(http.StatusOK).JSON(&out)
	if out.Project.SquadID == nil || *out.Project.SquadID != fx.Build {
		t.Fatalf("project squad_id = %v, want %s", out.Project.SquadID, fx.Build)
	}
	if got := projectWorkflowName(t, projectID); got != "SDLC Auto" {
		t.Fatalf("project workflow = %q, want SDLC Auto", got)
	}
}

// TestAssignSquadProjectRefusesSecondOpenProject: a squad works one open project
// at a time, so a second one is refused until the first is unassigned.
func TestAssignSquadProjectRefusesSecondOpenProject(t *testing.T) {
	fx := newIniWorkspace(t)
	first := dbfxProject(t, fx, "First")
	second := dbfxProject(t, fx, "Second")
	assignSquadProject(t, fx, fx.Build, first, nil).Want(http.StatusOK)
	assignSquadProject(t, fx, fx.Build, second, nil).Want(http.StatusConflict)

	var squadID *string
	dbfx.QueryRow(t, `SELECT squad_id::text FROM project WHERE id = $1`, first).Scan(&squadID)
	if squadID == nil || *squadID != fx.Build {
		t.Fatalf("first project squad_id = %v, want it to stay on %s", squadID, fx.Build)
	}
}

// TestAssignSquadProjectReassignsSameProject: re-sending the current project is not a second one.
func TestAssignSquadProjectReassignsSameProject(t *testing.T) {
	fx := newIniWorkspace(t)
	projectID := dbfxProject(t, fx, "Same")
	assignSquadProject(t, fx, fx.Build, projectID, nil).Want(http.StatusOK)
	assignSquadProject(t, fx, fx.Build, projectID, nil).Want(http.StatusOK)
}

// TestUnassignSquadProjectFreesSquad: after unassigning, the squad takes another project.
func TestUnassignSquadProjectFreesSquad(t *testing.T) {
	fx := newIniWorkspace(t)
	first := dbfxProject(t, fx, "First")
	second := dbfxProject(t, fx, "Second")
	assignSquadProject(t, fx, fx.Build, first, nil).Want(http.StatusOK)

	req := newRequestAs(fx.AdminID, "DELETE", "/api/squads/"+fx.Build+"/project", nil)
	req = withURLParams(req, "workspaceId", fx.ID, "id", fx.Build)
	testutil.Call(t, testHandler.UnassignSquadProject, req).Want(http.StatusOK)

	var squadID *string
	dbfx.QueryRow(t, `SELECT squad_id::text FROM project WHERE id = $1`, first).Scan(&squadID)
	if squadID != nil {
		t.Fatalf("project still assigned to %s after unassign", *squadID)
	}
	assignSquadProject(t, fx, fx.Build, second, nil).Want(http.StatusOK)
}

// TestAssignSquadProjectNeedsSDLCSquad: a squad without the SDLC agents cannot build.
func TestAssignSquadProjectNeedsSDLCSquad(t *testing.T) {
	fx := newIniWorkspace(t)
	projectID := dbfxProject(t, fx, "Plain")
	assignSquadProject(t, fx, fx.Product, projectID, nil).Want(http.StatusUnprocessableEntity)
}

// TestAssignSquadProjectRefusesProjectOnAnotherWorkflow: a project already on a
// different workflow is changed from its settings, not by the assignment.
func TestAssignSquadProjectRefusesProjectOnAnotherWorkflow(t *testing.T) {
	fx := newIniWorkspace(t)
	projectID := dbfxProject(t, fx, "Custom flow")
	dbfx.Exec(t, `UPDATE project SET workflow_id = gen_random_uuid() WHERE id = $1`, projectID)
	assignSquadProject(t, fx, fx.Build, projectID, nil).Want(http.StatusConflict)
}

func dbfxProject(t *testing.T, fx iniWorkspace, title string) string {
	t.Helper()
	return dbfx.Project(t, title+" "+fx.ID[:8], testutil.Cols{"workspace_id": fx.ID})
}

func projectWorkflowName(t *testing.T, projectID string) string {
	t.Helper()
	var name *string
	dbfx.QueryRow(t, `SELECT w.name FROM project p JOIN issue_workflow w ON w.id = p.workflow_id WHERE p.id = $1`, projectID).Scan(&name)
	if name == nil {
		return ""
	}
	return *name
}

// planningSquad makes a squad led by the quickstart's planning lead, and returns its id
// and the lead's id.
func planningSquad(t *testing.T, fx iniWorkspace, name string) (string, string) {
	t.Helper()
	lead := iniAgent(t, fx.ID, "Blueprint Lead")
	squad := dbfx.Squad(t, name+" "+fx.ID[:8], lead, testutil.Cols{"workspace_id": fx.ID})
	dbfx.SquadMember(t, squad, "agent", lead, testutil.Cols{"role": "leader"})
	return squad, lead
}

// createPlanningWorkflow stores a "BMAD Planning Auto" workflow whose blocked step is handed to the lead,
// as the quickstart creates it.
func createPlanningWorkflow(t *testing.T, fx iniWorkspace, lead string) {
	t.Helper()
	steps, err := issueworkflow.EncodeSteps(issueworkflow.Arrange([]issueworkflow.Step{
		{StatusKey: "backlog", Handler: issueworkflow.Handler{Type: issueworkflow.HandlerNone}},
		{StatusKey: "blocked", Handler: issueworkflow.Handler{Type: issueworkflow.HandlerAgent, ID: lead}},
	}))
	if err != nil {
		t.Fatalf("encode steps: %v", err)
	}
	if _, err := testHandler.Queries.CreateIssueWorkflow(context.Background(), db.CreateIssueWorkflowParams{
		WorkspaceID:      util.MustParseUUID(fx.ID),
		Name:             "BMAD Planning Auto",
		InitialStatusKey: "todo",
		Steps:            steps,
	}); err != nil {
		t.Fatalf("create planning workflow: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM issue_workflow WHERE workspace_id = $1`, fx.ID)
	})
}

// TestAssignSquadProjectGivesAPlanningSquadItsWorkflow: a squad led by the planning lead takes
// the planning workflow the quickstart made, not SDLC Auto.
func TestAssignSquadProjectGivesAPlanningSquadItsWorkflow(t *testing.T) {
	fx := newIniWorkspace(t)
	squad, lead := planningSquad(t, fx, "Planning")
	createPlanningWorkflow(t, fx, lead)
	projectID := dbfxProject(t, fx, "Plans")

	assignSquadProject(t, fx, squad, projectID, nil).Want(http.StatusOK)

	if got := projectWorkflowName(t, projectID); got != "BMAD Planning Auto" {
		t.Fatalf("project workflow = %q, want BMAD Planning Auto", got)
	}
}

// TestAssignSquadProjectNeedsThePlanningWorkflow: without the workflow the quickstart creates, the
// assignment says so instead of falling back to the SDLC workflow.
func TestAssignSquadProjectNeedsThePlanningWorkflow(t *testing.T) {
	fx := newIniWorkspace(t)
	squad, _ := planningSquad(t, fx, "Planning")
	projectID := dbfxProject(t, fx, "Plans")

	resp := assignSquadProject(t, fx, squad, projectID, nil).Want(http.StatusUnprocessableEntity)
	if got := resp.Text(); !strings.Contains(got, "planning_workflow_missing") {
		t.Fatalf("body = %s, want code planning_workflow_missing", got)
	}
	var withWorkflow int
	dbfx.QueryRow(t, `SELECT count(*) FROM project WHERE id = $1 AND workflow_id IS NOT NULL`, projectID).Scan(&withWorkflow)
	if withWorkflow != 0 {
		t.Fatalf("the project got a workflow although the planning workflow is missing")
	}
}
