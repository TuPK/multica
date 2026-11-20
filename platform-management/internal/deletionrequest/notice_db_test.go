package deletionrequest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// Spec R6 section 3c.9 (QT19): an immediate deletion notifies every owner of the
// workspace after the commit, a failed send does not stop the deletion, and an
// export requested with the deletion is queued for each workspace.

type fakeOwners struct{ emails []string }

func (f fakeOwners) OwnerEmails(context.Context, string) ([]string, error) { return f.emails, nil }

type fakeMail struct {
	mu   sync.Mutex
	to   []string
	fail error
}

func (f *fakeMail) SendDeletionNotice(_ context.Context, to, _ string, _ time.Time, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.to = append(f.to, to)
	return f.fail
}

func noticeService(f *fixture, owners OwnerLookup, mail DeletionMailer) *Service {
	return New(Deps{
		Runner: NewPgxRunner(f.pool), Reader: NewPgxReader(f.pool), Cache: f.cache,
		Owners: owners, Mail: mail, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}

func TestDeleteNowNoticesEveryOwnerAndAuditsEach(t *testing.T) {
	f := newFixture(t)
	ws := f.workspace(false)
	mail := &fakeMail{}
	svc := noticeService(f, fakeOwners{emails: []string{"one@example.test", "two@example.test"}}, mail)
	if _, _, err := svc.RequestWorkspace(context.Background(), f.staff, ws, Input{Reason: "closing", Confirm: f.slug(ws)}); err != nil {
		t.Fatalf("RequestWorkspace: %v", err)
	}
	if len(mail.to) != 2 {
		t.Fatalf("notices sent to %v, want both owners", mail.to)
	}
	if n := f.count(`SELECT count(*) FROM audit_event WHERE workspace_id = $1 AND action = 'deletion.notice_sent'`, ws); n != 2 {
		t.Fatalf("notice_sent audit rows = %d", n)
	}
}

func TestFailedNoticeDoesNotStopTheDeletion(t *testing.T) {
	f := newFixture(t)
	ws := f.workspace(false)
	svc := noticeService(f, fakeOwners{emails: []string{"owner@example.test"}}, &fakeMail{fail: errors.New("smtp down")})
	r, _, err := svc.RequestWorkspace(context.Background(), f.staff, ws, Input{Reason: "closing", Confirm: f.slug(ws)})
	if err != nil || r.State != StatePending || f.status(ws) != "pending_deletion" {
		t.Fatalf("deletion stopped by a failed notice: err=%v state=%s", err, r.State)
	}
	if n := f.count(`SELECT count(*) FROM audit_event WHERE workspace_id = $1 AND action = 'deletion.notice_failed'`, ws); n != 1 {
		t.Fatalf("notice_failed audit rows = %d", n)
	}
}

func TestExportFirstQueuesOneExportPerWorkspace(t *testing.T) {
	f := newFixture(t)
	ws := f.workspace(false)
	t.Cleanup(func() { f.pool.Exec(context.Background(), `DELETE FROM workspace_export WHERE workspace_id = $1`, ws) })
	svc := noticeService(f, nil, nil)
	if _, _, err := svc.RequestWorkspace(context.Background(), f.staff, ws, Input{Reason: "closing", Confirm: f.slug(ws), ExportFirst: true}); err != nil {
		t.Fatalf("RequestWorkspace: %v", err)
	}
	if n := f.count(`SELECT count(*) FROM workspace_export WHERE workspace_id = $1 AND status = 'queued'`, ws); n != 1 {
		t.Fatalf("queued exports = %d, want 1", n)
	}
	if n := f.count(`SELECT count(*) FROM platform_deletion_request WHERE target_id = $1 AND export_first`, ws); n != 1 {
		t.Fatalf("export_first on the request row = %d", n)
	}
}

// Spec R6 section 3c.9 rule 4: the account being deleted gets no notice of the
// workspace it takes down. A delete choice needs the account to be the sole owner,
// so the other members of that workspace are not owners and get no notice here.
func TestDeleteNowSendsNoticeToOwnersNotTheDeletedUser(t *testing.T) {
	af := newAccountFixture(t)
	ctx := context.Background()
	leaver, leaverEmail := af.user("leaver")
	_, memberEmail := af.user("member")
	ws := af.workspace(false)
	af.member(ws, leaver, "owner")
	memberID := af.scalar(`SELECT id::text FROM "user" WHERE email = $1`, memberEmail)
	af.member(ws, memberID, "member")

	mail := &fakeMail{}
	svc := New(Deps{
		Runner: NewPgxRunner(af.pool), Reader: NewPgxReader(af.pool), Cache: af.cache, Accounts: af.acc,
		Owners: NewPgxReader(af.pool), Mail: mail, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	_, _, err := svc.RequestAccount(ctx, af.staff, leaver, AccountInput{
		Reason: "legal request", Confirm: leaverEmail,
		Resolutions: []Resolution{{WorkspaceID: ws, Action: ActionDelete}},
	})
	if err != nil {
		t.Fatalf("RequestAccount: %v", err)
	}
	if len(mail.to) != 0 {
		t.Fatalf("notices sent to %v, want none: the deleted user is the only owner", mail.to)
	}
}

// Spec R6 section 3c.9 rule 5: a deletion that asks for an export while one is
// already queued or running reuses that export and queues no second one.
func TestExportFirstRefusedWhenExportAlreadyRunningReusesIt(t *testing.T) {
	f := newFixture(t)
	ws := f.workspace(false)
	ctx := context.Background()
	f.exec(`INSERT INTO workspace_export (id, workspace_id, requested_by, status) VALUES (gen_random_uuid(), $1, $2, 'running')`, ws, f.staff.ID)
	t.Cleanup(func() { f.pool.Exec(ctx, `DELETE FROM workspace_export WHERE workspace_id = $1`, ws) })
	svc := noticeService(f, fakeOwners{}, &fakeMail{})

	r, _, err := svc.RequestWorkspace(ctx, f.staff, ws, Input{Reason: "closing", Confirm: f.slug(ws), ExportFirst: true})
	if err != nil || r.State != StatePending {
		t.Fatalf("RequestWorkspace with a running export: state=%s err=%v", r.State, err)
	}
	if n := f.count(`SELECT count(*) FROM workspace_export WHERE workspace_id = $1`, ws); n != 1 {
		t.Fatalf("exports for the workspace = %d, want the running one reused", n)
	}
}
