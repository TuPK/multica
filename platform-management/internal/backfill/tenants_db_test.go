package backfill

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	tkept = "a0000000-0000-4000-8000-0000000000e1" // the tenant of wsLinked, not keyed by its workspace
	wsLnk = "a0000000-0000-4000-8000-0000000000e2"
)

// scopedTenants limits a run to the given workspaces: the test database is
// shared, and an unscoped run would give tenants to other tests' workspaces.
type scopedTenants struct {
	inner TenantRunner
	ids   map[string]bool
	// failAt, when positive, fails the batch with that 1-based number after it
	// wrote its rows, so the rollback is what the test observes.
	failAt int
	runs   int
	mu     sync.Mutex
}

func scopedTenantRunner(pool *pgxpool.Pool, ids ...string) *scopedTenants {
	set := map[string]bool{}
	for _, id := range ids {
		set[id] = true
	}
	return &scopedTenants{inner: NewPgxTenantRunner(pool), ids: set}
}

func (r *scopedTenants) InTx(ctx context.Context, fn func(TenantTx) error) error {
	r.mu.Lock()
	r.runs++
	n := r.runs
	r.mu.Unlock()
	return r.inner.InTx(ctx, func(tx TenantTx) error {
		if err := fn(scopedTenantTx{TenantTx: tx, ids: r.ids}); err != nil {
			return err
		}
		if r.failAt == n {
			return errors.New("injected batch failure")
		}
		return nil
	})
}

type scopedTenantTx struct {
	TenantTx
	ids map[string]bool
}

func (t scopedTenantTx) WithoutTenant(ctx context.Context, limit int) ([]Candidate, error) {
	all, err := t.TenantTx.WithoutTenant(ctx, math.MaxInt32)
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

func seedTenants(t *testing.T, ids ...string) *pgxpool.Pool {
	t.Helper()
	pool := seed(t, ids...)
	t.Cleanup(func() {
		for _, id := range append(ids, tkept) {
			_, _ = pool.Exec(context.Background(), `DELETE FROM tenant_workspace WHERE workspace_id = $1 OR tenant_id = $1`, id)
			_, _ = pool.Exec(context.Background(), `DELETE FROM tenant WHERE id = $1`, id)
		}
	})
	return pool
}

func tenantRows(t *testing.T, pool *pgxpool.Pool, ws string) (tenants, ownLinks, links int) {
	t.Helper()
	q := func(sql string) int {
		var n int
		if err := pool.QueryRow(context.Background(), sql, ws).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	return q(`SELECT count(*) FROM tenant WHERE id = $1`),
		q(`SELECT count(*) FROM tenant_workspace WHERE workspace_id = $1 AND tenant_id = $1`),
		q(`SELECT count(*) FROM tenant_workspace WHERE workspace_id = $1`)
}

func TestBackfillTenantsCreatesOneTenantPerOrphanWorkspace(t *testing.T) {
	pool := seedTenants(t, wsA, wsB, wsLnk)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO tenant (id, name) VALUES ($1, 'kept')`, tkept); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tenant_workspace (workspace_id, tenant_id) VALUES ($1, $2)`, wsLnk, tkept); err != nil {
		t.Fatal(err)
	}

	res, err := RunTenants(ctx, scopedTenantRunner(pool, wsA, wsB, wsLnk), 1, quietLog())
	if err != nil {
		t.Fatal(err)
	}
	if res.Tenants != 2 || res.Linked != 2 {
		t.Errorf("result = %+v, want 2 tenants and 2 links for the 2 orphans", res)
	}
	for _, ws := range []string{wsA, wsB} {
		if tenants, own, links := tenantRows(t, pool, ws); tenants != 1 || own != 1 || links != 1 {
			t.Errorf("%s: tenants=%d own links=%d links=%d, want 1 each", ws, tenants, own, links)
		}
	}
	var name string
	if err := pool.QueryRow(ctx, `SELECT name FROM tenant WHERE id = $1`, wsA).Scan(&name); err != nil || name != "backfill "+wsA {
		t.Errorf("tenant name = %q (%v), want the workspace's name", name, err)
	}
	// The linked workspace keeps its link and gets no tenant of its own.
	var tenantOf string
	if err := pool.QueryRow(ctx, `SELECT tenant_id::text FROM tenant_workspace WHERE workspace_id = $1`, wsLnk).Scan(&tenantOf); err != nil || tenantOf != tkept {
		t.Errorf("linked workspace tenant = %q (%v), want %s unchanged", tenantOf, err, tkept)
	}
	if tenants, _, _ := tenantRows(t, pool, wsLnk); tenants != 0 {
		t.Errorf("the linked workspace got a tenant of its own")
	}
}

func TestBackfillTenantsIsIdempotentAndParallelSafe(t *testing.T) {
	pool := seedTenants(t, wsA, wsB, wsC)
	ctx := context.Background()
	runner := scopedTenantRunner(pool, wsA, wsB, wsC)

	for i := 0; i < 2; i++ {
		if _, err := RunTenants(ctx, runner, 10, quietLog()); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}
	again, err := RunTenants(ctx, runner, 10, quietLog())
	if err != nil || again.Tenants != 0 || again.Linked != 0 {
		t.Fatalf("a third run = %+v, %v, want nothing to do", again, err)
	}

	// Two runs at once over workspaces that still have no tenant.
	if _, err := pool.Exec(ctx, `DELETE FROM tenant_workspace WHERE workspace_id = ANY($1)`, []string{wsA, wsB, wsC}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM tenant WHERE id = ANY($1)`, []string{wsA, wsB, wsC}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	start := make(chan struct{})
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = RunTenants(ctx, scopedTenantRunner(pool, wsA, wsB, wsC), 1, quietLog())
		}()
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("parallel run %d: %v", i, err)
		}
	}
	for _, ws := range []string{wsA, wsB, wsC} {
		if tenants, own, links := tenantRows(t, pool, ws); tenants != 1 || own != 1 || links != 1 {
			t.Errorf("%s after two parallel runs: tenants=%d own links=%d links=%d, want 1 each", ws, tenants, own, links)
		}
	}
}

// A batch that fails rolls back alone: the batches before it stay, and the next
// run finishes the rest.
func TestBackfillTenantsBatchesAreIndependent(t *testing.T) {
	pool := seedTenants(t, wsA, wsB, wsC)
	ctx := context.Background()
	runner := scopedTenantRunner(pool, wsA, wsB, wsC)
	runner.failAt = 2

	res, err := RunTenants(ctx, runner, 1, quietLog())
	if err == nil {
		t.Fatal("the injected failure was swallowed")
	}
	if res.Batches != 1 {
		t.Errorf("batches before the failure = %d, want 1", res.Batches)
	}
	if tenants, own, _ := tenantRows(t, pool, wsA); tenants != 1 || own != 1 {
		t.Errorf("the first batch was lost: tenants=%d own links=%d", tenants, own)
	}
	for _, ws := range []string{wsB, wsC} {
		if tenants, _, links := tenantRows(t, pool, ws); tenants != 0 || links != 0 {
			t.Errorf("%s: the failed batch left rows: tenants=%d links=%d", ws, tenants, links)
		}
	}

	runner.failAt = 0
	if _, err := RunTenants(ctx, runner, 1, quietLog()); err != nil {
		t.Fatal(err)
	}
	for _, ws := range []string{wsA, wsB, wsC} {
		if tenants, own, links := tenantRows(t, pool, ws); tenants != 1 || own != 1 || links != 1 {
			t.Errorf("%s after the rerun: tenants=%d own links=%d links=%d", ws, tenants, own, links)
		}
	}
}
