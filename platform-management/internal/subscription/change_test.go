package subscription

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/plan"
)

const (
	wsID    = "bbbbbbbb-0000-0000-0000-000000000001"
	wsName  = "Acme"
	staffID = "aaaaaaaa-0000-0000-0000-000000000003"
	staffEm = "billing@example.com"
)

var t0 = time.Date(2030, 5, 6, 7, 8, 9, 123456789, time.UTC)

func i64(n int64) *int64 { return &n }

func catalogue(t *testing.T) map[string]plan.Plan {
	t.Helper()
	all, err := plan.Catalogue()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]plan.Plan{}
	for _, p := range all {
		out[p.Key] = p
	}
	return out
}

// fakeStore is the tables a change touches, with transaction semantics: a
// failing fn leaves nothing behind.
type fakeStore struct {
	workspaces map[string]string
	plans      map[string]plan.Plan
	subs       map[string]Subscription
	audits     []audit.Entry

	failAudit error
	// raceOnInsert makes Insert lose to another transaction.
	raceOnInsert bool
	txs          int
}

func newFakeStore(t *testing.T) *fakeStore {
	plans := catalogue(t)
	archived := plans["pro"]
	archived.Key, archived.Name = "legacy", "Legacy"
	at := t0.Add(-time.Hour)
	archived.ArchivedAt = &at
	plans["legacy"] = archived
	return &fakeStore{workspaces: map[string]string{wsID: wsName}, plans: plans, subs: map[string]Subscription{}}
}

func (s *fakeStore) InTx(_ context.Context, fn func(Tx) error) error {
	s.txs++
	subs := map[string]Subscription{}
	for k, v := range s.subs {
		subs[k] = v
	}
	audits := append([]audit.Entry(nil), s.audits...)
	if err := fn(&fakeTx{s: s}); err != nil {
		s.subs, s.audits = subs, audits
		return err
	}
	return nil
}

type fakeTx struct{ s *fakeStore }

func (t *fakeTx) WorkspaceName(_ context.Context, id string) (string, bool, error) {
	n, ok := t.s.workspaces[id]
	return n, ok, nil
}

func (t *fakeTx) Get(_ context.Context, ws string) (Subscription, bool, error) {
	v, ok := t.s.subs[ws]
	return v, ok, nil
}

func (t *fakeTx) Insert(_ context.Context, sub Subscription) (Subscription, bool, error) {
	if t.s.raceOnInsert {
		return Subscription{}, false, nil
	}
	sub.Version, sub.UpdatedAt = 1, t0
	t.s.subs[sub.WorkspaceID] = sub
	return sub, true, nil
}

func (t *fakeTx) Update(_ context.Context, sub Subscription) (Subscription, error) {
	sub.Version = t.s.subs[sub.WorkspaceID].Version + 1
	sub.UpdatedAt = t0
	t.s.subs[sub.WorkspaceID] = sub
	return sub, nil
}

func (t *fakeTx) Plan(_ context.Context, key string) (plan.Plan, bool, error) {
	p, ok := t.s.plans[key]
	return p, ok, nil
}

func (t *fakeTx) InsertAuditEvent(_ context.Context, e audit.Entry) error {
	if t.s.failAudit != nil {
		return t.s.failAudit
	}
	t.s.audits = append(t.s.audits, e)
	return nil
}

// recordingCache records what the service cleared.
type recordingCache struct {
	workspaces []string
	all        int
}

func (c *recordingCache) Invalidate(ws string) { c.workspaces = append(c.workspaces, ws) }
func (c *recordingCache) InvalidateAll()       { c.all++ }

func newService(t *testing.T) (*Service, *fakeStore, *recordingCache) {
	t.Helper()
	store, cache := newFakeStore(t), &recordingCache{}
	return NewService(store, cache), store, cache
}

func proChange() Change {
	return Change{
		WorkspaceID: wsID, PlanKey: "pro", Seats: 5, Interval: IntervalMonth,
		Reason: "paid by bank transfer", ActorID: staffID, ActorLabel: staffEm,
	}
}

func apiErr(t *testing.T, err error, status int, code string) {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("error = %v, want *Error %d %s", err, status, code)
	}
	if e.Status != status || e.Code != code {
		t.Fatalf("error = %d %s (%s), want %d %s", e.Status, e.Code, e.Message, status, code)
	}
}

func TestFirstChangeCreatesTheRowAtVersionOne(t *testing.T) {
	svc, store, cache := newService(t)
	res, err := svc.Apply(context.Background(), proChange())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || res.Subscription.Version != 1 || res.Subscription.Status != StatusActive {
		t.Fatalf("result = %+v, want a changed active row at version 1", res)
	}
	if got := store.subs[wsID]; got.PlanKey != "pro" || got.Seats != 5 {
		t.Fatalf("stored = %+v", got)
	}
	if !reflect.DeepEqual(cache.workspaces, []string{wsID}) {
		t.Fatalf("cache cleared for %v, want [%s]", cache.workspaces, wsID)
	}
}

// Every change moves the version up by exactly one.
func TestEveryChangeIncrementsTheVersionByOne(t *testing.T) {
	svc, store, _ := newService(t)
	ctx := context.Background()
	c := proChange()
	for want := int64(1); want <= 4; want++ {
		res, err := svc.Apply(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		if res.Subscription.Version != want || store.subs[wsID].Version != want {
			t.Fatalf("after change %d: version %d (stored %d)", want, res.Subscription.Version, store.subs[wsID].Version)
		}
		c.Seats++ // each round changes something
	}
}

func TestChangeWritesOneStaffAuditRowAndClearsTheCache(t *testing.T) {
	svc, store, cache := newService(t)
	if _, err := svc.Apply(context.Background(), proChange()); err != nil {
		t.Fatal(err)
	}
	if len(store.audits) != 1 {
		t.Fatalf("audit rows = %d, want 1", len(store.audits))
	}
	e := store.audits[0]
	if e.Action != audit.ActionSubscriptionChanged || e.ActorType != audit.ActorStaff || e.ActorID != staffID || e.ActorLabel != staffEm {
		t.Errorf("entry = %+v", e)
	}
	if e.WorkspaceID != wsID || e.TargetType != "workspace" || e.TargetID != wsID || e.TargetLabel != wsName {
		t.Errorf("target = %+v", e)
	}
	if e.Detail["reason"] != "paid by bank transfer" {
		t.Errorf("reason missing from detail: %v", e.Detail)
	}
	if len(cache.workspaces) != 1 || cache.all != 0 {
		t.Errorf("cache cleared %v / all %d", cache.workspaces, cache.all)
	}
}

func TestMissingReasonIs400AndNothingIsWritten(t *testing.T) {
	for _, reason := range []string{"", "   ", "\n\t"} {
		svc, store, cache := newService(t)
		c := proChange()
		c.Reason = reason
		_, err := svc.Apply(context.Background(), c)
		apiErr(t, err, 400, "reason_required")
		if len(store.subs) != 0 || len(store.audits) != 0 || store.txs != 0 || len(cache.workspaces) != 0 {
			t.Errorf("reason %q: wrote %d rows, %d audits, %d txs, cleared %v", reason, len(store.subs), len(store.audits), store.txs, cache.workspaces)
		}
	}
}

func changes(t *testing.T, e audit.Entry) map[string]map[string]any {
	t.Helper()
	raw, ok := e.Detail["changes"].(map[string]any)
	if !ok {
		t.Fatalf("detail has no changes object: %v", e.Detail)
	}
	out := map[string]map[string]any{}
	for k, v := range raw {
		out[k] = v.(map[string]any)
	}
	return out
}

func keys(m map[string]map[string]any) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// The audit detail names the fields that changed, and no others.
func TestAuditDetailListsOnlyTheChangedFields(t *testing.T) {
	svc, store, _ := newService(t)
	ctx := context.Background()
	if _, err := svc.Apply(ctx, proChange()); err != nil {
		t.Fatal(err)
	}

	c := proChange()
	c.Seats = 9
	if _, err := svc.Apply(ctx, c); err != nil {
		t.Fatal(err)
	}
	got := changes(t, store.audits[1])
	if !reflect.DeepEqual(keys(got), []string{"seats"}) {
		t.Fatalf("changed fields = %v, want only seats", keys(got))
	}
	if got["seats"]["from"] != 5 || got["seats"]["to"] != 9 {
		t.Errorf("seats change = %v", got["seats"])
	}

	c.PlanKey, c.Interval = "business", IntervalYear
	end := t0.Add(24 * time.Hour)
	c.CurrentPeriodEnd = &end
	c.LimitOverrides = plan.Limits{"issue_count": i64(500), "autopilot_runs": nil}
	c.CancelAtPeriodEnd = true
	if _, err := svc.Apply(ctx, c); err != nil {
		t.Fatal(err)
	}
	got = changes(t, store.audits[2])
	want := []string{"cancel_at_period_end", "current_period_end", "interval", "limit_overrides", "plan"}
	if !reflect.DeepEqual(keys(got), want) {
		t.Fatalf("changed fields = %v, want %v", keys(got), want)
	}
	if got["plan"]["from"] != "pro" || got["plan"]["to"] != "business" {
		t.Errorf("plan change = %v", got["plan"])
	}
	if got["current_period_end"]["from"] != nil {
		t.Errorf("period end was unset, from = %v", got["current_period_end"]["from"])
	}
	if to := got["current_period_end"]["to"]; to != end.Truncate(time.Microsecond).Format(time.RFC3339Nano) {
		t.Errorf("period end to = %v", to)
	}
	overrides := got["limit_overrides"]["to"].(map[string]any)
	if v, ok := overrides["autopilot_runs"]; !ok || v != nil || overrides["issue_count"] != int64(500) {
		t.Errorf("overrides to = %v, want 500 and an explicit null", overrides)
	}
}

// audit.Record redacts any key that contains "key", which would hide plan_key.
// The detail names the field "plan", and no field name is redacted.
func TestAuditDetailSurvivesRedaction(t *testing.T) {
	svc, store, _ := newService(t)
	c := proChange()
	end := t0.Add(time.Hour)
	c.CurrentPeriodStart, c.CurrentPeriodEnd, c.TrialEnd, c.GraceUntil = &t0, &end, &end, &end
	c.CancelAtPeriodEnd = true
	c.Interval = IntervalYear
	c.LimitOverrides = plan.Limits{"agents": i64(4), "api_rate_per_minute": i64(9)}
	if _, err := svc.Apply(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	detail := store.audits[0].Detail
	if got := audit.Redact(detail); !reflect.DeepEqual(got, detail) {
		t.Fatalf("redaction changed the detail:\n got %v\nwant %v", got, detail)
	}
	if got := changes(t, store.audits[0]); len(got) != 9 || got["plan"]["to"] != "pro" {
		t.Fatalf("a first change from the free defaults shows %v", keys(got))
	}
}

// Nothing changed, nothing is written: no version step, no audit row, no cache clear.
func TestUnchangedWriteDoesNothing(t *testing.T) {
	svc, store, cache := newService(t)
	ctx := context.Background()
	if _, err := svc.Apply(ctx, proChange()); err != nil {
		t.Fatal(err)
	}
	*cache = recordingCache{}

	res, err := svc.Apply(ctx, proChange())
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed || res.Subscription.Version != 1 || store.subs[wsID].Version != 1 {
		t.Fatalf("result = %+v, stored version %d; want no change at version 1", res, store.subs[wsID].Version)
	}
	if len(store.audits) != 1 || len(cache.workspaces) != 0 {
		t.Fatalf("audits %d, cache cleared %v; want neither", len(store.audits), cache.workspaces)
	}
}

// A workspace with no row is on the free plan: a write that says so creates nothing.
func TestAWriteThatMatchesTheImplicitFreePlanCreatesNoRow(t *testing.T) {
	svc, store, cache := newService(t)
	res, err := svc.Apply(context.Background(), Change{
		WorkspaceID: wsID, PlanKey: "free", Interval: IntervalMonth, Reason: "reset", ActorID: staffID, ActorLabel: staffEm,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed || res.Subscription.Version != 0 || len(store.subs) != 0 || len(store.audits) != 0 || len(cache.workspaces) != 0 {
		t.Fatalf("result = %+v, rows %d, audits %d", res, len(store.subs), len(store.audits))
	}
}

// An instant is the same instant in any zone, and Postgres keeps microseconds.
func TestTimesCompareAsInstantsAtMicrosecondPrecision(t *testing.T) {
	svc, store, _ := newService(t)
	ctx := context.Background()
	c := proChange()
	end := time.Date(2030, 6, 1, 0, 0, 0, 123456789, time.UTC)
	c.CurrentPeriodEnd = &end
	if _, err := svc.Apply(ctx, c); err != nil {
		t.Fatal(err)
	}
	if got := store.subs[wsID].CurrentPeriodEnd; !got.Equal(end.Truncate(time.Microsecond)) {
		t.Fatalf("stored %v, want %v", got, end.Truncate(time.Microsecond))
	}

	same := end.In(time.FixedZone("x", 7*3600))
	c.CurrentPeriodEnd = &same
	res, err := svc.Apply(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed {
		t.Fatal("the same instant in another zone counted as a change")
	}
}

func TestUnknownAndArchivedPlans(t *testing.T) {
	svc, store, _ := newService(t)
	ctx := context.Background()

	c := proChange()
	c.PlanKey = "nope"
	_, err := svc.Apply(ctx, c)
	apiErr(t, err, 400, "unknown_plan")

	// A new assignment to an archived plan is refused.
	c.PlanKey = "legacy"
	_, err = svc.Apply(ctx, c)
	apiErr(t, err, 400, "plan_archived")
	if len(store.subs) != 0 {
		t.Fatal("a refused change wrote a row")
	}

	// A subscriber who is already on the archived plan keeps it.
	store.subs[wsID] = Subscription{WorkspaceID: wsID, PlanKey: "legacy", Status: StatusActive, Seats: 2, Interval: IntervalMonth, Version: 4}
	c.Seats = 3
	res, err := svc.Apply(ctx, c)
	if err != nil {
		t.Fatalf("an existing subscriber on an archived plan: %v", err)
	}
	if res.Subscription.Version != 5 || res.Subscription.PlanKey != "legacy" {
		t.Fatalf("result = %+v", res.Subscription)
	}
}

func TestSeatsMustReachThePlansMinimum(t *testing.T) {
	svc, _, _ := newService(t)
	c := proChange()
	c.PlanKey, c.Seats = "business", 4 // business needs 5
	_, err := svc.Apply(context.Background(), c)
	apiErr(t, err, 400, "seats_below_minimum")

	c.Seats = 5
	if _, err := svc.Apply(context.Background(), c); err != nil {
		t.Fatalf("seats at the minimum: %v", err)
	}
}

func TestInvalidFieldsAreRefused(t *testing.T) {
	bad := map[string]func(*Change){
		"negative seats":          func(c *Change) { c.Seats = -1 },
		"absurd seats":            func(c *Change) { c.Seats = 1 << 30 },
		"unknown interval":        func(c *Change) { c.Interval = "week" },
		"period ends first":       func(c *Change) { s, e := t0.Add(time.Hour), t0; c.CurrentPeriodStart, c.CurrentPeriodEnd = &s, &e },
		"unknown override":        func(c *Change) { c.LimitOverrides = plan.Limits{"typo": i64(1)} },
		"zero issue override":     func(c *Change) { c.LimitOverrides = plan.Limits{"issue_count": i64(0)} },
		"negative override":       func(c *Change) { c.LimitOverrides = plan.Limits{"agents": i64(-3)} },
		"missing plan key":        func(c *Change) { c.PlanKey = "" },
		"malformed plan key":      func(c *Change) { c.PlanKey = "Pro Plan" },
		"NUL in the reason":       func(c *Change) { c.Reason = "a\x00b" },
		"reason over 1000 chars":  func(c *Change) { c.Reason = strings.Repeat("x", 1001) },
		"no workspace":            func(c *Change) { c.WorkspaceID = "" },
		"workspace is not a uuid": func(c *Change) { c.WorkspaceID = "acme" },
	}
	for name, mutate := range bad {
		svc, store, _ := newService(t)
		c := proChange()
		mutate(&c)
		if _, err := svc.Apply(context.Background(), c); err == nil {
			t.Errorf("%s: accepted", name)
		} else if e := (*Error)(nil); !errors.As(err, &e) || e.Status != 400 {
			t.Errorf("%s: error = %v, want a 400", name, err)
		}
		if len(store.subs) != 0 || len(store.audits) != 0 {
			t.Errorf("%s: wrote something", name)
		}
	}
}

func TestUnknownWorkspaceIs404(t *testing.T) {
	svc, store, _ := newService(t)
	c := proChange()
	c.WorkspaceID = "cccccccc-0000-0000-0000-000000000009"
	_, err := svc.Apply(context.Background(), c)
	apiErr(t, err, 404, "not_found")
	if len(store.subs) != 0 {
		t.Fatal("a row was written for a workspace that does not exist")
	}
}

// If the audit row cannot be written, the change is not kept and the cache is
// left alone.
func TestFailedAuditWriteRollsTheChangeBack(t *testing.T) {
	svc, store, cache := newService(t)
	ctx := context.Background()
	if _, err := svc.Apply(ctx, proChange()); err != nil {
		t.Fatal(err)
	}
	*cache = recordingCache{}
	store.failAudit = errors.New("audit_event: disk full")

	c := proChange()
	c.Seats = 50
	if _, err := svc.Apply(ctx, c); err == nil {
		t.Fatal("no error when the audit write failed")
	}
	if got := store.subs[wsID]; got.Seats != 5 || got.Version != 1 {
		t.Fatalf("stored = %+v, want the change rolled back", got)
	}
	if len(cache.workspaces) != 0 {
		t.Fatalf("cache cleared for %v after a rolled-back change", cache.workspaces)
	}

	// The first write rolls back too: no row appears.
	svc2, store2, _ := newService(t)
	store2.failAudit = errors.New("disk full")
	if _, err := svc2.Apply(ctx, proChange()); err == nil || len(store2.subs) != 0 {
		t.Fatalf("first write: err %v, rows %d", err, len(store2.subs))
	}
}

// Two staff creating the first row at once: the loser is told to retry.
func TestLosingTheFirstInsertIs409(t *testing.T) {
	svc, store, cache := newService(t)
	store.raceOnInsert = true
	_, err := svc.Apply(context.Background(), proChange())
	apiErr(t, err, 409, "concurrent_change")
	if len(store.audits) != 0 || len(cache.workspaces) != 0 {
		t.Fatal("a lost insert left an audit row or cleared the cache")
	}
}

// Spec R6 section 3b.1 rule 5: an inactive plan keeps its subscribers: a seat
// purchase stays on that plan, and the next renewal is priced for it.
func TestInactivePlanKeepsExistingSubscriptionsRenewingAndBuyingSeats(t *testing.T) {
	svc, store, _ := newService(t)
	store.subs[wsID] = Subscription{WorkspaceID: wsID, PlanKey: "legacy", Status: StatusActive, Seats: 2, Interval: IntervalMonth, Version: 4}

	c := proChange()
	c.PlanKey = "legacy"
	c.Seats = 5
	res, err := svc.Apply(context.Background(), c)
	if err != nil {
		t.Fatalf("seat purchase on an inactive plan: %v", err)
	}
	if res.Subscription.PlanKey != "legacy" || res.Subscription.Seats != 5 {
		t.Fatalf("after the seat purchase: plan %q seats %d, want legacy with 5", res.Subscription.PlanKey, res.Subscription.Seats)
	}
	planKey, interval, seats := RenewalTerms(res.Subscription)
	if planKey != "legacy" || interval != IntervalMonth || seats != 5 {
		t.Fatalf("renewal terms = %s %s %d, want legacy month 5", planKey, interval, seats)
	}
}
