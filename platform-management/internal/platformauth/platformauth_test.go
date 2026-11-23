package platformauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/accountguard"
	"github.com/multica-ai/multica/platform-management/internal/emailhash"
	"github.com/multica-ai/multica/platform-management/internal/session"
	"github.com/multica-ai/multica/platform-management/internal/staff"
)

var sixDigits = regexp.MustCompile(`^[0-9]{6}$`)

func macOf(code string) []byte {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(code))
	return m.Sum(nil)
}

// --- send-code ---

func TestSendCodeEligibleSendsOneMail(t *testing.T) {
	e := newEnv(t)
	rec := e.sendCode(staffEmail)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
	if e.mailer.count() != 1 {
		t.Fatalf("mails = %d, want 1", e.mailer.count())
	}
	mail := e.mailer.last()
	if mail.to != staffEmail || !sixDigits.MatchString(mail.code) {
		t.Fatalf("mail = %+v", mail)
	}
	entry := e.codes.entries[emailhash.Of(staffEmail)]
	if entry == nil {
		t.Fatal("no code stored under the email hash")
	}
	if !hmac.Equal(entry.mac, macOf(mail.code)) {
		t.Error("stored value is not HMAC(secret, code)")
	}
	if strings.Contains(string(entry.mac), mail.code) {
		t.Error("the code is stored in the clear")
	}
	if len(e.codes.ttls) != 1 || e.codes.ttls[0] != 10*time.Minute {
		t.Errorf("ttls = %v, want 10m", e.codes.ttls)
	}
}

func TestSendCodeRespondsTheSameInEveryCase(t *testing.T) {
	type tc struct {
		name  string
		setup func(e *env)
		email string
		body  string // overrides the email when set
		mails int
	}
	cases := []tc{
		{"eligible", func(e *env) {}, staffEmail, "", 1},
		{"unknown email", func(e *env) {}, "nobody@example.com", "", 0},
		{"user without a staff row", func(e *env) { e.dir.addUser(otherEmail, otherUserID) }, otherEmail, "", 0},
		{"revoked staff row", func(e *env) { e.dir.revoke(staffUserID) }, staffEmail, "", 0},
		{"suspended account", func(e *env) {
			e.guard.recs[staffUserID] = accountguard.Record{Status: accountguard.Suspended}
		}, staffEmail, "", 0},
		{"deleted account", func(e *env) {
			e.guard.recs[staffUserID] = accountguard.Record{Status: accountguard.Deleted}
		}, staffEmail, "", 0},
		{"rate-limited email", func(e *env) {
			e.limiter.counts["mpa:rl:email:"+emailhash.Of(staffEmail)] = 5
		}, staffEmail, "", 0},
		{"rate-limited ip", func(e *env) {
			e.limiter.counts["mpa:rl:ip:203.0.113.9"] = 20
		}, staffEmail, "", 0},
		{"limiter down", func(e *env) { e.limiter.err = errors.New("redis down") }, staffEmail, "", 0},
		{"code store down", func(e *env) { e.codes.err = errors.New("redis down") }, staffEmail, "", 0},
		{"mailer down", func(e *env) { e.mailer.err = errors.New("smtp down") }, staffEmail, "", 0},
		{"directory down", func(e *env) { e.dir.err = errors.New("db down") }, staffEmail, "", 0},
		{"malformed json", func(e *env) {}, "", `{"email":`, 0},
		{"missing email", func(e *env) {}, "", `{}`, 0},
		{"not an address", func(e *env) {}, "", `{"email":"nope"}`, 0},
	}
	var wantBody string
	var wantHeaders http.Header
	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			c.setup(e)
			var rec *httptest.ResponseRecorder
			if c.body != "" {
				rec = e.post("/api/platform/auth/send-code", c.body)
			} else {
				rec = e.sendCode(c.email)
			}
			if rec.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want 204", rec.Code)
			}
			// The mailer fake refuses in the "mailer down" case, so count what it kept.
			if got := e.mailer.count(); got != c.mails {
				t.Fatalf("mails = %d, want %d", got, c.mails)
			}
			if i == 0 {
				wantBody, wantHeaders = rec.Body.String(), rec.Header().Clone()
				return
			}
			if rec.Body.String() != wantBody {
				t.Errorf("body = %q, want %q", rec.Body.String(), wantBody)
			}
			if !headersEqual(rec.Header(), wantHeaders) {
				t.Errorf("headers = %v, want %v", rec.Header(), wantHeaders)
			}
		})
	}
}

func headersEqual(a, b http.Header) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if strings.Join(v, ",") != strings.Join(b[k], ",") {
			return false
		}
	}
	return true
}

func TestSendCodeNormalisesTheEmail(t *testing.T) {
	e := newEnv(t)
	e.sendCode("  Ops@Example.COM ")
	if e.mailer.count() != 1 || e.mailer.last().to != staffEmail {
		t.Fatalf("sent = %+v", e.mailer.sent)
	}
}

func TestSendCodeRateLimitsPerEmailAndPerIP(t *testing.T) {
	e := newEnv(t)
	e.sendCode(staffEmail)
	seen := map[string]limitCall{}
	for _, c := range e.limiter.calls {
		seen[c.key] = c
	}
	email := seen["mpa:rl:email:"+emailhash.Of(staffEmail)]
	if email.limit != 5 || email.window != 10*time.Minute {
		t.Errorf("email limit = %+v, want 5 per 10m", email)
	}
	ip := seen["mpa:rl:ip:203.0.113.9"]
	if ip.limit != 20 || ip.window != 10*time.Minute {
		t.Errorf("ip limit = %+v, want 20 per 10m", ip)
	}
}

func TestSendCodeStopsSendingAfterFiveRequestsForOneEmail(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < 8; i++ {
		if rec := e.sendCode(staffEmail); rec.Code != http.StatusNoContent {
			t.Fatalf("request %d: status %d", i, rec.Code)
		}
	}
	if got := e.mailer.count(); got != 5 {
		t.Errorf("mails = %d, want 5", got)
	}
}

func TestSendCodeCountsUnknownEmailsAgainstTheIPLimit(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < 25; i++ {
		e.sendCode("nobody" + string(rune('a'+i)) + "@example.com")
	}
	e.sendCode(staffEmail)
	if e.mailer.count() != 0 {
		t.Error("the IP limit did not apply to the 26th request")
	}
}

func TestSendCodeSeedsAConfiguredAdminAtLogin(t *testing.T) {
	e := newEnv(t)
	e.dir.revoke(staffUserID)
	e.seeder.grant = true
	e.sendCode(staffEmail)
	if len(e.seeder.calls) != 1 || e.seeder.calls[0] != staffEmail {
		t.Fatalf("seeder calls = %v", e.seeder.calls)
	}
	if e.mailer.count() != 1 {
		t.Error("the seeded admin did not get a code")
	}
}

func TestSendCodeDoesNotSeedAnEmailOutsideTheAdminList(t *testing.T) {
	e := newEnv(t)
	e.dir.addUser(otherEmail, otherUserID)
	e.seeder.grant = true
	e.sendCode(otherEmail)
	if len(e.seeder.calls) != 0 || e.mailer.count() != 0 {
		t.Errorf("seeder calls = %v, mails = %d", e.seeder.calls, e.mailer.count())
	}
}

func TestSendCodeSeedingFailureSendsNothing(t *testing.T) {
	e := newEnv(t)
	e.dir.revoke(staffUserID)
	e.seeder.err = errors.New("db down")
	if rec := e.sendCode(staffEmail); rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
	if e.mailer.count() != 0 {
		t.Error("mail sent although the seeding failed")
	}
}

func TestSendCodeReplacesAnEarlierCode(t *testing.T) {
	e := newEnv(t)
	e.sendCode(staffEmail)
	first := e.mailer.last().code
	e.sendCode(staffEmail)
	second := e.mailer.last().code
	if first == second {
		t.Skip("two random codes collided")
	}
	if rec := e.verify(staffEmail, first); rec.Code != http.StatusBadRequest {
		t.Errorf("the replaced code verified: %d", rec.Code)
	}
}

// --- verify-code ---

func (e *env) signedIn(t *testing.T) string {
	t.Helper()
	e.sendCode(staffEmail)
	return e.mailer.last().code
}

func TestVerifyCodeSignsIn(t *testing.T) {
	e := newEnv(t)
	code := e.signedIn(t)
	rec := e.verify(staffEmail, code)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["user_id"] != staffUserID || body["role"] != "admin" || len(body) != 2 {
		t.Errorf("body = %v", body)
	}

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != session.CookieName {
		t.Fatalf("cookies = %v", cookies)
	}
	c := cookies[0]
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode || c.Path != "/" || c.Domain != "" {
		t.Errorf("cookie attributes = %+v", c)
	}
	claims, err := session.Verify(secret, c.Value, e.now)
	if err != nil {
		t.Fatalf("cookie token: %v", err)
	}
	if claims.UserID != staffUserID || !claims.AuthTime().Equal(e.now) {
		t.Errorf("claims = %+v", claims)
	}

	// The code works once.
	if e.codes.has(staffEmail) {
		t.Error("the code survived a successful verification")
	}
	if rec := e.verify(staffEmail, code); rec.Code != http.StatusBadRequest {
		t.Errorf("second use of the code: %d", rec.Code)
	}
}

func TestVerifyCodeCookieWithoutSecure(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.SecureCookies = false })
	rec := e.verify(staffEmail, e.signedIn(t))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if c := rec.Result().Cookies()[0]; c.Secure {
		t.Error("Secure is set although secure cookies are off")
	}
}

func assertInvalidCode(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["code"] != "invalid_code" {
		t.Fatalf("body = %s", rec.Body)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Error("a failure set a cookie")
	}
}

func TestVerifyCodeEveryFailureIsTheSame(t *testing.T) {
	wrong := func(code string) string {
		if code == "000000" {
			return "000001"
		}
		return "000000"
	}
	cases := []struct {
		name string
		run  func(e *env) *httptest.ResponseRecorder
	}{
		{"wrong code", func(e *env) *httptest.ResponseRecorder { return e.verify(staffEmail, wrong(e.signedIn(t))) }},
		{"no code requested", func(e *env) *httptest.ResponseRecorder { return e.verify(staffEmail, "123456") }},
		{"unknown email", func(e *env) *httptest.ResponseRecorder { return e.verify("nobody@example.com", "123456") }},
		{"expired code", func(e *env) *httptest.ResponseRecorder {
			code := e.signedIn(t)
			e.now = e.now.Add(10*time.Minute + time.Second)
			return e.verify(staffEmail, code)
		}},
		{"staff revoked after the code was sent", func(e *env) *httptest.ResponseRecorder {
			code := e.signedIn(t)
			e.dir.revoke(staffUserID)
			return e.verify(staffEmail, code)
		}},
		{"account suspended after the code was sent", func(e *env) *httptest.ResponseRecorder {
			code := e.signedIn(t)
			e.guard.recs[staffUserID] = accountguard.Record{Status: accountguard.Suspended}
			return e.verify(staffEmail, code)
		}},
		{"code store down", func(e *env) *httptest.ResponseRecorder {
			code := e.signedIn(t)
			e.codes.err = errors.New("redis down")
			return e.verify(staffEmail, code)
		}},
		{"malformed body", func(e *env) *httptest.ResponseRecorder {
			return e.post("/api/platform/auth/verify-code", `{"email":`)
		}},
		{"empty code", func(e *env) *httptest.ResponseRecorder { e.signedIn(t); return e.verify(staffEmail, "") }},
		{"code with letters", func(e *env) *httptest.ResponseRecorder { e.signedIn(t); return e.verify(staffEmail, "abcdef") }},
		{"very long code", func(e *env) *httptest.ResponseRecorder {
			e.signedIn(t)
			return e.verify(staffEmail, strings.Repeat("9", 10000))
		}},
	}
	var wantBody string
	var wantHeaders http.Header
	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := c.run(newEnv(t))
			assertInvalidCode(t, rec)
			if i == 0 {
				wantBody, wantHeaders = rec.Body.String(), rec.Header().Clone()
				return
			}
			if rec.Body.String() != wantBody || !headersEqual(rec.Header(), wantHeaders) {
				t.Errorf("response differs: %q %v", rec.Body.String(), rec.Header())
			}
		})
	}
}

func TestVerifyCodeFifthWrongAttemptDeletesTheCode(t *testing.T) {
	e := newEnv(t)
	code := e.signedIn(t)
	wrong := "000000"
	if code == wrong {
		wrong = "000001"
	}
	for i := 1; i <= 5; i++ {
		if !e.codes.has(staffEmail) {
			t.Fatalf("code deleted before attempt %d", i)
		}
		assertInvalidCode(t, e.verify(staffEmail, wrong))
	}
	if e.codes.has(staffEmail) {
		t.Fatal("the code survived the fifth wrong attempt")
	}
	assertInvalidCode(t, e.verify(staffEmail, code))
}

func TestVerifyCodeFourWrongAttemptsThenTheRightOne(t *testing.T) {
	e := newEnv(t)
	code := e.signedIn(t)
	wrong := "000000"
	if code == wrong {
		wrong = "000001"
	}
	for i := 0; i < 4; i++ {
		assertInvalidCode(t, e.verify(staffEmail, wrong))
	}
	if rec := e.verify(staffEmail, code); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestVerifyCodeAcceptsTheEmailInAnyCase(t *testing.T) {
	e := newEnv(t)
	code := e.signedIn(t)
	if rec := e.verify("OPS@example.com", code); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

// --- origin rule ---

func TestForeignOriginIsRefusedOnEveryPost(t *testing.T) {
	for _, path := range []string{"/api/platform/auth/send-code", "/api/platform/auth/verify-code", "/api/platform/auth/logout"} {
		t.Run(path, func(t *testing.T) {
			e := newEnv(t)
			for _, bad := range []string{"https://evil.example.com", "http://console.example.com", "https://console.example.com.evil.com", "null", "https://console.example.com/"} {
				rec := e.do(request{method: "POST", path: path, body: `{"email":"` + staffEmail + `","code":"123456"}`, headers: map[string]string{"Origin": bad}, remoteAddr: "203.0.113.9:1"})
				if rec.Code != http.StatusForbidden {
					t.Errorf("Origin %q: status = %d, want 403", bad, rec.Code)
					continue
				}
				var body map[string]string
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["code"] != "bad_origin" {
					t.Errorf("Origin %q: body = %s", bad, rec.Body)
				}
			}
			if e.mailer.count() != 0 || len(e.limiter.calls) != 0 {
				t.Error("a refused request had effects")
			}
		})
	}
}

func TestMissingOriginIsAllowedOnlyWithoutACookie(t *testing.T) {
	e := newEnv(t)
	rec := e.do(request{method: "POST", path: "/api/platform/auth/send-code", body: `{"email":"` + staffEmail + `"}`, remoteAddr: "203.0.113.9:1"})
	if rec.Code != http.StatusNoContent {
		t.Errorf("no Origin, no cookie: status = %d, want 204", rec.Code)
	}
	rec = e.do(request{method: "POST", path: "/api/platform/auth/logout", cookies: []*http.Cookie{sessionCookie("x")}, remoteAddr: "203.0.113.9:1"})
	if rec.Code != http.StatusForbidden {
		t.Errorf("no Origin, with a cookie: status = %d, want 403", rec.Code)
	}
}

func TestOriginMatchesCaseInsensitivelyOnTheHostOnly(t *testing.T) {
	e := newEnv(t)
	rec := e.do(request{method: "POST", path: "/api/platform/auth/logout", headers: map[string]string{"Origin": "https://CONSOLE.example.com"}, remoteAddr: "203.0.113.9:1"})
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d", rec.Code)
	}
}

// --- logout ---

func TestLogoutClearsTheCookie(t *testing.T) {
	e := newEnv(t)
	rec := e.post("/api/platform/auth/logout", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != session.CookieName || cookies[0].Value != "" || cookies[0].MaxAge >= 0 {
		t.Fatalf("cookies = %+v", cookies)
	}
}

// --- middleware ---

// failedResponse is what an unknown route answers in the same handler.
func (e *env) unknownRoute() *httptest.ResponseRecorder {
	return e.do(request{method: "GET", path: "/api/platform/no-such-route", remoteAddr: "203.0.113.9:1"})
}

func (e *env) me(r request) *httptest.ResponseRecorder {
	r.method, r.path = "GET", "/api/platform/me"
	if r.remoteAddr == "" {
		r.remoteAddr = "203.0.113.9:1"
	}
	return e.do(r)
}

func assertNotFoundLikeUnknownRoute(t *testing.T, e *env, rec *httptest.ResponseRecorder) {
	t.Helper()
	unknown := e.unknownRoute()
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body %s)", rec.Code, rec.Body)
	}
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown route status = %d", unknown.Code)
	}
	if rec.Body.String() != unknown.Body.String() || !headersEqual(rec.Header(), unknown.Header()) {
		t.Errorf("response differs from an unknown route:\n got %q %v\nwant %q %v", rec.Body.String(), rec.Header(), unknown.Body.String(), unknown.Header())
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Error("the 404 set a cookie")
	}
}

func TestMeReturnsRoleAndTenantID(t *testing.T) {
	const tenant = "11111111-aaaa-bbbb-cccc-000000000001"
	cases := []struct {
		name       string
		role       staff.Role
		tenant     string
		wantTenant any
	}{
		{"admin of a tenant", staff.RoleAdmin, tenant, tenant},
		{"system admin has no tenant", staff.RoleSystemAdmin, "", nil},
		{"support has no tenant", staff.RoleSupport, "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.dir.active[staffUserID] = tc.role
			e.dir.tenants[staffUserID] = tc.tenant
			rec := e.me(request{cookies: []*http.Cookie{sessionCookie(e.token(staffUserID, e.now.Add(-time.Hour)))}})
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			tid, present := body["tenant_id"]
			if !present || tid != tc.wantTenant {
				t.Errorf("tenant_id = %v (present %v), want %v", tid, present, tc.wantTenant)
			}
			if body["user_id"] != staffUserID || body["email"] != staffEmail || body["role"] != string(tc.role) || len(body) != 4 {
				t.Errorf("body = %v", body)
			}
		})
	}
}

func TestIdentityCarriesTenantID(t *testing.T) {
	const tenant = "11111111-aaaa-bbbb-cccc-000000000001"
	e := newEnv(t)
	e.dir.tenants[staffUserID] = tenant
	var got Identity
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = IdentityFrom(r.Context())
	})
	req := httptest.NewRequest("GET", "/x", nil)
	req.RemoteAddr = "203.0.113.9:1"
	req.AddCookie(sessionCookie(e.token(staffUserID, e.now)))
	e.h.RequirePlatformStaff()(inner).ServeHTTP(httptest.NewRecorder(), req)
	if got.UserID != staffUserID || got.TenantID != tenant {
		t.Errorf("identity = %+v, want tenant %s", got, tenant)
	}
}

func TestMiddlewareFailuresAreAll404(t *testing.T) {
	good := func(e *env) string { return e.token(staffUserID, e.now.Add(-time.Hour)) }
	cases := []struct {
		name  string
		setup func(e *env) request
	}{
		{"no cookie", func(e *env) request { return request{} }},
		{"multica_auth cookie only", func(e *env) request {
			return request{cookies: []*http.Cookie{{Name: "multica_auth", Value: good(e)}}}
		}},
		{"a token in Authorization only", func(e *env) request {
			return request{headers: map[string]string{"Authorization": "Bearer " + good(e)}}
		}},
		{"a PAT in Authorization", func(e *env) request {
			return request{headers: map[string]string{"Authorization": "Bearer mul_0123456789abcdef0123456789abcdef01234567"}}
		}},
		{"a valid cookie for a revoked staff row", func(e *env) request {
			tok := good(e)
			e.dir.revoke(staffUserID)
			return request{cookies: []*http.Cookie{sessionCookie(tok)}}
		}},
		{"a valid cookie for a user who was never staff", func(e *env) request {
			e.dir.addUser(otherEmail, otherUserID)
			return request{cookies: []*http.Cookie{sessionCookie(e.token(otherUserID, e.now))}}
		}},
		{"a suspended Delegora account", func(e *env) request {
			e.guard.recs[staffUserID] = accountguard.Record{Status: accountguard.Suspended}
			return request{cookies: []*http.Cookie{sessionCookie(good(e))}}
		}},
		{"a deleted Delegora account", func(e *env) request {
			e.guard.recs[staffUserID] = accountguard.Record{Status: accountguard.Deleted}
			return request{cookies: []*http.Cookie{sessionCookie(good(e))}}
		}},
		{"an account pending deletion", func(e *env) request {
			e.guard.recs[staffUserID] = accountguard.Record{Status: accountguard.PendingDeletion}
			return request{cookies: []*http.Cookie{sessionCookie(good(e))}}
		}},
		{"sessions revoked after auth_time", func(e *env) request {
			e.guard.recs[staffUserID] = accountguard.Record{Status: accountguard.Active, SessionsValidAfter: e.now.Add(-30 * time.Minute)}
			return request{cookies: []*http.Cookie{sessionCookie(good(e))}}
		}},
		{"a token older than 12 hours", func(e *env) request {
			return request{cookies: []*http.Cookie{sessionCookie(e.token(staffUserID, e.now.Add(-12*time.Hour-time.Second)))}}
		}},
		{"a session that never passed the second factor", func(e *env) request {
			tok, _ := session.Sign(secret, staffUserID, e.now)
			return request{cookies: []*http.Cookie{sessionCookie(tok)}}
		}},
		{"a token signed with another secret", func(e *env) request {
			tok, _ := SignMFASession("another-secret-another-secret-0123", staffUserID, e.now)
			return request{cookies: []*http.Cookie{sessionCookie(tok)}}
		}},
		{"a malformed token", func(e *env) request {
			return request{cookies: []*http.Cookie{sessionCookie("not-a-token")}}
		}},
		{"an empty cookie", func(e *env) request { return request{cookies: []*http.Cookie{sessionCookie("")}} }},
		{"an IP outside PLATFORM_ADMIN_CIDRS", func(e *env) request {
			return request{cookies: []*http.Cookie{sessionCookie(good(e))}, remoteAddr: "198.51.100.7:1"}
		}},
		{"the staff store failing", func(e *env) request {
			e.dir.staffErr = errors.New("db down")
			return request{cookies: []*http.Cookie{sessionCookie(good(e))}}
		}},
		{"the account guard failing", func(e *env) request {
			e.guard.err = errors.New("db down")
			return request{cookies: []*http.Cookie{sessionCookie(good(e))}}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t, withCIDRs("203.0.113.0/24"))
			rec := e.me(c.setup(e))
			assertNotFoundLikeUnknownRoute(t, e, rec)
		})
	}
}

func TestMiddlewareFailureMatchesTheConsoleNotFound(t *testing.T) {
	e := newEnv(t)
	rec := e.me(request{})
	console := httptest.NewRecorder()
	http.NotFoundHandler().ServeHTTP(console, httptest.NewRequest("GET", "/api/other", nil))
	if rec.Body.String() != console.Body.String() || rec.Header().Get("Content-Type") != console.Header().Get("Content-Type") {
		t.Errorf("got %q %v, console answers %q %v", rec.Body.String(), rec.Header(), console.Body.String(), console.Header())
	}
}

func TestMiddlewareReadsOnlyTheSessionCookie(t *testing.T) {
	e := newEnv(t)
	tok := e.token(staffUserID, e.now)
	rec := e.me(request{
		cookies: []*http.Cookie{sessionCookie(tok), {Name: "multica_auth", Value: "garbage"}},
		headers: map[string]string{"Authorization": "Bearer garbage"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: unrelated credentials changed the outcome", rec.Code)
	}
}

func TestMiddlewareAllowsAnIPInsideTheCIDRs(t *testing.T) {
	e := newEnv(t, withCIDRs("198.51.100.0/24", "203.0.113.0/24"))
	rec := e.me(request{cookies: []*http.Cookie{sessionCookie(e.token(staffUserID, e.now))}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestMiddlewareClientIPBehindTheConsoleProxy(t *testing.T) {
	// The console proxy connects from loopback and appends the browser's
	// address to X-Forwarded-For.
	e := newEnv(t, withCIDRs("203.0.113.0/24"))
	tok := sessionCookie(e.token(staffUserID, e.now))

	rec := e.me(request{cookies: []*http.Cookie{tok}, remoteAddr: "127.0.0.1:5555", headers: map[string]string{"X-Forwarded-For": "203.0.113.50"}})
	if rec.Code != http.StatusOK {
		t.Errorf("proxied client inside the CIDR: status = %d", rec.Code)
	}
	rec = e.me(request{cookies: []*http.Cookie{tok}, remoteAddr: "127.0.0.1:5555", headers: map[string]string{"X-Forwarded-For": "203.0.113.50, 198.51.100.9"}})
	if rec.Code != http.StatusNotFound {
		t.Errorf("a forged leading address must not count; status = %d", rec.Code)
	}
	// A header from a peer that is not the proxy is ignored.
	rec = e.me(request{cookies: []*http.Cookie{tok}, remoteAddr: "198.51.100.9:5555", headers: map[string]string{"X-Forwarded-For": "203.0.113.50"}})
	if rec.Code != http.StatusNotFound {
		t.Errorf("X-Forwarded-For from a remote peer was trusted; status = %d", rec.Code)
	}
}

func TestMiddlewareRolesRestrictAccess(t *testing.T) {
	e := newEnv(t)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := IdentityFrom(r.Context())
		if !ok {
			t.Error("no identity in the context")
		}
		w.Write([]byte(id.UserID + " " + string(id.Role)))
	})
	serve := func(mw func(http.Handler) http.Handler, tok string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/x", nil)
		req.RemoteAddr = "203.0.113.9:1"
		req.AddCookie(sessionCookie(tok))
		rec := httptest.NewRecorder()
		mw(inner).ServeHTTP(rec, req)
		return rec
	}
	tok := e.token(staffUserID, e.now)

	e.dir.active[staffUserID] = staff.RoleSupport
	if rec := serve(e.h.RequirePlatformStaff(staff.RoleAdmin), tok); rec.Code != http.StatusNotFound {
		t.Errorf("support on an admin route: %d", rec.Code)
	}
	if rec := serve(e.h.RequirePlatformStaff(staff.RoleSupport, staff.RoleAdmin), tok); rec.Code != http.StatusOK || rec.Body.String() != staffUserID+" support" {
		t.Errorf("support on a support route: %d %q", rec.Code, rec.Body)
	}
	if rec := serve(e.h.RequirePlatformStaff(), tok); rec.Code != http.StatusOK {
		t.Errorf("no role list means any staff role: %d", rec.Code)
	}
}

func TestRevocationTakesEffectOnTheNextRequest(t *testing.T) {
	e := newEnv(t)
	req := request{cookies: []*http.Cookie{sessionCookie(e.token(staffUserID, e.now))}}
	if rec := e.me(req); rec.Code != http.StatusOK {
		t.Fatalf("before revocation: %d", rec.Code)
	}
	e.dir.revoke(staffUserID)
	if rec := e.me(req); rec.Code != http.StatusNotFound {
		t.Fatalf("after revocation: %d", rec.Code)
	}
}

func TestSessionsValidAfterAtAuthTimeStillPasses(t *testing.T) {
	// Delegora's rule: a token issued at the mark survives it (whole seconds, strict <).
	e := newEnv(t)
	auth := e.now.Add(-time.Minute)
	e.guard.recs[staffUserID] = accountguard.Record{Status: accountguard.Active, SessionsValidAfter: auth}
	rec := e.me(request{cookies: []*http.Cookie{sessionCookie(e.token(staffUserID, auth))}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

// --- logs ---

func TestNothingSecretIsLogged(t *testing.T) {
	e := newEnv(t)
	code := e.signedIn(t)
	wrong := "000000"
	if code == wrong {
		wrong = "000001"
	}
	e.verify(staffEmail, wrong)
	e.mailer.err = errors.New("smtp down")
	e.sendCode(staffEmail)
	e.codes.err = errors.New("redis down")
	e.sendCode(staffEmail)
	e.codes.err = nil
	rec := e.verify(staffEmail, code)
	tok := rec.Result().Cookies()
	e.dir.staffErr = errors.New("db down")
	e.me(request{cookies: []*http.Cookie{sessionCookie("garbage-token-value")}})

	logs := e.logs.String()
	if logs == "" {
		t.Fatal("nothing was logged; the test would prove nothing")
	}
	secrets := []string{code, secret, "garbage-token-value", staffEmail}
	for _, c := range tok {
		secrets = append(secrets, c.Value)
	}
	for _, s := range secrets {
		if s != "" && strings.Contains(logs, s) {
			t.Errorf("logs contain %q:\n%s", s, logs)
		}
	}
}

func TestRequireOriginAcceptsEveryConfiguredConsoleOrigin(t *testing.T) {
	h := &Handler{Deps: Deps{Config: Config{
		ConsoleOrigin:  "http://localhost:30001",
		ConsoleOrigins: []string{"http://localhost:30001", "http://10.70.39.196:30001"},
	}}}
	ok := h.RequireOrigin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	for origin, want := range map[string]int{
		"http://10.70.39.196:30001": http.StatusNoContent,
		"http://localhost:30001":    http.StatusNoContent,
		"http://evil.example.com":   http.StatusForbidden,
	} {
		r := httptest.NewRequest(http.MethodPost, "/api/platform/auth/send-code", nil)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		ok.ServeHTTP(w, r)
		if w.Code != want {
			t.Errorf("origin %q: got %d, want %d", origin, w.Code, want)
		}
	}
}
