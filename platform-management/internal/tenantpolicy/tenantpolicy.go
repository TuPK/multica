// Package tenantpolicy holds the tenant policy floor (spec R6 section 3c.4,
// D101): the limits a tenant's policy sets, and the checks that run before a
// write. Multica reads the stored floor and merges it with each workspace's
// policy; this package does not merge.
package tenantpolicy

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Bounds the platform applies to a policy. They equal the workspace policy's
// bounds in server/internal/workspacepolicy (MaxDomains, MaxSessionMaxAgeHours):
// the platform does not import the server module, so keep the two in step.
const (
	MaxDomains            = 100
	MaxSessionMaxAgeHours = 24 * 365
)

// Policy is the floor a tenant sets for its workspaces.
type Policy struct {
	AllowedInviteDomains []string `json:"allowed_invite_domains"`
	ShareLinksEnabled    bool     `json:"share_links_enabled"`
	RequireSSO           bool     `json:"require_sso"`
	RequireMFA           bool     `json:"require_mfa"`
	// SessionMaxAgeHours is the longest a session may be since its login. Zero means no limit.
	SessionMaxAgeHours int32 `json:"session_max_age_hours"`
}

// Stored is a policy with the version of its row. A tenant without a row has
// Version 0 and no floor (spec rule 4).
type Stored struct {
	Policy
	Version int64 `json:"version"`
}

// Default is the policy of a tenant with no row: nothing is restricted.
func Default() Policy {
	return Policy{AllowedInviteDomains: []string{}, ShareLinksEnabled: true}
}

// NormalizeDomains trims and lower-cases each domain, drops duplicates and
// rejects an empty entry, a space, or an "@" (a domain is not an address).
func NormalizeDomains(in []string) ([]string, error) {
	if len(in) > MaxDomains {
		return nil, fmt.Errorf("at most %d domains are allowed", MaxDomains)
	}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		d := strings.ToLower(strings.TrimSpace(raw))
		if d == "" || strings.ContainsAny(d, " \t@") {
			return nil, errors.New("each domain must be a bare domain such as example.com")
		}
		if !slices.Contains(out, d) {
			out = append(out, d)
		}
	}
	return out, nil
}

// Validate checks a policy before it is stored.
func Validate(p Policy) error {
	if _, err := NormalizeDomains(p.AllowedInviteDomains); err != nil {
		return err
	}
	if p.SessionMaxAgeHours < 0 || p.SessionMaxAgeHours > MaxSessionMaxAgeHours {
		return fmt.Errorf("session_max_age_hours must be 0 to %d", MaxSessionMaxAgeHours)
	}
	return nil
}

// Changed names the fields that differ between a and b, in a fixed order, for the audit row.
func Changed(a, b Policy) []string {
	var out []string
	if !slices.Equal(a.AllowedInviteDomains, b.AllowedInviteDomains) {
		out = append(out, "allowed_invite_domains")
	}
	if a.ShareLinksEnabled != b.ShareLinksEnabled {
		out = append(out, "share_links_enabled")
	}
	if a.RequireSSO != b.RequireSSO {
		out = append(out, "require_sso")
	}
	if a.RequireMFA != b.RequireMFA {
		out = append(out, "require_mfa")
	}
	if a.SessionMaxAgeHours != b.SessionMaxAgeHours {
		out = append(out, "session_max_age_hours")
	}
	return out
}
