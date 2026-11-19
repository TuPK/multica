package audit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The staff management actions of spec R6 section 3.7. Their names are stored in
// audit_event.action, so a rename is a data change.
func TestAuditCatalogHasStaffRoleActions(t *testing.T) {
	for want, got := range map[string]string{
		"staff.role_changed":    ActionStaffRoleChanged,
		"staff.tenant_assigned": ActionStaffTenantAssigned,
	} {
		if got != want {
			t.Errorf("constant = %q, want %q", got, want)
		}
		listed := false
		for _, a := range Catalog {
			listed = listed || a == want
		}
		if !listed {
			t.Errorf("Catalog does not list %s", want)
		}
	}
}

// The staff add-member actions of spec R6 section 4.5.
func TestAuditCatalogHasMemberStaffActions(t *testing.T) {
	for want, got := range map[string]string{
		"member.staff_add_requested": ActionMemberStaffAddRequested,
		"member.staff_added":         ActionMemberStaffAdded,
		"member.staff_add_failed":    ActionMemberStaffAddFailed,
		"member.staff_add_canceled":  ActionMemberStaffAddCanceled,
	} {
		if got != want {
			t.Errorf("constant = %q, want %q", got, want)
		}
		listed := false
		for _, a := range Catalog {
			listed = listed || a == want
		}
		if !listed {
			t.Errorf("Catalog does not list %s", want)
		}
	}
}

func TestAuditCatalogHasDeletionActions(t *testing.T) {
	for want, got := range map[string]string{
		"workspace.immediate_deletion_requested": ActionWorkspaceImmediateDeletionRequested,
		"account.immediate_deletion_requested":   ActionAccountImmediateDeletionRequested,
		"runtime.unregister_requested":           ActionRuntimeUnregisterRequested,
		"tenant.policy_changed":                  ActionTenantPolicyChanged,
		"quota.request_created":                  ActionQuotaRequestCreated,
		"audit.exported":                         ActionAuditExported,
		"tenant.status_changed":                  ActionTenantStatusChanged,
		"deletion.notice_sent":                   ActionDeletionNoticeSent,
		"deletion.notice_failed":                 ActionDeletionNoticeFailed,
		"legal_hold.placed":                      ActionLegalHoldPlaced,
		"legal_hold.released":                    ActionLegalHoldReleased,
		"deletion.blocked_by_hold":               ActionDeletionBlockedByHold,
		"deletion.approval_requested":            ActionDeletionApprovalRequested,
		"deletion.approved":                      ActionDeletionApproved,
		"deletion.rejected":                      ActionDeletionRejected,
		"deletion.approval_expired":              ActionDeletionApprovalExpired,
		"quota.request_approved":                 ActionQuotaRequestApproved,
		"quota.request_denied":                   ActionQuotaRequestDenied,
		"runtime.disabled":                       ActionRuntimeDisabled,
		"runtime.enabled":                        ActionRuntimeEnabled,
		"member.staff_remove_requested":          ActionMemberStaffRemoveRequested,
		"member.staff_role_change_requested":     ActionMemberStaffRoleChangeRequested,
		"tenant.immediate_deletion_requested":    ActionTenantImmediateDeletionRequested,
		"tenant.deleted":                         ActionTenantDeleted,
		"deletion.request_failed":                ActionDeletionRequestFailed,
		"deletion.request_canceled":              ActionDeletionRequestCanceled,
		"deletion.request_retried":               ActionDeletionRequestRetried,
	} {
		if got != want {
			t.Errorf("constant = %q, want %q", got, want)
		}
		listed := false
		for _, a := range Catalog {
			listed = listed || a == want
		}
		if !listed {
			t.Errorf("Catalog does not list %s", want)
		}
	}
}

// Every platform writer of audit_event passes the entry's tenant to the query,
// so a row about a tenant keeps its tenant_id (spec R6 section 3.5, D84). A
// writer that builds InsertAuditEventParams without TenantID would drop it.
func TestAuditEntryTenantIDReachesTheWriter(t *testing.T) {
	var writers []string
	err := filepath.WalkDir(filepath.Join("..", "internal"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		if !strings.Contains(string(src), "db.InsertAuditEventParams{") {
			return nil
		}
		writers = append(writers, path)
		body := string(src)
		start := strings.Index(body, "db.InsertAuditEventParams{")
		end := start + strings.Index(body[start:], "\n\t})")
		if end < start || !strings.Contains(body[start:end], "TenantID:") {
			t.Errorf("%s builds InsertAuditEventParams without TenantID", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(writers) < 8 {
		t.Errorf("found %d audit writers, want at least the 8 of the platform: %v", len(writers), writers)
	}
}

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
		"workspace.status_changed":            ActionWorkspaceStatusChanged,
		"account.status_changed":              ActionAccountStatusChanged,
		"account.sessions_revoked":            ActionAccountSessionsRevoked,
		"staff.added":                         ActionStaffAdded,
		"staff.removed":                       ActionStaffRemoved,
		"note.added":                          ActionNoteAdded,
		"subscription.changed":                ActionSubscriptionChanged,
		"subscription.limit_override_changed": ActionSubscriptionLimitOverrideChanged,
		"plan.changed":                        ActionPlanChanged,
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

// The billing profile verification actions (spec 7b.1, rule 12) are stored in
// audit_event.action, so a rename is a data change.
func TestBillingProfileVerificationActionsAreDefinedAndCataloged(t *testing.T) {
	cases := map[string]string{
		"billing_profile.verified":   ActionBillingProfileVerified,
		"billing_profile.unverified": ActionBillingProfileUnverified,
	}
	for want, got := range cases {
		if got != want {
			t.Errorf("constant = %q, want %q", got, want)
		}
		found := false
		for _, a := range Catalog {
			found = found || a == want
		}
		if !found {
			t.Errorf("%s is not in Catalog", want)
		}
	}
}

// The plan price actions (spec 7b.4) are stored in audit_event.action, so a
// rename is a data change. Each is in Catalog, which the audit log filters read.
func TestPlanPriceActionsAreDefinedAndCataloged(t *testing.T) {
	cases := map[string]string{
		"plan.price_scheduled": ActionPlanPriceScheduled,
		"plan.price_applied":   ActionPlanPriceApplied,
		"plan.price_canceled":  ActionPlanPriceCanceled,
		"plan.activated":       ActionPlanActivated,
		"plan.deactivated":     ActionPlanDeactivated,
	}
	for want, got := range cases {
		if got != want {
			t.Errorf("constant = %q, want %q", got, want)
		}
		found := false
		for _, a := range Catalog {
			found = found || a == want
		}
		if !found {
			t.Errorf("%s is not in Catalog", want)
		}
	}
}

// The R5 subscription actions (spec 7b.1): the names are stored in audit_event.action.
func TestSubscriptionChangeCatalogActionsAreDefined(t *testing.T) {
	cases := map[string]string{
		"subscription.change_scheduled": ActionSubscriptionChangeScheduled,
		"subscription.change_canceled":  ActionSubscriptionChangeCanceled,
		"subscription.cancel_requested": ActionSubscriptionCancelRequested,
		"subscription.cancel_withdrawn": ActionSubscriptionCancelWithdrawn,
	}
	for want, got := range cases {
		if got != want {
			t.Errorf("constant = %q, want %q", got, want)
		}
	}
}

// The account group actions (spec 7b.2, rule 13) are stored in
// audit_event.action, so a rename is a data change. Each is in Catalog, which
// the audit log filters read.
func TestGroupCatalogActionsAreDefinedAndCataloged(t *testing.T) {
	cases := map[string]string{
		"group.created":             ActionGroupCreated,
		"group.updated":             ActionGroupUpdated,
		"group.deleted":             ActionGroupDeleted,
		"group.member_added":        ActionGroupMemberAdded,
		"group.member_removed":      ActionGroupMemberRemoved,
		"group.permissions_changed": ActionGroupPermissionsChanged,
	}
	for want, got := range cases {
		if got != want {
			t.Errorf("constant = %q, want %q", got, want)
		}
		found := false
		for _, a := range Catalog {
			found = found || a == want
		}
		if !found {
			t.Errorf("%s is not in Catalog", want)
		}
	}
}

// The finalize and welcome actions of spec 7b.5 are stored in audit_event.action,
// so a rename is a data change.
func TestAccountFinalizeAndWelcomeActionsAreDefined(t *testing.T) {
	cases := map[string]string{
		"account.provisioned":    ActionAccountProvisioned,
		"account.welcome_sent":   ActionAccountWelcomeSent,
		"account.welcome_resent": ActionAccountWelcomeResent,
		"account.welcome_failed": ActionAccountWelcomeFailed,
	}
	for want, got := range cases {
		if got != want {
			t.Errorf("constant = %q, want %q", got, want)
		}
		listed := false
		for _, a := range Catalog {
			listed = listed || a == want
		}
		if !listed {
			t.Errorf("%s is not in Catalog", want)
		}
	}
}

// The payment notice actions (spec 7.2) are stored in audit_event.action, so a
// rename is a data change. Each is in Catalog, which the audit log filters read.
func TestPaymentNoticeActionsAreDefinedAndCataloged(t *testing.T) {
	cases := map[string]string{
		"payment.notice_applied":      ActionPaymentNoticeApplied,
		"payment.notice_needs_review": ActionPaymentNoticeNeedsReview,
		"payment.notice_resolved":     ActionPaymentNoticeResolved,
		"payment.notice_dismissed":    ActionPaymentNoticeDismissed,
	}
	for want, got := range cases {
		if got != want {
			t.Errorf("constant = %q, want %q", got, want)
		}
		found := false
		for _, a := range Catalog {
			found = found || a == want
		}
		if !found {
			t.Errorf("%s is not in Catalog", want)
		}
	}
}

// The e-invoice actions (spec 7.3, audit) are stored in audit_event.action, so a
// rename is a data change. Each is in Catalog, which the audit log filters read.
func TestEinvoiceActionsAreDefinedAndCataloged(t *testing.T) {
	cases := map[string]string{
		"einvoice.requested":  ActionEinvoiceRequested,
		"einvoice.issued":     ActionEinvoiceIssued,
		"einvoice.needs_info": ActionEinvoiceNeedsInfo,
		"einvoice.failed":     ActionEinvoiceFailed,
		"einvoice.replaced":   ActionEinvoiceReplaced,
	}
	for want, got := range cases {
		if got == "" || got != want {
			t.Errorf("constant = %q, want %q", got, want)
		}
		found := false
		for _, a := range Catalog {
			found = found || a == want
		}
		if !found {
			t.Errorf("%s is not in Catalog", want)
		}
	}
}

// The guest flag actions (spec 7.1 rule 9, audit) are stored in
// audit_event.action, so a rename is a data change. Each is in Catalog, which
// the audit log filters read.
func TestGuestCatalogActionsAreDefinedAndCataloged(t *testing.T) {
	cases := map[string]string{
		"member.guest_set":   ActionMemberGuestSet,
		"member.guest_unset": ActionMemberGuestUnset,
	}
	for want, got := range cases {
		if got == "" || got != want {
			t.Errorf("constant = %q, want %q", got, want)
		}
		found := false
		for _, a := range Catalog {
			found = found || a == want
		}
		if !found {
			t.Errorf("%s is not in Catalog", want)
		}
	}
}
