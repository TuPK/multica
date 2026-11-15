package workspacestatus

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/testdb"
	"github.com/multica-ai/multica/platform-management/workspacestate"
)

// Database tests: skipped unless PLATFORM_TEST_DATABASE_URL names a *_test
// database and DATABASE_URL equals it. Each test uses a fresh random workspace
// id (no workspace row is needed: the table has no foreign key) and removes the
// rows it wrote.

type dbFixture struct {
	t     *testing.T
	pool  *pgxpool.Pool
	ws    string
	staff string
}

func newDBFixture(t *testing.T) *dbFixture {
	t.Helper()
	pool := testdb.Open(t)
	f := &dbFixture{t: t, pool: pool}
	ctx := context.Background()
	if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&f.ws); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&f.staff); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM platform_workspace WHERE workspace_id = $1`, f.ws)
		pool.Exec(ctx, `DELETE FROM audit_event WHERE workspace_id = $1`, f.ws)
	})
	return f
}

// recordingCache is the Redis stand-in: it holds the key Set must delete.
type recordingCache struct {
	m    map[string][]byte
	dels []string
}

func (c *recordingCache) Get(context.Context, string) ([]byte, bool, error) { return nil, false, nil }
func (c *recordingCache) Set(_ context.Context, k string, v []byte, _ time.Duration) error {
	c.m[k] = v
	return nil
}
func (c *recordingCache) Del(_ context.Context, k string) error {
	delete(c.m, k)
	c.dels = append(c.dels, k)
	return nil
}

func (f *dbFixture) service(cache *recordingCache) *Service {
	return New(NewPgxRunner(f.pool), workspacestate.New(nil, cache), slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func (f *dbFixture) auditCount() int {
	f.t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_event WHERE workspace_id = $1 AND action = $2 AND actor_type = 'staff' AND actor_id = $3`,
		f.ws, AuditAction, f.staff).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

func TestDBSetWritesRowAuditAndClearsTheCacheKey(t *testing.T) {
	f := newDBFixture(t)
	ctx := context.Background()
	key := workspacestate.Key(f.ws)
	cache := &recordingCache{m: map[string][]byte{key: []byte(`{"found":false}`)}}
	svc := f.service(cache)

	scheduled := time.Now().Add(30 * 24 * time.Hour).UTC().Truncate(time.Second)
	changed, err := svc.Set(ctx, Change{WorkspaceID: f.ws, Status: workspacestate.PendingDeletion, Reason: "owner request", ChangedBy: f.staff, DeletionScheduledAt: scheduled})
	if err != nil || !changed {
		t.Fatalf("Set = (%v, %v)", changed, err)
	}

	var status, reason, by string
	var at *time.Time
	if err := f.pool.QueryRow(ctx,
		`SELECT status, reason, changed_by::text, deletion_scheduled_at FROM platform_workspace WHERE workspace_id = $1`, f.ws).
		Scan(&status, &reason, &by, &at); err != nil {
		t.Fatalf("read row: %v", err)
	}
	if status != "pending_deletion" || reason != "owner request" || by != f.staff || at == nil || !at.Equal(scheduled) {
		t.Fatalf("row = %q %q %q %v", status, reason, by, at)
	}
	if n := f.auditCount(); n != 1 {
		t.Fatalf("audit rows = %d, want 1", n)
	}
	if _, ok := cache.m[key]; ok || len(cache.dels) != 1 || cache.dels[0] != key {
		t.Fatalf("cache key not deleted: %v %v", cache.m, cache.dels)
	}
}

func TestDBSetSameStatusWritesNothing(t *testing.T) {
	f := newDBFixture(t)
	ctx := context.Background()
	cache := &recordingCache{m: map[string][]byte{}}
	svc := f.service(cache)
	c := Change{WorkspaceID: f.ws, Status: workspacestate.ReadOnly, Reason: "grace period ended", ChangedBy: f.staff}
	if changed, err := svc.Set(ctx, c); err != nil || !changed {
		t.Fatalf("first Set = (%v, %v)", changed, err)
	}
	var firstAt time.Time
	f.pool.QueryRow(ctx, `SELECT changed_at FROM platform_workspace WHERE workspace_id = $1`, f.ws).Scan(&firstAt)

	c.Reason = "again"
	if changed, err := svc.Set(ctx, c); err != nil || changed {
		t.Fatalf("second Set = (%v, %v), want unchanged", changed, err)
	}
	var reason string
	var secondAt time.Time
	f.pool.QueryRow(ctx, `SELECT reason, changed_at FROM platform_workspace WHERE workspace_id = $1`, f.ws).Scan(&reason, &secondAt)
	if reason != "grace period ended" || !secondAt.Equal(firstAt) {
		t.Fatalf("a same-status Set rewrote the row: %q %v %v", reason, firstAt, secondAt)
	}
	if n := f.auditCount(); n != 1 || len(cache.dels) != 1 {
		t.Fatalf("audit rows = %d, cache deletions = %d, want 1 and 1", n, len(cache.dels))
	}
}

func TestDBSetBackToActiveKeepsARowAndAnAuditTrail(t *testing.T) {
	f := newDBFixture(t)
	ctx := context.Background()
	svc := f.service(&recordingCache{m: map[string][]byte{}})
	for _, s := range []workspacestate.Status{workspacestate.Suspended, workspacestate.Active} {
		if changed, err := svc.Set(ctx, Change{WorkspaceID: f.ws, Status: s, Reason: "r", ChangedBy: f.staff}); err != nil || !changed {
			t.Fatalf("Set(%s) = (%v, %v)", s, changed, err)
		}
	}
	var status string
	if err := f.pool.QueryRow(ctx, `SELECT status FROM platform_workspace WHERE workspace_id = $1`, f.ws).Scan(&status); err != nil || status != "active" {
		t.Fatalf("status = %q, %v", status, err)
	}
	if n := f.auditCount(); n != 2 {
		t.Fatalf("audit rows = %d, want 2", n)
	}
}

// A failure after the row is written must leave neither the row nor the audit
// entry: both belong to one transaction.
func TestDBSetRollsBackTheRowWhenTheAuditInsertFails(t *testing.T) {
	f := newDBFixture(t)
	ctx := context.Background()
	failing := &failingAuditRunner{Runner: NewPgxRunner(f.pool)}
	svc := New(failing, workspacestate.New(nil, &recordingCache{m: map[string][]byte{}}), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := svc.Set(ctx, Change{WorkspaceID: f.ws, Status: workspacestate.Suspended, Reason: "r", ChangedBy: f.staff}); err == nil {
		t.Fatal("Set succeeded despite the audit failure")
	}
	var n int
	f.pool.QueryRow(ctx, `SELECT count(*) FROM platform_workspace WHERE workspace_id = $1`, f.ws).Scan(&n)
	if n != 0 {
		t.Fatalf("the row survived a failed audit insert: %d", n)
	}
}

type failingAuditRunner struct{ Runner }

func (r *failingAuditRunner) InTx(ctx context.Context, fn func(Tx) error) error {
	return r.Runner.InTx(ctx, func(tx Tx) error { return fn(failingAuditTx{tx}) })
}

type failingAuditTx struct{ Tx }

func (failingAuditTx) InsertAuditEvent(context.Context, audit.Entry) error {
	return errors.New("audit insert failed")
}
