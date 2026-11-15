package billing

import "testing"

func TestParseRateAccepts(t *testing.T) {
	for _, s := range []string{"0", "0.0", "0.10", "0.1", "0.08", "0.085", "1", "1.0", "1.000000", "0.000001", "  0.10 "} {
		if _, err := ParseRate(s); err != nil {
			t.Errorf("ParseRate(%q): %v", s, err)
		}
	}
}

func TestParseRateRefuses(t *testing.T) {
	for _, s := range []string{
		"", " ", "-0.1", "+0.1", "1.01", "1.000001", "2", "10", ".1", "0.", "1e-1", "0x1", "NaN", "Inf",
		"0,10", "10%", "0.1234567", "0.1.2", "abc",
	} {
		if r, err := ParseRate(s); err == nil {
			t.Errorf("ParseRate(%q) = %+v, want an error", s, r)
		}
	}
}

func TestDefaultVATRateIsTenPercent(t *testing.T) {
	got, _, err := WithVAT(1000, DefaultVATRate)
	if err != nil || got != 100 {
		t.Fatalf("10%% of 1000 = %d, %v; want 100", got, err)
	}
}
