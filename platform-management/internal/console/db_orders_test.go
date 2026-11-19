package console

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/staff"
)

// Database tests for the order routes. They skip unless
// PLATFORM_TEST_DATABASE_URL names a *_test database and DATABASE_URL equals
// it. The database is shared by packages that run in parallel, so each test
// asserts on the rows of its own workspace and orders.

type orderFixture struct {
	*dbFixture
	ws string
}

func newOrderFixture(t *testing.T) *orderFixture {
	t.Helper()
	f := newDBFixture(t)
	owner, _ := f.user("owner")
	member, _ := f.user("member")
	o := &orderFixture{dbFixture: f, ws: f.workspace(owner, member)}
	t.Cleanup(func() {
		ctx := context.Background()
		f.pool.Exec(ctx, `DELETE FROM billing_order WHERE workspace_id = $1`, o.ws)
		f.pool.Exec(ctx, `DELETE FROM workspace_subscription WHERE workspace_id = $1`, o.ws)
	})
	return o
}

func (o *orderFixture) seedSubscription(version int64) {
	o.t.Helper()
	if _, err := o.pool.Exec(context.Background(),
		`INSERT INTO workspace_subscription (workspace_id, plan_key, status, seats, interval, version)
		 VALUES ($1, 'free', 'active', 3, 'month', $2)`, o.ws, version); err != nil {
		o.t.Fatalf("seed subscription: %v", err)
	}
}

func (o *orderFixture) subscriptionVersion() int64 {
	o.t.Helper()
	var v int64
	if err := o.pool.QueryRow(context.Background(),
		`SELECT version FROM workspace_subscription WHERE workspace_id = $1`, o.ws).Scan(&v); err != nil {
		o.t.Fatal(err)
	}
	return v
}

// seedOrder inserts an order of the fixture's workspace and returns its id.
// expiresIn is relative to now and may be negative.
func (o *orderFixture) seedOrder(status string, createdAgo, expiresIn time.Duration) string {
	o.t.Helper()
	var id string
	err := o.pool.QueryRow(context.Background(), `
		INSERT INTO billing_order (id, workspace_id, kind, plan_key, interval, seats, amount, vat_amount, currency,
		    status, provider, transfer_code, buyer, period_start, period_end, expires_at, created_at)
		VALUES (gen_random_uuid(), $1, 'subscribe', 'pro', 'month', 3, 600000, 60000, 'vnd',
		    $2, 'manual', 'MUL' || upper(substr(md5(random()::text), 1, 8)),
		    '{"name":"Acme","tax_code":"0101","address":"1 Main St","invoice_email":"ap@acme.test"}',
		    now(), now() + interval '1 month', now() + make_interval(secs => $3::float8), now() - make_interval(secs => $4::float8))
		RETURNING id::text`,
		o.ws, status, expiresIn.Seconds(), createdAgo.Seconds()).Scan(&id)
	if err != nil {
		o.t.Fatalf("seed order: %v", err)
	}
	return id
}

func (o *orderFixture) orderStatus(id string) string {
	o.t.Helper()
	var s string
	if err := o.pool.QueryRow(context.Background(), `SELECT status FROM billing_order WHERE id = $1`, id).Scan(&s); err != nil {
		o.t.Fatal(err)
	}
	return s
}

func (o *orderFixture) auditCount(action, orderID string) int {
	o.t.Helper()
	return o.count(`SELECT count(*) FROM audit_event WHERE action = $1 AND target_id = $2`, action, orderID)
}

func indexOf(ids []string, id string) int {
	for i, v := range ids {
		if v == id {
			return i
		}
	}
	return -1
}

func TestDBOrdersConfirmPaysTheOrderAndAuditsTheStaffEmail(t *testing.T) {
	f := newOrderFixture(t)
	billID, billEmail := f.staffUser(staff.RoleBilling)
	f.seedSubscription(4)
	id := f.seedOrder("pending", time.Hour, 24*time.Hour)

	rec := f.do(billID, "POST", "/api/platform/orders/"+id+"/confirm", `{"reason":"matched transfer"}`)
	wantStatus(t, rec, 200)
	if got := decodeJSON(t, rec); got["applied"] != true {
		t.Fatalf("body = %s", rec.Body)
	}
	if f.orderStatus(id) != "paid" || f.subscriptionVersion() != 5 {
		t.Errorf("status %s, version %d, want paid and 5", f.orderStatus(id), f.subscriptionVersion())
	}
	rows := f.staffRows("billing.order_paid", billID)
	if len(rows) != 1 || rows[0].actorLabel != billEmail || rows[0].targetID != id || rows[0].workspaceID != f.ws {
		t.Errorf("audit rows = %+v", rows)
	}

	again := f.do(billID, "POST", "/api/platform/orders/"+id+"/confirm", `{"reason":"again"}`)
	wantStatus(t, again, 200)
	if got := decodeJSON(t, again); got["applied"] != false {
		t.Errorf("second confirm body = %s", again.Body)
	}
	if f.subscriptionVersion() != 5 || f.auditCount("billing.order_paid", id) != 1 {
		t.Errorf("the second confirm wrote: version %d, paid rows %d", f.subscriptionVersion(), f.auditCount("billing.order_paid", id))
	}
}

func TestDBOrdersConfirmRefusesExpiredAndCanceledOrders(t *testing.T) {
	f := newOrderFixture(t)
	billID, _ := f.staffUser(staff.RoleBilling)
	late := f.seedOrder("pending", 2*time.Hour, -time.Minute)
	closed := f.seedOrder("expired", 2*time.Hour, -time.Minute)
	canceled := f.seedOrder("canceled", 2*time.Hour, time.Hour)

	for id, code := range map[string]string{late: "order_expired", closed: "order_expired", canceled: "order_not_pending"} {
		wantError(t, f.do(billID, "POST", "/api/platform/orders/"+id+"/confirm", `{"reason":"r"}`), 409, code)
	}
	if f.orderStatus(late) != "pending" || f.auditCount("billing.order_paid", late) != 0 {
		t.Error("a refused confirm changed the order")
	}
}

func TestDBOrdersCancelWritesTheAuditRowWithTheReason(t *testing.T) {
	f := newOrderFixture(t)
	billID, billEmail := f.staffUser(staff.RoleBilling)
	pending := f.seedOrder("pending", time.Hour, 24*time.Hour)
	paid := f.seedOrder("paid", time.Hour, 24*time.Hour)

	wantStatus(t, f.do(billID, "POST", "/api/platform/orders/"+pending+"/cancel", `{"reason":"customer asked"}`), 200)
	if f.orderStatus(pending) != "canceled" {
		t.Errorf("status = %s", f.orderStatus(pending))
	}
	rows := f.staffRows("billing.order_canceled", billID)
	if len(rows) != 1 || rows[0].actorLabel != billEmail || rows[0].targetID != pending || !strings.Contains(rows[0].detail, "customer asked") {
		t.Errorf("audit rows = %+v", rows)
	}
	wantError(t, f.do(billID, "POST", "/api/platform/orders/"+paid+"/cancel", `{"reason":"r"}`), 409, "order_not_pending")
	wantError(t, f.do(billID, "POST", "/api/platform/orders/"+pending+"/cancel", `{"reason":"r"}`), 409, "order_not_pending")
	wantError(t, f.do(billID, "POST", "/api/platform/orders/aaaaaaaa-0000-0000-0000-00000000dead/cancel", `{"reason":"r"}`), 404, "order_not_found")
}

func TestDBOrdersListOrderingTenantDetailAndOverview(t *testing.T) {
	f := newOrderFixture(t)
	billID, _ := f.staffUser(staff.RoleBilling)
	supportID, _ := f.staffUser(staff.RoleSupport)
	oldPending := f.seedOrder("pending", 3*time.Hour, 24*time.Hour)
	newPending := f.seedOrder("pending", time.Hour, 24*time.Hour)
	oldPaid := f.seedOrder("paid", 3*time.Hour, 24*time.Hour)
	newPaid := f.seedOrder("paid", time.Hour, 24*time.Hour)

	pending := orderIDs(t, f.do(billID, "GET", "/api/platform/orders", ""))
	if a, b := indexOf(pending, oldPending), indexOf(pending, newPending); a < 0 || b < 0 || a > b {
		t.Errorf("pending ids = %v, want the older order before the newer", pending)
	}
	paid := orderIDs(t, f.do(billID, "GET", "/api/platform/orders?status=paid", ""))
	if a, b := indexOf(paid, newPaid), indexOf(paid, oldPaid); a < 0 || b < 0 || a > b {
		t.Errorf("paid ids = %v, want the newer order before the older", paid)
	}
	wantError(t, f.do(billID, "GET", "/api/platform/orders?status=bogus", ""), 400, "invalid_status")
	if rec := f.do(supportID, "GET", "/api/platform/orders", ""); rec.Code != http.StatusNotFound {
		t.Errorf("support GET /orders = %d, want 404", rec.Code)
	}

	rec := f.do(supportID, "GET", "/api/platform/workspaces/"+f.ws, "")
	wantStatus(t, rec, 200)
	var detail struct {
		RecentOrders []struct {
			ID            string `json:"id"`
			WorkspaceName string `json:"workspace_name"`
		} `json:"recent_orders"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if len(detail.RecentOrders) != 4 {
		t.Fatalf("recent_orders = %+v, want the four orders of this workspace", detail.RecentOrders)
	}
	newest := map[string]bool{newPaid: true, newPending: true}
	oldest := map[string]bool{oldPaid: true, oldPending: true}
	for i, o := range detail.RecentOrders {
		if (i < 2 && !newest[o.ID]) || (i >= 2 && !oldest[o.ID]) || o.WorkspaceName == "" {
			t.Errorf("recent_orders[%d] = %+v, want newest first with the workspace name", i, o)
		}
	}

	ov := decodeJSON(t, f.do(supportID, "GET", "/api/platform/overview", ""))
	if n, _ := ov["pending_orders"].(float64); n < 2 {
		t.Errorf("pending_orders = %v, want at least the 2 this test made", ov["pending_orders"])
	}
}

// Two staff members confirm the same order at once: the row lock makes exactly
// one of them apply it.
func TestDBOrdersTwoConcurrentConfirmsApplyOnce(t *testing.T) {
	f := newOrderFixture(t)
	aID, _ := f.staffUser(staff.RoleBilling)
	bID, _ := f.staffUser(staff.RoleSystemAdmin)
	f.seedSubscription(1)
	id := f.seedOrder("pending", time.Hour, 24*time.Hour)

	var wg sync.WaitGroup
	recs := make([]*httptest.ResponseRecorder, 2)
	start := make(chan struct{})
	for i, who := range []string{aID, bID} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			recs[i] = f.do(who, "POST", "/api/platform/orders/"+id+"/confirm", `{"reason":"matched"}`)
		}()
	}
	close(start)
	wg.Wait()

	applied := 0
	for _, rec := range recs {
		if rec.Code != 200 {
			t.Fatalf("status %d, body %s", rec.Code, rec.Body)
		}
		if decodeJSON(t, rec)["applied"] == true {
			applied++
		}
	}
	if applied != 1 {
		t.Errorf("%d confirms applied the order, want exactly 1", applied)
	}
	if f.subscriptionVersion() != 2 || f.auditCount("billing.order_paid", id) != 1 || f.orderStatus(id) != "paid" {
		t.Errorf("version %d, paid rows %d, status %s", f.subscriptionVersion(), f.auditCount("billing.order_paid", id), f.orderStatus(id))
	}
}
