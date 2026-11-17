package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/multica-ai/multica/platform-management/workspacestate"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	"github.com/multica-ai/multica/server/internal/workspaceguard"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Database-backed: they run only against the dedicated *_test database named by
// PLATFORM_TEST_DATABASE_URL (see requirePlatformTestDatabase). The pure rules
// (the schedule, the due check, the configuration) are tested in
// internal/workspacedeletion, and the job's batch loop in internal/scheduler.
// These tests were written without a database available and have not been run.

// stateCache is the workspace-state guard's cache, with the keys it saw deleted.
type stateCache struct {
	mu      sync.Mutex
	deleted []string
}

func (*stateCache) Get(context.Context, string) ([]byte, bool, error) { return nil, false, nil }
func (*stateCache) Set(context.Context, string, []byte, time.Duration) error {
	return nil
}
func (c *stateCache) Del(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deleted = append(c.deleted, key)
	return nil
}
func (c *stateCache) deletedKeys() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.deleted...)
}

// withStateGuard runs h with the workspace-state guard in the request context,
// as the router's WithWorkspaceState middleware does.
func withStateGuard(cache *stateCache, h http.HandlerFunc) http.HandlerFunc {
	guard := workspacestate.New(workspaceguard.NewStore(testHandler.Queries), cache)
	return middleware.WithWorkspaceState(guard)(h).ServeHTTP
}

// activeStore reports every workspace as active. The cancel test uses it to
// watch cache invalidation without the guard's own refusal of a pending
// workspace getting in the way.
type activeStore struct{}

func (activeStore) WorkspaceState(context.Context, string) (workspacestate.Record, bool, error) {
	return workspacestate.Record{}, false, nil
}

func withActiveStateGuard(cache *stateCache, h http.HandlerFunc) http.HandlerFunc {
	return middleware.WithWorkspaceState(workspacestate.New(activeStore{}, cache))(h).ServeHTTP
}

// ownedWorkspace makes a workspace owned by the shared handler-test user.
func ownedWorkspace(t *testing.T, name string) string {
	t.Helper()
	wsID := dbfx.Workspace(t, name, "wsdel-"+uuid.NewString()[:8])
	dbfx.Member(t, wsID, testUserID, "owner")
	dbfx.Cleanup(t, `DELETE FROM platform_workspace WHERE workspace_id = $1::uuid`, wsID)
	return wsID
}

func deleteRequest(wsID string) *http.Request {
	return withURLParam(newRequest(http.MethodDelete, "/api/workspaces/"+wsID, nil), "id", wsID)
}

func cancelRequest(wsID string) *http.Request {
	return withURLParam(newRequest(http.MethodDelete, "/api/workspaces/"+wsID+"/deletion", nil), "id", wsID)
}

type platformWorkspaceRow struct {
	status    string
	reason    string
	scheduled *time.Time
}

// platformRow returns the workspace's platform_workspace row, or nil.
func platformRow(t *testing.T, wsID string) *platformWorkspaceRow {
	t.Helper()
	var row platformWorkspaceRow
	err := testPool.QueryRow(context.Background(),
		`SELECT status, reason, deletion_scheduled_at FROM platform_workspace WHERE workspace_id = $1::uuid`, wsID).
		Scan(&row.status, &row.reason, &row.scheduled)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return &row
}

// seedPending marks a workspace pending deletion at the given time.
func seedPending(t *testing.T, wsID string, at time.Time) {
	t.Helper()
	dbfx.Exec(t, `
		INSERT INTO platform_workspace (workspace_id, status, reason, deletion_scheduled_at)
		VALUES ($1::uuid, 'pending_deletion', 'owner_requested', $2)
		ON CONFLICT (workspace_id) DO UPDATE
		SET status = EXCLUDED.status, deletion_scheduled_at = EXCLUDED.deletion_scheduled_at`, wsID, at)
}

// setDeletionDays sets the grace period of the shared test handler for one test.
func setDeletionDays(t *testing.T, days int) {
	t.Helper()
	prev := testHandler.WorkspaceDeletionDays
	testHandler.WorkspaceDeletionDays = days
	t.Cleanup(func() { testHandler.WorkspaceDeletionDays = prev })
}

func workspaceExists(t *testing.T, wsID string) bool {
	t.Helper()
	return dbfx.Count(t, `SELECT count(*) FROM workspace WHERE id = $1::uuid`, wsID) == 1
}

// sso_domain and sso_connection carry no foreign key, so teardown removes them.
// A verified domain left behind would stay claimed by a workspace that is gone,
// and its unique index would keep every other workspace from claiming it.
func TestDeleteWorkspaceReleasesSSODomainAndConnection(t *testing.T) {
	requirePlatformTestDatabase(t)
	setDeletionDays(t, 0)
	wsID := ownedWorkspace(t, "Delete with sso")
	otherID := ownedWorkspace(t, "Claims the freed domain")
	domain := "sso-" + uuid.NewString()[:8] + ".example.test"
	dbfx.Cleanup(t, `DELETE FROM sso_domain WHERE domain = $1`, domain)
	dbfx.Cleanup(t, `DELETE FROM sso_connection WHERE workspace_id IN ($1::uuid, $2::uuid)`, wsID, otherID)
	// Assumption: a verified claim and a connection are the state a workspace
	// has once its single sign-on is set up.
	dbfx.Exec(t, `INSERT INTO sso_domain (workspace_id, domain, token, verified_at) VALUES ($1::uuid, $2, 'token', now())`, wsID, domain)
	dbfx.Exec(t, `INSERT INTO sso_connection (workspace_id, issuer, client_id, client_secret_enc) VALUES ($1::uuid, 'https://idp.example.test', 'client', '\x00')`, wsID)

	testutil.Call(t, testHandler.DeleteWorkspace, deleteRequest(wsID)).Want(http.StatusNoContent)

	if workspaceExists(t, wsID) {
		t.Fatal("the workspace still exists")
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM sso_domain WHERE workspace_id = $1::uuid`, wsID); n != 0 {
		t.Errorf("%d sso_domain rows remain for the deleted workspace", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM sso_connection WHERE workspace_id = $1::uuid`, wsID); n != 0 {
		t.Errorf("%d sso_connection rows remain for the deleted workspace", n)
	}

	// The claim code's own insert, then the verification that the unique index guards.
	if _, err := testHandler.Queries.AddSSODomain(context.Background(), db.AddSSODomainParams{
		WorkspaceID: util.MustParseUUID(otherID), Domain: domain, Token: "token",
	}); err != nil {
		t.Fatalf("another workspace cannot claim the domain: %v", err)
	}
	if _, err := testHandler.Queries.MarkSSODomainVerified(context.Background(), db.MarkSSODomainVerifiedParams{
		WorkspaceID: util.MustParseUUID(otherID), Domain: domain,
	}); err != nil {
		t.Fatalf("another workspace cannot verify the domain: %v", err)
	}
}

func TestDeleteWorkspaceWithZeroDaysDeletesAtOnce(t *testing.T) {
	requirePlatformTestDatabase(t)
	setDeletionDays(t, 0)
	wsID := ownedWorkspace(t, "Delete now")

	testutil.Call(t, testHandler.DeleteWorkspace, deleteRequest(wsID)).Want(http.StatusNoContent)

	if workspaceExists(t, wsID) {
		t.Fatal("the workspace still exists")
	}
	if row := platformRow(t, wsID); row != nil && row.status == "pending_deletion" {
		t.Fatalf("a pending_deletion row was written: %+v", row)
	}
}

func TestDeleteWorkspaceWithDaysSchedulesInsteadOfDeleting(t *testing.T) {
	requirePlatformTestDatabase(t)
	setDeletionDays(t, 14)
	wsID := ownedWorkspace(t, "Delete later")
	cache := &stateCache{}

	before := time.Now()
	resp := testutil.Call(t, withStateGuard(cache, testHandler.DeleteWorkspace), deleteRequest(wsID)).Want(http.StatusAccepted)
	after := time.Now()

	var out WorkspaceDeletionScheduledResponse
	resp.JSON(&out)
	if out.Status != "pending_deletion" {
		t.Fatalf("status = %q", out.Status)
	}
	at, err := time.Parse(time.RFC3339, out.DeletionScheduledAt)
	if err != nil {
		t.Fatalf("deletion_scheduled_at %q is not RFC 3339: %v", out.DeletionScheduledAt, err)
	}
	if at.Before(before.Add(14*24*time.Hour-time.Second)) || at.After(after.Add(14*24*time.Hour+time.Second)) {
		t.Fatalf("deletion_scheduled_at = %v, want 14 days after the request", at)
	}

	if !workspaceExists(t, wsID) {
		t.Fatal("the workspace was deleted")
	}
	row := platformRow(t, wsID)
	if row == nil || row.status != "pending_deletion" || row.reason != "owner_requested" {
		t.Fatalf("platform row = %+v", row)
	}
	if row.scheduled == nil || row.scheduled.Sub(at).Abs() > time.Second {
		t.Fatalf("stored schedule = %v, response said %v", row.scheduled, at)
	}
	if keys := cache.deletedKeys(); len(keys) != 1 || keys[0] != workspacestate.Key(wsID) {
		t.Fatalf("invalidated cache keys = %v, want [%s]", keys, workspacestate.Key(wsID))
	}
}

func TestDeleteWorkspaceByANonOwnerSchedulesNothing(t *testing.T) {
	requirePlatformTestDatabase(t)
	setDeletionDays(t, 14)
	wsID := dbfx.Workspace(t, "Not mine", "wsdel-"+uuid.NewString()[:8])
	dbfx.Member(t, wsID, testUserID, "admin")
	dbfx.Cleanup(t, `DELETE FROM platform_workspace WHERE workspace_id = $1::uuid`, wsID)

	testutil.Call(t, testHandler.DeleteWorkspace, deleteRequest(wsID)).Want(http.StatusForbidden)
	if row := platformRow(t, wsID); row != nil {
		t.Fatalf("a row was written: %+v", row)
	}
}

func TestCancelWorkspaceDeletionRestoresTheWorkspace(t *testing.T) {
	requirePlatformTestDatabase(t)
	wsID := ownedWorkspace(t, "Cancel me")
	seedPending(t, wsID, time.Now().Add(24*time.Hour))
	cache := &stateCache{}

	testutil.Call(t, withActiveStateGuard(cache, testHandler.CancelWorkspaceDeletion), cancelRequest(wsID)).Want(http.StatusNoContent)

	row := platformRow(t, wsID)
	if row == nil || row.status != "active" || row.scheduled != nil {
		t.Fatalf("platform row = %+v, want active with no schedule", row)
	}
	if keys := cache.deletedKeys(); len(keys) != 1 || keys[0] != workspacestate.Key(wsID) {
		t.Fatalf("invalidated cache keys = %v, want [%s]", keys, workspacestate.Key(wsID))
	}

	resp := testutil.Call(t, testHandler.CancelWorkspaceDeletion, cancelRequest(wsID)).Want(http.StatusNotFound)
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != "no_pending_deletion" {
		t.Fatalf("second cancel body = %v, want code no_pending_deletion", body)
	}
}

func TestCancelWorkspaceDeletionWithNoScheduleIs404(t *testing.T) {
	requirePlatformTestDatabase(t)
	wsID := ownedWorkspace(t, "Never scheduled")
	testutil.Call(t, testHandler.CancelWorkspaceDeletion, cancelRequest(wsID)).Want(http.StatusNotFound)
}

func TestCancelWorkspaceDeletionIsOwnerOnly(t *testing.T) {
	requirePlatformTestDatabase(t)
	wsID := dbfx.Workspace(t, "Admin cannot cancel", "wsdel-"+uuid.NewString()[:8])
	dbfx.Member(t, wsID, testUserID, "admin")
	dbfx.Cleanup(t, `DELETE FROM platform_workspace WHERE workspace_id = $1::uuid`, wsID)
	seedPending(t, wsID, time.Now().Add(24*time.Hour))

	testutil.Call(t, testHandler.CancelWorkspaceDeletion, cancelRequest(wsID)).Want(http.StatusForbidden)
	if row := platformRow(t, wsID); row == nil || row.status != "pending_deletion" {
		t.Fatalf("platform row = %+v, want it still pending", row)
	}
}

func TestDeleteDueWorkspaceDeletesADueWorkspace(t *testing.T) {
	requirePlatformTestDatabase(t)
	ctx := context.Background()
	wsID := ownedWorkspace(t, "Due")
	now := time.Now()
	seedPending(t, wsID, now.Add(-time.Minute))

	ids, err := testHandler.DueWorkspaceDeletions(ctx, now, 1000)
	if err != nil || !containsString(ids, wsID) {
		t.Fatalf("DueWorkspaceDeletions = %v, %v; want it to list %s", ids, err, wsID)
	}
	done, err := testHandler.DeleteDueWorkspace(ctx, wsID, now)
	if err != nil || !done {
		t.Fatalf("DeleteDueWorkspace = %v, %v", done, err)
	}
	if workspaceExists(t, wsID) {
		t.Fatal("the workspace still exists")
	}
	if row := platformRow(t, wsID); row != nil {
		t.Fatalf("the platform row is left behind: %+v", row)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM audit_event WHERE action = 'workspace.deleted' AND target_id = $1`, wsID); n != 1 {
		t.Fatalf("%d workspace.deleted audit rows, want 1", n)
	}
}

func TestDeleteDueWorkspaceLeavesAWorkspaceThatIsNotDue(t *testing.T) {
	requirePlatformTestDatabase(t)
	ctx := context.Background()
	wsID := ownedWorkspace(t, "Not due")
	now := time.Now()
	seedPending(t, wsID, now.Add(time.Hour))

	ids, err := testHandler.DueWorkspaceDeletions(ctx, now, 1000)
	if err != nil || containsString(ids, wsID) {
		t.Fatalf("DueWorkspaceDeletions = %v, %v; it must not list %s", ids, err, wsID)
	}
	done, err := testHandler.DeleteDueWorkspace(ctx, wsID, now)
	if err != nil || done {
		t.Fatalf("DeleteDueWorkspace = %v, %v; want false, nil", done, err)
	}
	if !workspaceExists(t, wsID) {
		t.Fatal("a workspace that is not due was deleted")
	}
}

func TestDeleteDueWorkspaceNeverDeletesACancelledWorkspace(t *testing.T) {
	requirePlatformTestDatabase(t)
	ctx := context.Background()
	wsID := ownedWorkspace(t, "Cancelled before the run")
	now := time.Now()
	seedPending(t, wsID, now.Add(-time.Minute))

	// The job listed the workspace, then the owner cancelled before it got to it.
	ids, err := testHandler.DueWorkspaceDeletions(ctx, now, 1000)
	if err != nil || !containsString(ids, wsID) {
		t.Fatalf("DueWorkspaceDeletions = %v, %v", ids, err)
	}
	testutil.Call(t, testHandler.CancelWorkspaceDeletion, cancelRequest(wsID)).Want(http.StatusNoContent)

	done, err := testHandler.DeleteDueWorkspace(ctx, wsID, now)
	if err != nil || done {
		t.Fatalf("DeleteDueWorkspace = %v, %v; want false, nil", done, err)
	}
	if !workspaceExists(t, wsID) {
		t.Fatal("a cancelled workspace was deleted")
	}
}

func TestDeleteDueWorkspaceRunInParallelDeletesOnce(t *testing.T) {
	requirePlatformTestDatabase(t)
	ctx := context.Background()
	wsID := ownedWorkspace(t, "Two runs")
	now := time.Now()
	seedPending(t, wsID, now.Add(-time.Minute))

	const runs = 2
	results := make([]bool, runs)
	errs := make([]error, runs)
	var wg sync.WaitGroup
	for i := 0; i < runs; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = testHandler.DeleteDueWorkspace(ctx, wsID, now)
		}(i)
	}
	wg.Wait()

	deleted := 0
	for i := range results {
		if errs[i] != nil {
			t.Fatalf("run %d: %v", i, errs[i])
		}
		if results[i] {
			deleted++
		}
	}
	if deleted != 1 {
		t.Fatalf("%d runs reported a deletion, want exactly 1", deleted)
	}
	if workspaceExists(t, wsID) {
		t.Fatal("the workspace still exists")
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM audit_event WHERE action = 'workspace.deleted' AND target_id = $1`, wsID); n != 1 {
		t.Fatalf("%d workspace.deleted audit rows, want 1", n)
	}
}

// A pending workspace refuses writes and still serves reads, through the real
// state store and the real decision table (the guard itself is unchanged).
func TestPendingWorkspaceRefusesWritesAndServesReads(t *testing.T) {
	requirePlatformTestDatabase(t)
	wsID := ownedWorkspace(t, "Pending state")
	seedPending(t, wsID, time.Now().Add(24*time.Hour))
	member, err := testHandler.getWorkspaceMember(context.Background(), testUserID, wsID)
	if err != nil {
		t.Fatal(err)
	}
	cache := &stateCache{}
	guard := workspacestate.New(workspaceguard.NewStore(testHandler.Queries), cache)

	admit := func(method, path, pattern string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		rctx := chi.NewRouteContext()
		rctx.RoutePatterns = []string{pattern}
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		w := httptest.NewRecorder()
		middleware.WithWorkspaceState(guard)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if testHandler.admitByWorkspaceState(w, r, member) {
				w.WriteHeader(http.StatusOK)
			}
		})).ServeHTTP(w, req)
		return w
	}

	if w := admit(http.MethodGet, "/api/workspaces/"+wsID, "/api/workspaces/{id}"); w.Code != http.StatusOK {
		t.Fatalf("read = %d, want 200", w.Code)
	}
	w := admit(http.MethodPost, "/api/issues", "/api/issues")
	if w.Code != http.StatusForbidden {
		t.Fatalf("write = %d, want 403", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != workspaceguard.CodePendingDeletion {
		t.Fatalf("write body = %v, want code %s", body, workspaceguard.CodePendingDeletion)
	}
}

// The account request is the platform's record of who asked for the account, so
// it outlives the workspace. It has no foreign key, so teardown clears the
// workspace_id it points at.
func TestDeleteWorkspaceClearsAccountRequestWorkspaceID(t *testing.T) {
	requirePlatformTestDatabase(t)
	setDeletionDays(t, 0)
	wsID := ownedWorkspace(t, "Delete with account request")
	email := "client-" + uuid.NewString()[:8] + "@example.test"
	reqID := dbfx.Insert(t, "platform_account_request", testutil.Cols{
		"id": uuid.NewString(), "kind": "individual", "email": email, "display_name": "Client Person",
		"workspace_name": "Client workspace", "plan_key": "free", "state": "completed",
		"user_id": testUserID, "workspace_id": wsID, "created_by": testUserID,
	})
	var createdAt time.Time
	if err := testPool.QueryRow(context.Background(),
		`SELECT created_at FROM platform_account_request WHERE id = $1::uuid`, reqID).Scan(&createdAt); err != nil {
		t.Fatal(err)
	}

	testutil.Call(t, testHandler.DeleteWorkspace, deleteRequest(wsID)).Want(http.StatusNoContent)

	if workspaceExists(t, wsID) {
		t.Fatal("the workspace still exists")
	}
	var (
		state, gotEmail string
		userID, gotWsID *string
		gotCreatedAt    time.Time
	)
	if err := testPool.QueryRow(context.Background(),
		`SELECT state, email, user_id::text, workspace_id::text, created_at FROM platform_account_request WHERE id = $1::uuid`, reqID).
		Scan(&state, &gotEmail, &userID, &gotWsID, &gotCreatedAt); err != nil {
		t.Fatalf("the account request row is gone: %v", err)
	}
	if gotWsID != nil {
		t.Errorf("workspace_id = %s; want NULL", *gotWsID)
	}
	if state != "completed" || gotEmail != email || userID == nil || *userID != testUserID || !gotCreatedAt.Equal(createdAt) {
		t.Errorf("other columns changed: state=%q email=%q user_id=%v created_at=%v", state, gotEmail, userID, gotCreatedAt)
	}
}
