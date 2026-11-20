package deletionrequest

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
)

// Database tests of phase 1 of an account deletion (spec R6 section 7.4). They
// run only when testdb.Open allows the test database.

type accountRecorder struct {
	mu        sync.Mutex
	cached    []string
	published []string
}

func (r *accountRecorder) Invalidate(_ context.Context, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cached = append(r.cached, userID)
	return nil
}

func (r *accountRecorder) Publish(_ context.Context, userID, kind string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.published = append(r.published, userID+":"+kind)
	return nil
}

// accountFixture wraps the workspace fixture with an account service and the
// cleanup of the account's own rows, which the workspace fixture does not know.
type accountFixture struct {
	*fixture
	acc *accountRecorder
	svc *Service
}

func newAccountFixture(t *testing.T) *accountFixture {
	t.Helper()
	f := newFixture(t)
	acc := &accountRecorder{}
	svc := New(Deps{
		Runner: NewPgxRunner(f.pool), Reader: NewPgxReader(f.pool), Cache: f.cache,
		Accounts: acc, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	af := &accountFixture{fixture: f, acc: acc, svc: svc}
	t.Cleanup(func() {
		ctx := context.Background()
		for _, u := range f.users {
			f.pool.Exec(ctx, `DELETE FROM platform_deletion_request WHERE target_id = $1`, u)
			f.pool.Exec(ctx, `DELETE FROM platform_account WHERE user_id = $1`, u)
			f.pool.Exec(ctx, `DELETE FROM audit_event WHERE target_id = $1`, u)
		}
	})
	return af
}

// member adds a user to a workspace with a role.
func (f *accountFixture) member(ws, user, role string) {
	f.exec(`INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, $3)`, ws, user, role)
}

func (f *accountFixture) accountStatus(user string) string {
	return f.scalar(`SELECT COALESCE((SELECT status FROM platform_account WHERE user_id = $1), 'active')`, user)
}

func TestAccountSoleOwnerUnresolvedWritesNothing(t *testing.T) {
	f := newAccountFixture(t)
	target, email := f.user("owner")
	other, _ := f.user("member")
	ws := f.workspace(false)
	f.member(ws, target, "owner")
	f.member(ws, other, "member")

	_, _, err := f.svc.RequestAccount(context.Background(), f.staff, target, AccountInput{Reason: "legal request", Confirm: email})
	de := asError(t, err)
	if de.Status != 409 || de.Code != CodeSoleOwnerUnresolved {
		t.Fatalf("got %d %s, want 409 sole_owner_unresolved", de.Status, de.Code)
	}
	if n := f.requestsFor(target); n != 0 {
		t.Fatalf("requests written = %d, want 0", n)
	}
	if f.accountStatus(target) != "active" || len(f.acc.cached) != 0 {
		t.Fatalf("account changed on a refused request")
	}
}

func TestAccountCoOwnerNeedsNoChoice(t *testing.T) {
	f := newAccountFixture(t)
	target, email := f.user("coowner")
	other, _ := f.user("coowner-two")
	ws := f.workspace(false)
	f.member(ws, target, "owner")
	f.member(ws, other, "owner")

	r, created, err := f.svc.RequestAccount(context.Background(), f.staff, target, AccountInput{Reason: "legal request", Confirm: email})
	if err != nil || !created {
		t.Fatalf("RequestAccount: created=%v err=%v", created, err)
	}
	if r.Kind != KindAccount || r.State != StatePending {
		t.Fatalf("request = %+v", r)
	}
}

func TestAccountRefusesStaffAndSelf(t *testing.T) {
	f := newAccountFixture(t)
	ctx := context.Background()
	target, email := f.user("staffed")
	f.exec(`INSERT INTO platform_staff (user_id, role) VALUES ($1, 'admin')`, target)

	_, _, err := f.svc.RequestAccount(ctx, f.staff, target, AccountInput{Reason: "r", Confirm: email})
	if de := asError(t, err); de.Code != CodeUserStaff || de.Status != 409 {
		t.Fatalf("staff target: got %d %s", de.Status, de.Code)
	}
	_, _, err = f.svc.RequestAccount(ctx, Actor{ID: f.staff.ID, Label: f.staff.Label}, f.staff.ID, AccountInput{Reason: "r", Confirm: f.staff.Label})
	if de := asError(t, err); de.Code != CodeCannotDeleteSelf || de.Status != 409 {
		t.Fatalf("self target: got %d %s", de.Status, de.Code)
	}
}

func TestAccountPhaseOneWritesRequestChildAndStatus(t *testing.T) {
	f := newAccountFixture(t)
	ctx := context.Background()
	target, email := f.user("leaver")
	successor, _ := f.user("successor")
	ws1 := f.workspace(false)
	ws2 := f.workspace(false)
	f.member(ws1, target, "owner")
	f.member(ws1, successor, "member")
	f.member(ws2, target, "owner")
	other, _ := f.user("other")
	f.member(ws2, other, "member")

	r, created, err := f.svc.RequestAccount(ctx, f.staff, target, AccountInput{
		Reason: "legal request", Confirm: email,
		Resolutions: []Resolution{
			{WorkspaceID: ws1, Action: ActionTransfer, SuccessorUserID: successor},
			{WorkspaceID: ws2, Action: ActionDelete},
		},
	})
	if err != nil || !created {
		t.Fatalf("RequestAccount: created=%v err=%v", created, err)
	}
	if f.accountStatus(target) != "pending_deletion" {
		t.Fatalf("account status = %q", f.accountStatus(target))
	}
	if n := f.count(`SELECT count(*) FROM platform_account WHERE user_id = $1 AND deletion_scheduled_at IS NOT NULL AND sessions_valid_after IS NOT NULL`, target); n != 1 {
		t.Fatalf("account row not due now with a session mark")
	}
	if f.status(ws2) != "pending_deletion" {
		t.Fatalf("delete-choice workspace status = %q", f.status(ws2))
	}
	if f.status(ws1) != "active" {
		t.Fatalf("transfer-choice workspace must not be touched in phase 1, status = %q", f.status(ws1))
	}
	child := f.count(`SELECT count(*) FROM platform_deletion_request WHERE parent_id = $1 AND kind = 'workspace' AND target_id = $2`, r.ID, ws2)
	if child != 1 {
		t.Fatalf("child workspace requests = %d, want 1", child)
	}
	if n := f.count(`SELECT count(*) FROM audit_event WHERE target_id = $1 AND action = 'account.immediate_deletion_requested' AND actor_type = 'staff'`, target); n != 1 {
		t.Fatalf("account request audit rows = %d", n)
	}
	if detail := f.scalar(`SELECT detail::text FROM audit_event WHERE target_id = $1 AND action = 'account.immediate_deletion_requested'`, target); strings.Contains(detail, email) {
		t.Fatalf("audit detail carries the email: %s", detail)
	}
	if len(f.acc.cached) != 1 || f.acc.cached[0] != target || len(f.acc.published) != 1 {
		t.Fatalf("cache=%v published=%v", f.acc.cached, f.acc.published)
	}
}

func TestAccountPreviewWritesNothing(t *testing.T) {
	f := newAccountFixture(t)
	target, _ := f.user("preview")
	other, _ := f.user("preview-member")
	ws := f.workspace(false)
	f.member(ws, target, "owner")
	f.member(ws, other, "member")

	p, err := f.svc.PreviewAccount(context.Background(), f.staff, target)
	if err != nil {
		t.Fatalf("PreviewAccount: %v", err)
	}
	if len(p.NeedChoice) != 1 || p.NeedChoice[0].ID != ws {
		t.Fatalf("need choice = %+v", p.NeedChoice)
	}
	if n := f.requestsFor(target); n != 0 || f.accountStatus(target) != "active" {
		t.Fatalf("preview wrote rows")
	}
}

// The preview lists, for each workspace that needs a choice, the members who
// may take over sole ownership: admins and members with a live account, never
// the account itself or a viewer (spec R6 section 7.3, GĐ11).
func TestPreviewListsSuccessorCandidatesForSoleOwner(t *testing.T) {
	f := newAccountFixture(t)
	target, _ := f.user("owner")
	admin, adminEmail := f.user("admin")
	member, memberEmail := f.user("member")
	viewer, _ := f.user("viewer")
	ws := f.workspace(false)
	f.member(ws, target, "owner")
	f.member(ws, admin, "admin")
	f.member(ws, member, "member")
	f.member(ws, viewer, "viewer")

	out, err := f.svc.PreviewAccount(context.Background(), f.staff, target)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.NeedChoice) != 1 {
		t.Fatalf("need_choice = %d, want 1", len(out.NeedChoice))
	}
	got := map[string]string{}
	for _, s := range out.NeedChoice[0].Successors {
		got[s.UserID] = s.Email
	}
	if len(got) != 2 || got[admin] != adminEmail || got[member] != memberEmail {
		t.Fatalf("successors = %v, want the admin and the member only", got)
	}
	if _, ok := got[target]; ok {
		t.Fatal("the account being deleted listed itself as a successor")
	}
	if _, ok := got[viewer]; ok {
		t.Fatal("a viewer was listed as a successor")
	}
}

// orderTx records the order of the phase-1 writes that matter for this test: the
// audit row, the request row and the account schedule.
type orderTx struct {
	Tx
	log *[]string
}

func (o orderTx) InsertAuditEvent(ctx context.Context, e audit.Entry) error {
	*o.log = append(*o.log, "audit")
	return o.Tx.InsertAuditEvent(ctx, e)
}

func (o orderTx) Insert(ctx context.Context, n NewRequest) (Request, bool, error) {
	*o.log = append(*o.log, "request")
	return o.Tx.Insert(ctx, n)
}

func (o orderTx) ScheduleAccount(ctx context.Context, userID, reason string, by Actor, at time.Time) error {
	*o.log = append(*o.log, "schedule")
	return o.Tx.ScheduleAccount(ctx, userID, reason, by, at)
}

type orderRunner struct {
	Runner
	log *[]string
}

func (o orderRunner) InTx(ctx context.Context, fn func(Tx) error) error {
	return o.Runner.InTx(ctx, func(tx Tx) error { return fn(orderTx{Tx: tx, log: o.log}) })
}

// Spec R6 section 7.4: the audit row of an account phase 1 is written in the same
// transaction before the account is scheduled, so a refused or rolled back phase 1
// leaves no schedule without its audit row.
func TestUserDeleteNowAuditIsFirstAndHasNoEmailInDetail(t *testing.T) {
	f := newAccountFixture(t)
	ctx := context.Background()
	target, email := f.user("leaver")
	ws := f.workspace(false)
	f.member(ws, target, "member")

	var order []string
	svc := f.svc
	svc.Runner = orderRunner{Runner: svc.Runner, log: &order}
	if _, _, err := svc.RequestAccount(ctx, f.staff, target, AccountInput{Reason: "legal request", Confirm: email}); err != nil {
		t.Fatalf("RequestAccount: %v", err)
	}
	auditAt, scheduleAt := -1, -1
	for i, step := range order {
		switch step {
		case "audit":
			if auditAt < 0 {
				auditAt = i
			}
		case "schedule":
			scheduleAt = i
		}
	}
	if auditAt < 0 || scheduleAt < 0 || auditAt > scheduleAt {
		t.Fatalf("write order = %v, want the audit row before the account schedule", order)
	}
	if n := f.count(`SELECT count(*) FROM audit_event WHERE target_id = $1 AND action = 'account.immediate_deletion_requested' AND detail::text LIKE '%' || $2 || '%'`, target, email); n != 0 {
		t.Fatalf("audit detail carries the email")
	}
}

// Spec R6 section 7.4: an account the user already scheduled for self-service
// deletion is brought forward to now by a platform delete-now.
func TestUserDeleteNowExpeditesSelfServiceSchedule(t *testing.T) {
	f := newAccountFixture(t)
	ctx := context.Background()
	target, email := f.user("leaver")
	ws := f.workspace(false)
	f.member(ws, target, "member")
	f.exec(`INSERT INTO platform_account (user_id, status, reason, deletion_scheduled_at)
	        VALUES ($1, 'pending_deletion', 'self service', now() + interval '14 days')`, target)

	if _, _, err := f.svc.RequestAccount(ctx, f.staff, target, AccountInput{Reason: "now", Confirm: email}); err != nil {
		t.Fatalf("RequestAccount: %v", err)
	}
	if n := f.count(`SELECT count(*) FROM platform_account WHERE user_id = $1 AND deletion_scheduled_at <= now() + interval '5 seconds'`, target); n != 1 {
		t.Fatalf("self-service schedule was not brought forward to now")
	}
}
