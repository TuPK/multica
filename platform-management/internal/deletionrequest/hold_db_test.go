package deletionrequest

import (
	"context"
	"io"
	"log/slog"
	"testing"
)

// Spec R6 section 3c.7 rule 3: a live hold on a workspace refuses its immediate
// deletion with no write, and the refusal is audited on its own.

func TestLegalHoldBlocksPhaseOneWithoutWriting(t *testing.T) {
	f := newFixture(t)
	ws := f.workspace(false)
	f.exec(`INSERT INTO legal_hold (id, scope_kind, scope_id, reason, case_ref) VALUES (gen_random_uuid(), 'workspace', $1, 'court order', 'CASE-9')`, ws)
	t.Cleanup(func() { f.pool.Exec(context.Background(), `DELETE FROM legal_hold WHERE scope_id = $1`, ws) })
	svc := New(Deps{Runner: NewPgxRunner(f.pool), Reader: NewPgxReader(f.pool), Cache: f.cache, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})

	_, _, err := svc.RequestWorkspace(context.Background(), f.staff, ws, Input{Reason: "closing", Confirm: f.slug(ws)})
	de := asError(t, err)
	if de.Code != CodeLegalHold || de.Status != 409 {
		t.Fatalf("got %d %s, want 409 legal_hold", de.Status, de.Code)
	}
	if f.requestsFor(ws) != 0 || f.status(ws) != "active" {
		t.Fatal("a held workspace was written to")
	}
	if n := f.count(`SELECT count(*) FROM audit_event WHERE workspace_id = $1 AND action = 'deletion.blocked_by_hold'`, ws); n != 1 {
		t.Fatalf("blocked audit rows = %d", n)
	}
}

// Spec R6 section 3c.7 rule 1: a hold on a tenant covers each workspace of it,
// and a hold on an account covers that account's deletion.
func TestHoldOnTenantCoversWorkspaceAndAccountChecks(t *testing.T) {
	t.Run("tenant hold covers a workspace asked for by its own id", func(t *testing.T) {
		f := newFixture(t)
		ws := f.workspace(true)
		f.exec(`INSERT INTO legal_hold (id, scope_kind, scope_id, reason, case_ref) VALUES (gen_random_uuid(), 'tenant', $1, 'court order', 'CASE-TW')`, f.tenant)
		t.Cleanup(func() { f.pool.Exec(context.Background(), `DELETE FROM legal_hold WHERE scope_id = $1`, f.tenant) })

		_, _, err := f.svc.RequestWorkspace(context.Background(), f.staff, ws, Input{Reason: "closing", Confirm: f.slug(ws)})
		if de := asError(t, err); de.Code != CodeLegalHold || de.Status != 409 {
			t.Fatalf("got %d %s, want 409 legal_hold", de.Status, de.Code)
		}
	})
	t.Run("account hold covers the account deletion", func(t *testing.T) {
		af := newAccountFixture(t)
		target, email := af.user("held")
		ws := af.workspace(false)
		af.member(ws, target, "member")
		af.exec(`INSERT INTO legal_hold (id, scope_kind, scope_id, reason, case_ref) VALUES (gen_random_uuid(), 'account', $1, 'court order', 'CASE-AC')`, target)
		t.Cleanup(func() { af.pool.Exec(context.Background(), `DELETE FROM legal_hold WHERE scope_id = $1`, target) })

		_, _, err := af.svc.RequestAccount(context.Background(), af.staff, target, AccountInput{Reason: "closing", Confirm: email})
		if de := asError(t, err); de.Code != CodeLegalHold || de.Status != 409 {
			t.Fatalf("got %d %s, want 409 legal_hold", de.Status, de.Code)
		}
	})
}

// Spec R6 section 3c.7 rule 3: a hold on a workspace, a tenant or an account
// refuses the deletion with no request row and no status change.
func TestDeleteNowRefusedWhileHeldWritesNothing(t *testing.T) {
	t.Run("workspace", func(t *testing.T) {
		f := newFixture(t)
		ws := f.workspace(false)
		f.exec(`INSERT INTO legal_hold (id, scope_kind, scope_id, reason, case_ref) VALUES (gen_random_uuid(), 'workspace', $1, 'court order', 'CASE-W')`, ws)
		t.Cleanup(func() { f.pool.Exec(context.Background(), `DELETE FROM legal_hold WHERE scope_id = $1`, ws) })

		_, _, err := f.svc.RequestWorkspace(context.Background(), f.staff, ws, Input{Reason: "closing", Confirm: f.slug(ws)})
		if de := asError(t, err); de.Code != CodeLegalHold {
			t.Fatalf("got %s, want legal_hold", de.Code)
		}
		if f.requestsFor(ws) != 0 || f.status(ws) != "active" {
			t.Fatalf("held workspace written: requests=%d status=%s", f.requestsFor(ws), f.status(ws))
		}
	})
	t.Run("tenant", func(t *testing.T) {
		f := newFixture(t)
		ws := f.workspace(true)
		name := f.scalar(`SELECT name FROM tenant WHERE id = $1`, f.tenant)
		f.exec(`INSERT INTO legal_hold (id, scope_kind, scope_id, reason, case_ref) VALUES (gen_random_uuid(), 'tenant', $1, 'court order', 'CASE-T2')`, f.tenant)
		t.Cleanup(func() { f.pool.Exec(context.Background(), `DELETE FROM legal_hold WHERE scope_id = $1`, f.tenant) })

		_, _, err := f.svc.RequestTenant(context.Background(), f.staff, f.tenant, Input{Reason: "closing", Confirm: name})
		if de := asError(t, err); de.Code != CodeLegalHold {
			t.Fatalf("got %s, want legal_hold", de.Code)
		}
		if f.requestsFor(f.tenant) != 0 || f.status(ws) != "active" {
			t.Fatalf("held tenant written: requests=%d status=%s", f.requestsFor(f.tenant), f.status(ws))
		}
	})
	t.Run("account", func(t *testing.T) {
		af := newAccountFixture(t)
		target, email := af.user("held")
		ws := af.workspace(false)
		af.member(ws, target, "member")
		af.exec(`INSERT INTO legal_hold (id, scope_kind, scope_id, reason, case_ref) VALUES (gen_random_uuid(), 'account', $1, 'court order', 'CASE-A2')`, target)
		t.Cleanup(func() { af.pool.Exec(context.Background(), `DELETE FROM legal_hold WHERE scope_id = $1`, target) })

		_, _, err := af.svc.RequestAccount(context.Background(), af.staff, target, AccountInput{Reason: "closing", Confirm: email})
		if de := asError(t, err); de.Code != CodeLegalHold {
			t.Fatalf("got %s, want legal_hold", de.Code)
		}
		if af.requestsFor(target) != 0 || af.accountStatus(target) != "active" {
			t.Fatalf("held account written: requests=%d status=%s", af.requestsFor(target), af.accountStatus(target))
		}
	})
}
