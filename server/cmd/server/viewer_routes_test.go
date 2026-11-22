package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/multica-ai/multica/server/internal/analytics"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/realtime"
	"github.com/multica-ai/multica/server/internal/workspaceguard"
)

// AC19 (spec 6.6): a viewer is refused by every write route of a workspace,
// except the personal-state allow-list. chi.Walk hands each route its own
// middleware chain. Each route is mounted with that chain in front of a
// sentinel handler, so no real handler runs and nothing is written. A sentinel
// reached means the route admits the viewer.
//
// Database-backed: it runs only when the integration fixture is up.
func TestViewerIsRefusedEveryWorkspaceWriteRoute(t *testing.T) {
	if testPool == nil {
		t.Skip("integration database not available")
	}
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, `UPDATE member SET role = 'viewer' WHERE workspace_id = $1 AND user_id = $2`, testWorkspaceID, testUserID); err != nil {
		t.Fatalf("make the test user a viewer: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `UPDATE member SET role = 'owner' WHERE workspace_id = $1 AND user_id = $2`, testWorkspaceID, testUserID)
	})

	router := NewRouter(testPool, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil)
	checked := 0
	err := chi.Walk(router, func(method, route string, _ http.Handler, mws ...func(http.Handler) http.Handler) error {
		if !isWriteMethod(method) {
			return nil
		}
		checked++
		reached, status := serveAsViewer(method, route, mws)
		key := method + " " + route
		reason, exempt := viewerExemption(method, route)
		switch {
		case workspaceguard.ViewerAllowsWrite(method, route):
			if !reached {
				t.Errorf("%s: the allow-list admits a viewer, but the request was refused with %d", key, status)
			}
		case exempt:
			// Not a workspace write, or one a handler refuses itself. The reason
			// is printed so the exemption is visible in the test output.
			t.Logf("%s: not checked here: %s", key, reason)
		default:
			if reached {
				t.Errorf("%s: a viewer reached the handler of a workspace write route", key)
			} else if status != http.StatusForbidden && status != http.StatusUnauthorized {
				// 401 is an authentication refusal of a route that takes only a
				// machine credential (the public API): the viewer is still refused.
				t.Errorf("%s: a viewer was refused with %d, want 403", key, status)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk routes: %v", err)
	}
	if checked == 0 {
		t.Fatal("no write routes found; the walk is broken")
	}
}

// viewerWriteExceptions are write routes that the workspace guard does not
// cover, by "METHOD route". Each reason says why the route is outside the
// guard. Every entry is either the caller's own state or enforced by its handler,
// which refuses a viewer itself and has a named test.
var viewerWriteExceptions = map[string]string{
	"POST /api/me/star-prompt/claim":   "the caller's own star prompt, no workspace data",
	"POST /api/me/star-prompt/starred": "the caller's own star prompt, no workspace data",
	"POST /api/site/star/turns":        "public marketing footer star",
	"POST /auth/sso/mobile/exchange":   "SSO sign-in exchange, no workspace data",
	"POST /api/apps/mcp":               "apps gateway outside the workspace routes; verifies its own task token",
	"POST /api/apps/mcp/{app}":         "apps gateway outside the workspace routes; verifies its own task token",
	// User-scoped: the caller's own account, tokens, or sign-in. No workspace data.
	"POST /api/auth/refresh":                             "user session refresh",
	"POST /api/cli-token":                                "issues the caller's CLI token",
	"POST /api/client-usage":                             "records the caller's client usage",
	"POST /api/contact-sales":                            "public sales contact form",
	"POST /api/feedback":                                 "product feedback, not workspace data",
	"POST /api/tokens/":                                  "the caller's personal access tokens",
	"POST /api/tokens/current/renew":                     "the caller's current token",
	"DELETE /api/tokens/{id}":                            "the caller's personal access tokens",
	"PATCH /api/me":                                      "the caller's profile",
	"PATCH /api/me/onboarding":                           "the caller's onboarding state",
	"POST /api/me/onboarding/cloud-waitlist":             "the caller's waitlist entry",
	"POST /api/me/onboarding/complete":                   "the caller's onboarding state",
	"POST /api/me/whats-new/seen":                        "the caller's what's-new state",
	"POST /api/me/consents":                              "the caller's consent records",
	"POST /api/me/deletion":                              "the caller's own account deletion",
	"DELETE /api/me/deletion":                            "the caller's own account deletion",
	"POST /api/me/mfa/enroll":                            "the caller's MFA",
	"POST /api/me/mfa/confirm":                           "the caller's MFA",
	"POST /api/me/mfa/disable":                           "the caller's MFA",
	"POST /api/me/sessions/revoke-all":                   "the caller's sessions",
	"POST /api/workspaces/":                              "creates a new workspace the caller owns",
	"POST /api/cloud-billing/checkout-sessions":          "the caller's Cloud billing, no workspace data",
	"POST /api/cloud-billing/portal-sessions":            "the caller's Cloud billing, no workspace data",
	"POST /api/integrations/composio/connect/init":       "the caller's own Composio connection",
	"DELETE /api/integrations/composio/connections/{id}": "the caller's own Composio connection",
	"POST /api/invitations/{id}/accept":                  "the invitee's own invitation; an existing member gets 409",
	"POST /api/invitations/{id}/decline":                 "the invitee's own invitation",
	"POST /api/share-links/join":                         "joins by link; an existing member gets 409 and no role change",
	"POST /auth/google":                                  "sign-in flow, no workspace",
	"POST /auth/logout":                                  "sign-in flow, no workspace",
	"POST /auth/mfa/verify":                              "sign-in flow, no workspace",
	"POST /auth/send-code":                               "sign-in flow, no workspace",
	"POST /auth/sso/discover":                            "sign-in flow, no workspace",
	"POST /auth/verify-code":                             "sign-in flow, no workspace",
	// Handler-enforced: the handler refuses a viewer itself (see the named test).
	"POST /api/upload-file": "handler-enforced: UploadFile calls refuseViewer (TestUploadFileRefusesAViewer)",
	// Handler-enforced: the workspace comes from the binding token, not from a
	// header, so the workspace middleware cannot see it. The redeem transaction
	// refuses a viewer (TestBindingRedeemRefusesAViewerAndAdmitsAMember).
	"POST /api/dingtalk/binding/redeem": "handler-enforced: redeem refuses a viewer in its transaction (TestBindingRedeemRefusesAViewerAndAdmitsAMember)",
	"POST /api/lark/binding/redeem":     "handler-enforced: redeem refuses a viewer in its transaction (TestBindingRedeemRefusesAViewerAndAdmitsAMember)",
	"POST /api/slack/binding/redeem":    "handler-enforced: redeem refuses a viewer in its transaction (TestBindingRedeemRefusesAViewerAndAdmitsAMember)",
	"POST /api/telegram/binding/redeem": "handler-enforced: redeem refuses a viewer in its transaction (TestBindingRedeemRefusesAViewerAndAdmitsAMember)",
	"POST /api/wecom/binding/redeem":    "handler-enforced: redeem refuses a viewer in its transaction (TestBindingRedeemRefusesAViewerAndAdmitsAMember)",
}

// viewerExemptPrefixes are whole route families outside the guard. Daemon
// routes take a daemon token or a user credential, and their handlers refuse a
// viewer through requireDaemonWorkspaceAccess (TestRequireDaemonWorkspaceAccessRefusesAViewer,
// TestDaemonRegisterRefusesAViewer). Plugin bridge writes are refused in
// pluginSessionCaller. Webhook senders authenticate with a token or signature.
var viewerExemptPrefixes = []struct{ prefix, reason string }{
	{"/api/daemon/", "handler-enforced: daemon guard refuses a viewer"},
	{"/api/plugin-bridge/v1/", "handler-enforced: pluginSessionCaller refuses a viewer's writes"},
	{"/api/webhooks/", "external senders authenticate by token or signature"},
}

// viewerExemption reports whether a write route is outside the workspace guard,
// and why.
func viewerExemption(method, route string) (string, bool) {
	if reason, ok := viewerWriteExceptions[method+" "+route]; ok {
		return reason, true
	}
	for _, p := range viewerExemptPrefixes {
		if strings.HasPrefix(route, p.prefix) {
			return p.reason, true
		}
	}
	return "", false
}

var pathParam = regexp.MustCompile(`\{[^}]+\}`)

func isWriteMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

// serveAsViewer runs one route's middleware chain as the test user, who is a
// viewer of the test workspace. It reports whether the sentinel was reached and
// the status the chain wrote when it was not.
func serveAsViewer(method, route string, mws []func(http.Handler) http.Handler) (bool, int) {
	reached := false
	sentinel := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusNoContent)
	})
	mux := chi.NewRouter()
	mux.With(mws...).Method(method, route, sentinel)

	path := pathParam.ReplaceAllStringFunc(route, func(p string) string {
		if p == "{id}" {
			return testWorkspaceID
		}
		return "00000000-0000-4000-8000-000000000001"
	})
	path = strings.TrimSuffix(path, "/*")
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("X-Workspace-ID", testWorkspaceID)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return reached, rec.Code
}
