package console

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/multica-ai/multica/platform-management/accountguard"
	"github.com/multica-ai/multica/platform-management/internal/accountcache"
	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/db"
	"github.com/multica-ai/multica/platform-management/internal/platformauth"
	"github.com/multica-ai/multica/platform-management/internal/session"
	"github.com/multica-ai/multica/platform-management/internal/staff"
	"github.com/multica-ai/multica/platform-management/internal/testdb"
	"github.com/multica-ai/multica/platform-management/internal/workspacestatus"
	"github.com/multica-ai/multica/platform-management/workspacestate"
)

// Database tests: skipped unless PLATFORM_TEST_DATABASE_URL names a *_test
// database and DATABASE_URL equals it. Each test makes its own users and
// workspace and removes them, with the rows the console wrote about them.
//
// They run the real Handler behind the real staff middleware, on the real
// store. Redis is replaced by an in-memory cache and a recording notifier.

type memCache struct {
	mu   sync.Mutex
	m    map[string][]byte
	dels []string
}

func (c *memCache) Get(_ context.Context, k string) ([]byte, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[k]
	return v, ok, nil
}

func (c *memCache) Set(_ context.Context, k string, v []byte, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[k] = v
	return nil
}

func (c *memCache) Del(_ context.Context, k string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, k)
	c.dels = append(c.dels, k)
	return nil
}

type dbFixture struct {
	t       *testing.T
	pool    *pgxpool.Pool
	h       http.Handler
	console *Handler
	auth    *platformauth.Handler
	deps    Deps
	cache   *memCache
	pub     *fakeNotifier

	userIDs   []string
	wsIDs     []string
	tenantIDs []string
}

func newDBFixture(t *testing.T) *dbFixture {
	t.Helper()
	pool := testdb.Open(t)
	seedCatalogue(t, pool)
	f := &dbFixture{t: t, pool: pool, cache: &memCache{m: map[string][]byte{}}, pub: &fakeNotifier{}}

	q := db.New(pool)
	store := staff.NewStore(q)
	guard := accountguard.New(store, f.cache)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	auth := platformauth.New(platformauth.Deps{
		Config:    platformauth.Config{SessionSecret: secret, ConsoleOrigin: origin},
		Directory: store,
		Guard:     guard,
		Log:       quiet,
	})
	payments, err := billing.New(billing.Deps{
		Runner: billing.NewPgxRunner(pool), Provider: billing.ProviderManual,
		Workspaces: workspacestate.New(nil, f.cache), Entitlements: noopEntitlements{}, Log: quiet,
	})
	if err != nil {
		t.Fatal(err)
	}
	f.auth = auth
	f.deps = Deps{
		RequireStaff:  auth.RequirePlatformStaff,
		RequireOrigin: auth.RequireOrigin,
		Reader:        NewPgxReader(pool),
		Runner:        NewPgxRunner(pool),
		Workspaces:    workspacestatus.New(workspacestatus.NewPgxRunner(pool), workspacestate.New(nil, f.cache), quiet),
		Cache:         guard,
		Notifier:      f.pub,
		Payments:      payments,
		Emails:        store,
		Log:           quiet,
	}
	f.console = New(f.deps)
	f.h = f.console
	t.Cleanup(f.cleanup)
	return f
}

func (f *dbFixture) cleanup() {
	ctx := context.Background()
	for _, ws := range f.wsIDs {
		f.pool.Exec(ctx, `DELETE FROM tenant_workspace WHERE workspace_id = $1`, ws)
		f.pool.Exec(ctx, `DELETE FROM audit_event WHERE workspace_id = $1`, ws)
		f.pool.Exec(ctx, `DELETE FROM platform_note WHERE workspace_id = $1`, ws)
		f.pool.Exec(ctx, `DELETE FROM platform_workspace WHERE workspace_id = $1`, ws)
		f.pool.Exec(ctx, `DELETE FROM member WHERE workspace_id = $1`, ws)
		f.pool.Exec(ctx, `DELETE FROM workspace WHERE id = $1`, ws)
	}
	for _, tn := range f.tenantIDs {
		f.pool.Exec(ctx, `DELETE FROM audit_event WHERE tenant_id = $1`, tn)
		f.pool.Exec(ctx, `DELETE FROM tenant_workspace WHERE tenant_id = $1`, tn)
		f.pool.Exec(ctx, `DELETE FROM tenant WHERE id = $1`, tn)
	}
	for _, u := range f.userIDs {
		f.pool.Exec(ctx, `DELETE FROM audit_event WHERE actor_id = $1 OR target_id = $1`, u)
		f.pool.Exec(ctx, `DELETE FROM platform_staff WHERE user_id = $1`, u)
		f.pool.Exec(ctx, `DELETE FROM platform_account WHERE user_id = $1`, u)
		f.pool.Exec(ctx, `DELETE FROM "user" WHERE id = $1`, u)
	}
}

func randomHex(t *testing.T) string {
	t.Helper()
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

func (f *dbFixture) user(prefix string) (id, email string) {
	f.t.Helper()
	email = prefix + "-" + randomHex(f.t) + "@example.test"
	err := f.pool.QueryRow(context.Background(),
		`INSERT INTO "user" (name, email) VALUES ($1, $2) RETURNING id::text`, "Console Test", email).Scan(&id)
	if err != nil {
		f.t.Fatalf("create user: %v", err)
	}
	f.userIDs = append(f.userIDs, id)
	return id, email
}

func (f *dbFixture) staffUser(role staff.Role) (id, email string) {
	f.t.Helper()
	id, email = f.user("staff")
	if _, err := f.pool.Exec(context.Background(),
		`INSERT INTO platform_staff (user_id, role) VALUES ($1, $2)`, id, string(role)); err != nil {
		f.t.Fatalf("create staff row: %v", err)
	}
	return id, email
}

// workspace creates a workspace owned by ownerID with one more member, in a
// tenant of its own keyed by the workspace id, as the server creates it.
func (f *dbFixture) workspace(ownerID, memberID string) string {
	f.t.Helper()
	id := f.workspaceIn("", ownerID, memberID)
	f.exec(`INSERT INTO tenant (id, name, created_by) VALUES ($1, 'Console Test tenant', $2)`, id, ownerID)
	f.exec(`INSERT INTO tenant_workspace (workspace_id, tenant_id) VALUES ($1, $1)`, id)
	f.tenantIDs = append(f.tenantIDs, id)
	return id
}

// workspaceIn creates a workspace owned by ownerID with one more member, linked
// to tenant when it is not empty.
func (f *dbFixture) workspaceIn(tenant, ownerID, memberID string) string {
	f.t.Helper()
	var id string
	suffix := randomHex(f.t)
	if err := f.pool.QueryRow(context.Background(),
		`INSERT INTO workspace (name, slug) VALUES ($1, $2) RETURNING id::text`,
		"Console Test "+suffix, "console-test-"+suffix).Scan(&id); err != nil {
		f.t.Fatalf("create workspace: %v", err)
	}
	f.wsIDs = append(f.wsIDs, id)
	if tenant != "" {
		f.exec(`INSERT INTO tenant_workspace (workspace_id, tenant_id) VALUES ($1, $2)`, id, tenant)
	}
	for user, role := range map[string]string{ownerID: "owner", memberID: "member"} {
		if _, err := f.pool.Exec(context.Background(),
			`INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, $3)`, id, user, role); err != nil {
			f.t.Fatalf("create member: %v", err)
		}
	}
	return id
}

func (f *dbFixture) do(user, method, path, body string) *httptest.ResponseRecorder {
	f.t.Helper()
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, f.request(user, method, path, body))
	return rec
}

// request builds a signed-in request for user. Write methods carry the
// console's Origin.
func (f *dbFixture) request(user, method, path, body string) *http.Request {
	f.t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rd)
	tok, err := platformauth.SignMFASession(secret, user, time.Now().Add(-time.Minute))
	if err != nil {
		f.t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: session.CookieName, Value: tok})
	if method != http.MethodGet {
		req.Header.Set("Origin", origin)
	}
	return req
}

func (f *dbFixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(context.Background(), sql, args...); err != nil {
		f.t.Fatalf("%s: %v", sql, err)
	}
}

func (f *dbFixture) count(query string, args ...any) int {
	f.t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

type auditRow struct{ actorType, actorID, actorLabel, workspaceID, targetID, detail string }

// staffRows returns the audit rows with the action and the actor.
func (f *dbFixture) staffRows(action, actorID string) []auditRow {
	f.t.Helper()
	rows, err := f.pool.Query(context.Background(),
		`SELECT actor_type, COALESCE(actor_id, ''), actor_label, COALESCE(workspace_id::text, ''), target_id, detail::text
		 FROM audit_event WHERE action = $1 AND actor_id = $2`, action, actorID)
	if err != nil {
		f.t.Fatal(err)
	}
	defer rows.Close()
	var out []auditRow
	for rows.Next() {
		var r auditRow
		if err := rows.Scan(&r.actorType, &r.actorID, &r.actorLabel, &r.workspaceID, &r.targetID, &r.detail); err != nil {
			f.t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

// Each write audits once, with the staff actor; the workspace's owner reads the
// rows about their workspace.
func TestDBEachWriteAuditsOnceWithTheStaffActor(t *testing.T) {
	f := newDBFixture(t)
	adminID, adminEmail := f.staffUser(staff.RoleSystemAdmin)
	ownerID, _ := f.user("owner")
	memberID, _ := f.user("member")
	hireID, _ := f.user("hire")
	hireEmail := f.emailOf(hireID)
	ws := f.workspace(ownerID, memberID)

	wantOK := func(rec *httptest.ResponseRecorder, status int) {
		t.Helper()
		if rec.Code != status {
			t.Fatalf("status = %d, want %d; %s", rec.Code, status, rec.Body.String())
		}
	}
	wantOK(f.do(adminID, "POST", "/api/platform/workspaces/"+ws+"/status", `{"status":"read_only","reason":"unpaid"}`), 200)
	wantOK(f.do(adminID, "POST", "/api/platform/workspaces/"+ws+"/notes", `{"body":"owner asked for a call"}`), 201)
	wantOK(f.do(adminID, "POST", "/api/platform/users/"+memberID+"/status", `{"status":"suspended","reason":"abuse"}`), 200)
	wantOK(f.do(adminID, "POST", "/api/platform/users/"+memberID+"/revoke-sessions", `{"reason":"abuse"}`), 200)
	wantOK(f.do(adminID, "POST", "/api/platform/staff", `{"email":"`+hireEmail+`","role":"support"}`), 201)
	wantOK(f.do(adminID, "DELETE", "/api/platform/staff/"+hireID, ""), 200)

	for _, c := range []struct {
		action    string
		workspace string
		target    string
		label     bool // the row carries the staff email as the actor label
	}{
		{"workspace.status_changed", ws, ws, true},
		{"note.added", ws, ws, true},
		{"account.status_changed", "", memberID, true},
		{"account.sessions_revoked", "", memberID, true},
		{"staff.added", "", hireID, true},
		{"staff.removed", "", hireID, true},
	} {
		rows := f.staffRows(c.action, adminID)
		if len(rows) != 1 {
			t.Errorf("%s: %d audit rows, want 1", c.action, len(rows))
			continue
		}
		r := rows[0]
		if r.actorType != "staff" || r.actorID != adminID || r.workspaceID != c.workspace || r.targetID != c.target {
			t.Errorf("%s: row = %+v", c.action, r)
		}
		if c.label && r.actorLabel != adminEmail {
			t.Errorf("%s: actor label = %q, want %q", c.action, r.actorLabel, adminEmail)
		}
	}

	// The workspace owner reads the rows about the workspace in the workspace
	// log. This is the query behind GET /api/workspaces/{id}/audit-events in
	// Delegora (server/pkg/db/queries/audit_event.sql, ListWorkspaceAuditEvents),
	// which the owner's route runs for a workspace they own.
	rows, err := f.pool.Query(context.Background(),
		`SELECT action, actor_type, detail::text FROM audit_event
		 WHERE workspace_id = $1 ORDER BY created_at DESC, id DESC LIMIT 101`, ws)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := map[string]string{}
	for rows.Next() {
		var action, actorType, detail string
		if err := rows.Scan(&action, &actorType, &detail); err != nil {
			t.Fatal(err)
		}
		if actorType != "staff" {
			t.Errorf("row %s has actor type %q", action, actorType)
		}
		seen[action] = detail
		if strings.Contains(detail, "owner asked for a call") {
			t.Errorf("row %s carries the note's text", action)
		}
	}
	for _, want := range []string{"workspace.status_changed", "note.added"} {
		if _, ok := seen[want]; !ok {
			t.Errorf("the owner's workspace log has no %s row", want)
		}
	}
	if _, ok := seen["account.status_changed"]; ok {
		t.Error("an account-level row is in the workspace log")
	}
}

func (f *dbFixture) emailOf(id string) string {
	f.t.Helper()
	var email string
	if err := f.pool.QueryRow(context.Background(), `SELECT email FROM "user" WHERE id = $1`, id).Scan(&email); err != nil {
		f.t.Fatal(err)
	}
	return email
}

func TestDBRefusedChangesWriteNothing(t *testing.T) {
	f := newDBFixture(t)
	adminID, _ := f.staffUser(staff.RoleSystemAdmin)
	ownerID, _ := f.user("owner")
	memberID, _ := f.user("member")
	ws := f.workspace(ownerID, memberID)

	for _, c := range []struct {
		path, body, code string
	}{
		{"/workspaces/" + ws + "/status", `{"status":"suspended"}`, "reason_required"},
		{"/workspaces/" + ws + "/status", `{"status":"suspended","reason":"   "}`, "reason_required"},
		{"/workspaces/" + ws + "/status", `{"status":"gone","reason":"r"}`, "invalid_status"},
		{"/users/" + memberID + "/status", `{"status":"suspended"}`, "reason_required"},
		{"/users/" + memberID + "/status", `{"status":"deleted","reason":"r"}`, "invalid_status"},
		{"/users/" + memberID + "/revoke-sessions", `{}`, "reason_required"},
	} {
		rec := f.do(adminID, "POST", "/api/platform"+c.path, c.body)
		if rec.Code != 400 || !strings.Contains(rec.Body.String(), `"code":"`+c.code+`"`) {
			t.Errorf("POST %s %s = %d %s, want 400 %s", c.path, c.body, rec.Code, rec.Body.String(), c.code)
		}
	}
	if n := f.count(`SELECT count(*) FROM platform_workspace WHERE workspace_id = $1`, ws); n != 0 {
		t.Errorf("platform_workspace rows = %d", n)
	}
	if n := f.count(`SELECT count(*) FROM platform_account WHERE user_id = $1`, memberID); n != 0 {
		t.Errorf("platform_account rows = %d", n)
	}
	if n := f.count(`SELECT count(*) FROM audit_event WHERE actor_id = $1`, adminID); n != 0 {
		t.Errorf("audit rows = %d", n)
	}
	if len(f.pub.sent) != 0 {
		t.Errorf("published %v", f.pub.sent)
	}
}

// The middleware answers 404 to a role a route does not admit.
func TestDBRolesAreEnforced(t *testing.T) {
	f := newDBFixture(t)
	supportID, _ := f.staffUser(staff.RoleSupport)
	billingID, _ := f.staffUser(staff.RoleBilling)
	ownerID, _ := f.user("owner")
	memberID, _ := f.user("member")
	ws := f.workspace(ownerID, memberID)

	// support has no /staff.
	for _, c := range []struct{ method, path, body string }{
		{"GET", "/api/platform/staff", ""},
		{"POST", "/api/platform/staff", `{"email":"x@example.test","role":"admin"}`},
		{"DELETE", "/api/platform/staff/" + billingID, ""},
	} {
		if rec := f.do(supportID, c.method, c.path, c.body); rec.Code != 404 {
			t.Errorf("support %s %s = %d, want 404", c.method, c.path, rec.Code)
		}
	}
	// billing cannot change a workspace state: 404, as the middleware answers.
	if rec := f.do(billingID, "POST", "/api/platform/workspaces/"+ws+"/status", `{"status":"suspended","reason":"r"}`); rec.Code != 404 {
		t.Errorf("billing status change = %d, want 404", rec.Code)
	}
	if n := f.count(`SELECT count(*) FROM platform_workspace WHERE workspace_id = $1`, ws); n != 0 {
		t.Errorf("a refused change wrote %d rows", n)
	}
	// support can read and write notes.
	if rec := f.do(supportID, "GET", "/api/platform/workspaces/"+ws, ""); rec.Code != 200 {
		t.Errorf("support tenant read = %d", rec.Code)
	}
}

// With every other administrator revoked, the last one cannot be removed.
func TestDBLastAdminRefusesTheLastRemoval(t *testing.T) {
	f := newDBFixture(t)
	ctx := context.Background()
	adminID, _ := f.staffUser(staff.RoleSystemAdmin)
	secondID, _ := f.staffUser(staff.RoleSystemAdmin)

	// The test database is shared: park the administrators other than these two
	// for the length of the test.
	rows, err := f.pool.Query(ctx,
		`UPDATE platform_staff SET revoked_at = now()
		 WHERE role = 'system_admin' AND revoked_at IS NULL AND user_id::text <> ALL($1::text[])
		 RETURNING user_id::text, created_at`, []string{adminID, secondID})
	if err != nil {
		t.Fatal(err)
	}
	type parked struct {
		id string
		at time.Time
	}
	var others []parked
	for rows.Next() {
		var p parked
		if err := rows.Scan(&p.id, &p.at); err != nil {
			t.Fatal(err)
		}
		others = append(others, p)
	}
	rows.Close()
	t.Cleanup(func() {
		for _, p := range others {
			f.pool.Exec(ctx,
				`UPDATE platform_staff SET revoked_at = NULL WHERE user_id = $1 AND created_at = $2 AND role = 'system_admin'`, p.id, p.at)
		}
	})

	if rec := f.do(adminID, "DELETE", "/api/platform/staff/"+secondID, ""); rec.Code != 200 {
		t.Fatalf("removing one of two administrators = %d %s", rec.Code, rec.Body.String())
	}
	rec := f.do(adminID, "DELETE", "/api/platform/staff/"+adminID, "")
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), `"code":"last_system_admin"`) {
		t.Fatalf("removing the last administrator = %d %s", rec.Code, rec.Body.String())
	}
	if n := f.count(`SELECT count(*) FROM platform_staff WHERE user_id = $1 AND revoked_at IS NULL`, adminID); n != 1 {
		t.Fatalf("the last administrator has %d active rows", n)
	}
	if n := f.count(`SELECT count(*) FROM audit_event WHERE action = 'staff.removed' AND target_id = $1`, adminID); n != 0 {
		t.Fatalf("a refused removal left %d audit rows", n)
	}
}

func TestDBStaffAddReactivatesARevokedRow(t *testing.T) {
	f := newDBFixture(t)
	adminID, _ := f.staffUser(staff.RoleSystemAdmin)
	otherID, otherEmail := f.staffUser(staff.RoleSupport)

	if rec := f.do(adminID, "DELETE", "/api/platform/staff/"+otherID, ""); rec.Code != 200 {
		t.Fatalf("remove = %d %s", rec.Code, rec.Body.String())
	}
	if rec := f.do(adminID, "POST", "/api/platform/staff", `{"email":"`+otherEmail+`","role":"billing"}`); rec.Code != 201 {
		t.Fatalf("add = %d %s", rec.Code, rec.Body.String())
	}
	var rows, active int
	var role string
	if err := f.pool.QueryRow(context.Background(),
		`SELECT count(*), count(*) FILTER (WHERE revoked_at IS NULL), max(role) FROM platform_staff WHERE user_id = $1`, otherID).
		Scan(&rows, &active, &role); err != nil {
		t.Fatal(err)
	}
	if rows != 1 || active != 1 || role != "billing" {
		t.Fatalf("rows=%d active=%d role=%s, want the revoked row reused with the new role", rows, active, role)
	}
	if rec := f.do(adminID, "POST", "/api/platform/staff", `{"email":"nobody-`+randomHex(t)+`@example.test","role":"support"}`); rec.Code != 404 ||
		!strings.Contains(rec.Body.String(), `"code":"no_such_account"`) {
		t.Fatalf("unknown email = %d %s", rec.Code, rec.Body.String())
	}
}

// An account status change keeps sessions_valid_after, and a session revocation
// keeps the status.
func TestDBAccountChangesPreserveTheOtherColumn(t *testing.T) {
	f := newDBFixture(t)
	adminID, _ := f.staffUser(staff.RoleSystemAdmin)
	memberID, _ := f.user("member")
	ownerID, _ := f.user("owner")
	f.workspace(ownerID, memberID)

	if rec := f.do(adminID, "POST", "/api/platform/users/"+memberID+"/revoke-sessions", `{"reason":"r"}`); rec.Code != 200 {
		t.Fatalf("revoke = %d %s", rec.Code, rec.Body.String())
	}
	var mark time.Time
	if err := f.pool.QueryRow(context.Background(),
		`SELECT sessions_valid_after FROM platform_account WHERE user_id = $1`, memberID).Scan(&mark); err != nil {
		t.Fatal(err)
	}
	if mark.Nanosecond() != 0 {
		t.Fatalf("sessions_valid_after = %v, want whole seconds", mark)
	}

	for _, status := range []string{"suspended", "active"} {
		if rec := f.do(adminID, "POST", "/api/platform/users/"+memberID+"/status", `{"status":"`+status+`","reason":"r"}`); rec.Code != 200 {
			t.Fatalf("status %s = %d %s", status, rec.Code, rec.Body.String())
		}
		var got time.Time
		var st string
		if err := f.pool.QueryRow(context.Background(),
			`SELECT status, sessions_valid_after FROM platform_account WHERE user_id = $1`, memberID).Scan(&st, &got); err != nil {
			t.Fatal(err)
		}
		if st != status || !got.Equal(mark) {
			t.Fatalf("after %s: status=%s sessions_valid_after=%v, want the mark %v kept", status, st, got, mark)
		}
	}

	// A revocation on a suspended account leaves it suspended.
	f.do(adminID, "POST", "/api/platform/users/"+memberID+"/status", `{"status":"suspended","reason":"r"}`)
	f.do(adminID, "POST", "/api/platform/users/"+memberID+"/revoke-sessions", `{"reason":"again"}`)
	if n := f.count(`SELECT count(*) FROM platform_account WHERE user_id = $1 AND status = 'suspended'`, memberID); n != 1 {
		t.Fatal("a revocation changed the status")
	}

	// The guard cache key was dropped and a notice was sent for each change.
	key := "mul:auth:acct:" + memberID
	f.cache.mu.Lock()
	dropped := 0
	for _, k := range f.cache.dels {
		if k == key {
			dropped++
		}
	}
	f.cache.mu.Unlock()
	if dropped != 5 {
		t.Errorf("cache key dropped %d times, want 5 (one per change)", dropped)
	}
	kinds := map[string]int{}
	for _, p := range f.pub.sent {
		if p.user == memberID {
			kinds[p.kind]++
		}
	}
	if kinds[accountcache.KindStatus] != 3 || kinds[accountcache.KindSessions] != 2 {
		t.Errorf("notices = %v", kinds)
	}
}

// Notes are in the platform's table and nowhere a workspace member can read:
// the audit row about a note does not carry its text, and the staff-only reads
// are the only ones that return it.
func TestDBNotesAreInvisibleToTheWorkspace(t *testing.T) {
	f := newDBFixture(t)
	adminID, _ := f.staffUser(staff.RoleSystemAdmin)
	ownerID, _ := f.user("owner")
	memberID, _ := f.user("member")
	ws := f.workspace(ownerID, memberID)
	text := "internal-only-" + randomHex(t)

	if rec := f.do(adminID, "POST", "/api/platform/workspaces/"+ws+"/notes", `{"body":"`+text+`"}`); rec.Code != 201 {
		t.Fatalf("add note = %d %s", rec.Code, rec.Body.String())
	}
	if rec := f.do(adminID, "GET", "/api/platform/workspaces/"+ws+"/notes", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), text) {
		t.Fatalf("staff cannot read the note: %d %s", rec.Code, rec.Body.String())
	}
	if n := f.count(`SELECT count(*) FROM audit_event WHERE detail::text LIKE '%' || $1 || '%' OR target_label LIKE '%' || $1 || '%'`, text); n != 0 {
		t.Fatalf("%d audit rows carry the note's text", n)
	}
	var count int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM platform_note WHERE workspace_id = $1`, ws).Scan(&count); err != nil || count != 1 {
		t.Fatalf("notes stored = %d, %v", count, err)
	}
}

func TestDBReadsReturnWhatTheWritesChanged(t *testing.T) {
	f := newDBFixture(t)
	adminID, _ := f.staffUser(staff.RoleSystemAdmin)
	ownerID, ownerEmail := f.user("owner")
	memberID, _ := f.user("member")
	ws := f.workspace(ownerID, memberID)

	f.do(adminID, "POST", "/api/platform/workspaces/"+ws+"/status", `{"status":"suspended","reason":"abuse"}`)
	f.do(adminID, "POST", "/api/platform/workspaces/"+ws+"/notes", `{"body":"n1"}`)

	m := decode(t, f.do(adminID, "GET", "/api/platform/workspaces/"+ws, ""))
	state, _ := m["state"].(map[string]any)
	owner, _ := m["owner"].(map[string]any)
	if m["status"] != "suspended" || m["member_count"] != float64(2) || m["notes_count"] != float64(1) ||
		state["reason"] != "abuse" || state["changed_by"] != adminID || owner["id"] != ownerID || owner["email"] != ownerEmail {
		t.Fatalf("tenant = %v", m)
	}

	// A search by the owner's email finds the workspace's tenant.
	list := decode(t, f.do(adminID, "GET", "/api/platform/tenants?q="+ownerEmail, ""))
	tenants, _ := list["tenants"].([]any)
	if len(tenants) != 1 {
		t.Fatalf("tenants = %v", list)
	}

	u := decode(t, f.do(adminID, "GET", "/api/platform/users/"+ownerID, ""))
	ms, _ := u["memberships"].([]any)
	if len(ms) != 1 || u["status"] != "active" {
		t.Fatalf("user = %v", u)
	}
	if rec := f.do(adminID, "GET", "/api/platform/overview", ""); rec.Code != 200 {
		t.Fatalf("overview = %d %s", rec.Code, rec.Body.String())
	}
	if rec := f.do(adminID, "GET", "/api/platform/audit-events?limit=5", ""); rec.Code != 200 {
		t.Fatalf("audit-events = %d %s", rec.Code, rec.Body.String())
	}
	if rec := f.do(adminID, "GET", "/api/platform/workspaces/"+ws+"/audit-events?action=workspace.status_changed", ""); rec.Code != 200 ||
		!strings.Contains(rec.Body.String(), `"workspace.status_changed"`) {
		t.Fatalf("tenant audit-events = %d %s", rec.Code, rec.Body.String())
	}
}

// by_plan counts a workspace with no subscription row under free, and one with a
// row under its plan. The shared test database may hold other workspaces, so the
// test reads the counts before and after.
func TestDBOverviewCountsWorkspacesByPlan(t *testing.T) {
	f := newDBFixture(t)
	owner, _ := f.user("owner")
	member, _ := f.user("member")
	reader := NewPgxReader(f.pool)
	ctx := context.Background()

	before, err := reader.Overview(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	f.workspace(owner, member) // no subscription row: free
	paid := f.workspace(owner, member)
	key := "t" + randomHex(t)
	if _, err := f.pool.Exec(ctx, `INSERT INTO workspace_subscription (workspace_id, plan_key, status, version) VALUES ($1, $2, 'active', 1)`, paid, key); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.pool.Exec(ctx, `DELETE FROM workspace_subscription WHERE workspace_id = $1`, paid) })

	after, err := reader.Overview(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got := after.ByPlan["free"] - before.ByPlan["free"]; got != 1 {
		t.Errorf("free grew by %d, want 1 (the workspace with no row)", got)
	}
	if after.ByPlan[key] != 1 {
		t.Errorf("by_plan[%s] = %d, want 1", key, after.ByPlan[key])
	}
}

// The reset deletes the user's user_mfa row in the same transaction as its
// audit row, and leaves other users' rows.
func TestDBMFAResetDeletesOnlyThatUsersRow(t *testing.T) {
	f := newDBFixture(t)
	ctx := context.Background()
	admin, adminEmail := f.staffUser(staff.RoleSystemAdmin)
	target, targetEmail := f.user("mfa-target")
	other, _ := f.user("mfa-other")
	for _, id := range []string{target, other} {
		if _, err := f.pool.Exec(ctx,
			`INSERT INTO user_mfa VALUES ($1::uuid, '\x00'::bytea, now(), '{}', 0, now())`, id); err != nil {
			t.Fatalf("create user_mfa row: %v", err)
		}
	}
	t.Cleanup(func() {
		f.pool.Exec(ctx, `DELETE FROM user_mfa WHERE user_id = ANY($1::uuid[])`, []string{target, other})
	})

	rec := f.do(admin, "POST", "/api/platform/users/"+target+"/mfa-reset", "")
	if rec.Code != 200 {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	const rows = `SELECT count(*) FROM user_mfa WHERE user_id = $1::uuid`
	if got, kept := f.count(rows, target), f.count(rows, other); got != 0 || kept != 1 {
		t.Fatalf("rows after reset: target %d, other %d", got, kept)
	}
	audits := f.staffRows("user.mfa_reset", admin)
	if len(audits) != 1 || audits[0].targetID != target || audits[0].actorLabel != adminEmail {
		t.Fatalf("audit rows = %+v, want one for %s (%s)", audits, target, targetEmail)
	}
}

// seedCatalogue makes the catalogue's plans visible, as the platform does when it
// starts: ApplyPayment prices a seat from the plan. The rows this adds are removed
// when the test ends, so the table is as it was for the other tests.
func seedCatalogue(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	before := map[string]bool{}
	rows, err := pool.Query(ctx, `SELECT key FROM plan`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			t.Fatal(err)
		}
		before[key] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	if _, err := plan.Seed(ctx, plan.NewPgxRunner(pool), quiet); err != nil {
		t.Fatal(err)
	}
	all, err := plan.Catalogue()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, p := range all {
			if !before[p.Key] {
				pool.Exec(ctx, `DELETE FROM plan WHERE key = $1`, p.Key)
			}
		}
	})
}
