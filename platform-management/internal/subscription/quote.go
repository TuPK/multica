package subscription

import (
	"errors"
	"math"
	"math/big"
	"net/http"
	"strconv"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/plan"
)

// Mode is how a plan change takes effect (spec 7b.1, rules 5 to 7).
type Mode string

const (
	// ModeCharge is an upgrade on the same seat model and interval: it takes
	// effect now, for the prorated difference (rule 5).
	ModeCharge Mode = "charge"
	// ModeSubscribe is a change from a free or trialing workspace: a full
	// period at the new price, from payment (rule 5).
	ModeSubscribe Mode = "subscribe"
	// ModeSchedule is a downgrade, a seat reduction or an interval change. It
	// is stored as pending_change and applies at the next renewal (rule 6).
	ModeSchedule Mode = "schedule"
)

// Request is a wanted plan change. Seats 0 means the current seats.
type Request struct {
	PlanKey  string
	Interval string
	Seats    int
}

// QuoteInput is what a quote is made from.
type QuoteInput struct {
	Current Subscription
	// HasRow is false for a workspace with no subscription row (the free plan).
	HasRow bool
	// CurrentPlan is the plan of Current. It is read only when Current has no
	// unit_price (a row written before unit_price was stored).
	CurrentPlan plan.Plan
	Target      plan.Plan
	Request     Request
	Now         time.Time
}

// Quote is the outcome of a plan change at one moment. Amount is before VAT:
// the prorated difference for ModeCharge, the full price for ModeSubscribe, and
// 0 for ModeSchedule.
type Quote struct {
	Mode   Mode
	Seats  int
	Amount int64
	// UnitPrice is the target plan's seat price for the interval.
	UnitPrice int64
}

// QuotePlanChange decides how a plan change takes effect and prices it. It
// refuses a request that changes nothing, or that the target plan cannot take.
func QuotePlanChange(in QuoteInput) (Quote, *Error) {
	if in.Request.Interval != IntervalMonth && in.Request.Interval != IntervalYear {
		return Quote{}, badRequest("invalid_subscription", "interval must be month or year")
	}
	if in.Request.Seats < 0 || in.Request.Seats > maxSeats {
		return Quote{}, badRequest("invalid_subscription", "seats must be from 0 to 1000000")
	}
	cur := in.Current

	// Rule 7: from a paid seat plan to a usage plan, the change waits for the end
	// of the paid period. It has no price and no seats. Any other way onto a
	// usage plan is the checkout route's.
	if in.Target.BillingModel == plan.BillingUsage && in.HasRow && cur.Status == StatusActive &&
		cur.PlanKey != plan.FreeKey && in.CurrentPlan.BillingModel == plan.BillingSeat {
		return Quote{Mode: ModeSchedule}, nil
	}

	unit, ok := SeatUnitPrice(in.Target, in.Request.Interval)
	if !ok {
		return Quote{}, badRequest("plan_not_purchasable", "the plan has no seat price for this interval")
	}
	minimum := minSeats(in.Target)

	// A workspace with no row, a free plan or a trial has no paid period to
	// prorate: it buys one period at the full price (rule 5).
	if !in.HasRow || cur.Status == StatusTrialing || cur.PlanKey == plan.FreeKey {
		seats := in.Request.Seats
		if seats == 0 {
			seats = minimum
		}
		if seats < minimum {
			return Quote{}, badRequest("seats_below_minimum", "the plan needs at least "+strconv.Itoa(minimum)+" seats")
		}
		amount, err := multiply(unit, seats)
		if err != nil {
			return Quote{}, badRequest("invalid_subscription", "the price of these seats is too large")
		}
		return Quote{Mode: ModeSubscribe, Seats: seats, Amount: amount, UnitPrice: unit}, nil
	}

	curUnit, ok := currentUnit(cur, in.CurrentPlan)
	if !ok {
		return Quote{}, conflict("subscription_not_changeable", "the current plan has no seat price to change from")
	}

	// Rule 5: on the same interval, a change that raises the price is an upgrade
	// now. Its seats are the larger of the current seats and the target's minimum.
	if cur.Interval == in.Request.Interval {
		upgradeSeats := max(cur.Seats, minimum)
		newTotal, err1 := multiply(unit, upgradeSeats)
		oldTotal, err2 := multiply(curUnit, cur.Seats)
		if err1 != nil || err2 != nil {
			return Quote{}, badRequest("invalid_subscription", "the price of these seats is too large")
		}
		if delta := newTotal - oldTotal; delta > 0 {
			if in.Request.Seats != 0 && in.Request.Seats != upgradeSeats {
				return Quote{}, badRequest("invalid_subscription",
					"an upgrade keeps seats at the larger of the current seats and the plan's minimum; send that number or omit seats")
			}
			if cur.CurrentPeriodStart == nil || cur.CurrentPeriodEnd == nil {
				return Quote{}, conflict("subscription_not_changeable", "the subscription has no current period")
			}
			amount, err := ProrateChange(delta, *cur.CurrentPeriodStart, *cur.CurrentPeriodEnd, in.Now)
			if err != nil {
				return Quote{}, conflict("subscription_not_changeable", "the current period has ended")
			}
			return Quote{Mode: ModeCharge, Seats: upgradeSeats, Amount: amount, UnitPrice: unit}, nil
		}
	}

	// Rule 6: a downgrade, a seat reduction or an interval change is stored for
	// the next renewal and changes nothing now.
	seats := in.Request.Seats
	if seats == 0 {
		seats = cur.Seats
	}
	if seats < minimum {
		return Quote{}, badRequest("seats_below_minimum", "the plan needs at least "+strconv.Itoa(minimum)+" seats")
	}
	if in.Target.Key == cur.PlanKey && in.Request.Interval == cur.Interval && seats == cur.Seats {
		return Quote{}, badRequest("no_change", "the subscription already has this plan, interval and seat count")
	}
	return Quote{Mode: ModeSchedule, Seats: seats, UnitPrice: unit}, nil
}

// SeatUnitPrice is a plan's seat price for an interval. ok is false when the plan
// sells no seats for it.
func SeatUnitPrice(p plan.Plan, interval string) (int64, bool) {
	var price *int64
	switch interval {
	case IntervalMonth:
		price = p.SeatPriceMonth
	case IntervalYear:
		price = p.SeatPriceYear
	default:
		return 0, false
	}
	if price == nil || *price <= 0 {
		return 0, false
	}
	return *price, true
}

// currentUnit is the seat price the current period was paid at: the stored
// unit_price, else the current plan's catalogue price for rows written before
// unit_price existed.
func currentUnit(cur Subscription, curPlan plan.Plan) (int64, bool) {
	if cur.UnitPrice != nil {
		return *cur.UnitPrice, *cur.UnitPrice > 0
	}
	return SeatUnitPrice(curPlan, cur.Interval)
}

// minSeats is the seat floor of a plan. A plan without one needs one seat.
func minSeats(p plan.Plan) int {
	if p.MinSeats != nil && *p.MinSeats > 1 {
		return *p.MinSeats
	}
	return 1
}

func multiply(price int64, seats int) (int64, error) {
	if price > math.MaxInt64/int64(seats) {
		return 0, errors.New("billing: price is too large")
	}
	return price * int64(seats), nil
}

func conflict(code, msg string) *Error {
	return &Error{Status: http.StatusConflict, Code: code, Message: msg}
}

// ProrateChange is the price of a change worth delta before VAT, for the days
// left in [periodStart, periodEnd) from now: delta x R / P, with P and R the days
// in the period and the days left, both rounded up, rounded half up to whole
// VND.
func ProrateChange(delta int64, periodStart, periodEnd, now time.Time) (int64, error) {
	if delta <= 0 {
		return 0, errors.New("subscription: the change must add to the price")
	}
	if !periodEnd.After(periodStart) {
		return 0, errors.New("subscription: the period must end after it starts")
	}
	if now.Before(periodStart) || !now.Before(periodEnd) {
		return 0, errors.New("subscription: the instant is outside the period")
	}
	p := daysCeil(periodEnd.Sub(periodStart))
	r := daysCeil(periodEnd.Sub(now))
	// round half up of delta x r / p, for positive operands: floor((2n + p) / 2p).
	n := new(big.Int).Mul(big.NewInt(delta), big.NewInt(r))
	num := new(big.Int).Add(new(big.Int).Lsh(n, 1), big.NewInt(p))
	out := num.Quo(num, big.NewInt(2*p))
	if !out.IsInt64() {
		return 0, errors.New("subscription: amount is too large")
	}
	return out.Int64(), nil
}

const day = 24 * time.Hour

// daysCeil is d in days, rounded up.
func daysCeil(d time.Duration) int64 {
	return int64((d + day - 1) / day)
}
