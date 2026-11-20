package console

import (
	"context"
	"testing"
	"time"
)

// The overview's series covers the current UTC day and the 29 before it, in any
// zone the clock is read in.
func TestSeriesStartIsThirtyUTCDaysEndingToday(t *testing.T) {
	zone := time.FixedZone("UTC+7", 7*3600)
	// 2026-11-20 23:30 in UTC+7 is still 2026-11-20 16:30 UTC.
	now := time.Date(2026, 11, 20, 23, 30, 0, 0, zone)
	want := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	if got := seriesStart(now); !got.Equal(want) {
		t.Fatalf("seriesStart = %v, want %v", got, want)
	}
	if got := seriesStart(now.AddDate(0, 0, 1)); !got.Equal(want.AddDate(0, 0, 1)) {
		t.Fatalf("seriesStart next day = %v, want %v", got, want.AddDate(0, 0, 1))
	}
}

// Database test: skipped unless PLATFORM_TEST_DATABASE_URL names a *_test
// database and DATABASE_URL equals it. The shared test database may hold other
// rows, so the test reads the overview before and after and checks its own rows
// add at least what it wrote. The usage meters are tested in internal/usage.
func TestDBOverviewCountsTheSeriesAndTheOperationsQueue(t *testing.T) {
	f := newDBFixture(t)
	owner, _ := f.user("owner")
	member, _ := f.user("member")
	reader := NewPgxReader(f.pool)
	ctx := context.Background()

	before, err := reader.Overview(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Series) != overviewSeriesDays {
		t.Fatalf("series has %d days, want %d", len(before.Series), overviewSeriesDays)
	}
	ws := f.workspace(owner, member) // created after the first read: today's count grows

	// A paid order created and paid today.
	var orderID string
	if err := f.pool.QueryRow(ctx, `
		INSERT INTO billing_order (id, workspace_id, kind, plan_key, interval, seats, amount, vat_amount, currency,
		    status, provider, transfer_code, buyer, period_start, period_end, expires_at, paid_at)
		VALUES (gen_random_uuid(), $1, 'subscribe', 'pro', 'month', 3, 1000, 0, 'vnd',
		    'paid', 'manual', 'MUL' || upper(substr(md5(random()::text), 1, 8)),
		    '{}', now(), now() + interval '1 month', now() + interval '1 day', now())
		RETURNING id::text`, ws).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.pool.Exec(ctx, `DELETE FROM billing_order WHERE id = $1`, orderID) })

	// A live legal hold on the workspace.
	if _, err := f.pool.Exec(ctx, `INSERT INTO legal_hold (id, scope_kind, scope_id, reason, case_ref)
		VALUES (gen_random_uuid(), 'workspace', $1, 'overview test', 'CASE-OVERVIEW')`, ws); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.pool.Exec(ctx, `DELETE FROM legal_hold WHERE scope_id = $1`, ws) })

	after, err := reader.Overview(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	last := overviewSeriesDays - 1
	if got := after.Series[last].Day; got != time.Now().UTC().Format(time.DateOnly) {
		t.Fatalf("newest series day = %s", got)
	}
	if d := after.Series[last].OrdersCreated - before.Series[last].OrdersCreated; d < 1 {
		t.Errorf("today's orders created grew by %d, want at least 1", d)
	}
	if d := after.Series[last].OrdersPaid - before.Series[last].OrdersPaid; d < 1 {
		t.Errorf("today's orders paid grew by %d, want at least 1", d)
	}
	if d := after.Series[last].WorkspacesCreated - before.Series[last].WorkspacesCreated; d < 1 {
		t.Errorf("today's workspaces created grew by %d, want at least 1", d)
	}
	if d := after.Operations.ActiveLegalHolds - before.Operations.ActiveLegalHolds; d < 1 {
		t.Errorf("active legal holds grew by %d, want at least 1", d)
	}
	if _, ok := after.RuntimesByStatus["online"]; !ok {
		t.Errorf("runtimes by status = %v, want an online count", after.RuntimesByStatus)
	}
}
