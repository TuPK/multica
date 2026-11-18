package einvoice

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/invoice"
)

const (
	testKey       = "k1:s3cret"
	testAccountID = "00000000-0000-0000-0000-000000000001"
)

type reply struct {
	status int
	body   string
}

func ok(body string) reply { return reply{http.StatusOK, body} }

type call struct {
	Method, Path, Auth, Body string
}

// fakeSePay answers each "METHOD path" from a queue; the last reply repeats.
type fakeSePay struct {
	srv    *httptest.Server
	mu     sync.Mutex
	calls  []call
	routes map[string][]reply
}

func newFakeSePay(t *testing.T) *fakeSePay {
	t.Helper()
	f := &fakeSePay{routes: map[string][]reply{}}
	f.route("POST /v1/token", ok(fixture(t, "token_response.json")))
	f.route("GET /v1/provider-accounts/"+testAccountID, ok(fixture(t, "provider_account_detail.json")))
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		key := r.Method + " " + r.URL.Path
		f.mu.Lock()
		f.calls = append(f.calls, call{r.Method, r.URL.Path, r.Header.Get("Authorization"), string(b)})
		q := f.routes[key]
		var rep reply
		switch {
		case len(q) == 0:
			rep = reply{http.StatusNotFound, `{"success":false,"error":{"code":"NOT_FOUND","message":"x"}}`}
		case len(q) == 1:
			rep = q[0]
		default:
			rep, f.routes[key] = q[0], q[1:]
		}
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(rep.status)
		_, _ = io.WriteString(w, rep.body)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeSePay) route(key string, replies ...reply) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.routes[key] = replies
}

func (f *fakeSePay) all() []call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]call(nil), f.calls...)
}

// api lists "METHOD path" of the calls other than the token fetch.
func (f *fakeSePay) api() []string {
	var out []string
	for _, c := range f.all() {
		if c.Path != "/v1/token" {
			out = append(out, c.Method+" "+c.Path)
		}
	}
	return out
}

func (f *fakeSePay) count(key string) int {
	n := 0
	for _, c := range f.all() {
		if c.Method+" "+c.Path == key {
			n++
		}
	}
	return n
}

func (f *fakeSePay) client(t *testing.T) *SePay {
	t.Helper()
	p, err := New(Config{
		BaseURL: f.srv.URL, APIKey: testKey, ProviderAccountID: testAccountID, TaxRate: 10,
		HTTPClient: f.srv.Client(), MaxCreatePolls: 5, PollInterval: time.Nanosecond,
		Now: func() time.Time { return time.Date(2026, 1, 26, 3, 4, 5, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func request() invoice.IssueRequest {
	return invoice.IssueRequest{
		ReferenceCode: "ORD-1-1", Currency: "vnd", Amount: 100000, VATAmount: 10000,
		Buyer: invoice.Buyer{Name: "Cong ty A", TaxCode: "0312345678", Address: "1 Pho A", Email: "a@example.com"},
	}
}

const trackingBody = `{"success":true,"data":{"tracking_code":"trk-1"}}`

func providerError(t *testing.T, err error) *invoice.ProviderError {
	t.Helper()
	var pe *invoice.ProviderError
	if !errors.As(err, &pe) {
		t.Fatalf("error %v is not a ProviderError", err)
	}
	return pe
}

func TestIssueSuccessPollsPendingThenIssued(t *testing.T) {
	f := newFakeSePay(t)
	pending := ok(fixture(t, "create_check_pending.json"))
	f.route("POST /v1/invoices/create", ok(trackingBody))
	f.route("GET /v1/invoices/create/check/trk-1", pending, pending, ok(fixture(t, "create_check_success.json")))
	f.route("POST /v1/invoices/issue", ok(`{"success":true,"data":{"tracking_code":"trk-2"}}`))
	issuePending := ok(fixture(t, "issue_check_pending.json"))
	f.route("GET /v1/invoices/issue/check/trk-2", issuePending, issuePending, ok(fixture(t, "issue_check_success.json")))
	p := f.client(t)

	tk, err := p.Issue(context.Background(), request())
	if err != nil {
		t.Fatal(err)
	}
	if tk.ReferenceCode != "ORD-1-1" || tk.TrackingCode != "trk-2" {
		t.Errorf("ticket = %+v", tk)
	}
	want := []string{
		"GET /v1/provider-accounts/" + testAccountID,
		"POST /v1/invoices/create",
		"GET /v1/invoices/create/check/trk-1",
		"GET /v1/invoices/create/check/trk-1",
		"GET /v1/invoices/create/check/trk-1",
		"POST /v1/invoices/issue",
	}
	if got := f.api(); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("calls = %v, want %v", got, want)
	}

	var body struct {
		TemplateCode string `json:"template_code"`
		Series       string `json:"invoice_series"`
		IssuedDate   string `json:"issued_date"`
		Currency     string `json:"currency"`
		AccountID    string `json:"provider_account_id"`
		Reference    string `json:"reference_code"`
		IsDraft      bool   `json:"is_draft"`
		Total        int64  `json:"total_amount"`
		Buyer        map[string]string
		Items        []map[string]any `json:"items"`
	}
	for _, c := range f.all() {
		if c.Path == "/v1/invoices/create" {
			if err := json.Unmarshal([]byte(c.Body), &body); err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(c.Auth, "Bearer ") {
				t.Errorf("create is not sent with a bearer token")
			}
		}
	}
	// 03:04:05 UTC is 10:04:05 in Ho Chi Minh City.
	if body.TemplateCode != "1" || body.Series != "C26TSE" || body.IssuedDate != "2026-01-26 10:04:05" ||
		body.Currency != "VND" || body.AccountID != testAccountID || body.Reference != "ORD-1-1" ||
		!body.IsDraft || body.Total != 110000 {
		t.Errorf("create body = %+v", body)
	}
	if len(body.Items) != 1 || body.Items[0]["unit_price"] != float64(100000) || body.Items[0]["tax_amount"] != float64(10000) || body.Items[0]["tax_rate"] != float64(10) {
		t.Errorf("items = %v", body.Items)
	}

	var raw map[string]json.RawMessage
	for _, c := range f.all() {
		if c.Path == "/v1/invoices/create" {
			_ = json.Unmarshal([]byte(c.Body), &raw)
		}
	}
	var buyer map[string]string
	_ = json.Unmarshal(raw["buyer"], &buyer)
	if buyer["type"] != "company" || buyer["legal_name"] != "Cong ty A" || buyer["tax_code"] != "0312345678" {
		t.Errorf("buyer = %v", buyer)
	}

	for i, wantState := range []invoice.PollState{invoice.PollPending, invoice.PollPending, invoice.PollSuccess} {
		poll, err := p.Status(context.Background(), "trk-2")
		if err != nil {
			t.Fatal(err)
		}
		if poll.State != wantState {
			t.Fatalf("poll %d state = %s, want %s", i, poll.State, wantState)
		}
		if wantState == invoice.PollSuccess && (poll.Invoice.Number != "589" || !poll.Invoice.Issued || poll.Invoice.ProviderInvoiceID != "REF-EXAMPLE-1" ||
			poll.Invoice.LookupURL == "" || poll.Invoice.IssuedAt.IsZero()) {
			t.Errorf("invoice = %+v", poll.Invoice)
		}
	}
}

func TestRateLimitIsTransientWithSpecSchedule(t *testing.T) {
	f := newFakeSePay(t)
	f.route("POST /v1/invoices/create", reply{http.StatusTooManyRequests, `{"success":false,"error":{"code":"RATE_LIMITED","message":"slow"}}`})
	_, err := f.client(t).Issue(context.Background(), request())
	if pe := providerError(t, err); pe.Kind != invoice.KindTransient {
		t.Fatalf("kind = %s, want transient", pe.Kind)
	}
	if d := invoice.Decide(err, 1, 8); d.State != invoice.StatePending || d.Delay != time.Minute {
		t.Errorf("attempt 1 = %+v", d)
	}
	if d := invoice.Decide(err, 2, 8); d.Delay != 2*time.Minute {
		t.Errorf("attempt 2 delay = %v", d.Delay)
	}
}

func TestClientErrorIsPermanentFailsImmediately(t *testing.T) {
	f := newFakeSePay(t)
	f.route("POST /v1/invoices/create", reply{http.StatusBadRequest, `{"success":false,"error":{"code":"VALIDATION_ERROR","message":"bad"}}`})
	_, err := f.client(t).Issue(context.Background(), request())
	pe := providerError(t, err)
	if pe.Kind != invoice.KindPermanent || pe.Code != "VALIDATION_ERROR" {
		t.Fatalf("error = %+v", pe)
	}
	if d := invoice.Decide(err, 1, 8); d.State != invoice.StateFailed {
		t.Errorf("state = %s, want failed", d.State)
	}
}

func TestMissingBuyerFieldNeedsInfoNoHTTP(t *testing.T) {
	f := newFakeSePay(t)
	p := f.client(t)
	req := request()
	req.Buyer.TaxCode = ""
	if got := invoice.CheckBuyer(p, req.Buyer); len(got) != 1 || got[0] != "tax_code" {
		t.Fatalf("CheckBuyer = %v, want [tax_code]", got)
	}
	_, err := p.Issue(context.Background(), req)
	if pe := providerError(t, err); pe.Kind != invoice.KindMissingFields {
		t.Fatalf("error = %v, want missing fields", err)
	}
	if d := invoice.Decide(err, 1, 8); d.State != invoice.StateNeedsInfo {
		t.Errorf("state = %s, want needs_info", d.State)
	}
	if n := len(f.all()); n != 0 {
		t.Errorf("server saw %d requests, want 0", n)
	}
}

func TestLookupReturnsExistingNoReissue(t *testing.T) {
	f := newFakeSePay(t)
	f.route("GET /v1/invoices/ORD-1-1", ok(fixture(t, "invoice_detail.json")))
	sub, err := invoice.Submit(context.Background(), f.client(t), request(), true)
	if err != nil {
		t.Fatal(err)
	}
	if !sub.Existing || !sub.Invoice.Issued || sub.Invoice.Number != "589" {
		t.Errorf("submission = %+v", sub)
	}
	if f.count("POST /v1/invoices/create")+f.count("POST /v1/invoices/issue") != 0 {
		t.Errorf("a create or issue was sent: %v", f.api())
	}
}

func TestLookupDraftIsNotIssued(t *testing.T) {
	f := newFakeSePay(t)
	f.route("GET /v1/invoices/ORD-1-1", ok(strings.Replace(fixture(t, "invoice_detail.json"), `"status": "issued"`, `"status": "draft"`, 1)))
	if _, err := f.client(t).Lookup(context.Background(), "ORD-1-1"); !errors.Is(err, invoice.ErrNoInvoice) {
		t.Errorf("err = %v, want ErrNoInvoice", err)
	}
	f.route("GET /v1/invoices/ORD-1-1", reply{http.StatusNotFound, `{"success":false,"error":{"code":"NOT_FOUND","message":"x"}}`})
	if _, err := f.client(t).Lookup(context.Background(), "ORD-1-1"); !errors.Is(err, invoice.ErrNoInvoice) {
		t.Errorf("404 err = %v, want ErrNoInvoice", err)
	}
}

func TestDraftExistsSkipsCreate(t *testing.T) {
	f := newFakeSePay(t)
	f.route("POST /v1/invoices/create", reply{http.StatusConflict, `{"success":false,"error":{"code":"EINVOICE_DOCUMENT_EXISTED","message":"x"}}`})
	f.route("GET /v1/invoices/ORD-1-1", ok(strings.Replace(fixture(t, "invoice_detail.json"), `"status": "issued"`, `"status": "draft"`, 1)))
	f.route("POST /v1/invoices/issue", ok(trackingBody))
	tk, err := f.client(t).Issue(context.Background(), request())
	if err != nil {
		t.Fatal(err)
	}
	if tk.TrackingCode != "trk-1" {
		t.Errorf("ticket = %+v", tk)
	}
	if n := f.count("POST /v1/invoices/create"); n != 1 {
		t.Errorf("create sent %d times, want 1", n)
	}
	if f.count("POST /v1/invoices/issue") != 1 || f.count("GET /v1/invoices/create/check/trk-1") != 0 {
		t.Errorf("calls = %v", f.api())
	}

	// An issued invoice under the reference is a permanent duplicate.
	f.route("GET /v1/invoices/ORD-1-1", ok(fixture(t, "invoice_detail.json")))
	_, err = f.client(t).Issue(context.Background(), request())
	if pe := providerError(t, err); pe.Kind != invoice.KindPermanent || pe.Code != "EINVOICE_DOCUMENT_EXISTED" {
		t.Errorf("error = %v", err)
	}
}

func TestCreateCheckOutcomes(t *testing.T) {
	f := newFakeSePay(t)
	f.route("POST /v1/invoices/create", ok(trackingBody))
	f.route("GET /v1/invoices/create/check/trk-1", ok(fixture(t, "create_check_failed.json")))
	_, err := f.client(t).Issue(context.Background(), request())
	if pe := providerError(t, err); pe.Kind != invoice.KindPermanent || pe.Code != "create_failed" {
		t.Errorf("failed: %v", err)
	}

	f.route("GET /v1/invoices/create/check/trk-1", ok(fixture(t, "create_check_pending.json")))
	_, err = f.client(t).Issue(context.Background(), request())
	if pe := providerError(t, err); pe.Kind != invoice.KindTransient || pe.Code != "create_pending" {
		t.Errorf("pending: %v", err)
	}
	if f.count("POST /v1/invoices/issue") != 0 {
		t.Errorf("issue sent without a created draft")
	}
}

func TestTokenRefreshedOnTokenExpired(t *testing.T) {
	f := newFakeSePay(t)
	f.route("GET /v1/invoices/issue/check/trk-2",
		reply{http.StatusUnauthorized, `{"success":false,"error":{"code":"TOKEN_EXPIRED","message":"x"}}`},
		ok(fixture(t, "issue_check_success.json")))
	poll, err := f.client(t).Status(context.Background(), "trk-2")
	if err != nil || poll.State != invoice.PollSuccess {
		t.Fatalf("poll = %+v, %v", poll, err)
	}
	if n := f.count("POST /v1/token"); n != 2 {
		t.Errorf("token fetched %d times, want 2", n)
	}
	if n := f.count("GET /v1/invoices/issue/check/trk-2"); n != 2 {
		t.Errorf("status sent %d times, want 2", n)
	}
	for _, c := range f.all() {
		if c.Path == "/v1/token" && !strings.HasPrefix(c.Auth, "Basic ") {
			t.Errorf("token request auth is not Basic")
		}
	}
}

func TestTokenIsCached(t *testing.T) {
	f := newFakeSePay(t)
	f.route("GET /v1/invoices/issue/check/trk-2", ok(fixture(t, "issue_check_pending.json")))
	p := f.client(t)
	for range 3 {
		if _, err := p.Status(context.Background(), "trk-2"); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.count("POST /v1/token"); n != 1 {
		t.Errorf("token fetched %d times, want 1", n)
	}
}

func TestNewRejectsBadConfig(t *testing.T) {
	good := Config{BaseURL: "https://einvoice-api-sandbox.sepay.vn", APIKey: testKey, ProviderAccountID: testAccountID, TaxRate: 10}
	if _, err := New(good); err != nil {
		t.Fatalf("good config: %v", err)
	}
	cases := map[string]func(*Config){
		"empty base url":   func(c *Config) { c.BaseURL = "" },
		"empty key":        func(c *Config) { c.APIKey = "" },
		"key no colon":     func(c *Config) { c.APIKey = "k1s3cret" },
		"empty client id":  func(c *Config) { c.APIKey = ":s3cret" },
		"empty secret":     func(c *Config) { c.APIKey = "k1:" },
		"empty account id": func(c *Config) { c.ProviderAccountID = "" },
		"tax rate 7":       func(c *Config) { c.TaxRate = 7 },
		"tax rate -1":      func(c *Config) { c.TaxRate = -1 },
	}
	for name, mut := range cases {
		c := good
		mut(&c)
		_, err := New(c)
		if err == nil {
			t.Errorf("%s: no error", name)
			continue
		}
		for _, secret := range []string{"s3cret", "k1"} {
			if c.APIKey != "" && strings.Contains(err.Error(), secret) {
				t.Errorf("%s: error leaks the key: %v", name, err)
			}
		}
	}
	for _, rate := range []int64{0, 5, 8, 10} {
		c := good
		c.TaxRate = rate
		if _, err := New(c); err != nil {
			t.Errorf("tax rate %d: %v", rate, err)
		}
	}
}

func TestErrorsNeverContainSecretsOrBuyerValues(t *testing.T) {
	leak := `{"success":false,"error":{"code":"VALIDATION_ERROR","message":"SECRET-NAME k1:s3cret"}}`
	for _, status := range []int{http.StatusBadRequest, http.StatusInternalServerError, http.StatusOK} {
		f := newFakeSePay(t)
		body := leak
		if status == http.StatusOK {
			body = `not json SECRET-NAME s3cret`
		}
		f.route("POST /v1/invoices/create", reply{status, body})
		req := request()
		req.Buyer.Name = "SECRET-NAME"
		_, err := f.client(t).Issue(context.Background(), req)
		if err == nil {
			t.Fatalf("status %d: no error", status)
		}
		for _, bad := range []string{"SECRET-NAME", "s3cret", "k1", "Bearer", "Basic"} {
			if strings.Contains(err.Error(), bad) {
				t.Errorf("status %d: error %q contains %q", status, err, bad)
			}
		}
	}

	// A failing token fetch must not leak the key either.
	f := newFakeSePay(t)
	f.route("POST /v1/token", reply{http.StatusUnauthorized, `{"success":false,"error":{"code":"UNAUTHORIZED","message":"k1:s3cret"}}`})
	_, err := f.client(t).Status(context.Background(), "trk")
	if err == nil || strings.Contains(err.Error(), "s3cret") || strings.Contains(err.Error(), "k1") {
		t.Errorf("token error = %v", err)
	}
}

func TestTransportErrorIsTransient(t *testing.T) {
	f := newFakeSePay(t)
	p := f.client(t)
	f.srv.Close()
	_, err := p.Status(context.Background(), "trk")
	if pe := providerError(t, err); pe.Kind != invoice.KindTransient || pe.Code != "network" {
		t.Errorf("error = %v", err)
	}
}
