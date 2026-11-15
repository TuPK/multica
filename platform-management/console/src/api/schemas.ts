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
  "workspace.status_changed",
  "auth.login",
  "auth.sessions_revoked",
  "pat.created",
  "pat.revoked",
  "account.export_requested",
  "account.deletion_requested",
  "account.deletion_canceled",
  "account.status_changed",
  "account.sessions_revoked",
  "consent.accepted",
  "staff.added",
  "staff.removed",
  "note.added",
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

export const TenantSchema = z.object({
  id: z.string(),
  name: text,
  slug: text,
  status: enumOf(TENANT_STATUSES),
  member_count: count,
  created_at: text,
  last_activity_at: text,
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

export const TenantDetailSchema = TenantSchema.extend({
  state: TenantStateSchema.prefault({}),
  notes_count: count,
  owner: z.object({ id: z.string(), email: text }).nullable().default(null),
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
});
export type Overview = z.output<typeof OverviewSchema>;

// Parsers. A list that fails to parse becomes an empty page. A single record
// has no honest empty value, so its fallback is null and the hook turns that
// into an error state instead of showing blank data.

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
