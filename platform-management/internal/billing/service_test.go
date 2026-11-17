package billing

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/paymentnotice"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
	"github.com/multica-ai/multica/platform-management/internal/workspacestatus"
	"github.com/multica-ai/multica/platform-management/workspacestate"
)

const (
	tWS    = "22222222-2222-2222-2222-222222222222"
	tStaff = "33333333-3333-3333-3333-333333333333"
	tUser  = "44444444-4444-4444-4444-444444444444"
)

// memory is an in-memory stand-in for the tables one billing transaction
// touches. memRunner copies it before fn and restores the copy when fn fails,
// as a rollback does.
type memory struct {
	orders    map[string]Order
	notices   map[string]paymentnotice.Notice
	subs      map[string]subscription.Subscription
	ws        map[string]workspacestate.Record
	wsReason  map[string]string
	audits    []audit.Entry
	collide   *int // the next n inserts report a transfer code collision; shared, so a rollback does not reset it
	auditFail string
	inserts   int
}

func newMemory() *memory {
	return &memory{
		collide: new(int), orders: map[string]Order{}, notices: map[string]paymentnotice.Notice{}, subs: map[string]subscription.Subscription{},
		ws: map[string]workspacestate.Record{}, wsReason: map[string]string{},
	}
}

func (m *memory) snapshot() *memory {
	c := *m
	c.orders, c.subs, c.ws, c.wsReason = maps.Clone(m.orders), maps.Clone(m.subs), maps.Clone(m.ws), maps.Clone(m.wsReason)
	c.notices = maps.Clone(m.notices)
	c.audits = slices.Clone(m.audits)
	return &c
}

type memRunner struct{ m *memory }

func (r *memRunner) InTx(_ context.Context, fn func(Tx) error) error {
	saved := r.m.snapshot()
	if err := fn(&memTx{m: r.m}); err != nil {
		*r.m = *saved
		return err
	}
	return nil
}

type memTx struct{ m *memory }

func (t *memTx) InsertOrder(_ context.Context, n NewOrder) (Order, bool, error) {
	if *t.m.collide > 0 {
		*t.m.collide--
		return Order{}, false, ErrTransferCodeTaken
	}
	if n.IdempotencyKey != "" {
		for _, o := range t.m.orders {
			if o.WorkspaceID == n.WorkspaceID && o.IdempotencyKey == n.IdempotencyKey {
				return Order{}, false, nil
			}
		}
	}
	t.m.inserts++
	o := Order{
		ID: fmt.Sprintf("aaaaaaaa-0000-0000-0000-%012d", t.m.inserts), WorkspaceID: n.WorkspaceID, Kind: n.Kind,
		PlanKey: n.PlanKey, Interval: n.Interval, Seats: n.Seats, Amount: n.Amount, VATAmount: n.VATAmount,
		Currency: n.Currency, Status: StatusPending, Provider: n.Provider, TransferCode: n.TransferCode,
		IdempotencyKey: n.IdempotencyKey, Buyer: n.Buyer, PeriodStart: n.PeriodStart, PeriodEnd: n.PeriodEnd,
		CreatedBy: n.CreatedBy, ExpiresAt: n.ExpiresAt, CreatedAt: n.CreatedAt,
	}
	t.m.orders[o.ID] = o
	return o, true, nil
}

func (t *memTx) OrderByIdempotencyKey(_ context.Context, ws, key string) (Order, bool, error) {
	for _, o := range t.m.orders {
		if o.WorkspaceID == ws && o.IdempotencyKey == key {
			return o, true, nil
		}
	}
	return Order{}, false, nil
}

func (t *memTx) LockOrder(_ context.Context, id string) (Order, bool, error) {
	o, ok := t.m.orders[id]
	return o, ok, nil
}

func (t *memTx) MarkPaid(_ context.Context, id, by string, at time.Time) (Order, error) {
	o := t.m.orders[id]
	o.Status, o.ConfirmedBy, o.PaidAt = StatusPaid, by, &at
	t.m.orders[id] = o
	return o, nil
}

func (t *memTx) DecideNotice(_ context.Context, d NoticeDecision, orderID string) (bool, error) {
	n, ok := t.m.notices[d.Notice.ID]
	if !ok || n.State != d.Notice.State {
		return false, nil
	}
	n.State, n.OrderID = d.To, orderID
	t.m.notices[n.ID] = n
	return true, nil
}

func (t *memTx) LockSubscription(_ context.Context, ws string) (subscription.Subscription, bool, error) {
	s, ok := t.m.subs[ws]
	return s, ok, nil
}

func (t *memTx) InsertSubscription(_ context.Context, s subscription.Subscription) (subscription.Subscription, bool, error) {
	s.Version = 1
	t.m.subs[s.WorkspaceID] = s
	return s, true, nil
}

func (t *memTx) UpdateSubscription(_ context.Context, s subscription.Subscription) (subscription.Subscription, error) {
	s.Version = t.m.subs[s.WorkspaceID].Version + 1
	t.m.subs[s.WorkspaceID] = s
	return s, nil
}

func (t *memTx) ReadOnlyForBilling(_ context.Context, ws string) (bool, error) {
	return t.m.ws[ws].Status == workspacestate.ReadOnly && t.m.wsReason[ws] == ReasonBilling, nil
}

func (t *memTx) Current(_ context.Context, ws string) (workspacestate.Record, bool, error) {
	r, ok := t.m.ws[ws]
	return r, ok, nil
}

func (t *memTx) Write(_ context.Context, c workspacestatus.Change) (bool, error) {
	t.m.ws[c.WorkspaceID] = workspacestate.Record{Status: c.Status}
	t.m.wsReason[c.WorkspaceID] = c.Reason
	return true, nil
}

func (t *memTx) InsertAuditEvent(_ context.Context, e audit.Entry) error {
	if e.Action == t.m.auditFail {
		return errors.New("audit write failed")
	}
	t.m.audits = append(t.m.audits, e)
	return nil
}

type invalidations struct{ ws, ent []string }

func (i *invalidations) Invalidate(_ context.Context, id string) error {
	i.ws = append(i.ws, id)
	return nil
}

type entCache struct{ i *invalidations }

func (c entCache) Invalidate(id string) { c.i.ent = append(c.i.ent, id) }
func (c entCache) InvalidateAll()       {}

var clock = time.Date(2025, 3, 1, 10, 0, 0, 0, time.UTC)

func newTestService(t *testing.T, m *memory) (*Service, *invalidations) {
	t.Helper()
	inv := &invalidations{}
	s, err := New(Deps{
		Runner: &memRunner{m}, Provider: ProviderManual, Workspaces: inv, Entitlements: entCache{inv},
		Now: func() time.Time { return clock },
	})
	if err != nil {
		t.Fatal(err)
	}
	return s, inv
}

func testDraft() Draft {
	start, end := date(2025, 3, 1), date(2025, 4, 1)
	return Draft{
		WorkspaceID: tWS, Kind: KindSubscribe, PlanKey: "pro", Interval: "month", Seats: 3,
		Amount: 447000, VATAmount: 44700, Currency: "vnd", PeriodStart: &start, PeriodEnd: &end,
		IdempotencyKey: "k1", CreatedBy: tUser,
	}
}

func actions(m *memory) []string {
	var out []string
	for _, a := range m.audits {
		out = append(out, a.Action)
	}
	return out
}

func TestNewRequiresItsDependencies(t *testing.T) {
	if _, err := New(Deps{}); err == nil {
		t.Fatal("New accepted empty Deps")
	}
}

func TestCreateOrderSetsExpiryAndAudit(t *testing.T) {
	m := newMemory()
	s, _ := newTestService(t, m)
	o, created, err := s.CreateOrder(context.Background(), testDraft())
	if err != nil || !created {
		t.Fatalf("CreateOrder = %v, %v", created, err)
	}
	if o.Status != StatusPending || o.Provider != ProviderManual || !ValidTransferCode(o.TransferCode) {
		t.Fatalf("order = %+v", o)
	}
	if !o.CreatedAt.Equal(clock) || !o.ExpiresAt.Equal(clock.Add(7*24*time.Hour)) {
		t.Fatalf("created %v, expires %v; want expiry seven days after creation", o.CreatedAt, o.ExpiresAt)
	}
	if string(o.Buyer) != `{}` {
		t.Errorf("buyer = %s, want {}", o.Buyer)
	}
	if got := actions(m); !slices.Equal(got, []string{audit.ActionBillingOrderCreated}) {
		t.Fatalf("audit = %v", got)
	}
	if m.audits[0].ActorType != audit.ActorMember || m.audits[0].ActorID != tUser {
		t.Errorf("audit actor = %s %s", m.audits[0].ActorType, m.audits[0].ActorID)
	}
	if m.audits[0].Detail["plan"] != "pro" {
		t.Errorf("audit detail lost the plan: %v", m.audits[0].Detail)
	}
}

func TestCreateOrderRepeatedKeyReturnsTheFirstOrder(t *testing.T) {
	m := newMemory()
	s, _ := newTestService(t, m)
	first, _, err := s.CreateOrder(context.Background(), testDraft())
	if err != nil {
		t.Fatal(err)
	}
	d := testDraft()
	d.Seats, d.Amount = 9, 1
	again, created, err := s.CreateOrder(context.Background(), d)
	if err != nil || created || again.ID != first.ID || again.Seats != 3 {
		t.Fatalf("again = %+v, created %v, err %v", again, created, err)
	}
	if len(m.orders) != 1 || len(m.audits) != 1 {
		t.Fatalf("orders %d, audits %d; a repeat must create nothing", len(m.orders), len(m.audits))
	}
	// The key is per workspace.
	other := testDraft()
	other.WorkspaceID = "55555555-5555-5555-5555-555555555555"
	if o, created, err := s.CreateOrder(context.Background(), other); err != nil || !created || o.ID == first.ID {
		t.Fatalf("same key in another workspace: %+v, %v, %v", o, created, err)
	}
}

func TestCreateOrderRetriesOneCollision(t *testing.T) {
	m := newMemory()
	*m.collide = 1
	calls := 0
	s, _ := newTestService(t, m)
	s.d.NewCode = func() (string, error) { calls++; return NewTransferCode() }
	if _, created, err := s.CreateOrder(context.Background(), testDraft()); err != nil || !created {
		t.Fatalf("CreateOrder after one collision = %v, %v", created, err)
	}
	if calls != 2 {
		t.Fatalf("codes generated = %d, want 2", calls)
	}

	m = newMemory()
	*m.collide = 2
	s, _ = newTestService(t, m)
	if _, _, err := s.CreateOrder(context.Background(), testDraft()); !errors.Is(err, ErrTransferCodeTaken) {
		t.Fatalf("two collisions: err = %v, want ErrTransferCodeTaken", err)
	}
	if len(m.orders) != 0 || len(m.audits) != 0 {
		t.Fatal("a failed create left rows behind")
	}
}

func TestCreateOrderRefusesBadDrafts(t *testing.T) {
	mutate := map[string]func(*Draft){
		"workspace id":        func(d *Draft) { d.WorkspaceID = "nope" },
		"created by":          func(d *Draft) { d.CreatedBy = "nope" },
		"kind":                func(d *Draft) { d.Kind = "usage" },
		"plan":                func(d *Draft) { d.PlanKey = "" },
		"interval":            func(d *Draft) { d.Interval = "week" },
		"seats":               func(d *Draft) { d.Seats = 0 },
		"amount":              func(d *Draft) { d.Amount = 0 },
		"negative vat":        func(d *Draft) { d.VATAmount = -1 },
		"currency":            func(d *Draft) { d.Currency = "dong" },
		"half a period":       func(d *Draft) { d.PeriodEnd = nil },
		"backwards period":    func(d *Draft) { d.PeriodStart, d.PeriodEnd = d.PeriodEnd, d.PeriodStart },
		"buyer not an object": func(d *Draft) { d.Buyer = []byte(`[1]`) },
		"buyer not json":      func(d *Draft) { d.Buyer = []byte(`{`) },
	}
	for name, f := range mutate {
		m := newMemory()
		s, _ := newTestService(t, m)
		d := testDraft()
		f(&d)
		if _, _, err := s.CreateOrder(context.Background(), d); err == nil {
			t.Errorf("%s: accepted", name)
		}
		if len(m.orders) != 0 {
			t.Errorf("%s: wrote an order", name)
		}
	}
}

func pendingOrder(t *testing.T, m *memory, s *Service, d Draft) Order {
	t.Helper()
	o, _, err := s.CreateOrder(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	m.audits = nil
	return o
}

func TestApplyPaymentAppliesOnce(t *testing.T) {
	m := newMemory()
	s, inv := newTestService(t, m)
	o := pendingOrder(t, m, s, testDraft())
	m.subs[tWS] = subscription.Subscription{WorkspaceID: tWS, PlanKey: "free", Status: "active", Seats: 3, Interval: "month", Version: 4}

	got, err := s.ApplyPayment(context.Background(), Confirmation{OrderID: o.ID, ConfirmedBy: tStaff, ActorLabel: "bill@example.com"})
	if err != nil || got.AlreadyPaid {
		t.Fatalf("ApplyPayment = %+v, %v", got, err)
	}
	if got.Order.Status != StatusPaid || got.Order.ConfirmedBy != tStaff || got.Order.PaidAt == nil || !got.Order.PaidAt.Equal(clock) {
		t.Fatalf("order = %+v", got.Order)
	}
	sub := m.subs[tWS]
	if sub.PlanKey != "pro" || sub.Seats != 3 || sub.Version != 5 {
		t.Fatalf("subscription = %+v; want pro, 3 seats, version 5", sub)
	}
	if got := actions(m); !slices.Equal(got, []string{audit.ActionBillingOrderPaid}) {
		t.Fatalf("audit = %v", got)
	}
	if m.audits[0].ActorType != audit.ActorStaff || m.audits[0].ActorID != tStaff || m.audits[0].TargetID != o.ID {
		t.Errorf("audit entry = %+v", m.audits[0])
	}
	if !slices.Equal(inv.ent, []string{tWS}) || len(inv.ws) != 0 {
		t.Errorf("invalidations: entitlement %v, workspace %v", inv.ent, inv.ws)
	}

	// The second confirmation changes nothing.
	again, err := s.ApplyPayment(context.Background(), Confirmation{OrderID: o.ID, ConfirmedBy: tStaff})
	if err != nil || !again.AlreadyPaid || again.Order.ID != o.ID {
		t.Fatalf("second ApplyPayment = %+v, %v", again, err)
	}
	if m.subs[tWS].Version != 5 || len(m.audits) != 1 || len(inv.ent) != 1 {
		t.Fatalf("second call wrote: version %d, audits %d, invalidations %d", m.subs[tWS].Version, len(m.audits), len(inv.ent))
	}
}

func TestApplyPaymentCreatesTheFirstSubscription(t *testing.T) {
	m := newMemory()
	s, _ := newTestService(t, m)
	o := pendingOrder(t, m, s, testDraft())
	if _, err := s.ApplyPayment(context.Background(), Confirmation{OrderID: o.ID, ConfirmedBy: tStaff}); err != nil {
		t.Fatal(err)
	}
	if sub, ok := m.subs[tWS]; !ok || sub.Version != 1 || sub.PlanKey != "pro" || sub.Status != subscription.StatusActive {
		t.Fatalf("subscription = %+v, %v", sub, ok)
	}
}

func TestApplyPaymentReactivatesAnOverdueWorkspace(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status workspacestate.Status
		reason string
		want   bool
	}{
		{"read_only for billing", workspacestate.ReadOnly, ReasonBilling, true},
		{"read_only for another reason", workspacestate.ReadOnly, "abuse", false},
		{"suspended", workspacestate.Suspended, ReasonBilling, false},
		{"active", workspacestate.Active, "", false},
	} {
		m := newMemory()
		s, inv := newTestService(t, m)
		o := pendingOrder(t, m, s, testDraft())
		m.ws[tWS], m.wsReason[tWS] = workspacestate.Record{Status: tc.status}, tc.reason
		got, err := s.ApplyPayment(context.Background(), Confirmation{OrderID: o.ID, ConfirmedBy: tStaff})
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		wantStatus := tc.status
		if tc.want {
			wantStatus = workspacestate.Active
		}
		if got.WorkspaceReactivated != tc.want || m.ws[tWS].Status != wantStatus {
			t.Errorf("%s: reactivated %v, status %s", tc.name, got.WorkspaceReactivated, m.ws[tWS].Status)
		}
		if tc.want && (!slices.Equal(inv.ws, []string{tWS}) || !slices.Contains(actions(m), audit.ActionWorkspaceStatusChanged)) {
			t.Errorf("%s: cache %v, audit %v", tc.name, inv.ws, actions(m))
		}
		if !tc.want && len(inv.ws) != 0 {
			t.Errorf("%s: workspace cache cleared without a change", tc.name)
		}
	}
}

func TestApplyPaymentRefusals(t *testing.T) {
	m := newMemory()
	s, _ := newTestService(t, m)
	o := pendingOrder(t, m, s, testDraft())

	// An expired order cannot be confirmed.
	s.d.Now = func() time.Time { return o.ExpiresAt.Add(time.Second) }
	if _, err := s.ApplyPayment(context.Background(), Confirmation{OrderID: o.ID, ConfirmedBy: tStaff}); !errors.Is(err, ErrOrderExpired) {
		t.Fatalf("expired: err = %v, want ErrOrderExpired", err)
	}
	if m.orders[o.ID].Status != StatusPending || len(m.subs) != 0 || len(m.audits) != 0 {
		t.Fatal("a refused confirmation wrote something")
	}
	s.d.Now = func() time.Time { return clock }

	for name, c := range map[string]Confirmation{
		"unknown order": {OrderID: "66666666-6666-6666-6666-666666666666", ConfirmedBy: tStaff},
		"bad order id":  {OrderID: "x", ConfirmedBy: tStaff},
		"no confirmer":  {OrderID: o.ID},
		"bad confirmer": {OrderID: o.ID, ConfirmedBy: "x"},
	} {
		if _, err := s.ApplyPayment(context.Background(), c); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := s.ApplyPayment(context.Background(), Confirmation{OrderID: "66666666-6666-6666-6666-666666666666", ConfirmedBy: tStaff}); !errors.Is(err, ErrOrderNotFound) {
		t.Errorf("unknown order: err = %v, want ErrOrderNotFound", err)
	}

	// A canceled order cannot be confirmed.
	c := m.orders[o.ID]
	c.Status = StatusCanceled
	m.orders[o.ID] = c
	if _, err := s.ApplyPayment(context.Background(), Confirmation{OrderID: o.ID, ConfirmedBy: tStaff}); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("canceled: err = %v, want ErrInvalidTransition", err)
	}
}

func TestApplyPaymentRollsBackOnAuditFailure(t *testing.T) {
	m := newMemory()
	s, inv := newTestService(t, m)
	o := pendingOrder(t, m, s, testDraft())
	m.subs[tWS] = subscription.Subscription{WorkspaceID: tWS, PlanKey: "free", Status: "active", Seats: 3, Interval: "month", Version: 4}
	m.ws[tWS], m.wsReason[tWS] = workspacestate.Record{Status: workspacestate.ReadOnly}, ReasonBilling
	m.auditFail = audit.ActionBillingOrderPaid

	if _, err := s.ApplyPayment(context.Background(), Confirmation{OrderID: o.ID, ConfirmedBy: tStaff}); err == nil {
		t.Fatal("ApplyPayment succeeded with a failing audit write")
	}
	if m.orders[o.ID].Status != StatusPending || m.subs[tWS].Version != 4 || m.subs[tWS].PlanKey != "free" ||
		m.ws[tWS].Status != workspacestate.ReadOnly || len(m.audits) != 0 {
		t.Fatalf("rollback incomplete: order %s, sub %+v, workspace %s, audits %v",
			m.orders[o.ID].Status, m.subs[tWS], m.ws[tWS].Status, actions(m))
	}
	if len(inv.ent) != 0 || len(inv.ws) != 0 {
		t.Fatal("caches were cleared for a change that did not commit")
	}
}

func TestApplyPaymentAddSeats(t *testing.T) {
	m := newMemory()
	s, _ := newTestService(t, m)
	d := testDraft()
	d.Kind, d.Seats, d.IdempotencyKey = KindAddSeats, 2, "k2"
	o := pendingOrder(t, m, s, d)
	// No subscription: nothing to add seats to.
	if _, err := s.ApplyPayment(context.Background(), Confirmation{OrderID: o.ID, ConfirmedBy: tStaff}); !errors.Is(err, ErrNoSubscription) {
		t.Fatalf("err = %v, want ErrNoSubscription", err)
	}
	if m.orders[o.ID].Status != StatusPending {
		t.Fatal("the order was marked paid")
	}
	start, end := date(2025, 3, 1), date(2025, 4, 1)
	m.subs[tWS] = subscription.Subscription{WorkspaceID: tWS, PlanKey: "pro", Status: "active", Seats: 5, Interval: "month", CurrentPeriodStart: &start, CurrentPeriodEnd: &end, Version: 2}
	if _, err := s.ApplyPayment(context.Background(), Confirmation{OrderID: o.ID, ConfirmedBy: tStaff}); err != nil {
		t.Fatal(err)
	}
	if sub := m.subs[tWS]; sub.Seats != 7 || sub.Version != 3 {
		t.Fatalf("subscription = %+v; want 7 seats at version 3", sub)
	}
}
