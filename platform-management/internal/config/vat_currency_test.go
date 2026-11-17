package config

import (
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/billing"
)

func TestVatRateOfForeignCurrencyDefaultsToZero(t *testing.T) {
	get := func(string) string { return "" }
	fail := func(variable, reason string) { t.Fatalf("%s: %s", variable, reason) }
	b := loadBilling(get, fail)
	rate := b.VATRateOf("usd")
	vat, total, err := billing.WithVAT(1000, rate)
	if err != nil {
		t.Fatalf("WithVAT with the foreign-currency rate: %v", err)
	}
	if vat != 0 || total != 1000 {
		t.Fatalf("VAT of a foreign-currency order = %d, total %d; want 0 and 1000", vat, total)
	}
	if got := b.VATRateOf("vnd"); got != b.VATRate {
		t.Fatal("VATRateOf(vnd) must be BILLING_VAT_RATE")
	}
}
