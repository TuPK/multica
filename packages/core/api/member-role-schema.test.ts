import { describe, expect, it } from "vitest";
import {
  MemberWithUserSchema,
  ShareLinkInfoSchema,
  ShareLinkSchema,
} from "./schemas";

// Spec 6.6: a role this client does not know is a viewer, the lowest privilege.
// A server that adds a role must not give the new role write access here.
const member = {
  id: "m1",
  workspace_id: "ws1",
  user_id: "u1",
  name: "Ada",
  email: "ada@example.com",
};

describe("member role schemas (spec 6.6)", () => {
  it("keeps every known role", () => {
    for (const role of ["owner", "admin", "member", "viewer"]) {
      expect(MemberWithUserSchema.parse({ ...member, role }).role).toBe(role);
    }
  });

  it("falls back to viewer for a role this client does not know", () => {
    expect(MemberWithUserSchema.parse({ ...member, role: "guest" }).role).toBe("viewer");
    expect(MemberWithUserSchema.parse({ ...member, role: "" }).role).toBe("viewer");
  });

  it("falls back to viewer on share links and share link info", () => {
    const link = {
      id: "l1",
      workspace_id: "ws1",
      code: "c",
      created_by: "u1",
      role: "viewer",
    };
    expect(ShareLinkSchema.parse(link).role).toBe("viewer");
    expect(ShareLinkSchema.parse({ ...link, role: "superuser" }).role).toBe("viewer");
    expect(ShareLinkInfoSchema.parse({ workspace_name: "W", role: "viewer" }).role).toBe("viewer");
    expect(ShareLinkInfoSchema.parse({ workspace_name: "W", role: "superuser" }).role).toBe("viewer");
  });
});
