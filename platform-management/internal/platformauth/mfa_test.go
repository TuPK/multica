package platformauth

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/mfa"
	"github.com/multica-ai/multica/platform-management/internal/session"
)

// wrongTOTP returns a six-digit code that is not valid at the test clock.
func (e *env) wrongTOTP() string {
	e.t.Helper()
	for _, c := range []string{"000000", "000001"} {
		if _, ok := mfa.MatchStep(mfaSecret, c, e.now); !ok {
			return c
		}
	}
	e.t.Fatal("no wrong code found")
	return ""
}

func TestStaffWithoutConfirmedMFACannotSignIn(t *testing.T) {
	e := newEnv(t)
	delete(e.mfa.rows, staffUserID)
	code := e.signedIn(t)

	rec := e.verifyWith(staffEmail, code, "123456")
	assertInvalidCode(t, rec)

	// The answer is the one a non-staff email gets.
	e.dir.addUser(otherEmail, otherUserID)
	e.sendCode(otherEmail)
	assertInvalidCode(t, e.verifyWith(otherEmail, "000000", "123456"))
	if rec.Body.String() != e.verifyWith(otherEmail, "000000", "123456").Body.String() {
		t.Error("a staff member without MFA gets a different body from a non-staff email")
	}
}

func TestEmailCodeAloneAsksForTheTOTPCode(t *testing.T) {
	e := newEnv(t)
	code := e.signedIn(t)

	rec := e.verifyWith(staffEmail, code, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["mfa_required"] != true || len(body) != 1 {
		t.Errorf("body = %v", body)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Error("a session was issued before the TOTP code")
	}
	if !e.codes.has(staffEmail) {
		t.Fatal("the email code was spent before the TOTP step")
	}

	// The same email code then completes the sign-in.
	if rec := e.verifyWith(staffEmail, code, e.totp()); rec.Code != http.StatusOK || len(rec.Result().Cookies()) != 1 {
		t.Fatalf("second step: status %d, body %s", rec.Code, rec.Body)
	}
	if e.codes.has(staffEmail) {
		t.Error("the email code survived the sign-in")
	}
}

func TestWrongEmailCodeNeverReachesTheTOTPStep(t *testing.T) {
	e := newEnv(t)
	e.signedIn(t)
	assertInvalidCode(t, e.verifyWith(staffEmail, "000000", ""))
}

func TestSessionRecordsMFAInAMR(t *testing.T) {
	e := newEnv(t)
	rec := e.verify(staffEmail, e.signedIn(t))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	claims, err := session.Verify(secret, rec.Result().Cookies()[0].Value, e.now)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(claims.AMR, "mfa") || !slices.Contains(claims.AMR, session.MethodEmailCode) {
		t.Errorf("amr = %v, want email_code and mfa", claims.AMR)
	}
	// The cookie opens the console.
	if rec := e.me(request{cookies: rec.Result().Cookies()}); rec.Code != http.StatusOK {
		t.Errorf("me with the new session: %d", rec.Code)
	}
}

func TestSignMFASessionVerifies(t *testing.T) {
	tok, err := SignMFASession(secret, staffUserID, t0)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := session.Verify(secret, tok, t0)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != staffUserID || !claims.AuthTime().Equal(t0) || !slices.Equal(claims.AMR, []string{"email_code", "mfa"}) {
		t.Errorf("claims = %+v", claims)
	}
}

func TestWrongTOTPCodeIsRefused(t *testing.T) {
	e := newEnv(t)
	code := e.signedIn(t)
	assertInvalidCode(t, e.verifyWith(staffEmail, code, e.wrongTOTP()))
	if !e.codes.has(staffEmail) {
		t.Fatal("one wrong TOTP code spent the email code")
	}
	// A malformed code is refused the same way.
	assertInvalidCode(t, e.verifyWith(staffEmail, code, "abc"))
	// The right one still works.
	if rec := e.verifyWith(staffEmail, code, e.totp()); rec.Code != http.StatusOK {
		t.Fatalf("right code after wrong ones: %d", rec.Code)
	}
}

func TestWrongTOTPCodesSpendTheEmailCodeAttempts(t *testing.T) {
	e := newEnv(t)
	code := e.signedIn(t)
	for i := 0; i < 5; i++ {
		assertInvalidCode(t, e.verifyWith(staffEmail, code, e.wrongTOTP()))
	}
	if e.codes.has(staffEmail) {
		t.Fatal("the email code survived five failed attempts")
	}
	assertInvalidCode(t, e.verifyWith(staffEmail, code, e.totp()))
}

func TestAnAttemptBeyondTheLimitIsRefusedEvenWithRightCodes(t *testing.T) {
	e := newEnv(t)
	code := e.signedIn(t)
	for i := 0; i < 4; i++ {
		assertInvalidCode(t, e.verifyWith(staffEmail, code, e.wrongTOTP()))
	}
	// The fifth attempt, right email code and no TOTP code, is allowed to ask.
	if rec := e.verifyWith(staffEmail, code, ""); rec.Code != http.StatusOK {
		t.Fatalf("fifth attempt: %d", rec.Code)
	}
	// A sixth attempt is over the limit although both codes are right.
	assertInvalidCode(t, e.verifyWith(staffEmail, code, e.totp()))
}

func TestATOTPStepCannotBeUsedTwice(t *testing.T) {
	e := newEnv(t)
	if rec := e.verify(staffEmail, e.signedIn(t)); rec.Code != http.StatusOK {
		t.Fatalf("first sign-in: %d", rec.Code)
	}

	// A new email code with the same TOTP code, in the same step.
	code := e.signedIn(t)
	assertInvalidCode(t, e.verifyWith(staffEmail, code, e.totp()))

	// The next step is accepted.
	e.now = e.now.Add(mfa.Period * time.Second)
	if rec := e.verifyWith(staffEmail, code, e.totp()); rec.Code != http.StatusOK {
		t.Fatalf("next step: %d", rec.Code)
	}
}

func TestAnEarlierStepThanTheLastUsedIsRefused(t *testing.T) {
	e := newEnv(t)
	// The user signed in to Multica with a code from the step after now.
	step, _ := mfa.MatchStep(mfaSecret, e.totp(), e.now)
	e.mfa.rows[staffUserID].lastStep = step + 1
	assertInvalidCode(t, e.verify(staffEmail, e.signedIn(t)))
}

// The development code signs in any active staff member on its own, with or without a
// confirmed second factor. Real email codes still need the TOTP step (see above).
func TestTheDevCodeSignsInStaffWithoutASecondFactor(t *testing.T) {
	e := newEnv(t, withDevCode)
	e.sendCode(staffEmail)
	if rec := e.verifyWith(staffEmail, devCode, ""); rec.Code != http.StatusOK || len(rec.Result().Cookies()) == 0 {
		t.Fatalf("dev code alone: status %d, cookies %d", rec.Code, len(rec.Result().Cookies()))
	}
}

func TestTheDevCodeStillRefusesUnknownAndNonStaffAccounts(t *testing.T) {
	e := newEnv(t, withDevCode)
	assertInvalidCode(t, e.verifyWith("nobody@example.com", devCode, ""))
}

func TestSignInFailsClosedWhenTheMFAStoreFails(t *testing.T) {
	e := newEnv(t)
	code := e.signedIn(t)
	e.mfa.err = http.ErrAbortHandler
	assertInvalidCode(t, e.verify(staffEmail, code))
}

func TestSignInFailsClosedWhenTheSecretDoesNotOpen(t *testing.T) {
	e := newEnv(t)
	other := testKey()
	other[0] ^= 0xff
	e.mfa.rows[staffUserID].sealed = seal(t, other, mfaSecret)
	assertInvalidCode(t, e.verify(staffEmail, e.signedIn(t)))
}
