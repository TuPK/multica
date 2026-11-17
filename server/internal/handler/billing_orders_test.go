package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/cloudruntime"
	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// --- Checkout forwarding: plan_key and seats.

func checkoutRequest(t *testing.T, body map[string]any) (*fakeCloudRuntimeProxy, *httptest.ResponseRecorder) {
	t.Helper()
	proxy := &fakeCloudRuntimeProxy{
		enabled: true,
		resp:    &cloudruntime.Response{StatusCode: http.StatusOK, Body: []byte(`{"url":"https://app.example.test/o"}`)},
	}
	useCloudRuntimeProxy(t, proxy)
	req := newRequest(http.MethodPost, "/api/cloud-subscriptions/checkout-sessions", body)
	req = withCloudSubscriptionWorkspace(req, "owner")
	w := httptest.NewRecorder()
	testHandler.CreateCloudWorkspaceSubscriptionCheckout(w, req)
	return proxy, w
}

func TestCloudSubscriptionCheckoutForwardsPlanKeyAndSeats(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.BillingWorkspaceSubscriptions, true)
	proxy, w := checkoutRequest(t, map[string]any{
		"interval": "year", "idempotency_key": "k1", "plan_key": "business", "seats": 7,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(proxy.req.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body["plan_key"] != "business" || body["seats"] != float64(7) {
		t.Fatalf("upstream body = %s, want plan_key and seats forwarded", proxy.req.Body)
	}
	if body["workspace_id"] != testWorkspaceID || body["interval"] != "year" {
		t.Fatalf("upstream body = %s", proxy.req.Body)
	}
}

func TestCloudSubscriptionCheckoutWithoutPlanKeyAndSeatsSendsNeither(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.BillingWorkspaceSubscriptions, true)
	proxy, w := checkoutRequest(t, map[string]any{"interval": "month", "idempotency_key": "k1"})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if strings.Contains(string(proxy.req.Body), "plan_key") || strings.Contains(string(proxy.req.Body), "seats") {
		t.Fatalf("upstream body = %s, want neither field", proxy.req.Body)
	}
}

func TestCloudSubscriptionCheckoutRefusesBadPlanKeyAndSeats(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.BillingWorkspaceSubscriptions, true)
	for name, body := range map[string]map[string]any{
		"zero seats":     {"interval": "month", "idempotency_key": "k", "seats": 0},
		"negative seats": {"interval": "month", "idempotency_key": "k", "seats": -2},
		"long plan key":  {"interval": "month", "idempotency_key": "k", "plan_key": strings.Repeat("p", 65)},
	} {
		proxy, w := checkoutRequest(t, body)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, body = %s", name, w.Code, w.Body.String())
		}
		if proxy.called {
			t.Errorf("%s: the upstream was called", name)
		}
	}
}

// --- The buyer fields.

func TestNormalizeOrderBuyer(t *testing.T) {
	okCases := map[string]orderBuyerJSON{
		"empty":                   {},
		"ten digit tax code":      {Name: "ACME", TaxCode: "0123456789"},
		"thirteen digit tax code": {TaxCode: "0123456789012"},
		"all four":                {Name: "ACME Co", TaxCode: "0123456789", Address: "1 Le Loi, Ha Noi", InvoiceEmail: "ke.toan@example.com"},
		"trimmed":                 {Name: "  ACME  ", TaxCode: " 0123456789 ", InvoiceEmail: " a@example.com "},
	}
	for name, in := range okCases {
		if _, err := normalizeOrderBuyer(in); err != nil {
			t.Errorf("%s: %+v", name, err)
		}
	}
	got, _ := normalizeOrderBuyer(orderBuyerJSON{Name: "  ACME  ", TaxCode: " 0123456789 "})
	if got.Name != "ACME" || got.TaxCode != "0123456789" {
		t.Errorf("normalized = %+v, want trimmed values", got)
	}

	bad := map[string]struct {
		in    orderBuyerJSON
		field string
	}{
		"nine digits":           {orderBuyerJSON{TaxCode: "012345678"}, "tax_code"},
		"eleven digits":         {orderBuyerJSON{TaxCode: "01234567890"}, "tax_code"},
		"twelve digits":         {orderBuyerJSON{TaxCode: "012345678901"}, "tax_code"},
		"fourteen digits":       {orderBuyerJSON{TaxCode: "01234567890123"}, "tax_code"},
		"letters":               {orderBuyerJSON{TaxCode: "01234567ab"}, "tax_code"},
		"dash":                  {orderBuyerJSON{TaxCode: "0123456789-001"}, "tax_code"},
		"long name":             {orderBuyerJSON{Name: strings.Repeat("n", maxBuyerNameChars+1)}, "name"},
		"long address":          {orderBuyerJSON{Address: strings.Repeat("a", maxBuyerAddressChars+1)}, "address"},
		"long email":            {orderBuyerJSON{InvoiceEmail: strings.Repeat("a", maxBuyerEmailChars) + "@example.com"}, "invoice_email"},
		"not an email":          {orderBuyerJSON{InvoiceEmail: "ke toan"}, "invoice_email"},
		"named email":           {orderBuyerJSON{InvoiceEmail: "Ke Toan <a@example.com>"}, "invoice_email"},
		"nul in the name":       {orderBuyerJSON{Name: "AC\x00ME"}, "name"},
		"long name, vietnamese": {orderBuyerJSON{Name: strings.Repeat("Đ", maxBuyerNameChars+1)}, "name"},
	}
	for name, tc := range bad {
		if _, err := normalizeOrderBuyer(tc.in); err == nil || err.field != tc.field {
			t.Errorf("%s: err = %+v, want a refusal of %s", name, err, tc.field)
		}
	}
	// A name of exactly the limit, in multi-byte characters, is accepted: the
	// limit counts characters, not bytes.
	if _, err := normalizeOrderBuyer(orderBuyerJSON{Name: strings.Repeat("Đ", maxBuyerNameChars)}); err != nil {
		t.Errorf("a name of %d characters: %+v", maxBuyerNameChars, err)
	}
}

// --- Plans and orders: database-backed, scoped to the workspace.

func billingOrder(t *testing.T, workspaceID, status string, createdAt time.Time) string {
	t.Helper()
	return dbfx.Insert(t, "billing_order", testutil.Cols{
		"id": testutil.Raw("gen_random_uuid()"), "workspace_id": workspaceID, "kind": "subscribe", "plan_key": "pro",
		"interval": "month", "seats": 2, "amount": 298000, "vat_amount": 29800, "currency": "vnd", "status": status,
		"provider": "manual", "transfer_code": "MUL" + strings.ToUpper(uuid.NewString()[:8]),
		"expires_at": createdAt.Add(7 * 24 * time.Hour), "created_at": createdAt,
	})
}

func billingPlan(t *testing.T, public bool, archived bool, month, year *int64) string {
	t.Helper()
	key := "t" + strings.ReplaceAll(uuid.NewString()[:12], "-", "")
	cols := testutil.Cols{
		"key": key, "name": "Test " + key, "currency": "vnd", "is_public": public, "sort": 9000,
		"seat_price_month": month, "seat_price_year": year,
	}
	if archived {
		cols["archived_at"] = testutil.Raw("now()")
	}
	dbfx.InsertNoID(t, "plan", cols, "key = $1", key)
	return key
}

func int64Ptr(v int64) *int64 { return &v }

func billingRequest(method, path string, body any, role string, orderID string) *http.Request {
	req := newRequest(method, path, body)
	req = withURLParams(req, "id", testWorkspaceID, "orderId", orderID)
	return withCloudSubscriptionWorkspace(req, role)
}

func TestWorkspacePlansListsPublicPlansForOwnerAndAdminOnly(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.BillingWorkspaceSubscriptions, true)
	public := billingPlan(t, true, false, int64Ptr(149000), int64Ptr(1490000))
	private := billingPlan(t, false, false, int64Ptr(1), int64Ptr(1))
	archived := billingPlan(t, true, true, int64Ptr(1), int64Ptr(1))

	for _, role := range []string{"member", "viewer"} {
		w := httptest.NewRecorder()
		testHandler.ListWorkspacePlans(w, billingRequest(http.MethodGet, "/api/workspaces/x/plans", nil, role, ""))
		if w.Code != http.StatusForbidden {
			t.Errorf("%s: status = %d, want 403", role, w.Code)
		}
	}
	for _, role := range []string{"owner", "admin"} {
		w := httptest.NewRecorder()
		testHandler.ListWorkspacePlans(w, billingRequest(http.MethodGet, "/api/workspaces/x/plans", nil, role, ""))
		if w.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, body = %s", role, w.Code, w.Body.String())
		}
		var out workspacePlansResponse
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		seen := map[string]workspacePlanJSON{}
		for _, p := range out.Plans {
			seen[p.Key] = p
		}
		if p, ok := seen[public]; !ok || p.SeatPriceMonth == nil || *p.SeatPriceMonth != 149000 || *p.SeatPriceYear != 1490000 || p.Currency != "vnd" {
			t.Errorf("%s: the public plan = %+v, present %v", role, p, ok)
		}
		if _, ok := seen[private]; ok {
			t.Errorf("%s: a private plan is listed", role)
		}
		if _, ok := seen[archived]; ok {
			t.Errorf("%s: an archived plan is listed", role)
		}
	}
}

func TestWorkspaceOrdersAreScopedToTheWorkspaceNewestFirst(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.BillingWorkspaceSubscriptions, true)
	t.Setenv("BILLING_BANK_NAME", "Vietcombank")
	t.Setenv("BILLING_BANK_ACCOUNT_NAME", "CONG TY TNHH MULTICA")
	t.Setenv("BILLING_BANK_ACCOUNT_NUMBER", "0123456789")

	other := dbfx.Workspace(t, "Orders Other", "orders-other-"+uuid.NewString()[:8])
	now := time.Now().UTC()
	older := billingOrder(t, testWorkspaceID, "paid", now.Add(-48*time.Hour))
	newer := billingOrder(t, testWorkspaceID, "pending", now.Add(-time.Hour))
	foreign := billingOrder(t, other, "pending", now)

	w := httptest.NewRecorder()
	testHandler.ListWorkspaceOrders(w, billingRequest(http.MethodGet, "/api/workspaces/x/orders", nil, "admin", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var out workspaceOrdersResponse
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, o := range out.Orders {
		ids = append(ids, o.ID)
		if o.WorkspaceID != testWorkspaceID {
			t.Errorf("order %s belongs to %s", o.ID, o.WorkspaceID)
		}
	}
	posNewer, posOlder := indexOf(ids, newer), indexOf(ids, older)
	if posNewer < 0 || posOlder < 0 || posNewer > posOlder {
		t.Errorf("ids = %v, want %s before %s", ids, newer, older)
	}
	if indexOf(ids, foreign) >= 0 {
		t.Errorf("another workspace's order %s is listed", foreign)
	}
	for _, o := range out.Orders {
		if o.ID != newer {
			continue
		}
		if o.TotalAmount != 327800 || o.Amount != 298000 || o.VATAmount != 29800 || o.Status != "pending" || o.Seats != 2 {
			t.Errorf("order = %+v", o)
		}
		if o.BankDetails.BankName != "Vietcombank" || o.BankDetails.AccountName != "CONG TY TNHH MULTICA" || o.BankDetails.AccountNumber != "0123456789" {
			t.Errorf("bank = %+v", o.BankDetails)
		}
		if !strings.HasPrefix(o.TransferCode, "MUL") || o.ExpiresAt == "" {
			t.Errorf("transfer code %q, expires_at %q", o.TransferCode, o.ExpiresAt)
		}
	}
}

func indexOf(ids []string, id string) int {
	for i, v := range ids {
		if v == id {
			return i
		}
	}
	return -1
}

func TestWorkspaceOrdersRefuseMembersAndAMachineCredential(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.BillingWorkspaceSubscriptions, true)
	order := billingOrder(t, testWorkspaceID, "pending", time.Now().UTC())

	calls := map[string]func(http.ResponseWriter, *http.Request){
		"list": testHandler.ListWorkspaceOrders, "get": testHandler.GetWorkspaceOrder, "buyer": testHandler.UpdateWorkspaceOrderBuyer,
	}
	for name, call := range calls {
		w := httptest.NewRecorder()
		call(w, billingRequest(http.MethodGet, "/api/workspaces/x/orders", map[string]any{}, "member", order))
		if w.Code != http.StatusForbidden {
			t.Errorf("%s as a member: status = %d, want 403", name, w.Code)
		}
		req := billingRequest(http.MethodGet, "/api/workspaces/x/orders", map[string]any{}, "owner", order)
		req.Header.Set("X-Actor-Source", "task_token")
		w = httptest.NewRecorder()
		call(w, req)
		if w.Code != http.StatusForbidden {
			t.Errorf("%s with a task token: status = %d, want 403", name, w.Code)
		}
	}
}

func TestWorkspaceOrdersNeedTheSubscriptionsFlag(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.BillingWorkspaceSubscriptions, false)
	w := httptest.NewRecorder()
	testHandler.ListWorkspaceOrders(w, billingRequest(http.MethodGet, "/api/workspaces/x/orders", nil, "owner", ""))
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestWorkspaceOrderOfAnotherWorkspaceIsNotFound(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.BillingWorkspaceSubscriptions, true)
	other := dbfx.Workspace(t, "Order Foreign", "order-foreign-"+uuid.NewString()[:8])
	foreign := billingOrder(t, other, "pending", time.Now().UTC())

	w := httptest.NewRecorder()
	testHandler.GetWorkspaceOrder(w, billingRequest(http.MethodGet, "/api/workspaces/x/orders/y", nil, "owner", foreign))
	if w.Code != http.StatusNotFound {
		t.Errorf("get: status = %d, want 404", w.Code)
	}
}

func buyerPayload() map[string]any {
	return map[string]any{
		"name": "CONG TY ACME", "tax_code": "0123456789", "address": "1 Le Loi, Ha Noi", "invoice_email": "ke.toan@example.com",
	}
}

func storedBuyer(t *testing.T, orderID string) string {
	t.Helper()
	var buyer string
	dbfx.QueryRow(t, `SELECT buyer::text FROM billing_order WHERE id = $1`, orderID).Scan(&buyer)
	return buyer
}

func TestWorkspaceOrderBuyerWritesAPendingOrderOfTheWorkspace(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.BillingWorkspaceSubscriptions, true)
	order := billingOrder(t, testWorkspaceID, "pending", time.Now().UTC())

	for _, role := range []string{"owner", "admin"} {
		w := httptest.NewRecorder()
		testHandler.UpdateWorkspaceOrderBuyer(w, billingRequest(http.MethodPut, "/api/workspaces/x/orders/y/buyer", buyerPayload(), role, order))
		if w.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, body = %s", role, w.Code, w.Body.String())
		}
	}
	var buyer orderBuyerJSON
	if err := json.Unmarshal([]byte(storedBuyer(t, order)), &buyer); err != nil {
		t.Fatal(err)
	}
	if buyer.Name != "CONG TY ACME" || buyer.TaxCode != "0123456789" || buyer.Address != "1 Le Loi, Ha Noi" || buyer.InvoiceEmail != "ke.toan@example.com" {
		t.Errorf("stored buyer = %+v", buyer)
	}

	// The response is the order with its new buyer.
	w := httptest.NewRecorder()
	testHandler.UpdateWorkspaceOrderBuyer(w, billingRequest(http.MethodPut, "/api/workspaces/x/orders/y/buyer", buyerPayload(), "owner", order))
	var out workspaceOrderJSON
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out.ID != order || out.Buyer.TaxCode != "0123456789" {
		t.Errorf("response = %s (%v)", w.Body.String(), err)
	}
}

func TestWorkspaceOrderBuyerLeavesAnOrderThatIsNotPendingAlone(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.BillingWorkspaceSubscriptions, true)
	for _, status := range []string{"paid", "expired", "canceled", "refunded"} {
		order := billingOrder(t, testWorkspaceID, status, time.Now().UTC())
		before := storedBuyer(t, order)
		w := httptest.NewRecorder()
		testHandler.UpdateWorkspaceOrderBuyer(w, billingRequest(http.MethodPut, "/api/workspaces/x/orders/y/buyer", buyerPayload(), "owner", order))
		if w.Code != http.StatusConflict {
			t.Fatalf("%s: status = %d, body = %s", status, w.Code, w.Body.String())
		}
		var out map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out["code"] != "order_not_pending" || out["status"] != status {
			t.Errorf("%s: body = %s", status, w.Body.String())
		}
		if after := storedBuyer(t, order); after != before {
			t.Errorf("%s: the buyer changed from %s to %s", status, before, after)
		}
	}
}

func TestWorkspaceOrderBuyerDoesNotWriteAnotherWorkspacesOrder(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.BillingWorkspaceSubscriptions, true)
	other := dbfx.Workspace(t, "Buyer Foreign", "buyer-foreign-"+uuid.NewString()[:8])
	foreign := billingOrder(t, other, "pending", time.Now().UTC())
	before := storedBuyer(t, foreign)

	w := httptest.NewRecorder()
	testHandler.UpdateWorkspaceOrderBuyer(w, billingRequest(http.MethodPut, "/api/workspaces/x/orders/y/buyer", buyerPayload(), "owner", foreign))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if after := storedBuyer(t, foreign); after != before {
		t.Errorf("another workspace's buyer changed from %s to %s", before, after)
	}

	w = httptest.NewRecorder()
	testHandler.UpdateWorkspaceOrderBuyer(w, billingRequest(http.MethodPut, "/api/workspaces/x/orders/y/buyer", buyerPayload(), "owner", uuid.NewString()))
	if w.Code != http.StatusNotFound {
		t.Errorf("unknown order: status = %d", w.Code)
	}
	w = httptest.NewRecorder()
	testHandler.UpdateWorkspaceOrderBuyer(w, billingRequest(http.MethodPut, "/api/workspaces/x/orders/y/buyer", buyerPayload(), "owner", "not-a-uuid"))
	if w.Code != http.StatusBadRequest {
		t.Errorf("malformed order id: status = %d", w.Code)
	}
}

func TestWorkspaceOrderBuyerValidatesTheTaxCode(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.BillingWorkspaceSubscriptions, true)
	order := billingOrder(t, testWorkspaceID, "pending", time.Now().UTC())
	put := func(taxCode string) *httptest.ResponseRecorder {
		body := buyerPayload()
		body["tax_code"] = taxCode
		w := httptest.NewRecorder()
		testHandler.UpdateWorkspaceOrderBuyer(w, billingRequest(http.MethodPut, "/api/workspaces/x/orders/y/buyer", body, "owner", order))
		return w
	}
	for _, taxCode := range []string{"012345678", "01234567890", "abcdefghij"} {
		w := put(taxCode)
		if w.Code != http.StatusBadRequest {
			t.Errorf("tax code %q: status = %d, want 400", taxCode, w.Code)
		}
		var out map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out["code"] != "buyer_invalid" || out["field"] != "tax_code" {
			t.Errorf("tax code %q: body = %s", taxCode, w.Body.String())
		}
	}
	for _, taxCode := range []string{"0123456789", "0123456789012", ""} {
		if w := put(taxCode); w.Code != http.StatusOK {
			t.Errorf("tax code %q: status = %d, body = %s", taxCode, w.Code, w.Body.String())
		}
	}
}

// A plan's scheduled price change reaches the page only on the plan the
// workspace is on, and only once its notice is sent (spec 7b.4 rule 11).
func TestListWorkspacePlansShowsScheduledPriceChangeAfterNoticeOnly(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.BillingWorkspaceSubscriptions, true)
	effective := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Second)

	for name, tc := range map[string]struct {
		onOwnPlan bool // the change is on the plan in use, not on another plan
		noticed   bool
		want      bool
	}{
		"plan in use, notice sent":     {onOwnPlan: true, noticed: true, want: true},
		"plan in use, notice not sent": {onOwnPlan: true, noticed: false, want: false},
		"another plan, notice sent":    {onOwnPlan: false, noticed: true, want: false},
	} {
		t.Run(name, func(t *testing.T) {
			inUse := billingPlan(t, true, false, int64Ptr(149000), int64Ptr(1490000))
			other := billingPlan(t, true, false, int64Ptr(249000), int64Ptr(2490000))
			dbfx.Exec(t, `INSERT INTO workspace_subscription (workspace_id, plan_key, status) VALUES ($1, $2, 'active')`, testWorkspaceID, inUse)
			dbfx.Cleanup(t, `DELETE FROM workspace_subscription WHERE workspace_id = $1`, testWorkspaceID)

			changed := other
			if tc.onOwnPlan {
				changed = inUse
			}
			cols := testutil.Cols{
				"plan_key": changed, "effective_from": effective, "reason": "test",
				"new": testutil.Raw(`'{"seat_price_month": 179000}'::jsonb`),
			}
			if tc.noticed {
				cols["notice_sent_at"] = testutil.Raw("now()")
			}
			dbfx.Insert(t, "plan_price_change", cols)

			w := httptest.NewRecorder()
			testHandler.ListWorkspacePlans(w, billingRequest(http.MethodGet, "/api/workspaces/x/plans", nil, "owner", ""))
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
			}
			var raw struct {
				Plans []map[string]json.RawMessage `json:"plans"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
				t.Fatal(err)
			}
			byKey := map[string]map[string]json.RawMessage{}
			for _, p := range raw.Plans {
				var key string
				_ = json.Unmarshal(p["key"], &key)
				byKey[key] = p
			}
			for _, key := range []string{inUse, other} {
				entry, ok := byKey[key]
				if !ok {
					t.Fatalf("plan %s is not listed", key)
				}
				field, present := entry["scheduled_price_change"]
				if !present {
					t.Fatalf("plan %s has no scheduled_price_change field; it must be an object or null", key)
				}
				if !(tc.want && key == inUse) {
					if string(field) != "null" {
						t.Errorf("plan %s: scheduled_price_change = %s, want null", key, field)
					}
					continue
				}
				var got struct {
					EffectiveFrom  string `json:"effective_from"`
					SeatPriceMonth *int64 `json:"seat_price_month"`
					SeatPriceYear  *int64 `json:"seat_price_year"`
				}
				if err := json.Unmarshal(field, &got); err != nil {
					t.Fatalf("plan %s: scheduled_price_change = %s: %v", key, field, err)
				}
				if got.EffectiveFrom != effective.Format(time.RFC3339) || got.SeatPriceMonth == nil || *got.SeatPriceMonth != 179000 ||
					got.SeatPriceYear == nil || *got.SeatPriceYear != 1490000 {
					t.Errorf("plan %s: scheduled_price_change = %s, want %s, 179000 a month, the current 1490000 a year",
						key, field, effective.Format(time.RFC3339))
				}
			}
		})
	}
}
