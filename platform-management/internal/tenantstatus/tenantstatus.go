// Package tenantstatus locks and unlocks a whole tenant (spec R6 section 3c.9,
// QT19). A lock writes every workspace of the tenant in one transaction with the
// reason tenant_status; an unlock restores only those workspaces, so a workspace
// that was locked for another reason stays locked.
package tenantstatus

import (
	"slices"
)

// Statuses a tenant may take. A tenant is never pending deletion: deletion has
// its own path (spec rule 1).
const (
	StatusActive    = "active"
	StatusReadOnly  = "read_only"
	StatusSuspended = "suspended"
	// ReasonTenant is the reason a workspace carries when its tenant locked it.
	ReasonTenant = "tenant_status"
)

// Valid reports whether status is one a tenant may take.
func Valid(status string) bool {
	return slices.Contains([]string{StatusActive, StatusReadOnly, StatusSuspended}, status)
}

// WorkspaceStatus is a workspace of a tenant with its current platform status and reason.
type WorkspaceStatus struct {
	ID     string
	Status string
	Reason string
}

// Targets returns the workspaces a change to target writes. A lock writes every
// workspace not already in the lock; an unlock writes only the workspaces the
// tenant lock put in place (reason tenant_status), leaving the others as they are.
func Targets(rows []WorkspaceStatus, target string) []string {
	var out []string
	for _, r := range rows {
		if target == StatusActive {
			if r.Reason == ReasonTenant && r.Status != StatusActive {
				out = append(out, r.ID)
			}
			continue
		}
		if r.Status != target {
			out = append(out, r.ID)
		}
	}
	return out
}
