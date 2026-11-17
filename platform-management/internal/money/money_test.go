package money

import (
	"math/big"
	"testing"
)

func TestDefaultCurrencyIsVND(t *testing.T) {
	if Default != "vnd" {
		t.Fatalf("Default = %q, want vnd (PLATFORM_CURRENCIES defaults to vnd, spec 7.4)", Default)
	}
}

func TestCurrencyExponentTable(t *testing.T) {
	// vnd has no minor unit: an amount is a whole number of dong (spec 7.4 rule 2).
	got, err := Exponent("vnd")
	if err != nil {
		t.Fatalf("Exponent(vnd): %v", err)
	}
	if got != 0 {
		t.Fatalf("Exponent(vnd) = %d, want 0", got)
	}
}

func TestUnknownCurrencyRejected(t *testing.T) {
	for _, code := range []string{"", "VND", "usd", "xyz"} {
		if _, err := Exponent(code); err == nil {
			t.Errorf("Exponent(%q) accepted a currency that is not in the enabled table", code)
		}
	}
}

func TestRoundingHalfUpMinorUnits(t *testing.T) {
	for _, tc := range []struct {
		name     string
		num, den int64
		want     int64
	}{
		{"exact", 10, 5, 2},
		{"below half rounds down", 14, 10, 1},
		{"half rounds up", 15, 10, 2},
		{"just above half rounds up", 16, 10, 2},
		{"zero", 0, 10, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := RoundHalfUp(big.NewInt(tc.num), big.NewInt(tc.den))
			if err != nil {
				t.Fatal(err)
			}
			if got.Int64() != tc.want {
				t.Fatalf("RoundHalfUp(%d, %d) = %s, want %d", tc.num, tc.den, got, tc.want)
			}
		})
	}
}

func TestRoundingRejectsNonPositiveDivisor(t *testing.T) {
	if _, err := RoundHalfUp(big.NewInt(5), big.NewInt(0)); err == nil {
		t.Fatal("RoundHalfUp accepted a zero divisor")
	}
}
