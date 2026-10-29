package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/util"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

func TestRuntimeQuickstartLifecycle(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	runtimeID, ownerID, _ := runtimeVisibilityFixture(t)

	// The fixture runtime has no daemon; give it one so daemon-token requests
	// match the runtime they claim.
	const daemonID = "quickstart-test-daemon"
	if _, err := testPool.Exec(ctx, `UPDATE agent_runtime SET daemon_id = $2 WHERE id = $1`, runtimeID, daemonID); err != nil {
		t.Fatalf("set runtime daemon id: %v", err)
	}
	daemonRequest := func(method string, body any) *http.Request {
		req := newDaemonTokenRequest(method, "/", body, testWorkspaceID, daemonID)
		return req.WithContext(middleware.WithDaemonTokenOwnerID(req.Context(), ownerID))
	}

	// 1. The owner queues a quickstart.
	w := httptest.NewRecorder()
	testHandler.InitiateRuntimeQuickstart(w, withURLParam(
		newRequestAs(ownerID, http.MethodPost, "/api/runtimes/"+runtimeID+"/quickstart", nil),
		"runtimeId", runtimeID,
	))
	if w.Code != http.StatusOK {
		t.Fatalf("initiate: status %d: %s", w.Code, w.Body.String())
	}
	var created QuickstartRequestResponse
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Status != "pending" {
		t.Fatalf("initiate status = %q, want pending", created.Status)
	}

	// 2. A second request is refused while the first is live.
	w = httptest.NewRecorder()
	testHandler.InitiateRuntimeQuickstart(w, withURLParam(
		newRequestAs(ownerID, http.MethodPost, "/api/runtimes/"+runtimeID+"/quickstart", nil),
		"runtimeId", runtimeID,
	))
	if w.Code != http.StatusConflict {
		t.Fatalf("second initiate: status %d, want 409", w.Code)
	}

	// 3. The next heartbeat claims it, and only once.
	runtimeUUID, err := util.ParseUUID(runtimeID)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := testHandler.claimPendingQuickstart(ctx, runtimeUUID)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if pending == nil || pending.ID != created.ID {
		t.Fatalf("claim = %+v, want request %s", pending, created.ID)
	}
	if len(pending.Bundle.Plugins) == 0 {
		t.Fatal("claimed request carries no bundle")
	}
	again, err := testHandler.claimPendingQuickstart(ctx, runtimeUUID)
	if err != nil || again != nil {
		t.Fatalf("second claim = %+v, %v; want nothing", again, err)
	}

	// 4. The daemon reads the skills it needs. The test workspace has none of
	// the bundle's skills, so each one comes back as missing.
	w = httptest.NewRecorder()
	testHandler.GetRuntimeQuickstartSkills(w, withURLParams(
		daemonRequest(http.MethodGet, nil),
		"runtimeId", runtimeID, "requestId", created.ID,
	))
	if w.Code != http.StatusOK {
		t.Fatalf("skills: status %d: %s", w.Code, w.Body.String())
	}
	var bundles protocol.QuickstartSkillBundles
	if err := json.Unmarshal(w.Body.Bytes(), &bundles); err != nil {
		t.Fatal(err)
	}
	if len(bundles.Missing) != len(pending.Bundle.Skills) {
		t.Fatalf("missing = %v, want every bundle skill %v", bundles.Missing, pending.Bundle.Skills)
	}

	// 5. The daemon reports its steps; the owner reads them back.
	steps := []protocol.QuickstartStep{{Name: "claude code", Status: protocol.QuickstartStepAlreadyInstalled}}
	w = httptest.NewRecorder()
	testHandler.ReportRuntimeQuickstartResult(w, withURLParams(
		daemonRequest(http.MethodPost, map[string]any{"status": "completed", "steps": steps}),
		"runtimeId", runtimeID, "requestId", created.ID,
	))
	if w.Code != http.StatusOK {
		t.Fatalf("report: status %d: %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	testHandler.GetRuntimeQuickstart(w, withURLParams(
		newRequestAs(ownerID, http.MethodGet, "/api/runtimes/"+runtimeID+"/quickstart/"+created.ID, nil),
		"runtimeId", runtimeID, "requestId", created.ID,
	))
	var final QuickstartRequestResponse
	if err := json.Unmarshal(w.Body.Bytes(), &final); err != nil {
		t.Fatal(err)
	}
	if final.Status != "completed" || len(final.Steps) != 1 || final.Steps[0].Name != "claude code" {
		t.Fatalf("final = %+v, want completed with the reported steps", final)
	}
}
