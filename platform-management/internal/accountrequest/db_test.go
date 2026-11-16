package accountrequest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/testdb"
)

// Database tests: skipped unless PLATFORM_TEST_DATABASE_URL names a *_test
// database and DATABASE_URL equals it. Every email carries a random tag and the
// test removes its rows and their audit rows.

type dbEnv struct {
	t       *testing.T
	pool    *pgxpool.Pool
	tag     string
	staff   string
	limiter *fakeLimiter
	svc     *Service
}

func newDBEnv(t *testing.T) *dbEnv {
	t.Helper()
	pool := testdb.Open(t)
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	e := &dbEnv{t: t, pool: pool, tag: "ar-" + hex.EncodeToString(b), limiter: &fakeLimiter{}}
	if err := pool.QueryRow(context.Background(), `SELECT gen_random_uuid()::text`).Scan(&e.staff); err != nil {
		t.Fatal(err)
	}
	e.svc = e.service(NewPgxRunner(pool))
	t.Cleanup(func() {
		ctx := context.Background()
		pool.Exec(ctx, `DELETE FROM audit_event WHERE actor_id = $1`, e.staff)
		pool.Exec(ctx, `DELETE FROM platform_account_request WHERE email LIKE $1`, e.tag+"%")
		pool.Exec(ctx, `DELETE FROM "user" WHERE email LIKE $1`, e.tag+"%")
	})
	return e
}

func (e *dbEnv) service(r Runner) *Service {
	return New(Deps{
		Runner: r, Reader: NewPgxReader(e.pool), Plans: plan.NewPgxReader(e.pool), Limiter: e.limiter,
		CreatePerHour: 30, IndividualMaxSeats: 5,
	})
}

func (e *dbEnv) email(n string) string { return e.tag + "-" + n + "@example.test" }

func (e *dbEnv) input(n string) Input {
	in := goodIndividual()
	in.Email = e.email(n)
	return in
}

func (e *dbEnv) actor() Actor { return Actor{ID: e.staff, Label: "staff@example.test"} }

func (e *dbEnv) count(query string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func TestAccountRequestDBCreateStoresTheRowAndTheAuditRow(t *testing.T) {
	e := newDBEnv(t)
	in := e.input("one")
	in.WorkspaceSlug = "ana-space"
	r, created, err := e.svc.Create(context.Background(), e.actor(), in)
	if err != nil || !created {
		t.Fatalf("created = %v, err = %v", created, err)
	}
	var kind, state, plan, interval, slug, createdBy string
	var seats, attempts int
	var userID, workspaceID *string
	err = e.pool.QueryRow(context.Background(),
		`SELECT kind, state, plan_key, interval, workspace_slug, seats, attempts, user_id::text, workspace_id::text, created_by::text
		 FROM platform_account_request WHERE id = $1`, r.ID).
		Scan(&kind, &state, &plan, &interval, &slug, &seats, &attempts, &userID, &workspaceID, &createdBy)
	if err != nil {
		t.Fatal(err)
	}
	if kind != "individual" || state != "pending" || plan != "free" || interval != "month" || slug != "ana-space" ||
		seats < 1 || attempts != 0 || userID != nil || workspaceID != nil || createdBy != e.staff {
		t.Errorf("row = %s %s %s %s %s %d %d %v %v %s", kind, state, plan, interval, slug, seats, attempts, userID, workspaceID, createdBy)
	}
	var actorType, label, target string
	var ws *string
	err = e.pool.QueryRow(context.Background(),
		`SELECT actor_type, target_label, target_id, workspace_id::text FROM audit_event
		 WHERE actor_id = $1 AND action = 'account.provision_requested'`, e.staff).Scan(&actorType, &label, &target, &ws)
	if err != nil {
		t.Fatalf("audit row: %v", err)
	}
	if actorType != "staff" || label != in.Email || target != r.ID || ws != nil {
		t.Errorf("audit row = %s %s %s %v", actorType, label, target, ws)
	}
	got, found, err := e.svc.Get(context.Background(), r.ID)
	if err != nil || !found || got.Email != in.Email || got.State != StatePending {
		t.Errorf("Get = %+v %v %v", got, found, err)
	}
	list, err := e.svc.List(context.Background(), ListQuery{State: StatePending, Limit: 500})
	if err != nil {
		t.Fatal(err)
	}
	var seen bool
	for _, x := range list {
		seen = seen || x.ID == r.ID
	}
	if !seen {
		t.Error("List does not return the new request")
	}
}

// testPlan stores a business seat plan under a key of its own, since the test
// database holds only the free plan, and removes it afterwards.
func (e *dbEnv) testPlan() string {
	e.t.Helper()
	key := "t" + e.tag[3:]
	p := plan.Plan{
		Key: key, Name: "Test business", Currency: "vnd", Audience: plan.AudienceBusiness,
		BillingModel: plan.BillingSeat, MinSeats: i(5), TrialDays: 14,
	}
	err := plan.NewPgxRunner(e.pool).InTx(context.Background(), func(tx plan.Tx) error { return tx.Upsert(context.Background(), p) })
	if err != nil {
		e.t.Fatalf("store test plan: %v", err)
	}
	e.t.Cleanup(func() { e.pool.Exec(context.Background(), `DELETE FROM plan WHERE key = $1`, key) })
	return key
}

func TestAccountRequestDBBusinessKeepsTheBillingProfile(t *testing.T) {
	e := newDBEnv(t)
	in := goodBusiness()
	in.Email = e.email("biz")
	in.PlanKey = e.testPlan()
	in.Trial = true
	r, _, err := e.svc.Create(context.Background(), e.actor(), in)
	if err != nil {
		t.Fatalf("%+v", err)
	}
	got, _, _ := e.svc.Get(context.Background(), r.ID)
	if got.BillingProfile == nil || got.BillingProfile.TaxCode != "0123456789" || got.PlanKey != in.PlanKey || got.Seats < 5 || !got.Trial {
		t.Errorf("request = %+v profile = %+v", got, got.BillingProfile)
	}
}

// Rule 5 on the real index: parallel creates for one email leave one row and
// all of them answer with it.
func TestAccountRequestDBParallelCreatesForOneEmailWriteOneRow(t *testing.T) {
	e := newDBEnv(t)
	const n = 8
	var wg sync.WaitGroup
	ids := make([]string, n)
	created := make([]bool, n)
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, c, err := e.svc.Create(context.Background(), e.actor(), e.input("race"))
			ids[i], created[i], errs[i] = r.ID, c, err
		}()
	}
	wg.Wait()
	wins := 0
	for i := range n {
		if errs[i] != nil {
			t.Fatalf("create %d: %v", i, errs[i])
		}
		if created[i] {
			wins++
		}
		if ids[i] == "" || ids[i] != ids[0] {
			t.Errorf("create %d answered with %q, want %q", i, ids[i], ids[0])
		}
	}
	if wins != 1 {
		t.Errorf("%d creates reported created, want 1", wins)
	}
	if c := e.count(`SELECT count(*) FROM platform_account_request WHERE email = $1`, e.email("race")); c != 1 {
		t.Errorf("%d rows, want 1", c)
	}
	if c := e.count(`SELECT count(*) FROM audit_event WHERE actor_id = $1 AND action = 'account.provision_requested'`, e.staff); c != 1 {
		t.Errorf("%d audit rows, want 1", c)
	}
}

func isUniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

// The indexes themselves: 967 on id, 968 on email while the request is live.
func TestAccountRequestDBIndexesHoldTheRules(t *testing.T) {
	e := newDBEnv(t)
	ctx := context.Background()
	const insert = `INSERT INTO platform_account_request (id, kind, email, display_name, workspace_name, plan_key, seats, created_by, state)
		VALUES ($1, 'individual', $2, 'n', 'w', 'free', 1, $3, $4)`
	var id1, id2 string
	if err := e.pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&id1); err != nil {
		t.Fatal(err)
	}
	if err := e.pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&id2); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, insert, id1, e.email("idx"), e.staff, "pending"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, insert, id1, e.email("other"), e.staff, "pending"); !isUniqueViolation(err) {
		t.Errorf("a repeated id: err = %v, want a unique violation", err)
	}
	if _, err := e.pool.Exec(ctx, insert, id2, e.email("idx"), e.staff, "provisioned"); !isUniqueViolation(err) {
		t.Errorf("a second live request for one email: err = %v, want a unique violation", err)
	}
	for _, closed := range []string{"completed", "failed", "canceled"} {
		var id string
		if err := e.pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := e.pool.Exec(ctx, insert, id, e.email("idx"), e.staff, closed); err != nil {
			t.Errorf("a %s request for the same email is refused: %v", closed, err)
		}
	}
}

func TestAccountRequestDBEmailOfAUserIsRefusedIgnoringCase(t *testing.T) {
	e := newDBEnv(t)
	mixed := e.tag + "-User@Example.TEST"
	if _, err := e.pool.Exec(context.Background(), `INSERT INTO "user" (name, email) VALUES ('Existing', $1)`, mixed); err != nil {
		t.Fatal(err)
	}
	in := e.input("user")
	in.Email = e.tag + "-USER@example.test"
	_, _, err := e.svc.Create(context.Background(), e.actor(), in)
	wantError(t, err, 409, "account_email_exists")
	if c := e.count(`SELECT count(*) FROM platform_account_request WHERE email LIKE $1`, e.tag+"%"); c != 0 {
		t.Errorf("%d request rows after a refused create", c)
	}
	if c := e.count(`SELECT count(*) FROM audit_event WHERE actor_id = $1`, e.staff); c != 0 {
		t.Errorf("%d audit rows after a refused create", c)
	}
	var name string
	if err := e.pool.QueryRow(context.Background(), `SELECT name FROM "user" WHERE email = $1`, mixed).Scan(&name); err != nil || name != "Existing" {
		t.Errorf("the user row changed: %q %v", name, err)
	}
}

type failingAudit struct{ Tx }

func (failingAudit) InsertAuditEvent(context.Context, audit.Entry) error {
	return errors.New("audit write failed")
}

type failingAuditRunner struct{ inner Runner }

func (r failingAuditRunner) InTx(ctx context.Context, fn func(Tx) error) error {
	return r.inner.InTx(ctx, func(tx Tx) error { return fn(failingAudit{tx}) })
}

// I7 on the real database: the audit write fails, the insert rolls back.
func TestAccountRequestDBAuditFailureLeavesNoRow(t *testing.T) {
	e := newDBEnv(t)
	svc := e.service(failingAuditRunner{NewPgxRunner(e.pool)})
	if _, created, err := svc.Create(context.Background(), e.actor(), e.input("i7")); err == nil || created {
		t.Fatalf("created = %v, err = %v, want an error", created, err)
	}
	if c := e.count(`SELECT count(*) FROM platform_account_request WHERE email = $1`, e.email("i7")); c != 0 {
		t.Errorf("%d rows after a failed audit write", c)
	}
}

func TestAccountRequestDBCancelOnlyPending(t *testing.T) {
	e := newDBEnv(t)
	ctx := context.Background()
	r, _, err := e.svc.Create(ctx, e.actor(), e.input("cancel"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := e.svc.Cancel(ctx, e.actor(), r.ID)
	if err != nil || got.State != StateCanceled {
		t.Fatalf("cancel = %+v %v", got, err)
	}
	if c := e.count(`SELECT count(*) FROM audit_event WHERE actor_id = $1 AND action = 'account.provision_canceled' AND target_id = $2`, e.staff, r.ID); c != 1 {
		t.Errorf("%d cancel audit rows, want 1", c)
	}
	_, err = e.svc.Cancel(ctx, e.actor(), r.ID)
	if ae := wantError(t, err, 409, "account_request_not_cancelable"); ae.State != "canceled" {
		t.Errorf("state = %q", ae.State)
	}

	other, _, _ := e.svc.Create(ctx, e.actor(), e.input("prov"))
	if _, err := e.pool.Exec(ctx, `UPDATE platform_account_request SET state = 'provisioned' WHERE id = $1`, other.ID); err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.Cancel(ctx, e.actor(), other.ID)
	wantError(t, err, 409, "account_request_not_cancelable")
	if _, err := e.svc.Cancel(ctx, e.actor(), "30000000-0000-4000-8000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown id: %v", err)
	}
}
