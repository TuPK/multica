package handler

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/mfa"
	"github.com/multica-ai/multica/server/internal/testutil"
)

type mfaChallengeResponse struct {
	MFARequired bool   `json:"mfa_required"`
	MFAToken    string `json:"mfa_token"`
	ExpiresIn   int    `json:"expires_in"`
	Token       string `json:"token"`
}

func pendingTokenFor(t *testing.T, userID string, amr ...string) string {
	t.Helper()
	token, _, err := auth.IssueMFAPendingToken(userID, amr, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func verifyMFARequest(token, code string) *http.Request {
	return testutil.JSONRequest(http.MethodPost, "/auth/mfa/verify", map[string]string{"mfa_token": token, "code": code})
}

func errorCode(t *testing.T, r *testutil.Response) string {
	t.Helper()
	return r.Map()["code"].(string)
}

// AC15: an account with MFA gets no session from the email code.
func TestMFAVerifyCodeGivesAPendingTokenNotASession(t *testing.T) {
	u := newMFAUser(t)
	confirmedMFA(t, u)
	dbfx.Cleanup(t, `DELETE FROM verification_code WHERE email = $1`, u.Email)
	createVerificationCodeForTest(t, u.Email, "123456")

	req := testutil.JSONRequest(http.MethodPost, "/auth/verify-code", map[string]string{"email": u.Email, "code": "123456"})
	res := testutil.Call(t, testHandler.VerifyCode, req).Want(http.StatusOK)
	var out mfaChallengeResponse
	res.JSON(&out)

	if out.Token != "" {
		t.Fatal("the response carries a session token")
	}
	if len(res.Result().Cookies()) != 0 {
		t.Fatalf("the response set cookies: %v", res.Result().Cookies())
	}
	if !out.MFARequired || out.MFAToken == "" || out.ExpiresIn != 300 {
		t.Fatalf("challenge = %+v, want mfa_required with a 300 s token", out)
	}
	pending, err := auth.ParseMFAPendingToken(out.MFAToken, time.Now())
	if err != nil || pending.UserID != u.ID {
		t.Fatalf("pending token = %+v, %v; want one for %s", pending, err, u.ID)
	}
	if _, err := auth.ParseSessionToken(out.MFAToken); err == nil {
		t.Fatal("the pending token passes as a session")
	}
}

func TestMFAGoogleLoginGivesAPendingTokenNotASession(t *testing.T) {
	u := newMFAUser(t)
	confirmedMFA(t, u)
	t.Setenv("GOOGLE_CLIENT_ID", "test-client")
	t.Setenv("GOOGLE_CLIENT_SECRET", "test-secret")
	h := newTestHandler(Config{AllowSignup: true})
	h.Queries = testHandler.Queries
	h.googleOAuthHTTPClient = &http.Client{Transport: googleRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "oauth2.googleapis.com" {
			return googleResponse(req, http.StatusOK, `{"access_token":"test-token"}`), nil
		}
		return googleResponse(req, http.StatusOK, fmt.Sprintf(`{"email":%q}`, u.Email)), nil
	})}
	req := testutil.JSONRequest(http.MethodPost, "/auth/google", `{"code":"c","redirect_uri":"http://localhost/auth/callback"}`)

	res := testutil.Call(t, h.GoogleLogin, req).Want(http.StatusOK)
	var out mfaChallengeResponse
	res.JSON(&out)
	if out.Token != "" || !out.MFARequired || out.MFAToken == "" || len(res.Result().Cookies()) != 0 {
		t.Fatalf("google sign-in with MFA gave %+v and %d cookies", out, len(res.Result().Cookies()))
	}
	pending, err := auth.ParseMFAPendingToken(out.MFAToken, time.Now())
	if err != nil || strings.Join(pending.AMR, ",") != "google" {
		t.Fatalf("pending token = %+v, %v; want the google first factor", pending, err)
	}
}

// An enrolment that was never confirmed is not a second factor.
func TestMFAUnconfirmedEnrolmentDoesNotChallenge(t *testing.T) {
	u := newMFAUser(t)
	testutil.Call(t, testHandler.EnrollMFA, mfaRequest(u.ID, "/api/me/mfa/enroll", nil)).Want(http.StatusOK)
	dbfx.Cleanup(t, `DELETE FROM verification_code WHERE email = $1`, u.Email)
	createVerificationCodeForTest(t, u.Email, "123456")

	req := testutil.JSONRequest(http.MethodPost, "/auth/verify-code", map[string]string{"email": u.Email, "code": "123456"})
	var out mfaChallengeResponse
	testutil.Call(t, testHandler.VerifyCode, req).Want(http.StatusOK).JSON(&out)
	if out.Token == "" || out.MFARequired {
		t.Fatalf("response = %+v, want a session", out)
	}
}

func TestAMRMFAVerifyAddsMFA(t *testing.T) {
	u := newMFAUser(t)
	secret, _ := confirmedMFA(t, u)

	var out LoginResponse
	res := testutil.Call(t, testHandler.VerifyMFA, verifyMFARequest(pendingTokenFor(t, u.ID, auth.AMROTP), totpAt(t, secret, 0))).
		Want(http.StatusOK)
	res.JSON(&out)
	assertAMR(t, out.Token, "otp", "mfa")
	if out.User.ID != u.ID {
		t.Fatalf("user = %s, want %s", out.User.ID, u.ID)
	}
	if len(res.Result().Cookies()) == 0 {
		t.Fatal("verify did not set the auth cookies")
	}

	// The first factor survives: a Google sign-in ends as ["google","mfa"].
	out = LoginResponse{}
	testutil.Call(t, testHandler.VerifyMFA, verifyMFARequest(pendingTokenFor(t, u.ID, auth.AMRGoogle), totpAt(t, secret, 1))).
		Want(http.StatusOK).JSON(&out)
	assertAMR(t, out.Token, "google", "mfa")
}

func TestMFAVerifySixthWrongCodeFailsTheToken(t *testing.T) {
	u := newMFAUser(t)
	secret, _ := confirmedMFA(t, u)
	token := pendingTokenFor(t, u.ID, auth.AMROTP)

	for i := 1; i <= 5; i++ {
		res := testutil.Call(t, testHandler.VerifyMFA, verifyMFARequest(token, "000000")).Want(http.StatusBadRequest)
		if got := errorCode(t, res); got != "invalid_code" {
			t.Fatalf("wrong code %d: code = %q, want invalid_code", i, got)
		}
	}
	// The token is dead now, even for the right code.
	res := testutil.Call(t, testHandler.VerifyMFA, verifyMFARequest(token, totpAt(t, secret, 0))).Want(http.StatusUnauthorized)
	if got := errorCode(t, res); got != "mfa_token_failed" {
		t.Fatalf("sixth attempt: code = %q, want mfa_token_failed", got)
	}

	// A fresh first factor starts again.
	testutil.Call(t, testHandler.VerifyMFA, verifyMFARequest(pendingTokenFor(t, u.ID, auth.AMROTP), totpAt(t, secret, 0))).
		Want(http.StatusOK)
}

func TestMFAVerifyExpiredPendingToken(t *testing.T) {
	u := newMFAUser(t)
	secret, _ := confirmedMFA(t, u)
	token, _, err := auth.IssueMFAPendingToken(u.ID, []string{auth.AMROTP}, time.Now().Add(-6*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	res := testutil.Call(t, testHandler.VerifyMFA, verifyMFARequest(token, totpAt(t, secret, 0))).Want(http.StatusUnauthorized)
	if got := errorCode(t, res); got != "mfa_token_invalid" {
		t.Fatalf("code = %q, want mfa_token_invalid", got)
	}
}

func TestMFAVerifyTOTPCodeCannotBeUsedTwiceInOneStep(t *testing.T) {
	u := newMFAUser(t)
	secret, _ := confirmedMFA(t, u)
	code := totpAt(t, secret, 0)

	testutil.Call(t, testHandler.VerifyMFA, verifyMFARequest(pendingTokenFor(t, u.ID, auth.AMROTP), code)).Want(http.StatusOK)
	res := testutil.Call(t, testHandler.VerifyMFA, verifyMFARequest(pendingTokenFor(t, u.ID, auth.AMROTP), code)).Want(http.StatusBadRequest)
	if got := errorCode(t, res); got != "invalid_code" {
		t.Fatalf("replayed code: code = %q, want invalid_code", got)
	}

	// An older step than the one used is refused too.
	testutil.Call(t, testHandler.VerifyMFA, verifyMFARequest(pendingTokenFor(t, u.ID, auth.AMROTP), totpAt(t, secret, -1))).
		Want(http.StatusBadRequest)
	// The next step is fine.
	testutil.Call(t, testHandler.VerifyMFA, verifyMFARequest(pendingTokenFor(t, u.ID, auth.AMROTP), totpAt(t, secret, 1))).
		Want(http.StatusOK)
}

func TestMFAVerifyRecoveryCodeWorksOnce(t *testing.T) {
	u := newMFAUser(t)
	_, recovery := confirmedMFA(t, u)

	// Case and a dash are forgiven on input.
	typed := strings.ToLower(recovery[0][:5]) + "-" + recovery[0][5:]
	var out LoginResponse
	testutil.Call(t, testHandler.VerifyMFA, verifyMFARequest(pendingTokenFor(t, u.ID, auth.AMROTP), typed)).
		Want(http.StatusOK).JSON(&out)
	assertAMR(t, out.Token, "otp", "mfa")

	var left []string
	dbfx.QueryRow(t, `SELECT recovery_hashes FROM user_mfa WHERE user_id = $1`, u.ID).Scan(&left)
	if len(left) != 9 {
		t.Fatalf("%d recovery hashes left, want 9", len(left))
	}

	testutil.Call(t, testHandler.VerifyMFA, verifyMFARequest(pendingTokenFor(t, u.ID, auth.AMROTP), recovery[0])).
		Want(http.StatusBadRequest)
	// The others still work.
	testutil.Call(t, testHandler.VerifyMFA, verifyMFARequest(pendingTokenFor(t, u.ID, auth.AMROTP), recovery[1])).
		Want(http.StatusOK)
}

func TestMFAVerifyPendingTokenWorksOnce(t *testing.T) {
	u := newMFAUser(t)
	secret, recovery := confirmedMFA(t, u)
	token := pendingTokenFor(t, u.ID, auth.AMROTP)

	testutil.Call(t, testHandler.VerifyMFA, verifyMFARequest(token, totpAt(t, secret, 0))).Want(http.StatusOK)
	res := testutil.Call(t, testHandler.VerifyMFA, verifyMFARequest(token, recovery[0])).Want(http.StatusUnauthorized)
	if got := errorCode(t, res); got != "mfa_token_failed" {
		t.Fatalf("reused token: code = %q, want mfa_token_failed", got)
	}
}

func TestMFAVerifyRefusesATokenThatIsNotPending(t *testing.T) {
	u := newMFAUser(t)
	secret, _ := confirmedMFA(t, u)
	session, err := testHandler.issueJWT(userForAMRTest(), []string{auth.AMROTP})
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{session, "", "garbage"} {
		res := testutil.Call(t, testHandler.VerifyMFA, verifyMFARequest(token, totpAt(t, secret, 0))).Want(http.StatusUnauthorized)
		if got := errorCode(t, res); got != "mfa_token_invalid" {
			t.Fatalf("token %.8q: code = %q, want mfa_token_invalid", token, got)
		}
	}
}

// If MFA was turned off after the first factor, the pending token is void
// rather than a session.
func TestMFAVerifyWithoutAConfirmedSecretIsRefused(t *testing.T) {
	u := newMFAUser(t)
	secret, _ := confirmedMFA(t, u)
	token := pendingTokenFor(t, u.ID, auth.AMROTP)
	testutil.Call(t, testHandler.DisableMFA, mfaRequest(u.ID, "/api/me/mfa/disable", map[string]string{"code": totpAt(t, secret, 0)})).
		Want(http.StatusNoContent)

	res := testutil.Call(t, testHandler.VerifyMFA, verifyMFARequest(token, totpAt(t, secret, 1))).Want(http.StatusUnauthorized)
	if got := errorCode(t, res); got != "mfa_token_invalid" {
		t.Fatalf("code = %q, want mfa_token_invalid", got)
	}
}

func TestMFAVerifyWithoutKeyReturns503(t *testing.T) {
	u := newMFAUser(t)
	secret, _ := confirmedMFA(t, u)
	t.Setenv(mfa.EnvKey, "")
	res := testutil.Call(t, testHandler.VerifyMFA, verifyMFARequest(pendingTokenFor(t, u.ID, auth.AMROTP), totpAt(t, secret, 0))).
		Want(http.StatusServiceUnavailable)
	if got := errorCode(t, res); got != "mfa_unavailable" {
		t.Fatalf("code = %q, want mfa_unavailable", got)
	}
}
