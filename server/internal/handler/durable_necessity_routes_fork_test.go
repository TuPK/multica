package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// TestNecessityRoute_StartAndList: a person starts a necessity test on a skill the seat holds
// (201). The seat's list shows it, and a second start on the same skill is refused while the first
// runs (409 necessity_test_running, spec 10.3).
func TestNecessityRoute_StartAndList(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.DurableSeats, true)
	e := newDigestEnv(t)
	squad := e.squad(t, "necessity route squad")
	dbfx.Exec(t, `UPDATE agent SET home_squad_id = $2 WHERE id = $1`, e.AgentID, squad)
	skill := dbfx.Insert(t, "skill", testutil.Cols{"workspace_id": e.WorkspaceID, "name": "necessity route skill"})
	dbfx.Exec(t, `INSERT INTO agent_skill (agent_id, skill_id) VALUES ($1, $2)`, e.AgentID, skill)
	t.Cleanup(func() {
		ctx := context.Background()
		testPool.Exec(ctx, `DELETE FROM shadow_config WHERE agent_id = $1`, e.AgentID)
		testPool.Exec(ctx, `DELETE FROM necessity_test WHERE agent_id = $1`, e.AgentID)
	})

	path := "/api/agents/" + e.AgentID + "/necessity"
	body := map[string]any{"skill_id": skill}
	w := httptest.NewRecorder()
	req := seatRequest("POST", path, e.WorkspaceID, body, "")
	req = withURLParam(req, "id", e.AgentID)
	testHandler.StartNecessityTest(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("start: status %d: %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = seatRequest("GET", path, e.WorkspaceID, nil, "")
	req = withURLParam(req, "id", e.AgentID)
	testHandler.ListNecessityTests(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"state":"running"`) || !strings.Contains(w.Body.String(), skill) {
		t.Fatalf("list: status %d body %s, want the running test on the skill", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = seatRequest("POST", path, e.WorkspaceID, body, "")
	req = withURLParam(req, "id", e.AgentID)
	testHandler.StartNecessityTest(w, req)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "necessity_test_running") {
		t.Fatalf("second start: status %d body %s, want 409 necessity_test_running", w.Code, w.Body.String())
	}
}

// TestNecessityRoute_StartInstructionBlock: a person starts a necessity test on one level-2 block
// of the seat's instructions (201, subject instruction_block). Sending both skill_id and
// instruction_block is refused (400), a heading the seat does not have is refused (422), and a
// second start on the same block is refused while the first runs (409, spec 10.2).
func TestNecessityRoute_StartInstructionBlock(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.DurableSeats, true)
	e := newDigestEnv(t)
	squad := e.squad(t, "necessity block route squad")
	dbfx.Exec(t, `UPDATE agent SET home_squad_id = $2, instructions = $3 WHERE id = $1`, e.AgentID, squad,
		"Giữ vai trò điều phối.\n\n## Checklist\nBước 1: kiểm tra.\n\n## Ghi chú\nKhác.")
	t.Cleanup(func() {
		ctx := context.Background()
		testPool.Exec(ctx, `DELETE FROM shadow_config WHERE agent_id = $1`, e.AgentID)
		testPool.Exec(ctx, `DELETE FROM necessity_test WHERE agent_id = $1`, e.AgentID)
	})

	path := "/api/agents/" + e.AgentID + "/necessity"
	post := func(body map[string]any) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := seatRequest("POST", path, e.WorkspaceID, body, "")
		req = withURLParam(req, "id", e.AgentID)
		testHandler.StartNecessityTest(w, req)
		return w
	}

	if w := post(map[string]any{"skill_id": "00000000-0000-0000-0000-000000000001", "instruction_block": "Checklist"}); w.Code != http.StatusBadRequest {
		t.Fatalf("both fields: status %d: %s, want 400", w.Code, w.Body.String())
	}
	if w := post(map[string]any{"instruction_block": "Không có"}); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("missing heading: status %d: %s, want 422", w.Code, w.Body.String())
	}
	w := post(map[string]any{"instruction_block": "Checklist"})
	if w.Code != http.StatusCreated || !strings.Contains(w.Body.String(), `"subject_type":"instruction_block"`) {
		t.Fatalf("start block: status %d body %s, want 201 instruction_block", w.Code, w.Body.String())
	}
	if w := post(map[string]any{"instruction_block": "Checklist"}); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "necessity_test_running") {
		t.Fatalf("second start: status %d body %s, want 409 necessity_test_running", w.Code, w.Body.String())
	}
}
