package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/server/internal/legalhold"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// placeHold writes the legal_hold row the platform writes when staff place a hold
// (spec R6 section 3c.7, D104).
func placeHold(t *testing.T, scopeKind, scopeID string) {
	t.Helper()
	dbfx.Exec(t, `INSERT INTO legal_hold (id, scope_kind, scope_id, reason, case_ref)
		VALUES (gen_random_uuid(), $1, $2::uuid, 'handler test reason', 'CASE-TEST')`, scopeKind, scopeID)
	dbfx.Cleanup(t, `DELETE FROM legal_hold WHERE scope_id = $1::uuid`, scopeID)
}

func linkWorkspaceToTenant(t *testing.T, wsID, tenantID string) {
	t.Helper()
	dbfx.Exec(t, `INSERT INTO tenant_workspace (workspace_id, tenant_id) VALUES ($1::uuid, $2::uuid)`, wsID, tenantID)
	dbfx.Cleanup(t, `DELETE FROM tenant_workspace WHERE workspace_id = $1::uuid`, wsID)
}

func newTenantForTest(t *testing.T) string {
	t.Helper()
	id := uuid.NewString()
	dbfx.Exec(t, `INSERT INTO tenant (id, name) VALUES ($1::uuid, 'legal hold tenant')`, id)
	dbfx.Cleanup(t, `DELETE FROM tenant WHERE id = $1::uuid`, id)
	return id
}

func legalHoldRowExists(t *testing.T, table, id string) bool {
	t.Helper()
	return dbfx.Count(t, `SELECT count(*) FROM `+table+` WHERE id = $1::uuid`, id) == 1
}

// A workspace hold refuses the staff teardown and leaves every row in place.
func TestWorkspaceHoldRefusesStaffDeletion(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ws := dbfx.Workspace(t, "held workspace", "held-"+uuid.NewString()[:8])
	dbfx.Member(t, ws, testUserID, "owner")
	placeHold(t, "workspace", ws)

	done, err := testHandler.DeleteWorkspaceForStaff(context.Background(), ws, staffOrigin(stagedStaffRequest(t, "workspace", ws, "")), nil)
	if done || !errors.Is(err, legalhold.ErrHeld) {
		t.Fatalf("DeleteWorkspaceForStaff = (%v, %v), want (false, legal hold)", done, err)
	}
	if !legalHoldRowExists(t, "workspace", ws) {
		t.Fatal("a held workspace was deleted")
	}
}

// A hold on the tenant covers every workspace of it, including one linked after
// the hold was placed.
func TestTenantHoldCoversWorkspaceLinkedLater(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	tenantID := newTenantForTest(t)
	placeHold(t, "tenant", tenantID)
	ws := dbfx.Workspace(t, "later linked", "later-"+uuid.NewString()[:8])
	dbfx.Member(t, ws, testUserID, "owner")
	linkWorkspaceToTenant(t, ws, tenantID)

	done, err := testHandler.DeleteWorkspaceForStaff(context.Background(), ws, staffOrigin(stagedStaffRequest(t, "workspace", ws, tenantID)), nil)
	if done || !errors.Is(err, legalhold.ErrHeld) {
		t.Fatalf("DeleteWorkspaceForStaff = (%v, %v), want (false, legal hold)", done, err)
	}
	if !legalHoldRowExists(t, "workspace", ws) {
		t.Fatal("a workspace of a held tenant was deleted")
	}
}

// The owner's own delete answers 409 legal_hold, with no reason or case reference.
func TestOwnerDeleteWorkspaceReturnsLegalHold409(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ws := dbfx.Workspace(t, "owner held", "owner-held-"+uuid.NewString()[:8])
	dbfx.Member(t, ws, testUserID, "owner")
	placeHold(t, "workspace", ws)

	req := withURLParam(newRequest("DELETE", "/api/workspaces/"+ws, nil), "id", ws)
	w := testutil.Call(t, testHandler.DeleteWorkspace, req).Want(http.StatusConflict)
	if body := w.Body.String(); !strings.Contains(body, "legal_hold") || strings.Contains(body, "CASE-TEST") || strings.Contains(body, "handler test reason") {
		t.Fatalf("body = %s, want legal_hold without the reason or case reference", body)
	}
	if !legalHoldRowExists(t, "workspace", ws) {
		t.Fatal("a held workspace was deleted by its owner")
	}
}

// A refused deletion leaves one audit row naming the hold id.
func TestDeletionBlockedByHoldWritesAuditRow(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ws := dbfx.Workspace(t, "audited hold", "audit-hold-"+uuid.NewString()[:8])
	dbfx.Member(t, ws, testUserID, "owner")
	placeHold(t, "workspace", ws)

	testHandler.DeleteWorkspaceForStaff(context.Background(), ws, staffOrigin(stagedStaffRequest(t, "workspace", ws, "")), nil)
	if n := dbfx.Count(t, `SELECT count(*) FROM audit_event WHERE workspace_id = $1::uuid AND action = $2 AND detail->>'legal_hold_id' IS NOT NULL`,
		ws, audit.ActionDeletionBlockedByHold); n != 1 {
		t.Fatalf("deletion.blocked_by_hold rows = %d, want 1", n)
	}
}

// An account hold refuses the account deletion, and so does a hold on a workspace
// the account is the only member of.
func TestAccountHoldRefusesAccountDeletion(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	uid := dbfx.User(t, "held account", "held-acct-"+uuid.NewString()[:8]+"@example.test")
	placeHold(t, "account", uid)

	done, err := testHandler.DeleteAccount(ctx, uid, time.Now())
	if done || !errors.Is(err, legalhold.ErrHeld) {
		t.Fatalf("DeleteAccount = (%v, %v), want (false, legal hold)", done, err)
	}
	if !legalHoldRowExists(t, `"user"`, uid) {
		t.Fatal("a held account was deleted")
	}
}

func TestDeleteAccountOfSoleMemberOfHeldWorkspaceIsRefused(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	uid := dbfx.User(t, "sole member", "sole-"+uuid.NewString()[:8]+"@example.test")
	ws := dbfx.Workspace(t, "sole held", "sole-held-"+uuid.NewString()[:8])
	dbfx.Member(t, ws, uid, "owner")
	placeHold(t, "workspace", ws)

	done, err := testHandler.DeleteAccount(ctx, uid, time.Now())
	if done || !errors.Is(err, legalhold.ErrHeld) {
		t.Fatalf("DeleteAccount = (%v, %v), want (false, legal hold)", done, err)
	}
	if !legalHoldRowExists(t, "workspace", ws) || !legalHoldRowExists(t, `"user"`, uid) {
		t.Fatal("a row was removed although the workspace is held")
	}
}

// Audit retention keeps the rows of a held workspace and deletes the others.
func TestAuditRetentionSkipsHeldRows(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	held := dbfx.Workspace(t, "retention held", "ret-held-"+uuid.NewString()[:8])
	free := dbfx.Workspace(t, "retention free", "ret-free-"+uuid.NewString()[:8])
	placeHold(t, "workspace", held)
	old := time.Now().AddDate(-3, 0, 0)
	for _, ws := range []string{held, free} {
		dbfx.Exec(t, `INSERT INTO audit_event (workspace_id, actor_type, action, created_at) VALUES ($1::uuid, 'system', 'test.old', $2)`, ws, old)
	}

	if _, err := testHandler.Queries.DeleteAuditEventsBefore(ctx, db.DeleteAuditEventsBeforeParams{
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		Limit:     1000000,
	}); err != nil {
		t.Fatal(err)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM audit_event WHERE workspace_id = $1::uuid AND action = 'test.old'`, held); n != 1 {
		t.Fatalf("held rows = %d, want 1", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM audit_event WHERE workspace_id = $1::uuid AND action = 'test.old'`, free); n != 0 {
		t.Fatalf("unheld rows = %d, want 0", n)
	}
}
