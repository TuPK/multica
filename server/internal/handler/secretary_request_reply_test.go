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
