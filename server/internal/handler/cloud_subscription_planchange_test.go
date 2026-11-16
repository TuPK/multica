package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/cloudruntime"
	"github.com/multica-ai/multica/server/internal/featureflags"
)

// The self-service plan-change routes (spec 7b.1, rules 5 to 8) forward to the
// platform's plan-changes, cancel and resume routes for the workspace in the path.
func TestCloudSubscriptionPlanChangeRoutesUseScopedPaths(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.BillingWorkspaceSubscriptions, true)

	cases := []struct {
		name       string
		method     string
		path       string
		role       string
		body       any
		key        string
		wantPath   string
		wantBody   map[string]any
		invoke     func(http.ResponseWriter, *http.Request)
		wantHeader bool
	}{
		{
			name: "owner previews a plan change without a key", method: http.MethodPost,
			path: "/api/cloud-subscriptions/plan-changes/preview", role: "owner",
			body:     map[string]any{"plan_key": "unlimited", "interval": "month", "seats": 0, "workspace_id": "smuggled"},
			wantPath: "/api/v1/subscriptions/" + testWorkspaceID + "/plan-changes/preview",
			wantBody: map[string]any{"plan_key": "unlimited", "interval": "month", "seats": float64(0)},
			invoke:   testHandler.PreviewCloudWorkspaceSubscriptionPlanChange,
		},
		{
			name: "admin applies a quoted upgrade", method: http.MethodPost,
			path: "/api/cloud-subscriptions/plan-changes", role: "admin", key: "pc-1",
			body:     map[string]any{"plan_key": "unlimited", "interval": "month", "accepted_amount": 333333, "expected_version": 4},
			wantPath: "/api/v1/subscriptions/" + testWorkspaceID + "/plan-changes",
			wantBody: map[string]any{"plan_key": "unlimited", "interval": "month", "seats": float64(0),
				"accepted_amount": float64(333333), "expected_version": float64(4), "idempotency_key": "pc-1"},
			invoke: testHandler.CreateCloudWorkspaceSubscriptionPlanChange, wantHeader: true,
		},
		{
			name: "owner withdraws the scheduled change", method: http.MethodDelete,
			path: "/api/cloud-subscriptions/plan-changes/scheduled", role: "owner", key: "clear-1",
			wantPath:   "/api/v1/subscriptions/" + testWorkspaceID + "/plan-changes/scheduled",
			invoke:     testHandler.CancelCloudWorkspaceSubscriptionScheduledPlanChange,
			wantHeader: true,
		},
		{
			name: "admin cancels at the period end", method: http.MethodPost,
			path: "/api/cloud-subscriptions/cancel", role: "admin", key: "cancel-1",
			wantPath:   "/api/v1/subscriptions/" + testWorkspaceID + "/cancel",
			invoke:     testHandler.CancelCloudWorkspaceSubscription,
			wantHeader: true,
		},
		{
			name: "owner resumes", method: http.MethodPost,
			path: "/api/cloud-subscriptions/resume", role: "owner", key: "resume-1",
			wantPath:   "/api/v1/subscriptions/" + testWorkspaceID + "/resume",
			invoke:     testHandler.ResumeCloudWorkspaceSubscription,
			wantHeader: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			proxy := &fakeCloudRuntimeProxy{enabled: true, resp: &cloudruntime.Response{StatusCode: http.StatusOK, Body: []byte(`{"ok":true}`)}}
			useCloudRuntimeProxy(t, proxy)

			req := withCloudSubscriptionWorkspace(newRequest(tc.method, tc.path, tc.body), tc.role)
			if tc.key != "" {
				req.Header.Set(idempotencyKeyHeader, tc.key)
			}
			w := httptest.NewRecorder()
			tc.invoke(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
			}
			if proxy.req.Method != tc.method || proxy.req.Path != tc.wantPath {
				t.Fatalf("upstream = %s %s, want %s %s", proxy.req.Method, proxy.req.Path, tc.method, tc.wantPath)
			}
			if proxy.req.UserID != testUserID || proxy.req.Op != "billing" {
				t.Fatalf("upstream user %q op %q", proxy.req.UserID, proxy.req.Op)
			}
			if tc.wantHeader && proxy.req.Headers.Get(idempotencyKeyHeader) != tc.key {
				t.Fatalf("upstream Idempotency-Key = %q, want %q", proxy.req.Headers.Get(idempotencyKeyHeader), tc.key)
			}
			if tc.wantBody != nil {
				var got map[string]any
				if err := json.Unmarshal(proxy.req.Body, &got); err != nil {
					t.Fatalf("upstream body %q: %v", proxy.req.Body, err)
				}
				if _, smuggled := got["workspace_id"]; smuggled {
					t.Fatalf("upstream body carries a client workspace id: %v", got)
				}
				for k, v := range tc.wantBody {
					if got[k] != v {
						t.Fatalf("upstream body[%s] = %v, want %v (body %v)", k, got[k], v, got)
					}
				}
			}
		})
	}
}

// Every write needs an Idempotency-Key and every body a plan key, an interval and
// non-negative numbers; a bad request never reaches the platform.
func TestCloudSubscriptionPlanChangeValidatesLocally(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.BillingWorkspaceSubscriptions, true)

	cases := []struct {
		name   string
		method string
		path   string
		body   any
		key    string
		invoke func(http.ResponseWriter, *http.Request)
	}{
		{"preview without plan_key", http.MethodPost, "/api/cloud-subscriptions/plan-changes/preview",
			map[string]any{"interval": "month"}, "", testHandler.PreviewCloudWorkspaceSubscriptionPlanChange},
		{"preview with a bad interval", http.MethodPost, "/api/cloud-subscriptions/plan-changes/preview",
			map[string]any{"plan_key": "pro", "interval": "week"}, "", testHandler.PreviewCloudWorkspaceSubscriptionPlanChange},
		{"preview with negative seats", http.MethodPost, "/api/cloud-subscriptions/plan-changes/preview",
			map[string]any{"plan_key": "pro", "interval": "month", "seats": -1}, "", testHandler.PreviewCloudWorkspaceSubscriptionPlanChange},
		{"change without a key", http.MethodPost, "/api/cloud-subscriptions/plan-changes",
			map[string]any{"plan_key": "pro", "interval": "month"}, "", testHandler.CreateCloudWorkspaceSubscriptionPlanChange},
		{"change with a negative amount", http.MethodPost, "/api/cloud-subscriptions/plan-changes",
			map[string]any{"plan_key": "pro", "interval": "month", "accepted_amount": -5}, "pc-x", testHandler.CreateCloudWorkspaceSubscriptionPlanChange},
		{"withdraw without a key", http.MethodDelete, "/api/cloud-subscriptions/plan-changes/scheduled",
			nil, "", testHandler.CancelCloudWorkspaceSubscriptionScheduledPlanChange},
		{"cancel without a key", http.MethodPost, "/api/cloud-subscriptions/cancel", nil, "", testHandler.CancelCloudWorkspaceSubscription},
		{"resume without a key", http.MethodPost, "/api/cloud-subscriptions/resume", nil, "", testHandler.ResumeCloudWorkspaceSubscription},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			proxy := &fakeCloudRuntimeProxy{enabled: true}
			useCloudRuntimeProxy(t, proxy)
			req := withCloudSubscriptionWorkspace(newRequest(tc.method, tc.path, tc.body), "owner")
			if tc.key != "" {
				req.Header.Set(idempotencyKeyHeader, tc.key)
			}
			w := httptest.NewRecorder()
			tc.invoke(w, req)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
			}
			if proxy.req.Path != "" {
				t.Fatalf("the platform was called: %s %s", proxy.req.Method, proxy.req.Path)
			}
		})
	}
}

// Members and viewers cannot write; the role check runs before the platform.
func TestCloudSubscriptionPlanChangeRoleChecks(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.BillingWorkspaceSubscriptions, true)

	proxy := &fakeCloudRuntimeProxy{enabled: true, resp: &cloudruntime.Response{StatusCode: http.StatusOK, Body: []byte(`{}`)}}
	useCloudRuntimeProxy(t, proxy)
	for _, role := range []string{"member", "viewer"} {
		req := withCloudSubscriptionWorkspace(newRequest(http.MethodPost, "/api/cloud-subscriptions/cancel", nil), role)
		req.Header.Set(idempotencyKeyHeader, "cancel-role")
		w := httptest.NewRecorder()
		testHandler.CancelCloudWorkspaceSubscription(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s: status = %d, want 403: %s", role, w.Code, w.Body.String())
		}
		req = withCloudSubscriptionWorkspace(newRequest(http.MethodPost, "/api/cloud-subscriptions/plan-changes/preview",
			map[string]any{"plan_key": "pro", "interval": "month"}), role)
		w = httptest.NewRecorder()
		testHandler.PreviewCloudWorkspaceSubscriptionPlanChange(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s: preview status = %d, want 403", role, w.Code)
		}
	}
	if proxy.req.Path != "" {
		t.Fatalf("a refused request reached the platform: %s", proxy.req.Path)
	}
}

// A refusal from the platform, such as a status outside the cancel table, is
// returned with its status and code as it is.
func TestCloudSubscriptionCancelPassesTheRefusalThrough(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.BillingWorkspaceSubscriptions, true)

	body := `{"error":"only an active or trialing subscription can be cancelled","code":"subscription_not_cancelable"}`
	proxy := &fakeCloudRuntimeProxy{enabled: true, resp: &cloudruntime.Response{StatusCode: http.StatusConflict, Body: []byte(body)}}
	useCloudRuntimeProxy(t, proxy)
	req := withCloudSubscriptionWorkspace(newRequest(http.MethodPost, "/api/cloud-subscriptions/cancel", nil), "owner")
	req.Header.Set(idempotencyKeyHeader, "cancel-409")
	w := httptest.NewRecorder()
	testHandler.CancelCloudWorkspaceSubscription(w, req)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "subscription_not_cancelable") {
		t.Fatalf("status %d, body %s", w.Code, w.Body.String())
	}
}
