package auditaction

import (
	"testing"

	"github.com/multica-ai/multica/platform-management/audit"
)

// The workspace audit log accepts the actions the platform console writes, so a
// workspace owner can filter by them.
func TestIsAcceptsEveryStaffAction(t *testing.T) {
	for _, a := range []string{
		"workspace.status_changed", "note.added", "account.status_changed",
		"account.sessions_revoked", "staff.added", "staff.removed",
	} {
		if !Is(a) {
			t.Errorf("%s is not accepted", a)
		}
	}
	for _, a := range []string{
		audit.ActionAccountStatusChanged, audit.ActionAccountSessionsRevoked,
		audit.ActionStaffAdded, audit.ActionStaffRemoved, audit.ActionNoteAdded,
	} {
		if !Is(a) {
			t.Errorf("audit constant %s is not accepted", a)
		}
	}
}

func TestIsAcceptsTheR1Actions(t *testing.T) {
	for _, a := range []string{
		audit.ActionMemberRoleChanged, audit.ActionPATRevoked, audit.ActionConsentAccepted,
		audit.ActionWorkspaceDeleted, audit.ActionAuthSessionsRevoked,
	} {
		if !Is(a) {
			t.Errorf("%s is not accepted", a)
		}
	}
}

func TestIsRefusesUnknownActions(t *testing.T) {
	for _, a := range []string{"", "nope", "staff.exploded", "Staff.Added", " staff.added", "staff.added "} {
		if Is(a) {
			t.Errorf("%q is accepted", a)
		}
	}
}

func TestCatalogHasNoDuplicates(t *testing.T) {
	seen := map[string]bool{}
	for _, a := range Catalog {
		if seen[a] {
			t.Errorf("%s is listed twice", a)
		}
		seen[a] = true
	}
}
