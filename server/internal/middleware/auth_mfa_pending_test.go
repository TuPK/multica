package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/multica-ai/multica/server/internal/auth"
)

// A token waiting on the second factor is not a session. Auth refuses it on
// every route, whether it comes in a header or the auth cookie.
func TestAuthRefusesMFAPendingToken(t *testing.T) {
	pending, _, err := auth.IssueMFAPendingToken("11111111-2222-3333-4444-555555555555", []string{auth.AMROTP}, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	// A token that carries a `sub` as well as the audience: the audience alone
	// must refuse it, whatever else is in the claims.
	withSub, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   "11111111-2222-3333-4444-555555555555",
		"email": "u@example.com",
		"aud":   auth.MFAPendingAudience,
		"exp":   time.Now().Add(time.Hour).Unix(),
	}).SignedString(auth.JWTSecret())
	if err != nil {
		t.Fatal(err)
	}

	handler := authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler must not be called for a pending token")
	}))

	for name, token := range map[string]string{"pending token": pending, "pending audience with sub": withSub} {
		for _, path := range []string{"/api/me", "/api/issues", "/api/me/mfa/disable"} {
			for _, method := range []string{http.MethodGet, http.MethodPost} {
				req := httptest.NewRequest(method, path, nil)
				req.Header.Set("Authorization", "Bearer "+token)
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, req)
				if w.Code != http.StatusUnauthorized && w.Code != http.StatusForbidden {
					t.Fatalf("%s: %s %s = %d, want 401 or 403", name, method, path, w.Code)
				}
			}
		}
	}

	// Via the cookie, on a safe method (no CSRF needed).
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(&http.Cookie{Name: auth.AuthCookieName, Value: withSub})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized && w.Code != http.StatusForbidden {
		t.Fatalf("cookie: GET /api/me = %d, want 401 or 403", w.Code)
	}
}
