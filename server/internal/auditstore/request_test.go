package auditstore

import (
	"net/http"
	"net/http/httptest"
	"testing"

	chimw "github.com/go-chi/chi/v5/middleware"
)

func TestRequestFieldsReturnsIPUserAgentAndRequestID(t *testing.T) {
	var gotReqID string
	h := chimw.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReqID = chimw.GetReqID(r.Context())
		ip, ua, id := RequestFields(r, func(*http.Request) string { return "203.0.113.7" })
		if ip != "203.0.113.7" || ua != "test-agent/1.0" || id != "req-123" {
			t.Errorf("RequestFields = (%q, %q, %q)", ip, ua, id)
		}
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("User-Agent", "test-agent/1.0")
	req.Header.Set("X-Request-Id", "req-123")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if gotReqID != "req-123" {
		t.Fatalf("middleware request id = %q", gotReqID)
	}
}

func TestRequestFieldsIgnoresOtherHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("User-Agent", "ua")
	req.Header.Set("Authorization", "Bearer secret-value")
	req.Header.Set("Cookie", "session=secret-cookie")
	req.Header.Set("X-Forwarded-For", "198.51.100.9")
	req.Header.Set("X-Api-Key", "secret-key")
	ip, ua, id := RequestFields(req, func(*http.Request) string { return "192.0.2.1" })
	if ip != "192.0.2.1" || ua != "ua" || id != "" {
		t.Fatalf("RequestFields = (%q, %q, %q)", ip, ua, id)
	}
	for _, v := range []string{ip, ua, id} {
		for _, bad := range []string{"secret", "198.51.100.9"} {
			if contains(v, bad) {
				t.Errorf("value %q leaks %q", v, bad)
			}
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
