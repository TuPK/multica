package handler

import (
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/issuestatus"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// Routing transitions, usage and delete reasons (docs/engineering/workflow-tab.md
// §3 and §4). Each test names the behavior it pins.

// urgentOrRejectedSteps is a review step with a conditional branch, a
// rejected branch and a default, the shape the legacy keys cannot express.
func urgentOrRejectedSteps() []map[string]any {
	return []map[string]any{
		{"status_key": "in_progress"},
		{"status_key": "blocked"},
		{"status_key": "in_review", "transitions": []map[string]any{
			{"id": "urgent", "label": "Urgent", "when": map[string]any{"field": "priority", "op": "gte", "value": "urgent"}, "to_status_key": "blocked"},
			{"id": "rejected", "label": "Rejected", "when": map[string]any{"field": "outcome", "op": "eq", "value": "rejected"}, "to_status_key": "in_progress"},
			{"id": "default", "label": "Default", "when": nil, "to_status_key": "done"},
		}},
	}
}

func findWorkflowStep(t *testing.T, workflow IssueWorkflowResponse, statusKey string) IssueWorkflowStepPayload {
	t.Helper()
	for _, s := range workflow.Steps {
		if s.StatusKey == statusKey {
			return s
		}
	}
	t.Fatalf("workflow %s has no step %q", workflow.ID, statusKey)
	return IssueWorkflowStepPayload{}
}

func TestWorkflowTransitionsRoundTripWithDerivedLegacyKeys(t *testing.T) {
	seedTestCatalog(t)
	created := createTestWorkflowSteps(t, "Routing round trip "+workflowTestSuffix(), urgentOrRejectedSteps())

	step := findWorkflowStep(t, created, "in_review")
	if len(step.Transitions) != 3 || step.Transitions[0].ID != "urgent" || step.Transitions[0].Label != "Urgent" {
		t.Fatalf("transitions = %+v, want the three stored branches in order", step.Transitions)
	}
	if step.Transitions[2].When != nil {
		t.Fatalf("default branch has a condition: %+v", step.Transitions[2].When)
	}
	if step.NextStatusKey != "done" || step.BackStatusKey != "in_progress" {
		t.Fatalf("derived next/back = %q/%q, want done/in_progress", step.NextStatusKey, step.BackStatusKey)
	}
}

// A client that predates transitions writes next and back keys. They convert
// to the same three branches the 977 migration produces.
func TestLegacyNextBackKeysConvertToTransitions(t *testing.T) {
	seedTestCatalog(t)
	created := createTestWorkflowSteps(t, "Legacy routing "+workflowTestSuffix(), []map[string]any{
		{"status_key": "in_progress"},
		{"status_key": "in_review", "next_status_key": "done", "back_status_key": "in_progress"},
	})

	step := findWorkflowStep(t, created, "in_review")
	var ids []string
	for _, tr := range step.Transitions {
		ids = append(ids, tr.ID)
	}
	if len(ids) != 3 || ids[0] != "approved" || ids[1] != "rejected" || ids[2] != "default" {
		t.Fatalf("transition ids = %v, want approved, rejected, default", ids)
	}
	if step.NextStatusKey != "done" || step.BackStatusKey != "in_progress" {
		t.Fatalf("derived next/back = %q/%q", step.NextStatusKey, step.BackStatusKey)
	}
}

// An edit that repeats the legacy keys a stored step already derives keeps
// the stored branches, so a conditional branch an old client cannot see
// survives its save.
func TestLegacyEditKeepsStoredTransitionsWhenRoutingUnchanged(t *testing.T) {
	seedTestCatalog(t)
	created := createTestWorkflowSteps(t, "Edit keeps routing "+workflowTestSuffix(), urgentOrRejectedSteps())

	var edited IssueWorkflowResponse
	req := withURLParams(workflowRequest("PATCH", "/api/issue-workflows/"+created.ID, map[string]any{
		"steps": []map[string]any{
			{"status_key": "in_progress"},
			{"status_key": "blocked"},
			{"status_key": "in_review", "next_status_key": "done", "back_status_key": "in_progress"},
		},
	}), "id", created.ID)
	testutil.Call(t, testHandler.UpdateIssueWorkflow, req).Want(http.StatusOK).JSON(&edited)

	step := findWorkflowStep(t, edited, "in_review")
	if len(step.Transitions) != 3 || step.Transitions[0].ID != "urgent" {
		t.Fatalf("transitions after legacy edit = %+v, want the stored urgent branch kept", step.Transitions)
	}
}

func TestWorkflowUsageListsProjectsAndAgents(t *testing.T) {
	seedTestCatalog(t)
	agentID := seededReadyAgentID(t)
	created := createTestWorkflowSteps(t, "Usage "+workflowTestSuffix(), []map[string]any{
		{"status_key": "in_progress", "handler": map[string]any{"type": "agent", "id": agentID}, "instructions": "Build it."},
	})
	projectID := createWorkflowTestProject(t, "Usage project "+workflowTestSuffix())
	setProjectWorkflow(t, projectID, map[string]any{"workflow_id": created.ID}).Want(http.StatusOK)

	var out struct {
		Usage []IssueWorkflowUsage `json:"usage"`
	}
	testutil.Call(t, testHandler.ListIssueWorkflowUsage, workflowRequest("GET", "/api/issue-workflows/usage", nil)).Want(http.StatusOK).JSON(&out)

	var found *IssueWorkflowUsage
	for i := range out.Usage {
		if out.Usage[i].WorkflowID == created.ID {
			found = &out.Usage[i]
		}
	}
	if found == nil {
		t.Fatalf("usage has no entry for workflow %s: %+v", created.ID, out.Usage)
	}
	if !workflowListHas(found.ProjectIDs, projectID) {
		t.Fatalf("project_ids = %v, want %s", found.ProjectIDs, projectID)
	}
	if !workflowListHas(found.AgentIDs, agentID) {
		t.Fatalf("agent_ids = %v, want the step's agent %s", found.AgentIDs, agentID)
	}
	if found.SquadIDs == nil {
		t.Fatalf("squad_ids is null, want an empty list")
	}
}

// Deleting a workflow a project uses is refused with the blocking reasons in
// the body, each with its ids.
func TestDeleteWorkflowReportsBlockingReasons(t *testing.T) {
	seedTestCatalog(t)
	created := createTestWorkflow(t, "Blocked delete "+workflowTestSuffix(), "in_review")
	projectID := createWorkflowTestProject(t, "Blocked delete project "+workflowTestSuffix())
	setProjectWorkflow(t, projectID, map[string]any{"workflow_id": created.ID}).Want(http.StatusOK)

	body := testutil.Call(t, testHandler.DeleteIssueWorkflow, withURLParams(workflowRequest("DELETE", "/api/issue-workflows/"+created.ID, nil), "id", created.ID)).
		Want(http.StatusConflict).Map()
	if body["code"] != "workflow_in_use" {
		t.Fatalf("code = %v, want workflow_in_use", body["code"])
	}
	reasons, ok := body["reasons"].([]any)
	if !ok || len(reasons) != 1 {
		t.Fatalf("reasons = %v, want one reason", body["reasons"])
	}
	reason := reasons[0].(map[string]any)
	if reason["code"] != "workflow_in_use" || reason["count"] != float64(1) {
		t.Fatalf("reason = %v", reason)
	}
	if ids, _ := reason["ids"].([]any); len(ids) != 1 || ids[0] != projectID {
		t.Fatalf("reason ids = %v, want [%s]", reason["ids"], projectID)
	}
}

// An outcome the step routes must be followed by the routed status. A status
// that contradicts it is refused, and the issue stays where it was.
func TestOutcomeRouteMustMatchTheStatusWritten(t *testing.T) {
	seedTestCatalog(t)
	created := createTestWorkflowSteps(t, "Outcome routing "+workflowTestSuffix(), urgentOrRejectedSteps())
	projectID := createWorkflowTestProject(t, "Outcome project "+workflowTestSuffix())
	setProjectWorkflow(t, projectID, map[string]any{"workflow_id": created.ID}).Want(http.StatusOK)
	issueID := dbfx.Issue(t, "route me "+workflowTestSuffix(), testutil.Cols{"project_id": projectID, "status": issuestatus.InReview})

	refused := updateIssueForTest(t, issueID, map[string]any{"status": "in_progress", "outcome": "approved"}).
		Want(http.StatusBadRequest).Map()
	if refused["code"] != "workflow_outcome_mismatch" || refused["expected_status"] != "done" {
		t.Fatalf("refusal = %v, want workflow_outcome_mismatch expecting done", refused)
	}
	if status, _, _ := issueStatusAndAssignee(t, issueID); status != issuestatus.InReview {
		t.Fatalf("refused write moved the issue to %q", status)
	}

	updateIssueForTest(t, issueID, map[string]any{"status": "in_progress", "outcome": "rejected"}).Want(http.StatusOK)
	if status, _, _ := issueStatusAndAssignee(t, issueID); status != issuestatus.InProgress {
		t.Fatalf("status = %q, want the rejected branch in_progress", status)
	}
}

func TestOutcomeMustBeAKnownValue(t *testing.T) {
	seedTestCatalog(t)
	issueID := dbfx.Issue(t, "bad outcome "+workflowTestSuffix(), testutil.Cols{"status": "todo"})
	body := updateIssueForTest(t, issueID, map[string]any{"status": "in_progress", "outcome": "maybe"}).
		Want(http.StatusBadRequest).Map()
	if body["code"] != "invalid_outcome" {
		t.Fatalf("code = %v, want invalid_outcome", body["code"])
	}
}

func workflowListHas(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
