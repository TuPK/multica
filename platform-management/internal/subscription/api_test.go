package subscription

import (
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

	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/trust"
)

const (
	ownerID  = "aaaaaaaa-1111-0000-0000-000000000001"
	adminID  = "aaaaaaaa-1111-0000-0000-000000000002"
	memberID = "aaaaaaaa-1111-0000-0000-000000000003"
	strangID = "aaaaaaaa-1111-0000-0000-000000000004"
)

type fakeMembers struct {
	roles map[string]string // user id -> role in wsID
	count int64
	// reserved is the number of seats held by live seat operations.
	reserved    int64
	reservedErr error
	err         error

	roleCalls int
}

func (f *fakeMembers) Role(_ context.Context, ws, user string) (string, bool, error) {
	f.roleCalls++
	if f.err != nil {
		return "", false, f.err
	}
	r, ok := f.roles[ws+"/"+user]
	return r, ok, nil
}

func (f *fakeMembers) Count(context.Context, string) (int64, error) { return f.count, f.err }

func (f *fakeMembers) Reserved(context.Context, string) (int64, error) {
	if f.reservedErr != nil {
		return 0, f.reservedErr
	}
	return f.reserved, f.err
}

type fakeSubs struct {
	rows map[string]Subscription
	err  error
}

func (f *fakeSubs) Get(_ context.Context, ws string) (Subscription, bool, error) {
	s, ok := f.rows[ws]
	return s, ok, f.err
}

type fakePlans struct {
	plans []plan.Plan
	err   error
}

func (f *fakePlans) Get(_ context.Context, key string) (plan.Plan, bool, error) {
	for _, p := range f.plans {
		if p.Key == key {
			return p, true, f.err
		}
	}
	return plan.Plan{}, false, f.err
}

func (f *fakePlans) List(context.Context) ([]plan.Plan, error) { return f.plans, f.err }

type fakePending struct {
	order PendingPurchase
	has   bool
	err   error
	calls int
}

func (f *fakePending) PendingSeats(context.Context, string, time.Time) (PendingPurchase, bool, error) {
	f.calls++
	return f.order, f.has && f.err == nil, f.err
}

type apiEnv struct {
	members *fakeMembers
	subs    *fakeSubs
	plans   *fakePlans
	pending *fakePending
	h       http.Handler
}

func newAPIEnv(t *testing.T) *apiEnv {
	t.Helper()
	all, err := plan.Catalogue()
	if err != nil {
		t.Fatal(err)
	}
	e := &apiEnv{
		members: &fakeMembers{
			roles: map[string]string{
				wsID + "/" + ownerID: "owner", wsID + "/" + adminID: "admin", wsID + "/" + memberID: "member",
			},
			count: 2,
		},
		subs:    &fakeSubs{rows: map[string]Subscription{}},
		plans:   &fakePlans{plans: all},
		pending: &fakePending{},
	}
	api := NewAPI(APIDeps{
		Subscriptions: e.subs, Plans: e.plans, Members: e.members, Pending: e.pending,
		Now: func() time.Time { return t0 },
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	e.h = api.Handler(trust.New(trust.DefaultCIDRs()).RequireUser)
	return e
}

func (e *apiEnv) get(path, user, remote string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = remote
	if user != "" {
		req.Header.Set(trust.UserHeader, user)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func (e *apiEnv) summary(user string) *httptest.ResponseRecorder {
	return e.get("/api/v1/subscriptions/"+wsID+"/summary", user, "10.0.0.7:4000")
}

func decodeMap(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body %q: %v", rec.Body, err)
	}
	return m
}

func obj(t *testing.T, v any, name string) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("%s = %#v, want an object", name, v)
	}
	return m
}

// checkSummarySchema holds the constraints of the billing page's zod schema
// (spec Appendix A), so a response that the page would reject as "unavailable"
// fails here instead.
func checkSummarySchema(t *testing.T, m map[string]any) {
	t.Helper()
	whole := func(v any, name string, min float64) {
		t.Helper()
		f, ok := v.(float64)
		if !ok || f != float64(int64(f)) || f < min {
			t.Errorf("%s = %#v, want an integer >= %v", name, v, min)
		}
	}
	ent := obj(t, m["entitlement"], "entitlement")
	for _, k := range []string{"workspace_id", "plan", "status"} {
		if s, ok := ent[k].(string); !ok || s == "" {
			t.Errorf("entitlement.%s = %#v, want a string", k, ent[k])
		}
	}
	whole(ent["seats"], "entitlement.seats", 0)
	whole(ent["version"], "entitlement.version", 0)
	limits := obj(t, ent["limits"], "entitlement.limits")
	for _, gate := range []string{"issue_count", "autopilot_runs"} {
		l := obj(t, limits[gate], "limits."+gate)
		switch l["mode"] {
		case "limited":
			whole(l["limit"], gate+".limit", 1)
		case "unlimited":
			if _, has := l["limit"]; has {
				t.Errorf("%s: an unlimited gate carries a limit", gate)
			}
		default:
			t.Errorf("%s.mode = %v", gate, l["mode"])
		}
	}
	for _, k := range []string{"current_period_end", "snapshot_expires_at"} {
		if v, present := ent[k]; present && v != nil {
			if _, ok := v.(string); !ok {
				t.Errorf("entitlement.%s = %#v, want a string or null", k, v)
			}
		}
	}
	switch m["billing_interval"] {
	case nil, "month", "year":
	default:
		t.Errorf("billing_interval = %v", m["billing_interval"])
	}
	whole(m["human_members"], "human_members", 0)
	if sc, present := m["seat_capacity"]; !present {
		t.Error("seat_capacity is absent; it must be an object or null")
	} else if sc != nil {
		c := obj(t, sc, "seat_capacity")
		whole(c["purchased"], "seat_capacity.purchased", 1)
		whole(c["version"], "seat_capacity.version", 1)
		for _, k := range []string{"used", "reserved", "available"} {
			whole(c[k], "seat_capacity."+k, 0)
		}
		if _, ok := c["overcommitted"].(bool); !ok {
			t.Errorf("seat_capacity.overcommitted = %#v", c["overcommitted"])
		}
		if v, present := c["pending_quantity"]; !present || v != nil {
			whole(v, "seat_capacity.pending_quantity", 1)
		}
	}
	for _, k := range []string{"cancel_at_period_end", "has_stripe_customer"} {
		if _, ok := m[k].(bool); !ok {
			t.Errorf("%s = %#v, want a bool", k, m[k])
		}
	}
	if v, present := m["grace_until"]; !present {
		t.Error("grace_until is absent; it must be a string or null")
	} else if _, isString := v.(string); v != nil && !isString {
		t.Errorf("grace_until = %#v", v)
	}
	actions := obj(t, m["available_actions"], "available_actions")
	for _, k := range []string{"checkout", "portal", "purchase_seats"} {
		if _, ok := actions[k].(bool); !ok {
			t.Errorf("available_actions.%s = %#v", k, actions[k])
		}
	}
}

// --- The role check: owner and admin read the summary.

func TestSummaryAdmitsOwnersAndAdminsOnly(t *testing.T) {
	cases := []struct {
		name, user string
		want       int
	}{
		{"owner", ownerID, 200},
		{"admin", adminID, 200},
		{"member", memberID, 403},
		{"non-member", strangID, 404},
	}
	for _, tc := range cases {
		e := newAPIEnv(t)
		rec := e.summary(tc.user)
		if rec.Code != tc.want {
			t.Errorf("%s: status %d, want %d (%s)", tc.name, rec.Code, tc.want, rec.Body)
		}
		if tc.want == 200 {
			checkSummarySchema(t, decodeMap(t, rec))
		}
	}
}

func TestSummaryRefusesAnyOtherRole(t *testing.T) {
	e := newAPIEnv(t)
	e.members.roles[wsID+"/"+memberID] = "viewer"
	if rec := e.summary(memberID); rec.Code != 403 {
		t.Fatalf("viewer: status %d, want 403", rec.Code)
	}
}

// A member of another workspace is a non-member of this one.
func TestSummaryChecksTheWorkspaceOfThePath(t *testing.T) {
	e := newAPIEnv(t)
	other := "cccccccc-0000-0000-0000-00000000000f"
	rec := e.get("/api/v1/subscriptions/"+other+"/summary", ownerID, "10.0.0.7:1")
	if rec.Code != 404 {
		t.Fatalf("status %d, want 404 for an owner of a different workspace", rec.Code)
	}
}

func TestSummaryNeedsATrustedUser(t *testing.T) {
	e := newAPIEnv(t)
	path := "/api/v1/subscriptions/" + wsID + "/summary"

	if rec := e.get(path, "", "10.0.0.7:1"); rec.Code != 401 {
		t.Errorf("no X-User-ID: status %d, want 401", rec.Code)
	}
	if rec := e.get(path, ownerID, "203.0.113.9:1"); rec.Code != 403 {
		t.Errorf("untrusted peer: status %d, want 403", rec.Code)
	}
	if e.members.roleCalls != 0 {
		t.Errorf("the role lookup ran %d times before the trust check passed", e.members.roleCalls)
	}
	if rec := e.get(path, "not-a-uuid", "10.0.0.7:1"); rec.Code != 401 {
		t.Errorf("malformed X-User-ID: status %d, want 401", rec.Code)
	}
}

func TestSummaryOfABadWorkspaceIDIs404(t *testing.T) {
	e := newAPIEnv(t)
	for _, id := range []string{"acme", "1", wsID + "0"} {
		if rec := e.get("/api/v1/subscriptions/"+id+"/summary", ownerID, "10.0.0.7:1"); rec.Code != 404 {
			t.Errorf("%q: status %d, want 404", id, rec.Code)
		}
	}
	if e.members.roleCalls != 0 {
		t.Error("a bad id reached the database")
	}
}

// --- The summary body.

func TestSummaryOfAFreeWorkspace(t *testing.T) {
	e := newAPIEnv(t)
	m := decodeMap(t, e.summary(ownerID))
	checkSummarySchema(t, m)

	ent := obj(t, m["entitlement"], "entitlement")
	if ent["workspace_id"] != wsID || ent["plan"] != "free" || ent["status"] != "active" || ent["seats"] != float64(3) || ent["version"] != float64(0) {
		t.Errorf("entitlement = %v", ent)
	}
	limits := obj(t, ent["limits"], "limits")
	if l := obj(t, limits["issue_count"], "issue_count"); l["mode"] != "limited" || l["limit"] != float64(200) {
		t.Errorf("issue_count = %v", l)
	}
	if l := obj(t, limits["autopilot_runs"], "autopilot_runs"); l["mode"] != "limited" || l["limit"] != float64(50) {
		t.Errorf("autopilot_runs = %v", l)
	}
	if ent["current_period_end"] != nil || m["billing_interval"] != nil || m["grace_until"] != nil {
		t.Errorf("a workspace with no subscription has no period, interval or grace: %v", m)
	}
	if m["human_members"] != float64(2) || m["cancel_at_period_end"] != false || m["has_stripe_customer"] != false || m["payment_provider"] != "manual" {
		t.Errorf("summary = %v", m)
	}
	// 3 fixed seats, 2 used. The version is the row's plus one: with no row, 1.
	want := map[string]any{
		"purchased": float64(3), "used": float64(2), "reserved": float64(0), "available": float64(1),
		"overcommitted": false, "version": float64(1), "pending_quantity": nil,
	}
	got := obj(t, m["seat_capacity"], "seat_capacity")
	for k, v := range want {
		if got[k] != v {
			t.Errorf("seat_capacity.%s = %v, want %v", k, got[k], v)
		}
	}
}

func actionsOf(t *testing.T, m map[string]any) map[string]any {
	t.Helper()
	return obj(t, m["available_actions"], "available_actions")
}

// A workspace on the free plan can order a plan, and cannot buy seats: the free
// plan has no seat price.
func TestSummaryOfAFreeWorkspaceOffersCheckoutOnly(t *testing.T) {
	e := newAPIEnv(t)
	got := actionsOf(t, decodeMap(t, e.summary(ownerID)))
	want := map[string]any{"checkout": true, "portal": false, "purchase_seats": false}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("available_actions.%s = %v, want %v", k, got[k], v)
		}
	}
}

func paidRow(start, end time.Time) Subscription {
	return Subscription{
		WorkspaceID: wsID, PlanKey: "pro", Status: StatusActive, Seats: 5, Interval: IntervalMonth,
		CurrentPeriodStart: &start, CurrentPeriodEnd: &end, Version: 4,
	}
}

func TestSummaryOffersSeatPurchaseOnlyInsideAPaidPeriod(t *testing.T) {
	day := 24 * time.Hour
	cases := []struct {
		name string
		edit func(*Subscription)
		want bool
	}{
		{"inside the period", func(*Subscription) {}, true},
		{"period ended", func(s *Subscription) { e := t0.Add(-day); s.CurrentPeriodEnd = &e }, false},
		{"period not started", func(s *Subscription) { st := t0.Add(day); s.CurrentPeriodStart = &st }, false},
		{"no period", func(s *Subscription) { s.CurrentPeriodStart, s.CurrentPeriodEnd = nil, nil }, false},
		{"past due", func(s *Subscription) { s.Status = "past_due" }, false},
		{"plan with no seat price", func(s *Subscription) { s.PlanKey = "free" }, false},
	}
	for _, tc := range cases {
		e := newAPIEnv(t)
		row := paidRow(t0.Add(-10*day), t0.Add(20*day))
		tc.edit(&row)
		e.subs.rows[wsID] = row
		m := decodeMap(t, e.summary(ownerID))
		checkSummarySchema(t, m)
		a := actionsOf(t, m)
		if a["purchase_seats"] != tc.want || a["checkout"] != true || a["portal"] != false {
			t.Errorf("%s: available_actions = %v, want purchase_seats %v", tc.name, a, tc.want)
		}
	}
}

// A pending add_seats order is the active purchase, and pending_quantity is the
// seat count that order leads to.
func TestSummaryReportsAPendingSeatPurchase(t *testing.T) {
	e := newAPIEnv(t)
	e.subs.rows[wsID] = paidRow(t0.Add(-24*time.Hour), t0.Add(24*time.Hour))
	exp := t0.Add(6 * 24 * time.Hour)
	e.pending.order, e.pending.has = PendingPurchase{OrderID: "dddddddd-0000-0000-0000-000000000001", Seats: 3, ExpiresAt: exp}, true
	m := decodeMap(t, e.summary(ownerID))
	checkSummarySchema(t, m)
	sc := obj(t, m["seat_capacity"], "seat_capacity")
	if sc["pending_quantity"] != float64(8) {
		t.Errorf("pending_quantity = %v, want 5 purchased + 3 ordered", sc["pending_quantity"])
	}
	ap := obj(t, sc["active_purchase"], "active_purchase")
	if ap["request_id"] != "dddddddd-0000-0000-0000-000000000001" || ap["target_seats"] != float64(8) || ap["status"] != "pending" || ap["expires_at"] != "2030-05-12T07:08:09.123456789Z" {
		t.Errorf("active_purchase = %v", ap)
	}
}

func TestSummaryWithoutAPendingPurchaseOmitsTheActivePurchase(t *testing.T) {
	e := newAPIEnv(t)
	e.subs.rows[wsID] = paidRow(t0.Add(-24*time.Hour), t0.Add(24*time.Hour))
	sc := obj(t, decodeMap(t, e.summary(ownerID))["seat_capacity"], "seat_capacity")
	if _, has := sc["active_purchase"]; has || sc["pending_quantity"] != nil {
		t.Errorf("seat_capacity = %v, want no active_purchase and a null pending_quantity", sc)
	}
}

// A workspace with no seat limit has no seat capacity, so the pending order is
// not read.
func TestSummaryDoesNotReadPendingPurchasesWithoutSeatCapacity(t *testing.T) {
	e := newAPIEnv(t)
	e.subs.rows[wsID] = Subscription{WorkspaceID: wsID, PlanKey: "enterprise", Status: "active", Interval: IntervalYear, Version: 2}
	e.summary(ownerID)
	if e.pending.calls != 0 {
		t.Errorf("the pending purchase was read %d times", e.pending.calls)
	}
}

func TestSummaryFailsWhenThePendingPurchaseCannotBeRead(t *testing.T) {
	e := newAPIEnv(t)
	e.subs.rows[wsID] = paidRow(t0.Add(-24*time.Hour), t0.Add(24*time.Hour))
	e.pending.err = errors.New("db down")
	if rec := e.summary(ownerID); rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestSummaryOfAPaidWorkspaceWithOverrides(t *testing.T) {
	e := newAPIEnv(t)
	end := time.Date(2030, 6, 1, 0, 0, 0, 0, time.UTC)
	grace := time.Date(2030, 6, 8, 0, 0, 0, 0, time.UTC)
	e.subs.rows[wsID] = Subscription{
		WorkspaceID: wsID, PlanKey: "pro", Status: "active", Seats: 5, Interval: IntervalYear,
		CurrentPeriodEnd: &end, GraceUntil: &grace, CancelAtPeriodEnd: true, Version: 7,
		LimitOverrides: plan.Limits{"issue_count": i64(500), "autopilot_runs": nil},
	}
	e.members.count = 4
	m := decodeMap(t, e.summary(adminID))
	checkSummarySchema(t, m)

	ent := obj(t, m["entitlement"], "entitlement")
	if ent["plan"] != "pro" || ent["seats"] != float64(5) || ent["version"] != float64(7) || ent["current_period_end"] != "2030-06-01T00:00:00Z" {
		t.Errorf("entitlement = %v", ent)
	}
	limits := obj(t, ent["limits"], "limits")
	if l := obj(t, limits["issue_count"], "issue_count"); l["mode"] != "limited" || l["limit"] != float64(500) {
		t.Errorf("issue_count = %v, want the override 500 over the plan's unlimited", l)
	}
	if l := obj(t, limits["autopilot_runs"], "autopilot_runs"); l["mode"] != "unlimited" {
		t.Errorf("autopilot_runs = %v, want the override to lift the plan's 2000", l)
	}
	if m["billing_interval"] != "year" || m["cancel_at_period_end"] != true || m["grace_until"] != "2030-06-08T00:00:00Z" {
		t.Errorf("summary = %v", m)
	}
	sc := obj(t, m["seat_capacity"], "seat_capacity")
	if sc["purchased"] != float64(5) || sc["used"] != float64(4) || sc["available"] != float64(1) || sc["version"] != float64(8) {
		t.Errorf("seat_capacity = %v, want 5 purchased, 4 used, version 7 + 1", sc)
	}
}

func TestSummaryOfAnOvercommittedWorkspace(t *testing.T) {
	e := newAPIEnv(t)
	e.subs.rows[wsID] = Subscription{WorkspaceID: wsID, PlanKey: "pro", Status: "active", Seats: 3, Interval: IntervalMonth, Version: 1}
	e.members.count = 8
	m := decodeMap(t, e.summary(ownerID))
	checkSummarySchema(t, m)
	sc := obj(t, m["seat_capacity"], "seat_capacity")
	if sc["available"] != float64(0) || sc["overcommitted"] != true || sc["used"] != float64(8) || m["human_members"] != float64(8) {
		t.Errorf("seat_capacity = %v; want no negative availability and the overcommit flag", sc)
	}
}

// A plan with no seat limit has no seat capacity to report.
func TestSummaryOfAWorkspaceWithoutASeatLimitHasNullCapacity(t *testing.T) {
	e := newAPIEnv(t)
	e.subs.rows[wsID] = Subscription{WorkspaceID: wsID, PlanKey: "enterprise", Status: "active", Seats: 0, Interval: IntervalYear, Version: 2}
	m := decodeMap(t, e.summary(ownerID))
	checkSummarySchema(t, m)
	if v, present := m["seat_capacity"]; !present || v != nil {
		t.Fatalf("seat_capacity = %#v, want an explicit null", v)
	}
	if ent := obj(t, m["entitlement"], "entitlement"); ent["seats"] != float64(0) {
		t.Errorf("entitlement.seats = %v", ent["seats"])
	}
}

func TestSummaryErrorsAreNotLeaked(t *testing.T) {
	secret := errors.New(`dial tcp 10.0.0.9:5432: password authentication failed`)
	for name, break_ := range map[string]func(*apiEnv){
		"members": func(e *apiEnv) { e.members.err = secret },
		"subs":    func(e *apiEnv) { e.subs.err = secret },
		"plans":   func(e *apiEnv) { e.plans.err = secret },
		"missing plan": func(e *apiEnv) {
			e.subs.rows[wsID] = Subscription{WorkspaceID: wsID, PlanKey: "gone", Interval: IntervalMonth}
		},
	} {
		e := newAPIEnv(t)
		break_(e)
		rec := e.summary(ownerID)
		if rec.Code != 500 {
			t.Errorf("%s: status %d, want 500", name, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "password") || strings.Contains(rec.Body.String(), "10.0.0.9") {
			t.Errorf("%s: the body leaks the cause: %s", name, rec.Body)
		}
	}
}

// --- Prices.

func (e *apiEnv) prices(user string) *httptest.ResponseRecorder {
	return e.get("/api/v1/subscriptions/"+wsID+"/prices", user, "10.0.0.7:4000")
}

func TestPricesAreTheEntryPaidPlansPerSeatPrice(t *testing.T) {
	e := newAPIEnv(t)
	rec := e.prices(memberID) // any user: the price list is not a privileged read
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	m := decodeMap(t, rec)
	for interval, amount := range map[string]float64{"month": 149000, "year": 1490000} {
		p := obj(t, m[interval], interval)
		if p["currency"] != "vnd" || p["unit_amount"] != amount || p["interval"] != interval || p["interval_count"] != float64(1) {
			t.Errorf("%s = %v, want vnd %v per %s", interval, p, amount, interval)
		}
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("cache-control = %q", cc)
	}
}

func TestPricesListThePublicPlansOnly(t *testing.T) {
	e := newAPIEnv(t)
	for i := range e.plans.plans {
		if e.plans.plans[i].Key == "business" {
			at := t0
			e.plans.plans[i].ArchivedAt = &at
		}
	}
	m := decodeMap(t, e.prices(ownerID))
	list, _ := m["plans"].([]any)
	var keys []string
	for _, p := range list {
		keys = append(keys, obj(t, p, "plan")["key"].(string))
	}
	// enterprise is private and business is archived.
	if strings.Join(keys, ",") != "free,pro" {
		t.Fatalf("plans = %v, want free,pro", keys)
	}
	pro := obj(t, list[1], "pro")
	if pro["seat_price_month"] != float64(149000) || pro["min_seats"] != float64(1) || pro["trial_days"] != float64(14) {
		t.Errorf("pro = %v", pro)
	}
	if free := obj(t, list[0], "free"); free["seats_fixed"] != float64(3) {
		t.Errorf("free = %v", free)
	}
}

// With no public paid plan the page is told the price is unavailable, which it
// renders as "price confirmed at checkout" (cloud_billing.go).
func TestPricesAre503WithoutAPublicPaidPlan(t *testing.T) {
	e := newAPIEnv(t)
	e.plans.plans = e.plans.plans[:1] // only free
	rec := e.prices(ownerID)
	if rec.Code != 503 {
		t.Fatalf("status %d, want 503", rec.Code)
	}
	if m := decodeMap(t, rec); m["code"] != "prices_unavailable" {
		t.Errorf("body = %v", m)
	}
}

func (e *apiEnv) pricesFor(query string) *httptest.ResponseRecorder {
	return e.get("/api/v1/subscriptions/"+wsID+"/prices"+query, ownerID, "10.0.0.7:4000")
}

func unitAmounts(t *testing.T, rec *httptest.ResponseRecorder) (month, year float64) {
	t.Helper()
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	m := decodeMap(t, rec)
	return obj(t, m["month"], "month")["unit_amount"].(float64), obj(t, m["year"], "year")["unit_amount"].(float64)
}

// The cheapest public paid plan is the entry plan, whatever the display order.
func TestPricesOfAFreeWorkspaceAreTheCheapestPublicPaidPlans(t *testing.T) {
	e := newAPIEnv(t)
	for i := range e.plans.plans { // put business before pro in display order
		switch e.plans.plans[i].Key {
		case "pro":
			e.plans.plans[i].Sort = 40
		}
	}
	if month, year := unitAmounts(t, e.pricesFor("")); month != 149000 || year != 1490000 {
		t.Errorf("prices = %v, %v; want pro's", month, year)
	}
}

func TestPricesOfAPaidWorkspaceAreItsCurrentPlans(t *testing.T) {
	e := newAPIEnv(t)
	e.subs.rows[wsID] = Subscription{WorkspaceID: wsID, PlanKey: "business", Status: "active", Seats: 5, Interval: IntervalMonth, Version: 1}
	if month, year := unitAmounts(t, e.pricesFor("")); month != 349000 || year != 3490000 {
		t.Errorf("prices = %v, %v; want business's", month, year)
	}
}

func TestPricesOfAWorkspaceOnTheFreePlanRowAreTheEntryPlans(t *testing.T) {
	e := newAPIEnv(t)
	e.subs.rows[wsID] = Subscription{WorkspaceID: wsID, PlanKey: "free", Status: "active", Interval: IntervalMonth, Version: 1}
	if month, _ := unitAmounts(t, e.pricesFor("")); month != 149000 {
		t.Errorf("month = %v, want pro's", month)
	}
}

func TestPricesTakeThePlanQuery(t *testing.T) {
	e := newAPIEnv(t)
	e.subs.rows[wsID] = Subscription{WorkspaceID: wsID, PlanKey: "pro", Status: "active", Seats: 5, Interval: IntervalMonth, Version: 1}
	if month, year := unitAmounts(t, e.pricesFor("?plan=business")); month != 349000 || year != 3490000 {
		t.Errorf("?plan=business: %v, %v", month, year)
	}
}

func TestPricesRefuseAPlanThatIsNotListedForTheWorkspace(t *testing.T) {
	e := newAPIEnv(t)
	for name, query := range map[string]string{
		"unknown":          "?plan=nope",
		"private":          "?plan=enterprise",
		"not a plan key":   "?plan=%00",
		"empty plan value": "?plan=",
	} {
		rec := e.pricesFor(query)
		if rec.Code != 404 || decodeMap(t, rec)["code"] != "plan_not_found" {
			t.Errorf("%s: status %d, body %s; want 404 plan_not_found", name, rec.Code, rec.Body)
		}
	}
	// The workspace's own private plan can be priced.
	e.subs.rows[wsID] = Subscription{WorkspaceID: wsID, PlanKey: "enterprise", Status: "active", Interval: IntervalYear, Version: 1}
	if rec := e.pricesFor("?plan=enterprise"); rec.Code == 404 {
		t.Errorf("a workspace on a private plan cannot read its prices: %s", rec.Body)
	}
}

func TestPricesOfAPlanWithNoSeatPriceAre503(t *testing.T) {
	e := newAPIEnv(t)
	rec := e.pricesFor("?plan=free")
	if rec.Code != 503 || decodeMap(t, rec)["code"] != "prices_unavailable" {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body)
	}
}

func TestPricesNeedATrustedUser(t *testing.T) {
	e := newAPIEnv(t)
	path := "/api/v1/subscriptions/" + wsID + "/prices"
	if rec := e.get(path, "", "10.0.0.7:1"); rec.Code != 401 {
		t.Errorf("no header: %d", rec.Code)
	}
	if rec := e.get(path, ownerID, "198.51.100.4:1"); rec.Code != 403 {
		t.Errorf("untrusted peer: %d", rec.Code)
	}
}

func TestOtherRoutesAndMethodsAreNotServed(t *testing.T) {
	e := newAPIEnv(t)
	for _, tc := range []struct{ method, path string }{
		{"POST", "/api/v1/subscriptions/" + wsID + "/summary"},
		{"GET", "/api/v1/subscriptions/" + wsID + "/portal-sessions"},
		{"POST", "/api/v1/subscriptions/checkout-sessions"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req.RemoteAddr = "10.0.0.7:1"
		req.Header.Set(trust.UserHeader, ownerID)
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, req)
		if rec.Code != 404 && rec.Code != 405 {
			t.Errorf("%s %s: status %d", tc.method, tc.path, rec.Code)
		}
	}
}

// The seats held by live seat operations are reported as reserved and are not
// available (Appendix B rule 4).
func TestSummaryCountsReservedSeats(t *testing.T) {
	e := newAPIEnv(t)
	e.subs.rows[wsID] = Subscription{WorkspaceID: wsID, PlanKey: "pro", Status: "active", Seats: 5, Interval: IntervalMonth, Version: 3}
	e.members.count = 2
	e.members.reserved = 2
	m := decodeMap(t, e.summary(ownerID))
	checkSummarySchema(t, m)
	sc := obj(t, m["seat_capacity"], "seat_capacity")
	if sc["reserved"] != float64(2) || sc["used"] != float64(2) || sc["available"] != float64(1) || sc["overcommitted"] != false {
		t.Errorf("seat_capacity = %v, want 2 reserved, 2 used, 1 available", sc)
	}
}

func TestSummaryReservedSeatsCannotMakeAvailabilityNegative(t *testing.T) {
	e := newAPIEnv(t)
	e.subs.rows[wsID] = Subscription{WorkspaceID: wsID, PlanKey: "pro", Status: "active", Seats: 3, Interval: IntervalMonth, Version: 1}
	e.members.count = 3
	e.members.reserved = 1
	sc := obj(t, decodeMap(t, e.summary(ownerID))["seat_capacity"], "seat_capacity")
	if sc["available"] != float64(0) || sc["overcommitted"] != false {
		t.Errorf("seat_capacity = %v, want 0 available and not overcommitted: held seats are not members", sc)
	}
}

func TestSummaryFailsWhenReservedSeatsCannotBeRead(t *testing.T) {
	e := newAPIEnv(t)
	e.subs.rows[wsID] = Subscription{WorkspaceID: wsID, PlanKey: "pro", Status: "active", Seats: 3, Interval: IntervalMonth, Version: 1}
	e.members.reservedErr = errors.New("db down")
	if rec := e.summary(ownerID); rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}
