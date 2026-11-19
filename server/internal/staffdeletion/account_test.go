package staffdeletion

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// stageAccountRequest writes the account request the platform writes in phase 1
// (spec R6 section 7.4), with the resolutions the staff member chose. The account
// is pending deletion and due now. The rows go away with the test.
func stageAccountRequest(t *testing.T, userID, email string, resolutions []map[string]string, state string) string {
	t.Helper()
	ctx := context.Background()
	res, err := json.Marshal(resolutions)
	if err != nil {
		t.Fatalf("encode resolutions: %v", err)
	}
	if _, err := testPool.Exec(ctx, `INSERT INTO platform_account (user_id, status, reason, changed_by, changed_at, sessions_valid_after, deletion_scheduled_at)
		VALUES ($1::uuid, 'pending_deletion', 'staff_immediate_deletion', $1::uuid, now(), now(), now())
		ON CONFLICT (user_id) DO UPDATE SET status = 'pending_deletion', deletion_scheduled_at = now(), sessions_valid_after = now()`, userID); err != nil {
		t.Fatalf("stage platform account: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM platform_account WHERE user_id = $1::uuid`, userID)
	})
	id := uuid.NewString()
	if _, err := testPool.Exec(ctx, `INSERT INTO platform_deletion_request
		(id, kind, target_id, target_label, reason, state, resolutions, requested_by, requested_by_label)
		VALUES ($1::uuid, 'account', $2::uuid, $3, 'staff account test', $4, $5::jsonb, $6::uuid, 'staff tester')`,
		id, userID, email, state, string(res), uuid.NewString()); err != nil {
		t.Fatalf("stage account request: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM platform_deletion_request WHERE id = $1::uuid OR parent_id = $1::uuid`, id)
	})
	return id
}

// stageChildRequest writes a workspace request that belongs to an account request
// (kind workspace, parent_id set), as the platform does for a delete choice.
func stageChildRequest(t *testing.T, parentID, workspaceID, state string) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := testPool.Exec(context.Background(), `INSERT INTO platform_deletion_request
		(id, kind, target_id, target_label, reason, state, parent_id, requested_by, requested_by_label)
		VALUES ($1::uuid, 'workspace', $2::uuid, 'child target', 'staff account test', $3, $4::uuid, $5::uuid, 'staff tester')`,
		id, workspaceID, state, parentID, uuid.NewString()); err != nil {
		t.Fatalf("stage child request: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM platform_deletion_request WHERE id = $1::uuid`, id)
	})
	return id
}

// accountUser makes a user with the given email, removed with the test.
func accountUser(t *testing.T, name string) (id, email string) {
	t.Helper()
	email = testutil.UniqueEmail("acct-" + name)
	return fx.User(t, name, email), email
}

func userExists(t *testing.T, id string) bool {
	t.Helper()
	return count(t, `SELECT count(*) FROM "user" WHERE id = $1::uuid AND name <> 'Deleted user'`, id) == 1
}

func memberRole(t *testing.T, ws, user string) string {
	t.Helper()
	var role string
	err := testPool.QueryRow(context.Background(), `SELECT role FROM member WHERE workspace_id = $1::uuid AND user_id = $2::uuid`, ws, user).Scan(&role)
	if err != nil {
		return ""
	}
	return role
}

func TestStaffAccountDeletionTransfersOwnershipThenDeletes(t *testing.T) {
	ctx := context.Background()
	user, email := accountUser(t, "transfer")
	successor, _ := accountUser(t, "successor")
	ws := fx.Workspace(t, "acct transfer", "sa-tr-"+uuid.NewString()[:8])
	fx.Member(t, ws, user, "owner")
	fx.Member(t, ws, successor, "admin")
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM audit_event WHERE workspace_id = $1::uuid OR target_id = $2`, ws, user)
	})

	reqID := stageAccountRequest(t, user, email, []map[string]string{
		{"workspace_id": ws, "action": "transfer", "successor_user_id": successor},
	}, "pending")

	if _, err := newRunner(nil).RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if got := memberRole(t, ws, successor); got != "owner" {
		t.Errorf("successor role = %q, want owner (transfer)", got)
	}
	if userExists(t, user) {
		t.Error("the account was not deleted")
	}
	if state, _, _ := requestState(t, reqID); state != "completed" {
		t.Errorf("account request state = %q, want completed", state)
	}
	if n := count(t, `SELECT count(*) FROM platform_deletion_request WHERE id = $1::uuid AND target_label = 'deleted user'`, reqID); n != 1 {
		t.Errorf("request target_label is not 'deleted user' after completion")
	}
	if n := count(t, `SELECT count(*) FROM audit_event WHERE action = 'member.role_changed' AND actor_type = 'staff'
		AND target_id = $1 AND detail->>'request_id' = $2`, successor, reqID); n != 1 {
		t.Errorf("member.role_changed rows by staff = %d, want 1", n)
	}
}

func TestStaffAccountDeletionDeletesWorkspaceThenAccount(t *testing.T) {
	ctx := context.Background()
	user, email := accountUser(t, "delete choice")
	other, _ := accountUser(t, "delete choice member")
	ws := fx.Workspace(t, "acct delete", "sa-dl-"+uuid.NewString()[:8])
	fx.Member(t, ws, user, "owner")
	fx.Member(t, ws, other, "member")
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM audit_event WHERE workspace_id = $1::uuid OR target_id = $2`, ws, user)
	})

	reqID := stageAccountRequest(t, user, email, []map[string]string{
		{"workspace_id": ws, "action": "delete"},
	}, "pending")
	childID := stageChildRequest(t, reqID, ws, "pending")
	stageAuditRow(t, "workspace.immediate_deletion_requested", ws, ws)

	// The child is the workspace deletion; the account waits for it, then runs.
	runner := newRunner(nil)
	for i := 0; i < 2; i++ {
		if _, err := runner.RunOnce(ctx); err != nil {
			t.Fatalf("RunOnce %d: %v", i+1, err)
		}
	}
	if state, _, _ := requestState(t, childID); state != "completed" {
		t.Errorf("child request state = %q, want completed", state)
	}
	if !workspaceGone(t, ws) {
		t.Error("the delete choice did not delete its workspace")
	}
	if userExists(t, user) {
		t.Error("the account was not deleted after its child completed")
	}
	if state, _, _ := requestState(t, reqID); state != "completed" {
		t.Errorf("account request state = %q, want completed", state)
	}
}

// A child that is still running holds the account: it is released, not failed,
// and the account is not deleted.
func TestStaffAccountDeletionWaitsForChildRequests(t *testing.T) {
	ctx := context.Background()
	user, email := accountUser(t, "wait child")
	ws := fx.Workspace(t, "acct wait", "sa-wt-"+uuid.NewString()[:8])
	fx.Member(t, ws, user, "owner")
	reqID := stageAccountRequest(t, user, email, []map[string]string{
		{"workspace_id": ws, "action": "delete"},
	}, "pending")
	stageChildRequest(t, reqID, ws, "running")

	if _, err := newRunner(nil).RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	state, attempts, code := requestState(t, reqID)
	if state != "pending" || attempts != 0 || code != "" {
		t.Errorf("account while child runs: state=%q attempts=%d error_code=%q, want pending/0/empty", state, attempts, code)
	}
	if !userExists(t, user) {
		t.Error("the account was deleted while a child request was still running")
	}
}

func TestStaffAccountDeletionFailsWhenChildFailed(t *testing.T) {
	ctx := context.Background()
	user, email := accountUser(t, "child failed")
	ws := fx.Workspace(t, "acct child failed", "sa-cf-"+uuid.NewString()[:8])
	fx.Member(t, ws, user, "owner")
	reqID := stageAccountRequest(t, user, email, []map[string]string{
		{"workspace_id": ws, "action": "delete"},
	}, "pending")
	stageChildRequest(t, reqID, ws, "failed")

	if _, err := newRunner(nil).RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	state, _, code := requestState(t, reqID)
	if state != "failed" || code != "child_failed" {
		t.Errorf("account request = %q/%q, want failed/child_failed", state, code)
	}
	if !userExists(t, user) {
		t.Error("the account was deleted although a child failed")
	}
}

// A successor who left the workspace between the phases stops the account, and
// the account is kept (spec R6 section 7.4, phase 2 step 2).
func TestStaffAccountDeletionSuccessorGoneFailsAndKeepsAccount(t *testing.T) {
	ctx := context.Background()
	user, email := accountUser(t, "successor gone")
	successor, _ := accountUser(t, "successor leaves")
	ws := fx.Workspace(t, "acct successor gone", "sa-sg-"+uuid.NewString()[:8])
	fx.Member(t, ws, user, "owner")
	reqID := stageAccountRequest(t, user, email, []map[string]string{
		{"workspace_id": ws, "action": "transfer", "successor_user_id": successor},
	}, "pending")

	if _, err := newRunner(nil).RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	state, _, code := requestState(t, reqID)
	if state != "failed" || code != "sole_owner_unresolved" {
		t.Errorf("account request = %q/%q, want failed/sole_owner_unresolved", state, code)
	}
	if !userExists(t, user) {
		t.Error("the account was deleted with its sole owner unresolved")
	}
}

// A co-owner needs no resolution: the account leaves and the workspace keeps its
// other owner. The audit rows name staff and carry no email.
func TestStaffAccountDeletionCoOwnerNeedsNoResolution(t *testing.T) {
	ctx := context.Background()
	user, email := accountUser(t, "co owner")
	coOwner, _ := accountUser(t, "co owner partner")
	ws := fx.Workspace(t, "acct co owner", "sa-co-"+uuid.NewString()[:8])
	fx.Member(t, ws, user, "owner")
	fx.Member(t, ws, coOwner, "owner")
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM audit_event WHERE workspace_id = $1::uuid OR target_id = $2`, ws, user)
	})
	reqID := stageAccountRequest(t, user, email, nil, "pending")

	if _, err := newRunner(nil).RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if userExists(t, user) {
		t.Error("the co-owner's account was not deleted")
	}
	if got := memberRole(t, ws, coOwner); got != "owner" {
		t.Errorf("the workspace kept no owner: partner role = %q", got)
	}
	if n := count(t, `SELECT count(*) FROM audit_event WHERE action = 'member.left' AND actor_type = 'staff'
		AND detail->>'origin' = 'staff' AND detail->>'request_id' = $1`, reqID); n != 1 {
		t.Errorf("member.left rows by staff with origin detail = %d, want 1", n)
	}
	if n := count(t, `SELECT count(*) FROM audit_event WHERE action = $1 AND actor_type = 'staff' AND target_id = $2::text
		AND target_label = 'deleted user' AND detail->>'request_id' = $3`, ActionAccountDeleted, user, reqID); n != 1 {
		t.Errorf("account.deleted rows by staff with the deleted-user label = %d, want 1", n)
	}
	if n := count(t, `SELECT count(*) FROM audit_event WHERE target_id = $1 AND target_label LIKE '%@%'`, user); n != 0 {
		t.Errorf("audit rows still carry the email of the deleted account: %d", n)
	}
}

// Accounts with a live or failed staff request are left to the staff job, so the
// self-service account_deletion job never deletes them first.
func TestAccountDeletionJobSkipsAccountsWithStaffRequest(t *testing.T) {
	ctx := context.Background()
	staffed, email := accountUser(t, "skip staffed")
	plain, _ := accountUser(t, "skip plain")
	stageAccountRequest(t, staffed, email, nil, "pending")
	if _, err := testPool.Exec(ctx, `INSERT INTO platform_account (user_id, status, reason, changed_by, changed_at, sessions_valid_after, deletion_scheduled_at)
		VALUES ($1::uuid, 'pending_deletion', 'user_requested', $1::uuid, now(), now(), now() - interval '1 minute')`, plain); err != nil {
		t.Fatalf("stage plain account: %v", err)
	}
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM platform_account WHERE user_id = $1::uuid`, plain) })

	due, err := testHandler.DueAccountDeletions(ctx, time.Now(), 1000000)
	if err != nil {
		t.Fatalf("DueAccountDeletions: %v", err)
	}
	seen := map[string]bool{}
	for _, id := range due {
		seen[id] = true
	}
	if !seen[plain] {
		t.Fatal("control: a due account without a staff request is not listed")
	}
	if seen[staffed] {
		t.Error("account_deletion lists an account with a live staff request")
	}
}
