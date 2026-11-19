import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider, type QueryKey } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { PlatformApiError } from "./client";
import {
  platformKeys,
  useAddStaff,
  useAddWorkspaceNote,
  useAuditEvents,
  useChangeWorkspaceStatus,
  useChangeUserStatus,
  useLogout,
  useMe,
  useOverview,
  useRemoveStaff,
  useRevokeUserSessions,
  useResetUserMfa,
  useSendCode,
  useStaff,
  useWorkspace,
  useWorkspaceAudit,
  useWorkspaceNotes,
  useWorkspaceUsage,
  useWorkspaceBillingProfile,
  useTenants,
  useTenant,
  useWorkspaces,
  useExportWorkspace,
  useUser,
  useUsers,
  useVerifyCode,
  useOrders,
  useConfirmOrder,
  useCancelOrder,
  usePlans,
  usePlan,
  usePlanHistory,
  useCreatePlan,
  usePutPlan,
  useActivatePlan,
  useDeactivatePlan,
  useDeletePlan,
  usePriceChanges,
  useSchedulePriceChange,
  useCancelScheduledPriceChange,
  useUpdateSubscription,
  useSetBillingProfileVerified,
  useUpdateWorkspaceBillingProfile,
  accountKeys,
  AccountApiError,
  useAccountRequests,
  useCreateAccountRequest,
  useCancelAccountRequest,
  useResendWelcome,
  useEinvoices,
  useOrderEinvoices,
  useUpdateStaff,
  useMemberRequests,
  useAddWorkspaceMember,
  useCancelMemberRequest,
  useDeleteWorkspaceNow,
  useDeleteTenantNow,
  useAccountDeletePreview,
  useDeleteAccountNow,
  useDeletionRequests,
  useCancelDeletionRequest,
  useRetryDeletionRequest,
  useTenantUsers,
  useTenantUser,
  useDeactivateTenantUser,
  useRemoveWorkspaceMember,
  useChangeWorkspaceMemberRole,
  useCreateQuotaRequest,
  useTenantResources,
  useSetTenantLimitOverride,
  useSystemResources,
  useRuntimes,
  useRuntime,
  useSetRuntimeStatus,
  useUnregisterRuntime,
  useTenantPolicy,
  useUpdateTenantPolicy,
  useSetTenantStatus,
  useQuotaRequests,
  useQuotaRequest,
  useApproveQuotaRequest,
  useDenyQuotaRequest,
  useApprovals,
  useApproveApproval,
  useRejectApproval,
  useLegalHolds,
  usePlaceLegalHold,
  useReleaseLegalHold,
  useExportAudit,
} from "./hooks";

type Handler = Response | ((url: URL, init: RequestInit | undefined) => Response);

interface Call {
  method: string;
  path: string;
  search: string;
  body: unknown;
}

/** Stubs fetch with a route table keyed by "METHOD /path" and records each call. */
function stubFetch(routes: Record<string, Handler>) {
  const calls: Call[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input), "http://console.test");
      const method = init?.method ?? "GET";
      calls.push({
        method,
        path: url.pathname,
        search: url.search,
        body: typeof init?.body === "string" ? JSON.parse(init.body) : undefined,
      });
      const handler = routes[`${method} ${url.pathname}`];
      if (!handler) return new Response(`no route for ${method} ${url.pathname}`, { status: 599 });
      return typeof handler === "function" ? handler(url, init) : handler.clone();
    }),
  );
  return calls;
}

const json = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
const noContent = () => new Response(null, { status: 204 });

function setup() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
  return { queryClient, wrapper };
}

const workspace = { id: "t1", name: "Acme", slug: "acme", status: "active", member_count: 2, created_at: "", last_activity_at: "" };
const tenant = { id: "t1", name: "Acme", status: "active", workspace_count: 2, max_workspaces: 3, best_plan_key: "pro", status_summary: { active: 2 }, created_at: "", last_activity_at: "" };
const user = { id: "u1", email: "a@example.com", name: "A", status: "active", memberships_count: 1, created_at: "" };
const event = (id: string) => ({ id, workspace_id: "t1", actor_type: "staff", action: "note.added", created_at: "" });

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("useMe", () => {
  it("returns the staff member under the me key", async () => {
    stubFetch({ "GET /api/platform/me": json(200, { user_id: "u1", email: "a@example.com", role: "support" }) });
    const { queryClient, wrapper } = setup();
    const { result } = renderHook(() => useMe(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual({ user_id: "u1", email: "a@example.com", role: "support", tenant_id: null });
    expect(queryClient.getQueryData(platformKeys.me())).toEqual(result.current.data);
  });

  it("returns null when the server answers 404", async () => {
    stubFetch({ "GET /api/platform/me": new Response("404 page not found", { status: 404 }) });
    const { wrapper } = setup();
    const { result } = renderHook(() => useMe(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toBeNull();
  });

  it("is an error for any other failure", async () => {
    stubFetch({ "GET /api/platform/me": json(500, { error: "internal error", code: "internal" }) });
    const { wrapper } = setup();
    const { result } = renderHook(() => useMe(), { wrapper });
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect((result.current.error as PlatformApiError).status).toBe(500);
  });

  it("is an error, not a signed-out state, for a body it cannot read", async () => {
    stubFetch({ "GET /api/platform/me": json(200, { nope: true }) });
    const { wrapper } = setup();
    const { result } = renderHook(() => useMe(), { wrapper });
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect((result.current.error as PlatformApiError).code).toBe("malformed_response");
  });
});

describe("query keys", () => {
  it("drop empty filter values", () => {
    expect(platformKeys.tenantList({ q: "" })).toEqual(platformKeys.tenantList({}));
    expect(platformKeys.tenantList({ q: "  acme " })).toEqual(platformKeys.tenantList({ q: "acme" }));
    expect(platformKeys.audit({ action: "", from: "2026-01-01T00:00:00Z" })).toEqual(
      platformKeys.audit({ from: "2026-01-01T00:00:00Z" }),
    );
  });

  it("include the filters", () => {
    expect(platformKeys.tenantList({ q: "a" })).not.toEqual(platformKeys.tenantList({ q: "b" }));
    expect(platformKeys.audit({ action: "note.added" })).not.toEqual(platformKeys.audit({ action: "staff.added" }));
    expect(platformKeys.workspaceAudit("t1", { actor_id: "u1" })).not.toEqual(platformKeys.workspaceAudit("t1", {}));
  });


  it("keep one workspace's audit under its own prefix", () => {
    const key = platformKeys.workspaceAudit("t1", { action: "note.added" });
    expect(key.slice(0, platformKeys.workspaceAudits("t1").length)).toEqual(platformKeys.workspaceAudits("t1"));
    expect(key.slice(0, platformKeys.workspaceAudits("t2").length)).not.toEqual(platformKeys.workspaceAudits("t2"));
  });

  it("keeps workspace usage keys under platform/workspaces", () => {
    const key = platformKeys.workspaceUsage("t1");
    expect(key.slice(0, platformKeys.workspaces().length)).toEqual(platformKeys.workspaces());
    expect(key).not.toEqual(platformKeys.workspaceUsage("t2"));
  });

  it("keeps workspace billing profile keys under platform/workspaces", () => {
    const key = platformKeys.workspaceBillingProfile("t1");
    expect(key.slice(0, platformKeys.workspaces().length)).toEqual(platformKeys.workspaces());
    expect(key).not.toEqual(platformKeys.workspaceBillingProfile("t2"));
  });
});

describe("reads", () => {
  it("useTenants requests with the search filter and caches under the filtered key", async () => {
    const calls = stubFetch({ "GET /api/platform/tenants": json(200, { tenants: [tenant], next_cursor: null }) });
    const { queryClient, wrapper } = setup();
    const { result } = renderHook(() => useTenants({ q: "acme" }), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([tenant]);
    expect(calls[0]?.search).toBe("?q=acme");
    expect(queryClient.getQueryState(platformKeys.tenantList({ q: "acme" }))?.status).toBe("success");
    expect(queryClient.getQueryState(platformKeys.tenantList({}))).toBeUndefined();
  });

  it("useTenants pages with the cursor and flattens the pages", async () => {
    const calls = stubFetch({
      "GET /api/platform/tenants": (url) =>
        url.searchParams.get("cursor") === "c1"
          ? json(200, { tenants: [{ ...tenant, id: "t2" }], next_cursor: null })
          : json(200, { tenants: [tenant], next_cursor: "c1" }),
    });
    const { wrapper } = setup();
    const { result } = renderHook(() => useTenants(), { wrapper });
    await waitFor(() => expect(result.current.hasNextPage).toBe(true));
    await act(async () => {
      await result.current.fetchNextPage();
    });
    await waitFor(() => expect(result.current.data?.map((t) => t.id)).toEqual(["t1", "t2"]));
    expect(result.current.hasNextPage).toBe(false);
    expect(calls.map((c) => c.search)).toEqual(["", "?cursor=c1"]);
  });

  it("useTenants shows a malformed list as empty", async () => {
    stubFetch({ "GET /api/platform/tenants": json(200, { tenants: "nope" }) });
    const { wrapper } = setup();
    const { result } = renderHook(() => useTenants(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([]);
  });

  it("useTenant reads /tenants/{id} under the tenant detail key", async () => {
    const calls = stubFetch({ "GET /api/platform/tenants/t1": json(200, { ...tenant, workspaces: [] }) });
    const { queryClient, wrapper } = setup();
    const { result } = renderHook(() => useTenant("t1"), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(calls[0]?.path).toBe("/api/platform/tenants/t1");
    expect(queryClient.getQueryState(platformKeys.tenant("t1"))?.status).toBe("success");
  });

  it("useWorkspace uses the detail key and is an error for a malformed body", async () => {
    stubFetch({ "GET /api/platform/workspaces/t1": json(200, workspace), "GET /api/platform/workspaces/t2": json(200, [1]) });
    const { queryClient, wrapper } = setup();
    const ok = renderHook(() => useWorkspace("t1"), { wrapper });
    const bad = renderHook(() => useWorkspace("t2"), { wrapper });
    await waitFor(() => expect(ok.result.current.isSuccess).toBe(true));
    await waitFor(() => expect(bad.result.current.isError).toBe(true));
    expect(queryClient.getQueryState(platformKeys.workspace("t1"))?.status).toBe("success");
    expect((bad.result.current.error as PlatformApiError).code).toBe("malformed_response");
  });

  it("useWorkspace does not fetch without an id", () => {
    const calls = stubFetch({});
    const { wrapper } = setup();
    renderHook(() => useWorkspace(undefined), { wrapper });
    expect(calls).toHaveLength(0);
  });

  it("useWorkspaceUsage fetches under the workspace usage key and errors on a malformed body", async () => {
    const validUsage = {
      workspace_id: "t1",
      period_start: "2026-11-01T00:00:00Z",
      period_end: "2026-12-01T00:00:00Z",
      rows: [
        {
          day: "2026-11-01",
          meter: "tasks_run",
          quantity: 12,
          unit_price: 30000,
          per: 1000,
          discount_pct: 0,
          net_amount: 360,
          note: "",
          order_id: null,
        },
      ],
      missing_days: ["2026-11-02"],
      unpaid_orders: [],
    };
    const calls = stubFetch({
      "GET /api/platform/workspaces/t1/usage": json(200, validUsage),
      "GET /api/platform/workspaces/t2/usage": json(200, { workspace_id: "t2" }),
      "GET /api/platform/workspaces/a%2Fb/usage": json(200, { ...validUsage, workspace_id: "a/b" }),
    });
    const { queryClient, wrapper } = setup();
    const ok = renderHook(() => useWorkspaceUsage("t1"), { wrapper });
    const bad = renderHook(() => useWorkspaceUsage("t2"), { wrapper });
    const encoded = renderHook(() => useWorkspaceUsage("a/b"), { wrapper });
    renderHook(() => useWorkspaceUsage(undefined), { wrapper });

    await waitFor(() => expect(ok.result.current.isSuccess).toBe(true));
    await waitFor(() => expect(bad.result.current.isError).toBe(true));
    await waitFor(() => expect(encoded.result.current.isSuccess).toBe(true));

    expect(ok.result.current.data).toEqual(validUsage);
    expect(queryClient.getQueryState(platformKeys.workspaceUsage("t1"))?.status).toBe("success");
    expect((bad.result.current.error as PlatformApiError).code).toBe("malformed_response");
    expect(calls.map((c) => c.path)).toContain("/api/platform/workspaces/a%2Fb/usage");
    expect(calls).toHaveLength(3);
  });

  it("useWorkspaceBillingProfile fetches under the billing profile key and errors on a malformed body", async () => {
    const validProfile = {
      workspace_id: "t1",
      account_kind: "business",
      legal_name: "Acme Co",
      tax_code: "0123456789",
      address: "1 Main St",
      invoice_email: "ap@acme.test",
      verified: true,
      verified_at: "2026-11-17T00:00:00Z",
    };
    const calls = stubFetch({
      "GET /api/platform/workspaces/t1/billing-profile": json(200, validProfile),
      "GET /api/platform/workspaces/t2/billing-profile": json(200, { workspace_id: "t2" }),
      "GET /api/platform/workspaces/a%2Fb/billing-profile": json(200, {
        ...validProfile,
        workspace_id: "a/b",
        verified: false,
        verified_at: null,
      }),
    });
    const { queryClient, wrapper } = setup();
    const ok = renderHook(() => useWorkspaceBillingProfile("t1"), { wrapper });
    const bad = renderHook(() => useWorkspaceBillingProfile("t2"), { wrapper });
    const encoded = renderHook(() => useWorkspaceBillingProfile("a/b"), { wrapper });
    renderHook(() => useWorkspaceBillingProfile(undefined), { wrapper });

    await waitFor(() => expect(ok.result.current.isSuccess).toBe(true));
    await waitFor(() => expect(bad.result.current.isError).toBe(true));
    await waitFor(() => expect(encoded.result.current.isSuccess).toBe(true));

    expect(ok.result.current.data).toEqual(validProfile);
    expect(queryClient.getQueryState(platformKeys.workspaceBillingProfile("t1"))?.status).toBe("success");
    expect((bad.result.current.error as PlatformApiError).code).toBe("malformed_response");
    expect(calls.map((c) => `${c.method} ${c.path}`)).toContain("GET /api/platform/workspaces/a%2Fb/billing-profile");
    expect(calls).toHaveLength(3);
  });

  it("useWorkspaceAudit sends the filters and keys by workspace and filters", async () => {
    const calls = stubFetch({
      "GET /api/platform/workspaces/t1/audit-events": json(200, { events: [event("e1")], next_cursor: null }),
    });
    const { queryClient, wrapper } = setup();
    const filters = { action: "note.added", from: "2026-01-01T00:00:00Z" };
    const { result } = renderHook(() => useWorkspaceAudit("t1", filters), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.map((e) => e.id)).toEqual(["e1"]);
    expect(new URLSearchParams(calls[0]?.search).get("action")).toBe("note.added");
    expect(new URLSearchParams(calls[0]?.search).get("from")).toBe("2026-01-01T00:00:00Z");
    expect(queryClient.getQueryState(platformKeys.workspaceAudit("t1", filters))?.status).toBe("success");
  });

  it("useWorkspaceNotes uses the notes key", async () => {
    stubFetch({
      "GET /api/platform/workspaces/t1/notes": json(200, { notes: [{ id: "n1", body: "hi" }], next_cursor: null }),
    });
    const { queryClient, wrapper } = setup();
    const { result } = renderHook(() => useWorkspaceNotes("t1"), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.[0]?.body).toBe("hi");
    expect(queryClient.getQueryState(platformKeys.workspaceNotes("t1"))?.status).toBe("success");
  });

  it("useUsers and useUser use the user keys", async () => {
    const calls = stubFetch({
      "GET /api/platform/users": json(200, { users: [user], next_cursor: null }),
      "GET /api/platform/users/u1": json(200, { ...user, memberships: [], account: { status: "active" } }),
    });
    const { queryClient, wrapper } = setup();
    const list = renderHook(() => useUsers({ q: "a@" }), { wrapper });
    const one = renderHook(() => useUser("u1"), { wrapper });
    await waitFor(() => expect(list.result.current.isSuccess).toBe(true));
    await waitFor(() => expect(one.result.current.isSuccess).toBe(true));
    expect(list.result.current.data).toEqual([user]);
    expect(calls.find((c) => c.path === "/api/platform/users")?.search).toBe("?q=a%40");
    expect(queryClient.getQueryState(platformKeys.userList({ q: "a@" }))?.status).toBe("success");
    expect(queryClient.getQueryState(platformKeys.user("u1"))?.status).toBe("success");
  });

  it("useStaff returns the staff array under the staff key", async () => {
    stubFetch({
      "GET /api/platform/staff": json(200, { staff: [{ user_id: "u1", email: "a@example.com", role: "admin" }] }),
    });
    const { queryClient, wrapper } = setup();
    const { result } = renderHook(() => useStaff(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.[0]?.role).toBe("admin");
    expect(queryClient.getQueryState(platformKeys.staff())?.status).toBe("success");
  });

  it("useAuditEvents sends the filters and keys by them", async () => {
    const calls = stubFetch({
      "GET /api/platform/audit-events": json(200, { events: [event("e1")], next_cursor: "c" }),
    });
    const { queryClient, wrapper } = setup();
    const filters = { actor_id: "u1", to: "2026-03-01T00:00:00Z" };
    const { result } = renderHook(() => useAuditEvents(filters), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.hasNextPage).toBe(true);
    const params = new URLSearchParams(calls[0]?.search);
    expect(params.get("actor_id")).toBe("u1");
    expect(params.get("to")).toBe("2026-03-01T00:00:00Z");
    expect(params.has("action")).toBe(false);
    expect(queryClient.getQueryState(platformKeys.audit(filters))?.status).toBe("success");
  });

  it("useOverview uses the overview key and is an error for a malformed body", async () => {
    stubFetch({ "GET /api/platform/overview": json(200, { workspaces_by_status: { active: 3 }, signups_7d: 1 }) });
    const first = setup();
    const ok = renderHook(() => useOverview(), { wrapper: first.wrapper });
    await waitFor(() => expect(ok.result.current.isSuccess).toBe(true));
    expect(ok.result.current.data?.workspaces_by_status.active).toBe(3);
    expect(first.queryClient.getQueryState(platformKeys.overview())?.status).toBe("success");

    stubFetch({ "GET /api/platform/overview": json(200, "x") });
    const second = setup();
    const bad = renderHook(() => useOverview(), { wrapper: second.wrapper });
    await waitFor(() => expect(bad.result.current.isError).toBe(true));
  });
});

describe("session mutations", () => {
  it("useSendCode posts the email and invalidates nothing", async () => {
    const calls = stubFetch({ "POST /api/platform/auth/send-code": noContent() });
    const { queryClient, wrapper } = setup();
    queryClient.setQueryData(platformKeys.me(), null);
    const { result } = renderHook(() => useSendCode(), { wrapper });
    await act(async () => {
      await result.current.mutateAsync({ email: "a@example.com" });
    });
    expect(calls[0]).toMatchObject({ method: "POST", path: "/api/platform/auth/send-code", body: { email: "a@example.com" } });
    expect(queryClient.getQueryState(platformKeys.me())?.isInvalidated).toBe(false);
  });

  it("useVerifyCode posts email and code, then refetches who is signed in", async () => {
    let signedIn = false;
    const calls = stubFetch({
      "POST /api/platform/auth/verify-code": () => {
        signedIn = true;
        return json(200, { user_id: "u1", role: "support" });
      },
      "GET /api/platform/me": () =>
        signedIn ? json(200, { user_id: "u1", email: "a@example.com", role: "support" }) : new Response("", { status: 404 }),
    });
    const { wrapper } = setup();
    const me = renderHook(() => useMe(), { wrapper });
    const verify = renderHook(() => useVerifyCode(), { wrapper });
    await waitFor(() => expect(me.result.current.data).toBeNull());
    await act(async () => {
      await verify.result.current.mutateAsync({ email: "a@example.com", code: "123456" });
    });
    await waitFor(() => expect(me.result.current.data?.user_id).toBe("u1"));
    expect(calls.find((c) => c.path.endsWith("/verify-code"))?.body).toEqual({ email: "a@example.com", code: "123456" });
  });

  it("useVerifyCode sends the authenticator code when it has one", async () => {
    const calls = stubFetch({ "POST /api/platform/auth/verify-code": json(200, { user_id: "u1", role: "support" }) });
    const { wrapper } = setup();
    const { result } = renderHook(() => useVerifyCode(), { wrapper });
    await act(async () => {
      await result.current.mutateAsync({ email: "a@example.com", code: "123456", totp: "654321" });
    });
    expect(calls[0]?.body).toEqual({ email: "a@example.com", code: "123456", totp: "654321" });
  });

  it("useVerifyCode reports that the authenticator code is needed, and does not refetch who is signed in", async () => {
    stubFetch({ "POST /api/platform/auth/verify-code": json(200, { mfa_required: true }) });
    const { queryClient, wrapper } = setup();
    queryClient.setQueryData(platformKeys.me(), null);
    const { result } = renderHook(() => useVerifyCode(), { wrapper });
    let answer: { mfaRequired: boolean } | undefined;
    await act(async () => {
      answer = await result.current.mutateAsync({ email: "a@example.com", code: "123456" });
    });
    expect(answer).toEqual({ mfaRequired: true });
    expect(queryClient.getQueryState(platformKeys.me())?.isInvalidated).toBe(false);
  });

  it("useVerifyCode rejects a malformed answer", async () => {
    stubFetch({ "POST /api/platform/auth/verify-code": json(200, { mfa_required: "yes" }) });
    const { wrapper } = setup();
    const { result } = renderHook(() => useVerifyCode(), { wrapper });
    await act(async () => {
      await expect(result.current.mutateAsync({ email: "a@example.com", code: "123456" })).rejects.toBeInstanceOf(Error);
    });
  });

  it("useVerifyCode rejects with the server's code for a wrong code", async () => {
    stubFetch({ "POST /api/platform/auth/verify-code": json(400, { error: "invalid code", code: "invalid_code" }) });
    const { wrapper } = setup();
    const { result } = renderHook(() => useVerifyCode(), { wrapper });
    await act(async () => {
      await expect(result.current.mutateAsync({ email: "a@example.com", code: "000000" })).rejects.toMatchObject({
        status: 400,
        code: "invalid_code",
      });
    });
  });

  it("useLogout posts, marks nobody signed in, and drops every other cached query", async () => {
    const calls = stubFetch({ "POST /api/platform/auth/logout": noContent() });
    const { queryClient, wrapper } = setup();
    queryClient.setQueryData(platformKeys.me(), { user_id: "u1", email: "a@example.com", role: "admin" });
    queryClient.setQueryData(platformKeys.workspace("t1"), workspace);
    queryClient.setQueryData(platformKeys.staff(), []);
    queryClient.setQueryData(["unrelated"], 1);
    const { result } = renderHook(() => useLogout(), { wrapper });
    await act(async () => {
      await result.current.mutateAsync();
    });
    expect(calls[0]).toMatchObject({ method: "POST", path: "/api/platform/auth/logout" });
    expect(queryClient.getQueryData(platformKeys.me())).toBeNull();
    expect(queryClient.getQueryData(platformKeys.workspace("t1"))).toBeUndefined();
    expect(queryClient.getQueryData(platformKeys.staff())).toBeUndefined();
    expect(queryClient.getQueryData(["unrelated"])).toBe(1);
  });
});

/**
 * Every query a mutation could touch is seeded; the ones it must invalidate are
 * listed in `invalidates` and the rest must stay fresh.
 */
const seeded: Record<string, QueryKey> = {
  me: platformKeys.me(),
  tenantList: platformKeys.tenantList({ q: "acme" }),
  tenantListAll: platformKeys.tenantList({}),
  workspace: platformKeys.workspace("t1"),
  otherWorkspace: platformKeys.workspace("t2"),
  workspaceAudit: platformKeys.workspaceAudit("t1", {}),
  workspaceAuditFiltered: platformKeys.workspaceAudit("t1", { action: "note.added" }),
  otherWorkspaceAudit: platformKeys.workspaceAudit("t2", {}),
  workspaceNotes: platformKeys.workspaceNotes("t1"),
  otherWorkspaceNotes: platformKeys.workspaceNotes("t2"),
  userList: platformKeys.userList({}),
  user: platformKeys.user("u1"),
  otherUser: platformKeys.user("u2"),
  staff: platformKeys.staff(),
  audit: platformKeys.audit({}),
  auditFiltered: platformKeys.audit({ action: "staff.added" }),
  overview: platformKeys.overview(),
};

interface MutationCase {
  name: string;
  route: string;
  response: Response;
  run: (hooks: ReturnType<typeof useAllMutations>) => Promise<unknown>;
  request: { method: string; path: string; body?: unknown };
  invalidates: string[];
}

function useAllMutations() {
  return {
    changeWorkspaceStatus: useChangeWorkspaceStatus(),
    addWorkspaceNote: useAddWorkspaceNote(),
    changeUserStatus: useChangeUserStatus(),
    revokeUserSessions: useRevokeUserSessions(),
    resetUserMfa: useResetUserMfa(),
    addStaff: useAddStaff(),
    removeStaff: useRemoveStaff(),
  };
}

const mutationCases: MutationCase[] = [
  {
    name: "useChangeWorkspaceStatus",
    route: "POST /api/platform/workspaces/t1/status",
    response: json(200, { changed: true, state: {} }),
    run: (h) => h.changeWorkspaceStatus.mutateAsync({ workspaceId: "t1", status: "suspended", reason: "abuse" }),
    request: { method: "POST", path: "/api/platform/workspaces/t1/status", body: { status: "suspended", reason: "abuse" } },
    invalidates: ["workspace", "tenantList", "tenantListAll", "workspaceAudit", "workspaceAuditFiltered", "audit", "auditFiltered", "overview"],
  },
  {
    name: "useAddWorkspaceNote",
    route: "POST /api/platform/workspaces/t1/notes",
    response: json(201, { id: "n1" }),
    run: (h) => h.addWorkspaceNote.mutateAsync({ workspaceId: "t1", body: "called" }),
    request: { method: "POST", path: "/api/platform/workspaces/t1/notes", body: { body: "called" } },
    invalidates: ["workspaceNotes", "workspace", "workspaceAudit", "workspaceAuditFiltered", "audit", "auditFiltered"],
  },
  {
    name: "useChangeUserStatus",
    route: "POST /api/platform/users/u1/status",
    response: json(200, { changed: true, status: "suspended" }),
    run: (h) => h.changeUserStatus.mutateAsync({ userId: "u1", status: "suspended", reason: "fraud" }),
    request: { method: "POST", path: "/api/platform/users/u1/status", body: { status: "suspended", reason: "fraud" } },
    invalidates: ["user", "userList", "audit", "auditFiltered"],
  },
  {
    name: "useRevokeUserSessions",
    route: "POST /api/platform/users/u1/revoke-sessions",
    response: json(200, { sessions_valid_after: "2026-01-01T00:00:00Z" }),
    run: (h) => h.revokeUserSessions.mutateAsync({ userId: "u1", reason: "lost laptop" }),
    request: { method: "POST", path: "/api/platform/users/u1/revoke-sessions", body: { reason: "lost laptop" } },
    invalidates: ["user", "audit", "auditFiltered"],
  },
  {
    name: "useResetUserMfa",
    route: "POST /api/platform/users/u1/mfa-reset",
    response: json(200, { reset: true }),
    run: (h) => h.resetUserMfa.mutateAsync({ userId: "u1" }),
    request: { method: "POST", path: "/api/platform/users/u1/mfa-reset" },
    invalidates: ["user", "audit", "auditFiltered"],
  },
  {
    name: "useAddStaff",
    route: "POST /api/platform/staff",
    response: json(201, { user_id: "u3", email: "b@example.com", role: "billing" }),
    run: (h) => h.addStaff.mutateAsync({ email: "b@example.com", role: "billing" }),
    request: { method: "POST", path: "/api/platform/staff", body: { email: "b@example.com", role: "billing" } },
    invalidates: ["staff", "audit", "auditFiltered"],
  },
  {
    name: "useRemoveStaff",
    route: "DELETE /api/platform/staff/u3",
    response: json(200, { user_id: "u3", role: "billing" }),
    run: (h) => h.removeStaff.mutateAsync({ userId: "u3" }),
    request: { method: "DELETE", path: "/api/platform/staff/u3" },
    invalidates: ["staff", "audit", "auditFiltered", "me"],
  },
];

describe("mutations", () => {
  it.each(mutationCases)("$name sends its request and invalidates exactly the queries it affects", async (c) => {
    const calls = stubFetch({ [c.route]: c.response });
    const { queryClient, wrapper } = setup();
    for (const key of Object.values(seeded)) queryClient.setQueryData(key, null);
    const { result } = renderHook(() => useAllMutations(), { wrapper });

    await act(async () => {
      await c.run(result.current);
    });

    expect(calls).toHaveLength(1);
    expect(calls[0]).toMatchObject(c.request);
    for (const [name, key] of Object.entries(seeded)) {
      const invalidated = queryClient.getQueryState(key)?.isInvalidated;
      expect([name, invalidated]).toEqual([name, c.invalidates.includes(name)]);
    }
  });

  it("does not invalidate anything when the server refuses", async () => {
    stubFetch({
      "DELETE /api/platform/staff/u1": json(400, { error: "the last active administrator cannot be removed", code: "last_admin" }),
    });
    const { queryClient, wrapper } = setup();
    for (const key of Object.values(seeded)) queryClient.setQueryData(key, null);
    const { result } = renderHook(() => useAllMutations(), { wrapper });

    await act(async () => {
      await expect(result.current.removeStaff.mutateAsync({ userId: "u1" })).rejects.toMatchObject({
        status: 400,
        code: "last_admin",
        message: "the last active administrator cannot be removed",
      });
    });
    for (const key of Object.values(seeded)) expect(queryClient.getQueryState(key)?.isInvalidated).toBe(false);
  });

  it("encodes an id that is not path-safe", async () => {
    const calls = stubFetch({ "POST /api/platform/workspaces/a%2Fb/status": json(200, {}) });
    const { wrapper } = setup();
    const { result } = renderHook(() => useChangeWorkspaceStatus(), { wrapper });
    await act(async () => {
      await result.current.mutateAsync({ workspaceId: "a/b", status: "active", reason: "ok" });
    });
    expect(calls[0]?.path).toBe("/api/platform/workspaces/a%2Fb/status");
  });
});

describe("orders and billing hooks", () => {
  const mockOrder = {
    id: "ord-1",
    workspace_id: "ws-1",
    workspace_name: "Acme",
    kind: "subscribe",
    plan_key: "pro",
    interval: "month",
    seats: 3,
    amount: 600000,
    vat_amount: 60000,
    currency: "vnd",
    status: "pending",
    transfer_code: "MUL12345678",
    expires_at: "2026-11-20T10:00:00Z",
    created_at: "2026-11-13T10:00:00Z",
    paid_at: null,
    buyer: {
      name: "Acme",
      tax_code: "010",
      address: "HN",
      invoice_email: "acme@example.com",
    },
  };

  it("useOrders fetches default pending orders oldest first", async () => {
    const calls = stubFetch({ "GET /api/platform/orders": json(200, { orders: [mockOrder] }) });
    const { wrapper } = setup();
    const { result } = renderHook(() => useOrders(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(calls[0]?.search).toBe("?status=pending");
    expect(result.current.data).toEqual([{ ...mockOrder, invoice_state: null }]);
  });

  it("useOrders fetches with specified status", async () => {
    const calls = stubFetch({ "GET /api/platform/orders": json(200, { orders: [{ ...mockOrder, status: "paid" }] }) });
    const { wrapper } = setup();
    const { result } = renderHook(() => useOrders("paid"), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(calls[0]?.search).toBe("?status=paid");
    expect(result.current.data?.[0]?.status).toBe("paid");
  });

  it("useConfirmOrder sends reason and optional transfer_reference", async () => {
    const calls = stubFetch({
      "POST /api/platform/orders/ord-1/confirm": json(200, { order: { ...mockOrder, status: "paid" }, applied: true }),
    });
    const { wrapper } = setup();
    const { result } = renderHook(() => useConfirmOrder(), { wrapper });

    await act(async () => {
      await result.current.mutateAsync({ orderId: "ord-1", reason: "wire received", transferReference: "REF123" });
    });

    expect(calls[0]?.path).toBe("/api/platform/orders/ord-1/confirm");
    expect(calls[0]?.body).toEqual({ reason: "wire received", transfer_reference: "REF123" });
  });

  it("useCancelOrder sends reason and invalidates workspace usage", async () => {
    const calls = stubFetch({
      "POST /api/platform/orders/ord-1/cancel": json(200, { order: { ...mockOrder, status: "canceled" }, applied: true }),
    });
    const { queryClient, wrapper } = setup();
    queryClient.setQueryData(platformKeys.workspaceUsage("ws-1"), null);
    const { result } = renderHook(() => useCancelOrder(), { wrapper });

    await act(async () => {
      await result.current.mutateAsync({ orderId: "ord-1", reason: "user request" });
    });

    expect(calls[0]?.path).toBe("/api/platform/orders/ord-1/cancel");
    expect(calls[0]?.body).toEqual({ reason: "user request" });
    expect(queryClient.getQueryState(platformKeys.workspaceUsage("ws-1"))?.isInvalidated).toBe(true);
  });
});

describe("plans hooks", () => {
  const mockPlan = {
    key: "pro",
    name: "Pro",
    description: "",
    limits: {},
    features: {},
    currency: "vnd",
    seat_price_month: 200000,
    seat_price_year: null,
    seats_fixed: null,
    min_seats: 1,
    trial_days: 14,
    is_public: true,
    sort: 10,
    archived_at: null,
    audience: "any",
    billing_model: "seat",
  };

  it("usePlans fetches plan list", async () => {
    const calls = stubFetch({ "GET /api/platform/plans": json(200, { plans: [mockPlan] }) });
    const { wrapper } = setup();
    const { result } = renderHook(() => usePlans(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(calls[0]?.path).toBe("/api/platform/plans");
    expect(result.current.data).toEqual([mockPlan]);
  });

  it("usePutPlan sends plan updates without seat price fields", async () => {
    const calls = stubFetch({
      "PUT /api/platform/plans/pro": json(200, { changed: true, plan: mockPlan }),
    });
    const { wrapper } = setup();
    const { result } = renderHook(() => usePutPlan(), { wrapper });

    await act(async () => {
      await result.current.mutateAsync({
        key: "pro",
        name: "Pro Plus",
        trial_days: 30,
        limits: {},
        features: {},
        currency: "vnd",
        seats_fixed: null,
        min_seats: 1,
        is_public: true,
        sort: 10,
        archived: false,
      });
    });

    expect(calls[0]?.path).toBe("/api/platform/plans/pro");
    expect(calls[0]?.body).toMatchObject({ name: "Pro Plus", trial_days: 30 });
    expect(calls[0]?.body).not.toHaveProperty("seat_price_month");
    expect(calls[0]?.body).not.toHaveProperty("seat_price_year");
    expect(calls[0]?.body).not.toHaveProperty("usage_rates");
    expect(calls[0]?.body).not.toHaveProperty("usage_terms");
  });

  it("useDeletePlan deletes a plan", async () => {
    const calls = stubFetch({
      "DELETE /api/platform/plans/legacy": json(200, { deleted: true }),
    });
    const { wrapper } = setup();
    const { result } = renderHook(() => useDeletePlan(), { wrapper });

    await act(async () => {
      await result.current.mutateAsync({ key: "legacy" });
    });

    expect(calls[0]?.path).toBe("/api/platform/plans/legacy");
  });
});

describe("price change hooks", () => {
  const mockChange = {
    id: "pc-1",
    plan_key: "pro",
    state: "scheduled",
    effective_from: "2026-12-20T00:00:00Z",
    old: { seat_price_month: 149000, seat_price_year: 1490000 },
    new: { seat_price_month: 199000, seat_price_year: 1990000 },
    reason: "Annual update",
    notice_sent_at: null,
    created_by: "u-admin",
    created_at: "2026-11-17T00:00:00Z",
    applied_at: null,
    canceled_by: "",
    canceled_at: null,
  };

  it("usePriceChanges fetches plan history and skips when key is undefined", async () => {
    const calls = stubFetch({
      "GET /api/platform/plans/pro/price-changes": json(200, { price_changes: [mockChange] }),
    });
    const { queryClient, wrapper } = setup();
    renderHook(() => usePriceChanges(undefined), { wrapper });
    expect(calls).toHaveLength(0);

    const { result } = renderHook(() => usePriceChanges("pro"), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(calls).toHaveLength(1);
    expect(calls[0]?.path).toBe("/api/platform/plans/pro/price-changes");
    expect(result.current.data).toEqual([mockChange]);
    expect(queryClient.getQueryState(platformKeys.priceChanges("pro"))?.status).toBe("success");
  });

  it("useSchedulePriceChange sends dry_run=1 without invalidating plans or audits", async () => {
    const calls = stubFetch({
      "POST /api/platform/plans/pro/price-changes": json(200, {
        dry_run: true,
        affected_subscribers: 4,
        notice_date: "2026-11-17",
      }),
    });
    const { queryClient, wrapper } = setup();
    queryClient.setQueryData(platformKeys.plans(), []);
    queryClient.setQueryData(platformKeys.priceChanges("pro"), []);
    queryClient.setQueryData(platformKeys.audit({}), { pages: [], pageParams: [] });

    const { result } = renderHook(() => useSchedulePriceChange(), { wrapper });
    let answer: unknown;
    await act(async () => {
      answer = await result.current.mutateAsync({
        key: "pro",
        effective_from: "2026-12-20T00:00:00Z",
        new: { seat_price_month: 199000 },
        reason: "Annual update",
        confirm_large_change: false,
        dryRun: true,
      });
    });

    expect(calls[0]?.path).toBe("/api/platform/plans/pro/price-changes");
    expect(calls[0]?.search).toBe("?dry_run=1");
    expect(calls[0]?.body).toEqual({
      effective_from: "2026-12-20T00:00:00Z",
      new: { seat_price_month: 199000 },
      reason: "Annual update",
      confirm_large_change: false,
    });
    expect(answer).toEqual({ dry_run: true, affected_subscribers: 4, notice_date: "2026-11-17" });
    expect(queryClient.getQueryState(platformKeys.plans())?.isInvalidated).toBe(false);
    expect(queryClient.getQueryState(platformKeys.priceChanges("pro"))?.isInvalidated).toBe(false);
    expect(queryClient.getQueryState(platformKeys.audit({}))?.isInvalidated).toBe(false);
  });

  it("useSchedulePriceChange submits without dry_run and invalidates plans and audits", async () => {
    const calls = stubFetch({
      "POST /api/platform/plans/pro/price-changes": json(201, {
        price_change: mockChange,
        applied: false,
        affected_subscribers: 4,
        notice_date: "2026-11-17",
      }),
    });
    const { queryClient, wrapper } = setup();
    queryClient.setQueryData(platformKeys.plans(), []);
    queryClient.setQueryData(platformKeys.priceChanges("pro"), []);
    queryClient.setQueryData(platformKeys.audit({}), { pages: [], pageParams: [] });

    const { result } = renderHook(() => useSchedulePriceChange(), { wrapper });
    await act(async () => {
      await result.current.mutateAsync({
        key: "pro",
        effective_from: "2026-12-20T00:00:00Z",
        new: { seat_price_month: 199000 },
        reason: "Annual update",
        confirm_large_change: true,
      });
    });

    expect(calls[0]?.search).toBe("");
    expect(calls[0]?.body).toEqual({
      effective_from: "2026-12-20T00:00:00Z",
      new: { seat_price_month: 199000 },
      reason: "Annual update",
      confirm_large_change: true,
    });
    expect(queryClient.getQueryState(platformKeys.plans())?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(platformKeys.priceChanges("pro"))?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(platformKeys.audit({}))?.isInvalidated).toBe(true);
  });

  it("useCancelScheduledPriceChange sends DELETE and invalidates plans and audits", async () => {
    const calls = stubFetch({
      "DELETE /api/platform/plans/pro/price-changes/scheduled": json(200, {
        price_change: { ...mockChange, state: "canceled" },
      }),
    });
    const { queryClient, wrapper } = setup();
    queryClient.setQueryData(platformKeys.plans(), []);
    queryClient.setQueryData(platformKeys.priceChanges("pro"), []);
    queryClient.setQueryData(platformKeys.audit({}), { pages: [], pageParams: [] });

    const { result } = renderHook(() => useCancelScheduledPriceChange(), { wrapper });
    await act(async () => {
      await result.current.mutateAsync({ key: "pro" });
    });

    expect(calls[0]).toMatchObject({
      method: "DELETE",
      path: "/api/platform/plans/pro/price-changes/scheduled",
    });
    expect(queryClient.getQueryState(platformKeys.plans())?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(platformKeys.priceChanges("pro"))?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(platformKeys.audit({}))?.isInvalidated).toBe(true);
  });
});

describe("subscription hook", () => {
  it("useUpdateSubscription sends PUT to /workspaces/{id}/subscription", async () => {
    const calls = stubFetch({
      "PUT /api/platform/workspaces/ws-1/subscription": json(200, {
        changed: true,
        subscription: { workspace_id: "ws-1", plan_key: "business", seats: 10, interval: "month", version: 2 },
      }),
    });
    const { wrapper } = setup();
    const { result } = renderHook(() => useUpdateSubscription(), { wrapper });

    await act(async () => {
      await result.current.mutateAsync({
        workspaceId: "ws-1",
        planKey: "business",
        seats: 10,
        interval: "month",
        reason: "upgrade request",
      });
    });

    expect(calls[0]?.path).toBe("/api/platform/workspaces/ws-1/subscription");
    expect(calls[0]?.body).toEqual({
      plan_key: "business",
      seats: 10,
      interval: "month",
      reason: "upgrade request",
    });
  });
});

describe("account request hooks", () => {
  const mockReq = {
    id: "req-1",
    kind: "individual",
    email: "ana@example.com",
    display_name: "Ana",
    workspace_name: "Ana Space",
    workspace_slug: "",
    plan_key: "free",
    seats: 1,
    interval: "month",
    trial: false,
    comp_reason: "",
    state: "pending",
    error_code: "",
    attempts: 0,
    created_by: "u-admin",
    created_at: "2026-11-16T10:00:00Z",
    updated_at: "2026-11-16T10:00:00Z",
    completed_at: null,
    welcome_sent_at: null,
    welcome_attempts: 0,
  };

  it("useAccountRequests fetches requests with optional state filter and errors on malformed body", async () => {
    const calls = stubFetch({
      "GET /api/platform/accounts": (url) =>
        url.searchParams.get("state") === "failed"
          ? json(200, { requests: "bad" })
          : json(200, { requests: [mockReq] }),
    });
    const { wrapper } = setup();

    const ok = renderHook(() => useAccountRequests(), { wrapper });
    await waitFor(() => expect(ok.result.current.isSuccess).toBe(true));
    expect(calls[0]?.search).toBe("");
    expect(ok.result.current.data?.[0]?.id).toBe("req-1");

    const bad = renderHook(() => useAccountRequests("failed"), { wrapper });
    await waitFor(() => expect(bad.result.current.isError).toBe(true));
    expect((bad.result.current.error as PlatformApiError).code).toBe("malformed_response");
  });

  it("useCreateAccountRequest sends Idempotency-Key and surfaces fields and Retry-After on errors", async () => {
    const seenHeaders: string[] = [];
    stubFetch({
      "POST /api/platform/accounts": (_url, init) => {
        const key = new Headers(init?.headers).get("Idempotency-Key") ?? "";
        seenHeaders.push(key);
        if (key === "key-invalid") {
          return json(400, {
            error: "the request has invalid fields",
            code: "account_request_invalid",
            fields: ["email", "workspace_slug"],
          });
        }
        if (key === "key-rate") {
          return new Response(
            JSON.stringify({ error: "too many requests", code: "account_create_rate_limited" }),
            { status: 429, headers: { "Content-Type": "application/json", "Retry-After": "90" } },
          );
        }
        return json(202, mockReq);
      },
    });
    const { queryClient, wrapper } = setup();
    queryClient.setQueryData(accountKeys.list(), [mockReq]);
    const { result } = renderHook(() => useCreateAccountRequest(), { wrapper });

    await act(async () => {
      const created = await result.current.mutateAsync({
        idempotencyKey: "key-ok",
        input: {
          kind: "individual",
          email: "ana@example.com",
          display_name: "Ana",
          workspace_name: "Ana Space",
          plan_key: "free",
          seats: 1,
          interval: "month",
        },
      });
      expect(created.id).toBe("req-1");
    });
    expect(seenHeaders[0]).toBe("key-ok");
    expect(queryClient.getQueryState(accountKeys.list())?.isInvalidated).toBe(true);

    await act(async () => {
      await expect(
        result.current.mutateAsync({
          idempotencyKey: "key-invalid",
          input: {
            kind: "individual",
            email: "bad",
            display_name: "Ana",
            workspace_name: "Ana Space",
            plan_key: "free",
            seats: 1,
            interval: "month",
          },
        }),
      ).rejects.toMatchObject({
        status: 400,
        code: "account_request_invalid",
        fields: ["email", "workspace_slug"],
      });
    });

    await act(async () => {
      try {
        await result.current.mutateAsync({
          idempotencyKey: "key-rate",
          input: {
            kind: "individual",
            email: "ana@example.com",
            display_name: "Ana",
            workspace_name: "Ana Space",
            plan_key: "free",
            seats: 1,
            interval: "month",
          },
        });
        throw new Error("expected rate limit error");
      } catch (err) {
        expect(err).toBeInstanceOf(AccountApiError);
        expect((err as AccountApiError).status).toBe(429);
        expect((err as AccountApiError).retryAfter).toBe("90");
      }
    });
  });

  it("useCancelAccountRequest and useResendWelcome invalidate account queries", async () => {
    const calls = stubFetch({
      "POST /api/platform/accounts/req-1/cancel": json(200, { ...mockReq, state: "canceled" }),
      "POST /api/platform/accounts/req-2/resend-welcome": new Response(
        JSON.stringify({ error: "rate limited", code: "account_welcome_rate_limited" }),
        { status: 429, headers: { "Content-Type": "application/json", "Retry-After": "60" } },
      ),
      "POST /api/platform/accounts/req-3/resend-welcome": noContent(),
    });
    const { queryClient, wrapper } = setup();
    queryClient.setQueryData(accountKeys.list(), [mockReq]);

    const cancelHook = renderHook(() => useCancelAccountRequest(), { wrapper });
    await act(async () => {
      const res = await cancelHook.result.current.mutateAsync({ id: "req-1" });
      expect(res.state).toBe("canceled");
    });
    expect(queryClient.getQueryState(accountKeys.list())?.isInvalidated).toBe(true);

    const resendHook = renderHook(() => useResendWelcome(), { wrapper });
    await act(async () => {
      await expect(resendHook.result.current.mutateAsync({ id: "req-2" })).rejects.toMatchObject({
        status: 429,
        retryAfter: "60",
      });
    });

    await act(async () => {
      const res = await resendHook.result.current.mutateAsync({ id: "req-3" });
      expect(res).toBeNull();
    });
    expect(calls.map((c) => c.path)).toEqual([
      "/api/platform/accounts/req-1/cancel",
      "/api/platform/accounts/req-2/resend-welcome",
      "/api/platform/accounts/req-3/resend-welcome",
    ]);
  });
});

describe("billing profile verification hook", () => {
  it("useSetBillingProfileVerified posts verify/unverify, invalidates workspace, and preserves incomplete fields", async () => {
    const calls = stubFetch({
      "POST /api/platform/workspaces/ws-1/billing-profile/verify": json(200, {
        workspace_id: "ws-1",
        account_kind: "business",
        verified: true,
        verified_at: "2026-11-17T00:00:00Z",
      }),
      "POST /api/platform/workspaces/ws-1/billing-profile/unverify": json(200, {
        workspace_id: "ws-1",
        account_kind: "business",
        verified: false,
        verified_at: null,
      }),
      "POST /api/platform/workspaces/ws-2/billing-profile/verify": json(409, {
        error: "billing profile is incomplete",
        code: "billing_profile_incomplete",
        fields: ["tax_code", "address"],
      }),
    });
    const { queryClient, wrapper } = setup();
    queryClient.setQueryData(platformKeys.workspace("ws-1"), null);
    queryClient.setQueryData(platformKeys.workspace("ws-2"), null);

    const { result } = renderHook(() => useSetBillingProfileVerified(), { wrapper });

    let verifiedRes: unknown;
    await act(async () => {
      verifiedRes = await result.current.mutateAsync({ workspaceId: "ws-1", verified: true });
    });
    expect(verifiedRes).toEqual({
      workspace_id: "ws-1",
      account_kind: "business",
      verified: true,
      verified_at: "2026-11-17T00:00:00Z",
    });
    expect(queryClient.getQueryState(platformKeys.workspace("ws-1"))?.isInvalidated).toBe(true);

    let unverifiedRes: unknown;
    await act(async () => {
      unverifiedRes = await result.current.mutateAsync({ workspaceId: "ws-1", verified: false });
    });
    expect(unverifiedRes).toEqual({
      workspace_id: "ws-1",
      account_kind: "business",
      verified: false,
      verified_at: null,
    });

    await act(async () => {
      await expect(
        result.current.mutateAsync({ workspaceId: "ws-2", verified: true }),
      ).rejects.toMatchObject({
        status: 409,
        code: "billing_profile_incomplete",
        details: { fields: ["tax_code", "address"] },
      });
    });
    expect(queryClient.getQueryState(platformKeys.workspace("ws-2"))?.isInvalidated).toBe(false);
    expect(calls.map((c) => `${c.method} ${c.path}`)).toEqual([
      "POST /api/platform/workspaces/ws-1/billing-profile/verify",
      "POST /api/platform/workspaces/ws-1/billing-profile/unverify",
      "POST /api/platform/workspaces/ws-2/billing-profile/verify",
    ]);
  });

  it("useSetBillingProfileVerified invalidates the workspace billing profile query key", async () => {
    stubFetch({
      "POST /api/platform/workspaces/ws-1/billing-profile/verify": json(200, {
        workspace_id: "ws-1",
        account_kind: "business",
        verified: true,
        verified_at: "2026-11-17T00:00:00Z",
      }),
      "POST /api/platform/workspaces/ws-1/billing-profile/unverify": json(200, {
        workspace_id: "ws-1",
        account_kind: "business",
        verified: false,
        verified_at: null,
      }),
    });
    const { queryClient, wrapper } = setup();
    queryClient.setQueryData(platformKeys.workspaceBillingProfile("ws-1"), {
      workspace_id: "ws-1",
      account_kind: "business",
      legal_name: "Acme Co",
      tax_code: "0123456789",
      address: "1 Main St",
      invoice_email: "ap@acme.test",
      verified: false,
      verified_at: null,
    });
    queryClient.setQueryData(platformKeys.workspaceBillingProfile("ws-2"), null);

    const { result } = renderHook(() => useSetBillingProfileVerified(), { wrapper });

    await act(async () => {
      await result.current.mutateAsync({ workspaceId: "ws-1", verified: true });
    });
    expect(queryClient.getQueryState(platformKeys.workspaceBillingProfile("ws-1"))?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(platformKeys.workspaceBillingProfile("ws-2"))?.isInvalidated).toBe(false);

    await act(async () => {
      await result.current.mutateAsync({ workspaceId: "ws-1", verified: false });
    });
    expect(queryClient.getQueryState(platformKeys.workspaceBillingProfile("ws-1"))?.isInvalidated).toBe(true);
  });

  it("useUpdateWorkspaceBillingProfile sends PUT, updates/invalidates workspace and billing profile caches, and surfaces field errors", async () => {
    const updated = {
      workspace_id: "ws-1",
      account_kind: "business" as const,
      legal_name: "Beta Co",
      tax_code: "0123456789",
      address: "2 New St",
      invoice_email: "ap@beta.test",
      verified: false,
      verified_at: null,
      changed: true,
    };
    const calls = stubFetch({
      "PUT /api/platform/workspaces/ws-1/billing-profile": json(200, updated),
      "PUT /api/platform/workspaces/a%2Fb/billing-profile": json(200, { ...updated, workspace_id: "a/b" }),
      "PUT /api/platform/workspaces/ws-incomplete/billing-profile": json(409, {
        error: "billing profile is incomplete",
        code: "billing_profile_incomplete",
        fields: ["legal_name", "tax_code"],
      }),
      "PUT /api/platform/workspaces/ws-invalid/billing-profile": json(400, {
        error: "billing profile has invalid fields",
        code: "billing_profile_invalid",
        fields: ["tax_code"],
      }),
      "PUT /api/platform/workspaces/ws-bad/billing-profile": json(200, { nope: true }),
    });
    const { queryClient, wrapper } = setup();
    queryClient.setQueryData(platformKeys.workspace("ws-1"), null);
    queryClient.setQueryData(platformKeys.workspaceBillingProfile("ws-1"), {
      workspace_id: "ws-1",
      account_kind: "business",
      legal_name: "Acme Co",
      tax_code: "0123456789",
      address: "1 Main St",
      invoice_email: "ap@acme.test",
      verified: true,
      verified_at: "2026-11-17T00:00:00Z",
    });
    queryClient.setQueryData(platformKeys.workspaceAudit("ws-1", {}), { pages: [], pageParams: [] });
    queryClient.setQueryData(platformKeys.audit({}), { pages: [], pageParams: [] });
    queryClient.setQueryData(platformKeys.workspace("ws-2"), null);
    queryClient.setQueryData(platformKeys.workspaceBillingProfile("ws-2"), null);
    queryClient.setQueryData(platformKeys.workspace("ws-incomplete"), null);

    const { result } = renderHook(() => useUpdateWorkspaceBillingProfile(), { wrapper });

    let res: unknown;
    await act(async () => {
      res = await result.current.mutateAsync({
        workspaceId: "ws-1",
        account_kind: "business",
        legal_name: "Beta Co",
        tax_code: "0123456789",
        address: "2 New St",
        invoice_email: "ap@beta.test",
      });
    });

    expect(res).toEqual(updated);
    expect(calls[0]).toMatchObject({
      method: "PUT",
      path: "/api/platform/workspaces/ws-1/billing-profile",
      body: {
        account_kind: "business",
        legal_name: "Beta Co",
        tax_code: "0123456789",
        address: "2 New St",
        invoice_email: "ap@beta.test",
      },
    });
    expect(queryClient.getQueryData(platformKeys.workspaceBillingProfile("ws-1"))).toEqual({
      workspace_id: "ws-1",
      account_kind: "business",
      legal_name: "Beta Co",
      tax_code: "0123456789",
      address: "2 New St",
      invoice_email: "ap@beta.test",
      verified: false,
      verified_at: null,
    });
    expect(queryClient.getQueryState(platformKeys.workspace("ws-1"))?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(platformKeys.workspaceBillingProfile("ws-1"))?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(platformKeys.workspaceAudit("ws-1", {}))?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(platformKeys.audit({}))?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(platformKeys.workspace("ws-2"))?.isInvalidated).toBe(false);
    expect(queryClient.getQueryState(platformKeys.workspaceBillingProfile("ws-2"))?.isInvalidated).toBe(false);

    await act(async () => {
      await result.current.mutateAsync({
        workspaceId: "a/b",
        account_kind: "individual",
      });
    });
    expect(calls[1]?.path).toBe("/api/platform/workspaces/a%2Fb/billing-profile");
    expect(calls[1]?.body).toEqual({ account_kind: "individual" });

    await act(async () => {
      await expect(
        result.current.mutateAsync({
          workspaceId: "ws-incomplete",
          account_kind: "business",
          legal_name: "",
          tax_code: "",
          address: "1 Main St",
          invoice_email: "ap@acme.test",
        }),
      ).rejects.toMatchObject({
        status: 409,
        code: "billing_profile_incomplete",
        details: { fields: ["legal_name", "tax_code"] },
      });
    });
    expect(queryClient.getQueryState(platformKeys.workspace("ws-incomplete"))?.isInvalidated).toBe(false);

    await act(async () => {
      await expect(
        result.current.mutateAsync({
          workspaceId: "ws-invalid",
          account_kind: "business",
          legal_name: "Acme Co",
          tax_code: "123",
          address: "1 Main St",
          invoice_email: "ap@acme.test",
        }),
      ).rejects.toMatchObject({
        status: 400,
        code: "billing_profile_invalid",
        details: { fields: ["tax_code"] },
      });
    });

    await act(async () => {
      await expect(
        result.current.mutateAsync({
          workspaceId: "ws-bad",
          account_kind: "individual",
          legal_name: "",
          tax_code: "",
          address: "",
          invoice_email: "",
        }),
      ).rejects.toMatchObject({
        code: "malformed_response",
      });
    });
  });
});

describe("e-invoices hooks", () => {
  it("useEinvoices fetches /api/platform/einvoices with optional state filter", async () => {
    const einvoice = {
      order_id: "ord-1",
      workspace_id: "ws-1",
      revision: 1,
      state: "failed",
      currency: "vnd",
      amount: 600000,
      vat_amount: 60000,
      provider: "sepay",
      invoice_number: "",
      lookup_url: "",
      attempts: 8,
      last_error_code: "rejected",
      missing_fields: [],
      next_attempt_at: "2026-11-18T12:00:00Z",
      updated_at: "2026-11-18T11:00:00Z",
    };
    const calls = stubFetch({
      "GET /api/platform/einvoices": () => json(200, { einvoices: [einvoice] }),
    });
    const { wrapper } = setup();

    const defaultQuery = renderHook(() => useEinvoices(), { wrapper });
    await waitFor(() => expect(defaultQuery.result.current.isSuccess).toBe(true));
    expect(defaultQuery.result.current.data).toEqual([einvoice]);
    expect(calls[0]).toMatchObject({
      method: "GET",
      path: "/api/platform/einvoices",
      search: "",
    });

    const filteredQuery = renderHook(() => useEinvoices("issued"), { wrapper });
    await waitFor(() => expect(filteredQuery.result.current.isSuccess).toBe(true));
    expect(calls[1]).toMatchObject({
      method: "GET",
      path: "/api/platform/einvoices",
      search: "?state=issued",
    });
  });

  it("useOrderEinvoices fetches /api/platform/orders/{id}/einvoices when orderId is provided", async () => {
    const historyItem = {
      revision: 1,
      state: "issued",
      invoice_number: "INV-1",
      lookup_url: "https://einvoice.sepay.vn/lookup/1",
      buyer: {
        name: "Acme",
        tax_code: "010",
        address: "HN",
        invoice_email: "acme@example.com",
      },
      attempts: 1,
      last_error_code: "",
      created_at: "2026-11-18T10:00:00Z",
      issued_at: "2026-11-18T10:05:00Z",
      updated_at: "2026-11-18T10:05:00Z",
    };
    const calls = stubFetch({
      "GET /api/platform/orders/ord-1/einvoices": json(200, { einvoices: [historyItem] }),
      "GET /api/platform/orders/ord-empty/einvoices": json(200, { einvoices: [] }),
      "GET /api/platform/orders/ord-missing/einvoices": json(404, {
        error: "order not found",
        code: "order_not_found",
      }),
      "GET /api/platform/orders/ord-bad/einvoices": json(200, { einvoices: "bad" }),
    });
    const { queryClient, wrapper } = setup();

    const idleQuery = renderHook(() => useOrderEinvoices(null), { wrapper });
    expect(idleQuery.result.current.fetchStatus).toBe("idle");
    expect(calls).toHaveLength(0);

    const historyQuery = renderHook(() => useOrderEinvoices("ord-1"), { wrapper });
    await waitFor(() => expect(historyQuery.result.current.isSuccess).toBe(true));
    expect(historyQuery.result.current.data).toEqual([historyItem]);
    expect(calls[0]?.path).toBe("/api/platform/orders/ord-1/einvoices");
    expect(queryClient.getQueryData(platformKeys.orderEinvoices("ord-1"))).toEqual([historyItem]);

    const emptyQuery = renderHook(() => useOrderEinvoices("ord-empty"), { wrapper });
    await waitFor(() => expect(emptyQuery.result.current.isSuccess).toBe(true));
    expect(emptyQuery.result.current.data).toEqual([]);

    const missingQuery = renderHook(() => useOrderEinvoices("ord-missing"), { wrapper });
    await waitFor(() => expect(missingQuery.result.current.isError).toBe(true));
    expect(missingQuery.result.current.error).toMatchObject({
      status: 404,
      code: "order_not_found",
    });

    const malformedQuery = renderHook(() => useOrderEinvoices("ord-bad"), { wrapper });
    await waitFor(() => expect(malformedQuery.result.current.isError).toBe(true));
    expect(malformedQuery.result.current.error).toMatchObject({
      code: "malformed_response",
    });
  });
});

describe("R6 S13 hooks (staff update, member requests, immediate deletion)", () => {
  it("useAddStaff and useUpdateStaff send tenant_id for admin and omit it for other roles", async () => {
    const calls = stubFetch({
      "POST /api/platform/staff": json(201, { user_id: "u2", email: "a@example.com", role: "admin", tenant_id: "t-1" }),
      "PUT /api/platform/staff/u2": json(200, { user_id: "u2", role: "system_admin", tenant_id: null }),
    });
    const { queryClient, wrapper } = setup();
    queryClient.setQueryData(platformKeys.staff(), []);

    const addHook = renderHook(() => useAddStaff(), { wrapper });
    await act(async () => {
      await addHook.result.current.mutateAsync({ email: "a@example.com", role: "admin", tenant_id: "t-1" });
    });
    expect(calls[0]).toMatchObject({
      method: "POST",
      path: "/api/platform/staff",
      body: { email: "a@example.com", role: "admin", tenant_id: "t-1" },
    });

    const updateHook = renderHook(() => useUpdateStaff(), { wrapper });
    await act(async () => {
      await updateHook.result.current.mutateAsync({ userId: "u2", role: "system_admin" });
    });
    expect(calls[1]).toMatchObject({
      method: "PUT",
      path: "/api/platform/staff/u2",
      body: { role: "system_admin" },
    });
    expect(queryClient.getQueryState(platformKeys.staff())?.isInvalidated).toBe(true);
  });

  it("member request hooks call /workspaces/{id}/members, /workspaces/{id}/member-requests, and /member-requests/{id}/cancel", async () => {
    const memberReq = {
      id: "mr-1",
      workspace_id: "ws-1",
      kind: "add",
      email: "new@example.com",
      role: "member",
      state: "pending",
      error_code: "",
      attempts: 0,
      user_created: false,
      requested_by: "u-admin",
      requested_by_label: "admin@example.com",
      created_at: "2026-11-19T10:00:00Z",
      updated_at: "2026-11-19T10:00:00Z",
      completed_at: null,
    };
    const calls = stubFetch({
      "GET /api/platform/workspaces/ws-1/member-requests": json(200, { requests: [memberReq] }),
      "POST /api/platform/workspaces/ws-1/members": json(202, memberReq),
      "POST /api/platform/member-requests/mr-1/cancel": json(200, { ...memberReq, state: "canceled" }),
    });
    const { queryClient, wrapper } = setup();

    const listHook = renderHook(() => useMemberRequests("ws-1"), { wrapper });
    await waitFor(() => expect(listHook.result.current.isSuccess).toBe(true));
    expect(listHook.result.current.data).toHaveLength(1);
    listHook.unmount();

    const disabledHook = renderHook(() => useMemberRequests("ws-2", false), { wrapper });
    expect(disabledHook.result.current.fetchStatus).toBe("idle");

    const addHook = renderHook(() => useAddWorkspaceMember(), { wrapper });
    await act(async () => {
      await addHook.result.current.mutateAsync({ workspaceId: "ws-1", email: "new@example.com", role: "member" });
    });
    expect(calls[1]).toMatchObject({
      method: "POST",
      path: "/api/platform/workspaces/ws-1/members",
      body: { email: "new@example.com", role: "member" },
    });
    expect(queryClient.getQueryState(platformKeys.memberRequests("ws-1"))?.isInvalidated).toBe(true);

    const cancelHook = renderHook(() => useCancelMemberRequest(), { wrapper });
    await act(async () => {
      await cancelHook.result.current.mutateAsync({ workspaceId: "ws-1", requestId: "mr-1" });
    });
    expect(calls[2]).toMatchObject({
      method: "POST",
      path: "/api/platform/member-requests/mr-1/cancel",
    });
  });

  it("immediate deletion and deletion-requests hooks call exact backend routes", async () => {
    const deletionReq = {
      id: "dr-1",
      kind: "workspace",
      target_id: "ws-1",
      target_label: "acme",
      reason: "cleanup",
      state: "pending",
      error_code: "",
      attempts: 0,
      tenant_id: "t-1",
      requested_by: "u-sys",
      requested_by_label: "sys@example.com",
      created_at: "2026-11-19T10:00:00Z",
      updated_at: "2026-11-19T10:00:00Z",
    };
    const accountDeletionReq = {
      ...deletionReq,
      id: "dr-acc",
      kind: "account",
      target_id: "u-9",
      target_label: "leaver@example.com",
      tenant_id: null,
    };
    const previewBody = {
      email: "leaver@example.com",
      need_choice: [{ workspace_id: "ws-1", slug: "acme", role: "owner", owner_count: 1, member_count: 3 }],
      deleted_with: [],
    };
    const calls = stubFetch({
      "POST /api/platform/workspaces/ws-1/delete-now": json(202, { request_id: "dr-1", state: "pending" }),
      "POST /api/platform/tenants/t-1/delete-now": json(202, { request_id: "dr-2", state: "pending" }),
      "GET /api/platform/users/u-9/delete-now-preview": json(200, previewBody),
      "POST /api/platform/users/u-9/delete-now": json(202, { request_id: "dr-3", state: "pending" }),
      "GET /api/platform/deletion-requests": json(200, { requests: [deletionReq, accountDeletionReq] }),
      "POST /api/platform/deletion-requests/dr-1/cancel": json(200, { ...deletionReq, state: "canceled" }),
      "POST /api/platform/deletion-requests/dr-1/retry": json(200, { ...deletionReq, state: "pending", attempts: 0 }),
    });
    const { wrapper } = setup();

    const wsDel = renderHook(() => useDeleteWorkspaceNow(), { wrapper });
    await act(async () => {
      await wsDel.result.current.mutateAsync({ workspaceId: "ws-1", reason: "cleanup", confirm: "acme" });
    });

    const tenantDel = renderHook(() => useDeleteTenantNow(), { wrapper });
    await act(async () => {
      await tenantDel.result.current.mutateAsync({ tenantId: "t-1", reason: "offboard", confirm: "Acme" });
    });

    const preview = renderHook(() => useAccountDeletePreview("u-9", true), { wrapper });
    await waitFor(() => expect(preview.result.current.isSuccess).toBe(true));
    expect(preview.result.current.data?.need_choice[0]?.successors).toEqual([]);
    preview.unmount();

    const userDel = renderHook(() => useDeleteAccountNow(), { wrapper });
    await act(async () => {
      await userDel.result.current.mutateAsync({
        userId: "u-9",
        reason: "legal request",
        confirm: "leaver@example.com",
        resolutions: [{ workspace_id: "ws-1", action: "delete" }],
      });
    });

    const listDel = renderHook(() => useDeletionRequests({ kind: "workspace", state: "failed" }), { wrapper });
    await waitFor(() => expect(listDel.result.current.isSuccess).toBe(true));
    expect(calls[4]?.search).toBe("?kind=workspace&state=failed");
    listDel.unmount();

    const accountListDel = renderHook(() => useDeletionRequests({ kind: "account" }), { wrapper });
    await waitFor(() => expect(accountListDel.result.current.isSuccess).toBe(true));
    expect(calls[5]?.search).toBe("");
    expect(accountListDel.result.current.data).toEqual([accountDeletionReq]);
    accountListDel.unmount();

    const cancelDel = renderHook(() => useCancelDeletionRequest(), { wrapper });
    await act(async () => {
      await cancelDel.result.current.mutateAsync({ id: "dr-1" });
    });

    const retryDel = renderHook(() => useRetryDeletionRequest(), { wrapper });
    await act(async () => {
      await retryDel.result.current.mutateAsync({ id: "dr-1" });
    });

    expect(calls.map((c) => `${c.method} ${c.path}`)).toEqual([
      "POST /api/platform/workspaces/ws-1/delete-now",
      "POST /api/platform/tenants/t-1/delete-now",
      "GET /api/platform/users/u-9/delete-now-preview",
      "POST /api/platform/users/u-9/delete-now",
      "GET /api/platform/deletion-requests",
      "GET /api/platform/deletion-requests",
      "POST /api/platform/deletion-requests/dr-1/cancel",
      "POST /api/platform/deletion-requests/dr-1/retry",
    ]);
  });

  it("R6 S23 workspace list, export, plan create/activate/deactivate/history hooks hit expected routes", async () => {
    const planObj = {
      key: "team",
      name: "Team",
      description: "Team plan",
      limits: { issue_count: 200 },
      features: {},
      currency: "vnd",
      seat_price_month: 100000,
      seat_price_year: null,
      seats_fixed: null,
      min_seats: 1,
      trial_days: 14,
      is_public: true,
      sort: 5,
      archived_at: null,
      audience: "business",
      billing_model: "seat",
    };

    const calls = stubFetch({
      "GET /api/platform/workspaces": (url) => {
        if (url.searchParams.get("cursor") === "cur-2") {
          return json(200, {
            workspaces: [
              {
                id: "ws-2",
                name: "Beta",
                slug: "beta",
                tenant_id: "t-1",
                tenant_name: "Acme",
                status: "suspended",
                plan_key: "team",
                member_count: 2,
                created_at: "2026-11-02T00:00:00Z",
              },
            ],
            next_cursor: null,
          });
        }
        return json(200, {
          workspaces: [
            {
              id: "ws-1",
              name: "Acme",
              slug: "acme",
              tenant_id: "t-1",
              tenant_name: "Acme",
              status: "active",
              plan_key: "team",
              member_count: 4,
              created_at: "2026-11-01T00:00:00Z",
            },
          ],
          next_cursor: "cur-2",
        });
      },
      "POST /api/platform/workspaces/ws-1/export": json(202, { export_id: "exp-1", status: "queued" }),
      "GET /api/platform/plans/team": json(200, { plan: planObj }),
      "POST /api/platform/plans": json(201, { plan: planObj }),
      "POST /api/platform/plans/team/deactivate": json(200, {
        changed: true,
        plan: { ...planObj, archived_at: "2026-11-20T00:00:00Z" },
      }),
      "POST /api/platform/plans/team/activate": json(200, {
        changed: true,
        plan: planObj,
      }),
      "GET /api/platform/plans/team/history": json(200, {
        history: [
          {
            at: "2026-11-20T00:00:00Z",
            actor: "root@example.com",
            action: "plan.activated",
            changes: [{ field: "archived_at", old: "2026-11-20T00:00:00Z", new: null }],
            reason: "",
          },
        ],
      }),
    });

    const { wrapper } = setup();

    const wsHook = renderHook(
      () => useWorkspaces({ q: "ac", status: "active", plan_key: "team", tenant_id: "t-1" }),
      { wrapper },
    );
    await waitFor(() => expect(wsHook.result.current.isSuccess).toBe(true));
    expect(calls[0]?.search).toBe("?q=ac&status=active&plan_key=team&tenant_id=t-1");
    expect(wsHook.result.current.hasNextPage).toBe(true);

    await act(async () => {
      await wsHook.result.current.fetchNextPage();
    });
    await waitFor(() => expect(wsHook.result.current.data).toHaveLength(2));
    expect(calls[1]?.search).toBe("?q=ac&status=active&plan_key=team&tenant_id=t-1&cursor=cur-2");
    wsHook.unmount();

    const exportHook = renderHook(() => useExportWorkspace(), { wrapper });
    await act(async () => {
      const res = await exportHook.result.current.mutateAsync({ workspaceId: "ws-1" });
      expect(res).toEqual({ export_id: "exp-1", status: "queued" });
    });

    const createHook = renderHook(() => useCreatePlan(), { wrapper });
    await act(async () => {
      const created = await createHook.result.current.mutateAsync({
        key: "team",
        name: "Team",
        description: "Team plan",
        limits: { issue_count: 200 },
        features: {},
        currency: "vnd",
        seat_price_month: 100000,
        seat_price_year: null,
        seats_fixed: null,
        min_seats: 1,
        trial_days: 14,
        is_public: true,
        sort: 5,
        audience: "business",
        billing_model: "seat",
      });
      expect(created?.plan.key).toBe("team");
    });

    const planDetail = renderHook(() => usePlan("team"), { wrapper });
    await waitFor(() => expect(planDetail.result.current.isSuccess).toBe(true));
    expect(planDetail.result.current.data?.name).toBe("Team");
    planDetail.unmount();

    const deactivateHook = renderHook(() => useDeactivatePlan(), { wrapper });
    await act(async () => {
      const res = await deactivateHook.result.current.mutateAsync({ key: "team" });
      expect(res?.changed).toBe(true);
    });

    const activateHook = renderHook(() => useActivatePlan(), { wrapper });
    await act(async () => {
      const res = await activateHook.result.current.mutateAsync({ key: "team" });
      expect(res?.changed).toBe(true);
    });

    const historyHook = renderHook(() => usePlanHistory("team"), { wrapper });
    await waitFor(() => expect(historyHook.result.current.isSuccess).toBe(true));
    expect(historyHook.result.current.data?.[0]?.action).toBe("plan.activated");
    historyHook.unmount();
  });
});

describe("R6 S24 tenant users, member changes and quota requests", () => {
  it("useTenantUsers fetches with filters and cursor paging and useTenantUser fetches detail", async () => {
    const calls = stubFetch({
      "GET /api/platform/tenants/t-1/users": (url) => {
        if (url.searchParams.get("cursor") === "cur-2") {
          return json(200, {
            users: [
              {
                user_id: "u-2",
                email: "bob@acme.test",
                name: "Bob",
                account_status: "suspended",
                is_staff: false,
                workspaces: [{ workspace_id: "ws-1", name: "Core", member_role: "admin" }],
                seats: 1,
                last_login_at: null,
              },
            ],
            next_cursor: null,
          });
        }
        return json(200, {
          users: [
            {
              user_id: "u-1",
              email: "alice@acme.test",
              name: "Alice",
              account_status: "active",
              is_staff: false,
              workspaces: [{ workspace_id: "ws-1", name: "Core", member_role: "member" }],
              seats: 1,
              last_login_at: "2026-11-18T10:00:00Z",
            },
          ],
          next_cursor: "cur-2",
        });
      },
      "GET /api/platform/tenants/t-1/users/u-1": json(200, {
        user_id: "u-1",
        email: "alice@acme.test",
        name: "Alice",
        account_status: "active",
        is_staff: false,
        workspaces: [{ workspace_id: "ws-1", name: "Core", member_role: "member" }],
        seats: 1,
        last_login_at: null,
      }),
    });

    const { wrapper } = setup();
    const listHook = renderHook(
      () =>
        useTenantUsers("t-1", {
          q: "alice",
          account_status: "active",
          workspace_id: "ws-1",
          member_role: "member",
        }),
      { wrapper },
    );
    await waitFor(() => expect(listHook.result.current.isSuccess).toBe(true));
    expect(listHook.result.current.data).toHaveLength(1);
    expect(calls[0]?.search).toContain("q=alice");
    expect(calls[0]?.search).toContain("account_status=active");
    expect(calls[0]?.search).toContain("workspace_id=ws-1");
    expect(calls[0]?.search).toContain("member_role=member");

    await act(async () => {
      await listHook.result.current.fetchNextPage();
    });
    await waitFor(() => expect(listHook.result.current.data).toHaveLength(2));
    expect(calls[1]?.search).toContain("cursor=cur-2");

    const detailHook = renderHook(() => useTenantUser("t-1", "u-1"), { wrapper });
    await waitFor(() => expect(detailHook.result.current.isSuccess).toBe(true));
    expect(detailHook.result.current.data?.email).toBe("alice@acme.test");
  });

  it("useDeactivateTenantUser, useRemoveWorkspaceMember, useChangeWorkspaceMemberRole and useCreateQuotaRequest mutate and invalidate", async () => {
    const calls = stubFetch({
      "POST /api/platform/tenants/t-1/users/u-1/deactivate": json(202, {
        request_ids: ["mr-1", "mr-2"],
      }),
      "POST /api/platform/workspaces/ws-1/members/u-1/remove": json(202, {
        request_id: "mr-3",
        kind: "remove",
      }),
      "POST /api/platform/workspaces/ws-1/members/u-1/role": json(202, {
        request_id: "mr-4",
        kind: "change_role",
      }),
      "POST /api/platform/workspaces/ws-1/quota-requests": json(201, {
        id: "qr-1",
        workspace_id: "ws-1",
        tenant_id: "t-1",
        gate: "agents",
        current_value: 5,
        requested_value: 20,
        approved_value: null,
        state: "pending",
        reason: "Need more agents",
        created_at: "2026-11-19T00:00:00Z",
      }),
    });

    const { queryClient, wrapper } = setup();
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

    const deactivateHook = renderHook(() => useDeactivateTenantUser(), { wrapper });
    await act(async () => {
      const res = await deactivateHook.result.current.mutateAsync({
        tenantId: "t-1",
        userId: "u-1",
        reason: "Offboarding",
      });
      expect(res.request_ids).toEqual(["mr-1", "mr-2"]);
    });
    expect(calls[0]).toMatchObject({
      method: "POST",
      path: "/api/platform/tenants/t-1/users/u-1/deactivate",
      body: { reason: "Offboarding" },
    });

    const removeHook = renderHook(() => useRemoveWorkspaceMember(), { wrapper });
    await act(async () => {
      const res = await removeHook.result.current.mutateAsync({
        workspaceId: "ws-1",
        userId: "u-1",
        tenantId: "t-1",
      });
      expect(res).toMatchObject({ request_id: "mr-3", kind: "remove" });
    });

    const roleHook = renderHook(() => useChangeWorkspaceMemberRole(), { wrapper });
    await act(async () => {
      const res = await roleHook.result.current.mutateAsync({
        workspaceId: "ws-1",
        userId: "u-1",
        role: "viewer",
        tenantId: "t-1",
      });
      expect(res).toMatchObject({ request_id: "mr-4", kind: "change_role" });
    });
    expect(calls[2]?.body).toEqual({ role: "viewer" });

    const quotaHook = renderHook(() => useCreateQuotaRequest(), { wrapper });
    await act(async () => {
      const res = await quotaHook.result.current.mutateAsync({
        workspaceId: "ws-1",
        gate: "agents",
        requested_value: 20,
        reason: "Need more agents",
      });
      expect(res.id).toBe("qr-1");
    });
    expect(calls[3]?.body).toEqual({
      gate: "agents",
      requested_value: 20,
      reason: "Need more agents",
    });

    const invalidatedKeys = invalidateSpy.mock.calls.map((c) => JSON.stringify(c[0]?.queryKey));
    expect(invalidatedKeys).toContain(JSON.stringify(platformKeys.tenantUsersAll("t-1")));
    expect(invalidatedKeys).toContain(JSON.stringify(platformKeys.memberRequests("ws-1")));
    expect(invalidatedKeys).toContain(JSON.stringify(platformKeys.quotaRequests()));
  });
});

describe("R6 S25 resources, limit overrides and runtimes hooks", () => {
  it("useTenantResources, useSystemResources and useSetTenantLimitOverride call exact routes and invalidate", async () => {
    const calls = stubFetch({
      "GET /api/platform/tenants/t-1/resources": json(200, {
        tenant_id: "t-1",
        resources: [{ type: "agents", used: 8, limit: 10, percent: 80, state: "warning" }],
        workspaces: [],
        complete: true,
      }),
      "GET /api/platform/resources": (url) =>
        url.searchParams.get("cursor") === "c1"
          ? json(200, {
              top_tenants: [{ tenant_id: "t-2", tenant_name: "Beta", agents: 2 }],
              over_limit: [],
              next_cursor: null,
              complete: true,
            })
          : json(200, {
              top_tenants: [{ tenant_id: "t-1", tenant_name: "Acme", agents: 8 }],
              over_limit: [
                {
                  tenant_id: "t-1",
                  tenant_name: "Acme",
                  workspace_id: "ws-1",
                  workspace_name: "Core",
                  type: "agents",
                  used: 8,
                  limit: 10,
                  percent: 80,
                  state: "warning",
                },
              ],
              next_cursor: "c1",
              complete: false,
            }),
      "POST /api/platform/tenants/t-1/limit-overrides": json(200, {
        gate: "agents",
        workspace_ids: ["ws-1"],
      }),
    });

    const { queryClient, wrapper } = setup();
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

    const tenantResHook = renderHook(() => useTenantResources("t-1"), { wrapper });
    await waitFor(() => expect(tenantResHook.result.current.isSuccess).toBe(true));
    expect(tenantResHook.result.current.data?.resources[0]?.state).toBe("warning");

    const sysHook = renderHook(() => useSystemResources({ sort: "agents" }), { wrapper });
    await waitFor(() => expect(sysHook.result.current.hasNextPage).toBe(true));
    expect(sysHook.result.current.data?.complete).toBe(false);
    await act(async () => {
      await sysHook.result.current.fetchNextPage();
    });
    await waitFor(() => expect(sysHook.result.current.data?.complete).toBe(true));
    expect(sysHook.result.current.data?.top_tenants.map((t) => t.tenant_id)).toEqual(["t-1", "t-2"]);
    expect(calls[1]?.search).toBe("?sort=agents");
    expect(calls[2]?.search).toBe("?sort=agents&cursor=c1");

    const overrideHook = renderHook(() => useSetTenantLimitOverride(), { wrapper });
    await act(async () => {
      await overrideHook.result.current.mutateAsync({
        tenantId: "t-1",
        gate: "agents",
        value: null,
        workspace_ids: ["ws-1"],
        reason: "Release limit",
      });
    });
    expect(
      calls.find((c) => c.path === "/api/platform/tenants/t-1/limit-overrides"),
    ).toMatchObject({
      method: "POST",
      path: "/api/platform/tenants/t-1/limit-overrides",
      body: { gate: "agents", value: null, workspace_ids: ["ws-1"], reason: "Release limit" },
    });

    const invalidatedKeys = invalidateSpy.mock.calls.map((c) => JSON.stringify(c[0]?.queryKey));
    expect(invalidatedKeys).toContain(JSON.stringify(platformKeys.tenantResources("t-1")));
    expect(invalidatedKeys).toContain(JSON.stringify(platformKeys.tenant("t-1")));
    expect(invalidatedKeys).toContain(JSON.stringify(platformKeys.resources()));
  });

  it("useRuntimes, useRuntime, useSetRuntimeStatus and useUnregisterRuntime call exact routes and invalidate", async () => {
    const rt = {
      id: "rt-1",
      name: "laptop",
      workspace_id: "ws-1",
      tenant_id: "t-1",
      status: "online",
      os: "darwin",
      daemon_version: "0.9.1",
      agent_version: "0.4.2",
      owner_id: "u-1",
      disabled: false,
      running_tasks: 1,
      last_seen_at: "2026-11-19T10:00:00Z",
      created_at: "2026-11-01T00:00:00Z",
    };
    const calls = stubFetch({
      "GET /api/platform/runtimes": json(200, { runtimes: [rt], next_cursor: null }),
      "GET /api/platform/runtimes/rt-1": json(200, { ...rt, tasks: [], failed_tasks_7d: 0 }),
      "POST /api/platform/runtimes/rt-1/status": json(200, { changed: true, disabled: true }),
      "POST /api/platform/runtimes/rt-1/unregister": json(202, {
        request_id: "dr-1",
        state: "pending",
      }),
    });

    const { queryClient, wrapper } = setup();
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

    const listHook = renderHook(
      () => useRuntimes({ q: "lap", status: "online", disabled: true, tenant_id: "t-1" }),
      { wrapper },
    );
    await waitFor(() => expect(listHook.result.current.isSuccess).toBe(true));
    expect(listHook.result.current.data).toHaveLength(1);
    expect(calls[0]?.search).toBe("?q=lap&status=online&tenant_id=t-1&disabled=true");

    const detailHook = renderHook(() => useRuntime("rt-1"), { wrapper });
    await waitFor(() => expect(detailHook.result.current.isSuccess).toBe(true));
    expect(detailHook.result.current.data?.name).toBe("laptop");

    const statusHook = renderHook(() => useSetRuntimeStatus(), { wrapper });
    await act(async () => {
      const res = await statusHook.result.current.mutateAsync({
        runtimeId: "rt-1",
        status: "disabled",
        reason: "Abuse report",
      });
      expect(res).toEqual({ changed: true, disabled: true });
    });
    expect(
      calls.find((c) => c.path === "/api/platform/runtimes/rt-1/status"),
    ).toMatchObject({
      method: "POST",
      path: "/api/platform/runtimes/rt-1/status",
      body: { status: "disabled", reason: "Abuse report" },
    });

    const unregisterHook = renderHook(() => useUnregisterRuntime(), { wrapper });
    await act(async () => {
      const res = await unregisterHook.result.current.mutateAsync({
        runtimeId: "rt-1",
        reason: "Retired",
        confirm: "laptop",
      });
      expect(res).toEqual({ request_id: "dr-1", state: "pending" });
    });
    expect(
      calls.find((c) => c.path === "/api/platform/runtimes/rt-1/unregister"),
    ).toMatchObject({
      method: "POST",
      path: "/api/platform/runtimes/rt-1/unregister",
      body: { reason: "Retired", confirm: "laptop" },
    });

    const invalidatedKeys = invalidateSpy.mock.calls.map((c) => JSON.stringify(c[0]?.queryKey));
    expect(invalidatedKeys).toContain(JSON.stringify(platformKeys.runtime("rt-1")));
    expect(invalidatedKeys).toContain(JSON.stringify(platformKeys.runtimes()));
    expect(invalidatedKeys).toContain(JSON.stringify(platformKeys.deletionRequests()));
  });

  it("supports tenant policy, tenant status, quota requests, approvals, legal holds, and audit export", async () => {
    const samplePolicy = {
      policy: {
        allowed_invite_domains: ["acme.com"],
        share_links_enabled: false,
        require_sso: true,
        require_mfa: false,
        session_max_age_hours: 24,
      },
      version: 2,
    };
    const sampleQuota = {
      id: "qr-1",
      workspace_id: "ws-1",
      tenant_id: "t-1",
      gate: "agents",
      current_value: 5,
      requested_value: 20,
      approved_value: null,
      state: "pending",
      reason: "Need more",
      decision_reason: "",
      requested_by: "u-owner",
      decided_by: "",
      created_at: "2026-11-19T10:00:00Z",
      decided_at: null,
    };
    const sampleHold = {
      id: "lh-1",
      scope_kind: "tenant",
      scope_id: "t-1",
      reason: "Subpoena",
      case_ref: "CASE-1",
      placed_by: "u-sys",
      released: false,
      released_by: "",
    };

    const calls = stubFetch({
      "GET /api/platform/tenants/t-1/policy": json(200, samplePolicy),
      "PUT /api/platform/tenants/t-1/policy": json(200, { ...samplePolicy, version: 3 }),
      "POST /api/platform/tenants/t-1/status": json(200, {
        status: "suspended",
        workspace_ids: ["ws-1"],
      }),
      "GET /api/platform/quota-requests": json(200, {
        requests: [sampleQuota],
        next_cursor: null,
      }),
      "GET /api/platform/quota-requests/qr-1": json(200, sampleQuota),
      "POST /api/platform/quota-requests/qr-1/approve": json(200, {
        ...sampleQuota,
        state: "approved",
        approved_value: 15,
      }),
      "POST /api/platform/quota-requests/qr-1/deny": json(200, {
        ...sampleQuota,
        state: "denied",
      }),
      "GET /api/platform/approvals": json(200, {
        approvals: [{ id: "dr-1", kind: "tenant", target_id: "t-1", state: "awaiting_approval" }],
      }),
      "POST /api/platform/approvals/dr-1/approve": json(200, {
        request_id: "dr-1",
        state: "pending",
      }),
      "POST /api/platform/approvals/dr-1/reject": json(200, {
        request_id: "dr-1",
        state: "canceled",
      }),
      "GET /api/platform/legal-holds": json(200, { legal_holds: [sampleHold] }),
      "POST /api/platform/legal-holds": json(201, sampleHold),
      "POST /api/platform/legal-holds/lh-1/release": json(200, {
        ...sampleHold,
        released: true,
      }),
      "GET /api/platform/audit-events/export": new Response('{"id":"ae-1"}\n', {
        status: 200,
        headers: { "Content-Type": "application/x-ndjson", "X-Next-Cursor": "cur-exp" },
      }),
    });

    const { queryClient, wrapper } = setup();
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

    const policyHook = renderHook(() => useTenantPolicy("t-1"), { wrapper });
    await waitFor(() => expect(policyHook.result.current.isSuccess).toBe(true));
    expect(policyHook.result.current.data?.version).toBe(2);

    const updatePolicyHook = renderHook(() => useUpdateTenantPolicy(), { wrapper });
    await act(async () => {
      const updated = await updatePolicyHook.result.current.mutateAsync({
        tenantId: "t-1",
        expected_version: 2,
        allowed_invite_domains: ["acme.com"],
        share_links_enabled: false,
        require_sso: true,
        require_mfa: true,
        session_max_age_hours: 24,
      });
      expect(updated.version).toBe(3);
    });

    const tenantStatusHook = renderHook(() => useSetTenantStatus(), { wrapper });
    await act(async () => {
      const res = await tenantStatusHook.result.current.mutateAsync({
        tenantId: "t-1",
        status: "suspended",
        reason: "Abuse",
      });
      expect(res.workspace_ids).toEqual(["ws-1"]);
    });

    const qrListHook = renderHook(() => useQuotaRequests({ state: "pending" }), { wrapper });
    await waitFor(() => expect(qrListHook.result.current.isSuccess).toBe(true));
    expect(qrListHook.result.current.data).toHaveLength(1);

    const qrOneHook = renderHook(() => useQuotaRequest("qr-1"), { wrapper });
    await waitFor(() => expect(qrOneHook.result.current.isSuccess).toBe(true));

    const qrApproveHook = renderHook(() => useApproveQuotaRequest(), { wrapper });
    await act(async () => {
      await qrApproveHook.result.current.mutateAsync({ id: "qr-1", value: 15, reason: "OK" });
    });

    const qrDenyHook = renderHook(() => useDenyQuotaRequest(), { wrapper });
    await act(async () => {
      await qrDenyHook.result.current.mutateAsync({ id: "qr-1", reason: "Over budget" });
    });

    const approvalsHook = renderHook(() => useApprovals(), { wrapper });
    await waitFor(() => expect(approvalsHook.result.current.isSuccess).toBe(true));
    expect(approvalsHook.result.current.data).toHaveLength(1);

    const approveApprovalHook = renderHook(() => useApproveApproval(), { wrapper });
    const rejectApprovalHook = renderHook(() => useRejectApproval(), { wrapper });
    await act(async () => {
      await approveApprovalHook.result.current.mutateAsync({ id: "dr-1" });
      await rejectApprovalHook.result.current.mutateAsync({ id: "dr-1" });
    });

    const holdsHook = renderHook(() => useLegalHolds(), { wrapper });
    await waitFor(() => expect(holdsHook.result.current.isSuccess).toBe(true));
    expect(holdsHook.result.current.data).toHaveLength(1);

    const placeHoldHook = renderHook(() => usePlaceLegalHold(), { wrapper });
    const releaseHoldHook = renderHook(() => useReleaseLegalHold(), { wrapper });
    await act(async () => {
      await placeHoldHook.result.current.mutateAsync({
        scope_kind: "tenant",
        scope_id: "t-1",
        reason: "Subpoena",
        case_ref: "CASE-1",
      });
      await releaseHoldHook.result.current.mutateAsync({ id: "lh-1" });
    });

    const exportHook = renderHook(() => useExportAudit(), { wrapper });
    await act(async () => {
      const exported = await exportHook.result.current.mutateAsync({
        from: "2026-11-01T00:00:00Z",
        to: "2026-11-10T00:00:00Z",
        format: "ndjson",
      });
      expect(exported.nextCursor).toBe("cur-exp");
      expect(exported.text).toContain("ae-1");
    });

    expect(calls.find((c) => c.path === "/api/platform/tenants/t-1/policy" && c.method === "PUT")).toBeDefined();
    expect(calls.find((c) => c.path === "/api/platform/audit-events/export")?.search).toBe(
      "?from=2026-11-01T00%3A00%3A00Z&to=2026-11-10T00%3A00%3A00Z&format=ndjson",
    );

    const invalidatedKeys = invalidateSpy.mock.calls.map((c) => JSON.stringify(c[0]?.queryKey));
    expect(invalidatedKeys).toContain(JSON.stringify(platformKeys.tenantPolicy("t-1")));
    expect(invalidatedKeys).toContain(JSON.stringify(platformKeys.quotaRequests()));
    expect(invalidatedKeys).toContain(JSON.stringify(platformKeys.approvals()));
    expect(invalidatedKeys).toContain(JSON.stringify(platformKeys.legalHolds()));
  });
});
