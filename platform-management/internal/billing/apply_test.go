package billing

import (
	"errors"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
)

func periodOrder(kind Kind) Order {
	start, end := date(2025, 3, 1), date(2025, 4, 1)
	o := testOrder()
	o.Kind = kind
	o.PeriodStart, o.PeriodEnd = &start, &end
	return o
}

func currentSub() subscription.Subscription {
	start, end, grace := date(2025, 2, 1), date(2025, 3, 1), date(2025, 3, 8)
	return subscription.Subscription{
		WorkspaceID: testOrder().WorkspaceID, PlanKey: "pro", Status: "past_due", Seats: 5, Interval: "month",
		CurrentPeriodStart: &start, CurrentPeriodEnd: &end, GraceUntil: &grace, CancelAtPeriodEnd: true,
		LimitOverrides: plan.Limits{"agents": i64(40)}, Version: 7,
	}
}

func TestSubscriptionAfterSubscribeWithoutARow(t *testing.T) {
	o := periodOrder(KindSubscribe)
	got, err := SubscriptionAfter(o, subscription.Subscription{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.WorkspaceID != o.WorkspaceID || got.PlanKey != "pro" || got.Seats != 3 || got.Interval != "month" ||
		got.Status != subscription.StatusActive || !got.CurrentPeriodStart.Equal(*o.PeriodStart) ||
		!got.CurrentPeriodEnd.Equal(*o.PeriodEnd) || got.GraceUntil != nil || got.LimitOverrides == nil {
		t.Fatalf("subscription = %+v", got)
	}
}

func TestSubscriptionAfterRenewAndChangePlanReplaceTheTerms(t *testing.T) {
	for _, kind := range []Kind{KindSubscribe, KindRenew, KindChangePlan} {
		o := periodOrder(kind)
		o.PlanKey, o.Interval, o.Seats = "team", "year", 9
		got, err := SubscriptionAfter(o, currentSub(), true)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if got.PlanKey != "team" || got.Interval != "year" || got.Seats != 9 || got.Status != subscription.StatusActive ||
			got.GraceUntil != nil || !got.CurrentPeriodStart.Equal(*o.PeriodStart) || !got.CurrentPeriodEnd.Equal(*o.PeriodEnd) {
			t.Errorf("%s: subscription = %+v", kind, got)
		}
		// What the order does not own stays as it was.
		if !got.CancelAtPeriodEnd || got.LimitOverrides["agents"] == nil || got.Version != 7 {
			t.Errorf("%s: changed a field the order does not own: %+v", kind, got)
		}
	}
}

func TestSubscriptionAfterAddSeatsAddsToTheCount(t *testing.T) {
	o := periodOrder(KindAddSeats) // 3 seats added
	got, err := SubscriptionAfter(o, currentSub(), true)
	if err != nil {
		t.Fatal(err)
	}
	cur := currentSub()
	if got.Seats != 8 {
		t.Errorf("seats = %d, want 5 + 3", got.Seats)
	}
	if got.PlanKey != cur.PlanKey || got.Interval != cur.Interval || got.Status != cur.Status ||
		!got.CurrentPeriodEnd.Equal(*cur.CurrentPeriodEnd) || !got.CurrentPeriodStart.Equal(*cur.CurrentPeriodStart) {
		t.Errorf("add_seats changed more than the seats: %+v", got)
	}
}

func TestSubscriptionAfterRefuses(t *testing.T) {
	noPeriod := periodOrder(KindSubscribe)
	noPeriod.PeriodEnd = nil
	backwards := periodOrder(KindRenew)
	backwards.PeriodStart, backwards.PeriodEnd = backwards.PeriodEnd, backwards.PeriodStart
	zeroSeats := periodOrder(KindSubscribe)
	zeroSeats.Seats = 0
	otherPlan := periodOrder(KindAddSeats)
	otherPlan.PlanKey = "team"
	unknown := periodOrder("usage")
	cases := []struct {
		name   string
		o      Order
		exists bool
		want   error
	}{
		{"no period", noPeriod, true, nil},
		{"period ends before it starts", backwards, true, nil},
		{"zero seats", zeroSeats, true, nil},
		{"add_seats without a subscription", periodOrder(KindAddSeats), false, ErrNoSubscription},
		{"add_seats for another plan", otherPlan, true, ErrPlanMismatch},
		{"unknown kind", unknown, true, nil},
	}
	for _, c := range cases {
		_, err := SubscriptionAfter(c.o, currentSub(), c.exists)
		if err == nil {
			t.Errorf("%s: accepted", c.name)
		} else if c.want != nil && !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
}
