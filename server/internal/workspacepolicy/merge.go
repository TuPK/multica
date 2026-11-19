package workspacepolicy

import "slices"

// DenyAllDomain stands for "no address is allowed": a domain list that a
// workspace and its tenant disagree on with nothing in common. It is a name no
// address can carry, so DomainAllowed refuses every address.
const DenyAllDomain = "deny-all.invalid"

// Merge returns the policy in force for a workspace under a tenant floor (spec
// R6 section 3c.4, D101). A workspace may only tighten the floor, so each field
// takes the stricter value: domains intersect, sign-in rules are the union, share
// links need both sides, and the session age is the shorter non-zero limit.
func Merge(tenant, workspace Policy) Policy {
	return Policy{
		AllowedInviteDomains: mergeDomains(tenant.AllowedInviteDomains, workspace.AllowedInviteDomains),
		ShareLinksEnabled:    tenant.ShareLinksEnabled && workspace.ShareLinksEnabled,
		RequireSSO:           tenant.RequireSSO || workspace.RequireSSO,
		RequireMFA:           tenant.RequireMFA || workspace.RequireMFA,
		SessionMaxAgeHours:   minLimit(tenant.SessionMaxAgeHours, workspace.SessionMaxAgeHours),
	}
}

// mergeDomains intersects two lists, where an empty list places no limit.
func mergeDomains(tenant, workspace []string) []string {
	switch {
	case len(tenant) == 0 && len(workspace) == 0:
		return []string{}
	case len(tenant) == 0:
		return slices.Clone(workspace)
	case len(workspace) == 0:
		return slices.Clone(tenant)
	}
	var out []string
	for _, d := range workspace {
		if slices.Contains(tenant, d) {
			out = append(out, d)
		}
	}
	if len(out) == 0 {
		return []string{DenyAllDomain}
	}
	return out
}

// minLimit returns the shorter of two session limits, where zero means no limit.
func minLimit(a, b int32) int32 {
	switch {
	case a == 0:
		return b
	case b == 0:
		return a
	case a < b:
		return a
	}
	return b
}
