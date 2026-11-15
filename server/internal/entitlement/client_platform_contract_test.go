package entitlement

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/cloudruntime"
)

// These tests are Multica's half of the contract with the platform service
// (platform-management). The platform writes its real responses to
// platform-management/contract/testdata with `go test ./contract -update`; here
// the unchanged client reads the same files from an httptest server. No database
// is involved.

const platformFixtureDir = "../../../platform-management/contract/testdata"

var platformPolicyFixtures = []string{"policy_free.json", "policy_paid_overrides.json", "policy_non_active.json"}

func platformFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(platformFixtureDir, name))
	if err != nil {
		t.Fatalf("%v\nthe platform writes this file: cd platform-management && go test ./contract -update", err)
	}
	return raw
}

// platformServer serves body as the platform would, on the path the client
// calls, and counts the requests.
func platformServer(t *testing.T, workspaceID uuid.UUID, body []byte) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/internal/entitlement-policies/"+workspaceID.String() {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		// The platform serves this route with no credential, as the client sends none.
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-User-ID") != "" {
			t.Errorf("the client sent a credential: %v", r.Header)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func platformClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	c, err := New(Config{BaseURL: baseURL})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func limitValue(t *testing.T, g Gate) int {
	t.Helper()
	if g.Limit == nil {
		t.Fatalf("gate %+v has no limit", g)
	}
	return *g.Limit
}

// normalizePolicy, the client's own validation, accepts every policy the
// platform serves.
func TestPlatformPolicyFixturesPassNormalizePolicy(t *testing.T) {
	for _, name := range platformPolicyFixtures {
		var wire wirePolicy
		if err := json.Unmarshal(platformFixture(t, name), &wire); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		fetched, err := normalizePolicy(wire)
		if err != nil {
			t.Errorf("%s: normalizePolicy: %v", name, err)
			continue
		}
		if fetched.validForSeconds != 30 {
			t.Errorf("%s: valid_for_seconds = %d, want 30", name, fetched.validForSeconds)
		}
		if fetched.snapshot.policyRevision <= 0 || fetched.snapshot.cloudValidUntil.IsZero() {
			t.Errorf("%s: snapshot = %+v", name, fetched.snapshot)
		}
		if len(wire.Gates) != 2 {
			t.Errorf("%s: %d gates, want the two the client reads", name, len(wire.Gates))
		}
	}
}

func TestPlatformFreePolicyGatesBehaveAsBefore(t *testing.T) {
	ws := uuid.New()
	srv, hits := platformServer(t, ws, platformFixture(t, "policy_free.json"))
	client := platformClient(t, srv.URL)
	ctx := context.Background()

	issues := client.Gate(ctx, ws, GateIssueCount)
	if issues.Gate.Action != ActionEnforce || limitValue(t, issues.Gate) != 200 || issues.Reason != ReasonRefreshed ||
		issues.SubscriptionVersion != 0 || issues.PolicyRevision <= 0 {
		t.Fatalf("issue_count = %+v", issues)
	}
	if issues.Gate.PeriodStart != nil || issues.Gate.Notifications != nil {
		t.Errorf("issue_count carries a period or notice: %+v", issues.Gate)
	}

	runs := client.Gate(ctx, ws, GateAutopilotRuns)
	if runs.Gate.Action != ActionEnforce || limitValue(t, runs.Gate) != 50 || runs.Reason != ReasonCacheFresh {
		t.Fatalf("autopilot_runs = %+v", runs)
	}
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	g := runs.Gate
	if g.PeriodStart == nil || !g.PeriodStart.Equal(start) || g.PeriodEnd == nil || !g.PeriodEnd.Equal(end) || g.ResetAt == nil || !g.ResetAt.Equal(end) {
		t.Errorf("period = %v %v %v, want the UTC calendar month of August 2026", g.PeriodStart, g.PeriodEnd, g.ResetAt)
	}
	if g.Notifications == nil || g.Notifications.OnRejection != NotificationFirstRejectionPerPeriod {
		t.Errorf("notifications = %+v", g.Notifications)
	}

	// valid_for_seconds is honoured: both gates came from one request.
	if hits.Load() != 1 {
		t.Errorf("requests = %d, want 1", hits.Load())
	}
}

func TestPlatformPaidPolicyWithOverrides(t *testing.T) {
	ws := uuid.New()
	srv, _ := platformServer(t, ws, platformFixture(t, "policy_paid_overrides.json"))
	client := platformClient(t, srv.URL)
	ctx := context.Background()

	// The plan's unlimited issue_count is an off gate.
	issues := client.Gate(ctx, ws, GateIssueCount)
	if issues.Gate.Action != ActionOff || issues.Gate.Limit != nil || issues.Reason != ReasonRefreshed || issues.SubscriptionVersion != 7 {
		t.Fatalf("issue_count = %+v, want off at version 7", issues)
	}
	runs := client.Gate(ctx, ws, GateAutopilotRuns)
	if runs.Gate.Action != ActionEnforce || limitValue(t, runs.Gate) != 3000 || runs.SubscriptionVersion != 7 {
		t.Fatalf("autopilot_runs = %+v, want the override 3000", runs)
	}
	if runs.Gate.PeriodStart == nil || runs.Gate.PeriodEnd == nil || runs.Gate.ResetAt == nil {
		t.Errorf("autopilot_runs has no period: %+v", runs.Gate)
	}
}

// A zero limit is a valid enforce gate: every run is refused.
func TestPlatformNonActiveWorkspaceBlocksAutopilotRuns(t *testing.T) {
	ws := uuid.New()
	srv, _ := platformServer(t, ws, platformFixture(t, "policy_non_active.json"))
	client := platformClient(t, srv.URL)
	ctx := context.Background()

	runs := client.Gate(ctx, ws, GateAutopilotRuns)
	if runs.Gate.Action != ActionEnforce || limitValue(t, runs.Gate) != 0 || runs.Reason != ReasonRefreshed {
		t.Fatalf("autopilot_runs = %+v, want enforce 0", runs)
	}
	if runs.Gate.PeriodStart == nil || runs.Gate.Notifications != nil {
		t.Errorf("autopilot_runs = %+v, want a period and no quota notice", runs.Gate)
	}
	if issues := client.Gate(ctx, ws, GateIssueCount); issues.Gate.Action != ActionOff {
		t.Errorf("issue_count = %+v, want the plan's (unlimited) gate", issues)
	}
}

// A policy without valid_until is not decoded: normalizePolicy refuses a zero
// valid_until, and the client then fails open, so the gate silently stops
// enforcing. The platform therefore always sends valid_until. This test shows what
// a platform bug in that one field would cost.
func TestPlatformPolicyWithoutValidUntilIsRefusedAndTheGateFailsOpen(t *testing.T) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(platformFixture(t, "policy_free.json"), &doc); err != nil {
		t.Fatal(err)
	}
	if _, ok := doc["valid_until"]; !ok {
		t.Fatal("the fixture has no valid_until to remove")
	}
	delete(doc, "valid_until")
	body, _ := json.Marshal(doc)

	var wire wirePolicy
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatalf("the JSON itself decodes: %v", err)
	}
	if _, err := normalizePolicy(wire); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("normalizePolicy = %v, want ErrInvalidPolicy", err)
	}

	ws := uuid.New()
	srv, _ := platformServer(t, ws, body)
	for _, gate := range []GateName{GateIssueCount, GateAutopilotRuns} {
		d := platformClient(t, srv.URL).Gate(context.Background(), ws, gate)
		if d.Gate.Action != ActionOff || d.Reason != ReasonInvalidPolicy {
			t.Errorf("%s = %+v, want off with %s", gate, d, ReasonInvalidPolicy)
		}
	}
}

// The client requires each of these fields; without one it refuses the policy
// and fails open. The fixtures carry them all, so a platform that stops sending
// one fails the platform's own comparison and this test.
func TestPlatformPolicyFixturesHaveEveryRequiredField(t *testing.T) {
	for _, name := range platformPolicyFixtures {
		var doc map[string]json.RawMessage
		if err := json.Unmarshal(platformFixture(t, name), &doc); err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"schema_version", "policy_revision", "valid_until", "valid_for_seconds", "gates"} {
			mutated := map[string]json.RawMessage{}
			for k, v := range doc {
				if k != field {
					mutated[k] = v
				}
			}
			body, _ := json.Marshal(mutated)
			var without wirePolicy
			if err := json.Unmarshal(body, &without); err != nil {
				t.Fatal(err)
			}
			// subscription_version is not listed: 0 is its zero value, so the
			// client cannot tell it from an omitted one. Every other field is
			// required.
			if _, err := normalizePolicy(without); !errors.Is(err, ErrInvalidPolicy) {
				t.Errorf("%s without %s: normalizePolicy = %v, want ErrInvalidPolicy", name, field, err)
			}
		}
	}
}

// The client holds off a policy whose subscription_version goes backwards, so the
// platform's version is the subscription row's, and only ever rises. The fixtures
// are the free workspace with no row (0) and two with rows.
func TestPlatformPolicyFixtureVersions(t *testing.T) {
	want := map[string]int64{"policy_free.json": 0, "policy_paid_overrides.json": 7, "policy_non_active.json": 2}
	for name, version := range want {
		var wire wirePolicy
		if err := json.Unmarshal(platformFixture(t, name), &wire); err != nil {
			t.Fatal(err)
		}
		if wire.SubscriptionVersion != version {
			t.Errorf("%s: subscription_version = %d, want %d", name, wire.SubscriptionVersion, version)
		}
	}
}

var lowerCurrency = regexp.MustCompile(`^[a-z]{3}$`)

// The prices fixture goes through the existing Cloud client the way the billing
// proxy calls it, and must satisfy the constraints of the page's schema
// (packages/core/api/schemas.ts, WorkspaceSubscriptionPricesSchema).
func TestPlatformPricesFixtureThroughTheCloudClient(t *testing.T) {
	const ws = "22222222-2222-4222-8222-222222222222"
	const user = "44444444-4444-4444-8444-444444444444"
	body := platformFixture(t, "prices.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/subscriptions/"+ws+"/prices" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		// The platform reads the user from this header, and only from a trusted peer.
		if got := r.Header.Get("X-User-ID"); got != user {
			t.Errorf("X-User-ID = %q, want %q", got, user)
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("the Cloud client sent an Authorization header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	resp, err := cloudruntime.NewClient(cloudruntime.Config{BaseURL: srv.URL}).Do(context.Background(), cloudruntime.Request{
		Method: http.MethodGet, Path: "/api/v1/subscriptions/" + ws + "/prices", UserID: user, Op: "billing",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(resp.Body, &raw); err != nil {
		t.Fatal(err)
	}
	amounts := map[string]int64{}
	for _, interval := range []string{"month", "year"} {
		var p priceWire
		if err := json.Unmarshal(raw[interval], &p); err != nil {
			t.Fatalf("%s: %v", interval, err)
		}
		if !lowerCurrency.MatchString(p.Currency) || p.UnitAmount <= 0 || p.Interval != interval || p.IntervalCount != 1 {
			t.Errorf("%s = %+v, want a positive amount, the same interval and an interval_count of 1", interval, p)
		}
		amounts[interval] = p.UnitAmount
	}
	if amounts["year"] <= amounts["month"] {
		t.Errorf("year %d is not above month %d", amounts["year"], amounts["month"])
	}
}

// priceWire holds the fields of one price that the page's schema checks.
type priceWire struct {
	Currency      string `json:"currency"`
	UnitAmount    int64  `json:"unit_amount"`
	Interval      string `json:"interval"`
	IntervalCount int    `json:"interval_count"`
}
