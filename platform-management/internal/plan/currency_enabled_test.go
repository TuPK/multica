package plan

import "testing"

func TestPlanWithDisabledCurrencyRejected(t *testing.T) {
	enabled := []string{"vnd"}
	if err := ValidateCurrency("vnd", enabled); err != nil {
		t.Fatalf("enabled currency rejected: %v", err)
	}
	if err := ValidateCurrency("usd", enabled); err == nil {
		t.Fatal("a currency outside PLATFORM_CURRENCIES was accepted")
	}
}
