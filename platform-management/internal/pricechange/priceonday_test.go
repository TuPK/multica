package pricechange

import (
	"context"
	"testing"
	"time"
)

// AC39 (spec 7b.4, rule 9d): the price of a past day is the `old` of the
// earliest applied change effective after 00:00 UTC of that day, else the
// current price.
func TestPricesOnADay(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 5, d, 0, 0, 0, 0, time.UTC) }
	current := Prices{FieldSeatPriceMonth: raw("300"), FieldUsageRates: raw(`{"v":3}`)}
	history := []Change{ // newest first, as Store.History returns them
		{State: StateApplied, EffectiveFrom: day(20), Old: Prices{FieldUsageRates: raw(`{"v":2}`)}},
		{State: StateApplied, EffectiveFrom: day(10), Old: Prices{FieldUsageRates: raw(`{"v":1}`), FieldSeatPriceMonth: raw("100")}},
		{State: StateCanceled, EffectiveFrom: day(3), Old: Prices{FieldUsageRates: raw(`{"v":0}`)}},
		{State: StateScheduled, EffectiveFrom: day(30), Old: Prices{FieldUsageRates: raw(`{"v":9}`)}},
	}
	for _, c := range []struct {
		name      string
		at        time.Time
		rates     string
		seatMonth string
	}{
		{"before every change", day(5), `{"v":1}`, "100"},
		{"the day before a change", day(9), `{"v":1}`, "100"},
		{"the day a change takes effect is already at the new price", day(10), `{"v":2}`, "300"},
		{"between the two changes", day(15), `{"v":2}`, "300"},
		{"the day of the second change", day(20), `{"v":3}`, "300"},
		{"after both", day(25), `{"v":3}`, "300"},
		{"a time of day counts as its day", day(10).Add(13 * time.Hour), `{"v":2}`, "300"},
	} {
		got := PricesOn(current, history, c.at)
		if string(got[FieldUsageRates]) != c.rates || string(got[FieldSeatPriceMonth]) != c.seatMonth {
			t.Errorf("%s: rates %s seat %s, want %s and %s", c.name, got[FieldUsageRates], got[FieldSeatPriceMonth], c.rates, c.seatMonth)
		}
	}
	if string(current[FieldUsageRates]) != `{"v":3}` {
		t.Error("PricesOn changed the current prices")
	}
}

func TestPriceOnDayReadsThePlanAndItsHistory(t *testing.T) {
	st := newFakeStore()
	s := newService(t, st, &fakeMail{}, nil)
	ctx := context.Background()
	from := time.Date(2026, 3, 11, 0, 0, 0, 0, time.UTC)
	if _, err := s.Schedule(ctx, admin, "pro", seatReq(from, "90000", "900000"), false, t0); err != nil {
		t.Fatal(err)
	}
	if err := s.Run(ctx, from); err != nil {
		t.Fatal(err)
	}
	for day, want := range map[time.Time]string{
		time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC): "100000",
		from: "90000",
	} {
		got, err := PriceOnDay(ctx, st, "pro", day)
		if err != nil {
			t.Fatal(err)
		}
		if string(got[FieldSeatPriceMonth]) != want {
			t.Errorf("%s: seat price %s, want %s", day.Format("2006-01-02"), got[FieldSeatPriceMonth], want)
		}
	}
}
