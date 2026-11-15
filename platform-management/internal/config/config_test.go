package config

import (
	"errors"
	"net/netip"
	"strings"
	"testing"
)

const goodSecret = "0123456789abcdef0123456789abcdef" // 32 bytes

func resendEnv() map[string]string {
	return map[string]string{
		"DATABASE_URL":            "postgres://u:p@db:5432/platform?sslmode=disable",
		"REDIS_URL":               "redis://cache:6379/0",
		"PLATFORM_ADMIN_EMAILS":   "ops@example.com",
		"PLATFORM_SESSION_SECRET": goodSecret,
		"RESEND_API_KEY":          "re_test",
		"RESEND_FROM_EMAIL":       "platform@example.com",
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
