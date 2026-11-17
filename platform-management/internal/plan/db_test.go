package plan

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/multica-ai/multica/platform-management/internal/db"
	"github.com/multica-ai/multica/platform-management/internal/testdb"
)

// Database tests: skipped unless PLATFORM_TEST_DATABASE_URL names a *_test
// database and DATABASE_URL equals it.

// savepointPool hands out nested transactions of one outer transaction, so a
// test can empty the plan table, seed it and roll everything back.
type savepointPool struct{ outer pgx.Tx }

func (p savepointPool) Begin(ctx context.Context) (pgx.Tx, error) { return p.outer.Begin(ctx) }

func randomKey(t *testing.T) string {
	t.Helper()
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return "t" + hex.EncodeToString(b)
}

// An empty plan table takes the whole embedded catalogue.
func TestDBSeedLoadsTheCatalogueIntoAnEmptyTable(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	outer, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer outer.Rollback(ctx) // leaves the shared table as it was

	if _, err := outer.Exec(ctx, `DELETE FROM plan`); err != nil {
		t.Fatal(err)
	}
	runner := NewPgxRunner(savepointPool{outer})
	n, err := Seed(ctx, runner, discard())
	if err != nil {
		t.Fatal(err)
	}
	want, _ := Catalogue()
	if n != len(want) {
		t.Fatalf("seeded %d, want %d", n, len(want))
	}

	got, err := NewPgxReader(outer).List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("table has %d plans, want %d", len(got), len(want))
	}
	for i, p := range want {
		if got[i].Key != p.Key || got[i].Name != p.Name || got[i].Currency != p.Currency || got[i].Sort != p.Sort {
			t.Errorf("row %d = %+v, want plan %s", i, got[i], p.Key)
		}
		if (got[i].SeatPriceMonth == nil) != (p.SeatPriceMonth == nil) || (p.SeatPriceMonth != nil && *got[i].SeatPriceMonth != *p.SeatPriceMonth) {
			t.Errorf("%s: seat_price_month = %v, want %v", p.Key, got[i].SeatPriceMonth, p.SeatPriceMonth)
		}
	}
	for i, p := range want {
		if got[i].Audience != p.Audience || got[i].BillingModel != p.BillingModel || got[i].Version != 0 {
			t.Errorf("%s: audience/billing_model/version = %q/%q/%d, want %q/%q/0", p.Key, got[i].Audience, got[i].BillingModel, got[i].Version, p.Audience, p.BillingModel)
		}
		gotGroups, wantGroups := got[i].Limits[GateMemberGroups], p.Limits[GateMemberGroups]
		if (gotGroups == nil) != (wantGroups == nil) || (wantGroups != nil && *gotGroups != *wantGroups) {
			t.Errorf("%s: member_groups = %v, want %v", p.Key, gotGroups, wantGroups)
		}
	}
	if free, ok, _ := NewPgxReader(outer).Get(ctx, "free"); !ok || free.SeatsFixed == nil || *free.SeatsFixed != 3 {
		t.Errorf("free = %+v, want 3 fixed seats", free)
	}
	// A limit that is unlimited in the catalogue is a null in the table.
	if pro, _, _ := NewPgxReader(outer).Get(ctx, "pro"); pro.Limits["issue_count"] != nil {
		t.Errorf("pro issue_count = %v, want unlimited", *pro.Limits["issue_count"])
	}
}

// A table with any row is left as it is.
// A table that holds only free gains the paid plans, and keeps the free row staff edited.
// A seeded payg row carries the usage prices and terms of spec 7b.3 in its
// jsonb columns, and a seat plan keeps the empty default (AC32).
func TestDBSeedStoresThePaygUsagePrices(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	outer, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer outer.Rollback(ctx)

	if _, err := outer.Exec(ctx, `DELETE FROM plan`); err != nil {
		t.Fatal(err)
	}
	if _, err := outer.Exec(ctx, `DELETE FROM audit_event WHERE action = 'plan.changed' AND target_type = 'plan' AND detail->>'deleted' = 'true'`); err != nil {
		t.Fatal(err)
	}
	if _, err := Seed(ctx, NewPgxRunner(savepointPool{outer}), discard()); err != nil {
		t.Fatal(err)
	}
	var billing, rates, terms string
	err = outer.QueryRow(ctx, `SELECT billing_model, usage_rates::text, usage_terms::text FROM plan WHERE key = 'payg'`).Scan(&billing, &rates, &terms)
	if err != nil {
		t.Fatalf("payg row: %v", err)
	}
	if billing != BillingUsage {
		t.Errorf("payg billing_model = %q, want usage", billing)
	}
	var ok bool
	err = outer.QueryRow(ctx, `SELECT usage_rates = '{"tasks_run": {"per": 1000, "price": 30000}, "storage_bytes": {"per": 1073741824, "price": 3000}, "members": {"per": 1, "price": 49000}}'::jsonb
		AND usage_terms = '{"discount_pct": {"business": 20}, "credit_limit": {"individual": 2000000, "business": 20000000}}'::jsonb
		FROM plan WHERE key = 'payg'`).Scan(&ok)
	if err != nil || !ok {
		t.Errorf("payg usage_rates = %s, usage_terms = %s (%v), want the spec 7b.3 values", rates, terms, err)
	}
	var seatRates string
	if err := outer.QueryRow(ctx, `SELECT usage_rates::text FROM plan WHERE key = 'pro'`).Scan(&seatRates); err != nil || seatRates != "{}" {
		t.Errorf("pro usage_rates = %q (%v), want {}", seatRates, err)
	}
}

func TestDBSeedAddsMissingPlansAndKeepsEditedRows(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	outer, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer outer.Rollback(ctx)

	if _, err := outer.Exec(ctx, `DELETE FROM plan`); err != nil {
		t.Fatal(err)
	}
	if _, err := outer.Exec(ctx, `DELETE FROM audit_event WHERE action = 'plan.changed' AND target_type = 'plan' AND detail->>'deleted' = 'true'`); err != nil {
		t.Fatal(err)
	}
	if _, err := outer.Exec(ctx, `INSERT INTO plan (key, name, currency) VALUES ('free', 'Free (edited)', 'vnd')`); err != nil {
		t.Fatal(err)
	}
	n, err := Seed(ctx, NewPgxRunner(savepointPool{outer}), discard())
	if err != nil {
		t.Fatal(err)
	}
	// payg is part of the spec's initial set (7b.3).
	if n != 5 {
		t.Fatalf("seeded %d plans into a table that had one, want 5", n)
	}
	var freeName string
	if err := outer.QueryRow(ctx, `SELECT name FROM plan WHERE key = 'free'`).Scan(&freeName); err != nil || freeName != "Free (edited)" {
		t.Fatalf("free name = %q (%v), want the staff edit kept", freeName, err)
	}
	var count int
	if err := outer.QueryRow(ctx, `SELECT count(*) FROM plan WHERE key IN ('free', 'pro', 'business', 'enterprise', 'unlimited', 'payg')`).Scan(&count); err != nil || count != 6 {
		t.Fatalf("catalogue plans = %d (%v), want 6", count, err)
	}
	// A row written before migration 953 reads as an any-audience seat plan at
	// version 0, and the seed leaves it so.
	free, ok, err := NewPgxReader(outer).Get(ctx, "free")
	if err != nil || !ok {
		t.Fatalf("Get free = %v, %v", ok, err)
	}
	if free.Audience != AudienceAny || free.BillingModel != BillingSeat || free.Version != 0 {
		t.Errorf("free audience/billing_model/version = %q/%q/%d, want any/seat/0", free.Audience, free.BillingModel, free.Version)
	}
	biz, ok, err := NewPgxReader(outer).Get(ctx, "business")
	if err != nil || !ok || biz.Audience != AudienceBusiness || biz.Version != 0 {
		t.Errorf("business = %+v (%v, %v), want the catalogue's audience business at version 0", biz, ok, err)
	}
}

// A row staff edited keeps its R5 values: the seed never overwrites an existing row.
func TestDBSeedKeepsTheAudienceOfAnExistingRow(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	outer, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer outer.Rollback(ctx)

	if _, err := outer.Exec(ctx, `DELETE FROM plan`); err != nil {
		t.Fatal(err)
	}
	if _, err := outer.Exec(ctx, `DELETE FROM audit_event WHERE action = 'plan.changed' AND target_type = 'plan' AND detail->>'deleted' = 'true'`); err != nil {
		t.Fatal(err)
	}
	if _, err := outer.Exec(ctx, `INSERT INTO plan (key, name, currency, audience, version) VALUES ('business', 'Business', 'vnd', 'individual', 7)`); err != nil {
		t.Fatal(err)
	}
	if _, err := Seed(ctx, NewPgxRunner(savepointPool{outer}), discard()); err != nil {
		t.Fatal(err)
	}
	biz, _, err := NewPgxReader(outer).Get(ctx, "business")
	if err != nil {
		t.Fatal(err)
	}
	if biz.Audience != AudienceIndividual || biz.Version != 7 || biz.Limits[GateMemberGroups] != nil {
		t.Errorf("business audience/version/limits = %q/%d/%v, want the existing row untouched", biz.Audience, biz.Version, biz.Limits)
	}
}

// A plan with a deletion row in the audit log is not restored by the seed.
func TestDBSeedDoesNotRestoreADeletedPlan(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	outer, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer outer.Rollback(ctx)

	if _, err := outer.Exec(ctx, `DELETE FROM plan`); err != nil {
		t.Fatal(err)
	}
	if _, err := outer.Exec(ctx, `DELETE FROM audit_event WHERE action = 'plan.changed' AND target_type = 'plan' AND detail->>'deleted' = 'true'`); err != nil {
		t.Fatal(err)
	}
	if _, err := outer.Exec(ctx, `INSERT INTO audit_event (actor_type, actor_label, action, target_type, target_id, target_label, detail, ip, user_agent, request_id)
		VALUES ('staff', 'test', 'plan.changed', 'plan', 'enterprise', 'Enterprise', '{"deleted": true}', '', '', '')`); err != nil {
		t.Fatal(err)
	}
	if _, err := Seed(ctx, NewPgxRunner(savepointPool{outer}), discard()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := outer.QueryRow(ctx, `SELECT count(*) FROM plan WHERE key = 'enterprise'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("enterprise rows = %d (%v), want 0: the deleted plan came back", count, err)
	}
}

// The unique index on key makes a second insert of a plan a no-op.
func TestDBInsertIfAbsentKeepsTheFirstRow(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	key := randomKey(t)
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM plan WHERE key = $1`, key) })

	runner := NewPgxRunner(pool)
	first := Plan{Key: key, Name: "First", Currency: "vnd", Limits: Limits{}, Features: map[string]bool{}}
	second := first
	second.Name = "Second"
	for _, tc := range []struct {
		p    Plan
		want bool
	}{{first, true}, {second, false}} {
		err := runner.InTx(ctx, func(tx Tx) error {
			ok, err := tx.InsertIfAbsent(ctx, tc.p)
			if ok != tc.want {
				t.Errorf("InsertIfAbsent(%s) = %v, want %v", tc.p.Name, ok, tc.want)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if got, _, _ := NewPgxReader(pool).Get(ctx, key); got.Name != "First" {
		t.Fatalf("name = %q, want the first row kept", got.Name)
	}
}

func TestDBPlanRoundTripThroughTheTable(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	key := randomKey(t)
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM plan WHERE key = $1`, key) })

	archived := time.Date(2030, 1, 2, 3, 4, 5, 123456000, time.UTC)
	seats := 4
	want := Plan{
		Key: key, Name: "Round trip", Currency: "vnd",
		Limits:         Limits{"issue_count": nil, "autopilot_runs": i64(9), "storage_bytes": i64(1 << 40)},
		Features:       map[string]bool{"sso": true, "audit_export": false},
		SeatPriceMonth: i64(1000), SeatPriceYear: nil, SeatsFixed: &seats, TrialDays: 3, IsPublic: true, Sort: 7, ArchivedAt: &archived,
	}
	runner := NewPgxRunner(pool)
	if err := runner.InTx(ctx, func(tx Tx) error { return tx.Upsert(ctx, want) }); err != nil {
		t.Fatal(err)
	}
	got, ok, err := NewPgxReader(pool).Get(ctx, key)
	if err != nil || !ok {
		t.Fatalf("Get = %v, %v", ok, err)
	}
	if v, present := got.Limits["issue_count"]; !present || v != nil {
		t.Errorf("issue_count = %v (present %v), want an explicit null", v, present)
	}
	if got.Name != want.Name || *got.Limits["autopilot_runs"] != 9 || *got.Limits["storage_bytes"] != 1<<40 ||
		!got.Features["sso"] || *got.SeatPriceMonth != 1000 || got.SeatPriceYear != nil || *got.SeatsFixed != 4 ||
		got.TrialDays != 3 || !got.IsPublic || got.Sort != 7 || got.ArchivedAt == nil || !got.ArchivedAt.Equal(archived) {
		t.Errorf("got %+v", got)
	}

	if err := runner.InTx(ctx, func(tx Tx) error {
		if n, err := tx.CountSubscriptions(ctx, key); err != nil || n != 0 {
			t.Errorf("CountSubscriptions = %d, %v", n, err)
		}
		if ok, err := tx.Delete(ctx, key); err != nil || !ok {
			t.Errorf("Delete = %v, %v", ok, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := NewPgxReader(pool).Get(ctx, key); ok {
		t.Fatal("the plan is still there")
	}
}

func upsertPlan(t *testing.T, r *PgxRunner, p Plan) {
	t.Helper()
	if err := r.InTx(context.Background(), func(tx Tx) error { return tx.Upsert(context.Background(), p) }); err != nil {
		t.Fatal(err)
	}
}

func readPlan(t *testing.T, pool db.DBTX, key string) Plan {
	t.Helper()
	got, ok, err := NewPgxReader(pool).Get(context.Background(), key)
	if err != nil || !ok {
		t.Fatalf("Get %s = %v, %v", key, ok, err)
	}
	return got
}

// version is 0 on creation and goes up by one on every edit, whatever the
// caller passes.
func TestDBVersionIncreasesByOnePerEdit(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	key := randomKey(t)
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM plan WHERE key = $1`, key) })
	runner := NewPgxRunner(pool)

	p := Plan{Key: key, Name: "V", Currency: "vnd", Limits: Limits{}, Features: map[string]bool{}, Audience: AudienceBusiness, BillingModel: BillingSeat}
	upsertPlan(t, runner, p)
	if v := readPlan(t, pool, key).Version; v != 0 {
		t.Fatalf("version after creation = %d, want 0", v)
	}
	for want := int64(1); want <= 3; want++ {
		p.Name = fmt.Sprintf("V%d", want)
		p.Version = 99 // ignored: the table owns the number
		upsertPlan(t, runner, p)
		if got := readPlan(t, pool, key); got.Version != want || got.Name != p.Name {
			t.Fatalf("after edit %d: version %d name %q, want version %d", want, got.Version, got.Name, want)
		}
	}

	// Seeding a row that exists is not an edit.
	err := runner.InTx(ctx, func(tx Tx) error {
		ok, err := tx.InsertIfAbsent(ctx, p)
		if ok {
			t.Error("InsertIfAbsent reported an insert for an existing key")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if v := readPlan(t, pool, key).Version; v != 3 {
		t.Fatalf("version after InsertIfAbsent = %d, want 3", v)
	}
}

func TestDBAudienceAndBillingModelRoundTripAndAnUnsetFieldKeepsTheStoredValue(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	key := randomKey(t)
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM plan WHERE key = $1`, key) })
	runner := NewPgxRunner(pool)

	p := Plan{
		Key: key, Name: "R5", Currency: "vnd", Features: map[string]bool{},
		Limits: Limits{GateMemberGroups: i64(50), "issue_count": nil}, Audience: AudienceBusiness, BillingModel: BillingSeat,
	}
	upsertPlan(t, runner, p)
	got := readPlan(t, pool, key)
	if got.Audience != AudienceBusiness || got.BillingModel != BillingSeat || got.Limits[GateMemberGroups] == nil || *got.Limits[GateMemberGroups] != 50 {
		t.Fatalf("got %+v", got)
	}

	// The console's plan form does not carry the R5 fields yet; an edit through
	// it must not reset them.
	p.Name, p.Audience, p.BillingModel = "R5 edited", "", ""
	upsertPlan(t, runner, p)
	got = readPlan(t, pool, key)
	if got.Name != "R5 edited" || got.Audience != AudienceBusiness || got.BillingModel != BillingSeat {
		t.Fatalf("after an edit with the fields unset: %+v, want the stored audience and billing model kept", got)
	}

	// A set field is written.
	p.Audience = AudienceIndividual
	upsertPlan(t, runner, p)
	if got = readPlan(t, pool, key); got.Audience != AudienceIndividual || got.Version != 2 {
		t.Fatalf("audience %q version %d, want individual and 2", got.Audience, got.Version)
	}

	// A plan created with the fields unset takes the defaults.
	bare := randomKey(t)
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM plan WHERE key = $1`, bare) })
	upsertPlan(t, runner, Plan{Key: bare, Name: "Bare", Currency: "vnd", Limits: Limits{}, Features: map[string]bool{}})
	if got = readPlan(t, pool, bare); got.Audience != AudienceAny || got.BillingModel != BillingSeat {
		t.Fatalf("a plan created with the fields unset: %q/%q, want any/seat", got.Audience, got.BillingModel)
	}
}
