package console

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/invoice"
)

func TestEinvoiceListDefaultsToFailedAndNeedsInfo(t *testing.T) {
	e := newEnv(t)
	nr, _ := withNotices(e)
	nr.invoices = []EinvoiceRow{{
		OrderID: orderA, WorkspaceID: wsID, Revision: 1, State: invoice.StateNeedsInfo, Currency: "vnd",
		Amount: 600000, VATAmount: 60000, Provider: "sepay", Attempts: 0,
		MissingFields: []string{"tax_code", "address"}, NextAttemptAt: t0, UpdatedAt: t0,
	}}
	rec := e.do(billID, "GET", "/api/platform/einvoices", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	if len(nr.lastInvStates) != 2 || nr.lastInvStates[0] != invoice.StateNeedsInfo || nr.lastInvStates[1] != invoice.StateFailed || nr.lastInvLimit != 50 {
		t.Errorf("asked for %v limit %d, want needs_info+failed and 50", nr.lastInvStates, nr.lastInvLimit)
	}
	var body struct {
		Einvoices []map[string]any `json:"einvoices"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Einvoices) != 1 {
		t.Fatalf("body = %s (%v)", rec.Body, err)
	}
	item := body.Einvoices[0]
	for _, k := range []string{"order_id", "workspace_id", "revision", "state", "currency", "amount", "vat_amount", "provider",
		"invoice_number", "lookup_url", "attempts", "last_error_code", "missing_fields", "next_attempt_at", "updated_at"} {
		if _, ok := item[k]; !ok {
			t.Errorf("item has no %q", k)
		}
	}
	if mf, _ := item["missing_fields"].([]any); len(mf) != 2 || mf[0] != "tax_code" {
		t.Errorf("missing_fields = %v", item["missing_fields"])
	}

	if rec := e.do(adminID, "GET", "/api/platform/einvoices?state="+string(invoice.StateIssued)+"&limit=200", ""); rec.Code != 200 || len(nr.lastInvStates) != 1 || nr.lastInvStates[0] != invoice.StateIssued || nr.lastInvLimit != 200 {
		t.Errorf("state filter: %d %s, states %v limit %d", rec.Code, rec.Body, nr.lastInvStates, nr.lastInvLimit)
	}
	for _, q := range []string{"state=cancelled", "state=x", "limit=0", "limit=201"} {
		if rec := e.do(billID, "GET", "/api/platform/einvoices?"+q, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("?%s = %d, want 400", q, rec.Code)
		}
	}
}

func TestEinvoiceListNeverReturnsBuyer(t *testing.T) {
	e := newEnv(t)
	nr, _ := withNotices(e)
	nr.invoices = []EinvoiceRow{{OrderID: orderA, WorkspaceID: wsID, Revision: 2, State: invoice.StateFailed, LastErrorCode: "rejected"}}
	rec := e.do(billID, "GET", "/api/platform/einvoices", "")
	if rec.Code != 200 {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(strings.ToLower(rec.Body.String()), "buyer") {
		t.Errorf("response mentions buyer: %s", rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"missing_fields":[]`) {
		t.Errorf("empty missing_fields is not an array: %s", rec.Body)
	}
	if strings.Contains(strings.ToLower(einvoiceListSQL), "buyer") {
		t.Errorf("the list query selects buyer:\n%s", einvoiceListSQL)
	}
}
