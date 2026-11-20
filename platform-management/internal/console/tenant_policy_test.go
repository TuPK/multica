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

// The read names the fields the floor sets, so the console can show which
// restrictions come from the tenant (spec R6 section 3c.4). effective_policy is
// the floor every workspace is held to.
func TestTenantPolicyReadReportsSourcesAndTenantFields(t *testing.T) {
	e := tenantPolicyEnv(t)
	e.reader.policies[tenantID] = tenantpolicy.Stored{
		Policy:  tenantpolicy.Policy{AllowedInviteDomains: []string{"example.com"}, ShareLinksEnabled: true, RequireSSO: true},
		Version: 1,
	}

	rec := e.do(suppID, "GET", "/api/platform/tenants/"+tenantID+"/policy", "")
	wantStatus(t, rec, http.StatusOK)
	body := decode(t, rec)

	fields, _ := body["tenant_fields"].([]any)
	if len(fields) != 2 || fields[0] != "allowed_invite_domains" || fields[1] != "require_sso" {
		t.Fatalf("tenant_fields = %v, want [allowed_invite_domains require_sso]", body["tenant_fields"])
	}
	sources, _ := body["sources"].(map[string]any)
	if sources["require_sso"] != "tenant" || sources["require_mfa"] != "default" {
		t.Fatalf("sources = %v", sources)
	}
	eff, _ := body["effective_policy"].(map[string]any)
	if eff["require_sso"] != true {
		t.Fatalf("effective_policy = %v", eff)
	}
}
