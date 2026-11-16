package usage

import (
	"context"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/subscription"
	"github.com/multica-ai/multica/platform-management/internal/testdb"
)

func TestUsageViewBuildsMetersFromTheMonthsRows(t *testing.T) {
	gib := int64(1) << 30
	in := UsageInput{
		Now: time.Date(2026, 8, 15, 10, 30, 0, 0, time.UTC), Currency: "vnd",
		Rates: Rates{
			MeterTasksRun:     {Per: 1000, Price: 30000},
			MeterStorageBytes: {Per: gib, Price: 3000},
			MeterMembers:      {Per: 1, Price: 49000},
		},
		DiscountPct: 20, CreditLimit: 2000000, Accrued: 123456, UnpaidOrderID: "dddddddd-0000-0000-0000-0000000000a1",
		Rows: []ChargeRow{
			{Day: day("2026-08-13"), Meter: MeterTasksRun, Quantity: 1200, UnitPrice: 30000, Per: 1000, DiscountPct: 20, NetAmount: 28800},
			{Day: day("2026-08-13"), Meter: MeterStorageBytes, Quantity: gib, UnitPrice: 3000, Per: gib, DiscountPct: 20, NetAmount: 77},
			{Day: day("2026-08-14"), Meter: MeterTasksRun, Quantity: 800, UnitPrice: 30000, Per: 1000, DiscountPct: 20, NetAmount: 19200},
			{Day: day("2026-08-14"), Meter: MeterStorageBytes, Quantity: 2 * gib, UnitPrice: 2500, Per: gib, DiscountPct: 10, NetAmount: 145},
		},
	}
	got := BuildUsage(in)
	if got.PeriodStart != "2026-08-01T00:00:00Z" || got.PeriodEnd != "2026-09-01T00:00:00Z" || got.Currency != "vnd" {
		t.Errorf("header = %s %s %s, want August 2026 in vnd", got.PeriodStart, got.PeriodEnd, got.Currency)
	}
	if got.AccruedAmount != 123456 || got.CreditLimit == nil || *got.CreditLimit != 2000000 ||
		got.UnpaidOrderID == nil || *got.UnpaidOrderID != in.UnpaidOrderID {
		t.Errorf("accrued %d, limit %v, unpaid %v; want the inputs", got.AccruedAmount, got.CreditLimit, got.UnpaidOrderID)
	}
	want := []subscription.UsageMeter{
		// A flow meter adds the days up.
		{Key: MeterTasksRun, Quantity: 2000, UnitPrice: 30000, Per: 1000, DiscountPct: 20, Amount: 48000},
		// A stock meter shows the newest rated day, at that day's price.
		{Key: MeterStorageBytes, Quantity: 2 * gib, UnitPrice: 2500, Per: gib, DiscountPct: 10, Amount: 222},
		// No row yet: zero, at the plan's rate and discount.
		{Key: MeterMembers, Quantity: 0, UnitPrice: 49000, Per: 1, DiscountPct: 20, Amount: 0},
	}
	if len(got.Meters) != len(want) {
		t.Fatalf("meters = %+v, want %d", got.Meters, len(want))
	}
	for i, w := range want {
		if got.Meters[i] != w {
			t.Errorf("meter %d = %+v, want %+v", i, got.Meters[i], w)
		}
	}

	in.CreditLimit, in.UnpaidOrderID, in.Rows = 0, "", nil
	empty := BuildUsage(in)
	if empty.CreditLimit != nil || empty.UnpaidOrderID != nil || len(empty.Meters) != 3 || empty.Meters[0].Amount != 0 {
		t.Errorf("a month with no rows and no limit = %+v, want null limit and order and three zero meters", empty)
	}
}

func TestUsageViewReadsAccrualLimitAndUnpaidOrder(t *testing.T) {
	pool := testdb.Open(t)
	f := newFixtureWithTerms(t, pool, Terms{
		DiscountPct: map[string]int64{"business": 20},
		CreditLimit: map[string]int64{"individual": 100000, "business": 500000},
	})
	ctx := context.Background()
	now := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)

	for _, tc := range []struct {
		name, kind string
		limit      int64
	}{{"individual", "", 100000}, {"business", "business", 500000}} {
		t.Run(tc.name, func(t *testing.T) {
			ws := f.workspace(wsSpec{kind: tc.kind, verified: true})
			f.cleanInvoices(ws)
			f.charge(ws, "2026-10-31", 7000) // an earlier month, not invoiced: counts in the accrual
			f.charge(ws, "2026-11-02", 1500) // this month
			f.charge(ws, "2026-11-03", 2500) // this month
			var orderID string
			if err := pool.QueryRow(ctx, `INSERT INTO billing_order (id, workspace_id, kind, plan_key, interval, seats, amount, vat_amount,
				    currency, status, provider, transfer_code, idempotency_key, buyer, period_start, period_end, created_at)
				VALUES (gen_random_uuid(), $1, 'usage', $2, 'month', 0, 9000, 900, 'vnd', 'pending', 'manual',
				    'MUL' || upper(substr(md5(random()::text), 1, 8)), 'usage:' || md5(random()::text), '{}', '2026-09-01', '2026-10-01', now())
				RETURNING id::text`, ws, f.planKey).Scan(&orderID); err != nil {
				t.Fatal(err)
			}
			mustExec(t, pool, `INSERT INTO usage_charge (workspace_id, day, meter, quantity, plan_key, unit_price, per, discount_pct, net_amount, order_id)
				VALUES ($1, '2026-09-30', 'tasks_run', 1, $2, 1, 1, 0, 9000, $3)`, ws, f.planKey, orderID)

			got, err := NewView(pool, true).Usage(ctx, ws, now)
			if err != nil {
				t.Fatal(err)
			}
			if got.PeriodStart != "2026-11-01T00:00:00Z" || got.PeriodEnd != "2026-12-01T00:00:00Z" || got.Currency != "vnd" {
				t.Errorf("header = %s %s %s", got.PeriodStart, got.PeriodEnd, got.Currency)
			}
			// 7000 + 1500 + 2500: the invoiced 9000 is not accrued.
			if got.AccruedAmount != 11000 {
				t.Errorf("accrued = %d, want 11000", got.AccruedAmount)
			}
			if got.CreditLimit == nil || *got.CreditLimit != tc.limit {
				t.Errorf("credit limit = %v, want %d for a %s account", got.CreditLimit, tc.limit, tc.name)
			}
			if got.UnpaidOrderID == nil || *got.UnpaidOrderID != orderID {
				t.Errorf("unpaid order = %v, want %s", got.UnpaidOrderID, orderID)
			}
			if len(got.Meters) != 3 || got.Meters[0].Key != MeterTasksRun || got.Meters[0].Amount != 4000 || got.Meters[0].Quantity != 2 {
				t.Errorf("meters = %+v, want tasks_run first with the month's two rows", got.Meters)
			}
			// The plan's rate and, for a verified business account, its discount.
			if got.Meters[2].Key != MeterMembers || got.Meters[2].UnitPrice != 49000 {
				t.Errorf("members meter = %+v, want the plan's price", got.Meters[2])
			}
			wantDiscount := int64(0)
			if tc.kind == "business" {
				wantDiscount = 20
			}
			if got.Meters[2].DiscountPct != wantDiscount {
				t.Errorf("members discount = %d, want %d", got.Meters[2].DiscountPct, wantDiscount)
			}
		})
	}
}
