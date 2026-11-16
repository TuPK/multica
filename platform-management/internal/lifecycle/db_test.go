package lifecycle

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
	"github.com/multica-ai/multica/platform-management/internal/testdb"
	"github.com/multica-ai/multica/platform-management/internal/workspacestatus"
	"github.com/multica-ai/multica/platform-management/workspacestate"
)

// Database tests: skipped unless PLATFORM_TEST_DATABASE_URL names a *_test
// database and DATABASE_URL equals it. Each test uses a fresh workspace id
// (these tables have no foreign key), asserts on its own rows only, because
// the job reads across workspaces, and removes what it wrote.

type dbFixture struct {
	t    *testing.T
	pool *pgxpool.Pool
	ws   string
	mail *fakeMail
	svc  *Service
}

func newDBFixture(t *testing.T) *dbFixture {
	t.Helper()
	pool := testdb.Open(t)
	f := &dbFixture{t: t, pool: pool, mail: &fakeMail{}}
	ctx := context.Background()
	if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&f.ws); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, q := range []string{
			`DELETE FROM workspace_subscription WHERE workspace_id = $1`,
			`DELETE FROM platform_workspace WHERE workspace_id = $1`,
			`DELETE FROM audit_event WHERE workspace_id = $1`,
			`DELETE FROM billing_notice WHERE workspace_id = $1`,
		} {
			pool.Exec(ctx, q, f.ws)
		}
	})
	cache := &cacheLog{}
	svc, err := New(Deps{
		Store: NewPgxStore(pool), Mail: f.mail, Workspaces: cache, Entitlements: policyCache{cache},
		AppURL: "https://app.example.com", UsageDueDays: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	f.svc = svc
	return f
}

func (f *dbFixture) seed(s subscription.Subscription) {
	f.t.Helper()
	s.WorkspaceID = f.ws
	_, err := f.pool.Exec(context.Background(),
		`INSERT INTO workspace_subscription
		   (workspace_id, plan_key, status, seats, interval, current_period_start, current_period_end,
		    trial_end, grace_until, cancel_at_period_end, version)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		s.WorkspaceID, s.PlanKey, s.Status, s.Seats, s.Interval, s.CurrentPeriodStart, s.CurrentPeriodEnd,
		s.TrialEnd, s.GraceUntil, s.CancelAtPeriodEnd, s.Version)
	if err != nil {
		f.t.Fatal(err)
	}
}

func (f *dbFixture) subscription() (status string, version int64) {
	f.t.Helper()
	if err := f.pool.QueryRow(context.Background(),
		`SELECT status, version FROM workspace_subscription WHERE workspace_id = $1`, f.ws).Scan(&status, &version); err != nil {
		f.t.Fatal(err)
	}
	return status, version
}

func (f *dbFixture) count(query string) int {
	f.t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), query, f.ws).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

// barrierStore makes two runs read the due rows before either writes.
type barrierStore struct {
	*PgxStore
	b *barrier
	t *testing.T
}

func (s barrierStore) Due(ctx context.Context, e Edge, now time.Time, limit int) ([]subscription.Subscription, error) {
	rows, err := s.PgxStore.Due(ctx, e, now, limit)
	if e == EdgePeriodLapsed {
		s.b.wait(s.t)
	}
	return rows, err
}

// The conditional update applies once when two runs race for the same row.
func TestDBConditionalMoveAppliesOnceUnderTwoConcurrentRuns(t *testing.T) {
	f := newDBFixture(t)
	f.seed(activeSub(f.ws, t0))
	b := &barrier{n: 2}
	cache := &cacheLog{}
	svc, err := New(Deps{
		Store: barrierStore{NewPgxStore(f.pool), b, t}, Mail: f.mail, Workspaces: cache,
		Entitlements: policyCache{cache}, AppURL: "https://app.example.com", UsageDueDays: 7,
	})
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = svc.Run(context.Background(), t0)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	}
	if status, version := f.subscription(); status != StatusPastDue || version != 4 {
		t.Errorf("subscription = %q version %d, want past_due version 4 (3 + one move)", status, version)
	}
}

// The read_only edge writes platform_workspace in the same transaction as the
// subscription.
func TestDBReadOnlyEdgeWritesThePlatformWorkspaceRow(t *testing.T) {
	f := newDBFixture(t)
	f.seed(pastDueSub(f.ws, t0))
	if _, err := f.svc.Run(context.Background(), t0.Add(GracePeriod)); err != nil {
		t.Fatal(err)
	}
	if status, _ := f.subscription(); status != StatusReadOnly {
		t.Errorf("subscription status = %q", status)
	}
	var status, reason string
	if err := f.pool.QueryRow(context.Background(),
		`SELECT status, reason FROM platform_workspace WHERE workspace_id = $1`, f.ws).Scan(&status, &reason); err != nil {
		t.Fatal(err)
	}
	if status != string(workspacestate.ReadOnly) || reason != billing.ReasonBilling {
		t.Errorf("platform_workspace = %q, %q", status, reason)
	}
	if n := f.count(`SELECT count(*) FROM audit_event WHERE workspace_id = $1`); n != 1 {
		t.Errorf("audit rows = %d, want 1", n)
	}
}

// A failure after both writes rolls the subscription change back with them.
func TestDBFailureAfterTheWritesRollsTheSubscriptionBack(t *testing.T) {
	f := newDBFixture(t)
	from := pastDueSub(f.ws, t0)
	f.seed(from)
	to := EdgeGraceEnded.Apply(from)

	boom := errors.New("boom")
	err := NewPgxStore(f.pool).InTx(context.Background(), func(tx Tx) error {
		if _, applied, err := tx.Move(context.Background(), from, to); err != nil || !applied {
			t.Fatalf("Move = %v, %v", applied, err)
		}
		if _, err := workspacestatus.SetInTx(context.Background(), tx, workspacestatus.Change{
			WorkspaceID: f.ws, Status: workspacestate.ReadOnly, Reason: billing.ReasonBilling, ChangedBy: systemUser,
		}); err != nil {
			t.Fatalf("SetInTx: %v", err)
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("InTx = %v, want the failure", err)
	}
	if status, version := f.subscription(); status != StatusPastDue || version != from.Version {
		t.Errorf("subscription = %q version %d, want it unchanged", status, version)
	}
	if n := f.count(`SELECT count(*) FROM platform_workspace WHERE workspace_id = $1`); n != 0 {
		t.Errorf("platform_workspace rows = %d, want 0", n)
	}
	if n := f.count(`SELECT count(*) FROM audit_event WHERE workspace_id = $1`); n != 0 {
		t.Errorf("audit rows = %d, want 0", n)
	}
}

// Move refuses a row that has changed since it was read.
func TestDBMoveRefusesAStaleVersion(t *testing.T) {
	f := newDBFixture(t)
	from := activeSub(f.ws, t0)
	f.seed(from)
	stale := from
	stale.Version--
	err := NewPgxStore(f.pool).InTx(context.Background(), func(tx Tx) error {
		_, applied, err := tx.Move(context.Background(), stale, EdgePeriodLapsed.Apply(stale))
		if err != nil || applied {
			t.Errorf("Move on a stale version = %v, %v", applied, err)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if status, version := f.subscription(); status != subscription.StatusActive || version != from.Version {
		t.Errorf("subscription = %q version %d, want it unchanged", status, version)
	}
}

// The unique index on (workspace_id, notice_key) refuses a duplicate.
func TestDBBillingNoticeIndexRefusesADuplicateKey(t *testing.T) {
	f := newDBFixture(t)
	ctx := context.Background()
	insert := `INSERT INTO billing_notice (workspace_id, notice_key, kind) VALUES ($1, 'remind:1:14', 'remind')`
	if _, err := f.pool.Exec(ctx, insert, f.ws); err != nil {
		t.Fatal(err)
	}
	_, err := f.pool.Exec(ctx, insert, f.ws)
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != "23505" || pg.ConstraintName != "billing_notice_key_uidx" {
		t.Fatalf("second insert = %v, want a unique violation of billing_notice_key_uidx", err)
	}
	// A different key, or another workspace's same key, is fine.
	store := NewPgxStore(f.pool)
	for _, c := range []struct {
		n    Notice
		want bool
	}{
		{Notice{f.ws, "remind:1:7", KindReminder}, true},
		{Notice{f.ws, "remind:1:7", KindReminder}, false},
	} {
		got, err := store.InsertNotice(ctx, c.n)
		if err != nil || got != c.want {
			t.Errorf("InsertNotice(%+v) = %v, %v; want %v", c.n, got, err, c.want)
		}
	}
	if err := store.DeleteNotice(ctx, f.ws, "remind:1:7"); err != nil {
		t.Fatal(err)
	}
	if got, err := store.InsertNotice(ctx, Notice{f.ws, "remind:1:7", KindReminder}); err != nil || !got {
		t.Errorf("InsertNotice after delete = %v, %v; want true", got, err)
	}
}

// The contact comes from the shared member and user tables: owners and admins
// only.
func TestDBContactListsOwnersAndAdmins(t *testing.T) {
	f := newDBFixture(t)
	ctx := context.Background()
	slug := "lifecycle-" + f.ws[:8]
	if err := f.pool.QueryRow(ctx, `INSERT INTO workspace (id, name, slug) VALUES ($1, 'Lifecycle Test', $2) RETURNING slug`, f.ws, slug).Scan(&slug); err != nil {
		t.Fatal(err)
	}
	var users []string
	for _, role := range []string{"owner", "admin", "member"} {
		var id string
		email := role + "-" + f.ws[:8] + "@example.com"
		if err := f.pool.QueryRow(ctx, `INSERT INTO "user" (name, email) VALUES ($1, $2) RETURNING id::text`, role, email).Scan(&id); err != nil {
			t.Fatal(err)
		}
		users = append(users, id)
		if _, err := f.pool.Exec(ctx, `INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, $3)`, f.ws, id, role); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		f.pool.Exec(ctx, `DELETE FROM member WHERE workspace_id = $1`, f.ws)
		f.pool.Exec(ctx, `DELETE FROM "user" WHERE id = ANY($1::uuid[])`, users)
		f.pool.Exec(ctx, `DELETE FROM workspace WHERE id = $1`, f.ws)
	})

	c, err := NewPgxStore(f.pool).Contact(ctx, f.ws)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"owner-" + f.ws[:8] + "@example.com", "admin-" + f.ws[:8] + "@example.com"}
	if c.Name != "Lifecycle Test" || c.Slug != slug || !slices.Equal(c.Emails, want) {
		t.Errorf("contact = %+v, want name, slug %q and %v", c, slug, want)
	}
	// A workspace that is gone has no one to write to.
	gone, err := NewPgxStore(f.pool).Contact(ctx, "00000000-0000-0000-0000-00000000dead")
	if err != nil || len(gone.Emails) != 0 {
		t.Errorf("contact of a missing workspace = %+v, %v", gone, err)
	}
}
