package usage

import (
	"math/big"
	"testing"
	"time"
)

const hundredTB = int64(100_000_000_000_000) // 100 TB in bytes

// exactCharge is round_half_up(q*price*(100-d) / (per*days*100)) in big.Int,
// with no int64 step before the end.
func exactCharge(q int64, r Rate, discount, days int64) *big.Int {
	num := new(big.Int).Mul(big.NewInt(q), big.NewInt(r.Price))
	num.Mul(num, big.NewInt(100-discount))
	den := new(big.Int).Mul(big.NewInt(r.Per), big.NewInt(days*100))
	twice := new(big.Int).Lsh(num, 1)
	twice.Add(twice, den)
	return twice.Quo(twice, new(big.Int).Lsh(den, 1))
}

// AC33: a quantity of 100 TB does not overflow, even at a price whose product
// with the quantity passes the int64 range.
func TestChargeOf100TBDoesNotOverflow(t *testing.T) {
	cases := []struct {
		name     string
		rate     Rate
		discount int64
		days     int64
	}{
		{"list price", rateStorage, 0, 31},
		{"business discount", rateStorage, 20, 28},
		// 1e14 bytes x 1e9 is 1e23, far past int64 (9.2e18).
		{"price at the platform maximum", Rate{Per: 1 << 30, Price: 1_000_000_000}, 0, 30},
		{"price at the maximum, per 1", Rate{Per: 1, Price: 1_000_000_000}, 0, 31},
	}
	for _, c := range cases {
		want := exactCharge(hundredTB, c.rate, c.discount, c.days)
		if !want.IsInt64() {
			continue // Charge reports an error for these; covered below
		}
		got, err := Charge(hundredTB, c.rate, c.discount, c.days)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got != want.Int64() {
			t.Errorf("%s: Charge = %d, want %d", c.name, got, want.Int64())
		}
	}
}

// A charge that cannot be held in an int64 is an error, never a wrapped number.
func TestChargeThatDoesNotFitAnInt64IsAnError(t *testing.T) {
	r := Rate{Per: 1, Price: 1_000_000_000}
	if exactCharge(hundredTB, r, 0, 1).IsInt64() {
		t.Skip("the case fits an int64")
	}
	if got, err := Charge(hundredTB, r, 0, 1); err == nil {
		t.Fatalf("Charge = %d with no error, want an error", got)
	}
}

func TestRateDayOf100TBStorageIsExact(t *testing.T) {
	day := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC) // October has 31 days
	rows, err := RateDay(Input{
		Day: day, PlanKey: "payg", Rates: Rates{MeterStorageBytes: rateStorage}, DiscountPct: 20, Status: StatusActive,
		Quantities: map[string]int64{MeterStorageBytes: hundredTB},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := exactCharge(hundredTB, rateStorage, 20, 31).Int64()
	for _, r := range rows {
		if r.Meter == MeterStorageBytes && (r.NetAmount != want || r.Quantity != hundredTB) {
			t.Fatalf("storage row = quantity %d net %d, want %d and %d", r.Quantity, r.NetAmount, hundredTB, want)
		}
	}
}
