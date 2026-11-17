package subscription

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/trust"
)

type fakeUsage struct {
	usage Usage
	calls int
}

func (f *fakeUsage) Usage(context.Context, string, time.Time) (Usage, error) {
	f.calls++
	return f.usage, nil
}

func usageEnv(t *testing.T) (*apiEnv, *fakeUsage) {
	t.Helper()
	e := newAPIEnv(t)
	e.plans.plans = append(e.plans.plans, plan.Plan{
		Key: "payg", Name: "Pay as you go", Currency: "vnd", IsPublic: true, Audience: plan.AudienceAny,
		BillingModel: plan.BillingUsage, Limits: plan.Limits{}, Features: map[string]bool{},
	})
	u := &fakeUsage{usage: Usage{
		PeriodStart: "2026-03-01T00:00:00Z", PeriodEnd: "2026-04-01T00:00:00Z", Currency: "vnd", AccruedAmount: 12345,
		Meters: []UsageMeter{{Key: "tasks_run", Quantity: 5, UnitPrice: 30000, Per: 1000, Amount: 150}},
	}}
	api := NewAPI(APIDeps{
		Subscriptions: e.subs, Plans: e.plans, Members: e.members, Pending: e.pending, Usage: u,
		Now: func() time.Time { return t0 },
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	e.h = api.Handler(trust.New(trust.DefaultCIDRs()).RequireUser)
	return e, u
}

func decodeRaw(t *testing.T, rec interface{ Result() *http.Response }) map[string]json.RawMessage {
	t.Helper()
	var out map[string]json.RawMessage
	if err := json.NewDecoder(rec.Result().Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSummaryUsageBillingModel(t *testing.T) {
	e, u := usageEnv(t)

	// A seat plan: billing_model seat, no usage object.
	rec := e.summary(ownerID)
	if rec.Code != http.StatusOK {
		t.Fatalf("summary: %d %s", rec.Code, rec.Body)
	}
	body := decodeRaw(t, rec)
	if string(body["billing_model"]) != `"seat"` {
		t.Errorf("billing_model = %s, want \"seat\"", body["billing_model"])
	}
	if _, has := body["usage"]; has {
		t.Errorf("a seat plan's summary has a usage object: %s", body["usage"])
	}
	if u.calls != 0 {
		t.Errorf("usage reads for a seat plan = %d, want 0", u.calls)
	}

	// A usage plan: billing_model usage and the usage block; no seats to manage.
	e.subs.rows[wsID] = Subscription{WorkspaceID: wsID, PlanKey: "payg", Status: StatusActive, Interval: IntervalMonth, Version: 2}
	rec = e.summary(ownerID)
	if rec.Code != http.StatusOK {
		t.Fatalf("usage summary: %d %s", rec.Code, rec.Body)
	}
	body = decodeRaw(t, rec)
	if string(body["billing_model"]) != `"usage"` {
		t.Errorf("billing_model = %s, want \"usage\"", body["billing_model"])
	}
	want, _ := json.Marshal(u.usage)
	if string(body["usage"]) != string(want) {
		t.Errorf("usage = %s, want %s", body["usage"], want)
	}
	if string(body["seat_capacity"]) != "null" {
		t.Errorf("seat_capacity = %s, want null", body["seat_capacity"])
	}
}

func TestSummaryUsageRouteServesTheBlockOfAUsagePlanOnly(t *testing.T) {
	e, u := usageEnv(t)
	path := "/api/v1/subscriptions/" + wsID + "/usage"

	rec := e.get(path, ownerID, "10.0.0.7:4000")
	if rec.Code != http.StatusOK || rec.Body.String() != "{\"billing_model\":\"seat\",\"usage\":null}\n" {
		t.Fatalf("seat plan: %d %q, want billing_model seat and a null usage", rec.Code, rec.Body)
	}

	e.subs.rows[wsID] = Subscription{WorkspaceID: wsID, PlanKey: "payg", Status: StatusActive, Interval: IntervalMonth, Version: 2}
	rec = e.get(path, adminID, "10.0.0.7:4000")
	if rec.Code != http.StatusOK {
		t.Fatalf("usage plan: %d %s", rec.Code, rec.Body)
	}
	body := decodeRaw(t, rec)
	want, _ := json.Marshal(u.usage)
	if string(body["billing_model"]) != `"usage"` || string(body["usage"]) != string(want) {
		t.Errorf("body = %s, want billing_model usage and the usage block %s", rec.Body, want)
	}

	// The same access rule as the summary.
	if rec := e.get(path, memberID, "10.0.0.7:4000"); rec.Code != http.StatusForbidden {
		t.Errorf("member: %d, want 403", rec.Code)
	}
	if rec := e.get(path, strangID, "10.0.0.7:4000"); rec.Code != http.StatusNotFound {
		t.Errorf("non-member: %d, want 404", rec.Code)
	}
	if rec := e.get(path, "", "10.0.0.7:4000"); rec.Code == http.StatusOK {
		t.Errorf("no user: %d, want a refusal", rec.Code)
	}
	if rec := e.get(path, ownerID, "203.0.113.9:1"); rec.Code != http.StatusForbidden {
		t.Errorf("untrusted peer: %d, want 403", rec.Code)
	}
}

// The billing profile is never part of the summary (spec 13, AC25): the summary
// has no profile reader, and no key or value of the profile may appear in it.
func TestSummaryNeverCarriesTheBillingProfile(t *testing.T) {
	e, _ := usageEnv(t)
	for _, planKey := range []string{"free", "payg"} {
		e.subs.rows[wsID] = Subscription{
			WorkspaceID: wsID, PlanKey: planKey, Status: "active", Interval: IntervalMonth, Version: 1,
		}
		rec := e.summary(ownerID)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s summary: %d %s", planKey, rec.Code, rec.Body)
		}
		body := strings.ToLower(rec.Body.String())
		for _, key := range []string{"legal_name", "tax_code", "invoice_email", "address", "billing_profile", "buyer"} {
			if strings.Contains(body, `"`+key+`"`) {
				t.Errorf("%s summary has a %q key: %s", planKey, key, rec.Body)
			}
		}
	}
}
