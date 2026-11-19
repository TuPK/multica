package console

import (
	"context"
	"testing"
	"time"
)

// Spec 7.4, rule 5: console totals are grouped by currency and never summed
// across currencies. The pending_orders count stays as it was.
func TestConsoleTotalsGroupedByCurrency(t *testing.T) {
	e := newEnv(t)
	e.reader.overview = Overview{
		PendingOrders: 3,
		PendingByCurrency: map[string]CurrencyTotal{
			"vnd": {Orders: 2, Amount: 1200000, VATAmount: 120000},
			"usd": {Orders: 1, Amount: 4900, VATAmount: 0},
		},
	}
	body := decodeJSON(t, e.do(suppID, "GET", "/api/platform/overview", ""))
	if body["pending_orders"] != 3.0 {
		t.Errorf("pending_orders = %v, want 3", body["pending_orders"])
	}
	groups, ok := body["pending_by_currency"].(map[string]any)
	if !ok || len(groups) != 2 {
		t.Fatalf("pending_by_currency = %v, want one group per currency", body["pending_by_currency"])
	}
	vnd, _ := groups["vnd"].(map[string]any)
	usd, _ := groups["usd"].(map[string]any)
	if vnd["orders"] != 2.0 || vnd["amount"] != 1200000.0 || vnd["vat_amount"] != 120000.0 {
		t.Errorf("vnd group = %v", vnd)
	}
	if usd["orders"] != 1.0 || usd["amount"] != 4900.0 {
		t.Errorf("usd group = %v", usd)
	}

	// With no pending orders the field is an empty object, not absent.
	e.reader.overview = Overview{}
	body = decodeJSON(t, e.do(suppID, "GET", "/api/platform/overview", ""))
	if groups, ok := body["pending_by_currency"].(map[string]any); !ok || len(groups) != 0 {
		t.Errorf("empty pending_by_currency = %v, want {}", body["pending_by_currency"])
	}
}

// The reader groups the pending orders of the database by their own currency.
// The shared test database may hold other pending orders, so the test reads the
// totals before and after.
func TestDBConsoleTotalsGroupedByCurrency(t *testing.T) {
	o := newOrderFixture(t)
	reader := NewPgxReader(o.pool)
	ctx := context.Background()
	before, err := reader.Overview(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		currency string
		amount   int64
	}{{"vnd", 600000}, {"usd", 4900}} {
		if _, err := o.pool.Exec(ctx, `
			INSERT INTO billing_order (id, workspace_id, kind, plan_key, interval, seats, amount, vat_amount, currency,
			    status, provider, transfer_code, buyer, period_start, period_end, expires_at)
			VALUES (gen_random_uuid(), $1, 'subscribe', 'pro', 'month', 3, $2, 0, $3,
			    'pending', 'manual', 'MUL' || upper(substr(md5(random()::text), 1, 8)),
			    '{}', now(), now() + interval '1 month', now() + interval '1 day')`, o.ws, c.amount, c.currency); err != nil {
			t.Fatal(err)
		}
	}
	after, err := reader.Overview(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for currency, amount := range map[string]int64{"vnd": 600000, "usd": 4900} {
		got := CurrencyTotal{
			Orders: after.PendingByCurrency[currency].Orders - before.PendingByCurrency[currency].Orders,
			Amount: after.PendingByCurrency[currency].Amount - before.PendingByCurrency[currency].Amount,
		}
		if got.Orders != 1 || got.Amount != amount {
			t.Errorf("%s grew by %+v, want 1 order of %d", currency, got, amount)
		}
	}
	if got := after.PendingOrders - before.PendingOrders; got != 2 {
		t.Errorf("pending_orders grew by %d, want 2", got)
	}
}
