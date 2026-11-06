package handler

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// askCall makes the Secretary's own call in its chat turn f.
func askCall(t *testing.T, f directiveRunFixture, body map[string]any) *testutil.Response {
	req := newRequest("POST", "/api/secretary/requests?workspace_id="+testWorkspaceID, body)
	req.Header.Set("X-Agent-ID", f.secretaryID)
	req.Header.Set("X-Task-ID", f.taskID)
	return testutil.Call(t, testHandler.SecretaryAsk, req)
}

// The one call files the parent request in the desk, the children, the metadata,
// and the deadline; the children carry the request as parent.
func TestSecretaryAsk_FilesTheWholeRequestInOneCall(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := seedDirectiveRun(t, "Is the spring launch on track?")
	dbfx.Project(t, "Secretary desk", testutil.Cols{"workspace_id": testWorkspaceID})
	growth := createHandlerTestAgent(t, "ask leader growth "+uuid.NewString(), nil)
	release := createHandlerTestAgent(t, "ask leader release "+uuid.NewString(), nil)

	deadline := time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339)
	var out SecretaryAskResponse
	askCall(t, f, map[string]any{
		"question":        "Is the spring launch on track, and what could delay it?",
		"chat_session_id": f.sessionID,
		"due_date":        time.Now().Add(72 * time.Hour).Format("2006-01-02"),
		"deadline":        deadline,
		"addressees": []map[string]any{
			{"agent_id": growth, "title": "Growth: what could delay the launch", "description": "The boss asks whether spring is on track."},
			{"agent_id": release, "title": "Release: is the date firm", "description": "The boss asks whether the date is firm."},
		},
	}).Want(http.StatusCreated).JSON(&out)

	if out.RequestID == "" || len(out.ChildIDs) != 2 || out.WakeupID == "" {
		t.Fatalf("response = %+v, want a request, two children and a wakeup", out)
	}
	var parent, sessionMeta, askedBy string
	dbfx.QueryRow(t, `SELECT COALESCE(parent_issue_id::text, ''), metadata->>'chat_session_id' FROM issue WHERE id = $1`, out.RequestID).Scan(&parent, &sessionMeta)
	if parent != "" || sessionMeta != f.sessionID {
		t.Fatalf("request parent=%q metadata chat=%q, want no parent and the chat %q", parent, sessionMeta, f.sessionID)
	}
	dbfx.QueryRow(t, `SELECT metadata->>'asked_by' FROM issue WHERE id = $1`, out.RequestID).Scan(&askedBy)
	if askedBy == "" {
		t.Fatal("request metadata has no asked_by")
	}
	for _, child := range out.ChildIDs {
		var p string
		dbfx.QueryRow(t, `SELECT COALESCE(parent_issue_id::text, '') FROM issue WHERE id = $1`, child).Scan(&p)
		if p != out.RequestID {
			t.Fatalf("child %s parent = %q, want %q", child, p, out.RequestID)
		}
	}
}

// The call is refused outside the Secretary's own turn in that chat, and for an
// invalid request body; nothing is filed on a refusal.
func TestSecretaryAsk_RefusedWithoutFilingAnything(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := seedDirectiveRun(t, "Is the spring launch on track?")
	dbfx.Project(t, "Secretary desk", testutil.Cols{"workspace_id": testWorkspaceID})
	leader := createHandlerTestAgent(t, "ask refusal leader "+uuid.NewString(), nil)
	deadline := time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339)
	good := func() map[string]any {
		return map[string]any{
			"question":        "Is the spring launch on track?",
			"chat_session_id": f.sessionID,
			"due_date":        time.Now().Add(72 * time.Hour).Format("2006-01-02"),
			"deadline":        deadline,
			"addressees":      []map[string]any{{"agent_id": leader, "title": "Growth", "description": "Answer the boss."}},
		}
	}

	other := good()
	other["chat_session_id"] = uuid.NewString()
	askCall(t, f, other).Want(http.StatusForbidden)

	empty := good()
	empty["question"] = "   "
	askCall(t, f, empty).Want(http.StatusBadRequest)

	badDue := good()
	badDue["due_date"] = "next week"
	askCall(t, f, badDue).Want(http.StatusBadRequest)

	tooMany := good()
	addressees := make([]map[string]any, 0, 21)
	for i := 0; i < 21; i++ {
		addressees = append(addressees, map[string]any{"agent_id": leader, "title": "x", "description": "y"})
	}
	tooMany["addressees"] = addressees
	askCall(t, f, tooMany).Want(http.StatusBadRequest)

	var filed int
	dbfx.QueryRow(t, `SELECT count(*) FROM issue WHERE metadata->>'chat_session_id' = $1`, f.sessionID).Scan(&filed)
	if filed != 0 {
		t.Fatalf("a refused call filed %d requests", filed)
	}
	_ = json.Valid
}
