package membergroup

import "slices"

// The permission keys. Adding one is a spec change (7b.2), a constant here, a
// route mapping and a test.
const (
	KeyMembersInvite         = "members.invite"
	KeyAuditRead             = "audit.read"
	KeyBillingView           = "billing.view"
	KeyBillingManage         = "billing.manage"
	KeyIntegrationsManage    = "integrations.manage"
	KeyRuntimeProfilesManage = "runtime_profiles.manage"
)

// catalogue is the fixed set of keys, in the order the catalogue route lists
// them.
var catalogue = []string{
	KeyMembersInvite,
	KeyAuditRead,
	KeyBillingView,
	KeyBillingManage,
	KeyIntegrationsManage,
	KeyRuntimeProfilesManage,
}

// Catalogue returns a copy of the catalogue, in its fixed order.
func Catalogue() []string { return slices.Clone(catalogue) }

// IsKnown reports whether key is in the catalogue.
func IsKnown(key string) bool { return slices.Contains(catalogue, key) }
