package usage

import (
	"math/big"
	"strings"
	"testing"
	"time"
)

// oracleCharge is an independent reference for Charge: exact rational
// arithmetic with the half-up rule applied through big.Rat, not the integer
// formula Charge uses.
func oracleCharge(quantity int64, r Rate, discountPct, days int64) int64 {
	num := new(big.Rat).SetInt64(quantity * r.Price)
	num.Mul(num, big.NewRat(100-discountPct, 100))
	num.Quo(num, new(big.Rat).SetInt64(r.Per*days))
	num.Add(num, big.NewRat(1, 2))
	return new(big.Int).Quo(num.Num(), num.Denom()).Int64()
}

var (
	ratePerTask    = Rate{Per: 1000, Price: 30000}
	rateStorage    = Rate{Per: 1 << 30, Price: 3000}
	rateMemberSeat = Rate{Per: 1, Price: 49000}
)

func TestChargeTableAgreesWithTheExactReference(t *testing.T) {
	meters := []struct {
		name string
		rate Rate
	}{
		{MeterTasksRun, ratePerTask},
		{MeterStorageBytes, rateStorage},
		{MeterMembers, rateMemberSeat},
	}
	// 100 TB is 1e14 bytes; the largest quantity the tests use.
	quantities := []int64{0, 1, 2, 3, 7, 999, 1000, 1 << 30, 1e9, 1e14}
	months := []int64{28, 29, 30, 31}
	discounts := []int64{0, 20, 100}

	for _, m := range meters {
		for _, days := range months {
			for _, disc := range discounts {
				for _, q := range quantities {
					got, err := Charge(q, m.rate, disc, days)
					if err != nil {
						t.Fatalf("%s q=%d days=%d disc=%d: %v", m.name, q, days, disc, err)
					}
					if want := oracleCharge(q, m.rate, disc, days); got != want {
						t.Errorf("%s q=%d days=%d disc=%d = %d, want %d", m.name, q, days, disc, got, want)
					}
				}
			}
		}
	}
}

func TestChargeKnownValues(t *testing.T) {
	cases := []struct {
		name     string
		quantity int64
		rate     Rate
		disc     int64
		days     int64
		want     int64
	}{
		{"one thousand tasks at the list price", 1000, ratePerTask, 0, 1, 30000},
		{"tasks with a 20 percent discount", 1000, ratePerTask, 20, 1, 24000},
		{"tasks with a 100 percent discount", 1000, ratePerTask, 100, 1, 0},
		{"one GiB-month over a 30 day month is one day's 100", 1 << 30, rateStorage, 0, 30, 100},
		{"ten members over a 28 day month", 10, rateMemberSeat, 0, 28, 17500},
		{"ten members over a 29 day month rounds 16896.55 up", 10, rateMemberSeat, 0, 29, 16897},
		{"ten members over a 31 day month", 10, rateMemberSeat, 0, 31, 15806},
		{"100 TB of storage over 31 days", 100 * 1000 * 1000 * 1000 * 1000, rateStorage, 0, 31, 9012799},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Charge(c.quantity, c.rate, c.disc, c.days)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Fatalf("Charge = %d, want %d", got, c.want)
			}
		})
	}
}

func TestChargeRoundsHalvesUpOnlyAtTheBoundary(t *testing.T) {
	cases := []struct {
		name     string
		quantity int64
		rate     Rate
		disc     int64
		want     int64
	}{
		{"exactly one half rounds up to one", 1, Rate{Per: 2, Price: 1}, 0, 1},
		{"one and a half rounds up to two", 3, Rate{Per: 2, Price: 1}, 0, 2},
		{"a third rounds down to zero", 1, Rate{Per: 3, Price: 1}, 0, 0},
		{"two thirds rounds up to one", 2, Rate{Per: 3, Price: 1}, 0, 1},
		{"4.5 after a 10 percent discount rounds up to five", 5, Rate{Per: 1, Price: 1}, 10, 5},
		{"4.4 after a 10 percent discount rounds down to four", 5, Rate{Per: 1, Price: 1}, 12, 4},
		{"a whole value stays whole", 4, Rate{Per: 1, Price: 1}, 0, 4},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Charge(c.quantity, c.rate, c.disc, 1)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Fatalf("Charge = %d, want %d", got, c.want)
			}
		})
	}
}

func TestChargeRefusesInputsThatCannotBeRated(t *testing.T) {
	cases := []struct {
		name     string
		quantity int64
		rate     Rate
		disc     int64
		days     int64
	}{
		{"negative quantity", -1, ratePerTask, 0, 1},
		{"negative price", 1, Rate{Per: 1, Price: -1}, 0, 1},
		{"zero per", 1, Rate{Per: 0, Price: 1}, 0, 1},
		{"discount above 100", 1, ratePerTask, 101, 1},
		{"negative discount", 1, ratePerTask, -1, 1},
		{"no days", 1, ratePerTask, 0, 0},
		{"result overflows int64", 1 << 62, Rate{Per: 1, Price: 1 << 40}, 0, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := Charge(c.quantity, c.rate, c.disc, c.days); err == nil {
				t.Fatal("Charge accepted an input it cannot rate")
			}
		})
	}
}

func TestDaysForIsOneForTasksAndTheUTCMonthForStockMeters(t *testing.T) {
	cases := []struct {
		meter string
		day   time.Time
		want  int64
	}{
		{MeterTasksRun, time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC), 1},
		{MeterStorageBytes, time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC), 28},
		{MeterMembers, time.Date(2028, 2, 29, 0, 0, 0, 0, time.UTC), 29},
		{MeterMembers, time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), 30},
		{MeterMembers, time.Date(2026, 12, 31, 23, 0, 0, 0, time.UTC), 31},
	}
	for _, c := range cases {
		if got := DaysFor(c.meter, c.day); got != c.want {
			t.Errorf("DaysFor(%s, %s) = %d, want %d", c.meter, c.day.Format("2006-01-02"), got, c.want)
		}
	}
}

func TestBusinessDiscountAppliesOnlyToBusinessAndWhenVerifiedWhenTheFlagIsOn(t *testing.T) {
	terms := Terms{DiscountPct: map[string]int64{"business": 20}}
	cases := []struct {
		name        string
		kind        string
		verified    bool
		requireFlag bool
		want        int64
	}{
		{"business, verified, flag on", "business", true, true, 20},
		{"business, not verified, flag on", "business", false, true, 0},
		{"business, not verified, flag off", "business", false, false, 20},
		{"business, verified, flag off", "business", true, false, 20},
		{"individual, verified, flag on", "individual", true, true, 0},
		{"individual, not verified, flag off", "individual", false, false, 0},
		{"unknown kind, flag off", "", false, false, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := BusinessDiscountPct(terms, c.kind, c.verified, c.requireFlag); got != c.want {
				t.Fatalf("discount = %d, want %d", got, c.want)
			}
		})
	}
}

func TestRateDayKeepsTheSnapshotOfEveryRow(t *testing.T) {
	day := time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC)
	rows, err := RateDay(Input{
		Day:         day,
		PlanKey:     "payg",
		Rates:       Rates{MeterTasksRun: ratePerTask, MeterStorageBytes: rateStorage, MeterMembers: rateMemberSeat},
		DiscountPct: 20,
		Status:      StatusActive,
		Quantities:  map[string]int64{MeterTasksRun: 1000, MeterStorageBytes: 1 << 30, MeterMembers: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want one per meter", len(rows))
	}
	byMeter := map[string]Row{}
	for _, r := range rows {
		byMeter[r.Meter] = r
	}
	tasks := byMeter[MeterTasksRun]
	want := Row{Meter: MeterTasksRun, Quantity: 1000, PlanKey: "payg", UnitPrice: 30000, Per: 1000, DiscountPct: 20, NetAmount: 24000}
	if tasks != want {
		t.Fatalf("tasks row = %+v, want %+v", tasks, want)
	}
	members := byMeter[MeterMembers]
	if members.UnitPrice != 49000 || members.Per != 1 || members.DiscountPct != 20 || members.Quantity != 10 || members.PlanKey != "payg" {
		t.Fatalf("members row lost its snapshot: %+v", members)
	}
	// 10 seats * 49000 * 80 / (1 * 30 * 100) = 13066.67, so 13067.
	if members.NetAmount != 13067 {
		t.Fatalf("members net = %d, want 13067", members.NetAmount)
	}
}

func TestRateDayChargesAMeterTheRatesDoNotNameAtZero(t *testing.T) {
	rows, err := RateDay(Input{
		Day: time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC), PlanKey: "payg",
		Rates:      Rates{MeterTasksRun: ratePerTask},
		Status:     StatusActive,
		Quantities: map[string]int64{MeterTasksRun: 1000, MeterMembers: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Meter == MeterMembers && (r.NetAmount != 0 || r.UnitPrice != 0 || r.Per != 1) {
			t.Fatalf("a meter with no rate = %+v, want a zero row with per 1", r)
		}
	}
}

func TestRateDayWritesZeroRowsWithANoteForSuspendedAndPendingDeletion(t *testing.T) {
	for _, status := range []string{StatusSuspended, StatusPendingDeletion} {
		t.Run(status, func(t *testing.T) {
			rows, err := RateDay(Input{
				Day: time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC), PlanKey: "payg",
				Rates:       Rates{MeterTasksRun: ratePerTask, MeterMembers: rateMemberSeat},
				DiscountPct: 0, Status: status,
				Quantities: map[string]int64{MeterTasksRun: 1000, MeterStorageBytes: 1 << 30, MeterMembers: 10},
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 3 {
				t.Fatalf("rows = %d, want one per meter", len(rows))
			}
			for _, r := range rows {
				if r.NetAmount != 0 || r.Note != NoteWorkspaceSuspended {
					t.Fatalf("row %s = net %d note %q, want 0 and %q", r.Meter, r.NetAmount, r.Note, NoteWorkspaceSuspended)
				}
			}
		})
	}
}

func TestRateDayStillChargesAReadOnlyWorkspace(t *testing.T) {
	rows, err := RateDay(Input{
		Day: time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC), PlanKey: "payg",
		Rates:      Rates{MeterTasksRun: ratePerTask},
		Status:     StatusReadOnly,
		Quantities: map[string]int64{MeterTasksRun: 1000},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Meter == MeterTasksRun && (r.NetAmount != 30000 || r.Note != "") {
			t.Fatalf("read_only tasks row = net %d note %q, want 30000 and no note", r.NetAmount, r.Note)
		}
	}
}

func TestRateDayRefusesANegativeQuantity(t *testing.T) {
	_, err := RateDay(Input{
		Day: time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC), PlanKey: "payg",
		Rates: Rates{}, Status: StatusActive, Quantities: map[string]int64{MeterMembers: -1},
	})
	if err == nil || !strings.Contains(err.Error(), MeterMembers) {
		t.Fatalf("err = %v, want a refusal naming the meter", err)
	}
}
