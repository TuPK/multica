import { describe, expect, it } from "vitest";
import { parseWithFallback } from "../api/schema";
import {
  MemberGroupDetailSchema,
  MemberGroupListSchema,
  MemberGroupSchema,
  MyPermissionsSchema,
  PermissionCatalogueSchema,
} from "./schema";

const group = {
  id: "g1",
  workspace_id: "ws1",
  name: "Accounting",
  description: "",
  permissions: ["billing.view"],
  member_count: 2,
  created_by: "u1",
  created_at: "2026-11-01T00:00:00Z",
  updated_at: "2026-11-02T00:00:00Z",
};

describe("group schemas (spec 7b.2)", () => {
  it("parses a group and keeps its permission keys", () => {
    const parsed = MemberGroupSchema.parse(group);
    expect(parsed.permissions).toEqual(["billing.view"]);
    expect(parsed.member_count).toBe(2);
    expect(parsed.created_by).toBe("u1");
  });

  it("parses a response that lacks the newer fields", () => {
    // An older server sends no permissions, no count and no creator.
    const parsed = MemberGroupSchema.parse({ id: "g1", name: "Ops" });
    expect(parsed.permissions).toEqual([]);
    expect(parsed.member_count).toBe(0);
    expect(parsed.created_by).toBeNull();
  });

  it("keeps a permission key this client does not know as a plain string", () => {
    const parsed = MemberGroupSchema.parse({ ...group, permissions: ["future.key"] });
    expect(parsed.permissions).toEqual(["future.key"]);
  });

  it("parses the group detail with its members and tolerates a missing list", () => {
    const detail = MemberGroupDetailSchema.parse({
      ...group,
      members: [{ user_id: "u2", added_by: null, added_at: "2026-11-03T00:00:00Z" }],
    });
    expect(detail.members[0]?.user_id).toBe("u2");
    expect(MemberGroupDetailSchema.parse(group).members).toEqual([]);
  });

  it("falls back to an empty list on a malformed group list", () => {
    expect(
      parseWithFallback([{ id: 7 }, group], MemberGroupListSchema, [], { endpoint: "t" }),
    ).toEqual([]);
  });

  it("falls back on a permission list that is not an array", () => {
    const fallback = { ...group, permissions: [] };
    const out = parseWithFallback({ ...group, permissions: "billing.view" }, MemberGroupSchema, fallback, {
      endpoint: "t",
    });
    expect(out).toBe(fallback);
  });

  it("reads the permission catalogue and tolerates a missing key list", () => {
    expect(PermissionCatalogueSchema.parse({ permissions: ["audit.read"] }).permissions).toEqual([
      "audit.read",
    ]);
    expect(PermissionCatalogueSchema.parse({}).permissions).toEqual([]);
  });

  it("reads permissions/me and falls back on a malformed body", () => {
    expect(MyPermissionsSchema.parse({ role: "member", permissions: ["audit.read"] })).toEqual({
      role: "member",
      permissions: ["audit.read"],
    });
    // A response without the permissions field still parses, with none granted.
    expect(MyPermissionsSchema.parse({ role: "member" }).permissions).toEqual([]);
    const fallback = { role: "", permissions: [] };
    expect(
      parseWithFallback({ role: "member", permissions: [1] }, MyPermissionsSchema, fallback, {
        endpoint: "GET /permissions/me",
      }),
    ).toBe(fallback);
  });
});
