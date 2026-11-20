package entitlement

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
)

const (
	wsA = "11111111-1111-1111-1111-111111111111"
	wsB = "22222222-2222-2222-2222-222222222222"
)

// fakeSource counts loads and returns what the test set.
type fakeSource struct {
	mu     sync.Mutex
	loads  map[string]int
	inputs map[string]Inputs
	err    error
	// onLoad runs inside Inputs, after the value is read and before it returns.
	onLoad func()
}

func newFakeSource(in map[string]Inputs) *fakeSource {
	return &fakeSource{loads: map[string]int{}, inputs: in}
}

func (f *fakeSource) Inputs(_ context.Context, ws string) (Inputs, error) {
	f.mu.Lock()
	f.loads[ws]++
	in, err := f.inputs[ws], f.err
	hook := f.onLoad
	f.mu.Unlock()
	if hook != nil {
		hook()
	}
	return in, err
}

func (f *fakeSource) set(ws string, in Inputs) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inputs[ws] = in
}

func (f *fakeSource) count(ws string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.loads[ws]
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func discard() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func newService(t *testing.T, src Source) (*Service, *clock) {
	t.Helper()
	clk := &clock{t: now}
	return NewService(src, clk.now, discard()), clk
}

func freeInputs(t *testing.T, version int64) Inputs {
	return Inputs{Plan: catalogue(t)["free"], Version: version, WorkspaceStatus: "active"}
}

func TestPolicyIsCachedForThirtySeconds(t *testing.T) {
	src := newFakeSource(map[string]Inputs{wsA: freeInputs(t, 1)})
	svc, clk := newService(t, src)
	ctx := context.Background()

	if _, err := svc.Policy(ctx, wsA); err != nil {
		t.Fatal(err)
	}
	clk.advance(29*time.Second + 999*time.Millisecond)
	if _, err := svc.Policy(ctx, wsA); err != nil {
		t.Fatal(err)
	}
	if n := src.count(wsA); n != 1 {
		t.Fatalf("loads after 29.999s = %d, want 1 (cached)", n)
	}

	clk.advance(time.Millisecond) // exactly 30s after the load
	if _, err := svc.Policy(ctx, wsA); err != nil {
		t.Fatal(err)
	}
	if n := src.count(wsA); n != 2 {
		t.Fatalf("loads at 30s = %d, want 2 (expired)", n)
	}
}

func TestPolicyCacheIsPerWorkspace(t *testing.T) {
	src := newFakeSource(map[string]Inputs{wsA: freeInputs(t, 1), wsB: freeInputs(t, 9)})
	svc, _ := newService(t, src)
	ctx := context.Background()

	a, _ := svc.Policy(ctx, wsA)
	b, _ := svc.Policy(ctx, wsB)
	if a.SubscriptionVersion != 1 || b.SubscriptionVersion != 9 {
		t.Fatalf("versions = %d, %d; want 1, 9", a.SubscriptionVersion, b.SubscriptionVersion)
	}
	svc.Invalidate(wsA)
	svc.Policy(ctx, wsA)
	svc.Policy(ctx, wsB)
	if src.count(wsA) != 2 || src.count(wsB) != 1 {
		t.Fatalf("loads = %d, %d; want 2, 1 (only A was cleared)", src.count(wsA), src.count(wsB))
	}
}

func TestInvalidateMakesTheNextPolicySeeTheWrite(t *testing.T) {
	src := newFakeSource(map[string]Inputs{wsA: freeInputs(t, 1)})
	svc, _ := newService(t, src)
	ctx := context.Background()

	before, _ := svc.Policy(ctx, wsA)
	src.set(wsA, Inputs{Plan: catalogue(t)["pro"], Version: 2, WorkspaceStatus: "active"})
	if cached, _ := svc.Policy(ctx, wsA); cached.SubscriptionVersion != before.SubscriptionVersion {
		t.Fatal("the change showed before the cache was cleared; the cache is not caching")
	}
	svc.Invalidate(wsA)
	after, _ := svc.Policy(ctx, wsA)
	if after.SubscriptionVersion != 2 || gateOf(t, after, "issue_count").Action != "off" {
		t.Fatalf("after Invalidate: %+v, want version 2 on the pro plan", after)
	}
}

func TestInvalidateAllClearsEveryWorkspace(t *testing.T) {
	src := newFakeSource(map[string]Inputs{wsA: freeInputs(t, 1), wsB: freeInputs(t, 1)})
	svc, _ := newService(t, src)
	ctx := context.Background()
	svc.Policy(ctx, wsA)
	svc.Policy(ctx, wsB)
	svc.InvalidateAll()
	svc.Policy(ctx, wsA)
	svc.Policy(ctx, wsB)
	if src.count(wsA) != 2 || src.count(wsB) != 2 {
		t.Fatalf("loads = %d, %d; want 2, 2", src.count(wsA), src.count(wsB))
	}
}

// A cached policy still names the period of the instant it is served, so a
// month boundary does not wait for the entry to expire.
func TestCachedPolicyIsBuiltAtTheInstantItIsServed(t *testing.T) {
	src := newFakeSource(map[string]Inputs{wsA: freeInputs(t, 1)})
	svc, clk := newService(t, src)
	clk.t = time.Date(2026, 8, 31, 23, 59, 50, 0, time.UTC)

	first, _ := svc.Policy(context.Background(), wsA)
	clk.advance(15 * time.Second) // 00:00:05 on 1 September, inside the 30s window
	second, _ := svc.Policy(context.Background(), wsA)
	if src.count(wsA) != 1 {
		t.Fatalf("loads = %d, want the second call served from the cache", src.count(wsA))
	}
	a, b := gateOf(t, first, "autopilot_runs"), gateOf(t, second, "autopilot_runs")
	if a.PeriodStart.Month() != time.August || b.PeriodStart.Month() != time.September {
		t.Errorf("periods start in %v then %v, want August then September", a.PeriodStart.Month(), b.PeriodStart.Month())
	}
	if !second.ValidUntil.After(first.ValidUntil) {
		t.Error("valid_until did not move with the clock")
	}
}

func TestSourceErrorIsReturnedAndNotCached(t *testing.T) {
	boom := errors.New("database down")
	src := newFakeSource(map[string]Inputs{wsA: freeInputs(t, 1)})
	src.err = boom
	svc, _ := newService(t, src)
	ctx := context.Background()

	if _, err := svc.Policy(ctx, wsA); !errors.Is(err, boom) {
		t.Fatalf("error = %v, want %v", err, boom)
	}
	src.mu.Lock()
	src.err = nil
	src.mu.Unlock()
	if _, err := svc.Policy(ctx, wsA); err != nil {
		t.Fatalf("after recovery: %v", err)
	}
	if src.count(wsA) != 2 {
		t.Fatalf("loads = %d, want a retry rather than a cached failure", src.count(wsA))
	}
}

// A read that began before a write and ends after it must not put the old data
// back in the cache.
func TestAWriteDuringALoadIsNotOverwrittenByTheOldRead(t *testing.T) {
	src := newFakeSource(map[string]Inputs{wsA: freeInputs(t, 1)})
	svc, _ := newService(t, src)
	ctx := context.Background()

	src.onLoad = func() {
		// The write lands after the read of version 1 and clears the entry.
		src.mu.Lock()
		src.onLoad = nil
		src.inputs[wsA] = freeInputs(t, 2)
		src.mu.Unlock()
		svc.Invalidate(wsA)
	}
	stale, _ := svc.Policy(ctx, wsA)
	if stale.SubscriptionVersion != 1 {
		t.Fatalf("the racing call returned version %d, want the version it read", stale.SubscriptionVersion)
	}
	next, _ := svc.Policy(ctx, wsA)
	if next.SubscriptionVersion != 2 {
		t.Fatalf("the next call returned version %d, want 2: the stale read was cached", next.SubscriptionVersion)
	}
}

func TestCacheIsBounded(t *testing.T) {
	src := newFakeSource(map[string]Inputs{})
	svc, _ := newService(t, src)
	svc.maxEntries = 3
	for i := 0; i < 10; i++ {
		ws := "00000000-0000-0000-0000-00000000000" + string(rune('0'+i))
		src.set(ws, freeInputs(t, 1))
		if _, err := svc.Policy(context.Background(), ws); err != nil {
			t.Fatal(err)
		}
	}
	if n := svc.size(); n > 3 {
		t.Fatalf("cache holds %d entries, want at most 3", n)
	}
}

func serveGET(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestHandlerServesThePolicy(t *testing.T) {
	src := newFakeSource(map[string]Inputs{wsA: freeInputs(t, 4)})
	svc, _ := newService(t, src)
	h := svc.Handler()

	rec := serveGET(h, "/api/v1/internal/entitlement-policies/"+wsA)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content type = %q", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("cache-control = %q, want no-store", cc)
	}
	for _, want := range []string{`"subscription_version":4`, `"schema_version":1`, `"valid_for_seconds":30`, `"autopilot_runs"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("body lacks %s: %s", want, rec.Body)
		}
	}
	if src.count(wsA) != 1 {
		t.Errorf("loads = %d", src.count(wsA))
	}
}

// The workspace in the path is the key, in the form Multica sends it.
func TestHandlerNormalisesTheWorkspaceID(t *testing.T) {
	src := newFakeSource(map[string]Inputs{wsA: freeInputs(t, 1)})
	svc, _ := newService(t, src)
	rec := serveGET(svc.Handler(), "/api/v1/internal/entitlement-policies/"+strings.ToUpper(wsA))
	if rec.Code != http.StatusOK || src.count(wsA) != 1 {
		t.Fatalf("status %d, loads %d; want 200 for the lower-cased id", rec.Code, src.count(wsA))
	}
}

func TestHandlerRejectsABadWorkspaceID(t *testing.T) {
	src := newFakeSource(nil)
	svc, _ := newService(t, src)
	for _, id := range []string{"not-a-uuid", "1", "11111111-1111-1111-1111-11111111111", "11111111111111111111111111111111"} {
		rec := serveGET(svc.Handler(), "/api/v1/internal/entitlement-policies/"+id)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%q: status %d, want 404", id, rec.Code)
		}
	}
	if len(src.loads) != 0 {
		t.Errorf("a bad id reached the database: %v", src.loads)
	}
}

func TestHandlerAnswers500WithoutLeakingTheCause(t *testing.T) {
	src := newFakeSource(map[string]Inputs{wsA: freeInputs(t, 1)})
	src.err = errors.New(`pq: password authentication failed for user "platform"`)
	svc, _ := newService(t, src)
	rec := serveGET(svc.Handler(), "/api/v1/internal/entitlement-policies/"+wsA)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "password") {
		t.Errorf("the body leaks the cause: %s", rec.Body)
	}
}

func TestHandlerOnlyAnswersGET(t *testing.T) {
	svc, _ := newService(t, newFakeSource(nil))
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		rec := httptest.NewRecorder()
		svc.Handler().ServeHTTP(rec, httptest.NewRequest(m, "/api/v1/internal/entitlement-policies/"+wsA, nil))
		if rec.Code == http.StatusOK {
			t.Errorf("%s answered 200", m)
		}
	}
}

// --- Loader: how the inputs are assembled from the three tables.

type fakePlans struct{ plans map[string]plan.Plan }

func (f fakePlans) Get(_ context.Context, key string) (plan.Plan, bool, error) {
	p, ok := f.plans[key]
	return p, ok, nil
}
func (f fakePlans) List(context.Context) ([]plan.Plan, error) { return nil, nil }

type fakeSubs struct {
	rows map[string]subscription.Subscription
}

func (f fakeSubs) Get(_ context.Context, ws string) (subscription.Subscription, bool, error) {
	s, ok := f.rows[ws]
	return s, ok, nil
}

type fakeStatus map[string]string

func (f fakeStatus) Status(_ context.Context, ws string) (string, error) {
	if s, ok := f[ws]; ok {
		return s, nil
	}
	return "active", nil
}

func TestLoaderTreatsAWorkspaceWithNoRowAsFreeAtVersionZero(t *testing.T) {
	l := Loader{Plans: fakePlans{catalogue(t)}, Subscriptions: fakeSubs{}, Status: fakeStatus{}}
	in, err := l.Inputs(context.Background(), wsA)
	if err != nil {
		t.Fatal(err)
	}
	if in.Plan.Key != "free" || in.Version != 0 || in.Overrides != nil || in.WorkspaceStatus != "active" {
		t.Fatalf("inputs = %+v, want the free plan at version 0", in)
	}
}

func TestLoaderReadsThePlanOverridesVersionAndWorkspaceState(t *testing.T) {
	l := Loader{
		Plans: fakePlans{catalogue(t)},
		Subscriptions: fakeSubs{rows: map[string]subscription.Subscription{wsA: {
			WorkspaceID: wsA, PlanKey: "pro", Version: 5, LimitOverrides: plan.Limits{"autopilot_runs": i64(9)},
		}}},
		Status: fakeStatus{wsA: "suspended"},
	}
	in, err := l.Inputs(context.Background(), wsA)
	if err != nil {
		t.Fatal(err)
	}
	if in.Plan.Key != "pro" || in.Version != 5 || *in.Overrides["autopilot_runs"] != 9 || in.WorkspaceStatus != "suspended" {
		t.Fatalf("inputs = %+v", in)
	}
}

// A subscription naming a plan that is gone is a data error. The caller gets an
// error, not a made-up plan: Multica treats the failure as "platform
// unavailable" and does not enforce.
func TestLoaderFailsWhenThePlanIsMissing(t *testing.T) {
	l := Loader{
		Plans:         fakePlans{catalogue(t)},
		Subscriptions: fakeSubs{rows: map[string]subscription.Subscription{wsA: {WorkspaceID: wsA, PlanKey: "gone"}}},
		Status:        fakeStatus{},
	}
	if _, err := l.Inputs(context.Background(), wsA); err == nil {
		t.Fatal("no error for a subscription on a missing plan")
	}
	empty := Loader{Plans: fakePlans{}, Subscriptions: fakeSubs{}, Status: fakeStatus{}}
	if _, err := empty.Inputs(context.Background(), wsA); err == nil {
		t.Fatal("no error when even the free plan is missing")
	}
}

func TestHandlerRefusesAMalformedUserHeader(t *testing.T) {
	src := newFakeSource(map[string]Inputs{wsA: freeInputs(t, 1)})
	svc, _ := newService(t, src)
	h := svc.Handler()
	path := "/api/v1/internal/entitlement-policies/" + wsA

	get := func(values ...string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		for _, v := range values {
			req.Header.Add("X-User-ID", v)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	for _, bad := range [][]string{
		{"not-a-uuid"},
		{"11111111111111111111111111111111"},
		{wsA, wsB},
	} {
		rec := get(bad...)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("X-User-ID %q: status = %d, want 401", bad, rec.Code)
		}
		want := `{"code":"unauthorized","error":"X-User-ID must be one UUID"}`
		if got := strings.TrimSpace(rec.Body.String()); got != want {
			t.Fatalf("X-User-ID %q: body = %s", bad, got)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Fatalf("Content-Type = %q", ct)
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
			t.Fatalf("Cache-Control = %q", cc)
		}
	}
	if n := src.loads[wsA]; n != 0 {
		t.Fatalf("loads after refused requests = %d, want 0", n)
	}

	for name, values := range map[string][]string{
		"no header": nil,
		"lower":     {wsA},
		"upper":     {strings.ToUpper(wsB)},
	} {
		if rec := get(values...); rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, body %s", name, rec.Code, rec.Body)
		}
	}
}

// reachesWithin30Seconds is the claim of spec R6 sections 3c.4 and 3c.5: a
// limit override written without an invalidation in this process reaches the
// policy at the latest when the cached inputs expire, 30 seconds after the load.
func reachesWithin30Seconds(t *testing.T, override int64) {
	t.Helper()
	src := newFakeSource(map[string]Inputs{wsA: freeInputs(t, 1)})
	svc, clk := newService(t, src)
	ctx := context.Background()

	if _, err := svc.Policy(ctx, wsA); err != nil {
		t.Fatal(err)
	}
	in := freeInputs(t, 2)
	in.Overrides = plan.Limits{plan.GateAgents: &override}
	src.set(wsA, in)

	clk.advance(29*time.Second + 999*time.Millisecond)
	stale, err := svc.Policy(ctx, wsA)
	if err != nil {
		t.Fatal(err)
	}
	if g := stale.Gates[plan.GateAgents]; g.Limit != nil && *g.Limit == override {
		t.Fatalf("the override showed before the cache expired")
	}

	clk.advance(time.Millisecond)
	fresh, err := svc.Policy(ctx, wsA)
	if err != nil {
		t.Fatal(err)
	}
	if g := fresh.Gates[plan.GateAgents]; g.Limit == nil || *g.Limit != override {
		t.Fatalf("the override is not in the policy 30 seconds after the load: %+v", g)
	}
	if fresh.ValidForSeconds != 30 {
		t.Fatalf("valid_for_seconds = %d, want 30", fresh.ValidForSeconds)
	}
}

// TestLimitOverridesReachPolicyWithin30Seconds: a limit written by an override
// reaches the policy within 30 seconds.
func TestLimitOverridesReachPolicyWithin30Seconds(t *testing.T) {
	reachesWithin30Seconds(t, 25)
}

// TestApprovedOverrideReachesPolicyWithin30Seconds: a quota request approved as an
// override reaches the policy within 30 seconds.
func TestApprovedOverrideReachesPolicyWithin30Seconds(t *testing.T) {
	reachesWithin30Seconds(t, 12)
}
