package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// API tests for workflows that carry a flow_config (S4 of
// docs/engineering/kitviet-workflow-plan.md). A flow workflow is created by a
// workspace admin; older clients that edit or list it must not lose its fields.

// flowWorkflowBody is a small valid flow: a plan step, one task step with a
// task, a human-only step and the blocked step.
func flowWorkflowBody(name, agentID string, extra map[string]any) map[string]any {
	body := map[string]any{
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
					"key": "work", "title": "Work", "kind": "work", "instructions": "Do it.",
					"handler": map[string]any{"type": "agent", "id": agentID},
				}},
				"transitions": []map[string]any{
					{"id": "blocked", "label": "Blocked", "when": map[string]any{"field": "tasks_outcome", "op": "eq", "value": "any_blocked"}, "to_status_key": "blocked"},
					{"id": "default", "label": "Default", "when": nil, "to_status_key": "in_review"},
				}},
			{"status_key": "in_review", "human_only": true},
			{"status_key": "blocked"},
		},
	}
	for k, v := range extra {
		body[k] = v
	}
	return body
}

func createFlowWorkflowForTest(t *testing.T, body map[string]any) IssueWorkflowResponse {
	t.Helper()
	var created IssueWorkflowResponse
	testutil.Call(t, testHandler.CreateIssueWorkflow, workflowRequest("POST", "/api/issue-workflows", body)).
		Want(http.StatusCreated).JSON(&created)
	t.Cleanup(func() {
		ctx := context.Background()
		testPool.Exec(ctx, `UPDATE squad SET active_workflow_id = NULL WHERE active_workflow_id = $1`, created.ID)
		testPool.Exec(ctx, `DELETE FROM issue_workflow WHERE id = $1`, created.ID)
	})
	return created
}

func TestCreateIssueWorkflow_FlowConfigRoundTrips(t *testing.T) {
	seedTestCatalog(t)
	agentID := seededReadyAgentID(t)
	created := createFlowWorkflowForTest(t, flowWorkflowBody("Flow create "+workflowTestSuffix(), agentID, map[string]any{"template_ref": "flow_test@abc12345"}))

	if created.FlowConfig == nil || len(created.FlowConfig.Switches) != 1 {
		t.Fatalf("flow_config = %+v, want the switch back", created.FlowConfig)
	}
	if created.TemplateRef == nil || *created.TemplateRef != "flow_test@abc12345" {
		t.Fatalf("template_ref = %v", created.TemplateRef)
	}
	if created.FlowAuto {
		t.Fatal("a new flow workflow has flow_auto on")
	}
	work := findWorkflowStep(t, created, "in_progress")
	if len(work.Tasks) != 1 || work.Tasks[0].Key != "work" || work.Tasks[0].Handler.ID != agentID {
		t.Fatalf("tasks = %+v", work.Tasks)
	}
	if !findWorkflowStep(t, created, "in_review").HumanOnly {
		t.Fatal("human_only was lost")
	}
	plain := createTestWorkflow(t, "Plain "+workflowTestSuffix(), "in_progress")
	if len(findWorkflowStep(t, plain, "in_progress").Tasks) != 0 || plain.FlowConfig != nil || plain.TemplateRef != nil {
		t.Fatalf("a plain workflow carries flow fields: %+v", plain)
	}
}

func TestCreateIssueWorkflow_FlowConfigNeedsAdmin(t *testing.T) {
	seedTestCatalog(t)
	agentID := seededReadyAgentID(t)
	_, asMember := addWorkflowTestMember(t, "flow-member")

	for name, body := range map[string]map[string]any{
		"flow_config":  flowWorkflowBody("Flow member "+workflowTestSuffix(), agentID, nil),
		"template_ref": {"name": "Ref member " + workflowTestSuffix(), "template_ref": "flow_x@1", "steps": statusSteps("backlog", "blocked")},
		"tasks": {"name": "Tasks member " + workflowTestSuffix(), "steps": []map[string]any{
			{"status_key": "in_progress", "tasks": []map[string]any{{"key": "a", "title": "a", "kind": "work", "handler": map[string]any{"type": "agent", "id": agentID}}}},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			got := testutil.Call(t, testHandler.CreateIssueWorkflow, asMember(workflowRequest("POST", "/api/issue-workflows", body))).
				Want(http.StatusForbidden).Map()
			if got["code"] != "flow_workflow_forbidden" {
				t.Fatalf("code = %v, want flow_workflow_forbidden", got["code"])
			}
		})
	}
}

func TestCreateIssueWorkflow_FlowFieldsNeedAValidFlow(t *testing.T) {
	seedTestCatalog(t)
	agentID := seededReadyAgentID(t)
	tests := map[string]map[string]any{
		"template_ref without flow_config": {"name": "No config " + workflowTestSuffix(), "template_ref": "flow_x@1", "steps": statusSteps("backlog", "blocked")},
		"tasks without flow_config": {"name": "No config tasks " + workflowTestSuffix(), "steps": []map[string]any{
			{"status_key": "in_progress", "tasks": []map[string]any{{"key": "a", "title": "a", "kind": "work", "handler": map[string]any{"type": "agent", "id": agentID}}}},
		}},
		"flow_config with no task": flowWorkflowBody("No task "+workflowTestSuffix(), agentID, map[string]any{"steps": statusSteps("backlog", "in_progress", "blocked")}),
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			got := testutil.Call(t, testHandler.CreateIssueWorkflow, workflowRequest("POST", "/api/issue-workflows", body)).
				Want(http.StatusBadRequest).Map()
			if got["code"] != "invalid_workflow" {
				t.Fatalf("code = %v, want invalid_workflow", got["code"])
			}
		})
	}
}

// A client that predates tasks edits a workflow from what it read: it sends no
// tasks, human_only or flow_config, and they must stay as stored.
func TestUpdateIssueWorkflow_OldClientKeepsTasks(t *testing.T) {
	seedTestCatalog(t)
	agentID := seededReadyAgentID(t)
	created := createFlowWorkflowForTest(t, flowWorkflowBody("Flow old client "+workflowTestSuffix(), agentID, nil))

	raw, err := json.Marshal(created.Steps)
	if err != nil {
		t.Fatal(err)
	}
	var steps []map[string]any
	if err := json.Unmarshal(raw, &steps); err != nil {
		t.Fatal(err)
	}
	for _, s := range steps {
		delete(s, "tasks")
		delete(s, "human_only")
	}
	renamed := "Flow old client renamed " + workflowTestSuffix()
	var updated IssueWorkflowResponse
	testutil.Call(t, testHandler.UpdateIssueWorkflow,
		withURLParam(workflowRequest("PATCH", "/api/issue-workflows/"+created.ID, map[string]any{"name": renamed, "steps": steps}), "id", created.ID)).
		Want(http.StatusOK).JSON(&updated)

	if updated.Name != renamed {
		t.Fatalf("name = %q", updated.Name)
	}
	work := findWorkflowStep(t, updated, "in_progress")
	if len(work.Tasks) != 1 || work.Tasks[0].Key != "work" {
		t.Fatalf("tasks after an old-client write = %+v", work.Tasks)
	}
	if !findWorkflowStep(t, updated, "in_review").HumanOnly {
		t.Fatal("human_only was dropped by an old-client write")
	}
	if updated.FlowConfig == nil || len(updated.FlowConfig.Switches) != 1 {
		t.Fatalf("flow_config after an old-client write = %+v", updated.FlowConfig)
	}
	var stored int
	dbfx.QueryRow(t, `SELECT count(*) FROM issue_workflow WHERE id = $1 AND flow_config IS NOT NULL`, created.ID).Scan(&stored)
	if stored != 1 {
		t.Fatal("flow_config is not stored after the write")
	}

	// A write that sends tasks sets them, an empty list included.
	steps = steps[:0]
	raw, _ = json.Marshal(created.Steps)
	_ = json.Unmarshal(raw, &steps)
	for _, s := range steps {
		if s["status_key"] == "in_review" {
			s["human_only"] = false
		}
	}
	testutil.Call(t, testHandler.UpdateIssueWorkflow,
		withURLParam(workflowRequest("PATCH", "/api/issue-workflows/"+created.ID, map[string]any{"steps": steps}), "id", created.ID)).
		Want(http.StatusOK).JSON(&updated)
	if findWorkflowStep(t, updated, "in_review").HumanOnly {
		t.Fatal("human_only stayed on after the write turned it off")
	}
}

func TestUpdateIssueWorkflow_TemplateManagedIs409(t *testing.T) {
	seedTestCatalog(t)
	agentID := seededReadyAgentID(t)
	created := createFlowWorkflowForTest(t, flowWorkflowBody("Flow managed "+workflowTestSuffix(), agentID, map[string]any{"template_ref": "flow_test@abc12345"}))

	patch := testutil.Call(t, testHandler.UpdateIssueWorkflow,
		withURLParam(workflowRequest("PATCH", "/api/issue-workflows/"+created.ID, map[string]any{"name": "Taken " + workflowTestSuffix()}), "id", created.ID)).
		Want(http.StatusConflict).Map()
	if patch["code"] != "workflow_managed_by_template" {
		t.Fatalf("PATCH code = %v, want workflow_managed_by_template", patch["code"])
	}
	handoff := testutil.Call(t, testHandler.SetIssueWorkflowHandoff,
		withURLParams(workflowRequest("PUT", "/api/issue-workflows/"+created.ID+"/handoffs/in_progress", handToAgent(agentID, "")),
			"id", created.ID, "status", "in_progress")).Want(http.StatusConflict).Map()
	if handoff["code"] != "workflow_managed_by_template" {
		t.Fatalf("handoff code = %v, want workflow_managed_by_template", handoff["code"])
	}
	var name string
	dbfx.QueryRow(t, `SELECT name FROM issue_workflow WHERE id = $1`, created.ID).Scan(&name)
	if name != created.Name {
		t.Fatalf("a refused write renamed the workflow to %q", name)
	}
}

func TestDeleteIssueWorkflow_BlockedByOpenFlowRequests(t *testing.T) {
	seedTestCatalog(t)
	agentID := seededReadyAgentID(t)
	created := createFlowWorkflowForTest(t, flowWorkflowBody("Flow delete "+workflowTestSuffix(), agentID, nil))
	open := dbfx.Issue(t, "open flow request", testutil.Cols{
		"flow": testutil.Raw(fmt.Sprintf(`'{"workflow_id":"%s","step":"todo","rework":0}'::jsonb`, created.ID)),
	})

	remove := func() *testutil.Response {
		return testutil.Call(t, testHandler.DeleteIssueWorkflow,
			withURLParam(workflowRequest("DELETE", "/api/issue-workflows/"+created.ID, nil), "id", created.ID))
	}
	body := remove().Want(http.StatusConflict).Map()
	if body["code"] != "workflow_has_flow_requests" {
		t.Fatalf("code = %v, want workflow_has_flow_requests", body["code"])
	}
	reasons, _ := body["reasons"].([]any)
	found := false
	for _, reason := range reasons {
		r, _ := reason.(map[string]any)
		if r["code"] != "workflow_has_flow_requests" {
			continue
		}
		found = true
		ids, _ := r["ids"].([]any)
		if r["count"] != float64(1) || len(ids) != 1 || ids[0] != open {
			t.Fatalf("reason = %v, want count 1 and the open request", r)
		}
	}
	if !found {
		t.Fatalf("reasons = %v, want workflow_has_flow_requests", reasons)
	}

	// A closed request no longer blocks.
	dbfx.Exec(t, `UPDATE issue SET flow = flow || '{"closed_at":"2026-01-01T00:00:00Z"}'::jsonb WHERE id = $1`, open)
	remove().Want(http.StatusNoContent)
}

func TestIssueWorkflowResponse_CountsOpenFlowRequests(t *testing.T) {
	seedTestCatalog(t)
	agentID := seededReadyAgentID(t)
	created := createFlowWorkflowForTest(t, flowWorkflowBody("Flow count "+workflowTestSuffix(), agentID, nil))
	for i := 0; i < 2; i++ {
		dbfx.Issue(t, fmt.Sprintf("open %d", i), testutil.Cols{
			"flow": testutil.Raw(fmt.Sprintf(`'{"workflow_id":"%s","step":"todo","rework":0}'::jsonb`, created.ID)),
		})
	}
	dbfx.Issue(t, "closed", testutil.Cols{
		"flow": testutil.Raw(fmt.Sprintf(`'{"workflow_id":"%s","step":"in_review","rework":0,"closed_at":"2026-01-01T00:00:00Z"}'::jsonb`, created.ID)),
	})

	var one IssueWorkflowResponse
	testutil.Call(t, testHandler.GetIssueWorkflow, withURLParam(workflowRequest("GET", "/api/issue-workflows/"+created.ID, nil), "id", created.ID)).
		Want(http.StatusOK).JSON(&one)
	if one.OpenFlowRequests != 2 {
		t.Fatalf("open_flow_requests = %d, want 2", one.OpenFlowRequests)
	}
	var list struct {
		Workflows []IssueWorkflowResponse `json:"workflows"`
	}
	testutil.Call(t, testHandler.ListIssueWorkflows, workflowRequest("GET", "/api/issue-workflows", nil)).Want(http.StatusOK).JSON(&list)
	for _, wf := range list.Workflows {
		if wf.ID == created.ID && wf.OpenFlowRequests != 2 {
			t.Fatalf("list open_flow_requests = %d, want 2", wf.OpenFlowRequests)
		}
		if wf.ID != created.ID && wf.IsDefault && wf.OpenFlowRequests != 0 {
			t.Fatalf("Default counts %d open requests", wf.OpenFlowRequests)
		}
	}
}

func TestSquadResponse_WorkflowFields(t *testing.T) {
	seedTestCatalog(t)
	agentID := seededReadyAgentID(t)
	squadID := dbfx.Squad(t, "Workflow fields "+workflowTestSuffix(), agentID)
	get := func() SquadResponse {
		var out SquadResponse
		testutil.Call(t, testHandler.GetSquad, squadScopeReq("", "GET", "/api/squads/"+squadID, nil, map[string]string{"id": squadID})).
			Want(http.StatusOK).JSON(&out)
		return out
	}

	none := get()
	if none.ActiveWorkflowID != nil || none.ActiveWorkflowName != nil || none.WorkflowCount != 0 || none.FlowEnforced {
		t.Fatalf("a squad with no workflow: %+v", none)
	}

	plain := insertSquadWorkflow(t, testWorkspaceID, squadID, "Library plain "+workflowTestSuffix(), "backlog", "in_progress", "blocked")
	dbfx.Exec(t, `UPDATE squad SET active_workflow_id = $2 WHERE id = $1`, squadID, plain)
	got := get()
	if got.ActiveWorkflowID == nil || *got.ActiveWorkflowID != plain || got.ActiveWorkflowName == nil || got.WorkflowCount != 1 || got.FlowEnforced {
		t.Fatalf("a squad on a plain workflow: %+v", got)
	}

	flow := createFlowWorkflowForTest(t, flowWorkflowBody("Library flow "+workflowTestSuffix(), agentID, nil))
	dbfx.Exec(t, `UPDATE issue_workflow SET squad_id = $2 WHERE id = $1`, flow.ID, squadID)
	dbfx.Exec(t, `UPDATE squad SET active_workflow_id = $2 WHERE id = $1`, squadID, flow.ID)
	got = get()
	if got.ActiveWorkflowID == nil || *got.ActiveWorkflowID != flow.ID || got.WorkflowCount != 2 || !got.FlowEnforced {
		t.Fatalf("a squad on a flow workflow: %+v", got)
	}
	if got.ActiveWorkflowName == nil || *got.ActiveWorkflowName != flow.Name {
		t.Fatalf("active_workflow_name = %v, want %q", got.ActiveWorkflowName, flow.Name)
	}

	var list []SquadResponse
	testutil.Call(t, testHandler.ListSquads, squadScopeReq("", "GET", "/api/squads", nil, nil)).Want(http.StatusOK).JSON(&list)
	listed := false
	for _, s := range list {
		if s.ID == squadID {
			listed = true
			if s.WorkflowCount != 2 || !s.FlowEnforced || s.ActiveWorkflowID == nil || *s.ActiveWorkflowID != flow.ID {
				t.Fatalf("the squad list shows %+v", s)
			}
		}
	}
	if !listed {
		t.Fatal("the squad is not in the list")
	}
}

// A flow workflow cannot be copied into a squad's library: the copy would
// carry the steps without the flow_config that makes them mean anything.
func TestCreateSquadWorkflow_FlowWorkflowCannotBeCopied(t *testing.T) {
	seedTestCatalog(t)
	agentID := seededReadyAgentID(t)
	squadID := dbfx.Squad(t, "Copy flow "+workflowTestSuffix(), agentID)
	flow := createFlowWorkflowForTest(t, flowWorkflowBody("Copy source "+workflowTestSuffix(), agentID, nil))

	got := testutil.Call(t, testHandler.CreateSquadWorkflow, squadScopeReq("", "POST", "/api/squads/"+squadID+"/workflows",
		map[string]any{"name": "Copy " + workflowTestSuffix(), "source_workflow_id": flow.ID}, map[string]string{"id": squadID})).
		Want(http.StatusConflict).Map()
	if got["code"] != "workflow_managed_by_template" {
		t.Fatalf("code = %v, want workflow_managed_by_template", got["code"])
	}
}
