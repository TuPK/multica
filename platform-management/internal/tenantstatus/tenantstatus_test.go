package tenantstatus

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/multica-ai/multica/platform-management/internal/testdb"
)

// Spec R6 section 3c.9 rule 1: a tenant lock writes every workspace, and an
// unlock restores only the workspaces the tenant lock set.

func TestLockWritesEveryWorkspaceNotAlreadyInThatState(t *testing.T) {
	rows := []WorkspaceStatus{
		{ID: "a", Status: StatusActive},
		{ID: "b", Status: StatusReadOnly, Reason: "billing"},
		{ID: "c", Status: StatusSuspended, Reason: ReasonTenant},
	}
	got := Targets(rows, StatusSuspended)
	if !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("lock targets = %v", got)
	}
}

func TestUnlockRestoresOnlyTenantLockedWorkspaces(t *testing.T) {
	rows := []WorkspaceStatus{
		{ID: "a", Status: StatusSuspended, Reason: ReasonTenant},
		{ID: "b", Status: StatusSuspended, Reason: "fraud review"},
		{ID: "c", Status: StatusActive},
	}
	got := Targets(rows, StatusActive)
	if !slices.Equal(got, []string{"a"}) {
		t.Fatalf("unlock targets = %v, want only the tenant-locked workspace", got)
	}
}

func TestValidStatuses(t *testing.T) {
	for _, ok := range []string{StatusActive, StatusReadOnly, StatusSuspended} {
		if !Valid(ok) {
			t.Errorf("%s must be valid", ok)
		}
	}
	if Valid("pending_deletion") {
		t.Error("pending_deletion is not a tenant status")
	}
}

// countingBeginner counts the transactions the service opens.
type countingBeginner struct {
	pool   *pgxpool.Pool
	begins int
}

func (c *countingBeginner) Begin(ctx context.Context) (pgx.Tx, error) {
	c.begins++
	return c.pool.Begin(ctx)
}

// Spec R6 section 3c.9 rule 1: a tenant lock writes every workspace of the tenant,
// the tenant row and one audit row in a single transaction.
func TestTenantStatusWritesAllWorkspacesInOneTransaction(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	var tenant, a, b string
	if err := pool.QueryRow(ctx, `INSERT INTO tenant (id, name) VALUES (gen_random_uuid(), 'Tenant status test ' || substr(md5(random()::text), 1, 8)) RETURNING id::text`).Scan(&tenant); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	for _, ws := range []*string{&a, &b} {
		if err := pool.QueryRow(ctx, `INSERT INTO workspace (name, slug) VALUES ('Tenant status test', 'ts-' || substr(md5(random()::text), 1, 12)) RETURNING id::text`).Scan(ws); err != nil {
			t.Fatalf("insert workspace: %v", err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO tenant_workspace (workspace_id, tenant_id) VALUES ($1, $2)`, *ws, tenant); err != nil {
			t.Fatalf("link workspace: %v", err)
		}
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM audit_event WHERE tenant_id = $1`, tenant)
		pool.Exec(ctx, `DELETE FROM platform_workspace WHERE workspace_id = ANY($1::uuid[])`, []string{a, b})
		pool.Exec(ctx, `DELETE FROM tenant_workspace WHERE tenant_id = $1`, tenant)
		pool.Exec(ctx, `DELETE FROM workspace WHERE id = ANY($1::uuid[])`, []string{a, b})
		pool.Exec(ctx, `DELETE FROM tenant WHERE id = $1`, tenant)
	})

	var staff string
	if err := pool.QueryRow(ctx, `INSERT INTO "user" (name, email) VALUES ('Tenant status test', 'ts-' || substr(md5(random()::text), 1, 12) || '@example.test') RETURNING id::text`).Scan(&staff); err != nil {
		t.Fatalf("insert staff user: %v", err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM "user" WHERE id = $1`, staff) })

	begins := &countingBeginner{pool: pool}
	ids, err := New(begins, nil).SetStatus(ctx, Actor{ID: staff, Label: "root@example.test"}, tenant, StatusSuspended, "review")
	if err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	if len(ids) != 2 || begins.begins != 1 {
		t.Fatalf("written workspaces = %v in %d transactions, want 2 in 1", ids, begins.begins)
	}
	for _, ws := range []string{a, b} {
		var status string
		if err := pool.QueryRow(ctx, `SELECT status FROM platform_workspace WHERE workspace_id = $1`, ws).Scan(&status); err != nil || status != StatusSuspended {
			t.Fatalf("workspace %s status = %q (%v), want suspended", ws, status, err)
		}
	}
	var tenantStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM tenant WHERE id = $1`, tenant).Scan(&tenantStatus); err != nil || tenantStatus != StatusSuspended {
		t.Fatalf("tenant status = %q (%v), want suspended", tenantStatus, err)
	}
}
