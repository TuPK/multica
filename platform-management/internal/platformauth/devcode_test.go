package platformauth

import (
	"net/http"
	"strings"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/emailhash"
)

const devCode = "666666"

func withDevCode(c *Config) { c.DevVerificationCode = devCode }

// newDevEnv is a handler with the dev code and no email provider.
func newDevEnv(t *testing.T) *env {
	t.Helper()
	e := newEnv(t, withDevCode)
	e.h.Mailer = nil
	return e
}

func TestSendCodeWithDevCodeAndNoProviderSendsNothing(t *testing.T) {
	e := newDevEnv(t)
	rec := e.sendCode(staffEmail)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
	if e.mailer.count() != 0 {
		t.Fatal("the mailer was called")
	}
	if e.codes.has(staffEmail) {
		t.Error("a code was stored although none can be sent")
	}
	logs := e.logs.String()
	if n := strings.Count(logs, emailhash.Of(staffEmail)); n != 1 {
		t.Errorf("email hash logged %d times, want 1:\n%s", n, logs)
	}
	for _, secret := range []string{staffEmail, devCode} {
		if strings.Contains(logs, secret) {
			t.Errorf("logs contain %q:\n%s", secret, logs)
		}
	}
}

func TestSendCodeWithDevCodeAndAProviderStillMails(t *testing.T) {
	e := newEnv(t, withDevCode)
	if rec := e.sendCode(staffEmail); rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
	if e.mailer.count() != 1 {
		t.Fatalf("mails = %d, want 1", e.mailer.count())
	}
	// The real code and the dev code both work.
	if rec := e.verify(staffEmail, e.mailer.last().code); rec.Code != http.StatusOK {
		t.Errorf("real code: status = %d", rec.Code)
	}
	if rec := e.verify(staffEmail, devCode); rec.Code != http.StatusOK {
		t.Errorf("dev code: status = %d", rec.Code)
	}
}

func TestVerifyCodeDevCodeSignsInStaff(t *testing.T) {
	e := newDevEnv(t)
	rec := e.verify(staffEmail, devCode)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"role":"admin"`) || !strings.Contains(rec.Body.String(), staffUserID) {
		t.Errorf("body = %s", rec.Body)
	}
	if len(rec.Result().Cookies()) != 1 {
		t.Errorf("cookies = %v", rec.Result().Cookies())
	}
	// The dev code is not used up.
	if rec := e.verify(staffEmail, devCode); rec.Code != http.StatusOK {
		t.Errorf("second use: status = %d", rec.Code)
	}
}

func TestVerifyCodeDevCodeForNonStaffIsRefusedLikeAWrongCode(t *testing.T) {
	e := newDevEnv(t)
	e.dir.addUser(otherEmail, otherUserID) // a user without a staff row
	nonStaff := e.verify(otherEmail, devCode)
	assertInvalidCode(t, nonStaff)
	unknown := e.verify("nobody@example.com", devCode)
	assertInvalidCode(t, unknown)
	wrong := e.verify(otherEmail, "000000")
	assertInvalidCode(t, wrong)
	if nonStaff.Body.String() != wrong.Body.String() || len(nonStaff.Result().Cookies()) != 0 {
		t.Errorf("non-staff answer %q differs from wrong code answer %q", nonStaff.Body, wrong.Body)
	}
	// A revoked staff member is refused too.
	e.dir.revoke(staffUserID)
	assertInvalidCode(t, e.verify(staffEmail, devCode))
}

func TestVerifyCodeWrongCodeIsRefusedWithADevCode(t *testing.T) {
	e := newDevEnv(t)
	rec := e.verify(staffEmail, "123456")
	assertInvalidCode(t, rec)
	if len(rec.Result().Cookies()) != 0 {
		t.Error("a cookie was set")
	}
}

func TestVerifyCodeDevCodeIsRefusedWhenNotConfigured(t *testing.T) {
	e := newEnv(t)
	assertInvalidCode(t, e.verify(staffEmail, devCode))
	assertInvalidCode(t, e.verify(staffEmail, ""))
}

func TestDevCodeComparisonIsConstantTime(t *testing.T) {
	// The comparison is subtle.ConstantTimeCompare over fixed-length HMACs, so
	// the submitted code's length and prefix do not change the work done.
	// Reviewed in devCodeMatches; here we pin its behaviour.
	e := newDevEnv(t)
	for code, want := range map[string]bool{devCode: true, "66666": false, "6666666": false, "": false, "666667": false} {
		if got := e.h.devCodeMatches(code); got != want {
			t.Errorf("devCodeMatches(%q) = %v, want %v", code, got, want)
		}
	}
	if newEnv(t).h.devCodeMatches("") {
		t.Error("an empty code matched when no dev code is set")
	}
}
