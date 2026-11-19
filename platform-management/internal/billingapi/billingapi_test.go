package billingapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
	"github.com/multica-ai/multica/platform-management/internal/trust"
)

const (
	wsID     = "bbbbbbbb-0000-0000-0000-000000000001"
	otherWS  = "bbbbbbbb-0000-0000-0000-0000000000ff"
	ownerID  = "aaaaaaaa-1111-0000-0000-000000000001"
	adminID  = "aaaaaaaa-1111-0000-0000-000000000002"
	memberID = "aaaaaaaa-1111-0000-0000-000000000003"
	strangID = "aaaaaaaa-1111-0000-0000-000000000004"
	appURL   = "https://app.example.com"
)

// now is 10 days into a 30-day period, so 20 of 30 days are left.
var (
	periodStart = time.Date(2030, 5, 1, 0, 0, 0, 0, time.UTC)
	periodEnd   = time.Date(2030, 5, 31, 0, 0, 0, 0, time.UTC)
	now         = time.Date(2030, 5, 11, 0, 0, 0, 0, time.UTC)
)

type fakeMembers struct {
	roles map[string]string
	count int64
	err   error
	calls int
}

func (f *fakeMembers) Role(_ context.Context, ws, user string) (string, bool, error) {
	f.calls++
	if f.err != nil {
		return "", false, f.err
	}
	r, ok := f.roles[ws+"/"+user]
	return r, ok, nil
}
func (f *fakeMembers) Count(context.Context, string) (int64, error)    { return f.count, f.err }
func (f *fakeMembers) Reserved(context.Context, string) (int64, error) { return 0, nil }

type fakeSubs struct {
	rows map[string]subscription.Subscription
	// profiles are the billing profiles; a workspace without one is individual.
	profiles map[string]subscription.BillingProfile
	err      error
}

func (f *fakeSubs) Profile(_ context.Context, ws string) (subscription.BillingProfile, bool, error) {
	p, ok := f.profiles[ws]
	return p, ok, f.err
}

func (f *fakeSubs) Get(_ context.Context, ws string) (subscription.Subscription, bool, error) {
	s, ok := f.rows[ws]
	return s, ok, f.err
}

type fakePlans struct{ plans []plan.Plan }

func (f *fakePlans) Get(_ context.Context, key string) (plan.Plan, bool, error) {
	for _, p := range f.plans {
		if p.Key == key {
			return p, true, nil
		}
	}
	return plan.Plan{}, false, nil
}
func (f *fakePlans) List(context.Context) ([]plan.Plan, error) { return f.plans, nil }

// fakeOrders keeps what CreateOrder was asked for and repeats an order for a
// repeated idempotency key, as the real service does.
type fakeOrders struct {
	drafts []billing.Draft
	byKey  map[string]billing.Order
	err    error
}

func (f *fakeOrders) CreateOrder(_ context.Context, d billing.Draft) (billing.Order, bool, error) {
	if f.err != nil {
		return billing.Order{}, false, f.err
	}
	if o, ok := f.byKey[d.WorkspaceID+"/"+d.IdempotencyKey]; ok && d.IdempotencyKey != "" {
		return o, false, nil
	}
	f.drafts = append(f.drafts, d)
	o := billing.Order{
		ID:          "dddddddd-0000-0000-0000-00000000000" + string(rune('0'+len(f.drafts))),
		WorkspaceID: d.WorkspaceID, Kind: d.Kind, PlanKey: d.PlanKey, Interval: d.Interval, Seats: d.Seats,
		Amount: d.Amount, VATAmount: d.VATAmount, Currency: d.Currency, Status: billing.StatusPending,
		Provider: billing.ProviderManual, TransferCode: "MULABCDEFGH", IdempotencyKey: d.IdempotencyKey,
		CreatedBy: d.CreatedBy,
	}
	if f.byKey == nil {
		f.byKey = map[string]billing.Order{}
	}
	f.byKey[d.WorkspaceID+"/"+d.IdempotencyKey] = o
	return o, true, nil
}

type fakeWorkspaces struct {
	slugs map[string]string
	err   error
}

func (f *fakeWorkspaces) Slug(_ context.Context, ws string) (string, bool, error) {
	s, ok := f.slugs[ws]
	return s, ok, f.err
}

// fakeDebts holds the unpaid usage invoice of each workspace.
type fakeDebts struct{ unpaid map[string]string }

func (f *fakeDebts) UnpaidUsageOrder(_ context.Context, ws string) (string, bool, error) {
	id, ok := f.unpaid[ws]
	return id, ok, nil
}

type env struct {
	debts       *fakeDebts
	members     *fakeMembers
	subs        *fakeSubs
	plans       *fakePlans
	orders      *fakeOrders
	spaces      *fakeWorkspaces
	changes     *fakeChanger
	changeOrder *fakeChangeOrders
	h           http.Handler
}

func newEnv(t *testing.T) *env {
	t.Helper()
	all, err := plan.Catalogue()
	if err != nil {
		t.Fatal(err)
	}
	e := &env{
		members: &fakeMembers{
			roles: map[string]string{
				wsID + "/" + ownerID: "owner", wsID + "/" + adminID: "admin", wsID + "/" + memberID: "member",
			},
			count: 2,
		},
		subs:        &fakeSubs{rows: map[string]subscription.Subscription{}},
		plans:       &fakePlans{plans: all},
		orders:      &fakeOrders{},
		spaces:      &fakeWorkspaces{slugs: map[string]string{wsID: "acme"}},
		debts:       &fakeDebts{unpaid: map[string]string{}},
		changes:     &fakeChanger{},
		changeOrder: &fakeChangeOrders{},
	}
	h, err := New(Deps{
		Subscriptions: e.subs, Plans: e.plans, Members: e.members, Orders: e.orders, Workspaces: e.spaces, UsageDebts: e.debts,
		Changes: e.changes, ChangeOrders: e.changeOrder, IndividualMaxSeats: 5,
		VATRate: billing.DefaultVATRate, AppURL: appURL,
		Now: func() time.Time { return now },
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	e.h = h.Handler(trust.New(trust.DefaultCIDRs()).RequireUser)
	return e
}

// paid puts the workspace on Pro, five seats, monthly, inside the period.
func (e *env) paid() subscription.Subscription {
	s := subscription.Subscription{
		WorkspaceID: wsID, PlanKey: "pro", Status: subscription.StatusActive, Seats: 5,
		Interval: subscription.IntervalMonth, CurrentPeriodStart: &periodStart, CurrentPeriodEnd: &periodEnd, Version: 4,
	}
	e.subs.rows[wsID] = s
	return s
}

func (e *env) do(method, path, user string, headers map[string]string, body any) *httptest.ResponseRecorder {
	var rd io.Reader
	if body != nil {
		raw, ok := body.([]byte)
		if !ok {
			var err error
			if raw, err = json.Marshal(body); err != nil {
				panic(err)
			}
		}
		rd = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, rd)
	req.RemoteAddr = "10.0.0.7:4000"
	if user != "" {
		req.Header.Set(trust.UserHeader, user)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

type obj = map[string]any

func decode(t *testing.T, rec *httptest.ResponseRecorder) obj {
	t.Helper()
	var m obj
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body %q: %v", rec.Body, err)
	}
	return m
}

func wantCode(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status %d, want %d: %s", rec.Code, status, rec.Body)
	}
	if code != "" {
		if got := decode(t, rec)["code"]; got != code {
			t.Fatalf("code = %v, want %q: %s", got, code, rec.Body)
		}
	}
}

// --- checkout-sessions

const checkoutPath = "/api/v1/subscriptions/checkout-sessions"

func (e *env) checkout(user string, body obj) *httptest.ResponseRecorder {
	return e.do("POST", checkoutPath, user, nil, body)
}

func TestCheckoutCreatesASubscribeOrderForAFreeWorkspace(t *testing.T) {
	e := newEnv(t)
	rec := e.checkout(ownerID, obj{
		"workspace_id": wsID, "interval": "month", "idempotency_key": "k1", "customer_email": "o@example.com",
	})
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if len(e.orders.drafts) != 1 {
		t.Fatalf("drafts = %d", len(e.orders.drafts))
	}
	d := e.orders.drafts[0]
	// The cheapest public paid plan is Pro. Two members, and Pro's minimum is 1.
	if d.Kind != billing.KindSubscribe || d.PlanKey != "pro" || d.Interval != "month" || d.Seats != 2 {
		t.Errorf("draft = %+v", d)
	}
	if d.Amount != 298000 || d.VATAmount != 29800 || d.Currency != "vnd" {
		t.Errorf("amount %d, VAT %d, currency %q; want 298000, 29800, vnd", d.Amount, d.VATAmount, d.Currency)
	}
	if d.WorkspaceID != wsID || d.CreatedBy != ownerID || d.IdempotencyKey != "k1" {
		t.Errorf("draft = %+v", d)
	}
	if d.PeriodStart == nil || !d.PeriodStart.Equal(now) || !d.PeriodEnd.Equal(time.Date(2030, 6, 11, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("period = %v to %v", d.PeriodStart, d.PeriodEnd)
	}
	m := decode(t, rec)
	id := "dddddddd-0000-0000-0000-000000000001"
	if m["request_id"] != id || m["session_id"] != id {
		t.Errorf("ids = %v, %v", m["request_id"], m["session_id"])
	}
	if m["url"] != appURL+"/acme/billing/orders/"+id {
		t.Errorf("url = %v", m["url"])
	}
}

func TestCheckoutTakesPlanKeyAndSeats(t *testing.T) {
	e := newEnv(t)
	e.setKind(subscription.AccountBusiness)
	rec := e.checkout(adminID, obj{
		"workspace_id": wsID, "interval": "year", "idempotency_key": "k", "plan_key": "business", "seats": 7,
	})
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	d := e.orders.drafts[0]
	if d.PlanKey != "business" || d.Seats != 7 || d.Interval != "year" || d.Amount != 3490000*7 {
		t.Errorf("draft = %+v", d)
	}
}

func TestCheckoutOfAPaidWorkspaceIsAChangePlanOrder(t *testing.T) {
	e := newEnv(t)
	e.setKind(subscription.AccountBusiness)
	e.paid()
	rec := e.checkout(ownerID, obj{"workspace_id": wsID, "interval": "month", "idempotency_key": "k", "plan_key": "business"})
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	d := e.orders.drafts[0]
	// Business needs at least 5 seats, and the workspace has 5.
	if d.Kind != billing.KindChangePlan || d.PlanKey != "business" || d.Seats != 5 {
		t.Errorf("draft = %+v", d)
	}
}

func TestCheckoutOfAPaidWorkspaceWithoutAPlanKeepsItsPlanAndSeats(t *testing.T) {
	e := newEnv(t)
	e.paid()
	e.members.count = 2
	rec := e.checkout(ownerID, obj{"workspace_id": wsID, "interval": "year", "idempotency_key": "k"})
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if d := e.orders.drafts[0]; d.PlanKey != "pro" || d.Seats != 5 || d.Kind != billing.KindChangePlan {
		t.Errorf("draft = %+v", d)
	}
}

func TestCheckoutDefaultSeatsCoverTheMembers(t *testing.T) {
	e := newEnv(t)
	e.members.count = 9
	e.setKind(subscription.AccountBusiness)
	e.checkout(ownerID, obj{"workspace_id": wsID, "interval": "month", "idempotency_key": "k"})
	if d := e.orders.drafts[0]; d.Seats != 9 {
		t.Errorf("seats = %d, want the 9 members", d.Seats)
	}
}

func TestCheckoutRepeatedKeyReturnsTheSameOrder(t *testing.T) {
	e := newEnv(t)
	body := obj{"workspace_id": wsID, "interval": "month", "idempotency_key": "same"}
	first, second := decode(t, e.checkout(ownerID, body)), decode(t, e.checkout(ownerID, body))
	if first["request_id"] != second["request_id"] || first["url"] != second["url"] || len(e.orders.drafts) != 1 {
		t.Errorf("first %v, second %v, drafts %d", first, second, len(e.orders.drafts))
	}
}

func TestCheckoutTakesTheKeyFromTheHeader(t *testing.T) {
	e := newEnv(t)
	rec := e.do("POST", checkoutPath, ownerID, map[string]string{"Idempotency-Key": "from-header"},
		obj{"workspace_id": wsID, "interval": "month"})
	if rec.Code != 200 || e.orders.drafts[0].IdempotencyKey != "from-header" {
		t.Fatalf("status %d, drafts %+v", rec.Code, e.orders.drafts)
	}
}

func TestCheckoutRefusesBadInput(t *testing.T) {
	cases := map[string]obj{
		"no workspace":       {"interval": "month", "idempotency_key": "k"},
		"bad workspace":      {"workspace_id": "acme", "interval": "month", "idempotency_key": "k"},
		"bad interval":       {"workspace_id": wsID, "interval": "week", "idempotency_key": "k"},
		"no key":             {"workspace_id": wsID, "interval": "month"},
		"key too long":       {"workspace_id": wsID, "interval": "month", "idempotency_key": strings.Repeat("k", 201)},
		"zero seats":         {"workspace_id": wsID, "interval": "month", "idempotency_key": "k", "seats": 0},
		"negative seats":     {"workspace_id": wsID, "interval": "month", "idempotency_key": "k", "seats": -1},
		"fractional seats":   {"workspace_id": wsID, "interval": "month", "idempotency_key": "k", "seats": 2.5},
		"too many seats":     {"workspace_id": wsID, "interval": "month", "idempotency_key": "k", "seats": billing.MaxSeats + 1},
		"below plan minimum": {"workspace_id": wsID, "interval": "month", "idempotency_key": "k", "plan_key": "business", "seats": 4},
	}
	for name, body := range cases {
		e := newEnv(t)
		e.setKind(subscription.AccountBusiness)
		rec := e.checkout(ownerID, body)
		if rec.Code != 400 {
			t.Errorf("%s: status %d, want 400: %s", name, rec.Code, rec.Body)
		}
		if len(e.orders.drafts) != 0 {
			t.Errorf("%s: an order was created", name)
		}
	}
	e := newEnv(t)
	if rec := e.do("POST", checkoutPath, ownerID, nil, []byte("not json")); rec.Code != 400 {
		t.Errorf("malformed body: status %d", rec.Code)
	}
}

func TestCheckoutRefusesAPlanTheWorkspaceCannotBuy(t *testing.T) {
	for name, key := range map[string]string{"unknown": "nope", "private": "enterprise"} {
		e := newEnv(t)
		rec := e.checkout(ownerID, obj{"workspace_id": wsID, "interval": "month", "idempotency_key": "k", "plan_key": key})
		if rec.Code != 404 {
			t.Errorf("%s: status %d, want 404", name, rec.Code)
		}
	}
	e := newEnv(t)
	rec := e.checkout(ownerID, obj{"workspace_id": wsID, "interval": "month", "idempotency_key": "k", "plan_key": "free"})
	wantCode(t, rec, 400, "plan_not_purchasable")
	if len(e.orders.drafts) != 0 {
		t.Error("an order was created")
	}
}

func (e *env) setKind(kind string) {
	e.subs.profiles = map[string]subscription.BillingProfile{wsID: {WorkspaceID: wsID, AccountKind: kind}}
}

func TestCheckoutRefusesABusinessPlanForAnIndividualWorkspace(t *testing.T) {
	for name, profiles := range map[string]string{"no profile row": "", "individual row": subscription.AccountIndividual} {
		e := newEnv(t)
		if profiles != "" {
			e.setKind(profiles)
		}
		rec := e.checkout(ownerID, obj{"workspace_id": wsID, "interval": "month", "idempotency_key": "k", "plan_key": "business"})
		wantCode(t, rec, 409, "plan_audience_mismatch")
		m := decode(t, rec)
		if m["plan_key"] != "business" || m["account_kind"] != "individual" {
			t.Errorf("%s: body = %v, want plan_key business and account_kind individual", name, m)
		}
		if len(e.orders.drafts) != 0 {
			t.Errorf("%s: an order was created", name)
		}
	}
}

func TestCheckoutLetsABusinessWorkspaceBuyABusinessPlan(t *testing.T) {
	e := newEnv(t)
	e.setKind(subscription.AccountBusiness)
	rec := e.checkout(ownerID, obj{"workspace_id": wsID, "interval": "month", "idempotency_key": "k", "plan_key": "business"})
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if len(e.orders.drafts) != 1 || e.orders.drafts[0].PlanKey != "business" {
		t.Fatalf("drafts = %+v", e.orders.drafts)
	}
}

func TestCheckoutRefusesAPlanOnlyForIndividualsToABusinessWorkspace(t *testing.T) {
	e := newEnv(t)
	e.setKind(subscription.AccountBusiness)
	for i := range e.plans.plans {
		if e.plans.plans[i].Key == "pro" {
			e.plans.plans[i].Audience = plan.AudienceIndividual
		}
	}
	rec := e.checkout(ownerID, obj{"workspace_id": wsID, "interval": "month", "idempotency_key": "k", "plan_key": "pro"})
	wantCode(t, rec, 409, "plan_audience_mismatch")
	if len(e.orders.drafts) != 0 {
		t.Error("an order was created")
	}
}

func TestCheckoutOfAnAnyPlanIsOpenToBothKinds(t *testing.T) {
	for _, kind := range []string{subscription.AccountIndividual, subscription.AccountBusiness} {
		e := newEnv(t)
		e.setKind(kind)
		rec := e.checkout(ownerID, obj{"workspace_id": wsID, "interval": "month", "idempotency_key": "k", "plan_key": "pro"})
		if rec.Code != 200 {
			t.Errorf("%s: status %d: %s", kind, rec.Code, rec.Body)
		}
	}
}

func TestCheckoutRefusesWhenTheProfileCannotBeRead(t *testing.T) {
	e := newEnv(t)
	// A profile read that fails must not let the order through.
	failing := &failingProfiles{fakeSubs: e.subs}
	h, err := New(Deps{
		Subscriptions: failing, Plans: e.plans, Members: e.members, Orders: e.orders, Workspaces: e.spaces,
		VATRate: billing.DefaultVATRate, AppURL: appURL, Now: func() time.Time { return now },
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	e.h = h.Handler(trust.New(trust.DefaultCIDRs()).RequireUser)
	rec := e.checkout(ownerID, obj{"workspace_id": wsID, "interval": "month", "idempotency_key": "k", "plan_key": "pro"})
	wantCode(t, rec, 500, "internal")
	if strings.Contains(rec.Body.String(), "profile boom") {
		t.Errorf("the cause leaked: %s", rec.Body)
	}
	if len(e.orders.drafts) != 0 {
		t.Error("an order was created")
	}
}

type failingProfiles struct{ *fakeSubs }

func (f *failingProfiles) Profile(context.Context, string) (subscription.BillingProfile, bool, error) {
	return subscription.BillingProfile{}, false, errors.New("profile boom")
}

func TestCheckoutRoleChecks(t *testing.T) {
	cases := []struct {
		name, user string
		want       int
	}{
		{"owner", ownerID, 200}, {"admin", adminID, 200}, {"member", memberID, 403}, {"non-member", strangID, 404},
	}
	for _, tc := range cases {
		e := newEnv(t)
		rec := e.checkout(tc.user, obj{"workspace_id": wsID, "interval": "month", "idempotency_key": "k"})
		if rec.Code != tc.want {
			t.Errorf("%s: status %d, want %d", tc.name, rec.Code, tc.want)
		}
		if tc.want != 200 && len(e.orders.drafts) != 0 {
			t.Errorf("%s: an order was created", tc.name)
		}
	}
}

func TestCheckoutOfAnUnknownWorkspaceIsNotFound(t *testing.T) {
	e := newEnv(t)
	e.members.roles[otherWS+"/"+ownerID] = "owner"
	rec := e.checkout(ownerID, obj{"workspace_id": otherWS, "interval": "month", "idempotency_key": "k"})
	if rec.Code != 404 || len(e.orders.drafts) != 0 {
		t.Fatalf("status %d, drafts %d: %s", rec.Code, len(e.orders.drafts), rec.Body)
	}
}

func TestCheckoutURLUsesTheConfiguredOriginAndEscapesTheSlug(t *testing.T) {
	e := newEnv(t)
	e.spaces.slugs[wsID] = "a/b?c"
	m := decode(t, e.checkout(ownerID, obj{"workspace_id": wsID, "interval": "month", "idempotency_key": "k"}))
	want := appURL + "/a%2Fb%3Fc/billing/orders/dddddddd-0000-0000-0000-000000000001"
	if m["url"] != want {
		t.Errorf("url = %v, want %v", m["url"], want)
	}
}

func TestCheckoutNeedsATrustedUser(t *testing.T) {
	e := newEnv(t)
	body := obj{"workspace_id": wsID, "interval": "month", "idempotency_key": "k"}
	if rec := e.checkout("", body); rec.Code != 401 {
		t.Errorf("no user: %d", rec.Code)
	}
	req := httptest.NewRequest("POST", checkoutPath, bytes.NewReader([]byte(`{}`)))
	req.RemoteAddr = "203.0.113.9:1"
	req.Header.Set(trust.UserHeader, ownerID)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Errorf("untrusted peer: %d", rec.Code)
	}
	if e.members.calls != 0 {
		t.Error("the role lookup ran before the trust check passed")
	}
}

func TestCheckoutErrorsAreNotLeaked(t *testing.T) {
	secret := errors.New("dial tcp 10.0.0.9:5432: password authentication failed")
	for name, break_ := range map[string]func(*env){
		"orders":  func(e *env) { e.orders.err = secret },
		"members": func(e *env) { e.members.err = secret },
		"subs":    func(e *env) { e.subs.err = secret },
		"slug":    func(e *env) { e.spaces.err = secret },
	} {
		e := newEnv(t)
		break_(e)
		rec := e.checkout(ownerID, obj{"workspace_id": wsID, "interval": "month", "idempotency_key": "k"})
		if rec.Code != 500 {
			t.Errorf("%s: status %d, want 500", name, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "password") || strings.Contains(rec.Body.String(), "10.0.0.9") {
			t.Errorf("%s: leaks the cause: %s", name, rec.Body)
		}
	}
}

// --- seats/purchase-preview

func previewPath(ws string) string  { return "/api/v1/subscriptions/" + ws + "/seats/purchase-preview" }
func purchasePath(ws string) string { return "/api/v1/subscriptions/" + ws + "/seats/purchases" }

func TestPreviewPricesExtraSeatsByProration(t *testing.T) {
	e := newEnv(t)
	e.setKind(subscription.AccountBusiness)
	sub := e.paid()
	rec := e.do("POST", previewPath(wsID), ownerID, nil, obj{"additional_seats": 3})
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	m := decode(t, rec)
	// 149000 x 3 seats x 20 of 30 days left = 298000, before VAT. The next
	// invoice is 8 seats at the full price.
	want := obj{
		"current_seats": float64(5), "additional_seats": float64(3), "resulting_seats": float64(8),
		"purchase_version": float64(sub.Version), "currency": "vnd",
		"proration_amount": float64(298000), "next_invoice_amount": float64(1192000),
		"quoted_at": "2030-05-11T00:00:00Z",
	}
	for k, v := range want {
		if m[k] != v {
			t.Errorf("%s = %v, want %v", k, m[k], v)
		}
	}
}

func TestPreviewRefusesWhatCannotBeBought(t *testing.T) {
	e := newEnv(t)
	for _, n := range []any{0, -1, 1.5, billing.MaxSeats} {
		e.paid()
		if rec := e.do("POST", previewPath(wsID), ownerID, nil, obj{"additional_seats": n}); rec.Code != 400 {
			t.Errorf("additional_seats %v: status %d, want 400", n, rec.Code)
		}
	}
	// No subscription, a plan with no seat price, a period that has ended.
	delete(e.subs.rows, wsID)
	wantCode(t, e.do("POST", previewPath(wsID), ownerID, nil, obj{"additional_seats": 1}), 409, "seat_purchase_unavailable")
	s := e.paid()
	s.PlanKey = "free"
	e.subs.rows[wsID] = s
	wantCode(t, e.do("POST", previewPath(wsID), ownerID, nil, obj{"additional_seats": 1}), 409, "seat_purchase_unavailable")
	s = e.paid()
	late := now.Add(-time.Hour)
	s.CurrentPeriodEnd = &late
	e.subs.rows[wsID] = s
	wantCode(t, e.do("POST", previewPath(wsID), ownerID, nil, obj{"additional_seats": 1}), 409, "seat_purchase_unavailable")
}

func TestPreviewAndPurchaseRoleChecks(t *testing.T) {
	for _, path := range []string{previewPath(wsID), purchasePath(wsID), "/api/v1/subscriptions/" + wsID + "/seats/reconcile"} {
		for user, want := range map[string]int{memberID: 403, strangID: 404} {
			e := newEnv(t)
			e.paid()
			rec := e.do("POST", path, user, nil, obj{"additional_seats": 1})
			if rec.Code != want {
				t.Errorf("%s as %s: status %d, want %d", path, user, rec.Code, want)
			}
		}
	}
}

func TestSeatRoutesOfABadWorkspaceIDAreNotFound(t *testing.T) {
	e := newEnv(t)
	for _, suffix := range []string{"/seats/purchase-preview", "/seats/purchases", "/seats/reconcile"} {
		if rec := e.do("POST", "/api/v1/subscriptions/acme"+suffix, ownerID, nil, obj{}); rec.Code != 404 {
			t.Errorf("%s: status %d", suffix, rec.Code)
		}
	}
	if e.members.calls != 0 {
		t.Error("a bad id reached the database")
	}
}

// --- seats/purchases

func (e *env) quote(t *testing.T) obj {
	t.Helper()
	rec := e.do("POST", previewPath(wsID), ownerID, nil, obj{"additional_seats": 3})
	if rec.Code != 200 {
		t.Fatalf("preview: %d %s", rec.Code, rec.Body)
	}
	return decode(t, rec)
}

func confirm(q obj, key string) obj {
	return obj{
		"additional_seats": 3, "expected_current_seats": q["current_seats"], "expected_purchase_version": q["purchase_version"],
		"accepted_proration_amount": q["proration_amount"], "currency": q["currency"], "idempotency_key": key,
	}
}

func TestPurchaseCreatesAPendingAddSeatsOrder(t *testing.T) {
	e := newEnv(t)
	e.setKind(subscription.AccountBusiness)
	e.paid()
	rec := e.do("POST", purchasePath(wsID), adminID, nil, confirm(e.quote(t), "buy-1"))
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	d := e.orders.drafts[0]
	if d.Kind != billing.KindAddSeats || d.PlanKey != "pro" || d.Interval != "month" || d.Seats != 3 {
		t.Errorf("draft = %+v", d)
	}
	if d.Amount != 298000 || d.VATAmount != 29800 || d.CreatedBy != adminID || d.IdempotencyKey != "buy-1" {
		t.Errorf("draft = %+v", d)
	}
	// An add_seats order lives in the subscription's current period.
	if d.PeriodStart == nil || !d.PeriodStart.Equal(periodStart) || !d.PeriodEnd.Equal(periodEnd) {
		t.Errorf("period = %v to %v", d.PeriodStart, d.PeriodEnd)
	}
	m := decode(t, rec)
	want := obj{
		"request_id": "dddddddd-0000-0000-0000-000000000001", "current_seats": float64(5), "additional_seats": float64(3),
		"resulting_seats": float64(8), "currency": "vnd", "proration_amount": float64(298000),
		"next_invoice_amount": float64(1192000), "status": "pending",
	}
	for k, v := range want {
		if m[k] != v {
			t.Errorf("%s = %v, want %v", k, m[k], v)
		}
	}
}

func TestPurchaseRefusesAStaleQuote(t *testing.T) {
	cases := map[string]struct {
		edit func(obj)
		code string
	}{
		"stale version":      {func(b obj) { b["expected_purchase_version"] = float64(3) }, "seat_purchase_stale"},
		"newer version":      {func(b obj) { b["expected_purchase_version"] = float64(5) }, "seat_purchase_stale"},
		"stale seats":        {func(b obj) { b["expected_current_seats"] = float64(4) }, "seat_purchase_stale"},
		"price changed":      {func(b obj) { b["accepted_proration_amount"] = float64(297999) }, "proration_mismatch"},
		"price waived":       {func(b obj) { b["accepted_proration_amount"] = float64(0) }, "proration_mismatch"},
		"different seats":    {func(b obj) { b["additional_seats"] = 4 }, "proration_mismatch"},
		"different currency": {func(b obj) { b["currency"] = "usd" }, "currency_mismatch"},
	}
	for name, tc := range cases {
		e := newEnv(t)
		e.setKind(subscription.AccountBusiness)
		e.paid()
		body := confirm(e.quote(t), "k")
		tc.edit(body)
		wantCode(t, e.do("POST", purchasePath(wsID), ownerID, nil, body), 409, tc.code)
		if len(e.orders.drafts) != 0 {
			t.Errorf("%s: an order was created", name)
		}
	}
}

func TestPurchaseAcceptsAnUpperCaseCurrency(t *testing.T) {
	e := newEnv(t)
	e.setKind(subscription.AccountBusiness)
	e.paid()
	body := confirm(e.quote(t), "k")
	body["currency"] = "VND"
	if rec := e.do("POST", purchasePath(wsID), ownerID, nil, body); rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
}

func TestPurchaseRefusesIncompleteConfirmations(t *testing.T) {
	for _, drop := range []string{"additional_seats", "expected_current_seats", "expected_purchase_version", "currency", "idempotency_key"} {
		e := newEnv(t)
		e.setKind(subscription.AccountBusiness)
		e.paid()
		body := confirm(e.quote(t), "k")
		delete(body, drop)
		if rec := e.do("POST", purchasePath(wsID), ownerID, nil, body); rec.Code != 400 {
			t.Errorf("without %s: status %d, want 400: %s", drop, rec.Code, rec.Body)
		}
	}
	e := newEnv(t)
	e.setKind(subscription.AccountBusiness)
	e.paid()
	body := confirm(e.quote(t), "k")
	body["accepted_proration_amount"] = -1
	if rec := e.do("POST", purchasePath(wsID), ownerID, nil, body); rec.Code != 400 {
		t.Errorf("negative amount: status %d", rec.Code)
	}
}

func TestPurchaseRepeatedKeyReturnsTheSameOrder(t *testing.T) {
	e := newEnv(t)
	e.setKind(subscription.AccountBusiness)
	e.paid()
	body := confirm(e.quote(t), "same")
	a, b := decode(t, e.do("POST", purchasePath(wsID), ownerID, nil, body)), decode(t, e.do("POST", purchasePath(wsID), ownerID, nil, body))
	if a["request_id"] != b["request_id"] || len(e.orders.drafts) != 1 {
		t.Errorf("a %v, b %v, drafts %d", a, b, len(e.orders.drafts))
	}
}

func TestPurchaseWithoutASubscriptionIsUnavailable(t *testing.T) {
	e := newEnv(t)
	body := obj{
		"additional_seats": 1, "expected_current_seats": 1, "expected_purchase_version": 1,
		"accepted_proration_amount": 1, "currency": "vnd", "idempotency_key": "k",
	}
	wantCode(t, e.do("POST", purchasePath(wsID), ownerID, nil, body), 409, "seat_purchase_unavailable")
}

// --- seats/reconcile and portal-sessions

func TestReconcileIsANoop(t *testing.T) {
	e := newEnv(t)
	sub := e.paid()
	rec := e.do("POST", "/api/v1/subscriptions/"+wsID+"/seats/reconcile", ownerID, nil, nil)
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	m := decode(t, rec)
	if m["workspace_id"] != wsID || m["action"] != "noop" || m["version"] != float64(sub.Version) {
		t.Errorf("body = %v", m)
	}
	// With no subscription row the version is 0.
	delete(e.subs.rows, wsID)
	if m := decode(t, e.do("POST", "/api/v1/subscriptions/"+wsID+"/seats/reconcile", ownerID, nil, nil)); m["version"] != float64(0) {
		t.Errorf("version = %v", m["version"])
	}
}

func TestPortalSessionsAre404(t *testing.T) {
	e := newEnv(t)
	rec := e.do("POST", "/api/v1/subscriptions/"+wsID+"/portal-sessions", ownerID, map[string]string{"Idempotency-Key": "k"}, nil)
	if rec.Code != 404 {
		t.Fatalf("status %d, want 404", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("content-type = %q", rec.Header().Get("Content-Type"))
	}
}

func TestResponsesAreNotCached(t *testing.T) {
	e := newEnv(t)
	e.paid()
	rec := e.do("POST", previewPath(wsID), ownerID, nil, obj{"additional_seats": 1})
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("cache-control = %q", rec.Header().Get("Cache-Control"))
	}
}

// --- configuration

func TestNewRefusesAnAppURLThatIsNotAnOrigin(t *testing.T) {
	for _, bad := range []string{"", "app.example.com", "/relative", "ftp://app.example.com", "javascript:alert(1)", "https://", "https://app.example.com/path"} {
		_, err := New(Deps{
			Subscriptions: &fakeSubs{}, Plans: &fakePlans{}, Members: &fakeMembers{}, Orders: &fakeOrders{}, Workspaces: &fakeWorkspaces{},
			VATRate: billing.DefaultVATRate, AppURL: bad,
		})
		if err == nil {
			t.Errorf("AppURL %q: New succeeded", bad)
		}
	}
	for _, good := range []string{"https://app.example.com", "http://localhost:3000"} {
		if _, err := New(Deps{
			Subscriptions: &fakeSubs{}, Plans: &fakePlans{}, Members: &fakeMembers{}, Orders: &fakeOrders{}, Workspaces: &fakeWorkspaces{},
			VATRate: billing.DefaultVATRate, AppURL: good,
		}); err != nil {
			t.Errorf("AppURL %q: %v", good, err)
		}
	}
}
