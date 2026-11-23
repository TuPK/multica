package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// platformSessionToken builds a well-formed platform console session token:
// base64url(payload) "." base64url(HMAC-SHA256(secret, payload)). The platform
// service's session package is internal to it, so the format is repeated here.
func platformSessionToken(secret string) string {
	now := time.Now()
	payload := fmt.Sprintf(`{"uid":"11111111-2222-3333-4444-555555555555","auth_time":%d,"amr":["email_code"],"exp":%d}`,
		now.Unix(), now.Add(12*time.Hour).Unix())
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// The platform console's mpa_session cookie is not a Delegora credential: Auth
// treats a request that carries only that cookie as one with no credentials.
// The request never reaches a lookup, so no database is needed.
func TestAuthIgnoresPlatformSessionCookie(t *testing.T) {
	handler := authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler must not be called for a platform session cookie")
	}))

	none := httptest.NewRecorder()
	handler.ServeHTTP(none, httptest.NewRequest(http.MethodGet, "/api/me", nil))
	if none.Code != http.StatusUnauthorized {
		t.Fatalf("baseline without credentials: status %d, want 401", none.Code)
	}

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		req := httptest.NewRequest(method, "/api/me", nil)
		req.AddCookie(&http.Cookie{Name: "mpa_session", Value: platformSessionToken("0123456789abcdef0123456789abcdef")})
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s with only mpa_session: status %d, want 401", method, w.Code)
		}
		if w.Body.String() != none.Body.String() {
			t.Fatalf("%s with only mpa_session: body %q, want the no-credentials body %q", method, w.Body.String(), none.Body.String())
		}
	}
}
