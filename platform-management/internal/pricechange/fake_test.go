package pricechange

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/lifecycle"
)

var t0 = time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

const (
	ws1 = "bbbbbbbb-0000-0000-0000-000000000001"
	ws2 = "bbbbbbbb-0000-0000-0000-000000000002" // pending deletion
	ws3 = "bbbbbbbb-0000-0000-0000-000000000003"
)

func raw(s string) json.RawMessage { return json.RawMessage(s) }

// fakeStore is the plan and plan_price_change tables with transaction
// semantics: InTx serialises callers, as the plan row lock does, and restores
// the state when fn fails.
type fakeStore struct {
	mu       sync.Mutex
	plans    map[string]PlanRow
	changes  []Change
	paid     map[string][]Workspace
	contacts map[string]lifecycle.Contact
	notices  map[string]bool
	audits   []audit.Entry
	seq      int
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		plans: map[string]PlanRow{
			"pro": {Key: "pro", Name: "Pro", BillingModel: "seat", Version: 3, Prices: Prices{
				FieldSeatPriceMonth: raw("100000"), FieldSeatPriceYear: raw("1000000"),
				FieldUsageRates: raw("{}"), FieldUsageTerms: raw("{}"),
			}},
			"free": {Key: "free", Name: "Free", BillingModel: "seat", Prices: Prices{
				FieldSeatPriceMonth: raw("0"), FieldSeatPriceYear: raw("0"),
				FieldUsageRates: raw("{}"), FieldUsageTerms: raw("{}"),
			}},
			"payg": {Key: "payg", Name: "Pay as you go", BillingModel: "usage", Prices: Prices{
				FieldSeatPriceMonth: raw("null"), FieldSeatPriceYear: raw("null"),
				FieldUsageRates: raw(`{"tasks_run":{"per":1000,"price":30000}}`),
				FieldUsageTerms: raw(`{"discount_pct":{"business":20}}`),
			}},
			"empty": {Key: "empty", Name: "No subscribers", BillingModel: "seat", Prices: Prices{
				FieldSeatPriceMonth: raw("50000"), FieldSeatPriceYear: raw("500000"),
				FieldUsageRates: raw("{}"), FieldUsageTerms: raw("{}"),
			}},
		},
		paid: map[string][]Workspace{
			"pro":  {{ID: ws1}, {ID: ws2, PendingDeletion: true}, {ID: ws3}},
			"payg": {{ID: ws1}},
		},
		contacts: map[string]lifecycle.Contact{
			ws1: {Name: "One", Slug: "one", Emails: []string{"owner1@example.com"}},
			ws2: {Name: "Two", Slug: "two", Emails: []string{"owner2@example.com"}},
			ws3: {Name: "Three", Slug: "three", Emails: []string{"owner3@example.com"}},
		},
		notices: map[string]bool{},
	}
}

func (s *fakeStore) InTx(_ context.Context, fn func(Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	plans := map[string]PlanRow{}
	for k, v := range s.plans {
		plans[k] = v
	}
	changes := slices.Clone(s.changes)
	audits := slices.Clone(s.audits)
	if err := fn(&fakeTx{s: s}); err != nil {
		s.plans, s.changes, s.audits = plans, changes, audits
		return err
	}
	return nil
}

func (s *fakeStore) PaidWorkspaces(_ context.Context, key string) ([]Workspace, error) {
	return s.paid[key], nil
}

func (s *fakeStore) History(_ context.Context, key string) ([]Change, error) {
	var out []Change
	for _, c := range slices.Backward(s.changes) {
		if c.PlanKey == key {
			out = append(out, c)
		}
	}
	return out, nil
}

func (s *fakeStore) Due(_ context.Context, now time.Time, limit int) ([]Change, error) {
	var out []Change
	for _, c := range s.changes {
		if c.State == StateScheduled && !c.EffectiveFrom.After(now) && len(out) < limit {
			out = append(out, c)
		}
	}
	return out, nil
}

func (s *fakeStore) NoticeDue(_ context.Context, limit int) ([]Change, error) {
	var out []Change
	for _, c := range s.changes {
		if c.State == StateScheduled && c.NoticeSentAt == nil && len(out) < limit {
			out = append(out, c)
		}
	}
	return out, nil
}

func (s *fakeStore) MarkNoticeSent(_ context.Context, id string, at time.Time) error {
	for i := range s.changes {
		if s.changes[i].ID == id {
			s.changes[i].NoticeSentAt = &at
		}
	}
	return nil
}

func (s *fakeStore) Contact(_ context.Context, ws string) (lifecycle.Contact, error) {
	return s.contacts[ws], nil
}

func (s *fakeStore) InsertNotice(_ context.Context, n lifecycle.Notice) (bool, error) {
	k := n.WorkspaceID + "|" + n.Key
	if s.notices[k] {
		return false, nil
	}
	s.notices[k] = true
	return true, nil
}

func (s *fakeStore) DeleteNotice(_ context.Context, ws, key string) error {
	delete(s.notices, ws+"|"+key)
	return nil
}

type fakeTx struct{ s *fakeStore }

func (t *fakeTx) LockPlan(_ context.Context, key string) (PlanRow, bool, error) {
	p, ok := t.s.plans[key]
	return p, ok, nil
}

func (t *fakeTx) Scheduled(_ context.Context, key string) (Change, bool, error) {
	for _, c := range t.s.changes {
		if c.PlanKey == key && c.State == StateScheduled {
			return c, true, nil
		}
	}
	return Change{}, false, nil
}

func (t *fakeTx) Insert(ctx context.Context, c Change) (Change, error) {
	if _, ok, _ := t.Scheduled(ctx, c.PlanKey); ok {
		return Change{}, ErrPending
	}
	t.s.seq++
	c.ID = fmt.Sprintf("cccccccc-0000-0000-0000-%012d", t.s.seq)
	c.State, c.CreatedAt = StateScheduled, t0
	t.s.changes = append(t.s.changes, c)
	return c, nil
}

func (t *fakeTx) MarkApplied(_ context.Context, id string, now time.Time) (Change, bool, error) {
	for i, c := range t.s.changes {
		if c.ID == id && c.State == StateScheduled && !c.EffectiveFrom.After(now) {
			t.s.changes[i].State, t.s.changes[i].AppliedAt = StateApplied, &now
			return t.s.changes[i], true, nil
		}
	}
	return Change{}, false, nil
}

func (t *fakeTx) SetPrices(_ context.Context, key string, p Prices) error {
	row := t.s.plans[key]
	merged := Prices{}
	for k, v := range row.Prices {
		merged[k] = v
	}
	for k, v := range p {
		merged[k] = v
	}
	row.Prices, row.Version = merged, row.Version+1
	t.s.plans[key] = row
	return nil
}

func (t *fakeTx) Cancel(_ context.Context, key, by string, now time.Time) (Change, bool, error) {
	for i, c := range t.s.changes {
		if c.PlanKey == key && c.State == StateScheduled {
			t.s.changes[i].State, t.s.changes[i].CanceledAt, t.s.changes[i].CanceledBy = StateCanceled, &now, by
			return t.s.changes[i], true, nil
		}
	}
	return Change{}, false, nil
}

func (t *fakeTx) InsertAuditEvent(_ context.Context, e audit.Entry) error {
	t.s.audits = append(t.s.audits, e)
	return nil
}

// fakeMail records the messages and can fail the next sends.
type fakeMail struct {
	mu   sync.Mutex
	sent []lifecycle.Message
	fail error
}

func (m *fakeMail) Send(_ context.Context, msg lifecycle.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail != nil {
		return m.fail
	}
	m.sent = append(m.sent, msg)
	return nil
}

func (m *fakeMail) recipients() []string {
	var out []string
	for _, msg := range m.sent {
		out = append(out, msg.To...)
	}
	slices.Sort(out)
	return out
}

type cacheCount struct{ n int }

func (c *cacheCount) InvalidateAll() { c.n++ }

var admin = Actor{ID: "aaaaaaaa-0000-0000-0000-000000000001", Label: "admin@example.com"}

func newService(t *testing.T, st Store, mail lifecycle.Sender, cache Invalidator) *Service {
	t.Helper()
	s, err := New(Deps{Store: st, Mail: mail, Cache: cache, AppURL: "https://app.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// apiErr returns the *Error inside err, or fails the test.
func apiErr(t *testing.T, err error) *Error {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("error = %v, want a *pricechange.Error", err)
	}
	return e
}

func seatReq(from time.Time, month, year string) Request {
	return Request{
		EffectiveFrom: from, Reason: "annual review",
		New: Prices{FieldSeatPriceMonth: raw(month), FieldSeatPriceYear: raw(year)},
	}
}
