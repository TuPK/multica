package handler

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/mfa"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

const mfaTestKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=" // 32 bytes, base64

// mfaUser is a user fixture whose user_mfa row is removed with it.
type mfaUser struct {
	ID    string
	Email string
}

func newMFAUser(t *testing.T) mfaUser {
	t.Helper()
	requirePlatformTestDatabase(t)
	t.Setenv(mfa.EnvKey, mfaTestKey)
	email := testutil.UniqueEmail("mfa")
	id := dbfx.User(t, "MFA User", email)
	dbfx.Cleanup(t, `DELETE FROM user_mfa WHERE user_id = $1`, id)
	return mfaUser{ID: id, Email: email}
}

// totpAt returns the code for the time step `steps` away from now.
func totpAt(t *testing.T, secret string, steps int) string {
	t.Helper()
	code, err := totp.GenerateCodeCustom(secret, time.Now().Add(time.Duration(steps)*30*time.Second), totp.ValidateOpts{
		Period: 30, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1,
	})
	if err != nil {
		t.Fatalf("generate totp: %v", err)
	}
	return code
}

// confirmedMFA stores a confirmed second factor for u and returns its secret
// and recovery codes, the way enrol and confirm would have left it.
func confirmedMFA(t *testing.T, u mfaUser) (secret string, recovery []string) {
	t.Helper()
	secret, _, err := mfa.NewSecret("Multica", u.Email)
	if err != nil {
		t.Fatal(err)
	}
	crypto, err := mfa.LoadCrypto()
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := crypto.Seal(secret)
	if err != nil {
		t.Fatal(err)
	}
	recovery, err = mfa.NewRecoveryCodes()
	if err != nil {
		t.Fatal(err)
	}
	hashes := make([]string, len(recovery))
	for i, c := range recovery {
		hashes[i] = crypto.HashRecoveryCode(c)
	}
	if _, err := testPool.Exec(context.Background(), `
		INSERT INTO user_mfa (user_id, secret_enc, confirmed_at, recovery_hashes)
		VALUES ($1, $2, now(), $3)`, u.ID, sealed, hashes); err != nil {
		t.Fatalf("insert user_mfa: %v", err)
	}
	return secret, recovery
}

func mfaRowCount(t *testing.T, userID string) int {
	t.Helper()
	return dbfx.Count(t, `SELECT count(*) FROM user_mfa WHERE user_id = $1`, userID)
}

func mfaRequest(userID, path string, body any) *http.Request {
	req := testutil.JSONRequest(http.MethodPost, path, body)
	req.Header.Set("X-User-ID", userID)
	return req
}

type enrollResponse struct {
	Secret string `json:"secret"`
	URI    string `json:"otpauth_uri"`
}

func TestMFAEnrollWithoutKeyReturns503AndStoresNothing(t *testing.T) {
	u := newMFAUser(t)
	t.Setenv(mfa.EnvKey, "")

	var out map[string]string
	testutil.Call(t, testHandler.EnrollMFA, mfaRequest(u.ID, "/api/me/mfa/enroll", nil)).
		Want(http.StatusServiceUnavailable).JSON(&out)
	if out["code"] != "mfa_unavailable" {
		t.Fatalf("code = %q, want mfa_unavailable", out["code"])
	}
	if n := mfaRowCount(t, u.ID); n != 0 {
		t.Fatalf("%d user_mfa rows stored with no key set", n)
	}
}

func TestMFAEnrollStoresTheSecretEncrypted(t *testing.T) {
	u := newMFAUser(t)

	var out enrollResponse
	testutil.Call(t, testHandler.EnrollMFA, mfaRequest(u.ID, "/api/me/mfa/enroll", nil)).
		Want(http.StatusOK).JSON(&out)
	if out.Secret == "" || !strings.HasPrefix(out.URI, "otpauth://totp/") || !strings.Contains(out.URI, out.Secret) {
		t.Fatalf("enrol response = %+v", out)
	}

	var stored []byte
	var confirmed *time.Time
	dbfx.QueryRow(t, `SELECT secret_enc, confirmed_at FROM user_mfa WHERE user_id = $1`, u.ID).Scan(&stored, &confirmed)
	if confirmed != nil {
		t.Fatal("enrol confirmed the secret")
	}
	if string(stored) == out.Secret || string(stored) == base64.StdEncoding.EncodeToString([]byte(out.Secret)) ||
		strings.Contains(string(stored), out.Secret) {
		t.Fatal("secret_enc holds the secret in plain text or as its base64")
	}
	crypto, err := mfa.LoadCrypto()
	if err != nil {
		t.Fatal(err)
	}
	opened, err := crypto.Open(stored)
	if err != nil || opened != out.Secret {
		t.Fatalf("secret_enc opens to %q, %v; want the enrolled secret", opened, err)
	}
}

func TestMFAEnrollAgainReplacesTheUnconfirmedSecret(t *testing.T) {
	u := newMFAUser(t)
	var first, second enrollResponse
	testutil.Call(t, testHandler.EnrollMFA, mfaRequest(u.ID, "/api/me/mfa/enroll", nil)).Want(http.StatusOK).JSON(&first)
	testutil.Call(t, testHandler.EnrollMFA, mfaRequest(u.ID, "/api/me/mfa/enroll", nil)).Want(http.StatusOK).JSON(&second)
	if first.Secret == second.Secret {
		t.Fatal("second enrol kept the first secret")
	}
	if n := mfaRowCount(t, u.ID); n != 1 {
		t.Fatalf("%d rows after two enrols, want 1", n)
	}
}

func TestMFAEnrollRefusedOnceConfirmed(t *testing.T) {
	u := newMFAUser(t)
	confirmedMFA(t, u)
	var before []byte
	dbfx.QueryRow(t, `SELECT secret_enc FROM user_mfa WHERE user_id = $1`, u.ID).Scan(&before)

	testutil.Call(t, testHandler.EnrollMFA, mfaRequest(u.ID, "/api/me/mfa/enroll", nil)).Want(http.StatusConflict)

	var after []byte
	dbfx.QueryRow(t, `SELECT secret_enc FROM user_mfa WHERE user_id = $1`, u.ID).Scan(&after)
	if string(before) != string(after) {
		t.Fatal("enrol replaced a confirmed secret")
	}
}

func TestMFAConfirmReturnsTenRecoveryCodesOnce(t *testing.T) {
	u := newMFAUser(t)
	var enrolled enrollResponse
	testutil.Call(t, testHandler.EnrollMFA, mfaRequest(u.ID, "/api/me/mfa/enroll", nil)).Want(http.StatusOK).JSON(&enrolled)

	// A wrong code does not confirm.
	testutil.Call(t, testHandler.ConfirmMFA, mfaRequest(u.ID, "/api/me/mfa/confirm", map[string]string{"code": "000000"})).
		Want(http.StatusBadRequest)
	if n := dbfx.Count(t, `SELECT count(*) FROM user_mfa WHERE user_id = $1 AND confirmed_at IS NOT NULL`, u.ID); n != 0 {
		t.Fatal("a wrong code confirmed MFA")
	}

	var out struct {
		RecoveryCodes []string `json:"recovery_codes"`
	}
	testutil.Call(t, testHandler.ConfirmMFA, mfaRequest(u.ID, "/api/me/mfa/confirm", map[string]string{"code": totpAt(t, enrolled.Secret, 0)})).
		Want(http.StatusOK).JSON(&out)
	if len(out.RecoveryCodes) != 10 {
		t.Fatalf("got %d recovery codes, want 10", len(out.RecoveryCodes))
	}

	var hashes []string
	var step int64
	dbfx.QueryRow(t, `SELECT recovery_hashes, last_used_step FROM user_mfa WHERE user_id = $1 AND confirmed_at IS NOT NULL`, u.ID).Scan(&hashes, &step)
	if len(hashes) != 10 {
		t.Fatalf("stored %d recovery hashes, want 10", len(hashes))
	}
	for _, code := range out.RecoveryCodes {
		for _, h := range hashes {
			if h == code {
				t.Fatalf("recovery code %q stored in plain text", code)
			}
		}
	}
	if step == 0 {
		t.Fatal("confirm did not record the step of the code it accepted")
	}

	// The codes are shown once: a second confirm is refused.
	testutil.Call(t, testHandler.ConfirmMFA, mfaRequest(u.ID, "/api/me/mfa/confirm", map[string]string{"code": totpAt(t, enrolled.Secret, 1)})).
		Want(http.StatusConflict)
}

func TestMFAConfirmWithoutEnrolmentIsRefused(t *testing.T) {
	u := newMFAUser(t)
	testutil.Call(t, testHandler.ConfirmMFA, mfaRequest(u.ID, "/api/me/mfa/confirm", map[string]string{"code": "123456"})).
		Want(http.StatusConflict)
}

func TestMFADisableNeedsAValidCode(t *testing.T) {
	u := newMFAUser(t)
	secret, _ := confirmedMFA(t, u)

	for _, code := range []string{"", "000000", "abcdef"} {
		testutil.Call(t, testHandler.DisableMFA, mfaRequest(u.ID, "/api/me/mfa/disable", map[string]string{"code": code})).
			Want(http.StatusBadRequest)
	}
	if n := mfaRowCount(t, u.ID); n != 1 {
		t.Fatal("MFA was disabled without a valid code")
	}

	testutil.Call(t, testHandler.DisableMFA, mfaRequest(u.ID, "/api/me/mfa/disable", map[string]string{"code": totpAt(t, secret, 0)})).
		Want(http.StatusNoContent)
	if n := mfaRowCount(t, u.ID); n != 0 {
		t.Fatal("the row survived a valid disable")
	}
}

func TestMFADisableWithARecoveryCode(t *testing.T) {
	u := newMFAUser(t)
	_, recovery := confirmedMFA(t, u)
	testutil.Call(t, testHandler.DisableMFA, mfaRequest(u.ID, "/api/me/mfa/disable", map[string]string{"code": recovery[3]})).
		Want(http.StatusNoContent)
	if n := mfaRowCount(t, u.ID); n != 0 {
		t.Fatal("the row survived a disable with a recovery code")
	}
}

func TestMFADisableWithoutMFAIsRefused(t *testing.T) {
	u := newMFAUser(t)
	testutil.Call(t, testHandler.DisableMFA, mfaRequest(u.ID, "/api/me/mfa/disable", map[string]string{"code": "123456"})).
		Want(http.StatusConflict)
}

func TestMFARoutesRefuseMachineCredentials(t *testing.T) {
	u := newMFAUser(t)
	for name, h := range map[string]http.HandlerFunc{
		"enroll":  testHandler.EnrollMFA,
		"confirm": testHandler.ConfirmMFA,
		"disable": testHandler.DisableMFA,
	} {
		req := mfaRequest(u.ID, "/api/me/mfa/"+name, map[string]string{"code": "123456"})
		req.Header.Set("X-Actor-Source", "task_token")
		testutil.Call(t, h, req).Want(http.StatusForbidden)
	}
	if n := mfaRowCount(t, u.ID); n != 0 {
		t.Fatal("a machine credential enrolled MFA")
	}
}
