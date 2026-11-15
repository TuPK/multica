import { describe, expect, it, vi, beforeEach } from "vitest";

const listWorkspaceAuditEvents = vi.fn();
const listMySecurityEvents = vi.fn();
vi.mock("../api", () => ({
  api: {
    listWorkspaceAuditEvents: (...a: unknown[]) => listWorkspaceAuditEvents(...a),
    listMySecurityEvents: (...a: unknown[]) => listMySecurityEvents(...a),
  },
}));

import {
  AUDIT_PAGE_SIZE,
  SECURITY_EVENTS_PAGE_SIZE,
  auditEventsOptions,
  auditKeys,
  securityEventsOptions,
} from "./queries";

beforeEach(() => {
  listWorkspaceAuditEvents.mockReset();
  listMySecurityEvents.mockReset();
});

describe("audit query keys", () => {
  it("scopes the workspace list to the workspace and its filters", () => {
    expect(auditKeys.list("w1", {})).not.toEqual(auditKeys.list("w2", {}));
    expect(auditKeys.list("w1", { action: "pat.created" })).not.toEqual(auditKeys.list("w1", {}));
    expect(auditKeys.list("w1", { from: "a" })).not.toEqual(auditKeys.list("w1", { to: "a" }));
  });

  it("is stable for equal filters whatever the property order", () => {
    expect(auditKeys.list("w1", { action: "x", to: "t" })).toEqual(
      auditKeys.list("w1", { to: "t", action: "x" }),
    );
  });

  it("sits under the workspace key so a workspace invalidation covers it", () => {
    expect(auditKeys.list("w1", {}).slice(0, 3)).toEqual(auditKeys.all("w1"));
    expect(auditKeys.all("w1").slice(0, 2)).toEqual(["workspaces", "w1"]);
  });

  it("keys security events by user, with no workspace", () => {
    expect(auditKeys.securityEvents("u1", 20)).not.toEqual(auditKeys.securityEvents("u2", 20));
    expect(auditKeys.securityEvents("u1", 20)).not.toContain("workspaces");
  });
});

describe("auditEventsOptions", () => {
  it("starts without a cursor and follows next_cursor", async () => {
    const opts = auditEventsOptions("w1", { action: "pat.created" });
    listWorkspaceAuditEvents.mockResolvedValue({ events: [], next_cursor: "c2" });

    await (opts.queryFn as (ctx: unknown) => Promise<unknown>)({ pageParam: null });
    expect(listWorkspaceAuditEvents).toHaveBeenCalledWith(
      "w1",
      { action: "pat.created" },
      { limit: AUDIT_PAGE_SIZE, cursor: null },
    );

    expect(opts.initialPageParam).toBeNull();
    const next = opts.getNextPageParam;
    expect(next({ events: [], next_cursor: "c2" }, [], null, [])).toBe("c2");
    expect(next({ events: [], next_cursor: null }, [], null, [])).toBeUndefined();
  });

  it("is disabled without a workspace", () => {
    expect(auditEventsOptions("").enabled).toBe(false);
  });
});

describe("securityEventsOptions", () => {
  it("asks for the most recent 20 by default", async () => {
    const opts = securityEventsOptions("u1");
    listMySecurityEvents.mockResolvedValue({ events: [], next_cursor: null });
    await (opts.queryFn as (ctx: unknown) => Promise<unknown>)({});
    expect(listMySecurityEvents).toHaveBeenCalledWith({ limit: SECURITY_EVENTS_PAGE_SIZE });
    expect(SECURITY_EVENTS_PAGE_SIZE).toBe(20);
  });

  it("is disabled without a user", () => {
    expect(securityEventsOptions("").enabled).toBe(false);
  });
});
