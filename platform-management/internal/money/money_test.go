package money

import "testing"

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
