import { z } from "zod";
import { parseWithFallback } from "@multica/core/api/schema";

// Every schema here is lenient on purpose: a missing optional field takes a
// default, and a string enum the console does not know becomes "unknown", so
// a newer server cannot blank a screen. Unknown keys are dropped.

export const WORKSPACE_STATUSES = ["active", "read_only", "suspended", "pending_deletion"] as const;
export const ACCOUNT_STATUSES = ["active", "suspended", "pending_deletion", "deleted"] as const;
export const STAFF_ROLES = ["system_admin", "admin", "support", "billing"] as const;
export const ACTOR_TYPES = ["member", "agent", "system", "staff"] as const;
export const ACCOUNT_KINDS = ["individual", "business"] as const;

/**
 * The audit action codes the console API's `action` filter accepts. Kept in
 * step with platform-management/audit/audit.go by a test.
 */
export const AUDIT_ACTIONS = [
  "member.role_changed",
  "member.guest_set",
  "member.guest_unset",
  "member.removed",
  "member.left",
  "invitation.created",
  "invitation.revoked",
  "invitation.accepted",
  "invitation.declined",
  "share_link.created",
  "share_link.revoked",
  "share_link.joined",
  "workspace.created",
  "workspace.updated",
  "workspace.deleted",
  "audit.exported",
  "auth.login",
  "auth.sessions_revoked",
  "pat.created",
  "pat.revoked",
  "account.export_requested",
  "account.deletion_requested",
  "account.deletion_canceled",
  "consent.accepted",
  "workspace.status_changed",
  "account.status_changed",
  "tenant.status_changed",
  "account.sessions_revoked",
  "staff.added",
  "staff.removed",
  "staff.role_changed",
  "staff.tenant_assigned",
  "member.staff_add_requested",
  "member.staff_added",
  "member.staff_add_failed",
  "member.staff_add_canceled",
  "workspace.immediate_deletion_requested",
  "account.immediate_deletion_requested",
  "tenant.immediate_deletion_requested",
  "tenant.deleted",
  "member.staff_remove_requested",
  "member.staff_role_change_requested",
  "plan.activated",
  "plan.deactivated",
  "runtime.disabled",
  "runtime.enabled",
  "runtime.unregister_requested",
  "subscription.limit_override_changed",
  "tenant.policy_changed",
  "deletion.request_failed",
  "deletion.request_canceled",
  "deletion.request_retried",
  "deletion.notice_sent",
  "deletion.notice_failed",
  "deletion.approval_requested",
  "deletion.approved",
  "deletion.rejected",
  "deletion.approval_expired",
  "deletion.blocked_by_hold",
  "legal_hold.placed",
  "legal_hold.released",
  "quota.request_created",
  "quota.request_approved",
  "quota.request_denied",
  "note.added",
  "subscription.changed",
  "plan.changed",
  "user.mfa_reset",
  "billing.order_created",
  "billing.order_paid",
  "billing.order_expired",
  "billing.order_canceled",
  "billing.usage_invoiced",
  "billing.usage_credit_limit_reached",
  "billing.usage_written_off",
  "billing_profile.verified",
  "billing_profile.unverified",
  "billing_profile.updated",
  "integration.connected",
  "integration.disconnected",
  "plugin.installed",
  "plugin.uninstalled",
  "plugin.enabled",
  "plugin.disabled",
  "plugin.configured",
  "plugin.token_rotated",
  "mcp_server.created",
  "mcp_server.updated",
  "mcp_server.deleted",
  "runtime_profile.created",
  "runtime_profile.updated",
  "runtime_profile.deleted",
  "agent.created",
  "agent.archived",
  "agent.permission_changed",
  "agent.env_updated",
  "runtime.visibility_changed",
  "runtime.deleted",
  "workspace.export_requested",
  "workspace.export_completed",
  "workspace.export_failed",
  "workspace.deletion_scheduled",
  "workspace.deletion_cancelled",
  "subscription.change_scheduled",
  "subscription.change_canceled",
  "subscription.cancel_requested",
  "subscription.cancel_withdrawn",
  "account.provision_requested",
  "account.provision_canceled",
  "account.provision_failed",
  "account.provisioned",
  "account.welcome_sent",
  "account.welcome_resent",
  "account.welcome_failed",
  "plan.price_scheduled",
  "plan.price_applied",
  "plan.price_canceled",
  "group.created",
  "group.updated",
  "group.deleted",
  "group.member_added",
  "group.member_removed",
  "group.permissions_changed",
  "payment.notice_applied",
  "payment.notice_needs_review",
  "payment.notice_resolved",
  "payment.notice_dismissed",
  "einvoice.requested",
  "einvoice.issued",
  "einvoice.needs_info",
  "einvoice.failed",
  "einvoice.replaced",
  "einvoice.retried",
] as const;

export type WorkspaceStatus = (typeof WORKSPACE_STATUSES)[number] | "unknown";
export type AccountStatus = (typeof ACCOUNT_STATUSES)[number] | "unknown";
export type StaffRole = (typeof STAFF_ROLES)[number] | "unknown";
export type ActorType = (typeof ACTOR_TYPES)[number] | "unknown";

function enumOf<const T extends readonly string[]>(values: T) {
  return z
    .string()
    .transform((v): T[number] | "unknown" => (values.includes(v) ? v : "unknown"))
    .default("unknown");
}

function optionalEnumOf<const T extends readonly string[]>(values: T) {
  return z
    .string()
    .transform((v): T[number] | "unknown" => (values.includes(v) ? v : "unknown"))
    .optional();
}

const text = z.string().default("");
const count = z.number().default(0);
const nullableText = z.string().nullable().default(null);
const nextCursor = z.string().nullable().default(null);

// /me

export const MeSchema = z.object({
  user_id: z.string(),
  email: text,
  role: enumOf(STAFF_ROLES),
  tenant_id: nullableText,
});
export type Me = z.output<typeof MeSchema>;

// Workspaces

// The newest daily usage row of a workspace. A workspace with no row has a null
// day and zeros. The fields are optional, not defaulted, so a response from a
// server that does not send them parses to the same object as before and the
// screens treat a missing or null day as "no usage".
const usageNumber = z.number().optional();
const usageFields = {
  usage_day: z.string().nullable().optional(),
  usage_members: usageNumber,
  usage_agents: usageNumber,
  usage_runtimes: usageNumber,
  usage_issues_total: usageNumber,
  usage_issues_created: usageNumber,
  usage_tasks_run: usageNumber,
  usage_storage_bytes: usageNumber,
};

// Counts of one workspace now, from the detail page's bounded query.
export const UsageCurrentSchema = z.object({
  members: count,
  agents: count,
  runtimes: count,
  issues_total: count,
  storage_bytes: count,
});
export type UsageCurrent = z.output<typeof UsageCurrentSchema>;

export const WorkspaceSchema = z.object({
  id: z.string(),
  name: text,
  slug: text,
  status: enumOf(WORKSPACE_STATUSES),
  member_count: count,
  created_at: text,
  last_activity_at: text,
  ...usageFields,
});
export type Workspace = z.output<typeof WorkspaceSchema>;

const WorkspaceStateSchema = z.object({
  status: enumOf(WORKSPACE_STATUSES),
  reason: text,
  changed_by: nullableText,
  changed_at: nullableText,
  deletion_scheduled_at: nullableText,
});
export type WorkspaceState = z.output<typeof WorkspaceStateSchema>;

// Billing & Orders

export const ORDER_STATUSES = ["pending", "paid", "expired", "canceled", "refunded"] as const;
export const ORDER_KINDS = ["subscribe", "change_plan", "add_seats", "renew", "usage"] as const;
export const ORDER_INVOICE_STATES = ["pending", "issued", "needs_info", "failed"] as const;

export type OrderStatus = (typeof ORDER_STATUSES)[number] | "unknown";
export type OrderKind = (typeof ORDER_KINDS)[number] | "unknown";
export type OrderInvoiceState = (typeof ORDER_INVOICE_STATES)[number];

function parseOrderInvoiceState(v: string | null): OrderInvoiceState | null {
  switch (v) {
    case "pending":
    case "issued":
    case "needs_info":
    case "failed":
      return v;
    default:
      return null;
  }
}

export const BuyerSchema = z.object({
  name: text,
  tax_code: text,
  address: text,
  invoice_email: text,
});
export type Buyer = z.output<typeof BuyerSchema>;

export const OrderSchema = z.object({
  id: z.string(),
  workspace_id: z.string(),
  workspace_name: text,
  kind: enumOf(ORDER_KINDS),
  plan_key: text,
  interval: text,
  seats: count,
  amount: count,
  vat_amount: count,
  currency: text,
  status: enumOf(ORDER_STATUSES),
  invoice_state: z.string().nullable().default(null).transform(parseOrderInvoiceState),
  transfer_code: text,
  expires_at: text,
  created_at: text,
  paid_at: nullableText,
  buyer: BuyerSchema.prefault({}),
});
export type Order = z.output<typeof OrderSchema>;

export const OrderListSchema = z.object({
  orders: z.array(OrderSchema).default([]),
});
export type OrderList = z.output<typeof OrderListSchema>;

export const OrderActionResultSchema = z.object({
  order: OrderSchema,
  applied: z.boolean().default(true),
});
export type OrderActionResult = z.output<typeof OrderActionResultSchema>;

// Payment notices

export const NOTICE_STATES = ["received", "needs_review", "applied", "resolved", "dismissed"] as const;
export type NoticeState = (typeof NOTICE_STATES)[number] | "unknown";

export const NoticeSchema = z.object({
  id: z.string(),
  provider: text,
  provider_event_id: text,
  state: enumOf(NOTICE_STATES),
  transfer_code: text,
  amount: count,
  currency: text,
  review_reason: text,
  order_id: text,
  received_at: text,
  decided_at: nullableText,
});
export type Notice = z.output<typeof NoticeSchema>;

export const NoticeCandidateSchema = z.object({
  order_id: z.string(),
  workspace_id: text,
  state: text,
  transfer_code: text,
  amount: count,
  vat_amount: count,
  currency: text,
  created_at: text,
  match: text,
});
export type NoticeCandidate = z.output<typeof NoticeCandidateSchema>;

export const NoticeDetailSchema = NoticeSchema.extend({
  candidates: z.array(NoticeCandidateSchema).default([]),
});
export type NoticeDetail = z.output<typeof NoticeDetailSchema>;

export const NoticeListSchema = z.object({
  notices: z.array(NoticeSchema).default([]),
});
export type NoticeList = z.output<typeof NoticeListSchema>;

// Subscriptions & Plans

export const LimitsSchema = z.record(z.string(), z.number().nullable()).default({});
export type Limits = z.output<typeof LimitsSchema>;

export const SubscriptionSchema = z.object({
  workspace_id: z.string(),
  plan_key: text,
  status: text,
  seats: count,
  interval: text,
  current_period_start: nullableText,
  current_period_end: nullableText,
  trial_end: nullableText,
  grace_until: nullableText,
  cancel_at_period_end: z.boolean().default(false),
  limit_overrides: LimitsSchema,
  version: z.number().default(0),
  updated_at: nullableText,
});
export type Subscription = z.output<typeof SubscriptionSchema>;

export const PutSubscriptionResponseSchema = z.object({
  changed: z.boolean().default(false),
  subscription: SubscriptionSchema,
});
export type PutSubscriptionResponse = z.output<typeof PutSubscriptionResponseSchema>;

export const PLAN_AUDIENCES = ["any", "individual", "business"] as const;
export const PLAN_BILLING_MODELS = ["seat", "usage"] as const;
export const PLAN_STATES = ["active", "inactive"] as const;

export type PlanAudience = (typeof PLAN_AUDIENCES)[number] | "unknown";
export type PlanBillingModel = (typeof PLAN_BILLING_MODELS)[number] | "unknown";
export type PlanState = (typeof PLAN_STATES)[number] | "unknown";

function enumWithDefault<const T extends readonly string[]>(values: T, fallback: T[number]) {
  return z
    .string()
    .default(fallback)
    .transform((v): T[number] | "unknown" => (values.includes(v) ? v : "unknown"));
}

export const PlanSchema = z.object({
  key: z.string(),
  name: text,
  description: text,
  limits: LimitsSchema,
  features: z.record(z.string(), z.boolean()).default({}),
  currency: text,
  seat_price_month: z.number().nullable().default(null),
  seat_price_year: z.number().nullable().default(null),
  seats_fixed: z.number().nullable().default(null),
  min_seats: z.number().nullable().default(null),
  trial_days: count,
  is_public: z.boolean().default(false),
  sort: count,
  archived_at: nullableText,
  state: optionalEnumOf(PLAN_STATES),
  audience: enumWithDefault(PLAN_AUDIENCES, "any"),
  billing_model: enumWithDefault(PLAN_BILLING_MODELS, "seat"),
  version: z.number().optional(),
});
export type Plan = z.output<typeof PlanSchema>;

export const PlanListSchema = z.object({
  plans: z.array(PlanSchema).default([]),
});
export type PlanList = z.output<typeof PlanListSchema>;

export const PlanDetailResponseSchema = z.object({
  changed: z.boolean().optional(),
  plan: PlanSchema,
});
export type PlanDetailResponse = z.output<typeof PlanDetailResponseSchema>;

export const PutPlanResponseSchema = z.object({
  changed: z.boolean().default(false),
  plan: PlanSchema,
});
export type PutPlanResponse = z.output<typeof PutPlanResponseSchema>;

export const DeletePlanResponseSchema = z.object({
  deleted: z.boolean(),
});
export type DeletePlanResponse = z.output<typeof DeletePlanResponseSchema>;

export const PlanHistoryChangeSchema = z.object({
  field: z.string(),
  old: z.unknown().default(null),
  new: z.unknown().default(null),
});
export type PlanHistoryChange = z.output<typeof PlanHistoryChangeSchema>;

export const PlanHistoryEntrySchema = z.object({
  at: z.string(),
  actor: text,
  action: enumOf(AUDIT_ACTIONS),
  changes: z.array(PlanHistoryChangeSchema).default([]),
  reason: text,
});
export type PlanHistoryEntry = z.output<typeof PlanHistoryEntrySchema>;

export const PlanHistorySchema = z.object({
  history: z.array(PlanHistoryEntrySchema).default([]),
});
export type PlanHistory = z.output<typeof PlanHistorySchema>;

/**
 * The answer to verify-code: `mfa_required` asks for the authenticator code
 * next. A signed-in answer carries no such field.
 */
export const VerifyCodeResponseSchema = z.object({
  mfa_required: z.boolean().default(false),
});
export type VerifyCodeResponse = z.output<typeof VerifyCodeResponseSchema>;

export const ResetMfaResponseSchema = z.object({
  reset: z.boolean(),
});
export type ResetMfaResponse = z.output<typeof ResetMfaResponseSchema>;

export const WorkspaceBillingProfileSchema = z.object({
  account_kind: enumOf(ACCOUNT_KINDS),
  verified: z.boolean().default(false),
  verified_at: nullableText,
});
export type WorkspaceBillingProfile = z.output<typeof WorkspaceBillingProfileSchema>;

export const BillingProfileSchema = z.object({
  workspace_id: text,
  account_kind: enumOf(ACCOUNT_KINDS),
  legal_name: text,
  tax_code: text,
  address: text,
  invoice_email: text,
  verified: z.boolean(),
  verified_at: nullableText,
});
export type BillingProfile = z.output<typeof BillingProfileSchema>;

export const BillingProfileVerifyResponseSchema = z.object({
  workspace_id: text,
  account_kind: enumOf(ACCOUNT_KINDS),
  verified: z.boolean(),
  verified_at: nullableText,
});
export type BillingProfileVerifyResponse = z.output<typeof BillingProfileVerifyResponseSchema>;

export const PutBillingProfileResponseSchema = BillingProfileSchema.extend({
  changed: z.boolean().default(false),
});
export type PutBillingProfileResponse = z.output<typeof PutBillingProfileResponseSchema>;

export const WorkspaceListItemSchema = z.object({
  id: z.string(),
  name: text,
  slug: text,
  tenant_id: text,
  tenant_name: text,
  status: enumOf(WORKSPACE_STATUSES),
  plan_key: text,
  member_count: count,
  owner_email: text,
  created_at: text,
  last_activity_at: text,
});
export type WorkspaceListItem = z.output<typeof WorkspaceListItemSchema>;

export const WorkspacePageSchema = z.object({
  workspaces: z.array(WorkspaceListItemSchema).default([]),
  next_cursor: nextCursor,
});
export type WorkspacePage = z.output<typeof WorkspacePageSchema>;

export const WorkspaceTenantSummarySchema = z.object({
  id: z.string(),
  name: text,
});
export type WorkspaceTenantSummary = z.output<typeof WorkspaceTenantSummarySchema>;

export const WorkspaceSeatsSchema = z.object({
  purchased: count,
  used: count,
  held: count,
});
export type WorkspaceSeats = z.output<typeof WorkspaceSeatsSchema>;

export const WorkspaceStatusHistoryEntrySchema = z.object({
  id: text,
  status: enumOf(WORKSPACE_STATUSES),
  reason: text,
  actor: text,
  created_at: text,
});
export type WorkspaceStatusHistoryEntry = z.output<typeof WorkspaceStatusHistoryEntrySchema>;

export const WorkspaceExportResponseSchema = z.object({
  export_id: z.string(),
  status: text,
});
export type WorkspaceExportResponse = z.output<typeof WorkspaceExportResponseSchema>;

export const WorkspaceDetailSchema = WorkspaceSchema.extend({
  state: WorkspaceStateSchema.prefault({}),
  notes_count: count,
  group_count: count,
  owner: z.object({ id: z.string(), email: text }).nullable().default(null),
  subscription: SubscriptionSchema.nullable().default(null),
  billing_profile: WorkspaceBillingProfileSchema.nullable().optional(),
  recent_orders: z.array(OrderSchema).default([]),
  usage_current: UsageCurrentSchema.optional(),
  tenant: WorkspaceTenantSummarySchema.optional(),
  seats: WorkspaceSeatsSchema.optional(),
  status_history: z.array(WorkspaceStatusHistoryEntrySchema).optional(),
  allowed: z.array(z.string()).optional(),
});
export type WorkspaceDetail = z.output<typeof WorkspaceDetailSchema>;

export const WorkspaceUsageRowSchema = z.object({
  day: z.string(),
  meter: z.string(),
  quantity: z.number(),
  unit_price: z.number().default(0),
  per: z.number().default(1),
  discount_pct: z.number().default(0),
  net_amount: z.number(),
  note: text,
  order_id: nullableText,
});
export type WorkspaceUsageRow = z.output<typeof WorkspaceUsageRowSchema>;

export const WorkspaceUsageUnpaidOrderSchema = z.object({
  id: z.string(),
  amount: z.number(),
  vat_amount: count,
  status: enumOf(ORDER_STATUSES),
  created_at: text,
  period_start: nullableText,
  period_end: nullableText,
});
export type WorkspaceUsageUnpaidOrder = z.output<typeof WorkspaceUsageUnpaidOrderSchema>;

export const WorkspaceUsageSchema = z.object({
  workspace_id: z.string(),
  period_start: z.string(),
  period_end: z.string(),
  rows: z.array(WorkspaceUsageRowSchema),
  missing_days: z.array(z.string()),
  unpaid_orders: z.array(WorkspaceUsageUnpaidOrderSchema),
});
export type WorkspaceUsage = z.output<typeof WorkspaceUsageSchema>;

// AI spend of a workspace over one window (GET .../ai-cost). cost_usd is the
// provider-reported charge plus the rate-table estimate; complete is false when
// some tokens had no rate, so the figure is a floor.
export const WorkspaceAICostSchema = z.object({
  window: text,
  since: text,
  until: text,
  cost_usd: z.number().default(0),
  complete: z.boolean().default(true),
});
export type WorkspaceAICost = z.output<typeof WorkspaceAICostSchema>;

// A tenant holds one or more workspaces (spec R6 section 5). max_workspaces is
// the tenant's effective limit; null, like an absent value, is no limit (I9).
export const TenantSchema = z.object({
  id: z.string(),
  name: text,
  status: enumOf(WORKSPACE_STATUSES),
  workspace_count: count,
  max_workspaces: z.number().nullable().default(null),
  best_plan_key: text,
  status_summary: z.record(z.string(), z.number()).default({}),
  created_at: text,
  last_activity_at: text,
});
export type Tenant = z.output<typeof TenantSchema>;

// One workspace of a tenant: names and counts only.
export const TenantWorkspaceSchema = z.object({
  id: z.string(),
  name: text,
  slug: text,
  status: enumOf(WORKSPACE_STATUSES),
  plan_key: text,
  member_count: count,
  owner: z.object({ id: z.string(), email: text }).nullable().default(null),
  created_at: text,
  ...usageFields,
});
export type TenantWorkspace = z.output<typeof TenantWorkspaceSchema>;

export const TenantDetailSchema = TenantSchema.extend({
  created_by: nullableText,
  workspaces: z.array(TenantWorkspaceSchema).default([]),
  // Pending optional fields not yet returned by GET /tenants/{id} on main.
  status_reason: z.string().optional(),
  legal_hold: z.boolean().optional(),
  legal_hold_id: z.string().nullable().optional(),
});
export type TenantDetail = z.output<typeof TenantDetailSchema>;

export const TenantPageSchema = z.object({
  tenants: z.array(TenantSchema).default([]),
  next_cursor: nextCursor,
});
export type TenantPage = z.output<typeof TenantPageSchema>;

// Users

export const UserSchema = z.object({
  id: z.string(),
  email: text,
  name: text,
  status: enumOf(ACCOUNT_STATUSES),
  memberships_count: count,
  created_at: text,
});
export type User = z.output<typeof UserSchema>;

const MembershipSchema = z.object({
  workspace_id: z.string(),
  name: text,
  // The workspace role is Delegora's, not the console's; shown as it comes.
  role: text,
});
export type Membership = z.output<typeof MembershipSchema>;

const AccountRecordSchema = z.object({
  status: enumOf(ACCOUNT_STATUSES),
  reason: text,
  changed_at: nullableText,
  sessions_valid_after: nullableText,
});
export type AccountRecord = z.output<typeof AccountRecordSchema>;

export const UserDetailSchema = UserSchema.extend({
  memberships: z.array(MembershipSchema).default([]),
  account: AccountRecordSchema.prefault({}),
  tenant_id: z.string().nullable().optional(),
  allocation: z.array(z.lazy(() => TenantUserAllocationItemSchema)).optional(),
  audit_events: z.array(z.lazy(() => AuditEventSchema)).optional(),
});
export type UserDetail = z.output<typeof UserDetailSchema>;

export const UserPageSchema = z.object({
  users: z.array(UserSchema).default([]),
  next_cursor: nextCursor,
});
export type UserPage = z.output<typeof UserPageSchema>;

// Notes

export const NoteSchema = z.object({
  id: z.string(),
  workspace_id: text,
  author_id: text,
  author_email: nullableText,
  body: text,
  created_at: text,
});
export type Note = z.output<typeof NoteSchema>;

export const NotePageSchema = z.object({
  notes: z.array(NoteSchema).default([]),
  next_cursor: nextCursor,
});
export type NotePage = z.output<typeof NotePageSchema>;

// Staff

export const StaffMemberSchema = z.object({
  user_id: z.string(),
  email: text,
  role: enumOf(STAFF_ROLES),
  tenant_id: nullableText,
  tenant_name: nullableText,
  created_by: nullableText,
  created_at: text,
});
export type StaffMember = z.output<typeof StaffMemberSchema>;

export const StaffListSchema = z.object({
  staff: z.array(StaffMemberSchema).default([]),
});
export type StaffList = z.output<typeof StaffListSchema>;

// Audit events

export const AuditEventSchema = z.object({
  id: z.string(),
  workspace_id: nullableText,
  actor_type: enumOf(ACTOR_TYPES),
  actor_id: nullableText,
  actor_label: text,
  // Shown as its catalog code; a code the console does not list is still shown.
  action: text,
  target_type: text,
  target_id: text,
  target_label: text,
  // The detail is free-form JSON; anything but an object is dropped, not fatal.
  detail: z.record(z.string(), z.unknown()).catch({}),
  ip: text,
  user_agent: text,
  request_id: text,
  created_at: text,
});
export type AuditEvent = z.output<typeof AuditEventSchema>;

export const AuditPageSchema = z.object({
  events: z.array(AuditEventSchema).default([]),
  next_cursor: nextCursor,
});
export type AuditPage = z.output<typeof AuditPageSchema>;

// Overview

// One UTC day of the overview's 30-day series. Usage meters are per-day flows.
const DailyCountPointSchema = z.object({
  day: z.string(),
  workspaces_created: count,
  orders_created: count,
  orders_paid: count,
  usage_issues_created: count,
  usage_tasks_run: count,
});

// Items waiting in the operations queue, counted as the list screens filter them.
const OperationsQueueSchema = z.object({
  pending_approvals: count,
  pending_deletions: count,
  pending_quota_requests: count,
  failed_einvoices: count,
  active_legal_holds: count,
});
const EMPTY_OPERATIONS_QUEUE = {
  pending_approvals: 0,
  pending_deletions: 0,
  pending_quota_requests: 0,
  failed_einvoices: 0,
  active_legal_holds: 0,
};

export const OverviewSchema = z.object({
  workspaces_by_status: z
    .record(z.string(), z.number())
    .default({})
    .transform((byStatus) => ({
      active: 0,
      read_only: 0,
      suspended: 0,
      pending_deletion: 0,
      ...byStatus,
    })),
  signups_7d: count,
  pending_orders: z.number().optional(),
  // Pending orders per currency code; amounts are minor units of that currency.
  pending_by_currency: z
    .record(
      z.string(),
      z.object({
        orders: z.number().int().nonnegative(),
        amount: z.number().int().nonnegative(),
        vat_amount: z.number().int().nonnegative(),
      }),
    )
    .catch({})
    .optional(),
  by_plan: z.record(z.string(), z.number()).optional(),
  // Totals of the newest usage row of each workspace that has one.
  usage_day: z.string().nullable().optional(),
  usage_workspaces: z.number().optional(),
  usage_members: z.number().optional(),
  usage_agents: z.number().optional(),
  usage_runtimes: z.number().optional(),
  usage_issues_total: z.number().optional(),
  usage_issues_created: z.number().optional(),
  usage_tasks_run: z.number().optional(),
  usage_storage_bytes: z.number().optional(),
  // Additions: an older server sends none of them. A malformed one falls back
  // to its empty value so the rest of the overview still renders.
  daily_series: z.array(DailyCountPointSchema).catch([]),
  runtimes_by_status: z
    .record(z.string(), z.number())
    .catch({})
    .transform((byStatus) => ({ online: 0, offline: 0, ...byStatus })),
  operations: OperationsQueueSchema.catch(EMPTY_OPERATIONS_QUEUE),
});
export type Overview = z.output<typeof OverviewSchema>;
export type DailyCountPoint = z.output<typeof DailyCountPointSchema>;

// Parsers. A list that fails to parse becomes an empty page. A single record
// has no honest empty value, so its fallback is null and the hook turns that
// into an error state instead of showing blank data.

export function parseOrderList(data: unknown): OrderList {
  return parseWithFallback<OrderList>(data, OrderListSchema, { orders: [] }, {
    endpoint: "GET /api/platform/orders",
  });
}

export function parseOrderActionResult(data: unknown, endpoint: string): OrderActionResult | null {
  return parseWithFallback<OrderActionResult | null>(data, OrderActionResultSchema, null, { endpoint });
}

export function parseNoticeList(data: unknown): NoticeList {
  return parseWithFallback<NoticeList>(data, NoticeListSchema, { notices: [] }, {
    endpoint: "GET /api/platform/payment-notices",
  });
}

export function parseNoticeDetail(data: unknown): NoticeDetail | null {
  return parseWithFallback<NoticeDetail | null>(data, NoticeDetailSchema, null, {
    endpoint: "GET /api/platform/payment-notices/{id}",
  });
}

export function parseNotice(data: unknown, endpoint: string): Notice | null {
  return parseWithFallback<Notice | null>(data, NoticeSchema, null, { endpoint });
}

export function parsePutSubscriptionResponse(data: unknown): PutSubscriptionResponse | null {
  return parseWithFallback<PutSubscriptionResponse | null>(data, PutSubscriptionResponseSchema, null, {
    endpoint: "PUT /api/platform/workspaces/{id}/subscription",
  });
}

export function parsePlanList(data: unknown): PlanList {
  return parseWithFallback<PlanList>(data, PlanListSchema, { plans: [] }, {
    endpoint: "GET /api/platform/plans",
  });
}

export function parsePlanDetail(
  data: unknown,
  endpoint = "GET /api/platform/plans/{key}",
): PlanDetailResponse | null {
  return parseWithFallback<PlanDetailResponse | null>(data, PlanDetailResponseSchema, null, {
    endpoint,
  });
}

export function parsePutPlanResponse(data: unknown): PutPlanResponse | null {
  return parseWithFallback<PutPlanResponse | null>(data, PutPlanResponseSchema, null, {
    endpoint: "PUT /api/platform/plans/{key}",
  });
}

export function parseDeletePlanResponse(data: unknown): DeletePlanResponse | null {
  return parseWithFallback<DeletePlanResponse | null>(data, DeletePlanResponseSchema, null, {
    endpoint: "DELETE /api/platform/plans/{key}",
  });
}

export function parsePlanHistory(data: unknown): PlanHistory {
  return parseWithFallback<PlanHistory>(data, PlanHistorySchema, { history: [] }, {
    endpoint: "GET /api/platform/plans/{key}/history",
  });
}

export function parseVerifyCodeResponse(data: unknown): VerifyCodeResponse | null {
  return parseWithFallback<VerifyCodeResponse | null>(data, VerifyCodeResponseSchema, null, {
    endpoint: "POST /api/platform/auth/verify-code",
  });
}

export function parseResetMfaResponse(data: unknown): ResetMfaResponse | null {
  return parseWithFallback<ResetMfaResponse | null>(data, ResetMfaResponseSchema, null, {
    endpoint: "POST /api/platform/users/{id}/mfa-reset",
  });
}

export function parseMe(data: unknown): Me | null {
  return parseWithFallback<Me | null>(data, MeSchema, null, { endpoint: "GET /api/platform/me" });
}

export function parseTenantPage(data: unknown): TenantPage {
  return parseWithFallback<TenantPage>(data, TenantPageSchema, { tenants: [], next_cursor: null }, {
    endpoint: "GET /api/platform/tenants",
  });
}

export function parseTenantDetail(data: unknown): TenantDetail | null {
  return parseWithFallback<TenantDetail | null>(data, TenantDetailSchema, null, {
    endpoint: "GET /api/platform/tenants/{id}",
  });
}

export function parseWorkspacePage(data: unknown): WorkspacePage {
  return parseWithFallback<WorkspacePage>(
    data,
    WorkspacePageSchema,
    { workspaces: [], next_cursor: null },
    { endpoint: "GET /api/platform/workspaces" },
  );
}

export function parseWorkspaceDetail(data: unknown): WorkspaceDetail | null {
  return parseWithFallback<WorkspaceDetail | null>(data, WorkspaceDetailSchema, null, {
    endpoint: "GET /api/platform/workspaces/{id}",
  });
}

export function parseWorkspaceExportResponse(data: unknown): WorkspaceExportResponse | null {
  return parseWithFallback<WorkspaceExportResponse | null>(
    data,
    WorkspaceExportResponseSchema,
    null,
    { endpoint: "POST /api/platform/workspaces/{id}/export" },
  );
}

export function parseBillingProfile(data: unknown): BillingProfile | null {
  return parseWithFallback<BillingProfile | null>(data, BillingProfileSchema, null, {
    endpoint: "GET /api/platform/workspaces/{id}/billing-profile",
  });
}

export function parseBillingProfileVerifyResponse(
  data: unknown,
  endpoint = "POST /api/platform/workspaces/{id}/billing-profile/verify",
): BillingProfileVerifyResponse | null {
  return parseWithFallback<BillingProfileVerifyResponse | null>(
    data,
    BillingProfileVerifyResponseSchema,
    null,
    { endpoint },
  );
}

export function parsePutBillingProfileResponse(data: unknown): PutBillingProfileResponse | null {
  return parseWithFallback<PutBillingProfileResponse | null>(
    data,
    PutBillingProfileResponseSchema,
    null,
    { endpoint: "PUT /api/platform/workspaces/{id}/billing-profile" },
  );
}

export function parseWorkspaceUsage(data: unknown): WorkspaceUsage | null {
  return parseWithFallback<WorkspaceUsage | null>(data, WorkspaceUsageSchema, null, {
    endpoint: "GET /api/platform/workspaces/{id}/usage",
  });
}

export function parseWorkspaceAICost(data: unknown): WorkspaceAICost | null {
  return parseWithFallback<WorkspaceAICost | null>(data, WorkspaceAICostSchema, null, {
    endpoint: "GET /api/platform/workspaces/{id}/ai-cost",
  });
}

export function parseUserPage(data: unknown): UserPage {
  return parseWithFallback<UserPage>(data, UserPageSchema, { users: [], next_cursor: null }, {
    endpoint: "GET /api/platform/users",
  });
}

export function parseUserDetail(data: unknown): UserDetail | null {
  return parseWithFallback<UserDetail | null>(data, UserDetailSchema, null, {
    endpoint: "GET /api/platform/users/{id}",
  });
}

export function parseNotePage(data: unknown): NotePage {
  return parseWithFallback<NotePage>(data, NotePageSchema, { notes: [], next_cursor: null }, {
    endpoint: "GET /api/platform/workspaces/{id}/notes",
  });
}

export function parseStaffList(data: unknown): StaffList {
  return parseWithFallback<StaffList>(data, StaffListSchema, { staff: [] }, {
    endpoint: "GET /api/platform/staff",
  });
}

export function parseAuditPage(data: unknown, endpoint: string): AuditPage {
  return parseWithFallback<AuditPage>(data, AuditPageSchema, { events: [], next_cursor: null }, { endpoint });
}

export function parseOverview(data: unknown): Overview | null {
  return parseWithFallback<Overview | null>(data, OverviewSchema, null, {
    endpoint: "GET /api/platform/overview",
  });
}

// Account requests (spec 7b.5)

export const ACCOUNT_REQUEST_STATES = [
  "pending",
  "provisioned",
  "completed",
  "failed",
  "canceled",
] as const;
export const BILLING_INTERVALS = ["month", "year"] as const;

export type AccountKind = (typeof ACCOUNT_KINDS)[number] | "unknown";
export type AccountRequestState = (typeof ACCOUNT_REQUEST_STATES)[number] | "unknown";
export type BillingInterval = (typeof BILLING_INTERVALS)[number] | "unknown";

export const AccountRequestSchema = z.object({
  id: z.string(),
  kind: enumOf(ACCOUNT_KINDS),
  email: text,
  display_name: text,
  workspace_name: text,
  workspace_slug: text,
  plan_key: text,
  seats: count,
  interval: enumOf(BILLING_INTERVALS),
  trial: z.boolean().default(false),
  comp_reason: text,
  state: enumOf(ACCOUNT_REQUEST_STATES),
  error_code: text,
  attempts: count,
  user_id: text,
  workspace_id: text,
  created_by: text,
  created_at: text,
  updated_at: text,
  completed_at: nullableText,
  welcome_sent_at: nullableText,
  welcome_attempts: count,
  logged_in: z.boolean().nullable().default(null),
});
export type AccountRequest = z.output<typeof AccountRequestSchema>;

export const AccountRequestListSchema = z.object({
  requests: z.array(AccountRequestSchema),
});
export type AccountRequestList = z.output<typeof AccountRequestListSchema>;

export const ResendWelcomeResponseSchema = z.union([
  AccountRequestSchema,
  z.object({
    sent: z.boolean().optional(),
    welcome_sent_at: z.string().nullable().optional(),
    welcome_attempts: z.number().optional(),
  }),
]);
export type ResendWelcomeResponse = z.output<typeof ResendWelcomeResponseSchema>;

export function parseAccountRequestList(data: unknown): AccountRequestList | null {
  return parseWithFallback<AccountRequestList | null>(data, AccountRequestListSchema, null, {
    endpoint: "GET /api/platform/accounts",
  });
}

export function parseAccountRequest(
  data: unknown,
  endpoint = "POST /api/platform/accounts",
): AccountRequest | null {
  return parseWithFallback<AccountRequest | null>(data, AccountRequestSchema, null, {
    endpoint,
  });
}

export function parseResendWelcomeResponse(data: unknown): ResendWelcomeResponse | null {
  return parseWithFallback<ResendWelcomeResponse | null>(data, ResendWelcomeResponseSchema, null, {
    endpoint: "POST /api/platform/accounts/{id}/resend-welcome",
  });
}

// Price changes (spec 7b.4)

export const PRICE_CHANGE_STATES = ["scheduled", "applied", "canceled"] as const;
export type PriceChangeState = (typeof PRICE_CHANGE_STATES)[number] | "unknown";

export const PriceSnapshotSchema = z.record(z.string(), z.unknown());
export type PriceSnapshot = z.output<typeof PriceSnapshotSchema>;

export const PriceChangeSchema = z.object({
  id: z.string(),
  plan_key: z.string(),
  state: z
    .string()
    .transform((v): PriceChangeState =>
      (PRICE_CHANGE_STATES as readonly string[]).includes(v) ? (v as PriceChangeState) : "unknown",
    ),
  effective_from: z.string(),
  old: PriceSnapshotSchema,
  new: PriceSnapshotSchema,
  reason: text,
  notice_sent_at: nullableText,
  created_by: text,
  created_at: z.string(),
  applied_at: nullableText,
  canceled_by: text,
  canceled_at: nullableText,
});
export type PriceChange = z.output<typeof PriceChangeSchema>;

export const PriceChangeListSchema = z.object({
  price_changes: z.array(PriceChangeSchema),
});
export type PriceChangeList = z.output<typeof PriceChangeListSchema>;

export const PriceChangeDryRunResponseSchema = z.object({
  dry_run: z.boolean(),
  affected_subscribers: z.number().int().nonnegative(),
  notice_date: nullableText,
});
export type PriceChangeDryRunResponse = z.output<typeof PriceChangeDryRunResponseSchema>;

export const SchedulePriceChangeResponseSchema = z.object({
  price_change: PriceChangeSchema,
  applied: z.boolean(),
  affected_subscribers: z.number().int().nonnegative().default(0),
  notice_date: z.string().default(""),
});
export type SchedulePriceChangeResponse = z.output<typeof SchedulePriceChangeResponseSchema>;

export const CancelPriceChangeResponseSchema = z.object({
  price_change: PriceChangeSchema,
});
export type CancelPriceChangeResponse = z.output<typeof CancelPriceChangeResponseSchema>;

export function parsePriceChangeList(data: unknown): PriceChangeList {
  return parseWithFallback<PriceChangeList>(data, PriceChangeListSchema, { price_changes: [] }, {
    endpoint: "GET /api/platform/plans/{key}/price-changes",
  });
}

export function parsePriceChangeDryRunResponse(data: unknown): PriceChangeDryRunResponse | null {
  return parseWithFallback<PriceChangeDryRunResponse | null>(
    data,
    PriceChangeDryRunResponseSchema,
    null,
    { endpoint: "POST /api/platform/plans/{key}/price-changes?dry_run=1" },
  );
}

export function parseSchedulePriceChangeResponse(data: unknown): SchedulePriceChangeResponse | null {
  return parseWithFallback<SchedulePriceChangeResponse | null>(
    data,
    SchedulePriceChangeResponseSchema,
    null,
    { endpoint: "POST /api/platform/plans/{key}/price-changes" },
  );
}

export function parseCancelPriceChangeResponse(data: unknown): CancelPriceChangeResponse | null {
  return parseWithFallback<CancelPriceChangeResponse | null>(
    data,
    CancelPriceChangeResponseSchema,
    null,
    { endpoint: "DELETE /api/platform/plans/{key}/price-changes/scheduled" },
  );
}

// E-invoices (spec 7.3, 7.6)

export const EINVOICE_STATES = [
  "pending",
  "issued",
  "needs_info",
  "failed",
  "cancelled",
] as const;
export type EinvoiceState = (typeof EINVOICE_STATES)[number] | "unknown";

export function isSafeHttpsUrl(value: string): boolean {
  const trimmed = value.trim();
  if (!trimmed.startsWith("https://")) return false;
  try {
    return new URL(trimmed).protocol === "https:";
  } catch {
    return false;
  }
}

function sanitizeHttpsUrl(value: string): string {
  return isSafeHttpsUrl(value) ? value.trim() : "";
}

export const EinvoiceSchema = z.object({
  order_id: z.string().min(1),
  workspace_id: text,
  revision: z.number().default(1),
  state: enumOf(EINVOICE_STATES),
  currency: text,
  amount: count,
  vat_amount: count,
  provider: text,
  invoice_number: text,
  lookup_url: text.transform(sanitizeHttpsUrl),
  attempts: count,
  last_error_code: text,
  missing_fields: z.array(z.string()).default([]),
  next_attempt_at: text,
  updated_at: text,
});
export type Einvoice = z.output<typeof EinvoiceSchema>;

export const EinvoiceListSchema = z.object({
  einvoices: z.array(EinvoiceSchema).default([]),
});
export type EinvoiceList = z.output<typeof EinvoiceListSchema>;

export function parseEinvoiceList(data: unknown): EinvoiceList {
  return parseWithFallback<EinvoiceList>(data, EinvoiceListSchema, { einvoices: [] }, {
    endpoint: "GET /api/platform/einvoices",
  });
}

export type OrderEinvoiceState = OrderInvoiceState | "unknown";

export const OrderEinvoiceSchema = z.object({
  revision: z.number().int().nonnegative(),
  state: enumOf(ORDER_INVOICE_STATES),
  invoice_number: text,
  lookup_url: text.transform(sanitizeHttpsUrl),
  buyer: BuyerSchema.prefault({}),
  attempts: count,
  last_error_code: text,
  created_at: text,
  issued_at: nullableText,
  updated_at: text,
});
export type OrderEinvoice = z.output<typeof OrderEinvoiceSchema>;

export const OrderEinvoiceListSchema = z.object({
  einvoices: z.array(OrderEinvoiceSchema),
});
export type OrderEinvoiceList = z.output<typeof OrderEinvoiceListSchema>;

export function parseOrderEinvoiceList(data: unknown): OrderEinvoiceList | null {
  return parseWithFallback<OrderEinvoiceList | null>(data, OrderEinvoiceListSchema, null, {
    endpoint: "GET /api/platform/orders/{id}/einvoices",
  });
}

// Staff update, member requests, deletion requests, and account deletion preview (R6 / S13)

const nullableErrorText = z
  .union([z.string(), z.null()])
  .optional()
  .transform((v) => v ?? "");

export const UpdateStaffResponseSchema = z.object({
  user_id: z.string(),
  role: enumOf(STAFF_ROLES),
  tenant_id: nullableText,
});
export type UpdateStaffResponse = z.output<typeof UpdateStaffResponseSchema>;

export function parseUpdateStaffResponse(data: unknown): UpdateStaffResponse | null {
  return parseWithFallback<UpdateStaffResponse | null>(data, UpdateStaffResponseSchema, null, {
    endpoint: "PUT /api/platform/staff/{user_id}",
  });
}

export const MEMBER_ROLES = ["admin", "member", "viewer"] as const;
export type MemberRole = (typeof MEMBER_ROLES)[number];

export const MEMBER_REQUEST_STATES = ["pending", "completed", "failed", "canceled"] as const;
export type MemberRequestState = (typeof MEMBER_REQUEST_STATES)[number] | "unknown";

export const MemberRequestSchema = z.object({
  id: z.string(),
  workspace_id: text,
  kind: text,
  email: text,
  role: text,
  state: enumOf(MEMBER_REQUEST_STATES),
  error_code: nullableErrorText,
  attempts: count,
  user_id: text,
  member_id: text,
  user_created: z.boolean().default(false),
  requested_by: text,
  requested_by_label: text,
  created_at: text,
  updated_at: text,
  completed_at: nullableText,
});
export type MemberRequest = z.output<typeof MemberRequestSchema>;

export const MemberRequestListSchema = z.object({
  requests: z.array(MemberRequestSchema).default([]),
});
export type MemberRequestList = z.output<typeof MemberRequestListSchema>;

export function parseMemberRequest(data: unknown, endpoint: string): MemberRequest | null {
  return parseWithFallback<MemberRequest | null>(data, MemberRequestSchema, null, { endpoint });
}

export function parseMemberRequestList(data: unknown): MemberRequestList {
  return parseWithFallback<MemberRequestList>(data, MemberRequestListSchema, { requests: [] }, {
    endpoint: "GET /api/platform/workspaces/{id}/member-requests",
  });
}

export const DELETION_KINDS = ["workspace", "tenant", "account"] as const;
export type DeletionKind = (typeof DELETION_KINDS)[number] | "unknown";

export const DELETION_REQUEST_STATES = [
  "awaiting_approval",
  "pending",
  "running",
  "completed",
  "failed",
  "canceled",
  "legal_hold",
  "export_timeout",
] as const;
export type DeletionRequestState = (typeof DELETION_REQUEST_STATES)[number] | "unknown";

export const DeletionRequestSchema = z.object({
  id: z.string(),
  kind: enumOf(DELETION_KINDS),
  target_id: text,
  target_label: text,
  reason: text,
  state: enumOf(DELETION_REQUEST_STATES),
  error_code: nullableErrorText,
  attempts: count,
  tenant_id: nullableText,
  requested_by: text,
  requested_by_label: text,
  created_at: text,
  updated_at: text,
  export_first: z.boolean().optional(),
  approved_by: z.string().nullable().optional(),
  approved_at: z.string().nullable().optional(),
});
export type DeletionRequest = z.output<typeof DeletionRequestSchema>;

export const DeletionRequestListSchema = z.object({
  requests: z.array(DeletionRequestSchema).default([]),
});
export type DeletionRequestList = z.output<typeof DeletionRequestListSchema>;

export function parseDeletionRequest(data: unknown, endpoint: string): DeletionRequest | null {
  return parseWithFallback<DeletionRequest | null>(data, DeletionRequestSchema, null, { endpoint });
}

export function parseDeletionRequestList(data: unknown): DeletionRequestList {
  return parseWithFallback<DeletionRequestList>(data, DeletionRequestListSchema, { requests: [] }, {
    endpoint: "GET /api/platform/deletion-requests",
  });
}

export const DeletionStartResponseSchema = z.object({
  request_id: z.string(),
  state: enumOf(DELETION_REQUEST_STATES),
});
export type DeletionStartResponse = z.output<typeof DeletionStartResponseSchema>;

export function parseDeletionStartResponse(data: unknown, endpoint: string): DeletionStartResponse | null {
  return parseWithFallback<DeletionStartResponse | null>(data, DeletionStartResponseSchema, null, { endpoint });
}

export const AccountPreviewSuccessorSchema = z.object({
  user_id: z.string(),
  email: text,
  name: text,
  role: text,
  status: z.string().default("active"),
});
export type AccountPreviewSuccessor = z.output<typeof AccountPreviewSuccessorSchema>;

export const AccountPreviewWorkspaceSchema = z.object({
  workspace_id: z.string(),
  slug: text,
  role: text,
  owner_count: count,
  member_count: count,
  successors: z.array(AccountPreviewSuccessorSchema).default([]),
});
export type AccountPreviewWorkspace = z.output<typeof AccountPreviewWorkspaceSchema>;

export const AccountDeletePreviewSchema = z.object({
  email: text,
  need_choice: z.array(AccountPreviewWorkspaceSchema).default([]),
  deleted_with: z.array(AccountPreviewWorkspaceSchema).default([]),
});
export type AccountDeletePreview = z.output<typeof AccountDeletePreviewSchema>;

export function parseAccountDeletePreview(data: unknown): AccountDeletePreview | null {
  return parseWithFallback<AccountDeletePreview | null>(data, AccountDeletePreviewSchema, null, {
    endpoint: "GET /api/platform/users/{id}/delete-now-preview",
  });
}

// Tenant users, member changes, and quota requests (R6 / S24)

export const TENANT_MEMBER_ROLES = ["owner", "admin", "member", "viewer"] as const;
export type TenantMemberRole = (typeof TENANT_MEMBER_ROLES)[number] | "unknown";

export const TenantUserWorkspaceSchema = z.object({
  workspace_id: z.string(),
  name: text,
  member_role: enumOf(TENANT_MEMBER_ROLES),
});
export type TenantUserWorkspace = z.output<typeof TenantUserWorkspaceSchema>;

export const TenantUserSchema = z.object({
  user_id: z.string(),
  email: text,
  name: text,
  account_status: enumOf(ACCOUNT_STATUSES),
  is_staff: z.boolean().default(false),
  workspaces: z.array(TenantUserWorkspaceSchema).default([]),
  seats: count,
  last_login_at: nullableText,
});
export type TenantUser = z.output<typeof TenantUserSchema>;

export const TenantUserPageSchema = z.object({
  users: z.array(TenantUserSchema).default([]),
  next_cursor: nextCursor,
});
export type TenantUserPage = z.output<typeof TenantUserPageSchema>;

export const TenantUserAllocationItemSchema = z.object({
  workspace_id: z.string(),
  workspace_name: text,
  member_role: enumOf(TENANT_MEMBER_ROLES),
  occupies_seat: z.boolean().default(true),
  seats_purchased: z.number().nullable().default(null),
  seats_used: z.number().nullable().default(null),
  limits: LimitsSchema,
  usage: z.record(z.string(), z.number()).default({}),
});
export type TenantUserAllocationItem = z.output<typeof TenantUserAllocationItemSchema>;

export const TenantUserDetailSchema = TenantUserSchema.extend({
  allocation: z
    .array(TenantUserAllocationItemSchema)
    .nullish()
    .transform((v) => v ?? []),
  audit_events: z
    .array(AuditEventSchema)
    .nullish()
    .transform((v) => v ?? []),
});
export type TenantUserDetail = z.output<typeof TenantUserDetailSchema>;

export const DeactivateTenantUserResponseSchema = z.object({
  request_ids: z.array(z.string()).default([]),
});
export type DeactivateTenantUserResponse = z.output<typeof DeactivateTenantUserResponseSchema>;

export const MEMBER_CHANGE_KINDS = ["remove", "change_role"] as const;
export type MemberChangeKind = (typeof MEMBER_CHANGE_KINDS)[number] | "unknown";

export const MemberChangeResponseSchema = z.union([
  z.object({
    request_id: z.string(),
    kind: enumOf(MEMBER_CHANGE_KINDS),
    changed: z.boolean().optional(),
  }),
  z.object({
    changed: z.literal(false),
    request_id: z.string().optional(),
    kind: optionalEnumOf(MEMBER_CHANGE_KINDS),
  }),
]);
export type MemberChangeResponse = z.output<typeof MemberChangeResponseSchema>;

export const QUOTA_REQUEST_STATES = ["pending", "approved", "denied", "canceled"] as const;
export type QuotaRequestState = (typeof QUOTA_REQUEST_STATES)[number] | "unknown";

export const QuotaRequestSchema = z.object({
  id: z.string(),
  workspace_id: text,
  tenant_id: text,
  gate: text,
  current_value: z.number().nullable().default(null),
  requested_value: count,
  approved_value: z.number().nullable().default(null),
  state: enumOf(QUOTA_REQUEST_STATES),
  reason: text,
  decision_reason: text,
  requested_by: text,
  requested_by_label: text,
  decided_by: text,
  created_at: text,
  decided_at: nullableText,
});
export type QuotaRequest = z.output<typeof QuotaRequestSchema>;

export function parseTenantUserPage(data: unknown): TenantUserPage {
  return parseWithFallback<TenantUserPage>(
    data,
    TenantUserPageSchema,
    { users: [], next_cursor: null },
    { endpoint: "GET /api/platform/tenants/{id}/users" },
  );
}

export function parseTenantUserDetail(data: unknown): TenantUserDetail | null {
  return parseWithFallback<TenantUserDetail | null>(data, TenantUserDetailSchema, null, {
    endpoint: "GET /api/platform/tenants/{id}/users/{user_id}",
  });
}

export function parseDeactivateTenantUserResponse(
  data: unknown,
): DeactivateTenantUserResponse | null {
  return parseWithFallback<DeactivateTenantUserResponse | null>(
    data,
    DeactivateTenantUserResponseSchema,
    null,
    { endpoint: "POST /api/platform/tenants/{id}/users/{user_id}/deactivate" },
  );
}

export function parseMemberChangeResponse(
  data: unknown,
  endpoint: string,
): MemberChangeResponse | null {
  return parseWithFallback<MemberChangeResponse | null>(data, MemberChangeResponseSchema, null, {
    endpoint,
  });
}

export function parseQuotaRequest(data: unknown, endpoint: string): QuotaRequest | null {
  return parseWithFallback<QuotaRequest | null>(data, QuotaRequestSchema, null, { endpoint });
}

// Resources, limit overrides, and runtimes (R6 / S25)

function nullableArray<T extends z.ZodTypeAny>(item: T) {
  return z
    .array(item)
    .nullish()
    .transform((v) => v ?? []);
}

export const LIMIT_GATES = [
  "issue_count",
  "autopilot_runs",
  "agents",
  "runtimes",
  "storage_bytes",
  "api_rate_per_minute",
  "audit_retention_days",
  "member_groups",
  "max_workspaces",
] as const;
export type LimitGate = (typeof LIMIT_GATES)[number];

export const TenantLimitOverrideResponseSchema = z.object({
  gate: z.string(),
  workspace_ids: nullableArray(z.string()),
});
export type TenantLimitOverrideResponse = z.output<typeof TenantLimitOverrideResponseSchema>;

export function parseTenantLimitOverrideResponse(
  data: unknown,
): TenantLimitOverrideResponse | null {
  return parseWithFallback<TenantLimitOverrideResponse | null>(
    data,
    TenantLimitOverrideResponseSchema,
    null,
    { endpoint: "POST /api/platform/tenants/{id}/limit-overrides" },
  );
}

export const RESOURCE_STATES = ["ok", "warning", "over"] as const;
export type ResourceState = (typeof RESOURCE_STATES)[number] | "unknown";

export const ResourceUsageItemSchema = z
  .object({
    type: text,
    label: text,
    unit: text,
    used: count,
    limit: z.number().nullable().default(null),
    percent: z.number().nullable().default(null),
    state: optionalEnumOf(RESOURCE_STATES),
    status: optionalEnumOf(RESOURCE_STATES),
    trend_7d: nullableArray(z.number()),
  })
  .transform(({ status, state, ...rest }) => ({
    ...rest,
    state: state ?? status ?? ("unknown" as const),
  }));
export type ResourceUsageItem = z.output<typeof ResourceUsageItemSchema>;

export const TenantWorkspaceResourceSchema = z.object({
  workspace_id: text,
  workspace_name: text,
  plan_key: text,
  resources: nullableArray(ResourceUsageItemSchema),
});
export type TenantWorkspaceResource = z.output<typeof TenantWorkspaceResourceSchema>;

export const TenantResourcesSchema = z
  .object({
    tenant_id: text,
    resources: nullableArray(ResourceUsageItemSchema).optional(),
    totals: nullableArray(ResourceUsageItemSchema).optional(),
    workspaces: nullableArray(TenantWorkspaceResourceSchema),
    complete: z.boolean().default(false),
  })
  .transform(({ resources, totals, ...rest }) => ({
    ...rest,
    resources: resources ?? totals ?? [],
  }));
export type TenantResources = z.output<typeof TenantResourcesSchema>;

export function parseTenantResources(data: unknown): TenantResources | null {
  return parseWithFallback<TenantResources | null>(data, TenantResourcesSchema, null, {
    endpoint: "GET /api/platform/tenants/{id}/resources",
  });
}

export const SystemResourceTypeSummarySchema = z.object({
  type: text,
  label: text,
  unit: text,
  used: count,
  limit: z.number().nullable().default(null),
  percent: z.number().nullable().default(null),
});
export type SystemResourceTypeSummary = z.output<typeof SystemResourceTypeSummarySchema>;

export const SystemResourcePlanSummarySchema = z.object({
  plan_key: text,
  tenant_count: count,
  workspace_count: count,
  totals: z.record(z.string(), z.number()).default({}),
});
export type SystemResourcePlanSummary = z.output<typeof SystemResourcePlanSummarySchema>;

export const SystemTopTenantSchema = z
  .object({
    tenant_id: text,
    tenant_name: text,
    best_plan_key: text,
    workspace_count: count,
    members: count,
    agents: count,
    runtimes: count,
    issues_total: count,
    storage_bytes: count,
    autopilot_runs: count,
    state: optionalEnumOf(RESOURCE_STATES),
    status: optionalEnumOf(RESOURCE_STATES),
    resources: nullableArray(ResourceUsageItemSchema),
  })
  .transform(({ status, state, ...rest }) => ({
    ...rest,
    state: state ?? status ?? ("unknown" as const),
  }));
export type SystemTopTenant = z.output<typeof SystemTopTenantSchema>;

export const OverLimitResourceSchema = z
  .object({
    tenant_id: text,
    tenant_name: text,
    workspace_id: text,
    workspace_name: text,
    type: text,
    used: count,
    limit: z.number().nullable().default(null),
    percent: z.number().nullable().default(null),
    state: optionalEnumOf(RESOURCE_STATES),
    status: optionalEnumOf(RESOURCE_STATES),
  })
  .transform(({ status, state, ...rest }) => ({
    ...rest,
    state: state ?? status ?? ("unknown" as const),
  }));
export type OverLimitResource = z.output<typeof OverLimitResourceSchema>;

export const SystemResourcesSchema = z
  .object({
    by_type: nullableArray(SystemResourceTypeSummarySchema),
    by_plan: nullableArray(SystemResourcePlanSummarySchema),
    top_tenants: nullableArray(SystemTopTenantSchema).optional(),
    top: nullableArray(SystemTopTenantSchema).optional(),
    tenants: nullableArray(SystemTopTenantSchema),
    over_limit: nullableArray(OverLimitResourceSchema),
    next_cursor: nextCursor,
    complete: z.boolean().default(false),
  })
  .transform(({ top_tenants, top, ...rest }) => ({
    ...rest,
    top_tenants: top_tenants ?? top ?? [],
  }));
export type SystemResources = z.output<typeof SystemResourcesSchema>;

export function parseSystemResources(data: unknown): SystemResources {
  return parseWithFallback<SystemResources>(
    data,
    SystemResourcesSchema,
    {
      by_type: [],
      by_plan: [],
      top_tenants: [],
      tenants: [],
      over_limit: [],
      next_cursor: null,
      complete: false,
    },
    { endpoint: "GET /api/platform/resources" },
  );
}

export const RUNTIME_STATUSES = ["online", "offline"] as const;
export type RuntimeStatus = (typeof RUNTIME_STATUSES)[number] | "unknown";

export const RUNTIME_TASK_STATUSES = [
  "queued",
  "pending",
  "running",
  "completed",
  "failed",
  "canceled",
  "cancelled",
] as const;
export type RuntimeTaskStatus = (typeof RUNTIME_TASK_STATUSES)[number] | "unknown";

export const RuntimeSchema = z.object({
  id: z.string(),
  name: text,
  workspace_id: text,
  tenant_id: text,
  status: enumOf(RUNTIME_STATUSES),
  os: text,
  daemon_version: text,
  agent_version: text,
  owner_id: text,
  disabled: z.boolean().default(false),
  running_tasks: count,
  last_seen_at: nullableText,
  created_at: text,
});
export type Runtime = z.output<typeof RuntimeSchema>;

export const RuntimePageSchema = z.object({
  runtimes: nullableArray(RuntimeSchema),
  next_cursor: nextCursor,
});
export type RuntimePage = z.output<typeof RuntimePageSchema>;

export const RuntimeTaskSchema = z.object({
  id: z.string(),
  status: enumOf(RUNTIME_TASK_STATUSES),
  has_error: z.boolean().default(false),
  created_at: text,
  completed_at: nullableText,
});
export type RuntimeTask = z.output<typeof RuntimeTaskSchema>;

export const RuntimeDetailSchema = RuntimeSchema.extend({
  tasks: nullableArray(RuntimeTaskSchema),
  failed_tasks_7d: count,
  audit_events: nullableArray(AuditEventSchema),
});
export type RuntimeDetail = z.output<typeof RuntimeDetailSchema>;

export const SetRuntimeStatusResponseSchema = z.object({
  changed: z.boolean().default(false),
  disabled: z.boolean().default(false),
});
export type SetRuntimeStatusResponse = z.output<typeof SetRuntimeStatusResponseSchema>;

export function parseRuntimePage(data: unknown): RuntimePage {
  return parseWithFallback<RuntimePage>(
    data,
    RuntimePageSchema,
    { runtimes: [], next_cursor: null },
    { endpoint: "GET /api/platform/runtimes" },
  );
}

export function parseRuntimeDetail(data: unknown): RuntimeDetail | null {
  return parseWithFallback<RuntimeDetail | null>(data, RuntimeDetailSchema, null, {
    endpoint: "GET /api/platform/runtimes/{id}",
  });
}

export function parseSetRuntimeStatusResponse(data: unknown): SetRuntimeStatusResponse | null {
  return parseWithFallback<SetRuntimeStatusResponse | null>(
    data,
    SetRuntimeStatusResponseSchema,
    null,
    { endpoint: "POST /api/platform/runtimes/{id}/status" },
  );
}

export function parseRuntimeUnregisterResponse(data: unknown): DeletionStartResponse | null {
  return parseWithFallback<DeletionStartResponse | null>(
    data,
    DeletionStartResponseSchema,
    null,
    { endpoint: "POST /api/platform/runtimes/{id}/unregister" },
  );
}

// Tenant policy, tenant status, quota request listing, approvals, and legal holds (R6 / S36)

export const TenantPolicyValuesSchema = z.object({
  allowed_invite_domains: nullableArray(z.string()),
  share_links_enabled: z.boolean().default(true),
  require_sso: z.boolean().default(false),
  require_mfa: z.boolean().default(false),
  session_max_age_hours: z.number().int().nonnegative().default(0),
});
export type TenantPolicyValues = z.output<typeof TenantPolicyValuesSchema>;

export const POLICY_SOURCES = ["tenant", "workspace", "default"] as const;
export type PolicySource = (typeof POLICY_SOURCES)[number] | "unknown";

export const TenantPolicyResponseSchema = z.object({
  policy: TenantPolicyValuesSchema.prefault({}),
  version: z.number().int().nonnegative().default(0),
  // Pending backend fields: GET /tenants/{id}/policy currently returns { policy, version } only.
  effective_policy: TenantPolicyValuesSchema.optional(),
  sources: z.record(z.string(), enumOf(POLICY_SOURCES)).default({}),
  tenant_fields: nullableArray(z.string()),
});
export type TenantPolicyResponse = z.output<typeof TenantPolicyResponseSchema>;

export function parseTenantPolicyResponse(
  data: unknown,
  endpoint = "GET /api/platform/tenants/{id}/policy",
): TenantPolicyResponse | null {
  return parseWithFallback<TenantPolicyResponse | null>(data, TenantPolicyResponseSchema, null, {
    endpoint,
  });
}

export const TenantStatusResponseSchema = z.object({
  status: enumOf(WORKSPACE_STATUSES),
  workspace_ids: nullableArray(z.string()),
});
export type TenantStatusResponse = z.output<typeof TenantStatusResponseSchema>;

export function parseTenantStatusResponse(data: unknown): TenantStatusResponse | null {
  return parseWithFallback<TenantStatusResponse | null>(data, TenantStatusResponseSchema, null, {
    endpoint: "POST /api/platform/tenants/{id}/status",
  });
}

export const QuotaRequestPageSchema = z.object({
  requests: nullableArray(QuotaRequestSchema),
  next_cursor: nextCursor,
});
export type QuotaRequestPage = z.output<typeof QuotaRequestPageSchema>;

export function parseQuotaRequestPage(data: unknown): QuotaRequestPage {
  return parseWithFallback<QuotaRequestPage>(
    data,
    QuotaRequestPageSchema,
    { requests: [], next_cursor: null },
    { endpoint: "GET /api/platform/quota-requests" },
  );
}

export const ApprovalListSchema = z.object({
  approvals: nullableArray(DeletionRequestSchema),
});
export type ApprovalList = z.output<typeof ApprovalListSchema>;

export function parseApprovalList(data: unknown): ApprovalList {
  return parseWithFallback<ApprovalList>(
    data,
    ApprovalListSchema,
    { approvals: [] },
    { endpoint: "GET /api/platform/approvals" },
  );
}

export const LEGAL_HOLD_SCOPE_KINDS = ["tenant", "workspace", "account"] as const;
export type LegalHoldScopeKind = (typeof LEGAL_HOLD_SCOPE_KINDS)[number] | "unknown";

export const LegalHoldSchema = z.object({
  id: z.string(),
  scope_kind: enumOf(LEGAL_HOLD_SCOPE_KINDS),
  scope_id: text,
  reason: text,
  case_ref: text,
  placed_by: text,
  released: z.boolean().default(false),
  released_by: text,
  placed_at: nullableText,
  released_at: nullableText,
});
export type LegalHold = z.output<typeof LegalHoldSchema>;

export const LegalHoldListSchema = z.object({
  legal_holds: nullableArray(LegalHoldSchema),
});
export type LegalHoldList = z.output<typeof LegalHoldListSchema>;

export function parseLegalHold(data: unknown, endpoint: string): LegalHold | null {
  return parseWithFallback<LegalHold | null>(data, LegalHoldSchema, null, { endpoint });
}

export function parseLegalHoldList(data: unknown): LegalHoldList {
  return parseWithFallback<LegalHoldList>(
    data,
    LegalHoldListSchema,
    { legal_holds: [] },
    { endpoint: "GET /api/platform/legal-holds" },
  );
}
