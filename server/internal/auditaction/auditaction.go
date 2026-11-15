// Package auditaction lists the audit actions the workspace audit log accepts
// as a filter. It has no database dependency, so its tests run anywhere.
package auditaction

import "github.com/multica-ai/multica/platform-management/audit"

// Catalog lists every action name the audit package defines, which is every
// action either server writes: the R1 actions, and the ones the platform
// console writes with the staff actor.
var Catalog = []string{
	audit.ActionMemberRoleChanged,
	audit.ActionMemberRemoved,
	audit.ActionMemberLeft,
	audit.ActionInvitationCreated,
	audit.ActionInvitationRevoked,
	audit.ActionInvitationAccepted,
	audit.ActionInvitationDeclined,
	audit.ActionShareLinkCreated,
	audit.ActionShareLinkRevoked,
	audit.ActionShareLinkJoined,
	audit.ActionWorkspaceCreated,
	audit.ActionWorkspaceUpdated,
	audit.ActionWorkspaceDeleted,
	audit.ActionAuthLogin,
	audit.ActionAuthSessionsRevoked,
	audit.ActionPATCreated,
	audit.ActionPATRevoked,
	audit.ActionAccountExportRequested,
	audit.ActionAccountDeletionRequest,
	audit.ActionAccountDeletionCanceled,
	audit.ActionConsentAccepted,

	audit.ActionWorkspaceStatusChanged,
	audit.ActionNoteAdded,
	audit.ActionAccountStatusChanged,
	audit.ActionAccountSessionsRevoked,
	audit.ActionStaffAdded,
	audit.ActionStaffRemoved,
}

// Is reports whether s is a known action.
func Is(s string) bool {
	for _, a := range Catalog {
		if a == s {
			return true
		}
	}
	return false
}
