package lifecycle

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
	"github.com/multica-ai/multica/platform-management/workspacestate"
)

const admin = "admin@example.com"

func withPrefix(keys []string, prefix string) []string {
	var out []string
	for _, k := range keys {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	return out
}

func mustEqualTime(t *testing.T, name string, got *time.Time, want time.Time) {
	t.Helper()
	if got == nil || !got.Equal(want) {
		t.Errorf("%s = %v, want %s", name, got, want)
	}
}

// Every edge of the diagram fires at its boundary and not one second before.
func TestLifecycleEdgesFireAtTheBoundary(t *testing.T) {
	boundary := t0.Add(40 * day)
	cases := []struct {
		name       string
		edge       Edge
		sub        func(ws string) subscription.Subscription
		boundary   time.Time
		wantStatus string
		wantWS     *wsRow // nil: the workspace has no platform_workspace row
		check      func(t *testing.T, after subscription.Subscription)
	}{
		{
			name: "trial ends", edge: EdgeTrialEnded, boundary: boundary, wantStatus: StatusFree,
			sub: func(ws string) subscription.Subscription { return trialingSub(ws, boundary) },
			check: func(t *testing.T, after subscription.Subscription) {
				mustEqualTime(t, "trial_end", after.TrialEnd, boundary) // a workspace gets one trial
			},
		},
		{
			name: "cancelled period ends", edge: EdgeCanceledLapsed, boundary: boundary, wantStatus: StatusFree,
			sub: func(ws string) subscription.Subscription {
				s := activeSub(ws, boundary)
				s.CancelAtPeriodEnd = true
				return s
			},
		},
		{
			name: "period ends unpaid", edge: EdgePeriodLapsed, boundary: boundary, wantStatus: StatusPastDue,
			sub: func(ws string) subscription.Subscription { return activeSub(ws, boundary) },
			check: func(t *testing.T, after subscription.Subscription) {
				mustEqualTime(t, "grace_until", after.GraceUntil, boundary.Add(7*day))
				if after.PlanKey != "pro" || after.Seats != 5 || after.CurrentPeriodEnd == nil {
					t.Errorf("past_due changed the plan or the period: %+v", after)
				}
			},
		},
		{
			name: "grace ends", edge: EdgeGraceEnded, boundary: boundary, wantStatus: StatusReadOnly,
			sub:    func(ws string) subscription.Subscription { return pastDueSub(ws, boundary.Add(-GracePeriod)) },
			wantWS: &wsRow{Status: workspacestate.ReadOnly, Reason: billing.ReasonBilling},
			check: func(t *testing.T, after subscription.Subscription) {
				// The read-only period is counted from grace_until, so it stays.
				mustEqualTime(t, "grace_until", after.GraceUntil, boundary)
			},
		},
		{
			name: "read-only period ends", edge: EdgeReadOnlyEnded, boundary: boundary, wantStatus: StatusFree,
			sub:    func(ws string) subscription.Subscription { return readOnlySub(ws, boundary.Add(-ReadOnlyPeriod)) },
			wantWS: &wsRow{Status: workspacestate.Active, Reason: "subscription ended: free plan"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ws := wsID(1)
			setup := func() *env {
				e := newEnv(t)
				e.store.put(c.sub(ws))
				e.store.contact(ws, admin)
				if c.edge == EdgeReadOnlyEnded {
					e.store.ws[ws] = wsRow{Status: workspacestate.ReadOnly, Reason: billing.ReasonBilling}
				}
				return e
			}

			// One second before: nothing moves, and no state email goes out.
			early := setup()
			before := early.store.sub(ws)
			r := early.run(t, c.boundary.Add(-time.Second))
			if r.total() != 0 || !reflect.DeepEqual(early.store.sub(ws), before) {
				t.Fatalf("one second before the boundary: report %+v, subscription %+v", r, early.store.sub(ws))
			}
			if keys := withPrefix(early.store.noticeKeys(ws), "state:"); len(keys) != 0 {
				t.Errorf("one second before the boundary: state notices %v", keys)
			}
			if c.edge == EdgeReadOnlyEnded {
				if row, _ := early.store.wsRow(ws); row.Status != workspacestate.ReadOnly {
					t.Errorf("workspace left read_only early: %+v", row)
				}
			}

			// At the boundary: one move, to the right status, once.
			e := setup()
			before = e.store.sub(ws)
			r = e.run(t, c.boundary)
			if r.Moved[c.edge] != 1 || r.total() != 1 {
				t.Fatalf("at the boundary: report %+v, want exactly %s once", r, c.edge)
			}
			after := e.store.sub(ws)
			if after.Status != c.wantStatus || after.Version != before.Version+1 {
				t.Fatalf("after = status %q version %d, want %q version %d", after.Status, after.Version, c.wantStatus, before.Version+1)
			}
			if c.wantStatus == StatusFree {
				if after.PlanKey != "free" || after.Seats != 0 || after.GraceUntil != nil ||
					after.CancelAtPeriodEnd || after.CurrentPeriodStart != nil || after.CurrentPeriodEnd != nil {
					t.Errorf("free subscription = %+v", after)
				}
			}
			if c.check != nil {
				c.check(t, after)
			}
			row, found := e.store.wsRow(ws)
			switch {
			case c.wantWS == nil && found:
				t.Errorf("platform_workspace row written: %+v", row)
			case c.wantWS != nil && (!found || row.Status != c.wantWS.Status || row.Reason != c.wantWS.Reason):
				t.Errorf("platform_workspace = %+v (found %v), want %+v", row, found, *c.wantWS)
			}
			wantKey := fmt.Sprintf("state:%s:%d", c.wantStatus, after.Version)
			if keys := withPrefix(e.store.noticeKeys(ws), "state:"); !slices.Equal(keys, []string{wantKey}) {
				t.Errorf("state notices = %v, want [%s]", keys, wantKey)
			}
			if !slices.Contains(e.cache.policies, ws) {
				t.Error("the entitlement policy cache was not cleared")
			}
			wantWSCache := c.wantWS != nil
			if got := slices.Contains(e.cache.workspaces, ws); got != wantWSCache {
				t.Errorf("workspace state cache cleared = %v, want %v", got, wantWSCache)
			}
		})
	}
}

// A workspace that staff suspended is not made read_only by the lapse, and a
// read_only workspace that staff set for another reason is not reactivated.
func TestLifecycleLeavesStaffDecisionsOnTheWorkspaceAlone(t *testing.T) {
	t.Run("suspended stays suspended", func(t *testing.T) {
		e := newEnv(t)
		e.store.put(pastDueSub(wsID(1), t0))
		e.store.ws[wsID(1)] = wsRow{Status: workspacestate.Suspended, Reason: "abuse"}
		e.run(t, t0.Add(GracePeriod))
		if got := e.store.sub(wsID(1)).Status; got != StatusReadOnly {
			t.Errorf("subscription status = %q, want read_only", got)
		}
		if row, _ := e.store.wsRow(wsID(1)); row.Status != workspacestate.Suspended || row.Reason != "abuse" {
			t.Errorf("workspace = %+v, want it untouched", row)
		}
	})
	t.Run("manual read_only stays", func(t *testing.T) {
		e := newEnv(t)
		e.store.put(readOnlySub(wsID(1), t0))
		e.store.ws[wsID(1)] = wsRow{Status: workspacestate.ReadOnly, Reason: "manual"}
		e.run(t, t0.Add(ReadOnlyPeriod))
		if got := e.store.sub(wsID(1)).Status; got != StatusFree {
			t.Errorf("subscription status = %q, want free", got)
		}
		if row, _ := e.store.wsRow(wsID(1)); row.Status != workspacestate.ReadOnly || row.Reason != "manual" {
			t.Errorf("workspace = %+v, want it untouched", row)
		}
	})
}

func TestEdgeDueIgnoresRowsWithoutTheirDate(t *testing.T) {
	noEnd := activeSub(wsID(1), t0)
	noEnd.CurrentPeriodEnd = nil
	noTrial := trialingSub(wsID(1), t0)
	noTrial.TrialEnd = nil
	noGrace := pastDueSub(wsID(1), t0)
	noGrace.GraceUntil = nil
	noGraceRO := readOnlySub(wsID(1), t0)
	noGraceRO.GraceUntil = nil
	far := t0.Add(1000 * day)
	for name, c := range map[string]struct {
		e Edge
		s subscription.Subscription
	}{
		"active without period end":    {EdgePeriodLapsed, noEnd},
		"canceled without period end":  {EdgeCanceledLapsed, noEnd},
		"trialing without trial end":   {EdgeTrialEnded, noTrial},
		"past_due without grace":       {EdgeGraceEnded, noGrace},
		"read_only without grace":      {EdgeReadOnlyEnded, noGraceRO},
		"free is never due for a move": {EdgePeriodLapsed, func() subscription.Subscription { s := activeSub(wsID(1), t0); s.Status = StatusFree; return s }()},
	} {
		if c.e.Due(c.s, far) {
			t.Errorf("%s: due", name)
		}
	}
	// An active subscription that stops at the end of the period never goes past_due.
	canceling := activeSub(wsID(1), t0)
	canceling.CancelAtPeriodEnd = true
	if EdgePeriodLapsed.Due(canceling, far) || !EdgeCanceledLapsed.Due(canceling, far) {
		t.Error("a cancel_at_period_end subscription must take the cancel edge only")
	}
}

// Running the job again changes nothing, including after a stop long enough
// to owe several steps at once.
func TestLifecycleSecondRunChangesNothing(t *testing.T) {
	e := newEnv(t)
	ws := wsID(1)
	e.store.put(activeSub(ws, t0))
	e.store.contact(ws, admin)

	now := t0.Add(40 * day) // the job was stopped: three steps are owed
	first := e.run(t, now)
	if first.Moved[EdgePeriodLapsed] != 1 || first.Moved[EdgeGraceEnded] != 1 || first.Moved[EdgeReadOnlyEnded] != 1 || first.total() != 3 {
		t.Fatalf("first run = %+v, want one move on each of the three edges", first)
	}
	after := e.store.sub(ws)
	if after.Status != StatusFree || after.Version != 6 {
		t.Fatalf("after the first run: status %q version %d", after.Status, after.Version)
	}
	mails, notices := e.mail.count(), e.store.noticeCount()
	rows, audits := e.store.wsRows(), len(e.store.audits)

	for i := 0; i < 2; i++ {
		second := e.run(t, now)
		if second.total() != 0 || second.Skipped != 0 || second.Reminders != 0 || second.StateEmails != 0 {
			t.Fatalf("run %d: report %+v, want nothing", i+2, second)
		}
	}
	if !reflect.DeepEqual(e.store.sub(ws), after) || e.mail.count() != mails || e.store.noticeCount() != notices ||
		e.store.wsRows() != rows || len(e.store.audits) != audits {
		t.Error("a later run changed the subscription, the workspace, the audit log or the notices")
	}
}

// barrier holds callers until n have arrived, so that two runs both read their
// rows before either writes.
type barrier struct {
	mu      sync.Mutex
	n       int
	waiting int
	gate    chan struct{}
}

func (b *barrier) wait(t *testing.T) {
	b.mu.Lock()
	if b.gate == nil {
		b.gate = make(chan struct{})
	}
	gate := b.gate
	b.waiting++
	if b.waiting == b.n {
		close(gate)
		b.gate, b.waiting = nil, 0
	}
	b.mu.Unlock()
	select {
	case <-gate:
	case <-time.After(5 * time.Second):
		t.Error("the second run never reached the barrier")
	}
}

// Two runs at the same moment, both having read every due row before either
// writes: each edge is applied once and each notice is sent once.
func TestLifecycleParallelRunsApplyEachEdgeOnce(t *testing.T) {
	e := newEnv(t)
	now := t0.Add(100 * day)
	fixtures := map[Edge]string{
		EdgeTrialEnded: wsID(1), EdgeCanceledLapsed: wsID(2), EdgePeriodLapsed: wsID(3),
		EdgeGraceEnded: wsID(4), EdgeReadOnlyEnded: wsID(5),
	}
	e.store.put(trialingSub(wsID(1), now))
	canceling := activeSub(wsID(2), now)
	canceling.CancelAtPeriodEnd = true
	e.store.put(canceling)
	e.store.put(activeSub(wsID(3), now))
	e.store.put(pastDueSub(wsID(4), now.Add(-GracePeriod)))
	e.store.put(readOnlySub(wsID(5), now.Add(-ReadOnlyPeriod)))
	e.store.ws[wsID(5)] = wsRow{Status: workspacestate.ReadOnly, Reason: billing.ReasonBilling}
	// A reminder that both runs find due.
	e.store.put(activeSub(wsID(6), now.Add(13*day)))
	for n := 1; n <= 6; n++ {
		e.store.contact(wsID(n), admin)
	}
	before := map[string]int64{}
	for _, ws := range fixtures {
		before[ws] = e.store.sub(ws).Version
	}

	barriers := map[string]*barrier{"reminders": {n: 2}}
	for _, edge := range Edges {
		barriers[string(edge)] = &barrier{n: 2}
	}
	e.store.hook = func(point string) { barriers[point].wait(t) }

	var wg sync.WaitGroup
	reports := make([]Report, 2)
	errs := make([]error, 2)
	for i := range reports {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reports[i], errs[i] = e.svc.Run(context.Background(), now)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	}

	moved, skipped, reminders, stateMails := map[Edge]int{}, 0, 0, 0
	for _, r := range reports {
		for edge, n := range r.Moved {
			moved[edge] += n
		}
		skipped += r.Skipped
		reminders += r.Reminders
		stateMails += r.StateEmails
	}
	for _, edge := range Edges {
		if moved[edge] != 1 {
			t.Errorf("edge %s applied %d times across the two runs, want once", edge, moved[edge])
		}
	}
	if skipped != len(Edges) {
		t.Errorf("skipped = %d, want %d (the losing run of each edge)", skipped, len(Edges))
	}
	for _, ws := range fixtures {
		if got := e.store.sub(ws).Version; got != before[ws]+1 {
			t.Errorf("%s: version %d, want %d: the row moved more than once", ws, got, before[ws]+1)
		}
	}
	if reminders != 1 || stateMails != len(Edges) || e.mail.count() != 1+len(Edges) {
		t.Errorf("emails: %d reminders, %d state, %d sent in all; want 1, %d, %d",
			reminders, stateMails, e.mail.count(), len(Edges), 1+len(Edges))
	}
	if got := e.store.noticeCount(); got != 1+len(Edges) {
		t.Errorf("billing_notice rows = %d, want %d", got, 1+len(Edges))
	}
	if got := len(e.store.audits); got != 2 { // read_only, then back to active
		t.Errorf("workspace audit rows = %d, want 2", got)
	}
}

// pay confirms a renewal order for the workspace's subscription at now.
func pay(t *testing.T, e *env, ws string, now time.Time) billing.Applied {
	t.Helper()
	svc, err := billing.New(billing.Deps{
		Runner: e.store.billingRunner(), Provider: billing.ProviderManual,
		Workspaces: e.cache, Entitlements: policyCache{e.cache}, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	id := fmt.Sprintf("11111111-1111-1111-1111-%012d", len(e.store.orders)+1)
	start, end := now, now.Add(30*day)
	e.store.orders[id] = billing.Order{
		ID: id, WorkspaceID: ws, Kind: billing.KindRenew, PlanKey: "pro", Interval: "month", Seats: 5,
		Amount: 745000, Currency: "vnd", Status: billing.StatusPending,
		PeriodStart: &start, PeriodEnd: &end, ExpiresAt: now.Add(7 * day), CreatedAt: now,
	}
	got, err := svc.ApplyPayment(context.Background(), billing.Confirmation{OrderID: id, ConfirmedBy: staffID})
	if err != nil {
		t.Fatalf("ApplyPayment: %v", err)
	}
	return got
}

// AC13: a payment at any step returns the subscription to active, and a
// workspace that is read_only for billing leaves read_only.
func TestLifecyclePaymentAtEachStepReturnsActive(t *testing.T) {
	end := t0.Add(30 * day)
	steps := []struct {
		name  string
		at    time.Time // when the payment arrives
		setup func(e *env)
		want  string // the status just before the payment
	}{
		{"trialing", end.Add(-day), func(e *env) { e.store.put(trialingSub(wsID(1), end)) }, StatusTrialing},
		{"active", end.Add(-day), func(e *env) { e.store.put(activeSub(wsID(1), end)) }, subscription.StatusActive},
		{"past_due", end.Add(day), func(e *env) { e.store.put(activeSub(wsID(1), end)) }, StatusPastDue},
		{"read_only", end.Add(GracePeriod + day), func(e *env) { e.store.put(activeSub(wsID(1), end)) }, StatusReadOnly},
		{"free", end.Add(GracePeriod + ReadOnlyPeriod + day), func(e *env) { e.store.put(activeSub(wsID(1), end)) }, StatusFree},
	}
	for _, c := range steps {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			c.setup(e)
			e.store.contact(wsID(1), admin)
			// The job brings the subscription to the step, one run per hour
			// on the way, as it would in service.
			for at := end.Add(-day); !at.After(c.at); at = at.Add(12 * time.Hour) {
				if c.name == "trialing" || c.name == "active" {
					break
				}
				e.run(t, at)
			}
			if got := e.store.sub(wsID(1)).Status; got != c.want {
				t.Fatalf("status before the payment = %q, want %q", got, c.want)
			}
			row, hadRow := e.store.wsRow(wsID(1))
			wantReactivated := c.want == StatusReadOnly
			if hadRow && (row.Status == workspacestate.ReadOnly) != wantReactivated {
				t.Fatalf("workspace before the payment = %+v", row)
			}

			got := pay(t, e, wsID(1), c.at)

			after := e.store.sub(wsID(1))
			if after.Status != subscription.StatusActive || after.GraceUntil != nil || after.PlanKey != "pro" ||
				after.CurrentPeriodEnd == nil || !after.CurrentPeriodEnd.Equal(c.at.Add(30*day)) {
				t.Errorf("subscription after the payment = %+v", after)
			}
			if got.WorkspaceReactivated != wantReactivated {
				t.Errorf("WorkspaceReactivated = %v, want %v", got.WorkspaceReactivated, wantReactivated)
			}
			if row, found := e.store.wsRow(wsID(1)); found && row.Status != workspacestate.Active {
				t.Errorf("workspace after the payment = %+v, want active", row)
			}
			// The job has nothing to do for a paid subscription.
			if r := e.run(t, c.at); r.total() != 0 {
				t.Errorf("the job moved a paid subscription: %+v", r)
			}
		})
	}
}

// Reminders go out at 14, 7 and 1 days before the period ends, each once.
func TestLifecycleRemindersAtFourteenSevenAndOneDays(t *testing.T) {
	end := t0.Add(60 * day)
	e := newEnv(t)
	ws := wsID(1)
	e.store.put(activeSub(ws, end))
	e.store.contact(ws, admin)

	steps := []struct {
		at   time.Time
		want int // reminders sent by this run
		days int
	}{
		{end.Add(-14*day - time.Second), 0, 0},
		{end.Add(-14 * day), 1, 14},
		{end.Add(-14 * day), 0, 0}, // the same moment again
		{end.Add(-10 * day), 0, 0},
		{end.Add(-7*day - time.Second), 0, 0},
		{end.Add(-7 * day), 1, 7},
		{end.Add(-3 * day), 0, 0},
		{end.Add(-day - time.Second), 0, 0},
		{end.Add(-day), 1, 1},
		{end.Add(-time.Hour), 0, 0},
	}
	sent := 0
	for _, s := range steps {
		r := e.run(t, s.at)
		if r.Reminders != s.want || r.total() != 0 {
			t.Fatalf("run at end%+v: report %+v, want %d reminders", s.at.Sub(end), r, s.want)
		}
		sent += s.want
		if e.mail.count() != sent {
			t.Fatalf("run at end%+v: %d emails in all, want %d", s.at.Sub(end), e.mail.count(), sent)
		}
	}
	wantKeys := []string{
		fmt.Sprintf("remind:%d:1", end.Unix()), fmt.Sprintf("remind:%d:14", end.Unix()), fmt.Sprintf("remind:%d:7", end.Unix()),
	}
	if got := e.store.noticeKeys(ws); !slices.Equal(got, wantKeys) {
		t.Errorf("notice keys = %v, want %v", got, wantKeys)
	}
	for i, m := range e.mail.messages() {
		if !slices.Equal(m.To, []string{admin}) || m.Subject == "" || m.Body == "" {
			t.Errorf("reminder %d = %+v", i, m)
		}
	}
}

// A run that starts inside a window sends that window's reminder only, not the
// larger ones it missed.
func TestLifecycleSendsOnlyTheCurrentReminder(t *testing.T) {
	end := t0.Add(60 * day)
	e := newEnv(t)
	e.store.put(activeSub(wsID(1), end))
	e.store.contact(wsID(1), admin)
	e.run(t, end.Add(-5*day))
	if got, want := e.store.noticeKeys(wsID(1)), []string{fmt.Sprintf("remind:%d:7", end.Unix())}; !slices.Equal(got, want) {
		t.Errorf("notice keys = %v, want %v", got, want)
	}
}

// Only an active subscription that renews gets reminders.
func TestLifecycleRemindsOnlyActiveSubscriptionsThatRenew(t *testing.T) {
	now := t0
	e := newEnv(t)
	canceling := activeSub(wsID(1), now.Add(5*day))
	canceling.CancelAtPeriodEnd = true
	e.store.put(canceling)
	e.store.put(pastDueSub(wsID(2), now.Add(-day)))
	trial := trialingSub(wsID(3), now.Add(5*day))
	e.store.put(trial)
	e.store.put(activeSub(wsID(4), now.Add(200*day)))
	for n := 1; n <= 4; n++ {
		e.store.contact(wsID(n), admin)
	}
	e.run(t, now)
	if e.mail.count() != 0 || e.store.noticeCount() != 0 {
		t.Errorf("sent %d emails and wrote %d notices, want none", e.mail.count(), e.store.noticeCount())
	}
}

// A failed send removes its row, so the next run sends the reminder.
func TestLifecycleFailedReminderIsRetried(t *testing.T) {
	end := t0.Add(60 * day)
	e := newEnv(t)
	ws := wsID(1)
	e.store.put(activeSub(ws, end))
	e.store.contact(ws, admin)
	now := end.Add(-14 * day)

	e.mail.fail = 1
	r, err := e.svc.Run(context.Background(), now)
	if err == nil {
		t.Fatal("Run did not report the failed send")
	}
	if strings.Contains(err.Error(), admin) {
		t.Errorf("the error names the recipient: %v", err)
	}
	if r.Reminders != 0 || e.mail.count() != 0 || e.store.noticeCount() != 0 {
		t.Fatalf("after the failed send: %+v, %d emails, %d notices; want no email and no row", r, e.mail.count(), e.store.noticeCount())
	}
	// The row went in before the send and out after the failure.
	if want := []string{"insert remind:" + fmt.Sprint(end.Unix()) + ":14", "delete remind:" + fmt.Sprint(end.Unix()) + ":14"}; !slices.Equal(e.store.noticeLog, want) {
		t.Errorf("notice log = %v, want %v", e.store.noticeLog, want)
	}

	r = e.run(t, now.Add(time.Hour))
	if r.Reminders != 1 || e.mail.count() != 1 || e.store.noticeCount() != 1 {
		t.Fatalf("retry: %+v, %d emails, %d notices; want the reminder sent once", r, e.mail.count(), e.store.noticeCount())
	}
	e.run(t, now.Add(2*time.Hour))
	if e.mail.count() != 1 {
		t.Errorf("sent again after a success: %d emails", e.mail.count())
	}
}

// The state email goes out once, to the owners and admins, with the new status
// in its key.
func TestLifecycleStateEmailIsSentOncePerTransition(t *testing.T) {
	e := newEnv(t)
	ws := wsID(1)
	e.store.put(activeSub(ws, t0))
	e.store.contact(ws, "owner@example.com", "admin@example.com")

	r := e.run(t, t0)
	if r.StateEmails != 1 || e.mail.count() != 1 {
		t.Fatalf("report %+v, %d emails; want one state email", r, e.mail.count())
	}
	m := e.mail.messages()[0]
	if !slices.Equal(m.To, []string{"owner@example.com", "admin@example.com"}) {
		t.Errorf("recipients = %v", m.To)
	}
	if want := fmt.Sprintf("state:past_due:%d", e.store.sub(ws).Version); !slices.Contains(e.store.noticeKeys(ws), want) {
		t.Errorf("notice keys %v lack %s", e.store.noticeKeys(ws), want)
	}
	for i := 0; i < 3; i++ {
		e.run(t, t0.Add(time.Duration(i+1)*time.Hour))
	}
	if e.mail.count() != 1 {
		t.Errorf("%d emails after repeated runs, want 1", e.mail.count())
	}
}

// A move that another writer got to first sends nothing and writes nothing.
func TestLifecycleSkippedTransitionSendsNothing(t *testing.T) {
	e := newEnv(t)
	ws := wsID(1)
	e.store.put(pastDueSub(ws, t0))
	e.store.ws[ws] = wsRow{Status: workspacestate.Active}
	e.store.contact(ws, admin)
	now := t0.Add(GracePeriod)

	// A payment lands between the job's read and its write.
	e.store.hook = func(point string) {
		if point != string(EdgeGraceEnded) {
			return
		}
		e.store.mu.Lock()
		defer e.store.mu.Unlock()
		s := e.store.subs[ws]
		s.Status, s.GraceUntil, s.Version = subscription.StatusActive, nil, s.Version+1
		s.CurrentPeriodEnd = tp(now.Add(30 * day))
		e.store.subs[ws] = s
	}
	paid := e.store.sub(ws)
	paid.Status, paid.GraceUntil, paid.Version = subscription.StatusActive, nil, paid.Version+1
	paid.CurrentPeriodEnd = tp(now.Add(30 * day))

	r := e.run(t, now)
	if r.Skipped != 1 || r.total() != 0 || r.StateEmails != 0 {
		t.Fatalf("report %+v, want one skipped move and no email", r)
	}
	if got := e.store.sub(ws); !reflect.DeepEqual(got, paid) {
		t.Errorf("the subscription changed: %+v", got)
	}
	if row, _ := e.store.wsRow(ws); row.Status != workspacestate.Active {
		t.Errorf("workspace = %+v, want active", row)
	}
	if e.mail.count() != 0 || len(withPrefix(e.store.noticeKeys(ws), "state:")) != 0 || len(e.store.audits) != 0 {
		t.Errorf("a skipped move left %d emails, notices %v, %d audit rows", e.mail.count(), e.store.noticeKeys(ws), len(e.store.audits))
	}
}

// A failure after the subscription write rolls the subscription back too.
func TestLifecycleWorkspaceWriteFailureRollsTheSubscriptionBack(t *testing.T) {
	e := newEnv(t)
	ws := wsID(1)
	e.store.put(pastDueSub(ws, t0))
	e.store.contact(ws, admin)
	before := e.store.sub(ws)

	e.store.failWrite = true
	_, err := e.svc.Run(context.Background(), t0.Add(GracePeriod))
	if err == nil {
		t.Fatal("Run did not report the failure")
	}
	if got := e.store.sub(ws); !reflect.DeepEqual(got, before) {
		t.Errorf("the subscription stayed changed: %+v", got)
	}
	if _, found := e.store.wsRow(ws); found || e.mail.count() != 0 || e.store.noticeCount() != 0 {
		t.Errorf("the failed step left a workspace row, an email or a notice behind")
	}
	// The next run takes the step.
	if r := e.run(t, t0.Add(GracePeriod+time.Hour)); r.Moved[EdgeGraceEnded] != 1 {
		t.Errorf("the retry = %+v", r)
	}
}

// A state email that fails to send leaves the row out, and the transition in.
func TestLifecycleFailedStateEmailKeepsTheTransition(t *testing.T) {
	e := newEnv(t)
	ws := wsID(1)
	e.store.put(activeSub(ws, t0))
	e.store.contact(ws, admin)
	e.mail.fail = 1
	r, err := e.svc.Run(context.Background(), t0)
	if err == nil {
		t.Fatal("Run did not report the failed send")
	}
	if r.Moved[EdgePeriodLapsed] != 1 || e.store.sub(ws).Status != StatusPastDue {
		t.Errorf("the transition was lost: %+v, status %q", r, e.store.sub(ws).Status)
	}
	if e.store.noticeCount() != 0 {
		t.Errorf("notice rows = %d, want none after the failed send", e.store.noticeCount())
	}
}

// A workspace with nobody to write to sends nothing, logs it, and the run
// goes on to the next workspace.
func TestLifecycleWorkspaceWithoutRecipientsSendsNothing(t *testing.T) {
	var logs bytes.Buffer
	e := newEnv(t)
	svc, err := New(Deps{
		Store: e.store, Mail: e.mail, Workspaces: e.cache, Entitlements: policyCache{e.cache},
		AppURL: "https://app.example.com", Log: slog.New(slog.NewTextHandler(&logs, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	e.store.put(activeSub(wsID(1), t0)) // no contact at all
	e.store.put(activeSub(wsID(2), t0))
	e.store.contact(wsID(2)) // a contact with no address
	e.store.put(activeSub(wsID(3), t0))
	e.store.contact(wsID(3), admin)
	e.store.put(activeSub(wsID(4), t0.Add(14*day))) // a reminder, no contact

	r, err := svc.Run(context.Background(), t0)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if r.Moved[EdgePeriodLapsed] != 3 {
		t.Errorf("moved = %+v: a missing address must not stop the transition", r.Moved)
	}
	if e.mail.count() != 1 || r.StateEmails != 1 {
		t.Errorf("%d emails, report %+v; want only the workspace with an address", e.mail.count(), r)
	}
	if r.NoRecipients != 3 {
		t.Errorf("NoRecipients = %d, want 3", r.NoRecipients)
	}
	for _, ws := range []string{wsID(1), wsID(2), wsID(4)} {
		if keys := e.store.noticeKeys(ws); len(keys) != 0 {
			t.Errorf("%s: notice rows %v for an email that was not sent", ws, keys)
		}
	}
	if !strings.Contains(logs.String(), "no owner or admin with an email") {
		t.Errorf("the log does not say so:\n%s", logs.String())
	}
}

// cancel_at_period_end goes straight to free, with no grace period.
func TestLifecycleCancelAtPeriodEndGoesStraightToFree(t *testing.T) {
	e := newEnv(t)
	ws := wsID(1)
	s := activeSub(ws, t0)
	s.CancelAtPeriodEnd = true
	e.store.put(s)
	e.store.contact(ws, admin)

	r := e.run(t, t0)
	if r.Moved[EdgeCanceledLapsed] != 1 || r.total() != 1 {
		t.Fatalf("report %+v", r)
	}
	after := e.store.sub(ws)
	if after.Status != StatusFree || after.GraceUntil != nil || after.Version != s.Version+1 {
		t.Errorf("subscription = %+v", after)
	}
	if _, found := e.store.wsRow(ws); found {
		t.Error("the workspace was touched")
	}
	if want := []string{fmt.Sprintf("state:free:%d", after.Version)}; !slices.Equal(e.store.noticeKeys(ws), want) {
		t.Errorf("notices = %v, want %v", e.store.noticeKeys(ws), want)
	}
	// Later runs, however long after, leave it alone.
	if r := e.run(t, t0.Add(100*day)); r.total() != 0 {
		t.Errorf("a free subscription moved: %+v", r)
	}
}

// The chain from a paid period to free changes only the rows the transitions
// write: no subscription, workspace or notice row is lost, and a bystander is
// untouched. The test asserts each count, not a total.
func TestLifecycleChainKeepsTheFixtureRowCounts(t *testing.T) {
	e := newEnv(t)
	a, bystander := wsID(1), wsID(2)
	end := t0.Add(30 * day)
	e.store.put(activeSub(a, end))
	e.store.put(activeSub(bystander, end.Add(2000*day)))
	e.store.contact(a, admin)
	e.store.contact(bystander, admin)
	other := e.store.sub(bystander)

	steps := []struct {
		name                 string
		at                   time.Time
		subs, wsRows, notice int
		status               string
	}{
		{"before any notice", end.Add(-15 * day), 2, 0, 0, subscription.StatusActive},
		{"14-day reminder", end.Add(-14 * day), 2, 0, 1, subscription.StatusActive},
		{"past_due", end, 2, 0, 2, StatusPastDue},
		{"read_only", end.Add(GracePeriod), 2, 1, 3, StatusReadOnly},
		{"still read_only", end.Add(GracePeriod + ReadOnlyPeriod - time.Second), 2, 1, 3, StatusReadOnly},
		{"free", end.Add(GracePeriod + ReadOnlyPeriod), 2, 1, 4, StatusFree},
		{"much later", end.Add(900 * day), 2, 1, 4, StatusFree},
	}
	for _, s := range steps {
		e.run(t, s.at)
		if got := e.store.subCount(); got != s.subs {
			t.Errorf("%s: workspace_subscription rows = %d, want %d", s.name, got, s.subs)
		}
		if got := e.store.wsRows(); got != s.wsRows {
			t.Errorf("%s: platform_workspace rows = %d, want %d", s.name, got, s.wsRows)
		}
		if got := e.store.noticeCount(); got != s.notice {
			t.Errorf("%s: billing_notice rows = %d, want %d", s.name, got, s.notice)
		}
		if got := e.store.sub(a).Status; got != s.status {
			t.Errorf("%s: status = %q, want %q", s.name, got, s.status)
		}
	}
	if !reflect.DeepEqual(e.store.sub(bystander), other) || len(e.store.noticeKeys(bystander)) != 0 {
		t.Error("the bystander workspace changed")
	}
	if row, _ := e.store.wsRow(a); row.Status != workspacestate.Active {
		t.Errorf("workspace after the free step = %+v, want active", row)
	}
	if got := len(e.store.audits); got != 2 {
		t.Errorf("audit rows = %d, want 2 (read_only, then active)", got)
	}
}

func TestNewRequiresItsDependencies(t *testing.T) {
	e := newEnv(t)
	full := Deps{Store: e.store, Mail: e.mail, Workspaces: e.cache, Entitlements: policyCache{e.cache}, AppURL: "https://app.example.com"}
	if _, err := New(full); err != nil {
		t.Fatalf("New(full) = %v", err)
	}
	for name, mutate := range map[string]func(*Deps){
		"store": func(d *Deps) { d.Store = nil }, "mail": func(d *Deps) { d.Mail = nil },
		"workspaces": func(d *Deps) { d.Workspaces = nil }, "entitlements": func(d *Deps) { d.Entitlements = nil },
		"app url": func(d *Deps) { d.AppURL = "" },
	} {
		d := full
		mutate(&d)
		if _, err := New(d); err == nil {
			t.Errorf("New without %s succeeded", name)
		}
	}
}

func TestRunStopsWhenTheContextEnds(t *testing.T) {
	e := newEnv(t)
	e.store.put(activeSub(wsID(1), t0))
	e.store.contact(wsID(1), admin)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.svc.Run(ctx, t0); err == nil {
		t.Error("Run ignored a cancelled context")
	}
	if e.store.sub(wsID(1)).Status != subscription.StatusActive {
		t.Error("a cancelled run moved a subscription")
	}
}
