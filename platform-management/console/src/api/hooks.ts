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
  parseUserDetail,
  parseUserPage,
  parseOrderList,
  parseOrderActionResult,
  parsePlanList,
  parsePutPlanResponse,
  parseDeletePlanResponse,
  parsePutSubscriptionResponse,
  type Me,
  type Overview,
  type StaffRole,
  type TenantDetail,
  type TenantStatus,
  type UserDetail,
  type Order,
  type Plan,
  type Subscription,
  type Limits,
} from "./schemas";

export type { Subscription, Order, Plan, Limits };

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

/** Variables: `{ email, code }`. A wrong code rejects with code `invalid_code`. */
export function useVerifyCode() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { email: string; code: string }): Promise<void> => {
      await platformFetch("/auth/verify-code", { method: "POST", body: { email: vars.email, code: vars.code } });
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: platformKeys.me() }),
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
