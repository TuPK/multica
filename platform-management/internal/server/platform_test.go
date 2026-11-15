package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/console"
)

func namedHandler(name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, name)
	})
}

// Every console route reaches the console handler, whatever the method, and
// every other path under /api/platform/ stays with the sign-in handler.
func TestPlatformHandlerRoutesConsolePathsToTheConsole(t *testing.T) {
	h := NewPlatformHandler(namedHandler("auth"), namedHandler("console"))
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
		"/api/platform/plans",
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
	api := NewAPIHandler(HealthHandler(), NewPlatformHandler(namedHandler("auth"), namedHandler("console")))
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
