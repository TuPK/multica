// @vitest-environment node
import { describe, expect, it } from "vitest";
import { canWriteInWorkspace, type WorkspaceBlockReason } from "./workspace-access";

describe("canWriteInWorkspace", () => {
  it.each(["owner", "admin", "member"] as const)(
    "allows write for %s when there is no workspace block",
    (role) => {
      expect(canWriteInWorkspace({ role, block: null })).toBe(true);
      expect(canWriteInWorkspace({ role, block: undefined })).toBe(true);
    },
  );

  it("denies write for viewer even when workspace is active", () => {
    expect(canWriteInWorkspace({ role: "viewer", block: null })).toBe(false);
    expect(canWriteInWorkspace({ role: "viewer", block: undefined })).toBe(false);
  });

  it.each(["guest", "", null, undefined])(
    "denies write for unknown or unloaded role (%j)",
    (role) => {
      expect(canWriteInWorkspace({ role, block: null })).toBe(false);
    },
  );

  it.each<WorkspaceBlockReason>(["read_only", "suspended", "pending_deletion"])(
    "denies write for all roles when workspace block is %s",
    (block) => {
      for (const role of ["owner", "admin", "member", "viewer", null] as const) {
        expect(canWriteInWorkspace({ role, block })).toBe(false);
      }
    },
  );
});
