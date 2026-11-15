package planlimit

import "strings"

// BlockedDomainsEnv names the comma-separated list of email domains refused at
// sign-up.
const BlockedDomainsEnv = "SIGNUP_BLOCKED_EMAIL_DOMAINS"

// ParseBlockedDomains splits the list, trims and lower-cases each entry, and
// drops empty ones.
func ParseBlockedDomains(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if d := normalizeDomain(part); d != "" {
			out = append(out, d)
		}
	}
	return out
}

// EmailDomainBlocked reports whether the address's domain equals an entry or
// ends with "." plus an entry. `mail.example.com` matches `example.com`;
// `notexample.com` does not. Matching is case-insensitive.
func EmailDomainBlocked(email string, blocked []string) bool {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}
	domain := normalizeDomain(email[at+1:])
	if domain == "" {
		return false
	}
	for _, entry := range blocked {
		entry = normalizeDomain(entry)
		if entry == "" {
			continue
		}
		if domain == entry || strings.HasSuffix(domain, "."+entry) {
			return true
		}
	}
	return false
}

// normalizeDomain lower-cases a host and drops the trailing dot that makes it
// fully qualified.
func normalizeDomain(s string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s)), ".")
}
