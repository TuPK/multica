export type StaffRole = "system_admin" | "admin" | "support" | "billing" | "unknown";

export interface MeResponse {
  user_id: string;
  email: string;
  role: StaffRole;
  tenant_id?: string | null;
}

export interface TenantRow {
  id: string;
  name: string;
  slug: string;
  status: string;
  member_count: number;
  created_at: string;
  last_activity_at: string;
}

export interface TenantState {
  status: string;
  reason: string;
  changed_by?: string | null;
  changed_at?: string | null;
  deletion_scheduled_at?: string | null;
}

export interface TenantOwner {
  id: string;
  email: string;
}

export interface TenantDetail extends TenantRow {
  state: TenantState;
  notes_count: number;
  owner?: TenantOwner | null;
}

export interface UserRow {
  id: string;
  email: string;
  name: string;
  status: string;
  memberships_count: number;
  created_at: string;
}

export interface Membership {
  workspace_id: string;
  name: string;
  role: string;
}

export interface AccountRecord {
  status: string;
  reason: string;
  changed_at?: string | null;
  sessions_valid_after?: string | null;
}

export interface UserDetail extends UserRow {
  memberships: Membership[];
  account: AccountRecord;
}

export interface Note {
  id: string;
  workspace_id: string;
  author_id: string;
  author_email?: string | null;
  body: string;
  created_at: string;
}

export interface StaffMember {
  user_id: string;
  email: string;
  role: string;
  tenant_id?: string | null;
  tenant_name?: string | null;
  created_by?: string | null;
  created_at: string;
}

export interface AuditEvent {
  id: string;
  workspace_id?: string | null;
  actor_type: string;
  actor_id?: string | null;
  actor_label: string;
  action: string;
  target_type: string;
  target_id: string;
  target_label: string;
  detail: Record<string, unknown> | null;
  ip: string;
  user_agent: string;
  request_id: string;
  created_at: string;
}

export interface OverviewData {
  workspaces_by_status: Record<string, number>;
  signups_7d: number;
}

export { AUDIT_ACTIONS as CATALOG_ACTIONS } from "../api/schemas";
