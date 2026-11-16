package billing

import (
	"context"
	"errors"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/testdb"
	"github.com/multica-ai/multica/platform-management/workspacestate"
)

// Database tests: skipped unless PLATFORM_TEST_DATABASE_URL names a *_test
// database and DATABASE_URL equals it. Each test uses a fresh random workspace
// id (the tables have no foreign key) and removes the rows it wrote.

type dbFixture struct {
	t     *testing.T
	pool  *pgxpool.Pool
	ws    string
	staff string
	user  string
	inv   *invalidations
}

func newDBFixture(t *testing.T) *dbFixture {
	t.Helper()
	pool := testdb.Open(t)
	seedCatalogue(t, pool)
	f := &dbFixture{t: t, pool: pool, inv: &invalidations{}}
	ctx := context.Background()
	for _, dst := range []*string{&f.ws, &f.staff, &f.user} {
		if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, q := range []string{
			`DELETE FROM billing_order WHERE workspace_id = $1`,
			`DELETE FROM workspace_subscription WHERE workspace_id = $1`,
			`DELETE FROM platform_workspace WHERE workspace_id = $1`,
			`DELETE FROM audit_event WHERE workspace_id = $1`,
		} {
			pool.Exec(ctx, q, f.ws)
		}
	})
	return f
}

func (f *dbFixture) service(runner Runner, now func() time.Time) *Service {
	f.t.Helper()
	if runner == nil {
		runner = NewPgxRunner(f.pool)
	}
	s, err := New(Deps{
		Runner: runner, Provider: ProviderManual, Workspaces: f.inv, Entitlements: entCache{f.inv}, Now: now,
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return s
}

func (f *dbFixture) draft(key string) Draft {
	start, end := time.Now().UTC().Truncate(time.Second), time.Now().UTC().Truncate(time.Second).AddDate(0, 1, 0)
	return Draft{
		WorkspaceID: f.ws, Kind: KindSubscribe, PlanKey: "pro", Interval: "month", Seats: 3,
		Amount: 447000, VATAmount: 44700, Currency: "vnd", PeriodStart: &start, PeriodEnd: &end,
		IdempotencyKey: key, CreatedBy: f.user,
	}
}

func (f *dbFixture) seedSubscription(version int64) {
	f.t.Helper()
	if _, err := f.pool.Exec(context.Background(),
		`INSERT INTO workspace_subscription (workspace_id, plan_key, status, seats, interval, version)
		 VALUES ($1, 'free', 'active', 3, 'month', $2)`, f.ws, version); err != nil {
		f.t.Fatal(err)
	}
}

func (f *dbFixture) subscriptionVersion() (version int64, plan string) {
	f.t.Helper()
	if err := f.pool.QueryRow(context.Background(),
		`SELECT version, plan_key FROM workspace_subscription WHERE workspace_id = $1`, f.ws).Scan(&version, &plan); err != nil {
		f.t.Fatal(err)
	}
	return version, plan
}

func (f *dbFixture) count(query string, args ...any) int {
	f.t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

// AC12: confirming an order twice applies it once.
func TestDBConfirmingTwiceAppliesOnce(t *testing.T) {
	f := newDBFixture(t)
	f.seedSubscription(4)
	s := f.service(nil, nil)
	ctx := context.Background()
	o, created, err := s.CreateOrder(ctx, f.draft("k"))
	if err != nil || !created {
		t.Fatalf("CreateOrder = %v, %v", created, err)
	}

	first, err := s.ApplyPayment(ctx, Confirmation{OrderID: o.ID, ConfirmedBy: f.staff})
	if err != nil || first.AlreadyPaid || first.Order.Status != StatusPaid {
		t.Fatalf("first ApplyPayment = %+v, %v", first, err)
	}
	second, err := s.ApplyPayment(ctx, Confirmation{OrderID: o.ID, ConfirmedBy: f.staff})
	if err != nil || !second.AlreadyPaid || second.Order.ID != o.ID || second.Order.Status != StatusPaid {
		t.Fatalf("second ApplyPayment = %+v, %v", second, err)
	}
	if v, plan := f.subscriptionVersion(); v != 5 || plan != "pro" {
		t.Fatalf("subscription version %d plan %s; want 5 and pro (one increment)", v, plan)
	}
	if n := f.count(`SELECT count(*) FROM audit_event WHERE workspace_id = $1 AND action = $2`, f.ws, audit.ActionBillingOrderPaid); n != 1 {
		t.Fatalf("billing.order_paid rows = %d, want 1", n)
	}
}

func TestDBConcurrentConfirmationsApplyOnce(t *testing.T) {
	f := newDBFixture(t)
	f.seedSubscription(0)
	s := f.service(nil, nil)
	ctx := context.Background()
	o, _, err := s.CreateOrder(ctx, f.draft("k"))
	if err != nil {
		t.Fatal(err)
	}

	const callers = 8
	results := make([]Applied, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = s.ApplyPayment(ctx, Confirmation{OrderID: o.ID, ConfirmedBy: f.staff})
		}()
	}
	wg.Wait()

	applied := 0
	for i := range callers {
		if errs[i] != nil {
			t.Fatalf("caller %d: %v", i, errs[i])
		}
		if !results[i].AlreadyPaid {
			applied++
		}
	}
	if applied != 1 {
		t.Fatalf("%d callers applied the order, want exactly 1", applied)
	}
	if v, _ := f.subscriptionVersion(); v != 1 {
		t.Fatalf("subscription version = %d, want 1", v)
	}
}

func TestDBExpiredOrderCannotBeConfirmed(t *testing.T) {
	f := newDBFixture(t)
	f.seedSubscription(4)
	created := time.Now().Add(-8 * 24 * time.Hour)
	old := f.service(nil, func() time.Time { return created })
	ctx := context.Background()
	o, _, err := old.CreateOrder(ctx, f.draft("k"))
	if err != nil {
		t.Fatal(err)
	}
	if !IsExpired(o.ExpiresAt, time.Now()) {
		t.Fatalf("expires_at %v is not in the past", o.ExpiresAt)
	}

	_, err = f.service(nil, nil).ApplyPayment(ctx, Confirmation{OrderID: o.ID, ConfirmedBy: f.staff})
	if !errors.Is(err, ErrOrderExpired) {
		t.Fatalf("err = %v, want ErrOrderExpired", err)
	}
	if v, plan := f.subscriptionVersion(); v != 4 || plan != "free" {
		t.Fatalf("subscription changed to version %d plan %s", v, plan)
	}
	if n := f.count(`SELECT count(*) FROM billing_order WHERE id = $1 AND status = 'pending'`, o.ID); n != 1 {
		t.Fatal("the order is no longer pending")
	}
}

func TestDBRepeatedIdempotencyKeyReturnsTheFirstOrder(t *testing.T) {
	f := newDBFixture(t)
	s := f.service(nil, nil)
	ctx := context.Background()
	first, created, err := s.CreateOrder(ctx, f.draft("same"))
	if err != nil || !created {
		t.Fatalf("first = %v, %v", created, err)
	}
	d := f.draft("same")
	d.Seats = 9
	again, created, err := s.CreateOrder(ctx, d)
	if err != nil || created || again.ID != first.ID || again.Seats != 3 || again.TransferCode != first.TransferCode {
		t.Fatalf("again = %+v, created %v, err %v", again, created, err)
	}
	if n := f.count(`SELECT count(*) FROM billing_order WHERE workspace_id = $1`, f.ws); n != 1 {
		t.Fatalf("orders = %d, want 1", n)
	}
	if n := f.count(`SELECT count(*) FROM audit_event WHERE workspace_id = $1 AND action = $2`, f.ws, audit.ActionBillingOrderCreated); n != 1 {
		t.Fatalf("billing.order_created rows = %d, want 1", n)
	}
	// Orders with no key never collide with each other.
	for range 2 {
		if _, created, err := s.CreateOrder(ctx, f.draft("")); err != nil || !created {
			t.Fatalf("keyless create = %v, %v", created, err)
		}
	}
}

func TestDBPaymentReactivatesAnOverdueWorkspace(t *testing.T) {
	f := newDBFixture(t)
	f.seedSubscription(2)
	if _, err := f.pool.Exec(context.Background(),
		`INSERT INTO platform_workspace (workspace_id, status, reason) VALUES ($1, 'read_only', 'billing')`, f.ws); err != nil {
		t.Fatal(err)
	}
	s := f.service(nil, nil)
	ctx := context.Background()
	o, _, err := s.CreateOrder(ctx, f.draft("k"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.ApplyPayment(ctx, Confirmation{OrderID: o.ID, ConfirmedBy: f.staff, ActorLabel: "bill@example.com"})
	if err != nil || !got.WorkspaceReactivated {
		t.Fatalf("ApplyPayment = %+v, %v", got, err)
	}
	var status string
	if err := f.pool.QueryRow(ctx, `SELECT status FROM platform_workspace WHERE workspace_id = $1`, f.ws).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(workspacestate.Active) {
		t.Fatalf("workspace status = %s, want active", status)
	}
	if len(f.inv.ws) != 1 || f.inv.ws[0] != f.ws {
		t.Fatalf("workspace cache invalidations = %v", f.inv.ws)
	}

	// A read_only workspace with another reason is left alone.
	f2 := newDBFixture(t)
	f2.seedSubscription(2)
	if _, err := f2.pool.Exec(ctx, `INSERT INTO platform_workspace (workspace_id, status, reason) VALUES ($1, 'read_only', 'abuse')`, f2.ws); err != nil {
		t.Fatal(err)
	}
	s2 := f2.service(nil, nil)
	o2, _, err := s2.CreateOrder(ctx, f2.draft("k"))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s2.ApplyPayment(ctx, Confirmation{OrderID: o2.ID, ConfirmedBy: f2.staff}); err != nil || got.WorkspaceReactivated {
		t.Fatalf("other-reason ApplyPayment = %+v, %v", got, err)
	}
}

// failingAudit makes the billing.order_paid write fail inside the transaction.
type failingAuditRunner struct{ inner Runner }

func (r failingAuditRunner) InTx(ctx context.Context, fn func(Tx) error) error {
	return r.inner.InTx(ctx, func(tx Tx) error { return fn(failingAuditTx{tx}) })
}

type failingAuditTx struct{ Tx }

func (t failingAuditTx) InsertAuditEvent(ctx context.Context, e audit.Entry) error {
	if e.Action == audit.ActionBillingOrderPaid {
		return errors.New("audit write failed")
	}
	return t.Tx.InsertAuditEvent(ctx, e)
}

func TestDBFailedAuditWriteRollsBackTheSubscription(t *testing.T) {
	f := newDBFixture(t)
	f.seedSubscription(4)
	if _, err := f.pool.Exec(context.Background(),
		`INSERT INTO platform_workspace (workspace_id, status, reason) VALUES ($1, 'read_only', 'billing')`, f.ws); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	o, _, err := f.service(nil, nil).CreateOrder(ctx, f.draft("k"))
	if err != nil {
		t.Fatal(err)
	}

	failing := f.service(failingAuditRunner{NewPgxRunner(f.pool)}, nil)
	if _, err := failing.ApplyPayment(ctx, Confirmation{OrderID: o.ID, ConfirmedBy: f.staff}); err == nil {
		t.Fatal("ApplyPayment succeeded with a failing audit write")
	}
	if v, plan := f.subscriptionVersion(); v != 4 || plan != "free" {
		t.Fatalf("subscription version %d plan %s; the update was not rolled back", v, plan)
	}
	if n := f.count(`SELECT count(*) FROM billing_order WHERE id = $1 AND status = 'pending' AND paid_at IS NULL`, o.ID); n != 1 {
		t.Fatal("the order is not pending again")
	}
	var status string
	if err := f.pool.QueryRow(ctx, `SELECT status FROM platform_workspace WHERE workspace_id = $1`, f.ws).Scan(&status); err != nil || status != "read_only" {
		t.Fatalf("workspace status = %q, %v; want read_only", status, err)
	}
	if len(f.inv.ent) != 0 || len(f.inv.ws) != 0 {
		t.Fatal("caches were cleared for a change that did not commit")
	}
}

// seedCatalogue makes the catalogue's plans visible, as the platform does when it
// starts: ApplyPayment prices a seat from the plan. The rows this adds are removed
// when the test ends, so the table is as it was for the other tests.
func seedCatalogue(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	before := map[string]bool{}
	rows, err := pool.Query(ctx, `SELECT key FROM plan`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			t.Fatal(err)
		}
		before[key] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	if _, err := plan.Seed(ctx, plan.NewPgxRunner(pool), quiet); err != nil {
		t.Fatal(err)
	}
	all, err := plan.Catalogue()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, p := range all {
			if !before[p.Key] {
				pool.Exec(ctx, `DELETE FROM plan WHERE key = $1`, p.Key)
			}
		}
	})
}
