package console

import (
	"context"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/quota"
)

// Spec R6 section 3c.5 (QT7): staff create a request for a workspace in its
// scope (M48), see requests by scope (M46), and only system_admin decides (M47).

const qrID = "7c000000-0000-0000-0000-000000000001"

// fakeQuota records the calls and answers with its fields.
type fakeQuota struct {
	created  []quota.Actor
	decided  []string
	err      error
	lastGate string
}

func (f *fakeQuota) Create(_ context.Context, by quota.Actor, ws, tenant, gate string, requested int64, reason string) (quota.Request, error) {
	f.created = append(f.created, by)
	f.lastGate = gate
	if f.err != nil {
		return quota.Request{}, f.err
	}
	return quota.Request{ID: qrID, WorkspaceID: ws, TenantID: tenant, Gate: gate, RequestedValue: requested, State: quota.StatePending, Reason: reason, CreatedAt: t0}, nil
}

func (f *fakeQuota) Approve(_ context.Context, _ quota.Actor, id string, _ *int64, _ string) (quota.Request, error) {
	f.decided = append(f.decided, "approve "+id)
	if f.err != nil {
		return quota.Request{}, f.err
	}
	return quota.Request{ID: id, State: quota.StateApproved, CreatedAt: t0}, nil
}

func (f *fakeQuota) Deny(_ context.Context, _ quota.Actor, id, _ string) (quota.Request, error) {
	f.decided = append(f.decided, "deny "+id)
	if f.err != nil {
		return quota.Request{}, f.err
	}
	return quota.Request{ID: id, State: quota.StateDenied, CreatedAt: t0}, nil
}

func TestQuotaCreateIsScopedAndForwardsTheGate(t *testing.T) {
	e := newEnv(t)
	path := "/api/platform/workspaces/" + wsID + "/quota-requests"
	rec := e.do(tadminID, "POST", path, `{"gate":"max_workspaces","requested_value":5,"reason":"growing team"}`)
	wantStatus(t, rec, http.StatusCreated)
	if len(e.quota.created) != 1 || e.quota.lastGate != "max_workspaces" || e.quota.created[0].ID != tadminID {
		t.Fatalf("quota calls = %+v", e.quota.created)
	}
	wantUnknownRouteOrNotFound(t, e.do(tadminID, "POST", "/api/platform/workspaces/"+foreignWSID+"/quota-requests", `{"gate":"max_workspaces","requested_value":5,"reason":"r"}`))
	wantUnknownRouteOrNotFound(t, e.do(suppID, "POST", path, `{"gate":"max_workspaces","requested_value":5,"reason":"r"}`))
}

func TestQuotaCreateReportsAPendingRequestAsConflict(t *testing.T) {
	e := newEnv(t)
	e.quota.err = &quota.Error{Status: http.StatusConflict, Code: quota.CodePending, Message: "a request for this gate is already pending"}
	wantCode(t, e.do(adminID, "POST", "/api/platform/workspaces/"+wsID+"/quota-requests", `{"gate":"max_workspaces","requested_value":5,"reason":"r"}`), 409, "quota_request_pending")
}

func TestQuotaApproveAndDenyAreSystemAdminOnly(t *testing.T) {
	e := newEnv(t)
	wantUnknownRouteOrNotFound(t, e.do(tadminID, "POST", "/api/platform/quota-requests/"+qrID+"/approve", `{"reason":"ok"}`))
	wantUnknownRouteOrNotFound(t, e.do(tadminID, "POST", "/api/platform/quota-requests/"+qrID+"/deny", `{"reason":"no"}`))
	if len(e.quota.decided) != 0 {
		t.Fatalf("an admin decided a request: %v", e.quota.decided)
	}
	wantStatus(t, e.do(adminID, "POST", "/api/platform/quota-requests/"+qrID+"/approve", `{"reason":"ok"}`), http.StatusOK)
	wantStatus(t, e.do(adminID, "POST", "/api/platform/quota-requests/"+qrID+"/deny", `{"reason":"no"}`), http.StatusOK)
	if len(e.quota.decided) != 2 {
		t.Fatalf("decisions = %v", e.quota.decided)
	}
}

func TestQuotaListIsScopedByTenant(t *testing.T) {
	e := newEnv(t)
	e.reader.quotaRows = []quota.Request{
		{ID: qrID, WorkspaceID: wsID, TenantID: tenantID, Gate: "max_workspaces", State: quota.StatePending, CreatedAt: t0},
		{ID: "7c000000-0000-0000-0000-000000000002", WorkspaceID: foreignWSID, TenantID: otherTenantID, Gate: "max_workspaces", State: quota.StatePending, CreatedAt: t0},
	}
	rec := e.do(tadminID, "GET", "/api/platform/quota-requests", "")
	wantStatus(t, rec, http.StatusOK)
	if rows, _ := decode(t, rec)["requests"].([]any); len(rows) != 1 {
		t.Fatalf("admin saw %d requests, want its tenant's only", len(rows))
	}
	wantUnknownRouteOrNotFound(t, e.do(suppID, "GET", "/api/platform/quota-requests", ""))
}
