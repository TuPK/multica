package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/billingapi"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
	"github.com/multica-ai/multica/platform-management/internal/trust"
)

const (
	wsUnlimited = "55555555-5555-4555-8555-555555555555"
	orderID     = "66666666-6666-4666-8666-666666666666"
	appURL      = "https://app.example.com"
)

type members struct {
	count, reserved int64
}

func (m members) Role(context.Context, string, string) (string, bool, error) {
	return "owner", true, nil
}
func (m members) Count(context.Context, string) (int64, error)    { return m.count, nil }
func (m members) Reserved(context.Context, string) (int64, error) { return m.reserved, nil }

type pending map[string]subscription.PendingPurchase

func (p pending) PendingSeats(_ context.Context, ws string, _ time.Time) (subscription.PendingPurchase, bool, error) {
	v, ok := p[ws]
	return v, ok, nil
}

type orders struct{}

func (orders) CreateOrder(_ context.Context, d billing.Draft) (billing.Order, bool, error) {
	return billing.Order{ID: orderID, WorkspaceID: d.WorkspaceID, Kind: d.Kind, Status: billing.StatusPending}, true, nil
}

type slugs struct{}

func (slugs) Slug(context.Context, string) (string, bool, error) { return "acme", true, nil }

// billingMux serves the platform's seven billing routes over fakes that tell
// the stories of the fixtures: a free workspace with no subscription row, a
// paid workspace with a seat purchase waiting for its transfer, and a workspace
// on a contract plan with no seat limit.
func billingMux(t *testing.T) http.Handler {
	t.Helper()
	paidStart := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	paidEnd := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	subRows := subs{
		wsPaid: {
			WorkspaceID: wsPaid, PlanKey: "pro", Status: "active", Seats: 5, Interval: "month",
			CurrentPeriodStart: &paidStart, CurrentPeriodEnd: &paidEnd, Version: 7,
		},
		wsUnlimited: {WorkspaceID: wsUnlimited, PlanKey: "enterprise", Status: "active", Interval: "year", Version: 2},
	}
	plans := catalogue(t)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := members{count: 4}
	reads := subscription.NewAPI(subscription.APIDeps{
		Subscriptions: subRows, Plans: plans, Members: m,
		Pending: pending{wsPaid: {OrderID: orderID, Seats: 3, ExpiresAt: fixedNow.Add(7 * 24 * time.Hour)}},
		Now:     func() time.Time { return fixedNow }, Log: quiet,
	})
	writes, err := billingapi.New(billingapi.Deps{
		Subscriptions: subRows, Plans: plans, Members: m, Orders: orders{}, Workspaces: slugs{},
		VATRate: billing.DefaultVATRate, IndividualMaxSeats: 5, AppURL: appURL, Now: func() time.Time { return fixedNow }, Log: quiet,
	})
	if err != nil {
		t.Fatal(err)
	}
	peers := trust.New(trust.DefaultCIDRs())
	mux := http.NewServeMux()
	readHandler, writeHandler := reads.Handler(peers.RequireUser), writes.Handler(peers.RequireUser)
	for _, p := range subscription.APIPatterns {
		mux.Handle(p, readHandler)
	}
	for _, p := range billingapi.Patterns {
		mux.Handle(p, writeHandler)
	}
	return mux
}

// call sends one request as Multica's proxy would: from a trusted peer, with
// X-User-ID.
func call(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rd = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, rd)
	req.RemoteAddr = "10.0.0.7:4000"
	req.Header.Set(trust.UserHeader, userID)
	rec := httptest.NewRecorder()
	billingMux(t).ServeHTTP(rec, req)
	return rec
}

const subscriptions = "/api/v1/subscriptions/"

type obj = map[string]any

// billingCases are the responses the client's zod schemas read. Each of the
// seven routes has one; portal-sessions answers 404 and has no fixture.
var billingCases = []struct {
	file, method, path string
	body               obj
	status             int
}{
	{"summary_free.json", "GET", subscriptions + wsFree + "/summary", nil, 200},
	{"summary_paid_pending_purchase.json", "GET", subscriptions + wsPaid + "/summary", nil, 200},
	{"summary_unlimited_seats.json", "GET", subscriptions + wsUnlimited + "/summary", nil, 200},
	{"prices_plan_query.json", "GET", subscriptions + wsFree + "/prices?plan=business", nil, 200},
	{"checkout_session.json", "POST", subscriptions + "checkout-sessions",
		obj{"workspace_id": wsFree, "interval": "month", "idempotency_key": "k-checkout", "plan_key": "pro", "seats": 3}, 200},
	{"seat_purchase_preview.json", "POST", subscriptions + wsPaid + "/seats/purchase-preview", obj{"additional_seats": 3}, 200},
	{"seat_purchase.json", "POST", subscriptions + wsPaid + "/seats/purchases", obj{
		"additional_seats": 3, "expected_current_seats": 5, "expected_purchase_version": 7,
		// 149000 x 3 seats x 17 of 31 days left, rounded half up.
		"accepted_proration_amount": 245129, "currency": "vnd", "idempotency_key": "k-purchase",
	}, 200},
	{"seat_reconcile.json", "POST", subscriptions + wsPaid + "/seats/reconcile", nil, 200},
}

func TestBillingFixturesMatchThePlatformsResponses(t *testing.T) {
	for _, tc := range billingCases {
		t.Run(tc.file, func(t *testing.T) {
			rec := call(t, tc.method, tc.path, tc.body)
			if rec.Code != tc.status {
				t.Fatalf("status %d, want %d: %s", rec.Code, tc.status, rec.Body)
			}
			got := indent(t, rec.Body.Bytes())
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

func TestPortalSessionsAreNotServed(t *testing.T) {
	rec := call(t, "POST", subscriptions+wsPaid+"/portal-sessions", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404", rec.Code)
	}
}

// The fixtures tell the Appendix A edge cases the TypeScript test relies on, so
// one regenerated into something else is noticed here as well.
func TestBillingFixturesCarryTheirScenario(t *testing.T) {
	read := func(name string) obj {
		raw, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		var m obj
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	free := read("summary_free.json")["seat_capacity"].(obj)
	if free["version"] != float64(1) {
		t.Errorf("a workspace with no subscription row: seat_capacity.version = %v, want 1", free["version"])
	}
	if v, present := read("summary_unlimited_seats.json")["seat_capacity"]; !present || v != nil {
		t.Errorf("a workspace without a seat limit: seat_capacity = %#v, want null", v)
	}
	paid := read("summary_paid_pending_purchase.json")["seat_capacity"].(obj)
	active, _ := paid["active_purchase"].(obj)
	if paid["pending_quantity"] != float64(8) || active["status"] != "pending" || active["request_id"] != orderID {
		t.Errorf("a pending seat purchase: seat_capacity = %v", paid)
	}
	if read("seat_purchase_preview.json")["currency"] != "vnd" || read("seat_purchase.json")["currency"] != "vnd" {
		t.Error("currency must be lower case")
	}
	if read("checkout_session.json")["url"] != appURL+"/acme/billing/orders/"+orderID {
		t.Errorf("checkout url = %v", read("checkout_session.json")["url"])
	}
}
