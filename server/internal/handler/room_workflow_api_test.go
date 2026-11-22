package handler

import (
	"context"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// API tests for the room-owned workflow policy and its write preview
// (docs/engineering/squad-room-workflow-spec.md, sections 4 to 6).

// resetRoomWorkflowPolicy clears the workspace's stored choice before and after
// a test, so tests do not see each other's policy.
func resetRoomWorkflowPolicy(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	clear := func() {
		testPool.Exec(ctx, `UPDATE workspace SET settings = settings - 'non_request_workflow' WHERE id = $1`, testWorkspaceID)
	}
	clear()
	t.Cleanup(clear)
}

func roomWorkflowPolicyPath() string {
	return "/api/workspaces/" + testWorkspaceID + "/room-workflow"
}

func TestRoomWorkflowPolicy_DefaultsToNoActionAndRoundTrips(t *testing.T) {
	resetRoomWorkflowPolicy(t)

	var got roomWorkflowPolicyEnvelope
	testutil.Call(t, testHandler.GetRoomWorkflowPolicy,
		withURLParam(newRequest("GET", roomWorkflowPolicyPath(), nil), "id", testWorkspaceID)).
		Want(http.StatusOK).JSON(&got)
	if got.NonRequestWorkflow.Kind != "no_action" {
		t.Fatalf("default kind = %q, want no_action", got.NonRequestWorkflow.Kind)
	}

	testutil.Call(t, testHandler.PutRoomWorkflowPolicy,
		withURLParam(newRequest("PUT", roomWorkflowPolicyPath(), map[string]any{
			"non_request_workflow": map[string]any{"kind": "default"},
		}), "id", testWorkspaceID)).Want(http.StatusOK)

	testutil.Call(t, testHandler.GetRoomWorkflowPolicy,
		withURLParam(newRequest("GET", roomWorkflowPolicyPath(), nil), "id", testWorkspaceID)).
		Want(http.StatusOK).JSON(&got)
	if got.NonRequestWorkflow.Kind != "default" {
		t.Fatalf("kind after PUT = %q, want default", got.NonRequestWorkflow.Kind)
	}
}

func TestRoomWorkflowPolicy_WorkflowChoiceMustBeInTheWorkspace(t *testing.T) {
	resetRoomWorkflowPolicy(t)
	wf := createTestWorkflow(t, "Policy choice "+workflowTestSuffix())

	testutil.Call(t, testHandler.PutRoomWorkflowPolicy,
		withURLParam(newRequest("PUT", roomWorkflowPolicyPath(), map[string]any{
			"non_request_workflow": map[string]any{"kind": "workflow", "workflow_id": wf.ID},
		}), "id", testWorkspaceID)).Want(http.StatusOK)

	var got roomWorkflowPolicyEnvelope
	testutil.Call(t, testHandler.GetRoomWorkflowPolicy,
		withURLParam(newRequest("GET", roomWorkflowPolicyPath(), nil), "id", testWorkspaceID)).
		Want(http.StatusOK).JSON(&got)
	if got.NonRequestWorkflow.Kind != "workflow" || got.NonRequestWorkflow.WorkflowID != wf.ID {
		t.Fatalf("policy = %+v, want the workflow %s", got.NonRequestWorkflow, wf.ID)
	}

	testutil.Call(t, testHandler.PutRoomWorkflowPolicy,
		withURLParam(newRequest("PUT", roomWorkflowPolicyPath(), map[string]any{
			"non_request_workflow": map[string]any{"kind": "workflow", "workflow_id": "00000000-0000-0000-0000-000000000000"},
		}), "id", testWorkspaceID)).Want(http.StatusBadRequest)
}

func TestRoomWorkflowPolicy_RejectsUnknownKind(t *testing.T) {
	resetRoomWorkflowPolicy(t)
	testutil.Call(t, testHandler.PutRoomWorkflowPolicy,
		withURLParam(newRequest("PUT", roomWorkflowPolicyPath(), map[string]any{
			"non_request_workflow": map[string]any{"kind": "fallback"},
		}), "id", testWorkspaceID)).Want(http.StatusBadRequest)
}

func TestRoomWorkflowPolicy_SettingsWriteKeepsTheStoredChoice(t *testing.T) {
	resetRoomWorkflowPolicy(t)
	testutil.Call(t, testHandler.PutRoomWorkflowPolicy,
		withURLParam(newRequest("PUT", roomWorkflowPolicyPath(), map[string]any{
			"non_request_workflow": map[string]any{"kind": "default"},
		}), "id", testWorkspaceID)).Want(http.StatusOK)

	// A settings form loaded before the choice was made sends settings without it.
	testutil.Call(t, testHandler.UpdateWorkspace,
		withURLParam(newRequest("PATCH", "/api/workspaces/"+testWorkspaceID, map[string]any{
			"settings": map[string]any{"unrelated": true},
		}), "id", testWorkspaceID)).Want(http.StatusOK)

	var got roomWorkflowPolicyEnvelope
	testutil.Call(t, testHandler.GetRoomWorkflowPolicy,
		withURLParam(newRequest("GET", roomWorkflowPolicyPath(), nil), "id", testWorkspaceID)).
		Want(http.StatusOK).JSON(&got)
	if got.NonRequestWorkflow.Kind != "default" {
		t.Fatalf("kind after a settings write = %q, want default", got.NonRequestWorkflow.Kind)
	}
}

func TestRoomWorkflowCheck_RoomWithoutWorkflowNeedsOne(t *testing.T) {
	resetRoomWorkflowPolicy(t)
	agentID := seededReadyAgentID(t)
	project := createTestProjectForResources(t, "Room check "+workflowTestSuffix())
	squadID := seedSquad(t, agentID, "No workflow room "+workflowTestSuffix(), false)
	testPool.Exec(context.Background(), `UPDATE squad SET active_workflow_id = NULL WHERE id = $1`, squadID)

	var got roomWorkflowCheck
	testutil.Call(t, testHandler.CheckRoomWorkflow,
		withURLParam(newRequest("GET", "/api/workspaces/"+testWorkspaceID+"/room-workflow/check?project_id="+project.ID+"&assignee_type=squad&assignee_id="+squadID, nil), "id", testWorkspaceID)).
		Want(http.StatusOK).JSON(&got)
	if !got.NeedsWorkflow || got.Code != "room_workflow_required" || got.SquadID != squadID {
		t.Fatalf("check = %+v, want room_workflow_required for squad %s", got, squadID)
	}
}

func TestRoomWorkflowCheck_RoomWithWorkflowNeedsNothing(t *testing.T) {
	resetRoomWorkflowPolicy(t)
	agentID := seededReadyAgentID(t)
	project := createTestProjectForResources(t, "Room check ok "+workflowTestSuffix())
	squadID := seedSquad(t, agentID, "Workflow room "+workflowTestSuffix(), false)
	wf := createTestWorkflow(t, "Room workflow "+workflowTestSuffix())
	testPool.Exec(context.Background(), `UPDATE squad SET active_workflow_id = $2 WHERE id = $1`, squadID, wf.ID)

	var got roomWorkflowCheck
	testutil.Call(t, testHandler.CheckRoomWorkflow,
		withURLParam(newRequest("GET", "/api/workspaces/"+testWorkspaceID+"/room-workflow/check?project_id="+project.ID+"&assignee_type=squad&assignee_id="+squadID, nil), "id", testWorkspaceID)).
		Want(http.StatusOK).JSON(&got)
	if got.NeedsWorkflow {
		t.Fatalf("check = %+v, want no refusal for a room with a workflow", got)
	}
}

// giveSquadWorkflow makes a room own the workflow: its active workflow is wf.
func giveSquadWorkflow(t *testing.T, squadID, workflowID string) {
	t.Helper()
	dbfx.Exec(t, `UPDATE squad SET active_workflow_id = $2 WHERE id = $1`, squadID, workflowID)
}

// giveSquadDefaultWorkflow gives a room, in workspace ws, a workflow with the
// built-in Default steps: the workflow an issue of a room without one used before
// the room rules. The workflow is written in ws, the squad's own workspace.
func giveSquadDefaultWorkflow(t *testing.T, ws, squadID string) string {
	t.Helper()
	var steps []issueworkflow.Step
	for _, key := range issueworkflow.BuiltInDefaultKeys() {
		steps = append(steps, issueworkflow.Step{StatusKey: key, Handler: issueworkflow.Handler{Type: issueworkflow.HandlerNone}})
	}
	encoded, err := issueworkflow.EncodeSteps(issueworkflow.Arrange(steps))
	if err != nil {
		t.Fatalf("encode steps: %v", err)
	}
	row, err := testHandler.Queries.CreateIssueWorkflow(context.Background(), db.CreateIssueWorkflowParams{
		WorkspaceID:      util.MustParseUUID(ws),
		Name:             "Room default " + workflowTestSuffix(),
		InitialStatusKey: "todo",
		Steps:            encoded,
	})
	if err != nil {
		t.Fatalf("create room workflow: %v", err)
	}
	giveSquadWorkflow(t, squadID, uuidToString(row.ID))
	t.Cleanup(func() {
		ctx := context.Background()
		testPool.Exec(ctx, `UPDATE squad SET active_workflow_id = NULL WHERE id = $1`, squadID)
		testPool.Exec(ctx, `DELETE FROM issue_workflow WHERE id = $1`, row.ID)
	})
	return uuidToString(row.ID)
}

// useAsWorkspaceDefault makes workflowID the workspace's Default workflow, the one
// an issue outside any room follows (spec section 3). An empty id clears the
// choice, so the built-in Default applies. The previous Default row is parked
// under a changed name and restored after the test, since the workspace may hold
// only one Default and a second row named Default would clash.
func useAsWorkspaceDefault(t *testing.T, workflowID string) {
	t.Helper()
	ctx := context.Background()
	rows, err := testPool.Query(ctx, `SELECT id::text, name FROM issue_workflow WHERE workspace_id = $1 AND is_default AND id::text <> $2`, testWorkspaceID, workflowID)
	if err != nil {
		t.Fatalf("list the workspace default: %v", err)
	}
	type parked struct{ id, name string }
	var previous []parked
	for rows.Next() {
		var p parked
		if err := rows.Scan(&p.id, &p.name); err != nil {
			t.Fatalf("scan the workspace default: %v", err)
		}
		previous = append(previous, p)
	}
	rows.Close()
	for _, p := range previous {
		if _, err := testPool.Exec(ctx, `UPDATE issue_workflow SET is_default = false, name = name || ' (parked by test)' WHERE id = $1`, p.id); err != nil {
			t.Fatalf("park the workspace default: %v", err)
		}
		t.Cleanup(func() {
			testPool.Exec(context.Background(), `UPDATE issue_workflow SET is_default = true, name = $2 WHERE id = $1`, p.id, p.name)
		})
	}
	if workflowID == "" {
		return
	}
	if _, err := testPool.Exec(ctx, `UPDATE issue_workflow SET is_default = true WHERE id = $1 AND workspace_id = $2`, workflowID, testWorkspaceID); err != nil {
		t.Fatalf("set the workspace default: %v", err)
	}
	resetDefaultWorkflow(t)
}

// roomWithWorkflow makes a squad, led by a test agent, whose active workflow is
// workflowID: a room of its own, which is what the room rules read.
func roomWithWorkflow(t *testing.T, label, workflowID string) string {
	t.Helper()
	squadID := dbfx.Squad(t, label+" "+workflowTestSuffix(), seededReadyAgentID(t))
	giveSquadWorkflow(t, squadID, workflowID)
	return squadID
}

// roomPiece makes an issue that is a piece of a request in the room: a sub-issue
// assigned to the squad. A workflow hands these off on a status move; a member's
// move of the squad's own top-level request is not a handoff.
func roomPiece(t *testing.T, projectID, squadID, title string, cols testutil.Cols) string {
	t.Helper()
	request := dbfx.Issue(t, "request of "+title, testutil.Cols{
		"project_id": projectID, "status": "in_progress", "assignee_type": "squad", "assignee_id": squadID,
	})
	merged := testutil.Cols{
		"project_id": projectID, "assignee_type": "squad", "assignee_id": squadID, "parent_issue_id": request,
	}
	for k, v := range cols {
		merged[k] = v
	}
	return dbfx.Issue(t, title, merged)
}
