package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func assertAMR(t *testing.T, token string, want ...string) {
	t.Helper()
	claims, err := auth.ParseSessionToken(token)
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	if _, present := claims[auth.AMRClaim]; !present {
		t.Fatalf("amr claim missing from the signed token")
	}
	got := auth.SessionAMR(claims)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("amr = %v, want %v", got, want)
	}
}

func TestAMRVerifyCodeSetsOTP(t *testing.T) {
	requirePlatformTestDatabase(t)
	email := testutil.UniqueEmail("amr-code")
	dbfx.User(t, "AMR Code", email)
	dbfx.Cleanup(t, `DELETE FROM verification_code WHERE email = $1`, email)
	createVerificationCodeForTest(t, email, "123456")
	req := testutil.JSONRequest(http.MethodPost, "/auth/verify-code", map[string]string{"email": email, "code": "123456"})
	var out struct {
		Token string `json:"token"`
	}
	testutil.Call(t, testHandler.VerifyCode, req).Want(http.StatusOK).JSON(&out)
	assertAMR(t, out.Token, "otp")
}

func TestAMRGoogleLoginSetsGoogle(t *testing.T) {
	requirePlatformTestDatabase(t)
	t.Setenv("GOOGLE_CLIENT_ID", "test-client")
	t.Setenv("GOOGLE_CLIENT_SECRET", "test-secret")
	email := testutil.UniqueEmail("amr-google")
	dbfx.User(t, "AMR Google", email)
	h := newTestHandler(Config{AllowSignup: true})
	h.Queries = testHandler.Queries
	h.googleOAuthHTTPClient = &http.Client{Transport: googleRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "oauth2.googleapis.com" {
			return googleResponse(req, http.StatusOK, `{"access_token":"test-token"}`), nil
		}
		return googleResponse(req, http.StatusOK, fmt.Sprintf(`{"email":%q}`, email)), nil
	})}
	req := httptest.NewRequest(http.MethodPost, "/auth/google", strings.NewReader(`{"code":"c","redirect_uri":"http://localhost/auth/callback"}`))
	var out struct {
		Token string `json:"token"`
	}
	testutil.Call(t, h.GoogleLogin, req).Want(http.StatusOK).JSON(&out)
	assertAMR(t, out.Token, "google")
}

// A token minted from an existing session carries that session's methods.
func TestAMRCliTokenCopiesCallerSession(t *testing.T) {
	requirePlatformTestDatabase(t)
	caller, err := testHandler.issueJWT(userForAMRTest(), []string{"google"})
	if err != nil {
		t.Fatal(err)
	}
	req := newRequest("POST", "/api/cli-token", nil)
	req.Header.Set("Authorization", "Bearer "+caller)
	var out struct {
		Token string `json:"token"`
	}
	testutil.Call(t, testHandler.IssueCliToken, req).Want(http.StatusOK).JSON(&out)
	assertAMR(t, out.Token, "google")
}

func userForAMRTest() db.User {
	return db.User{ID: parseUUID(testUserID), Email: handlerTestEmail, Name: handlerTestName}
}
