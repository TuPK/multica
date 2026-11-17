package subscription

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/testdb"
	"github.com/multica-ai/multica/platform-management/internal/trust"
)

// Database tests: skipped unless PLATFORM_TEST_DATABASE_URL names a *_test
// database and DATABASE_URL equals it. Each test makes its own users, workspace
// and plan key and removes them, with the rows the code wrote about them.

type dbFixture struct {
	t    *testing.T
	pool *pgxpool.Pool

	userIDs []string
	wsIDs   []string
	planKey []string
}

func newDBFixture(t *testing.T) *dbFixture {
	t.Helper()
	f := &dbFixture{t: t, pool: testdb.Open(t)}
	t.Cleanup(f.cleanup)
	return f
}

func (f *dbFixture) cleanup() {
	ctx := context.Background()
	for _, ws := range f.wsIDs {
		f.pool.Exec(ctx, `DELETE FROM audit_event WHERE workspace_id = $1`, ws)
		f.pool.Exec(ctx, `DELETE FROM billing_order WHERE workspace_id = $1`, ws)
		f.pool.Exec(ctx, `DELETE FROM platform_seat_operation WHERE workspace_id = $1`, ws)
		f.pool.Exec(ctx, `DELETE FROM workspace_subscription WHERE workspace_id = $1`, ws)
		f.pool.Exec(ctx, `DELETE FROM billing_profile WHERE workspace_id = $1`, ws)
		f.pool.Exec(ctx, `DELETE FROM member WHERE workspace_id = $1`, ws)
		f.pool.Exec(ctx, `DELETE FROM workspace WHERE id = $1`, ws)
	}
	for _, u := range f.userIDs {
		f.pool.Exec(ctx, `DELETE FROM "user" WHERE id = $1`, u)
	}
	for _, k := range f.planKey {
		f.pool.Exec(ctx, `DELETE FROM plan WHERE key = $1`, k)
	}
}

func hexSuffix(t *testing.T) string {
	t.Helper()
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

func (f *dbFixture) user() string {
	f.t.Helper()
	var id string
	if err := f.pool.QueryRow(context.Background(),
		`INSERT INTO "user" (name, email) VALUES ('Billing Test', $1) RETURNING id::text`,
		"billing-"+hexSuffix(f.t)+"@example.test").Scan(&id); err != nil {
		f.t.Fatalf("create user: %v", err)
	}
	f.userIDs = append(f.userIDs, id)
	return id
}

// workspace creates a workspace with the given members (user id -> role).
func (f *dbFixture) workspace(members map[string]string) string {
	f.t.Helper()
	var id string
	s := hexSuffix(f.t)
	if err := f.pool.QueryRow(context.Background(),
		`INSERT INTO workspace (name, slug) VALUES ($1, $2) RETURNING id::text`, "Billing Test "+s, "billing-test-"+s).Scan(&id); err != nil {
		f.t.Fatalf("create workspace: %v", err)
	}
	f.wsIDs = append(f.wsIDs, id)
	for user, role := range members {
		if _, err := f.pool.Exec(context.Background(),
			`INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, $3)`, id, user, role); err != nil {
			f.t.Fatalf("create member: %v", err)
		}
	}
	return id
}

// plan inserts a plan with a key of its own, so parallel packages never meet.
func (f *dbFixture) plan(mutate func(*plan.Plan)) plan.Plan {
	f.t.Helper()
	p := plan.Plan{
		Key: "t" + hexSuffix(f.t), Name: "DB test plan", Currency: "vnd", IsPublic: false,
		Limits: plan.Limits{"issue_count": i64(100), "autopilot_runs": i64(10)}, Features: map[string]bool{},
	}
	if mutate != nil {
		mutate(&p)
	}
	f.planKey = append(f.planKey, p.Key)
	err := plan.NewPgxRunner(f.pool).InTx(context.Background(), func(tx plan.Tx) error { return tx.Upsert(context.Background(), p) })
	if err != nil {
		f.t.Fatalf("create plan: %v", err)
	}
	return p
}

func (f *dbFixture) randomUUID() string {
	f.t.Helper()
	var id string
	if err := f.pool.QueryRow(context.Background(), `SELECT gen_random_uuid()::text`).Scan(&id); err != nil {
		f.t.Fatal(err)
	}
	return id
}

func (f *dbFixture) count(query string, args ...any) int {
	f.t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

func (f *dbFixture) service(cache Invalidator) *Service {
	return NewService(NewPgxRunner(f.pool), cache)
}

func TestDBApplyWritesTheRowTheVersionAndTheAuditRow(t *testing.T) {
	f := newDBFixture(t)
	ws, staffUser := f.workspace(nil), f.randomUUID()
	p := f.plan(func(p *plan.Plan) { n := 2; p.MinSeats = &n })
	cache := &recordingCache{}
	svc := f.service(cache)
	ctx := context.Background()

	c := Change{
		WorkspaceID: ws, PlanKey: p.Key, Seats: 4, Interval: IntervalYear, Reason: "db test",
		LimitOverrides: plan.Limits{"issue_count": nil, "agents": i64(7)}, ActorID: staffUser, ActorLabel: "billing@example.test",
	}
	first, err := svc.Apply(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if first.Subscription.Version != 1 || !first.Changed {
		t.Fatalf("first = %+v", first)
	}
	c.Seats = 6
	second, err := svc.Apply(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if second.Subscription.Version != 2 {
		t.Fatalf("second version = %d, want 2", second.Subscription.Version)
	}
	if _, err := svc.Apply(ctx, c); err != nil { // unchanged
		t.Fatal(err)
	}

	stored, found, err := NewPgxReader(f.pool).Get(ctx, ws)
	if err != nil || !found {
		t.Fatalf("Get = %v, %v", found, err)
	}
	if stored.Version != 2 || stored.Seats != 6 || stored.PlanKey != p.Key || stored.Interval != IntervalYear || stored.Status != StatusActive {
		t.Fatalf("stored = %+v", stored)
	}
	if v, ok := stored.LimitOverrides["issue_count"]; !ok || v != nil || *stored.LimitOverrides["agents"] != 7 {
		t.Fatalf("overrides = %v, want an explicit null and 7", stored.LimitOverrides)
	}

	if n := f.count(`SELECT count(*) FROM audit_event WHERE workspace_id = $1 AND action = $2 AND actor_type = 'staff' AND actor_id = $3`,
		ws, audit.ActionSubscriptionChanged, staffUser); n != 2 {
		t.Fatalf("audit rows = %d, want 2 (the unchanged call writes none)", n)
	}
	if len(cache.workspaces) != 2 {
		t.Fatalf("cache cleared %d times, want 2", len(cache.workspaces))
	}
}

// failingAudit makes the audit write of every transaction fail.
type failingAudit struct {
	inner Runner
	err   error
}

func (r failingAudit) InTx(ctx context.Context, fn func(Tx) error) error {
	return r.inner.InTx(ctx, func(tx Tx) error { return fn(auditFailsTx{Tx: tx, err: r.err}) })
}

type auditFailsTx struct {
	Tx
	err error
}

func (t auditFailsTx) InsertAuditEvent(context.Context, audit.Entry) error { return t.err }

func TestDBFailedAuditWriteRollsTheChangeBack(t *testing.T) {
	f := newDBFixture(t)
	ws, staffUser := f.workspace(nil), f.randomUUID()
	p := f.plan(nil)
	ctx := context.Background()
	c := Change{WorkspaceID: ws, PlanKey: p.Key, Seats: 3, Interval: IntervalMonth, Reason: "db test", ActorID: staffUser, ActorLabel: "x"}

	good := f.service(&recordingCache{})
	if _, err := good.Apply(ctx, c); err != nil {
		t.Fatal(err)
	}

	cache := &recordingCache{}
	bad := NewService(failingAudit{inner: NewPgxRunner(f.pool), err: errors.New("audit_event is full")}, cache)
	c.Seats = 30
	if _, err := bad.Apply(ctx, c); err == nil {
		t.Fatal("no error when the audit write failed")
	}
	stored, _, err := NewPgxReader(f.pool).Get(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Seats != 3 || stored.Version != 1 {
		t.Fatalf("stored = seats %d version %d, want 3 and 1: the change was kept", stored.Seats, stored.Version)
	}
	if len(cache.workspaces) != 0 {
		t.Fatal("the cache was cleared for a rolled-back change")
	}

	// The first write rolls back too: no row appears.
	ws2 := f.workspace(nil)
	c.WorkspaceID = ws2
	if _, err := bad.Apply(ctx, c); err == nil {
		t.Fatal("no error on a failed first write")
	}
	if n := f.count(`SELECT count(*) FROM workspace_subscription WHERE workspace_id = $1`, ws2); n != 0 {
		t.Fatalf("%d rows after a rolled-back first write", n)
	}
}

func TestDBFirstInsertLosesToAnExistingRow(t *testing.T) {
	f := newDBFixture(t)
	ws, staffUser := f.workspace(nil), f.randomUUID()
	p := f.plan(nil)
	ctx := context.Background()
	if _, err := f.service(&recordingCache{}).Apply(ctx, Change{
		WorkspaceID: ws, PlanKey: p.Key, Interval: IntervalMonth, Seats: 1, Reason: "r", ActorID: staffUser, ActorLabel: "x",
	}); err != nil {
		t.Fatal(err)
	}
	err := NewPgxRunner(f.pool).InTx(ctx, func(tx Tx) error {
		_, ok, err := tx.Insert(ctx, Subscription{
			WorkspaceID: ws, PlanKey: p.Key, Status: StatusActive, Interval: IntervalMonth, LimitOverrides: plan.Limits{},
		})
		if err != nil {
			return err
		}
		if ok {
			t.Error("a second first-insert reported success")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDBSummaryRoleChecksAgainstMemberRows(t *testing.T) {
	f := newDBFixture(t)
	owner, admin, member, stranger := f.user(), f.user(), f.user(), f.user()
	ws := f.workspace(map[string]string{owner: "owner", admin: "admin", member: "member"})
	p := f.plan(func(p *plan.Plan) { n := 3; p.SeatsFixed = &n })
	if _, err := f.service(&recordingCache{}).Apply(context.Background(), Change{
		WorkspaceID: ws, PlanKey: p.Key, Interval: IntervalMonth, Reason: "r", ActorID: f.randomUUID(), ActorLabel: "x",
	}); err != nil {
		t.Fatal(err)
	}

	api := NewAPI(APIDeps{
		Subscriptions: NewPgxReader(f.pool), Plans: plan.NewPgxReader(f.pool), Members: NewPgxMembers(f.pool),
		Pending: NewPgxPending(f.pool), Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}).Handler(trust.New(trust.DefaultCIDRs()).RequireUser)

	get := func(user string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions/"+ws+"/summary", nil)
		req.RemoteAddr = "127.0.0.1:5000"
		req.Header.Set(trust.UserHeader, user)
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, req)
		return rec
	}
	for name, tc := range map[string]struct {
		user string
		want int
	}{"owner": {owner, 200}, "admin": {admin, 200}, "member": {member, 403}, "non-member": {stranger, 404}} {
		if rec := get(tc.user); rec.Code != tc.want {
			t.Errorf("%s: status %d, want %d (%s)", name, rec.Code, tc.want, rec.Body)
		}
	}

	m := decodeMap(t, get(owner))
	checkSummarySchema(t, m)
	if m["human_members"] != float64(3) {
		t.Errorf("human_members = %v, want the 3 member rows", m["human_members"])
	}
	if sc := obj(t, m["seat_capacity"], "seat_capacity"); sc["purchased"] != float64(3) || sc["used"] != float64(3) || sc["available"] != float64(0) {
		t.Errorf("seat_capacity = %v", sc)
	}
}

// insertOrder writes a billing_order row for the workspace, for the reads under test.
func (f *dbFixture) insertOrder(ws, kind, status string, seats int, createdAt, expiresAt time.Time) string {
	f.t.Helper()
	var id string
	if err := f.pool.QueryRow(context.Background(), `
		INSERT INTO billing_order (id, workspace_id, kind, plan_key, interval, seats, amount, vat_amount, currency,
			status, provider, transfer_code, expires_at, created_at)
		VALUES (gen_random_uuid(), $1, $2, 'pro', 'month', $3, 100000, 10000, 'vnd', $4, 'manual', $5, $6, $7)
		RETURNING id::text`, ws, kind, seats, status, "MUL"+hexSuffix(f.t), expiresAt, createdAt).Scan(&id); err != nil {
		f.t.Fatalf("create order: %v", err)
	}
	return id
}

// The summary of a paid workspace counts its member rows, the seats held by live
// seat operations, and the add_seats order waiting for its transfer.
func TestDBSummaryReflectsTheSubscriptionTheHeldSeatsAndThePendingPurchase(t *testing.T) {
	f := newDBFixture(t)
	owner, other := f.user(), f.user()
	ws := f.workspace(map[string]string{owner: "owner", other: "member"})
	p := f.plan(func(p *plan.Plan) {
		m, y := int64(100000), int64(1000000)
		p.SeatPriceMonth, p.SeatPriceYear = &m, &y
	})
	ctx := context.Background()
	now := time.Now().UTC()
	start, end := now.Add(-24*time.Hour), now.Add(24*time.Hour)
	if _, err := f.pool.Exec(ctx, `
		INSERT INTO workspace_subscription (workspace_id, plan_key, status, seats, interval, current_period_start, current_period_end, version)
		VALUES ($1, $2, 'active', 5, 'month', $3, $4, 6)`, ws, p.Key, start, end); err != nil {
		t.Fatal(err)
	}
	// One invitation holds a seat; an expired one holds none.
	for _, expires := range []time.Time{now.Add(time.Hour), now.Add(-time.Hour)} {
		if _, err := f.pool.Exec(ctx, `
			INSERT INTO platform_seat_operation (workspace_id, operation_token, kind, subject_id, state, expires_at)
			VALUES ($1, gen_random_uuid(), 'invitation', gen_random_uuid(), 'reserved', $2)`, ws, expires); err != nil {
			t.Fatal(err)
		}
	}
	f.insertOrder(ws, "add_seats", "paid", 9, now.Add(-time.Hour), now.Add(time.Hour))
	f.insertOrder(ws, "add_seats", "pending", 7, now.Add(-2*time.Hour), now.Add(-time.Minute)) // expired
	f.insertOrder(ws, "subscribe", "pending", 6, now.Add(-time.Hour), now.Add(time.Hour))      // not seats
	f.insertOrder(ws, "add_seats", "pending", 2, now.Add(-3*time.Hour), now.Add(time.Hour))
	newest := f.insertOrder(ws, "add_seats", "pending", 3, now.Add(-time.Minute), now.Add(time.Hour))

	api := NewAPI(APIDeps{
		Subscriptions: NewPgxReader(f.pool), Plans: plan.NewPgxReader(f.pool), Members: NewPgxMembers(f.pool),
		Pending: NewPgxPending(f.pool), Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}).Handler(trust.New(trust.DefaultCIDRs()).RequireUser)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions/"+ws+"/summary", nil)
	req.RemoteAddr = "127.0.0.1:5000"
	req.Header.Set(trust.UserHeader, owner)
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)

	m := decodeMap(t, rec)
	checkSummarySchema(t, m)
	sc := obj(t, m["seat_capacity"], "seat_capacity")
	if sc["purchased"] != float64(5) || sc["used"] != float64(2) || sc["reserved"] != float64(1) || sc["available"] != float64(2) || sc["version"] != float64(7) {
		t.Errorf("seat_capacity = %v, want 5 purchased, 2 used, 1 held, 2 available, version 6 + 1", sc)
	}
	ap := obj(t, sc["active_purchase"], "active_purchase")
	if sc["pending_quantity"] != float64(8) || ap["request_id"] != newest || ap["target_seats"] != float64(8) {
		t.Errorf("pending_quantity = %v, active_purchase = %v, want the newest pending add_seats order (%s)", sc["pending_quantity"], ap, newest)
	}
	if a := obj(t, m["available_actions"], "available_actions"); a["purchase_seats"] != true {
		t.Errorf("available_actions = %v", a)
	}
}

// pending_change and unit_price are written by Insert and Update and read back;
// Update replaces them, and a nil clears them.
func TestDBPendingChangeAndUnitPriceRoundTrip(t *testing.T) {
	f := newDBFixture(t)
	ws := f.workspace(nil)
	p := f.plan(nil)
	ctx := context.Background()
	runner := NewPgxRunner(f.pool)

	requestedAt := time.Date(2031, 2, 3, 4, 5, 6, 0, time.UTC)
	price := int64(149000)
	base := Subscription{
		WorkspaceID: ws, PlanKey: p.Key, Status: StatusActive, Seats: 3, Interval: IntervalMonth,
		LimitOverrides: plan.Limits{},
		PendingChange:  &PendingChange{PlanKey: plan.FreeKey, Seats: 2, Interval: IntervalYear, RequestedBy: f.randomUUID(), RequestedAt: requestedAt},
		UnitPrice:      &price,
	}
	err := runner.InTx(ctx, func(tx Tx) error {
		_, ok, err := tx.Insert(ctx, base)
		if err == nil && !ok {
			t.Error("Insert lost")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	got, found, err := NewPgxReader(f.pool).Get(ctx, ws)
	if err != nil || !found {
		t.Fatalf("Get = %v, %v", found, err)
	}
	if got.PendingChange == nil || *got.PendingChange != *base.PendingChange {
		t.Fatalf("pending change = %+v, want %+v", got.PendingChange, base.PendingChange)
	}
	if got.UnitPrice == nil || *got.UnitPrice != price {
		t.Fatalf("unit price = %v, want %d", got.UnitPrice, price)
	}

	// Update with new values replaces them; the version moves by one.
	newer := got
	newer.PendingChange = &PendingChange{PlanKey: p.Key, Seats: 9, Interval: IntervalMonth, RequestedBy: base.PendingChange.RequestedBy, RequestedAt: requestedAt.Add(time.Hour)}
	otherPrice := int64(1)
	newer.UnitPrice = &otherPrice
	var updated Subscription
	if err := runner.InTx(ctx, func(tx Tx) error { var e error; updated, e = tx.Update(ctx, newer); return e }); err != nil {
		t.Fatal(err)
	}
	if updated.Version != got.Version+1 || updated.PendingChange == nil || *updated.PendingChange != *newer.PendingChange || *updated.UnitPrice != 1 {
		t.Fatalf("updated = %+v, %+v, %v", updated.Version, updated.PendingChange, updated.UnitPrice)
	}

	// Update with nil clears them.
	cleared := updated
	cleared.PendingChange, cleared.UnitPrice = nil, nil
	if err := runner.InTx(ctx, func(tx Tx) error { var e error; updated, e = tx.Update(ctx, cleared); return e }); err != nil {
		t.Fatal(err)
	}
	if again, _, _ := NewPgxReader(f.pool).Get(ctx, ws); again.PendingChange != nil || again.UnitPrice != nil {
		t.Fatalf("cleared row = %+v, %v; want both empty", again.PendingChange, again.UnitPrice)
	}
}

// A row written without the two columns, as every row before migration 953 was,
// reads with both empty, and a staff change through Apply keeps what is there.
func TestDBOldRowWithoutPendingChangeReadsCleanly(t *testing.T) {
	f := newDBFixture(t)
	ws, staffUser := f.workspace(nil), f.randomUUID()
	p := f.plan(nil)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `INSERT INTO workspace_subscription (workspace_id, plan_key, status, seats, version)
		VALUES ($1, $2, 'active', 3, 1)`, ws, p.Key); err != nil {
		t.Fatal(err)
	}
	got, found, err := NewPgxReader(f.pool).Get(ctx, ws)
	if err != nil || !found {
		t.Fatalf("Get = %v, %v", found, err)
	}
	if got.PendingChange != nil || got.UnitPrice != nil {
		t.Fatalf("old row = %+v, %v; want both empty", got.PendingChange, got.UnitPrice)
	}

	// A staff change reads the row, edits a copy and writes it: the two new
	// fields ride along instead of being reset.
	if _, err := f.pool.Exec(ctx, `UPDATE workspace_subscription SET pending_change = '{"plan_key":"free","seats":1,"interval":"month","requested_by":"x","requested_at":"2031-01-01T00:00:00Z"}', unit_price = 5 WHERE workspace_id = $1`, ws); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service(&recordingCache{}).Apply(ctx, Change{
		WorkspaceID: ws, PlanKey: p.Key, Seats: 4, Interval: IntervalMonth, Reason: "r", ActorID: staffUser, ActorLabel: "x",
	}); err != nil {
		t.Fatal(err)
	}
	after, _, _ := NewPgxReader(f.pool).Get(ctx, ws)
	if after.Seats != 4 || after.PendingChange == nil || after.PendingChange.PlanKey != "free" || after.UnitPrice == nil || *after.UnitPrice != 5 {
		t.Fatalf("after a staff change: seats %d, pending %+v, unit price %v; want the two fields kept", after.Seats, after.PendingChange, after.UnitPrice)
	}
}

// dbConsole is the console routes on the real plan tables.
func (f *dbFixture) dbConsole(t *testing.T) *consoleEnv {
	t.Helper()
	e := &consoleEnv{t: t, subs: newFakeStore(t), profs: newFakeProfiles(t), cache: &recordingCache{}, policy: &recordingCache{}}
	e.h = buildConsole(t, e, plan.NewPgxReader(f.pool), plan.NewPgxRunner(f.pool))
	return e
}

// putPlanCurrency sends the full plan with only its currency changed.
func (f *dbFixture) putPlanCurrency(e *consoleEnv, key, currency string) *httptest.ResponseRecorder {
	body := strings.Replace(teamBody, `"currency":"vnd"`, `"currency":"`+currency+`"`, 1)
	return e.do(adminUser, "PUT", "/api/platform/plans/"+key, body)
}

func (f *dbFixture) storedCurrency(key string) string {
	f.t.Helper()
	var c string
	if err := f.pool.QueryRow(context.Background(), `SELECT currency FROM plan WHERE key = $1`, key).Scan(&c); err != nil {
		f.t.Fatal(err)
	}
	return c
}

// The plan audit rows carry the key as target and no workspace.
func (f *dbFixture) dropPlanAudits(key string) {
	f.t.Cleanup(func() {
		f.pool.Exec(context.Background(), `DELETE FROM audit_event WHERE target_type = 'plan' AND target_id = $1`, key)
	})
}

func TestDBCurrencyLockedWithSubscriptionRow(t *testing.T) {
	f := newDBFixture(t)
	e := f.dbConsole(t)
	p := f.plan(nil)
	f.dropPlanAudits(p.Key)
	wantStatus(t, f.putPlanCurrency(e, p.Key, "vnd"), 200)
	wantStatus(t, f.putPlanCurrency(e, p.Key, "usd"), 200) // not in use yet
	wantStatus(t, f.putPlanCurrency(e, p.Key, "vnd"), 200)

	ws := f.workspace(nil)
	now := time.Now().UTC()
	if _, err := f.pool.Exec(context.Background(), `
		INSERT INTO workspace_subscription (workspace_id, plan_key, status, seats, interval, current_period_start, current_period_end, version)
		VALUES ($1, $2, 'active', 1, 'month', $3, $4, 1)`, ws, p.Key, now.Add(-time.Hour), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	wantCode(t, f.putPlanCurrency(e, p.Key, "usd"), 409, "plan_currency_locked")
	if got := f.storedCurrency(p.Key); got != "vnd" {
		t.Errorf("stored currency = %q after a refused change", got)
	}
	wantStatus(t, f.putPlanCurrency(e, p.Key, "vnd"), 200)
}

func TestDBCurrencyLockedWithBillingOrder(t *testing.T) {
	f := newDBFixture(t)
	e := f.dbConsole(t)
	p := f.plan(nil)
	f.dropPlanAudits(p.Key)
	wantStatus(t, f.putPlanCurrency(e, p.Key, "vnd"), 200)

	ws := f.workspace(nil)
	now := time.Now().UTC()
	if _, err := f.pool.Exec(context.Background(), `
		INSERT INTO billing_order (id, workspace_id, kind, plan_key, interval, seats, amount, vat_amount, currency,
			status, provider, transfer_code, expires_at, created_at)
		VALUES (gen_random_uuid(), $1, 'new', $2, 'month', 1, 100000, 10000, 'vnd', 'expired', 'manual', $3, $4, $5)`,
		ws, p.Key, "MUL"+hexSuffix(t), now.Add(time.Hour), now); err != nil {
		t.Fatal(err)
	}
	wantCode(t, f.putPlanCurrency(e, p.Key, "usd"), 409, "plan_currency_locked")
	if got := f.storedCurrency(p.Key); got != "vnd" {
		t.Errorf("stored currency = %q after a refused change", got)
	}
	wantStatus(t, f.putPlanCurrency(e, p.Key, "vnd"), 200)
}
