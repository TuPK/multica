package billing

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/plan"
)

// Billing intervals, as stored in billing_order and workspace_subscription.
const (
	IntervalMonth = "month"
	IntervalYear  = "year"
)

// MaxSeats is the most seats one order can carry, the same bound the plan
// catalogue puts on a seat count.
const MaxSeats = 1_000_000

const day = 24 * time.Hour

// seatPrice returns a plan's seat price for an interval. A plan with no price
// for the interval, or a price of 0 (the free plan), does not sell seats.
func seatPrice(p plan.Plan, interval string) (int64, error) {
	var price *int64
	switch interval {
	case IntervalMonth:
		price = p.SeatPriceMonth
	case IntervalYear:
		price = p.SeatPriceYear
	default:
		return 0, fmt.Errorf("billing: unknown interval %q", interval)
	}
	if price == nil || *price <= 0 {
		return 0, fmt.Errorf("billing: plan %q has no seat price for %s", p.Key, interval)
	}
	return *price, nil
}

// Price is the seat price of the plan for the interval times the seats, before
// VAT. A plan that does not sell seats is an error.
func Price(p plan.Plan, interval string, seats int) (int64, error) {
	price, err := seatPrice(p, interval)
	if err != nil {
		return 0, err
	}
	if seats < 1 || seats > MaxSeats {
		return 0, fmt.Errorf("billing: seats must be from 1 to %d", MaxSeats)
	}
	if price > math.MaxInt64/int64(seats) {
		return 0, errors.New("billing: price is too large")
	}
	return price * int64(seats), nil
}

// PeriodEnd is the end of the period that starts at start: one month or one year
// later, in UTC. When the later month is shorter, the end is its last day, so a
// period starting on 31 January ends on 28 February (29 in a leap year).
func PeriodEnd(start time.Time, interval string) (time.Time, error) {
	start = start.UTC()
	var months int
	switch interval {
	case IntervalMonth:
		months = 1
	case IntervalYear:
		months = 12
	default:
		return time.Time{}, fmt.Errorf("billing: unknown interval %q", interval)
	}
	y, m, d := start.Date()
	target := time.Date(y, m+time.Month(months), 1, start.Hour(), start.Minute(), start.Second(), start.Nanosecond(), time.UTC)
	last := time.Date(target.Year(), target.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	return time.Date(target.Year(), target.Month(), min(d, last), start.Hour(), start.Minute(), start.Second(), start.Nanosecond(), time.UTC), nil
}

// daysCeil is the length of d in days, rounded up.
func daysCeil(d time.Duration) int64 {
	return int64((d + day - 1) / day)
}

// ProrateAdditionalSeats is the price of extra seats bought at now, for the days
// left in the period [periodStart, periodEnd). With P the days in the period and
// R the days left from now, both rounded up, the amount is
//
//	planSeatPrice x seats x R / P
//
// rounded half up to whole VND. It is before VAT. now must fall inside the
// period: a purchase before the start or at or after the end is refused.
func ProrateAdditionalSeats(planSeatPrice int64, seats int, periodStart, periodEnd, now time.Time) (int64, error) {
	if planSeatPrice <= 0 {
		return 0, errors.New("billing: the seat price must be positive")
	}
	if seats < 1 || seats > MaxSeats {
		return 0, fmt.Errorf("billing: seats must be from 1 to %d", MaxSeats)
	}
	if !periodEnd.After(periodStart) {
		return 0, errors.New("billing: the period must end after it starts")
	}
	if now.Before(periodStart) {
		return 0, errors.New("billing: the purchase is before the period starts")
	}
	if !now.Before(periodEnd) {
		return 0, errors.New("billing: the period has ended")
	}
	p := daysCeil(periodEnd.Sub(periodStart))
	r := daysCeil(periodEnd.Sub(now))
	n := new(big.Int).Mul(big.NewInt(planSeatPrice), big.NewInt(int64(seats)))
	n.Mul(n, big.NewInt(r))
	out := divRoundHalfUp(n, big.NewInt(p))
	if !out.IsInt64() {
		return 0, errors.New("billing: amount is too large")
	}
	return out.Int64(), nil
}
