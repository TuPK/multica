import { z } from "zod";
import { parseWithFallback } from "@multica/core/api/schema";

// Every schema here is lenient on purpose: a missing optional field takes a
// default, and a string enum the console does not know becomes "unknown", so
// a newer server cannot blank a screen. Unknown keys are dropped.

export const TENANT_STATUSES = ["active", "read_only", "suspended", "pending_deletion"] as const;
export const ACCOUNT_STATUSES = ["active", "suspended", "pending_deletion", "deleted"] as const;
export const STAFF_ROLES = ["support", "billing", "admin"] as const;
export const ACTOR_TYPES = ["member", "agent", "system", "staff"] as const;

/**
 * The audit action codes the console API's `action` filter accepts. Kept in
 * step with platform-management/audit/audit.go by a test.
 */
export const AUDIT_ACTIONS = [
  "member.role_changed",
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
  "account.sessions_revoked",
  "staff.added",
  "staff.removed",
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
  "plan.price_scheduled",
  "plan.price_applied",
  "plan.price_canceled",
  "group.created",
  "group.updated",
  "group.deleted",
  "group.member_added",
  "group.member_removed",
  "group.permissions_changed",
] as const;

export type TenantStatus = (typeof TENANT_STATUSES)[number] | "unknown";
export type AccountStatus = (typeof ACCOUNT_STATUSES)[number] | "unknown";
export type StaffRole = (typeof STAFF_ROLES)[number] | "unknown";
export type ActorType = (typeof ACTOR_TYPES)[number] | "unknown";

function enumOf<const T extends readonly string[]>(values: T) {
  return z
    .string()
    .transform((v): T[number] | "unknown" => (values.includes(v) ? v : "unknown"))
    .default("unknown");
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
});
export type Me = z.output<typeof MeSchema>;

// Tenants

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

export const TenantSchema = z.object({
  id: z.string(),
  name: text,
  slug: text,
  status: enumOf(TENANT_STATUSES),
  member_count: count,
  created_at: text,
  last_activity_at: text,
  ...usageFields,
});
export type Tenant = z.output<typeof TenantSchema>;

const TenantStateSchema = z.object({
  status: enumOf(TENANT_STATUSES),
  reason: text,
  changed_by: nullableText,
  changed_at: nullableText,
  deletion_scheduled_at: nullableText,
});
export type TenantState = z.output<typeof TenantStateSchema>;

// Billing & Orders

export const ORDER_STATUSES = ["pending", "paid", "expired", "canceled", "refunded"] as const;
export const ORDER_KINDS = ["subscribe", "change_plan", "add_seats", "renew"] as const;

export type OrderStatus = (typeof ORDER_STATUSES)[number] | "unknown";
export type OrderKind = (typeof ORDER_KINDS)[number] | "unknown";

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

export const PlanSchema = z.object({
  key: z.string(),
  name: text,
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
  version: z.number().optional(),
});
export type Plan = z.output<typeof PlanSchema>;

export const PlanListSchema = z.object({
  plans: z.array(PlanSchema).default([]),
});
export type PlanList = z.output<typeof PlanListSchema>;

export const PutPlanResponseSchema = z.object({
  changed: z.boolean().default(false),
  plan: PlanSchema,
});
export type PutPlanResponse = z.output<typeof PutPlanResponseSchema>;

export const DeletePlanResponseSchema = z.object({
  deleted: z.boolean(),
});
export type DeletePlanResponse = z.output<typeof DeletePlanResponseSchema>;

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

export const TenantDetailSchema = TenantSchema.extend({
  state: TenantStateSchema.prefault({}),
  notes_count: count,
  owner: z.object({ id: z.string(), email: text }).nullable().default(null),
  subscription: SubscriptionSchema.nullable().default(null),
  recent_orders: z.array(OrderSchema).default([]),
  usage_current: UsageCurrentSchema.optional(),
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
  // The workspace role is Multica's, not the console's; shown as it comes.
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
});
export type Overview = z.output<typeof OverviewSchema>;

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

export function parsePutSubscriptionResponse(data: unknown): PutSubscriptionResponse | null {
  return parseWithFallback<PutSubscriptionResponse | null>(data, PutSubscriptionResponseSchema, null, {
    endpoint: "PUT /api/platform/tenants/{id}/subscription",
  });
}

export function parsePlanList(data: unknown): PlanList {
  return parseWithFallback<PlanList>(data, PlanListSchema, { plans: [] }, {
    endpoint: "GET /api/platform/plans",
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
    endpoint: "GET /api/platform/tenants/{id}/notes",
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
