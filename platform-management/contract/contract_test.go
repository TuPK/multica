package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/entitlement"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
	"github.com/multica-ai/multica/platform-management/internal/trust"
	"github.com/multica-ai/multica/platform-management/internal/usage"
)

var update = flag.Bool("update", false, "rewrite the fixtures in testdata from the handlers' output")

const (
	wsFree      = "11111111-1111-4111-8111-111111111111"
	wsPaid      = "22222222-2222-4222-8222-222222222222"
	wsSuspended = "33333333-3333-4333-8333-333333333333"
	userID      = "44444444-4444-4444-8444-444444444444"
)

// fixedNow is inside August 2026, so the autopilot period is 1 August to 1
// September.
var fixedNow = time.Date(2026, 8, 15, 10, 30, 45, 0, time.UTC)

type plans struct{ list []plan.Plan }

func (p plans) Get(_ context.Context, key string) (plan.Plan, bool, error) {
	for _, x := range p.list {
		if x.Key == key {
			return x, true, nil
		}
	}
	return plan.Plan{}, false, nil
}
func (p plans) List(context.Context) ([]plan.Plan, error) { return p.list, nil }

type subs map[string]subscription.Subscription

func (s subs) Get(_ context.Context, ws string) (subscription.Subscription, bool, error) {
	v, ok := s[ws]
	return v, ok, nil
}

// Profile makes subs a billingapi.SubscriptionReader: no row is an individual workspace.
func (s subs) Profile(context.Context, string) (subscription.BillingProfile, bool, error) {
	return subscription.BillingProfile{}, false, nil
}

type statuses map[string]string

func (s statuses) Status(_ context.Context, ws string) (string, error) {
	if v, ok := s[ws]; ok {
		return v, nil
	}
	return "active", nil
}

func limit(n int64) *int64 { return &n }

func catalogue(t *testing.T) plans {
	t.Helper()
	all, err := plan.Catalogue()
	if err != nil {
		t.Fatal(err)
	}
	return plans{all}
}

// policyBody returns the platform's response for one workspace.
func policyBody(t *testing.T, ws string) []byte {
	t.Helper()
	loader := entitlement.Loader{
		Plans: catalogue(t),
		Subscriptions: subs{
			// A paid workspace: the Pro plan, with a higher autopilot limit than
			// the plan's 2000. The plan's issue_count is unlimited, so the policy
			// carries one enforce gate and one off gate.
			wsPaid: {
				WorkspaceID: wsPaid, PlanKey: "pro", Status: "active", Seats: 5, Interval: "month", Version: 7,
				LimitOverrides: plan.Limits{"autopilot_runs": limit(3000), "agents": limit(5)},
			},
			wsSuspended: {WorkspaceID: wsSuspended, PlanKey: "pro", Status: "active", Seats: 2, Interval: "month", Version: 2},
		},
		Status: statuses{wsSuspended: "suspended"},
	}
	svc := entitlement.NewService(loader, func() time.Time { return fixedNow }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	rec := httptest.NewRecorder()
	svc.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/internal/entitlement-policies/"+ws, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("policy of %s: status %d: %s", ws, rec.Code, rec.Body)
	}
	return rec.Body.Bytes()
}

// pricesBody returns the platform's prices response, as Multica's proxy would
// ask for it: from a trusted peer, with X-User-ID.
func pricesBody(t *testing.T) []byte {
	t.Helper()
	api := subscription.NewAPI(subscription.APIDeps{
		Subscriptions: subs{}, Plans: catalogue(t), Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}).Handler(trust.New(trust.DefaultCIDRs()).RequireUser)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions/"+wsPaid+"/prices", nil)
	req.RemoteAddr = "10.0.0.7:4000"
	req.Header.Set(trust.UserHeader, userID)
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("prices: status %d: %s", rec.Code, rec.Body)
	}
	return rec.Body.Bytes()
}

// indent is how a fixture is stored: two-space JSON with a final newline.
func indent(t *testing.T, raw []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := json.Indent(&out, bytes.TrimSpace(raw), "", "  "); err != nil {
		t.Fatalf("response is not JSON: %v\n%s", err, raw)
	}
	out.WriteByte('\n')
	return out.Bytes()
}

func TestFixturesMatchThePlatformsResponses(t *testing.T) {
	cases := []struct {
		file string
		body func(*testing.T) []byte
	}{
		{"policy_free.json", func(t *testing.T) []byte { return policyBody(t, wsFree) }},
		{"policy_paid_overrides.json", func(t *testing.T) []byte { return policyBody(t, wsPaid) }},
		{"policy_non_active.json", func(t *testing.T) []byte { return policyBody(t, wsSuspended) }},
		{"prices.json", pricesBody},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			got := indent(t, tc.body(t))
			path := filepath.Join("testdata", tc.file)
			if *update {
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v\nrun: go test ./contract -update", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("%s differs from the platform's response.\nrun: go test ./contract -update, and review the diff.\n--- fixture\n%s\n--- response\n%s", tc.file, want, got)
			}
		})
	}
}

// The three policies tell the story the Multica side asserts, so a fixture that
// was regenerated into something else is noticed here.
func TestFixturesCarryTheirScenario(t *testing.T) {
	type gate struct {
		Action string `json:"action"`
		Limit  *int64 `json:"limit"`
	}
	type policy struct {
		SubscriptionVersion int64           `json:"subscription_version"`
		ValidUntil          string          `json:"valid_until"`
		ValidForSeconds     int64           `json:"valid_for_seconds"`
		Gates               map[string]gate `json:"gates"`
	}
	read := func(name string) policy {
		raw, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		var p policy
		if err := json.Unmarshal(raw, &p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	check := func(name string, version int64, issue, autopilot gate) {
		t.Helper()
		p := read(name)
		if p.SubscriptionVersion != version || p.ValidUntil == "" || p.ValidForSeconds != 30 {
			t.Errorf("%s: header = %+v", name, p)
		}
		for gateName, want := range map[string]gate{"issue_count": issue, "autopilot_runs": autopilot} {
			got := p.Gates[gateName]
			if got.Action != want.Action || (want.Limit == nil) != (got.Limit == nil) || (want.Limit != nil && *got.Limit != *want.Limit) {
				t.Errorf("%s: %s = %+v, want %+v", name, gateName, got, want)
			}
		}
	}
	check("policy_free.json", 0, gate{"enforce", limit(200)}, gate{"enforce", limit(50)})
	check("policy_paid_overrides.json", 7, gate{"off", nil}, gate{"enforce", limit(3000)})
	check("policy_non_active.json", 2, gate{"off", nil}, gate{"enforce", limit(0)})
}

const (
	wsUsage     = "77777777-7777-4777-8777-777777777777"
	usageOrder  = "dddddddd-0000-4000-8000-0000000000a1"
	gibibyte    = int64(1) << 30
	usageLimit  = 2000000
	usageAccrue = 1234567
)

// paygPlan is the pay as you go plan of spec 7b.3. The initial catalogue does
// not carry it yet, so the fixtures build it from the table of that section.
func paygPlan() plan.Plan {
	return plan.Plan{
		Key: "payg", Name: "Pay as you go", Currency: "vnd", IsPublic: true, Audience: plan.AudienceAny,
		BillingModel: plan.BillingUsage, Limits: plan.Limits{}, Features: map[string]bool{},
	}
}

// fixedUsage answers with the usage block of August 2026, assembled by the same
// function the database reader uses, from fixed rows.
type fixedUsage struct{}

func (fixedUsage) Usage(_ context.Context, _ string, now time.Time) (subscription.Usage, error) {
	row := func(d int, meter string, qty, price, per, net int64) usage.ChargeRow {
		return usage.ChargeRow{
			Day: time.Date(2026, 8, d, 0, 0, 0, 0, time.UTC), Meter: meter, Quantity: qty,
			UnitPrice: price, Per: per, DiscountPct: 20, NetAmount: net,
		}
	}
	return usage.BuildUsage(usage.UsageInput{
		Now: now, Currency: "vnd", DiscountPct: 20, CreditLimit: usageLimit, Accrued: usageAccrue, UnpaidOrderID: usageOrder,
		Rates: usage.Rates{
			usage.MeterTasksRun:     {Per: 1000, Price: 30000},
			usage.MeterStorageBytes: {Per: gibibyte, Price: 3000},
			usage.MeterMembers:      {Per: 1, Price: 49000},
		},
		Rows: []usage.ChargeRow{
			row(13, usage.MeterTasksRun, 1200, 30000, 1000, 28800),
			row(14, usage.MeterTasksRun, 800, 30000, 1000, 19200),
			row(14, usage.MeterStorageBytes, 2*gibibyte, 3000, gibibyte, 155),
			row(14, usage.MeterMembers, 10, 49000, 1, 12645),
		},
	}), nil
}

// usageBody returns the platform's response to GET path for a workspace on the
// pay as you go plan.
func usageBody(t *testing.T, path string) []byte {
	t.Helper()
	api := subscription.NewAPI(subscription.APIDeps{
		Subscriptions: subs{wsUsage: {WorkspaceID: wsUsage, PlanKey: "payg", Status: "active", Interval: "month", Version: 3}},
		Plans:         plans{append(catalogue(t).list, paygPlan())},
		Members:       members{count: 4}, Usage: fixedUsage{},
		Now: func() time.Time { return fixedNow }, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}).Handler(trust.New(trust.DefaultCIDRs()).RequireUser)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions/"+wsUsage+path, nil)
	req.RemoteAddr = "10.0.0.7:4000"
	req.Header.Set(trust.UserHeader, userID)
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: status %d: %s", path, rec.Code, rec.Body)
	}
	return rec.Body.Bytes()
}

// The usage block of a pay as you go workspace, as the summary and GET /usage
// carry it. The Multica client's schemas read these two files.
func TestFixturesUsageSummaryAndUsage(t *testing.T) {
	for _, tc := range []struct{ file, path string }{
		{"usage.json", "/usage"},
		{"summary_usage.json", "/summary"},
	} {
		t.Run(tc.file, func(t *testing.T) {
			got := indent(t, usageBody(t, tc.path))
			path := filepath.Join("testdata", tc.file)
			if *update {
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v\nrun: go test ./contract -update", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("%s differs from the platform's response.\nrun: go test ./contract -update, and review the diff.\n--- fixture\n%s\n--- response\n%s", tc.file, want, got)
			}
		})
	}
}
