import { describe, expect, it } from "vitest";
import { parseWithFallback } from "../api/schema";
import {
  AuditEventPageSchema,
  EMPTY_AUDIT_EVENT_PAGE,
  type AuditEventPage,
} from "./schema";

function parse(raw: unknown): AuditEventPage {
  return parseWithFallback<AuditEventPage>(raw, AuditEventPageSchema, EMPTY_AUDIT_EVENT_PAGE, {
    endpoint: "GET /api/workspaces/{id}/audit-events",
  });
}

const event = {
  id: "e1",
  workspace_id: "w1",
  actor_type: "member",
  actor_id: "u1",
  actor_label: "Ada",
  action: "member.role_changed",
  target_type: "member",
  target_id: "m1",
  target_label: "Bob",
  detail: { from: "member", to: "admin" },
  ip: "203.0.113.5",
  user_agent: "UA",
  request_id: "r1",
  created_at: "2026-01-02T03:04:05Z",
};

describe("AuditEventPageSchema", () => {
  it("reads a full page", () => {
    expect(parse({ events: [event], next_cursor: "abc" })).toEqual({
      events: [event],
      next_cursor: "abc",
    });
  });

  it("keeps an action it does not know", () => {
    const page = parse({ events: [{ ...event, action: "billing.plan_changed" }], next_cursor: null });
    expect(page.events[0]?.action).toBe("billing.plan_changed");
  });

  it("defaults missing optional fields", () => {
    const page = parse({ events: [{ id: "e2", action: "auth.login", created_at: "2026-01-01T00:00:00Z" }] });
    expect(page.next_cursor).toBeNull();
    expect(page.events[0]).toMatchObject({
      workspace_id: null,
      actor_id: null,
      actor_label: "",
      target_label: "",
      detail: {},
      ip: "",
    });
  });

  it("keeps fields a newer server adds", () => {
    expect(parse({ events: [{ ...event, region: "eu" }], next_cursor: null }).events[0]).toMatchObject({
      region: "eu",
    });
  });

  it.each([
    null,
    undefined,
    "ok",
    42,
    [],
    { events: "none" },
    { events: [{ action: 3 }] },
    { events: [{ id: "x" }] },
    { events: [], next_cursor: 7 },
  ])("falls back to an empty last page for a malformed body: %j", (raw) => {
    expect(parse(raw)).toEqual(EMPTY_AUDIT_EVENT_PAGE);
  });

  it("offers no cursor for an unreadable body", () => {
    expect(parse({ events: 5, next_cursor: "abc" }).next_cursor).toBeNull();
  });
});
