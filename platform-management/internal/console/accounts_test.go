package console

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/accountrequest"
	"github.com/multica-ai/multica/platform-management/internal/platformauth"
	"github.com/multica-ai/multica/platform-management/internal/session"
	"github.com/multica-ai/multica/platform-management/internal/staff"
)

const (
	reqID  = "dddddddd-0000-0000-0000-000000000001"
	reqNew = "dddddddd-0000-0000-0000-000000000002"
)

const createBody = `{"kind":"individual","email":"Ana@Example.com","display_name":"Ana","workspace_name":"Ana's space"}`

// fakeAccounts is the AccountRequests dependency, recording what the handler
// passes on.
type fakeAccounts struct {
	calls   int
	actors  []accountrequest.Actor
	inputs  []accountrequest.Input
	listed  []accountrequest.ListQuery
	rows    map[string]accountrequest.Request
	created bool
	err     error
}

func (f *fakeAccounts) Create(_ context.Context, a accountrequest.Actor, in accountrequest.Input) (accountrequest.Request, bool, error) {
	f.calls++
	f.actors = append(f.actors, a)
	f.inputs = append(f.inputs, in)
	if f.err != nil {
		return accountrequest.Request{}, false, f.err
	}
	r := f.rows[reqNew]
	return r, f.created, nil
}

func (f *fakeAccounts) Cancel(_ context.Context, a accountrequest.Actor, id string) (accountrequest.Request, error) {
	f.calls++
	f.actors = append(f.actors, a)
	if f.err != nil {
		return accountrequest.Request{}, f.err
	}
	r, ok := f.rows[id]
	if !ok {
		return accountrequest.Request{}, accountrequest.ErrNotFound
	}
	r.State = accountrequest.StateCanceled
	return r, nil
}

func (f *fakeAccounts) List(_ context.Context, q accountrequest.ListQuery) ([]accountrequest.Request, error) {
	f.calls++
	f.listed = append(f.listed, q)
	return []accountrequest.Request{f.rows[reqID]}, nil
}

func (f *fakeAccounts) Get(_ context.Context, id string) (accountrequest.Request, bool, error) {
	f.calls++
	r, ok := f.rows[id]
	return r, ok, nil
}

type accountsEnv struct {
	t   *testing.T
	h   http.Handler
	acc *fakeAccounts
	dir *fakeDirectory
}

func newAccountsEnv(t *testing.T) *accountsEnv {
	t.Helper()
	e := &accountsEnv{
		t: t,
		dir: &fakeDirectory{
			roles:  map[string]staff.Role{adminID: staff.RoleAdmin, suppID: staff.RoleSupport, billID: staff.RoleBilling},
			emails: map[string]string{adminID: adminMail, suppID: suppMail, billID: billMail, custID: custMail},
		},
		acc: &fakeAccounts{
			rows: map[string]accountrequest.Request{
				reqID: {
					ID: reqID, Kind: "individual", Email: "old@example.com", DisplayName: "Old", WorkspaceName: "Old space",
					PlanKey: "free", Seats: 1, Interval: "month", State: accountrequest.StatePending, CreatedBy: adminID,
					CreatedAt: t0, UpdatedAt: t0,
					BillingProfile: &accountrequest.BillingProfile{LegalName: "Hidden Co", TaxCode: "0123456789", Address: "A", InvoiceEmail: "i@x.co"},
				},
				reqNew: {
					ID: reqNew, Kind: "individual", Email: "ana@example.com", DisplayName: "Ana", WorkspaceName: "Ana's space",
					PlanKey: "free", Seats: 1, Interval: "month", State: accountrequest.StatePending, CreatedBy: adminID,
					CreatedAt: t0, UpdatedAt: t0,
				},
			},
			created: true,
		},
	}
	auth := platformauth.New(platformauth.Deps{
		Config:    platformauth.Config{SessionSecret: secret, ConsoleOrigin: origin},
		Directory: e.dir,
		Guard:     activeGuard{},
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:       func() time.Time { return t0 },
	})
	e.h = New(Deps{
		RequireStaff:  auth.RequirePlatformStaff,
		RequireOrigin: auth.RequireOrigin,
		Accounts:      e.acc,
		Emails:        e.dir,
		Now:           func() time.Time { return t0 },
	})
	return e
}

func (e *accountsEnv) do(user, method, path, body string) *httptest.ResponseRecorder {
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

// Spec 7b.5 rule 1 and the API table: only admin reaches the four routes;
// support, billing, a customer and a caller with no session get the 404 of an
// unknown route, and the service is never called.
func TestAccountRequestRoutesAreAdminOnly(t *testing.T) {
	routes := []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/api/platform/accounts", "", 200},
		{"GET", "/api/platform/accounts/" + reqID, "", 200},
		{"POST", "/api/platform/accounts", createBody, 202},
		{"POST", "/api/platform/accounts/" + reqID + "/cancel", "", 200},
	}
	for _, rt := range routes {
		e := newAccountsEnv(t)
		if rec := e.do(adminID, rt.method, rt.path, rt.body); rec.Code != rt.status {
			t.Errorf("admin %s %s = %d, want %d; %s", rt.method, rt.path, rec.Code, rt.status, rec.Body.String())
		}
		for name, user := range map[string]string{"support": suppID, "billing": billID, "customer": custID, "no session": ""} {
			e := newAccountsEnv(t)
			rec := e.do(user, rt.method, rt.path, rt.body)
			if rec.Code != http.StatusNotFound || rec.Body.String() != "404 page not found\n" {
				t.Errorf("%s %s %s = %d %q, want the 404 of an unknown route", name, rt.method, rt.path, rec.Code, rec.Body.String())
			}
			if e.acc.calls != 0 {
				t.Errorf("%s %s %s reached the service", name, rt.method, rt.path)
			}
		}
	}
}

func TestAccountRequestWrongMethodIsAnUnknownRoute(t *testing.T) {
	e := newAccountsEnv(t)
	for _, c := range [][2]string{
		{"DELETE", "/api/platform/accounts"}, {"PUT", "/api/platform/accounts/" + reqID},
		{"GET", "/api/platform/accounts/" + reqID + "/cancel"}, {"POST", "/api/platform/accounts/" + reqID},
	} {
		wantUnknownRoute(t, e.do(adminID, c[0], c[1], ""))
	}
}

func TestAccountCreateAnswers202WithTheRequest(t *testing.T) {
	e := newAccountsEnv(t)
	rec := e.do(adminID, "POST", "/api/platform/accounts", createBody)
	wantStatus(t, rec, 202)
	body := decode(t, rec)
	if body["id"] != reqNew || body["state"] != "pending" || body["email"] != "ana@example.com" {
		t.Errorf("body = %v", body)
	}
	if len(e.acc.actors) != 1 || e.acc.actors[0].ID != adminID || e.acc.actors[0].Label != adminMail {
		t.Errorf("actor = %+v, want the signed-in admin and their email", e.acc.actors)
	}
	if in := e.acc.inputs[0]; in.Kind != "individual" || in.Email != "Ana@Example.com" || in.DisplayName != "Ana" || in.WorkspaceName != "Ana's space" {
		t.Errorf("input = %+v", in)
	}
}

// Rule 5: a live request for the email comes back with 200.
func TestAccountCreateOfALiveEmailAnswers200(t *testing.T) {
	e := newAccountsEnv(t)
	e.acc.created = false
	rec := e.do(adminID, "POST", "/api/platform/accounts", createBody)
	wantStatus(t, rec, 200)
	if decode(t, rec)["id"] != reqNew {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestAccountCreateDecodesTheWholeInput(t *testing.T) {
	e := newAccountsEnv(t)
	body := `{"kind":"business","email":"b@x.co","display_name":"B","workspace_name":"W","workspace_slug":"w-1","plan_key":"business",
		"seats":7,"interval":"year","trial":true,"comp_reason":"","billing_profile":{"legal_name":"L","tax_code":"0123456789","address":"A","invoice_email":"i@x.co"}}`
	wantStatus(t, e.do(adminID, "POST", "/api/platform/accounts", body), 202)
	in := e.acc.inputs[0]
	if in.WorkspaceSlug != "w-1" || in.PlanKey != "business" || in.Seats != 7 || in.Interval != "year" || !in.Trial ||
		in.BillingProfile == nil || in.BillingProfile.LegalName != "L" || in.BillingProfile.InvoiceEmail != "i@x.co" {
		t.Errorf("input = %+v profile = %+v", in, in.BillingProfile)
	}
}

func TestAccountCreateRefusesABodyThatIsNotAnInput(t *testing.T) {
	e := newAccountsEnv(t)
	for _, body := range []string{`{"kind":"individual","surprise":1}`, `[]`, `not json`, `{} {}`} {
		wantCode(t, e.do(adminID, "POST", "/api/platform/accounts", body), 400, "invalid_body")
	}
	if e.acc.calls != 0 {
		t.Error("a malformed body reached the service")
	}
}

func TestAccountCreateMapsServiceErrorsToTheSpecCodes(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
		extra  func(*testing.T, map[string]any, http.Header)
	}{
		{"invalid with fields", &accountrequest.Error{Status: 400, Code: "account_request_invalid", Message: "bad", Fields: []string{"email", "kind"}}, 400, "account_request_invalid",
			func(t *testing.T, b map[string]any, _ http.Header) {
				f, _ := b["fields"].([]any)
				if len(f) != 2 || f[0] != "email" || f[1] != "kind" {
					t.Errorf("fields = %v", b["fields"])
				}
			}},
		{"activation", &accountrequest.Error{Status: 400, Code: "account_activation_required", Message: "m"}, 400, "account_activation_required", nil},
		{"email exists", &accountrequest.Error{Status: 409, Code: "account_email_exists", Message: "m"}, 409, "account_email_exists", nil},
		{"audience", &accountrequest.Error{Status: 409, Code: "plan_audience_mismatch", Message: "m"}, 409, "plan_audience_mismatch", nil},
		{"rate limited", &accountrequest.Error{Status: 429, Code: "account_create_rate_limited", Message: "m", RetryAfter: 90 * time.Second}, 429, "account_create_rate_limited",
			func(t *testing.T, _ map[string]any, h http.Header) {
				if got := h.Get("Retry-After"); got != "90" {
					t.Errorf("Retry-After = %q, want 90", got)
				}
			}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newAccountsEnv(t)
			e.acc.err = c.err
			rec := e.do(adminID, "POST", "/api/platform/accounts", createBody)
			wantCode(t, rec, c.status, c.code)
			if c.extra != nil {
				c.extra(t, decode(t, rec), rec.Header())
			}
		})
	}
}

func TestAccountCreateHidesAnUnexpectedErrorBehindA500(t *testing.T) {
	e := newAccountsEnv(t)
	e.acc.err = errors.New("pq: connection to 10.0.0.5 refused")
	rec := e.do(adminID, "POST", "/api/platform/accounts", createBody)
	wantCode(t, rec, 500, "internal")
	if strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Error("the response repeats the cause")
	}
}

func TestAccountRequestListAndGet(t *testing.T) {
	e := newAccountsEnv(t)
	rec := e.do(adminID, "GET", "/api/platform/accounts?state=pending&limit=10", "")
	wantStatus(t, rec, 200)
	list, _ := decode(t, rec)["requests"].([]any)
	if len(list) != 1 {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if q := e.acc.listed[0]; q.State != "pending" || q.Limit != 10 {
		t.Errorf("query = %+v", q)
	}
	for _, body := range []string{rec.Body.String(), e.do(adminID, "GET", "/api/platform/accounts/"+reqID, "").Body.String()} {
		if strings.Contains(body, "Hidden Co") || strings.Contains(body, "0123456789") {
			t.Errorf("the billing profile of a third party is in the response: %s", body)
		}
	}
	one := decode(t, e.do(adminID, "GET", "/api/platform/accounts/"+reqID, ""))
	for _, key := range []string{"id", "kind", "email", "display_name", "workspace_name", "plan_key", "seats", "state", "error_code", "created_by", "created_at"} {
		if _, ok := one[key]; !ok {
			t.Errorf("response lacks %s: %v", key, one)
		}
	}
	for _, key := range []string{"user_id", "workspace_id"} {
		if _, ok := one[key]; ok {
			t.Errorf("response has %s before the request has one", key)
		}
	}
}

func TestAccountRequestListRefusesBadFilters(t *testing.T) {
	e := newAccountsEnv(t)
	for _, q := range []string{"?state=bogus", "?limit=0", "?limit=abc", "?limit=1000"} {
		wantCode(t, e.do(adminID, "GET", "/api/platform/accounts"+q, ""), 400, "invalid_filter")
	}
}

func TestAccountRequestGetUnknownIs404(t *testing.T) {
	e := newAccountsEnv(t)
	wantCode(t, e.do(adminID, "GET", "/api/platform/accounts/eeeeeeee-0000-0000-0000-000000000009", ""), 404, "not_found")
	wantCode(t, e.do(adminID, "GET", "/api/platform/accounts/not-a-uuid", ""), 404, "not_found")
}

func TestAccountRequestCancel(t *testing.T) {
	e := newAccountsEnv(t)
	rec := e.do(adminID, "POST", "/api/platform/accounts/"+reqID+"/cancel", "")
	wantStatus(t, rec, 200)
	if decode(t, rec)["state"] != "canceled" {
		t.Errorf("body = %s", rec.Body.String())
	}
	if e.acc.actors[0].ID != adminID || e.acc.actors[0].Label != adminMail {
		t.Errorf("actor = %+v", e.acc.actors[0])
	}
	wantCode(t, e.do(adminID, "POST", "/api/platform/accounts/eeeeeeee-0000-0000-0000-000000000009/cancel", ""), 404, "not_found")

	e.acc.err = &accountrequest.Error{Status: 409, Code: "account_request_not_cancelable", Message: "m", State: "provisioned"}
	rec = e.do(adminID, "POST", "/api/platform/accounts/"+reqID+"/cancel", "")
	wantCode(t, rec, 409, "account_request_not_cancelable")
	if decode(t, rec)["state"] != "provisioned" {
		t.Errorf("body = %s", rec.Body.String())
	}
}

// A write without the console's Origin is refused before the service.
func TestAccountRequestWritesNeedTheConsoleOrigin(t *testing.T) {
	e := newAccountsEnv(t)
	req := httptest.NewRequest("POST", "/api/platform/accounts", strings.NewReader(createBody))
	tok, _ := platformauth.SignMFASession(secret, adminID, t0.Add(-time.Minute))
	req.AddCookie(&http.Cookie{Name: session.CookieName, Value: tok})
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	if rec.Code < 400 || e.acc.calls != 0 {
		t.Errorf("status %d, service calls %d", rec.Code, e.acc.calls)
	}
}
