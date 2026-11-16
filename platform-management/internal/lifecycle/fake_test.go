package lifecycle

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
	"github.com/multica-ai/multica/platform-management/internal/workspacestatus"
	"github.com/multica-ai/multica/platform-management/workspacestate"
)

// The tests run on a fake clock: every time is an offset from t0.
var t0 = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

const day = 24 * time.Hour

// wsID returns a workspace id for fixture n.
func wsID(n int) string { return fmt.Sprintf("00000000-0000-0000-0000-%012d", n) }

const staffID = "99999999-9999-9999-9999-999999999999"

type wsRow struct {
	Status workspacestate.Status
	Reason string
}

// fakeStore is an in-memory stand-in for the tables the job touches. A
// transaction holds the lock for its whole length and restores a snapshot when
// it fails, as a rollback does, so two runs cannot interleave inside one.
type fakeStore struct {
	mu       sync.Mutex
	subs     map[string]subscription.Subscription
	ws       map[string]wsRow
	audits   []audit.Entry
	notices  map[string]Notice
	contacts map[string]Contact
	orders   map[string]billing.Order

	// hook runs after Due or Reminders has read its rows and before it
	// returns them, outside the lock. The point is the edge, or "reminders".
	hook func(point string)
	// failWrite makes the next workspace write fail.
	failWrite bool
	// failAudit makes every audit insert fail.
	failAudit bool
	// noticeOrder records the order of notice inserts and deletes.
	noticeLog []string
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		subs: map[string]subscription.Subscription{}, ws: map[string]wsRow{},
		notices: map[string]Notice{}, contacts: map[string]Contact{}, orders: map[string]billing.Order{},
	}
}

func (f *fakeStore) put(s subscription.Subscription) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.subs[s.WorkspaceID] = s
}

func (f *fakeStore) sub(id string) subscription.Subscription {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.subs[id]
}

// contact gives the workspace one admin address and a name.
func (f *fakeStore) contact(id string, emails ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.contacts[id] = Contact{Name: "Workspace " + id[len(id)-3:], Slug: "ws-" + id[len(id)-3:], Emails: emails}
}

func (f *fakeStore) noticeKeys(id string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, n := range f.notices {
		if n.WorkspaceID == id {
			out = append(out, n.Key)
		}
	}
	sort.Strings(out)
	return out
}

func (f *fakeStore) noticeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.notices)
}

func (f *fakeStore) wsRows() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.ws)
}

func (f *fakeStore) subCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.subs)
}

func (f *fakeStore) wsRow(id string) (wsRow, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.ws[id]
	return r, ok
}

func (f *fakeStore) Due(_ context.Context, e Edge, now time.Time, limit int) ([]subscription.Subscription, error) {
	f.mu.Lock()
	var out []subscription.Subscription
	for _, s := range f.subs {
		if e.Due(s, now) {
			out = append(out, s)
		}
	}
	f.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].WorkspaceID < out[j].WorkspaceID })
	if len(out) > limit {
		out = out[:limit]
	}
	if f.hook != nil {
		f.hook(string(e))
	}
	return out, nil
}

func (f *fakeStore) Reminders(_ context.Context, now, until time.Time, limit int) ([]subscription.Subscription, error) {
	f.mu.Lock()
	var out []subscription.Subscription
	for _, s := range f.subs {
		if s.Status == subscription.StatusActive && !s.CancelAtPeriodEnd && s.CurrentPeriodEnd != nil &&
			s.CurrentPeriodEnd.After(now) && !s.CurrentPeriodEnd.After(until) {
			out = append(out, s)
		}
	}
	f.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].WorkspaceID < out[j].WorkspaceID })
	if len(out) > limit {
		out = out[:limit]
	}
	if f.hook != nil {
		f.hook("reminders")
	}
	return out, nil
}

type snapshot struct {
	subs   map[string]subscription.Subscription
	ws     map[string]wsRow
	audits []audit.Entry
	orders map[string]billing.Order
}

func (f *fakeStore) snapshot() snapshot {
	return snapshot{maps.Clone(f.subs), maps.Clone(f.ws), slices.Clone(f.audits), maps.Clone(f.orders)}
}

func (f *fakeStore) restore(s snapshot) {
	f.subs, f.ws, f.audits, f.orders = s.subs, s.ws, s.audits, s.orders
}

func (f *fakeStore) InTx(_ context.Context, fn func(Tx) error) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	saved := f.snapshot()
	if err := fn(&fakeTx{txCore{f}}); err != nil {
		f.restore(saved)
		return err
	}
	return nil
}

func (f *fakeStore) Contact(_ context.Context, id string) (Contact, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.contacts[id], nil
}

func (f *fakeStore) InsertNotice(_ context.Context, n Notice) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := n.WorkspaceID + "|" + n.Key
	if _, dup := f.notices[k]; dup {
		return false, nil
	}
	f.notices[k] = n
	f.noticeLog = append(f.noticeLog, "insert "+n.Key)
	return true, nil
}

func (f *fakeStore) DeleteNotice(_ context.Context, id, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.notices, id+"|"+key)
	f.noticeLog = append(f.noticeLog, "delete "+key)
	return nil
}

// txCore is the part of a transaction the lifecycle and the billing fakes
// share: the workspace state writes of workspacestatus.Tx.
type txCore struct{ f *fakeStore }

func (t txCore) Current(_ context.Context, id string) (workspacestate.Record, bool, error) {
	r, ok := t.f.ws[id]
	return workspacestate.Record{Status: r.Status}, ok, nil
}

func (t txCore) Write(_ context.Context, c workspacestatus.Change) (bool, error) {
	if t.f.failWrite {
		t.f.failWrite = false
		return false, fmt.Errorf("disk full")
	}
	if cur, ok := t.f.ws[c.WorkspaceID]; ok && cur.Status == c.Status {
		return false, nil
	}
	t.f.ws[c.WorkspaceID] = wsRow{Status: c.Status, Reason: c.Reason}
	return true, nil
}

func (t txCore) InsertAuditEvent(_ context.Context, e audit.Entry) error {
	if t.f.failAudit {
		return fmt.Errorf("audit insert failed")
	}
	t.f.audits = append(t.f.audits, e)
	return nil
}

type fakeTx struct{ txCore }

func (t *fakeTx) Move(_ context.Context, from, to subscription.Subscription) (subscription.Subscription, bool, error) {
	cur, ok := t.f.subs[from.WorkspaceID]
	if !ok || cur.Status != from.Status || cur.Version != from.Version {
		return subscription.Subscription{}, false, nil
	}
	cur.PlanKey, cur.Status, cur.Seats = to.PlanKey, to.Status, to.Seats
	cur.CurrentPeriodStart, cur.CurrentPeriodEnd, cur.GraceUntil = to.CurrentPeriodStart, to.CurrentPeriodEnd, to.GraceUntil
	cur.CancelAtPeriodEnd = to.CancelAtPeriodEnd
	cur.Version++
	t.f.subs[from.WorkspaceID] = cur
	return cur, true, nil
}

func (t *fakeTx) WorkspaceState(_ context.Context, id string) (workspacestate.Status, string, bool, error) {
	r, ok := t.f.ws[id]
	return r.Status, r.Reason, ok, nil
}

// ExpireOrder is the conditional update of ExpireBillingOrder: it applies to a
// pending order whose expires_at is at or before now.
func (t *fakeTx) ExpireOrder(_ context.Context, id string, now time.Time) (billing.Order, bool, error) {
	o, ok := t.f.orders[id]
	if !ok || o.Status != billing.StatusPending || o.ExpiresAt.After(now) {
		return billing.Order{}, false, nil
	}
	o.Status = billing.StatusExpired
	t.f.orders[id] = o
	return o, true, nil
}

func (f *fakeStore) DueOrders(_ context.Context, now time.Time, limit int) ([]billing.Order, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []billing.Order
	for _, o := range f.orders {
		if o.Status == billing.StatusPending && !o.ExpiresAt.After(now) {
			out = append(out, o)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// billingRunner is the billing.Runner over the same store, so a payment and
// the job act on the same rows.
func (f *fakeStore) billingRunner() billing.Runner { return billingRunner{f} }

type billingRunner struct{ f *fakeStore }

func (r billingRunner) InTx(_ context.Context, fn func(billing.Tx) error) error {
	r.f.mu.Lock()
	defer r.f.mu.Unlock()
	saved := r.f.snapshot()
	if err := fn(&billingTx{txCore{r.f}}); err != nil {
		r.f.restore(saved)
		return err
	}
	return nil
}

type billingTx struct{ txCore }

func (t *billingTx) InsertOrder(context.Context, billing.NewOrder) (billing.Order, bool, error) {
	return billing.Order{}, false, fmt.Errorf("not used")
}

func (t *billingTx) OrderByIdempotencyKey(context.Context, string, string) (billing.Order, bool, error) {
	return billing.Order{}, false, fmt.Errorf("not used")
}

func (t *billingTx) LockOrder(_ context.Context, id string) (billing.Order, bool, error) {
	o, ok := t.f.orders[id]
	return o, ok, nil
}

func (t *billingTx) MarkPaid(_ context.Context, id, by string, at time.Time) (billing.Order, error) {
	o := t.f.orders[id]
	o.Status, o.ConfirmedBy, o.PaidAt = billing.StatusPaid, by, &at
	t.f.orders[id] = o
	return o, nil
}

func (t *billingTx) LockSubscription(_ context.Context, id string) (subscription.Subscription, bool, error) {
	s, ok := t.f.subs[id]
	return s, ok, nil
}

func (t *billingTx) InsertSubscription(_ context.Context, s subscription.Subscription) (subscription.Subscription, bool, error) {
	if _, ok := t.f.subs[s.WorkspaceID]; ok {
		return subscription.Subscription{}, false, nil
	}
	s.Version = 1
	t.f.subs[s.WorkspaceID] = s
	return s, true, nil
}

func (t *billingTx) UpdateSubscription(_ context.Context, s subscription.Subscription) (subscription.Subscription, error) {
	s.Version = t.f.subs[s.WorkspaceID].Version + 1
	t.f.subs[s.WorkspaceID] = s
	return s, nil
}

func (t *billingTx) ReadOnlyForBilling(_ context.Context, id string) (bool, error) {
	r, ok := t.f.ws[id]
	return ok && r.Status == workspacestate.ReadOnly && r.Reason == billing.ReasonBilling, nil
}

// fakeMail records what is sent. fail makes the next n sends fail.
type fakeMail struct {
	mu   sync.Mutex
	sent []Message
	fail int
}

func (m *fakeMail) Send(_ context.Context, msg Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail > 0 {
		m.fail--
		return fmt.Errorf("smtp: connection refused")
	}
	m.sent = append(m.sent, msg)
	return nil
}

func (m *fakeMail) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sent)
}

func (m *fakeMail) messages() []Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.sent)
}

// cacheLog records invalidations for both caches.
type cacheLog struct {
	mu         sync.Mutex
	workspaces []string
	policies   []string
}

func (c *cacheLog) Invalidate(_ context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.workspaces = append(c.workspaces, id)
	return nil
}

type policyCache struct{ c *cacheLog }

func (p policyCache) Invalidate(id string) {
	p.c.mu.Lock()
	defer p.c.mu.Unlock()
	p.c.policies = append(p.c.policies, id)
}

func (p policyCache) InvalidateAll() {}

// env is a Service over the fakes.
type env struct {
	store *fakeStore
	mail  *fakeMail
	cache *cacheLog
	svc   *Service
}

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{store: newFakeStore(), mail: &fakeMail{}, cache: &cacheLog{}}
	svc, err := New(Deps{
		Store: e.store, Mail: e.mail, Workspaces: e.cache, Entitlements: policyCache{e.cache},
		AppURL: "https://app.example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	e.svc = svc
	return e
}

func (e *env) run(t *testing.T, now time.Time) Report {
	t.Helper()
	r, err := e.svc.Run(context.Background(), now)
	if err != nil {
		t.Fatalf("Run(%s): %v", now.Format(time.RFC3339), err)
	}
	return r
}

func tp(t time.Time) *time.Time { return &t }

// activeSub is a paid subscription whose period ends at end.
func activeSub(ws string, end time.Time) subscription.Subscription {
	return subscription.Subscription{
		WorkspaceID: ws, PlanKey: "pro", Status: subscription.StatusActive, Seats: 5, Interval: "month",
		CurrentPeriodStart: tp(end.Add(-30 * day)), CurrentPeriodEnd: tp(end), LimitOverrides: plan.Limits{}, Version: 3,
	}
}

func trialingSub(ws string, trialEnd time.Time) subscription.Subscription {
	s := activeSub(ws, trialEnd)
	s.Status, s.TrialEnd = StatusTrialing, tp(trialEnd)
	s.CurrentPeriodStart, s.CurrentPeriodEnd = nil, nil
	return s
}

func pastDueSub(ws string, periodEnd time.Time) subscription.Subscription {
	s := activeSub(ws, periodEnd)
	s.Status, s.GraceUntil = StatusPastDue, tp(periodEnd.Add(GracePeriod))
	return s
}

func readOnlySub(ws string, graceUntil time.Time) subscription.Subscription {
	s := activeSub(ws, graceUntil.Add(-GracePeriod))
	s.Status, s.GraceUntil = StatusReadOnly, tp(graceUntil)
	return s
}
