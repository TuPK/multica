package handler

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// A children-done run may reply into a chat only when the request names that
// chat and the chat is the Secretary's own chat with the run's originator. Any
// other chat, or no usable chat id, leaves the run with no reply session.
func TestSecretaryRequestReplySession_OnlyTheSecretarysChatWithTheBoss(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	cases := map[string]func(t *testing.T, f directiveRunFixture) string{
		"a chat that belongs to another person": func(t *testing.T, f directiveRunFixture) string {
			other := dbfx.User(t, "Other person", "other-"+uuid.NewString()+"@example.test")
			return dbfx.ChatSession(t, f.secretaryID, testutil.Cols{"creator_id": other})
		},
		"a chat with an agent that is not the Secretary": func(t *testing.T, f directiveRunFixture) string {
			other := dbfx.Agent(t, "not the secretary "+uuid.NewString(), handlerTestRuntimeID(t), testutil.Cols{"workspace_id": testWorkspaceID})
			return dbfx.ChatSession(t, other, testutil.Cols{"creator_id": testUserID})
		},
		"a chat id that does not exist": func(t *testing.T, f directiveRunFixture) string {
			return uuid.NewString()
		},
		"a value that is not an id": func(t *testing.T, f directiveRunFixture) string {
			return "not-an-id"
		},
	}
	for name, chatFor := range cases {
		t.Run(name, func(t *testing.T) {
			f := seedDirectiveRun(t, "Is the spring launch on track?")
			desk := dbfx.Project(t, "Secretary desk", testutil.Cols{"workspace_id": testWorkspaceID})
			leader := createHandlerTestAgent(t, "reply link leader", nil)

			var request IssueResponse
			testutil.Call(t, testHandler.CreateIssue, secretaryCall(f, "POST", "/api/issues?workspace_id="+testWorkspaceID, map[string]any{
				"title":         "Launch assessment",
				"project_id":    desk,
				"assignee_type": "agent",
				"assignee_id":   f.secretaryID,
			})).Want(http.StatusCreated).JSON(&request)
			dbfx.Exec(t, `UPDATE issue SET metadata = jsonb_build_object('chat_session_id', $2::text) WHERE id = $1`, request.ID, chatFor(t, f))

			var child IssueResponse
			testutil.Call(t, testHandler.CreateIssue, secretaryCall(f, "POST", "/api/issues?workspace_id="+testWorkspaceID, map[string]any{
				"title":           "Growth: what could delay the launch",
				"status":          "in_progress",
				"parent_issue_id": request.ID,
				"assignee_type":   "agent",
				"assignee_id":     leader,
			})).Want(http.StatusCreated).JSON(&child)
			answerAsAgent(t, child.ID, leader, "Answer: on track.")
			updateChildStatus(t, child.ID, "done")

			runs := childDoneRuns(t, request.ID)
			if len(runs) != 1 {
				t.Fatalf("child_done runs = %d, want 1", len(runs))
			}
			var replySession string
			dbfx.QueryRow(t, `SELECT COALESCE(reply_chat_session_id::text, '') FROM agent_task_queue WHERE id = $1`, runs[0].ID).Scan(&replySession)
			if replySession != "" {
				t.Fatalf("run was linked to chat %q; it must have no reply session", replySession)
			}
		})
	}
}

// answerAsAgent records the agent's answer on an issue: the comment the Secretary
// reads when the issue closes.
func answerAsAgent(t *testing.T, issueID, agentID, content string) {
	t.Helper()
	dbfx.Insert(t, "comment", testutil.Cols{
		"issue_id":     issueID,
		"workspace_id": testWorkspaceID,
		"author_type":  "agent",
		"author_id":    agentID,
		"content":      content,
	})
}

// A child of a Secretary request closes only after an agent has answered on it.
func TestSecretaryRequestChild_CloseNeedsAnAnswer(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := seedDirectiveRun(t, "Is the spring launch on track?")
	desk := dbfx.Project(t, "Secretary desk", testutil.Cols{"workspace_id": testWorkspaceID})
	leader := createHandlerTestAgent(t, "answer gate leader", nil)

	var request IssueResponse
	testutil.Call(t, testHandler.CreateIssue, secretaryCall(f, "POST", "/api/issues?workspace_id="+testWorkspaceID, map[string]any{
		"title":         "Launch answer gate",
		"project_id":    desk,
		"assignee_type": "agent",
		"assignee_id":   f.secretaryID,
	})).Want(http.StatusCreated).JSON(&request)

	var child IssueResponse
	testutil.Call(t, testHandler.CreateIssue, secretaryCall(f, "POST", "/api/issues?workspace_id="+testWorkspaceID, map[string]any{
		"title":           "Growth: answer before closing",
		"status":          "in_progress",
		"parent_issue_id": request.ID,
		"assignee_type":   "agent",
		"assignee_id":     leader,
	})).Want(http.StatusCreated).JSON(&child)

	closeChild := func() *testutil.Response {
		req := newRequest("PUT", "/api/issues/"+child.ID, map[string]any{"status": "done"})
		req = withURLParam(req, "id", child.ID)
		return testutil.Call(t, testHandler.UpdateIssue, req)
	}
	closeChild().Want(http.StatusUnprocessableEntity)

	answerAsAgent(t, child.ID, leader, "Answer: the spring launch holds for 14 March.")
	closeChild().Want(http.StatusOK)
}

// The assessment gate: an assessment is written from a request's answers, after
// every leader has closed its child (or at the deadline). Other kinds and other
// runs are not gated by the answers.
func TestSecretaryAssessment_GatedByTheRequestsAnswers(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := seedDirectiveRun(t, "Is the spring launch on track?")
	desk := dbfx.Project(t, "Secretary desk", testutil.Cols{"workspace_id": testWorkspaceID})
	leaderA := createHandlerTestAgent(t, "assessment leader A", nil)
	leaderB := createHandlerTestAgent(t, "assessment leader B", nil)

	var request IssueResponse
	testutil.Call(t, testHandler.CreateIssue, secretaryCall(f, "POST", "/api/issues?workspace_id="+testWorkspaceID, map[string]any{
		"title":         "Spring launch assessment gate",
		"project_id":    desk,
		"assignee_type": "agent",
		"assignee_id":   f.secretaryID,
	})).Want(http.StatusCreated).JSON(&request)
	dbfx.Exec(t, `UPDATE issue SET metadata = jsonb_build_object('chat_session_id', $2::text) WHERE id = $1`, request.ID, f.sessionID)

	makeChild := func(title string, leader string) IssueResponse {
		var child IssueResponse
		testutil.Call(t, testHandler.CreateIssue, secretaryCall(f, "POST", "/api/issues?workspace_id="+testWorkspaceID, map[string]any{
			"title":           title,
			"status":          "in_progress",
			"parent_issue_id": request.ID,
			"assignee_type":   "agent",
			"assignee_id":     leader,
		})).Want(http.StatusCreated).JSON(&child)
		return child
	}
	childA := makeChild("Growth answer", leaderA)
	childB := makeChild("Release answer", leaderB)

	reply := func(runID, kind string) *testutil.Response {
		req := newRequest("POST", "/api/chat/sessions/"+f.sessionID+"/reply", map[string]string{"content": "Assessment: on track.", "kind": kind})
		req = asAgent(req, f.secretaryID, runID)
		req = withURLParam(req, "sessionId", f.sessionID)
		return testutil.Call(t, testHandler.ReplyChatSession, req)
	}

	// The children-done run starts only once every child has closed, so while B is
	// open no such run exists. Leader A closes alone first.
	answerAsAgent(t, childA.ID, leaderA, "Answer: on track for 14 March.")
	updateChildStatus(t, childA.ID, "done")
	if runs := childDoneRuns(t, request.ID); len(runs) != 0 {
		t.Fatalf("a children-done run started while a child is open: %+v", runs)
	}

	// A run that reads the request while B is open is refused an assessment; a
	// fact is still allowed. The run is bound to the request's chat as a request run.
	var owner string
	dbfx.QueryRow(t, `SELECT COALESCE(originator_user_id::text, '') FROM agent_task_queue WHERE id = $1`, f.taskID).Scan(&owner)
	openRun := dbfx.Task(t, f.secretaryID, testutil.Cols{
		"runtime_id":            handlerTestRuntimeID(t),
		"issue_id":              request.ID,
		"reply_chat_session_id": f.sessionID,
		"status":                "running",
		"originator_user_id":    owner,
		"accountable_user_id":   owner,
		"context":               `{"wakeup_id": "` + uuid.NewString() + `"}`,
	})
	reply(openRun, "assessment").Want(http.StatusConflict)
	reply(openRun, "bogus").Want(http.StatusBadRequest)
	reply(openRun, "fact").Want(http.StatusCreated)

	// Once B has answered and closed, every leader has closed and the assessment passes.
	answerAsAgent(t, childB.ID, leaderB, "Answer: the tax rules are the risk.")
	updateChildStatus(t, childB.ID, "done")
	runs := childDoneRuns(t, request.ID)
	if len(runs) == 0 {
		t.Fatal("no children-done run after every child closed")
	}
	reply(runs[len(runs)-1].ID, "assessment").Want(http.StatusCreated)
}

// An assessment from a run that reads no request is refused.
func TestSecretaryAssessment_RefusedOutsideARequest(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	env := newChatReplyEnv(t)
	task := env.runFor(t, env.secretary, env.session, testUserID)
	req := newRequest("POST", "/api/chat/sessions/"+env.session+"/reply", map[string]string{"content": "On track.", "kind": "assessment"})
	req = asAgent(req, env.secretary, task)
	req = withURLParam(req, "sessionId", env.session)
	testutil.Call(t, testHandler.ReplyChatSession, req).Want(http.StatusConflict)
}

// The batch close obeys the same rule as a single close: one child without an
// answer refuses the whole batch.
func TestSecretaryRequestChild_BatchCloseNeedsAnAnswer(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := seedDirectiveRun(t, "Is the spring launch on track?")
	desk := dbfx.Project(t, "Secretary desk", testutil.Cols{"workspace_id": testWorkspaceID})
	leader := createHandlerTestAgent(t, "batch gate leader", nil)
	request := dbfx.Issue(t, "Batch gate request", testutil.Cols{"workspace_id": testWorkspaceID, "project_id": desk, "assignee_type": "agent", "assignee_id": f.secretaryID})
	child := dbfx.Issue(t, "Batch gate child", testutil.Cols{"workspace_id": testWorkspaceID, "parent_issue_id": request, "status": "in_progress", "assignee_type": "agent", "assignee_id": leader})

	batch := func() *testutil.Response {
		req := newRequest("POST", "/api/issues/batch-update?workspace_id="+testWorkspaceID, map[string]any{
			"issue_ids": []string{child},
			"updates":   map[string]any{"status": "done"},
		})
		return testutil.Call(t, testHandler.BatchUpdateIssues, req)
	}
	batch().Want(http.StatusUnprocessableEntity)
	answerAsAgent(t, child, leader, "Answer: on track.")
	batch().Want(http.StatusOK)
}
