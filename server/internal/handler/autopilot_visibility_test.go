package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestAutopilotHiddenProjectAnswersMissing(t *testing.T) {
	f := newRestrictedFixture(t)
	missing := uuid.NewString()
	agentID := createHandlerTestAgent(t, "AutopilotHiddenProjectAgent", nil)

	create := func(userID, projectID string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		testHandler.CreateAutopilot(w, newRequestAs(userID, "POST", "/api/autopilots?workspace_id="+testWorkspaceID, map[string]any{
			"title": "hidden project autopilot", "assignee_id": agentID,
			"execution_mode": "create_issue", "project_id": projectID,
		}))
		if w.Code == http.StatusCreated {
			var a struct{ ID string }
			json.Unmarshal(w.Body.Bytes(), &a)
			t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM autopilot WHERE id = $1`, a.ID) })
		}
		return w
	}
	update := func(userID, autopilotID, projectID string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := withURLParam(newRequestAs(userID, "PATCH", "/api/autopilots/"+autopilotID+"?workspace_id="+testWorkspaceID,
			map[string]any{"project_id": projectID}), "id", autopilotID)
		testHandler.UpdateAutopilot(w, req)
		return w
	}

	wantMissing := create(f.outsiderID, missing)
	hidden := create(f.outsiderID, f.restrictedProjectID)
	if wantMissing.Code == http.StatusCreated {
		t.Fatalf("missing project create unexpectedly succeeded: %s", wantMissing.Body.String())
	}
	if hidden.Code != wantMissing.Code || errorBody(t, hidden)["error"] != errorBody(t, wantMissing)["error"] {
		t.Fatalf("hidden create = %d %s, want same as missing %d %s", hidden.Code, hidden.Body.String(), wantMissing.Code, wantMissing.Body.String())
	}

	own := createAutopilotAs(t, f.outsiderID, "hidden-project-upd")
	wantMissingUp := update(f.outsiderID, own, missing)
	hiddenUp := update(f.outsiderID, own, f.restrictedProjectID)
	if wantMissingUp.Code == http.StatusOK {
		t.Fatalf("missing project update unexpectedly succeeded: %s", wantMissingUp.Body.String())
	}
	if hiddenUp.Code != wantMissingUp.Code || errorBody(t, hiddenUp)["error"] != errorBody(t, wantMissingUp)["error"] {
		t.Fatalf("hidden update = %d %s, want same as missing %d %s", hiddenUp.Code, hiddenUp.Body.String(), wantMissingUp.Code, wantMissingUp.Body.String())
	}

	for name, userID := range map[string]string{"grantee": f.granteeID, "owner": testUserID} {
		if w := create(userID, f.restrictedProjectID); w.Code != http.StatusCreated {
			t.Fatalf("%s create: want 201, got %d %s", name, w.Code, w.Body.String())
		}
		mine := createAutopilotAs(t, userID, "hidden-project-ok-"+name)
		if w := update(userID, mine, f.restrictedProjectID); w.Code != http.StatusOK {
			t.Fatalf("%s update: want 200, got %d %s", name, w.Code, w.Body.String())
		}
	}
}
