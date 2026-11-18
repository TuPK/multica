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

// Routes: POST /einvoices/{orderId}/retry (billing, admin) and
// POST /einvoices/{orderId}/replace (admin). Role, body and not-found tests
// run on a fake; state, audit and job tests run on the platform test database
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
	replace int
}

func (f *fakeEinvoiceOps) Latest(context.Context, string) (invoice.Record, error) {
	return f.latest, f.err
}

func (f *fakeEinvoiceOps) Retry(context.Context, string, int, string) (invoice.Record, error) {
	f.retries++
	return f.latest, f.err
}

func (f *fakeEinvoiceOps) Replace(context.Context, string, int, invoice.Buyer, string, string) (invoice.Record, error) {
	f.replace++
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

func TestEinvoiceReplaceRequiresAdmin(t *testing.T) {
	e := newEnv(t)
	ops := withEinvoiceOps(e, invoice.Record{OrderID: orderA, Revision: 1, State: invoice.StateIssued, Buyer: []byte(fullBuyer)}, nil)
	body := `{"reason":"wrong tax code"}`
	for _, id := range []string{billID, suppID} {
		rec := e.do(id, "POST", "/api/platform/einvoices/"+orderA+"/replace", body)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s got %d %s, want the 404 of an unknown route", id, rec.Code, rec.Body)
		}
	}
	if ops.replace != 0 {
		t.Errorf("a non-admin reached the replace")
	}
	if rec := e.do(adminID, "POST", "/api/platform/einvoices/"+orderA+"/replace", body); rec.Code != http.StatusOK {
		t.Errorf("admin got %d %s, want 200", rec.Code, rec.Body)
	}
}

func TestEinvoiceReplaceRejectsEmptyReasonWith400(t *testing.T) {
	e := newEnv(t)
	ops := withEinvoiceOps(e, invoice.Record{OrderID: orderA, Revision: 1, State: invoice.StateIssued, Buyer: []byte(fullBuyer)}, nil)
	for _, body := range []string{`{}`, `{"reason":""}`, `{"reason":"   "}`, `{"buyer":{"name":"x"}}`} {
		rec := e.do(adminID, "POST", "/api/platform/einvoices/"+orderA+"/replace", body)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"reason_required"`) {
			t.Errorf("body %s got %d %s, want 400 reason_required", body, rec.Code, rec.Body)
		}
	}
	if ops.replace != 0 {
		t.Errorf("a request without a reason reached the replace")
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

func TestEinvoiceReplaceCreatesRevisionAndKeepsOld(t *testing.T) {
	x := newEinvoiceDB(t)
	admin, _ := x.staffUser(staff.RoleAdmin)
	order := x.seed(invoice.StateIssued, fullBuyer)
	newBuyer := `{"name":"Cong ty B","tax_code":"0109999999","address":"2 Pho B","email":"b@example.com"}`
	rec := x.post(admin, order, "replace", `{"reason":" wrong tax code ","buyer":`+newBuyer+`}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	body := decodeMap(t, rec)
	if body["revision"] != float64(2) || body["state"] != "pending" || body["order_id"] != order {
		t.Errorf("response = %s", rec.Body)
	}
	old, next := x.record(order, 1), x.record(order, 2)
	if old.State != invoice.StateCancelled || old.CancelReason != "wrong tax code" || old.CancelledBy != admin {
		t.Errorf("old = state %s reason %q by %q", old.State, old.CancelReason, old.CancelledBy)
	}
	if next.State != invoice.StatePending || buyerOf(t, next.Buyer) != buyerOf(t, []byte(newBuyer)) {
		t.Errorf("new = state %s buyer %s", next.State, next.Buyer)
	}
	if x.fake.IssueCalls() != 0 {
		t.Errorf("the provider was called by the route")
	}

	// Without a buyer in the body the replacement keeps the record's buyer.
	order2 := x.seed(invoice.StateIssued, fullBuyer)
	if rec := x.post(admin, order2, "replace", `{"reason":"redo"}`); rec.Code != http.StatusOK {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	if got := x.record(order2, 2); buyerOf(t, got.Buyer) != buyerOf(t, []byte(fullBuyer)) {
		t.Errorf("kept buyer = %s", got.Buyer)
	}
}

func TestEinvoiceReplaceOnPendingReturns409(t *testing.T) {
	x := newEinvoiceDB(t)
	admin, _ := x.staffUser(staff.RoleAdmin)
	for _, s := range []invoice.State{invoice.StatePending, invoice.StateFailed} {
		order := x.seed(s, fullBuyer)
		rec := x.post(admin, order, "replace", `{"reason":"redo"}`)
		if rec.Code != http.StatusConflict || decodeMap(t, rec)["code"] != "einvoice_state_conflict" {
			t.Errorf("%s: got %d %s", s, rec.Code, rec.Body)
		}
		if n := x.count(`SELECT count(*) FROM einvoice_issuance WHERE order_id = $1`, order); n != 1 {
			t.Errorf("%s: %d rows, want 1", s, n)
		}
	}
}

func TestEinvoiceReplaceWithoutIssuedRecordReturns404(t *testing.T) {
	x := newEinvoiceDB(t)
	admin, _ := x.staffUser(staff.RoleAdmin)
	var order string
	if err := x.pool.QueryRow(context.Background(), `SELECT gen_random_uuid()::text`).Scan(&order); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"replace", "retry"} {
		body := ""
		if action == "replace" {
			body = `{"reason":"redo"}`
		}
		if rec := x.post(admin, order, action, body); rec.Code != http.StatusNotFound {
			t.Errorf("%s: got %d %s, want 404", action, rec.Code, rec.Body)
		}
	}
}

func TestEinvoiceReplaceAuditHasNoBuyerValues(t *testing.T) {
	x := newEinvoiceDB(t)
	admin, _ := x.staffUser(staff.RoleAdmin)
	order := x.seed(invoice.StateIssued, fullBuyer)
	newBuyer := `{"name":"Cong ty B","tax_code":"0109999999","address":"2 Pho B","email":"b@example.com"}`
	if rec := x.post(admin, order, "replace", `{"reason":"wrong tax code","buyer":`+newBuyer+`}`); rec.Code != http.StatusOK {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	logs := x.audits(order, "einvoice.replaced")
	if len(logs) != 1 {
		t.Fatalf("got %d einvoice.replaced rows, want exactly 1", len(logs))
	}
	var d map[string]any
	_ = json.Unmarshal([]byte(logs[0]), &d)
	for k := range d {
		switch k {
		case "order_id", "revision", "new_revision", "reason":
		default:
			t.Errorf("unexpected detail key %q", k)
		}
	}
	for _, v := range []string{"Cong ty", "0312345678", "0109999999", "Pho", "example.com"} {
		if strings.Contains(logs[0], v) {
			t.Errorf("detail has buyer value %q: %s", v, logs[0])
		}
	}
}

// With no e-invoice provider configured, retry and replace answer 503 with a
// code the client can show, and write nothing.
func TestEinvoiceRoutesAnswer503WhenNoProviderAndWriteNothing(t *testing.T) {
	x := newEinvoiceDB(t)
	x.h.(*Handler).Einvoices = invoice.NewService(x.store, nil, invoice.Config{StartAt: time.Now().Add(-24 * time.Hour), MaxAttempts: 3})
	billing, _ := x.staffUser(staff.RoleBilling)
	admin, _ := x.staffUser(staff.RoleAdmin)
	failed := x.seed(invoice.StateFailed, fullBuyer)
	issued := x.seed(invoice.StateIssued, fullBuyer)

	if rec := x.post(billing, failed, "retry", ""); rec.Code != http.StatusServiceUnavailable || decodeMap(t, rec)["code"] != "einvoice_provider_not_configured" {
		t.Errorf("retry got %d %s, want 503 einvoice_provider_not_configured", rec.Code, rec.Body)
	}
	if rec := x.post(admin, issued, "replace", `{"reason":"wrong tax code"}`); rec.Code != http.StatusServiceUnavailable || decodeMap(t, rec)["code"] != "einvoice_provider_not_configured" {
		t.Errorf("replace got %d %s, want 503 einvoice_provider_not_configured", rec.Code, rec.Body)
	}
	if got := x.record(failed, 1); got.State != invoice.StateFailed || got.Attempts != 4 {
		t.Errorf("failed record changed: state %s attempts %d", got.State, got.Attempts)
	}
	if got := x.record(issued, 1); got.State != invoice.StateIssued {
		t.Errorf("the replace target changed: state %s", got.State)
	}
	var revisions int
	if err := x.pool.QueryRow(context.Background(), `SELECT count(*) FROM einvoice_issuance WHERE order_id = $1`, issued).Scan(&revisions); err != nil {
		t.Fatal(err)
	}
	if revisions != 1 {
		t.Errorf("order has %d revisions, want 1", revisions)
	}
	if n := len(x.audits(failed, "einvoice.retried")) + len(x.audits(issued, "einvoice.replaced")); n != 0 {
		t.Errorf("got %d audit rows for refused actions, want 0", n)
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

func TestEinvoiceReplaceUnsupportedIs503(t *testing.T) {
	e := newEnv(t)
	withEinvoiceOps(e, invoice.Record{}, invoice.ErrReplaceUnsupported)
	rec := e.do(adminID, "POST", "/api/platform/einvoices/"+orderA+"/replace", `{"reason":"wrong tax code"}`)
	if rec.Code != http.StatusServiceUnavailable || decodeMap(t, rec)["code"] != "einvoice_replace_unsupported" {
		t.Errorf("got %d %s, want 503 einvoice_replace_unsupported", rec.Code, rec.Body)
	}
}
