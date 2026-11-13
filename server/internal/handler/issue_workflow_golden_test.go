package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/catalog"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// Golden tests for the workflows that predate the enforced squad workflow
// engine (docs/engineering/kitviet-workflow-plan.md, S0.3). SDLC Auto and BMAD
// Planning Auto have no flow_config; every later step of the engine must leave
// these writes exactly as they are. The workflows are built from the catalog
// manifests the squad quickstart uses, so the steps, instructions and routing
// are the shipped ones. Only the statuses a test needs are listed, and every
// role is bound to a test agent.

type goldenCatalogStep struct {
	StatusKey    string  `json:"statusKey"`
	Role         *string `json:"role"`
	OptionalRole bool    `json:"optionalRole"`
	Instructions string  `json:"instructions"`
	Next         string  `json:"next"`
	Back         string  `json:"back"`
	ImplicitNext bool    `json:"implicitNext"`
}

type goldenCatalogWorkflow struct {
	Name  string              `json:"name"`
	Steps []goldenCatalogStep `json:"steps"`
}

// goldenWorkflowSteps turns the listed steps of a catalog workflow into the
// body of a workflow write, binding each role to the agent given for it.
func goldenWorkflowSteps(t *testing.T, id string, keep []string, agents map[string]string) []map[string]any {
	t.Helper()
	raw := catalog.WorkflowsJSON()
	if len(raw) == 0 {
		t.Fatalf("catalog workflows: %s", catalog.BuildHint)
	}
	var all map[string]goldenCatalogWorkflow
	if err := json.Unmarshal(raw, &all); err != nil {
		t.Fatalf("parse catalog workflows: %v", err)
	}
	wf, ok := all[id]
	if !ok {
		t.Fatalf("catalog has no workflow %q", id)
	}
	wanted := map[string]bool{}
	for _, key := range keep {
		wanted[key] = true
	}
	var steps []map[string]any
	for _, st := range wf.Steps {
		if !wanted[st.StatusKey] {
			continue
		}
		step := map[string]any{"status_key": st.StatusKey, "instructions": st.Instructions}
		if st.Role != nil {
			agentID, has := agents[*st.Role]
			if !has {
				t.Fatalf("workflow %q step %q: no test agent for role %q", id, st.StatusKey, *st.Role)
			}
			step["handler"] = map[string]any{"type": "agent", "id": agentID}
		}
		if st.Next != "" && !st.ImplicitNext {
			step["next_status_key"] = st.Next
		}
		if st.Back != "" {
			step["back_status_key"] = st.Back
		}
		steps = append(steps, step)
	}
	if len(steps) != len(keep) {
		t.Fatalf("workflow %q: found %d of the %d wanted steps", id, len(steps), len(keep))
	}
	return steps
}

// ensureGoldenStatus makes a catalog status the workflow lists exist in the
// shared test workspace, and removes it again when the test made it.
func ensureGoldenStatus(t *testing.T, key, category string) {
	t.Helper()
	seedTestCatalog(t)
	var n int
	dbfx.QueryRow(t, `SELECT count(*) FROM issue_status WHERE workspace_id = $1 AND key = $2`, testWorkspaceID, key).Scan(&n)
	if n == 0 {
		createTestCustomStatus(t, key, category)
	}
}

// goldenLeadAgent is a second ready agent, so the workflow's blocked step has
// a handler that is not the step handlers.
func goldenLeadAgent(t *testing.T, likeAgentID string) string {
	t.Helper()
	var runtimeID string
	dbfx.QueryRow(t, `SELECT runtime_id::text FROM agent WHERE id = $1`, likeAgentID).Scan(&runtimeID)
	id := dbfx.Agent(t, "Golden lead "+workflowTestSuffix(), runtimeID)
	// Open to the whole workspace: an agent's run, which has no human originator, may hand work to
	// it. A private agent is refused.
	dbfx.Exec(t, `UPDATE agent SET visibility = 'workspace', permission_mode = 'public_to' WHERE id = $1`, id)
	dbfx.Insert(t, "agent_invocation_target", testutil.Cols{"agent_id": id, "target_type": "workspace", "target_id": testWorkspaceID})
	return id
}

// goldenRun makes a running run of the agent on the issue, bound to the step.
func goldenRun(t *testing.T, issueID, agentID, step string) string {
	t.Helper()
	var runtimeID string
	dbfx.QueryRow(t, `SELECT runtime_id::text FROM agent WHERE id = $1`, agentID).Scan(&runtimeID)
	return dbfx.Task(t, agentID, testutil.Cols{
		"runtime_id": runtimeID, "issue_id": issueID, "status": "running",
		"started_at": time.Now(), "workflow_step": step,
	})
}

func goldenWantStatus(t *testing.T, issueID, wantStatus, wantAssignee string) {
	t.Helper()
	status, _, assigneeID := issueStatusAndAssignee(t, issueID)
	if status != wantStatus {
		t.Fatalf("status = %q, want %q", status, wantStatus)
	}
	if wantAssignee != "" && assigneeID != wantAssignee {
		t.Fatalf("assignee = %q, want %q", assigneeID, wantAssignee)
	}
}

func TestWorkflowGolden_SDLCAutoWrites(t *testing.T) {
	seedTestCatalog(t)
	resetDefaultWorkflow(t)
	ensureGoldenStatus(t, "qa", "started")
	devID := seededReadyAgentID(t)
	leadID := goldenLeadAgent(t, devID)
	steps := goldenWorkflowSteps(t, "sdlc_auto",
		[]string{"backlog", "todo", "in_progress", "in_review", "qa", "blocked"},
		map[string]string{"developer": devID, "reviewer": devID, "qa": devID, "lead": leadID})
	// The catalog workflow routes with next/back only. The shipped designer template of SDLC Auto
	// routes a rejected review back to In Progress and anything else on to QA; give the review step
	// that routing so the outcome-versus-status rule is exercised.
	for _, step := range steps {
		if step["status_key"] == "in_review" {
			step["transitions"] = []map[string]any{
				{"id": "rejected", "label": "Rejected", "when": map[string]any{"field": "outcome", "op": "eq", "value": "rejected"}, "to_status_key": "in_progress"},
				{"id": "default", "label": "Approved", "when": nil, "to_status_key": "qa"},
			}
		}
	}
	wf := createTestWorkflowSteps(t, "Golden SDLC Auto "+workflowTestSuffix(), steps)
	projectID := createWorkflowTestProject(t, "Golden SDLC project")
	setProjectWorkflow(t, projectID, map[string]any{"workflow_id": wf.ID}).Want(http.StatusOK)

	// To Do -> In Progress hands the issue to the developer and queues a run.
	issueID := dbfx.Issue(t, "golden sdlc", testutil.Cols{"project_id": projectID, "status": "todo"})
	updateIssueForTest(t, issueID, map[string]any{"status": "in_progress"}).Want(http.StatusOK)
	goldenWantStatus(t, issueID, "in_progress", devID)
	if n := taskCountFor(t, issueID, devID); n != 1 {
		t.Fatalf("runs queued by the developer handoff = %d, want 1", n)
	}
	var taskID string
	dbfx.QueryRow(t, `SELECT id::text FROM agent_task_queue WHERE issue_id = $1 AND agent_id = $2`, issueID, devID).Scan(&taskID)
	if got := taskWorkflowStep(t, taskID); got != "in_progress" {
		t.Fatalf("handoff run step = %q, want in_progress", got)
	}

	// A status the workflow does not list is refused.
	createTestCustomStatus(t, "golden_unlisted", "started")
	refused := updateIssueForTest(t, issueID, map[string]any{"status": "golden_unlisted"}).Want(http.StatusBadRequest).Map()
	if refused["code"] != "status_not_in_workflow" {
		t.Fatalf("code = %v, want status_not_in_workflow", refused["code"])
	}
	goldenWantStatus(t, issueID, "in_progress", "")

	// An agent run does not skip the steps after its own to done.
	skipped := agentUpdateIssue(t, issueID, devID, taskID, map[string]any{"status": "done"}).Want(http.StatusConflict).Map()
	if skipped["code"] != "workflow_step_skipped" || skipped["next_status"] != "in_review" {
		t.Fatalf("refusal = %v, want workflow_step_skipped with next_status in_review", skipped)
	}
	goldenWantStatus(t, issueID, "in_progress", "")

	// The run's own move to review is allowed.
	agentUpdateIssue(t, issueID, devID, taskID, map[string]any{"status": "in_review"}).Want(http.StatusOK)
	goldenWantStatus(t, issueID, "in_review", devID)

	// Review routes approved to QA; a status that contradicts it is refused.
	mismatch := agentUpdateIssue(t, issueID, devID, taskID, map[string]any{"status": "in_progress", "outcome": "approved"}).
		Want(http.StatusBadRequest).Map()
	if mismatch["code"] != "workflow_outcome_mismatch" || mismatch["expected_status"] != "qa" {
		t.Fatalf("refusal = %v, want workflow_outcome_mismatch expecting qa", mismatch)
	}
	goldenWantStatus(t, issueID, "in_review", "")
	agentUpdateIssue(t, issueID, devID, taskID, map[string]any{"status": "qa", "outcome": "approved"}).Want(http.StatusOK)
	goldenWantStatus(t, issueID, "qa", devID)

	// A member moves the issue while the run works: the run's decision is refused.
	updateIssueForTest(t, issueID, map[string]any{"status": "todo"}).Want(http.StatusOK)
	goldenWantStatus(t, issueID, "todo", "")
	moved := agentUpdateIssue(t, issueID, devID, taskID, map[string]any{"status": "done"}).Want(http.StatusConflict).Map()
	if moved["code"] != "workflow_step_moved" || moved["step"] != "qa" || moved["current_status"] != "todo" {
		t.Fatalf("refusal = %v, want workflow_step_moved from qa to todo", moved)
	}
	goldenWantStatus(t, issueID, "todo", "")

	// Three returns from review: the first two go back to In Progress, the
	// third is turned into a block handed to the lead.
	returned := dbfx.Issue(t, "golden sdlc rework", testutil.Cols{
		"project_id": projectID, "status": "in_review", "assignee_type": "agent", "assignee_id": devID,
	})
	for round := 1; round <= 3; round++ {
		dbfx.Exec(t, `UPDATE issue SET status = 'in_review', assignee_type = 'agent', assignee_id = $2 WHERE id = $1`, returned, devID)
		run := goldenRun(t, returned, devID, "in_review")
		agentUpdateIssue(t, returned, devID, run, map[string]any{"status": "in_progress", "outcome": "rejected"}).Want(http.StatusOK)
		if round < 3 {
			goldenWantStatus(t, returned, "in_progress", "")
			// The activity listener is not running in handler tests; record the return it would.
			recordReturns(t, returned, 1)
		}
	}
	goldenWantStatus(t, returned, "blocked", leadID)
	var report string
	dbfx.QueryRow(t, `SELECT content FROM comment WHERE issue_id = $1 ORDER BY created_at DESC LIMIT 1`, returned).Scan(&report)
	for _, want := range []string{"BLOCKED", "Blocker-class: rework-loop", "3 times"} {
		if !strings.Contains(report, want) {
			t.Fatalf("rework report is missing %q:\n%s", want, report)
		}
	}
}

func TestWorkflowGolden_PlanningAutoWrites(t *testing.T) {
	seedTestCatalog(t)
	resetDefaultWorkflow(t)
	ensureGoldenStatus(t, "research", "started")
	ensureGoldenStatus(t, "plan_review", "started")
	ensureGoldenStatus(t, "stage_done", "done")
	ensureGoldenStatus(t, "plan_ready", "started")
	workerID := seededReadyAgentID(t)
	leadID := goldenLeadAgent(t, workerID)
	steps := goldenWorkflowSteps(t, "planning_auto",
		[]string{"backlog", "todo", "research", "plan_review", "stage_done", "plan_ready", "blocked"},
		map[string]string{"plan_researcher": workerID, "plan_reviewer": workerID, "plan_lead": leadID})
	wf := createTestWorkflowSteps(t, "Golden Planning Auto "+workflowTestSuffix(), steps)
	projectID := createWorkflowTestProject(t, "Golden Planning project")
	setProjectWorkflow(t, projectID, map[string]any{"workflow_id": wf.ID}).Want(http.StatusOK)

	// Entering Research hands the issue to the researcher.
	issueID := dbfx.Issue(t, "golden planning", testutil.Cols{"project_id": projectID, "status": "todo"})
	updateIssueForTest(t, issueID, map[string]any{"status": "research"}).Want(http.StatusOK)
	goldenWantStatus(t, issueID, "research", workerID)
	var taskID string
	dbfx.QueryRow(t, `SELECT id::text FROM agent_task_queue WHERE issue_id = $1 AND agent_id = $2`, issueID, workerID).Scan(&taskID)
	if got := taskWorkflowStep(t, taskID); got != "research" {
		t.Fatalf("handoff run step = %q, want research", got)
	}

	createTestCustomStatus(t, "golden_unlisted_plan", "started")
	refused := updateIssueForTest(t, issueID, map[string]any{"status": "golden_unlisted_plan"}).Want(http.StatusBadRequest).Map()
	if refused["code"] != "status_not_in_workflow" {
		t.Fatalf("code = %v, want status_not_in_workflow", refused["code"])
	}

	skipped := agentUpdateIssue(t, issueID, workerID, taskID, map[string]any{"status": "done"}).Want(http.StatusConflict).Map()
	if skipped["code"] != "workflow_step_skipped" || skipped["next_status"] != "stage_done" {
		t.Fatalf("refusal = %v, want workflow_step_skipped with next_status stage_done", skipped)
	}

	// Approved routes to Stage done, rejected to Blocked.
	mismatch := agentUpdateIssue(t, issueID, workerID, taskID, map[string]any{"status": "blocked", "outcome": "approved"}).
		Want(http.StatusBadRequest).Map()
	if mismatch["code"] != "workflow_outcome_mismatch" || mismatch["expected_status"] != "stage_done" {
		t.Fatalf("refusal = %v, want workflow_outcome_mismatch expecting stage_done", mismatch)
	}
	goldenWantStatus(t, issueID, "research", "")
	agentUpdateIssue(t, issueID, workerID, taskID, map[string]any{"status": "blocked", "outcome": "rejected"}).Want(http.StatusOK)
	goldenWantStatus(t, issueID, "blocked", leadID)

	passed := dbfx.Issue(t, "golden planning passes", testutil.Cols{
		"project_id": projectID, "status": "research", "assignee_type": "agent", "assignee_id": workerID,
	})
	run := goldenRun(t, passed, workerID, "research")
	agentUpdateIssue(t, passed, workerID, run, map[string]any{"status": "stage_done", "outcome": "approved"}).Want(http.StatusOK)
	goldenWantStatus(t, passed, "stage_done", "")
}
