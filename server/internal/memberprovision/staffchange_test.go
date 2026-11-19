package memberprovision

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/analytics"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/handler"
	"github.com/multica-ai/multica/server/internal/realtime"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

var (
	staffHandlerOnce sync.Once
	staffHandlerInst *handler.Handler
)

// staffChanger is the real handler, built the way the server builds it, for the
// remove and change_role requests the job runs through it.
func staffChanger(t *testing.T) *handler.Handler {
	t.Helper()
	staffHandlerOnce.Do(func() {
		hub := realtime.NewHub()
		go hub.Run()
		staffHandlerInst = handler.New(db.New(testPool), testPool, hub, events.New(), service.NewEmailService(), nil, nil, analytics.NoopClient{}, handler.Config{})
	})
	return staffHandlerInst
}

// changeRunner is the job over the real handler, for remove and change_role.
func (w *world) changeRunner(t *testing.T) *Runner {
	t.Helper()
	d := w.deps
	d.Members = staffChanger(t)
	return New(d)
}

// stageChange writes a pending remove or change_role request as the platform does.
func (w *world) stageChange(kind, targetUserID, previousRole, role string) uuid.UUID {
	w.t.Helper()
	id := uuid.New()
	email := ""
	if err := testPool.QueryRow(context.Background(), `SELECT email FROM "user" WHERE id = $1::uuid`, targetUserID).Scan(&email); err != nil {
		w.t.Fatalf("read target email: %v", err)
	}
	if _, err := testPool.Exec(context.Background(), `
		INSERT INTO platform_member_request (id, workspace_id, kind, target_user_id, previous_role, email, role, requested_by, requested_by_label)
		VALUES ($1, $2::uuid, $3, $4::uuid, $5, $6, $7, $8::uuid, 'staff-label@example.test')`,
		id, w.ws, kind, targetUserID, previousRole, email, role, w.staff); err != nil {
		w.t.Fatalf("insert %s request: %v", kind, err)
	}
	return id
}

// mustRunOnce runs the job once and fails the test on error.
func mustRunOnce(t *testing.T, r *Runner) int {
	t.Helper()
	n, err := r.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	return n
}

// wantDone checks a request completed. A remove leaves member_id empty.
func (w *world) wantDone(id uuid.UUID) {
	w.t.Helper()
	if r := w.row(id); r.State != "completed" || r.ErrorCode != "" || !r.Completed {
		w.t.Fatalf("request = %+v, want completed", r)
	}
}

// memberOf adds a user with a role to the workspace and returns the user id.
func (w *world) memberOf(name, role string) string {
	w.t.Helper()
	id := w.f.User(w.t, name, testutil.UniqueEmail(name))
	w.f.Member(w.t, w.ws, id, role)
	return id
}

func (w *world) memberRoleByID(userID string) string {
	w.t.Helper()
	var role string
	err := testPool.QueryRow(context.Background(), `SELECT role FROM member WHERE workspace_id = $1::uuid AND user_id = $2::uuid`, w.ws, userID).Scan(&role)
	if err != nil {
		return ""
	}
	return role
}

func (w *world) auditCount(action, targetID string) int {
	w.t.Helper()
	return count(w.t, `SELECT count(*) FROM audit_event WHERE workspace_id = $1::uuid AND action = $2 AND target_id = $3 AND actor_type = 'staff'`, w.ws, action, targetID)
}

func TestMemberRemoveUsesRevokeAndRemoveMember(t *testing.T) {
	w := newWorld(t)
	target := w.memberOf("remove target", "member")
	reqID := w.stageChange("remove", target, "member", "member")

	if n := mustRunOnce(t, w.changeRunner(t)); n != 1 {
		t.Fatalf("RunOnce = %d, want 1", n)
	}
	if got := w.memberRoleByID(target); got != "" {
		t.Errorf("the removed user still has role %q", got)
	}
	if got := w.memberRoleByID(w.owner); got != "owner" {
		t.Errorf("the owner's row changed: %q", got)
	}
	w.wantDone(reqID)
	if n := w.auditCount("member.removed", target); n != 1 {
		t.Errorf("member.removed rows by staff = %d, want 1", n)
	}
}

func TestMemberRemoveRefusesSoleOwner(t *testing.T) {
	w := newWorld(t)
	reqID := w.stageChange("remove", w.owner, "owner", "owner")

	mustRunOnce(t, w.changeRunner(t))
	w.wantFailed(reqID, "sole_owner")
	if got := w.memberRoleByID(w.owner); got != "owner" {
		t.Errorf("the sole owner was removed: role %q", got)
	}
}

// Two runs, in parallel and then again, remove the member once.
func TestMemberRemoveTwiceAndParallelRemovesOnce(t *testing.T) {
	w := newWorld(t)
	target := w.memberOf("remove twice", "member")
	reqID := w.stageChange("remove", target, "member", "member")

	runner := w.changeRunner(t)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runner.RunOnce(context.Background())
		}()
	}
	wg.Wait()
	runner.RunOnce(context.Background())

	w.wantDone(reqID)
	if n := w.auditCount("member.removed", target); n != 1 {
		t.Errorf("member.removed rows = %d, want exactly 1", n)
	}
}

// The audit row names the tenant of the workspace, so the tenant's admin reads it.
func TestMemberRemoveAuditCarriesTenantAndStaffActor(t *testing.T) {
	w := newWorld(t)
	target := w.memberOf("remove tenant", "member")
	tenantID := uuid.NewString()
	if _, err := testPool.Exec(context.Background(), `INSERT INTO tenant (id, name) VALUES ($1::uuid, 'member tenant')`, tenantID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM tenant WHERE id = $1::uuid`, tenantID) })
	if _, err := testPool.Exec(context.Background(), `INSERT INTO tenant_workspace (workspace_id, tenant_id) VALUES ($1::uuid, $2::uuid)`, w.ws, tenantID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM tenant_workspace WHERE workspace_id = $1::uuid`, w.ws)
	})
	w.stageChange("remove", target, "member", "member")

	mustRunOnce(t, w.changeRunner(t))
	if n := count(t, `SELECT count(*) FROM audit_event WHERE action = 'member.removed' AND target_id = $1 AND tenant_id = $2::uuid AND actor_type = 'staff'`, target, tenantID); n != 1 {
		t.Errorf("member.removed rows with the tenant id and a staff actor = %d, want 1", n)
	}
}

func TestMemberChangeRoleUpdatesRoleAndAudits(t *testing.T) {
	w := newWorld(t)
	target := w.memberOf("change role", "viewer")
	reqID := w.stageChange("change_role", target, "viewer", "admin")

	mustRunOnce(t, w.changeRunner(t))
	if got := w.memberRoleByID(target); got != "admin" {
		t.Errorf("role = %q, want admin", got)
	}
	w.wantDone(reqID)
	if n := w.auditCount("member.role_changed", target); n != 1 {
		t.Errorf("member.role_changed rows by staff = %d, want 1", n)
	}
}

// Owner is immutable on either side of a change (spec R6 D94).
func TestMemberChangeRoleRefusesOwnerOnEitherSide(t *testing.T) {
	w := newWorld(t)
	member := w.memberOf("to owner", "member")
	toOwner := w.stageChange("change_role", member, "member", "owner")
	fromOwner := w.stageChange("change_role", w.owner, "owner", "member")

	mustRunOnce(t, w.changeRunner(t))
	w.wantFailed(toOwner, "owner_role_immutable")
	w.wantFailed(fromOwner, "owner_role_immutable")
	if got := w.memberRoleByID(member); got != "member" {
		t.Errorf("a refused change moved the role to %q", got)
	}
}

// A role that moved since the platform read it fails, and nothing is written.
func TestMemberChangeRoleStaleRoleFails(t *testing.T) {
	w := newWorld(t)
	target := w.memberOf("stale role", "admin")
	reqID := w.stageChange("change_role", target, "member", "viewer")

	mustRunOnce(t, w.changeRunner(t))
	w.wantFailed(reqID, "role_changed_meanwhile")
	if got := w.memberRoleByID(target); got != "admin" {
		t.Errorf("a stale change wrote role %q", got)
	}
}
