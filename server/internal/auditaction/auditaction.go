// Package auditaction lists the audit actions the workspace audit log accepts
// as a filter. It has no database dependency, so its tests run anywhere.
package auditaction

import "github.com/multica-ai/multica/platform-management/audit"

// Catalog lists every action name the audit package defines, which is every
// action either server writes. It is the audit package's own list, so the two
// filters cannot drift from the constants.
var Catalog = audit.Catalog

// Is reports whether s is a known action.
func Is(s string) bool {
	for _, a := range Catalog {
		if a == s {
			return true
		}
	}
	return false
}
