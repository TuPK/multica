package audit

import "testing"

// The R1 catalog: every name must be a non-empty constant of this package.
func TestR1CatalogActionsAreDefined(t *testing.T) {
	cases := map[string]string{
		"member.role_changed":   ActionMemberRoleChanged,
		"member.removed":        ActionMemberRemoved,
		"member.left":           ActionMemberLeft,
		"invitation.created":    ActionInvitationCreated,
		"invitation.revoked":    ActionInvitationRevoked,
		"invitation.accepted":   ActionInvitationAccepted,
		"invitation.declined":   ActionInvitationDeclined,
		"share_link.created":    ActionShareLinkCreated,
		"share_link.revoked":    ActionShareLinkRevoked,
		"share_link.joined":     ActionShareLinkJoined,
		"workspace.created":     ActionWorkspaceCreated,
		"workspace.updated":     ActionWorkspaceUpdated,
		"workspace.deleted":     ActionWorkspaceDeleted,
		"auth.login":            ActionAuthLogin,
		"auth.sessions_revoked": ActionAuthSessionsRevoked,
		"pat.created":           ActionPATCreated,
		"pat.revoked":           ActionPATRevoked,
	}
	for want, got := range cases {
		if got == "" {
			t.Errorf("%s: constant is empty", want)
		}
		if got != want {
			t.Errorf("constant = %q, want %q", got, want)
		}
	}
}

// The console actions are written by the staff API. Their names are stored in
// audit_event.action, so a rename is a data change.
func TestConsoleCatalogActionsAreDefined(t *testing.T) {
	cases := map[string]string{
		"workspace.status_changed": ActionWorkspaceStatusChanged,
		"account.status_changed":   ActionAccountStatusChanged,
		"account.sessions_revoked": ActionAccountSessionsRevoked,
		"staff.added":              ActionStaffAdded,
		"staff.removed":            ActionStaffRemoved,
		"note.added":               ActionNoteAdded,
	}
	for want, got := range cases {
		if got != want {
			t.Errorf("constant = %q, want %q", got, want)
		}
	}
}
