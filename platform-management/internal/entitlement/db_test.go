package entitlement

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
	"github.com/multica-ai/multica/platform-management/internal/testdb"
)

// Database tests: skipped unless PLATFORM_TEST_DATABASE_URL names a *_test
// database and DATABASE_URL equals it. The workspaces and plans are the test's
// own; the free plan is read from the table (and added when the table lacks it,
// which is what the platform's start would do).

type dbFixture struct {
	t    *testing.T
	pool *pgxpool.Pool
	ws   []string
	keys []string
}

func newDBFixture(t *testing.T) *dbFixture {
	t.Helper()
	f := &dbFixture{t: t, pool: testdb.Open(t)}
	t.Cleanup(func() {
		ctx := context.Background()
		for _, ws := range f.ws {
			f.pool.Exec(ctx, `DELETE FROM workspace_subscription WHERE workspace_id = $1`, ws)
			f.pool.Exec(ctx, `DELETE FROM platform_workspace WHERE workspace_id = $1`, ws)
		}
		for _, k := range f.keys {
			f.pool.Exec(ctx, `DELETE FROM plan WHERE key = $1`, k)
		}
	})
	return f
}

func (f *dbFixture) newID() string {
	f.t.Helper()
	var id string
	if err := f.pool.QueryRow(context.Background(), `SELECT gen_random_uuid()::text`).Scan(&id); err != nil {
		f.t.Fatal(err)
	}
	f.ws = append(f.ws, id)
	return id
}

func (f *dbFixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(context.Background(), sql, args...); err != nil {
		f.t.Fatal(err)
	}
}

// freePlan returns the free plan of the table, adding the catalogue's when the
// table has none.
func (f *dbFixture) freePlan() plan.Plan {
	f.t.Helper()
	ctx := context.Background()
	r := plan.NewPgxReader(f.pool)
	if p, ok, err := r.Get(ctx, plan.FreeKey); err != nil {
		f.t.Fatal(err)
	} else if ok {
		return p
	}
	cat, err := plan.Catalogue()
	if err != nil {
		f.t.Fatal(err)
	}
	runner := plan.NewPgxRunner(f.pool)
	if err := runner.InTx(ctx, func(tx plan.Tx) error {
		_, err := tx.InsertIfAbsent(ctx, cat[0])
		return err
	}); err != nil {
		f.t.Fatal(err)
	}
	p, _, _ := r.Get(ctx, plan.FreeKey)
	return p
}

func (f *dbFixture) service() *Service {
	return NewService(NewPgxLoader(f.pool), time.Now, discard())
}

func randomKey(t *testing.T) string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return "t" + hex.EncodeToString(b)
}

func (f *dbFixture) fetch(svc *Service, ws string) Policy {
	f.t.Helper()
	rec := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/internal/entitlement-policies/"+ws, nil))
	if rec.Code != http.StatusOK {
		f.t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var p Policy
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		f.t.Fatal(err)
	}
	return p
}

func limitOf(g Gate) int64 {
	if g.Limit == nil {
		return -1
	}
	return *g.Limit
}

// A workspace with no subscription row is on the free plan at version 0.
func TestDBPolicyOfAWorkspaceWithNoRowIsTheFreePlan(t *testing.T) {
	f := newDBFixture(t)
	free := f.freePlan()
	ws := f.newID()

	p := f.fetch(f.service(), ws)
	if p.SubscriptionVersion != 0 || p.SchemaVersion != 1 || p.ValidForSeconds != 30 {
		t.Fatalf("policy = %+v", p)
	}
	for _, gate := range []string{plan.GateIssueCount, plan.GateAutopilotRuns} {
		want := free.Limits[gate]
		got := p.Gates[gate]
		if (want == nil) != (got.Limit == nil) || (want != nil && *want != *got.Limit) {
			t.Errorf("%s = %+v, want the free plan's %v", gate, got, want)
		}
	}
}

func TestDBPolicyFollowsTheSubscriptionAndWorkspaceRows(t *testing.T) {
	f := newDBFixture(t)
	f.freePlan()
	key := randomKey(t)
	f.keys = append(f.keys, key)
	f.exec(`INSERT INTO plan (key, name, currency, limits) VALUES ($1, 'DB test', 'vnd', '{"issue_count": null, "autopilot_runs": 2000}')`, key)
	ws := f.newID()
	f.exec(`INSERT INTO workspace_subscription (workspace_id, plan_key, status, version, limit_overrides)
	        VALUES ($1, $2, 'active', 7, '{"issue_count": 500, "autopilot_runs": null}')`, ws, key)

	svc := f.service()
	p := f.fetch(svc, ws)
	if p.SubscriptionVersion != 7 {
		t.Errorf("subscription_version = %d, want the row's 7", p.SubscriptionVersion)
	}
	if g := p.Gates[plan.GateIssueCount]; g.Action != ActionEnforce || limitOf(g) != 500 {
		t.Errorf("issue_count = %+v, want the override 500 over the plan's unlimited", g)
	}
	if g := p.Gates[plan.GateAutopilotRuns]; g.Action != ActionOff || g.PeriodStart == nil || g.PeriodEnd == nil {
		t.Errorf("autopilot_runs = %+v, want an explicit null override to lift 2000, with its period", g)
	}

	// A workspace that is not active may not start autopilot runs.
	f.exec(`INSERT INTO platform_workspace (workspace_id, status, reason) VALUES ($1, 'suspended', 'db test')`, ws)
	svc.Invalidate(ws)
	if g := f.fetch(svc, ws).Gates[plan.GateAutopilotRuns]; g.Action != ActionEnforce || limitOf(g) != 0 {
		t.Errorf("suspended: autopilot_runs = %+v, want enforce 0", g)
	}
	f.exec(`UPDATE platform_workspace SET status = 'active' WHERE workspace_id = $1`, ws)
	svc.Invalidate(ws)
	if g := f.fetch(svc, ws).Gates[plan.GateAutopilotRuns]; g.Action != ActionOff {
		t.Errorf("active again: autopilot_runs = %+v, want off", g)
	}
}

// A subscription on a plan that is not in the table is an error, not a guess.
func TestDBPolicyOfASubscriptionOnAMissingPlanIs500(t *testing.T) {
	f := newDBFixture(t)
	f.freePlan()
	ws := f.newID()
	f.exec(`INSERT INTO workspace_subscription (workspace_id, plan_key, status, version) VALUES ($1, 'no_such_plan', 'active', 1)`, ws)
	rec := httptest.NewRecorder()
	f.service().Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/internal/entitlement-policies/"+ws, nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500", rec.Code)
	}
}

// The staff write of the subscription package clears this process's cache, so
// the next policy shows the new version at once.
func TestDBStaffWriteIsVisibleInTheNextPolicy(t *testing.T) {
	f := newDBFixture(t)
	f.freePlan()
	key := randomKey(t)
	f.keys = append(f.keys, key)
	f.exec(`INSERT INTO plan (key, name, currency, limits) VALUES ($1, 'DB test', 'vnd', '{"issue_count": 10}')`, key)
	ws := f.newID()
	f.exec(`INSERT INTO workspace (id, name, slug) VALUES ($1, 'Policy DB test', $2)`, ws, "policy-db-"+key)
	t.Cleanup(func() { f.pool.Exec(context.Background(), `DELETE FROM audit_event WHERE workspace_id = $1`, ws) })
	t.Cleanup(func() { f.pool.Exec(context.Background(), `DELETE FROM workspace WHERE id = $1`, ws) })

	svc := f.service()
	before := f.fetch(svc, ws)
	staff := f.newID()
	if _, err := subscription.NewService(subscription.NewPgxRunner(f.pool), svc).Apply(context.Background(), subscription.Change{
		WorkspaceID: ws, PlanKey: key, Interval: subscription.IntervalMonth, Reason: "db test",
		LimitOverrides: plan.Limits{"issue_count": func() *int64 { n := int64(77); return &n }()},
		ActorID:        staff, ActorLabel: "billing@example.test",
	}); err != nil {
		t.Fatal(err)
	}
	after := f.fetch(svc, ws)
	if after.SubscriptionVersion != before.SubscriptionVersion+1 || limitOf(after.Gates[plan.GateIssueCount]) != 77 {
		t.Fatalf("before %+v\nafter %+v", before, after)
	}
}
