package handler

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// stagedStaffRequest writes the row the platform writes in phase 1 of an
// immediate deletion (spec R6 section 6.3), so the Multica side can execute it.
// tenantID may be empty. The row goes away with the test.
func stagedStaffRequest(t *testing.T, kind, targetID, tenantID string) string {
	t.Helper()
	id := uuid.NewString()
	var tenant any
	if tenantID != "" {
		tenant = tenantID
	}
	dbfx.Exec(t, `INSERT INTO platform_deletion_request
		(id, kind, target_id, target_label, reason, state, tenant_id, requested_by, requested_by_label)
		VALUES ($1::uuid, $2, $3::uuid, 'staff target', 'handler test', 'running', $4, $5::uuid, 'staff tester')`,
		id, kind, targetID, tenant, uuid.NewString())
	dbfx.Cleanup(t, `DELETE FROM platform_deletion_request WHERE id = $1::uuid`, id)
	return id
}

// staffOrigin is the origin the staff_deletion job passes for a request.
func staffOrigin(requestID string) DeletionOrigin {
	return DeletionOrigin{
		Kind:       DeletionOriginStaff,
		RequestID:  requestID,
		StaffID:    uuid.NewString(),
		StaffLabel: "staff tester",
	}
}

// leftoverRows counts the rows that name workspace wsID in every table that
// has a workspace_id column, audit rows aside. The two deletion paths must leave
// the same counts.
func leftoverRows(t *testing.T, wsID string) map[string]int {
	t.Helper()
	ctx := context.Background()
	rows, err := testPool.Query(ctx, `SELECT table_name FROM information_schema.columns
		WHERE table_schema = 'public' AND column_name = 'workspace_id' AND table_name <> 'audit_event'
		ORDER BY table_name`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table: %v", err)
		}
		tables = append(tables, name)
	}
	rows.Close()
	out := map[string]int{}
	for _, table := range tables {
		var n int
		if err := testPool.QueryRow(ctx, `SELECT count(*) FROM "`+table+`" WHERE workspace_id = $1::uuid`, wsID).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if n > 0 {
			out[table] = n
		}
	}
	return out
}

func TestStaffDeletionDeletesWorkspaceThroughExistingTeardown(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ws := dbfx.Workspace(t, "staff delete", "stf-"+uuid.NewString()[:8])
	dbfx.Member(t, ws, testUserID, "owner")
	dbfx.Issue(t, "staff delete issue", testutil.Cols{"workspace_id": ws})
	dbfx.Cleanup(t, `DELETE FROM audit_event WHERE target_id = $1`, ws)
	reqID := stagedStaffRequest(t, "workspace", ws, "")

	done, err := testHandler.DeleteWorkspaceForStaff(context.Background(), ws, staffOrigin(reqID), nil)
	if err != nil || !done {
		t.Fatalf("DeleteWorkspaceForStaff = (%v, %v), want (true, nil)", done, err)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM workspace WHERE id = $1::uuid`, ws); n != 0 {
		t.Errorf("workspace rows = %d after the staff deletion, want 0", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM member WHERE workspace_id = $1::uuid`, ws); n != 0 {
		t.Errorf("member rows = %d after the staff deletion, want 0", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM issue WHERE workspace_id = $1::uuid`, ws); n != 0 {
		t.Errorf("issue rows = %d after the staff deletion, want 0", n)
	}
}

func TestStaffDeletionFailureRollsBackAndKeepsWorkspaceData(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ws := dbfx.Workspace(t, "staff rollback", "stf-rb-"+uuid.NewString()[:8])
	dbfx.Member(t, ws, testUserID, "owner")
	dbfx.Issue(t, "kept issue", testutil.Cols{"workspace_id": ws})
	dbfx.Cleanup(t, `DELETE FROM audit_event WHERE target_id = $1`, ws)
	reqID := stagedStaffRequest(t, "workspace", ws, "")

	inject := func(stage string) error {
		if stage == "audit" {
			return errors.New("injected failure before the audit row")
		}
		return nil
	}
	done, err := testHandler.DeleteWorkspaceForStaff(context.Background(), ws, staffOrigin(reqID), inject)
	if err == nil || done {
		t.Fatalf("DeleteWorkspaceForStaff = (%v, %v), want (false, error)", done, err)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM workspace WHERE id = $1::uuid`, ws); n != 1 {
		t.Errorf("workspace rows = %d after a failed deletion, want 1 (one transaction)", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM issue WHERE workspace_id = $1::uuid`, ws); n != 1 {
		t.Errorf("issue rows = %d after a failed deletion, want 1", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM audit_event WHERE target_id = $1 AND action = 'workspace.deleted'`, ws); n != 0 {
		t.Errorf("workspace.deleted rows = %d after a failed deletion, want 0", n)
	}
}

func TestStaffDeletionAuditActorIsStaffWithOriginDetail(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ws := dbfx.Workspace(t, "staff audit", "stf-au-"+uuid.NewString()[:8])
	dbfx.Member(t, ws, testUserID, "owner")
	dbfx.Cleanup(t, `DELETE FROM audit_event WHERE target_id = $1`, ws)
	reqID := stagedStaffRequest(t, "workspace", ws, "")
	origin := staffOrigin(reqID)

	if _, err := testHandler.DeleteWorkspaceForStaff(context.Background(), ws, origin, nil); err != nil {
		t.Fatalf("DeleteWorkspaceForStaff: %v", err)
	}
	n := dbfx.Count(t, `SELECT count(*) FROM audit_event
		WHERE action = 'workspace.deleted' AND actor_type = 'staff' AND actor_id = $1
		  AND detail->>'origin' = 'staff' AND detail->>'request_id' = $2`, origin.StaffID, reqID)
	if n != 1 {
		t.Errorf("workspace.deleted rows by staff with origin detail = %d, want 1", n)
	}
}

func TestStaffDeletionAuditRowKeepsTenantIDAfterWorkspaceIsGone(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ws := dbfx.Workspace(t, "staff tenant audit", "stf-ta-"+uuid.NewString()[:8])
	dbfx.Member(t, ws, testUserID, "owner")
	tenantID := tenantFixture(t, ws)
	dbfx.Cleanup(t, `DELETE FROM audit_event WHERE target_id = $1`, ws)
	reqID := stagedStaffRequest(t, "workspace", ws, tenantID)

	if _, err := testHandler.DeleteWorkspaceForStaff(context.Background(), ws, staffOrigin(reqID), nil); err != nil {
		t.Fatalf("DeleteWorkspaceForStaff: %v", err)
	}
	n := dbfx.Count(t, `SELECT count(*) FROM audit_event
		WHERE action = 'workspace.deleted' AND target_id = $1 AND workspace_id IS NULL AND tenant_id = $2::uuid`, ws, tenantID)
	if n != 1 {
		t.Errorf("workspace.deleted rows with a null workspace_id and the tenant id = %d, want 1", n)
	}
}

// The staff path must leave exactly the rows the existing teardown leaves.
func TestStaffDeletionEquivalentToDeleteWorkspace(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	build := func(name string) string {
		ws := dbfx.Workspace(t, name, "stf-eq-"+uuid.NewString()[:8])
		dbfx.Member(t, ws, testUserID, "owner")
		issue := dbfx.Issue(t, name+" issue", testutil.Cols{"workspace_id": ws})
		dbfx.Comment(t, issue, "comment", testutil.Cols{"workspace_id": ws})
		return ws
	}
	plain := build("eq plain")
	staff := build("eq staff")

	teardown(t, plain)
	reqID := stagedStaffRequest(t, "workspace", staff, "")
	if _, err := testHandler.DeleteWorkspaceForStaff(context.Background(), staff, staffOrigin(reqID), nil); err != nil {
		t.Fatalf("DeleteWorkspaceForStaff: %v", err)
	}
	want, got := leftoverRows(t, plain), leftoverRows(t, staff)
	if len(want) != len(got) {
		t.Errorf("tables with leftover rows: teardown %v, staff %v", want, got)
	}
	for table, n := range want {
		if got[table] != n {
			t.Errorf("table %s: teardown left %d rows, staff left %d", table, n, got[table])
		}
	}
}

func TestCreateWorkspaceIntoDeletingTenantIs409(t *testing.T) {
	requireTenantDB(t)
	tenantID, _ := ownedTenant(t, 1)
	stagedStaffRequest(t, "tenant", tenantID, tenantID)
	slug := newSlug()
	trackCreatedWorkspace(t, slug)

	testutil.Call(t, testHandler.CreateWorkspace, createWorkspaceRequest(slug, tenantID)).
		Want(http.StatusConflict)
	if n := dbfx.Count(t, `SELECT count(*) FROM workspace WHERE slug = $1`, slug); n != 0 {
		t.Errorf("workspace rows for %s = %d, want 0 (the request is refused)", slug, n)
	}
	if n := linksOf(t, tenantID); n != 1 {
		t.Errorf("workspaces in the deleting tenant = %d, want 1", n)
	}
}
