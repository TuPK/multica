package config

import "testing"

func TestPlatformCurrenciesDefaultsToVND(t *testing.T) {
	fail := func(variable, reason string) { t.Fatalf("%s: %s", variable, reason) }
	b := loadBilling(func(string) string { return "" }, fail)
	if len(b.Currencies) != 1 || b.Currencies[0] != "vnd" {
		t.Fatalf("Currencies = %v, want [vnd]", b.Currencies)
	}
}

func TestPlatformCurrenciesRejectsUnknown(t *testing.T) {
	var failed string
	fail := func(variable, reason string) { failed = variable }
	get := func(k string) string {
		if k == "PLATFORM_CURRENCIES" {
			return "vnd,xyz"
		}
		return ""
	}
	loadBilling(get, fail)
	if failed != "PLATFORM_CURRENCIES" {
		t.Fatalf("an unknown currency was accepted (failed = %q)", failed)
	}
}
