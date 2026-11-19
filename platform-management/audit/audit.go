// Package audit defines the audit-event entry, the secret redaction filter and
// the Record writer. It depends on the standard library only; the server
// supplies the database through the Writer interface.
package audit

import (
	"context"
	"errors"
	"strings"
)

// R1 action catalog (spec 4.3).
const (
	ActionMemberRoleChanged = "member.role_changed"
	ActionMemberGuestSet    = "member.guest_set"
	ActionMemberGuestUnset  = "member.guest_unset"
	ActionMemberRemoved     = "member.removed"
	ActionMemberLeft        = "member.left"

	ActionInvitationCreated  = "invitation.created"
	ActionInvitationRevoked  = "invitation.revoked"
	ActionInvitationAccepted = "invitation.accepted"
	ActionInvitationDeclined = "invitation.declined"

	ActionShareLinkCreated = "share_link.created"
	ActionShareLinkRevoked = "share_link.revoked"
	ActionShareLinkJoined  = "share_link.joined"

	ActionWorkspaceCreated = "workspace.created"
	ActionWorkspaceUpdated = "workspace.updated"
	ActionWorkspaceDeleted = "workspace.deleted"

	ActionAuthLogin               = "auth.login"
	ActionAuthSessionsRevoked     = "auth.sessions_revoked"
	ActionPATCreated              = "pat.created"
	ActionPATRevoked              = "pat.revoked"
	ActionAccountExportRequested  = "account.export_requested"
	ActionAccountDeletionRequest  = "account.deletion_requested"
	ActionAccountDeletionCanceled = "account.deletion_canceled"
	ActionConsentAccepted         = "consent.accepted"
)

// Console actions: written by the platform's staff API, with the staff actor.
const (
	ActionWorkspaceStatusChanged = "workspace.status_changed"
	ActionAccountStatusChanged   = "account.status_changed"
	ActionAccountSessionsRevoked = "account.sessions_revoked"
	ActionStaffAdded             = "staff.added"
	ActionStaffRemoved           = "staff.removed"
	ActionStaffRoleChanged       = "staff.role_changed"
	ActionStaffTenantAssigned    = "staff.tenant_assigned"
	ActionNoteAdded              = "note.added"
	ActionSubscriptionChanged    = "subscription.changed"
	ActionPlanChanged            = "plan.changed"
	ActionUserMFAReset           = "user.mfa_reset"
)

// Staff add-member actions (spec R6 section 4.5). member.staff_added has
// actor_type staff: the actor is the staff member who made the request, written
// by Multica's member_provisioning job. All four carry the workspace_id and the
// tenant_id of the workspace's tenant.
const (
	ActionMemberStaffAddRequested = "member.staff_add_requested"
	ActionMemberStaffAdded        = "member.staff_added"
	ActionMemberStaffAddFailed    = "member.staff_add_failed"
	ActionMemberStaffAddCanceled  = "member.staff_add_canceled"
)

// Staff immediate deletion actions (spec R6 section 6.6). The requested actions
// are written in phase 1 by staff and carry the tenant_id of the target's tenant,
// so an admin still reads them after the workspace is gone. tenant.deleted is
// written by Multica's staff_deletion job when the last workspace of the tenant
// has gone. deletion.request_failed has actor_type system; the other two staff.
const (
	ActionWorkspaceImmediateDeletionRequested = "workspace.immediate_deletion_requested"
	ActionAccountImmediateDeletionRequested   = "account.immediate_deletion_requested"
	ActionTenantImmediateDeletionRequested    = "tenant.immediate_deletion_requested"
	ActionTenantDeleted                       = "tenant.deleted"
	ActionDeletionRequestFailed               = "deletion.request_failed"
	ActionDeletionRequestCanceled             = "deletion.request_canceled"
	ActionDeletionRequestRetried              = "deletion.request_retried"
)

// Billing actions (spec 5.14): written when an order is created, paid, canceled
// by staff, or expires.
const (
	ActionBillingOrderCreated  = "billing.order_created"
	ActionBillingOrderPaid     = "billing.order_paid"
	ActionBillingOrderExpired  = "billing.order_expired"
	ActionBillingOrderCanceled = "billing.order_canceled"
)

// R4 payment notice actions (spec 7.2, audit): applied and needs_review are
// written by the system when the reconciler decides a bank transfer notice;
// resolved and dismissed are written by staff and carry a required reason. The
// detail holds ids and amounts, never the notice body, a signature or a secret.
const (
	ActionPaymentNoticeApplied     = "payment.notice_applied"
	ActionPaymentNoticeNeedsReview = "payment.notice_needs_review"
	ActionPaymentNoticeResolved    = "payment.notice_resolved"
	ActionPaymentNoticeDismissed   = "payment.notice_dismissed"
)

// R5 usage billing actions (spec 7b.3, audit table). usage_invoiced is written by
// the system in the transaction that creates a usage invoice. usage_written_off
// is written by staff in place of billing.order_canceled when a usage invoice is
// written off.
const (
	ActionBillingUsageInvoiced           = "billing.usage_invoiced"
	ActionBillingUsageCreditLimitReached = "billing.usage_credit_limit_reached"
	ActionBillingUsageWrittenOff         = "billing.usage_written_off"
)

// R2 actions (spec 5.14), written by Multica in the transaction of the change.
// Details name fields and ids, never a token, an environment value or an MCP
// configuration.
const (
	ActionIntegrationConnected    = "integration.connected"
	ActionIntegrationDisconnected = "integration.disconnected"

	ActionPluginInstalled    = "plugin.installed"
	ActionPluginUninstalled  = "plugin.uninstalled"
	ActionPluginEnabled      = "plugin.enabled"
	ActionPluginDisabled     = "plugin.disabled"
	ActionPluginConfigured   = "plugin.configured"
	ActionPluginTokenRotated = "plugin.token_rotated"

	ActionMCPServerCreated = "mcp_server.created"
	ActionMCPServerUpdated = "mcp_server.updated"
	ActionMCPServerDeleted = "mcp_server.deleted"

	ActionRuntimeProfileCreated = "runtime_profile.created"
	ActionRuntimeProfileUpdated = "runtime_profile.updated"
	ActionRuntimeProfileDeleted = "runtime_profile.deleted"

	ActionAgentCreated           = "agent.created"
	ActionAgentArchived          = "agent.archived"
	ActionAgentPermissionChanged = "agent.permission_changed"
	ActionAgentEnvUpdated        = "agent.env_updated"

	ActionRuntimeVisibilityChanged = "runtime.visibility_changed"
	ActionRuntimeDeleted           = "runtime.deleted"

	ActionWorkspaceExportRequested = "workspace.export_requested"
	ActionWorkspaceExportCompleted = "workspace.export_completed"
	ActionWorkspaceExportFailed    = "workspace.export_failed"

	ActionWorkspaceDeletionScheduled = "workspace.deletion_scheduled"
	ActionWorkspaceDeletionCancelled = "workspace.deletion_cancelled"
)

// Catalog lists every action name this package defines. The audit log filters
// of Multica and of the platform console read it, so a new action is listed
// here and nowhere else; a test fails when a constant is missing from it.
var Catalog = []string{
	ActionMemberRoleChanged,
	ActionMemberGuestSet,
	ActionMemberGuestUnset,
	ActionMemberRemoved,
	ActionMemberLeft,
	ActionInvitationCreated,
	ActionInvitationRevoked,
	ActionInvitationAccepted,
	ActionInvitationDeclined,
	ActionShareLinkCreated,
	ActionShareLinkRevoked,
	ActionShareLinkJoined,
	ActionWorkspaceCreated,
	ActionWorkspaceUpdated,
	ActionWorkspaceDeleted,
	ActionAuthLogin,
	ActionAuthSessionsRevoked,
	ActionPATCreated,
	ActionPATRevoked,
	ActionAccountExportRequested,
	ActionAccountDeletionRequest,
	ActionAccountDeletionCanceled,
	ActionConsentAccepted,
	ActionWorkspaceStatusChanged,
	ActionAccountStatusChanged,
	ActionAccountSessionsRevoked,
	ActionStaffAdded,
	ActionStaffRemoved,
	ActionStaffRoleChanged,
	ActionStaffTenantAssigned,
	ActionMemberStaffAddRequested,
	ActionMemberStaffAdded,
	ActionMemberStaffAddFailed,
	ActionMemberStaffAddCanceled,
	ActionWorkspaceImmediateDeletionRequested,
	ActionAccountImmediateDeletionRequested,
	ActionTenantImmediateDeletionRequested,
	ActionTenantDeleted,
	ActionDeletionRequestFailed,
	ActionDeletionRequestCanceled,
	ActionDeletionRequestRetried,
	ActionNoteAdded,
	ActionSubscriptionChanged,
	ActionPlanChanged,
	ActionUserMFAReset,
	ActionBillingOrderCreated,
	ActionBillingOrderPaid,
	ActionBillingOrderExpired,
	ActionBillingOrderCanceled,
	ActionIntegrationConnected,
	ActionIntegrationDisconnected,
	ActionPluginInstalled,
	ActionPluginUninstalled,
	ActionPluginEnabled,
	ActionPluginDisabled,
	ActionPluginConfigured,
	ActionPluginTokenRotated,
	ActionMCPServerCreated,
	ActionMCPServerUpdated,
	ActionMCPServerDeleted,
	ActionRuntimeProfileCreated,
	ActionRuntimeProfileUpdated,
	ActionRuntimeProfileDeleted,
	ActionAgentCreated,
	ActionAgentArchived,
	ActionAgentPermissionChanged,
	ActionAgentEnvUpdated,
	ActionRuntimeVisibilityChanged,
	ActionRuntimeDeleted,
	ActionWorkspaceExportRequested,
	ActionWorkspaceExportCompleted,
	ActionWorkspaceExportFailed,
	ActionWorkspaceDeletionScheduled,
	ActionWorkspaceDeletionCancelled,
	ActionSubscriptionBackfilled,
	ActionWorkspaceAccountKindChanged,
	ActionBillingProfileVerified,
	ActionBillingProfileUnverified,
	ActionBillingProfileUpdated,
	ActionPlanPriceScheduled,
	ActionAccountProvisionRequested,
	ActionAccountProvisionCanceled,
	ActionAccountProvisionFailed,
	ActionAccountProvisioned,
	ActionAccountWelcomeSent,
	ActionAccountWelcomeResent,
	ActionAccountWelcomeFailed,
	ActionPlanPriceApplied,
	ActionPlanPriceCanceled,
	ActionPlanActivated,
	ActionPlanDeactivated,
	ActionSubscriptionChangeScheduled,
	ActionSubscriptionChangeCanceled,
	ActionSubscriptionCancelRequested,
	ActionSubscriptionCancelWithdrawn,
	ActionGroupCreated,
	ActionGroupUpdated,
	ActionGroupDeleted,
	ActionGroupMemberAdded,
	ActionGroupMemberRemoved,
	ActionGroupPermissionsChanged,
	ActionBillingUsageInvoiced,
	ActionBillingUsageCreditLimitReached,
	ActionBillingUsageWrittenOff,
	ActionPaymentNoticeApplied,
	ActionPaymentNoticeNeedsReview,
	ActionPaymentNoticeResolved,
	ActionPaymentNoticeDismissed,
	ActionEinvoiceRequested,
	ActionEinvoiceIssued,
	ActionEinvoiceNeedsInfo,
	ActionEinvoiceFailed,
	ActionEinvoiceReplaced,
	ActionEinvoiceRetried,
}

// E-invoice actions (spec 7.3, audit): written in the transaction of the status
// change they record. The system writes requested, issued, needs_info and
// failed; staff write replaced, with a required reason, and retried. The detail holds the
// order id, revisions, the invoice number and provider invoice id, error codes,
// attempt counts and missing field names, never a buyer value.
const (
	ActionEinvoiceRequested = "einvoice.requested"
	ActionEinvoiceIssued    = "einvoice.issued"
	ActionEinvoiceNeedsInfo = "einvoice.needs_info"
	ActionEinvoiceFailed    = "einvoice.failed"
	ActionEinvoiceReplaced  = "einvoice.replaced"
	ActionEinvoiceRetried   = "einvoice.retried"
)

// R5 subscription actions (spec 7b.1, rules 5 to 8): written by the platform in the
// transaction of a scheduled plan change, a cancelled one, and a cancel or resume
// request. The detail holds the plan, seats and interval, or the period end.
const (
	ActionSubscriptionChangeScheduled = "subscription.change_scheduled"
	ActionSubscriptionChangeCanceled  = "subscription.change_canceled"
	ActionSubscriptionCancelRequested = "subscription.cancel_requested"
	ActionSubscriptionCancelWithdrawn = "subscription.cancel_withdrawn"
)

// Cutover action (gate G-R2): written by the platform-backfill command in the
// transaction that gives a workspace with no subscription row the free plan.
const ActionSubscriptionBackfilled = "subscription.backfilled"

// R5 action (spec 7b.1, rule 3): written by the platform in the transaction
// that changes a workspace's account kind; the detail holds the old and new kind.
const ActionWorkspaceAccountKindChanged = "workspace.account_kind_changed"

// R5 actions (spec 7b.1, rule 12): written by the platform in the transaction in
// which staff verify or unverify a workspace's billing profile. The detail holds
// the old and new verified_at, never the tax code.
const (
	ActionBillingProfileVerified   = "billing_profile.verified"
	ActionBillingProfileUnverified = "billing_profile.unverified"
	// ActionBillingProfileUpdated is a change of the four profile fields with the
	// account kind unchanged. The detail names the fields, never their values.
	ActionBillingProfileUpdated = "billing_profile.updated"
)

// R5 account request actions (spec 7b.5, audit table): written by the platform in the
// transaction that creates or cancels a request, with the staff actor, no workspace_id and
// the request's email as the target label. ActionAccountProvisionFailed is written when a
// request moves to failed.
const (
	ActionAccountProvisionRequested = "account.provision_requested"
	ActionAccountProvisionCanceled  = "account.provision_canceled"
	ActionAccountProvisionFailed    = "account.provision_failed"
)

// R5 account request actions written by the platform's finalize job (spec 7b.5,
// rules 7, 9 and 13). account.provisioned carries the workspace_id, so the owner
// reads it in the workspace log; its actor is the staff member who made the
// request. The welcome actions have no workspace_id.
const (
	ActionAccountProvisioned   = "account.provisioned"
	ActionAccountWelcomeSent   = "account.welcome_sent"
	ActionAccountWelcomeResent = "account.welcome_resent"
	ActionAccountWelcomeFailed = "account.welcome_failed"
)

// R5 plan price actions (spec 7b.4, rule 15): written in the transaction that
// schedules, applies or cancels a price change. The detail holds plan (the plan
// key, which Redact would hide under a name containing "key"), effective_from,
// the old and new prices and the reason.
const (
	ActionPlanPriceScheduled = "plan.price_scheduled"
	ActionPlanPriceApplied   = "plan.price_applied"
	ActionPlanPriceCanceled  = "plan.price_canceled"
	ActionPlanActivated      = "plan.activated"
	ActionPlanDeactivated    = "plan.deactivated"
)

// R5 account group actions (spec 7b.2, rule 13): written by Multica in the
// transaction of the change. group.updated records only the names of the fields
// that changed; group.permissions_changed records the old and new key arrays.
const (
	ActionGroupCreated            = "group.created"
	ActionGroupUpdated            = "group.updated"
	ActionGroupDeleted            = "group.deleted"
	ActionGroupMemberAdded        = "group.member_added"
	ActionGroupMemberRemoved      = "group.member_removed"
	ActionGroupPermissionsChanged = "group.permissions_changed"
)

// Actor types.
const (
	ActorMember = "member"
	ActorAgent  = "agent"
	ActorSystem = "system"
	ActorStaff  = "staff"
)

// Redacted replaces the value of a sensitive detail key.
const Redacted = "[redacted]"

// Entry mirrors the audit_event columns. IDs are plain strings. An empty
// WorkspaceID, TenantID or ActorID is stored as NULL.
type Entry struct {
	ID          string
	WorkspaceID string
	TenantID    string
	ActorType   string
	ActorID     string
	ActorLabel  string
	Action      string
	TargetType  string
	TargetID    string
	TargetLabel string
	Detail      map[string]any
	IP          string
	UserAgent   string
	RequestID   string
}

// Writer persists one audit entry.
type Writer interface {
	InsertAuditEvent(ctx context.Context, e Entry) error
}

var sensitiveKeyParts = []string{"secret", "token", "password", "key", "env"}

func sensitive(key string) bool {
	k := strings.ToLower(key)
	for _, p := range sensitiveKeyParts {
		if strings.Contains(k, p) {
			return true
		}
	}
	return false
}

// Redact returns a copy of detail with the value of every sensitive key
// replaced by Redacted, at any depth. The input is not modified.
func Redact(detail map[string]any) map[string]any {
	if detail == nil {
		return nil
	}
	out := make(map[string]any, len(detail))
	for k, v := range detail {
		if sensitive(k) {
			out[k] = Redacted
			continue
		}
		out[k] = redactValue(v)
	}
	return out
}

func redactValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return Redact(t)
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = redactValue(e)
		}
		return out
	case []map[string]any:
		out := make([]map[string]any, len(t))
		for i, e := range t {
			out[i] = Redact(e)
		}
		return out
	default:
		return v
	}
}

// Record validates e, redacts its detail and writes it. Write errors are
// returned so the caller can fail the surrounding transaction.
func Record(ctx context.Context, w Writer, e Entry) error {
	if e.Action == "" {
		return errors.New("audit: action is required")
	}
	if e.ActorType == "" {
		return errors.New("audit: actor type is required")
	}
	e.Detail = Redact(e.Detail)
	return w.InsertAuditEvent(ctx, e)
}
