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
