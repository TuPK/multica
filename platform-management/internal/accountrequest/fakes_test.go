package accountrequest

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/plan"
)

var t0 = time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)

const (
	adminA = "00000000-0000-4000-8000-0000000000a1"
	adminB = "00000000-0000-4000-8000-0000000000b2"
)

// fakeStore is an in-memory Runner. A transaction that returns an error leaves
// no row and no audit entry, as a rolled-back one does.
type fakeStore struct {
	mu     sync.Mutex
	rows   map[string]Request
	users  map[string]string // lower-case email -> user id
	audits []audit.Entry
	seq    int
	txs    int

	auditErr error
	// racer, when set, is a live request for the same email that another
	// transaction commits between this one's lookup and its insert.
	racer *Request
}

func newFakeStore() *fakeStore {
	return &fakeStore{rows: map[string]Request{}, users: map[string]string{}}
}

func (s *fakeStore) InTx(_ context.Context, fn func(Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.txs++
	rows := make(map[string]Request, len(s.rows))
	for k, v := range s.rows {
		rows[k] = v
	}
	audits, seq := slices.Clone(s.audits), s.seq
	if err := fn(&fakeTx{s}); err != nil {
		s.rows, s.audits, s.seq = rows, audits, seq
		return err
	}
	return nil
}

func (s *fakeStore) live(email string) (Request, bool) {
	for _, r := range s.rows {
		if r.Email == email && (r.State == StatePending || r.State == StateProvisioned) {
			return r, true
		}
	}
	return Request{}, false
}

type fakeTx struct{ s *fakeStore }

func (t *fakeTx) UserIDByEmail(_ context.Context, email string) (string, bool, error) {
	id, ok := t.s.users[email]
	return id, ok, nil
}

func (t *fakeTx) LiveByEmail(_ context.Context, email string) (Request, bool, error) {
	r, ok := t.s.live(email)
	return r, ok, nil
}

func (t *fakeTx) Insert(_ context.Context, in Input, createdBy string) (Request, bool, error) {
	if t.s.racer != nil {
		t.s.rows[t.s.racer.ID] = *t.s.racer
		t.s.racer = nil
	}
	if _, ok := t.s.live(in.Email); ok {
		return Request{}, false, nil
	}
	t.s.seq++
	r := Request{
		ID: fmt.Sprintf("10000000-0000-4000-8000-%012d", t.s.seq), Kind: in.Kind, Email: in.Email,
		DisplayName: in.DisplayName, WorkspaceName: in.WorkspaceName, WorkspaceSlug: in.WorkspaceSlug,
		PlanKey: in.PlanKey, Seats: in.Seats, Interval: in.Interval, Trial: in.Trial, CompReason: in.CompReason,
		BillingProfile: in.BillingProfile, State: StatePending, CreatedBy: createdBy, CreatedAt: t0, UpdatedAt: t0,
	}
	t.s.rows[r.ID] = r
	return r, true, nil
}

func (t *fakeTx) Lock(_ context.Context, id string) (Request, bool, error) {
	r, ok := t.s.rows[id]
	return r, ok, nil
}

func (t *fakeTx) Cancel(_ context.Context, id string) (Request, error) {
	r, ok := t.s.rows[id]
	if !ok || r.State != StatePending {
		return Request{}, errors.New("fake: cancel of a request that is not pending")
	}
	r.State = StateCanceled
	t.s.rows[id] = r
	return r, nil
}

func (t *fakeTx) InsertAuditEvent(_ context.Context, e audit.Entry) error {
	if t.s.auditErr != nil {
		return t.s.auditErr
	}
	t.s.audits = append(t.s.audits, e)
	return nil
}

func (s *fakeStore) List(_ context.Context, q ListQuery) ([]Request, error) {
	var out []Request
	for _, r := range s.rows {
		if q.State == "" || r.State == q.State {
			out = append(out, r)
		}
	}
	slices.SortFunc(out, func(a, b Request) int { return -compareStrings(a.ID, b.ID) })
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out, nil
}

func (s *fakeStore) Get(_ context.Context, id string) (Request, bool, error) {
	r, ok := s.rows[id]
	return r, ok, nil
}

func compareStrings(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// fakeLimiter stands in for Redis: a counter per key.
type fakeLimiter struct {
	mu     sync.Mutex
	counts map[string]int
	calls  []limiterCall
	err    error
}

type limiterCall struct {
	key    string
	limit  int
	window time.Duration
}

func (l *fakeLimiter) Allow(_ context.Context, key string, limit int, window time.Duration) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, limiterCall{key, limit, window})
	if l.err != nil {
		return false, l.err
	}
	if l.counts == nil {
		l.counts = map[string]int{}
	}
	l.counts[key]++
	return l.counts[key] <= limit, nil
}

type fakePlans map[string]plan.Plan

func (p fakePlans) Get(_ context.Context, key string) (plan.Plan, bool, error) {
	v, ok := p[key]
	return v, ok, nil
}

func (p fakePlans) List(context.Context) ([]plan.Plan, error) { return nil, nil }

type env struct {
	store   *fakeStore
	limiter *fakeLimiter
	svc     *Service
}

func newEnv(perHour int) *env {
	e := &env{store: newFakeStore(), limiter: &fakeLimiter{}}
	e.svc = New(Deps{
		Runner: e.store, Reader: e.store, Plans: fakePlans(plans()), Limiter: e.limiter,
		CreatePerHour: perHour, IndividualMaxSeats: 5, Now: func() time.Time { return t0 },
	})
	return e
}

func (e *env) auditActions() []string {
	var out []string
	for _, a := range e.store.audits {
		out = append(out, a.Action)
	}
	return out
}

func (e *env) count() int { return len(e.store.rows) }

func admin(id string) Actor { return Actor{ID: id, Label: id + "@staff.example"} }

// fakeWelcomeMailer records each welcome it is asked to send. err is returned
// for every call; delay holds the call open, to let a parallel run overlap.
type fakeWelcomeMailer struct {
	mu    sync.Mutex
	calls []welcomeCall
	err   error
	delay time.Duration
}

type welcomeCall struct{ to, workspace, plan, appURL string }

func (m *fakeWelcomeMailer) SendWelcome(_ context.Context, to, workspaceName, planName, appURL string) error {
	m.mu.Lock()
	m.calls = append(m.calls, welcomeCall{to, workspaceName, planName, appURL})
	err, delay := m.err, m.delay
	m.mu.Unlock()
	time.Sleep(delay)
	return err
}

func (m *fakeWelcomeMailer) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

func (m *fakeWelcomeMailer) setErr(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
}

// fakeInvalidator records the workspaces whose cached policy was dropped.
type fakeInvalidator struct {
	mu  sync.Mutex
	ids []string
}

func (c *fakeInvalidator) Invalidate(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ids = append(c.ids, id)
}

func (c *fakeInvalidator) has(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Contains(c.ids, id)
}
