package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/internal/analytics"
	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/realtime"
)

// Routes that are registered outside the auth middleware. A pending token
// reaches them only as ordinary request data; they must never turn it into a
// session (no Set-Cookie, no session token in the body).
var mfaPendingPublicRoutes = map[string]string{
	"GET /health":                               "liveness probe",
	"GET /healthz":                              "liveness probe",
	"GET /readyz":                               "readiness probe",
	"GET /health/realtime":                      "realtime health probe",
	"GET /api/config":                           "public client config",
	"POST /auth/send-code":                      "login start",
	"POST /auth/verify-code":                    "first-factor login",
	"POST /auth/google":                         "first-factor login",
	"POST /auth/logout":                         "clears the cookie, no auth",
	"POST /auth/sso/mobile/exchange":            "SSO login exchange",
	"GET /api/site/star":                        "public marketing footer star",
	"POST /api/site/star/turns":                 "public marketing footer star",
	"POST /api/apps/mcp":                        "apps gateway, verifies its own task token",
	"POST /auth/sso/discover":                   "SSO login start",
	"GET /auth/sso/start":                       "SSO login start",
	"GET /auth/sso/callback":                    "first-factor login",
	"POST /api/contact-sales":                   "public form",
	"GET /api/share-links/{code}":               "public share link, authorised by the code",
	"GET /api/avatars/{sig}/*":                  "signed URL",
	"GET /api/attachments/{id}/signed-download": "signed URL",
	"GET /plugin-surfaces/{token}":              "authorised by the URL token",
	"GET /ws":                                   "WebSocket; authenticates in its own handshake",
	"GET /api/github/setup":                     "GitHub redirect callback",
	"GET /api/integrations/composio/callback":   "OAuth redirect callback",
	"POST /api/webhooks/autopilots/{token}":     "authorised by the URL token",
	"POST /api/webhooks/github":                 "authorised by webhook signature",
	"POST /api/webhooks/stripe":                 "authorised by webhook signature",
	"POST /api/webhooks/vcs/{connectionId}":     "authorised by webhook signature",
}

// Routes that take a pending token by design.
var mfaPendingAcceptingRoutes = map[string]string{
	"POST /auth/mfa/verify": "second-factor verify; reads the pending token itself",
}

var routeParamRe = regexp.MustCompile(`\{[^}/]*\}`)

// AC15: a token that proves only the first factor opens no route except the
// MFA verify route.
func TestMFAPendingTokenCallsNoRouteButMFAVerify(t *testing.T) {
	router := NewRouter(nil, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil)

	pending, _, err := auth.IssueMFAPendingToken("11111111-2222-3333-4444-555555555555", []string{auth.AMROTP}, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	type route struct{ method, pattern string }
	var routes []route
	err = chi.Walk(router, func(method, pattern string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		routes = append(routes, route{method, pattern})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	const minRoutes = 100
	if len(routes) < minRoutes {
		t.Fatalf("walked %d routes, want at least %d", len(routes), minRoutes)
	}

	seen := map[string]bool{}
	var leaked []string
	refused, accepting, public := 0, 0, 0
	for _, rt := range routes {
		key := rt.method + " " + rt.pattern
		seen[key] = true
		path := strings.ReplaceAll(routeParamRe.ReplaceAllString(rt.pattern, "11111111-2222-3333-4444-555555555555"), "/*", "/x")
		req := httptest.NewRequest(rt.method, path, nil)
		req.Header.Set("Authorization", "Bearer "+pending)
		w := httptest.NewRecorder()

		func() {
			defer func() {
				if rec := recover(); rec != nil {
					w.Code = http.StatusInternalServerError
					w.Body.WriteString(fmt.Sprint("panic: ", rec))
				}
			}()
			router.ServeHTTP(w, req)
		}()

		if _, ok := mfaPendingAcceptingRoutes[key]; ok {
			accepting++
			continue
		}
		if _, ok := mfaPendingPublicRoutes[key]; ok {
			public++
			for _, c := range w.Result().Cookies() {
				if c.Value != "" {
					t.Errorf("%s: public route set cookie %q for a pending token", key, c.Name)
				}
			}
			if strings.Contains(w.Body.String(), `"token"`) {
				t.Errorf("%s: public route issued a session for a pending token", key)
			}
			continue
		}
		if w.Code != http.StatusUnauthorized && w.Code != http.StatusForbidden {
			leaked = append(leaked, fmt.Sprintf("%s -> %d %s", key, w.Code, strings.TrimSpace(w.Body.String())))
		}
		refused++
	}

	for key := range mfaPendingAcceptingRoutes {
		if !seen[key] {
			t.Errorf("exception %q is not a registered route", key)
		}
	}
	for key := range mfaPendingPublicRoutes {
		if !seen[key] {
			t.Errorf("exception %q is not a registered route", key)
		}
	}
	sort.Strings(leaked)
	if len(leaked) > 0 {
		t.Errorf("%d routes did not refuse a pending token:\n%s", len(leaked), strings.Join(leaked, "\n"))
	}
	t.Logf("walked %d routes: %d refused, %d accepting, %d public", len(routes), refused, accepting, public)
}
