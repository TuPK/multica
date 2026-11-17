// @vitest-environment node
import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import {
  AUDIT_ACTIONS,
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
  parsePutSubscriptionResponse,
  parsePlanList,
  parsePutPlanResponse,
  parseDeletePlanResponse,
  parseVerifyCodeResponse,
  parseResetMfaResponse,
  parseAccountRequestList,
  parseAccountRequest,
  parseResendWelcomeResponse,
  parseTenantUsage,
} from "./schemas";

const tenant = {
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
    expect(parseMe({ user_id: "u1", email: "a@example.com", role: "admin" })).toEqual({
      user_id: "u1",
      email: "a@example.com",
      role: "admin",
    });
  });

  it("falls back to null for a malformed body", () => {
    for (const body of [null, "x", [], {}, { user_id: 7 }]) expect(parseMe(body)).toBeNull();
  });

  it("maps an unknown role to unknown and defaults a missing role and email", () => {
    expect(parseMe({ user_id: "u1", role: "auditor" })?.role).toBe("unknown");
    expect(parseMe({ user_id: "u1" })).toEqual({ user_id: "u1", email: "", role: "unknown" });
  });
});

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
      slug: "",
      status: "unknown",
      member_count: 0,
      created_at: "",
      last_activity_at: "",
    });
    expect(page.next_cursor).toBeNull();
  });

  it("maps an unknown status to unknown", () => {
    expect(parseTenantPage({ tenants: [{ ...tenant, status: "archived" }] }).tenants[0]?.status).toBe("unknown");
  });

  it("parses a detail with its state and owner", () => {
    const detail = parseTenantDetail({
      ...tenant,
      state: { status: "suspended", reason: "abuse", changed_by: "u9", changed_at: "2026-02-02T00:00:00Z", deletion_scheduled_at: null },
      notes_count: 2,
      owner: { id: "u1", email: "a@example.com" },
    });
    expect(detail?.state.status).toBe("suspended");
    expect(detail?.state.reason).toBe("abuse");
    expect(detail?.owner).toEqual({ id: "u1", email: "a@example.com" });
    expect(detail?.notes_count).toBe(2);
  });

  it("defaults a missing state, owner and notes count in a detail", () => {
    const detail = parseTenantDetail(tenant);
    expect(detail?.state).toEqual({
      status: "unknown",
      reason: "",
      changed_by: null,
      changed_at: null,
      deletion_scheduled_at: null,
    });
    expect(detail?.owner).toBeNull();
    expect(detail?.notes_count).toBe(0);
  });

  it("falls back to null for a malformed detail", () => {
    for (const body of [null, "x", {}, { id: 7 }]) expect(parseTenantDetail(body)).toBeNull();
  });
});

describe("tenant usage", () => {
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
    expect(parseTenantPage({ tenants: [{ ...tenant, ...usage }] }).tenants[0]).toEqual({ ...tenant, ...usage });
  });

  it("accepts a workspace with no row: a null day and zeros", () => {
    const parsed = parseTenantPage({ tenants: [{ ...tenant, ...noRow }] }).tenants[0];
    expect(parsed?.usage_day).toBeNull();
    expect(parsed?.usage_storage_bytes).toBe(0);
  });

  it("accepts a tenant that carries no usage fields at all", () => {
    const parsed = parseTenantPage({ tenants: [tenant] }).tenants[0];
    expect(parsed).toEqual(tenant);
    expect(parsed?.usage_day).toBeUndefined();
  });

  it("refuses a page whose usage fields are malformed", () => {
    const empty = { tenants: [], next_cursor: null };
    for (const bad of [{ usage_members: "many" }, { usage_storage_bytes: null }, { usage_day: 20261114 }]) {
      expect(parseTenantPage({ tenants: [{ ...tenant, ...bad }] })).toEqual(empty);
    }
  });

  it("parses the current counts of a detail and leaves a missing block out", () => {
    const current = { members: 4, agents: 2, runtimes: 1, issues_total: 41, storage_bytes: 4096 };
    expect(parseTenantDetail({ ...tenant, ...usage, usage_current: current })?.usage_current).toEqual(current);
    expect(parseTenantDetail({ ...tenant, usage_current: { members: 4 } })?.usage_current).toEqual({
      members: 4,
      agents: 0,
      runtimes: 0,
      issues_total: 0,
      storage_bytes: 0,
    });
    expect(parseTenantDetail(tenant)?.usage_current).toBeUndefined();
    expect(parseTenantDetail({ ...tenant, usage_current: { members: "x" } })).toBeNull();
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
  const member = { user_id: "u1", email: "a@example.com", role: "admin", created_by: null, created_at: "2026-01-01T00:00:00Z" };

  it("parses the list", () => {
    expect(parseStaffList({ staff: [member] })).toEqual({ staff: [member] });
  });

  it("maps an unknown role to unknown", () => {
    expect(parseStaffList({ staff: [{ ...member, role: "owner" }] }).staff[0]?.role).toBe("unknown");
  });

  it("falls back to an empty list for a malformed body", () => {
    for (const body of [null, [], { staff: [{ email: "no id" }] }, { staff: "x" }]) {
      expect(parseStaffList(body)).toEqual({ staff: [] });
    }
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

  it("parses a well-formed orders list", () => {
    const parsed = parseOrderList({ orders: [order] });
    expect(parsed.orders).toHaveLength(1);
    expect(parsed.orders[0]?.id).toBe("ord-1");
    expect(parsed.orders[0]?.transfer_code).toBe("MUL12345678");
    expect(parsed.orders[0]?.status).toBe("pending");
  });

  it("falls back to empty list on malformed orders body", () => {
    for (const body of [null, undefined, "invalid", 123, { orders: "not-an-array" }]) {
      expect(parseOrderList(body)).toEqual({ orders: [] });
    }
  });

  it("parses a well-formed order action result", () => {
    const parsed = parseOrderActionResult({ order, applied: true }, "POST /orders/{id}/confirm");
    expect(parsed).toEqual({ order, applied: true });
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
  };

  it("parses a well-formed plan list", () => {
    const parsed = parsePlanList({ plans: [plan] });
    expect(parsed.plans).toHaveLength(1);
    expect(parsed.plans[0]?.key).toBe("pro");
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
    });
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

describe("tenant monthly usage (GET /tenants/{id}/usage)", () => {
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

  it("parses a well-formed tenant usage response", () => {
    expect(parseTenantUsage(validUsage)).toEqual(validUsage);
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
    expect(parseTenantUsage(emptyUsage)).toEqual(emptyUsage);
  });

  it("maps an unknown unpaid order status to unknown", () => {
    const parsed = parseTenantUsage({
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
      expect(parseTenantUsage(body)).toBeNull();
    }
  });
});
