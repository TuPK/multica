package handler

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// tenantFixture makes a tenant with one linked row per workspace id, removed at
// the end of the test whatever the test deleted.
func tenantFixture(t *testing.T, workspaceIDs ...string) string {
	t.Helper()
	tenantID := uuid.NewString()
	dbfx.Exec(t, `INSERT INTO tenant (id, name) VALUES ($1::uuid, 'handler test tenant')`, tenantID)
	dbfx.Cleanup(t, `DELETE FROM tenant WHERE id = $1::uuid`, tenantID)
	for _, ws := range workspaceIDs {
		dbfx.Exec(t, `INSERT INTO tenant_workspace (workspace_id, tenant_id) VALUES ($1::uuid, $2::uuid)`, ws, tenantID)
		dbfx.Cleanup(t, `DELETE FROM tenant_workspace WHERE workspace_id = $1::uuid`, ws)
	}
	return tenantID
}

// teardown runs deleteWorkspaceData in its own transaction and commits.
func teardown(t *testing.T, wsID string) {
	t.Helper()
	ctx := context.Background()
	tx, err := testHandler.TxStarter.Begin(ctx)
	if err != nil {
		t.Fatalf("begin teardown: %v", err)
	}
	defer tx.Rollback(ctx)
	if _, err := testHandler.deleteWorkspaceData(ctx, tx, parseUUID(wsID)); err != nil {
		t.Fatalf("teardown of %s: %v", wsID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit teardown: %v", err)
	}
}

func TestDeleteWorkspaceDetachesTenant(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	a := dbfx.Workspace(t, "tenant detach a", "tnt-a-"+uuid.NewString()[:8])
	b := dbfx.Workspace(t, "tenant detach b", "tnt-b-"+uuid.NewString()[:8])
	tenantID := tenantFixture(t, a, b)

	teardown(t, a)
	if n := dbfx.Count(t, `SELECT count(*) FROM tenant WHERE id = $1::uuid`, tenantID); n != 1 {
		t.Errorf("tenant rows = %d after the first workspace went, want 1", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM tenant_workspace WHERE tenant_id = $1::uuid`, tenantID); n != 1 {
		t.Errorf("links = %d after the first workspace went, want 1", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM tenant_workspace WHERE workspace_id = $1::uuid`, a); n != 0 {
		t.Errorf("the deleted workspace still has a link")
	}

	teardown(t, b)
	if n := dbfx.Count(t, `SELECT count(*) FROM tenant WHERE id = $1::uuid`, tenantID); n != 0 {
		t.Errorf("tenant rows = %d after the last workspace went, want 0", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM tenant_workspace WHERE tenant_id = $1::uuid`, tenantID); n != 0 {
		t.Errorf("links = %d after the last workspace went, want 0", n)
	}
}

// A creation holds the tenant lock while it counts the tenant's workspaces; a
// deletion of one of them must wait for it.
func TestDeleteWorkspaceTenantDetachWaitsForTenantLock(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	a := dbfx.Workspace(t, "tenant lock a", "tnt-lock-"+uuid.NewString()[:8])
	tenantID := tenantFixture(t, a)

	holder, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Rollback(ctx)
	if _, err := holder.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1::text), hashtext('max_workspaces'))`, tenantID); err != nil {
		t.Fatalf("hold the tenant lock: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		teardown(t, a)
	}()
	select {
	case <-done:
		t.Fatal("the deletion finished while another transaction held the tenant lock")
	case <-time.After(400 * time.Millisecond):
	}
	if err := holder.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the deletion did not finish after the tenant lock was released")
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM tenant WHERE id = $1::uuid`, tenantID); n != 0 {
		t.Errorf("tenant rows = %d, want 0", n)
	}
}

// Deleting a tenant's last workspace deletes the tenant and leaves the staff row
// that points at it alone: the admin then has an empty scope until a
// system_admin reassigns or revokes them (spec R6 section 3.5, rule 5).
func TestDeleteWorkspaceLeavesPlatformStaffRowsUntouched(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	a := dbfx.Workspace(t, "tenant staff", "tnt-staff-"+uuid.NewString()[:8])
	tenantID := tenantFixture(t, a)
	adminID := dbfx.User(t, "Tenant Admin", testutil.UniqueEmail("tenant-admin"))
	dbfx.Exec(t, `INSERT INTO platform_staff (user_id, role, tenant_id) VALUES ($1::uuid, 'admin', $2::uuid)`, adminID, tenantID)
	dbfx.Cleanup(t, `DELETE FROM platform_staff WHERE user_id = $1::uuid`, adminID)

	before := dbfx.Count(t, `SELECT count(*) FROM platform_staff WHERE user_id = $1::uuid AND role = 'admin' AND tenant_id = $2::uuid AND revoked_at IS NULL`, adminID, tenantID)
	if before != 1 {
		t.Fatalf("fixture: %d staff rows", before)
	}
	teardown(t, a)
	if n := dbfx.Count(t, `SELECT count(*) FROM tenant WHERE id = $1::uuid`, tenantID); n != 0 {
		t.Fatalf("tenant rows = %d, want 0", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM platform_staff WHERE user_id = $1::uuid AND role = 'admin' AND tenant_id = $2::uuid AND revoked_at IS NULL`, adminID, tenantID); n != 1 {
		t.Errorf("the staff row changed: %d rows still match, want 1", n)
	}
}
