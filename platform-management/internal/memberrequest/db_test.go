package memberrequest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/testdb"
)

// Database tests: skipped unless PLATFORM_TEST_DATABASE_URL names a *_test
// database and DATABASE_URL equals it. Each test makes its own users and
// workspace and removes them with the rows the service wrote.

type fixture struct {
	t      *testing.T
	pool   *pgxpool.Pool
	svc    *Service
	seats  *recordingSeats
	users  []string
	wss    []string
	tenant string
	staff  Actor
}

type recordingSeats struct {
	mu    sync.Mutex
	calls [][2]string
	err   error
}

func (r *recordingSeats) Release(_ context.Context, ws, token string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, [2]string{ws, token})
	return r.err
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool := testdb.Open(t)
	f := &fixture{t: t, pool: pool, seats: &recordingSeats{}}
	f.svc = New(Deps{
		Runner: NewPgxRunner(pool), Reader: NewPgxReader(pool), Seats: f.seats,
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
		f.pool.Exec(ctx, `DELETE FROM platform_member_request WHERE workspace_id = $1`, ws)
		f.pool.Exec(ctx, `DELETE FROM audit_event WHERE workspace_id = $1`, ws)
		f.pool.Exec(ctx, `DELETE FROM platform_workspace WHERE workspace_id = $1`, ws)
		f.pool.Exec(ctx, `DELETE FROM tenant_workspace WHERE workspace_id = $1`, ws)
		f.pool.Exec(ctx, `DELETE FROM member WHERE workspace_id = $1`, ws)
		f.pool.Exec(ctx, `DELETE FROM workspace WHERE id = $1`, ws)
	}
	if f.tenant != "" {
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

func (f *fixture) user(prefix string) (id, email string) {
	f.t.Helper()
	var suffix string
	if err := f.pool.QueryRow(context.Background(), `SELECT substr(md5(random()::text), 1, 12)`).Scan(&suffix); err != nil {
		f.t.Fatal(err)
	}
	email = prefix + "-" + suffix + "@example.test"
	if err := f.pool.QueryRow(context.Background(),
		`INSERT INTO "user" (name, email) VALUES ('Member Request Test', $1) RETURNING id::text`, email).Scan(&id); err != nil {
		f.t.Fatalf("create user: %v", err)
	}
	f.users = append(f.users, id)
	return id, email
}

// workspace creates a workspace in a tenant (made on first use).
func (f *fixture) workspace() string {
	f.t.Helper()
	if f.tenant == "" {
		if err := f.pool.QueryRow(context.Background(),
			`INSERT INTO tenant (id, name) VALUES (gen_random_uuid(), 'Member request test tenant') RETURNING id::text`).Scan(&f.tenant); err != nil {
			f.t.Fatal(err)
		}
	}
	var id string
	if err := f.pool.QueryRow(context.Background(),
		`INSERT INTO workspace (name, slug) VALUES ('Member Request Test', 'mr-' || substr(md5(random()::text), 1, 12)) RETURNING id::text`).Scan(&id); err != nil {
		f.t.Fatalf("create workspace: %v", err)
	}
	f.wss = append(f.wss, id)
	f.exec(`INSERT INTO tenant_workspace (workspace_id, tenant_id) VALUES ($1, $2)`, id, f.tenant)
	return id
}

func (f *fixture) setStatus(ws, status string) {
	f.t.Helper()
	f.exec(`INSERT INTO platform_workspace (workspace_id, status, reason) VALUES ($1, $2, 'member request test')`, ws, status)
}

func (f *fixture) auditRows(ws, action string) int {
	f.t.Helper()
	return f.count(`SELECT count(*) FROM audit_event WHERE workspace_id = $1 AND action = $2`, ws, action)
}

func wantError(t *testing.T, err error, status int, code string) *Error {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) || e.Status != status || e.Code != code {
		t.Fatalf("error = %v, want %d %s", err, status, code)
	}
	return e
}

func TestMemberRequestCreateWritesPendingRowAndAudit(t *testing.T) {
	f := newFixture(t)
	ws := f.workspace()
	r, created, err := f.svc.Create(context.Background(), f.staff, ws, Input{Email: "New.Person@Example.test", Role: "member"})
	if err != nil || !created {
		t.Fatalf("Create = %+v, %v, %v", r, created, err)
	}
	if r.State != StatePending || r.Kind != KindAdd || r.Email != "new.person@example.test" || r.Role != "member" ||
		r.WorkspaceID != ws || r.RequestedBy != f.staff.ID || r.RequestedByLabel != f.staff.Label || r.Attempts != 0 {
		t.Fatalf("request = %+v", r)
	}
	var actorType, actorLabel, tenant, target, label, role string
	err = f.pool.QueryRow(context.Background(), `
		SELECT actor_type, actor_label, COALESCE(tenant_id::text, ''), target_id, target_label, detail->>'role'
		FROM audit_event WHERE workspace_id = $1 AND action = $2`, ws, audit.ActionMemberStaffAddRequested).
		Scan(&actorType, &actorLabel, &tenant, &target, &label, &role)
	if err != nil {
		t.Fatalf("audit row: %v", err)
	}
	if actorType != "staff" || actorLabel != f.staff.Label || tenant != f.tenant || target != r.ID || label != r.Email || role != "member" {
		t.Errorf("audit row = %s %s %s %s %s %s", actorType, actorLabel, tenant, target, label, role)
	}
}

func TestMemberRequestStaffEmailIsRefused(t *testing.T) {
	f := newFixture(t)
	ws := f.workspace()
	staffID, staffEmail := f.user("colleague")
	f.exec(`INSERT INTO platform_staff (user_id, role) VALUES ($1, 'support')`, staffID)

	_, _, err := f.svc.Create(context.Background(), f.staff, ws, Input{Email: staffEmail, Role: "member"})
	wantError(t, err, 409, CodeStaffCannotBeAdded)
	// The comparison ignores case.
	_, _, err = f.svc.Create(context.Background(), f.staff, ws, Input{Email: " " + upper(staffEmail), Role: "member"})
	wantError(t, err, 409, CodeStaffCannotBeAdded)
	if n := f.count(`SELECT count(*) FROM platform_member_request WHERE workspace_id = $1`, ws); n != 0 {
		t.Fatalf("a refused request left %d rows", n)
	}
	// A revoked staff member is an ordinary person again.
	f.exec(`UPDATE platform_staff SET revoked_at = now() WHERE user_id = $1`, staffID)
	if _, created, err := f.svc.Create(context.Background(), f.staff, ws, Input{Email: staffEmail, Role: "member"}); err != nil || !created {
		t.Fatalf("revoked staff = %v, %v, want a request", created, err)
	}
}

func upper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - 32
		}
	}
	return string(b)
}

func TestMemberRequestWorkspaceMustBeActive(t *testing.T) {
	for _, status := range []string{"read_only", "suspended", "pending_deletion"} {
		t.Run(status, func(t *testing.T) {
			f := newFixture(t)
			ws := f.workspace()
			f.setStatus(ws, status)
			_, _, err := f.svc.Create(context.Background(), f.staff, ws, Input{Email: "x@example.test", Role: "member"})
			e := wantError(t, err, 409, CodeWorkspaceNotActive)
			if e.State != status {
				t.Errorf("status in the error = %q, want %q", e.State, status)
			}
			if n := f.count(`SELECT count(*) FROM platform_member_request WHERE workspace_id = $1`, ws); n != 0 {
				t.Errorf("rows = %d, want none", n)
			}
		})
	}
	t.Run("active row", func(t *testing.T) {
		f := newFixture(t)
		ws := f.workspace()
		f.setStatus(ws, "active")
		if _, created, err := f.svc.Create(context.Background(), f.staff, ws, Input{Email: "x@example.test", Role: "member"}); err != nil || !created {
			t.Fatalf("active workspace: %v %v", created, err)
		}
	})
	t.Run("missing workspace", func(t *testing.T) {
		f := newFixture(t)
		_, _, err := f.svc.Create(context.Background(), f.staff, "99999999-0000-4000-8000-0000000000aa", Input{Email: "x@example.test", Role: "member"})
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("error = %v, want ErrNotFound", err)
		}
	})
}

func TestMemberRequestAlreadyMember(t *testing.T) {
	f := newFixture(t)
	ws := f.workspace()
	memberID, memberEmail := f.user("member")
	f.exec(`INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'member')`, ws, memberID)
	_, _, err := f.svc.Create(context.Background(), f.staff, ws, Input{Email: upper(memberEmail), Role: "admin"})
	wantError(t, err, 409, CodeAlreadyMember)
	if n := f.count(`SELECT count(*) FROM platform_member_request WHERE workspace_id = $1`, ws); n != 0 {
		t.Fatalf("rows = %d", n)
	}
}

func TestMemberRequestResendReturnsLiveRequest(t *testing.T) {
	f := newFixture(t)
	ws := f.workspace()
	first, created, err := f.svc.Create(context.Background(), f.staff, ws, Input{Email: "again@example.test", Role: "member"})
	if err != nil || !created {
		t.Fatal(created, err)
	}
	again, created, err := f.svc.Create(context.Background(), f.staff, ws, Input{Email: "AGAIN@example.test", Role: "viewer"})
	if err != nil || created {
		t.Fatalf("second Create created=%v err=%v, want the live request and created=false", created, err)
	}
	if again.ID != first.ID || again.Role != "member" {
		t.Errorf("second request = %+v, want the first (%s)", again, first.ID)
	}
	if n := f.auditRows(ws, audit.ActionMemberStaffAddRequested); n != 1 {
		t.Errorf("audit rows = %d, want 1: a repeat writes none", n)
	}
	// A different workspace is a different request.
	other := f.workspace()
	if _, created, err := f.svc.Create(context.Background(), f.staff, other, Input{Email: "again@example.test", Role: "member"}); err != nil || !created {
		t.Errorf("same email, other workspace: %v %v", created, err)
	}
	// Once closed, the email can be requested again.
	if _, err := f.svc.Cancel(context.Background(), f.staff, first.ID); err != nil {
		t.Fatal(err)
	}
	if second, created, err := f.svc.Create(context.Background(), f.staff, ws, Input{Email: "again@example.test", Role: "member"}); err != nil || !created || second.ID == first.ID {
		t.Errorf("after cancel: %+v %v %v", second, created, err)
	}
}

func TestMemberRequestConcurrentSameEmailWritesOneRow(t *testing.T) {
	f := newFixture(t)
	ws := f.workspace()
	const n = 8
	var wg sync.WaitGroup
	ids := make([]string, n)
	created := make([]bool, n)
	errs := make([]error, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			r, c, err := f.svc.Create(context.Background(), f.staff, ws, Input{Email: "race@example.test", Role: "member"})
			ids[i], created[i], errs[i] = r.ID, c, err
		}()
	}
	close(start)
	wg.Wait()
	made := 0
	for i := range ids {
		if errs[i] != nil {
			t.Fatalf("call %d: %v", i, errs[i])
		}
		if created[i] {
			made++
		}
		if ids[i] != ids[0] {
			t.Errorf("call %d answered request %s, want %s", i, ids[i], ids[0])
		}
	}
	if made != 1 {
		t.Errorf("%d calls created a request, want exactly 1", made)
	}
	if rows := f.count(`SELECT count(*) FROM platform_member_request WHERE workspace_id = $1`, ws); rows != 1 {
		t.Errorf("rows = %d, want 1", rows)
	}
	if rows := f.auditRows(ws, audit.ActionMemberStaffAddRequested); rows != 1 {
		t.Errorf("audit rows = %d, want 1", rows)
	}
}

// failingAuditRunner runs the real transaction with an audit writer that fails,
// to show the row does not outlive its audit row (I7).
type failingAuditRunner struct{ Runner }

func (r failingAuditRunner) InTx(ctx context.Context, fn func(Tx) error) error {
	return r.Runner.InTx(ctx, func(tx Tx) error { return fn(failingAuditTx{tx}) })
}

type failingAuditTx struct{ Tx }

func (failingAuditTx) InsertAuditEvent(context.Context, audit.Entry) error {
	return errors.New("audit insert failed")
}

func TestMemberRequestAuditAndRowShareOneTransaction(t *testing.T) {
	f := newFixture(t)
	ws := f.workspace()
	svc := New(Deps{Runner: failingAuditRunner{NewPgxRunner(f.pool)}, Reader: NewPgxReader(f.pool), Seats: f.seats})
	if _, _, err := svc.Create(context.Background(), f.staff, ws, Input{Email: "norow@example.test", Role: "member"}); err == nil {
		t.Fatal("Create succeeded with a failing audit writer")
	}
	if n := f.count(`SELECT count(*) FROM platform_member_request WHERE workspace_id = $1`, ws); n != 0 {
		t.Fatalf("the row survived the audit failure: %d", n)
	}

	// The same for cancel: the request stays pending.
	r, _, err := f.svc.Create(context.Background(), f.staff, ws, Input{Email: "stay@example.test", Role: "member"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Cancel(context.Background(), f.staff, r.ID); err == nil {
		t.Fatal("Cancel succeeded with a failing audit writer")
	}
	if got, _, _ := f.svc.Get(context.Background(), r.ID); got.State != StatePending {
		t.Errorf("state = %s after a failed cancel, want pending", got.State)
	}
	if len(f.seats.calls) != 0 {
		t.Errorf("a failed cancel released a seat: %v", f.seats.calls)
	}
}

func TestMemberRequestCancelReleasesSeatAndAudits(t *testing.T) {
	f := newFixture(t)
	ws := f.workspace()
	r, _, err := f.svc.Create(context.Background(), f.staff, ws, Input{Email: "cancel@example.test", Role: "member"})
	if err != nil {
		t.Fatal(err)
	}
	other := Actor{ID: f.staff.ID, Label: "other staff label"}
	got, err := f.svc.Cancel(context.Background(), other, r.ID)
	if err != nil || got.State != StateCanceled {
		t.Fatalf("Cancel = %+v, %v", got, err)
	}
	if len(f.seats.calls) != 1 || f.seats.calls[0] != [2]string{ws, r.ID} {
		t.Errorf("seat releases = %v, want one for (workspace, request id as the token)", f.seats.calls)
	}
	var actorType, label, tenant string
	if err := f.pool.QueryRow(context.Background(), `SELECT actor_type, actor_label, COALESCE(tenant_id::text, '') FROM audit_event WHERE workspace_id = $1 AND action = $2`,
		ws, audit.ActionMemberStaffAddCanceled).Scan(&actorType, &label, &tenant); err != nil {
		t.Fatalf("cancel audit row: %v", err)
	}
	if actorType != "staff" || label != "other staff label" || tenant != f.tenant {
		t.Errorf("cancel audit row = %s %s %s", actorType, label, tenant)
	}

	e := wantErrorOf(t, f, other, r.ID)
	if e.State != StateCanceled {
		t.Errorf("state in the refusal = %q, want canceled", e.State)
	}
	if len(f.seats.calls) != 1 {
		t.Errorf("a refused cancel released a seat again: %v", f.seats.calls)
	}
	if _, err := f.svc.Cancel(context.Background(), other, "99999999-0000-4000-8000-0000000000bb"); !errors.Is(err, ErrNotFound) {
		t.Errorf("cancel of a missing request = %v, want ErrNotFound", err)
	}
	// A request Delegora has finished is not cancelable.
	done, _, _ := f.svc.Create(context.Background(), f.staff, ws, Input{Email: "done@example.test", Role: "member"})
	f.exec(`UPDATE platform_member_request SET state = 'completed' WHERE id = $1`, done.ID)
	if _, err := f.svc.Cancel(context.Background(), other, done.ID); err == nil {
		t.Error("a completed request was canceled")
	} else {
		wantError(t, err, 409, CodeNotCancelable)
	}
}

func wantErrorOf(t *testing.T, f *fixture, by Actor, id string) *Error {
	t.Helper()
	_, err := f.svc.Cancel(context.Background(), by, id)
	return wantError(t, err, 409, CodeNotCancelable)
}

// A seat that cannot be released does not undo the cancel: the request is
// canceled and the recovery worker retires the claim.
func TestMemberRequestCancelSurvivesASeatReleaseFailure(t *testing.T) {
	f := newFixture(t)
	ws := f.workspace()
	f.seats.err = errors.New("seat service down")
	r, _, err := f.svc.Create(context.Background(), f.staff, ws, Input{Email: "seatdown@example.test", Role: "member"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.svc.Cancel(context.Background(), f.staff, r.ID)
	if err != nil || got.State != StateCanceled {
		t.Fatalf("Cancel = %+v, %v", got, err)
	}
}

func TestMemberRequestListAndGet(t *testing.T) {
	f := newFixture(t)
	ws := f.workspace()
	a, _, _ := f.svc.Create(context.Background(), f.staff, ws, Input{Email: "a@example.test", Role: "member"})
	b, _, _ := f.svc.Create(context.Background(), f.staff, ws, Input{Email: "b@example.test", Role: "viewer"})
	list, err := f.svc.List(context.Background(), ws)
	if err != nil || len(list) != 2 || list[0].ID != b.ID || list[1].ID != a.ID {
		t.Fatalf("List = %+v, %v, want newest first", list, err)
	}
	if got, found, err := f.svc.Get(context.Background(), a.ID); err != nil || !found || got.Email != "a@example.test" {
		t.Errorf("Get = %+v %v %v", got, found, err)
	}
	if _, found, err := f.svc.Get(context.Background(), "not-a-uuid"); err != nil || found {
		t.Errorf("Get of a bad id = %v %v, want not found", found, err)
	}
}
