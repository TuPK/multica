import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query";
import { PlatformApiError, platformFetch, platformFetchText, type PlatformTextResponse } from "./client";
import {
  parseAuditPage,
  parseMe,
  parseNotePage,
  parseOverview,
  parseStaffList,
  parseWorkspaceDetail,
  parseWorkspacePage,
  parseWorkspaceExportResponse,
  parseTenantDetail,
  parseTenantPage,
  parseWorkspaceAICost,
  parseWorkspaceUsage,
  parseUserDetail,
  parseUserPage,
  parseOrderList,
  parseOrderActionResult,
  parseNotice,
  parseNoticeDetail,
  parseNoticeList,
  parsePlanList,
  parsePlanDetail,
  parsePlanHistory,
  parsePutPlanResponse,
  parseDeletePlanResponse,
  parseVerifyCodeResponse,
  parseResetMfaResponse,
  parsePutSubscriptionResponse,
  parseBillingProfile,
  parseBillingProfileVerifyResponse,
  parsePutBillingProfileResponse,
  parseAccountRequestList,
  parseAccountRequest,
  parseResendWelcomeResponse,
  parsePriceChangeList,
  parsePriceChangeDryRunResponse,
  parseSchedulePriceChangeResponse,
  parseCancelPriceChangeResponse,
  parseEinvoiceList,
  parseOrderEinvoiceList,
  parseUpdateStaffResponse,
  parseMemberRequest,
  parseMemberRequestList,
  parseDeletionRequest,
  parseDeletionRequestList,
  parseDeletionStartResponse,
  parseAccountDeletePreview,
  parseTenantUserPage,
  parseTenantUserDetail,
  parseDeactivateTenantUserResponse,
  parseMemberChangeResponse,
  parseQuotaRequest,
  parseTenantLimitOverrideResponse,
  parseTenantResources,
  parseSystemResources,
  parseRuntimePage,
  parseRuntimeDetail,
  parseSetRuntimeStatusResponse,
  parseRuntimeUnregisterResponse,
  parseTenantPolicyResponse,
  parseTenantStatusResponse,
  parseQuotaRequestPage,
  parseApprovalList,
  parseLegalHold,
  parseLegalHoldList,
  type Me,
  type Overview,
  type StaffRole,
  type TenantDetail,
  type WorkspaceDetail,
  type WorkspaceListItem,
  type WorkspaceExportResponse,
  type WorkspaceStatus,
  type WorkspaceAICost,
  type WorkspaceUsage,
  type UserDetail,
  type Order,
  type OrderInvoiceState,
  type OrderEinvoice,
  type OrderEinvoiceState,
  type Notice,
  type NoticeCandidate,
  type NoticeDetail,
  type NoticeState,
  type Plan,
  type PlanAudience,
  type PlanBillingModel,
  type PlanDetailResponse,
  type PlanHistoryEntry,
  type Subscription,
  type BillingProfile,
  type BillingProfileVerifyResponse,
  type PutBillingProfileResponse,
  type Limits,
  type AccountRequest,
  type AccountRequestState,
  type PriceChange,
  type PriceChangeState,
  type PriceSnapshot,
  type PriceChangeDryRunResponse,
  type SchedulePriceChangeResponse,
  type CancelPriceChangeResponse,
  type Einvoice,
  type EinvoiceState,
  type UpdateStaffResponse,
  type MemberRole,
  type MemberRequest,
  type DeletionRequest,
  type DeletionStartResponse,
  type AccountDeletePreview,
  type TenantUser,
  type TenantUserDetail,
  type TenantUserWorkspace,
  type TenantUserAllocationItem,
  type TenantMemberRole,
  type DeactivateTenantUserResponse,
  type MemberChangeResponse,
  type QuotaRequest,
  type LimitGate,
  type TenantLimitOverrideResponse,
  type ResourceState,
  type ResourceUsageItem,
  type TenantWorkspaceResource,
  type TenantResources,
  type SystemResourceTypeSummary,
  type SystemResourcePlanSummary,
  type SystemTopTenant,
  type OverLimitResource,
  type SystemResources,
  type RuntimeStatus,
  type RuntimeTaskStatus,
  type Runtime,
  type RuntimeTask,
  type RuntimeDetail,
  type SetRuntimeStatusResponse,
  type TenantPolicyValues,
  type PolicySource,
  type TenantPolicyResponse,
  type TenantStatusResponse,
  type QuotaRequestPage,
  type ApprovalList,
  type LegalHoldScopeKind,
  type LegalHold,
  type LegalHoldList,
} from "./schemas";

export type {
  Subscription,
  BillingProfile,
  BillingProfileVerifyResponse,
  PutBillingProfileResponse,
  Order,
  OrderInvoiceState,
  OrderEinvoice,
  OrderEinvoiceState,
  Notice,
  NoticeCandidate,
  NoticeDetail,
  NoticeState,
  Einvoice,
  EinvoiceState,
  Plan,
  PlanAudience,
  PlanBillingModel,
  PlanDetailResponse,
  PlanHistoryEntry,
  Limits,
  WorkspaceUsage,
  WorkspaceAICost,
  WorkspaceListItem,
  WorkspaceExportResponse,
  PriceChange,
  PriceChangeState,
  PriceSnapshot,
  PriceChangeDryRunResponse,
  SchedulePriceChangeResponse,
  CancelPriceChangeResponse,
  UpdateStaffResponse,
  MemberRole,
  MemberRequest,
  DeletionRequest,
  DeletionStartResponse,
  AccountDeletePreview,
  TenantUser,
  TenantUserDetail,
  TenantUserWorkspace,
  TenantUserAllocationItem,
  TenantMemberRole,
  DeactivateTenantUserResponse,
  MemberChangeResponse,
  QuotaRequest,
  QuotaRequestPage,
  LimitGate,
  TenantLimitOverrideResponse,
  ResourceState,
  ResourceUsageItem,
  TenantWorkspaceResource,
  TenantResources,
  SystemResourceTypeSummary,
  SystemResourcePlanSummary,
  SystemTopTenant,
  OverLimitResource,
  SystemResources,
  RuntimeStatus,
  RuntimeTaskStatus,
  Runtime,
  RuntimeTask,
  RuntimeDetail,
  SetRuntimeStatusResponse,
  TenantPolicyValues,
  PolicySource,
  TenantPolicyResponse,
  TenantStatusResponse,
  ApprovalList,
  LegalHoldScopeKind,
  LegalHold,
  LegalHoldList,
  PlatformTextResponse,
};

/** The states staff may set on a workspace. */
export type SettableWorkspaceStatus = Exclude<WorkspaceStatus, "unknown">;
/** The states staff may set on an account. */
export type SettableAccountStatus = "active" | "suspended";
/** The roles staff may grant. */
export type SettableStaffRole = Exclude<StaffRole, "unknown">;

// Filters

export interface SearchFilters {
  /** Substring of the name, slug or email. */
  q?: string;
}

export interface WorkspaceFilters {
  q?: string;
  status?: string;
  plan_key?: string;
  tenant_id?: string;
}

export interface AuditFilters {
  /** One of AUDIT_ACTIONS. */
  action?: string;
  actor_id?: string;
  /** RFC 3339, inclusive. */
  from?: string;
  /** RFC 3339, exclusive. */
  to?: string;
}

export interface DeletionRequestFilters {
  kind?: string;
  state?: string;
}

export interface TenantUserFilters {
  q?: string;
  account_status?: string;
  workspace_id?: string;
  member_role?: string;
}

export interface SystemResourceFilters {
  sort?: string;
  top?: number;
}

export interface RuntimeFilters {
  q?: string;
  status?: string;
  tenant_id?: string;
  workspace_id?: string;
  disabled?: boolean;
}

export interface QuotaRequestFilters {
  state?: string;
}

export interface AuditExportFilters {
  from: string;
  to: string;
  format: "ndjson" | "csv";
  action?: string;
  actor_id?: string;
  workspace_id?: string;
  cursor?: string;
  limit?: number;
}

/** Drops empty values, so `{}` and `{ q: "" }` are one query. */
function clean<T extends object>(filters: T): T {
  const out: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(filters)) {
    if (typeof value === "string" && value.trim() !== "") {
      out[key] = value.trim();
    } else if (typeof value === "boolean" && value) {
      out[key] = value;
    } else if (typeof value === "number" && Number.isFinite(value)) {
      out[key] = value;
    }
  }
  return out as T;
}

function queryString(params: Record<string, string | undefined>): string {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value) search.set(key, value);
  }
  const s = search.toString();
  return s === "" ? "" : `?${s}`;
}

function pathSegment(id: string): string {
  return encodeURIComponent(id);
}

// Query keys. Every key starts with "platform", so one prefix reaches them all.

export const platformKeys = {
  all: ["platform"] as const,
  me: () => ["platform", "me"] as const,
  workspaces: () => ["platform", "workspaces"] as const,
  workspaceLists: () => ["platform", "workspaces", "list"] as const,
  workspaceList: (filters: WorkspaceFilters) => ["platform", "workspaces", "list", clean(filters)] as const,
  tenants: () => ["platform", "tenants"] as const,
  tenantLists: () => ["platform", "tenants", "list"] as const,
  tenantList: (filters: SearchFilters) => ["platform", "tenants", "list", clean(filters)] as const,
  tenant: (id: string) => ["platform", "tenants", "detail", id] as const,
  tenantPolicy: (id: string) => ["platform", "tenants", "policy", id] as const,
  tenantUsersAll: (tenantId: string) => ["platform", "tenants", "users", tenantId] as const,
  tenantUserList: (tenantId: string, filters: TenantUserFilters) =>
    ["platform", "tenants", "users", tenantId, "list", clean(filters)] as const,
  tenantUser: (tenantId: string, userId: string) =>
    ["platform", "tenants", "users", tenantId, "detail", userId] as const,
  tenantResources: (id: string) => ["platform", "tenants", "resources", id] as const,
  resources: () => ["platform", "resources"] as const,
  resourceList: (filters: SystemResourceFilters) =>
    ["platform", "resources", "list", clean(filters)] as const,
  runtimes: () => ["platform", "runtimes"] as const,
  runtimeList: (filters: RuntimeFilters) =>
    ["platform", "runtimes", "list", clean(filters)] as const,
  runtime: (id: string) => ["platform", "runtimes", "detail", id] as const,
  quotaRequests: () => ["platform", "quota-requests"] as const,
  quotaRequestList: (filters: QuotaRequestFilters) =>
    ["platform", "quota-requests", "list", clean(filters)] as const,
  quotaRequest: (id: string) => ["platform", "quota-requests", "detail", id] as const,
  approvals: () => ["platform", "approvals"] as const,
  legalHolds: () => ["platform", "legal-holds"] as const,
  workspace: (id: string) => ["platform", "workspaces", "detail", id] as const,
  workspaceUsage: (id: string) => ["platform", "workspaces", "usage", id] as const,
  workspaceAICost: (id: string, window: string) => ["platform", "workspaces", "ai-cost", id, window] as const,
  workspaceBillingProfile: (id: string) => ["platform", "workspaces", "billing-profile", id] as const,
  workspaceAudits: (id: string) => ["platform", "workspaces", "audit", id] as const,
  workspaceAudit: (id: string, filters: AuditFilters) => ["platform", "workspaces", "audit", id, clean(filters)] as const,
  workspaceNotes: (id: string) => ["platform", "workspaces", "notes", id] as const,
  memberRequests: (workspaceId: string) => ["platform", "workspaces", "member-requests", workspaceId] as const,
  users: () => ["platform", "users"] as const,
  userLists: () => ["platform", "users", "list"] as const,
  userList: (filters: SearchFilters) => ["platform", "users", "list", clean(filters)] as const,
  user: (id: string) => ["platform", "users", "detail", id] as const,
  accountDeletePreview: (userId: string) => ["platform", "users", "delete-now-preview", userId] as const,
  staff: () => ["platform", "staff"] as const,
  deletionRequests: () => ["platform", "deletion-requests"] as const,
  deletionRequestList: (filters: DeletionRequestFilters) => ["platform", "deletion-requests", clean(filters)] as const,
  orders: () => ["platform", "orders"] as const,
  orderList: (status: string) => ["platform", "orders", "list", status] as const,
  orderEinvoices: (id: string) => ["platform", "orders", "einvoices", id] as const,
  notices: () => ["platform", "payment-notices"] as const,
  noticeList: () => ["platform", "payment-notices", "list"] as const,
  notice: (id: string) => ["platform", "payment-notices", "detail", id] as const,
  einvoices: () => ["platform", "einvoices"] as const,
  einvoiceList: (state = "") => ["platform", "einvoices", "list", state] as const,
  plans: () => ["platform", "plans"] as const,
  plan: (key: string) => ["platform", "plans", "detail", key] as const,
  planHistory: (key: string) => ["platform", "plans", key, "history"] as const,
  priceChanges: (key: string) => ["platform", "plans", key, "price-changes"] as const,
  audits: () => ["platform", "audit"] as const,
  audit: (filters: AuditFilters) => ["platform", "audit", clean(filters)] as const,
  overview: () => ["platform", "overview"] as const,
};

function malformedResponse(): PlatformApiError {
  return new PlatformApiError(200, "malformed_response", "The server answered with data the console cannot read");
}

function orMalformed<T>(value: T | null): T {
  if (value === null) throw malformedResponse();
  return value;
}

// Session

/**
 * The signed-in staff member, or null when the server answers 404, which means
 * nobody is signed in.
 */
export function useMe() {
  return useQuery({
    queryKey: platformKeys.me(),
    queryFn: async ({ signal }): Promise<Me | null> => {
      let body: unknown;
      try {
        body = await platformFetch("/me", { signal });
      } catch (err) {
        if (err instanceof PlatformApiError && err.status === 404) return null;
        throw err;
      }
      return orMalformed(parseMe(body));
    },
  });
}

/** Variables: `{ email }`. The server answers 204 for every address. */
export function useSendCode() {
  return useMutation({
    mutationFn: async (vars: { email: string }): Promise<void> => {
      await platformFetch("/auth/send-code", { method: "POST", body: { email: vars.email } });
    },
  });
}

/**
 * Variables: `{ email, code, totp? }`. Resolves `{ mfaRequired: true }` when the
 * email code is right and the authenticator code is still needed: submit again
 * with `totp`. A wrong code rejects with code `invalid_code`.
 */
export function useVerifyCode() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { email: string; code: string; totp?: string }): Promise<{ mfaRequired: boolean }> => {
      const res = parseVerifyCodeResponse(
        await platformFetch("/auth/verify-code", {
          method: "POST",
          body: { email: vars.email, code: vars.code, ...(vars.totp === undefined ? {} : { totp: vars.totp }) },
        }),
      );
      if (res === null) throw new Error("Unexpected answer from the server");
      return { mfaRequired: res.mfa_required };
    },
    onSuccess: ({ mfaRequired }) =>
      mfaRequired ? undefined : queryClient.invalidateQueries({ queryKey: platformKeys.me() }),
  });
}

/** Signs out and drops everything the console cached for the staff member. */
export function useLogout() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (): Promise<void> => {
      await platformFetch("/auth/logout", { method: "POST" });
    },
    onSuccess: () => {
      queryClient.removeQueries({
        queryKey: platformKeys.all,
        predicate: (query) => query.queryKey[1] !== "me",
      });
      queryClient.setQueryData<Me | null>(platformKeys.me(), null);
    },
  });
}

// Workspaces

// The lists below are infinite queries, so a screen can load more without
// losing the pages it has. `data` is the flat array of every loaded row; page
// with `fetchNextPage` while `hasNextPage`.

export function useTenants(filters: SearchFilters = {}) {
  const f = clean(filters);
  return useInfiniteQuery({
    queryKey: platformKeys.tenantList(f),
    queryFn: async ({ pageParam, signal }) =>
      parseTenantPage(await platformFetch(`/tenants${queryString({ q: f.q, cursor: pageParam })}`, { signal })),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    select: (data) => data.pages.flatMap((page) => page.tenants),
  });
}

export function useWorkspaces(filters: WorkspaceFilters = {}) {
  const f = clean(filters);
  return useInfiniteQuery({
    queryKey: platformKeys.workspaceList(f),
    queryFn: async ({ pageParam, signal }) =>
      parseWorkspacePage(
        await platformFetch(
          `/workspaces${queryString({
            q: f.q,
            status: f.status,
            plan_key: f.plan_key,
            tenant_id: f.tenant_id,
            cursor: pageParam,
          })}`,
          { signal },
        ),
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    select: (data) => data.pages.flatMap((page) => page.workspaces),
  });
}

export function useTenant(id: string | undefined) {
  return useQuery({
    queryKey: platformKeys.tenant(id ?? ""),
    enabled: !!id,
    queryFn: async ({ signal }): Promise<TenantDetail> =>
      orMalformed(parseTenantDetail(await platformFetch(`/tenants/${pathSegment(id ?? "")}`, { signal }))),
  });
}

export function useWorkspace(id: string | undefined) {
  return useQuery({
    queryKey: platformKeys.workspace(id ?? ""),
    enabled: !!id,
    queryFn: async ({ signal }): Promise<WorkspaceDetail> =>
      orMalformed(parseWorkspaceDetail(await platformFetch(`/workspaces/${pathSegment(id ?? "")}`, { signal }))),
  });
}

export function useWorkspaceUsage(id: string | undefined) {
  return useQuery({
    queryKey: platformKeys.workspaceUsage(id ?? ""),
    enabled: !!id,
    queryFn: async ({ signal }): Promise<WorkspaceUsage> =>
      orMalformed(parseWorkspaceUsage(await platformFetch(`/workspaces/${pathSegment(id ?? "")}/usage`, { signal }))),
  });
}

export function useWorkspaceAICost(id: string | undefined, window: string) {
  return useQuery({
    queryKey: platformKeys.workspaceAICost(id ?? "", window),
    enabled: !!id,
    queryFn: async ({ signal }): Promise<WorkspaceAICost> =>
      orMalformed(
        parseWorkspaceAICost(
          await platformFetch(`/workspaces/${pathSegment(id ?? "")}/ai-cost?window=${encodeURIComponent(window)}`, { signal }),
        ),
      ),
  });
}

export function useWorkspaceBillingProfile(id: string | undefined) {
  return useQuery({
    queryKey: platformKeys.workspaceBillingProfile(id ?? ""),
    enabled: !!id,
    queryFn: async ({ signal }): Promise<BillingProfile> =>
      orMalformed(
        parseBillingProfile(
          await platformFetch(`/workspaces/${pathSegment(id ?? "")}/billing-profile`, { signal }),
        ),
      ),
  });
}

function invalidateAll(queryClient: QueryClient, keys: readonly (readonly unknown[])[]) {
  return Promise.all(keys.map((queryKey) => queryClient.invalidateQueries({ queryKey })));
}

/** Variables: `{ workspaceId, status, reason }`. The reason is required. */
export function useChangeWorkspaceStatus() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { workspaceId: string; status: SettableWorkspaceStatus; reason: string }): Promise<void> => {
      await platformFetch(`/workspaces/${pathSegment(vars.workspaceId)}/status`, {
        method: "POST",
        body: { status: vars.status, reason: vars.reason },
      });
    },
    onSuccess: (_data, vars) =>
      invalidateAll(queryClient, [
        platformKeys.workspace(vars.workspaceId),
        platformKeys.workspaceLists(),
        platformKeys.tenants(),
        platformKeys.workspaceAudits(vars.workspaceId),
        platformKeys.audits(),
        platformKeys.overview(),
      ]),
  });
}

/** Queue a workspace export bundle (support, admin, system_admin). */
export function useExportWorkspace() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { workspaceId: string }) => {
      const res = await platformFetch(`/workspaces/${pathSegment(vars.workspaceId)}/export`, {
        method: "POST",
      });
      return parseWorkspaceExportResponse(res);
    },
    onSuccess: (_data, vars) =>
      invalidateAll(queryClient, [
        platformKeys.workspace(vars.workspaceId),
        platformKeys.workspaceAudits(vars.workspaceId),
        platformKeys.audits(),
      ]),
  });
}

/** A workspace's audit events, newest first. */
export function useWorkspaceAudit(id: string | undefined, filters: AuditFilters = {}) {
  const f = clean(filters);
  return useInfiniteQuery({
    queryKey: platformKeys.workspaceAudit(id ?? "", f),
    enabled: !!id,
    queryFn: async ({ pageParam, signal }) =>
      parseAuditPage(
        await platformFetch(
          `/workspaces/${pathSegment(id ?? "")}/audit-events${queryString({ ...f, cursor: pageParam })}`,
          { signal },
        ),
        "GET /api/platform/workspaces/{id}/audit-events",
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    select: (data) => data.pages.flatMap((page) => page.events),
  });
}

/** A workspace's notes, newest first. */
export function useWorkspaceNotes(id: string | undefined) {
  return useInfiniteQuery({
    queryKey: platformKeys.workspaceNotes(id ?? ""),
    enabled: !!id,
    queryFn: async ({ pageParam, signal }) =>
      parseNotePage(
        await platformFetch(`/workspaces/${pathSegment(id ?? "")}/notes${queryString({ cursor: pageParam })}`, { signal }),
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    select: (data) => data.pages.flatMap((page) => page.notes),
  });
}

/** Variables: `{ workspaceId, body }`. */
export function useAddWorkspaceNote() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { workspaceId: string; body: string }): Promise<void> => {
      await platformFetch(`/workspaces/${pathSegment(vars.workspaceId)}/notes`, {
        method: "POST",
        body: { body: vars.body },
      });
    },
    onSuccess: (_data, vars) =>
      invalidateAll(queryClient, [
        platformKeys.workspaceNotes(vars.workspaceId),
        platformKeys.workspace(vars.workspaceId),
        platformKeys.workspaceAudits(vars.workspaceId),
        platformKeys.audits(),
      ]),
  });
}

// Users

export function useUsers(filters: SearchFilters = {}) {
  const f = clean(filters);
  return useInfiniteQuery({
    queryKey: platformKeys.userList(f),
    queryFn: async ({ pageParam, signal }) =>
      parseUserPage(await platformFetch(`/users${queryString({ q: f.q, cursor: pageParam })}`, { signal })),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    select: (data) => data.pages.flatMap((page) => page.users),
  });
}

export function useUser(id: string | undefined) {
  return useQuery({
    queryKey: platformKeys.user(id ?? ""),
    enabled: !!id,
    queryFn: async ({ signal }): Promise<UserDetail> =>
      orMalformed(parseUserDetail(await platformFetch(`/users/${pathSegment(id ?? "")}`, { signal }))),
  });
}

/** Variables: `{ userId, status, reason }`. The reason is required. */
export function useChangeUserStatus() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { userId: string; status: SettableAccountStatus; reason: string }): Promise<void> => {
      await platformFetch(`/users/${pathSegment(vars.userId)}/status`, {
        method: "POST",
        body: { status: vars.status, reason: vars.reason },
      });
    },
    onSuccess: (_data, vars) =>
      invalidateAll(queryClient, [platformKeys.user(vars.userId), platformKeys.userLists(), platformKeys.audits()]),
  });
}

/** Variables: `{ userId, reason }`. The reason is required. */
export function useRevokeUserSessions() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { userId: string; reason: string }): Promise<void> => {
      await platformFetch(`/users/${pathSegment(vars.userId)}/revoke-sessions`, {
        method: "POST",
        body: { reason: vars.reason },
      });
    },
    onSuccess: (_data, vars) => invalidateAll(queryClient, [platformKeys.user(vars.userId), platformKeys.audits()]),
  });
}

/**
 * Variables: `{ userId }`. Deletes the user's two-step sign-in and emails them.
 * Resolves `{ reset: false }` when the user had none. A staff member's own is
 * refused with code `self_reset_refused`.
 */
export function useResetUserMfa() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { userId: string }) => {
      const res = parseResetMfaResponse(
        await platformFetch(`/users/${pathSegment(vars.userId)}/mfa-reset`, { method: "POST" }),
      );
      if (res === null) throw new Error("Unexpected answer from the server");
      return res;
    },
    onSuccess: (_data, vars) => invalidateAll(queryClient, [platformKeys.user(vars.userId), platformKeys.audits()]),
  });
}

// Staff

/** The active staff, as a flat array. Admin only; any other role gets a 404 error. */
export function useStaff() {
  return useQuery({
    queryKey: platformKeys.staff(),
    queryFn: async ({ signal }) => parseStaffList(await platformFetch("/staff", { signal })).staff,
  });
}

/** Variables: `{ email, role, tenant_id? }`. Rejects with `no_such_account`, `already_staff`, or `tenant_required`. */
export function useAddStaff() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { email: string; role: SettableStaffRole; tenant_id?: string }): Promise<void> => {
      const body: Record<string, unknown> = { email: vars.email, role: vars.role };
      if (vars.role === "admin" && vars.tenant_id) body.tenant_id = vars.tenant_id;
      await platformFetch("/staff", { method: "POST", body });
    },
    onSuccess: () => invalidateAll(queryClient, [platformKeys.staff(), platformKeys.audits()]),
  });
}

/** Variables: `{ userId, role, tenant_id? }`. Rejects with `tenant_required`, `tenant_not_allowed`, `tenant_invalid`, or `last_system_admin`. */
export function useUpdateStaff() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: {
      userId: string;
      role: SettableStaffRole;
      tenant_id?: string | null;
    }): Promise<UpdateStaffResponse> => {
      const body: Record<string, unknown> = { role: vars.role };
      if (vars.role === "admin" && vars.tenant_id) body.tenant_id = vars.tenant_id;
      const res = await platformFetch(`/staff/${pathSegment(vars.userId)}`, { method: "PUT", body });
      return orMalformed(parseUpdateStaffResponse(res));
    },
    onSuccess: () => invalidateAll(queryClient, [platformKeys.staff(), platformKeys.audits(), platformKeys.me()]),
  });
}

/** Variables: `{ userId }`. Rejects with code `last_admin` for the last administrator. */
export function useRemoveStaff() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { userId: string }): Promise<void> => {
      await platformFetch(`/staff/${pathSegment(vars.userId)}`, { method: "DELETE" });
    },
    // Removing oneself ends the session, so who is signed in is refetched too.
    onSuccess: () => invalidateAll(queryClient, [platformKeys.staff(), platformKeys.audits(), platformKeys.me()]),
  });
}

// Audit and overview

/** The system-wide audit log, newest first. */
export function useAuditEvents(filters: AuditFilters = {}) {
  const f = clean(filters);
  return useInfiniteQuery({
    queryKey: platformKeys.audit(f),
    queryFn: async ({ pageParam, signal }) =>
      parseAuditPage(
        await platformFetch(`/audit-events${queryString({ ...f, cursor: pageParam })}`, { signal }),
        "GET /api/platform/audit-events",
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    select: (data) => data.pages.flatMap((page) => page.events),
  });
}

export function useOverview() {
  return useQuery({
    queryKey: platformKeys.overview(),
    queryFn: async ({ signal }): Promise<Overview> =>
      orMalformed(parseOverview(await platformFetch("/overview", { signal }))),
  });
}

// Orders & Billing

/** List orders by status (pending, paid, expired, canceled, refunded). Default: pending. */
export function useOrders(status = "pending") {
  return useQuery({
    queryKey: platformKeys.orderList(status),
    queryFn: async ({ signal }): Promise<Order[]> => {
      const res = await platformFetch(`/orders${queryString({ status })}`, { signal });
      return parseOrderList(res).orders;
    },
  });
}

/** Variables: `{ orderId, reason, transferReference }`. The reason is required. */
export function useConfirmOrder() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { orderId: string; reason: string; transferReference?: string }) => {
      const body: Record<string, unknown> = { reason: vars.reason };
      if (vars.transferReference !== undefined) body.transfer_reference = vars.transferReference;
      const res = await platformFetch(`/orders/${pathSegment(vars.orderId)}/confirm`, {
        method: "POST",
        body,
      });
      return parseOrderActionResult(res, "POST /api/platform/orders/{id}/confirm");
    },
    onSuccess: () =>
      invalidateAll(queryClient, [
        platformKeys.orders(),
        platformKeys.workspaces(),
        platformKeys.tenants(),
        platformKeys.overview(),
        platformKeys.audits(),
      ]),
  });
}

/** Variables: `{ orderId, reason }`. The reason is required. */
export function useCancelOrder() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { orderId: string; reason: string }) => {
      const res = await platformFetch(`/orders/${pathSegment(vars.orderId)}/cancel`, {
        method: "POST",
        body: { reason: vars.reason },
      });
      return parseOrderActionResult(res, "POST /api/platform/orders/{id}/cancel");
    },
    onSuccess: () =>
      invalidateAll(queryClient, [
        platformKeys.orders(),
        platformKeys.workspaces(),
        platformKeys.overview(),
        platformKeys.audits(),
      ]),
  });
}

// Payment notices

/** List payment notices awaiting staff review, oldest first. Billing or Admin only. */
export function usePaymentNotices() {
  return useQuery({
    queryKey: platformKeys.noticeList(),
    queryFn: async ({ signal }): Promise<Notice[]> => {
      const res = await platformFetch(`/payment-notices${queryString({ state: "needs_review" })}`, { signal });
      return parseNoticeList(res).notices;
    },
  });
}

/** Fetch one payment notice and its candidate orders. Billing or Admin only. */
export function usePaymentNotice(id: string | null) {
  return useQuery({
    queryKey: platformKeys.notice(id ?? ""),
    enabled: id !== null,
    queryFn: async ({ signal }): Promise<NoticeDetail> => {
      const res = await platformFetch(`/payment-notices/${pathSegment(id ?? "")}`, { signal });
      return orMalformed(parseNoticeDetail(res));
    },
  });
}

/** Variables: `{ noticeId, orderId, reason }`. Matches a payment notice to a pending order. */
export function useResolvePaymentNotice() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { noticeId: string; orderId: string; reason: string }): Promise<Notice> => {
      const res = await platformFetch(`/payment-notices/${pathSegment(vars.noticeId)}/resolve`, {
        method: "POST",
        body: { order_id: vars.orderId, reason: vars.reason },
      });
      return orMalformed(parseNotice(res, "POST /api/platform/payment-notices/{id}/resolve"));
    },
    onSuccess: () =>
      invalidateAll(queryClient, [
        platformKeys.notices(),
        platformKeys.orders(),
        platformKeys.audits(),
        platformKeys.overview(),
      ]),
  });
}

/** Variables: `{ noticeId, reason }`. Dismisses a payment notice without matching an order. */
export function useDismissPaymentNotice() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { noticeId: string; reason: string }): Promise<Notice> => {
      const res = await platformFetch(`/payment-notices/${pathSegment(vars.noticeId)}/dismiss`, {
        method: "POST",
        body: { reason: vars.reason },
      });
      return orMalformed(parseNotice(res, "POST /api/platform/payment-notices/{id}/dismiss"));
    },
    onSuccess: () =>
      invalidateAll(queryClient, [
        platformKeys.notices(),
        platformKeys.orders(),
        platformKeys.audits(),
        platformKeys.overview(),
      ]),
  });
}

// Plans

/** List all plans. System admin or admin (read-only). */
export function usePlans(enabled = true) {
  return useQuery({
    queryKey: platformKeys.plans(),
    enabled,
    queryFn: async ({ signal }): Promise<Plan[]> => {
      const res = await platformFetch("/plans", { signal });
      return parsePlanList(res).plans;
    },
  });
}

/** Fetch one plan detail by key. System admin only. */
export function usePlan(key: string | undefined, enabled = true) {
  return useQuery({
    queryKey: platformKeys.plan(key ?? ""),
    enabled: Boolean(key) && enabled,
    queryFn: async ({ signal }): Promise<Plan> => {
      const res = await platformFetch(`/plans/${pathSegment(key ?? "")}`, { signal });
      return orMalformed(parsePlanDetail(res)).plan;
    },
  });
}

/** Fetch a plan's audit history (oldest/newest per server, plan.changed / activated / deactivated / price). System admin or billing. */
export function usePlanHistory(key: string | undefined, enabled = true) {
  return useQuery({
    queryKey: platformKeys.planHistory(key ?? ""),
    enabled: Boolean(key) && enabled,
    queryFn: async ({ signal }): Promise<PlanHistoryEntry[]> => {
      const res = await platformFetch(`/plans/${pathSegment(key ?? "")}/history`, { signal });
      return parsePlanHistory(res).history;
    },
  });
}

export interface CreatePlanInput {
  key: string;
  name: string;
  description: string;
  limits: Limits;
  features: Record<string, boolean>;
  currency: string;
  seat_price_month: number | null;
  seat_price_year: number | null;
  seats_fixed: number | null;
  min_seats: number | null;
  trial_days: number;
  is_public: boolean;
  sort: number;
  audience: Exclude<PlanAudience, "unknown">;
  billing_model: Exclude<PlanBillingModel, "unknown">;
}

/** Create a new plan. System admin only. */
export function useCreatePlan() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: CreatePlanInput) => {
      const res = await platformFetch("/plans", {
        method: "POST",
        body: vars,
      });
      return parsePlanDetail(res, "POST /api/platform/plans");
    },
    onSuccess: () =>
      invalidateAll(queryClient, [
        platformKeys.plans(),
        platformKeys.audits(),
      ]),
  });
}

export interface PutPlanInput {
  key: string;
  name: string;
  description?: string;
  limits: Limits;
  features: Record<string, boolean>;
  currency: string;
  seats_fixed: number | null;
  min_seats: number | null;
  trial_days: number;
  is_public: boolean;
  sort: number;
  archived: boolean;
  expected_version?: number;
}

/** Create or update non-price plan fields. System admin only. */
export function usePutPlan() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: PutPlanInput) => {
      const {
        key,
        name,
        description,
        limits,
        features,
        currency,
        seats_fixed,
        min_seats,
        trial_days,
        is_public,
        sort,
        archived,
        expected_version,
      } = vars;
      const body: Record<string, unknown> = {
        name,
        ...(description !== undefined ? { description } : {}),
        limits,
        features,
        currency,
        seats_fixed,
        min_seats,
        trial_days,
        is_public,
        sort,
        archived,
        ...(expected_version !== undefined ? { expected_version } : {}),
      };
      const res = await platformFetch(`/plans/${pathSegment(key)}`, {
        method: "PUT",
        body,
      });
      return parsePutPlanResponse(res);
    },
    onSuccess: (_data, vars) =>
      invalidateAll(queryClient, [
        platformKeys.plans(),
        platformKeys.plan(vars.key),
        platformKeys.planHistory(vars.key),
        platformKeys.audits(),
      ]),
  });
}

/** Activate an archived/inactive plan. System admin only. */
export function useActivatePlan() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { key: string }) => {
      const res = await platformFetch(`/plans/${pathSegment(vars.key)}/activate`, {
        method: "POST",
      });
      return parsePlanDetail(res, "POST /api/platform/plans/{key}/activate");
    },
    onSuccess: (_data, vars) =>
      invalidateAll(queryClient, [
        platformKeys.plans(),
        platformKeys.plan(vars.key),
        platformKeys.planHistory(vars.key),
        platformKeys.audits(),
      ]),
  });
}

/** Deactivate a plan. System admin only. */
export function useDeactivatePlan() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { key: string }) => {
      const res = await platformFetch(`/plans/${pathSegment(vars.key)}/deactivate`, {
        method: "POST",
      });
      return parsePlanDetail(res, "POST /api/platform/plans/{key}/deactivate");
    },
    onSuccess: (_data, vars) =>
      invalidateAll(queryClient, [
        platformKeys.plans(),
        platformKeys.plan(vars.key),
        platformKeys.planHistory(vars.key),
        platformKeys.audits(),
      ]),
  });
}

/** Delete an unused plan. System admin only. */
export function useDeletePlan() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { key: string }) => {
      const res = await platformFetch(`/plans/${pathSegment(vars.key)}`, {
        method: "DELETE",
      });
      return parseDeletePlanResponse(res);
    },
    onSuccess: (_data, vars) =>
      invalidateAll(queryClient, [
        platformKeys.plans(),
        platformKeys.plan(vars.key),
        platformKeys.planHistory(vars.key),
        platformKeys.audits(),
      ]),
  });
}

// Price changes (spec 7b.4)

/** List price-change history and any scheduled change for a plan. Billing or Admin. */
export function usePriceChanges(key: string | undefined, enabled = true) {
  return useQuery({
    queryKey: platformKeys.priceChanges(key ?? ""),
    enabled: Boolean(key) && enabled,
    queryFn: async ({ signal }): Promise<PriceChange[]> => {
      const res = await platformFetch(`/plans/${pathSegment(key ?? "")}/price-changes`, { signal });
      return parsePriceChangeList(res).price_changes;
    },
  });
}

export interface SchedulePriceChangeInput {
  key: string;
  effective_from: string;
  new: {
    seat_price_month?: number;
    seat_price_year?: number;
  };
  reason: string;
  confirm_large_change?: boolean;
  dryRun?: boolean;
}

/**
 * Preview (`dryRun: true`) or schedule a price change for a plan. Admin only.
 * Only non-dry submissions invalidate plans and audit caches.
 */
export function useSchedulePriceChange() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (
      vars: SchedulePriceChangeInput,
    ): Promise<PriceChangeDryRunResponse | SchedulePriceChangeResponse> => {
      const { key, dryRun, confirm_large_change, effective_from, new: nextPrices, reason } = vars;
      const path = `/plans/${pathSegment(key)}/price-changes${dryRun ? "?dry_run=1" : ""}`;
      const res = await platformFetch(path, {
        method: "POST",
        body: {
          effective_from,
          new: nextPrices,
          reason,
          confirm_large_change: Boolean(confirm_large_change),
        },
      });
      if (dryRun) {
        return orMalformed(parsePriceChangeDryRunResponse(res));
      }
      return orMalformed(parseSchedulePriceChangeResponse(res));
    },
    onSuccess: (_data, vars) => {
      if (vars.dryRun) return undefined;
      return invalidateAll(queryClient, [
        platformKeys.plans(),
        platformKeys.audits(),
      ]);
    },
  });
}

/** Cancel a plan's currently scheduled price change. Admin only. */
export function useCancelScheduledPriceChange() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { key: string }): Promise<CancelPriceChangeResponse> => {
      const res = await platformFetch(`/plans/${pathSegment(vars.key)}/price-changes/scheduled`, {
        method: "DELETE",
      });
      return orMalformed(parseCancelPriceChangeResponse(res));
    },
    onSuccess: () =>
      invalidateAll(queryClient, [
        platformKeys.plans(),
        platformKeys.audits(),
      ]),
  });
}

// Subscription

export interface UpdateSubscriptionInput {
  workspaceId: string;
  planKey: string;
  seats: number;
  interval: string;
  currentPeriodStart?: string | null;
  currentPeriodEnd?: string | null;
  trialEnd?: string | null;
  graceUntil?: string | null;
  cancelAtPeriodEnd?: boolean;
  limitOverrides?: Limits;
  reason: string;
}

/** Update a workspace subscription. Billing or Admin only. */
export function useUpdateSubscription() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: UpdateSubscriptionInput) => {
      const body: Record<string, unknown> = {
        plan_key: vars.planKey,
        seats: vars.seats,
        interval: vars.interval,
        reason: vars.reason,
      };
      if (vars.currentPeriodStart !== undefined) body.current_period_start = vars.currentPeriodStart;
      if (vars.currentPeriodEnd !== undefined) body.current_period_end = vars.currentPeriodEnd;
      if (vars.trialEnd !== undefined) body.trial_end = vars.trialEnd;
      if (vars.graceUntil !== undefined) body.grace_until = vars.graceUntil;
      if (vars.cancelAtPeriodEnd !== undefined) body.cancel_at_period_end = vars.cancelAtPeriodEnd;
      if (vars.limitOverrides !== undefined) body.limit_overrides = vars.limitOverrides;

      const res = await platformFetch(`/workspaces/${pathSegment(vars.workspaceId)}/subscription`, {
        method: "PUT",
        body,
      });
      return parsePutSubscriptionResponse(res);
    },
    onSuccess: (_data, vars) =>
      invalidateAll(queryClient, [
        platformKeys.workspace(vars.workspaceId),
        platformKeys.workspaces(),
        platformKeys.tenants(),
        platformKeys.overview(),
        platformKeys.audits(),
      ]),
  });
}

/** Verify or unverify a workspace's billing profile. Billing or Admin only. */
export function useSetBillingProfileVerified() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: {
      workspaceId: string;
      verified: boolean;
    }): Promise<BillingProfileVerifyResponse> => {
      const action = vars.verified ? "verify" : "unverify";
      const res = await platformFetch(
        `/workspaces/${pathSegment(vars.workspaceId)}/billing-profile/${action}`,
        { method: "POST" },
      );
      return orMalformed(
        parseBillingProfileVerifyResponse(
          res,
          `POST /api/platform/workspaces/{id}/billing-profile/${action}`,
        ),
      );
    },
    onSuccess: (data, vars) => {
      queryClient.setQueryData<BillingProfile>(
        platformKeys.workspaceBillingProfile(vars.workspaceId),
        (prev) => (prev ? { ...prev, ...data } : prev),
      );
      return invalidateAll(queryClient, [
        platformKeys.workspace(vars.workspaceId),
        platformKeys.workspaceBillingProfile(vars.workspaceId),
        platformKeys.workspaceAudits(vars.workspaceId),
        platformKeys.audits(),
      ]);
    },
  });
}

export interface UpdateWorkspaceBillingProfileInput {
  workspaceId: string;
  account_kind: "individual" | "business";
  legal_name?: string;
  tax_code?: string;
  address?: string;
  invoice_email?: string;
}

/** Update a workspace's billing profile. Billing or Admin only. */
export function useUpdateWorkspaceBillingProfile() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (
      vars: UpdateWorkspaceBillingProfileInput,
    ): Promise<PutBillingProfileResponse> => {
      const body: Record<string, unknown> = {
        account_kind: vars.account_kind,
      };
      if (vars.legal_name !== undefined) body.legal_name = vars.legal_name;
      if (vars.tax_code !== undefined) body.tax_code = vars.tax_code;
      if (vars.address !== undefined) body.address = vars.address;
      if (vars.invoice_email !== undefined) body.invoice_email = vars.invoice_email;

      const res = await platformFetch(
        `/workspaces/${pathSegment(vars.workspaceId)}/billing-profile`,
        {
          method: "PUT",
          body,
        },
      );
      return orMalformed(parsePutBillingProfileResponse(res));
    },
    onSuccess: (data, vars) => {
      const { changed: _changed, ...profile } = data;
      queryClient.setQueryData<BillingProfile>(
        platformKeys.workspaceBillingProfile(vars.workspaceId),
        profile,
      );
      return invalidateAll(queryClient, [
        platformKeys.workspace(vars.workspaceId),
        platformKeys.workspaceBillingProfile(vars.workspaceId),
        platformKeys.workspaceAudits(vars.workspaceId),
        platformKeys.audits(),
      ]);
    },
  });
}

// Account Requests (spec 7b.5)

export type { AccountRequest, AccountRequestState };

export const accountKeys = {
  all: ["platform", "accounts"] as const,
  list: (state?: string) => ["platform", "accounts", "list", state || "all"] as const,
};

export interface AccountBillingProfileInput {
  legal_name: string;
  tax_code: string;
  address: string;
  invoice_email: string;
}

export interface CreateAccountInput {
  kind: "individual" | "business";
  email: string;
  display_name: string;
  workspace_name: string;
  workspace_slug?: string;
  plan_key: string;
  seats: number;
  interval: "month" | "year";
  trial?: boolean;
  comp_reason?: string;
  billing_profile?: AccountBillingProfileInput | null;
}

/** A PlatformApiError enriched with field names and Retry-After wait for account endpoints. */
export class AccountApiError extends PlatformApiError {
  readonly fields: string[];
  readonly retryAfter: string | null;
  readonly state: string | null;

  constructor(
    status: number,
    code: string | null,
    message: string,
    fields: string[] = [],
    retryAfter: string | null = null,
    state: string | null = null,
  ) {
    super(status, code, message);
    this.name = "AccountApiError";
    this.fields = fields;
    this.retryAfter = retryAfter;
    this.state = state;
  }
}

async function accountFetch(
  path: string,
  init: Omit<RequestInit, "body" | "credentials"> & { body?: unknown } = {},
): Promise<{ status: number; data: unknown }> {
  const { body, headers, ...rest } = init;
  const requestHeaders = new Headers(headers);
  requestHeaders.set("Accept", "application/json");
  if (body !== undefined) requestHeaders.set("Content-Type", "application/json");

  const res = await fetch(`/api/platform${path}`, {
    ...rest,
    headers: requestHeaders,
    body: body === undefined ? undefined : JSON.stringify(body),
    credentials: "same-origin",
  });

  if (!res.ok) {
    const retryAfter = res.headers.get("Retry-After");
    const fallbackMessage = `Request failed with status ${res.status}`;
    let parsed: unknown = null;
    try {
      parsed = JSON.parse(await res.text());
    } catch {
      throw new AccountApiError(res.status, null, fallbackMessage, [], retryAfter, null);
    }
    if (typeof parsed !== "object" || parsed === null) {
      throw new AccountApiError(res.status, null, fallbackMessage, [], retryAfter, null);
    }
    const obj = parsed as { code?: unknown; error?: unknown; fields?: unknown; state?: unknown };
    const code = typeof obj.code === "string" ? obj.code : null;
    const message = typeof obj.error === "string" && obj.error !== "" ? obj.error : fallbackMessage;
    const fields = Array.isArray(obj.fields)
      ? obj.fields.filter((f): f is string => typeof f === "string")
      : [];
    const state = typeof obj.state === "string" ? obj.state : null;
    throw new AccountApiError(res.status, code, message, fields, retryAfter, state);
  }

  const text = await res.text();
  if (text === "") return { status: res.status, data: null };
  try {
    return { status: res.status, data: JSON.parse(text) };
  } catch {
    throw new PlatformApiError(res.status, "malformed_response", "The server answered with a body that is not JSON");
  }
}

/** List account requests, optionally filtered by state. Admin only. */
export function useAccountRequests(state?: Exclude<AccountRequestState, "unknown"> | "") {
  const normalizedState = state || undefined;
  return useQuery({
    queryKey: accountKeys.list(normalizedState),
    queryFn: async ({ signal }): Promise<AccountRequest[]> => {
      const res = await platformFetch(`/accounts${queryString({ state: normalizedState })}`, { signal });
      return orMalformed(parseAccountRequestList(res)).requests;
    },
  });
}

/** Create an account request with an Idempotency-Key header. Admin only. */
export function useCreateAccountRequest() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { input: CreateAccountInput; idempotencyKey: string }): Promise<AccountRequest> => {
      const { input, idempotencyKey } = vars;
      const body: Record<string, unknown> = {
        kind: input.kind,
        email: input.email,
        display_name: input.display_name,
        workspace_name: input.workspace_name,
        workspace_slug: input.workspace_slug ?? "",
        plan_key: input.plan_key,
        seats: input.seats,
        interval: input.interval,
        trial: Boolean(input.trial),
        comp_reason: input.comp_reason ?? "",
      };
      if (input.billing_profile) {
        body.billing_profile = input.billing_profile;
      }
      const res = await accountFetch("/accounts", {
        method: "POST",
        headers: { "Idempotency-Key": idempotencyKey },
        body,
      });
      return orMalformed(parseAccountRequest(res.data, "POST /api/platform/accounts"));
    },
    onSuccess: () => invalidateAll(queryClient, [accountKeys.all, platformKeys.audits()]),
  });
}

/** Cancel a pending account request. Admin only. */
export function useCancelAccountRequest() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { id: string }): Promise<AccountRequest> => {
      const res = await accountFetch(`/accounts/${pathSegment(vars.id)}/cancel`, {
        method: "POST",
      });
      return orMalformed(parseAccountRequest(res.data, "POST /api/platform/accounts/{id}/cancel"));
    },
    onSuccess: () => invalidateAll(queryClient, [accountKeys.all, platformKeys.audits()]),
  });
}

/** Resend the welcome email for a completed or finalize_failed account request. Admin only. */
export function useResendWelcome() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { id: string }) => {
      const res = await accountFetch(`/accounts/${pathSegment(vars.id)}/resend-welcome`, {
        method: "POST",
      });
      if (res.data === null) return null;
      return orMalformed(parseResendWelcomeResponse(res.data));
    },
    onSuccess: () => invalidateAll(queryClient, [accountKeys.all, platformKeys.audits()]),
  });
}

// E-invoices (spec 7.3, 7.6)

export type EinvoiceFilterState = Exclude<EinvoiceState, "cancelled" | "unknown"> | "";

/** List e-invoice issuance records, defaulting to failed and needs_info. Billing or Admin only. */
export function useEinvoices(state: EinvoiceFilterState = "") {
  return useQuery({
    queryKey: platformKeys.einvoiceList(state),
    queryFn: async ({ signal }): Promise<Einvoice[]> => {
      const res = await platformFetch(`/einvoices${queryString({ state: state || undefined })}`, { signal });
      return parseEinvoiceList(res).einvoices;
    },
  });
}

/** Fetch the e-invoice revision history for an order, newest first. Billing or Admin only. */
export function useOrderEinvoices(orderId: string | null) {
  return useQuery({
    queryKey: platformKeys.orderEinvoices(orderId ?? ""),
    enabled: orderId !== null && orderId !== "",
    queryFn: async ({ signal }): Promise<OrderEinvoice[]> => {
      const res = await platformFetch(`/orders/${pathSegment(orderId ?? "")}/einvoices`, { signal });
      return orMalformed(parseOrderEinvoiceList(res)).einvoices;
    },
  });
}

// Workspace member requests (R6 / S13)

/** List async member-add requests for a workspace (system_admin or admin). */
export function useMemberRequests(workspaceId: string, enabled = true) {
  return useQuery({
    queryKey: platformKeys.memberRequests(workspaceId),
    enabled: enabled && workspaceId !== "",
    queryFn: async ({ signal }): Promise<MemberRequest[]> => {
      const res = await platformFetch(`/workspaces/${pathSegment(workspaceId)}/member-requests`, { signal });
      return parseMemberRequestList(res).requests;
    },
  });
}

/** Add a member to a workspace (`admin`, `member`, or `viewer`). */
export function useAddWorkspaceMember() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { workspaceId: string; email: string; role: MemberRole }): Promise<MemberRequest> => {
      const res = await platformFetch(`/workspaces/${pathSegment(vars.workspaceId)}/members`, {
        method: "POST",
        body: { email: vars.email, role: vars.role },
      });
      return orMalformed(parseMemberRequest(res, "POST /api/platform/workspaces/{id}/members"));
    },
    onSuccess: (_data, vars) =>
      invalidateAll(queryClient, [
        platformKeys.memberRequests(vars.workspaceId),
        platformKeys.workspace(vars.workspaceId),
        platformKeys.audits(),
      ]),
  });
}

/** Cancel a pending workspace member request. */
export function useCancelMemberRequest() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { workspaceId: string; requestId: string }): Promise<MemberRequest> => {
      const res = await platformFetch(`/member-requests/${pathSegment(vars.requestId)}/cancel`, {
        method: "POST",
      });
      return orMalformed(parseMemberRequest(res, "POST /api/platform/member-requests/{id}/cancel"));
    },
    onSuccess: (_data, vars) =>
      invalidateAll(queryClient, [
        platformKeys.memberRequests(vars.workspaceId),
        platformKeys.audits(),
      ]),
  });
}

// Immediate deletions & deletion requests (R6 / S13)

/** Start immediate deletion of a workspace (system_admin or admin within tenant). */
export function useDeleteWorkspaceNow() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: {
      workspaceId: string;
      reason: string;
      confirm: string;
      export_first?: boolean;
    }): Promise<DeletionStartResponse> => {
      const res = await platformFetch(`/workspaces/${pathSegment(vars.workspaceId)}/delete-now`, {
        method: "POST",
        body: {
          reason: vars.reason,
          confirm: vars.confirm,
          ...(vars.export_first !== undefined ? { export_first: vars.export_first } : {}),
        },
      });
      return orMalformed(parseDeletionStartResponse(res, "POST /api/platform/workspaces/{id}/delete-now"));
    },
    onSuccess: (_data, vars) =>
      invalidateAll(queryClient, [
        platformKeys.workspace(vars.workspaceId),
        platformKeys.workspaces(),
        platformKeys.tenants(),
        platformKeys.deletionRequests(),
        platformKeys.audits(),
      ]),
  });
}

/** Start immediate deletion of a tenant (system_admin only). */
export function useDeleteTenantNow() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: {
      tenantId: string;
      reason: string;
      confirm: string;
      export_first?: boolean;
    }): Promise<DeletionStartResponse> => {
      const res = await platformFetch(`/tenants/${pathSegment(vars.tenantId)}/delete-now`, {
        method: "POST",
        body: {
          reason: vars.reason,
          confirm: vars.confirm,
          ...(vars.export_first !== undefined ? { export_first: vars.export_first } : {}),
        },
      });
      return orMalformed(parseDeletionStartResponse(res, "POST /api/platform/tenants/{id}/delete-now"));
    },
    onSuccess: (_data, vars) =>
      invalidateAll(queryClient, [
        platformKeys.tenant(vars.tenantId),
        platformKeys.tenants(),
        platformKeys.workspaces(),
        platformKeys.deletionRequests(),
        platformKeys.audits(),
      ]),
  });
}

/** Ownership impact preview before deleting an account immediately (system_admin only). */
export function useAccountDeletePreview(userId: string, enabled = false) {
  return useQuery({
    queryKey: platformKeys.accountDeletePreview(userId),
    enabled: enabled && userId !== "",
    queryFn: async ({ signal }): Promise<AccountDeletePreview> => {
      const res = await platformFetch(`/users/${pathSegment(userId)}/delete-now-preview`, { signal });
      return orMalformed(parseAccountDeletePreview(res));
    },
  });
}

export interface DeleteAccountResolution {
  workspace_id: string;
  action: "transfer" | "delete";
  successor_user_id?: string;
}

/** Start immediate deletion of an account with ownership resolutions (system_admin only). */
export function useDeleteAccountNow() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: {
      userId: string;
      reason: string;
      confirm: string;
      resolutions?: DeleteAccountResolution[];
    }): Promise<DeletionStartResponse> => {
      const res = await platformFetch(`/users/${pathSegment(vars.userId)}/delete-now`, {
        method: "POST",
        body: {
          reason: vars.reason,
          confirm: vars.confirm,
          resolutions: vars.resolutions ?? [],
        },
      });
      return orMalformed(parseDeletionStartResponse(res, "POST /api/platform/users/{id}/delete-now"));
    },
    onSuccess: (_data, vars) =>
      invalidateAll(queryClient, [
        platformKeys.user(vars.userId),
        platformKeys.users(),
        platformKeys.deletionRequests(),
        platformKeys.audits(),
      ]),
  });
}

/**
 * List immediate deletion requests.
 * Note: the backend's validDeletionKind rejects `?kind=account` and
 * validDeletionState rejects `awaiting_approval` / `legal_hold` / `export_timeout`
 * on main, so we omit them from the server query and filter client-side.
 */
export function useDeletionRequests(filters: DeletionRequestFilters = {}) {
  const f = clean(filters);
  return useQuery({
    queryKey: platformKeys.deletionRequestList(f),
    queryFn: async ({ signal }): Promise<DeletionRequest[]> => {
      const serverKind = f.kind && f.kind !== "account" ? f.kind : undefined;
      const clientOnlyState =
        f.state === "awaiting_approval" || f.state === "legal_hold" || f.state === "export_timeout";
      const serverState = f.state && !clientOnlyState ? f.state : undefined;
      const res = await platformFetch(
        `/deletion-requests${queryString({ kind: serverKind, state: serverState })}`,
        { signal },
      );
      let list = parseDeletionRequestList(res).requests;
      if (f.kind === "account") {
        list = list.filter((r) => r.kind === "account");
      }
      if (clientOnlyState) {
        list = list.filter((r) => r.state === f.state || r.error_code === f.state);
      }
      return list;
    },
  });
}

/** Cancel a pending or failed deletion request. */
export function useCancelDeletionRequest() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { id: string }): Promise<DeletionRequest> => {
      const res = await platformFetch(`/deletion-requests/${pathSegment(vars.id)}/cancel`, {
        method: "POST",
      });
      return orMalformed(parseDeletionRequest(res, "POST /api/platform/deletion-requests/{id}/cancel"));
    },
    onSuccess: () => invalidateAll(queryClient, [platformKeys.deletionRequests(), platformKeys.audits()]),
  });
}

/** Retry a failed deletion request. */
export function useRetryDeletionRequest() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { id: string }): Promise<DeletionRequest> => {
      const res = await platformFetch(`/deletion-requests/${pathSegment(vars.id)}/retry`, {
        method: "POST",
      });
      return orMalformed(parseDeletionRequest(res, "POST /api/platform/deletion-requests/{id}/retry"));
    },
    onSuccess: () => invalidateAll(queryClient, [platformKeys.deletionRequests(), platformKeys.audits()]),
  });
}

// Tenant users & member management (R6 / S24)

/** Page users who belong to at least one workspace of the tenant. */
export function useTenantUsers(tenantId: string | undefined, filters: TenantUserFilters = {}) {
  const f = clean(filters);
  return useInfiniteQuery({
    queryKey: platformKeys.tenantUserList(tenantId ?? "", f),
    enabled: Boolean(tenantId),
    queryFn: async ({ pageParam, signal }) =>
      parseTenantUserPage(
        await platformFetch(
          `/tenants/${pathSegment(tenantId ?? "")}/users${queryString({
            q: f.q,
            account_status: f.account_status,
            workspace_id: f.workspace_id,
            member_role: f.member_role,
            cursor: pageParam,
          })}`,
          { signal },
        ),
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    select: (data) => data.pages.flatMap((page) => page.users),
  });
}

/** Fetch one user's memberships and allocation/audit summary inside a tenant. */
export function useTenantUser(
  tenantId: string | undefined,
  userId: string | undefined,
  enabled = true,
) {
  return useQuery({
    queryKey: platformKeys.tenantUser(tenantId ?? "", userId ?? ""),
    enabled: Boolean(tenantId && userId) && enabled,
    queryFn: async ({ signal }): Promise<TenantUserDetail> =>
      orMalformed(
        parseTenantUserDetail(
          await platformFetch(
            `/tenants/${pathSegment(tenantId ?? "")}/users/${pathSegment(userId ?? "")}`,
            { signal },
          ),
        ),
      ),
  });
}

/** Queue member removal requests across all workspaces of a tenant for one user. */
export function useDeactivateTenantUser() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: {
      tenantId: string;
      userId: string;
      reason: string;
    }): Promise<DeactivateTenantUserResponse> => {
      const res = await platformFetch(
        `/tenants/${pathSegment(vars.tenantId)}/users/${pathSegment(vars.userId)}/deactivate`,
        {
          method: "POST",
          body: { reason: vars.reason },
        },
      );
      return orMalformed(parseDeactivateTenantUserResponse(res));
    },
    onSuccess: (_data, vars) =>
      invalidateAll(queryClient, [
        platformKeys.tenantUsersAll(vars.tenantId),
        platformKeys.tenantUser(vars.tenantId, vars.userId),
        platformKeys.tenant(vars.tenantId),
        platformKeys.user(vars.userId),
        platformKeys.audits(),
      ]),
  });
}

/** Queue a member removal request for a user in a single workspace. */
export function useRemoveWorkspaceMember() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: {
      workspaceId: string;
      userId: string;
      tenantId?: string;
    }): Promise<MemberChangeResponse> => {
      const res = await platformFetch(
        `/workspaces/${pathSegment(vars.workspaceId)}/members/${pathSegment(vars.userId)}/remove`,
        { method: "POST" },
      );
      return orMalformed(
        parseMemberChangeResponse(
          res,
          "POST /api/platform/workspaces/{id}/members/{user_id}/remove",
        ),
      );
    },
    onSuccess: (_data, vars) =>
      invalidateAll(queryClient, [
        platformKeys.memberRequests(vars.workspaceId),
        platformKeys.workspace(vars.workspaceId),
        platformKeys.user(vars.userId),
        platformKeys.audits(),
        ...(vars.tenantId
          ? [
              platformKeys.tenantUsersAll(vars.tenantId),
              platformKeys.tenantUser(vars.tenantId, vars.userId),
            ]
          : []),
      ]),
  });
}

/** Queue a role change request (`admin`, `member`, or `viewer`) for a workspace member. */
export function useChangeWorkspaceMemberRole() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: {
      workspaceId: string;
      userId: string;
      role: MemberRole;
      tenantId?: string;
    }): Promise<MemberChangeResponse> => {
      const res = await platformFetch(
        `/workspaces/${pathSegment(vars.workspaceId)}/members/${pathSegment(vars.userId)}/role`,
        {
          method: "POST",
          body: { role: vars.role },
        },
      );
      return orMalformed(
        parseMemberChangeResponse(
          res,
          "POST /api/platform/workspaces/{id}/members/{user_id}/role",
        ),
      );
    },
    onSuccess: (_data, vars) =>
      invalidateAll(queryClient, [
        platformKeys.memberRequests(vars.workspaceId),
        platformKeys.workspace(vars.workspaceId),
        platformKeys.user(vars.userId),
        platformKeys.audits(),
        ...(vars.tenantId
          ? [
              platformKeys.tenantUsersAll(vars.tenantId),
              platformKeys.tenantUser(vars.tenantId, vars.userId),
            ]
          : []),
      ]),
  });
}

/** Submit a workspace quota increase request (system_admin or admin within tenant). */
export function useCreateQuotaRequest() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: {
      workspaceId: string;
      gate: string;
      requested_value: number;
      reason: string;
    }): Promise<QuotaRequest> => {
      const res = await platformFetch(
        `/workspaces/${pathSegment(vars.workspaceId)}/quota-requests`,
        {
          method: "POST",
          body: {
            gate: vars.gate,
            requested_value: vars.requested_value,
            reason: vars.reason,
          },
        },
      );
      return orMalformed(
        parseQuotaRequest(res, "POST /api/platform/workspaces/{id}/quota-requests"),
      );
    },
    onSuccess: (_data, vars) =>
      invalidateAll(queryClient, [
        platformKeys.quotaRequests(),
        platformKeys.workspace(vars.workspaceId),
        platformKeys.audits(),
      ]),
  });
}

// Resources & tenant limit overrides (R6 / S25)

function mergeUniqueBy<T>(items: T[], keyOf: (item: T) => string): T[] {
  const seen = new Set<string>();
  const out: T[] = [];
  for (const item of items) {
    const key = keyOf(item);
    if (!seen.has(key)) {
      seen.add(key);
      out.push(item);
    }
  }
  return out;
}

/**
 * Fetch resource usage per type against plan limits for one tenant.
 * Pending backend endpoint (`GET /tenants/{id}/resources`, plan S20; not on main yet).
 */
export function useTenantResources(tenantId: string | undefined, enabled = true) {
  return useQuery({
    queryKey: platformKeys.tenantResources(tenantId ?? ""),
    enabled: Boolean(tenantId) && enabled,
    queryFn: async ({ signal }): Promise<TenantResources> =>
      orMalformed(
        parseTenantResources(
          await platformFetch(`/tenants/${pathSegment(tenantId ?? "")}/resources`, { signal }),
        ),
      ),
  });
}

/**
 * Fetch system-wide resource usage summary, top tenants, and over-limit rows (system_admin only).
 * Pending backend endpoint (`GET /resources`, plan S20; not on main yet).
 */
export function useSystemResources(filters: SystemResourceFilters = {}, enabled = true) {
  const f = clean(filters);
  return useInfiniteQuery({
    queryKey: platformKeys.resourceList(f),
    enabled,
    queryFn: async ({ pageParam, signal }) =>
      parseSystemResources(
        await platformFetch(
          `/resources${queryString({
            sort: f.sort,
            top: f.top !== undefined ? String(f.top) : undefined,
            cursor: pageParam,
          })}`,
          { signal },
        ),
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    select: (data) => {
      const first = data.pages[0];
      const last = data.pages[data.pages.length - 1];
      return {
        by_type: first?.by_type ?? [],
        by_plan: first?.by_plan ?? [],
        top_tenants: mergeUniqueBy(
          data.pages.flatMap((page) => page.top_tenants),
          (t) => t.tenant_id || t.tenant_name,
        ),
        tenants: mergeUniqueBy(
          data.pages.flatMap((page) => page.tenants),
          (t) => t.tenant_id || t.tenant_name,
        ),
        over_limit: mergeUniqueBy(
          data.pages.flatMap((page) => page.over_limit),
          (o) => `${o.tenant_id}:${o.workspace_id}:${o.type}`,
        ),
        complete: last?.complete ?? false,
      };
    },
  });
}

/**
 * Set or release (`value: null`) one limit gate across all or selected workspaces of a tenant.
 * Live backend endpoint (`POST /tenants/{id}/limit-overrides`, system_admin and billing).
 */
export function useSetTenantLimitOverride() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: {
      tenantId: string;
      gate: string;
      value: number | null;
      workspace_ids?: string[];
      reason: string;
    }): Promise<TenantLimitOverrideResponse> => {
      const body: Record<string, unknown> = {
        gate: vars.gate,
        value: vars.value,
        reason: vars.reason,
      };
      if (vars.workspace_ids && vars.workspace_ids.length > 0) {
        body.workspace_ids = vars.workspace_ids;
      }
      const res = await platformFetch(
        `/tenants/${pathSegment(vars.tenantId)}/limit-overrides`,
        {
          method: "POST",
          body,
        },
      );
      return orMalformed(parseTenantLimitOverrideResponse(res));
    },
    onSuccess: (_data, vars) =>
      invalidateAll(queryClient, [
        platformKeys.tenantResources(vars.tenantId),
        platformKeys.tenant(vars.tenantId),
        platformKeys.tenants(),
        platformKeys.workspaces(),
        platformKeys.resources(),
        platformKeys.audits(),
      ]),
  });
}

// Runtimes (R6 / S25)

/** Page runtimes visible to the staff member (all for system_admin/support; own tenant for admin). */
export function useRuntimes(filters: RuntimeFilters = {}, enabled = true) {
  const f = clean(filters);
  return useInfiniteQuery({
    queryKey: platformKeys.runtimeList(f),
    enabled,
    queryFn: async ({ pageParam, signal }) =>
      parseRuntimePage(
        await platformFetch(
          `/runtimes${queryString({
            q: f.q,
            status: f.status,
            tenant_id: f.tenant_id,
            workspace_id: f.workspace_id,
            disabled: f.disabled ? "true" : undefined,
            cursor: pageParam,
          })}`,
          { signal },
        ),
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    select: (data) => data.pages.flatMap((page) => page.runtimes),
  });
}

/** Fetch one runtime with recent tasks (without content) and 7-day failed task count. */
export function useRuntime(id: string | undefined, enabled = true) {
  return useQuery({
    queryKey: platformKeys.runtime(id ?? ""),
    enabled: Boolean(id) && enabled,
    queryFn: async ({ signal }): Promise<RuntimeDetail> =>
      orMalformed(
        parseRuntimeDetail(await platformFetch(`/runtimes/${pathSegment(id ?? "")}`, { signal })),
      ),
  });
}

/** Disable or enable a runtime with a required reason. */
export function useSetRuntimeStatus() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: {
      runtimeId: string;
      status: "disabled" | "enabled";
      reason: string;
    }): Promise<SetRuntimeStatusResponse> => {
      const res = await platformFetch(`/runtimes/${pathSegment(vars.runtimeId)}/status`, {
        method: "POST",
        body: { status: vars.status, reason: vars.reason },
      });
      return orMalformed(parseSetRuntimeStatusResponse(res));
    },
    onSuccess: (_data, vars) =>
      invalidateAll(queryClient, [
        platformKeys.runtime(vars.runtimeId),
        platformKeys.runtimes(),
        platformKeys.audits(),
      ]),
  });
}

/** Request runtime unregistration with required reason and runtime name confirmation. */
export function useUnregisterRuntime() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: {
      runtimeId: string;
      reason: string;
      confirm: string;
    }): Promise<DeletionStartResponse> => {
      const res = await platformFetch(`/runtimes/${pathSegment(vars.runtimeId)}/unregister`, {
        method: "POST",
        body: { reason: vars.reason, confirm: vars.confirm },
      });
      return orMalformed(parseRuntimeUnregisterResponse(res));
    },
    onSuccess: (_data, vars) =>
      invalidateAll(queryClient, [
        platformKeys.runtime(vars.runtimeId),
        platformKeys.runtimes(),
        platformKeys.deletionRequests(),
        platformKeys.audits(),
      ]),
  });
}

// Tenant policy, tenant status, quota requests, approvals, legal holds, and audit export (R6 / S36)

/** Fetch the effective policy and tenant floor values for a tenant. */
export function useTenantPolicy(tenantId: string | undefined, enabled = true) {
  return useQuery({
    queryKey: platformKeys.tenantPolicy(tenantId ?? ""),
    enabled: Boolean(tenantId) && enabled,
    queryFn: async ({ signal }): Promise<TenantPolicyResponse> =>
      orMalformed(
        parseTenantPolicyResponse(
          await platformFetch(`/tenants/${pathSegment(tenantId ?? "")}/policy`, { signal }),
        ),
      ),
  });
}

/** Update the tenant policy floor using optimistic version concurrency (`expected_version`). */
export function useUpdateTenantPolicy() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: {
      tenantId: string;
      expected_version: number;
      allowed_invite_domains: string[];
      share_links_enabled: boolean;
      require_sso: boolean;
      require_mfa: boolean;
      session_max_age_hours: number;
    }): Promise<TenantPolicyResponse> => {
      const res = await platformFetch(`/tenants/${pathSegment(vars.tenantId)}/policy`, {
        method: "PUT",
        body: {
          expected_version: vars.expected_version,
          allowed_invite_domains: vars.allowed_invite_domains,
          share_links_enabled: vars.share_links_enabled,
          require_sso: vars.require_sso,
          require_mfa: vars.require_mfa,
          session_max_age_hours: vars.session_max_age_hours,
        },
      });
      return orMalformed(
        parseTenantPolicyResponse(res, "PUT /api/platform/tenants/{id}/policy"),
      );
    },
    onSuccess: (_data, vars) =>
      invalidateAll(queryClient, [
        platformKeys.tenantPolicy(vars.tenantId),
        platformKeys.tenant(vars.tenantId),
        platformKeys.audits(),
      ]),
  });
}

/** Lock or unlock all workspaces of a tenant (system_admin). */
export function useSetTenantStatus() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: {
      tenantId: string;
      status: SettableWorkspaceStatus;
      reason: string;
    }): Promise<TenantStatusResponse> => {
      const res = await platformFetch(`/tenants/${pathSegment(vars.tenantId)}/status`, {
        method: "POST",
        body: { status: vars.status, reason: vars.reason },
      });
      return orMalformed(parseTenantStatusResponse(res));
    },
    onSuccess: (_data, vars) =>
      invalidateAll(queryClient, [
        platformKeys.tenant(vars.tenantId),
        platformKeys.tenants(),
        platformKeys.workspaces(),
        platformKeys.audits(),
      ]),
  });
}

/** List quota increase requests (all for system_admin/billing; own tenant for admin). */
export function useQuotaRequests(filters: QuotaRequestFilters = {}, enabled = true) {
  const f = clean(filters);
  return useInfiniteQuery({
    queryKey: platformKeys.quotaRequestList(f),
    enabled,
    queryFn: async ({ pageParam, signal }) =>
      parseQuotaRequestPage(
        await platformFetch(
          `/quota-requests${queryString({ state: f.state, cursor: pageParam })}`,
          { signal },
        ),
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    select: (data) => data.pages.flatMap((page) => page.requests),
  });
}

/** Fetch one quota request by id. */
export function useQuotaRequest(id: string | undefined, enabled = true) {
  return useQuery({
    queryKey: platformKeys.quotaRequest(id ?? ""),
    enabled: Boolean(id) && enabled,
    queryFn: async ({ signal }): Promise<QuotaRequest> =>
      orMalformed(
        parseQuotaRequest(
          await platformFetch(`/quota-requests/${pathSegment(id ?? "")}`, { signal }),
          "GET /api/platform/quota-requests/{id}",
        ),
      ),
  });
}

/** Approve a pending quota request with an optional adjusted value and required reason. */
export function useApproveQuotaRequest() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: {
      id: string;
      value?: number;
      reason: string;
    }): Promise<QuotaRequest> => {
      const res = await platformFetch(`/quota-requests/${pathSegment(vars.id)}/approve`, {
        method: "POST",
        body: {
          ...(vars.value !== undefined ? { value: vars.value } : {}),
          reason: vars.reason,
        },
      });
      return orMalformed(
        parseQuotaRequest(res, "POST /api/platform/quota-requests/{id}/approve"),
      );
    },
    onSuccess: (_data, vars) =>
      invalidateAll(queryClient, [
        platformKeys.quotaRequests(),
        platformKeys.quotaRequest(vars.id),
        platformKeys.workspaces(),
        platformKeys.tenants(),
        platformKeys.audits(),
      ]),
  });
}

/** Deny a pending quota request with a required reason. */
export function useDenyQuotaRequest() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { id: string; reason: string }): Promise<QuotaRequest> => {
      const res = await platformFetch(`/quota-requests/${pathSegment(vars.id)}/deny`, {
        method: "POST",
        body: { reason: vars.reason },
      });
      return orMalformed(parseQuotaRequest(res, "POST /api/platform/quota-requests/{id}/deny"));
    },
    onSuccess: (_data, vars) =>
      invalidateAll(queryClient, [
        platformKeys.quotaRequests(),
        platformKeys.quotaRequest(vars.id),
        platformKeys.audits(),
      ]),
  });
}

/** List two-person deletion requests awaiting approval (system_admin). */
export function useApprovals(enabled = true) {
  return useQuery({
    queryKey: platformKeys.approvals(),
    enabled,
    queryFn: async ({ signal }): Promise<DeletionRequest[]> =>
      parseApprovalList(await platformFetch("/approvals", { signal })).approvals,
  });
}

/** Second-person approval for a pending deletion request (system_admin). */
export function useApproveApproval() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { id: string }): Promise<DeletionStartResponse> => {
      const res = await platformFetch(`/approvals/${pathSegment(vars.id)}/approve`, {
        method: "POST",
      });
      return orMalformed(parseDeletionStartResponse(res, "POST /api/platform/approvals/{id}/approve"));
    },
    onSuccess: () =>
      invalidateAll(queryClient, [
        platformKeys.approvals(),
        platformKeys.deletionRequests(),
        platformKeys.audits(),
      ]),
  });
}

/** Reject a deletion request awaiting approval (system_admin). */
export function useRejectApproval() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { id: string }): Promise<DeletionStartResponse> => {
      const res = await platformFetch(`/approvals/${pathSegment(vars.id)}/reject`, {
        method: "POST",
      });
      return orMalformed(parseDeletionStartResponse(res, "POST /api/platform/approvals/{id}/reject"));
    },
    onSuccess: () =>
      invalidateAll(queryClient, [
        platformKeys.approvals(),
        platformKeys.deletionRequests(),
        platformKeys.audits(),
      ]),
  });
}

/** List legal holds across tenants, workspaces, and accounts (system_admin). */
export function useLegalHolds(enabled = true) {
  return useQuery({
    queryKey: platformKeys.legalHolds(),
    enabled,
    queryFn: async ({ signal }): Promise<LegalHold[]> =>
      parseLegalHoldList(await platformFetch("/legal-holds", { signal })).legal_holds,
  });
}

/** Place a legal hold with a required reason and case reference (system_admin). */
export function usePlaceLegalHold() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: {
      scope_kind: Exclude<LegalHoldScopeKind, "unknown">;
      scope_id: string;
      reason: string;
      case_ref: string;
    }): Promise<LegalHold> => {
      const res = await platformFetch("/legal-holds", {
        method: "POST",
        body: {
          scope_kind: vars.scope_kind,
          scope_id: vars.scope_id,
          reason: vars.reason,
          case_ref: vars.case_ref,
        },
      });
      return orMalformed(parseLegalHold(res, "POST /api/platform/legal-holds"));
    },
    onSuccess: () =>
      invalidateAll(queryClient, [
        platformKeys.legalHolds(),
        platformKeys.tenants(),
        platformKeys.workspaces(),
        platformKeys.users(),
        platformKeys.deletionRequests(),
        platformKeys.audits(),
      ]),
  });
}

/** Release an active legal hold (system_admin). */
export function useReleaseLegalHold() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { id: string }): Promise<LegalHold> => {
      const res = await platformFetch(`/legal-holds/${pathSegment(vars.id)}/release`, {
        method: "POST",
      });
      return orMalformed(parseLegalHold(res, "POST /api/platform/legal-holds/{id}/release"));
    },
    onSuccess: () =>
      invalidateAll(queryClient, [
        platformKeys.legalHolds(),
        platformKeys.tenants(),
        platformKeys.workspaces(),
        platformKeys.users(),
        platformKeys.deletionRequests(),
        platformKeys.audits(),
      ]),
  });
}

/** Export audit events in NDJSON or CSV format for up to a 31-day window. */
export function useExportAudit() {
  return useMutation({
    mutationFn: async (vars: AuditExportFilters): Promise<PlatformTextResponse> =>
      platformFetchText(
        `/audit-events/export${queryString({
          from: vars.from,
          to: vars.to,
          format: vars.format,
          action: vars.action,
          actor_id: vars.actor_id,
          workspace_id: vars.workspace_id,
          cursor: vars.cursor,
          limit: vars.limit !== undefined ? String(vars.limit) : undefined,
        })}`,
        {
          accept: vars.format === "csv" ? "text/csv" : "application/x-ndjson",
        },
      ),
  });
}
