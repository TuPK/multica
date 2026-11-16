// Package auditaction lists the audit actions the workspace audit log accepts
// as a filter. It has no database dependency, so its tests run anywhere.
package auditaction

import "github.com/multica-ai/multica/platform-management/audit"

// PolicyUpdated is written when a workspace owner changes the workspace
// policy. It is not yet an audit package constant, so Catalog adds it here.
const PolicyUpdated = "policy.updated"

// Catalog lists every action name the audit package defines plus PolicyUpdated,
// which is every action either server writes.
var Catalog = append(append([]string(nil), audit.Catalog...), PolicyUpdated)

// Is reports whether s is a known action.
func Is(s string) bool {
	for _, a := range Catalog {
		if a == s {
			return true
		}
	}
	return false
}
