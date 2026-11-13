package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// The write guards on a request a flow runs (R3 to R7 of
// docs/engineering/kitviet-workflow-spec.md, section 4.2).

// agentCall sends a request as an agent's run on the issue, the way the CLI of a
// running agent does.
func agentCall(t *testing.T, issueID, agentID, step string, h http.HandlerFunc, method, path string, body map[string]any) *testutil.Response {
	t.Helper()
	runID := goldenRun(t, issueID, agentID, step)
	req := withURLParam(newRequest(method, path, body), "id", issueID)
	req.Header.Set("X-Actor-Source", "task_token")
	req.Header.Set("X-Agent-ID", agentID)
	req.Header.Set("X-Task-ID", runID)
	return testutil.Call(t, h, req)
}

func agentSetStatus(t *testing.T, issueID, agentID, step, status string) *testutil.Response {
	t.Helper()
	return agentCall(t, issueID, agentID, step, testHandler.UpdateIssue,
		"PUT", "/api/issues/"+issueID, map[string]any{"status": status})
}

// moveRequest puts the request at a step of the flow, as the engine would have.
func moveRequest(t *testing.T, request db.Issue, step string) {
	t.Helper()
	dbfx.Exec(t, `UPDATE issue SET status = $2, flow = flow || jsonb_build_object('step', $2::text) WHERE id = $1`, request.ID, step)
}

func TestGuard_HumanOnlyStep403(t *testing.T) {
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)
	moveRequest(t, request, "in_review")
	id := uuidToString(request.ID)

	// An agent may not leave the step, nor enter it.
	wantFlowRejection(t, agentSetStatus(t, id, f.Agents.Handover, "in_review", "done"), http.StatusForbidden, "flow_human_only")
	moveRequest(t, request, "handover")
	wantFlowRejection(t, agentSetStatus(t, id, f.Agents.Handover, "handover", "in_review"), http.StatusForbidden, "flow_human_only")
	moveRequest(t, request, "in_review")

	// A member who does not manage the squad may not either.
	_, asMember := addWorkflowTestMember(t, "flow-plain")
	body := testutil.Call(t, testHandler.UpdateIssue, asMember(withURLParam(newRequest("PUT", "/api/issues/"+id, map[string]any{"status": "done"}), "id", id))).
		Want(http.StatusForbidden).Map()
	if body["code"] != "flow_forbidden" {
		t.Fatalf("code = %v, want flow_forbidden", body["code"])
	}
	if status, _, _ := issueStatusAndAssignee(t, id); status != "in_review" {
		t.Fatalf("status = %q, the refused moves must not apply", status)
	}

	// The member who manages the squad closes it; the flow ends with it.
	updateIssueForTest(t, id, map[string]any{"status": "done"}).Want(http.StatusOK)
	var closed *string
	dbfx.QueryRow(t, `SELECT flow->>'closed_at' FROM issue WHERE id = $1`, id).Scan(&closed)
	if closed == nil {
		t.Fatal("a request closed by a member has no closed_at")
	}
}

func TestGuard_AgentWriteToFlowRequest409(t *testing.T) {
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)
	f.plan(t, request, nil)
	id := uuidToString(request.ID)

	body := agentSetStatus(t, id, f.Agents.Docs, "collect", "draft").Want(http.StatusConflict).Map()
	if body["code"] != "flow_managed_request" || body["step"] != "collect" || body["hint"] != "multica issue flow advance" {
		t.Fatalf("single update refusal = %v", body)
	}
	// The same rule holds on the batch path.
	batch := agentCall(t, id, f.Agents.Docs, "collect", testHandler.BatchUpdateIssues,
		"POST", "/api/issues/batch-update?workspace_id="+testWorkspaceID,
		map[string]any{"issue_ids": []string{id}, "updates": map[string]any{"status": "draft"}})
	wantFlowRejection(t, batch, http.StatusConflict, "flow_managed_request")
	if status, _, _ := issueStatusAndAssignee(t, id); status != "collect" {
		t.Fatalf("status = %q, the refused writes must not apply", status)
	}
	// An agent may still write anything but the status of the request.
	agentCall(t, id, f.Agents.Docs, "collect", testHandler.UpdateIssue,
		"PUT", "/api/issues/"+id, map[string]any{"title": "renamed by an agent " + workflowTestSuffix()}).Want(http.StatusOK)
}

func TestGuard_AgentCreateChildOfFlowRequest409(t *testing.T) {
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)
	f.plan(t, request, nil)
	id := uuidToString(request.ID)

	body := agentCall(t, id, f.Agents.Docs, "collect", testHandler.CreateIssue,
		"POST", "/api/issues?workspace_id="+testWorkspaceID,
		map[string]any{"title": "an agent's sub-issue " + workflowTestSuffix(), "parent_issue_id": id}).Want(http.StatusConflict).Map()
	if body["code"] != "flow_children_managed" || body["step"] != "collect" {
		t.Fatalf("refusal = %v, want flow_children_managed at collect", body)
	}
	// A member may add a sub-issue.
	var child IssueResponse
	testutil.Call(t, testHandler.CreateIssue, newRequest("POST", "/api/issues?workspace_id="+testWorkspaceID,
		map[string]any{"title": "a member's sub-issue " + workflowTestSuffix(), "parent_issue_id": id})).Want(http.StatusCreated).JSON(&child)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM issue WHERE id = $1`, child.ID) })
	if child.Flow != nil {
		t.Fatalf("a member's sub-issue is a task: %+v", child.Flow)
	}
}

// The public API does not take the columns that make an issue a task.
func TestGuard_PublicAPIIgnoresFlowColumns(t *testing.T) {
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)
	id := uuidToString(request.ID)

	var child IssueResponse
	testutil.Call(t, testHandler.CreateIssue, newRequest("POST", "/api/issues?workspace_id="+testWorkspaceID, map[string]any{
		"title": "forged task " + workflowTestSuffix(), "parent_issue_id": id,
		"flow_step": "collect", "flow_task": "docs", "flow_round": 0, "flow": map[string]any{"step": "collect"},
	})).Want(http.StatusCreated).JSON(&child)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM issue WHERE id = $1`, child.ID) })
	row := loadIssueRow(t, child.ID)
	if row.FlowTask.Valid || row.FlowStep.Valid || len(row.Flow) != 0 {
		t.Fatalf("the create took the flow columns: %+v", row)
	}
	updateIssueForTest(t, child.ID, map[string]any{"flow_task": "docs", "flow": map[string]any{"step": "collect"}}).Want(http.StatusOK)
	if row := loadIssueRow(t, child.ID); row.FlowTask.Valid || len(row.Flow) != 0 {
		t.Fatalf("the update took the flow columns: %+v", row)
	}
}

// A gate is never given to an agent that did work in the same round, even when
// the definition passed validation (it checks only the step before).
func TestGuard_GateSameAgent422(t *testing.T) {
	seedTestCatalog(t)
	ensureGoldenStatus(t, "collect", "started")
	ensureGoldenStatus(t, "gate", "started")
	worker := seededReadyAgentID(t)
	drafter := goldenLeadAgent(t, worker)
	agent := func(id string) map[string]any { return map[string]any{"type": "agent", "id": id} }
	cond := func(v string) map[string]any { return map[string]any{"field": "tasks_outcome", "op": "eq", "value": v} }
	body := map[string]any{
		"name":        "Same agent " + workflowTestSuffix(),
		"flow_config": map[string]any{"version": 1},
		"steps": []map[string]any{
			{"status_key": "todo", "transitions": []map[string]any{{"id": "default", "label": "Default", "when": nil, "to_status_key": "collect"}}},
			{"status_key": "collect", "tasks": []map[string]any{
				{"key": "read", "title": "Read", "kind": "work", "handler": agent(worker)},
			}, "transitions": []map[string]any{{"id": "default", "label": "Default", "when": nil, "to_status_key": "in_progress"}}},
			{"status_key": "in_progress", "tasks": []map[string]any{
				{"key": "draft", "title": "Draft", "kind": "work", "handler": agent(drafter)},
			}, "transitions": []map[string]any{{"id": "default", "label": "Default", "when": nil, "to_status_key": "gate"}}},
			{"status_key": "gate", "tasks": []map[string]any{
				{"key": "check", "title": "Check", "kind": "gate", "handler": agent(worker), "requires": []string{"outcome"}},
			}, "transitions": []map[string]any{
				{"id": "pass", "label": "Pass", "when": cond("all_approved"), "to_status_key": "done"},
				{"id": "default", "label": "Default", "when": nil, "to_status_key": "blocked"},
			}},
			{"status_key": "blocked"},
		},
	}
	wf := createFlowWorkflowForTest(t, body)
	squadID := dbfx.Squad(t, "Same agent squad "+workflowTestSuffix(), worker)
	dbfx.Exec(t, `UPDATE issue_workflow SET squad_id = $2 WHERE id = $1`, wf.ID, squadID)
	dbfx.Exec(t, `UPDATE squad SET active_workflow_id = $2 WHERE id = $1`, squadID, wf.ID)
	fx := tradeFlowFixture{Workflow: wf, SquadID: squadID}
	request := fx.newRequest(t)

	fx.plan(t, request, nil)
	finishTask(t, tasksAt(t, request, "collect", 0)["read"], worker, "")
	advance(t, request)
	finishTask(t, tasksAt(t, request, "in_progress", 0)["draft"], drafter, "")

	_, err := testHandler.AdvanceFlow(context.Background(), loadIssueRow(t, uuidToString(request.ID)))
	var rej *flowRejection
	if !asFlowRejection(err, &rej) || rej.Code != "flow_gate_same_agent" || rej.Status != http.StatusUnprocessableEntity {
		t.Fatalf("AdvanceFlow = %v, want flow_gate_same_agent", err)
	}
	if got := len(tasksAt(t, request, "gate", 0)); got != 0 {
		t.Fatalf("the gate opened for the agent that did the work: %d tasks", got)
	}
}

// projectWithPlainWorkflow is a project on a workflow that lists neither the
// flow's steps nor in_progress.
func projectWithPlainWorkflow(t *testing.T, label string) string {
	t.Helper()
	wf := createTestWorkflow(t, label+" "+workflowTestSuffix(), "in_review")
	projectID := createWorkflowTestProject(t, label+" project")
	setProjectWorkflow(t, projectID, map[string]any{"workflow_id": wf.ID}).Want(http.StatusOK)
	return projectID
}

func TestGuard_ChildTasksExemptFromProjectWorkflow(t *testing.T) {
	f := newTradeFlowFixture(t)
	projectID := projectWithPlainWorkflow(t, "Exempt")
	request := f.newRequest(t)
	dbfx.Exec(t, `UPDATE issue SET project_id = $2 WHERE id = $1`, request.ID, projectID)
	f.plan(t, request, nil)
	docs := tasksAt(t, request, "collect", 0)["docs"]
	if !loadIssueRow(t, docs).ProjectID.Valid {
		t.Fatal("the task did not inherit the project")
	}

	// in_progress is not in the project's workflow; a task takes it anyway.
	updateIssueForTest(t, docs, map[string]any{"status": "in_progress"}).Want(http.StatusOK)
	plain := dbfx.Issue(t, "plain issue in the project "+workflowTestSuffix(), testutil.Cols{"project_id": projectID, "status": "todo"})
	body := updateIssueForTest(t, plain, map[string]any{"status": "in_progress"}).Want(http.StatusBadRequest).Map()
	if body["code"] != "status_not_in_workflow" {
		t.Fatalf("a plain issue: code = %v, want status_not_in_workflow", body["code"])
	}
	// A manager's move of the request is checked against the flow's own steps.
	body = updateIssueForTest(t, uuidToString(request.ID), map[string]any{"status": "in_progress"}).Want(http.StatusBadRequest).Map()
	if body["code"] != "status_not_in_workflow" {
		t.Fatalf("a request moved outside its flow: code = %v, want status_not_in_workflow", body["code"])
	}
	updateIssueForTest(t, uuidToString(request.ID), map[string]any{"status": "draft"}).Want(http.StatusOK)
}

// A squad on a flow keeps routing its other issues through the project's workflow.
func TestGuard_PlainSquadIssueKeepsProjectWorkflow(t *testing.T) {
	f := newTradeFlowFixture(t)
	projectID := projectWithPlainWorkflow(t, "Keeps")
	dbfx.Exec(t, `UPDATE agent SET home_squad_id = $2 WHERE id = $1`, f.Agents.Docs, f.SquadID)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `UPDATE agent SET home_squad_id = NULL WHERE id = $1`, f.Agents.Docs)
	})

	issue := dbfx.Issue(t, "agent issue in the project "+workflowTestSuffix(), testutil.Cols{
		"project_id": projectID, "status": "todo", "assignee_type": "agent", "assignee_id": f.Agents.Docs,
	})
	// collect is a step of the flow, not of the project's workflow: refused.
	body := updateIssueForTest(t, issue, map[string]any{"status": "collect"}).Want(http.StatusBadRequest).Map()
	if body["code"] != "status_not_in_workflow" {
		t.Fatalf("code = %v, want the project's workflow to apply", body["code"])
	}
	updateIssueForTest(t, issue, map[string]any{"status": "in_review"}).Want(http.StatusOK)
}

func TestGuard_FlowEngineOffStepsAside(t *testing.T) {
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)
	f.plan(t, request, nil)
	id := uuidToString(request.ID)
	docs := tasksAt(t, request, "collect", 0)["docs"]
	withFeatureFlag(t, testHandler, "flow_engine", false)

	// With the engine off the request and its task are ordinary issues.
	agentSetStatus(t, id, f.Agents.Docs, "collect", "in_progress").Want(http.StatusOK)
	updateIssueForTest(t, docs, map[string]any{"status": "done"}).Want(http.StatusOK)
	if _, ok := flowRequestState(loadIssueRow(t, id)); !ok {
		t.Fatal("turning the engine off erased the stored flow state")
	}
}

func TestClaimFlowBrief_WithoutProject(t *testing.T) {
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)
	if request.ProjectID.Valid {
		t.Fatal("the fixture request has a project")
	}
	run := db.AgentTaskQueue{AgentID: parseUUID(f.Agents.Docs)}

	// The lead reads the plan brief on the request at the plan step.
	brief := claimBriefForTest(t, uuidToString(request.ID), run)
	for _, want := range []string{"multica issue flow plan", "customs_lookup", "Do not create sub-issues"} {
		if !strings.Contains(brief, want) {
			t.Fatalf("lead brief is missing %q:\n%s", want, brief)
		}
	}
	f.plan(t, request, nil)
	if after := claimBriefForTest(t, uuidToString(request.ID), run); !strings.Contains(after, "multica issue flow advance") || strings.Contains(after, "flow plan") {
		t.Fatalf("lead brief after the plan:\n%s", after)
	}

	// A task reads its own commands, with its own identifier.
	docs := tasksAt(t, request, "collect", 0)["docs"]
	taskBrief := claimBriefForTest(t, docs, db.AgentTaskQueue{AgentID: parseUUID(f.Agents.Docs)})
	identifier := fmt.Sprintf("-%d", loadIssueRow(t, docs).Number)
	for _, want := range []string{"the task \"docs\"", "multica issue status", identifier + " done", "blocked --outcome blocked"} {
		if !strings.Contains(taskBrief, want) {
			t.Fatalf("task brief is missing %q:\n%s", want, taskBrief)
		}
	}
	// A gate says how to give its verdict.
	finishTask(t, docs, f.Agents.Docs, "")
	advance(t, request)
	finishTask(t, tasksAt(t, request, "draft", 0)["crosscheck"], f.Agents.Drafter, "")
	advance(t, request)
	gate := tasksAt(t, request, "gate", 0)["docs_gate"]
	gateBrief := claimBriefForTest(t, gate, db.AgentTaskQueue{AgentID: parseUUID(f.Agents.DocsGate)})
	if !strings.Contains(gateBrief, "--outcome approved") || !strings.Contains(gateBrief, "--outcome rejected") || !strings.Contains(gateBrief, "multica attachment download") {
		t.Fatalf("gate brief:\n%s", gateBrief)
	}
}

func TestClaimBrief_NonFlowUnchanged(t *testing.T) {
	seedTestCatalog(t)
	resetDefaultWorkflow(t)
	agentID := seededReadyAgentID(t)
	projectID := deliveryProject(t, "Brief unchanged project", agentID)
	issueID := dbfx.Issue(t, "brief unchanged", testutil.Cols{
		"project_id": projectID, "status": "in_progress", "assignee_type": "agent", "assignee_id": agentID,
	})
	brief := claimBriefForTest(t, issueID, db.AgentTaskQueue{AgentID: parseUUID(agentID)})
	if !strings.Contains(brief, "This project's workflow — Default") || strings.Contains(brief, "multica issue flow") {
		t.Fatalf("a plain issue reads the project's brief:\n%s", brief)
	}
}
