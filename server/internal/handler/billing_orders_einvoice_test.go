package handler

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// einvoiceRecord inserts an einvoice_issuance row the way the platform writes
// it. The buyer values are fixture data, not a product assertion.
func einvoiceRecord(t *testing.T, workspaceID, orderID string, revision int, state string, over testutil.Cols) string {
	t.Helper()
	cols := testutil.Cols{
		"order_id": orderID, "revision": revision, "workspace_id": workspaceID, "state": state,
		"buyer": `{"name":"OLD NAME","tax_code":"","address":"","invoice_email":""}`,
	}
	for k, v := range over {
		cols[k] = v
	}
	return dbfx.Insert(t, "einvoice_issuance", cols)
}

func invoiceBuyerRequest(role, orderID string, body any) *http.Request {
	return billingRequest(http.MethodPut, "/api/workspaces/x/orders/y/invoice-buyer", body, role, orderID)
}

type einvoiceRow struct {
	State, Buyer, Missing, Number string
	Revision                      int
	UpdatedAt                     time.Time
}

func readEinvoice(t *testing.T, id string) einvoiceRow {
	t.Helper()
	var row einvoiceRow
	dbfx.QueryRow(t, `SELECT state, buyer::text, missing_fields::text, invoice_number, revision, updated_at
		FROM einvoice_issuance WHERE id = $1`, id).Scan(&row.State, &row.Buyer, &row.Missing, &row.Number, &row.Revision, &row.UpdatedAt)
	return row
}

func getOrderJSON(t *testing.T, orderID string) map[string]json.RawMessage {
	t.Helper()
	var out map[string]json.RawMessage
	testutil.Call(t, testHandler.GetWorkspaceOrder, billingRequest(http.MethodGet, "/api/workspaces/x/orders/y", nil, "owner", orderID)).
		Want(http.StatusOK).JSON(&out)
	return out
}

func TestOrderJSONIncludesEinvoiceWhenPresent(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.BillingWorkspaceSubscriptions, true)
	order := billingOrder(t, testWorkspaceID, "paid", time.Now().UTC())
	einvoiceRecord(t, testWorkspaceID, order, 1, "issued", testutil.Cols{"invoice_number": "OLD-1", "lookup_url": "https://example.test/old"})
	einvoiceRecord(t, testWorkspaceID, order, 2, "needs_info", testutil.Cols{
		"missing_fields": `["tax_code","address"]`, "invoice_number": "", "lookup_url": "",
	})

	out := getOrderJSON(t, order)
	var inv struct {
		State         string   `json:"state"`
		Number        string   `json:"number"`
		LookupURL     string   `json:"lookup_url"`
		MissingFields []string `json:"missing_fields"`
	}
	if err := json.Unmarshal(out["einvoice"], &inv); err != nil {
		t.Fatalf("einvoice = %s (%v)", out["einvoice"], err)
	}
	// The highest revision is the one the page shows.
	if inv.State != "needs_info" || inv.Number != "" || len(inv.MissingFields) != 2 || inv.MissingFields[0] != "tax_code" {
		t.Errorf("einvoice = %+v", inv)
	}

	// An issued latest revision carries its number and link.
	issuedOrder := billingOrder(t, testWorkspaceID, "paid", time.Now().UTC())
	einvoiceRecord(t, testWorkspaceID, issuedOrder, 1, "issued", testutil.Cols{"invoice_number": "A-42", "lookup_url": "https://example.test/42"})
	out = getOrderJSON(t, issuedOrder)
	if err := json.Unmarshal(out["einvoice"], &inv); err != nil {
		t.Fatal(err)
	}
	if inv.State != "issued" || inv.Number != "A-42" || inv.LookupURL != "https://example.test/42" || inv.MissingFields == nil {
		t.Errorf("issued einvoice = %+v", inv)
	}
}

func TestOrderJSONWithoutEinvoiceStillValid(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.BillingWorkspaceSubscriptions, true)
	order := billingOrder(t, testWorkspaceID, "paid", time.Now().UTC())
	// A record of another workspace for the same order id must not leak in.
	other := dbfx.Workspace(t, "Einvoice Foreign", "einvoice-foreign-"+uuid.NewString()[:8])
	einvoiceRecord(t, other, order, 1, "issued", nil)

	out := getOrderJSON(t, order)
	var keys []string
	for k := range out {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	// The keys of an order before the einvoice field existed.
	want := []string{
		"amount", "bank_details", "buyer", "created_at", "currency", "expires_at", "id", "interval", "kind",
		"paid_at", "period_end", "period_start", "plan_key", "provider", "seats", "status", "total_amount",
		"transfer_code", "vat_amount", "workspace_id",
	}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Errorf("keys = %v, want %v", keys, want)
	}
}

func TestInvoiceBuyerUpdateMovesNeedsInfoToPending(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.BillingWorkspaceSubscriptions, true)
	for _, state := range []string{"needs_info", "failed"} {
		for _, role := range []string{"owner", "admin"} {
			order := billingOrder(t, testWorkspaceID, "paid", time.Now().UTC())
			id := einvoiceRecord(t, testWorkspaceID, order, 1, state, testutil.Cols{
				"missing_fields": `["tax_code"]`, "attempts": 3, "last_error_code": "provider_rejected",
				"updated_at": time.Now().Add(-time.Hour).UTC(),
			})
			before := readEinvoice(t, id)

			var out workspaceOrderJSON
			testutil.Call(t, testHandler.UpdateWorkspaceOrderInvoiceBuyer, invoiceBuyerRequest(role, order, buyerPayload())).
				Want(http.StatusOK).JSON(&out)
			if out.ID != order || out.Einvoice == nil || out.Einvoice.State != "pending" {
				t.Errorf("%s/%s: response = %+v", state, role, out)
			}

			after := readEinvoice(t, id)
			var buyer orderBuyerJSON
			if err := json.Unmarshal([]byte(after.Buyer), &buyer); err != nil {
				t.Fatal(err)
			}
			if after.State != "pending" || buyer.TaxCode != "0123456789" || buyer.Name != "CONG TY ACME" {
				t.Errorf("%s/%s: stored = %+v", state, role, after)
			}
			if after.Missing != "[]" {
				t.Errorf("%s/%s: missing_fields = %s, want []", state, role, after.Missing)
			}
			if !after.UpdatedAt.After(before.UpdatedAt) {
				t.Errorf("%s/%s: updated_at did not move", state, role)
			}
			// Nothing outside the four columns changed.
			var attempts int
			var lastErr string
			dbfx.QueryRow(t, `SELECT attempts, last_error_code FROM einvoice_issuance WHERE id = $1`, id).Scan(&attempts, &lastErr)
			if attempts != 3 || lastErr != "provider_rejected" {
				t.Errorf("%s/%s: attempts = %d, last_error_code = %q", state, role, attempts, lastErr)
			}
		}
	}
}

func TestInvoiceBuyerUpdateOnIssuedReturns409(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.BillingWorkspaceSubscriptions, true)
	for _, state := range []string{"issued", "cancelled", "pending"} {
		order := billingOrder(t, testWorkspaceID, "paid", time.Now().UTC())
		id := einvoiceRecord(t, testWorkspaceID, order, 1, state, testutil.Cols{"invoice_number": "A-1"})
		before := readEinvoice(t, id)

		var out map[string]string
		testutil.Call(t, testHandler.UpdateWorkspaceOrderInvoiceBuyer, invoiceBuyerRequest("owner", order, buyerPayload())).
			Want(http.StatusConflict).JSON(&out)
		if out["code"] != "einvoice_already_issued" {
			t.Errorf("%s: body = %v", state, out)
		}
		if after := readEinvoice(t, id); after != before {
			t.Errorf("%s: the record changed from %+v to %+v", state, before, after)
		}
	}

	// Only the latest revision counts: an older failed one is not editable.
	order := billingOrder(t, testWorkspaceID, "paid", time.Now().UTC())
	old := einvoiceRecord(t, testWorkspaceID, order, 1, "failed", nil)
	einvoiceRecord(t, testWorkspaceID, order, 2, "issued", testutil.Cols{"invoice_number": "A-2"})
	before := readEinvoice(t, old)
	testutil.Call(t, testHandler.UpdateWorkspaceOrderInvoiceBuyer, invoiceBuyerRequest("owner", order, buyerPayload())).
		Want(http.StatusConflict)
	if after := readEinvoice(t, old); after != before {
		t.Errorf("an older revision changed from %+v to %+v", before, after)
	}
}

func TestInvoiceBuyerUpdateValidates(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.BillingWorkspaceSubscriptions, true)
	order := billingOrder(t, testWorkspaceID, "paid", time.Now().UTC())
	id := einvoiceRecord(t, testWorkspaceID, order, 1, "needs_info", nil)
	before := readEinvoice(t, id)

	bad := buyerPayload()
	bad["tax_code"] = "12345"
	var out map[string]string
	testutil.Call(t, testHandler.UpdateWorkspaceOrderInvoiceBuyer, invoiceBuyerRequest("owner", order, bad)).
		Want(http.StatusBadRequest).JSON(&out)
	if out["code"] != "buyer_invalid" || out["field"] != "tax_code" {
		t.Errorf("body = %v", out)
	}
	bad = buyerPayload()
	bad["invoice_email"] = "not-an-email"
	testutil.Call(t, testHandler.UpdateWorkspaceOrderInvoiceBuyer, invoiceBuyerRequest("owner", order, bad)).
		Want(http.StatusBadRequest)
	testutil.Call(t, testHandler.UpdateWorkspaceOrderInvoiceBuyer, invoiceBuyerRequest("owner", order, "not an object")).
		Want(http.StatusBadRequest)
	testutil.Call(t, testHandler.UpdateWorkspaceOrderInvoiceBuyer, invoiceBuyerRequest("owner", "not-a-uuid", buyerPayload())).
		Want(http.StatusBadRequest)
	if after := readEinvoice(t, id); after != before {
		t.Errorf("a refused buyer changed the record from %+v to %+v", before, after)
	}

	// Unknown order, foreign order and an order without an invoice are 404.
	testutil.Call(t, testHandler.UpdateWorkspaceOrderInvoiceBuyer, invoiceBuyerRequest("owner", uuid.NewString(), buyerPayload())).
		Want(http.StatusNotFound)
	noInvoice := billingOrder(t, testWorkspaceID, "paid", time.Now().UTC())
	testutil.Call(t, testHandler.UpdateWorkspaceOrderInvoiceBuyer, invoiceBuyerRequest("owner", noInvoice, buyerPayload())).
		Want(http.StatusNotFound)
	other := dbfx.Workspace(t, "Einvoice Foreign 2", "einvoice-foreign-"+uuid.NewString()[:8])
	foreign := billingOrder(t, other, "paid", time.Now().UTC())
	foreignInv := einvoiceRecord(t, other, foreign, 1, "needs_info", nil)
	foreignBefore := readEinvoice(t, foreignInv)
	testutil.Call(t, testHandler.UpdateWorkspaceOrderInvoiceBuyer, invoiceBuyerRequest("owner", foreign, buyerPayload())).
		Want(http.StatusNotFound)
	if after := readEinvoice(t, foreignInv); after != foreignBefore {
		t.Errorf("another workspace's record changed")
	}
}

func TestInvoiceBuyerUpdateDoesNotTouchOrderBuyer(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.BillingWorkspaceSubscriptions, true)
	order := billingOrder(t, testWorkspaceID, "paid", time.Now().UTC())
	einvoiceRecord(t, testWorkspaceID, order, 1, "needs_info", nil)
	before := storedBuyer(t, order)

	testutil.Call(t, testHandler.UpdateWorkspaceOrderInvoiceBuyer, invoiceBuyerRequest("owner", order, buyerPayload())).
		Want(http.StatusOK)
	if after := storedBuyer(t, order); after != before {
		t.Errorf("billing_order.buyer changed from %s to %s", before, after)
	}
}

func TestInvoiceBuyerRouteNeedsHumanActorAndRole(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.BillingWorkspaceSubscriptions, true)
	order := billingOrder(t, testWorkspaceID, "paid", time.Now().UTC())
	id := einvoiceRecord(t, testWorkspaceID, order, 1, "needs_info", nil)
	before := readEinvoice(t, id)

	testutil.Call(t, testHandler.UpdateWorkspaceOrderInvoiceBuyer, invoiceBuyerRequest("member", order, buyerPayload())).
		Want(http.StatusForbidden)
	for _, source := range []string{"task_token", "cloud_pat"} {
		req := invoiceBuyerRequest("owner", order, buyerPayload())
		req.Header.Set("X-Actor-Source", source)
		testutil.Call(t, testHandler.UpdateWorkspaceOrderInvoiceBuyer, req).Want(http.StatusForbidden)
	}
	if after := readEinvoice(t, id); after != before {
		t.Errorf("a refused caller changed the record from %+v to %+v", before, after)
	}
}

func TestAuditDetailHasFieldNamesOnly(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.BillingWorkspaceSubscriptions, true)
	order := billingOrder(t, testWorkspaceID, "paid", time.Now().UTC())
	einvoiceRecord(t, testWorkspaceID, order, 1, "needs_info", nil)
	dbfx.Cleanup(t, `DELETE FROM audit_event WHERE target_id = $1`, order)

	testutil.Call(t, testHandler.UpdateWorkspaceOrderInvoiceBuyer, invoiceBuyerRequest("owner", order, buyerPayload())).
		Want(http.StatusOK)

	var action, detail string
	dbfx.QueryRow(t, `SELECT action, detail::text FROM audit_event WHERE target_id = $1`, order).Scan(&action, &detail)
	if action != "einvoice.buyer_updated" {
		t.Errorf("action = %q", action)
	}
	for _, value := range []string{"CONG TY ACME", "0123456789", "1 Le Loi", "ke.toan@example.com", "OLD NAME"} {
		if strings.Contains(detail, value) {
			t.Errorf("the audit detail holds a buyer value %q: %s", value, detail)
		}
	}
	var parsed struct {
		Fields []string `json:"fields"`
	}
	if err := json.Unmarshal([]byte(detail), &parsed); err != nil {
		t.Fatal(err)
	}
	if strings.Join(parsed.Fields, ",") != "name,tax_code,address,invoice_email" {
		t.Errorf("fields = %v", parsed.Fields)
	}
}
