package plan

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

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
	if n != 3 {
		t.Fatalf("seeded %d plans into a table that had one, want 3", n)
	}
	var freeName string
	if err := outer.QueryRow(ctx, `SELECT name FROM plan WHERE key = 'free'`).Scan(&freeName); err != nil || freeName != "Free (edited)" {
		t.Fatalf("free name = %q (%v), want the staff edit kept", freeName, err)
	}
	var count int
	if err := outer.QueryRow(ctx, `SELECT count(*) FROM plan WHERE key IN ('free', 'pro', 'business', 'enterprise')`).Scan(&count); err != nil || count != 4 {
		t.Fatalf("catalogue plans = %d (%v), want 4", count, err)
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
