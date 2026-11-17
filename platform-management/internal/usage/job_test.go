package usage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/multica-ai/multica/platform-management/internal/testdb"
)

// Database tests of the usage_rating job. They skip unless
// PLATFORM_TEST_DATABASE_URL names a *_test database. The job rates every usage
// plan in the shared database, so each test asserts on its own fixture rows.

// jobNow is a fixed clock: the closed days are the ones before 2026-11-16.
var jobNow = time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)

func day(s string) time.Time {
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return d
}

func randomSuffix(t *testing.T) string {
	t.Helper()
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

// fixture is one usage plan with its rates and terms, shared by the workspaces
// of one test.
type fixture struct {
	t       *testing.T
	pool    *pgxpool.Pool
	planKey string
}

type wsSpec struct {
	status   string // platform_workspace status; empty means no row (active)
	kind     string // billing_profile account kind; empty means no row (individual)
	verified bool
}

func newFixture(t *testing.T, pool *pgxpool.Pool) fixture {
	t.Helper()
	return newFixtureWithTerms(t, pool, Terms{DiscountPct: map[string]int64{"business": 20}})
}

func newFixtureWithTerms(t *testing.T, pool *pgxpool.Pool, terms Terms) fixture {
	t.Helper()
	f := fixture{t: t, pool: pool, planKey: "payg-job-" + randomSuffix(t)}
	rates := map[string]Rate{
		MeterTasksRun:     {Per: 1000, Price: 30000},
		MeterStorageBytes: {Per: 1 << 30, Price: 3000},
		MeterMembers:      {Per: 1, Price: 49000},
	}
	ratesJSON, _ := json.Marshal(rates)
	termsJSON, _ := json.Marshal(terms)
	mustExec(t, pool, `INSERT INTO plan (key, name, currency, billing_model, audience, usage_rates, usage_terms, is_public)
		VALUES ($1, 'Pay as you go (test)', 'vnd', 'usage', 'any', $2, $3, false)`, f.planKey, ratesJSON, termsJSON)
	t.Cleanup(func() {
		f.exec(`DELETE FROM usage_charge WHERE plan_key = $1`, f.planKey)
		f.exec(`DELETE FROM platform_usage_daily WHERE workspace_id IN (SELECT workspace_id FROM workspace_subscription WHERE plan_key = $1)`, f.planKey)
		f.exec(`DELETE FROM workspace_subscription WHERE plan_key = $1`, f.planKey)
		f.exec(`DELETE FROM plan WHERE key = $1`, f.planKey)
	})
	return f
}

func (f fixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(context.Background(), sql, args...); err != nil {
		f.t.Errorf("cleanup: %v", err)
	}
}

func mustExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %.60s: %v", sql, err)
	}
}

// workspace creates a workspace on the fixture's plan with the given status and
// account kind, and returns its id.
func (f fixture) workspace(s wsSpec) string {
	f.t.Helper()
	ctx := context.Background()
	var ws string
	if err := f.pool.QueryRow(ctx, `INSERT INTO workspace (name, slug) VALUES ($1, $2) RETURNING id::text`,
		"Rating "+randomSuffix(f.t), "rating-"+randomSuffix(f.t)).Scan(&ws); err != nil {
		f.t.Fatal(err)
	}
	mustExec(f.t, f.pool, `INSERT INTO workspace_subscription (workspace_id, plan_key, status, seats, interval,
		current_period_start, current_period_end, cancel_at_period_end, limit_overrides, version, updated_at)
		VALUES ($1, $2, 'active', 0, 'month', '2026-11-01', '2026-12-01', false, '{}', 1, now())`, ws, f.planKey)
	if s.status != "" {
		mustExec(f.t, f.pool, `INSERT INTO platform_workspace (workspace_id, status, reason) VALUES ($1, $2, 'test')`, ws, s.status)
	}
	if s.kind != "" {
		var verifiedAt any
		if s.verified {
			verifiedAt = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
		}
		mustExec(f.t, f.pool, `INSERT INTO billing_profile (workspace_id, account_kind, legal_name, tax_code, address, invoice_email, verified_at, updated_at)
			VALUES ($1, $2, 'Test Co', '0123456789', 'Hanoi', 'billing@example.test', $3, now())`, ws, s.kind, verifiedAt)
	}
	f.t.Cleanup(func() {
		f.exec(`DELETE FROM billing_profile WHERE workspace_id = $1`, ws)
		f.exec(`DELETE FROM platform_workspace WHERE workspace_id = $1`, ws)
	})
	return ws
}

// snapshot writes the day's usage row: 1000 tasks, 1 GiB, 10 members.
func (f fixture) snapshot(ws string, d time.Time) {
	f.t.Helper()
	mustExec(f.t, f.pool, `INSERT INTO platform_usage_daily (workspace_id, day, members, agents, runtimes, issues_total,
		issues_created, tasks_run, storage_bytes, computed_at)
		VALUES ($1, $2, 10, 1, 1, 1, 1, 1000, $3, now())`, ws, d, int64(1)<<30)
}

func (f fixture) charges(ws string, d time.Time) map[string]chargeRow {
	f.t.Helper()
	rows, err := f.pool.Query(context.Background(), `SELECT meter, net_amount, unit_price, per, discount_pct, plan_key, note
		FROM usage_charge WHERE workspace_id = $1 AND day = $2`, ws, d)
	if err != nil {
		f.t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]chargeRow{}
	for rows.Next() {
		var c chargeRow
		if err := rows.Scan(&c.meter, &c.net, &c.unitPrice, &c.per, &c.discountPct, &c.planKey, &c.note); err != nil {
			f.t.Fatal(err)
		}
		out[c.meter] = c
	}
	if err := rows.Err(); err != nil {
		f.t.Fatal(err)
	}
	return out
}

func (f fixture) countCharges(ws string) int {
	f.t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM usage_charge WHERE workspace_id = $1`, ws).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

type chargeRow struct {
	meter       string
	net         int64
	unitPrice   int64
	per         int64
	discountPct int64
	planKey     string
	note        string
}

func TestDBRatingWritesOneRowPerDayAndMeterWithTheRightAmounts(t *testing.T) {
	pool := testdb.Open(t)
	f := newFixture(t, pool)
	ws := f.workspace(wsSpec{})
	d := day("2026-11-14")
	f.snapshot(ws, d)

	if _, err := NewRater(pool, RaterConfig{}).Run(context.Background(), jobNow); err != nil {
		t.Fatal(err)
	}
	got := f.charges(ws, d)
	if len(got) != 3 {
		t.Fatalf("charge rows for the day = %d, want one per meter", len(got))
	}
	// 1000 tasks at 30000 per 1000; 1 GiB-month at 3000 over 30 days; 10 seats
	// at 49000 over 30 days, 16333.33 rounded down.
	want := map[string]int64{MeterTasksRun: 30000, MeterStorageBytes: 100, MeterMembers: 16333}
	for meter, net := range want {
		if got[meter].net != net {
			t.Errorf("%s net = %d, want %d", meter, got[meter].net, net)
		}
		if got[meter].planKey != f.planKey {
			t.Errorf("%s plan key = %q, want %q", meter, got[meter].planKey, f.planKey)
		}
	}
}

func TestDBRunningTwiceOrInParallelWritesNoSecondRow(t *testing.T) {
	pool := testdb.Open(t)
	f := newFixture(t, pool)
	ws := f.workspace(wsSpec{})
	f.snapshot(ws, day("2026-11-14"))
	r := NewRater(pool, RaterConfig{})

	for i := 0; i < 2; i++ {
		if _, err := r.Run(context.Background(), jobNow); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.countCharges(ws); n != 3 {
		t.Fatalf("after two runs: %d rows, want 3", n)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := r.Run(context.Background(), jobNow)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if n := f.countCharges(ws); n != 3 {
		t.Fatalf("after parallel runs: %d rows, want 3", n)
	}
}

func TestDBADayWithoutDataIsNotChargedAndALateRowIsChargedWhenItArrives(t *testing.T) {
	pool := testdb.Open(t)
	f := newFixture(t, pool)
	ws := f.workspace(wsSpec{})
	r := NewRater(pool, RaterConfig{})

	f.snapshot(ws, day("2026-11-10"))
	if _, err := r.Run(context.Background(), jobNow); err != nil {
		t.Fatal(err)
	}
	if n := len(f.charges(ws, day("2026-11-11"))); n != 0 {
		t.Fatalf("a day with no snapshot row has %d charge rows, want 0", n)
	}
	if n := len(f.charges(ws, day("2026-11-10"))); n != 3 {
		t.Fatalf("the snapshotted day has %d charge rows, want 3", n)
	}

	// The 11th's row arrives late: the next run charges it.
	f.snapshot(ws, day("2026-11-11"))
	if _, err := r.Run(context.Background(), jobNow); err != nil {
		t.Fatal(err)
	}
	if n := len(f.charges(ws, day("2026-11-11"))); n != 3 {
		t.Fatalf("the late day has %d charge rows, want 3", n)
	}
	// The 16th is still open: a row for it is not charged until the 16th ends.
	f.snapshot(ws, day("2026-11-16"))
	if _, err := r.Run(context.Background(), jobNow); err != nil {
		t.Fatal(err)
	}
	if n := len(f.charges(ws, day("2026-11-16"))); n != 0 {
		t.Fatalf("an open day has %d charge rows, want 0", n)
	}
}

func TestDBSuspendedAndPendingDeletionWriteZeroRowsAndReadOnlyIsCharged(t *testing.T) {
	pool := testdb.Open(t)
	f := newFixture(t, pool)
	d := day("2026-11-14")
	suspended := f.workspace(wsSpec{status: "suspended"})
	pending := f.workspace(wsSpec{status: "pending_deletion"})
	readOnly := f.workspace(wsSpec{status: "read_only"})
	for _, ws := range []string{suspended, pending, readOnly} {
		f.snapshot(ws, d)
	}
	if _, err := NewRater(pool, RaterConfig{}).Run(context.Background(), jobNow); err != nil {
		t.Fatal(err)
	}

	for _, ws := range []string{suspended, pending} {
		for meter, c := range f.charges(ws, d) {
			if c.net != 0 || c.note != NoteWorkspaceSuspended {
				t.Errorf("%s %s = net %d note %q, want 0 and %q", ws, meter, c.net, c.note, NoteWorkspaceSuspended)
			}
		}
		if n := len(f.charges(ws, d)); n != 3 {
			t.Errorf("suspended workspace has %d rows, want a zero row per meter", n)
		}
	}
	tasks := f.charges(readOnly, d)[MeterTasksRun]
	if tasks.net != 30000 || tasks.note != "" {
		t.Errorf("read_only tasks = net %d note %q, want 30000 and no note", tasks.net, tasks.note)
	}
}

func TestDBThePriceSnapshotStaysWhenThePlanPriceChangesLater(t *testing.T) {
	pool := testdb.Open(t)
	f := newFixture(t, pool)
	ws := f.workspace(wsSpec{})
	d := day("2026-11-14")
	f.snapshot(ws, d)
	r := NewRater(pool, RaterConfig{})
	if _, err := r.Run(context.Background(), jobNow); err != nil {
		t.Fatal(err)
	}

	newRates, _ := json.Marshal(map[string]Rate{MeterTasksRun: {Per: 1000, Price: 99999}})
	mustExec(t, pool, `UPDATE plan SET usage_rates = $2 WHERE key = $1`, f.planKey, newRates)
	inserted, err := r.Run(context.Background(), jobNow)
	if err != nil {
		t.Fatal(err)
	}
	if inserted != 0 {
		t.Fatalf("a rerun after a price change wrote %d rows, want 0", inserted)
	}
	tasks := f.charges(ws, d)[MeterTasksRun]
	if tasks.unitPrice != 30000 || tasks.net != 30000 || tasks.per != 1000 || tasks.planKey != f.planKey {
		t.Fatalf("tasks row after the change = %+v, want the rated snapshot", tasks)
	}
}

func TestDBBusinessDiscountFollowsVerificationWhenTheFlagIsOn(t *testing.T) {
	pool := testdb.Open(t)
	f := newFixture(t, pool)
	verified := f.workspace(wsSpec{kind: "business", verified: true})
	unverified := f.workspace(wsSpec{kind: "business"})
	d := day("2026-11-14")
	f.snapshot(verified, d)
	f.snapshot(unverified, d)
	if _, err := NewRater(pool, RaterConfig{RequireVerification: true}).Run(context.Background(), jobNow); err != nil {
		t.Fatal(err)
	}

	if got := f.charges(verified, d)[MeterTasksRun]; got.discountPct != 20 || got.net != 24000 {
		t.Errorf("verified business tasks = discount %d net %d, want 20 and 24000", got.discountPct, got.net)
	}
	if got := f.charges(unverified, d)[MeterTasksRun]; got.discountPct != 0 || got.net != 30000 {
		t.Errorf("unverified business tasks = discount %d net %d, want 0 and 30000", got.discountPct, got.net)
	}
}

func TestJobRegistersUnderItsNameAndInterval(t *testing.T) {
	if JobName != "usage_rating" {
		t.Fatalf("JobName = %q", JobName)
	}
	if Interval <= 0 {
		t.Fatal("Interval must be positive")
	}
}

// AC39 (spec 7b.4, rule 9d): the rating of day D uses the price in force at
// 00:00 UTC of D, also when the job rates it after a later price change.
func TestDBRatingUsesThePriceInForceAtMidnightOfTheDayEvenWhenRunLate(t *testing.T) {
	pool := testdb.Open(t)
	f := newFixture(t, pool)
	ws := f.workspace(wsSpec{})
	before, from, after := day("2026-11-13"), day("2026-11-14"), day("2026-11-15")
	for _, d := range []time.Time{before, from, after} {
		f.snapshot(ws, d)
	}

	// The tasks price went from 30000 to 99999 per 1000 tasks, effective at
	// 00:00 UTC on 14 November, and the discount from 20 to 0 with it. The job
	// has not run since before the change.
	oldRates, _ := json.Marshal(map[string]Rate{MeterTasksRun: {Per: 1000, Price: 30000}})
	oldTerms, _ := json.Marshal(Terms{DiscountPct: map[string]int64{"business": 20}})
	newRates, _ := json.Marshal(map[string]Rate{MeterTasksRun: {Per: 1000, Price: 99999}})
	mustExec(t, pool, `UPDATE plan SET usage_rates = $2 WHERE key = $1`, f.planKey, newRates)
	mustExec(t, pool, `INSERT INTO plan_price_change (plan_key, state, effective_from, old, new, reason, applied_at)
		VALUES ($1, 'applied', $2, jsonb_build_object('usage_rates', $3::jsonb, 'usage_terms', $4::jsonb),
		        jsonb_build_object('usage_rates', $5::jsonb), 'test', $2)`,
		f.planKey, from, oldRates, oldTerms, newRates)
	t.Cleanup(func() { f.exec(`DELETE FROM plan_price_change WHERE plan_key = $1`, f.planKey) })

	if _, err := NewRater(pool, RaterConfig{}).Run(context.Background(), jobNow); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name  string
		day   time.Time
		price int64
		net   int64
	}{
		{"the day before the change", before, 30000, 30000},
		{"the day the change takes effect", from, 99999, 99999},
		{"the day after", after, 99999, 99999},
	} {
		got := f.charges(ws, c.day)[MeterTasksRun]
		if got.unitPrice != c.price || got.net != c.net {
			t.Errorf("%s: unit price %d net %d, want %d and %d", c.name, got.unitPrice, got.net, c.price, c.net)
		}
	}
}
