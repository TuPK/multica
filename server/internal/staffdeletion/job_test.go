package staffdeletion

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/multica-ai/multica/server/internal/analytics"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/handler"
	"github.com/multica-ai/multica/server/internal/realtime"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

var (
	testPool    *pgxpool.Pool
	testHandler *handler.Handler
	fx          *testutil.Fixture
)

// requireTestDatabaseURL refuses any database whose name does not end in _test.
func requireTestDatabaseURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("DATABASE_URL is empty; set it to a database whose name ends in _test")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("DATABASE_URL is not a valid URL: %w", err)
	}
	name := strings.TrimPrefix(u.Path, "/")
	if name == "multica" || !strings.HasSuffix(name, "_test") {
		return fmt.Errorf("database %q is not a test database; use one whose name ends in _test", name)
	}
	return nil
}

func TestMain(m *testing.M) {
	ctx := context.Background()
	dbURL := os.Getenv("DATABASE_URL")
	if err := requireTestDatabaseURL(dbURL); err != nil {
		fmt.Fprintln(os.Stderr, "staffdeletion tests:", err)
		os.Exit(1)
	}
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "staffdeletion tests: connect:", err)
		os.Exit(1)
	}
	if err := pool.Ping(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "staffdeletion tests: ping:", err)
		os.Exit(1)
	}
	hub := realtime.NewHub()
	go hub.Run()
	testPool = pool
	testHandler = handler.New(db.New(pool), pool, hub, events.New(), service.NewEmailService(), nil, nil, analytics.NoopClient{}, handler.Config{})
	fx = testutil.New(pool, "", "")
	code := m.Run()
	pool.Close()
	os.Exit(code)
}

// newRunner builds the job over the real handler, with an optional failure hook.
func newRunner(inject func(stage string) error) *Runner {
	return New(Deps{
		Pool:    testPool,
		Queries: db.New(testPool),
		Eraser:  testHandler,
		Inject:  inject,
	})
}

// stageRequest writes a pending request as the platform does in phase 1, with
// the phase-1 audit row that goes with it. The rows go away with the test.
func stageRequest(t *testing.T, kind, targetID, tenantID, state string) string {
	t.Helper()
	id := uuid.NewString()
	var tenant any
	if tenantID != "" {
		tenant = tenantID
	}
	if _, err := testPool.Exec(context.Background(), `INSERT INTO platform_deletion_request
		(id, kind, target_id, target_label, reason, state, tenant_id, requested_by, requested_by_label)
		VALUES ($1::uuid, $2, $3::uuid, 'staff target', 'staff job test', $4, $5, $6::uuid, 'staff tester')`,
		id, kind, targetID, state, tenant, uuid.NewString()); err != nil {
		t.Fatalf("stage request: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM platform_deletion_request WHERE id = $1::uuid`, id)
	})
	return id
}

func stageAuditRow(t *testing.T, action, workspaceID, targetID string) {
	t.Helper()
	if _, err := testPool.Exec(context.Background(), `INSERT INTO audit_event
		(workspace_id, actor_type, actor_label, action, target_type, target_id, detail)
		VALUES (NULLIF($1, '')::uuid, 'staff', 'staff tester', $2, 'workspace', $3, '{"reason":"staff_immediate_deletion"}')`,
		workspaceID, action, targetID); err != nil {
		t.Fatalf("stage audit row: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM audit_event WHERE target_id = $1 AND action = $2`, targetID, action)
	})
}

func count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

// requestState reads the state, attempts and error code of a request.
func requestState(t *testing.T, id string) (state string, attempts int, errorCode string) {
	t.Helper()
	if err := testPool.QueryRow(context.Background(),
		`SELECT state, attempts, error_code FROM platform_deletion_request WHERE id = $1::uuid`, id,
	).Scan(&state, &attempts, &errorCode); err != nil {
		t.Fatalf("read request: %v", err)
	}
	return state, attempts, errorCode
}

// workspaceWithOwner makes a workspace with one owner, removed with the test.
func workspaceWithOwner(t *testing.T, name string) string {
	t.Helper()
	owner := fx.User(t, name+" owner", testutil.UniqueEmail("staffdel"))
	ws := fx.Workspace(t, name, "sd-"+uuid.NewString()[:8])
	fx.Member(t, ws, owner, "owner")
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM audit_event WHERE workspace_id = $1::uuid OR target_id = $1`, ws)
	})
	return ws
}

func workspaceGone(t *testing.T, ws string) bool {
	t.Helper()
	return count(t, `SELECT count(*) FROM workspace WHERE id = $1::uuid`, ws) == 0
}

func TestStaffWorkspaceRequestRunsToCompletion(t *testing.T) {
	ws := workspaceWithOwner(t, "staff job single")
	reqID := stageRequest(t, "workspace", ws, "", "pending")
	stageAuditRow(t, "workspace.immediate_deletion_requested", ws, ws)

	if _, err := newRunner(nil).RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if !workspaceGone(t, ws) {
		t.Error("workspace still exists after the staff request ran")
	}
	if state, _, _ := requestState(t, reqID); state != "completed" {
		t.Errorf("request state = %q, want completed", state)
	}
	if n := count(t, `SELECT count(*) FROM platform_deletion_request WHERE id = $1::uuid AND completed_at IS NOT NULL`, reqID); n != 1 {
		t.Errorf("completed_at not set on the completed request")
	}
}

// Two runs in parallel, after the request was already claimed by one of them,
// delete the workspace once.
func TestStaffDeletionTwiceAndParallelDeletesOnce(t *testing.T) {
	ws := workspaceWithOwner(t, "staff job twice")
	stageRequest(t, "workspace", ws, "", "pending")
	stageAuditRow(t, "workspace.immediate_deletion_requested", ws, ws)

	runner := newRunner(nil)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := runner.RunOnce(context.Background()); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("RunOnce: %v", err)
	}
	if _, err := runner.RunOnce(context.Background()); err != nil {
		t.Fatalf("third RunOnce: %v", err)
	}
	if n := count(t, `SELECT count(*) FROM audit_event WHERE action = 'workspace.deleted' AND target_id = $1`, ws); n != 1 {
		t.Errorf("workspace.deleted rows = %d, want exactly 1", n)
	}
}

func TestStaffDeletionRetryAfterFailureCompletes(t *testing.T) {
	ws := workspaceWithOwner(t, "staff job retry")
	reqID := stageRequest(t, "workspace", ws, "", "pending")
	stageAuditRow(t, "workspace.immediate_deletion_requested", ws, ws)

	failOnce := true
	runner := newRunner(func(stage string) error {
		if stage == "audit" && failOnce {
			failOnce = false
			return errors.New("transient")
		}
		return nil
	})
	if _, err := runner.RunOnce(context.Background()); err != nil {
		t.Fatalf("first RunOnce: %v", err)
	}
	state, attempts, code := requestState(t, reqID)
	if state != "pending" || attempts != 1 || code != "delete_failed" {
		t.Fatalf("after one failure: state=%q attempts=%d error_code=%q, want pending/1/delete_failed", state, attempts, code)
	}
	if workspaceGone(t, ws) {
		t.Error("workspace deleted by a failed run")
	}

	if _, err := runner.RunOnce(context.Background()); err != nil {
		t.Fatalf("second RunOnce: %v", err)
	}
	if state, _, _ := requestState(t, reqID); state != "completed" {
		t.Errorf("request state after retry = %q, want completed", state)
	}
	if !workspaceGone(t, ws) {
		t.Error("workspace still exists after the retry completed")
	}
}

func TestStaffDeletionFailureMarksFailedAfterThreeAttemptsAndKeepsPhaseOneAudit(t *testing.T) {
	ws := workspaceWithOwner(t, "staff job fail")
	reqID := stageRequest(t, "workspace", ws, "", "pending")
	stageAuditRow(t, "workspace.immediate_deletion_requested", ws, ws)

	runner := newRunner(func(stage string) error {
		if stage == "audit" {
			return errors.New("injected")
		}
		return nil
	})
	for run, want := range []string{"pending", "pending", "failed"} {
		if _, err := runner.RunOnce(context.Background()); err != nil {
			t.Fatalf("run %d: %v", run+1, err)
		}
		state, attempts, code := requestState(t, reqID)
		if state != want || attempts != run+1 || code != "delete_failed" {
			t.Fatalf("after run %d: state=%q attempts=%d error_code=%q, want %s/%d/delete_failed",
				run+1, state, attempts, code, want, run+1)
		}
	}
	if workspaceGone(t, ws) {
		t.Error("the failed deletion removed workspace data; the teardown must be one transaction")
	}
	if n := count(t, `SELECT count(*) FROM audit_event WHERE action = 'workspace.immediate_deletion_requested' AND target_id = $1`, ws); n != 1 {
		t.Errorf("phase-1 audit rows = %d after failures, want 1", n)
	}
}

func TestStaffDeletionTenantDeletesEveryWorkspaceThenTenant(t *testing.T) {
	a := workspaceWithOwner(t, "staff tenant a")
	b := workspaceWithOwner(t, "staff tenant b")
	tenantID := uuid.NewString()
	if _, err := testPool.Exec(context.Background(), `INSERT INTO tenant (id, name) VALUES ($1::uuid, 'staff job tenant')`, tenantID); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM tenant WHERE id = $1::uuid`, tenantID) })
	for _, ws := range []string{a, b} {
		if _, err := testPool.Exec(context.Background(), `INSERT INTO tenant_workspace (workspace_id, tenant_id) VALUES ($1::uuid, $2::uuid)`, ws, tenantID); err != nil {
			t.Fatalf("link workspace: %v", err)
		}
	}
	reqID := stageRequest(t, "tenant", tenantID, tenantID, "pending")
	stageAuditRow(t, "tenant.immediate_deletion_requested", "", tenantID)

	if _, err := newRunner(nil).RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if !workspaceGone(t, a) || !workspaceGone(t, b) {
		t.Error("a workspace of the tenant survived the tenant deletion")
	}
	if n := count(t, `SELECT count(*) FROM tenant WHERE id = $1::uuid`, tenantID); n != 0 {
		t.Errorf("tenant rows = %d after the deletion, want 0", n)
	}
	if state, _, _ := requestState(t, reqID); state != "completed" {
		t.Errorf("tenant request state = %q, want completed", state)
	}
	if n := count(t, `SELECT count(*) FROM audit_event WHERE action = 'tenant.deleted' AND tenant_id = $1::uuid`, tenantID); n != 1 {
		t.Errorf("tenant.deleted rows = %d, want 1 (with the tenant id kept)", n)
	}
	if n := count(t, `SELECT count(*) FROM audit_event WHERE action = 'workspace.deleted' AND tenant_id = $1::uuid AND detail->>'request_id' = $2`, tenantID, reqID); n != 2 {
		t.Errorf("workspace.deleted rows of the tenant request = %d, want 2", n)
	}
}

// A workspace with a live or failed staff request is left to the staff job, so
// the hourly workspace_deletion job never deletes it first.
func TestWorkspaceDeletionJobSkipsWorkspacesWithLiveOrFailedStaffRequest(t *testing.T) {
	ctx := context.Background()
	live := workspaceWithOwner(t, "skip live")
	failed := workspaceWithOwner(t, "skip failed")
	plain := workspaceWithOwner(t, "skip plain")
	stageRequest(t, "workspace", live, "", "pending")
	stageRequest(t, "workspace", failed, "", "failed")
	for _, ws := range []string{live, failed, plain} {
		if _, err := testPool.Exec(ctx, `INSERT INTO platform_workspace (workspace_id, status, reason, deletion_scheduled_at)
			VALUES ($1::uuid, 'pending_deletion', 'staff_immediate_deletion', now() - interval '1 minute')`, ws); err != nil {
			t.Fatalf("stage platform workspace: %v", err)
		}
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM platform_workspace WHERE workspace_id = $1::uuid`, ws) })
	}

	due, err := testHandler.DueWorkspaceDeletions(ctx, time.Now(), 1000000)
	if err != nil {
		t.Fatalf("DueWorkspaceDeletions: %v", err)
	}
	seen := map[string]bool{}
	for _, id := range due {
		seen[id] = true
	}
	if !seen[plain] {
		t.Fatal("control: a due workspace without a staff request is not listed")
	}
	if seen[live] || seen[failed] {
		t.Errorf("workspace_deletion lists a workspace with a live or failed staff request")
	}
}

// Account and runtime requests are not this job's; it leaves them untouched.
func TestStaffDeletionLeavesOtherKindsForTheirJobs(t *testing.T) {
	ws := workspaceWithOwner(t, "staff job other kinds")
	reqID := stageRequest(t, "bogus", uuid.NewString(), "", "pending")
	if _, err := newRunner(nil).RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if state, attempts, _ := requestState(t, reqID); state != "pending" || attempts != 0 {
		t.Errorf("bogus-kind request = %q attempts %d, want untouched pending/0", state, attempts)
	}
	if workspaceGone(t, ws) {
		t.Error("an unrelated workspace was deleted")
	}
}
