package pricechange

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/testdb"
)

// Database tests: skipped unless PLATFORM_TEST_DATABASE_URL names a *_test
// database and DATABASE_URL equals it. Each test uses its own plan key and
// workspaces, asserts on its own rows only, and removes what it wrote.

type dbFixture struct {
	t    *testing.T
	pool *pgxpool.Pool
	key  string
	mail *fakeMail
	svc  *Service
	// paidWS and pastDueWS have a paid subscription; trialWS has none. owners
	// maps a workspace to its owner's address.
	paidWS, pastDueWS, trialWS string
	owners                     map[string]string
}

func newDBFixture(t *testing.T) *dbFixture {
	t.Helper()
	pool := testdb.Open(t)
	ctx := context.Background()
	b := make([]byte, 5)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	suffix := hex.EncodeToString(b)
	f := &dbFixture{t: t, pool: pool, key: "pc" + suffix, mail: &fakeMail{}, owners: map[string]string{}}
	f.exec(`INSERT INTO plan (key, name, currency, seat_price_month, seat_price_year, usage_rates, usage_terms)
	        VALUES ($1, 'Price change test', 'vnd', 100000, 1000000, '{"tasks_run":{"per":1000,"price":30000}}', '{}')`, f.key)
	var users, workspaces []string
	for i, st := range []struct{ status, field string }{{"active", "paidWS"}, {"past_due", "pastDueWS"}, {"trialing", "trialWS"}} {
		var ws, user string
		f.query(&ws, `INSERT INTO workspace (name, slug) VALUES ($1, $2) RETURNING id::text`, "PC "+st.status, "pc-"+suffix+"-"+st.status)
		email := st.status + "-" + suffix + "@example.com"
		f.query(&user, `INSERT INTO "user" (name, email) VALUES ($1, $2) RETURNING id::text`, "Owner "+st.status, email)
		f.exec(`INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`, ws, user)
		f.exec(`INSERT INTO workspace_subscription (workspace_id, plan_key, status, seats) VALUES ($1, $2, $3, 3)`, ws, f.key, st.status)
		f.owners[ws] = email
		users, workspaces = append(users, user), append(workspaces, ws)
		switch i {
		case 0:
			f.paidWS = ws
		case 1:
			f.pastDueWS = ws
		default:
			f.trialWS = ws
		}
	}
	t.Cleanup(func() {
		for _, q := range []string{
			`DELETE FROM plan_price_change WHERE plan_key = $1`,
			`DELETE FROM audit_event WHERE target_type = 'plan' AND target_id = $1`,
			`DELETE FROM plan WHERE key = $1`,
		} {
			pool.Exec(ctx, q, f.key)
		}
		pool.Exec(ctx, `DELETE FROM billing_notice WHERE workspace_id = ANY($1::uuid[])`, workspaces)
		pool.Exec(ctx, `DELETE FROM workspace_subscription WHERE workspace_id = ANY($1::uuid[])`, workspaces)
		pool.Exec(ctx, `DELETE FROM member WHERE workspace_id = ANY($1::uuid[])`, workspaces)
		pool.Exec(ctx, `DELETE FROM "user" WHERE id = ANY($1::uuid[])`, users)
		pool.Exec(ctx, `DELETE FROM workspace WHERE id = ANY($1::uuid[])`, workspaces)
	})
	svc, err := New(Deps{Store: NewPgxStore(pool), Mail: f.mail, AppURL: "https://app.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	f.svc = svc
	return f
}

func (f *dbFixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(context.Background(), sql, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *dbFixture) query(dst any, sql string, args ...any) {
	f.t.Helper()
	if err := f.pool.QueryRow(context.Background(), sql, args...).Scan(dst); err != nil {
		f.t.Fatal(err)
	}
}

func (f *dbFixture) planState() (month, year int64, version int64) {
	f.t.Helper()
	f.query(&month, `SELECT seat_price_month FROM plan WHERE key = $1`, f.key)
	f.query(&year, `SELECT seat_price_year FROM plan WHERE key = $1`, f.key)
	f.query(&version, `SELECT version FROM plan WHERE key = $1`, f.key)
	return
}

func (f *dbFixture) count(sql string, args ...any) int {
	f.t.Helper()
	var n int
	f.query(&n, sql, args...)
	return n
}

// The partial unique index (migration 965) exists under its name and covers
// only scheduled rows.
func TestDBMigrationsCreateTheTableAndTheIndex(t *testing.T) {
	f := newDBFixture(t)
	if n := f.count(`SELECT count(*) FROM information_schema.columns WHERE table_name = 'plan_price_change'
	                 AND column_name IN ('id','plan_key','state','effective_from','old','new','reason','notice_sent_at',
	                                     'created_by','created_at','applied_at','canceled_by','canceled_at')`); n != 13 {
		t.Errorf("plan_price_change has %d of the 13 columns", n)
	}
	var def string
	f.query(&def, `SELECT indexdef FROM pg_indexes WHERE tablename = 'plan_price_change' AND indexname = 'plan_price_change_scheduled_uidx'`)
	if def == "" || !strings.Contains(def, "UNIQUE") || !strings.Contains(def, "(plan_key)") || !strings.Contains(def, "scheduled") {
		t.Errorf("index definition = %q", def)
	}
}

// The index alone stops a second scheduled row, even for a writer that did not
// take the plan row lock (spec 7b.4 rule 3).
func TestDBUniqueIndexRefusesASecondScheduledRow(t *testing.T) {
	f := newDBFixture(t)
	ctx := context.Background()
	store := NewPgxStore(f.pool)
	insert := func() error {
		return store.InTx(ctx, func(tx Tx) error {
			_, err := tx.Insert(ctx, Change{
				PlanKey: f.key, EffectiveFrom: t0, Reason: "r",
				Old: Prices{FieldSeatPriceMonth: raw("1")}, New: Prices{FieldSeatPriceMonth: raw("2")},
			})
			return err
		})
	}
	if err := insert(); err != nil {
		t.Fatal(err)
	}
	if err := insert(); !errors.Is(err, ErrPending) {
		t.Fatalf("second insert error = %v, want ErrPending", err)
	}
	// A canceled row does not hold the slot.
	if _, err := f.svc.Cancel(ctx, admin, f.key, t0); err != nil {
		t.Fatal(err)
	}
	if err := insert(); err != nil {
		t.Fatalf("insert after cancel: %v", err)
	}
}

func TestDBPaidWorkspacesAndHistory(t *testing.T) {
	f := newDBFixture(t)
	ctx := context.Background()
	store := NewPgxStore(f.pool)
	got, err := store.PaidWorkspaces(ctx, f.key)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, w := range got {
		ids = append(ids, w.ID)
	}
	slices.Sort(ids)
	want := []string{f.paidWS, f.pastDueWS}
	slices.Sort(want)
	if !slices.Equal(ids, want) {
		t.Errorf("paid workspaces = %v, want the active and past_due ones %v", ids, want)
	}
	// A workspace pending deletion is flagged.
	f.exec(`INSERT INTO platform_workspace (workspace_id, status, reason) VALUES ($1, 'pending_deletion', 'test')`, f.pastDueWS)
	t.Cleanup(func() { f.pool.Exec(ctx, `DELETE FROM platform_workspace WHERE workspace_id = $1`, f.pastDueWS) })
	got, _ = store.PaidWorkspaces(ctx, f.key)
	for _, w := range got {
		if w.PendingDeletion != (w.ID == f.pastDueWS) {
			t.Errorf("workspace %s pending_deletion = %v", w.ID, w.PendingDeletion)
		}
	}
}

// Test 5: the apply job, in parallel, applies once with one version bump.
func TestDBParallelApplyAppliesOnceWithOneVersionBump(t *testing.T) {
	f := newDBFixture(t)
	ctx := context.Background()
	effective := t0.Add(31 * 24 * time.Hour)
	if _, err := f.svc.Schedule(ctx, admin, f.key, seatReq(effective, "120000", "1200000"), false, t0); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var failed atomic.Int32
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := f.svc.Run(ctx, effective); err != nil {
				failed.Add(1)
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	month, year, version := f.planState()
	if month != 120000 || year != 1200000 || version != 1 {
		t.Errorf("plan = month %d year %d version %d, want 120000, 1200000 and 1 (one bump from 0)", month, year, version)
	}
	if n := f.count(`SELECT count(*) FROM audit_event WHERE action = $1 AND target_id = $2`, audit.ActionPlanPriceApplied, f.key); n != 1 {
		t.Errorf("%d applied audit rows, want 1", n)
	}
	if n := f.count(`SELECT count(*) FROM plan_price_change WHERE plan_key = $1 AND state = 'applied'`, f.key); n != 1 {
		t.Errorf("%d applied rows, want 1", n)
	}
	if err := f.svc.Run(ctx, effective.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, _, v := f.planState(); v != 1 {
		t.Errorf("version = %d after a later run, want 1", v)
	}
}

// Rule 3: two admins submitting at once, one wins.
func TestDBConcurrentSchedulesLeaveOneScheduledChange(t *testing.T) {
	f := newDBFixture(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	var ok, pending atomic.Int32
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.svc.Schedule(ctx, admin, f.key, seatReq(t0.Add(31*24*time.Hour), "120000", "1200000"), false, t0)
			var e *Error
			switch {
			case err == nil:
				ok.Add(1)
			case errors.As(err, &e) && e.Code == "price_change_pending":
				pending.Add(1)
			default:
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 1 || pending.Load() != 5 {
		t.Errorf("%d succeeded, %d pending; want 1 and 5", ok.Load(), pending.Load())
	}
	if n := f.count(`SELECT count(*) FROM plan_price_change WHERE plan_key = $1`, f.key); n != 1 {
		t.Errorf("%d rows stored, want 1", n)
	}
}

// A usage change at 00:00 UTC is applied to the jsonb columns, and the seat
// prices stay as they were.
func TestDBApplyWritesTheUsageFields(t *testing.T) {
	f := newDBFixture(t)
	ctx := context.Background()
	f.exec(`UPDATE plan SET billing_model = 'usage', seat_price_month = NULL, seat_price_year = NULL WHERE key = $1`, f.key)
	at := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	req := Request{EffectiveFrom: at, Reason: "r", New: Prices{
		FieldUsageRates: raw(`{"tasks_run":{"per":1000,"price":35000}}`),
		FieldUsageTerms: raw(`{"discount_pct":{"business":25}}`),
	}}
	res, err := f.svc.Schedule(ctx, admin, f.key, req, false, t0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Change.State != StateScheduled {
		t.Fatalf("state = %s", res.Change.State)
	}
	if err := f.svc.Run(ctx, at); err != nil {
		t.Fatal(err)
	}
	var rates, terms []byte
	f.query(&rates, `SELECT usage_rates FROM plan WHERE key = $1`, f.key)
	f.query(&terms, `SELECT usage_terms FROM plan WHERE key = $1`, f.key)
	var r map[string]map[string]int64
	if err := json.Unmarshal(rates, &r); err != nil || r["tasks_run"]["price"] != 35000 {
		t.Errorf("usage_rates = %s", rates)
	}
	if string(terms) != `{"discount_pct": {"business": 25}}` {
		t.Errorf("usage_terms = %s", terms)
	}
	var month *int64
	f.query(&month, `SELECT seat_price_month FROM plan WHERE key = $1`, f.key)
	if month != nil {
		t.Errorf("seat_price_month = %d, want NULL", *month)
	}
}

// Tests 6 and 7 on the real tables: the notice goes to each owner once, a cancel
// after it sends one correction, and the history lists the change as canceled.
func TestDBNoticeThenCancelSendsOneCorrection(t *testing.T) {
	f := newDBFixture(t)
	ctx := context.Background()
	effective := t0.Add(31 * 24 * time.Hour)
	if _, err := f.svc.Schedule(ctx, admin, f.key, seatReq(effective, "120000", "1200000"), false, t0); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := f.svc.Run(ctx, t0); err != nil {
			t.Fatal(err)
		}
	}
	wantMail := []string{f.owners[f.paidWS], f.owners[f.pastDueWS]}
	slices.Sort(wantMail)
	if got := f.mail.recipients(); !slices.Equal(got, wantMail) {
		t.Fatalf("notice recipients = %v, want %v", got, wantMail)
	}
	f.mail.sent = nil
	if _, err := f.svc.Cancel(ctx, admin, f.key, t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got := f.mail.recipients(); !slices.Equal(got, wantMail) {
		t.Fatalf("correction recipients = %v, want %v", got, wantMail)
	}
	if month, _, version := f.planState(); month != 100000 || version != 0 {
		t.Errorf("plan = month %d version %d, want the old price and version 0", month, version)
	}
	hist, err := f.svc.History(ctx, f.key)
	if err != nil || len(hist) != 1 || hist[0].State != StateCanceled || hist[0].CanceledBy != admin.ID {
		t.Fatalf("history = %+v, %v", hist, err)
	}
	for _, action := range []string{audit.ActionPlanPriceScheduled, audit.ActionPlanPriceCanceled} {
		if n := f.count(`SELECT count(*) FROM audit_event WHERE action = $1 AND target_id = $2`, action, f.key); n != 1 {
			t.Errorf("%d %s rows, want 1", n, action)
		}
	}
}

// Spec R6 section 7b.4 rule 2 (D88): a scheduled price change moves the plan's
// prices and nothing else. The paid period and the order already created keep
// their values.
func TestPriceChangeDoesNotTouchPaidPeriodsOrCreatedOrders(t *testing.T) {
	f := newDBFixture(t)
	ctx := context.Background()
	periodStart := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	f.exec(`UPDATE workspace_subscription SET current_period_start = $2, current_period_end = $3, version = 7 WHERE workspace_id = $1`, f.paidWS, periodStart, periodEnd)
	var orderID string
	f.query(&orderID, `INSERT INTO billing_order (id, workspace_id, kind, plan_key, interval, seats, amount, currency, status, provider, transfer_code)
	        VALUES (gen_random_uuid(), $1, 'subscribe', $2, 'month', 3, 300000, 'vnd', 'paid', 'test', 'PC-ORDER') RETURNING id::text`, f.paidWS, f.key)
	t.Cleanup(func() { f.pool.Exec(ctx, `DELETE FROM billing_order WHERE id = $1`, orderID) })

	at := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	if _, err := f.svc.Schedule(ctx, admin, f.key, seatReq(at, "150000", "1500000"), false, t0); err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	if err := f.svc.Run(ctx, at); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if month, _, _ := f.planState(); month != 150000 {
		t.Fatalf("plan month price = %d, want the new 150000", month)
	}

	var gotStart, gotEnd time.Time
	var version int64
	f.query(&gotStart, `SELECT current_period_start FROM workspace_subscription WHERE workspace_id = $1`, f.paidWS)
	f.query(&gotEnd, `SELECT current_period_end FROM workspace_subscription WHERE workspace_id = $1`, f.paidWS)
	f.query(&version, `SELECT version FROM workspace_subscription WHERE workspace_id = $1`, f.paidWS)
	if !gotStart.Equal(periodStart) || !gotEnd.Equal(periodEnd) || version != 7 {
		t.Fatalf("paid period changed: start %v end %v version %d", gotStart, gotEnd, version)
	}
	var amount int64
	f.query(&amount, `SELECT amount FROM billing_order WHERE id = $1`, orderID)
	if amount != 300000 {
		t.Fatalf("created order amount = %d, want 300000", amount)
	}
}
