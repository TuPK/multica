package billing

import (
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/plan"
)

func i64(v int64) *int64 { return &v }

func proPlan() plan.Plan {
	return plan.Plan{Key: "pro", Currency: "vnd", SeatPriceMonth: i64(149000), SeatPriceYear: i64(1490000)}
}

func TestPrice(t *testing.T) {
	odd := plan.Plan{Key: "odd", Currency: "vnd", SeatPriceMonth: i64(149001), SeatPriceYear: i64(1490001)}
	cases := []struct {
		name     string
		p        plan.Plan
		interval string
		seats    int
		want     int64
	}{
		{"one seat, month", proPlan(), "month", 1, 149000},
		{"three seats, month: 3 * 149000", proPlan(), "month", 3, 447000},
		{"five seats, year: 5 * 1490000", proPlan(), "year", 5, 7450000},
		{"VND has no decimal part: the product stays exact", odd, "month", 3, 447003},
		{"VND has no decimal part, year", odd, "year", 7, 10430007},
		{"the most seats a plan allows", proPlan(), "month", 1_000_000, 149_000_000_000},
	}
	for _, c := range cases {
		got, err := Price(c.p, c.interval, c.seats)
		if err != nil || got != c.want {
			t.Errorf("%s: Price = %d, %v; want %d", c.name, got, err, c.want)
		}
	}
}

func TestPriceRefuses(t *testing.T) {
	free := plan.Plan{Key: "free", Currency: "vnd", SeatPriceMonth: i64(0), SeatPriceYear: i64(0)}
	noPrice := plan.Plan{Key: "usage", Currency: "vnd"}
	monthOnly := plan.Plan{Key: "m", Currency: "vnd", SeatPriceMonth: i64(1000)}
	huge := plan.Plan{Key: "huge", Currency: "vnd", SeatPriceMonth: i64(1 << 62)}
	cases := []struct {
		name     string
		p        plan.Plan
		interval string
		seats    int
	}{
		{"free plan", free, "month", 1},
		{"plan with no seat price", noPrice, "month", 1},
		{"no price for the year", monthOnly, "year", 1},
		{"unknown interval", proPlan(), "week", 1},
		{"empty interval", proPlan(), "", 1},
		{"zero seats", proPlan(), "month", 0},
		{"negative seats", proPlan(), "month", -1},
		{"more seats than allowed", proPlan(), "month", 1_000_001},
		{"product overflows int64", huge, "month", 4},
	}
	for _, c := range cases {
		if got, err := Price(c.p, c.interval, c.seats); err == nil {
			t.Errorf("%s: Price = %d, want an error", c.name, got)
		}
	}
}

func date(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func TestPeriodEnd(t *testing.T) {
	cases := []struct {
		name     string
		start    time.Time
		interval string
		want     time.Time
	}{
		{"month, first day", date(2025, 3, 1), "month", date(2025, 4, 1)},
		{"month, a period start at year end", date(2025, 12, 15), "month", date(2026, 1, 15)},
		{"month, 31st into a 30-day month", date(2025, 3, 31), "month", date(2025, 4, 30)},
		{"month, 31st into February", date(2025, 1, 31), "month", date(2025, 2, 28)},
		{"month, leap-year February", date(2024, 1, 31), "month", date(2024, 2, 29)},
		{"month, from a leap February", date(2024, 2, 29), "month", date(2024, 3, 29)},
		{"year", date(2025, 6, 10), "year", date(2026, 6, 10)},
		{"year across a leap day", date(2023, 3, 1), "year", date(2024, 3, 1)},
		{"year from 29 February", date(2024, 2, 29), "year", date(2025, 2, 28)},
		{"year, the day after a leap day", date(2024, 3, 1), "year", date(2025, 3, 1)},
	}
	for _, c := range cases {
		got, err := PeriodEnd(c.start, c.interval)
		if err != nil || !got.Equal(c.want) {
			t.Errorf("%s: PeriodEnd = %v, %v; want %v", c.name, got, err, c.want)
		}
	}
	if _, err := PeriodEnd(date(2025, 1, 1), "week"); err == nil {
		t.Error("unknown interval accepted")
	}
	// A time in another zone is read in UTC.
	plus7 := time.FixedZone("ICT", 7*3600)
	got, err := PeriodEnd(time.Date(2025, 3, 1, 5, 0, 0, 0, plus7), "month") // 2025-02-28 22:00 UTC
	if err != nil || !got.Equal(time.Date(2025, 3, 28, 22, 0, 0, 0, time.UTC)) {
		t.Errorf("PeriodEnd in a zone = %v, %v", got, err)
	}
}

// 600000 = 3 seats at 200000. R and P are counted in days, rounded up.
func TestProrateAdditionalSeats(t *testing.T) {
	at := func(y int, m time.Month, d, h int) time.Time { return time.Date(y, m, d, h, 0, 0, 0, time.UTC) }
	cases := []struct {
		name       string
		price      int64
		seats      int
		start, end time.Time
		now        time.Time
		want       int64
	}{
		// 31-day period; the whole period remains: full price.
		{"first day (now = period start)", 200000, 3, date(2025, 3, 1), date(2025, 4, 1), date(2025, 3, 1), 600000},
		// 600000 * 1 / 31 = 19354.83 -> 19355
		{"last day (one day left)", 200000, 3, date(2025, 3, 1), date(2025, 4, 1), at(2025, 3, 31, 0), 19355},
		// 12 hours left rounds up to 1 day: same as the last day.
		{"last half day counts as one day", 200000, 3, date(2025, 3, 1), date(2025, 4, 1), at(2025, 3, 31, 12), 19355},
		// 2024-02-01..2024-03-01 is 29 days; 14 days after the start leaves 15. 600000 * 15 / 29 = 310344.83
		{"leap-year February", 200000, 3, date(2024, 2, 1), date(2024, 3, 1), date(2024, 2, 15), 310345},
		// the same dates in a non-leap year: 28 days, 14 left. 600000 * 14 / 28 = 300000 exactly.
		{"non-leap February", 200000, 3, date(2025, 2, 1), date(2025, 3, 1), date(2025, 2, 15), 300000},
		// 2025-01-20..2025-02-20 is 31 days; now 2025-02-05 leaves 15. 149000 * 2 * 15 / 31 = 144193.5483 -> 144194
		{"period that ends in a different month", 149000, 2, date(2025, 1, 20), date(2025, 2, 20), date(2025, 2, 5), 144194},
		// exactly .5: price 1, 1 seat, 2-day period, 1 day left: 1 * 1 / 2 = 0.5 -> 1
		{"half rounds up", 1, 1, date(2025, 3, 1), date(2025, 3, 3), date(2025, 3, 2), 1},
		// 1 * 1 / 3 = 0.33 -> 0
		{"below half rounds down", 1, 1, date(2025, 3, 1), date(2025, 3, 4), date(2025, 3, 3), 0},
		// 3-day period, 2 days left: 5 * 2 / 3 = 3.33 -> 3
		{"two thirds", 5, 1, date(2025, 3, 1), date(2025, 3, 4), date(2025, 3, 2), 3},
		// a year period: 365 days, 100 days left: 1490000 * 4 * 100 / 365 = 1632876.7123 -> 1632877
		{"year period", 1490000, 4, date(2025, 1, 1), date(2026, 1, 1), date(2025, 9, 23), 1632877},
	}
	for _, c := range cases {
		got, err := ProrateAdditionalSeats(c.price, c.seats, c.start, c.end, c.now)
		if err != nil || got != c.want {
			t.Errorf("%s: = %d, %v; want %d", c.name, got, err, c.want)
		}
	}
}

func TestProrateAdditionalSeatsRefuses(t *testing.T) {
	start, end := date(2025, 3, 1), date(2025, 4, 1)
	cases := []struct {
		name       string
		price      int64
		seats      int
		start, end time.Time
		now        time.Time
	}{
		{"now before the period start", 200000, 1, start, end, start.Add(-time.Second)},
		{"now at the period end", 200000, 1, start, end, end},
		{"now after the period end", 200000, 1, start, end, end.Add(time.Hour)},
		{"empty period", 200000, 1, start, start, start},
		{"end before start", 200000, 1, end, start, start},
		{"zero seats", 200000, 0, start, end, start},
		{"negative seats", 200000, -1, start, end, start},
		{"zero price", 0, 1, start, end, start},
		{"negative price", -1, 1, start, end, start},
		{"more seats than allowed", 200000, 1_000_001, start, end, start},
	}
	for _, c := range cases {
		if got, err := ProrateAdditionalSeats(c.price, c.seats, c.start, c.end, c.now); err == nil {
			t.Errorf("%s: = %d, want an error", c.name, got)
		}
	}
}
