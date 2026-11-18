package config

import (
	"strings"
	"testing"
	"time"
)

const (
	einvoiceKey     = "client-id-x:client-secret-y"
	einvoiceAccount = "00000000-0000-0000-0000-000000000001"
)

func sepayEnv() map[string]string {
	env := resendEnv()
	env["PLATFORM_EINVOICE_PROVIDER"] = "sepay"
	env["PLATFORM_EINVOICE_SEPAY_API_KEY"] = einvoiceKey
	env["PLATFORM_EINVOICE_SEPAY_BASE_URL"] = "https://einvoice-api-sandbox.sepay.vn"
	env["PLATFORM_EINVOICE_SEPAY_PROVIDER_ACCOUNT_ID"] = einvoiceAccount
	env["PLATFORM_EINVOICE_START_AT"] = "2026-02-01T00:00:00+07:00"
	return env
}

func TestEinvoiceOffByDefault(t *testing.T) {
	cfg, err := load(resendEnv())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Einvoice.Provider != "" || cfg.Einvoice.MaxAttempts != 8 {
		t.Errorf("einvoice = %+v", cfg.Einvoice)
	}
}

func TestEinvoiceSepayLoads(t *testing.T) {
	env := sepayEnv()
	env["PLATFORM_EINVOICE_MAX_ATTEMPTS"] = "5"
	cfg, err := load(env)
	if err != nil {
		t.Fatal(err)
	}
	e := cfg.Einvoice
	if e.Provider != "sepay" || e.SepayAPIKey != einvoiceKey || e.SepayBaseURL != "https://einvoice-api-sandbox.sepay.vn" ||
		e.SepayProviderAccountID != einvoiceAccount || e.MaxAttempts != 5 ||
		!e.StartAt.Equal(time.Date(2026, 1, 31, 17, 0, 0, 0, time.UTC)) {
		t.Errorf("einvoice = %+v", e)
	}
	if _, err := load(func() map[string]string {
		env := sepayEnv()
		env["PLATFORM_EINVOICE_SEPAY_BASE_URL"] = "https://einvoice-api.sepay.vn"
		return env
	}()); err != nil {
		t.Errorf("production host: %v", err)
	}
}

func TestEinvoiceValidationNamesVariableNeverValue(t *testing.T) {
	cases := []struct {
		variable, value string
	}{
		{"PLATFORM_EINVOICE_PROVIDER", "other-provider"},
		{"PLATFORM_EINVOICE_SEPAY_API_KEY", ""},
		{"PLATFORM_EINVOICE_SEPAY_API_KEY", "nocolonsecret"},
		{"PLATFORM_EINVOICE_SEPAY_API_KEY", ":onlysecret"},
		{"PLATFORM_EINVOICE_SEPAY_BASE_URL", ""},
		{"PLATFORM_EINVOICE_SEPAY_BASE_URL", "http://einvoice-api.sepay.vn"},
		{"PLATFORM_EINVOICE_SEPAY_BASE_URL", "https://evil.example.com"},
		{"PLATFORM_EINVOICE_SEPAY_PROVIDER_ACCOUNT_ID", ""},
		{"PLATFORM_EINVOICE_SEPAY_PROVIDER_ACCOUNT_ID", "not-a-uuid"},
		{"PLATFORM_EINVOICE_START_AT", ""},
		{"PLATFORM_EINVOICE_START_AT", "yesterday"},
		{"PLATFORM_EINVOICE_MAX_ATTEMPTS", "0"},
		{"PLATFORM_EINVOICE_MAX_ATTEMPTS", "many"},
	}
	for _, c := range cases {
		env := sepayEnv()
		env[c.variable] = c.value
		_, err := load(env)
		if err == nil {
			t.Errorf("%s=%q: no error", c.variable, c.value)
			continue
		}
		found := false
		for _, v := range variables(err) {
			found = found || v == c.variable
		}
		if !found {
			t.Errorf("%s=%q: error %v does not name the variable", c.variable, c.value, err)
		}
		for _, secret := range []string{einvoiceKey, "other-provider", "nocolonsecret", "onlysecret", "evil.example.com", "not-a-uuid", "yesterday"} {
			if c.value != "" && strings.Contains(err.Error(), secret) {
				t.Errorf("%s: error leaks a value: %v", c.variable, err)
			}
		}
	}
}

func TestEinvoiceMaxAttemptsValidatedWhenOff(t *testing.T) {
	env := resendEnv()
	env["PLATFORM_EINVOICE_MAX_ATTEMPTS"] = "0"
	if _, err := load(env); err == nil {
		t.Error("a bad PLATFORM_EINVOICE_MAX_ATTEMPTS is accepted")
	}
}
