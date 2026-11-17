// Package config reads and validates the platform service's environment.
package config

import (
	"errors"
	"net/mail"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/mfa"
	"github.com/multica-ai/multica/platform-management/internal/money"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/trust"
)

const (
	defaultAPIPort     = "8095"
	defaultConsolePort = "30001"
	defaultSMTPPort    = "25"

	defaultConsoleOrigin = "http://localhost:30001"

	minSessionSecretBytes = 32

	// EmailVariable names the email requirement in an Error when no provider
	// is configured: either of the two variables starts a provider.
	EmailVariable = "RESEND_API_KEY or SMTP_HOST"
)

// Email providers. SMTP wins when both are configured, as in the Multica
// server's email service.
const (
	ProviderResend = "resend"
	ProviderSMTP   = "smtp"
)

// Error is the one error type Load returns. Variable names the environment
// variable at fault; Reason never contains the variable's value.
type Error struct {
	Variable string
	Reason   string
}

func (e *Error) Error() string { return e.Variable + ": " + e.Reason }

// Email holds the settings for the provider that was configured. The sender
// itself is not part of this package.
type Email struct {
	Provider string
	// From is the sender address: RESEND_FROM_EMAIL for Resend,
	// SMTP_FROM_EMAIL for SMTP.
	From string

	ResendAPIKey string

	SMTPHost        string
	SMTPPort        string
	SMTPUsername    string
	SMTPPassword    string
	SMTPTLSImplicit bool
	SMTPTLSInsecure bool
	// SMTPEHLOName is the raw SMTP_EHLO_NAME; the sender chooses the fallback.
	SMTPEHLOName string
}

// Billing holds the order settings.
type Billing struct {
	// VATRate is BILLING_VAT_RATE; 0.10 when it is not set.
	VATRate billing.Rate
	// BusinessDiscountRequiresVerification is PLATFORM_BUSINESS_DISCOUNT_REQUIRES_VERIFICATION;
	// true when it is not set. With it on, the Business discount on usage
	// charges applies only to a verified billing profile (spec D45).
	BusinessDiscountRequiresVerification bool
	// UsageDueDays is PLATFORM_USAGE_DUE_DAYS: the days a usage invoice may stay
	// unpaid before the subscription goes past_due; 7 when it is not set.
	UsageDueDays int
	// UsageMinInvoice is PLATFORM_USAGE_MIN_INVOICE: the smallest monthly usage
	// invoice, in the currency's smallest unit; 10000 when it is not set. Less
	// carries over to the next month.
	UsageMinInvoice int64
	// Bank is where the manual provider tells customers to transfer.
	Bank billing.BankDetails
}

// VATRateOf returns the VAT rate for a currency. Only the default currency has
// BILLING_VAT_RATE; any other currency is 0 (spec 7.4 rule 4). No
// BILLING_VAT_RATE_<CURRENCY> variable is read yet, as vnd is the only enabled currency.
func (b Billing) VATRateOf(currency string) billing.Rate {
	if currency == money.Default {
		return b.VATRate
	}
	zero, _ := billing.ParseRate("0") // constant input, cannot fail
	return zero
}

// EmptyBankVariables names the BILLING_BANK_* variables that are empty.
func (b Billing) EmptyBankVariables() []string {
	var out []string
	for _, f := range []struct{ name, value string }{
		{"BILLING_BANK_ACCOUNT_NAME", b.Bank.AccountName},
		{"BILLING_BANK_ACCOUNT_NUMBER", b.Bank.AccountNumber},
		{"BILLING_BANK_NAME", b.Bank.BankName},
	} {
		if f.value == "" {
			out = append(out, f.name)
		}
	}
	return out
}

// Price holds the limits of a plan price change (spec 7b.4 rules 4, 5 and 7).
type Price struct {
	// NoticeDays is PLATFORM_PRICE_NOTICE_DAYS: the days of notice before a
	// price increase takes effect for paid subscribers; 30 when it is not set.
	NoticeDays int
	// ConfirmPct is PLATFORM_PRICE_CONFIRM_PCT: the change, in percent, above
	// which the admin must confirm; 50 when it is not set.
	ConfirmPct int
	// Max is PLATFORM_PRICE_MAX: the largest price of one unit; 1000000000
	// when it is not set.
	Max int64
}

// Payments holds the payment provider settings (spec 7.2).
type Payments struct {
	// Registry maps each currency to its provider, from PAYMENT_PROVIDERS and
	// PAYMENT_WEBHOOK_SECRET_<NAME>.
	Registry *billing.Registry
	// WebhookPort is PLATFORM_WEBHOOK_PORT; empty means the webhook listener is off.
	WebhookPort string
	// WebhookMaxBytes is PAYMENT_WEBHOOK_MAX_BYTES: the body limit of a webhook
	// notice in bytes; 65536 when it is not set.
	WebhookMaxBytes int64
}

type Config struct {
	Billing       Billing
	Payments      Payments
	Price         Price
	DatabaseURL   string
	RedisURL      string
	AdminEmails   []string
	SessionSecret string
	// MFASecretKey is MULTICA_MFA_SECRET_KEY decoded: the key Multica seals
	// TOTP secrets with, shared so the console can check staff codes.
	MFASecretKey []byte
	APIPort      string
	ConsolePort  string
	Email        Email
	// DevVerificationCode is PLATFORM_DEV_VERIFICATION_CODE: empty, or six
	// digits that staff sign in with in development. Never set in production.
	DevVerificationCode string
	FrontendOrigin      string
	// AppURL is the origin of the Multica web app, as scheme://host[:port],
	// lower case and without a trailing slash. The billing routes build the
	// address of an order page from it.
	AppURL string

	// AdminCIDRs restricts the console to these networks. Empty means no
	// restriction.
	AdminCIDRs []netip.Prefix
	// TrustedCIDRs are the networks from which the API believes X-User-ID. The
	// default is the loopback and RFC 1918 private ranges.
	TrustedCIDRs []netip.Prefix
	// ConsoleOrigin is the console's origin as scheme://host[:port], lower
	// case and without a trailing slash.
	ConsoleOrigin string
	// SecureCookies sets the Secure attribute on the session cookie.
	SecureCookies bool
	// APIRateCeiling is PLATFORM_API_RATE_CEILING: the API rate per minute a
	// limited plan gets when it leaves the rate empty (spec 7b.3 rule 2).
	APIRateCeiling int64
	// AccountCreatePerHour is PLATFORM_ACCOUNT_CREATE_PER_HOUR: the account
	// requests one staff member may create in an hour (spec 7b.5 rule 1).
	AccountCreatePerHour int
	// IndividualMaxSeats is PLATFORM_INDIVIDUAL_MAX_SEATS: the most seats an
	// individual account may be created with (spec 7b.1, 7b.5 rule 6).
	IndividualMaxSeats int
}

// Load reads the configuration through getenv and reports every problem it
// finds, each as an *Error inside the returned error.
func Load(getenv func(string) string) (Config, error) {
	var errs []error
	fail := func(variable, reason string) {
		errs = append(errs, &Error{Variable: variable, Reason: reason})
	}
	get := func(name string) string { return strings.TrimSpace(getenv(name)) }
	required := func(name string) string {
		v := get(name)
		if v == "" {
			fail(name, "is required")
		}
		return v
	}

	var cfg Config
	cfg.DatabaseURL = required("DATABASE_URL")

	cfg.RedisURL = required("REDIS_URL")
	if cfg.RedisURL != "" {
		if u, err := url.Parse(cfg.RedisURL); err != nil || (u.Scheme != "redis" && u.Scheme != "rediss" && u.Scheme != "unix") {
			fail("REDIS_URL", "must be a redis://, rediss:// or unix:// URL")
		}
	}

	if raw := required("PLATFORM_ADMIN_EMAILS"); raw != "" {
		admins, err := parseAdminEmails(raw)
		if err != nil {
			fail("PLATFORM_ADMIN_EMAILS", err.Error())
		}
		cfg.AdminEmails = admins
	}

	// The secret is read untrimmed: its length is counted in bytes as given.
	cfg.SessionSecret = getenv("PLATFORM_SESSION_SECRET")
	switch {
	case strings.TrimSpace(cfg.SessionSecret) == "":
		fail("PLATFORM_SESSION_SECRET", "is required")
	case len(cfg.SessionSecret) < minSessionSecretBytes:
		fail("PLATFORM_SESSION_SECRET", "must be at least "+strconv.Itoa(minSessionSecretBytes)+" bytes")
	}

	// The same variable and value as the Multica server's, so the console can
	// open the TOTP secrets that Multica sealed.
	if raw := get(mfa.EnvKey); raw == "" {
		fail(mfa.EnvKey, "is required")
	} else if key, err := mfa.ParseKey(raw); err != nil {
		fail(mfa.EnvKey, err.Error())
	} else {
		cfg.MFASecretKey = key
	}

	cfg.APIPort = portOrDefault(get("PLATFORM_API_PORT"), defaultAPIPort)
	cfg.ConsolePort = portOrDefault(get("PLATFORM_CONSOLE_PORT"), defaultConsolePort)
	apiOK := validPort(cfg.APIPort)
	consoleOK := validPort(cfg.ConsolePort)
	if !apiOK {
		fail("PLATFORM_API_PORT", "must be a port number from 1 to 65535")
	}
	if !consoleOK {
		fail("PLATFORM_CONSOLE_PORT", "must be a port number from 1 to 65535")
	}
	if apiOK && consoleOK && cfg.APIPort == cfg.ConsolePort {
		fail("PLATFORM_CONSOLE_PORT", "must differ from PLATFORM_API_PORT")
	}

	cfg.DevVerificationCode = loadDevVerificationCode(get, fail)
	cfg.Email = loadEmail(get, getenv, fail, cfg.DevVerificationCode != "")
	cfg.FrontendOrigin = get("FRONTEND_ORIGIN")

	if raw := get("PLATFORM_ADMIN_CIDRS"); raw != "" {
		cidrs, err := parseCIDRs(raw)
		if err != nil {
			fail("PLATFORM_ADMIN_CIDRS", err.Error())
		}
		cfg.AdminCIDRs = cidrs
	}

	cfg.TrustedCIDRs = trust.DefaultCIDRs()
	if raw := get("PLATFORM_TRUSTED_CIDRS"); raw != "" {
		cidrs, err := parseCIDRs(raw)
		if err != nil {
			fail("PLATFORM_TRUSTED_CIDRS", err.Error())
		}
		cfg.TrustedCIDRs = cidrs
	}

	origin := get("PLATFORM_CONSOLE_ORIGIN")
	if origin == "" {
		origin = defaultConsoleOrigin
	}
	if normalized, ok := normalizeOrigin(origin); ok {
		cfg.ConsoleOrigin = normalized
	} else {
		fail("PLATFORM_CONSOLE_ORIGIN", "must be an absolute http or https origin such as https://console.example.com")
	}

	cfg.SecureCookies = true
	switch strings.ToLower(get("PLATFORM_SECURE_COOKIES")) {
	case "", "true":
	case "false":
		cfg.SecureCookies = false
	default:
		fail("PLATFORM_SECURE_COOKIES", "must be true or false")
	}

	if raw := required("PLATFORM_APP_URL"); raw != "" {
		if normalized, ok := normalizeOrigin(raw); ok {
			cfg.AppURL = normalized
		} else {
			fail("PLATFORM_APP_URL", "must be an absolute http or https origin such as https://app.example.com")
		}
	}

	cfg.Billing = loadBilling(get, fail)
	cfg.Payments = loadPayments(get, getenv, fail, cfg.Billing.Bank, cfg.APIPort, cfg.ConsolePort)

	cfg.APIRateCeiling = plan.DefaultAPIRateCeiling
	if raw := get("PLATFORM_API_RATE_CEILING"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 1 {
			fail("PLATFORM_API_RATE_CEILING", "must be a whole number, 1 or more")
		} else {
			cfg.APIRateCeiling = n
		}
	}

	cfg.AccountCreatePerHour = wholeNumber(get, fail, "PLATFORM_ACCOUNT_CREATE_PER_HOUR", 30)
	cfg.IndividualMaxSeats = wholeNumber(get, fail, "PLATFORM_INDIVIDUAL_MAX_SEATS", 5)

	cfg.Price = Price{
		NoticeDays: wholeNumber(get, fail, "PLATFORM_PRICE_NOTICE_DAYS", 30),
		ConfirmPct: wholeNumber(get, fail, "PLATFORM_PRICE_CONFIRM_PCT", 50),
		Max:        int64(wholeNumber(get, fail, "PLATFORM_PRICE_MAX", 1_000_000_000)),
	}
	if cfg.Price.ConfirmPct > 100 {
		fail("PLATFORM_PRICE_CONFIRM_PCT", "must be a whole number from 1 to 100")
	}

	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}
	return cfg, nil
}

// wholeNumber reads name as a whole number of 1 or more, or returns def when it
// is empty.
func wholeNumber(get func(string) string, fail func(variable, reason string), name string, def int) int {
	raw := get(name)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		fail(name, "must be a whole number, 1 or more")
		return def
	}
	return n
}

// loadDevVerificationCode reads the development sign-in code. It refuses a
// malformed code, and any code when APP_ENV is production.
func loadDevVerificationCode(get func(string) string, fail func(variable, reason string)) string {
	const name = "PLATFORM_DEV_VERIFICATION_CODE"
	code := get(name)
	if code == "" {
		return ""
	}
	if len(code) != 6 || strings.Trim(code, "0123456789") != "" {
		fail(name, "must be empty or exactly six digits")
		return ""
	}
	if strings.EqualFold(get("APP_ENV"), "production") {
		fail(name, "must not be set when APP_ENV is production")
		return ""
	}
	return code
}

// loadBilling reads the VAT rate and the bank details. The bank details may be
// empty; the caller logs which ones are.
func loadBilling(get func(string) string, fail func(variable, reason string)) Billing {
	b := Billing{
		VATRate: billing.DefaultVATRate, BusinessDiscountRequiresVerification: true,
		UsageDueDays:    wholeNumber(get, fail, "PLATFORM_USAGE_DUE_DAYS", 7),
		UsageMinInvoice: int64(wholeNumber(get, fail, "PLATFORM_USAGE_MIN_INVOICE", 10000)),
	}
	if raw := get("BILLING_VAT_RATE"); raw != "" {
		rate, err := billing.ParseRate(raw)
		if err != nil {
			fail("BILLING_VAT_RATE", err.Error())
		} else {
			b.VATRate = rate
		}
	}
	switch raw := get("PLATFORM_BUSINESS_DISCOUNT_REQUIRES_VERIFICATION"); raw {
	case "", "true":
	case "false":
		b.BusinessDiscountRequiresVerification = false
	default:
		fail("PLATFORM_BUSINESS_DISCOUNT_REQUIRES_VERIFICATION", "must be true or false")
	}
	b.Bank = billing.BankDetails{
		AccountName:   get("BILLING_BANK_ACCOUNT_NAME"),
		AccountNumber: get("BILLING_BANK_ACCOUNT_NUMBER"),
		BankName:      get("BILLING_BANK_NAME"),
	}
	return b
}

const (
	defaultWebhookMaxBytes = 65536
	maxWebhookMaxBytes     = 10 << 20
)

// loadPayments reads the payment providers, the webhook port and the webhook
// body limit. A provider that needs a webhook secret refuses the start when the
// secret is missing, and the platform must have a provider for vnd, the only
// currency this release bills.
func loadPayments(get func(string) string, getenv func(string) string, fail func(variable, reason string), bank billing.BankDetails, apiPort, consolePort string) Payments {
	secret := func(provider string) string {
		return strings.TrimSpace(getenv("PAYMENT_WEBHOOK_SECRET_" + strings.ToUpper(provider)))
	}
	p := Payments{WebhookMaxBytes: defaultWebhookMaxBytes}
	registry, err := billing.NewRegistry(getenv("PAYMENT_PROVIDERS"), bank, secret)
	if err != nil {
		fail("PAYMENT_PROVIDERS", err.Error())
	} else if _, _, err := registry.ForCurrency(money.Default); err != nil {
		fail("PAYMENT_PROVIDERS", "must list a provider for "+money.Default)
	} else {
		p.Registry = registry
	}
	if raw := get("PLATFORM_WEBHOOK_PORT"); raw != "" {
		switch {
		case !validPort(raw):
			fail("PLATFORM_WEBHOOK_PORT", "must be a port number from 1 to 65535")
		case raw == apiPort || raw == consolePort:
			fail("PLATFORM_WEBHOOK_PORT", "must differ from PLATFORM_API_PORT and PLATFORM_CONSOLE_PORT")
		default:
			p.WebhookPort = raw
		}
	}
	if raw := getenv("PAYMENT_WEBHOOK_MAX_BYTES"); raw != "" {
		n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil || n < 1 || n > maxWebhookMaxBytes {
			fail("PAYMENT_WEBHOOK_MAX_BYTES", "must be a whole number of bytes from 1 to 10485760")
		} else {
			p.WebhookMaxBytes = n
		}
	}
	return p
}

// loadEmail reads the email provider. With noProviderOK (a dev code is set) an
// absent provider is not an error and the result is empty.
func loadEmail(get func(string) string, getenv func(string) string, fail func(variable, reason string), noProviderOK bool) Email {
	smtpHost := get("SMTP_HOST")
	resendKey := strings.TrimSpace(getenv("RESEND_API_KEY"))

	switch {
	case smtpHost != "":
		e := Email{
			Provider:        ProviderSMTP,
			From:            get("SMTP_FROM_EMAIL"),
			SMTPHost:        smtpHost,
			SMTPPort:        get("SMTP_PORT"),
			SMTPUsername:    getenv("SMTP_USERNAME"),
			SMTPPassword:    getenv("SMTP_PASSWORD"),
			SMTPTLSInsecure: getenv("SMTP_TLS_INSECURE") == "true",
			SMTPEHLOName:    get("SMTP_EHLO_NAME"),
		}
		if e.SMTPPort == "" {
			e.SMTPPort = defaultSMTPPort
		}
		if e.From == "" {
			fail("SMTP_FROM_EMAIL", "is required when SMTP_HOST is set")
		}
		switch mode := strings.ToLower(get("SMTP_TLS")); mode {
		case "":
			e.SMTPTLSImplicit = e.SMTPPort == "465"
		case "implicit", "smtps", "ssl":
			e.SMTPTLSImplicit = true
		case "starttls":
		default:
			fail("SMTP_TLS", "must be starttls, implicit, smtps or ssl")
		}
		return e
	case resendKey != "":
		e := Email{
			Provider:     ProviderResend,
			From:         get("RESEND_FROM_EMAIL"),
			ResendAPIKey: resendKey,
		}
		if e.From == "" {
			fail("RESEND_FROM_EMAIL", "is required when RESEND_API_KEY is set")
		}
		return e
	case noProviderOK:
		return Email{}
	default:
		fail(EmailVariable, "one email provider is required: RESEND_API_KEY with RESEND_FROM_EMAIL, or SMTP_HOST with SMTP_FROM_EMAIL")
		return Email{}
	}
}

func parseAdminEmails(raw string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		addr, err := mail.ParseAddress(part)
		if err != nil || addr.Address != part {
			return nil, errors.New("must be a comma-separated list of plain email addresses")
		}
		part = strings.ToLower(part)
		if !seen[part] {
			seen[part] = true
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("must list at least one email address")
	}
	return out, nil
}

func portOrDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func validPort(v string) bool {
	n, err := strconv.Atoi(v)
	return err == nil && n >= 1 && n <= 65535 && strconv.Itoa(n) == v
}

func parseCIDRs(raw string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(part)
		if err != nil {
			return nil, errors.New("must be a comma-separated list of CIDRs such as 10.0.0.0/8")
		}
		out = append(out, prefix.Masked())
	}
	if len(out) == 0 {
		return nil, errors.New("must list at least one CIDR when set")
	}
	return out, nil
}

// normalizeOrigin accepts scheme://host[:port] with an http or https scheme
// and nothing after the host but an optional slash.
func normalizeOrigin(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Hostname() == "" {
		return "", false
	}
	if u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", false
	}
	return u.Scheme + "://" + strings.ToLower(u.Host), true
}
