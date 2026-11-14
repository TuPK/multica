/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { setApiInstance } from "../api";
import type { ApiClient } from "../api/client";
import { workspaceKeys } from "../workspace/queries";
import { flowDraftKeys } from "../flow-drafts/queries";
import { packKeys, flowInsightsOptions } from "./queries";
import { useCommitPack, useFlowInsights, useImportFlowProposal, usePreviewPack } from "./index";
import { packConflictPrevious, packErrorCode, packNameConflicts } from "./errors";

vi.mock("../hooks", () => ({ useWorkspaceId: () => "ws-1" }));

const fakeApi = {
  previewPack: vi.fn(),
  previewPackUpload: vi.fn(),
  commitPack: vi.fn(),
  importFlowProposal: vi.fn(),
  getFlowInsights: vi.fn(),
};

function setup() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  return { qc, wrapper };
}

beforeEach(() => setApiInstance(fakeApi as unknown as ApiClient));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("packKeys", () => {
  it("includes the workspace id in every key", () => {
    expect(packKeys.all("ws-1")).toEqual(["packs", "ws-1"]);
    expect(packKeys.insights("ws-1", "wf-1", false)).toEqual(["packs", "ws-1", "insights", "wf-1", false]);
    expect(packKeys.insights("ws-1", "wf-1", true)).not.toEqual(packKeys.insights("ws-1", "wf-1", false));
    expect(flowInsightsOptions("ws-1", "wf-1", false).queryKey).toEqual(packKeys.insights("ws-1", "wf-1", false));
  });
});

describe("usePreviewPack", () => {
  it("previews a source and a zip through their own calls", async () => {
    const { wrapper } = setup();
    fakeApi.previewPack.mockResolvedValue({ digest: "sha256:a" });
    fakeApi.previewPackUpload.mockResolvedValue({ digest: "sha256:b" });
    const { result } = renderHook(() => usePreviewPack(), { wrapper });

    await act(async () => {
      await result.current.mutateAsync({ source: { kind: "github", url: "https://github.com/a/b" } });
    });
    expect(fakeApi.previewPack).toHaveBeenCalledWith({ kind: "github", url: "https://github.com/a/b" });

    const file = new File(["z"], "p.zip");
    await act(async () => {
      await result.current.mutateAsync({ file });
    });
    expect(fakeApi.previewPackUpload).toHaveBeenCalledWith(file);
  });
});

describe("useCommitPack", () => {
  it("waits for the server and then refreshes the squads and the drafts, without guessing before", async () => {
    const { qc, wrapper } = setup();
    qc.setQueryData(workspaceKeys.squads("ws-1"), []);
    qc.setQueryData(flowDraftKeys.all("ws-1"), []);
    let answer!: (row: unknown) => void;
    fakeApi.commitPack.mockImplementation(() => new Promise((resolve) => (answer = resolve)));
    const { result } = renderHook(() => useCommitPack(), { wrapper });

    let pending!: Promise<unknown>;
    act(() => {
      pending = result.current.mutateAsync({ digest: "sha256:a" });
    });
    await waitFor(() => expect(fakeApi.commitPack).toHaveBeenCalledWith({ digest: "sha256:a" }));
    expect(qc.getQueryState(workspaceKeys.squads("ws-1"))?.isInvalidated).toBe(false);

    await act(async () => {
      answer({ install_id: "i-1", squad_id: "sq-1" });
      await pending;
    });
    expect(qc.getQueryState(workspaceKeys.squads("ws-1"))?.isInvalidated).toBe(true);
    expect(qc.getQueryState(flowDraftKeys.all("ws-1"))?.isInvalidated).toBe(true);
  });

  it("leaves the cache alone when the server refuses", async () => {
    const { qc, wrapper } = setup();
    qc.setQueryData(workspaceKeys.squads("ws-1"), []);
    fakeApi.commitPack.mockRejectedValue(Object.assign(new Error("conflict"), { status: 409, body: { code: "pack_conflict" } }));
    const { result } = renderHook(() => useCommitPack(), { wrapper });
    await act(async () => {
      await expect(result.current.mutateAsync({ digest: "sha256:a" })).rejects.toThrow("conflict");
    });
    expect(qc.getQueryState(workspaceKeys.squads("ws-1"))?.isInvalidated).toBe(false);
  });
});

describe("useImportFlowProposal", () => {
  it("puts the draft the server answered with in the cache and marks the list stale", async () => {
    const { qc, wrapper } = setup();
    qc.setQueryData(flowDraftKeys.list("ws-1", "sq-1"), { drafts: [], total: 0 });
    fakeApi.importFlowProposal.mockResolvedValue({ id: "d-1", revision: 2 });
    const { result } = renderHook(() => useImportFlowProposal(), { wrapper });
    await act(async () => {
      await result.current.mutateAsync({ squadId: "sq-1", draftId: "d-1", revision: 1 });
    });
    expect(fakeApi.importFlowProposal).toHaveBeenCalledWith("sq-1", "d-1", { revision: 1 });
    expect(qc.getQueryData(flowDraftKeys.detail("ws-1", "sq-1", "d-1"))).toMatchObject({ id: "d-1", revision: 2 });
    expect(qc.getQueryState(flowDraftKeys.list("ws-1", "sq-1"))?.isInvalidated).toBe(true);
  });
});

describe("useFlowInsights", () => {
  it("reads the numbers of a version and waits until it has a version and the feature is on", async () => {
    const { wrapper } = setup();
    fakeApi.getFlowInsights.mockResolvedValue({ requests: { total: 3, open: 0, by_end: {} } });
    const off = renderHook(() => useFlowInsights("wf-1", { includeTrial: false, enabled: false }), { wrapper });
    const none = renderHook(() => useFlowInsights(null, { includeTrial: false }), { wrapper });
    const on = renderHook(() => useFlowInsights("wf-1", { includeTrial: true }), { wrapper });
    await waitFor(() => expect(on.result.current.data?.requests.total).toBe(3));
    expect(fakeApi.getFlowInsights).toHaveBeenCalledTimes(1);
    expect(fakeApi.getFlowInsights).toHaveBeenCalledWith("wf-1", { includeTrial: true });
    expect(off.result.current.fetchStatus).toBe("idle");
    expect(none.result.current.fetchStatus).toBe("idle");
  });
});

describe("pack error helpers", () => {
  const conflict = Object.assign(new Error("x"), {
    body: { code: "pack_conflict", previous: { install_id: "i-0", version: "1.0.0", squad_id: "sq-0", installed_at: "2026-11-01T00:00:00Z" } },
  });

  it("reads the code and the earlier install of a conflict", () => {
    expect(packErrorCode(conflict)).toBe("pack_conflict");
    expect(packConflictPrevious(conflict)).toEqual({ install_id: "i-0", version: "1.0.0", squad_id: "sq-0", installed_at: "2026-11-01T00:00:00Z" });
  });

  it("reads a body that is not what it should be as nothing", () => {
    expect(packConflictPrevious(new Error("x"))).toBeNull();
    expect(packConflictPrevious(Object.assign(new Error("x"), { body: { previous: "old" } }))).toBeNull();
    expect(packErrorCode(null)).toBeUndefined();
  });

  it("lists the names that are taken", () => {
    const err = Object.assign(new Error("x"), { body: { code: "pack_name_conflict", skills: ["a", 2, "b"], agents: ["Writer"] } });
    expect(packNameConflicts(err)).toEqual({ skills: ["a", "b"], agents: ["Writer"] });
    expect(packNameConflicts(new Error("x"))).toEqual({ skills: [], agents: [] });
  });
});
