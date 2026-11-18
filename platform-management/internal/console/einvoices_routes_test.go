package console

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/invoice"
	"github.com/multica-ai/multica/platform-management/internal/staff"
)

// Route: POST /einvoices/{orderId}/retry (billing, admin). Role and not-found
// tests run on a fake; state, audit and job tests run on the platform test database
// with the real invoice store and the fake provider.

const (
	fullBuyer      = `{"name":"Cong ty A","tax_code":"0312345678","address":"1 Pho A","email":"a@example.com"}`
	noTaxCodeBuyer = `{"name":"Cong ty A","tax_code":"","address":"1 Pho A","email":"a@example.com"}`
)

// fakeEinvoiceOps records the calls the gate and body tests must not reach.
type fakeEinvoiceOps struct {
	latest  invoice.Record
	err     error
	retries int
}

func (f *fakeEinvoiceOps) Latest(context.Context, string) (invoice.Record, error) {
	return f.latest, f.err
}

func (f *fakeEinvoiceOps) Retry(context.Context, string, int, string) (invoice.Record, error) {
	f.retries++
	return f.latest, f.err
}

func withEinvoiceOps(e *env, rec invoice.Record, err error) *fakeEinvoiceOps {
	ops := &fakeEinvoiceOps{latest: rec, err: err}
	e.h.(*Handler).Einvoices = ops
	return ops
}

func TestEinvoiceRetryRequiresBillingOrAdmin(t *testing.T) {
	e := newEnv(t)
	ops := withEinvoiceOps(e, invoice.Record{OrderID: orderA, Revision: 1, State: invoice.StateFailed}, nil)
	rec := e.do(suppID, "POST", "/api/platform/einvoices/"+orderA+"/retry", "")
	// The staff middleware refuses a role the route does not admit with the 404
	// of an unknown route, as on every other console route.
	if rec.Code != http.StatusNotFound {
		t.Fatalf("support got %d %s, want the 404 of an unknown route", rec.Code, rec.Body)
	}
	if ops.retries != 0 {
		t.Errorf("support reached the retry")
	}
	for _, id := range []string{billID, adminID} {
		if rec := e.do(id, "POST", "/api/platform/einvoices/"+orderA+"/retry", ""); rec.Code != http.StatusOK {
			t.Errorf("%s got %d %s, want 200", id, rec.Code, rec.Body)
		}
	}
}

// --- database-backed tests ---

type einvoiceDB struct {
	*dbFixture
	store *invoice.PgxStore
	fake  *invoice.Fake
	svc   *invoice.Service
	// orders are removed with their audit rows at the end of the test.
	orders []string
}

func newEinvoiceDB(t *testing.T) *einvoiceDB {
	t.Helper()
	f := newDBFixture(t)
	x := &einvoiceDB{dbFixture: f, store: invoice.NewPgxStore(f.pool), fake: invoice.NewFake()}
	x.svc = invoice.NewService(x.store, x.fake, invoice.Config{StartAt: time.Now().Add(-24 * time.Hour), MaxAttempts: 3})
	f.h.(*Handler).Einvoices = x.svc
	t.Cleanup(func() {
		ctx := context.Background()
		for _, id := range x.orders {
			f.pool.Exec(ctx, `DELETE FROM einvoice_issuance WHERE order_id = $1`, id)
			f.pool.Exec(ctx, `DELETE FROM audit_event WHERE target_type = 'einvoice' AND target_id = $1`, id)
		}
	})
	return x
}

// seed inserts revision 1 of a new order in the given state with the buyer.
func (x *einvoiceDB) seed(state invoice.State, buyer string) string {
	x.t.Helper()
	ctx := context.Background()
	var order, ws string
	if err := x.pool.QueryRow(ctx, `SELECT gen_random_uuid()::text, gen_random_uuid()::text`).Scan(&order, &ws); err != nil {
		x.t.Fatal(err)
	}
	x.orders = append(x.orders, order)
	if _, err := x.store.Insert(ctx, invoice.NewRecord{OrderID: order, WorkspaceID: ws, Provider: "fake",
		Currency: "VND", Amount: 600000, VATAmount: 60000, Buyer: []byte(buyer)}); err != nil {
		x.t.Fatal(err)
	}
	if state != invoice.StatePending {
		providerID := ""
		if state == invoice.StateIssued {
			providerID = "fake-inv-seed"
		}
		if _, err := x.pool.Exec(ctx, `UPDATE einvoice_issuance SET state = $2, attempts = 4, last_error_code = 'rejected',
			next_attempt_at = now() + interval '30 days', provider_invoice_id = $3
			WHERE order_id = $1`, order, string(state), providerID); err != nil {
			x.t.Fatal(err)
		}
	}
	return order
}

func (x *einvoiceDB) record(order string, revision int) invoice.Record {
	x.t.Helper()
	r, err := x.store.Get(context.Background(), order, revision)
	if err != nil {
		x.t.Fatalf("get %s/%d: %v", order, revision, err)
	}
	return r
}

func (x *einvoiceDB) audits(order, action string) []string {
	x.t.Helper()
	rows, err := x.pool.Query(context.Background(),
		`SELECT detail::text FROM audit_event WHERE target_id = $1 AND action = $2`, order, action)
	if err != nil {
		x.t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			x.t.Fatal(err)
		}
		out = append(out, d)
	}
	return out
}

func (x *einvoiceDB) post(user, order, action, body string) *httptest.ResponseRecorder {
	x.t.Helper()
	return x.do(user, "POST", "/api/platform/einvoices/"+order+"/"+action, body)
}

func buyerOf(t *testing.T, raw []byte) invoice.Buyer {
	t.Helper()
	var b invoice.Buyer
	if err := json.Unmarshal(raw, &b); err != nil {
		t.Fatalf("buyer is not JSON: %v", err)
	}
	return b
}

func decodeMap(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body is not JSON: %s", rec.Body)
	}
	return m
}

func TestEinvoiceRetryMovesFailedToPending(t *testing.T) {
	x := newEinvoiceDB(t)
	billing, _ := x.staffUser(staff.RoleBilling)
	order := x.seed(invoice.StateFailed, fullBuyer)

	before := time.Now().Add(-time.Second)
	rec := x.post(billing, order, "retry", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	body := decodeMap(t, rec)
	if body["state"] != "pending" || body["order_id"] != order || body["revision"] != float64(1) || body["attempts"] != float64(0) {
		t.Errorf("response = %s", rec.Body)
	}
	if _, ok := body["missing_fields"]; !ok || strings.Contains(rec.Body.String(), "Cong ty") {
		t.Errorf("response is not the list shape or leaks the buyer: %s", rec.Body)
	}
	got := x.record(order, 1)
	if got.State != invoice.StatePending || got.Attempts != 0 || got.NextAttemptAt.Before(before) || got.NextAttemptAt.After(time.Now().Add(time.Minute)) {
		t.Errorf("record = state %s attempts %d next %v", got.State, got.Attempts, got.NextAttemptAt)
	}
	logs := x.audits(order, "einvoice.retried")
	if len(logs) != 1 {
		t.Fatalf("got %d einvoice.retried rows, want 1", len(logs))
	}
	var d map[string]any
	_ = json.Unmarshal([]byte(logs[0]), &d)
	if d["order_id"] != order || d["revision"] != float64(1) || d["previous_state"] != "failed" || d["attempts_reset"] != true {
		t.Errorf("detail = %s", logs[0])
	}
	if strings.Contains(logs[0], "Cong ty") || strings.Contains(logs[0], "0312345678") || strings.Contains(logs[0], "a@example.com") {
		t.Errorf("detail has buyer values: %s", logs[0])
	}
	if rows := x.staffRows("einvoice.retried", billing); len(rows) != 1 || rows[0].actorType != "staff" {
		t.Errorf("audit actor rows = %v", rows)
	}
}

func TestEinvoiceRetryMovesNeedsInfoToPendingWhenBuyerComplete(t *testing.T) {
	x := newEinvoiceDB(t)
	admin, _ := x.staffUser(staff.RoleAdmin)
	order := x.seed(invoice.StateNeedsInfo, fullBuyer)
	rec := x.post(admin, order, "retry", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	if got := x.record(order, 1); got.State != invoice.StatePending || got.Attempts != 0 || len(got.MissingFields) != 0 {
		t.Errorf("record = %+v", got)
	}
	logs := x.audits(order, "einvoice.retried")
	if len(logs) != 1 || !strings.Contains(logs[0], `"needs_info"`) {
		t.Errorf("audit = %v", logs)
	}
}

func TestEinvoiceRetryJobPicksUpRetriedRecord(t *testing.T) {
	x := newEinvoiceDB(t)
	admin, _ := x.staffUser(staff.RoleAdmin)
	order := x.seed(invoice.StateFailed, fullBuyer)
	if rec := x.post(admin, order, "retry", ""); rec.Code != http.StatusOK {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	if _, err := x.svc.Issue(context.Background()); err != nil {
		t.Fatal(err)
	}
	if x.fake.IssueCalls() < 1 {
		t.Errorf("the provider was not called")
	}
	got := x.record(order, 1)
	if got.State != invoice.StateIssued || got.InvoiceNumber == "" {
		t.Errorf("after the job: state %s number %q", got.State, got.InvoiceNumber)
	}
}

func TestEinvoiceRetryWithIncompleteBuyerReturns409AndKeepsState(t *testing.T) {
	x := newEinvoiceDB(t)
	admin, _ := x.staffUser(staff.RoleAdmin)
	order := x.seed(invoice.StateNeedsInfo, noTaxCodeBuyer)
	rec := x.post(admin, order, "retry", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	body := decodeMap(t, rec)
	fields, _ := body["fields"].([]any)
	if body["code"] != "einvoice_buyer_incomplete" || len(fields) != 1 || fields[0] != "tax_code" {
		t.Errorf("body = %s", rec.Body)
	}
	if strings.Contains(rec.Body.String(), "Cong ty") || strings.Contains(rec.Body.String(), "a@example.com") {
		t.Errorf("response has buyer values: %s", rec.Body)
	}
	if got := x.record(order, 1); got.State != invoice.StateNeedsInfo || got.Attempts != 4 {
		t.Errorf("record changed: state %s attempts %d", got.State, got.Attempts)
	}
	if n := len(x.audits(order, "einvoice.retried")); n != 0 {
		t.Errorf("%d audit rows for a refused retry", n)
	}
}

func TestEinvoiceRetryOnIssuedReturns409(t *testing.T) {
	x := newEinvoiceDB(t)
	admin, _ := x.staffUser(staff.RoleAdmin)
	order := x.seed(invoice.StateIssued, fullBuyer)
	rec := x.post(admin, order, "retry", "")
	if rec.Code != http.StatusConflict || decodeMap(t, rec)["code"] != "einvoice_state_conflict" {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	if got := x.record(order, 1); got.State != invoice.StateIssued {
		t.Errorf("state = %s", got.State)
	}
	if n := len(x.audits(order, "einvoice.retried")); n != 0 {
		t.Errorf("%d audit rows for a refused retry", n)
	}
}

func TestEinvoiceRetryWithoutRecordReturns404(t *testing.T) {
	x := newEinvoiceDB(t)
	admin, _ := x.staffUser(staff.RoleAdmin)
	var order string
	if err := x.pool.QueryRow(context.Background(), `SELECT gen_random_uuid()::text`).Scan(&order); err != nil {
		t.Fatal(err)
	}
	if rec := x.post(admin, order, "retry", ""); rec.Code != http.StatusNotFound {
		t.Errorf("got %d %s, want 404", rec.Code, rec.Body)
	}
}

// With no e-invoice provider configured, retry answers 503 with a
// code the client can show, and write nothing.
func TestEinvoiceRoutesAnswer503WhenNoProviderAndWriteNothing(t *testing.T) {
	x := newEinvoiceDB(t)
	x.h.(*Handler).Einvoices = invoice.NewService(x.store, nil, invoice.Config{StartAt: time.Now().Add(-24 * time.Hour), MaxAttempts: 3})
	billing, _ := x.staffUser(staff.RoleBilling)
	failed := x.seed(invoice.StateFailed, fullBuyer)

	if rec := x.post(billing, failed, "retry", ""); rec.Code != http.StatusServiceUnavailable || decodeMap(t, rec)["code"] != "einvoice_provider_not_configured" {
		t.Errorf("retry got %d %s, want 503 einvoice_provider_not_configured", rec.Code, rec.Body)
	}
	if got := x.record(failed, 1); got.State != invoice.StateFailed || got.Attempts != 4 {
		t.Errorf("failed record changed: state %s attempts %d", got.State, got.Attempts)
	}
	if n := len(x.audits(failed, "einvoice.retried")); n != 0 {
		t.Errorf("got %d audit rows for a refused retry, want 0", n)
	}
}

// The service is not wired at all (the production state until the provider
// lands): the same 503, not a 500.
func TestEinvoiceRoutesAnswer503WhenServiceNotWired(t *testing.T) {
	e := newEnv(t)
	rec := e.do(billID, "POST", "/api/platform/einvoices/"+orderA+"/retry", "")
	if rec.Code != http.StatusServiceUnavailable || decodeMap(t, rec)["code"] != "einvoice_provider_not_configured" {
		t.Errorf("retry got %d %s, want 503 einvoice_provider_not_configured", rec.Code, rec.Body)
	}
}

// The replace route was removed (2026-11-18): the router answers 404 even for
// an admin, and no handler runs.
func TestReplaceRouteIsGone(t *testing.T) {
	e := newEnv(t)
	ops := withEinvoiceOps(e, invoice.Record{OrderID: orderA, Revision: 1, State: invoice.StateIssued, Buyer: []byte(fullBuyer)}, nil)
	rec := e.do(adminID, "POST", "/api/platform/einvoices/"+orderA+"/replace", `{"reason":"wrong tax code"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d %s, want 404 from the router", rec.Code, rec.Body)
	}
	if ops.retries != 0 {
		t.Errorf("an e-invoice operation ran")
	}
}
