package auditstore

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multica-ai/multica/platform-management/audit"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// A tenant-scoped entry keeps its tenant_id; an entry with none stores NULL.
func TestAuditStoreWritesTenantID(t *testing.T) {
	dsn := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PLATFORM_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	const (
		withTenant    = "auditstore-tenant-with"
		withoutTenant = "auditstore-tenant-without"
		tenantID      = "00000000-0000-0000-0000-00000000d001"
	)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM audit_event WHERE request_id IN ($1, $2)`, withTenant, withoutTenant)
	})

	store := New(db.New(pool))
	for _, e := range []audit.Entry{
		{ActorType: audit.ActorStaff, Action: audit.ActionStaffAdded, RequestID: withTenant, TenantID: tenantID},
		{ActorType: audit.ActorStaff, Action: audit.ActionStaffAdded, RequestID: withoutTenant},
	} {
		if err := audit.Record(ctx, store, e); err != nil {
			t.Fatalf("record %s: %v", e.RequestID, err)
		}
	}

	var got *string
	if err := pool.QueryRow(ctx, `SELECT tenant_id::text FROM audit_event WHERE request_id = $1`, withTenant).Scan(&got); err != nil {
		t.Fatalf("read tenant row: %v", err)
	}
	if got == nil || *got != tenantID {
		t.Errorf("tenant_id = %v, want %s", got, tenantID)
	}
	if err := pool.QueryRow(ctx, `SELECT tenant_id::text FROM audit_event WHERE request_id = $1`, withoutTenant).Scan(&got); err != nil {
		t.Fatalf("read plain row: %v", err)
	}
	if got != nil {
		t.Errorf("tenant_id = %v, want NULL", *got)
	}
}
