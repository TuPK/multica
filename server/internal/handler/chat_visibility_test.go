package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// chatRequestAs builds a request that runs as userID with that user's member row.
func chatRequestAs(t *testing.T, userID, method, path string, body any) *http.Request {
	t.Helper()
	req := requestAs(userID, method, path, body)
	row, err := testHandler.Queries.GetMemberByUserAndWorkspace(context.Background(), db.GetMemberByUserAndWorkspaceParams{
		UserID:      util.MustParseUUID(userID),
		WorkspaceID: util.MustParseUUID(testWorkspaceID),
	})
	if err != nil {
		t.Fatalf("load member row: %v", err)
	}
	return req.WithContext(middleware.SetMemberContext(req.Context(), testWorkspaceID, row))
}

func errorBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode error body %q: %v", w.Body.String(), err)
	}
	return m
}

func TestChatSessionHiddenProjectAnswersMissing(t *testing.T) {
	f := newRestrictedFixture(t)
	agentID := createHandlerTestAgent(t, "ChatHiddenProjectAgent", []byte("[]"))
	missing := uuid.NewString()

	create := func(userID, projectID string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		testHandler.CreateChatSession(w, chatRequestAs(t, userID, "POST", "/api/chat/sessions",
			map[string]any{"agent_id": agentID, "project_id": projectID}))
		return w
	}
	update := func(userID, sessionID, projectID string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := withURLParam(chatRequestAs(t, userID, "PATCH", "/api/chat/sessions/"+sessionID,
			map[string]any{"project_id": projectID}), "sessionId", sessionID)
		testHandler.UpdateChatSession(w, req)
		return w
	}
	ownSession := func(userID string) string {
		var id string
		if err := testPool.QueryRow(context.Background(), `
			INSERT INTO chat_session (workspace_id, agent_id, creator_id, title, status, explicitly_created_at)
			VALUES ($1, $2, $3, 'hidden project fixture', 'active', now()) RETURNING id`,
			testWorkspaceID, agentID, userID).Scan(&id); err != nil {
			t.Fatalf("insert chat session: %v", err)
		}
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM chat_session WHERE id = $1`, id) })
		return id
	}
	cleanup := func(w *httptest.ResponseRecorder) {
		if w.Code == http.StatusCreated {
			var s struct{ ID string }
			json.Unmarshal(w.Body.Bytes(), &s)
			t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM chat_session WHERE id = $1`, s.ID) })
		}
	}

	wantMissing := create(f.outsiderID, missing)
	if wantMissing.Code != http.StatusNotFound {
		t.Fatalf("missing project create: want 404, got %d %s", wantMissing.Code, wantMissing.Body.String())
	}
	hidden := create(f.outsiderID, f.restrictedProjectID)
	cleanup(hidden)
	if hidden.Code != wantMissing.Code || errorBody(t, hidden)["error"] != errorBody(t, wantMissing)["error"] {
		t.Fatalf("hidden create = %d %s, want same as missing %d %s", hidden.Code, hidden.Body.String(), wantMissing.Code, wantMissing.Body.String())
	}

	outsiderSession := ownSession(f.outsiderID)
	wantMissingUp := update(f.outsiderID, outsiderSession, missing)
	if wantMissingUp.Code != http.StatusNotFound {
		t.Fatalf("missing project update: want 404, got %d %s", wantMissingUp.Code, wantMissingUp.Body.String())
	}
	hiddenUp := update(f.outsiderID, outsiderSession, f.restrictedProjectID)
	if hiddenUp.Code != wantMissingUp.Code || errorBody(t, hiddenUp)["error"] != errorBody(t, wantMissingUp)["error"] {
		t.Fatalf("hidden update = %d %s, want same as missing %d %s", hiddenUp.Code, hiddenUp.Body.String(), wantMissingUp.Code, wantMissingUp.Body.String())
	}

	for name, userID := range map[string]string{"grantee": f.granteeID, "owner": testUserID} {
		w := create(userID, f.restrictedProjectID)
		cleanup(w)
		if w.Code != http.StatusCreated {
			t.Fatalf("%s create: want 201, got %d %s", name, w.Code, w.Body.String())
		}
		if w := update(userID, ownSession(userID), f.restrictedProjectID); w.Code != http.StatusOK {
			t.Fatalf("%s update: want 200, got %d %s", name, w.Code, w.Body.String())
		}
	}
}
