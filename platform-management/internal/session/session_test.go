package session

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

const (
	secret = "0123456789abcdef0123456789abcdef"
	userID = "11111111-2222-3333-4444-555555555555"
)

var now = time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)

func mustSign(t *testing.T, secret string, authTime time.Time) string {
	t.Helper()
	tok, err := Sign(secret, userID, authTime)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	return tok
}

func TestSignedTokenVerifies(t *testing.T) {
	tok := mustSign(t, secret, now)
	claims, err := Verify(secret, tok, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.UserID != userID {
		t.Errorf("UserID = %q", claims.UserID)
	}
	if got := claims.AuthTime(); !got.Equal(now) {
		t.Errorf("AuthTime = %v, want %v", got, now)
	}
	if len(claims.AMR) != 1 || claims.AMR[0] != "email_code" {
		t.Errorf("AMR = %v", claims.AMR)
	}
	if claims.Exp != now.Add(MaxAge).Unix() {
		t.Errorf("Exp = %d, want auth_time + 12h", claims.Exp)
	}
}

func TestTokenShape(t *testing.T) {
	tok := mustSign(t, secret, now)
	parts := strings.Split(tok, ".")
	if len(parts) != 2 {
		t.Fatalf("token has %d parts, want 2", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("payload is not base64url: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatalf("payload is not JSON: %v", err)
	}
	for _, k := range []string{"uid", "auth_time", "amr", "exp"} {
		if _, ok := m[k]; !ok {
			t.Errorf("payload lacks %q", k)
		}
	}
}

func TestVerifyRejects(t *testing.T) {
	good := mustSign(t, secret, now)
	goodParts := strings.Split(good, ".")

	forge := func(claims map[string]any, key string) string {
		payload, _ := json.Marshal(claims)
		p := base64.RawURLEncoding.EncodeToString(payload)
		// A holder of the key can sign any payload; Verify must still check it.
		return p + "." + sign(key, payload)
	}
	validClaims := func() map[string]any {
		return map[string]any{"uid": userID, "auth_time": now.Unix(), "amr": []string{"email_code"}, "exp": now.Add(MaxAge).Unix()}
	}
	withClaim := func(k string, v any) map[string]any {
		c := validClaims()
		c[k] = v
		return c
	}

	tests := []struct {
		name string
		tok  string
		at   time.Time
	}{
		{"another secret", mustSign(t, "another-secret-another-secret-0123", now), now},
		{"expired", good, now.Add(MaxAge)},
		{"auth_time over 12 hours old", forge(withClaim("exp", now.Add(48*time.Hour).Unix()), secret), now.Add(MaxAge + time.Second)},
		{"auth_time in the future", forge(withClaim("auth_time", now.Add(time.Hour).Unix()), secret), now},
		{"empty uid", forge(withClaim("uid", ""), secret), now},
		{"payload changed", base64.RawURLEncoding.EncodeToString([]byte(`{"uid":"x","auth_time":1,"exp":9999999999}`)) + "." + goodParts[1], now},
		{"signature changed", goodParts[0] + "." + strings.Repeat("A", len(goodParts[1])), now},
		{"empty", "", now},
		{"no dot", goodParts[0], now},
		{"three parts", good + ".x", now},
		{"payload not base64", "!!!." + goodParts[1], now},
		{"signature not base64", goodParts[0] + ".!!!", now},
		{"payload not JSON", base64.RawURLEncoding.EncodeToString([]byte("nope")) + "." + sign(secret, []byte("nope")), now},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Verify(secret, tc.tok, tc.at); err == nil {
				t.Fatal("Verify accepted the token")
			}
		})
	}
}

func TestVerifyAcceptsAnAuthTimeJustInsideTheLimit(t *testing.T) {
	tok := mustSign(t, secret, now)
	if _, err := Verify(secret, tok, now.Add(MaxAge-time.Second)); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestCookieAttributes(t *testing.T) {
	c := NewCookie("tok", true, now)
	if c.Name != "mpa_session" || c.Value != "tok" {
		t.Errorf("name/value = %q/%q", c.Name, c.Value)
	}
	if !c.HttpOnly || !c.Secure {
		t.Errorf("HttpOnly=%v Secure=%v", c.HttpOnly, c.Secure)
	}
	if c.SameSite != http.SameSiteStrictMode {
		t.Errorf("SameSite = %v", c.SameSite)
	}
	if c.Path != "/" || c.Domain != "" {
		t.Errorf("Path=%q Domain=%q", c.Path, c.Domain)
	}
	if !c.Expires.Equal(now.Add(12*time.Hour)) || c.MaxAge != 12*3600 {
		t.Errorf("Expires=%v MaxAge=%d", c.Expires, c.MaxAge)
	}
	header := c.String()
	for _, want := range []string{"HttpOnly", "Secure", "SameSite=Strict", "Path=/"} {
		if !strings.Contains(header, want) {
			t.Errorf("header %q lacks %s", header, want)
		}
	}
	if strings.Contains(header, "Domain") {
		t.Errorf("header %q has a Domain attribute", header)
	}
}

func TestCookieWithoutSecure(t *testing.T) {
	c := NewCookie("tok", false, now)
	if c.Secure || strings.Contains(c.String(), "Secure") {
		t.Errorf("Secure present: %q", c.String())
	}
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode {
		t.Errorf("HttpOnly=%v SameSite=%v", c.HttpOnly, c.SameSite)
	}
}

func TestClearCookie(t *testing.T) {
	for _, secure := range []bool{true, false} {
		c := ClearCookie(secure)
		if c.Name != "mpa_session" || c.Value != "" || c.MaxAge >= 0 {
			t.Errorf("clear cookie = %+v", c)
		}
		if !c.HttpOnly || c.Secure != secure || c.SameSite != http.SameSiteStrictMode || c.Path != "/" || c.Domain != "" {
			t.Errorf("clear cookie attributes = %+v", c)
		}
		if !c.Expires.Before(time.Now()) {
			t.Errorf("Expires = %v", c.Expires)
		}
	}
}
