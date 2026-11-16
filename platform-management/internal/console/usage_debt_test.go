package console

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/staff"
	"github.com/multica-ai/multica/platform-management/internal/usage"
)

// Database tests of the usage debt in the console (spec 7b.3, rules 13 and 14):
// the write-off and the tenant usage tab.

// usageDebt is a workspace on a pay as you go plan, past due, with an unpaid
// usage invoice that holds two charge rows.
type usageDebt struct {
	*orderFixture
	planKey string
	orderID string
}

func newUsageDebt(t *testing.T) *usageDebt {
	t.Helper()
	d := &usageDebt{orderFixture: newOrderFixture(t), planKey: "payg-co-" + randomHex(t)}
	ctx := context.Background()
	if _, err := d.pool.Exec(ctx, `INSERT INTO plan (key, name, currency, billing_model) VALUES ($1, 'Pay as you go (test)', 'vnd', 'usage')`, d.planKey); err != nil {
		t.Fatal(err)
	}
	if _, err := d.pool.Exec(ctx, `INSERT INTO workspace_subscription (workspace_id, plan_key, status, seats, interval, grace_until, version)
		VALUES ($1, $2, 'read_only', 0, 'month', now() - interval '1 day', 5)`, d.ws, d.planKey); err != nil {
		t.Fatal(err)
	}
	if _, err := d.pool.Exec(ctx, `INSERT INTO platform_workspace (workspace_id, status, reason) VALUES ($1, 'read_only', 'billing')
		ON CONFLICT (workspace_id) DO UPDATE SET status = 'read_only', reason = 'billing'`, d.ws); err != nil {
		t.Fatal(err)
	}
	if err := d.pool.QueryRow(ctx, `INSERT INTO billing_order (id, workspace_id, kind, plan_key, interval, seats, amount, vat_amount,
		    currency, status, provider, transfer_code, idempotency_key, buyer, period_start, period_end, created_at)
		VALUES (gen_random_uuid(), $1, 'usage', $2, 'month', 0, 50000, 5000, 'vnd', 'pending', 'manual',
		    'MUL' || upper(substr(md5(random()::text), 1, 8)), 'usage:' || md5(random()::text), '{}',
		    '2026-10-01', '2026-11-01', now() - interval '20 days')
		RETURNING id::text`, d.ws, d.planKey).Scan(&d.orderID); err != nil {
		t.Fatal(err)
	}
	for _, day := range []string{"2026-10-30", "2026-10-31"} {
		if _, err := d.pool.Exec(ctx, `INSERT INTO usage_charge (workspace_id, day, meter, quantity, plan_key, unit_price, per, discount_pct, net_amount, order_id)
			VALUES ($1, $2, $5, 1, $3, 1, 1, 0, 25000, $4)`, d.ws, day, d.planKey, d.orderID, usage.MeterTasksRun); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		d.pool.Exec(ctx, `DELETE FROM usage_charge WHERE workspace_id = $1`, d.ws)
		d.pool.Exec(ctx, `DELETE FROM platform_usage_daily WHERE workspace_id = $1`, d.ws)
		d.pool.Exec(ctx, `DELETE FROM plan WHERE key = $1`, d.planKey)
	})
	return d
}

func (d *usageDebt) subscriptionStatus() string {
	d.t.Helper()
	var s string
	if err := d.pool.QueryRow(context.Background(), `SELECT status FROM workspace_subscription WHERE workspace_id = $1`, d.ws).Scan(&s); err != nil {
		d.t.Fatal(err)
	}
	return s
}

func (d *usageDebt) workspaceStatus() (status, reason string) {
	d.t.Helper()
	if err := d.pool.QueryRow(context.Background(), `SELECT status, reason FROM platform_workspace WHERE workspace_id = $1`, d.ws).Scan(&status, &reason); err != nil {
		d.t.Fatal(err)
	}
	return status, reason
}

func TestWriteOffUsageOrderAdminOnlyWithReason(t *testing.T) {
	d := newUsageDebt(t)
	billID, _ := d.staffUser(staff.RoleBilling)
	adminID, adminEmail := d.staffUser(staff.RoleAdmin)
	path := "/api/platform/orders/" + d.orderID + "/cancel"

	wantError(t, d.do(billID, "POST", path, `{"reason":"customer dispute"}`), 403, "usage_cancel_admin_only")
	wantError(t, d.do(adminID, "POST", path, `{}`), 400, "reason_required")
	if got := d.orderStatus(d.orderID); got != "pending" {
		t.Fatalf("order status after the refusals = %q, want pending", got)
	}

	rec := d.do(adminID, "POST", path, `{"reason":"customer dispute"}`)
	wantStatus(t, rec, 200)
	if got := d.orderStatus(d.orderID); got != "canceled" {
		t.Fatalf("order status = %q, want canceled", got)
	}
	rows := d.staffRows("billing.usage_written_off", adminID)
	if len(rows) != 1 || rows[0].actorLabel != adminEmail || rows[0].targetID != d.orderID || rows[0].workspaceID != d.ws {
		t.Fatalf("billing.usage_written_off rows = %+v", rows)
	}
	var detail struct {
		Reason    string `json:"reason"`
		OrderID   string `json:"order_id"`
		Amount    int64  `json:"amount"`
		VATAmount int64  `json:"vat_amount"`
		Lines     int64  `json:"lines"`
	}
	if err := json.Unmarshal([]byte(rows[0].detail), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Reason != "customer dispute" || detail.OrderID != d.orderID || detail.Amount != 50000 || detail.VATAmount != 5000 || detail.Lines != 2 {
		t.Errorf("detail = %+v, want the reason, order, amount, VAT and 2 lines", detail)
	}
	if n := d.auditCount("billing.order_canceled", d.orderID); n != 0 {
		t.Errorf("billing.order_canceled rows = %d, want none: the write-off replaces it", n)
	}

	// The last debt is gone: the subscription and the workspace are restored.
	if got := d.subscriptionStatus(); got != "active" {
		t.Errorf("subscription status = %q, want active", got)
	}
	if status, reason := d.workspaceStatus(); status != "active" || reason != "usage debt written off" {
		t.Errorf("workspace = %q, %q, want active with the write-off reason", status, reason)
	}

	// A second write-off is refused: the order is not pending.
	wantError(t, d.do(adminID, "POST", path, `{"reason":"again"}`), 409, "order_not_pending")
}

func TestWriteOffKeepsLinesOutOfNextInvoice(t *testing.T) {
	d := newUsageDebt(t)
	adminID, _ := d.staffUser(staff.RoleAdmin)
	wantStatus(t, d.do(adminID, "POST", "/api/platform/orders/"+d.orderID+"/cancel", `{"reason":"goodwill"}`), 200)

	inv := usage.NewInvoicer(d.pool, usage.InvoicerConfig{VATRate: billing.DefaultVATRate, MinInvoice: 1})
	if _, err := inv.Run(context.Background(), time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if n := d.count(`SELECT count(*) FROM billing_order WHERE workspace_id = $1 AND kind = 'usage'`, d.ws); n != 1 {
		t.Errorf("usage orders after the next invoicing run = %d, want only the written-off one", n)
	}
	if n := d.count(`SELECT count(*) FROM usage_charge WHERE workspace_id = $1 AND order_id::text = $2`, d.ws, d.orderID); n != 2 {
		t.Errorf("rows still on the written-off order = %d, want 2", n)
	}
}

func TestTenantUsageShowsMonthRowsMissingDaysAndUnpaidOrders(t *testing.T) {
	d := newUsageDebt(t)
	supportID, _ := d.staffUser(staff.RoleSupport)
	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	ctx := context.Background()
	if _, err := d.pool.Exec(ctx, `INSERT INTO usage_charge (workspace_id, day, meter, quantity, plan_key, unit_price, per, discount_pct, net_amount, note)
		VALUES ($1, $2, $4, 7, $3, 30000, 1000, 20, 168, 'a note')`, d.ws, start, d.planKey, usage.MeterTasksRun); err != nil {
		t.Fatal(err)
	}
	if _, err := d.pool.Exec(ctx, `INSERT INTO platform_usage_daily (workspace_id, day, members) VALUES ($1, $2, 7)`, d.ws, start); err != nil {
		t.Fatal(err)
	}

	rec := d.do(supportID, "GET", "/api/platform/tenants/"+d.ws+"/usage", "")
	wantStatus(t, rec, 200)
	var body struct {
		WorkspaceID string `json:"workspace_id"`
		PeriodStart string `json:"period_start"`
		PeriodEnd   string `json:"period_end"`
		Rows        []struct {
			Day         string  `json:"day"`
			Meter       string  `json:"meter"`
			Quantity    int64   `json:"quantity"`
			UnitPrice   int64   `json:"unit_price"`
			Per         int64   `json:"per"`
			DiscountPct int64   `json:"discount_pct"`
			NetAmount   int64   `json:"net_amount"`
			Note        string  `json:"note"`
			OrderID     *string `json:"order_id"`
		} `json:"rows"`
		MissingDays  []string `json:"missing_days"`
		UnpaidOrders []struct {
			ID        string `json:"id"`
			Amount    int64  `json:"amount"`
			VATAmount int64  `json:"vat_amount"`
			Status    string `json:"status"`
		} `json:"unpaid_orders"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.WorkspaceID != d.ws || body.PeriodStart != start.Format(time.RFC3339) || body.PeriodEnd != start.AddDate(0, 1, 0).Format(time.RFC3339) {
		t.Errorf("header = %s %s %s, want the workspace and this UTC month", body.WorkspaceID, body.PeriodStart, body.PeriodEnd)
	}
	if len(body.Rows) != 1 || body.Rows[0].Day != start.Format("2006-01-02") || body.Rows[0].Meter != usage.MeterTasksRun ||
		body.Rows[0].Quantity != 7 || body.Rows[0].UnitPrice != 30000 || body.Rows[0].Per != 1000 ||
		body.Rows[0].DiscountPct != 20 || body.Rows[0].NetAmount != 168 || body.Rows[0].Note != "a note" || body.Rows[0].OrderID != nil {
		t.Errorf("rows = %+v, want this month's one row only", body.Rows)
	}
	// The days from the start of the month to yesterday that have no snapshot row.
	var want []string
	for day := start.AddDate(0, 0, 1); day.Before(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)); day = day.AddDate(0, 0, 1) {
		want = append(want, day.Format("2006-01-02"))
	}
	if len(body.MissingDays) != len(want) || (len(want) > 0 && body.MissingDays[0] != want[0]) {
		t.Errorf("missing_days = %v, want %v", body.MissingDays, want)
	}
	if len(body.UnpaidOrders) != 1 || body.UnpaidOrders[0].ID != d.orderID || body.UnpaidOrders[0].Amount != 50000 ||
		body.UnpaidOrders[0].VATAmount != 5000 || body.UnpaidOrders[0].Status != "pending" {
		t.Errorf("unpaid_orders = %+v, want the pending invoice", body.UnpaidOrders)
	}

	// A workspace on a seat plan has no usage rows and no missing days, as empty lists.
	if _, err := d.pool.Exec(ctx, `UPDATE workspace_subscription SET plan_key = 'free' WHERE workspace_id = $1`, d.ws); err != nil {
		t.Fatal(err)
	}
	seat := decodeJSON(t, d.do(supportID, "GET", "/api/platform/tenants/"+d.ws+"/usage", ""))
	if rows, ok := seat["rows"].([]any); !ok || len(rows) != 0 {
		t.Errorf("rows of a seat plan = %v, want []", seat["rows"])
	}
	if days, ok := seat["missing_days"].([]any); !ok || len(days) != 0 {
		t.Errorf("missing_days of a seat plan = %v, want []", seat["missing_days"])
	}
	wantError(t, d.do(supportID, "GET", "/api/platform/tenants/00000000-0000-4000-8000-000000000000/usage", ""), 404, "not_found")
}
