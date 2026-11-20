package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/entitlement"
	"github.com/multica-ai/multica/server/internal/entitlement/entitlementtest"
	"github.com/multica-ai/multica/server/internal/membergroup"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// withAgentsLimit sets the plan limit of agents for one workspace, through the
// entitlement stub, and restores the provider when the test ends.
func withAgentsLimit(t *testing.T, wsID string, limit int) {
	t.Helper()
	withAgentsLimits(t, map[string]int{wsID: limit})
}

// withAgentsLimits sets the agents limit of several workspaces on one stub.
func withAgentsLimits(t *testing.T, limits map[string]int) {
	t.Helper()
	prior := testHandler.Entitlements
	stub := entitlementtest.New()
	for wsID, limit := range limits {
		limit := limit
		stub.Set(uuid.MustParse(wsID), entitlement.GateAgents, entitlement.Decision{
			Gate: entitlement.Gate{Action: entitlement.ActionEnforce, Limit: &limit},
		})
	}
	testHandler.Entitlements = stub
	t.Cleanup(func() { testHandler.Entitlements = prior })
}

func quotaRequestCall(t *testing.T, wsID string, body map[string]any) *testutil.Response {
	t.Helper()
	req := withURLParam(newRequest("POST", "/api/workspaces/"+wsID+"/quota-requests", body), "id", wsID)
	return testutil.Call(t, testHandler.CreateQuotaRequest, req)
}

// The owner asks for a higher limit: one pending row, at the effective limit.
func TestCreateQuotaRequestByOwner(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ws := dbfx.Workspace(t, "quota owner", "quota-owner-"+uuid.NewString()[:8])
	dbfx.Member(t, ws, testUserID, "owner")
	withAgentsLimit(t, ws, 10)

	quotaRequestCall(t, ws, map[string]any{"gate": "agents", "requested_value": 20, "reason": "the team needs more agents"}).Want(http.StatusCreated)
	if n := dbfx.Count(t, `SELECT count(*) FROM quota_request WHERE workspace_id = $1::uuid AND gate = 'agents' AND state = 'pending' AND requested_value = 20 AND current_value = 10`, ws); n != 1 {
		t.Fatalf("pending quota rows = %d, want 1", n)
	}
}

// A value that does not exceed the effective limit is a 400 naming the field.
func TestQuotaRequestValueMustExceedEffectiveLimit(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ws := dbfx.Workspace(t, "quota low", "quota-low-"+uuid.NewString()[:8])
	dbfx.Member(t, ws, testUserID, "owner")
	withAgentsLimit(t, ws, 10)

	w := quotaRequestCall(t, ws, map[string]any{"gate": "agents", "requested_value": 10, "reason": "same as now"}).Want(http.StatusBadRequest)
	if body := w.Body.String(); !quotaBodyHas(body, "quota_request_invalid", "requested_value") {
		t.Fatalf("body = %s, want quota_request_invalid naming requested_value", body)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM quota_request WHERE workspace_id = $1::uuid`, ws); n != 0 {
		t.Fatalf("rows = %d after a refused request, want 0", n)
	}
}

// One pending request per gate: a second one is 409 quota_request_pending.
func TestDuplicatePendingIs409(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ws := dbfx.Workspace(t, "quota dup", "quota-dup-"+uuid.NewString()[:8])
	dbfx.Member(t, ws, testUserID, "owner")
	withAgentsLimit(t, ws, 10)

	body := map[string]any{"gate": "agents", "requested_value": 20, "reason": "first ask for agents"}
	quotaRequestCall(t, ws, body).Want(http.StatusCreated)
	w := quotaRequestCall(t, ws, body).Want(http.StatusConflict)
	if !quotaBodyHas(w.Body.String(), "quota_request_pending") {
		t.Fatalf("body = %s, want quota_request_pending", w.Body.String())
	}
}

// The list shows only the workspace's own requests.
func TestListQuotaRequestsScopedToWorkspace(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	mine := dbfx.Workspace(t, "quota list mine", "quota-mine-"+uuid.NewString()[:8])
	other := dbfx.Workspace(t, "quota list other", "quota-other-"+uuid.NewString()[:8])
	dbfx.Member(t, mine, testUserID, "owner")
	dbfx.Member(t, other, testUserID, "owner")
	withAgentsLimits(t, map[string]int{mine: 10, other: 10})
	quotaRequestCall(t, mine, map[string]any{"gate": "agents", "requested_value": 20, "reason": "list me"}).Want(http.StatusCreated)
	quotaRequestCall(t, other, map[string]any{"gate": "agents", "requested_value": 30, "reason": "not mine"}).Want(http.StatusCreated)

	req := withURLParam(newRequest("GET", "/api/workspaces/"+mine+"/quota-requests", nil), "id", mine)
	var out struct {
		QuotaRequests []struct {
			RequestedValue int64 `json:"requested_value"`
		} `json:"quota_requests"`
	}
	testutil.Call(t, testHandler.ListQuotaRequests, req).Want(http.StatusOK).JSON(&out)
	if len(out.QuotaRequests) != 1 || out.QuotaRequests[0].RequestedValue != 20 {
		t.Fatalf("list = %+v, want only the workspace's own request", out.QuotaRequests)
	}
}

// Deleting the workspace removes its quota requests (spec R6 3c.5, manifest keep/delete).
func TestDeleteWorkspaceRemovesQuotaRequests(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ws := dbfx.Workspace(t, "quota delete", "quota-del-"+uuid.NewString()[:8])
	dbfx.Member(t, ws, testUserID, "owner")
	withAgentsLimit(t, ws, 10)
	quotaRequestCall(t, ws, map[string]any{"gate": "agents", "requested_value": 20, "reason": "delete me"}).Want(http.StatusCreated)

	done, err := testHandler.DeleteWorkspaceForStaff(context.Background(), ws, staffOrigin(stagedStaffRequest(t, "workspace", ws, "")), nil)
	if err != nil || !done {
		t.Fatalf("DeleteWorkspaceForStaff = (%v, %v), want (true, nil)", done, err)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM quota_request WHERE workspace_id = $1::uuid`, ws); n != 0 {
		t.Fatalf("quota rows = %d after the workspace is deleted, want 0", n)
	}
}

func quotaBodyHas(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

// quotaCreateRouter mounts CreateQuotaRequest behind the same guards as the
// router's billing.manage group: the permission key, then the human-actor gate.
func quotaCreateRouter() http.Handler {
	r := chi.NewRouter()
	r.Route("/api/workspaces/{id}", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireWorkspacePermissionFromURL(testHandler.Queries, "id", membergroup.KeyBillingManage))
			r.Use(RequireHumanActor)
			r.Post("/quota-requests", testHandler.CreateQuotaRequest)
		})
	})
	return r
}

// A plain member cannot file a quota request: billing.manage refuses it.
func TestCreateQuotaRequestByPlainMemberIsPermissionDenied(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ws := dbfx.Workspace(t, "quota member", "quota-member-"+uuid.NewString()[:8])
	dbfx.Member(t, ws, testUserID, "member")
	withAgentsLimit(t, ws, 10)

	req := newRequest("POST", "/api/workspaces/"+ws+"/quota-requests", map[string]any{"gate": "agents", "requested_value": 20, "reason": "more agents"})
	w := httptest.NewRecorder()
	quotaCreateRouter().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a plain member; body = %s", w.Code, w.Body.String())
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM quota_request WHERE workspace_id = $1::uuid`, ws); n != 0 {
		t.Fatalf("rows = %d after a refused request, want 0", n)
	}
}

// A machine credential (task token) is refused even when its owner could file.
func TestQuotaRequestMachineTokenIs403(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ws := dbfx.Workspace(t, "quota machine", "quota-machine-"+uuid.NewString()[:8])
	dbfx.Member(t, ws, testUserID, "owner")
	withAgentsLimit(t, ws, 10)

	req := newRequest("POST", "/api/workspaces/"+ws+"/quota-requests", map[string]any{"gate": "agents", "requested_value": 20, "reason": "from a task"})
	req.Header.Set("X-Actor-Source", "task_token")
	w := httptest.NewRecorder()
	quotaCreateRouter().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a machine token; body = %s", w.Code, w.Body.String())
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM quota_request WHERE workspace_id = $1::uuid`, ws); n != 0 {
		t.Fatalf("rows = %d after a refused request, want 0", n)
	}
}
