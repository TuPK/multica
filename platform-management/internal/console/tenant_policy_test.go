package console

import (
	"net/http"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/tenantpolicy"
)

// Spec R6 section 3c.4 (QT6): the tenant floor. Writing needs a plan feature in
// the tenant; a stale expected version is 409; each change is audited with the
// tenant id and the old and new values.

func tenantPolicyEnv(t *testing.T) *env {
	t.Helper()
	e := newEnv(t)
	e.reader.policies = map[string]tenantpolicy.Stored{}
	e.reader.policyFeature = true
	return e
}

func TestTenantPolicyNeedsTheWorkspacePoliciesFeature(t *testing.T) {
	e := tenantPolicyEnv(t)
	e.reader.policyFeature = false
	rec := e.do(tadminID, "PUT", "/api/platform/tenants/"+tenantID+"/policy", `{"require_sso":true}`)
	wantCode(t, rec, http.StatusPaymentRequired, "plan_feature_unavailable")
	if decode(t, rec)["feature"] != "workspace_policies" || len(e.store.tenantPolicy) != 0 {
		t.Fatalf("refused write: %s", rec.Body.String())
	}
}

func TestTenantPolicyVersionConflictWritesNothing(t *testing.T) {
	e := tenantPolicyEnv(t)
	e.store.tenantPolicy = map[string]tenantpolicy.Stored{tenantID: {Policy: tenantpolicy.Default(), Version: 3}}
	wantCode(t, e.do(tadminID, "PUT", "/api/platform/tenants/"+tenantID+"/policy", `{"expected_version":2,"require_sso":true}`), 409, "policy_version_conflict")
	if e.store.tenantPolicy[tenantID].Version != 3 || e.store.tenantPolicy[tenantID].RequireSSO {
		t.Fatal("a conflicting write changed the policy")
	}
}

func TestTenantPolicyWriteIsAuditedWithOldAndNew(t *testing.T) {
	e := tenantPolicyEnv(t)
	rec := e.do(tadminID, "PUT", "/api/platform/tenants/"+tenantID+"/policy", `{"require_sso":true,"allowed_invite_domains":["Example.com"]}`)
	wantStatus(t, rec, http.StatusOK)
	stored := e.store.tenantPolicy[tenantID]
	if !stored.RequireSSO || stored.Version != 1 || len(stored.AllowedInviteDomains) != 1 || stored.AllowedInviteDomains[0] != "example.com" {
		t.Fatalf("stored = %+v", stored)
	}
	found := false
	for _, a := range e.store.audits {
		if a.Action == "tenant.policy_changed" && a.TenantID == tenantID && a.ActorType == "staff" {
			found = true
		}
	}
	if !found {
		t.Fatal("no tenant.policy_changed audit row with the tenant id")
	}
}

func TestTenantPolicyRejectsBadDomainsAndAges(t *testing.T) {
	e := tenantPolicyEnv(t)
	wantCode(t, e.do(tadminID, "PUT", "/api/platform/tenants/"+tenantID+"/policy", `{"allowed_invite_domains":["user@example.com"]}`), 400, "invalid_policy")
	wantCode(t, e.do(tadminID, "PUT", "/api/platform/tenants/"+tenantID+"/policy", `{"session_max_age_hours":-1}`), 400, "invalid_policy")
}

func TestTenantPolicyOutsideTenantIsNotFound(t *testing.T) {
	e := tenantPolicyEnv(t)
	wantUnknownRouteOrNotFound(t, e.do(tadminID, "PUT", "/api/platform/tenants/"+otherTenantID+"/policy", `{"require_sso":true}`))
	wantUnknownRouteOrNotFound(t, e.do(tadminID, "GET", "/api/platform/tenants/"+otherTenantID+"/policy", ""))
}

func TestTenantPolicyReadDefaultsWithoutRow(t *testing.T) {
	e := tenantPolicyEnv(t)
	rec := e.do(suppID, "GET", "/api/platform/tenants/"+tenantID+"/policy", "")
	wantStatus(t, rec, http.StatusOK)
	if decode(t, rec)["version"] != float64(0) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}
