// @vitest-environment node
import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import {
  AUDIT_ACTIONS,
  parseAuditPage,
  parseMe,
  parseNotice,
  parseNoticeDetail,
  parseNoticeList,
  parseNotePage,
  parseOverview,
  parseStaffList,
  parseWorkspaceDetail,
  parseTenantDetail,
  parseTenantPage,
  parseUserDetail,
  parseUserPage,
  parseOrderList,
  parseOrderActionResult,
  parsePutSubscriptionResponse,
  parsePlanList,
  parsePutPlanResponse,
  parseDeletePlanResponse,
  parseVerifyCodeResponse,
  parseResetMfaResponse,
  parseAccountRequestList,
  parseAccountRequest,
  parseResendWelcomeResponse,
  parseWorkspaceUsage,
  parsePriceChangeList,
  parsePriceChangeDryRunResponse,
  parseSchedulePriceChangeResponse,
  parseCancelPriceChangeResponse,
  parseBillingProfileVerifyResponse,
  parseBillingProfile,
  parsePutBillingProfileResponse,
  EINVOICE_STATES,
  parseEinvoiceList,
  OrderEinvoiceListSchema,
  parseOrderEinvoiceList,
  parseUpdateStaffResponse,
  parseMemberRequest,
  parseMemberRequestList,
  parseDeletionRequest,
  parseDeletionRequestList,
  parseDeletionStartResponse,
  parseAccountDeletePreview,
  parseWorkspacePage,
  parsePlanDetail,
  parsePlanHistory,
  parseWorkspaceExportResponse,
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
} from "./schemas";

const workspace = {
  id: "t1",
  name: "Acme",
  slug: "acme",
  status: "active",
  member_count: 3,
  created_at: "2026-01-01T00:00:00Z",
  last_activity_at: "2026-02-01T00:00:00Z",
};

const user = {
  id: "u1",
  email: "a@example.com",
  name: "A",
  status: "active",
  memberships_count: 1,
  created_at: "2026-01-01T00:00:00Z",
};

describe("audit action codes", () => {
  it("match the codes the Go audit package defines", () => {
    const source = readFileSync(new URL("../../../audit/audit.go", import.meta.url), "utf8");
    const goCodes = [...source.matchAll(/^\s*Action\w+\s*=\s*"([^"]+)"/gm)].map((m) => m[1]);
    expect(goCodes.length).toBeGreaterThan(0);
    expect([...AUDIT_ACTIONS].sort()).toEqual([...goCodes].sort());
  });
});

describe("me", () => {
  it("parses a well-formed body", () => {
    expect(parseMe({ user_id: "u1", email: "a@example.com", role: "admin", tenant_id: "t1" })).toEqual({
      user_id: "u1",
      email: "a@example.com",
      role: "admin",
      tenant_id: "t1",
    });
    expect(parseMe({ user_id: "u1", email: "s@example.com", role: "system_admin", tenant_id: null })).toEqual({
      user_id: "u1",
      email: "s@example.com",
      role: "system_admin",
      tenant_id: null,
    });
  });

  it("falls back to null for a malformed body", () => {
    for (const body of [null, "x", [], {}, { user_id: 7 }, { user_id: "u1", tenant_id: 42 }]) {
      expect(parseMe(body)).toBeNull();
    }
  });

  it("me response accepts a missing or null tenant_id and an unknown role falls back", () => {
    expect(parseMe({ user_id: "u1", role: "auditor" })?.role).toBe("unknown");
    expect(parseMe({ user_id: "u1" })).toEqual({
      user_id: "u1",
      email: "",
      role: "unknown",
      tenant_id: null,
    });
    expect(parseMe({ user_id: "u1", email: "a@example.com", role: "support", tenant_id: null })).toEqual({
      user_id: "u1",
      email: "a@example.com",
      role: "support",
      tenant_id: null,
    });
  });
});

const tenant = {
  id: "t1",
  name: "Acme",
  status: "active",
  workspace_count: 2,
  max_workspaces: 3,
  best_plan_key: "pro",
  status_summary: { active: 1, suspended: 1 },
  created_at: "2026-01-01T00:00:00Z",
  last_activity_at: "2026-02-01T00:00:00Z",
};

describe("tenants", () => {
  it("parses a page and keeps the cursor", () => {
    const page = parseTenantPage({ tenants: [tenant], next_cursor: "abc" });
    expect(page.tenants).toEqual([tenant]);
    expect(page.next_cursor).toBe("abc");
  });

  it("falls back to an empty page for a malformed body", () => {
    const empty = { tenants: [], next_cursor: null };
    for (const body of [null, "x", { tenants: "nope" }, { tenants: [{ name: "no id" }] }]) {
      expect(parseTenantPage(body)).toEqual(empty);
    }
  });

  it("defaults missing optional fields and an absent cursor", () => {
    const page = parseTenantPage({ tenants: [{ id: "t1" }] });
    expect(page.tenants[0]).toEqual({
      id: "t1",
      name: "",
      status: "unknown",
      workspace_count: 0,
      max_workspaces: null,
      best_plan_key: "",
      status_summary: {},
      created_at: "",
      last_activity_at: "",
    });
    expect(page.next_cursor).toBeNull();
  });

  it("tenant schema falls back for unknown fields and null max_workspaces", () => {
    // A null limit is no limit; an unknown field is ignored; an unknown status is unknown (I9).
    const parsed = parseTenantPage({
      tenants: [{ ...tenant, max_workspaces: null, status: "archived", added_by_a_newer_server: { x: 1 } }],
    }).tenants[0];
    expect(parsed?.max_workspaces).toBeNull();
    expect(parsed?.status).toBe("unknown");
    expect(parsed).not.toHaveProperty("added_by_a_newer_server");
    // A string where a number belongs makes the page malformed, not a zero limit.
    expect(parseTenantPage({ tenants: [{ ...tenant, max_workspaces: "ten" }] })).toEqual({ tenants: [], next_cursor: null });
  });

  it("parses a tenant detail with its workspaces and their usage", () => {
    const detail = parseTenantDetail({
      ...tenant,
      created_by: "u1",
      workspaces: [
        {
          id: "w1",
          name: "Main",
          slug: "main",
          status: "active",
          plan_key: "pro",
          member_count: 3,
          owner: { id: "u1", email: "a@example.com" },
          created_at: "2026-01-01T00:00:00Z",
          usage_day: "2026-11-14",
          usage_members: 3,
        },
        { id: "w2" },
      ],
    });
    expect(detail?.workspaces).toHaveLength(2);
    expect(detail?.workspaces[0]?.owner).toEqual({ id: "u1", email: "a@example.com" });
    expect(detail?.workspaces[0]?.usage_members).toBe(3);
    expect(detail?.workspaces[1]).toMatchObject({ name: "", plan_key: "", owner: null, status: "unknown" });
    expect(parseTenantDetail(tenant)?.workspaces).toEqual([]);
    for (const body of [null, "x", {}, { id: 7 }]) expect(parseTenantDetail(body)).toBeNull();
  });
});

describe("workspaces", () => {
  it("parses a detail with its state and owner", () => {
    const detail = parseWorkspaceDetail({
      ...workspace,
      state: { status: "suspended", reason: "abuse", changed_by: "u9", changed_at: "2026-02-02T00:00:00Z", deletion_scheduled_at: null },
      notes_count: 2,
      group_count: 4,
      owner: { id: "u1", email: "a@example.com" },
    });
    expect(detail?.state.status).toBe("suspended");
    expect(detail?.state.reason).toBe("abuse");
    expect(detail?.owner).toEqual({ id: "u1", email: "a@example.com" });
    expect(detail?.notes_count).toBe(2);
    expect(detail?.group_count).toBe(4);
  });

  it("defaults a missing state, owner, group count and notes count in a detail", () => {
    const detail = parseWorkspaceDetail(workspace);
    expect(detail?.state).toEqual({
      status: "unknown",
      reason: "",
      changed_by: null,
      changed_at: null,
      deletion_scheduled_at: null,
    });
    expect(detail?.owner).toBeNull();
    expect(detail?.notes_count).toBe(0);
    expect(detail?.group_count).toBe(0);
  });

  it("falls back to null for a malformed detail", () => {
    for (const body of [null, "x", {}, { id: 7 }]) expect(parseWorkspaceDetail(body)).toBeNull();
  });
});

describe("workspace usage", () => {
  const usage = {
    usage_day: "2026-11-14",
    usage_members: 3,
    usage_agents: 2,
    usage_runtimes: 1,
    usage_issues_total: 40,
    usage_issues_created: 5,
    usage_tasks_run: 9,
    usage_storage_bytes: 2048,
  };
  const noRow = {
    usage_day: null,
    usage_members: 0,
    usage_agents: 0,
    usage_runtimes: 0,
    usage_issues_total: 0,
    usage_issues_created: 0,
    usage_tasks_run: 0,
    usage_storage_bytes: 0,
  };

  it("parses the usage of a workspace that has a row", () => {
    expect(parseWorkspaceDetail({ ...workspace, ...usage })).toMatchObject(usage);
  });

  it("accepts a workspace with no row: a null day and zeros", () => {
    const parsed = parseWorkspaceDetail({ ...workspace, ...noRow });
    expect(parsed?.usage_day).toBeNull();
    expect(parsed?.usage_storage_bytes).toBe(0);
  });

  it("accepts a workspace that carries no usage fields at all", () => {
    const parsed = parseWorkspaceDetail(workspace);
    expect(parsed?.usage_day).toBeUndefined();
  });

  it("refuses a detail whose usage fields are malformed", () => {
    for (const bad of [{ usage_members: "many" }, { usage_storage_bytes: null }, { usage_day: 20261114 }]) {
      expect(parseWorkspaceDetail({ ...workspace, ...bad })).toBeNull();
    }
  });

  it("parses the current counts of a detail and leaves a missing block out", () => {
    const current = { members: 4, agents: 2, runtimes: 1, issues_total: 41, storage_bytes: 4096 };
    expect(parseWorkspaceDetail({ ...workspace, ...usage, usage_current: current })?.usage_current).toEqual(current);
    expect(parseWorkspaceDetail({ ...workspace, usage_current: { members: 4 } })?.usage_current).toEqual({
      members: 4,
      agents: 0,
      runtimes: 0,
      issues_total: 0,
      storage_bytes: 0,
    });
    expect(parseWorkspaceDetail(workspace)?.usage_current).toBeUndefined();
    expect(parseWorkspaceDetail({ ...workspace, usage_current: { members: "x" } })).toBeNull();
  });

  it("parses the usage totals of the overview and leaves them out when absent", () => {
    const totals = {
      usage_day: "2026-11-14",
      usage_workspaces: 2,
      usage_members: 7,
      usage_agents: 3,
      usage_runtimes: 2,
      usage_issues_total: 90,
      usage_issues_created: 8,
      usage_tasks_run: 11,
      usage_storage_bytes: 9000,
    };
    expect(parseOverview({ workspaces_by_status: {}, signups_7d: 0, ...totals })).toMatchObject(totals);
    const bare = parseOverview({ workspaces_by_status: {}, signups_7d: 0 });
    expect(bare?.usage_day).toBeUndefined();
    expect(parseOverview({ workspaces_by_status: {}, usage_tasks_run: "x" })).toBeNull();
  });
});

describe("users", () => {
  it("parses a page", () => {
    expect(parseUserPage({ users: [user], next_cursor: null })).toEqual({ users: [user], next_cursor: null });
  });

  it("falls back to an empty page for a malformed body", () => {
    for (const body of [null, 3, { users: {} }, { users: [{ email: "no id" }] }]) {
      expect(parseUserPage(body)).toEqual({ users: [], next_cursor: null });
    }
  });

  it("maps an unknown account status to unknown", () => {
    expect(parseUserPage({ users: [{ ...user, status: "banned" }] }).users[0]?.status).toBe("unknown");
  });

  it("parses a detail with memberships and the account record", () => {
    const detail = parseUserDetail({
      ...user,
      memberships: [{ workspace_id: "t1", name: "Acme", role: "owner" }],
      account: { status: "suspended", reason: "fraud", changed_at: "2026-02-02T00:00:00Z", sessions_valid_after: null },
    });
    expect(detail?.memberships).toEqual([{ workspace_id: "t1", name: "Acme", role: "owner" }]);
    expect(detail?.account.status).toBe("suspended");
    expect(detail?.account.reason).toBe("fraud");
  });

  it("defaults missing memberships and account in a detail", () => {
    const detail = parseUserDetail(user);
    expect(detail?.memberships).toEqual([]);
    expect(detail?.account).toEqual({ status: "unknown", reason: "", changed_at: null, sessions_valid_after: null });
  });

  it("falls back to null for a malformed detail", () => {
    for (const body of [null, [], { memberships: [] }]) expect(parseUserDetail(body)).toBeNull();
  });
});

describe("notes", () => {
  const note = {
    id: "n1",
    workspace_id: "t1",
    author_id: "u1",
    author_email: "a@example.com",
    body: "called the owner",
    created_at: "2026-02-01T00:00:00Z",
  };

  it("parses a page", () => {
    expect(parseNotePage({ notes: [note], next_cursor: "c" })).toEqual({ notes: [note], next_cursor: "c" });
  });

  it("accepts a null author email and defaults missing fields", () => {
    const page = parseNotePage({ notes: [{ id: "n1", author_email: null }] });
    expect(page.notes[0]).toEqual({
      id: "n1",
      workspace_id: "",
      author_id: "",
      author_email: null,
      body: "",
      created_at: "",
    });
  });

  it("falls back to an empty page for a malformed body", () => {
    for (const body of [null, "x", { notes: [{ body: "no id" }] }, { notes: 1 }]) {
      expect(parseNotePage(body)).toEqual({ notes: [], next_cursor: null });
    }
  });
});

describe("staff", () => {
  const member = {
    user_id: "u1",
    email: "a@example.com",
    role: "admin",
    tenant_id: "t-1",
    tenant_name: null,
    created_by: null,
    created_at: "2026-01-01T00:00:00Z",
  };

  it("parses the list", () => {
    expect(parseStaffList({ staff: [member] })).toEqual({ staff: [member] });
    expect(parseStaffList({ staff: [{ user_id: "u2", email: "s@example.com", role: "system_admin" }] }).staff[0]).toEqual({
      user_id: "u2",
      email: "s@example.com",
      role: "system_admin",
      tenant_id: null,
      tenant_name: null,
      created_by: null,
      created_at: "",
    });
  });

  it("maps an unknown role to unknown", () => {
    expect(parseStaffList({ staff: [{ ...member, role: "owner" }] }).staff[0]?.role).toBe("unknown");
  });

  it("falls back to an empty list for a malformed body", () => {
    for (const body of [null, [], { staff: [{ email: "no id" }] }, { staff: "x" }]) {
      expect(parseStaffList(body)).toEqual({ staff: [] });
    }
  });

  it("parses PUT /staff/{user_id} response", () => {
    expect(parseUpdateStaffResponse({ user_id: "u1", role: "admin", tenant_id: "t-1" })).toEqual({
      user_id: "u1",
      role: "admin",
      tenant_id: "t-1",
    });
    expect(parseUpdateStaffResponse({ user_id: "u1", role: "future_role", tenant_id: null })).toEqual({
      user_id: "u1",
      role: "unknown",
      tenant_id: null,
    });
    expect(parseUpdateStaffResponse({})).toBeNull();
  });
});

describe("audit events", () => {
  const event = {
    id: "e1",
    workspace_id: "t1",
    actor_type: "staff",
    actor_id: "u1",
    actor_label: "a@example.com",
    action: "workspace.status_changed",
    target_type: "workspace",
    target_id: "t1",
    target_label: "Acme",
    detail: { from: "active", to: "suspended" },
    ip: "10.0.0.1",
    user_agent: "ua",
    request_id: "r1",
    created_at: "2026-02-01T00:00:00Z",
  };

  it("parses a page", () => {
    expect(parseAuditPage({ events: [event], next_cursor: "c" }, "test")).toEqual({ events: [event], next_cursor: "c" });
  });

  it("falls back to an empty page for a malformed body", () => {
    for (const body of [null, "x", { events: [{ action: "no id" }] }, { events: {} }]) {
      expect(parseAuditPage(body, "test")).toEqual({ events: [], next_cursor: null });
    }
  });

  it("maps an unknown actor type to unknown and keeps an unlisted action code", () => {
    const parsed = parseAuditPage({ events: [{ ...event, actor_type: "robot", action: "billing.charged" }] }, "test");
    expect(parsed.events[0]?.actor_type).toBe("unknown");
    expect(parsed.events[0]?.action).toBe("billing.charged");
  });

  it("replaces a detail that is not an object with an empty one", () => {
    for (const detail of [[1, 2], "text", 7, null, undefined]) {
      expect(parseAuditPage({ events: [{ ...event, detail }] }, "test").events[0]?.detail).toEqual({});
    }
  });

  it("accepts a null workspace and actor", () => {
    const parsed = parseAuditPage({ events: [{ ...event, workspace_id: null, actor_id: null }] }, "test");
    expect(parsed.events[0]?.workspace_id).toBeNull();
    expect(parsed.events[0]?.actor_id).toBeNull();
  });
});

describe("overview", () => {
  it("parses the counts", () => {
    const overview = parseOverview({
      workspaces_by_status: { active: 5, read_only: 1, suspended: 2, pending_deletion: 0 },
      signups_7d: 9,
    });
    expect(overview).toEqual({
      workspaces_by_status: { active: 5, read_only: 1, suspended: 2, pending_deletion: 0 },
      signups_7d: 9,
    });
  });

  it("zero-fills a missing known status and keeps an unknown one", () => {
    const overview = parseOverview({ workspaces_by_status: { active: 4, archived: 2 } });
    expect(overview?.workspaces_by_status).toEqual({
      active: 4,
      read_only: 0,
      suspended: 0,
      pending_deletion: 0,
      archived: 2,
    });
    expect(overview?.signups_7d).toBe(0);
  });

  it("falls back to null for a malformed body", () => {
    for (const body of [null, "x", [], { workspaces_by_status: { active: "many" } }, { signups_7d: "9" }]) {
      expect(parseOverview(body)).toBeNull();
    }
  });
});

describe("orders", () => {
  const order = {
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

  it("parses a well-formed orders list including usage kind", () => {
    const parsed = parseOrderList({ orders: [order, { ...order, id: "ord-u", kind: "usage" }] });
    expect(parsed.orders).toHaveLength(2);
    expect(parsed.orders[0]?.id).toBe("ord-1");
    expect(parsed.orders[0]?.transfer_code).toBe("MUL12345678");
    expect(parsed.orders[0]?.status).toBe("pending");
    expect(parsed.orders[1]?.kind).toBe("usage");
  });

  it("falls back to empty list on malformed orders body", () => {
    for (const body of [null, undefined, "invalid", 123, { orders: "not-an-array" }]) {
      expect(parseOrderList(body)).toEqual({ orders: [] });
    }
  });

  it("parses a well-formed order action result", () => {
    const parsed = parseOrderActionResult({ order, applied: true }, "POST /orders/{id}/confirm");
    expect(parsed).toEqual({ order: { ...order, invoice_state: null }, applied: true });
  });

  it("parses invoice_state for allowed values, defaults absent or null to null, and maps unknown strings to null", () => {
    for (const state of ["pending", "issued", "needs_info", "failed"] as const) {
      const parsed = parseOrderList({ orders: [{ ...order, invoice_state: state }] });
      expect(parsed.orders[0]?.invoice_state).toBe(state);
    }
    expect(parseOrderList({ orders: [{ ...order, invoice_state: null }] }).orders[0]?.invoice_state).toBeNull();
    expect(parseOrderList({ orders: [order] }).orders[0]?.invoice_state).toBeNull();
    for (const unknownState of ["cancelled", "archived", "something_else", ""]) {
      expect(
        parseOrderList({ orders: [{ ...order, invoice_state: unknownState }] }).orders[0]?.invoice_state,
      ).toBeNull();
    }
    expect(parseOrderList({ orders: [{ ...order, invoice_state: 42 }] })).toEqual({ orders: [] });
  });

  it("falls back to null on malformed order action result", () => {
    for (const body of [null, undefined, "invalid", {}, { order: null }, { order: "not-an-object" }]) {
      expect(parseOrderActionResult(body, "POST /orders/{id}/confirm")).toBeNull();
    }
  });
});

describe("subscriptions", () => {
  const subscription = {
    workspace_id: "ws-1",
    plan_key: "pro",
    status: "active",
    seats: 5,
    interval: "month",
    current_period_start: "2026-11-01T00:00:00Z",
    current_period_end: "2026-12-01T00:00:00Z",
    trial_end: null,
    grace_until: null,
    cancel_at_period_end: false,
    limit_overrides: { issue_count: 500 },
    version: 1,
    updated_at: "2026-11-01T00:00:00Z",
  };

  it("parses a well-formed putSubscription response", () => {
    const parsed = parsePutSubscriptionResponse({ changed: true, subscription });
    expect(parsed).toEqual({ changed: true, subscription });
  });

  it("falls back to null on malformed putSubscription response", () => {
    for (const body of [null, undefined, "bad", {}, { changed: true }, { subscription: "bad" }]) {
      expect(parsePutSubscriptionResponse(body)).toBeNull();
    }
  });
});

describe("plans", () => {
  const plan = {
    key: "pro",
    name: "Pro",
    description: "",
    limits: { issue_count: 500 },
    features: { sso: true },
    currency: "vnd",
    seat_price_month: 200000,
    seat_price_year: 2000000,
    seats_fixed: null,
    min_seats: 1,
    trial_days: 14,
    is_public: true,
    sort: 10,
    archived_at: null,
    audience: "any",
    billing_model: "seat",
  };

  it("parses a well-formed plan list and applies audience/billing_model defaults and unknown fallback", () => {
    const parsed = parsePlanList({
      plans: [
        plan,
        { key: "payg", audience: "business", billing_model: "usage" },
        { key: "future", audience: "enterprise", billing_model: "credits" },
      ],
    });
    expect(parsed.plans).toHaveLength(3);
    expect(parsed.plans[0]?.key).toBe("pro");
    expect(parsed.plans[0]?.audience).toBe("any");
    expect(parsed.plans[0]?.billing_model).toBe("seat");
    expect(parsed.plans[1]?.audience).toBe("business");
    expect(parsed.plans[1]?.billing_model).toBe("usage");
    expect(parsed.plans[2]?.audience).toBe("unknown");
    expect(parsed.plans[2]?.billing_model).toBe("unknown");
  });

  it("falls back to empty list on malformed plan list", () => {
    for (const body of [null, undefined, "bad", 42, { plans: "not-an-array" }]) {
      expect(parsePlanList(body)).toEqual({ plans: [] });
    }
  });

  it("parses a well-formed putPlan response", () => {
    const parsed = parsePutPlanResponse({ changed: true, plan });
    expect(parsed).toEqual({ changed: true, plan });
  });

  it("falls back to null on malformed putPlan response", () => {
    for (const body of [null, undefined, "bad", {}, { plan: null }]) {
      expect(parsePutPlanResponse(body)).toBeNull();
    }
  });

  it("parses a well-formed deletePlan response", () => {
    const parsed = parseDeletePlanResponse({ deleted: true });
    expect(parsed).toEqual({ deleted: true });
  });

  it("falls back to null on malformed deletePlan response", () => {
    for (const body of [null, undefined, "bad", {}, { deleted: "yes" }]) {
      expect(parseDeletePlanResponse(body)).toBeNull();
    }
  });
});

describe("verify-code response", () => {
  it("asks for the authenticator code when the server says so", () => {
    expect(parseVerifyCodeResponse({ mfa_required: true })).toEqual({ mfa_required: true });
  });

  it("reads a signed-in answer as no further step", () => {
    for (const body of [{ user_id: "u1", role: "admin" }, {}, { mfa_required: false }]) {
      expect(parseVerifyCodeResponse(body)).toEqual({ mfa_required: false });
    }
  });

  it("falls back to null on a malformed body", () => {
    for (const body of [null, undefined, "bad", 42, [], { mfa_required: "yes" }, { mfa_required: null }]) {
      expect(parseVerifyCodeResponse(body)).toBeNull();
    }
  });
});

describe("mfa-reset response", () => {
  it("parses a well-formed response", () => {
    expect(parseResetMfaResponse({ reset: true })).toEqual({ reset: true });
    expect(parseResetMfaResponse({ reset: false })).toEqual({ reset: false });
  });

  it("falls back to null on a malformed body", () => {
    for (const body of [null, undefined, "bad", 42, {}, { reset: "yes" }, { reset: null }]) {
      expect(parseResetMfaResponse(body)).toBeNull();
    }
  });
});

describe("account requests", () => {
  const accountReq = {
    id: "req-1",
    kind: "business",
    email: "boss@acme.example",
    display_name: "Boss",
    workspace_name: "Acme",
    workspace_slug: "acme",
    plan_key: "business",
    seats: 5,
    interval: "month",
    trial: true,
    comp_reason: "",
    state: "pending",
    error_code: "",
    attempts: 0,
    created_by: "u-admin",
    created_at: "2026-11-16T00:00:00Z",
    updated_at: "2026-11-16T00:00:00Z",
    completed_at: null,
    welcome_sent_at: null,
    welcome_attempts: 0,
  };

  it("parses a well-formed account request list and defaults missing optional fields", () => {
    const parsed = parseAccountRequestList({ requests: [accountReq] });
    expect(parsed?.requests).toHaveLength(1);
    expect(parsed?.requests[0]).toEqual({
      ...accountReq,
      user_id: "",
      workspace_id: "",
      logged_in: null,
    });
  });

  it("parses logged_in as boolean or null and defaults absent logged_in to null", () => {
    expect(parseAccountRequest({ ...accountReq, logged_in: false })?.logged_in).toBe(false);
    expect(parseAccountRequest({ ...accountReq, logged_in: true })?.logged_in).toBe(true);
    expect(parseAccountRequest({ ...accountReq, logged_in: null })?.logged_in).toBeNull();
    expect(parseAccountRequest(accountReq)?.logged_in).toBeNull();
    expect(parseAccountRequest({ ...accountReq, logged_in: "yes" })).toBeNull();
  });

  it("maps unknown kind, state, and interval to unknown", () => {
    const parsed = parseAccountRequest({
      id: "req-2",
      kind: "enterprise",
      state: "retrying",
      interval: "decade",
    });
    expect(parsed?.kind).toBe("unknown");
    expect(parsed?.state).toBe("unknown");
    expect(parsed?.interval).toBe("unknown");
  });

  it("falls back to null on malformed account request list or item", () => {
    for (const body of [null, undefined, "bad", {}, { requests: "nope" }, { requests: [{ email: "no-id" }] }]) {
      expect(parseAccountRequestList(body)).toBeNull();
    }
    for (const body of [null, "bad", [], {}, { id: 7 }]) {
      expect(parseAccountRequest(body)).toBeNull();
    }
  });

  it("parses resend-welcome responses and rejects malformed ones", () => {
    expect(parseResendWelcomeResponse({ sent: true })).toEqual({ sent: true });
    expect(parseResendWelcomeResponse(accountReq)).toMatchObject({ id: "req-1" });
    for (const body of [null, "bad", 42, [], { sent: "yes" }]) {
      expect(parseResendWelcomeResponse(body)).toBeNull();
    }
  });
});

describe("workspace monthly usage (GET /workspaces/{id}/usage)", () => {
  const validUsage = {
    workspace_id: "t1",
    period_start: "2026-11-01T00:00:00Z",
    period_end: "2026-12-01T00:00:00Z",
    rows: [
      {
        day: "2026-11-01",
        meter: "tasks_run",
        quantity: 7,
        unit_price: 30000,
        per: 1000,
        discount_pct: 20,
        net_amount: 168,
        note: "",
        order_id: null,
      },
      {
        day: "2026-11-02",
        meter: "storage_bytes",
        quantity: 1073741824,
        unit_price: 3000,
        per: 1073741824,
        discount_pct: 0,
        net_amount: 100,
        note: "workspace_suspended",
        order_id: "ord-u1",
      },
    ],
    missing_days: ["2026-11-03", "2026-11-04"],
    unpaid_orders: [
      {
        id: "ord-u1",
        amount: 50000,
        vat_amount: 5000,
        status: "pending",
        created_at: "2026-11-01T00:00:00Z",
        period_start: "2026-10-01T00:00:00Z",
        period_end: "2026-11-01T00:00:00Z",
      },
    ],
  };

  it("parses a well-formed workspace usage response", () => {
    expect(parseWorkspaceUsage(validUsage)).toEqual(validUsage);
  });

  it("parses an empty seat-plan response and nullable period bounds on unpaid orders", () => {
    const emptyUsage = {
      workspace_id: "t1",
      period_start: "2026-11-01T00:00:00Z",
      period_end: "2026-12-01T00:00:00Z",
      rows: [],
      missing_days: [],
      unpaid_orders: [
        {
          id: "ord-u2",
          amount: 12000,
          vat_amount: 1200,
          status: "pending",
          created_at: "2026-11-10T00:00:00Z",
          period_start: null,
          period_end: null,
        },
      ],
    };
    expect(parseWorkspaceUsage(emptyUsage)).toEqual(emptyUsage);
  });

  it("maps an unknown unpaid order status to unknown", () => {
    const parsed = parseWorkspaceUsage({
      ...validUsage,
      unpaid_orders: [{ ...validUsage.unpaid_orders[0], status: "in_collection" }],
    });
    expect(parsed?.unpaid_orders[0]?.status).toBe("unknown");
  });

  it("falls back to null on malformed responses", () => {
    const { missing_days: _missingDays, ...withoutMissingDays } = validUsage;
    const { rows: _rows, ...withoutRows } = validUsage;
    const { unpaid_orders: _unpaid, ...withoutUnpaidOrders } = validUsage;
    const malformedBodies = [
      null,
      undefined,
      "bad",
      [],
      {},
      withoutMissingDays,
      withoutRows,
      withoutUnpaidOrders,
      { ...validUsage, missing_days: "2026-11-03" },
      { ...validUsage, missing_days: [20261103] },
      { ...validUsage, rows: [{ ...validUsage.rows[0], quantity: "many" }] },
      { ...validUsage, rows: [{ ...validUsage.rows[0], quantity: null }] },
      { ...validUsage, rows: [{ ...validUsage.rows[0], day: 20261101 }] },
      { ...validUsage, rows: [{ ...validUsage.rows[0], day: null }] },
      { ...validUsage, rows: [{ ...validUsage.rows[0], net_amount: "168" }] },
      { ...validUsage, unpaid_orders: [{ ...validUsage.unpaid_orders[0], amount: "50000" }] },
      { ...validUsage, unpaid_orders: [{ id: "ord-u1" }] },
    ];
    for (const body of malformedBodies) {
      expect(parseWorkspaceUsage(body)).toBeNull();
    }
  });
});

describe("price changes", () => {
  const change = {
    id: "pc-1",
    plan_key: "pro",
    state: "scheduled",
    effective_from: "2026-12-20T00:00:00Z",
    old: { seat_price_month: 149000, seat_price_year: 1490000 },
    new: { seat_price_month: 199000, seat_price_year: 1990000 },
    reason: "Annual pricing update",
    notice_sent_at: null,
    created_by: "u-admin",
    created_at: "2026-11-17T00:00:00Z",
    applied_at: null,
    canceled_by: "",
    canceled_at: null,
  };

  it("parses a price-change list and maps unknown states to unknown", () => {
    const parsed = parsePriceChangeList({
      price_changes: [change, { ...change, id: "pc-2", state: "superseded" }],
    });
    expect(parsed.price_changes).toHaveLength(2);
    expect(parsed.price_changes[0]).toEqual(change);
    expect(parsed.price_changes[1]?.state).toBe("unknown");
  });

  it("parses a Change without optional new fields", () => {
    const minimal = {
      id: "pc-min",
      plan_key: "pro",
      state: "applied",
      effective_from: "2026-12-20T00:00:00Z",
      old: { seat_price_month: 149000 },
      new: { seat_price_month: 199000 },
      reason: "Adjustment",
      created_by: "u-admin",
      created_at: "2026-11-17T00:00:00Z",
    };
    const parsed = parsePriceChangeList({ price_changes: [minimal] });
    expect(parsed.price_changes[0]).toEqual({
      ...minimal,
      notice_sent_at: null,
      applied_at: null,
      canceled_by: "",
      canceled_at: null,
    });
  });

  it("falls back to an empty list on malformed price-change list bodies", () => {
    const malformed = [
      null,
      undefined,
      "bad",
      [],
      {},
      { price_changes: "not-an-array" },
      { price_changes: [{ ...change, id: undefined }] },
      { price_changes: [{ ...change, id: null }] },
      { price_changes: [{ ...change, plan_key: 123 }] },
      { price_changes: [{ ...change, state: null }] },
      { price_changes: [{ ...change, effective_from: null }] },
      { price_changes: [{ ...change, old: null }] },
      { price_changes: [{ ...change, new: "not-an-object" }] },
      { price_changes: [{ ...change, reason: null }] },
      { price_changes: [{ ...change, canceled_by: null }] },
    ];
    for (const body of malformed) {
      expect(parsePriceChangeList(body)).toEqual({ price_changes: [] });
    }
  });

  it("parses dry-run responses and falls back to null on malformed ones", () => {
    expect(
      parsePriceChangeDryRunResponse({
        dry_run: true,
        affected_subscribers: 12,
        notice_date: "2026-11-17",
      }),
    ).toEqual({
      dry_run: true,
      affected_subscribers: 12,
      notice_date: "2026-11-17",
    });

    expect(
      parsePriceChangeDryRunResponse({
        dry_run: true,
        affected_subscribers: 0,
        notice_date: null,
      }),
    ).toEqual({
      dry_run: true,
      affected_subscribers: 0,
      notice_date: null,
    });

    for (const bad of [
      null,
      "bad",
      {},
      { dry_run: "true", affected_subscribers: 1, notice_date: null },
      { dry_run: true, affected_subscribers: "1", notice_date: null },
      { dry_run: true, affected_subscribers: -1, notice_date: null },
      { dry_run: true, affected_subscribers: null, notice_date: null },
      { dry_run: true, affected_subscribers: 2, notice_date: 20261117 },
      { affected_subscribers: 2, notice_date: null },
    ]) {
      expect(parsePriceChangeDryRunResponse(bad)).toBeNull();
    }
  });

  it("parses schedule responses (including when optional notice_date is omitted) and falls back to null on malformed ones", () => {
    const full = {
      price_change: change,
      applied: false,
      affected_subscribers: 5,
      notice_date: "2026-11-17",
    };
    expect(parseSchedulePriceChangeResponse(full)).toEqual(full);

    expect(
      parseSchedulePriceChangeResponse({
        price_change: change,
        applied: true,
      }),
    ).toEqual({
      price_change: change,
      applied: true,
      affected_subscribers: 0,
      notice_date: "",
    });

    for (const bad of [
      null,
      "bad",
      {},
      { applied: false, affected_subscribers: 0, notice_date: "" },
      { price_change: null, applied: false, affected_subscribers: 0, notice_date: "" },
      { price_change: { id: "pc-1" }, applied: false },
      { price_change: change, applied: "no" },
      { price_change: change, applied: null },
      { price_change: change, applied: false, affected_subscribers: "5" },
      { price_change: change, applied: false, notice_date: null },
    ]) {
      expect(parseSchedulePriceChangeResponse(bad)).toBeNull();
    }
  });

  it("parses cancel responses and falls back to null on malformed ones", () => {
    const canceled = { ...change, state: "canceled", canceled_by: "u-admin", canceled_at: "2026-11-18T00:00:00Z" };
    expect(parseCancelPriceChangeResponse({ price_change: canceled })).toEqual({ price_change: canceled });

    for (const bad of [
      null,
      undefined,
      "bad",
      {},
      { price_change: null },
      { price_change: { id: "pc-1" } },
    ]) {
      expect(parseCancelPriceChangeResponse(bad)).toBeNull();
    }
  });
});

describe("billing profile verification", () => {
  it("parses the verify response", () => {
    expect(
      parseBillingProfileVerifyResponse({
        workspace_id: "ws-1",
        account_kind: "business",
        verified: true,
        verified_at: "2026-11-17T00:00:00Z",
        extra_field: "ignored",
      }),
    ).toEqual({
      workspace_id: "ws-1",
      account_kind: "business",
      verified: true,
      verified_at: "2026-11-17T00:00:00Z",
    });

    expect(
      parseBillingProfileVerifyResponse({
        workspace_id: "ws-1",
        account_kind: "business",
        verified: false,
        verified_at: null,
      }),
    ).toEqual({
      workspace_id: "ws-1",
      account_kind: "business",
      verified: false,
      verified_at: null,
    });

    expect(
      parseBillingProfileVerifyResponse({
        workspace_id: "ws-1",
        account_kind: "enterprise",
        verified: true,
        verified_at: "2026-11-17T00:00:00Z",
      })?.account_kind,
    ).toBe("unknown");
  });

  it("rejects a response missing verified", () => {
    for (const bad of [
      null,
      undefined,
      "bad",
      42,
      [],
      {},
      { workspace_id: "ws-1", account_kind: "business", verified_at: null },
      { workspace_id: "ws-1", account_kind: "business", verified: "yes", verified_at: null },
      { workspace_id: "ws-1", account_kind: "business", verified: null, verified_at: null },
    ]) {
      expect(parseBillingProfileVerifyResponse(bad)).toBeNull();
    }
  });
});

describe("workspace billing profile (GET /workspaces/{id}/billing-profile)", () => {
  const validBusiness = {
    workspace_id: "ws-1",
    account_kind: "business",
    legal_name: "Acme Co",
    tax_code: "0123456789",
    address: "1 Main St",
    invoice_email: "ap@acme.test",
    verified: true,
    verified_at: "2026-11-17T00:00:00Z",
  };

  const individualDefault = {
    workspace_id: "ws-2",
    account_kind: "individual",
    legal_name: "",
    tax_code: "",
    address: "",
    invoice_email: "",
    verified: false,
    verified_at: null,
  };

  it("parses a verified business profile and an unverified individual default", () => {
    expect(parseBillingProfile({ ...validBusiness, extra_field: "ignored" })).toEqual(validBusiness);
    expect(parseBillingProfile(individualDefault)).toEqual(individualDefault);
  });

  it("maps an unknown account_kind string to unknown", () => {
    expect(parseBillingProfile({ ...validBusiness, account_kind: "enterprise" })?.account_kind).toBe("unknown");
  });

  it("rejects a response with a bad account_kind or malformed fields", () => {
    for (const bad of [
      null,
      undefined,
      "bad",
      42,
      [],
      {},
      { ...validBusiness, account_kind: 123 },
      { ...validBusiness, account_kind: null },
      { ...validBusiness, account_kind: ["business"] },
      { ...validBusiness, verified: undefined },
      { ...validBusiness, verified: "yes" },
      { ...validBusiness, verified: null },
    ]) {
      expect(parseBillingProfile(bad)).toBeNull();
    }
  });
});

describe("put billing profile response (PUT /workspaces/{id}/billing-profile)", () => {
  const validPut = {
    workspace_id: "ws-1",
    account_kind: "business",
    legal_name: "Acme Co",
    tax_code: "0123456789",
    address: "1 Main St",
    invoice_email: "ap@acme.test",
    verified: false,
    verified_at: null,
    changed: true,
  };

  it("parses a well-formed PUT response and strips unknown keys", () => {
    expect(parsePutBillingProfileResponse({ ...validPut, extra_field: "ignored" })).toEqual(validPut);
    expect(
      parsePutBillingProfileResponse({
        ...validPut,
        verified: true,
        verified_at: "2026-11-17T00:00:00Z",
        changed: false,
      }),
    ).toEqual({
      ...validPut,
      verified: true,
      verified_at: "2026-11-17T00:00:00Z",
      changed: false,
    });
  });

  it("defaults optional fields and maps unknown account_kind to unknown", () => {
    expect(
      parsePutBillingProfileResponse({
        workspace_id: "ws-1",
        account_kind: "enterprise",
        verified: false,
      }),
    ).toEqual({
      workspace_id: "ws-1",
      account_kind: "unknown",
      legal_name: "",
      tax_code: "",
      address: "",
      invoice_email: "",
      verified: false,
      verified_at: null,
      changed: false,
    });
  });

  it("falls back to null on malformed PUT responses", () => {
    for (const bad of [
      null,
      undefined,
      "bad",
      42,
      [],
      {},
      { ...validPut, account_kind: 123 },
      { ...validPut, account_kind: null },
      { ...validPut, verified: undefined },
      { ...validPut, verified: "no" },
      { ...validPut, verified: null },
      { ...validPut, changed: "yes" },
      { ...validPut, changed: null },
    ]) {
      expect(parsePutBillingProfileResponse(bad)).toBeNull();
    }
  });
});

describe("payment notices", () => {
  const notice = {
    id: "11111111-1111-4111-8111-111111111111",
    provider: "sepay",
    provider_event_id: "evt-123",
    state: "needs_review",
    transfer_code: "MUL12345678",
    amount: 660000,
    currency: "vnd",
    review_reason: "ambiguous_code",
    order_id: "",
    received_at: "2026-11-17T10:00:00Z",
    decided_at: null,
  };
  const candidate = {
    order_id: "22222222-2222-4222-8222-222222222222",
    workspace_id: "33333333-3333-4333-8333-333333333333",
    state: "pending",
    transfer_code: "MUL12345678",
    amount: 600000,
    vat_amount: 60000,
    currency: "vnd",
    created_at: "2026-11-13T10:00:00Z",
    match: "code",
  };

  it("parses a well-formed notice list", () => {
    const parsed = parseNoticeList({ notices: [notice] });
    expect(parsed.notices).toHaveLength(1);
    expect(parsed.notices[0]?.transfer_code).toBe("MUL12345678");
  });

  it("falls back to an empty list for a malformed body", () => {
    for (const body of [null, undefined, "invalid", 123, { notices: "not-an-array" }]) {
      expect(parseNoticeList(body)).toEqual({ notices: [] });
    }
  });

  it("parses a detail with candidates and maps an unknown state to unknown", () => {
    const parsed = parseNoticeDetail({ ...notice, state: "some_future_state", candidates: [candidate] });
    expect(parsed?.state).toBe("unknown");
    expect(parsed?.candidates).toHaveLength(1);
    expect(parsed?.candidates[0]?.match).toBe("code");
  });

  it("falls back to null for a malformed detail", () => {
    for (const body of [null, undefined, "invalid", {}, { ...notice, id: 5 }, { ...notice, candidates: "x" }]) {
      expect(parseNoticeDetail(body)).toBeNull();
    }
  });

  it("parses a resolve or dismiss answer and falls back to null when malformed", () => {
    expect(parseNotice(notice, "POST /api/platform/payment-notices/{id}/dismiss")?.id).toBe(notice.id);
    for (const body of [null, undefined, "invalid", {}, { id: 1 }]) {
      expect(parseNotice(body, "POST /api/platform/payment-notices/{id}/resolve")).toBeNull();
    }
  });
});

describe("e-invoices (GET /api/platform/einvoices)", () => {
  const item = {
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
    last_error_code: "provider_timeout",
    missing_fields: ["tax_code", "address"],
    next_attempt_at: "2026-11-18T12:00:00Z",
    updated_at: "2026-11-18T11:00:00Z",
  };

  it("parses a valid e-invoice list with all known states and strips non-https lookup URLs", () => {
    expect(EINVOICE_STATES).toEqual(["pending", "issued", "needs_info", "failed", "cancelled"]);
    const parsed = parseEinvoiceList({
      einvoices: [
        item,
        {
          ...item,
          order_id: "ord-2",
          state: "issued",
          invoice_number: "00001234",
          lookup_url: "https://einvoice.sepay.vn/lookup/abc",
          missing_fields: [],
        },
        {
          ...item,
          order_id: "ord-3",
          state: "issued",
          lookup_url: "http://insecure.example.com/inv/3",
        },
      ],
    });
    expect(parsed.einvoices).toHaveLength(3);
    expect(parsed.einvoices[0]).toEqual(item);
    expect(parsed.einvoices[1]?.lookup_url).toBe("https://einvoice.sepay.vn/lookup/abc");
    expect(parsed.einvoices[2]?.lookup_url).toBe("");
  });

  it("defaults optional fields and maps unknown state to unknown", () => {
    const parsed = parseEinvoiceList({
      einvoices: [{ order_id: "ord-4", state: "future_state" }],
    });
    expect(parsed.einvoices[0]).toEqual({
      order_id: "ord-4",
      workspace_id: "",
      revision: 1,
      state: "unknown",
      currency: "",
      amount: 0,
      vat_amount: 0,
      provider: "",
      invoice_number: "",
      lookup_url: "",
      attempts: 0,
      last_error_code: "",
      missing_fields: [],
      next_attempt_at: "",
      updated_at: "",
    });
  });

  it("falls back to an empty list when the payload is malformed", () => {
    for (const bad of [
      null,
      undefined,
      "bad",
      123,
      { einvoices: "not-an-array" },
      { einvoices: [{ state: "failed" }] },
      { einvoices: [{ order_id: 42, state: "failed" }] },
    ]) {
      expect(parseEinvoiceList(bad)).toEqual({ einvoices: [] });
    }
  });
});

describe("order e-invoice history (GET /api/platform/orders/{id}/einvoices)", () => {
  const historyItem = {
    revision: 2,
    state: "issued",
    invoice_number: "00001234",
    lookup_url: "https://einvoice.sepay.vn/lookup/abc",
    buyer: {
      name: "Acme Vietnam Ltd",
      tax_code: "0101234567",
      address: "123 Le Loi, HCMC",
      invoice_email: "billing@acme.vn",
    },
    attempts: 1,
    last_error_code: "",
    created_at: "2026-11-18T10:00:00Z",
    issued_at: "2026-11-18T10:05:00Z",
    updated_at: "2026-11-18T10:05:00Z",
  };

  it("accepts an empty einvoices list", () => {
    expect(OrderEinvoiceListSchema.safeParse({ einvoices: [] })).toEqual({
      success: true,
      data: { einvoices: [] },
    });
    expect(parseOrderEinvoiceList({ einvoices: [] })).toEqual({ einvoices: [] });
  });

  it("parses revisions with exact fields, strips non-https lookup URLs, and maps unknown state to unknown", () => {
    const parsed = parseOrderEinvoiceList({
      einvoices: [
        historyItem,
        {
          ...historyItem,
          revision: 1,
          state: "cancelled",
          invoice_number: "",
          lookup_url: "http://insecure.example.com/inv/1",
          attempts: 3,
          last_error_code: "provider_timeout",
          issued_at: null,
        },
      ],
    });
    expect(parsed?.einvoices).toHaveLength(2);
    expect(parsed?.einvoices[0]).toEqual(historyItem);
    expect(parsed?.einvoices[1]).toEqual({
      ...historyItem,
      revision: 1,
      state: "unknown",
      invoice_number: "",
      lookup_url: "",
      attempts: 3,
      last_error_code: "provider_timeout",
      issued_at: null,
    });
  });

  it("rejects wrong-shape history responses", () => {
    for (const bad of [
      null,
      undefined,
      "bad",
      123,
      [],
      {},
      { einvoices: "not-an-array" },
      { einvoices: [{ ...historyItem, revision: undefined }] },
      { einvoices: [{ state: "issued" }] },
      { einvoices: [{ ...historyItem, revision: "2" }] },
      { einvoices: [{ ...historyItem, revision: null }] },
      { einvoices: [{ ...historyItem, state: 42 }] },
      { einvoices: [{ ...historyItem, buyer: "not-an-object" }] },
    ]) {
      expect(OrderEinvoiceListSchema.safeParse(bad).success).toBe(false);
      expect(parseOrderEinvoiceList(bad)).toBeNull();
    }
  });
});

describe("member requests and deletion requests", () => {
  const memberReq = {
    id: "mr-1",
    workspace_id: "ws-1",
    kind: "add",
    email: "new@example.com",
    role: "member",
    state: "pending",
    error_code: "",
    attempts: 0,
    user_id: "",
    member_id: "",
    user_created: false,
    requested_by: "u-admin",
    requested_by_label: "admin@example.com",
    created_at: "2026-11-19T10:00:00Z",
    updated_at: "2026-11-19T10:00:00Z",
    completed_at: null,
  };

  const deletionReq = {
    id: "dr-1",
    kind: "workspace",
    target_id: "ws-1",
    target_label: "acme",
    reason: "Customer requested",
    state: "failed",
    error_code: "storage_cleanup_failed",
    attempts: 3,
    tenant_id: "t-1",
    requested_by: "u-admin",
    requested_by_label: "admin@example.com",
    created_at: "2026-11-19T10:00:00Z",
    updated_at: "2026-11-19T10:15:00Z",
  };

  it("parses member request lists and single items", () => {
    expect(parseMemberRequestList({ requests: [memberReq] }).requests).toEqual([memberReq]);
    expect(parseMemberRequest(memberReq, "POST /api/platform/workspaces/{id}/members")).toEqual(memberReq);
    expect(parseMemberRequestList({ requests: "nope" })).toEqual({ requests: [] });
    expect(parseMemberRequest(null, "GET /api/platform/member-requests/{id}")).toBeNull();
  });

  it("unknown request state and error_code fall back", () => {
    const memberParsed = parseMemberRequest(
      { id: "mr-2", workspace_id: "ws-1", state: "retrying_later", error_code: null },
      "GET /api/platform/member-requests/{id}",
    );
    expect(memberParsed?.state).toBe("unknown");
    expect(memberParsed?.error_code).toBe("");

    const deletionParsed = parseDeletionRequest(
      { id: "dr-2", kind: "future_kind", state: "awaiting_something", error_code: null },
      "GET /api/platform/deletion-requests/{id}",
    );
    expect(deletionParsed?.kind).toBe("unknown");
    expect(deletionParsed?.state).toBe("unknown");
    expect(deletionParsed?.error_code).toBe("");

    const listParsed = parseDeletionRequestList({
      requests: [
        deletionReq,
        { id: "dr-3", state: "future_state", error_code: "new_server_code" },
      ],
    });
    expect(listParsed.requests).toHaveLength(2);
    expect(listParsed.requests[1]?.state).toBe("unknown");
    expect(listParsed.requests[1]?.error_code).toBe("new_server_code");
    expect(listParsed.requests[1]?.tenant_id).toBeNull();
  });

  it("parses deletion start response and account delete preview", () => {
    expect(parseDeletionStartResponse({ request_id: "dr-1", state: "pending" }, "POST /workspaces/{id}/delete-now")).toEqual({
      request_id: "dr-1",
      state: "pending",
    });
    expect(parseDeletionStartResponse({ request_id: "dr-2", state: "future" }, "POST /tenants/{id}/delete-now")?.state).toBe("unknown");
    expect(parseDeletionStartResponse({}, "POST /tenants/{id}/delete-now")).toBeNull();

    const preview = parseAccountDeletePreview({
      email: "leaver@example.com",
      need_choice: [
        { workspace_id: "ws-1", slug: "acme", role: "owner", owner_count: 1, member_count: 3 },
      ],
      deleted_with: [],
    });
    expect(preview).toEqual({
      email: "leaver@example.com",
      need_choice: [
        {
          workspace_id: "ws-1",
          slug: "acme",
          role: "owner",
          owner_count: 1,
          member_count: 3,
          successors: [],
        },
      ],
      deleted_with: [],
    });
    expect(parseAccountDeletePreview({ need_choice: "bad" })).toBeNull();
  });
});

describe("R6 S23 plan lifecycle, history and workspace list schemas", () => {
  it("unknown plan state and workspace status fall back", () => {
    const plans = parsePlanList({
      plans: [
        { key: "pro", name: "Pro", state: "sunset_soon", description: "For teams" },
        { key: "legacy", name: "Legacy", archived_at: "2026-01-01T00:00:00Z" },
      ],
    });
    expect(plans.plans[0]?.state).toBe("unknown");
    expect(plans.plans[0]?.description).toBe("For teams");
    expect(plans.plans[1]?.description).toBe("");
    expect(plans.plans[1]?.state).toBeUndefined();

    const singlePlan = parsePlanDetail({
      plan: { key: "pro", name: "Pro", state: "future_state" },
    });
    expect(singlePlan?.plan.state).toBe("unknown");
    expect(parsePlanDetail(null)).toBeNull();

    const wsPage = parseWorkspacePage({
      workspaces: [
        {
          id: "ws-1",
          name: "Acme",
          slug: "acme",
          tenant_id: "t-1",
          tenant_name: "Acme Corp",
          status: "frozen_by_future_server",
          plan_key: "pro",
          member_count: 5,
          created_at: "2026-11-01T00:00:00Z",
        },
        { id: "ws-2" },
      ],
      next_cursor: "cur-2",
    });
    expect(wsPage.workspaces).toHaveLength(2);
    expect(wsPage.workspaces[0]?.status).toBe("unknown");
    expect(wsPage.workspaces[1]?.status).toBe("unknown");
    expect(wsPage.workspaces[1]?.plan_key).toBe("");
    expect(wsPage.next_cursor).toBe("cur-2");
    expect(parseWorkspacePage({ workspaces: "bad" })).toEqual({ workspaces: [], next_cursor: null });

    const detail = parseWorkspaceDetail({
      ...workspace,
      tenant: { id: "t-1", name: "Acme Corp" },
      seats: { purchased: 10, used: 4, held: 1 },
      allowed: ["suspend", "export"],
      status_history: [
        {
          id: "sh-1",
          status: "future_ws_state",
          reason: "Test",
          actor: "staff@example.com",
          created_at: "2026-11-18T00:00:00Z",
        },
      ],
    });
    expect(detail?.tenant).toEqual({ id: "t-1", name: "Acme Corp" });
    expect(detail?.seats).toEqual({ purchased: 10, used: 4, held: 1 });
    expect(detail?.allowed).toEqual(["suspend", "export"]);
    expect(detail?.status_history?.[0]?.status).toBe("unknown");
  });

  it("parses plan history and workspace export response", () => {
    const history = parsePlanHistory({
      history: [
        {
          at: "2026-11-19T09:00:00Z",
          actor: "root@example.test",
          action: "plan.changed",
          changes: [{ field: "name", old: "Pro", new: "Pro Plus" }],
          reason: "rename",
        },
        {
          at: "2026-11-18T09:00:00Z",
          actor: "root@example.test",
          action: "plan.activated",
        },
      ],
    });
    expect(history.history).toHaveLength(2);
    expect(history.history[0]?.changes).toEqual([{ field: "name", old: "Pro", new: "Pro Plus" }]);
    expect(history.history[1]?.changes).toEqual([]);
    expect(history.history[1]?.reason).toBe("");
    expect(parsePlanHistory({ history: "bad" })).toEqual({ history: [] });

    expect(parseWorkspaceExportResponse({ export_id: "exp-1", status: "queued" })).toEqual({
      export_id: "exp-1",
      status: "queued",
    });
    expect(parseWorkspaceExportResponse({})).toBeNull();
  });
});

describe("R6 S24 tenant users, member changes and quota requests", () => {
  it("tenant user row falls back for unknown account_status", () => {
    const page = parseTenantUserPage({
      users: [
        {
          user_id: "u-1",
          email: "alice@acme.test",
          name: "Alice",
          account_status: "future_locked_state",
          is_staff: false,
          workspaces: [
            { workspace_id: "ws-1", name: "Core", member_role: "member" },
            { workspace_id: "ws-2", name: "Lab", member_role: "super_owner" },
          ],
          seats: 2,
          last_login_at: "2026-11-18T10:00:00Z",
        },
        {
          user_id: "u-2",
        },
      ],
      next_cursor: "cur-u2",
    });

    expect(page.users).toHaveLength(2);
    expect(page.next_cursor).toBe("cur-u2");
    expect(page.users[0]?.account_status).toBe("unknown");
    expect(page.users[0]?.workspaces[0]?.member_role).toBe("member");
    expect(page.users[0]?.workspaces[1]?.member_role).toBe("unknown");
    expect(page.users[1]).toEqual({
      user_id: "u-2",
      email: "",
      name: "",
      account_status: "unknown",
      is_staff: false,
      workspaces: [],
      seats: 0,
      last_login_at: null,
    });
    expect(parseTenantUserPage({ users: "bad" })).toEqual({ users: [], next_cursor: null });
  });

  it("parses tenant user detail and defaults missing allocation and audit_events", () => {
    const minimal = parseTenantUserDetail({
      user_id: "u-1",
      email: "alice@acme.test",
      name: "Alice",
      account_status: "active",
      is_staff: false,
      workspaces: [{ workspace_id: "ws-1", name: "Core", member_role: "admin" }],
      seats: 1,
      last_login_at: null,
    });
    expect(minimal?.allocation).toEqual([]);
    expect(minimal?.audit_events).toEqual([]);

    const withAllocation = parseTenantUserDetail({
      user_id: "u-1",
      account_status: "active",
      allocation: [
        {
          workspace_id: "ws-1",
          workspace_name: "Core",
          member_role: "future_role",
          occupies_seat: true,
          seats_purchased: 10,
          seats_used: 4,
          limits: { agents: 5 },
          usage: { agents: 2 },
        },
      ],
      audit_events: [
        {
          id: "ae-1",
          action: "member.staff_role_change_requested",
          actor_type: "staff",
          created_at: "2026-11-19T00:00:00Z",
        },
      ],
    });
    expect(withAllocation?.allocation[0]?.member_role).toBe("unknown");
    expect(withAllocation?.allocation[0]?.limits).toEqual({ agents: 5 });
    expect(withAllocation?.audit_events).toHaveLength(1);
    expect(parseTenantUserDetail(null)).toBeNull();
  });

  it("parses tenant deactivate, member change and quota request responses", () => {
    expect(parseDeactivateTenantUserResponse({ request_ids: ["mr-1", "mr-2"] })).toEqual({
      request_ids: ["mr-1", "mr-2"],
    });
    expect(parseDeactivateTenantUserResponse(null)).toBeNull();

    expect(
      parseMemberChangeResponse(
        { request_id: "mr-3", kind: "change_role" },
        "POST /api/platform/workspaces/{id}/members/{user_id}/role",
      ),
    ).toEqual({ request_id: "mr-3", kind: "change_role" });
    expect(
      parseMemberChangeResponse(
        { changed: false },
        "POST /api/platform/workspaces/{id}/members/{user_id}/role",
      ),
    ).toEqual({ changed: false });
    expect(
      parseMemberChangeResponse(null, "POST /api/platform/workspaces/{id}/members/{user_id}/remove"),
    ).toBeNull();

    const qr = parseQuotaRequest(
      {
        id: "qr-1",
        workspace_id: "ws-1",
        tenant_id: "t-1",
        gate: "agents",
        current_value: 5,
        requested_value: 20,
        approved_value: null,
        state: "future_state",
        reason: "Growing team",
        created_at: "2026-11-19T12:00:00Z",
      },
      "POST /api/platform/workspaces/{id}/quota-requests",
    );
    expect(qr?.id).toBe("qr-1");
    expect(qr?.state).toBe("unknown");
    expect(parseQuotaRequest({}, "POST /api/platform/workspaces/{id}/quota-requests")).toBeNull();
  });
});

describe("R6 S25 resources, limit overrides and runtimes schemas", () => {
  it("parses tenant resources with defaults and unknown state fallback", () => {
    const parsed = parseTenantResources({
      tenant_id: "t-1",
      complete: true,
      resources: [
        { type: "agents", used: 8, limit: 10, percent: 80, state: "warning" },
        { type: "runtimes", used: 3, limit: null, state: "future_state" },
        {},
      ],
      workspaces: [
        {
          workspace_id: "ws-1",
          workspace_name: "Core",
          plan_key: "pro",
          resources: [{ type: "agents", used: 8, limit: 10, percent: 80, status: "over" }],
        },
      ],
    });
    expect(parsed?.complete).toBe(true);
    expect(parsed?.resources[0]).toMatchObject({ type: "agents", used: 8, limit: 10, percent: 80, state: "warning" });
    expect(parsed?.resources[1]).toMatchObject({ type: "runtimes", used: 3, limit: null, percent: null, state: "unknown" });
    expect(parsed?.resources[2]).toMatchObject({ type: "", used: 0, limit: null, percent: null, state: "unknown" });
    expect(parsed?.workspaces[0]?.resources[0]?.state).toBe("over");

    expect(parseTenantResources({})).toEqual({
      tenant_id: "",
      resources: [],
      workspaces: [],
      complete: false,
    });
    expect(parseTenantResources(null)).toBeNull();
  });

  it("parses system resources with top tenants, over_limit, cursor, complete and malformed fallback", () => {
    const parsed = parseSystemResources({
      by_type: [{ type: "agents", used: 10, limit: 20, percent: 50 }],
      by_plan: [{ plan_key: "pro", tenant_count: 3, workspace_count: 5, totals: { agents: 10 } }],
      top_tenants: [{ tenant_id: "t-1", tenant_name: "Acme", agents: 10, state: "warning" }],
      over_limit: [
        {
          tenant_id: "t-1",
          tenant_name: "Acme",
          workspace_id: "ws-1",
          workspace_name: "Core",
          type: "agents",
          used: 12,
          limit: 10,
          percent: 120,
          state: "over",
        },
      ],
      next_cursor: "cur-2",
      complete: false,
    });
    expect(parsed.top_tenants).toHaveLength(1);
    expect(parsed.top_tenants[0]?.state).toBe("warning");
    expect(parsed.over_limit[0]?.state).toBe("over");
    expect(parsed.next_cursor).toBe("cur-2");
    expect(parsed.complete).toBe(false);

    expect(parseSystemResources(null)).toEqual({
      by_type: [],
      by_plan: [],
      top_tenants: [],
      tenants: [],
      over_limit: [],
      next_cursor: null,
      complete: false,
    });
  });

  it("parses limit override response", () => {
    expect(
      parseTenantLimitOverrideResponse({ gate: "agents", workspace_ids: ["ws-1", "ws-2"] }),
    ).toEqual({ gate: "agents", workspace_ids: ["ws-1", "ws-2"] });
    expect(parseTenantLimitOverrideResponse({})).toBeNull();
  });

  it("parses runtimes page, strips raw device_info/metadata/ip, and falls back for unknown status", () => {
    const page = parseRuntimePage({
      runtimes: [
        {
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
          ip: "10.0.0.1",
          device_info: "secret host",
          metadata: { host: "secret" },
        },
        { id: "rt-2", status: "rebooting" },
      ],
      next_cursor: "cur-rt",
    });
    expect(page.runtimes).toHaveLength(2);
    expect(page.runtimes[0]).not.toHaveProperty("ip");
    expect(page.runtimes[0]).not.toHaveProperty("device_info");
    expect(page.runtimes[0]).not.toHaveProperty("metadata");
    expect(page.runtimes[1]?.status).toBe("unknown");
    expect(page.next_cursor).toBe("cur-rt");

    expect(parseRuntimePage("bad")).toEqual({ runtimes: [], next_cursor: null });

    const detail = parseRuntimeDetail({
      id: "rt-1",
      name: "laptop",
      status: "offline",
      disabled: true,
      failed_tasks_7d: 3,
      tasks: [{ id: "task-1", status: "failed", has_error: true, error: "secret stack" }],
      audit_events: null,
    });
    expect(detail?.disabled).toBe(true);
    expect(detail?.failed_tasks_7d).toBe(3);
    expect(detail?.tasks[0]).not.toHaveProperty("error");
    expect(detail?.audit_events).toEqual([]);
    expect(parseRuntimeDetail(null)).toBeNull();

    expect(parseSetRuntimeStatusResponse({ changed: true, disabled: true })).toEqual({
      changed: true,
      disabled: true,
    });
    expect(
      parseRuntimeUnregisterResponse({ request_id: "dr-1", state: "pending" }),
    ).toEqual({ request_id: "dr-1", state: "pending" });
  });
});
