package handler

import (
	"context"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// A squad whose active workflow is a flow (S10 of
// docs/engineering/kitviet-workflow-plan.md, decision D11).

// flowSquadIn makes a squad in the workspace with a flow workflow in its
// library, active or not, and returns the squad, the workflow and the two agents
// the flow hands its tasks to.
func flowSquadIn(t *testing.T, fx iniWorkspace, name string, active bool) (squadID, workflowID, workAgent, gateAgent string) {
	t.Helper()
	suffix := workflowTestSuffix()
	lead := iniAgent(t, fx.ID, name+" lead "+suffix)
	workAgent = iniAgent(t, fx.ID, name+" worker "+suffix)
	gateAgent = iniAgent(t, fx.ID, name+" gate "+suffix)
	squadID = dbfx.Squad(t, name+" "+suffix, lead, testutil.Cols{"workspace_id": fx.ID})
	dbfx.SquadMember(t, squadID, "agent", lead, testutil.Cols{"role": "leader"})

	var wf IssueWorkflowResponse
	req := workspaceRequest(fx.AdminID, fx.ID, "POST", "/api/issue-workflows", flowGuardWorkflowBody(name+" flow "+suffix, workAgent, gateAgent))
	testutil.Call(t, testHandler.CreateIssueWorkflow, req).Want(http.StatusCreated).JSON(&wf)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `UPDATE squad SET active_workflow_id = NULL WHERE active_workflow_id = $1`, wf.ID)
		testPool.Exec(context.Background(), `DELETE FROM issue_workflow WHERE id = $1`, wf.ID)
	})
	if active {
		dbfx.Exec(t, `UPDATE issue_workflow SET squad_id = $2 WHERE id = $1`, wf.ID, squadID)
		dbfx.Exec(t, `UPDATE squad SET active_workflow_id = $2 WHERE id = $1`, squadID, wf.ID)
	}
	return squadID, wf.ID, workAgent, gateAgent
}

func TestSetSquadActiveWorkflow_FlowWorkflowDoesNotRemap(t *testing.T) {
	fx := newIniWorkspace(t)
	projectID := dbfxProject(t, fx, "Flow switch")
	squadID, workflowID, _, _ := flowSquadIn(t, fx, "Switch", false)
	plainID := insertSquadWorkflow(t, fx.ID, squadID, "Plain library "+workflowTestSuffix(), "backlog", "in_progress", "blocked")
	dbfx.Exec(t, `UPDATE squad SET active_workflow_id = $2 WHERE id = $1`, squadID, plainID)

	// An issue of the squad sits on a status the flow does not list.
	issueID := dbfx.Issue(t, "squad issue "+workflowTestSuffix(), testutil.Cols{
		"workspace_id": fx.ID, "project_id": projectID, "status": "in_progress",
		"assignee_type": "squad", "assignee_id": squadID, "flow": nil,
	})
	// A task and a request of a flow keep the flow's own state out of the census.
	taskParent := dbfx.Issue(t, "flow request "+workflowTestSuffix(), testutil.Cols{
		"workspace_id": fx.ID, "project_id": projectID, "status": "collect",
		"assignee_type": "squad", "assignee_id": squadID,
		"flow": testutil.Raw(`'{"workflow_id":"` + workflowID + `","step":"collect","rework":0}'::jsonb`),
	})

	var dry struct {
		Plan workflowMappingPlan `json:"plan"`
	}
	setSquadActiveWorkflow(t, fx, squadID, workflowID, nil, true).Want(http.StatusOK).JSON(&dry)
	if len(dry.Plan.Required) != 0 || dry.Plan.AffectedIssues != 0 {
		t.Fatalf("the plan for a flow = %+v, want nothing to map", dry.Plan)
	}
	setSquadActiveWorkflow(t, fx, squadID, workflowID, nil, false).Want(http.StatusOK)

	if got := squadActiveWorkflowID(t, squadID); got != workflowID {
		t.Fatalf("active workflow = %q, want the flow", got)
	}
	if got := squadWorkflowIssueStatus(t, issueID); got != "in_progress" {
		t.Fatalf("the squad's issue moved to %q, a flow does not remap issues", got)
	}
	if got := squadWorkflowIssueStatus(t, taskParent); got != "collect" {
		t.Fatalf("the flow request moved to %q", got)
	}

	// Switching back to the plain workflow leaves the request where its flow put it.
	setSquadActiveWorkflow(t, fx, squadID, plainID, nil, false).Want(http.StatusOK)
	if got := squadWorkflowIssueStatus(t, taskParent); got != "collect" {
		t.Fatalf("switching away from the flow moved its request to %q", got)
	}
}

func TestAssignSquadProject_FlowSquadReturns200(t *testing.T) {
	fx := newIniWorkspace(t)
	squadID, workflowID, workAgent, _ := flowSquadIn(t, fx, "Assign", true)
	projectID := dbfxProject(t, fx, "Flow assign")
	dbfx.Exec(t, `UPDATE agent SET home_squad_id = $2 WHERE id = $1`, workAgent, squadID)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `UPDATE agent SET home_squad_id = NULL WHERE id = $1`, workAgent)
	})
	plain := dbfx.Issue(t, "plain issue of the flow squad "+workflowTestSuffix(), testutil.Cols{
		"workspace_id": fx.ID, "project_id": projectID, "status": "in_progress",
		"assignee_type": "agent", "assignee_id": workAgent,
	})

	assignSquadProject(t, fx, squadID, projectID, nil).Want(http.StatusOK)

	if got := squadActiveWorkflowID(t, squadID); got != workflowID {
		t.Fatalf("assigning the project replaced the squad's flow with %q", got)
	}
	if got := squadWorkflowIssueStatus(t, plain); got != "in_progress" {
		t.Fatalf("an ordinary issue of the squad moved to %q", got)
	}
}

func TestAssignSquadProject_NoWorkflowNonSDLCStill422(t *testing.T) {
	fx := newIniWorkspace(t)
	projectID := dbfxProject(t, fx, "No workflow")
	body := assignSquadProject(t, fx, fx.Analysis, projectID, nil).Want(http.StatusUnprocessableEntity).Map()
	if body["code"] != "build_squad_not_sdlc" {
		t.Fatalf("code = %v, want build_squad_not_sdlc", body["code"])
	}
}

// In a project, a flow squad's request follows its flow and its tasks are not
// held to the project's workflow, while the squad's other issues are.
func TestFlowSquadInProject_RequestUsesFlowOthersUseProject(t *testing.T) {
	f := newTradeFlowFixture(t)
	projectID := projectWithPlainWorkflow(t, "Request in project")
	dbfx.Exec(t, `UPDATE agent SET home_squad_id = $2 WHERE id = $1`, f.Agents.Docs, f.SquadID)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `UPDATE agent SET home_squad_id = NULL WHERE id = $1`, f.Agents.Docs)
	})
	request := f.newRequest(t)
	dbfx.Exec(t, `UPDATE issue SET project_id = $2 WHERE id = $1`, request.ID, projectID)

	state := f.plan(t, request, nil)
	if state.Step != "collect" || requestStatus(t, request) != "collect" {
		t.Fatalf("the request in a project is at %q (status %q), want collect", state.Step, requestStatus(t, request))
	}
	docs := tasksAt(t, request, "collect", 0)["docs"]
	finishTask(t, docs, f.Agents.Docs, "")
	if resp := advance(t, request); resp.State.Step != "draft" {
		t.Fatalf("the request is at %q after its task closed, want draft", resp.State.Step)
	}

	other := dbfx.Issue(t, "other issue of the squad "+workflowTestSuffix(), testutil.Cols{
		"project_id": projectID, "status": "todo", "assignee_type": "agent", "assignee_id": f.Agents.Docs,
	})
	body := updateIssueForTest(t, other, map[string]any{"status": "draft"}).Want(http.StatusBadRequest).Map()
	if body["code"] != "status_not_in_workflow" {
		t.Fatalf("an ordinary issue of the squad: code = %v, want the project's workflow to apply", body["code"])
	}
}
