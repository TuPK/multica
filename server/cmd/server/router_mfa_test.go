package main

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/multica-ai/multica/server/internal/analytics"
	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/realtime"
)

var routeParam = regexp.MustCompile(`\{[^}]*\}`)

// serve runs one request and reports its status. A handler that panics (a
// public route reaching for the database this test does not have) reports 0.
func serve(router http.Handler, method, path, bearer string) (status int) {
	defer func() {
		if recover() != nil {
			status = 0
		}
	}()
	req := httptest.NewRequest(method, path, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec.Code
}

// AC15: the token that waits on the second factor calls no other API. Every
// route that refuses an anonymous caller with 401 must refuse the pending
// token with 401 or 403.
func TestRouterMFAPendingTokenCallsNoOtherAPI(t *testing.T) {
	t.Setenv("MULTICA_MFA_SECRET_KEY", "")
	router, _ := NewRouterWithOptions(nil, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil, RouterOptions{})

	pending, _, err := auth.IssueMFAPendingToken("11111111-2222-3333-4444-555555555555", []string{auth.AMROTP}, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	wantRoutes := map[string]bool{
		http.MethodPost + " /auth/mfa/verify":    false,
		http.MethodPost + " /api/me/mfa/enroll":  false,
		http.MethodPost + " /api/me/mfa/confirm": false,
		http.MethodPost + " /api/me/mfa/disable": false,
	}
	protected := 0
	err = chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if _, ok := wantRoutes[method+" "+route]; ok {
			wantRoutes[method+" "+route] = true
		}
		if method == http.MethodPost && route == "/auth/mfa/verify" {
			return nil // takes the pending token by design
		}
		path := strings.ReplaceAll(routeParam.ReplaceAllString(route, "x"), "*", "x")
		if serve(router, method, path, "") != http.StatusUnauthorized {
			return nil // a public route: it does not read a session
		}
		protected++
		if got := serve(router, method, path, pending); got != http.StatusUnauthorized && got != http.StatusForbidden {
			t.Errorf("%s %s with a pending token = %d, want 401 or 403", method, route, got)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for route, found := range wantRoutes {
		if !found {
			t.Errorf("route %s is not registered", route)
		}
	}
	if protected < 50 {
		t.Fatalf("only %d protected routes were exercised; the walk is not reaching the API", protected)
	}
	t.Logf("%d protected routes refused the pending token", protected)
}
