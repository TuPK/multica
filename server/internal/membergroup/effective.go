package membergroup

import "sort"

// Roles that hold every key.
const (
	roleOwner = "owner"
	roleAdmin = "admin"
)

// Effective returns the permissions of a person: the keys their role implies,
// joined with the keys of every group they are in. owner and admin hold all
// six; member and viewer imply none. billing.manage implies billing.view. A key
// that is not in the catalogue, stored by another build, grants nothing. Groups
// only add: there is no deny and no order between groups.
//
// The result is sorted and never nil.
func Effective(role string, groupPermissions ...[]string) []string {
	have := map[string]bool{}
	if role == roleOwner || role == roleAdmin {
		for _, key := range catalogue {
			have[key] = true
		}
	}
	for _, keys := range groupPermissions {
		for _, key := range keys {
			if IsKnown(key) {
				have[key] = true
			}
		}
	}
	if have[KeyBillingManage] {
		have[KeyBillingView] = true
	}
	out := make([]string, 0, len(have))
	for key := range have {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// Has reports whether effective, as Effective returns it, holds key.
func Has(effective []string, key string) bool {
	for _, k := range effective {
		if k == key {
			return true
		}
	}
	return false
}
