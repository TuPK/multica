import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query";
import { PlatformApiError, platformFetch } from "./client";
import {
  parseAuditPage,
  parseMe,
  parseNotePage,
  parseOverview,
  parseStaffList,
  parseTenantDetail,
  parseTenantPage,
  parseTenantUsage,
  parseUserDetail,
  parseUserPage,
  parseOrderList,
  parseOrderActionResult,
  parsePlanList,
  parsePutPlanResponse,
  parseDeletePlanResponse,
  parseVerifyCodeResponse,
  parseResetMfaResponse,
  parsePutSubscriptionResponse,
  parseAccountRequestList,
  parseAccountRequest,
  parseResendWelcomeResponse,
  type Me,
  type Overview,
  type StaffRole,
  type TenantDetail,
  type TenantStatus,
  type TenantUsage,
  type UserDetail,
  type Order,
  type Plan,
  type Subscription,
  type Limits,
  type AccountRequest,
  type AccountRequestState,
} from "./schemas";

export type { Subscription, Order, Plan, Limits, TenantUsage };

/** The states staff may set on a workspace. */
export type SettableTenantStatus = Exclude<TenantStatus, "unknown">;
/** The states staff may set on an account. */
export type SettableAccountStatus = "active" | "suspended";
/** The roles staff may grant. */
export type SettableStaffRole = Exclude<StaffRole, "unknown">;

// Filters

export interface SearchFilters {
  /** Substring of the name, slug or email. */
  q?: string;
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

/** Drops empty values, so `{}` and `{ q: "" }` are one query. */
function clean<T extends object>(filters: T): T {
  const out: Record<string, string> = {};
  for (const [key, value] of Object.entries(filters)) {
    if (typeof value === "string" && value.trim() !== "") out[key] = value.trim();
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
  tenants: () => ["platform", "tenants"] as const,
  tenantLists: () => ["platform", "tenants", "list"] as const,
  tenantList: (filters: SearchFilters) => ["platform", "tenants", "list", clean(filters)] as const,
  tenant: (id: string) => ["platform", "tenants", "detail", id] as const,
  tenantUsage: (id: string) => ["platform", "tenants", "usage", id] as const,
  tenantAudits: (id: string) => ["platform", "tenants", "audit", id] as const,
  tenantAudit: (id: string, filters: AuditFilters) => ["platform", "tenants", "audit", id, clean(filters)] as const,
  tenantNotes: (id: string) => ["platform", "tenants", "notes", id] as const,
  users: () => ["platform", "users"] as const,
  userLists: () => ["platform", "users", "list"] as const,
  userList: (filters: SearchFilters) => ["platform", "users", "list", clean(filters)] as const,
  user: (id: string) => ["platform", "users", "detail", id] as const,
  staff: () => ["platform", "staff"] as const,
  orders: () => ["platform", "orders"] as const,
  orderList: (status: string) => ["platform", "orders", "list", status] as const,
  plans: () => ["platform", "plans"] as const,
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

// Tenants

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

export function useTenant(id: string | undefined) {
  return useQuery({
    queryKey: platformKeys.tenant(id ?? ""),
    enabled: !!id,
    queryFn: async ({ signal }): Promise<TenantDetail> =>
      orMalformed(parseTenantDetail(await platformFetch(`/tenants/${pathSegment(id ?? "")}`, { signal }))),
  });
}

export function useTenantUsage(id: string | undefined) {
  return useQuery({
    queryKey: platformKeys.tenantUsage(id ?? ""),
    enabled: !!id,
    queryFn: async ({ signal }): Promise<TenantUsage> =>
      orMalformed(parseTenantUsage(await platformFetch(`/tenants/${pathSegment(id ?? "")}/usage`, { signal }))),
  });
}

function invalidateAll(queryClient: QueryClient, keys: readonly (readonly unknown[])[]) {
  return Promise.all(keys.map((queryKey) => queryClient.invalidateQueries({ queryKey })));
}

/** Variables: `{ tenantId, status, reason }`. The reason is required. */
export function useChangeTenantStatus() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { tenantId: string; status: SettableTenantStatus; reason: string }): Promise<void> => {
      await platformFetch(`/tenants/${pathSegment(vars.tenantId)}/status`, {
        method: "POST",
        body: { status: vars.status, reason: vars.reason },
      });
    },
    onSuccess: (_data, vars) =>
      invalidateAll(queryClient, [
        platformKeys.tenant(vars.tenantId),
        platformKeys.tenantLists(),
        platformKeys.tenantAudits(vars.tenantId),
        platformKeys.audits(),
        platformKeys.overview(),
      ]),
  });
}

/** A workspace's audit events, newest first. */
export function useTenantAudit(id: string | undefined, filters: AuditFilters = {}) {
  const f = clean(filters);
  return useInfiniteQuery({
    queryKey: platformKeys.tenantAudit(id ?? "", f),
    enabled: !!id,
    queryFn: async ({ pageParam, signal }) =>
      parseAuditPage(
        await platformFetch(
          `/tenants/${pathSegment(id ?? "")}/audit-events${queryString({ ...f, cursor: pageParam })}`,
          { signal },
        ),
        "GET /api/platform/tenants/{id}/audit-events",
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    select: (data) => data.pages.flatMap((page) => page.events),
  });
}

/** A workspace's notes, newest first. */
export function useTenantNotes(id: string | undefined) {
  return useInfiniteQuery({
    queryKey: platformKeys.tenantNotes(id ?? ""),
    enabled: !!id,
    queryFn: async ({ pageParam, signal }) =>
      parseNotePage(
        await platformFetch(`/tenants/${pathSegment(id ?? "")}/notes${queryString({ cursor: pageParam })}`, { signal }),
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    select: (data) => data.pages.flatMap((page) => page.notes),
  });
}

/** Variables: `{ tenantId, body }`. */
export function useAddTenantNote() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { tenantId: string; body: string }): Promise<void> => {
      await platformFetch(`/tenants/${pathSegment(vars.tenantId)}/notes`, {
        method: "POST",
        body: { body: vars.body },
      });
    },
    onSuccess: (_data, vars) =>
      invalidateAll(queryClient, [
        platformKeys.tenantNotes(vars.tenantId),
        platformKeys.tenant(vars.tenantId),
        platformKeys.tenantAudits(vars.tenantId),
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

/** Variables: `{ email, role }`. Rejects with code `no_such_account` or `already_staff`. */
export function useAddStaff() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { email: string; role: SettableStaffRole }): Promise<void> => {
      await platformFetch("/staff", { method: "POST", body: { email: vars.email, role: vars.role } });
    },
    onSuccess: () => invalidateAll(queryClient, [platformKeys.staff(), platformKeys.audits()]),
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
        platformKeys.tenants(),
        platformKeys.overview(),
        platformKeys.audits(),
      ]),
  });
}

// Plans

/** List all plans. Admin only. */
export function usePlans() {
  return useQuery({
    queryKey: platformKeys.plans(),
    queryFn: async ({ signal }): Promise<Plan[]> => {
      const res = await platformFetch("/plans", { signal });
      return parsePlanList(res).plans;
    },
  });
}

export interface PutPlanInput {
  key: string;
  name: string;
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
  archived: boolean;
}

/** Create or update a plan. Admin only. */
export function usePutPlan() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: PutPlanInput) => {
      const { key, ...body } = vars;
      const res = await platformFetch(`/plans/${pathSegment(key)}`, {
        method: "PUT",
        body,
      });
      return parsePutPlanResponse(res);
    },
    onSuccess: () =>
      invalidateAll(queryClient, [
        platformKeys.plans(),
        platformKeys.audits(),
      ]),
  });
}

/** Delete an unused plan. Admin only. */
export function useDeletePlan() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { key: string }) => {
      const res = await platformFetch(`/plans/${pathSegment(vars.key)}`, {
        method: "DELETE",
      });
      return parseDeletePlanResponse(res);
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
  tenantId: string;
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

      const res = await platformFetch(`/tenants/${pathSegment(vars.tenantId)}/subscription`, {
        method: "PUT",
        body,
      });
      return parsePutSubscriptionResponse(res);
    },
    onSuccess: (_data, vars) =>
      invalidateAll(queryClient, [
        platformKeys.tenant(vars.tenantId),
        platformKeys.tenants(),
        platformKeys.overview(),
        platformKeys.audits(),
      ]),
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
