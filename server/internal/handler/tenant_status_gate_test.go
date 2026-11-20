package handler

import (
	"net/http"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// setTenantStatus writes the status the platform sets on a tenant (spec R6 3c.9, D106).
func setTenantStatus(t *testing.T, tenantID, status string) {
	t.Helper()
	dbfx.Exec(t, `UPDATE tenant SET status = $2 WHERE id = $1::uuid`, tenantID, status)
}

// A workspace may not join a tenant that is suspended or read only (spec R6 3c.9).
func TestCreateWorkspaceIntoSuspendedTenantIs409(t *testing.T) {
	requireTenantDB(t)
	tenantID, _ := ownedTenant(t, 1)
	setTenantStatus(t, tenantID, "suspended")

	slug := newSlug()
	trackCreatedWorkspace(t, slug)
	w := testutil.Call(t, testHandler.CreateWorkspace, createWorkspaceRequest(slug, tenantID)).Want(http.StatusConflict)
	if !strings.Contains(w.Body.String(), "tenant_not_active") {
		t.Fatalf("body = %s, want tenant_not_active", w.Body.String())
	}
}

func TestCreateWorkspaceIntoReadOnlyTenantIs409(t *testing.T) {
	requireTenantDB(t)
	tenantID, _ := ownedTenant(t, 1)
	setTenantStatus(t, tenantID, "read_only")

	slug := newSlug()
	trackCreatedWorkspace(t, slug)
	w := testutil.Call(t, testHandler.CreateWorkspace, createWorkspaceRequest(slug, tenantID)).Want(http.StatusConflict)
	if !strings.Contains(w.Body.String(), "tenant_not_active") {
		t.Fatalf("body = %s, want tenant_not_active", w.Body.String())
	}
}

// An active tenant takes the workspace as before.
func TestCreateWorkspaceIntoActiveTenantUnchanged(t *testing.T) {
	requireTenantDB(t)
	tenantID, _ := ownedTenant(t, 1)
	setTenantStatus(t, tenantID, "active")

	slug := newSlug()
	trackCreatedWorkspace(t, slug)
	testutil.Call(t, testHandler.CreateWorkspace, createWorkspaceRequest(slug, tenantID)).Want(http.StatusCreated)
	if n := linksOf(t, tenantID); n != 2 {
		t.Fatalf("tenant links = %d, want 2", n)
	}
}
