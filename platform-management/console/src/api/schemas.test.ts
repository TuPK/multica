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
