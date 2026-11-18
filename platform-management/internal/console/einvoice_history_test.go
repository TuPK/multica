package console

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/invoice"
)

// Distinctive buyer values: no response body or log line may contain them.
const (
	secretTaxCode = "9988776655-321"
	secretAddress = "77 Hidden Alley, District 9"
)

// The isolation scan reads string literals that start with "issue" as a content table.
var (
	issuedState = string(invoice.StateIssued)
	issuedAtKey = issuedState + "_at"
)

const secretBuyer = `{"name":"Cong ty Bi Mat","tax_code":"` + secretTaxCode + `","address":"` + secretAddress + `","email":"bi@mat.example"}`

func historyRow(rev int, state invoice.State, buyer string) EinvoiceHistoryRow {
	issued := t0.Add(time.Duration(rev) * time.Hour)
	row := EinvoiceHistoryRow{
		Revision: rev, State: state, InvoiceNumber: "INV-" + string(rune('0'+rev)), LookupURL: "https://lookup.example/" + string(rune('0'+rev)),
		Buyer: []byte(buyer), Attempts: rev, LastErrorCode: "rejected", CreatedAt: t0, UpdatedAt: t0.Add(time.Minute),
	}
	if state == invoice.StateIssued {
		row.IssuedAt = &issued
	}
	return row
}

func TestOrdersListShowsInvoiceState(t *testing.T) {
	e := newEnv(t)
	seedOrder(e, orderA, billing.StatusPaid, t0.Add(-2*time.Hour))
	seedOrder(e, orderB, billing.StatusPaid, t0.Add(-time.Hour))
	seedOrder(e, orderC, billing.StatusPaid, t0)
	e.reader.invoiceStates = map[string]string{orderA: issuedState, orderC: string(invoice.StateCancelled)}
	rec := e.do(billID, "GET", "/api/platform/orders?status=paid", "")
	if rec.Code != 200 {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	var body struct {
		Orders []map[string]any `json:"orders"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	got := map[string]any{}
	for _, o := range body.Orders {
		v, present := o["invoice_state"]
		if !present {
			t.Errorf("order %v has no invoice_state key", o["id"])
		}
		got[o["id"].(string)] = v
	}
	if got[orderA] != issuedState {
		t.Errorf("order with a first record: invoice_state = %v, want the stored state", got[orderA])
	}
	if got[orderB] != nil {
		t.Errorf("order without a record: invoice_state = %v, want null", got[orderB])
	}
	if got[orderC] != nil {
		t.Errorf("order whose latest state is cancelled: invoice_state = %v, want null", got[orderC])
	}
}

func TestOrderMutationResponsesHaveNoInvoiceState(t *testing.T) {
	e := newEnv(t)
	seedOrder(e, orderA, billing.StatusPending, t0.Add(-time.Hour))
	rec := e.do(billID, "POST", "/api/platform/orders/"+orderA+"/cancel", `{"reason":"duplicate"}`)
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "invoice_state") {
		t.Errorf("cancel response = %d %s, want 200 without invoice_state", rec.Code, rec.Body)
	}
}

func TestOrderEinvoiceHistoryNewestFirstWithExactFields(t *testing.T) {
	e := newEnv(t)
	seedOrder(e, orderA, billing.StatusPaid, t0)
	e.reader.history = map[string][]EinvoiceHistoryRow{orderA: {
		historyRow(2, invoice.StateIssued, `{"name":"B2","tax_code":"2222","address":"2 Rd","email":"b2@example.com"}`),
		historyRow(1, invoice.StateFailed, `{"name":"B1","tax_code":"1111","address":"1 Rd","email":"b1@example.com"}`),
	}}
	rec := e.do(billID, "GET", "/api/platform/orders/"+orderA+"/einvoices", "")
	if rec.Code != 200 {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	var body struct {
		Einvoices []map[string]any `json:"einvoices"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Einvoices) != 2 {
		t.Fatalf("body = %s (%v)", rec.Body, err)
	}
	if body.Einvoices[0]["revision"] != float64(2) || body.Einvoices[1]["revision"] != float64(1) {
		t.Errorf("revisions = %v, %v, want 2 then 1", body.Einvoices[0]["revision"], body.Einvoices[1]["revision"])
	}
	first := body.Einvoices[0]
	want := []string{"revision", "state", "invoice_number", "lookup_url", "buyer", "attempts", "last_error_code", "created_at", issuedAtKey, "updated_at"}
	if len(first) != len(want) {
		t.Errorf("item has %d fields %v, want exactly %v", len(first), first, want)
	}
	for _, k := range want {
		if _, ok := first[k]; !ok {
			t.Errorf("item has no %q", k)
		}
	}
	if first["state"] != issuedState || first["invoice_number"] != "INV-2" || first["attempts"] != float64(2) || first["last_error_code"] != "rejected" {
		t.Errorf("item = %v", first)
	}
	if first["created_at"] != ts(t0) || first["updated_at"] != ts(t0.Add(time.Minute)) || first[issuedAtKey] != ts(t0.Add(2*time.Hour)) {
		t.Errorf("timestamps = %v %v %v", first["created_at"], first["updated_at"], first[issuedAtKey])
	}
	buyer, _ := first["buyer"].(map[string]any)
	if buyer["name"] != "B2" || buyer["tax_code"] != "2222" || buyer["address"] != "2 Rd" || buyer["invoice_email"] != "b2@example.com" || len(buyer) != 4 {
		t.Errorf("buyer = %v, want the snapshot with email as invoice_email", buyer)
	}
	if body.Einvoices[1][issuedAtKey] != nil {
		t.Errorf("failed revision completion time = %v, want null", body.Einvoices[1][issuedAtKey])
	}
}

func TestOrderEinvoiceHistoryUnparsableBuyerReadsEmpty(t *testing.T) {
	e := newEnv(t)
	seedOrder(e, orderA, billing.StatusPaid, t0)
	e.reader.history = map[string][]EinvoiceHistoryRow{orderA: {historyRow(1, invoice.StatePending, `[1,2]`)}}
	rec := e.do(billID, "GET", "/api/platform/orders/"+orderA+"/einvoices", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"buyer":{"name":"","tax_code":"","address":"","invoice_email":""}`) {
		t.Errorf("got %d %s, want an empty buyer", rec.Code, rec.Body)
	}
}

func TestOrderEinvoiceHistoryEmptyIsAnEmptyArray(t *testing.T) {
	e := newEnv(t)
	seedOrder(e, orderA, billing.StatusPaid, t0)
	rec := e.do(adminID, "GET", "/api/platform/orders/"+orderA+"/einvoices", "")
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"einvoices":[]}` {
		t.Errorf("got %d %s, want 200 {\"einvoices\":[]}", rec.Code, rec.Body)
	}
}

func TestOrderEinvoiceHistoryUnknownAndMalformedOrder(t *testing.T) {
	e := newEnv(t)
	wantError(t, e.do(billID, "GET", "/api/platform/orders/"+orderB+"/einvoices", ""), 404, "order_not_found")
	wantStatus(t, e.do(billID, "GET", "/api/platform/orders/not-a-uuid/einvoices", ""), 404)
}

func TestOrderEinvoiceHistoryRequiresBillingOrAdmin(t *testing.T) {
	e := newEnv(t)
	seedOrder(e, orderA, billing.StatusPaid, t0)
	// The staff middleware answers a refused role with the 404 of an unknown
	// route, as on every other console route; it is not a 403.
	if rec := e.do(suppID, "GET", "/api/platform/orders/"+orderA+"/einvoices", ""); rec.Code != http.StatusNotFound {
		t.Errorf("support got %d %s, want 404", rec.Code, rec.Body)
	}
	for _, id := range []string{billID, adminID} {
		if rec := e.do(id, "GET", "/api/platform/orders/"+orderA+"/einvoices", ""); rec.Code != http.StatusOK {
			t.Errorf("%s got %d %s, want 200", id, rec.Code, rec.Body)
		}
	}
}

func TestOrderEinvoiceHistoryNeverLeaksBuyerInErrorsOrLogs(t *testing.T) {
	e := newEnv(t)
	seedOrder(e, orderA, billing.StatusPaid, t0)
	e.reader.history = map[string][]EinvoiceHistoryRow{orderA: {historyRow(1, invoice.StateIssued, secretBuyer)}}
	check := func(label string, rec interface{ String() string }) {
		t.Helper()
		if s := rec.String(); strings.Contains(s, secretTaxCode) || strings.Contains(s, secretAddress) {
			t.Errorf("%s contains buyer values", label)
		}
	}
	// Success: the buyer is the revision's own snapshot and is part of the body,
	// so only the error paths and the logs are checked for absence.
	rec := e.do(billID, "GET", "/api/platform/orders/"+orderB+"/einvoices", "")
	wantError(t, rec, 404, "order_not_found")
	check("404 body", rec.Body)

	e.reader.historyErr = errors.New("history read failed")
	rec = e.do(billID, "GET", "/api/platform/orders/"+orderA+"/einvoices", "")
	if rec.Code != 500 {
		t.Fatalf("forced error: got %d %s, want 500", rec.Code, rec.Body)
	}
	check("500 body", rec.Body)
	check("log", e.logs)
}

// --- database-backed tests ---

// seedRevision adds a revision of an order's e-invoice in state with the buyer.
func (o *orderFixture) seedRevision(order string, revision int, state invoice.State, buyer string) {
	o.t.Helper()
	if _, err := o.pool.Exec(context.Background(), `INSERT INTO einvoice_issuance (order_id, revision, workspace_id, state, buyer, invoice_number, lookup_url, attempts, last_error_code, `+issuedAtKey+`)
		VALUES ($1, $2::int, $3, $4::text, $5::jsonb, 'INV-'||$2::int::text, 'https://lookup.example/'||$2::int::text, $2::int, 'rejected', CASE WHEN $4::text = $6::text THEN now() END)`,
		order, revision, o.ws, string(state), buyer, issuedState); err != nil {
		o.t.Fatalf("seed revision: %v", err)
	}
	o.t.Cleanup(func() {
		o.pool.Exec(context.Background(), `DELETE FROM einvoice_issuance WHERE order_id = $1`, order)
	})
}

func invoiceStates(t *testing.T, f *orderFixture, user, status string) map[string]any {
	t.Helper()
	rec := f.do(user, "GET", "/api/platform/orders?status="+status, "")
	wantStatus(t, rec, 200)
	var body struct {
		Orders []struct {
			ID    string `json:"id"`
			State any    `json:"invoice_state"`
		} `json:"orders"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	out := map[string]any{}
	for _, o := range body.Orders {
		out[o.ID] = o.State
	}
	return out
}

func TestDBOrdersListInvoiceStateIsTheHighestRevision(t *testing.T) {
	f := newOrderFixture(t)
	billID, _ := f.staffUser("billing")
	issued := f.seedOrder("paid", 3*time.Hour, 24*time.Hour)
	none := f.seedOrder("paid", 2*time.Hour, 24*time.Hour)
	several := f.seedOrder("paid", time.Hour, 24*time.Hour)
	cancelled := f.seedOrder("paid", 30*time.Minute, 24*time.Hour)
	f.seedRevision(issued, 1, invoice.StateIssued, secretBuyer)
	f.seedRevision(several, 1, invoice.StateIssued, secretBuyer)
	f.seedRevision(several, 2, invoice.StateNeedsInfo, secretBuyer)
	f.seedRevision(cancelled, 1, invoice.StateIssued, secretBuyer)
	f.seedRevision(cancelled, 2, invoice.StateCancelled, secretBuyer)

	got := invoiceStates(t, f, billID, "paid")
	if got[issued] != issuedState {
		t.Errorf("order with one record: %v, want the stored state", got[issued])
	}
	if v, ok := got[none]; !ok || v != nil {
		t.Errorf("order without a record: %v (listed %v), want null", v, ok)
	}
	if got[several] != "needs_info" {
		t.Errorf("order with revisions 1 and 2: %v, want the state of revision 2", got[several])
	}
	if v, ok := got[cancelled]; !ok || v != nil {
		t.Errorf("order whose latest revision is cancelled: %v (listed %v), want null", v, ok)
	}
}

func TestDBOrderEinvoiceHistoryReadsTheRevisionSnapshot(t *testing.T) {
	f := newOrderFixture(t)
	billID, _ := f.staffUser("billing")
	order := f.seedOrder("paid", time.Hour, 24*time.Hour)
	empty := f.seedOrder("paid", 2*time.Hour, 24*time.Hour)
	f.seedRevision(order, 1, invoice.StateIssued, secretBuyer)
	f.seedRevision(order, 2, invoice.StateFailed, `{"name":"New","tax_code":"5555","address":"5 Rd","email":"n@example.com"}`)

	rec := f.do(billID, "GET", "/api/platform/orders/"+order+"/einvoices", "")
	wantStatus(t, rec, 200)
	var body struct {
		Einvoices []struct {
			Revision int    `json:"revision"`
			State    string `json:"state"`
			Attempts int    `json:"attempts"`
			Buyer    struct {
				Name         string `json:"name"`
				TaxCode      string `json:"tax_code"`
				Address      string `json:"address"`
				InvoiceEmail string `json:"invoice_email"`
			} `json:"buyer"`
		} `json:"einvoices"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Einvoices) != 2 {
		t.Fatalf("body = %s (%v)", rec.Body, err)
	}
	a, b := body.Einvoices[0], body.Einvoices[1]
	if a.Revision != 2 || a.State != "failed" || a.Buyer.Name != "New" || a.Buyer.InvoiceEmail != "n@example.com" {
		t.Errorf("revision 2 = %+v", a)
	}
	// billing_order.buyer says Acme; the revision's own snapshot wins.
	if b.Revision != 1 || b.State != issuedState || b.Buyer.TaxCode != secretTaxCode || b.Buyer.Address != secretAddress {
		t.Errorf("revision 1 = %+v", b)
	}

	rec = f.do(billID, "GET", "/api/platform/orders/"+empty+"/einvoices", "")
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"einvoices":[]}` {
		t.Errorf("order without records: %d %s", rec.Code, rec.Body)
	}
	wantError(t, f.do(billID, "GET", "/api/platform/orders/eeeeeeee-9999-0000-0000-000000000009/einvoices", ""), 404, "order_not_found")
}
