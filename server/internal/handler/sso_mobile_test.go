package handler

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/mfa"
	"github.com/multica-ai/multica/server/internal/sso"
)

const ssoMobileVerifier = "mobile-verifier-0123456789-abcdefghijklmnopqrstuvwxyz"

func ssoMobileChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// startMobileLogin is startLogin for client=mobile.
func (e *ssoEnv) startMobileLogin(challenge string) (*url.URL, *http.Cookie) {
	e.t.Helper()
	req := httptest.NewRequest(http.MethodGet,
		"/auth/sso/start?connection="+e.connID+"&client=mobile&challenge="+challenge, nil)
	rec := e.serve(req)
	if rec.Code != http.StatusFound {
		e.t.Fatalf("mobile start: %d %s", rec.Code, rec.Body.String())
	}
	var pending *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == sso.PendingCookie {
			pending = c
		}
	}
	if pending == nil {
		e.t.Fatal("mobile start set no pending cookie")
	}
	cb, err := url.Parse(e.idp.Authorize(e.t, rec.Header().Get("Location")))
	if err != nil {
		e.t.Fatal(err)
	}
	return cb, pending
}

func (e *ssoEnv) exchange(code, verifier string) *httptest.ResponseRecorder {
	return e.serve(newRequest(http.MethodPost, "/auth/sso/mobile/exchange", map[string]any{"code": code, "verifier": verifier}))
}

// mobileCode runs a full mobile login and returns the code in the app redirect.
func (e *ssoEnv) mobileCode(verifier string) string {
	e.t.Helper()
	cb, pending := e.startMobileLogin(ssoMobileChallenge(verifier))
	rec := e.callback(cb, pending)
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "multica://sso-complete?code=") {
		e.t.Fatalf("callback Location = %q", loc)
	}
	u, err := url.Parse(loc)
	if err != nil {
		e.t.Fatal(err)
	}
	return u.Query().Get("code")
}

func assertExchangeInvalid(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("exchange answered %d %s, want 400", rec.Code, rec.Body.String())
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Code != "sso_exchange_invalid" {
		t.Fatalf("exchange code = %q, want sso_exchange_invalid (%s)", body.Code, rec.Body.String())
	}
}

func TestSSOMobileStartRequiresValidChallenge(t *testing.T) {
	e := newSSOEnv(t)
	good := ssoMobileChallenge(ssoMobileVerifier)
	for name, challenge := range map[string]string{"missing": "", "42 chars": good[:42]} {
		req := httptest.NewRequest(http.MethodGet, "/auth/sso/start?connection="+e.connID+"&client=mobile&challenge="+challenge, nil)
		rec := e.serve(req)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"invalid_challenge"`) {
			t.Fatalf("%s: got %d %s, want 400 invalid_challenge", name, rec.Code, rec.Body.String())
		}
	}

	_, pending := e.startMobileLogin(good)
	crypto, err := mfa.LoadCrypto()
	if err != nil {
		t.Fatal(err)
	}
	pend, err := sso.OpenPending(crypto, pending.Value, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !pend.Mobile || pend.Challenge != good {
		t.Fatalf("pending = %+v, want Mobile with the challenge", pend)
	}
}

func TestSSOMobileCallbackRedirectsToAppWithoutWebCookies(t *testing.T) {
	e := newSSOEnv(t)
	cb, pending := e.startMobileLogin(ssoMobileChallenge(ssoMobileVerifier))
	rec := e.callback(cb, pending)
	if rec.Code != http.StatusFound {
		t.Fatalf("callback: %d %s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "multica://sso-complete?code=") {
		t.Fatalf("Location = %q", loc)
	}
	if authCookie(rec) != "" {
		t.Fatal("a mobile sign-in set the web auth cookie")
	}
	u, _ := url.Parse(loc)
	crypto, err := mfa.LoadCrypto()
	if err != nil {
		t.Fatal(err)
	}
	h, err := openSSOHandoff(crypto, u.Query().Get("code"), time.Now())
	if err != nil {
		t.Fatalf("code does not open as a handoff: %v", err)
	}
	if h.Token == "" || h.Challenge != ssoMobileChallenge(ssoMobileVerifier) {
		t.Fatalf("handoff = %+v", h)
	}
	if _, err := sso.OpenPending(crypto, u.Query().Get("code"), time.Now()); err == nil {
		t.Fatal("a handoff opened as a pending login")
	}
}

func TestSSOMobileCallbackFailureRedirectsToAppWithError(t *testing.T) {
	e := newSSOEnv(t)
	e.idp.Email = "outsider-" + uuid.NewString()[:8] + "@other.test"
	cb, pending := e.startMobileLogin(ssoMobileChallenge(ssoMobileVerifier))
	rec := e.callback(cb, pending)
	if got := rec.Header().Get("Location"); got != "multica://sso-complete?error=domain_not_allowed" {
		t.Fatalf("Location = %q", got)
	}
}

func TestSSOMobileExchangeReturnsSessionForMatchingVerifier(t *testing.T) {
	e := newSSOEnv(t)
	code := e.mobileCode(ssoMobileVerifier)
	rec := e.exchange(code, ssoMobileVerifier)
	if rec.Code != http.StatusOK {
		t.Fatalf("exchange: %d %s", rec.Code, rec.Body.String())
	}
	var body LoginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Token == "" || body.User.Email != e.idp.Email {
		t.Fatalf("body = %+v, want a token and %s", body, e.idp.Email)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatal("exchange set cookies")
	}
}

func TestSSOMobileExchangeRefusesWrongVerifier(t *testing.T) {
	e := newSSOEnv(t)
	code := e.mobileCode(ssoMobileVerifier)
	assertExchangeInvalid(t, e.exchange(code, ssoMobileVerifier+"x"))
}

func TestSSOMobileExchangeRefusesExpiredCode(t *testing.T) {
	e := newSSOEnv(t)
	crypto, err := mfa.LoadCrypto()
	if err != nil {
		t.Fatal(err)
	}
	code, err := sealSSOHandoff(crypto, ssoHandoff{
		Purpose: ssoHandoffPurpose, Token: "t", Challenge: ssoMobileChallenge(ssoMobileVerifier),
		ExpiresAt: time.Now().Add(-time.Second).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertExchangeInvalid(t, e.exchange(code, ssoMobileVerifier))
}
