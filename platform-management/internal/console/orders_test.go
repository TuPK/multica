package console

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
)

const (
	orderA = "eeeeeeee-1111-0000-0000-000000000001"
	orderB = "eeeeeeee-1111-0000-0000-000000000002"
	orderC = "eeeeeeee-1111-0000-0000-000000000003"
)

// seedOrder stores an order of the test workspace. It expires a day after t0
// unless the caller changes it.
func seedOrder(e *env, id string, status billing.Status, createdAt time.Time) billing.Order {
	e.t.Helper()
	start, end := t0.Add(-time.Hour), t0.AddDate(0, 1, 0)
	o := billing.Order{
		ID: id, WorkspaceID: wsID, Kind: billing.KindSubscribe, PlanKey: "pro", Interval: "month", Seats: 3,
		Amount: 600000, VATAmount: 60000, Currency: "vnd", Status: status, Provider: billing.ProviderManual,
		TransferCode: "MUL" + strings.ToUpper(id[len(id)-8:]), PeriodStart: &start, PeriodEnd: &end,
		Buyer:     json.RawMessage(`{"name":"Acme Co","tax_code":"0101","address":"1 Main St","invoice_email":"ap@acme.test"}`),
		ExpiresAt: t0.Add(24 * time.Hour), CreatedAt: createdAt,
	}
	e.store.orders[id] = o
	return o
}

func orderIDs(t *testing.T, rec *httptest.ResponseRecorder) []string {
	t.Helper()
	var body struct {
		Orders []struct {
			ID string `json:"id"`
		} `json:"orders"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %s: %v", rec.Body, err)
	}
	ids := []string{}
	for _, o := range body.Orders {
		ids = append(ids, o.ID)
	}
	return ids
}

func errCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %s: %v", rec.Body, err)
	}
	code, _ := body["code"].(string)
	return code
}

func wantError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status || errCode(t, rec) != code {
		t.Fatalf("got %d %s, want %d %s", rec.Code, rec.Body, status, code)
	}
}

func auditNames(e *env) []string {
	out := []string{}
	for _, a := range e.store.audits {
		out = append(out, a.Action)
	}
	return out
}

// 1. support gets 404 on the order routes; billing and admin get through.
func TestPlatformBillingOrderRoutesAdmitBillingAndAdminOnly(t *testing.T) {
	routes := []struct{ method, path, body string }{
		{"GET", "/api/platform/orders", ""},
		{"POST", "/api/platform/orders/" + orderA + "/confirm", `{"reason":"r"}`},
		{"POST", "/api/platform/orders/" + orderA + "/cancel", `{"reason":"r"}`},
	}
	for _, rt := range routes {
		e := newEnv(t)
		seedOrder(e, orderA, billing.StatusPending, t0)

		rec := e.do(suppID, rt.method, rt.path, rt.body)
		if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "order") {
			t.Errorf("support %s %s = %d %s, want the 404 of an unknown route", rt.method, rt.path, rec.Code, rec.Body)
		}
		if got := e.do("", rt.method, rt.path, rt.body); got.Code != http.StatusNotFound {
			t.Errorf("anonymous %s %s = %d, want 404", rt.method, rt.path, got.Code)
		}
		if e.store.orders[orderA].Status != billing.StatusPending || len(e.store.audits) != 0 {
			t.Errorf("support %s %s changed something", rt.method, rt.path)
		}
		for _, who := range []string{billID, adminID} {
			e := newEnv(t)
			seedOrder(e, orderA, billing.StatusPending, t0)
			if rec := e.do(who, rt.method, rt.path, rt.body); rec.Code != http.StatusOK {
				t.Errorf("%s %s %s = %d %s, want 200", who, rt.method, rt.path, rec.Code, rec.Body)
			}
		}
	}
}

func TestPlatformBillingOrderWritesNeedTheConsoleOrigin(t *testing.T) {
	e := newEnv(t)
	seedOrder(e, orderA, billing.StatusPending, t0)
	for _, action := range []string{"confirm", "cancel"} {
		rec := e.doWith(billID, "POST", "/api/platform/orders/"+orderA+"/"+action, `{"reason":"r"}`, "https://evil.example")
		if rec.Code == http.StatusOK {
			t.Errorf("%s with a foreign origin = 200", action)
		}
	}
	if e.store.orders[orderA].Status != billing.StatusPending {
		t.Fatal("a cross-origin request changed the order")
	}
}

// 2. billing confirms a pending order.
func TestPlatformBillingConfirmPaysTheOrderAndAuditsTheStaffEmail(t *testing.T) {
	e := newEnv(t)
	seedOrder(e, orderA, billing.StatusPending, t0)
	e.store.subs[wsID] = subscription.Subscription{WorkspaceID: wsID, PlanKey: "pro", Interval: "month", Seats: 2, Status: subscription.StatusActive, Version: 4}

	rec := e.do(billID, "POST", "/api/platform/orders/"+orderA+"/confirm", `{"reason":"matched transfer MULAAAA","transfer_reference":"FT123"}`)
	wantStatus(t, rec, 200)

	var body struct {
		Order   map[string]any `json:"order"`
		Applied bool           `json:"applied"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Applied || body.Order["status"] != "paid" || body.Order["id"] != orderA || body.Order["workspace_name"] != wsName {
		t.Errorf("body = %s", rec.Body)
	}
	if body.Order["paid_at"] == nil {
		t.Errorf("paid_at is null: %s", rec.Body)
	}
	if got := e.store.orders[orderA]; got.Status != billing.StatusPaid || got.ConfirmedBy != billID {
		t.Errorf("order = %+v, want paid and confirmed by the caller", got)
	}
	if got := e.store.subs[wsID].Version; got != 5 {
		t.Errorf("subscription version = %d, want 5", got)
	}
	if len(e.store.audits) != 1 {
		t.Fatalf("audit rows = %v, want exactly billing.order_paid", auditNames(e))
	}
	a := e.store.audits[0]
	if a.Action != audit.ActionBillingOrderPaid || a.ActorType != audit.ActorStaff || a.ActorID != billID || a.ActorLabel != billMail || a.TargetID != orderA {
		t.Errorf("audit entry = %+v", a)
	}
}

// 3. confirming twice applies once.
func TestPlatformBillingConfirmTwiceAppliesOnce(t *testing.T) {
	e := newEnv(t)
	seedOrder(e, orderA, billing.StatusPending, t0)
	path := "/api/platform/orders/" + orderA + "/confirm"
	wantStatus(t, e.do(billID, "POST", path, `{"reason":"first"}`), 200)
	versionAfterFirst := e.store.subs[wsID].Version
	audits := len(e.store.audits)

	rec := e.do(adminID, "POST", path, `{"reason":"again"}`)
	wantStatus(t, rec, 200)
	var body struct {
		Order   map[string]any `json:"order"`
		Applied bool           `json:"applied"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Applied || body.Order["status"] != "paid" {
		t.Errorf("second confirm body = %s, want applied false on the paid order", rec.Body)
	}
	if e.store.subs[wsID].Version != versionAfterFirst || len(e.store.audits) != audits {
		t.Errorf("the second confirm wrote: version %d, audits %v", e.store.subs[wsID].Version, auditNames(e))
	}
	if e.store.orders[orderA].ConfirmedBy != billID {
		t.Errorf("confirmed_by = %s, want the first confirmer", e.store.orders[orderA].ConfirmedBy)
	}
}

// 4. expired and canceled orders cannot be confirmed.
func TestPlatformBillingConfirmRefusesExpiredAndCanceledOrders(t *testing.T) {
	e := newEnv(t)
	path := func(id string) string { return "/api/platform/orders/" + id + "/confirm" }
	const reason = `{"reason":"r"}`

	late := seedOrder(e, orderA, billing.StatusPending, t0)
	late.ExpiresAt = t0.Add(-time.Second)
	e.store.orders[orderA] = late
	wantError(t, e.do(billID, "POST", path(orderA), reason), 409, "order_expired")

	seedOrder(e, orderB, billing.StatusExpired, t0)
	wantError(t, e.do(billID, "POST", path(orderB), reason), 409, "order_expired")

	seedOrder(e, orderC, billing.StatusCanceled, t0)
	wantError(t, e.do(billID, "POST", path(orderC), reason), 409, "order_not_pending")

	refunded := seedOrder(e, "eeeeeeee-1111-0000-0000-000000000004", billing.StatusRefunded, t0)
	wantError(t, e.do(billID, "POST", path(refunded.ID), reason), 409, "order_not_pending")

	if len(e.store.audits) != 0 || len(e.store.subs) != 0 {
		t.Errorf("a refused confirm wrote: audits %v, subscriptions %v", auditNames(e), e.store.subs)
	}
}

func TestPlatformBillingUnknownOrderIs404OrderNotFound(t *testing.T) {
	e := newEnv(t)
	for _, action := range []string{"confirm", "cancel"} {
		wantError(t, e.do(billID, "POST", "/api/platform/orders/"+orderA+"/"+action, `{"reason":"r"}`), 404, "order_not_found")
		wantError(t, e.do(billID, "POST", "/api/platform/orders/not-a-uuid/"+action, `{"reason":"r"}`), 404, "order_not_found")
	}
}

// 5. cancel.
func TestPlatformBillingCancelMovesAPendingOrderToCanceledAndAudits(t *testing.T) {
	e := newEnv(t)
	seedOrder(e, orderA, billing.StatusPending, t0)

	rec := e.do(billID, "POST", "/api/platform/orders/"+orderA+"/cancel", `{"reason":"customer asked"}`)
	wantStatus(t, rec, 200)
	var body struct {
		Order   map[string]any `json:"order"`
		Applied bool           `json:"applied"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Applied || body.Order["status"] != "canceled" || body.Order["workspace_name"] != wsName {
		t.Errorf("body = %s", rec.Body)
	}
	if e.store.orders[orderA].Status != billing.StatusCanceled {
		t.Errorf("stored status = %s", e.store.orders[orderA].Status)
	}
	if len(e.store.audits) != 1 {
		t.Fatalf("audit rows = %v", auditNames(e))
	}
	a := e.store.audits[0]
	if a.Action != audit.ActionBillingOrderCanceled || a.ActorType != audit.ActorStaff || a.ActorID != billID ||
		a.ActorLabel != billMail || a.TargetType != "billing_order" || a.TargetID != orderA || a.WorkspaceID != wsID {
		t.Errorf("audit entry = %+v", a)
	}
	if a.Detail["reason"] != "customer asked" {
		t.Errorf("audit detail = %v, want the reason", a.Detail)
	}
	if _, ok := e.store.subs[wsID]; ok {
		t.Error("cancel touched the subscription")
	}
}

func TestPlatformBillingCancelRefusesAnOrderThatIsNotPending(t *testing.T) {
	for _, status := range []billing.Status{billing.StatusPaid, billing.StatusExpired, billing.StatusCanceled, billing.StatusRefunded} {
		e := newEnv(t)
		seedOrder(e, orderA, status, t0)
		rec := e.do(billID, "POST", "/api/platform/orders/"+orderA+"/cancel", `{"reason":"r"}`)
		wantError(t, rec, 409, "order_not_pending")
		if e.store.orders[orderA].Status != status || len(e.store.audits) != 0 {
			t.Errorf("%s: a refused cancel changed something", status)
		}
	}
}

func TestPlatformBillingFailedCancelAuditRollsTheStatusBack(t *testing.T) {
	e := newEnv(t)
	seedOrder(e, orderA, billing.StatusPending, t0)
	e.store.failAudit = fmt.Errorf("disk full")
	rec := e.do(billID, "POST", "/api/platform/orders/"+orderA+"/cancel", `{"reason":"r"}`)
	wantError(t, rec, 500, "internal")
	if e.store.orders[orderA].Status != billing.StatusPending {
		t.Fatal("the order was canceled without its audit row")
	}
	if strings.Contains(rec.Body.String(), "disk full") {
		t.Error("the response leaks the internal error")
	}
}

// 6. a reason is required.
func TestPlatformBillingConfirmAndCancelNeedAReason(t *testing.T) {
	bodies := []string{`{}`, `{"reason":""}`, `{"reason":"   "}`}
	for _, action := range []string{"confirm", "cancel"} {
		for _, body := range bodies {
			e := newEnv(t)
			seedOrder(e, orderA, billing.StatusPending, t0)
			rec := e.do(billID, "POST", "/api/platform/orders/"+orderA+"/"+action, body)
			wantError(t, rec, 400, "reason_required")
			if e.store.orders[orderA].Status != billing.StatusPending || len(e.store.audits) != 0 {
				t.Errorf("%s %s: wrote something", action, body)
			}
		}
	}
}

func TestPlatformBillingReasonAndReferenceAreBounded(t *testing.T) {
	e := newEnv(t)
	seedOrder(e, orderA, billing.StatusPending, t0)
	path := "/api/platform/orders/" + orderA + "/confirm"

	wantError(t, e.do(billID, "POST", path, `{"reason":"`+strings.Repeat("x", 501)+`"}`), 400, "reason_too_long")
	wantError(t, e.do(billID, "POST", path, `{"reason":"ok","transfer_reference":"`+strings.Repeat("x", 101)+`"}`), 400, "transfer_reference_too_long")
	wantError(t, e.do(billID, "POST", path, `{"reason":"ok","extra":1}`), 400, "invalid_body")
	if e.store.orders[orderA].Status != billing.StatusPending {
		t.Fatal("a refused confirm paid the order")
	}
	wantStatus(t, e.do(billID, "POST", path, `{"reason":"`+strings.Repeat("x", 500)+`"}`), 200)
}

// 7. the list.
func TestPlatformBillingListDefaultsToPendingOldestFirst(t *testing.T) {
	e := newEnv(t)
	seedOrder(e, orderB, billing.StatusPending, t0.Add(2*time.Hour))
	seedOrder(e, orderA, billing.StatusPending, t0.Add(time.Hour))
	seedOrder(e, orderC, billing.StatusPaid, t0)

	rec := e.do(billID, "GET", "/api/platform/orders", "")
	wantStatus(t, rec, 200)
	if got := orderIDs(t, rec); strings.Join(got, ",") != orderA+","+orderB {
		t.Errorf("ids = %v, want the pending orders, oldest first", got)
	}
	if e.reader.lastOrderStatus != billing.StatusPending || e.reader.lastNewestFirst || e.reader.lastOrderLimit != 200 {
		t.Errorf("reader asked %q newest=%v limit=%d", e.reader.lastOrderStatus, e.reader.lastNewestFirst, e.reader.lastOrderLimit)
	}
}

func TestPlatformBillingListOfOtherStatusesIsNewestFirst(t *testing.T) {
	for _, status := range []billing.Status{billing.StatusPaid, billing.StatusExpired, billing.StatusCanceled, billing.StatusRefunded} {
		e := newEnv(t)
		seedOrder(e, orderA, status, t0.Add(time.Hour))
		seedOrder(e, orderB, status, t0.Add(2*time.Hour))
		seedOrder(e, orderC, billing.StatusPending, t0.Add(3*time.Hour))

		rec := e.do(adminID, "GET", "/api/platform/orders?status="+string(status), "")
		wantStatus(t, rec, 200)
		if got := orderIDs(t, rec); strings.Join(got, ",") != orderB+","+orderA {
			t.Errorf("%s: ids = %v, want newest first", status, got)
		}
		if !e.reader.lastNewestFirst {
			t.Errorf("%s: the reader was asked for oldest first", status)
		}
	}
}

func TestPlatformBillingListWithAnUnknownStatusIs400(t *testing.T) {
	e := newEnv(t)
	for _, q := range []string{"?status=nope", "?status=", "?status=PAID", "?status=pending&status=paid"} {
		rec := e.do(billID, "GET", "/api/platform/orders"+q, "")
		if q == "?status=" {
			// An empty value means no filter, like an absent one.
			wantStatus(t, rec, 200)
			continue
		}
		wantError(t, rec, 400, "invalid_status")
	}
}

func TestPlatformBillingOrderJSONShape(t *testing.T) {
	e := newEnv(t)
	seedOrder(e, orderA, billing.StatusPending, t0)
	rec := e.do(billID, "GET", "/api/platform/orders", "")
	wantStatus(t, rec, 200)

	var body struct {
		Orders []map[string]any `json:"orders"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Orders) != 1 {
		t.Fatalf("body %s: %v", rec.Body, err)
	}
	o := body.Orders[0]
	want := map[string]any{
		"id": orderA, "workspace_id": wsID, "workspace_name": wsName, "kind": "subscribe", "plan_key": "pro",
		"interval": "month", "seats": float64(3), "amount": float64(600000), "vat_amount": float64(60000),
		"currency": "vnd", "status": "pending", "transfer_code": "MUL00000001",
		"expires_at": t0.Add(24 * time.Hour).UTC().Format(time.RFC3339Nano), "created_at": t0.UTC().Format(time.RFC3339Nano),
		"paid_at": nil,
	}
	for k, v := range want {
		if got, ok := o[k]; !ok || got != v {
			t.Errorf("%s = %v (present %v), want %v", k, got, ok, v)
		}
	}
	buyer, _ := o["buyer"].(map[string]any)
	if buyer["name"] != "Acme Co" || buyer["tax_code"] != "0101" || buyer["address"] != "1 Main St" || buyer["invoice_email"] != "ap@acme.test" {
		t.Errorf("buyer = %v", o["buyer"])
	}
	for _, k := range []string{"provider", "confirmed_by", "created_by", "idempotency_key", "provider_ref"} {
		if _, ok := o[k]; ok {
			t.Errorf("order JSON has the field %q the contract does not list", k)
		}
	}
}

func TestPlatformBillingOrderJSONToleratesAnEmptyBuyer(t *testing.T) {
	e := newEnv(t)
	o := seedOrder(e, orderA, billing.StatusPending, t0)
	o.Buyer = nil
	e.store.orders[orderA] = o
	rec := e.do(billID, "GET", "/api/platform/orders", "")
	wantStatus(t, rec, 200)
	if !strings.Contains(rec.Body.String(), `"buyer":{"name":"","tax_code":"","address":"","invoice_email":""}`) {
		t.Errorf("body = %s", rec.Body)
	}
}

// Tenant detail and overview.
func TestPlatformBillingTenantDetailReturnsTheRecentOrders(t *testing.T) {
	e := newEnv(t)
	seedOrder(e, orderA, billing.StatusPaid, t0.Add(time.Hour))
	seedOrder(e, orderB, billing.StatusPending, t0.Add(2*time.Hour))

	// Every staff role reads it.
	for _, who := range []string{suppID, billID, adminID} {
		rec := e.do(who, "GET", "/api/platform/tenants/"+wsID, "")
		wantStatus(t, rec, 200)
		var body struct {
			RecentOrders []map[string]any `json:"recent_orders"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if len(body.RecentOrders) != 2 || body.RecentOrders[0]["id"] != orderB || body.RecentOrders[1]["id"] != orderA {
			t.Errorf("%s: recent_orders = %v, want the two orders, newest first", who, body.RecentOrders)
		}
	}
	if e.reader.lastRecentWS != wsID || e.reader.lastRecentLimit != 20 {
		t.Errorf("reader asked for %s limit %d, want %s limit 20", e.reader.lastRecentWS, e.reader.lastRecentLimit, wsID)
	}
}

func TestPlatformBillingTenantDetailWithNoOrdersHasAnEmptyList(t *testing.T) {
	e := newEnv(t)
	rec := e.do(suppID, "GET", "/api/platform/tenants/"+wsID, "")
	wantStatus(t, rec, 200)
	if !strings.Contains(rec.Body.String(), `"recent_orders":[]`) {
		t.Errorf("body = %s", rec.Body)
	}
}

func TestPlatformBillingOverviewCountsPendingOrders(t *testing.T) {
	e := newEnv(t)
	e.reader.overview = Overview{WorkspacesTotal: 3, PendingOrders: 7}
	for _, who := range []string{suppID, billID, adminID} {
		rec := e.do(who, "GET", "/api/platform/overview", "")
		wantStatus(t, rec, 200)
		if got := decodeJSON(t, rec)["pending_orders"]; got != float64(7) {
			t.Errorf("%s: pending_orders = %v", who, got)
		}
	}
}

func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body %s: %v", rec.Body, err)
	}
	return m
}
