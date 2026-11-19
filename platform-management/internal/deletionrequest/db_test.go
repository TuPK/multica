package deletionrequest

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/multica-ai/multica/platform-management/internal/testdb"
)

// Database tests: skipped unless PLATFORM_TEST_DATABASE_URL names a *_test
// database and DATABASE_URL equals it. Each test makes its own rows and removes
// them, along with the rows the service wrote.

type fixture struct {
	t      *testing.T
	pool   *pgxpool.Pool
	svc    *Service
	cache  *recordingCache
	users  []string
	wss    []string
	tenant string
	staff  Actor
}

type recordingCache struct {
	mu  sync.Mutex
	ids []string
}

func (r *recordingCache) Invalidate(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ids = append(r.ids, id)
	return nil
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool := testdb.Open(t)
	f := &fixture{t: t, pool: pool, cache: &recordingCache{}}
	f.svc = New(Deps{
		Runner: NewPgxRunner(pool), Reader: NewPgxReader(pool), Cache: f.cache,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	staffID, staffEmail := f.user("staff")
	f.staff = Actor{ID: staffID, Label: staffEmail}
	t.Cleanup(f.cleanup)
	return f
}

func (f *fixture) cleanup() {
	ctx := context.Background()
	for _, ws := range f.wss {
		f.pool.Exec(ctx, `DELETE FROM platform_deletion_request WHERE target_id = $1 OR parent_id = $1`, ws)
		f.pool.Exec(ctx, `DELETE FROM audit_event WHERE workspace_id = $1`, ws)
		f.pool.Exec(ctx, `DELETE FROM platform_workspace WHERE workspace_id = $1`, ws)
		f.pool.Exec(ctx, `DELETE FROM tenant_workspace WHERE workspace_id = $1`, ws)
		f.pool.Exec(ctx, `DELETE FROM workspace WHERE id = $1`, ws)
	}
	if f.tenant != "" {
		f.pool.Exec(ctx, `DELETE FROM platform_deletion_request WHERE target_id = $1`, f.tenant)
		f.pool.Exec(ctx, `DELETE FROM audit_event WHERE tenant_id = $1`, f.tenant)
		f.pool.Exec(ctx, `DELETE FROM tenant WHERE id = $1`, f.tenant)
	}
	for _, u := range f.users {
		f.pool.Exec(ctx, `DELETE FROM platform_staff WHERE user_id = $1`, u)
		f.pool.Exec(ctx, `DELETE FROM "user" WHERE id = $1`, u)
	}
}

func (f *fixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(context.Background(), sql, args...); err != nil {
		f.t.Fatalf("%s: %v", sql, err)
	}
}

func (f *fixture) count(sql string, args ...any) int {
	f.t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		f.t.Fatalf("%s: %v", sql, err)
	}
	return n
}

func (f *fixture) scalar(sql string, args ...any) string {
	f.t.Helper()
	var s string
	if err := f.pool.QueryRow(context.Background(), sql, args...).Scan(&s); err != nil {
		f.t.Fatalf("%s: %v", sql, err)
	}
	return s
}

func (f *fixture) user(prefix string) (id, email string) {
	f.t.Helper()
	suffix := f.scalar(`SELECT substr(md5(random()::text), 1, 12)`)
	email = prefix + "-" + suffix + "@example.test"
	if err := f.pool.QueryRow(context.Background(),
		`INSERT INTO "user" (name, email) VALUES ('Deletion Request Test', $1) RETURNING id::text`, email).Scan(&id); err != nil {
		f.t.Fatalf("create user: %v", err)
	}
	f.users = append(f.users, id)
	return id, email
}

// workspace creates a workspace; in the tenant when inTenant is set (the tenant
// is made on first use).
func (f *fixture) workspace(inTenant bool) string {
	f.t.Helper()
	if inTenant && f.tenant == "" {
		f.tenant = f.scalar(`INSERT INTO tenant (id, name) VALUES (gen_random_uuid(), 'Deletion Test Tenant ' || substr(md5(random()::text), 1, 8)) RETURNING id::text`)
	}
	id := f.scalar(`INSERT INTO workspace (name, slug) VALUES ('Deletion Test', 'dr-' || substr(md5(random()::text), 1, 12)) RETURNING id::text`)
	f.wss = append(f.wss, id)
	if inTenant {
		f.exec(`INSERT INTO tenant_workspace (workspace_id, tenant_id) VALUES ($1, $2)`, id, f.tenant)
	}
	return id
}

func (f *fixture) slug(ws string) string {
	return f.scalar(`SELECT slug FROM workspace WHERE id = $1`, ws)
}

func (f *fixture) status(ws string) string {
	return f.scalar(`SELECT COALESCE((SELECT status FROM platform_workspace WHERE workspace_id = $1), 'active')`, ws)
}

func (f *fixture) auditRows(ws, action string) int {
	return f.count(`SELECT count(*) FROM audit_event WHERE workspace_id = $1 AND action = $2`, ws, action)
}

func (f *fixture) requestsFor(target string) int {
	return f.count(`SELECT count(*) FROM platform_deletion_request WHERE target_id = $1`, target)
}

func TestDeleteNowConfirmMismatchWritesNothing(t *testing.T) {
	f := newFixture(t)
	ws := f.workspace(false)
	_, _, err := f.svc.RequestWorkspace(context.Background(), f.staff, ws, Input{Reason: "policy", Confirm: "not-the-slug"})
	if e, ok := err.(*Error); !ok || e.Code != CodeConfirmationMismatch || e.Status != 400 {
		t.Fatalf("got %v, want confirmation_mismatch 400", err)
	}
	if n := f.requestsFor(ws); n != 0 {
		t.Fatalf("requests written: %d", n)
	}
	if f.status(ws) != "active" || len(f.cache.ids) != 0 {
		t.Fatalf("status changed or cache cleared on a mismatch")
	}
}

func TestDeleteNowPhaseOneWritesAuditRequestAndStatusInOneTransaction(t *testing.T) {
	f := newFixture(t)
	ws := f.workspace(true)
	r, created, err := f.svc.RequestWorkspace(context.Background(), f.staff, ws, Input{Reason: "legal hold lifted", Confirm: f.slug(ws)})
	if err != nil || !created {
		t.Fatalf("RequestWorkspace: created=%v err=%v", created, err)
	}
	if r.State != StatePending || r.Kind != KindWorkspace || r.TenantID != f.tenant {
		t.Fatalf("request = %+v", r)
	}
	if f.status(ws) != "pending_deletion" {
		t.Fatalf("status = %q", f.status(ws))
	}
	if n := f.auditRows(ws, "workspace.immediate_deletion_requested"); n != 1 {
		t.Fatalf("request audit rows = %d", n)
	}
	tenantOnAudit := f.scalar(`SELECT COALESCE(tenant_id::text, '') FROM audit_event WHERE workspace_id = $1 AND action = 'workspace.immediate_deletion_requested'`, ws)
	if tenantOnAudit != f.tenant {
		t.Fatalf("audit tenant_id = %q, want %q", tenantOnAudit, f.tenant)
	}
	if len(f.cache.ids) != 1 || f.cache.ids[0] != ws {
		t.Fatalf("cache keys cleared = %v", f.cache.ids)
	}
}

func TestDeleteNowSetsPendingDeletionDueNowAndClearsCache(t *testing.T) {
	f := newFixture(t)
	ws := f.workspace(false)
	if _, _, err := f.svc.RequestWorkspace(context.Background(), f.staff, ws, Input{Reason: "r", Confirm: f.slug(ws)}); err != nil {
		t.Fatal(err)
	}
	if f.count(`SELECT count(*) FROM platform_workspace WHERE workspace_id = $1 AND deletion_scheduled_at <= now() + interval '5 seconds'`, ws) != 1 {
		t.Fatalf("deletion_scheduled_at is not due now")
	}
	reason := f.scalar(`SELECT reason FROM platform_workspace WHERE workspace_id = $1`, ws)
	if reason != ReasonImmediate {
		t.Fatalf("status reason = %q", reason)
	}
}

func TestDeleteNowExpeditesAlreadyScheduledDeletion(t *testing.T) {
	f := newFixture(t)
	ws := f.workspace(false)
	f.exec(`INSERT INTO platform_workspace (workspace_id, status, reason, deletion_scheduled_at)
	        VALUES ($1, 'pending_deletion', 'scheduled', now() + interval '14 days')`, ws)
	if _, _, err := f.svc.RequestWorkspace(context.Background(), f.staff, ws, Input{Reason: "now", Confirm: f.slug(ws)}); err != nil {
		t.Fatal(err)
	}
	if f.count(`SELECT count(*) FROM platform_workspace WHERE workspace_id = $1 AND deletion_scheduled_at <= now() + interval '5 seconds'`, ws) != 1 {
		t.Fatalf("scheduled deletion was not brought forward to now")
	}
}

func TestDeleteNowSecondRequestReturnsLiveOne(t *testing.T) {
	f := newFixture(t)
	ws := f.workspace(false)
	first, _, err := f.svc.RequestWorkspace(context.Background(), f.staff, ws, Input{Reason: "one", Confirm: f.slug(ws)})
	if err != nil {
		t.Fatal(err)
	}
	second, created, err := f.svc.RequestWorkspace(context.Background(), f.staff, ws, Input{Reason: "two", Confirm: f.slug(ws)})
	if err != nil || created {
		t.Fatalf("second request: created=%v err=%v", created, err)
	}
	if second.ID != first.ID {
		t.Fatalf("second request id %s, want live %s", second.ID, first.ID)
	}
	if n := f.requestsFor(ws); n != 1 {
		t.Fatalf("live requests = %d", n)
	}
}

func TestDeleteNowTenantListsEveryWorkspaceAndAuditsEach(t *testing.T) {
	f := newFixture(t)
	a := f.workspace(true)
	b := f.workspace(true)
	tenantName := f.scalar(`SELECT name FROM tenant WHERE id = $1`, f.tenant)
	r, created, err := f.svc.RequestTenant(context.Background(), f.staff, f.tenant, Input{Reason: "closing", Confirm: tenantName})
	if err != nil || !created {
		t.Fatalf("RequestTenant: created=%v err=%v", created, err)
	}
	if r.Kind != KindTenant {
		t.Fatalf("kind = %q", r.Kind)
	}
	for _, ws := range []string{a, b} {
		if f.status(ws) != "pending_deletion" {
			t.Fatalf("workspace %s status = %q", ws, f.status(ws))
		}
		if n := f.auditRows(ws, "workspace.immediate_deletion_requested"); n != 1 {
			t.Fatalf("workspace %s request audit rows = %d", ws, n)
		}
	}
	if n := f.count(`SELECT count(*) FROM audit_event WHERE tenant_id = $1 AND action = 'tenant.immediate_deletion_requested'`, f.tenant); n != 1 {
		t.Fatalf("tenant audit rows = %d", n)
	}
}

func TestDeleteNowRejectsUnknownWorkspace(t *testing.T) {
	f := newFixture(t)
	_, _, err := f.svc.RequestWorkspace(context.Background(), f.staff, "0f0f0f0f-0000-4000-8000-00000000abcd", Input{Reason: "r", Confirm: "x"})
	if err != ErrNotFound {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestCancelRestoresActiveAndAudits(t *testing.T) {
	f := newFixture(t)
	ws := f.workspace(false)
	r, _, err := f.svc.RequestWorkspace(context.Background(), f.staff, ws, Input{Reason: "r", Confirm: f.slug(ws)})
	if err != nil {
		t.Fatal(err)
	}
	out, err := f.svc.Cancel(context.Background(), f.staff, r.ID)
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if out.State != StateCanceled {
		t.Fatalf("state = %q", out.State)
	}
	if f.status(ws) != "active" {
		t.Fatalf("workspace status after cancel = %q", f.status(ws))
	}
	if n := f.auditRows(ws, "deletion.request_canceled"); n != 1 {
		t.Fatalf("cancel audit rows = %d", n)
	}
	if _, err := f.svc.Cancel(context.Background(), f.staff, r.ID); err == nil {
		t.Fatal("a second cancel of a canceled request must be refused")
	}
}

func TestRetryResetsAttempts(t *testing.T) {
	f := newFixture(t)
	ws := f.workspace(false)
	r, _, err := f.svc.RequestWorkspace(context.Background(), f.staff, ws, Input{Reason: "r", Confirm: f.slug(ws)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Retry(context.Background(), f.staff, r.ID); err == nil {
		t.Fatal("retry of a pending request must be refused")
	}
	f.exec(`UPDATE platform_deletion_request SET state = 'failed', attempts = 3, error_code = 'x' WHERE id = $1`, r.ID)
	out, err := f.svc.Retry(context.Background(), f.staff, r.ID)
	if err != nil {
		t.Fatalf("Retry: %v", err)
	}
	if out.State != StatePending || out.Attempts != 0 || out.ErrorCode != "" {
		t.Fatalf("after retry: %+v", out)
	}
	if n := f.auditRows(ws, "deletion.request_retried"); n != 1 {
		t.Fatalf("retry audit rows = %d", n)
	}
}

func TestDeletionRequestVisibilityForTenant(t *testing.T) {
	f := newFixture(t)
	ws := f.workspace(true)
	if _, _, err := f.svc.RequestWorkspace(context.Background(), f.staff, ws, Input{Reason: "r", Confirm: f.slug(ws)}); err != nil {
		t.Fatal(err)
	}
	other := f.workspace(false)
	if _, _, err := f.svc.RequestWorkspace(context.Background(), f.staff, other, Input{Reason: "r", Confirm: f.slug(other)}); err != nil {
		t.Fatal(err)
	}
	rows, err := f.svc.List(context.Background(), Filter{TenantID: f.tenant})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].TargetID != ws {
		t.Fatalf("tenant list = %+v, want only the tenant's workspace", rows)
	}
}
