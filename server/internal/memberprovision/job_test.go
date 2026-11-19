package memberprovision

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
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/seatcapacity"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

var testPool *pgxpool.Pool

// requireTestDatabaseURL refuses any database whose name does not end in _test
// (same rule as scripts/require-test-database.sh).
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
		fmt.Fprintln(os.Stderr, "memberprovision tests:", err)
		os.Exit(1)
	}
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "memberprovision tests: connect:", err)
		os.Exit(1)
	}
	if err := pool.Ping(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "memberprovision tests: ping:", err)
		os.Exit(1)
	}
	testPool = pool
	code := m.Run()
	pool.Close()
	os.Exit(code)
}

// ---- fake seat protocol ----------------------------------------------------

type seatCall struct {
	Method    string
	Workspace uuid.UUID
	Token     uuid.UUID
	Member    uuid.UUID
}

// fakeSeats keeps the stateful rules of spec Appendix B that this job relies
// on: Claim is idempotent by token and refuses when no seat is free; Confirm
// turns the hold into a used seat; Release is idempotent and answers "released"
// for a token it does not hold.
type fakeSeats struct {
	mu        sync.Mutex
	unmanaged bool
	purchased int
	used      int
	ops       map[uuid.UUID]string
	calls     []seatCall

	claimErr   error
	confirmErr error
	onConfirm  func(call seatCall)
}

func newSeats(purchased int) *fakeSeats {
	return &fakeSeats{purchased: purchased, ops: map[uuid.UUID]string{}}
}

func (f *fakeSeats) RecoveryAvailable() bool { return true }

func (f *fakeSeats) reserved() int {
	n := 0
	for _, s := range f.ops {
		if s == "consuming" || s == "reserved" {
			n++
		}
	}
	return n
}

func (f *fakeSeats) ClaimShareJoin(_ context.Context, ws, token uuid.UUID) (seatcapacity.Decision, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, seatCall{Method: "claim", Workspace: ws, Token: token})
	if f.claimErr != nil {
		return seatcapacity.Decision{}, f.claimErr
	}
	if f.unmanaged {
		return seatcapacity.Decision{}, nil
	}
	if st, ok := f.ops[token]; ok {
		return seatcapacity.Decision{Managed: true, Allowed: st != "released", Reason: map[bool]string{true: "", false: "released"}[st != "released"]}, nil
	}
	if f.purchased-f.used-f.reserved() <= 0 {
		return seatcapacity.Decision{Managed: true, Allowed: false, Reason: "capacity_full"}, nil
	}
	f.ops[token] = "consuming"
	return seatcapacity.Decision{Managed: true, Allowed: true}, nil
}

func (f *fakeSeats) Confirm(_ context.Context, ws, token, member uuid.UUID) (seatcapacity.Decision, error) {
	f.mu.Lock()
	call := seatCall{Method: "confirm", Workspace: ws, Token: token, Member: member}
	f.calls = append(f.calls, call)
	hook, err := f.onConfirm, f.confirmErr
	f.mu.Unlock()
	if hook != nil {
		hook(call)
	}
	if err != nil {
		return seatcapacity.Decision{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if st, ok := f.ops[token]; ok && st != "used" && st != "released" {
		f.ops[token] = "used"
		f.used++
	}
	return seatcapacity.Decision{Managed: true, Allowed: true}, nil
}

func (f *fakeSeats) Release(_ context.Context, ws, token uuid.UUID) (seatcapacity.Decision, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, seatCall{Method: "release", Workspace: ws, Token: token})
	if st, ok := f.ops[token]; ok && st != "released" {
		f.ops[token] = "released"
		return seatcapacity.Decision{Managed: true, Allowed: true}, nil
	}
	return seatcapacity.Decision{Managed: true, Allowed: false, Reason: "released"}, nil
}

func (f *fakeSeats) ReserveInvitation(context.Context, uuid.UUID, uuid.UUID, time.Time) (seatcapacity.Decision, error) {
	return seatcapacity.Decision{}, errors.New("unexpected ReserveInvitation")
}
func (f *fakeSeats) Consume(context.Context, uuid.UUID, uuid.UUID) (seatcapacity.Decision, error) {
	return seatcapacity.Decision{}, errors.New("unexpected Consume")
}
func (f *fakeSeats) ReleaseMember(context.Context, uuid.UUID, uuid.UUID) (seatcapacity.Decision, error) {
	return seatcapacity.Decision{}, errors.New("unexpected ReleaseMember")
}
func (f *fakeSeats) GetOperation(context.Context, uuid.UUID, uuid.UUID) (seatcapacity.Decision, error) {
	return seatcapacity.Decision{}, errors.New("unexpected GetOperation")
}

func (f *fakeSeats) methods() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.calls))
	for i, c := range f.calls {
		out[i] = c.Method
	}
	return out
}

func (f *fakeSeats) count(method string) int {
	n := 0
	for _, m := range f.methods() {
		if m == method {
			n++
		}
	}
	return n
}

// ---- fixtures --------------------------------------------------------------

type fakeBus struct {
	mu     sync.Mutex
	events []events.Event
}

func (b *fakeBus) Publish(e events.Event) {
	b.mu.Lock()
	b.events = append(b.events, e)
	b.mu.Unlock()
}

func token() string { return strings.ReplaceAll(uuid.NewString(), "-", "")[:12] }

type world struct {
	t     *testing.T
	f     *testutil.Fixture
	ws    string
	owner string
	staff string // the staff user who asked
	seats *fakeSeats
	bus   *fakeBus
	run   *Runner
	deps  Deps
}

func newWorld(t *testing.T) *world {
	t.Helper()
	f := testutil.New(testPool, "", "")
	w := &world{t: t, f: f, seats: newSeats(10), bus: &fakeBus{}}
	w.owner = f.User(t, "Owner", testutil.UniqueEmail("mp-owner"))
	w.staff = f.User(t, "Staff", testutil.UniqueEmail("mp-staff"))
	w.ws = f.Workspace(t, "Member provision "+token(), "mp-"+token())
	f.Member(t, w.ws, w.owner, "owner")
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = testPool.Exec(ctx, `DELETE FROM audit_event WHERE workspace_id = $1::uuid`, w.ws)
		_, _ = testPool.Exec(ctx, `DELETE FROM platform_member_request WHERE workspace_id = $1::uuid`, w.ws)
		_, _ = testPool.Exec(ctx, `DELETE FROM platform_workspace WHERE workspace_id = $1::uuid`, w.ws)
		_, _ = testPool.Exec(ctx, `DELETE FROM member WHERE workspace_id = $1::uuid`, w.ws)
	})
	w.deps = Deps{Pool: testPool, Queries: db.New(testPool), Seats: w.seats, Bus: w.bus}
	w.run = New(w.deps)
	return w
}

func (w *world) rebuild() { w.run = New(w.deps) }

// request writes a pending add request the way the platform does.
func (w *world) request(email, role string) uuid.UUID {
	w.t.Helper()
	id := uuid.New()
	if _, err := testPool.Exec(context.Background(), `
		INSERT INTO platform_member_request (id, workspace_id, kind, email, role, requested_by, requested_by_label)
		VALUES ($1, $2::uuid, 'add', $3, $4, $5::uuid, $6)`,
		id, w.ws, strings.ToLower(email), role, w.staff, "staff-label@example.test"); err != nil {
		w.t.Fatalf("insert request: %v", err)
	}
	w.t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM user_consent WHERE user_id IN (SELECT id FROM "user" WHERE lower(email) = lower($1))`, email)
		_, _ = testPool.Exec(context.Background(), `DELETE FROM member WHERE user_id IN (SELECT id FROM "user" WHERE lower(email) = lower($1) AND name <> 'Preexisting')`, email)
		_, _ = testPool.Exec(context.Background(), `DELETE FROM "user" WHERE lower(email) = lower($1) AND name <> 'Preexisting'`, email)
	})
	return id
}

func (w *world) email(prefix string) string { return prefix + "-" + token() + "@example.test" }

type reqRow struct {
	State       string
	ErrorCode   string
	Attempts    int
	UserID      pgtype.UUID
	MemberID    pgtype.UUID
	UserCreated bool
	Completed   bool
}

func (w *world) row(id uuid.UUID) reqRow {
	w.t.Helper()
	var r reqRow
	err := testPool.QueryRow(context.Background(),
		`SELECT state, error_code, attempts, user_id, member_id, user_created, completed_at IS NOT NULL FROM platform_member_request WHERE id = $1`, id).
		Scan(&r.State, &r.ErrorCode, &r.Attempts, &r.UserID, &r.MemberID, &r.UserCreated, &r.Completed)
	if err != nil {
		w.t.Fatalf("read request: %v", err)
	}
	return r
}

func count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func (w *world) members(email string) int {
	return count(w.t, `SELECT count(*) FROM member m JOIN "user" u ON u.id = m.user_id WHERE m.workspace_id = $1::uuid AND lower(u.email) = lower($2)`, w.ws, email)
}

func (w *world) users(email string) int {
	return count(w.t, `SELECT count(*) FROM "user" WHERE lower(email) = lower($1)`, email)
}

func (w *world) runOnce() int {
	w.t.Helper()
	n, err := w.run.RunOnce(context.Background())
	if err != nil {
		w.t.Fatalf("RunOnce: %v", err)
	}
	return n
}

func (w *world) wantCompleted(id uuid.UUID) reqRow {
	w.t.Helper()
	r := w.row(id)
	if r.State != "completed" || r.ErrorCode != "" || !r.Completed || !r.MemberID.Valid || !r.UserID.Valid {
		w.t.Fatalf("request = %+v, want completed with a user and a member", r)
	}
	return r
}

func (w *world) wantFailed(id uuid.UUID, code string) {
	w.t.Helper()
	if r := w.row(id); r.State != "failed" || r.ErrorCode != code {
		w.t.Fatalf("request = %+v, want failed %s", r, code)
	}
}

// preexistingUser makes a user the job must find, not create.
func (w *world) preexistingUser(email string) string {
	w.t.Helper()
	return w.f.User(w.t, "Preexisting", email)
}

func (w *world) roleOf(email string) string {
	w.t.Helper()
	var role string
	err := testPool.QueryRow(context.Background(),
		`SELECT m.role FROM member m JOIN "user" u ON u.id = m.user_id WHERE m.workspace_id = $1::uuid AND lower(u.email) = lower($2)`, w.ws, email).Scan(&role)
	if err != nil {
		w.t.Fatalf("role of %s: %v", email, err)
	}
	return role
}

// ---- user handling ---------------------------------------------------------

func TestMemberProvisioningUsesExistingUser(t *testing.T) {
	w := newWorld(t)
	email := w.email("exists")
	uid := w.preexistingUser(email)
	id := w.request(strings.ToUpper(email), "member")

	if n := w.runOnce(); n != 1 {
		t.Fatalf("RunOnce = %d, want 1", n)
	}
	r := w.wantCompleted(id)
	if r.UserCreated || uuid.UUID(r.UserID.Bytes).String() != uid {
		t.Errorf("request = %+v, want the existing user %s and user_created = false", r, uid)
	}
	if w.users(email) != 1 || w.members(email) != 1 {
		t.Errorf("users = %d, members = %d, want 1 and 1", w.users(email), w.members(email))
	}
	if count(t, `SELECT count(*) FROM "user" WHERE id = $1::uuid AND onboarded_at IS NOT NULL`, uid) != 1 {
		t.Errorf("the existing user was not marked onboarded")
	}
}

func TestMemberProvisioningCreatesOnboardedUserWithoutSessionPasswordOrToken(t *testing.T) {
	w := newWorld(t)
	email := w.email("Fresh.Person")
	id := w.request(email, "member")
	w.runOnce()
	r := w.wantCompleted(id)
	if !r.UserCreated {
		t.Errorf("user_created = false, want true")
	}
	var name string
	var onboarded bool
	if err := testPool.QueryRow(context.Background(), `SELECT name, onboarded_at IS NOT NULL FROM "user" WHERE id = $1`, r.UserID).Scan(&name, &onboarded); err != nil {
		t.Fatal(err)
	}
	if name != strings.Split(strings.ToLower(email), "@")[0] || !onboarded {
		t.Errorf("name = %q onboarded = %v, want the part before @ and onboarded", name, onboarded)
	}
	if n := count(t, `SELECT count(*) FROM personal_access_token WHERE user_id = $1`, r.UserID); n != 0 {
		t.Errorf("%d personal access tokens", n)
	}
	if n := count(t, `SELECT count(*) FROM verification_code WHERE lower(email) = lower($1)`, email); n != 0 {
		t.Errorf("%d login codes", n)
	}
}

func TestMemberProvisioningWorksWhenSignupIsDisabled(t *testing.T) {
	t.Setenv("ALLOW_SIGNUP", "false")
	w := newWorld(t)
	email := w.email("nosignup")
	id := w.request(email, "member")
	w.runOnce()
	w.wantCompleted(id)
	if w.members(email) != 1 {
		t.Errorf("members = %d, want 1", w.members(email))
	}
}

func TestMemberProvisioningBlockedDomainFails(t *testing.T) {
	w := newWorld(t)
	w.deps.BlockedEmailDomains = []string{"blocked.test"}
	w.rebuild()
	email := "nope-" + token() + "@blocked.test"
	id := w.request(email, "member")
	w.runOnce()
	w.wantFailed(id, "email_domain_blocked")
	if w.users(email) != 0 || w.members(email) != 0 {
		t.Errorf("a blocked request left %d users and %d members", w.users(email), w.members(email))
	}
	if len(w.seats.methods()) != 0 {
		t.Errorf("a blocked request reached the seat service: %v", w.seats.methods())
	}
	if n := count(t, `SELECT count(*) FROM audit_event WHERE workspace_id = $1::uuid AND action = $2 AND actor_type = 'system' AND detail->>'error_code' = 'email_domain_blocked'`,
		w.ws, audit.ActionMemberStaffAddFailed); n != 1 {
		t.Errorf("failure audit rows = %d, want 1 with actor system", n)
	}
}

func TestMemberProvisioningSuspendedOrPendingDeletionUserFails(t *testing.T) {
	for _, status := range []string{"suspended", "pending_deletion"} {
		t.Run(status, func(t *testing.T) {
			w := newWorld(t)
			email := w.email("acct-" + status)
			uid := w.preexistingUser(email)
			if _, err := testPool.Exec(context.Background(), `INSERT INTO platform_account (user_id, status, reason) VALUES ($1::uuid, $2, 'test')`, uid, status); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_, _ = testPool.Exec(context.Background(), `DELETE FROM platform_account WHERE user_id = $1::uuid`, uid)
			})
			id := w.request(email, "member")
			w.runOnce()
			w.wantFailed(id, "account_not_active")
			if w.members(email) != 0 {
				t.Errorf("a %s user was added", status)
			}
			if len(w.seats.methods()) != 0 {
				t.Errorf("seat calls = %v, want none before the account check", w.seats.methods())
			}
		})
	}
}

func TestMemberProvisioningWorkspaceMustStillBeActive(t *testing.T) {
	w := newWorld(t)
	if _, err := testPool.Exec(context.Background(), `INSERT INTO platform_workspace (workspace_id, status, reason) VALUES ($1::uuid, 'suspended', 'test')`, w.ws); err != nil {
		t.Fatal(err)
	}
	email := w.email("suspendedws")
	id := w.request(email, "member")
	w.runOnce()
	w.wantFailed(id, "workspace_not_active")
	if w.members(email) != 0 || w.users(email) != 0 {
		t.Errorf("a request on a suspended workspace made a user or member")
	}
}

func TestMemberProvisioningAlreadyMemberFails(t *testing.T) {
	w := newWorld(t)
	email := w.email("already")
	uid := w.preexistingUser(email)
	w.f.Member(t, w.ws, uid, "member")
	id := w.request(email, "admin")
	w.runOnce()
	w.wantFailed(id, "already_member")
	if w.roleOf(email) != "member" {
		t.Errorf("an existing member's role changed")
	}
}

func TestMemberProvisioningRolesApplied(t *testing.T) {
	for _, role := range []string{"admin", "member", "viewer"} {
		t.Run(role, func(t *testing.T) {
			w := newWorld(t)
			email := w.email("role-" + role)
			id := w.request(email, role)
			w.runOnce()
			w.wantCompleted(id)
			if got := w.roleOf(email); got != role {
				t.Errorf("role = %s, want %s", got, role)
			}
		})
	}
	t.Run("owner is refused", func(t *testing.T) {
		w := newWorld(t)
		email := w.email("role-owner")
		id := w.request(email, "owner")
		w.runOnce()
		w.wantFailed(id, "internal")
		if w.members(email) != 0 {
			t.Errorf("an owner request made a member")
		}
	})
}

// ---- seats -----------------------------------------------------------------

func TestMemberProvisioningSeatFullReleasesAndCreatesNoMember(t *testing.T) {
	w := newWorld(t)
	w.seats.purchased = 0
	email := w.email("full")
	id := w.request(email, "member")
	w.runOnce()
	w.wantFailed(id, "capacity_full")
	if w.members(email) != 0 || w.users(email) != 0 {
		t.Errorf("a full workspace got %d members and %d users", w.members(email), w.users(email))
	}
	if got := strings.Join(w.seats.methods(), ","); got != "claim,release" {
		t.Errorf("seat calls = %s, want claim,release", got)
	}
	if c := w.seats.calls[0]; c.Token != id || c.Workspace.String() != w.ws {
		t.Errorf("claim = %+v, want the request id as the token", c)
	}
}

func TestMemberProvisioningOvercommittedFails(t *testing.T) {
	w := newWorld(t)
	w.seats.claimErr = &seatcapacity.HTTPError{StatusCode: 409, Code: "capacity_overcommitted"}
	email := w.email("over")
	id := w.request(email, "member")
	w.runOnce()
	w.wantFailed(id, "capacity_overcommitted")
	if w.members(email) != 0 {
		t.Errorf("an overcommitted workspace got a member")
	}
}

func TestMemberProvisioningConfirmsSeatAfterCommit(t *testing.T) {
	w := newWorld(t)
	email := w.email("confirm")
	id := w.request(email, "member")
	var visible bool
	var seen seatCall
	w.seats.onConfirm = func(c seatCall) {
		seen = c
		visible = count(t, `SELECT count(*) FROM member WHERE id = $1::uuid`, c.Member) == 1 &&
			count(t, `SELECT count(*) FROM platform_member_request WHERE id = $1 AND state = 'completed'`, id) == 1
	}
	w.runOnce()
	r := w.wantCompleted(id)
	if !visible {
		t.Errorf("Confirm ran before the member and the completed state were committed")
	}
	if seen.Token != id || seen.Member != uuid.UUID(r.MemberID.Bytes) || seen.Workspace.String() != w.ws {
		t.Errorf("confirm = %+v, want token %s and member %s", seen, id, uuid.UUID(r.MemberID.Bytes))
	}
	if got := strings.Join(w.seats.methods(), ","); got != "claim,confirm" {
		t.Errorf("seat calls = %s, want claim,confirm", got)
	}
}

func TestMemberProvisioningUnmanagedWorkspaceSkipsSeat(t *testing.T) {
	w := newWorld(t)
	w.seats.unmanaged = true
	email := w.email("unmanaged")
	id := w.request(email, "member")
	w.runOnce()
	w.wantCompleted(id)
	if got := strings.Join(w.seats.methods(), ","); got != "claim" {
		t.Errorf("seat calls = %s, want only the claim: nothing to confirm or release", got)
	}
}

func TestMemberProvisioningNoSeatClientMeansUnmanaged(t *testing.T) {
	w := newWorld(t)
	w.deps.Seats = nil
	w.rebuild()
	email := w.email("noseats")
	id := w.request(email, "member")
	w.runOnce()
	w.wantCompleted(id)
}

func TestMemberProvisioningTransportFailureRetriesThenFailsAndReleases(t *testing.T) {
	w := newWorld(t)
	w.seats.claimErr = errors.New("connection refused")
	email := w.email("transport")
	id := w.request(email, "member")

	w.runOnce()
	if r := w.row(id); r.State != "pending" || r.Attempts != 1 {
		t.Fatalf("after the first failure = %+v, want pending with 1 attempt", r)
	}
	w.runOnce()
	if r := w.row(id); r.State != "pending" || r.Attempts != 2 {
		t.Fatalf("after the second failure = %+v, want pending with 2 attempts", r)
	}
	if w.seats.count("release") != 0 {
		t.Fatalf("a retryable failure released the seat: a retry would not find it")
	}
	w.runOnce()
	w.wantFailed(id, "seat_capacity_unavailable")
	if w.seats.count("release") != 1 {
		t.Errorf("seat calls = %v, want one release at the end", w.seats.methods())
	}
	if w.members(email) != 0 || w.users(email) != 0 {
		t.Errorf("a failed request left %d members, %d users", w.members(email), w.users(email))
	}
	// A failed request is not picked up again.
	w.runOnce()
	if r := w.row(id); r.Attempts != 3 {
		t.Errorf("attempts = %d after a fourth run, want 3", r.Attempts)
	}
}

func TestMemberProvisioningTwoRequestsForLastSeatOneWins(t *testing.T) {
	w := newWorld(t)
	w.seats.purchased = 1 // the one seat of the owner is not counted by the fake: one free seat
	first := w.request(w.email("last-a"), "member")
	second := w.request(w.email("last-b"), "member")
	w.runOnce()
	a, b := w.row(first), w.row(second)
	states := []string{a.State, b.State}
	if !(states[0] == "completed" && states[1] == "failed") && !(states[0] == "failed" && states[1] == "completed") {
		t.Fatalf("states = %v, want one completed and one failed", states)
	}
	loser := a
	if a.State == "completed" {
		loser = b
	}
	if loser.ErrorCode != "capacity_full" {
		t.Errorf("loser error = %q, want capacity_full", loser.ErrorCode)
	}
}

// ---- audit and events -------------------------------------------------------

func TestMemberProvisioningAuditActorIsStaffWithWorkspaceID(t *testing.T) {
	w := newWorld(t)
	tenant := uuid.NewString()
	for _, sql := range []string{
		`INSERT INTO tenant (id, name) VALUES ($1::uuid, 'member provision test')`,
	} {
		if _, err := testPool.Exec(context.Background(), sql, tenant); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := testPool.Exec(context.Background(), `INSERT INTO tenant_workspace (workspace_id, tenant_id) VALUES ($1::uuid, $2::uuid)`, w.ws, tenant); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM tenant_workspace WHERE workspace_id = $1::uuid`, w.ws)
		_, _ = testPool.Exec(context.Background(), `DELETE FROM tenant WHERE id = $1::uuid`, tenant)
	})
	email := w.email("audited")
	id := w.request(email, "viewer")
	w.runOnce()
	r := w.wantCompleted(id)

	var actorType, actorID, label, ws, tenantOnRow, targetType, targetID, role, reqID string
	var created bool
	err := testPool.QueryRow(context.Background(), `
		SELECT actor_type, COALESCE(actor_id, ''), actor_label, COALESCE(workspace_id::text, ''), COALESCE(tenant_id::text, ''),
		       target_type, target_id, detail->>'role', detail->>'request_id', (detail->>'user_created')::boolean
		FROM audit_event WHERE workspace_id = $1::uuid AND action = $2`, w.ws, audit.ActionMemberStaffAdded).
		Scan(&actorType, &actorID, &label, &ws, &tenantOnRow, &targetType, &targetID, &role, &reqID, &created)
	if err != nil {
		t.Fatalf("audit row: %v", err)
	}
	if actorType != "staff" || actorID != w.staff || label != "staff-label@example.test" || ws != w.ws || tenantOnRow != tenant {
		t.Errorf("audit actor = %s %s %s, workspace %s, tenant %s", actorType, actorID, label, ws, tenantOnRow)
	}
	if targetType != "member" || targetID != uuid.UUID(r.MemberID.Bytes).String() || role != "viewer" || reqID != id.String() || !created {
		t.Errorf("audit target = %s %s, detail role=%s request_id=%s user_created=%v", targetType, targetID, role, reqID, created)
	}
}

func TestMemberProvisioningAuditFailureRollsBackMemberAndReleasesSeat(t *testing.T) {
	w := newWorld(t)
	w.deps.Inject = func(stage string) error {
		if stage == "audit" {
			return errors.New("audit write failed")
		}
		return nil
	}
	w.rebuild()
	email := w.email("auditfail")
	id := w.request(email, "member")
	for i := 0; i < 3; i++ {
		w.runOnce()
	}
	w.wantFailed(id, "internal")
	if w.members(email) != 0 || w.users(email) != 0 {
		t.Errorf("the audit failure left %d members and %d users", w.members(email), w.users(email))
	}
	if w.seats.count("release") != 1 || w.seats.count("confirm") != 0 {
		t.Errorf("seat calls = %v, want a release and no confirm", w.seats.methods())
	}
}

func TestMemberProvisioningPublishesMemberAdded(t *testing.T) {
	w := newWorld(t)
	email := w.email("published")
	id := w.request(email, "admin")
	w.runOnce()
	r := w.wantCompleted(id)
	w.bus.mu.Lock()
	defer w.bus.mu.Unlock()
	if len(w.bus.events) != 1 {
		t.Fatalf("events = %d, want 1", len(w.bus.events))
	}
	e := w.bus.events[0]
	if e.Type != protocol.EventMemberAdded || e.WorkspaceID != w.ws {
		t.Errorf("event = %s in %s", e.Type, e.WorkspaceID)
	}
	payload, _ := e.Payload.(map[string]any)
	member, _ := payload["member"].(map[string]any)
	if member["user_id"] != uuid.UUID(r.UserID.Bytes).String() || member["role"] != "admin" || member["email"] != strings.ToLower(email) {
		t.Errorf("payload member = %v", payload["member"])
	}
	if payload["workspace_name"] == nil || payload["workspace_name"] == "" {
		t.Errorf("payload has no workspace_name: %v", payload)
	}
}

// A failed request publishes nothing.
func TestMemberProvisioningFailureDoesNotPublish(t *testing.T) {
	w := newWorld(t)
	w.seats.purchased = 0
	w.request(w.email("nopub"), "member")
	w.runOnce()
	if len(w.bus.events) != 0 {
		t.Errorf("events = %d, want none", len(w.bus.events))
	}
}

// ---- idempotence -----------------------------------------------------------

func TestMemberProvisioningTwiceAndParallelMakesOneMember(t *testing.T) {
	w := newWorld(t)
	email := w.email("twice")
	id := w.request(email, "member")
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = w.run.RunOnce(context.Background())
		}()
	}
	wg.Wait()
	w.runOnce()
	w.wantCompleted(id)
	if w.members(email) != 1 || w.users(email) != 1 {
		t.Errorf("members = %d, users = %d, want 1 and 1", w.members(email), w.users(email))
	}
	if n := count(t, `SELECT count(*) FROM audit_event WHERE workspace_id = $1::uuid AND action = $2`, w.ws, audit.ActionMemberStaffAdded); n != 1 {
		t.Errorf("audit rows = %d, want 1", n)
	}
	if w.seats.count("claim") != 1 || w.seats.count("confirm") != 1 {
		t.Errorf("seat calls = %v, want one claim and one confirm", w.seats.methods())
	}
}

// Request kinds this job does not handle stay untouched.
func TestMemberProvisioningLeavesOtherKindsAlone(t *testing.T) {
	w := newWorld(t)
	id := uuid.New()
	if _, err := testPool.Exec(context.Background(), `
		INSERT INTO platform_member_request (id, workspace_id, kind, email, role, requested_by)
		VALUES ($1, $2::uuid, 'remove', $3, 'member', $4::uuid)`, id, w.ws, w.email("remove"), w.staff); err != nil {
		t.Fatal(err)
	}
	if n := w.runOnce(); n != 0 {
		t.Errorf("RunOnce = %d, want 0", n)
	}
	if r := w.row(id); r.State != "pending" || r.Attempts != 0 {
		t.Errorf("a remove request changed: %+v", r)
	}
}
