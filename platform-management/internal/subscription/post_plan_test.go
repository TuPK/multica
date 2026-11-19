package subscription

import (
	"context"
	"strings"
	"testing"
)

// Spec 7b.4 rule 2 and 13: POST /plans creates a plan, private ones included,
// with its initial prices; a key that exists is a 409.
func TestPostPlanCreatesAPlan(t *testing.T) {
	f := newDBFixture(t)
	e := f.dbConsole(t)
	key := "n" + hexSuffix(t)
	f.planKey = append(f.planKey, key)
	f.dropPlanAudits(key)
	body := func(key string) string {
		return `{"key":"` + key + `","name":"Acme","limits":{"issue_count":500},"features":{"sso":true},
"currency":"vnd","seat_price_month":120000,"seat_price_year":1200000,"min_seats":2,
"trial_days":0,"is_public":false,"sort":40,"audience":"business","billing_model":"seat"}`
	}

	wantStatus(t, e.do(adminUser, "POST", "/api/platform/plans", body(key)), 201)
	var audience, model, currency string
	var month int64
	var public bool
	var version int64
	if err := f.pool.QueryRow(context.Background(),
		`SELECT audience, billing_model, currency, seat_price_month, is_public, version FROM plan WHERE key = $1`, key,
	).Scan(&audience, &model, &currency, &month, &public, &version); err != nil {
		t.Fatalf("plan not stored: %v", err)
	}
	if audience != "business" || model != "seat" || currency != "vnd" || month != 120000 || public || version != 0 {
		t.Fatalf("stored = %s %s %s %d public=%v version=%d", audience, model, currency, month, public, version)
	}
	if n := f.count(`SELECT count(*) FROM audit_event WHERE action = 'plan.changed' AND target_type = 'plan' AND target_id = $1 AND detail->>'created' = 'true'`, key); n != 1 {
		t.Fatalf("plan.changed created rows = %d, want 1", n)
	}

	again := strings.Replace(body(key), `"Acme"`, `"Other"`, 1)
	wantCode(t, e.do(adminUser, "POST", "/api/platform/plans", again), 409, "plan_exists")
	if name := f.planName(key); name != "Acme" {
		t.Fatalf("a refused POST changed the plan name to %q", name)
	}

	wantCode(t, e.do(adminUser, "POST", "/api/platform/plans", body("Bad-Key")), 400, "invalid_plan")
	wantCode(t, e.do(adminUser, "POST", "/api/platform/plans", strings.Replace(body(key+"x"), `"vnd"`, `"usd"`, 1)), 400, "price_invalid")
	wantStatus(t, e.do(suppUser, "POST", "/api/platform/plans", body(key+"y")), 404)
}

func (f *dbFixture) planName(key string) string {
	f.t.Helper()
	var n string
	if err := f.pool.QueryRow(context.Background(), `SELECT name FROM plan WHERE key = $1`, key).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}
