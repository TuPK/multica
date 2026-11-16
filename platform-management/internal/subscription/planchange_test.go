package subscription

import (
	"context"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/plan"
)

// pcPending is a workspace with (orderID != "") or without a pending order.
type pcPending struct{ orderID string }

func (f pcPending) PendingOrder(context.Context, string) (string, bool, error) {
	return f.orderID, f.orderID != "", nil
}

var pcActor = Actor{ID: staffID, Label: "member@example.com"}

// pcSub is a paid pro workspace of 5 seats, paying 100000 a seat for the period
// 1 May to 1 June 2030. t0 is 6 May, inside it.
func pcSub(version int64) Subscription {
	start, end := time.Date(2030, 5, 1, 0, 0, 0, 0, time.UTC), time.Date(2030, 6, 1, 0, 0, 0, 0, time.UTC)
	return Subscription{
		WorkspaceID: wsID, PlanKey: "pro", Status: StatusActive, Seats: 5, Interval: IntervalMonth,
		CurrentPeriodStart: &start, CurrentPeriodEnd: &end, UnitPrice: i64(100000), Version: version,
		LimitOverrides: plan.Limits{},
	}
}

func pcService(t *testing.T, cur *Subscription, orders PendingOrders) (*PlanChanges, *fakeStore, *recordingCache) {
	t.Helper()
	store, cache := newFakeStore(t), &recordingCache{}
	if cur != nil {
		store.subs[wsID] = *cur
	}
	return NewPlanChanges(store, cache, orders, func() time.Time { return t0 }), store, cache
}

func TestScheduleStoresOnlyThePendingChange(t *testing.T) {
	cur := pcSub(4)
	svc, store, _ := pcService(t, &cur, pcPending{})
	res, err := svc.Schedule(context.Background(), ScheduleChange{
		WorkspaceID: wsID, PlanKey: "starter", Interval: IntervalYear, Seats: 3,
	}, pcActor)
	if err != nil {
		t.Fatal(err)
	}
	got := store.subs[wsID]
	if got.PlanKey != "pro" || got.Seats != 5 || got.Interval != IntervalMonth || got.Status != StatusActive {
		t.Fatalf("the current terms moved: %+v", got)
	}
	if got.PendingChange == nil || got.PendingChange.PlanKey != "starter" || got.PendingChange.Seats != 3 ||
		got.PendingChange.Interval != IntervalYear || got.PendingChange.RequestedBy != staffID {
		t.Fatalf("pending change = %+v", got.PendingChange)
	}
	if !res.Changed || got.Version != 5 {
		t.Fatalf("result = %+v, version %d; want changed at version 5", res, got.Version)
	}
	if len(store.audits) != 1 || store.audits[0].Action != audit.ActionSubscriptionChangeScheduled {
		t.Fatalf("audit = %+v, want one %s row", store.audits, audit.ActionSubscriptionChangeScheduled)
	}
}

func TestScheduleTheSameChangeAgainWritesNothing(t *testing.T) {
	cur := pcSub(4)
	svc, store, _ := pcService(t, &cur, pcPending{})
	req := ScheduleChange{WorkspaceID: wsID, PlanKey: "starter", Interval: IntervalYear, Seats: 3}
	if _, err := svc.Schedule(context.Background(), req, pcActor); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Schedule(context.Background(), req, pcActor)
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed || store.subs[wsID].Version != 5 || len(store.audits) != 1 {
		t.Fatalf("second call changed %+v, version %d, audit rows %d", res.Changed, store.subs[wsID].Version, len(store.audits))
	}
}

func TestScheduleRefusesAPendingOrder(t *testing.T) {
	cur := pcSub(4)
	svc, store, _ := pcService(t, &cur, pcPending{orderID: "dddddddd-0000-0000-0000-000000000009"})
	_, err := svc.Schedule(context.Background(), ScheduleChange{WorkspaceID: wsID, PlanKey: "starter", Interval: IntervalMonth, Seats: 5}, pcActor)
	apiErr(t, err, 409, "order_pending")
	if e := err.(*Error); e.Detail["order_id"] != "dddddddd-0000-0000-0000-000000000009" {
		t.Fatalf("detail = %v, want the pending order's id", e.Detail)
	}
	if store.subs[wsID].PendingChange != nil || len(store.audits) != 0 {
		t.Fatalf("a refused change wrote: %+v, audit %d", store.subs[wsID].PendingChange, len(store.audits))
	}
}

func TestScheduleRefusesAStaleVersion(t *testing.T) {
	cur := pcSub(4)
	svc, store, _ := pcService(t, &cur, pcPending{})
	_, err := svc.Schedule(context.Background(), ScheduleChange{
		WorkspaceID: wsID, PlanKey: "starter", Interval: IntervalMonth, Seats: 5, ExpectedVersion: 3,
	}, pcActor)
	apiErr(t, err, 409, "quote_changed")
	if store.subs[wsID].PendingChange != nil || len(store.audits) != 0 {
		t.Fatal("a stale quote wrote")
	}
}

func TestScheduleNeedsAPaidRow(t *testing.T) {
	svc, store, _ := pcService(t, nil, pcPending{})
	_, err := svc.Schedule(context.Background(), ScheduleChange{WorkspaceID: wsID, PlanKey: "starter", Interval: IntervalMonth, Seats: 5}, pcActor)
	apiErr(t, err, 409, "subscription_not_changeable")
	if len(store.subs) != 0 || len(store.audits) != 0 {
		t.Fatal("a workspace with no row was written")
	}
}

func TestCancelScheduledKeepsEverythingElse(t *testing.T) {
	cur := pcSub(4)
	cur.PendingChange = &PendingChange{PlanKey: "starter", Seats: 3, Interval: IntervalYear, RequestedBy: staffID, RequestedAt: t0}
	svc, store, _ := pcService(t, &cur, pcPending{})
	res, err := svc.CancelScheduled(context.Background(), wsID, pcActor)
	if err != nil {
		t.Fatal(err)
	}
	got := store.subs[wsID]
	if got.PendingChange != nil || got.PlanKey != "pro" || got.Seats != 5 || !res.Changed {
		t.Fatalf("after cancelling the scheduled change: %+v, result %+v", got, res)
	}
	if len(store.audits) != 1 || store.audits[0].Action != audit.ActionSubscriptionChangeCanceled {
		t.Fatalf("audit = %+v, want one %s row", store.audits, audit.ActionSubscriptionChangeCanceled)
	}
}

func TestCancelScheduledWithNothingPendingWritesNothing(t *testing.T) {
	cur := pcSub(4)
	svc, store, _ := pcService(t, &cur, pcPending{})
	res, err := svc.CancelScheduled(context.Background(), wsID, pcActor)
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed || store.subs[wsID].Version != 4 || len(store.audits) != 0 {
		t.Fatalf("no-op changed: %+v, version %d, audit %d", res, store.subs[wsID].Version, len(store.audits))
	}
}

// Spec 7b.1 rule 8: the state table of cancel and resume.
func TestCancelAndResumeFollowTheStateTable(t *testing.T) {
	cases := []struct {
		name       string
		status     string
		cancelFlag bool
		now        time.Time
		trialEnd   *time.Time
		wantCancel bool // CanCancel
		wantResume bool // CanResume
	}{
		{"active inside the period", StatusActive, false, t0, nil, true, false},
		{"active with cancel requested, before the end", StatusActive, true, t0, nil, true, true},
		{"trialing inside the trial", StatusTrialing, false, t0, nil, true, false},
		{"past_due", "past_due", false, t0, nil, false, false},
		{"read_only", "read_only", false, t0, nil, false, false},
		{"canceled", "canceled", false, t0, nil, false, false},
		{"active, period already over", StatusActive, true, time.Date(2030, 6, 1, 0, 0, 0, 0, time.UTC), nil, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := pcSub(4)
			s.Status, s.CancelAtPeriodEnd, s.TrialEnd = c.status, c.cancelFlag, c.trialEnd
			if got := CanCancel(s); got != c.wantCancel {
				t.Errorf("CanCancel = %v, want %v", got, c.wantCancel)
			}
			if got := CanResume(s, c.now); got != c.wantResume {
				t.Errorf("CanResume = %v, want %v", got, c.wantResume)
			}
		})
	}
}

func TestTrialingResumesBeforeTheTrialEnds(t *testing.T) {
	s := pcSub(4)
	s.Status, s.CancelAtPeriodEnd = StatusTrialing, true
	trialEnd := time.Date(2030, 5, 20, 0, 0, 0, 0, time.UTC)
	s.TrialEnd = &trialEnd
	if !CanResume(s, t0) {
		t.Fatal("a trial that has not ended should resume")
	}
	if CanResume(s, trialEnd) {
		t.Fatal("a trial that has ended should not resume")
	}
}

func TestCancelRefusesAStatusOutsideTheTable(t *testing.T) {
	cur := pcSub(4)
	cur.Status = "past_due"
	svc, store, _ := pcService(t, &cur, pcPending{})
	_, err := svc.SetCancel(context.Background(), wsID, true, pcActor)
	apiErr(t, err, 409, "subscription_not_cancelable")
	if store.subs[wsID].CancelAtPeriodEnd || len(store.audits) != 0 {
		t.Fatal("a refused cancel wrote")
	}
}

func TestCancelThenResumeWritesOneAuditRowEach(t *testing.T) {
	cur := pcSub(4)
	cur.PendingChange = &PendingChange{PlanKey: "starter", Seats: 3, Interval: IntervalYear, RequestedBy: staffID, RequestedAt: t0}
	svc, store, _ := pcService(t, &cur, pcPending{})
	if _, err := svc.SetCancel(context.Background(), wsID, true, pcActor); err != nil {
		t.Fatal(err)
	}
	if !store.subs[wsID].CancelAtPeriodEnd {
		t.Fatal("cancel did not set cancel_at_period_end")
	}
	if store.subs[wsID].PendingChange == nil {
		t.Fatal("cancel removed the pending change; it must stay until the subscription resumes")
	}
	if _, err := svc.SetCancel(context.Background(), wsID, false, pcActor); err != nil {
		t.Fatal(err)
	}
	if store.subs[wsID].CancelAtPeriodEnd {
		t.Fatal("resume did not clear cancel_at_period_end")
	}
	if len(store.audits) != 2 || store.audits[0].Action != audit.ActionSubscriptionCancelRequested ||
		store.audits[1].Action != audit.ActionSubscriptionCancelWithdrawn {
		t.Fatalf("audit = %+v, want cancel_requested then cancel_withdrawn", store.audits)
	}
}

func TestRepeatedCancelWritesNothing(t *testing.T) {
	cur := pcSub(4)
	cur.CancelAtPeriodEnd = true
	svc, store, _ := pcService(t, &cur, pcPending{})
	res, err := svc.SetCancel(context.Background(), wsID, true, pcActor)
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed || store.subs[wsID].Version != 4 || len(store.audits) != 0 {
		t.Fatalf("repeated cancel changed %+v, version %d, audit %d", res, store.subs[wsID].Version, len(store.audits))
	}
}

func TestResumeAfterThePeriodIsRefused(t *testing.T) {
	cur := pcSub(4)
	cur.CancelAtPeriodEnd = true
	svc, store, _ := pcService(t, &cur, pcPending{})
	svc.now = func() time.Time { return time.Date(2030, 6, 2, 0, 0, 0, 0, time.UTC) }
	_, err := svc.SetCancel(context.Background(), wsID, false, pcActor)
	apiErr(t, err, 409, "subscription_not_resumable")
	if !store.subs[wsID].CancelAtPeriodEnd || len(store.audits) != 0 {
		t.Fatal("a refused resume wrote")
	}
}
