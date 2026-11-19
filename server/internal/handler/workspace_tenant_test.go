package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/entitlement"
	"github.com/multica-ai/multica/server/internal/entitlement/entitlementtest"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// requireTenantDB skips the test when the package has no database.
func requireTenantDB(t *testing.T) {
	t.Helper()
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
}

// ownedTenant makes a tenant whose n workspaces the suite's user owns, oldest
// first, and returns the tenant id and the workspace ids. The rows go away with
// the test.
func ownedTenant(t *testing.T, n int) (string, []string) {
	t.Helper()
	var ids []string
	for i := 0; i < n; i++ {
		ws := dbfx.Workspace(t, "tenant gate", "tnt-gate-"+uuid.NewString()[:8])
		dbfx.Member(t, ws, testUserID, "owner")
		ids = append(ids, ws)
	}
	return tenantFixture(t, ids...), ids
}

// trackCreatedWorkspace removes what CreateWorkspace made for the slug, tenant
// rows included.
func trackCreatedWorkspace(t *testing.T, slug string) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		testPool.Exec(ctx, `DELETE FROM tenant WHERE id IN (SELECT id FROM workspace WHERE slug = $1)`, slug)
		testPool.Exec(ctx, `DELETE FROM tenant_workspace WHERE workspace_id IN (SELECT id FROM workspace WHERE slug = $1)`, slug)
		testPool.Exec(ctx, `DELETE FROM audit_event WHERE workspace_id IN (SELECT id FROM workspace WHERE slug = $1)`, slug)
		testPool.Exec(ctx, `DELETE FROM workspace WHERE slug = $1`, slug)
	})
}

// limitTenantAt makes the max_workspaces gate enforce limit for the policy of
// workspace ws (the workspace CreateWorkspace reads it from).
func limitTenantAt(t *testing.T, ws string, limit int) {
	t.Helper()
	stub := entitlementtest.New()
	stub.Set(uuid.MustParse(ws), entitlement.GateMaxWorkspaces, entitlement.Decision{
		Gate: entitlement.Gate{Action: entitlement.ActionEnforce, Limit: &limit},
	})
	prior := testHandler.Entitlements
	testHandler.Entitlements = stub
	t.Cleanup(func() { testHandler.Entitlements = prior })
}

type createBody map[string]any

func createWorkspaceRequest(slug, tenantID string) *http.Request {
	body := createBody{"name": "Tenant gate " + slug, "slug": slug}
	if tenantID != "" {
		body["tenant_id"] = tenantID
	}
	return newRequest("POST", "/api/workspaces", body)
}

func newSlug() string { return "tnt-new-" + uuid.NewString()[:8] }

func linksOf(t *testing.T, tenantID string) int {
	t.Helper()
	return dbfx.Count(t, `SELECT count(*) FROM tenant_workspace WHERE tenant_id = $1::uuid`, tenantID)
}

func TestCreateWorkspaceWithoutTenantIDKeepsResponseAndAddsTenantID(t *testing.T) {
	requireTenantDB(t)
	slug := newSlug()
	trackCreatedWorkspace(t, slug)
	before := dbfx.Count(t, `SELECT count(*) FROM tenant`)

	resp := testutil.Call(t, testHandler.CreateWorkspace, createWorkspaceRequest(slug, "")).Want(http.StatusCreated)
	var body map[string]any
	resp.JSON(&body)
	for _, key := range []string{"id", "name", "slug", "description", "context", "settings", "repos", "issue_prefix", "avatar_url", "secretary_agent_id", "created_at", "updated_at"} {
		if _, ok := body[key]; !ok {
			t.Errorf("response lost the field %q", key)
		}
	}
	id, _ := body["id"].(string)
	if body["tenant_id"] != id || id == "" {
		t.Errorf("tenant_id = %v, want the new workspace's id %q (a tenant of its own)", body["tenant_id"], id)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM tenant`); n != before+1 {
		t.Errorf("tenants went from %d to %d, want one more", before, n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM tenant_workspace WHERE workspace_id = $1::uuid AND tenant_id = $1::uuid`, id); n != 1 {
		t.Errorf("links to its own tenant = %d, want 1", n)
	}
}

func TestCreateWorkspaceIntoTenantAsOwner(t *testing.T) {
	requireTenantDB(t)
	tenantID, _ := ownedTenant(t, 1)
	slug := newSlug()
	trackCreatedWorkspace(t, slug)
	before := dbfx.Count(t, `SELECT count(*) FROM tenant`)

	resp := testutil.Call(t, testHandler.CreateWorkspace, createWorkspaceRequest(slug, tenantID)).Want(http.StatusCreated)
	var created map[string]any
	resp.JSON(&created)
	if created["tenant_id"] != tenantID {
		t.Errorf("tenant_id = %v, want %s", created["tenant_id"], tenantID)
	}
	if n := linksOf(t, tenantID); n != 2 {
		t.Errorf("workspaces in the tenant = %d, want 2", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM tenant`); n != before {
		t.Errorf("a tenant was created: %d -> %d", before, n)
	}
	// The caller is the owner of the new workspace.
	if n := dbfx.Count(t, `SELECT count(*) FROM member WHERE workspace_id = $1::uuid AND user_id = $2::uuid AND role = 'owner'`, created["id"], testUserID); n != 1 {
		t.Errorf("owner rows = %d, want 1", n)
	}
}

// A tenant the caller does not own a workspace of, one they only belong to, and
// one that does not exist all answer the same 404.
func TestCreateWorkspaceIntoForeignTenantIs404(t *testing.T) {
	requireTenantDB(t)
	stranger := dbfx.User(t, "Tenant stranger", testutil.UniqueEmail("tenant-stranger"))
	foreignWS := dbfx.Workspace(t, "foreign tenant", "tnt-foreign-"+uuid.NewString()[:8])
	dbfx.Member(t, foreignWS, stranger, "owner")
	foreign := tenantFixture(t, foreignWS)

	memberWS := dbfx.Workspace(t, "member tenant", "tnt-member-"+uuid.NewString()[:8])
	dbfx.Member(t, memberWS, stranger, "owner")
	dbfx.Member(t, memberWS, testUserID, "member")
	asMember := tenantFixture(t, memberWS)

	var bodies []string
	for name, tenantID := range map[string]string{"foreign": foreign, "member only": asMember, "missing": uuid.NewString()} {
		slug := newSlug()
		trackCreatedWorkspace(t, slug)
		resp := testutil.Call(t, testHandler.CreateWorkspace, createWorkspaceRequest(slug, tenantID))
		resp.Want(http.StatusNotFound)
		var body struct {
			Code string `json:"code"`
		}
		resp.JSON(&body)
		if body.Code != "tenant_not_found" {
			t.Errorf("%s: code = %q, want tenant_not_found; body %s", name, body.Code, resp.Text())
		}
		bodies = append(bodies, resp.Text())
		if n := dbfx.Count(t, `SELECT count(*) FROM workspace WHERE slug = $1`, slug); n != 0 {
			t.Errorf("%s: a refused request created a workspace", name)
		}
	}
	for _, b := range bodies[1:] {
		if b != bodies[0] {
			t.Errorf("the 404 bodies differ: %q and %q", bodies[0], b)
		}
	}
	if n := linksOf(t, foreign); n != 1 {
		t.Errorf("the foreign tenant has %d workspaces, want 1", n)
	}
}

func TestCreateWorkspaceIntoTenantRefusedAtMaxWorkspaces(t *testing.T) {
	requireTenantDB(t)
	tenantID, ws := ownedTenant(t, 2)
	limitTenantAt(t, ws[0], 2)
	slug := newSlug()
	trackCreatedWorkspace(t, slug)

	wantPlanLimit(t, testutil.Call(t, testHandler.CreateWorkspace, createWorkspaceRequest(slug, tenantID)), "max_workspaces", 2, 2)
	if n := dbfx.Count(t, `SELECT count(*) FROM workspace WHERE slug = $1`, slug); n != 0 {
		t.Errorf("a refused creation left a workspace")
	}
	if n := linksOf(t, tenantID); n != 2 {
		t.Errorf("workspaces in the tenant = %d, want 2", n)
	}

	// One slot more and it passes.
	limitTenantAt(t, ws[0], 3)
	testutil.Call(t, testHandler.CreateWorkspace, createWorkspaceRequest(slug, tenantID)).Want(http.StatusCreated)
	if n := linksOf(t, tenantID); n != 3 {
		t.Errorf("workspaces in the tenant = %d, want 3", n)
	}
}

func TestConcurrentCreateAtLastTenantSlotAdmitsExactlyOne(t *testing.T) {
	requireTenantDB(t)
	tenantID, ws := ownedTenant(t, 1)
	limitTenantAt(t, ws[0], 2)
	slugs := []string{newSlug(), newSlug()}
	for _, s := range slugs {
		trackCreatedWorkspace(t, s)
	}

	var wg sync.WaitGroup
	codes := make([]int, len(slugs))
	start := make(chan struct{})
	for i, slug := range slugs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			rec := httptest.NewRecorder()
			testHandler.CreateWorkspace(rec, createWorkspaceRequest(slug, tenantID))
			codes[i] = rec.Code
		}()
	}
	close(start)
	wg.Wait()

	created, refused := 0, 0
	for _, c := range codes {
		switch c {
		case http.StatusCreated:
			created++
		case http.StatusPaymentRequired:
			refused++
		}
	}
	if created != 1 || refused != 1 {
		t.Fatalf("status codes = %v, want one 201 and one 402", codes)
	}
	if n := linksOf(t, tenantID); n != 2 {
		t.Errorf("workspaces in the tenant = %d, want 2", n)
	}
}

func TestCreateWorkspaceIntoTenantUnlimitedWhenGateOff(t *testing.T) {
	requireTenantDB(t)
	tenantID, _ := ownedTenant(t, 1)
	prior := testHandler.Entitlements
	testHandler.Entitlements = entitlementtest.New() // every gate off
	t.Cleanup(func() { testHandler.Entitlements = prior })
	for i := 0; i < 3; i++ {
		slug := newSlug()
		trackCreatedWorkspace(t, slug)
		testutil.Call(t, testHandler.CreateWorkspace, createWorkspaceRequest(slug, tenantID)).Want(http.StatusCreated)
	}
	if n := linksOf(t, tenantID); n != 4 {
		t.Errorf("workspaces in the tenant = %d, want 4", n)
	}
}

// The tenant lock is taken with the tenant's id as scope even when the gate is
// not enforced, so a creation and a deletion in one tenant always queue (D68).
func TestCreateWorkspaceTakesTenantLockWithTenantScope(t *testing.T) {
	requireTenantDB(t)
	ctx := context.Background()
	tenantID, _ := ownedTenant(t, 1)
	prior := testHandler.Entitlements
	testHandler.Entitlements = entitlementtest.New() // gate off: the lock is still taken
	t.Cleanup(func() { testHandler.Entitlements = prior })

	holder, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Rollback(ctx)
	if _, err := holder.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1::text), hashtext('max_workspaces'))`, tenantID); err != nil {
		t.Fatal(err)
	}

	slug := newSlug()
	trackCreatedWorkspace(t, slug)
	done := make(chan int, 1)
	go func() {
		rec := httptest.NewRecorder()
		testHandler.CreateWorkspace(rec, createWorkspaceRequest(slug, tenantID))
		done <- rec.Code
	}()

	waiting := func() int {
		return dbfx.Count(t, `
			SELECT count(*) FROM pg_locks
			WHERE locktype = 'advisory' AND NOT granted AND objsubid = 2
			  AND classid::bigint = (hashtext($1::text)::bigint & 4294967295)
			  AND objid::bigint = (hashtext('max_workspaces')::bigint & 4294967295)`, tenantID)
	}
	deadline := time.Now().Add(5 * time.Second)
	for waiting() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the creation never waited on the tenant's advisory lock")
		}
		select {
		case code := <-done:
			t.Fatalf("the creation finished with %d while the tenant lock was held", code)
		case <-time.After(50 * time.Millisecond):
		}
	}
	if err := holder.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-done:
		if code != http.StatusCreated {
			t.Fatalf("status = %d, want 201", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the creation did not finish after the lock was released")
	}
}

// A tenant above its limit after a downgrade keeps every workspace; only a new
// one is refused (I10).
func TestDowngradeNeverDeletesWorkspaces(t *testing.T) {
	requireTenantDB(t)
	tenantID, ws := ownedTenant(t, 3)
	limitTenantAt(t, ws[0], 1)
	slug := newSlug()
	trackCreatedWorkspace(t, slug)

	wantPlanLimit(t, testutil.Call(t, testHandler.CreateWorkspace, createWorkspaceRequest(slug, tenantID)), "max_workspaces", 1, 3)
	if n := linksOf(t, tenantID); n != 3 {
		t.Errorf("workspaces in the tenant = %d, want all 3 kept", n)
	}
	for _, id := range ws {
		if n := dbfx.Count(t, `SELECT count(*) FROM workspace WHERE id = $1::uuid`, id); n != 1 {
			t.Errorf("workspace %s was deleted", id)
		}
	}
}

// Creating a workspace gives the staff nothing: an admin's scope comes from the
// tenant link, and no platform_staff row changes (spec R6 section 3.5, AC93).
func TestNewWorkspaceInTenantIsInAdminScopeWithoutStaffGrant(t *testing.T) {
	requireTenantDB(t)
	tenantID, _ := ownedTenant(t, 1)
	adminID := dbfx.User(t, "Scope Admin", testutil.UniqueEmail("scope-admin"))
	dbfx.Exec(t, `INSERT INTO platform_staff (user_id, role, tenant_id) VALUES ($1::uuid, 'admin', $2::uuid)`, adminID, tenantID)
	dbfx.Cleanup(t, `DELETE FROM platform_staff WHERE user_id = $1::uuid`, adminID)
	before := dbfx.Count(t, `SELECT count(*) FROM platform_staff`)

	slug := newSlug()
	trackCreatedWorkspace(t, slug)
	var created WorkspaceResponse
	testutil.Call(t, testHandler.CreateWorkspace, createWorkspaceRequest(slug, tenantID)).Want(http.StatusCreated).JSON(&created)

	if n := dbfx.Count(t, `SELECT count(*) FROM tenant_workspace WHERE workspace_id = $1::uuid AND tenant_id = $2::uuid`, created.ID, tenantID); n != 1 {
		t.Errorf("the new workspace is not linked to the admin's tenant")
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM platform_staff`); n != before {
		t.Errorf("platform_staff rows went from %d to %d", before, n)
	}
}

func TestCreateWorkspaceDoesNotGrantStaffAnyTenant(t *testing.T) {
	requireTenantDB(t)
	staffID := dbfx.User(t, "Staff Support", testutil.UniqueEmail("staff-support"))
	dbfx.Exec(t, `INSERT INTO platform_staff (user_id, role) VALUES ($1::uuid, 'support')`, staffID)
	dbfx.Cleanup(t, `DELETE FROM platform_staff WHERE user_id = $1::uuid`, staffID)

	slug := newSlug()
	trackCreatedWorkspace(t, slug)
	testutil.Call(t, testHandler.CreateWorkspace, createWorkspaceRequest(slug, "")).Want(http.StatusCreated)
	if n := dbfx.Count(t, `SELECT count(*) FROM platform_staff WHERE tenant_id IS NOT NULL AND user_id = $1::uuid`, staffID); n != 0 {
		t.Errorf("a platform_staff row got a tenant_id from workspace creation")
	}
}

func TestGetWorkspaceReturnsTenantID(t *testing.T) {
	requireTenantDB(t)
	tenantID, ws := ownedTenant(t, 1)
	req := withURLParam(newRequest("GET", "/api/workspaces/"+ws[0], nil), "id", ws[0])
	var got WorkspaceResponse
	testutil.Call(t, testHandler.GetWorkspace, req).Want(http.StatusOK).JSON(&got)
	if got.TenantID != tenantID {
		t.Errorf("tenant_id = %q, want %q", got.TenantID, tenantID)
	}
	// A workspace with no link has no tenant_id.
	bare := dbfx.Workspace(t, "no tenant", "tnt-bare-"+uuid.NewString()[:8])
	dbfx.Member(t, bare, testUserID, "owner")
	req = withURLParam(newRequest("GET", "/api/workspaces/"+bare, nil), "id", bare)
	got = WorkspaceResponse{}
	testutil.Call(t, testHandler.GetWorkspace, req).Want(http.StatusOK).JSON(&got)
	if got.TenantID != "" {
		t.Errorf("tenant_id = %q, want none", got.TenantID)
	}
}
