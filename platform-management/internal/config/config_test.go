package config

import (
	"errors"
	"net/netip"
	"strings"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/billing"
)

const goodSecret = "0123456789abcdef0123456789abcdef" // 32 bytes

func resendEnv() map[string]string {
	return map[string]string{
		"DATABASE_URL":            "postgres://u:p@db:5432/platform?sslmode=disable",
		"REDIS_URL":               "redis://cache:6379/0",
		"PLATFORM_ADMIN_EMAILS":   "ops@example.com",
		"PLATFORM_SESSION_SECRET": goodSecret,
		"MULTICA_MFA_SECRET_KEY":  "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=",
		"RESEND_API_KEY":          "re_test",
		"RESEND_FROM_EMAIL":       "platform@example.com",

		"PLATFORM_APP_URL": "https://app.example.com",

		"BILLING_BANK_ACCOUNT_NAME":   "CONG TY TNHH MULTICA",
		"BILLING_BANK_ACCOUNT_NUMBER": "0123456789",
		"BILLING_BANK_NAME":           "Vietcombank",
	}
}

func smtpEnv() map[string]string {
	env := resendEnv()
	delete(env, "RESEND_API_KEY")
	delete(env, "RESEND_FROM_EMAIL")
	env["SMTP_HOST"] = "smtp.example.com"
	env["SMTP_FROM_EMAIL"] = "platform@example.com"
	return env
}

func load(env map[string]string) (Config, error) {
	return Load(func(k string) string { return env[k] })
}

// variables lists the variable names carried by every *Error in err.
func variables(err error) []string {
	var out []string
	var walk func(error)
	walk = func(e error) {
		if e == nil {
			return
		}
		if j, ok := e.(interface{ Unwrap() []error }); ok {
			for _, inner := range j.Unwrap() {
				walk(inner)
			}
			return
		}
		var ce *Error
		if errors.As(e, &ce) {
			out = append(out, ce.Variable)
		}
	}
	walk(err)
	return out
}

func TestLoadCompleteConfiguration(t *testing.T) {
	cfg, err := load(resendEnv())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DatabaseURL == "" || cfg.RedisURL == "" {
		t.Fatalf("urls not set: %+v", cfg)
	}
	if cfg.Email.Provider != ProviderResend || cfg.Email.ResendAPIKey != "re_test" || cfg.Email.From != "platform@example.com" {
		t.Fatalf("email = %+v", cfg.Email)
	}
	if len(cfg.AdminEmails) != 1 || cfg.AdminEmails[0] != "ops@example.com" {
		t.Fatalf("admins = %v", cfg.AdminEmails)
	}
}

func TestLoadSMTPConfiguration(t *testing.T) {
	env := smtpEnv()
	env["SMTP_USERNAME"] = "mailer"
	env["SMTP_PASSWORD"] = "pw"
	env["SMTP_EHLO_NAME"] = "platform.example.com"
	env["SMTP_TLS_INSECURE"] = "true"
	env["FRONTEND_ORIGIN"] = "https://console.example.com"
	cfg, err := load(env)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	e := cfg.Email
	if e.Provider != ProviderSMTP || e.SMTPHost != "smtp.example.com" || e.SMTPPort != "25" || e.From != "platform@example.com" {
		t.Fatalf("email = %+v", e)
	}
	if e.SMTPUsername != "mailer" || e.SMTPPassword != "pw" || e.SMTPEHLOName != "platform.example.com" || !e.SMTPTLSInsecure || e.SMTPTLSImplicit {
		t.Fatalf("email = %+v", e)
	}
	if cfg.FrontendOrigin != "https://console.example.com" {
		t.Fatalf("FrontendOrigin = %q", cfg.FrontendOrigin)
	}
}

func TestLoadSMTPWinsWhenBothProvidersAreSet(t *testing.T) {
	env := smtpEnv()
	env["RESEND_API_KEY"] = "re_test"
	env["RESEND_FROM_EMAIL"] = "resend@example.com"
	cfg, err := load(env)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Email.Provider != ProviderSMTP || cfg.Email.From != "platform@example.com" {
		t.Fatalf("email = %+v", cfg.Email)
	}
}

func TestLoadSMTPTLSMode(t *testing.T) {
	cases := []struct {
		name     string
		port     string
		mode     string
		implicit bool
		wantErr  bool
	}{
		{"default starttls", "", "", false, false},
		{"explicit starttls", "587", "starttls", false, false},
		{"implicit", "", "implicit", true, false},
		{"smtps", "", "SMTPS", true, false},
		{"ssl", "", "ssl", true, false},
		{"port 465 implies implicit", "465", "", true, false},
		{"port 465 with starttls", "465", "starttls", false, false},
		{"unknown mode", "", "plain", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := smtpEnv()
			env["SMTP_PORT"] = tc.port
			env["SMTP_TLS"] = tc.mode
			cfg, err := load(env)
			if tc.wantErr {
				if got := variables(err); len(got) != 1 || got[0] != "SMTP_TLS" {
					t.Fatalf("variables = %v, err = %v", got, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.Email.SMTPTLSImplicit != tc.implicit {
				t.Fatalf("implicit = %v, want %v", cfg.Email.SMTPTLSImplicit, tc.implicit)
			}
		})
	}
}

func TestLoadMissingRequiredVariableNamesIt(t *testing.T) {
	for _, name := range []string{
		"DATABASE_URL",
		"REDIS_URL",
		"PLATFORM_ADMIN_EMAILS",
		"PLATFORM_SESSION_SECRET",
	} {
		t.Run(name, func(t *testing.T) {
			env := resendEnv()
			delete(env, name)
			_, err := load(env)
			if err == nil {
				t.Fatal("Load succeeded without " + name)
			}
			got := variables(err)
			if len(got) != 1 || got[0] != name {
				t.Fatalf("variables = %v, want [%s]", got, name)
			}
			if !strings.Contains(err.Error(), name) {
				t.Fatalf("error text %q does not name %s", err, name)
			}
		})
	}
}

func TestLoadBlankRequiredVariableCountsAsMissing(t *testing.T) {
	env := resendEnv()
	env["DATABASE_URL"] = "   "
	_, err := load(env)
	if got := variables(err); len(got) != 1 || got[0] != "DATABASE_URL" {
		t.Fatalf("variables = %v, err = %v", got, err)
	}
}

func TestLoadMissingEmailConfigurationNamesIt(t *testing.T) {
	cases := []struct {
		name string
		edit func(env map[string]string)
		want string
	}{
		{"no provider", func(env map[string]string) {
			delete(env, "RESEND_API_KEY")
			delete(env, "RESEND_FROM_EMAIL")
		}, EmailVariable},
		{"resend key without sender", func(env map[string]string) {
			delete(env, "RESEND_FROM_EMAIL")
		}, "RESEND_FROM_EMAIL"},
		{"resend sender without key", func(env map[string]string) {
			delete(env, "RESEND_API_KEY")
		}, EmailVariable},
		{"smtp host without sender", func(env map[string]string) {
			for k, v := range smtpEnv() {
				env[k] = v
			}
			delete(env, "SMTP_FROM_EMAIL")
		}, "SMTP_FROM_EMAIL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := resendEnv()
			tc.edit(env)
			_, err := load(env)
			got := variables(err)
			if len(got) != 1 || got[0] != tc.want {
				t.Fatalf("variables = %v, want [%s], err = %v", got, tc.want, err)
			}
		})
	}
}

func TestLoadSessionSecretLength(t *testing.T) {
	env := resendEnv()
	env["PLATFORM_SESSION_SECRET"] = strings.Repeat("s", 31)
	_, err := load(env)
	if got := variables(err); len(got) != 1 || got[0] != "PLATFORM_SESSION_SECRET" {
		t.Fatalf("31 bytes: variables = %v, err = %v", got, err)
	}
	env["PLATFORM_SESSION_SECRET"] = strings.Repeat("s", 32)
	if _, err := load(env); err != nil {
		t.Fatalf("32 bytes: %v", err)
	}
	// Bytes, not runes: 16 two-byte runes make 32 bytes.
	env["PLATFORM_SESSION_SECRET"] = strings.Repeat("é", 16)
	if _, err := load(env); err != nil {
		t.Fatalf("16 two-byte runes: %v", err)
	}
}

func TestLoadErrorsDoNotContainSecretValues(t *testing.T) {
	env := resendEnv()
	env["PLATFORM_SESSION_SECRET"] = "short-secret-value"
	_, err := load(env)
	if err == nil || strings.Contains(err.Error(), "short-secret-value") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadAdminEmails(t *testing.T) {
	env := resendEnv()
	env["PLATFORM_ADMIN_EMAILS"] = " Ops@Example.com, second@example.com ,,ops@example.com"
	cfg, err := load(env)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{"ops@example.com", "second@example.com"}
	if strings.Join(cfg.AdminEmails, ",") != strings.Join(want, ",") {
		t.Fatalf("admins = %v, want %v", cfg.AdminEmails, want)
	}

	for _, bad := range []string{" , ,", "not-an-address", "ops@example.com, Ops <ops2@example.com>"} {
		env["PLATFORM_ADMIN_EMAILS"] = bad
		_, err := load(env)
		if got := variables(err); len(got) != 1 || got[0] != "PLATFORM_ADMIN_EMAILS" {
			t.Fatalf("%q: variables = %v, err = %v", bad, got, err)
		}
	}
}

func TestLoadPortDefaultsAndOverrides(t *testing.T) {
	cfg, err := load(resendEnv())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.APIPort != "8095" || cfg.ConsolePort != "30001" {
		t.Fatalf("ports = %s, %s", cfg.APIPort, cfg.ConsolePort)
	}

	env := resendEnv()
	env["PLATFORM_API_PORT"] = "9001"
	env["PLATFORM_CONSOLE_PORT"] = "9002"
	cfg, err = load(env)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.APIPort != "9001" || cfg.ConsolePort != "9002" {
		t.Fatalf("ports = %s, %s", cfg.APIPort, cfg.ConsolePort)
	}
}

func TestLoadRejectsBadPorts(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"api not a number", map[string]string{"PLATFORM_API_PORT": "http"}, "PLATFORM_API_PORT"},
		{"api out of range", map[string]string{"PLATFORM_API_PORT": "70000"}, "PLATFORM_API_PORT"},
		{"console zero", map[string]string{"PLATFORM_CONSOLE_PORT": "0"}, "PLATFORM_CONSOLE_PORT"},
		{"same port", map[string]string{"PLATFORM_API_PORT": "9000", "PLATFORM_CONSOLE_PORT": "9000"}, "PLATFORM_CONSOLE_PORT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := resendEnv()
			for k, v := range tc.env {
				env[k] = v
			}
			_, err := load(env)
			if got := variables(err); len(got) != 1 || got[0] != tc.want {
				t.Fatalf("variables = %v, want [%s], err = %v", got, tc.want, err)
			}
		})
	}
}

func TestLoadRejectsUnsupportedRedisURL(t *testing.T) {
	env := resendEnv()
	env["REDIS_URL"] = "http://cache:6379"
	_, err := load(env)
	if got := variables(err); len(got) != 1 || got[0] != "REDIS_URL" {
		t.Fatalf("variables = %v, err = %v", got, err)
	}
}

func TestLoadReportsEveryMissingVariable(t *testing.T) {
	_, err := load(map[string]string{})
	got := strings.Join(variables(err), ",")
	for _, name := range []string{"DATABASE_URL", "REDIS_URL", "PLATFORM_ADMIN_EMAILS", "PLATFORM_SESSION_SECRET", EmailVariable} {
		if !strings.Contains(got, name) {
			t.Errorf("variables %q lack %s", got, name)
		}
	}
}

func TestLoadConsoleSettingsDefaults(t *testing.T) {
	cfg, err := load(resendEnv())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.AdminCIDRs) != 0 {
		t.Errorf("AdminCIDRs = %v, want none", cfg.AdminCIDRs)
	}
	if cfg.ConsoleOrigin != "http://localhost:30001" {
		t.Errorf("ConsoleOrigin = %q", cfg.ConsoleOrigin)
	}
	if !cfg.SecureCookies {
		t.Error("SecureCookies must default to true")
	}
}

func TestLoadAdminCIDRs(t *testing.T) {
	env := resendEnv()
	env["PLATFORM_ADMIN_CIDRS"] = " 10.0.0.0/8, 2001:db8::/32 ,,192.0.2.7/32"
	cfg, err := load(env)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.AdminCIDRs) != 3 {
		t.Fatalf("AdminCIDRs = %v", cfg.AdminCIDRs)
	}
	if !cfg.AdminCIDRs[0].Contains(netip.MustParseAddr("10.1.2.3")) {
		t.Error("10.0.0.0/8 should contain 10.1.2.3")
	}
}

func TestLoadInvalidAdminCIDRStopsTheStart(t *testing.T) {
	for _, raw := range []string{"10.0.0.0/33", "not-a-cidr", "10.0.0.1", "10.0.0.0/8,bad"} {
		env := resendEnv()
		env["PLATFORM_ADMIN_CIDRS"] = raw
		_, err := load(env)
		if err == nil {
			t.Errorf("PLATFORM_ADMIN_CIDRS=%q: no error", raw)
			continue
		}
		if vars := variables(err); len(vars) != 1 || vars[0] != "PLATFORM_ADMIN_CIDRS" {
			t.Errorf("PLATFORM_ADMIN_CIDRS=%q: variables = %v", raw, vars)
		}
	}
}

func TestLoadConsoleOrigin(t *testing.T) {
	good := map[string]string{
		"https://console.example.com":  "https://console.example.com",
		"https://Console.Example.com/": "https://console.example.com",
		"http://localhost:30001":       "http://localhost:30001",
		" http://127.0.0.1:8080 ":      "http://127.0.0.1:8080",
	}
	for raw, want := range good {
		env := resendEnv()
		env["PLATFORM_CONSOLE_ORIGIN"] = raw
		cfg, err := load(env)
		if err != nil {
			t.Errorf("origin %q: %v", raw, err)
			continue
		}
		if cfg.ConsoleOrigin != want {
			t.Errorf("origin %q: got %q, want %q", raw, cfg.ConsoleOrigin, want)
		}
	}
}

func TestLoadBadConsoleOriginStopsTheStart(t *testing.T) {
	for _, raw := range []string{
		"ftp://console.example.com",
		"javascript:alert(1)",
		"console.example.com",
		"//console.example.com",
		"https://",
		"https://console.example.com/path",
		"https://console.example.com?x=1",
		"https://user:pw@console.example.com",
	} {
		env := resendEnv()
		env["PLATFORM_CONSOLE_ORIGIN"] = raw
		_, err := load(env)
		if err == nil {
			t.Errorf("PLATFORM_CONSOLE_ORIGIN=%q: no error", raw)
			continue
		}
		if vars := variables(err); len(vars) != 1 || vars[0] != "PLATFORM_CONSOLE_ORIGIN" {
			t.Errorf("PLATFORM_CONSOLE_ORIGIN=%q: variables = %v", raw, vars)
		}
		if strings.Contains(err.Error(), "pw@") {
			t.Errorf("error repeats the credentials: %v", err)
		}
	}
}

func TestLoadSecureCookies(t *testing.T) {
	for raw, want := range map[string]bool{"true": true, "false": false, " FALSE ": false} {
		env := resendEnv()
		env["PLATFORM_SECURE_COOKIES"] = raw
		cfg, err := load(env)
		if err != nil {
			t.Errorf("PLATFORM_SECURE_COOKIES=%q: %v", raw, err)
			continue
		}
		if cfg.SecureCookies != want {
			t.Errorf("PLATFORM_SECURE_COOKIES=%q: got %v", raw, cfg.SecureCookies)
		}
	}
	env := resendEnv()
	env["PLATFORM_SECURE_COOKIES"] = "maybe"
	_, err := load(env)
	if vars := variables(err); len(vars) != 1 || vars[0] != "PLATFORM_SECURE_COOKIES" {
		t.Errorf("variables = %v (err %v)", vars, err)
	}
}

func TestLoadTrustedCIDRsDefaultToLoopbackAndPrivateRanges(t *testing.T) {
	cfg, err := load(resendEnv())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, addr := range []string{"127.0.0.1", "::1", "10.2.3.4", "172.20.0.1", "192.168.5.5"} {
		if !containsAddr(cfg.TrustedCIDRs, addr) {
			t.Errorf("default TrustedCIDRs does not contain %s", addr)
		}
	}
	for _, addr := range []string{"203.0.113.9", "172.32.0.1", "8.8.8.8"} {
		if containsAddr(cfg.TrustedCIDRs, addr) {
			t.Errorf("default TrustedCIDRs contains %s", addr)
		}
	}
}

func containsAddr(cidrs []netip.Prefix, addr string) bool {
	a := netip.MustParseAddr(addr)
	for _, c := range cidrs {
		if c.Contains(a) {
			return true
		}
	}
	return false
}

// A configured list replaces the defaults: it does not add to them.
func TestLoadTrustedCIDRsReplaceTheDefaults(t *testing.T) {
	env := resendEnv()
	env["PLATFORM_TRUSTED_CIDRS"] = " 10.9.0.0/16 ,,203.0.113.0/24"
	cfg, err := load(env)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.TrustedCIDRs) != 2 || !containsAddr(cfg.TrustedCIDRs, "203.0.113.9") || containsAddr(cfg.TrustedCIDRs, "127.0.0.1") {
		t.Fatalf("TrustedCIDRs = %v", cfg.TrustedCIDRs)
	}
}

func TestLoadInvalidTrustedCIDRStopsTheStart(t *testing.T) {
	for _, raw := range []string{"10.0.0.0/33", "not-a-cidr", "10.0.0.1", "10.0.0.0/8,bad", ",,"} {
		env := resendEnv()
		env["PLATFORM_TRUSTED_CIDRS"] = raw
		_, err := load(env)
		if err == nil {
			t.Errorf("PLATFORM_TRUSTED_CIDRS=%q: no error", raw)
			continue
		}
		if vars := variables(err); len(vars) != 1 || vars[0] != "PLATFORM_TRUSTED_CIDRS" {
			t.Errorf("PLATFORM_TRUSTED_CIDRS=%q: variables = %v", raw, vars)
		}
	}
}

func TestBillingDefaults(t *testing.T) {
	cfg, err := load(resendEnv())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, _, err := billing.WithVAT(1000, cfg.Billing.VATRate); err != nil || got != 100 {
		t.Fatalf("default VAT on 1000 = %d, %v; want 100 (10%%)", got, err)
	}
	want := billing.BankDetails{AccountName: "CONG TY TNHH MULTICA", AccountNumber: "0123456789", BankName: "Vietcombank"}
	if cfg.Billing.Bank != want {
		t.Fatalf("bank = %+v, want %+v", cfg.Billing.Bank, want)
	}
}

func TestBillingVATRateIsRead(t *testing.T) {
	env := resendEnv()
	env["BILLING_VAT_RATE"] = " 0.08 "
	cfg, err := load(env)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, _, _ := billing.WithVAT(1000, cfg.Billing.VATRate); got != 80 {
		t.Fatalf("VAT on 1000 at 0.08 = %d, want 80", got)
	}
	env["BILLING_VAT_RATE"] = "0"
	cfg, err = load(env)
	if err != nil {
		t.Fatalf("Load with rate 0: %v", err)
	}
	if got, _, _ := billing.WithVAT(1000, cfg.Billing.VATRate); got != 0 {
		t.Fatalf("VAT at rate 0 = %d, want 0", got)
	}
}

func TestBusinessDiscountRequiresVerificationByDefault(t *testing.T) {
	cfg, err := load(resendEnv())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Billing.BusinessDiscountRequiresVerification {
		t.Fatal("the Business discount requires verification unless the flag says otherwise (spec D45)")
	}
	env := resendEnv()
	env["PLATFORM_BUSINESS_DISCOUNT_REQUIRES_VERIFICATION"] = "false"
	cfg, err = load(env)
	if err != nil {
		t.Fatalf("Load with the flag off: %v", err)
	}
	if cfg.Billing.BusinessDiscountRequiresVerification {
		t.Fatal("the flag is false but the config says it is on")
	}
}

func TestBusinessDiscountFlagRefusesOtherValues(t *testing.T) {
	for _, bad := range []string{"no", "0", "TRUE"} {
		env := resendEnv()
		env["PLATFORM_BUSINESS_DISCOUNT_REQUIRES_VERIFICATION"] = bad
		_, err := load(env)
		if err == nil {
			t.Errorf("PLATFORM_BUSINESS_DISCOUNT_REQUIRES_VERIFICATION=%q: Load succeeded", bad)
			continue
		}
		if vs := variables(err); len(vs) != 1 || vs[0] != "PLATFORM_BUSINESS_DISCOUNT_REQUIRES_VERIFICATION" {
			t.Errorf("value %q: variables = %v", bad, vs)
		}
	}
}

func TestInvalidVATRateStopsTheStart(t *testing.T) {
	for _, bad := range []string{"abc", "-0.1", "1.5", "10%", "0.1234567", "1e-1"} {
		env := resendEnv()
		env["BILLING_VAT_RATE"] = bad
		_, err := load(env)
		if err == nil {
			t.Errorf("BILLING_VAT_RATE=%q: Load succeeded", bad)
			continue
		}
		if vs := variables(err); len(vs) != 1 || vs[0] != "BILLING_VAT_RATE" {
			t.Errorf("BILLING_VAT_RATE=%q: variables = %v", bad, vs)
		}
		if strings.Contains(err.Error(), bad) {
			t.Errorf("error repeats the value %q: %v", bad, err)
		}
	}
}

func TestEmptyBankDetailsStartAndAreReported(t *testing.T) {
	names := []string{"BILLING_BANK_ACCOUNT_NAME", "BILLING_BANK_ACCOUNT_NUMBER", "BILLING_BANK_NAME"}
	for _, name := range names {
		for _, value := range []string{"", "   "} {
			env := resendEnv()
			env[name] = value
			cfg, err := load(env)
			if err != nil {
				t.Errorf("%s=%q: Load: %v", name, value, err)
				continue
			}
			if got := cfg.Billing.EmptyBankVariables(); len(got) != 1 || got[0] != name {
				t.Errorf("%s=%q: EmptyBankVariables = %v", name, value, got)
			}
		}
	}
	cfg, err := load(resendEnv())
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Billing.EmptyBankVariables(); len(got) != 0 {
		t.Errorf("complete bank details reported empty: %v", got)
	}
}

func TestEmptyBankDetailsGiveEmptyManualInstructions(t *testing.T) {
	env := resendEnv()
	for _, name := range []string{"BILLING_BANK_ACCOUNT_NAME", "BILLING_BANK_ACCOUNT_NUMBER", "BILLING_BANK_NAME"} {
		delete(env, name)
	}
	cfg, err := load(env)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Billing.Bank != (billing.BankDetails{}) {
		t.Fatalf("bank = %+v, want empty", cfg.Billing.Bank)
	}
	got, err := billing.NewManualProvider(cfg.Billing.Bank).CreatePaymentRequest(billing.Order{TransferCode: "MULABCDEFGH", Currency: "vnd"})
	if err != nil {
		t.Fatalf("CreatePaymentRequest: %v", err)
	}
	if got.BankAccountName != "" || got.BankAccountNumber != "" || got.BankName != "" {
		t.Fatalf("instructions = %+v", got)
	}
}

func TestDevVerificationCodeMustBeEmptyOrSixDigits(t *testing.T) {
	for _, bad := range []string{"12345", "1234567", "12345a", "abcdef", "123 56", "１２３４５６", "12345\n6"} {
		env := resendEnv()
		env["PLATFORM_DEV_VERIFICATION_CODE"] = bad
		_, err := load(env)
		if err == nil {
			t.Errorf("code %q: no error", bad)
			continue
		}
		if vs := variables(err); len(vs) != 1 || vs[0] != "PLATFORM_DEV_VERIFICATION_CODE" {
			t.Errorf("code %q: variables = %v", bad, vs)
		}
		if strings.Contains(err.Error(), bad) {
			t.Errorf("error repeats the value %q: %v", bad, err)
		}
	}
	for _, good := range []string{"", "666666", "000000"} {
		env := resendEnv()
		env["PLATFORM_DEV_VERIFICATION_CODE"] = good
		cfg, err := load(env)
		if err != nil {
			t.Errorf("code %q: %v", good, err)
		} else if cfg.DevVerificationCode != good {
			t.Errorf("code %q: DevVerificationCode = %q", good, cfg.DevVerificationCode)
		}
	}
}

func TestDevVerificationCodeIsRefusedInProduction(t *testing.T) {
	for _, appEnv := range []string{"production", "Production", "PRODUCTION", " production "} {
		env := resendEnv()
		env["APP_ENV"] = appEnv
		env["PLATFORM_DEV_VERIFICATION_CODE"] = "666666"
		_, err := load(env)
		if err == nil {
			t.Errorf("APP_ENV=%q: no error", appEnv)
			continue
		}
		if vs := variables(err); len(vs) != 1 || vs[0] != "PLATFORM_DEV_VERIFICATION_CODE" {
			t.Errorf("APP_ENV=%q: variables = %v", appEnv, vs)
		}
		if strings.Contains(err.Error(), "666666") {
			t.Errorf("error repeats the code: %v", err)
		}
	}
	for _, appEnv := range []string{"development", "", "staging"} {
		env := resendEnv()
		env["APP_ENV"] = appEnv
		env["PLATFORM_DEV_VERIFICATION_CODE"] = "666666"
		if _, err := load(env); err != nil {
			t.Errorf("APP_ENV=%q: %v", appEnv, err)
		}
	}
	env := resendEnv()
	env["APP_ENV"] = "production"
	if _, err := load(env); err != nil {
		t.Errorf("production without a dev code: %v", err)
	}
}

func TestEmailProviderIsOptionalOnlyWithADevCode(t *testing.T) {
	env := resendEnv()
	delete(env, "RESEND_API_KEY")
	delete(env, "RESEND_FROM_EMAIL")
	_, err := load(env)
	if vs := variables(err); len(vs) != 1 || vs[0] != EmailVariable {
		t.Fatalf("no dev code, no provider: err = %v", err)
	}

	env["PLATFORM_DEV_VERIFICATION_CODE"] = "666666"
	cfg, err := load(env)
	if err != nil {
		t.Fatalf("dev code, no provider: %v", err)
	}
	if cfg.Email != (Email{}) {
		t.Errorf("email = %+v, want empty", cfg.Email)
	}

	// A half-configured provider is still an error.
	env["RESEND_API_KEY"] = "re_test"
	if _, err := load(env); err == nil {
		t.Error("RESEND_API_KEY without RESEND_FROM_EMAIL started")
	}
}

func TestLoadAppURL(t *testing.T) {
	good := map[string]string{
		"https://app.example.com":  "https://app.example.com",
		"https://App.Example.com/": "https://app.example.com",
		"http://localhost:3000":    "http://localhost:3000",
		" http://127.0.0.1:3000/ ": "http://127.0.0.1:3000",
	}
	for raw, want := range good {
		env := resendEnv()
		env["PLATFORM_APP_URL"] = raw
		cfg, err := load(env)
		if err != nil {
			t.Errorf("PLATFORM_APP_URL=%q: %v", raw, err)
			continue
		}
		if cfg.AppURL != want {
			t.Errorf("PLATFORM_APP_URL=%q: AppURL = %q, want %q", raw, cfg.AppURL, want)
		}
	}
}

func TestBadOrMissingAppURLStopsTheStart(t *testing.T) {
	for _, raw := range []string{
		"",
		"   ",
		"ftp://app.example.com",
		"javascript:alert(1)",
		"app.example.com",
		"//app.example.com",
		"https://",
		"https://app.example.com/path",
		"https://app.example.com?x=1",
		"https://app.example.com#top",
		"https://user:pw@app.example.com",
	} {
		env := resendEnv()
		env["PLATFORM_APP_URL"] = raw
		_, err := load(env)
		if err == nil {
			t.Errorf("PLATFORM_APP_URL=%q: no error", raw)
			continue
		}
		if vars := variables(err); len(vars) != 1 || vars[0] != "PLATFORM_APP_URL" {
			t.Errorf("PLATFORM_APP_URL=%q: variables = %v", raw, vars)
		}
		if strings.Contains(err.Error(), "pw@") {
			t.Errorf("error repeats the credentials: %v", err)
		}
	}
}

func TestMFASecretKey(t *testing.T) {
	t.Run("loaded as bytes", func(t *testing.T) {
		cfg, err := load(resendEnv())
		if err != nil {
			t.Fatal(err)
		}
		if len(cfg.MFASecretKey) != 32 {
			t.Fatalf("key length = %d, want 32", len(cfg.MFASecretKey))
		}
	})
	for name, value := range map[string]string{
		"missing":    "",
		"not base64": "%%%not-base64%%%",
		"too short":  "c2hvcnQ=",
	} {
		t.Run(name, func(t *testing.T) {
			env := resendEnv()
			env["MULTICA_MFA_SECRET_KEY"] = value
			_, err := load(env)
			if err == nil {
				t.Fatal("expected an error")
			}
			if got := variables(err); len(got) != 1 || got[0] != "MULTICA_MFA_SECRET_KEY" {
				t.Fatalf("variables = %v", got)
			}
			if value != "" && strings.Contains(err.Error(), value) {
				t.Fatalf("error repeats the value: %v", err)
			}
		})
	}
}

// PLATFORM_API_RATE_CEILING is the API rate a limited plan gets when it leaves
// the rate empty (spec 7b.3 rule 2). Default 6000; a whole number of 1 or more.
func TestLoadAPIRateCeiling(t *testing.T) {
	cfg, err := load(resendEnv())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIRateCeiling != 6000 {
		t.Errorf("default APIRateCeiling = %d, want 6000", cfg.APIRateCeiling)
	}

	env := resendEnv()
	env["PLATFORM_API_RATE_CEILING"] = " 4500 "
	cfg, err = load(env)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIRateCeiling != 4500 {
		t.Errorf("APIRateCeiling = %d, want 4500", cfg.APIRateCeiling)
	}
}

func TestLoadAPIRateCeilingRefusesANonWholeNumber(t *testing.T) {
	for _, raw := range []string{"abc", "1.5", "6k", "99999999999999999999999"} {
		env := resendEnv()
		env["PLATFORM_API_RATE_CEILING"] = raw
		_, err := load(env)
		if got := variables(err); len(got) != 1 || got[0] != "PLATFORM_API_RATE_CEILING" {
			t.Errorf("%q: variables = %v, want [PLATFORM_API_RATE_CEILING] (err %v)", raw, got, err)
		}
	}
}

func TestLoadAPIRateCeilingRefusesBelowOne(t *testing.T) {
	for _, raw := range []string{"0", "-1"} {
		env := resendEnv()
		env["PLATFORM_API_RATE_CEILING"] = raw
		_, err := load(env)
		if got := variables(err); len(got) != 1 || got[0] != "PLATFORM_API_RATE_CEILING" {
			t.Errorf("%q: variables = %v, want [PLATFORM_API_RATE_CEILING] (err %v)", raw, got, err)
		}
	}
}

func TestPriceSettingsDefaults(t *testing.T) {
	cfg, err := load(resendEnv())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Price; got.NoticeDays != 30 || got.ConfirmPct != 50 || got.Max != 1_000_000_000 {
		t.Fatalf("price settings = %+v, want 30 days, 50 percent, 1000000000", got)
	}
}

func TestPriceSettingsAreRead(t *testing.T) {
	env := resendEnv()
	env["PLATFORM_PRICE_NOTICE_DAYS"] = " 45 "
	env["PLATFORM_PRICE_CONFIRM_PCT"] = "100"
	env["PLATFORM_PRICE_MAX"] = "5000000"
	cfg, err := load(env)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Price; got.NoticeDays != 45 || got.ConfirmPct != 100 || got.Max != 5_000_000 {
		t.Fatalf("price settings = %+v", got)
	}
}

func TestInvalidPriceSettingsStopTheStart(t *testing.T) {
	for _, name := range []string{"PLATFORM_PRICE_NOTICE_DAYS", "PLATFORM_PRICE_CONFIRM_PCT", "PLATFORM_PRICE_MAX"} {
		for _, raw := range []string{"abc", "1.5", "0", "-1"} {
			env := resendEnv()
			env[name] = raw
			_, err := load(env)
			if vars := variables(err); len(vars) != 1 || vars[0] != name {
				t.Errorf("%s=%q: variables = %v, want [%s]", name, raw, vars, name)
			}
		}
	}
	env := resendEnv()
	env["PLATFORM_PRICE_CONFIRM_PCT"] = "101"
	if vars := variables(func() error { _, err := load(env); return err }()); len(vars) != 1 || vars[0] != "PLATFORM_PRICE_CONFIRM_PCT" {
		t.Errorf("CONFIRM_PCT=101: variables = %v", vars)
	}
}
