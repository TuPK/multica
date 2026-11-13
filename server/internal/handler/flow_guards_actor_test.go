package handler

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// Who may write to a task a flow opened (E1 to E4 of
// docs/engineering/flow-engine-generalization-spec.md, section 5). The status
// rules of flow_guards_close_test.go say what a task needs to close; these say
// who may close it, move it, cancel it or hand it to someone else.

// agentUpdate sends a PUT on a task as the run of an agent.
func agentUpdate(t *testing.T, issueID, agentID string, body map[string]any) *testutil.Response {
	t.Helper()
	return agentCall(t, issueID, agentID, "in_review", testHandler.UpdateIssue, "PUT", "/api/issues/"+issueID, body)
}

// memberUpdate sends a PUT on an issue as another member of the workspace, one
// who manages no squad.
func memberUpdate(t *testing.T, as func(*http.Request) *http.Request, issueID string, body map[string]any) *testutil.Response {
	t.Helper()
	return testutil.Call(t, testHandler.UpdateIssue, as(withURLParam(newRequest("PUT", "/api/issues/"+issueID, body), "id", issueID)))
}

func agentBatch(t *testing.T, issueID, agentID string, updates map[string]any, ids ...string) *testutil.Response {
	t.Helper()
	return agentCall(t, issueID, agentID, "in_review", testHandler.BatchUpdateIssues, "POST", "/api/issues/batch-update?workspace_id="+testWorkspaceID,
		map[string]any{"issue_ids": ids, "updates": updates})
}

func memberBatch(t *testing.T, as func(*http.Request) *http.Request, updates map[string]any, ids ...string) *testutil.Response {
	t.Helper()
	return testutil.Call(t, testHandler.BatchUpdateIssues, as(newRequest("POST", "/api/issues/batch-update?workspace_id="+testWorkspaceID,
		map[string]any{"issue_ids": ids, "updates": updates})))
}

// activityDetails reads the details of the one activity row of an action.
func activityDetails(t *testing.T, issueID, action string) map[string]any {
	t.Helper()
	if n := dbfx.Count(t, `SELECT count(*) FROM activity_log WHERE issue_id = $1 AND action = $2`, issueID, action); n != 1 {
		t.Fatalf("%d %q rows on the issue, want 1", n, action)
	}
	var raw []byte
	dbfx.QueryRow(t, `SELECT details FROM activity_log WHERE issue_id = $1 AND action = $2`, issueID, action).Scan(&raw)
	var details map[string]any
	if err := json.Unmarshal(raw, &details); err != nil {
		t.Fatalf("details of %s: %v", action, err)
	}
	return details
}

// H1: an agent closes a task that is another agent's.
func TestFlowGuard_AgentCannotCloseAnothersTask(t *testing.T) {
	fx := newFlowFixture(t)

	body := wantFlowRejection(t, agentUpdate(t, fx.Gate, fx.WorkAgent, map[string]any{"status": "done", "outcome": "approved"}),
		http.StatusForbidden, "flow_task_not_assignee")
	if body["task"] != "check" {
		t.Fatalf("task = %v, want check", body["task"])
	}
	if status, _, _ := issueStatusAndAssignee(t, fx.Gate); status != "in_progress" {
		t.Fatalf("status = %q, a refused close must not apply", status)
	}
	if got := issueOutcome(t, fx.Gate); got != "" {
		t.Fatalf("outcome = %q, the verdict of the wrong agent was stored", got)
	}

	// The agent the gate is given to closes it.
	agentUpdate(t, fx.Gate, fx.GateAgent, map[string]any{"status": "done", "outcome": "approved"}).Want(http.StatusOK)
	if got := issueOutcome(t, fx.Gate); got != "approved" {
		t.Fatalf("outcome = %q, want approved", got)
	}
	// ... and it is not an on-behalf close.
	if n := dbfx.Count(t, `SELECT count(*) FROM activity_log WHERE issue_id = $1 AND action = 'flow_manual_close'`, fx.Gate); n != 0 {
		t.Fatalf("the assignee's own close was logged as a close on behalf (%d rows)", n)
	}
}

// H1 for a member: the verdict buttons of the state panel send exactly this.
func TestFlowGuard_PlainMemberCannotCloseTask(t *testing.T) {
	fx := newFlowFixture(t)
	_, plain := addWorkflowTestMember(t, "flow-plain-close")

	wantFlowRejection(t, memberUpdate(t, plain, fx.Gate, map[string]any{"status": "done", "outcome": "approved"}),
		http.StatusForbidden, "flow_task_not_assignee")
	if status, _, _ := issueStatusAndAssignee(t, fx.Gate); status != "in_progress" {
		t.Fatalf("status = %q, a refused close must not apply", status)
	}
	if got := issueOutcome(t, fx.Gate); got != "" {
		t.Fatalf("outcome = %q, a member who manages nothing stored a verdict", got)
	}
}

// D4: a member who manages the squad closes a task for its agent, and the log says so.
func TestFlowGuard_ManagerClosesOnBehalfAndIsLogged(t *testing.T) {
	fx := newFlowFixture(t)

	closeIssue(t, fx.Gate, map[string]any{"outcome": "rejected"}).Want(http.StatusOK)
	if got := issueOutcome(t, fx.Gate); got != "rejected" {
		t.Fatalf("outcome = %q, want rejected", got)
	}
	details := activityDetails(t, fx.Gate, "flow_manual_close")
	if details["task"] != "check" || details["verdict"] != "rejected" || details["on_behalf_of"] != fx.GateAgent ||
		details["files"] != float64(0) || details["activation"] != float64(0) {
		t.Fatalf("details = %v, want task check, verdict rejected, files 0, activation 0, on behalf of the gate agent", details)
	}
	var actorType, actorID string
	dbfx.QueryRow(t, `SELECT actor_type, actor_id::text FROM activity_log WHERE issue_id = $1 AND action = 'flow_manual_close'`, fx.Gate).Scan(&actorType, &actorID)
	if actorType != "member" || actorID != testUserID {
		t.Fatalf("actor = %s %s, want the member who closed it", actorType, actorID)
	}
}

// The rules for what a task needs to close hold for whoever closes it, and the
// files the closer uploaded count.
func TestFlowGuard_ManagerOnBehalfStillNeedsTheFiles(t *testing.T) {
	fx := newFlowFixture(t)

	wantFlowRejection(t, closeIssue(t, fx.Work, nil), http.StatusUnprocessableEntity, "flow_attachment_required")
	// A file somebody else uploaded is not the closer's.
	otherUser, _ := addWorkflowTestMember(t, "flow-uploader")
	dbfx.Insert(t, "attachment", testutil.Cols{
		"workspace_id": testWorkspaceID, "issue_id": fx.Work,
		"uploader_type": "member", "uploader_id": otherUser,
		"filename": "theirs.md", "url": "https://files.example.test/theirs.md", "content_type": "text/markdown", "size_bytes": 3,
	})
	wantFlowRejection(t, closeIssue(t, fx.Work, nil), http.StatusUnprocessableEntity, "flow_attachment_required")

	dbfx.Insert(t, "attachment", testutil.Cols{
		"workspace_id": testWorkspaceID, "issue_id": fx.Work,
		"uploader_type": "member", "uploader_id": testUserID,
		"filename": "mine.md", "url": "https://files.example.test/mine.md", "content_type": "text/markdown", "size_bytes": 4,
	})
	closeIssue(t, fx.Work, nil).Want(http.StatusOK)
	details := activityDetails(t, fx.Work, "flow_manual_close")
	if details["files"] != float64(1) || details["on_behalf_of"] != fx.WorkAgent {
		t.Fatalf("details = %v, want 1 file, on behalf of the work agent", details)
	}
}

// H2: an agent hands a task to someone else without touching its status.
func TestFlowGuard_AgentCannotReassignFlowTask(t *testing.T) {
	fx := newFlowFixture(t)
	other := goldenLeadAgent(t, fx.WorkAgent)

	body := wantFlowRejection(t, agentUpdate(t, fx.Work, fx.WorkAgent, map[string]any{"assignee_type": "agent", "assignee_id": other}),
		http.StatusConflict, "flow_task_assignee_locked")
	if body["task"] != "draft" {
		t.Fatalf("task = %v, want draft", body["task"])
	}
	if _, _, assignee := issueStatusAndAssignee(t, fx.Work); assignee != fx.WorkAgent {
		t.Fatalf("assignee = %s, a refused write must not apply", assignee)
	}
	// Unassigning is a change of assignee too.
	wantFlowRejection(t, agentUpdate(t, fx.Work, fx.WorkAgent, map[string]any{"assignee_type": nil, "assignee_id": nil}),
		http.StatusConflict, "flow_task_assignee_locked")
	// Writing the assignee it already has, or anything else, is allowed.
	agentUpdate(t, fx.Work, fx.WorkAgent, map[string]any{"assignee_type": "agent", "assignee_id": fx.WorkAgent, "title": "renamed " + workflowTestSuffix()}).Want(http.StatusOK)
	// A member who manages the squad may reassign.
	updateIssueForTest(t, fx.Work, map[string]any{"assignee_type": "agent", "assignee_id": other}).Want(http.StatusOK)
	if _, _, assignee := issueStatusAndAssignee(t, fx.Work); assignee != other {
		t.Fatalf("assignee = %s, want the manager's choice", assignee)
	}
}

// H3: an agent cancels the task that holds a file requirement, and the step passes without the file.
func TestFlowGuard_AgentCannotCancelWorkTask(t *testing.T) {
	fx := newFlowFixture(t)

	wantFlowRejection(t, agentUpdate(t, fx.Work, fx.WorkAgent, map[string]any{"status": "cancelled"}),
		http.StatusConflict, "flow_task_cancel_forbidden")
	if status, _, _ := issueStatusAndAssignee(t, fx.Work); status != "in_progress" {
		t.Fatalf("status = %q, a refused cancel must not apply", status)
	}
	// Not even a gate.
	wantFlowRejection(t, agentUpdate(t, fx.Gate, fx.GateAgent, map[string]any{"status": "cancelled"}),
		http.StatusConflict, "flow_task_cancel_forbidden")
	// The request did not move.
	var step string
	dbfx.QueryRow(t, `SELECT flow->>'step' FROM issue WHERE id = $1`, fx.Request).Scan(&step)
	if step != "in_progress" {
		t.Fatalf("the request is at %q after refused cancels", step)
	}
}

// H4: a verdict that is in is not for the agent to change.
func TestFlowGuard_ClosedVerdictFrozenForAgent(t *testing.T) {
	fx := newFlowFixture(t)

	agentUpdate(t, fx.Gate, fx.GateAgent, map[string]any{"status": "done", "outcome": "rejected"}).Want(http.StatusOK)
	wantFlowRejection(t, agentUpdate(t, fx.Gate, fx.GateAgent, map[string]any{"status": "done", "outcome": "approved"}),
		http.StatusConflict, "flow_task_closed")
	if got := issueOutcome(t, fx.Gate); got != "rejected" {
		t.Fatalf("outcome = %q, the agent changed a closed verdict", got)
	}
	// Nor can it reopen the task.
	wantFlowRejection(t, agentUpdate(t, fx.Gate, fx.GateAgent, map[string]any{"status": "in_progress"}),
		http.StatusConflict, "flow_task_closed")
	if status, _, _ := issueStatusAndAssignee(t, fx.Gate); status != "done" {
		t.Fatalf("status = %q, want done", status)
	}
}

func TestFlowGuard_ManagerCanReopenClosedTask(t *testing.T) {
	fx := newFlowFixture(t)
	_, plain := addWorkflowTestMember(t, "flow-plain-reopen")

	agentUpdate(t, fx.Gate, fx.GateAgent, map[string]any{"status": "done", "outcome": "rejected"}).Want(http.StatusOK)
	wantFlowRejection(t, memberUpdate(t, plain, fx.Gate, map[string]any{"status": "in_progress"}), http.StatusForbidden, "flow_task_not_assignee")

	updateIssueForTest(t, fx.Gate, map[string]any{"status": "in_progress"}).Want(http.StatusOK)
	if status, _, _ := issueStatusAndAssignee(t, fx.Gate); status != "in_progress" {
		t.Fatalf("status = %q, want the task reopened", status)
	}
	if got := issueOutcome(t, fx.Gate); got != "" {
		t.Fatalf("outcome = %q, a reopened task keeps no verdict", got)
	}
	details := activityDetails(t, fx.Gate, "flow_manual_reopen")
	if details["task"] != "check" || details["from"] != "done" || details["on_behalf_of"] != fx.GateAgent {
		t.Fatalf("details = %v, want task check reopened from done, on behalf of the gate agent", details)
	}
}

// The batch path applies the same rules (issue.go reads them through checkFlowWrite).
func TestFlowGuard_BatchPathSameRules(t *testing.T) {
	fx := newFlowFixture(t)
	_, plain := addWorkflowTestMember(t, "flow-plain-batch")
	other := goldenLeadAgent(t, fx.WorkAgent)

	// H1 by agent and by member.
	wantFlowRejection(t, agentBatch(t, fx.Gate, fx.WorkAgent, map[string]any{"status": "done", "outcome": "approved"}, fx.Gate),
		http.StatusForbidden, "flow_task_not_assignee")
	wantFlowRejection(t, memberBatch(t, plain, map[string]any{"status": "done", "outcome": "approved"}, fx.Gate),
		http.StatusForbidden, "flow_task_not_assignee")
	// H2.
	wantFlowRejection(t, agentBatch(t, fx.Work, fx.WorkAgent, map[string]any{"assignee_type": "agent", "assignee_id": other}, fx.Work),
		http.StatusConflict, "flow_task_assignee_locked")
	// H3.
	wantFlowRejection(t, agentBatch(t, fx.Work, fx.WorkAgent, map[string]any{"status": "cancelled"}, fx.Work),
		http.StatusConflict, "flow_task_cancel_forbidden")
	if status, _, assignee := issueStatusAndAssignee(t, fx.Work); status != "in_progress" || assignee != fx.WorkAgent {
		t.Fatalf("work task = %s / %s after refused batches", status, assignee)
	}
	if got := issueOutcome(t, fx.Gate); got != "" {
		t.Fatalf("gate outcome = %q after refused batches", got)
	}
	// H4: close it for real, then try to flip the verdict.
	memberBatch(t, func(r *http.Request) *http.Request { return r }, map[string]any{"status": "done", "outcome": "rejected"}, fx.Gate).Want(http.StatusOK)
	wantFlowRejection(t, agentBatch(t, fx.Gate, fx.GateAgent, map[string]any{"status": "done", "outcome": "approved"}, fx.Gate),
		http.StatusConflict, "flow_task_closed")
	// The refusal of one item refuses the batch.
	plainIssue := dbfx.Issue(t, "plain issue in the batch "+workflowTestSuffix())
	wantFlowRejection(t, agentBatch(t, fx.Work, fx.WorkAgent, map[string]any{"status": "cancelled"}, plainIssue, fx.Work),
		http.StatusConflict, "flow_task_cancel_forbidden")
	if status, _, _ := issueStatusAndAssignee(t, plainIssue); status != "todo" {
		t.Fatalf("the batch wrote the plain issue (status %q) though another item was refused", status)
	}
}
