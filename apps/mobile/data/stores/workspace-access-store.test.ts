// @vitest-environment node
import { beforeEach, describe, expect, it } from "vitest";
import { useWorkspaceAccessStore } from "./workspace-access-store";

describe("useWorkspaceAccessStore", () => {
  beforeEach(() => {
    useWorkspaceAccessStore.getState().clearAll();
  });

  it("records and clears workspace blocks per slug", () => {
    useWorkspaceAccessStore.getState().setBlock("acme", "read_only");
    useWorkspaceAccessStore.getState().setBlock("beta", "suspended");
    expect(useWorkspaceAccessStore.getState().blocks).toEqual({
      acme: "read_only",
      beta: "suspended",
    });

    useWorkspaceAccessStore.getState().clearWorkspace("acme");
    expect(useWorkspaceAccessStore.getState().blocks).toEqual({
      beta: "suspended",
    });

    useWorkspaceAccessStore.getState().clearAll();
    expect(useWorkspaceAccessStore.getState().blocks).toEqual({});
  });
});
