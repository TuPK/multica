package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/multica-ai/multica/server/internal/cloudruntime"
	"github.com/multica-ai/multica/server/internal/featureflags"
)

const platformUsageBody = `{"billing_model":"usage","usage":{"period_start":"2031-03-01T00:00:00Z","period_end":"2031-04-01T00:00:00Z","currency":"vnd","accrued_amount":123,"credit_limit":null,"unpaid_order_id":null,"meters":[]}}`

func TestCloudSubscriptionUsageProxiesPlatform(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.BillingWorkspaceSubscriptions, true)
	proxy := &fakeCloudRuntimeProxy{
		enabled: true,
		resp:    &cloudruntime.Response{StatusCode: http.StatusOK, Body: []byte(platformUsageBody)},
	}
	useCloudRuntimeProxy(t, proxy)

	for _, role := range []string{"owner", "admin"} {
		proxy.called = false
		w := httptest.NewRecorder()
		testHandler.GetCloudWorkspaceSubscriptionUsage(w, withCloudSubscriptionWorkspace(newRequest(http.MethodGet, "/api/cloud-subscriptions/usage", nil), role))
		if w.Code != http.StatusOK || w.Body.String() != platformUsageBody {
			t.Fatalf("%s: status = %d, body = %s", role, w.Code, w.Body.String())
		}
		if proxy.req.Method != http.MethodGet || proxy.req.Path != "/api/v1/subscriptions/"+testWorkspaceID+"/usage" || proxy.req.UserID != testUserID {
			t.Errorf("%s: upstream request = %+v", role, proxy.req)
		}
	}

	for _, role := range []string{"member", "viewer"} {
		proxy.called = false
		w := httptest.NewRecorder()
		testHandler.GetCloudWorkspaceSubscriptionUsage(w, withCloudSubscriptionWorkspace(newRequest(http.MethodGet, "/api/cloud-subscriptions/usage", nil), role))
		if w.Code != http.StatusForbidden || proxy.called {
			t.Errorf("%s: status = %d, upstream called = %v, want 403 and not called", role, w.Code, proxy.called)
		}
	}

	withFeatureFlag(t, testHandler, featureflags.BillingWorkspaceSubscriptions, false)
	proxy.called = false
	w := httptest.NewRecorder()
	testHandler.GetCloudWorkspaceSubscriptionUsage(w, withCloudSubscriptionWorkspace(newRequest(http.MethodGet, "/api/cloud-subscriptions/usage", nil), "owner"))
	if w.Code != http.StatusForbidden || proxy.called {
		t.Errorf("flag off: status = %d, upstream called = %v", w.Code, proxy.called)
	}
}
