package seat

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
	"github.com/multica-ai/multica/platform-management/internal/testdb"
)

// Database tests: skipped unless PLATFORM_TEST_DATABASE_URL names a *_test
// database and DATABASE_URL equals it. Each test makes its own workspace, users
// and plan key and removes them.

type dbFixture struct {
	t    *testing.T
	pool *pgxpool.Pool
	svc  *Service
	ws   string
	plan string
	user []string
}

func suffix(t *testing.T) string {
	t.Helper()
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

// newDBFixture makes a workspace on a plan with the given purchased seats and
// members.
func newDBFixture(t *testing.T, purchased, members int) *dbFixture {
	t.Helper()
	f := &dbFixture{t: t, pool: testdb.Open(t)}
	f.svc = NewService(NewPgxRunner(f.pool), time.Now)
	ctx := context.Background()
	t.Cleanup(func() {
		bg := context.Background()
		f.pool.Exec(bg, `DELETE FROM platform_seat_operation WHERE workspace_id = $1`, f.ws)
		f.pool.Exec(bg, `DELETE FROM workspace_subscription WHERE workspace_id = $1`, f.ws)
		f.pool.Exec(bg, `DELETE FROM member WHERE workspace_id = $1`, f.ws)
		f.pool.Exec(bg, `DELETE FROM workspace WHERE id = $1`, f.ws)
		for _, u := range f.user {
			f.pool.Exec(bg, `DELETE FROM "user" WHERE id = $1`, u)
		}
		f.pool.Exec(bg, `DELETE FROM plan WHERE key = $1`, f.plan)
	})
	s := suffix(t)
	f.plan = "seat" + s
	err := plan.NewPgxRunner(f.pool).InTx(ctx, func(tx plan.Tx) error {
		return tx.Upsert(ctx, plan.Plan{
			Key: f.plan, Name: "Seat test", Currency: "vnd", Limits: plan.Limits{}, Features: map[string]bool{},
		})
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO workspace (name, slug) VALUES ($1, $2) RETURNING id::text`,
		"Seat Test "+s, "seat-test-"+s).Scan(&f.ws); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO workspace_subscription (workspace_id, plan_key, status, seats, version)
		VALUES ($1, $2, 'active', $3, 7)`, f.ws, f.plan, purchased); err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	for i := 0; i < members; i++ {
		f.addMember()
	}
	return f
}

func (f *dbFixture) addMember() {
	f.t.Helper()
	ctx := context.Background()
	var u string
	if err := f.pool.QueryRow(ctx, `INSERT INTO "user" (name, email) VALUES ('Seat Test', $1) RETURNING id::text`,
		"seat-"+suffix(f.t)+"@example.test").Scan(&u); err != nil {
		f.t.Fatalf("create user: %v", err)
	}
	f.user = append(f.user, u)
	if _, err := f.pool.Exec(ctx, `INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'member')`, f.ws, u); err != nil {
		f.t.Fatalf("create member: %v", err)
	}
}

func newUUID(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	h := hex.EncodeToString(b)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

// Two invitations race for the last seat: exactly one is allowed and the other
// is refused as full.
func TestDBConcurrentInvitationsForTheLastSeat(t *testing.T) {
	f := newDBFixture(t, 2, 1)
	const racers = 8
	results := make([]Decision, racers)
	errs := make([]error, racers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			id := newUUID(t)
			results[i], errs[i] = f.svc.Reserve(context.Background(), f.ws, id, id, time.Now().Add(time.Hour))
		}()
	}
	close(start)
	wg.Wait()
	allowed := 0
	for i, d := range results {
		if errs[i] != nil {
			t.Fatalf("racer %d: %v", i, errs[i])
		}
		if d.Allowed {
			allowed++
		} else if d.Reason != ReasonCapacityFull {
			t.Errorf("racer %d: reason %q, want %s", i, d.Reason, ReasonCapacityFull)
		}
	}
	if allowed != 1 {
		t.Fatalf("%d invitations got the last seat, want 1", allowed)
	}
}

// A member that existed before the platform was enabled is counted, with no
// platform_seat_operation row for it.
func TestDBPreexistingMembersAreCounted(t *testing.T) {
	f := newDBFixture(t, 2, 2)
	id := newUUID(t)
	d, err := f.svc.Reserve(context.Background(), f.ws, id, id, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if d.Allowed || d.Reason != ReasonCapacityFull || d.Capacity == nil || d.Capacity.UsedSeats != 2 || d.Capacity.Version != 7 {
		t.Fatalf("decision = %+v capacity = %+v", d, d.Capacity)
	}
	f.addMember()
	if _, err := f.svc.Reserve(context.Background(), f.ws, newUUID(t), id, time.Now().Add(time.Hour)); err != ErrOvercommitted {
		t.Fatalf("error = %v, want ErrOvercommitted for 3 members on 2 seats", err)
	}
}

// Delegora's handler holds WorkspaceLocker (a session advisory lock on one
// connection) while it calls the platform. The call must not wait for it.
func TestDBDoesNotWaitForTheHandlersWorkspaceLock(t *testing.T) {
	f := newDBFixture(t, 3, 1)
	ctx := context.Background()
	conn, err := f.pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	// The key Delegora's locker derives (workspace_locker.go, workspaceAdvisoryLockKey).
	sum := sha256.Sum256(append([]byte("multica-seat-capacity:"), uuidBytes(f.ws)...))
	key := int64(binary.BigEndian.Uint64(sum[:8]))
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, key); err != nil {
		t.Fatal(err)
	}
	defer conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, key)

	h := NewHandler(f.svc, nil).Mux()
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	id := newUUID(t)
	d, err := f.svc.Reserve(callCtx, f.ws, id, id, time.Now().Add(time.Hour))
	if err != nil || !d.Allowed {
		t.Fatalf("reserve while the handler lock is held: %+v, %v", d, err)
	}
	if rec := do(h, http.MethodGet, "operations/"+id, ""); rec.Code != http.StatusOK {
		t.Errorf("read = %d", rec.Code)
	}
}

// The summary's reserved count counts live holds only.
func TestDBSummaryReservedCount(t *testing.T) {
	f := newDBFixture(t, 5, 1)
	ctx := context.Background()
	live, claim, expired, released := newUUID(t), newUUID(t), newUUID(t), newUUID(t)
	if _, err := f.svc.Reserve(ctx, f.ws, live, live, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Claim(ctx, f.ws, claim, claim); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Reserve(ctx, f.ws, expired, expired, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE platform_seat_operation SET expires_at = now() - interval '1 minute'
		WHERE workspace_id = $1 AND operation_token = $2`, f.ws, expired); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Reserve(ctx, f.ws, released, released, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Release(ctx, f.ws, released); err != nil {
		t.Fatal(err)
	}
	got, err := subscription.NewPgxMembers(f.pool).Reserved(ctx, f.ws)
	if err != nil {
		t.Fatal(err)
	}
	if got != 2 {
		t.Fatalf("reserved = %d, want 2 (the live reservation and the claim)", got)
	}
}
