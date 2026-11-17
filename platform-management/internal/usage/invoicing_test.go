package usage

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/db"
	"github.com/multica-ai/multica/platform-management/internal/lifecycle"
	"github.com/multica-ai/multica/platform-management/internal/testdb"
)

// Database tests of the usage_invoicing job and of the credit limit (spec 7b.3,
// rules 11 and 12). The jobs read every workspace in the shared database, so
// each test asserts on its own workspace's rows.

const testMinInvoice = 10000

// invoicingNow is inside November 2026: the closed month is October.
var invoicingNow = time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)

func tenPercent(t *testing.T) billing.Rate {
	t.Helper()
	r, err := billing.ParseRate("0.10")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func newInvoicer(t *testing.T, f fixture) *Invoicer {
	t.Helper()
	return NewInvoicer(f.pool, InvoicerConfig{VATRate: tenPercent(t), MinInvoice: testMinInvoice})
}

// charge writes one uninvoiced usage_charge row of a day.
func (f fixture) charge(ws, d string, net int64) {
	f.t.Helper()
	mustExec(f.t, f.pool, `INSERT INTO usage_charge (workspace_id, day, meter, quantity, plan_key, unit_price, per, discount_pct, net_amount)
		VALUES ($1, $2, 'tasks_run', 1, $3, 1, 1, 0, $4)`, ws, day(d), f.planKey, net)
}

type usageOrder struct {
	id, kind, planKey, interval, status, provider, code, key string
	seats                                                    int
	amount, vat                                              int64
	expiresNull, createdByNull                               bool
	periodStart, periodEnd                                   time.Time
}

// orders returns the usage orders of a workspace, oldest first, and removes
// them and their audit rows when the test ends.
func (f fixture) orders(ws string) []usageOrder {
	f.t.Helper()
	rows, err := f.pool.Query(context.Background(), `SELECT id::text, kind, plan_key, interval, status, provider, transfer_code,
		idempotency_key, seats, amount, vat_amount, expires_at IS NULL, created_by IS NULL, period_start, period_end
		FROM billing_order WHERE workspace_id = $1 AND kind = 'usage' ORDER BY created_at, id`, ws)
	if err != nil {
		f.t.Fatal(err)
	}
	defer rows.Close()
	var out []usageOrder
	for rows.Next() {
		var o usageOrder
		if err := rows.Scan(&o.id, &o.kind, &o.planKey, &o.interval, &o.status, &o.provider, &o.code, &o.key,
			&o.seats, &o.amount, &o.vat, &o.expiresNull, &o.createdByNull, &o.periodStart, &o.periodEnd); err != nil {
			f.t.Fatal(err)
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		f.t.Fatal(err)
	}
	return out
}

// cleanInvoices removes what the invoicing of ws wrote.
func (f fixture) cleanInvoices(ws string) {
	f.t.Cleanup(func() {
		f.exec(`DELETE FROM billing_order WHERE workspace_id = $1`, ws)
		f.exec(`DELETE FROM audit_event WHERE workspace_id = $1`, ws)
		f.exec(`DELETE FROM billing_notice WHERE workspace_id = $1`, ws)
	})
}

func (f fixture) invoicedRows(ws, orderID string) int {
	f.t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM usage_charge WHERE workspace_id = $1 AND order_id::text = $2`, ws, orderID).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

func (f fixture) uninvoicedRows(ws string) int {
	f.t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM usage_charge WHERE workspace_id = $1 AND order_id IS NULL`, ws).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

func (f fixture) auditRows(ws, action string) int {
	f.t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_event WHERE workspace_id = $1 AND action = $2`, ws, action).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

func (f fixture) auditKind(ws, action string) string {
	f.t.Helper()
	var kind string
	if err := f.pool.QueryRow(context.Background(), `SELECT detail->>'kind' FROM audit_event WHERE workspace_id = $1 AND action = $2`, ws, action).Scan(&kind); err != nil {
		f.t.Fatal(err)
	}
	return kind
}

func TestUsageInvoiceMonthlyTotalsVAT(t *testing.T) {
	pool := testdb.Open(t)
	f := newFixture(t, pool)
	ws := f.workspace(wsSpec{})
	f.cleanInvoices(ws)
	f.charge(ws, "2026-10-30", 5000)
	f.charge(ws, "2026-10-31", 10005)
	f.charge(ws, "2026-11-02", 9999) // this month: not on the October invoice

	if _, err := newInvoicer(t, f).Run(context.Background(), invoicingNow); err != nil {
		t.Fatal(err)
	}
	orders := f.orders(ws)
	if len(orders) != 1 {
		t.Fatalf("usage orders = %d, want 1", len(orders))
	}
	o := orders[0]
	// 15005 at 10 percent is 1500.5, rounded half up.
	if o.amount != 15005 || o.vat != 1501 {
		t.Errorf("amount/vat = %d/%d, want 15005/1501", o.amount, o.vat)
	}
	if o.status != "pending" || o.provider != "manual" || o.planKey != f.planKey || o.interval != "month" || o.seats != 0 {
		t.Errorf("order = %+v, want a pending manual month order of the plan with 0 seats", o)
	}
	if !o.expiresNull || !o.createdByNull || o.code == "" {
		t.Errorf("expires_at null = %v, created_by null = %v, transfer code %q; want null, null and a code", o.expiresNull, o.createdByNull, o.code)
	}
	if !o.periodStart.Equal(day("2026-10-01")) || !o.periodEnd.Equal(day("2026-11-01")) {
		t.Errorf("period = [%s, %s), want October", o.periodStart, o.periodEnd)
	}
	if o.key != "usage:"+ws+":2026-10-31" {
		t.Errorf("idempotency key = %q", o.key)
	}
	if n := f.invoicedRows(ws, o.id); n != 2 {
		t.Errorf("rows on the invoice = %d, want 2", n)
	}
	if n := f.uninvoicedRows(ws); n != 1 {
		t.Errorf("uninvoiced rows = %d, want the one of this month", n)
	}
	if n := f.auditRows(ws, "billing.usage_invoiced"); n != 1 || f.auditKind(ws, "billing.usage_invoiced") != "monthly" {
		t.Errorf("billing.usage_invoiced rows = %d (kind %q), want one monthly", n, f.auditKind(ws, "billing.usage_invoiced"))
	}
}

func TestUsageMathVATRounding(t *testing.T) {
	rows := func(nets ...int64) []db.ListUninvoicedUsageChargesRow {
		out := make([]db.ListUninvoicedUsageChargesRow, len(nets))
		for i, n := range nets {
			out[i] = db.ListUninvoicedUsageChargesRow{Day: pgtype.Date{Time: day("2026-10-30").AddDate(0, 0, i), Valid: true}, NetAmount: n}
		}
		return out
	}
	for _, tc := range []struct {
		name         string
		nets         []int64
		amount, vat  int64
		start, end   string
		lastIncluded string
	}{
		{"half rounds up", []int64{5000, 10005}, 15005, 1501, "2026-10-01", "2026-11-01", "2026-10-31"},
		{"below half rounds down", []int64{5000, 10004}, 15004, 1500, "2026-10-01", "2026-11-01", "2026-10-31"},
		{"a carried-over invoice spans months", []int64{9000, 1000, 5}, 10005, 1001, "2026-10-01", "2026-12-01", "2026-11-01"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, err := invoiceDraft("11111111-1111-4111-8111-111111111111", "payg", "vnd", rows(tc.nets...), tenPercent(t), nil)
			if err != nil {
				t.Fatal(err)
			}
			if d.Kind != billing.KindUsage || d.Seats != 0 || d.Interval != "month" || d.Currency != "vnd" || d.PlanKey != "payg" {
				t.Errorf("draft = %+v, want a usage order of plan payg for 0 seats on a month in vnd", d)
			}
			if d.Amount != tc.amount || d.VATAmount != tc.vat {
				t.Errorf("amount/vat = %d/%d, want %d/%d", d.Amount, d.VATAmount, tc.amount, tc.vat)
			}
			if d.PeriodStart == nil || d.PeriodEnd == nil || !d.PeriodStart.Equal(day(tc.start)) || !d.PeriodEnd.Equal(day(tc.end)) {
				t.Errorf("period = %v to %v, want %s to %s", d.PeriodStart, d.PeriodEnd, tc.start, tc.end)
			}
			if want := "usage:11111111-1111-4111-8111-111111111111:" + tc.lastIncluded; d.IdempotencyKey != want {
				t.Errorf("idempotency key = %q, want %q", d.IdempotencyKey, want)
			}
			if string(d.Buyer) != "{}" {
				t.Errorf("buyer = %s, want {} without a profile", d.Buyer)
			}
		})
	}
}

func TestUsageInvoiceMinimumCarriesOver(t *testing.T) {
	pool := testdb.Open(t)
	f := newFixture(t, pool)
	ws := f.workspace(wsSpec{})
	f.cleanInvoices(ws)
	f.charge(ws, "2026-10-31", 8000)
	inv := newInvoicer(t, f)

	if _, err := inv.Run(context.Background(), invoicingNow); err != nil {
		t.Fatal(err)
	}
	if n := len(f.orders(ws)); n != 0 {
		t.Fatalf("usage orders under the minimum = %d, want none", n)
	}
	if n := f.uninvoicedRows(ws); n != 1 {
		t.Fatalf("uninvoiced rows = %d, want the row carried over", n)
	}

	f.charge(ws, "2026-11-20", 5000)
	if _, err := inv.Run(context.Background(), time.Date(2026, 12, 2, 10, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	orders := f.orders(ws)
	if len(orders) != 1 || orders[0].amount != 13000 {
		t.Fatalf("usage orders = %+v, want one of 13000 with both months", orders)
	}
	if !orders[0].periodStart.Equal(day("2026-10-01")) || !orders[0].periodEnd.Equal(day("2026-12-01")) {
		t.Errorf("period = [%s, %s), want October and November", orders[0].periodStart, orders[0].periodEnd)
	}
	if n := f.uninvoicedRows(ws); n != 0 {
		t.Errorf("uninvoiced rows = %d, want 0", n)
	}
}

func TestUsageInvoiceFinalWhenLeavingUsage(t *testing.T) {
	pool := testdb.Open(t)
	f := newFixture(t, pool)
	seatPlan := "seat-" + randomSuffix(t)
	mustExec(t, pool, `INSERT INTO plan (key, name, currency) VALUES ($1, 'Seat (test)', 'vnd')`, seatPlan)
	t.Cleanup(func() { f.exec(`DELETE FROM plan WHERE key = $1`, seatPlan) })

	for _, to := range []string{"free", seatPlan} {
		t.Run(to, func(t *testing.T) {
			ws := f.workspace(wsSpec{})
			f.cleanInvoices(ws)
			// Four thousand is under the minimum, and the day is in the current month.
			f.charge(ws, "2026-11-10", 4000)
			mustExec(t, pool, `UPDATE workspace_subscription SET plan_key = $2 WHERE workspace_id = $1`, ws, to)

			if _, err := newInvoicer(t, f).Run(context.Background(), invoicingNow); err != nil {
				t.Fatal(err)
			}
			orders := f.orders(ws)
			if len(orders) != 1 || orders[0].amount != 4000 || orders[0].planKey != to {
				t.Fatalf("usage orders = %+v, want one final invoice of 4000 on plan %s", orders, to)
			}
			if !orders[0].periodStart.Equal(day("2026-11-01")) || !orders[0].periodEnd.Equal(day("2026-12-01")) {
				t.Errorf("period = [%s, %s), want November", orders[0].periodStart, orders[0].periodEnd)
			}
			if f.auditKind(ws, "billing.usage_invoiced") != "final" {
				t.Errorf("audit kind = %q, want final", f.auditKind(ws, "billing.usage_invoiced"))
			}
			if n := f.uninvoicedRows(ws); n != 0 {
				t.Errorf("uninvoiced rows = %d, want 0", n)
			}
		})
	}
}

func TestUsageInvoiceRunTwiceAndParallelCreatesOneOrder(t *testing.T) {
	pool := testdb.Open(t)
	f := newFixture(t, pool)
	ws := f.workspace(wsSpec{})
	f.cleanInvoices(ws)
	f.charge(ws, "2026-10-30", 9000)
	f.charge(ws, "2026-10-31", 9000)
	inv := newInvoicer(t, f)

	for range 2 {
		if _, err := inv.Run(context.Background(), invoicingNow); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(f.orders(ws)); n != 1 {
		t.Fatalf("usage orders after two runs = %d, want 1", n)
	}

	// Parallel runs on a second workspace, before any invoice exists.
	other := f.workspace(wsSpec{})
	f.cleanInvoices(other)
	f.charge(other, "2026-10-30", 9000)
	f.charge(other, "2026-10-31", 9000)
	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = inv.Run(context.Background(), invoicingNow)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("parallel Run: %v", err)
		}
	}
	orders := f.orders(other)
	if len(orders) != 1 || orders[0].amount != 18000 {
		t.Fatalf("usage orders after parallel runs = %+v, want one of 18000", orders)
	}
	if n := f.auditRows(other, "billing.usage_invoiced"); n != 1 {
		t.Errorf("billing.usage_invoiced rows = %d, want 1", n)
	}
}

type mailLog struct {
	mu   sync.Mutex
	sent []lifecycle.Message
}

func (m *mailLog) Send(_ context.Context, msg lifecycle.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, msg)
	return nil
}

func (m *mailLog) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sent)
}

// owner makes the workspace's owner, who has an email address.
func (f fixture) owner(ws string) string {
	f.t.Helper()
	email := "owner-" + randomSuffix(f.t) + "@example.test"
	var id string
	if err := f.pool.QueryRow(context.Background(), `INSERT INTO "user" (name, email) VALUES ('Owner', $1) RETURNING id::text`, email).Scan(&id); err != nil {
		f.t.Fatal(err)
	}
	mustExec(f.t, f.pool, `INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`, ws, id)
	f.t.Cleanup(func() {
		f.exec(`DELETE FROM member WHERE workspace_id = $1`, ws)
		f.exec(`DELETE FROM "user" WHERE id = $1`, id)
	})
	return email
}

func (f fixture) zeroSnapshot(ws, d string) {
	f.t.Helper()
	mustExec(f.t, f.pool, `INSERT INTO platform_usage_daily (workspace_id, day, members, agents, runtimes, issues_total,
		issues_created, tasks_run, storage_bytes, computed_at) VALUES ($1, $2, 0, 0, 0, 0, 0, 0, 0, now())`, ws, day(d))
}

func TestUsageInvoiceCreditLimitTempInvoiceAndEightyPercentNoticeOnce(t *testing.T) {
	pool := testdb.Open(t)
	// A day of the snapshot (1000 tasks, 1 GiB, 10 members) is 46433 in a
	// 30-day month: two days are 92866, between 80 and 100 percent of the
	// limit, and three days pass it.
	f := newFixtureWithTerms(t, pool, Terms{CreditLimit: map[string]int64{"individual": 100000}})
	ws := f.workspace(wsSpec{})
	f.cleanInvoices(ws)
	ownerEmail := f.owner(ws)
	mail := &mailLog{}
	rater := NewRater(pool, RaterConfig{VATRate: tenPercent(t), Mail: mail, AppURL: "https://app.example.com"})
	ctx := context.Background()

	f.snapshot(ws, day("2026-11-10"))
	f.snapshot(ws, day("2026-11-11"))
	if _, err := rater.Run(ctx, jobNow); err != nil {
		t.Fatal(err)
	}
	if n := mail.count(); n != 1 {
		t.Fatalf("emails after crossing 80 percent = %d, want 1", n)
	}
	msg := mail.sent[0]
	if len(msg.To) != 1 || msg.To[0] != ownerEmail {
		t.Errorf("email goes to %v, want the owner %s", msg.To, ownerEmail)
	}
	var kind string
	if err := pool.QueryRow(ctx, `SELECT kind FROM billing_notice WHERE workspace_id = $1 AND notice_key = 'usage_80:2026-11'`, ws).Scan(&kind); err != nil || kind != "usage_80" {
		t.Fatalf("billing_notice usage_80:2026-11 = %q (%v), want kind usage_80", kind, err)
	}
	if n := len(f.orders(ws)); n != 0 {
		t.Fatalf("usage orders below the limit = %d, want none", n)
	}

	// A second rating run in the same month, still under the limit: no second email.
	f.zeroSnapshot(ws, "2026-11-12")
	if _, err := rater.Run(ctx, jobNow); err != nil {
		t.Fatal(err)
	}
	if n := mail.count(); n != 1 {
		t.Fatalf("emails after a second run = %d, want still 1", n)
	}

	// The third full day reaches 100 percent: a temporary invoice, no minimum.
	f.snapshot(ws, day("2026-11-13"))
	if _, err := rater.Run(ctx, jobNow); err != nil {
		t.Fatal(err)
	}
	orders := f.orders(ws)
	if len(orders) != 1 || orders[0].amount != 139299 || orders[0].vat != 13930 {
		t.Fatalf("usage orders = %+v, want one temporary invoice of 139299 plus 13930 VAT", orders)
	}
	if n := f.uninvoicedRows(ws); n != 0 {
		t.Errorf("uninvoiced rows = %d, want 0", n)
	}
	if f.auditRows(ws, "billing.usage_credit_limit_reached") != 1 || f.auditKind(ws, "billing.usage_invoiced") != "credit_limit" {
		t.Errorf("audit rows: credit_limit_reached %d, usage_invoiced kind %q; want one and credit_limit",
			f.auditRows(ws, "billing.usage_credit_limit_reached"), f.auditKind(ws, "billing.usage_invoiced"))
	}
	if n := mail.count(); n != 1 {
		t.Errorf("emails after the invoice = %d, want still 1", n)
	}
}

func TestUsageInvoiceCreditLimitHasNoMinimum(t *testing.T) {
	pool := testdb.Open(t)
	f := newFixtureWithTerms(t, pool, Terms{CreditLimit: map[string]int64{"individual": 2000}})
	ws := f.workspace(wsSpec{})
	f.cleanInvoices(ws)
	// 100 tasks cost 3000: over the limit and far under the minimum invoice.
	mustExec(t, pool, `INSERT INTO platform_usage_daily (workspace_id, day, members, agents, runtimes, issues_total,
		issues_created, tasks_run, storage_bytes, computed_at) VALUES ($1, $2, 0, 0, 0, 0, 0, 100, 0, now())`, ws, day("2026-11-14"))

	if _, err := NewRater(pool, RaterConfig{VATRate: tenPercent(t)}).Run(context.Background(), jobNow); err != nil {
		t.Fatal(err)
	}
	orders := f.orders(ws)
	if len(orders) != 1 || orders[0].amount != 3000 {
		t.Fatalf("usage orders = %+v, want one of 3000", orders)
	}
}
