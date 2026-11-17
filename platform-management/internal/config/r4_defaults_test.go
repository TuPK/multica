package config

import (
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/billing"
)

// TestEveryR4SettingEmptyBehavesAsBefore is AC61 for the configuration: with
// every R4 variable unset or empty, the platform behaves as it did before R4.
// PLATFORM_EINVOICE_PROVIDER is not read by Load yet (the e-invoice service is
// wired with a nil provider in cmd/platform-server), so empty cannot change
// anything and there is no field to assert on.
func TestEveryR4SettingEmptyBehavesAsBefore(t *testing.T) {
	env := resendEnv()
	for _, name := range []string{
		"PLATFORM_WEBHOOK_PORT", "PLATFORM_EINVOICE_PROVIDER", "PAYMENT_PROVIDERS",
		"PLATFORM_CURRENCIES", "BILLING_VAT_RATE_USD",
	} {
		env[name] = ""
	}
	cfg, err := Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("Load with every R4 setting empty: %v", err)
	}

	if got := cfg.Billing.Currencies; len(got) != 1 || got[0] != "vnd" {
		t.Errorf("Currencies = %v, want [vnd]", got)
	}
	zero, err := billing.ParseRate("0")
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Billing.VATRateOf("usd"); got != zero {
		t.Errorf("VATRateOf(usd) = %v, want 0%%", got)
	}
	if got := cfg.Payments.WebhookPort; got != "" {
		t.Errorf("WebhookPort = %q, want empty (no listener)", got)
	}
	if cfg.Payments.Registry == nil {
		t.Fatal("payment registry is nil")
	}
	if _, name, err := cfg.Payments.Registry.ForCurrency("vnd"); err != nil || name != billing.ProviderManual {
		t.Errorf("ForCurrency(vnd) = %q, %v, want %q", name, err, billing.ProviderManual)
	}
}
