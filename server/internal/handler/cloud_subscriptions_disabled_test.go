package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/multica-ai/multica/server/internal/cloudruntime"
	"github.com/multica-ai/multica/server/internal/featureflags"
)

// TestCloudSubscriptionRoutesAnswerAlikeWithoutCloudURL is AC48: with
// MULTICA_CLOUD_URL empty, every route of /api/cloud-subscriptions answers with
// the status and error code of the routes that existed before the platform
// management routes (the owner-credit billing route and the seven first
// subscription routes), and none reaches the upstream.
func TestCloudSubscriptionRoutesAnswerAlikeWithoutCloudURL(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.BillingWorkspaceSubscriptions, true)

	type route struct {
		name, method, path string
		body               any
		invoke             func(http.ResponseWriter, *http.Request)
	}
	h := testHandler
	const sub = "/api/cloud-subscriptions"
	existing := []route{
		{"summary", "GET", sub + "/summary", nil, h.GetCloudWorkspaceSubscriptionSummary},
		{"prices", "GET", sub + "/prices", nil, h.GetCloudWorkspaceSubscriptionPrices},
		{"checkout", "POST", sub + "/checkout-sessions", map[string]any{"interval": "month", "idempotency_key": "k1"}, h.CreateCloudWorkspaceSubscriptionCheckout},
		{"seat preview", "POST", sub + "/seats/purchase-preview", map[string]any{"additional_seats": 1}, h.PreviewCloudWorkspaceSubscriptionSeatPurchase},
		{"seat purchase", "POST", sub + "/seats/purchases", map[string]any{
			"additional_seats": 1, "expected_current_seats": 5, "expected_purchase_version": 1,
			"accepted_proration_amount": 100, "currency": "vnd", "idempotency_key": "k2",
		}, h.PurchaseCloudWorkspaceSubscriptionSeats},
		{"seat reconcile", "POST", sub + "/seats/reconcile", nil, h.ReconcileCloudWorkspaceSubscriptionSeats},
		{"portal", "POST", sub + "/portal-sessions", nil, h.CreateCloudWorkspaceSubscriptionPortal},
	}
	added := []route{
		{"usage", "GET", sub + "/usage", nil, h.GetCloudWorkspaceSubscriptionUsage},
		{"billing profile", "PUT", sub + "/billing-profile", map[string]any{"account_kind": "individual"}, h.UpdateCloudWorkspaceBillingProfile},
		{"plan change preview", "POST", sub + "/plan-changes/preview", map[string]any{"plan_key": "pro", "interval": "month", "seats": 3}, h.PreviewCloudWorkspaceSubscriptionPlanChange},
		{"plan change", "POST", sub + "/plan-changes", map[string]any{"plan_key": "pro", "interval": "month", "seats": 3}, h.CreateCloudWorkspaceSubscriptionPlanChange},
		{"cancel scheduled plan change", "DELETE", sub + "/plan-changes/scheduled", nil, h.CancelCloudWorkspaceSubscriptionScheduledPlanChange},
		{"cancel", "POST", sub + "/cancel", nil, h.CancelCloudWorkspaceSubscription},
		{"resume", "POST", sub + "/resume", nil, h.ResumeCloudWorkspaceSubscription},
	}

	type answer struct {
		status int
		code   string
	}
	call := func(t *testing.T, rt route) answer {
		t.Helper()
		req := withCloudSubscriptionWorkspace(newRequest(rt.method, rt.path, rt.body), "owner")
		req.Header.Set(idempotencyKeyHeader, "disabled-request-1")
		w := httptest.NewRecorder()
		rt.invoke(w, req)
		var body struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		return answer{w.Code, body.Code}
	}

	proxies := map[string]cloudRuntimeProxy{
		"empty client":   cloudruntime.NewClient(cloudruntime.Config{}),
		"disabled proxy": &fakeCloudRuntimeProxy{enabled: false},
		"no proxy":       nil,
	}
	for label, proxy := range proxies {
		t.Run(label, func(t *testing.T) {
			useCloudRuntimeProxy(t, proxy)

			// The owner-credit route is the reference of the original surface.
			w := httptest.NewRecorder()
			h.GetCloudBillingBalance(w, newRequest(http.MethodGet, "/api/cloud-billing/balance", nil))
			var ref struct {
				Code string `json:"code"`
			}
			_ = json.Unmarshal(w.Body.Bytes(), &ref)
			want := answer{w.Code, ref.Code}
			if want.status != http.StatusForbidden || want.code != "cloud_runtime_not_configured" {
				t.Fatalf("reference answer = %+v, body %s", want, w.Body.String())
			}

			for _, rt := range append(existing, added...) {
				if got := call(t, rt); got != want {
					t.Errorf("%s %s = %+v, want %+v", rt.method, rt.path, got, want)
				}
			}
		})
	}
}
