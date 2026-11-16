package config

import "testing"

// PLATFORM_USAGE_DUE_DAYS (default 7) and PLATFORM_USAGE_MIN_INVOICE (default
// 10000) are the usage invoice settings of spec 7b.3; both are whole numbers of
// 1 or more.
func TestLoadUsageInvoiceSettings(t *testing.T) {
	cfg, err := load(resendEnv())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Billing.UsageDueDays != 7 || cfg.Billing.UsageMinInvoice != 10000 {
		t.Errorf("defaults = %d days, %d minimum; want 7 and 10000", cfg.Billing.UsageDueDays, cfg.Billing.UsageMinInvoice)
	}

	env := resendEnv()
	env["PLATFORM_USAGE_DUE_DAYS"] = " 14 "
	env["PLATFORM_USAGE_MIN_INVOICE"] = "50000"
	cfg, err = load(env)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Billing.UsageDueDays != 14 || cfg.Billing.UsageMinInvoice != 50000 {
		t.Errorf("got %d days, %d minimum; want 14 and 50000", cfg.Billing.UsageDueDays, cfg.Billing.UsageMinInvoice)
	}
}

func TestLoadUsageInvoiceSettingsRefuseBadNumbers(t *testing.T) {
	for _, name := range []string{"PLATFORM_USAGE_DUE_DAYS", "PLATFORM_USAGE_MIN_INVOICE"} {
		for _, raw := range []string{"abc", "1.5", "0", "-1"} {
			env := resendEnv()
			env[name] = raw
			_, err := load(env)
			if got := variables(err); len(got) != 1 || got[0] != name {
				t.Errorf("%s=%q: variables = %v, want [%s] (err %v)", name, raw, got, name, err)
			}
		}
	}
}
