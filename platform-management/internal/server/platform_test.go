package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/billingapi"
	"github.com/multica-ai/multica/platform-management/internal/console"
	"github.com/multica-ai/multica/platform-management/internal/entitlement"
	"github.com/multica-ai/multica/platform-management/internal/seat"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
	"github.com/multica-ai/multica/platform-management/internal/trust"
)

func namedHandler(name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, name)
	})
}

// Every console route reaches the console handler, whatever the method, and
// every other path under /api/platform/ stays with the sign-in handler.
func TestPlatformHandlerRoutesConsolePathsToTheConsole(t *testing.T) {
	h := NewPlatformHandler(namedHandler("auth"), namedHandler("console"), namedHandler("billing"))
	const id = "11111111-2222-3333-4444-555555555555"

	patterns := console.Patterns()
	if len(patterns) == 0 {
		t.Fatal("the console has no routes")
	}
	for _, p := range patterns {
		path := strings.NewReplacer("{id}", id, "{user_id}", id).Replace(p)
		for _, method := range []string{"GET", "POST", "PUT", "DELETE"} {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
			if got := rec.Body.String(); got != "console" {
				t.Errorf("%s %s reached %q, want the console", method, path, got)
			}
		}
	}

	for _, path := range []string{
		"/api/platform/me",
		"/api/platform/auth/send-code",
		"/api/platform/tenants/",
		"/api/platform/tenants/" + id + "/unknown",
		"/api/platform/plans/",
		"/api/platform/users/" + id + "/mfa-reset",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if got := rec.Body.String(); got != "auth" {
			t.Errorf("GET %s reached %q, want the sign-in handler", path, got)
		}
	}
}

// Through the API listener's handler, an unmatched console path is the same 404
// as any other.
func TestAPIHandlerKeepsOtherPathsOutOfThePlatform(t *testing.T) {
	api := NewAPIHandler(trust.New(trust.DefaultCIDRs()), HealthHandler(),
		NewPlatformHandler(namedHandler("auth"), namedHandler("console"), namedHandler("billing")))
	for path, want := range map[string]int{
		"/api/platform/overview": 200,
		"/api/v1/anything":       404,
		"/overview":              404,
	} {
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != want {
			t.Errorf("GET %s = %d, want %d", path, rec.Code, want)
		}
	}
}

// The plan and subscription routes reach the billing handler, whatever the
// method, and the console's routes still reach the console.
func TestPlatformHandlerRoutesPlanAndSubscriptionPathsToBilling(t *testing.T) {
	h := NewPlatformHandler(namedHandler("auth"), namedHandler("console"), namedHandler("billing"))
	const id = "11111111-2222-3333-4444-555555555555"

	patterns := subscription.ConsolePatterns()
	if len(patterns) == 0 {
		t.Fatal("billing has no routes")
	}
	for _, p := range patterns {
		path := strings.NewReplacer("{id}", id, "{key}", "pro").Replace(p)
		for _, method := range []string{"GET", "POST", "PUT", "DELETE"} {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
			if got := rec.Body.String(); got != "billing" {
				t.Errorf("%s %s reached %q, want billing", method, path, got)
			}
		}
	}

	// A tenant path that is not the subscription's still goes to the console.
	for _, path := range []string{"/api/platform/tenants/" + id, "/api/platform/tenants/" + id + "/status", "/api/platform/tenants/" + id + "/notes"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if got := rec.Body.String(); got != "console" {
			t.Errorf("GET %s reached %q, want the console", path, got)
		}
	}
}

func apiRequest(h http.Handler, method, path, remote, user string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = remote
	if user != "" {
		req.Header.Set(trust.UserHeader, user)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAPIHandlerMountsTheInternalRoutes(t *testing.T) {
	api := NewAPIHandler(trust.New(trust.DefaultCIDRs()), HealthHandler(), namedHandler("platform"),
		Route{Pattern: "GET /api/v1/internal/entitlement-policies/{workspaceId}", Handler: namedHandler("policy")},
		Route{Pattern: "GET /api/v1/subscriptions/{ws}/summary", Handler: namedHandler("summary")},
	)
	const id = "11111111-2222-3333-4444-555555555555"
	for path, want := range map[string]string{
		"/api/v1/internal/entitlement-policies/" + id: "policy",
		"/api/v1/subscriptions/" + id + "/summary":    "summary",
		"/api/platform/overview":                      "platform",
	} {
		if rec := apiRequest(api, "GET", path, "10.0.0.7:1", ""); rec.Body.String() != want {
			t.Errorf("GET %s reached %q, want %q", path, rec.Body.String(), want)
		}
	}
	for _, path := range []string{"/api/v1/subscriptions/" + id + "/portal-sessions", "/api/v1/internal/other", "/"} {
		if rec := apiRequest(api, "GET", path, "10.0.0.7:1", ""); rec.Code != 404 {
			t.Errorf("GET %s = %d, want 404", path, rec.Code)
		}
	}
}

// The trust rule covers the whole API listener: a route cannot forget it.
func TestAPIHandlerRefusesXUserIDFromAnUntrustedPeerEverywhere(t *testing.T) {
	api := NewAPIHandler(trust.New(trust.DefaultCIDRs()), HealthHandler(), namedHandler("platform"),
		Route{Pattern: "GET /api/v1/internal/entitlement-policies/{workspaceId}", Handler: namedHandler("policy")},
	)
	const id = "11111111-2222-3333-4444-555555555555"
	const user = "99999999-2222-3333-4444-555555555555"
	for _, path := range []string{"/healthz", "/api/platform/overview", "/api/v1/internal/entitlement-policies/" + id, "/nothing"} {
		if rec := apiRequest(api, "GET", path, "203.0.113.9:1", user); rec.Code != 403 {
			t.Errorf("untrusted GET %s with X-User-ID = %d, want 403", path, rec.Code)
		}
		if rec := apiRequest(api, "GET", path, "10.0.0.7:1", user); rec.Code == 403 {
			t.Errorf("trusted GET %s with X-User-ID = 403", path)
		}
	}
	// The policy route needs no header: the client sends none.
	if rec := apiRequest(api, "GET", "/api/v1/internal/entitlement-policies/"+id, "203.0.113.9:1", ""); rec.Body.String() != "policy" {
		t.Errorf("an untrusted peer without the header = %d %q, want the policy route", rec.Code, rec.Body)
	}
}

// Every internal route the platform serves registers on one mux without a
// conflict, and each pattern of the billing routes reaches its own handler: the
// checkout path has no workspace segment, so it must not be taken for another
// route's.
func TestAPIHandlerMountsEveryBillingRoute(t *testing.T) {
	var routes []Route
	add := func(name string, patterns []string) {
		for _, p := range patterns {
			routes = append(routes, Route{Pattern: p, Handler: namedHandler(name + " " + p)})
		}
	}
	add("policy", []string{entitlement.PolicyPattern})
	add("reads", subscription.APIPatterns)
	add("billing", billingapi.Patterns)
	add("seat", seat.Patterns)
	api := NewAPIHandler(trust.New(trust.DefaultCIDRs()), HealthHandler(), namedHandler("platform"), routes...)

	const id = "11111111-2222-3333-4444-555555555555"
	for _, tc := range []struct{ method, path, want string }{
		{"POST", "/api/v1/subscriptions/checkout-sessions", "billing " + billingapi.CheckoutPattern},
		{"POST", "/api/v1/subscriptions/" + id + "/seats/purchase-preview", "billing " + billingapi.PreviewPattern},
		{"POST", "/api/v1/subscriptions/" + id + "/seats/purchases", "billing " + billingapi.PurchasePattern},
		{"POST", "/api/v1/subscriptions/" + id + "/seats/reconcile", "billing " + billingapi.ReconcilePattern},
		{"POST", "/api/v1/subscriptions/" + id + "/portal-sessions", "billing " + billingapi.PortalPattern},
		{"GET", "/api/v1/subscriptions/" + id + "/summary", "reads " + subscription.SummaryPattern},
		{"GET", "/api/v1/subscriptions/" + id + "/prices", "reads " + subscription.PricesPattern},
	} {
		if rec := apiRequest(api, tc.method, tc.path, "10.0.0.7:1", ""); rec.Body.String() != tc.want {
			t.Errorf("%s %s reached %q, want %q", tc.method, tc.path, rec.Body.String(), tc.want)
		}
	}
}
