package subscription

import (
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/plan"
)

func qInt(v int64) *int64 { return &v }
func qIntPtr(v int) *int  { return &v }

// qPlan is a seat-billed plan with the given monthly and yearly seat prices.
func qPlan(key string, month, year int64, minSeats *int) plan.Plan {
	return plan.Plan{
		Key: key, Currency: "VND", SeatPriceMonth: qInt(month), SeatPriceYear: qInt(year),
		MinSeats: minSeats, Audience: plan.AudienceAny, BillingModel: "seat",
	}
}

var (
	qPro     = qPlan("pro", 100000, 1000000, nil)
	qTeam    = qPlan("team", 200000, 2000000, nil)
	qAlt     = qPlan("alt", 100000, 1000000, nil) // same prices as pro
	qStarter = qPlan("starter", 50000, 500000, qIntPtr(3))
	qMinFour = qPlan("team4", 200000, 2000000, qIntPtr(4))
	qFree    = plan.Plan{Key: plan.FreeKey, Currency: "VND", Audience: plan.AudienceAny, BillingModel: "seat"}
)

func qAt(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

// qPaidMonthly is a pro workspace paying 100000 a seat for March 2030 (31 days).
func qPaidMonthly(seats int) Subscription {
	start, end := qAt("2030-03-01T00:00:00Z"), qAt("2030-04-01T00:00:00Z")
	return Subscription{
		WorkspaceID: "ws", PlanKey: "pro", Status: StatusActive, Seats: seats, Interval: IntervalMonth,
		CurrentPeriodStart: &start, CurrentPeriodEnd: &end, UnitPrice: qInt(100000),
	}
}

func qInput(cur Subscription, target plan.Plan, interval string, seats int, now string) QuoteInput {
	return QuoteInput{
		Current: cur, HasRow: true, CurrentPlan: qPro, Target: target,
		Request: Request{PlanKey: target.Key, Interval: interval, Seats: seats}, Now: qAt(now),
	}
}

// TestProrateChangeTable covers the proration itself (spec 7b.1, rule 5).
func TestProrateChangeTable(t *testing.T) {
	marStart, marEnd := qAt("2030-03-01T00:00:00Z"), qAt("2030-04-01T00:00:00Z")
	janStart, janEnd := qAt("2030-01-01T00:00:00Z"), qAt("2031-01-01T00:00:00Z")
	cases := []struct {
		name       string
		delta      int64
		start, end time.Time
		now        string
		want       int64
	}{
		{"mid-period: 16 of 31 days left", 500000, marStart, marEnd, "2030-03-16T00:00:00Z", 258065},
		{"period start: all 31 days left", 500000, marStart, marEnd, "2030-03-01T00:00:00Z", 500000},
		{"period last day: one day left", 500000, marStart, marEnd, "2030-03-31T12:00:00Z", 16129},
		{"annual period: 183 of 365 days left", 5000000, janStart, janEnd, "2030-07-02T00:00:00Z", 2506849},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ProrateChange(c.delta, c.start, c.end, qAt(c.now))
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if got != c.want {
				t.Fatalf("amount = %d, want %d", got, c.want)
			}
		})
	}
}

func TestProrateChangeRefusesAnInstantOutsideThePeriod(t *testing.T) {
	start, end := qAt("2030-03-01T00:00:00Z"), qAt("2030-04-01T00:00:00Z")
	for _, now := range []string{"2030-02-28T23:59:59Z", "2030-04-01T00:00:00Z"} {
		if _, err := ProrateChange(500000, start, end, qAt(now)); err == nil {
			t.Errorf("%s: accepted an instant outside the period", now)
		}
	}
}

// TestPlanChangeQuoteTable is the table of spec 7b.1 rules 5 and 6.
func TestPlanChangeQuoteTable(t *testing.T) {
	cases := []struct {
		name   string
		in     QuoteInput
		mode   Mode
		seats  int
		amount int64
		code   string
	}{
		{
			name: "mid-period upgrade charges the prorated difference",
			in:   qInput(qPaidMonthly(5), qTeam, IntervalMonth, 0, "2030-03-16T00:00:00Z"),
			mode: ModeCharge, seats: 5, amount: 258065,
		},
		{
			name: "upgrade at the period start charges the whole difference",
			in:   qInput(qPaidMonthly(5), qTeam, IntervalMonth, 0, "2030-03-01T00:00:00Z"),
			mode: ModeCharge, seats: 5, amount: 500000,
		},
		{
			name: "upgrade on the last day charges one day",
			in:   qInput(qPaidMonthly(5), qTeam, IntervalMonth, 0, "2030-03-31T12:00:00Z"),
			mode: ModeCharge, seats: 5, amount: 16129,
		},
		{
			name: "annual period upgrade",
			in: func() QuoteInput {
				start, end := qAt("2030-01-01T00:00:00Z"), qAt("2031-01-01T00:00:00Z")
				cur := Subscription{PlanKey: "pro", Status: StatusActive, Seats: 5, Interval: IntervalYear,
					CurrentPeriodStart: &start, CurrentPeriodEnd: &end, UnitPrice: qInt(1000000)}
				return qInput(cur, qTeam, IntervalYear, 0, "2030-07-02T00:00:00Z")
			}(),
			mode: ModeCharge, seats: 5, amount: 2506849,
		},
		{
			name: "seats of an upgrade are raised to the target's min_seats",
			in: func() QuoteInput {
				cur := qPaidMonthly(2)
				return qInput(cur, qMinFour, IntervalMonth, 0, "2030-03-01T00:00:00Z")
			}(),
			mode: ModeCharge, seats: 4, amount: 600000,
		},
		{
			name: "delta of zero is a downgrade-branch change",
			in:   qInput(qPaidMonthly(5), qAlt, IntervalMonth, 0, "2030-03-16T00:00:00Z"),
			mode: ModeSchedule, seats: 5,
		},
		{
			name: "negative delta is a downgrade-branch change",
			in:   qInput(qPaidMonthly(5), qStarter, IntervalMonth, 0, "2030-03-16T00:00:00Z"),
			mode: ModeSchedule, seats: 5,
		},
		{
			name: "seat reduction below the member count is scheduled, not refused",
			in:   qInput(qPaidMonthly(5), qPro, IntervalMonth, 2, "2030-03-16T00:00:00Z"),
			mode: ModeSchedule, seats: 2,
		},
		{
			name: "interval change is scheduled even when the price rises",
			in:   qInput(qPaidMonthly(5), qTeam, IntervalYear, 0, "2030-03-16T00:00:00Z"),
			mode: ModeSchedule, seats: 5,
		},
		{
			name: "a free workspace subscribes at the full price",
			in: QuoteInput{Current: Subscription{}, HasRow: false, CurrentPlan: qFree, Target: qTeam,
				Request: Request{PlanKey: "team", Interval: IntervalMonth, Seats: 3}, Now: qAt("2030-03-16T00:00:00Z")},
			mode: ModeSubscribe, seats: 3, amount: 600000,
		},
		{
			name: "a workspace on the free plan row subscribes at the full price",
			in: func() QuoteInput {
				cur := Subscription{PlanKey: plan.FreeKey, Status: StatusActive, Seats: 1, Interval: IntervalMonth}
				return QuoteInput{Current: cur, HasRow: true, CurrentPlan: qFree, Target: qTeam,
					Request: Request{PlanKey: "team", Interval: IntervalMonth, Seats: 2}, Now: qAt("2030-03-16T00:00:00Z")}
			}(),
			mode: ModeSubscribe, seats: 2, amount: 400000,
		},
		{
			name: "a trialing workspace subscribes at the full price",
			in: func() QuoteInput {
				cur := qPaidMonthly(5)
				cur.Status = StatusTrialing
				return qInput(cur, qTeam, IntervalMonth, 5, "2030-03-16T00:00:00Z")
			}(),
			mode: ModeSubscribe, seats: 5, amount: 1000000,
		},
		{
			name: "the same plan, interval and seats is refused as no change",
			in:   qInput(qPaidMonthly(5), qPro, IntervalMonth, 5, "2030-03-16T00:00:00Z"),
			code: "no_change",
		},
		{
			name: "a scheduled change below the target's min_seats is refused",
			in:   qInput(qPaidMonthly(5), qStarter, IntervalMonth, 2, "2030-03-16T00:00:00Z"),
			code: "seats_below_minimum",
		},
		{
			name: "a plan without a seat price is refused",
			in:   qInput(qPaidMonthly(5), qFree, IntervalMonth, 0, "2030-03-16T00:00:00Z"),
			code: "plan_not_purchasable",
		},
		{
			name: "an upgrade with other seats than its own is refused",
			in:   qInput(qPaidMonthly(5), qTeam, IntervalMonth, 7, "2030-03-16T00:00:00Z"),
			code: "invalid_subscription",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, refusal := QuotePlanChange(c.in)
			if c.code != "" {
				if refusal == nil || refusal.Code != c.code {
					t.Fatalf("refusal = %+v, want code %s", refusal, c.code)
				}
				return
			}
			if refusal != nil {
				t.Fatalf("refused: %+v", refusal)
			}
			if got.Mode != c.mode || got.Seats != c.seats || got.Amount != c.amount {
				t.Fatalf("quote = %+v, want mode %s seats %d amount %d", got, c.mode, c.seats, c.amount)
			}
		})
	}
}

// A downgrade that the amount does not decide still reports its target price.
func TestPlanChangeQuoteCarriesTheTargetUnitPrice(t *testing.T) {
	got, refusal := QuotePlanChange(qInput(qPaidMonthly(5), qTeam, IntervalMonth, 0, "2030-03-01T00:00:00Z"))
	if refusal != nil {
		t.Fatal(refusal)
	}
	if got.UnitPrice != 200000 {
		t.Fatalf("unit price = %d, want 200000", got.UnitPrice)
	}
}

// A row written before unit_price was stored is priced from its plan's catalogue
// price, so an upgrade of it still has a difference to charge.
func TestPlanChangeQuoteFallsBackToTheCataloguePrice(t *testing.T) {
	cur := qPaidMonthly(5)
	cur.UnitPrice = nil
	got, refusal := QuotePlanChange(qInput(cur, qTeam, IntervalMonth, 0, "2030-03-01T00:00:00Z"))
	if refusal != nil {
		t.Fatal(refusal)
	}
	if got.Mode != ModeCharge || got.Amount != 500000 {
		t.Fatalf("quote = %+v, want a charge of 500000", got)
	}
}
