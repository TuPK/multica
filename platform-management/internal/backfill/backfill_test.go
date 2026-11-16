package backfill

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/testdb"
)

// Fixed ids: Run walks workspaces in id order, so wsA is handled before wsB and
// wsB before wsC. Other workspaces in the shared test database are not asserted on.
const (
	wsA    = "a0000000-0000-4000-8000-000000000001"
	wsB    = "a0000000-0000-4000-8000-000000000002"
	wsC    = "a0000000-0000-4000-8000-000000000003"
	wsPaid = "a0000000-0000-4000-8000-0000000000f0"
)

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// seed opens the test database and creates the given workspaces. The cleanup
// registered here runs before testdb's pool is closed (cleanups run last-in, first-out).
func seed(t *testing.T, ids ...string) *pgxpool.Pool {
	t.Helper()
	pool := testdb.Open(t)
	ctx := context.Background()
	for _, id := range ids {
		if _, err := pool.Exec(ctx, `INSERT INTO workspace (id, name, slug) VALUES ($1, $2, $3)`, id, "backfill "+id, "backfill-"+id); err != nil {
			t.Fatalf("seed workspace %s: %v", id, err)
		}
	}
	t.Cleanup(func() {
		for _, id := range ids {
			_, _ = pool.Exec(context.Background(), `DELETE FROM audit_event WHERE workspace_id = $1`, id)
			_, _ = pool.Exec(context.Background(), `DELETE FROM workspace_subscription WHERE workspace_id = $1`, id)
			_, _ = pool.Exec(context.Background(), `DELETE FROM workspace WHERE id = $1`, id)
		}
	})
	return pool
}

// scoped limits a run to the given workspaces. The test database is shared with
// other packages' tests, which create workspaces while this one runs; an
// unscoped run would write subscription rows into their fixtures.
func scoped(pool *pgxpool.Pool, ids ...string) Runner {
	set := map[string]bool{}
	for _, id := range ids {
		set[id] = true
	}
	return scopedRunner{inner: NewPgxRunner(pool), ids: set}
}

type scopedRunner struct {
	inner Runner
	ids   map[string]bool
}

func (r scopedRunner) InTx(ctx context.Context, fn func(Tx) error) error {
	return r.inner.InTx(ctx, func(tx Tx) error {
		return fn(scopedTx{Tx: tx, ids: r.ids})
	})
}

type scopedTx struct {
	Tx
	ids map[string]bool
}

func (t scopedTx) Unsubscribed(ctx context.Context, limit int) ([]Candidate, error) {
	all, err := t.Tx.Unsubscribed(ctx, math.MaxInt32)
	if err != nil {
		return nil, err
	}
	var out []Candidate
	for _, c := range all {
		if t.ids[c.ID] && len(out) < limit {
			out = append(out, c)
		}
	}
	return out, nil
}

func subscriptionCount(t *testing.T, pool *pgxpool.Pool, id string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM workspace_subscription WHERE workspace_id = $1`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func backfillAuditCount(t *testing.T, pool *pgxpool.Pool, id string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_event WHERE workspace_id = $1 AND action = $2`, id, audit.ActionSubscriptionBackfilled).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRunGivesWorkspaceWithNoRowTheFreePlan(t *testing.T) {
	pool := seed(t, wsA)
	if _, err := Run(context.Background(), scoped(pool, wsA), BatchSize, quietLog()); err != nil {
		t.Fatal(err)
	}

	var planKey, status, interval string
	var seats int32
	var version int64
	var periodEnd, trialEnd, graceUntil *string
	err := pool.QueryRow(context.Background(), `
		SELECT plan_key, status, seats, "interval", version,
		       current_period_end::text, trial_end::text, grace_until::text
		FROM workspace_subscription WHERE workspace_id = $1`, wsA).
		Scan(&planKey, &status, &seats, &interval, &version, &periodEnd, &trialEnd, &graceUntil)
	if err != nil {
		t.Fatalf("read backfilled row: %v", err)
	}
	if planKey != "free" || status != "active" || seats != 0 || interval != "month" || version != 1 {
		t.Fatalf("row = plan %q status %q seats %d interval %q version %d; want free/active/0/month/1",
			planKey, status, seats, interval, version)
	}
	if periodEnd != nil || trialEnd != nil || graceUntil != nil {
		t.Fatalf("dates = %v %v %v; want none", periodEnd, trialEnd, graceUntil)
	}

	var actorType, targetType, targetID, detailPlan, detailSource string
	err = pool.QueryRow(context.Background(), `
		SELECT actor_type, target_type, target_id, detail->>'plan', detail->>'source'
		FROM audit_event WHERE workspace_id = $1 AND action = $2`, wsA, audit.ActionSubscriptionBackfilled).
		Scan(&actorType, &targetType, &targetID, &detailPlan, &detailSource)
	if err != nil {
		t.Fatalf("read backfill audit row: %v", err)
	}
	if actorType != audit.ActorSystem || targetType != "workspace" || targetID != wsA {
		t.Fatalf("audit actor %q target %q/%q", actorType, targetType, targetID)
	}
	// "plan", not "plan_key": audit.Record redacts any key containing "key".
	if detailPlan != "free" || detailSource != "cutover" {
		t.Fatalf("audit detail plan %q source %q; want free/cutover", detailPlan, detailSource)
	}
}

func TestRunLeavesWorkspaceWithRowUnchanged(t *testing.T) {
	pool := seed(t, wsPaid)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO workspace_subscription (workspace_id, plan_key, status, seats, "interval", version)
		VALUES ($1, 'team', 'active', 5, 'year', 3)`, wsPaid); err != nil {
		t.Fatal(err)
	}

	if _, err := Run(ctx, scoped(pool, wsA, wsB), BatchSize, quietLog()); err != nil {
		t.Fatal(err)
	}

	var planKey string
	var seats int32
	var version int64
	if err := pool.QueryRow(ctx, `SELECT plan_key, seats, version FROM workspace_subscription WHERE workspace_id = $1`, wsPaid).
		Scan(&planKey, &seats, &version); err != nil {
		t.Fatal(err)
	}
	if planKey != "team" || seats != 5 || version != 3 {
		t.Fatalf("row changed: plan %q seats %d version %d", planKey, seats, version)
	}
	if n := backfillAuditCount(t, pool, wsPaid); n != 0 {
		t.Fatalf("backfill audit rows for a subscribed workspace = %d, want 0", n)
	}
}

func TestRunSecondRunInsertsAndAuditsNothing(t *testing.T) {
	pool := seed(t, wsA, wsB)
	ctx := context.Background()
	if _, err := Run(ctx, scoped(pool, wsA, wsB), BatchSize, quietLog()); err != nil {
		t.Fatal(err)
	}
	if n := backfillAuditCount(t, pool, wsA) + backfillAuditCount(t, pool, wsB); n != 2 {
		t.Fatalf("audit rows after first run = %d, want 2", n)
	}

	var before string
	if err := pool.QueryRow(ctx, `SELECT string_agg(workspace_id::text || ':' || version || ':' || updated_at::text, ',' ORDER BY workspace_id) FROM workspace_subscription WHERE workspace_id IN ($1, $2)`, wsA, wsB).Scan(&before); err != nil {
		t.Fatal(err)
	}

	if _, err := Run(ctx, scoped(pool, wsA, wsB), BatchSize, quietLog()); err != nil {
		t.Fatal(err)
	}
	if n := backfillAuditCount(t, pool, wsA) + backfillAuditCount(t, pool, wsB); n != 2 {
		t.Fatalf("audit rows after second run = %d, want 2", n)
	}
	var after string
	if err := pool.QueryRow(ctx, `SELECT string_agg(workspace_id::text || ':' || version || ':' || updated_at::text, ',' ORDER BY workspace_id) FROM workspace_subscription WHERE workspace_id IN ($1, $2)`, wsA, wsB).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("rows changed on second run:\n before %s\n after  %s", before, after)
	}
}

// faultRunner fails the audit write for one target, the way a full disk would.
type faultRunner struct {
	inner      Runner
	failTarget string
}

func (r faultRunner) InTx(ctx context.Context, fn func(Tx) error) error {
	return r.inner.InTx(ctx, func(tx Tx) error {
		return fn(faultTx{Tx: tx, failTarget: r.failTarget})
	})
}

type faultTx struct {
	Tx
	failTarget string
}

func (t faultTx) InsertAuditEvent(ctx context.Context, e audit.Entry) error {
	if e.TargetID == t.failTarget {
		return errors.New("audit write failed")
	}
	return t.Tx.InsertAuditEvent(ctx, e)
}

func TestRunRollsBackTheBatchWhoseAuditWriteFails(t *testing.T) {
	pool := seed(t, wsA, wsB, wsC)
	ctx := context.Background()
	// Batches of one: wsA commits, wsB's insert is rolled back with its failed
	// audit write, and wsC is never reached.
	runner := faultRunner{inner: scoped(pool, wsA, wsB, wsC), failTarget: wsB}
	if _, err := Run(ctx, runner, 1, quietLog()); err == nil {
		t.Fatal("Run returned nil, want the audit failure")
	}

	if n := subscriptionCount(t, pool, wsA); n != 1 {
		t.Fatalf("wsA rows = %d, want 1 (its batch committed)", n)
	}
	if n := subscriptionCount(t, pool, wsB); n != 0 {
		t.Fatalf("wsB rows = %d, want 0 (its batch rolled back)", n)
	}
	if n := subscriptionCount(t, pool, wsC); n != 0 {
		t.Fatalf("wsC rows = %d, want 0 (the run stopped before it)", n)
	}
	if n := backfillAuditCount(t, pool, wsB); n != 0 {
		t.Fatalf("wsB audit rows = %d, want 0", n)
	}
}

func TestRunRejectsEmptyBatch(t *testing.T) {
	pool := testdb.Open(t)
	if _, err := Run(context.Background(), NewPgxRunner(pool), 0, quietLog()); err == nil {
		t.Fatal("batch size 0 accepted")
	}
}
