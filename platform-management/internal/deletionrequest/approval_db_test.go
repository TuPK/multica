package deletionrequest

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

// Spec R6 section 3c.6 (QT10): with tenant.delete_now in the approval list, a
// tenant deletion waits for a second system_admin; the requester cannot approve;
// an approval runs phase 1; a reject cancels; an awaiting request expires after 24 hours.

func approvalService(f *fixture, actions ...string) *Service {
	set := map[string]bool{}
	for _, a := range actions {
		set[a] = true
	}
	return New(Deps{
		Runner: NewPgxRunner(f.pool), Reader: NewPgxReader(f.pool), Cache: f.cache,
		ApprovalActions: set, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}

// second makes a second active system_admin and returns its actor.
func second(f *fixture) Actor {
	id, email := f.user("approver")
	f.exec(`INSERT INTO platform_staff (user_id, role) VALUES ($1, 'system_admin')`, id)
	return Actor{ID: id, Label: email}
}

// adminCountTx is a Tx that reports a fixed number of active system administrators.
// The count is global, so the rule is tested through this fake, not the shared table.
type adminCountTx struct {
	Tx
	n int64
}

func (a adminCountTx) CountSystemAdmins(context.Context) (int64, error) { return a.n, nil }

func TestApprovalNeedsTwoAdminsAndWritesNothingWithOne(t *testing.T) {
	svc := approvalService(&fixture{}, ActionTenantDelete)
	if err := svc.requireSecondAdmin(context.Background(), adminCountTx{n: 1}); asError(t, err).Code != CodeApproverUnavailable {
		t.Fatalf("one admin: %v, want approver_unavailable", err)
	}
	if err := svc.requireSecondAdmin(context.Background(), adminCountTx{n: 2}); err != nil {
		t.Fatalf("two admins must be enough: %v", err)
	}
}

func TestTenantDeletionWaitsForASecondAdminAndApprovalRunsPhaseOne(t *testing.T) {
	f := newFixture(t)
	f.exec(`INSERT INTO platform_staff (user_id, role) VALUES ($1, 'system_admin')`, f.staff.ID)
	t.Cleanup(func() { f.pool.Exec(context.Background(), `DELETE FROM platform_staff WHERE user_id = $1`, f.staff.ID) })
	approver := second(f)
	ws := f.workspace(true)
	name := f.scalar(`SELECT name FROM tenant WHERE id = $1`, f.tenant)
	svc := approvalService(f, ActionTenantDelete)
	ctx := context.Background()

	r, created, err := svc.RequestTenant(ctx, f.staff, f.tenant, Input{Reason: "closing", Confirm: name})
	if err != nil || !created || r.State != StateAwaitingApproval {
		t.Fatalf("request: created=%v state=%s err=%v", created, r.State, err)
	}
	if f.status(ws) != "active" {
		t.Fatalf("an awaiting request changed the workspace to %q", f.status(ws))
	}

	if _, err := svc.Approve(ctx, f.staff, r.ID); asError(t, err).Code != CodeApproverMustDiffer {
		t.Fatalf("the requester approved their own request: %v", err)
	}
	done, err := svc.Approve(ctx, approver, r.ID)
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if done.State != StatePending || f.status(ws) != "pending_deletion" {
		t.Fatalf("after approval: state=%s status=%s", done.State, f.status(ws))
	}
	if n := f.scalar(`SELECT approved_by::text FROM platform_deletion_request WHERE id = $1`, r.ID); n != approver.ID {
		t.Fatalf("approved_by = %s", n)
	}
}

func TestRejectCancelsAndAwaitingRequestsExpireAfterADay(t *testing.T) {
	f := newFixture(t)
	f.exec(`INSERT INTO platform_staff (user_id, role) VALUES ($1, 'system_admin')`, f.staff.ID)
	t.Cleanup(func() { f.pool.Exec(context.Background(), `DELETE FROM platform_staff WHERE user_id = $1`, f.staff.ID) })
	approver := second(f)
	ctx := context.Background()
	svc := approvalService(f, ActionTenantDelete)

	f.workspace(true)
	name := f.scalar(`SELECT name FROM tenant WHERE id = $1`, f.tenant)
	r, _, err := svc.RequestTenant(ctx, f.staff, f.tenant, Input{Reason: "closing", Confirm: name})
	if err != nil {
		t.Fatalf("RequestTenant: %v", err)
	}
	if got, err := svc.Reject(ctx, approver, r.ID); err != nil || got.State != StateCanceled {
		t.Fatalf("Reject: state=%s err=%v", got.State, err)
	}

	// A new request for the same tenant (the rejected one no longer holds it) ages past the TTL.
	stale, _, err := svc.RequestTenant(ctx, f.staff, f.tenant, Input{Reason: "again", Confirm: name})
	if err != nil || stale.State != StateAwaitingApproval {
		t.Fatalf("second request: state=%s err=%v", stale.State, err)
	}
	f.exec(`UPDATE platform_deletion_request SET created_at = $2 WHERE id = $1`, stale.ID, time.Now().Add(-25*time.Hour))
	n, err := svc.ExpireApprovals(ctx)
	if err != nil || n < 1 {
		t.Fatalf("ExpireApprovals = %d, %v", n, err)
	}
	if f.scalar(`SELECT state FROM platform_deletion_request WHERE id = $1`, stale.ID) != StateCanceled {
		t.Fatal("an expired approval was not canceled")
	}
}

// countRunner reports a fixed number of system administrators inside every
// transaction, so the one-admin rule is checked without depending on the
// administrators the shared test database already holds.
type countRunner struct {
	Runner
	n int64
}

func (c countRunner) InTx(ctx context.Context, fn func(Tx) error) error {
	return c.Runner.InTx(ctx, func(tx Tx) error { return fn(adminCountTx{Tx: tx, n: c.n}) })
}

// Spec R6 section 3c.6 rule 5: with one active system_admin the request is refused
// at once with 409 approver_unavailable, and no request row or status change is written.
func TestSingleSystemAdminCannotRequireApproval(t *testing.T) {
	f := newFixture(t)
	ws := f.workspace(true)
	name := f.scalar(`SELECT name FROM tenant WHERE id = $1`, f.tenant)
	svc := approvalService(f, ActionTenantDelete)
	svc.Runner = countRunner{Runner: svc.Runner, n: 1}

	_, created, err := svc.RequestTenant(context.Background(), f.staff, f.tenant, Input{Reason: "closing", Confirm: name})
	if de := asError(t, err); de.Code != CodeApproverUnavailable || de.Status != 409 || created {
		t.Fatalf("created=%v err=%v, want 409 approver_unavailable and no request", created, err)
	}
	if n := f.requestsFor(f.tenant); n != 0 {
		t.Fatalf("requests written for the tenant: %d", n)
	}
	if f.status(ws) != "active" {
		t.Fatalf("a refused request changed the workspace to %q", f.status(ws))
	}
}

// Spec R6 section 3c.6 rule 4: an awaiting request left past 24 hours is canceled
// once. A second run of the job finds nothing and writes no second audit row.
func TestApprovalExpiresAfter24HoursIdempotent(t *testing.T) {
	f := newFixture(t)
	f.exec(`INSERT INTO platform_staff (user_id, role) VALUES ($1, 'system_admin')`, f.staff.ID)
	t.Cleanup(func() { f.pool.Exec(context.Background(), `DELETE FROM platform_staff WHERE user_id = $1`, f.staff.ID) })
	second(f)
	f.workspace(true)
	name := f.scalar(`SELECT name FROM tenant WHERE id = $1`, f.tenant)
	svc := approvalService(f, ActionTenantDelete)
	ctx := context.Background()

	r, _, err := svc.RequestTenant(ctx, f.staff, f.tenant, Input{Reason: "closing", Confirm: name})
	if err != nil || r.State != StateAwaitingApproval {
		t.Fatalf("RequestTenant: state=%s err=%v", r.State, err)
	}
	t.Cleanup(func() { f.pool.Exec(ctx, `DELETE FROM audit_event WHERE target_id = $1`, r.ID) })
	f.exec(`UPDATE platform_deletion_request SET created_at = $2 WHERE id = $1`, r.ID, time.Now().Add(-25*time.Hour))

	for run := 1; run <= 2; run++ {
		if _, err := svc.ExpireApprovals(ctx); err != nil {
			t.Fatalf("run %d: ExpireApprovals: %v", run, err)
		}
	}
	if s := f.scalar(`SELECT state FROM platform_deletion_request WHERE id = $1`, r.ID); s != StateCanceled {
		t.Fatalf("state = %s, want canceled", s)
	}
	if n := f.count(`SELECT count(*) FROM audit_event WHERE action = 'deletion.approval_expired' AND target_id = $1`, r.ID); n != 1 {
		t.Fatalf("expiry audit rows = %d, want 1", n)
	}
}
