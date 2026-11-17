package planlimit

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/entitlement"
	"github.com/multica-ai/multica/server/internal/entitlement/entitlementtest"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// fakeStore records the order of what runs in the transaction.
type fakeStore struct {
	events   *[]string
	lockErr  error
	keys     []string
	indebted bool
}

func (f *fakeStore) OwnerHasUnpaidUsageInvoice(context.Context, pgtype.UUID) (bool, error) {
	return f.indebted, nil
}

func (f *fakeStore) LockPlanGate(_ context.Context, arg db.LockPlanGateParams) error {
	*f.events = append(*f.events, "lock:"+arg.Scope+"|"+arg.Gate)
	return f.lockErr
}

func (f *fakeStore) ListOwnedWorkspacePlanKeys(context.Context, pgtype.UUID) ([]string, error) {
	*f.events = append(*f.events, "list")
	return f.keys, nil
}

func pg(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: true} }

func counting(events *[]string, n int64) CountFunc {
	return func(context.Context) (int64, error) {
		*events = append(*events, "count")
		return n, nil
	}
}

func limitDecision(action entitlement.Action, limit int) entitlement.Decision {
	d := entitlement.Decision{Gate: entitlement.Gate{Action: action}, PolicyRevision: 2}
	if action == entitlement.ActionEnforce {
		d.Gate.Limit = &limit
	}
	return d
}

func resolved(t *testing.T, gate entitlement.GateName, d *entitlement.Decision) (Policy, uuid.UUID) {
	t.Helper()
	ws := uuid.New()
	stub := entitlementtest.New()
	if d != nil {
		stub.Set(ws, gate, *d)
	}
	return Resolve(context.Background(), stub, pg(ws), gate), ws
}

func TestResolve(t *testing.T) {
	ws := uuid.New()
	for name, tc := range map[string]struct {
		provider entitlement.Provider
		decision *entitlement.Decision
		want     Policy
	}{
		"no provider is off":      {provider: nil, want: Policy{Gate: entitlement.GateAgents}},
		"a missing gate is off":   {provider: entitlementtest.New(), want: Policy{Gate: entitlement.GateAgents}},
		"enforce carries limit":   {decision: ptr(limitDecision(entitlement.ActionEnforce, 3)), want: Policy{Gate: entitlement.GateAgents, Enforced: true, Limit: 3}},
		"enforce with limit zero": {decision: ptr(limitDecision(entitlement.ActionEnforce, 0)), want: Policy{Gate: entitlement.GateAgents, Enforced: true, Limit: 0}},
		"observe does not block":  {decision: ptr(limitDecision(entitlement.ActionObserve, 0)), want: Policy{Gate: entitlement.GateAgents}},
		"off":                     {decision: ptr(limitDecision(entitlement.ActionOff, 0)), want: Policy{Gate: entitlement.GateAgents}},
	} {
		t.Run(name, func(t *testing.T) {
			provider := tc.provider
			if tc.decision != nil {
				stub := entitlementtest.New()
				stub.Set(ws, entitlement.GateAgents, *tc.decision)
				provider = stub
			}
			if got := Resolve(context.Background(), provider, pg(ws), entitlement.GateAgents); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Resolve = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestResolveNilWorkspaceIsOff(t *testing.T) {
	stub := entitlementtest.New()
	stub.Set(uuid.Nil, entitlement.GateAgents, limitDecision(entitlement.ActionEnforce, 1))
	if p := Resolve(context.Background(), stub, pg(uuid.Nil), entitlement.GateAgents); p.Enforced {
		t.Errorf("Resolve = %+v, want off for the nil workspace", p)
	}
}

func ptr[T any](v T) *T { return &v }

// An off gate takes no lock and counts nothing: a self-hosted install, and a
// plan without the quota, run exactly the code they ran before.
func TestAdmitOffGateTouchesNothing(t *testing.T) {
	var events []string
	p := Policy{Gate: entitlement.GateAgents}
	if err := p.Admit(context.Background(), &fakeStore{events: &events}, pg(uuid.New()), 1, counting(&events, 0)); err != nil {
		t.Fatalf("Admit = %v, want nil", err)
	}
	if err := p.Precheck(context.Background(), pg(uuid.New()), 1, counting(&events, 99)); err != nil {
		t.Fatalf("Precheck = %v, want nil", err)
	}
	if len(events) != 0 {
		t.Fatalf("an off gate ran %v", events)
	}
}

// The lock comes before the count, on the pair (workspace, gate): two creations
// at the last slot queue on it, and the second counts what the first committed.
func TestAdmitTakesTheAdvisoryLockBeforeCounting(t *testing.T) {
	var events []string
	ws := uuid.New()
	p := Policy{Gate: entitlement.GateStorageBytes, Enforced: true, Limit: 10}
	if err := p.Admit(context.Background(), &fakeStore{events: &events}, pg(ws), 1, counting(&events, 0)); err != nil {
		t.Fatal(err)
	}
	want := []string{"lock:" + ws.String() + "|storage_bytes", "count"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %q, want %q", events, want)
	}
}

func TestAdmitLockFailureIsAnErrorNotAnAdmission(t *testing.T) {
	var events []string
	boom := errors.New("lock failed")
	p := Policy{Gate: entitlement.GateAgents, Enforced: true, Limit: 5}
	err := p.Admit(context.Background(), &fakeStore{events: &events, lockErr: boom}, pg(uuid.New()), 1, counting(&events, 0))
	if !errors.Is(err, boom) {
		t.Fatalf("Admit = %v, want the lock error", err)
	}
	var reached *ReachedError
	if errors.As(err, &reached) {
		t.Fatal("a failed lock must not read as a plan limit")
	}
	for _, e := range events {
		if e == "count" {
			t.Fatal("counted without the lock")
		}
	}
}

func TestAdmitCountFailureIsAnError(t *testing.T) {
	var events []string
	boom := errors.New("count failed")
	p := Policy{Gate: entitlement.GateAgents, Enforced: true, Limit: 5}
	err := p.Admit(context.Background(), &fakeStore{events: &events}, pg(uuid.New()), 1,
		func(context.Context) (int64, error) { return 0, boom })
	if !errors.Is(err, boom) {
		t.Fatalf("Admit = %v, want the count error", err)
	}
}

// used + requested > limit refuses; equal passes. Admit and Precheck decide
// alike: the precheck is the same rule without the lock.
func TestAdmitAndPrecheckBoundary(t *testing.T) {
	cases := []struct {
		name                   string
		limit, used, requested int64
		refused                bool
	}{
		{"under the limit", 3, 1, 1, false},
		{"filling the last slot", 3, 2, 1, false},
		{"at the limit", 3, 3, 1, true},
		{"over the limit already", 3, 5, 1, true},
		{"empty with limit 0", 0, 0, 1, true},
		{"storage fits exactly", 100, 60, 40, false},
		{"storage one byte over", 100, 60, 41, true},
		{"storage with a large upload on an empty workspace", 100, 0, 101, true},
		{"large numbers do not overflow", 214748364800, 214748364000, 800, false},
		{"large numbers one over", 214748364800, 214748364000, 801, true},
	}
	for _, tc := range cases {
		for _, mode := range []string{"admit", "precheck"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				var events []string
				p := Policy{Gate: entitlement.GateStorageBytes, Enforced: true, Limit: tc.limit}
				count := func(context.Context) (int64, error) { return tc.used, nil }
				var err error
				if mode == "admit" {
					err = p.Admit(context.Background(), &fakeStore{events: &events}, pg(uuid.New()), tc.requested, count)
				} else {
					err = p.Precheck(context.Background(), pg(uuid.New()), tc.requested, count)
				}
				var reached *ReachedError
				switch {
				case tc.refused && !errors.As(err, &reached):
					t.Fatalf("= %v, want ReachedError", err)
				case !tc.refused && err != nil:
					t.Fatalf("= %v, want nil", err)
				}
				if tc.refused && (reached.Used != tc.used || reached.Limit != tc.limit || reached.Gate != entitlement.GateStorageBytes) {
					t.Errorf("error = %+v, want gate storage_bytes used %d limit %d", reached, tc.used, tc.limit)
				}
			})
		}
	}
}

// A precheck takes no lock: it only lets a path that knows the size in advance
// refuse before it stores anything. The locked Admit stays the authority.
func TestPrecheckTakesNoLock(t *testing.T) {
	var events []string
	p := Policy{Gate: entitlement.GateStorageBytes, Enforced: true, Limit: 10}
	_ = p.Precheck(context.Background(), pg(uuid.New()), 1, counting(&events, 0))
	if !reflect.DeepEqual(events, []string{"count"}) {
		t.Fatalf("events = %q, want only the count", events)
	}
}

func TestReachedErrorBodyIsTheContract(t *testing.T) {
	err := &ReachedError{Gate: entitlement.GateAgents, Limit: 3, Used: 3}
	raw, e := json.Marshal(err.Body())
	if e != nil {
		t.Fatal(e)
	}
	var got map[string]any
	if e := json.Unmarshal(raw, &got); e != nil {
		t.Fatal(e)
	}
	want := map[string]any{
		"error": got["error"],
		"code":  "plan_limit_reached",
		"gate":  "agents",
		"limit": float64(3),
		"used":  float64(3),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("body = %v, want %v", got, want)
	}
	if msg, ok := got["error"].(string); !ok || msg == "" {
		t.Errorf("error = %v, want a message", got["error"])
	}
	// Numbers are JSON numbers, not strings: the client's schema is z.number().
	if strings.Contains(string(raw), `"limit":"`) || strings.Contains(string(raw), `"used":"`) {
		t.Errorf("limit or used is a string: %s", raw)
	}
}

func TestReachedErrorBodyKeepsLargeNumbersExact(t *testing.T) {
	err := &ReachedError{Gate: entitlement.GateStorageBytes, Limit: 214748364800, Used: 214748364000}
	raw, _ := json.Marshal(err.Body())
	if !strings.Contains(string(raw), `"limit":214748364800`) || !strings.Contains(string(raw), `"used":214748364000`) {
		t.Errorf("body = %s", raw)
	}
}

func TestReachedErrorIsAnError(t *testing.T) {
	var err error = &ReachedError{Gate: entitlement.GateRuntimes, Limit: 2, Used: 2}
	var reached *ReachedError
	if !errors.As(err, &reached) || !strings.Contains(err.Error(), "runtimes") {
		t.Fatalf("err = %v", err)
	}
}

func TestAdmitUsesTheResolvedPolicy(t *testing.T) {
	d := limitDecision(entitlement.ActionEnforce, 1)
	p, ws := resolved(t, entitlement.GateRuntimes, &d)
	var events []string
	err := p.Admit(context.Background(), &fakeStore{events: &events}, pg(ws), 1, counting(&events, 1))
	var reached *ReachedError
	if !errors.As(err, &reached) || reached.Gate != entitlement.GateRuntimes || reached.Limit != 1 || reached.Used != 1 {
		t.Fatalf("Admit = %v", err)
	}
}

// AdmitEach is the rule for a batch that must not fail as a whole: items are
// admitted in order while they fit, and one that does not fit is refused and
// takes nothing, so a smaller item after it can still fit.
func TestAdmitEach(t *testing.T) {
	p := Policy{Gate: entitlement.GateStorageBytes, Enforced: true, Limit: 10}
	var events []string
	ws := uuid.New()
	refusals, err := p.AdmitEach(context.Background(), &fakeStore{events: &events}, pg(ws), []int64{4, 7, 5, 2}, counting(&events, 1))
	if err != nil {
		t.Fatal(err)
	}
	// used 1: 4 fits (5), 7 does not (12 > 10), 5 fits (10), 2 does not (12 > 10).
	if refusals[0] != nil || refusals[2] != nil {
		t.Errorf("refusals = %v, want items 0 and 2 admitted", refusals)
	}
	if r := refusals[1]; r == nil || r.Used != 5 || r.Limit != 10 || r.Gate != entitlement.GateStorageBytes {
		t.Errorf("item 1 = %+v, want refused with used 5 limit 10", r)
	}
	if r := refusals[3]; r == nil || r.Used != 10 {
		t.Errorf("item 3 = %+v, want refused with used 10", r)
	}
	if want := []string{"lock:" + ws.String() + "|storage_bytes", "count"}; !reflect.DeepEqual(events, want) {
		t.Errorf("events = %q, want the lock before the count: %q", events, want)
	}
}

func TestAdmitEachOffGateAdmitsEverythingAndTouchesNothing(t *testing.T) {
	var events []string
	refusals, err := (Policy{Gate: entitlement.GateStorageBytes}).AdmitEach(context.Background(), &fakeStore{events: &events}, pg(uuid.New()), []int64{1 << 40, 1}, counting(&events, 0))
	if err != nil || len(refusals) != 2 || refusals[0] != nil || refusals[1] != nil {
		t.Fatalf("refusals = %v, err = %v", refusals, err)
	}
	if len(events) != 0 {
		t.Fatalf("events = %q", events)
	}
}
