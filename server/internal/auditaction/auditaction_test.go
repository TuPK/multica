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

// The log filter accepts every action the audit package defines that either
// server writes: the R2 actions of Multica and the billing and plan actions of
// the platform.
func TestIsAcceptsTheR2Actions(t *testing.T) {
	for _, a := range []string{
		audit.ActionIntegrationConnected, audit.ActionIntegrationDisconnected,
		audit.ActionPluginInstalled, audit.ActionPluginUninstalled, audit.ActionPluginEnabled,
		audit.ActionPluginDisabled, audit.ActionPluginConfigured, audit.ActionPluginTokenRotated,
		audit.ActionMCPServerCreated, audit.ActionMCPServerUpdated, audit.ActionMCPServerDeleted,
		audit.ActionRuntimeProfileCreated, audit.ActionRuntimeProfileUpdated, audit.ActionRuntimeProfileDeleted,
		audit.ActionAgentCreated, audit.ActionAgentArchived, audit.ActionAgentPermissionChanged, audit.ActionAgentEnvUpdated,
		audit.ActionRuntimeVisibilityChanged, audit.ActionRuntimeDeleted,
		audit.ActionWorkspaceExportRequested, audit.ActionWorkspaceExportCompleted, audit.ActionWorkspaceExportFailed,
		audit.ActionWorkspaceDeletionScheduled, audit.ActionWorkspaceDeletionCancelled,
		audit.ActionSubscriptionChanged, audit.ActionPlanChanged,
		audit.ActionBillingOrderCreated, audit.ActionBillingOrderPaid,
		audit.ActionBillingOrderExpired, audit.ActionBillingOrderCanceled,
	} {
		if a == "" {
			t.Error("an audit constant is empty")
		}
		if !Is(a) {
			t.Errorf("%q is not accepted", a)
		}
	}
}
