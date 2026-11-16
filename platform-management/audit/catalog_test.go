package audit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

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
		"subscription.changed":     ActionSubscriptionChanged,
		"plan.changed":             ActionPlanChanged,
	}
	for want, got := range cases {
		if got != want {
			t.Errorf("constant = %q, want %q", got, want)
		}
	}
}

// The billing actions are stored in audit_event.action, so a rename is a data
// change.
func TestBillingCatalogActionsAreDefined(t *testing.T) {
	cases := map[string]string{
		"billing.order_created":  ActionBillingOrderCreated,
		"billing.order_paid":     ActionBillingOrderPaid,
		"billing.order_expired":  ActionBillingOrderExpired,
		"billing.order_canceled": ActionBillingOrderCanceled,
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

// The R2 actions written by Multica (spec 5.14). Their names are stored in
// audit_event.action, so a rename is a data change.
func TestR2CatalogActionsAreDefined(t *testing.T) {
	cases := map[string]string{
		"integration.connected":        ActionIntegrationConnected,
		"integration.disconnected":     ActionIntegrationDisconnected,
		"plugin.installed":             ActionPluginInstalled,
		"plugin.uninstalled":           ActionPluginUninstalled,
		"plugin.enabled":               ActionPluginEnabled,
		"plugin.disabled":              ActionPluginDisabled,
		"plugin.configured":            ActionPluginConfigured,
		"plugin.token_rotated":         ActionPluginTokenRotated,
		"mcp_server.created":           ActionMCPServerCreated,
		"mcp_server.updated":           ActionMCPServerUpdated,
		"mcp_server.deleted":           ActionMCPServerDeleted,
		"runtime_profile.created":      ActionRuntimeProfileCreated,
		"runtime_profile.updated":      ActionRuntimeProfileUpdated,
		"runtime_profile.deleted":      ActionRuntimeProfileDeleted,
		"agent.created":                ActionAgentCreated,
		"agent.archived":               ActionAgentArchived,
		"agent.permission_changed":     ActionAgentPermissionChanged,
		"agent.env_updated":            ActionAgentEnvUpdated,
		"runtime.visibility_changed":   ActionRuntimeVisibilityChanged,
		"runtime.deleted":              ActionRuntimeDeleted,
		"workspace.export_requested":   ActionWorkspaceExportRequested,
		"workspace.export_completed":   ActionWorkspaceExportCompleted,
		"workspace.export_failed":      ActionWorkspaceExportFailed,
		"workspace.deletion_scheduled": ActionWorkspaceDeletionScheduled,
		"workspace.deletion_cancelled": ActionWorkspaceDeletionCancelled,
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

// Catalog is non-empty, holds no empty name and no duplicate, so one name never
// means two events.
func TestCatalogHasNoEmptyOrDuplicateAction(t *testing.T) {
	if len(Catalog) == 0 {
		t.Fatal("Catalog is empty")
	}
	seen := map[string]bool{}
	for _, a := range Catalog {
		if a == "" {
			t.Error("Catalog holds an empty action")
		}
		if seen[a] {
			t.Errorf("%s is listed twice", a)
		}
		seen[a] = true
	}
}

// Catalog lists every Action constant of audit.go. The filters of the console
// and of the workspace log read it, so a constant left out of it could be
// written but not filtered by.
func TestCatalogListsEveryActionConstant(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "audit.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	for _, a := range Catalog {
		listed[a] = true
	}
	found := 0
	for _, d := range file.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			for i, n := range vs.Names {
				if !strings.HasPrefix(n.Name, "Action") || i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok {
					continue
				}
				found++
				value, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatal(err)
				}
				if !listed[value] {
					t.Errorf("%s (%q) is not in Catalog", n.Name, value)
				}
			}
		}
	}
	if found != len(Catalog) {
		t.Errorf("audit.go defines %d actions, Catalog lists %d", found, len(Catalog))
	}
}

// The cutover action of the platform-backfill command (gate G-R2). Its name is
// stored in audit_event.action, so a rename is a data change.
func TestCutoverCatalogActionIsDefined(t *testing.T) {
	if ActionSubscriptionBackfilled == "" {
		t.Fatal("ActionSubscriptionBackfilled is empty")
	}
	if ActionSubscriptionBackfilled != "subscription.backfilled" {
		t.Fatalf("constant = %q, want subscription.backfilled", ActionSubscriptionBackfilled)
	}
	for _, a := range Catalog {
		if a == ActionSubscriptionBackfilled {
			return
		}
	}
	t.Fatal("Catalog does not list subscription.backfilled")
}

// The R5 action of the account kind change (spec 7b.1, rule 3). Its name is
// stored in audit_event.action, so a rename is a data change.
func TestR5AccountKindActionIsDefined(t *testing.T) {
	if ActionWorkspaceAccountKindChanged != "workspace.account_kind_changed" {
		t.Fatalf("constant = %q, want workspace.account_kind_changed", ActionWorkspaceAccountKindChanged)
	}
	for _, a := range Catalog {
		if a == ActionWorkspaceAccountKindChanged {
			return
		}
	}
	t.Fatal("Catalog does not list workspace.account_kind_changed")
}
