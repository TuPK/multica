package pricechange

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/accountguard"
	"github.com/multica-ai/multica/platform-management/internal/platformauth"
	"github.com/multica-ai/multica/platform-management/internal/session"
	"github.com/multica-ai/multica/platform-management/internal/staff"
)

const (
	secret = "0123456789abcdef0123456789abcdef"
	origin = "https://console.example.com"

	adminUser = "aaaaaaaa-0000-0000-0000-000000000001"
	suppUser  = "aaaaaaaa-0000-0000-0000-000000000002"
	billUser  = "aaaaaaaa-0000-0000-0000-000000000003"

	changesPath   = "/api/platform/plans/pro/price-changes"
	scheduledPath = "/api/platform/plans/pro/price-changes/scheduled"
	postBody      = `{"effective_from":"2026-04-10T12:00:00Z","reason":"annual review","new":{"seat_price_month":120000,"seat_price_year":1200000}}`
)

type fakeDirectory struct{}

func (fakeDirectory) UserIDByEmail(context.Context, string) (string, bool, error) {
	return "", false, nil
}

func (fakeDirectory) ActiveStaff(_ context.Context, id string) (staff.Role, string, bool, error) {
	switch id {
	case adminUser:
		return staff.RoleSystemAdmin, "", true, nil
	case suppUser:
		return staff.RoleSupport, "", true, nil
	case billUser:
		return staff.RoleBilling, "", true, nil
	}
	return "", "", false, nil
}

func (fakeDirectory) EmailByUserID(_ context.Context, id string) (string, error) {
	if id == adminUser {
		return "admin@example.com", nil
	}
	return "", errors.New("no such user")
}

type activeGuard struct{}

func (activeGuard) State(context.Context, string) (accountguard.Record, error) {
	return accountguard.Record{Status: accountguard.Active}, nil
}

type env struct {
	t  *testing.T
	h  http.Handler
	st *fakeStore
}

func newEnv(t *testing.T) *env {
	t.Helper()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	auth := platformauth.New(platformauth.Deps{
		Config:    platformauth.Config{SessionSecret: secret, ConsoleOrigin: origin},
		Directory: fakeDirectory{}, Guard: activeGuard{}, Log: quiet,
		Now: func() time.Time { return t0 },
	})
	st := newFakeStore()
	h := NewHandler(HandlerDeps{
		RequireStaff: auth.RequirePlatformStaff, RequireOrigin: auth.RequireOrigin,
		Service: newService(t, st, &fakeMail{}, nil), Emails: fakeDirectory{},
		Log: quiet, Now: func() time.Time { return t0 },
	})
	return &env{t: t, h: h, st: st}
}

func (e *env) do(user, method, path, body string) *httptest.ResponseRecorder {
	e.t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rd)
	if user != "" {
		tok, err := platformauth.SignMFASession(secret, user, t0.Add(-time.Minute))
		if err != nil {
			e.t.Fatal(err)
		}
		req.AddCookie(&http.Cookie{Name: session.CookieName, Value: tok})
	}
	if method != http.MethodGet {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body is not a JSON object: %q", rec.Body)
	}
	return m
}

// --- Test 1: who may call what.

func TestRoutesAdmitOnlyTheirRoles(t *testing.T) {
	cases := []struct {
		name, method, path, body string
		allowed                  []string
	}{
		{"history", "GET", changesPath, "", []string{billUser, adminUser}},
		{"schedule", "POST", changesPath, postBody, []string{adminUser}},
		{"dry run", "POST", changesPath + "?dry_run=1", postBody, []string{adminUser}},
		{"cancel", "DELETE", scheduledPath, "", []string{adminUser}},
	}
	for _, tc := range cases {
		for _, user := range []string{"", suppUser, billUser, adminUser} {
			e := newEnv(t)
			if tc.method == "DELETE" {
				// Something to cancel, so an allowed call is a success.
				if rec := e.do(adminUser, "POST", changesPath, postBody); rec.Code != 201 {
					t.Fatalf("setup: %d %s", rec.Code, rec.Body)
				}
				e.st.audits = nil
			}
			before := len(e.st.changes)
			rec := e.do(user, tc.method, tc.path, tc.body)
			allowed := false
			for _, a := range tc.allowed {
				allowed = allowed || a == user
			}
			if allowed && rec.Code >= 400 {
				t.Errorf("%s as %q: status %d, want success; %s", tc.name, user, rec.Code, rec.Body)
			}
			if !allowed && rec.Code != 404 {
				t.Errorf("%s as %q: status %d, want 404", tc.name, user, rec.Code)
			}
			wrote := len(e.st.audits) != 0 || len(e.st.changes) != before ||
				(tc.method == "DELETE" && e.st.changes[0].State != StateScheduled)
			if !allowed && wrote {
				t.Errorf("%s as %q: a refused call wrote", tc.name, user)
			}
		}
	}
}

func TestOtherMethodsAre404(t *testing.T) {
	e := newEnv(t)
	for _, c := range [][2]string{{"PUT", changesPath}, {"DELETE", changesPath}, {"GET", scheduledPath}, {"POST", scheduledPath}} {
		if rec := e.do(adminUser, c[0], c[1], ""); rec.Code != 404 {
			t.Errorf("%s %s: status %d, want 404", c[0], c[1], rec.Code)
		}
	}
}

func TestWritesNeedTheConsoleOrigin(t *testing.T) {
	e := newEnv(t)
	tok, _ := platformauth.SignMFASession(secret, adminUser, t0.Add(-time.Minute))
	req := httptest.NewRequest("POST", changesPath, strings.NewReader(postBody))
	req.AddCookie(&http.Cookie{Name: session.CookieName, Value: tok})
	req.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	if rec.Code != 403 || len(e.st.changes) != 0 {
		t.Fatalf("status %d, %d changes; want 403 and none", rec.Code, len(e.st.changes))
	}
}

// A session without the mfa claim is otherwise valid: signed cookie, console
// origin, active admin. Only the second factor is missing.
func TestScheduleNeedsAnMFASession(t *testing.T) {
	post := func(e *env, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", changesPath, strings.NewReader(postBody))
		req.AddCookie(&http.Cookie{Name: session.CookieName, Value: token})
		req.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, req)
		return rec
	}
	authTime := t0.Add(-time.Minute)
	noMFA, err := session.Sign(secret, adminUser, authTime)
	if err != nil {
		t.Fatal(err)
	}
	withMFA, err := platformauth.SignMFASession(secret, adminUser, authTime)
	if err != nil {
		t.Fatal(err)
	}

	e := newEnv(t)
	if rec := post(e, noMFA); rec.Code != 404 || len(e.st.changes) != 0 || len(e.st.audits) != 0 {
		t.Fatalf("without mfa: status %d, %d changes, %d audits; want 404 and none", rec.Code, len(e.st.changes), len(e.st.audits))
	}
	if rec := post(e, withMFA); rec.Code != 201 || len(e.st.changes) != 1 {
		t.Fatalf("with mfa: status %d, %d changes; want 201 and one: %s", rec.Code, len(e.st.changes), rec.Body)
	}
}

// Every staff route that writes under the price-change patterns: a session
// without the mfa claim gets the 404 of an unknown route and changes nothing.
func TestPriceWriteRoutesNeedAnMFASession(t *testing.T) {
	authTime := t0.Add(-time.Minute)
	noMFA, err := session.Sign(secret, adminUser, authTime)
	if err != nil {
		t.Fatal(err)
	}
	withMFA, err := platformauth.SignMFASession(secret, adminUser, authTime)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, method, path, body string
		seed                     bool // schedule a change first so the route has something to act on
	}{
		{"schedule", "POST", changesPath, postBody, false},
		{"cancel", "DELETE", scheduledPath, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			if c.seed {
				if rec := e.do(adminUser, "POST", changesPath, postBody); rec.Code != 201 {
					t.Fatalf("seed: %d %s", rec.Code, rec.Body)
				}
			}
			send := func(token string) *httptest.ResponseRecorder {
				req := httptest.NewRequest(c.method, c.path, strings.NewReader(c.body))
				req.AddCookie(&http.Cookie{Name: session.CookieName, Value: token})
				req.Header.Set("Origin", origin)
				rec := httptest.NewRecorder()
				e.h.ServeHTTP(rec, req)
				return rec
			}
			changes, audits := len(e.st.changes), len(e.st.audits)
			e.st.mu.Lock()
			before := append([]Change(nil), e.st.changes...)
			e.st.mu.Unlock()
			if rec := send(noMFA); rec.Code != 404 || len(e.st.changes) != changes || len(e.st.audits) != audits {
				t.Fatalf("without mfa: status %d, %d changes (want %d), %d audits (want %d)", rec.Code, len(e.st.changes), changes, len(e.st.audits), audits)
			}
			e.st.mu.Lock()
			same := len(before) == len(e.st.changes)
			for i := range before {
				if same && before[i].State != e.st.changes[i].State {
					same = false
				}
			}
			e.st.mu.Unlock()
			if !same {
				t.Fatal("without mfa: a stored change was modified")
			}
			if rec := send(withMFA); rec.Code == 404 {
				t.Fatalf("with mfa: status 404, want the request to get past the MFA check: %s", rec.Body)
			}
		})
	}
}

func TestScheduleHistoryAndCancelOverHTTP(t *testing.T) {
	e := newEnv(t)
	rec := e.do(adminUser, "POST", changesPath, postBody)
	if rec.Code != 201 {
		t.Fatalf("schedule: %d %s", rec.Code, rec.Body)
	}
	if got := decode(t, rec)["price_change"].(map[string]any)["state"]; got != "scheduled" {
		t.Errorf("state = %v", got)
	}
	rec = e.do(billUser, "GET", changesPath, "")
	list := decode(t, rec)["price_changes"].([]any)
	if rec.Code != 200 || len(list) != 1 {
		t.Fatalf("history: %d %s", rec.Code, rec.Body)
	}
	if rec = e.do(adminUser, "DELETE", scheduledPath, ""); rec.Code != 200 {
		t.Fatalf("cancel: %d %s", rec.Code, rec.Body)
	}
	rec = e.do(adminUser, "DELETE", scheduledPath, "")
	if rec.Code != 404 || decode(t, rec)["code"] != "price_change_not_found" {
		t.Fatalf("second cancel: %d %s", rec.Code, rec.Body)
	}
}

func TestPendingErrorBody(t *testing.T) {
	e := newEnv(t)
	e.do(adminUser, "POST", changesPath, postBody)
	rec := e.do(adminUser, "POST", changesPath, postBody)
	if rec.Code != 409 || decode(t, rec)["code"] != "price_change_pending" {
		t.Fatalf("status %d body %s", rec.Code, rec.Body)
	}
}

func TestDryRunOverHTTPSavesNothing(t *testing.T) {
	e := newEnv(t)
	rec := e.do(adminUser, "POST", changesPath+"?dry_run=1", postBody)
	body := decode(t, rec)
	if rec.Code != 200 || body["affected_subscribers"] != 3.0 || body["notice_date"] != "2026-03-10" {
		t.Fatalf("status %d body %s", rec.Code, rec.Body)
	}
	if len(e.st.changes) != 0 || len(e.st.audits) != 0 {
		t.Fatal("a dry run wrote")
	}
}

func TestBadBodyIs400(t *testing.T) {
	e := newEnv(t)
	for _, body := range []string{`{`, `{"effective_from":"2026-04-10T12:00:00Z","reason":"r","new":{"seat_price_month":1},"extra":1}`, `{"effective_from":"yesterday","reason":"r","new":{}}`} {
		if rec := e.do(adminUser, "POST", changesPath, body); rec.Code != 400 {
			t.Errorf("body %s: status %d, want 400", body, rec.Code)
		}
	}
}

func TestPriceErrorsCarryTheirFields(t *testing.T) {
	e := newEnv(t)
	rec := e.do(adminUser, "POST", changesPath, `{"effective_from":"2026-04-10T12:00:00Z","reason":"r","new":{"seat_price_month":-1,"seat_price_year":1}}`)
	body := decode(t, rec)
	if rec.Code != 400 || body["code"] != "price_invalid" || body["field"] != "seat_price_month" {
		t.Fatalf("status %d body %s", rec.Code, rec.Body)
	}
}

// --- Test 2: a price edit in place is refused.

func TestRefusePriceFieldsRefusesEachPriceField(t *testing.T) {
	var reached bool
	h := RefusePriceFields(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	for _, f := range PriceFields {
		reached = false
		req := httptest.NewRequest("PUT", "/api/platform/plans/pro", strings.NewReader(`{"name":"Pro","`+f+`":1}`))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		body := decode(t, rec)
		if rec.Code != 400 || body["code"] != "price_field_via_schedule" || reached {
			t.Errorf("%s: status %d body %s reached %v", f, rec.Code, rec.Body, reached)
		}
		if fields, _ := body["fields"].([]any); len(fields) != 1 || fields[0] != f {
			t.Errorf("%s: fields = %v", f, body["fields"])
		}
	}
}

func TestRefusePriceFieldsNamesEveryOffendingField(t *testing.T) {
	h := RefusePriceFields(http.NotFoundHandler())
	req := httptest.NewRequest("PUT", "/api/platform/plans/pro", strings.NewReader(`{"seat_price_year":1,"usage_terms":{},"name":"x"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	fields, _ := decode(t, rec)["fields"].([]any)
	if len(fields) != 2 || fields[0] != "seat_price_year" || fields[1] != "usage_terms" {
		t.Fatalf("fields = %v, want [seat_price_year usage_terms]", fields)
	}
}

func TestRefusePriceFieldsPassesOtherBodiesAndMethodsOn(t *testing.T) {
	var got string
	h := RefusePriceFields(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = r.Method + " " + string(b)
	}))
	for _, c := range [][2]string{{"PUT", `{"name":"Pro","limits":{}}`}, {"DELETE", ""}, {"PUT", `not json`}} {
		got = ""
		req := httptest.NewRequest(c[0], "/api/platform/plans/pro", strings.NewReader(c[1]))
		h.ServeHTTP(httptest.NewRecorder(), req)
		if got != c[0]+" "+c[1] {
			t.Errorf("%s %q: next saw %q", c[0], c[1], got)
		}
	}
}

// Every route of the handler names an action of the matrix: system_admin
// schedules and cancels, billing and system_admin read the history.
func TestEveryRouteInEveryRouteTableHasAMatrixRow(t *testing.T) {
	known := map[string]bool{}
	for _, a := range staff.Actions() {
		known[a] = true
	}
	seen := 0
	for _, rt := range routes(nil) {
		for method, ep := range rt.methods {
			seen++
			if !known[ep.action] {
				t.Errorf("%s %s has action %q, which is not a row of the matrix", method, rt.pattern, ep.action)
			}
			if len(staff.RolesWith(ep.action)) == 0 {
				t.Errorf("%s %s lists no role", method, rt.pattern)
			}
		}
	}
	if seen != 3 {
		t.Errorf("route table has %d routes, want 3", seen)
	}
}

func TestRouteRolesFollowTheMatrix(t *testing.T) {
	roles := map[string][]staff.Role{}
	for _, rt := range routes(nil) {
		for method, ep := range rt.methods {
			roles[method+" "+rt.pattern] = staff.RolesWith(ep.action)
		}
	}
	for route, want := range map[string][]staff.Role{
		"GET " + Patterns[0]:    {staff.RoleSystemAdmin, staff.RoleBilling},
		"POST " + Patterns[0]:   {staff.RoleSystemAdmin},
		"DELETE " + Patterns[1]: {staff.RoleSystemAdmin},
	} {
		if fmt.Sprint(roles[route]) != fmt.Sprint(want) {
			t.Errorf("%s roles = %v, want %v", route, roles[route], want)
		}
	}
}
