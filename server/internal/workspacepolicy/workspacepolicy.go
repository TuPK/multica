// Package workspacepolicy holds the pure decisions of the per-workspace
// policies of the platform spec 6.4: which invitation domains match, whether a
// session is too old, and whether it passed the second factor. It has no
// database and no HTTP, so every rule here is tested without either.
package workspacepolicy

import (
	"fmt"
	"reflect"
	"strings"
	"time"
)

// Refusal codes. Clients branch on them, so they are part of the API.
const (
	CodeInviteDomainNotAllowed = "invite_domain_not_allowed"
	CodeShareLinksDisabled     = "share_links_disabled"
)

// Bounds of a policy write.
const (
	MaxDomains            = 100
	MaxSessionMaxAgeHours = 24 * 365
)

// amrMFA is the sign-in method a session records once it passed the second
// factor. It equals auth.AMRMFA; this package does not import auth.
const amrMFA = "mfa"

// Policy is the policy of one workspace.
type Policy struct {
	AllowedInviteDomains []string
	ShareLinksEnabled    bool
	RequireSSO           bool
	RequireMFA           bool
	// SessionMaxAgeHours is the longest a session may be since its login. Zero
	// means no limit.
	SessionMaxAgeHours int32
}

// Default is the policy of a workspace with no row: nothing is restricted.
func Default() Policy {
	return Policy{AllowedInviteDomains: []string{}, ShareLinksEnabled: true}
}

// DomainAllowed reports whether email may be invited or may join under the
// allowed domains. An empty list allows every address. An entry allows its own
// domain and its subdomains, so "example.com" allows "dev.example.com" and not
// "evilexample.com" or "example.com.evil.io". The domain is what follows the
// last "@".
func DomainAllowed(allowed []string, email string) bool {
	if len(allowed) == 0 {
		return true
	}
	email = strings.TrimSpace(email)
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}
	domain := strings.TrimSuffix(strings.ToLower(email[at+1:]), ".")
	if domain == "" {
		return false
	}
	for _, entry := range allowed {
		entry = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(entry)), ".")
		if entry == "" {
			continue
		}
		if domain == entry || strings.HasSuffix(domain, "."+entry) {
			return true
		}
	}
	return false
}

// NormalizeDomains validates a domain list from a request and returns it
// lower-cased, without a leading "@", and without duplicates. A nil input
// returns an empty, non-nil list.
func NormalizeDomains(in []string) ([]string, error) {
	if len(in) > MaxDomains {
		return nil, fmt.Errorf("at most %d domains are allowed", MaxDomains)
	}
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, raw := range in {
		d := strings.ToLower(strings.TrimSpace(raw))
		d = strings.TrimPrefix(d, "@")
		if !validDomain(d) {
			return nil, fmt.Errorf("%q is not a valid domain", raw)
		}
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out, nil
}

func validDomain(d string) bool {
	if d == "" || len(d) > 253 || !strings.Contains(d, ".") {
		return false
	}
	for _, label := range strings.Split(d, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}

// HasMFA reports whether the sign-in methods of a session include the second
// factor.
func HasMFA(amr []string) bool {
	for _, m := range amr {
		if m == amrMFA {
			return true
		}
	}
	return false
}

// SessionTooOld reports whether a session whose login was at authTime (Unix
// seconds) is older than maxAgeHours at now. Zero hours means no limit. When a
// limit is set and the login time is unknown, the session cannot prove its
// age and counts as too old.
func SessionTooOld(authTime int64, authTimeKnown bool, now time.Time, maxAgeHours int32) bool {
	if maxAgeHours <= 0 {
		return false
	}
	if !authTimeKnown {
		return true
	}
	return now.Unix()-authTime > int64(maxAgeHours)*3600
}

// Changes returns, for each field that differs between old and next, its old
// and new value. It is the detail of the policy.updated audit entry.
func Changes(old, next Policy) map[string]any {
	out := map[string]any{}
	add := func(field string, o, n any) {
		if !reflect.DeepEqual(o, n) {
			out[field] = map[string]any{"old": o, "new": n}
		}
	}
	add("allowed_invite_domains", nonNil(old.AllowedInviteDomains), nonNil(next.AllowedInviteDomains))
	add("share_links_enabled", old.ShareLinksEnabled, next.ShareLinksEnabled)
	add("require_sso", old.RequireSSO, next.RequireSSO)
	add("require_mfa", old.RequireMFA, next.RequireMFA)
	add("session_max_age_hours", old.SessionMaxAgeHours, next.SessionMaxAgeHours)
	return out
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
