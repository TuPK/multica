package billing

import (
	"math"
	"testing"
)

func mustRate(t *testing.T, s string) Rate {
	t.Helper()
	r, err := ParseRate(s)
	if err != nil {
		t.Fatalf("ParseRate(%q): %v", s, err)
	}
	return r
}

// Expected values are computed by hand: vat = floor((2*amount*num + den) / (2*den)).
func TestWithVATRoundsHalfUp(t *testing.T) {
	cases := []struct {
		name       string
		amount     int64
		rate       string
		vat, total int64
	}{
		{"exact", 1000, "0.10", 100, 1100},
		{"below half: 4 * 0.1 = 0.4", 4, "0.10", 0, 4},
		{"exactly half: 5 * 0.1 = 0.5 rounds up", 5, "0.10", 1, 6},
		{"above half: 6 * 0.1 = 0.6", 6, "0.10", 1, 7},
		{"149000 seats at 10%", 447000, "0.10", 44700, 491700},
		{"half at 8%: 6.25 is not whole, 13 * 0.08 = 1.04", 13, "0.08", 1, 14},
		{"half at 2%: 25 * 0.02 = 0.5 rounds up", 25, "0.02", 1, 26},
		{"just under half at 2%: 24 * 0.02 = 0.48", 24, "0.02", 0, 24},
		{"8%: 6 * 0.08 = 0.48", 6, "0.08", 0, 6},
		{"8%: 25 * 0.08 = 2 exactly", 25, "0.08", 2, 27},
		{"half at 0.5: 1 * 0.5 = 0.5 rounds up", 1, "0.5", 1, 2},
		{"3 * 0.5 = 1.5 rounds up to 2", 3, "0.5", 2, 5},
		{"rate 0", 123456, "0", 0, 123456},
		{"rate 1", 123456, "1", 123456, 246912},
		{"zero amount", 0, "0.10", 0, 0},
		{"six-decimal rate: 500000 * 0.000001 = 0.5", 500000, "0.000001", 1, 500001},
		{"six-decimal rate: 499999 * 0.000001 = 0.499999", 499999, "0.000001", 0, 499999},
	}
	for _, c := range cases {
		vat, total, err := WithVAT(c.amount, mustRate(t, c.rate))
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if vat != c.vat || total != c.total {
			t.Errorf("%s: WithVAT(%d, %s) = %d, %d; want %d, %d", c.name, c.amount, c.rate, vat, total, c.vat, c.total)
		}
	}
}

func TestWithVATRefuses(t *testing.T) {
	if _, _, err := WithVAT(-1, DefaultVATRate); err == nil {
		t.Error("negative amount accepted")
	}
	if _, _, err := WithVAT(100, Rate{}); err == nil {
		t.Error("unset rate accepted")
	}
	if _, _, err := WithVAT(math.MaxInt64, DefaultVATRate); err == nil {
		t.Error("a total above int64 was accepted")
	}
}

func TestWithVATLargeAmountDoesNotOverflowInTheMiddle(t *testing.T) {
	// 8e18 * 0.1 = 8e17 exactly; the doubled numerator 8e18 * 2 exceeds int64 if done naively.
	vat, total, err := WithVAT(8_000_000_000_000_000_000, mustRate(t, "0.10"))
	if err != nil {
		t.Fatal(err)
	}
	if vat != 800_000_000_000_000_000 || total != 8_800_000_000_000_000_000 {
		t.Fatalf("vat, total = %d, %d", vat, total)
	}
}
