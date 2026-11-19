package console

import (
	"context"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/tenantstatus"
)

// Spec R6 section 3c.9 (QT19, M52): a tenant lock is one call for every workspace,
// with the scope of the caller; billing gets the flat 404.

type fakeTenantStatus struct {
	calls []string
}

func (f *fakeTenantStatus) SetStatus(_ context.Context, by tenantstatus.Actor, tenant, status, reason string) ([]string, error) {
	f.calls = append(f.calls, by.ID+" "+tenant+" "+status+" "+reason)
	return []string{wsID}, nil
}

func TestTenantLockIsScopedToTheCaller(t *testing.T) {
	e := newEnv(t)
	path := "/api/platform/tenants/" + tenantID + "/status"
	wantStatus(t, e.do(tadminID, "POST", path, `{"status":"suspended","reason":"abuse"}`), http.StatusOK)
	wantStatus(t, e.do(suppID, "POST", path, `{"status":"read_only","reason":"review"}`), http.StatusOK)
	wantUnknownRouteOrNotFound(t, e.do(tadminID, "POST", "/api/platform/tenants/"+otherTenantID+"/status", `{"status":"suspended","reason":"r"}`))
	wantUnknownRouteOrNotFound(t, e.do(billID, "POST", path, `{"status":"suspended","reason":"r"}`))
	if len(e.tstat.calls) != 2 {
		t.Fatalf("status calls = %v", e.tstat.calls)
	}
}

func TestTenantLockRejectsPendingDeletionAndNoReason(t *testing.T) {
	e := newEnv(t)
	path := "/api/platform/tenants/" + tenantID + "/status"
	wantCode(t, e.do(adminID, "POST", path, `{"status":"pending_deletion","reason":"r"}`), 400, "invalid_status")
	wantCode(t, e.do(adminID, "POST", path, `{"status":"suspended","reason":""}`), 400, "reason_required")
}
