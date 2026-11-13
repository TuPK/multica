/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { setApiInstance } from "../api";
import type { ApiClient } from "../api/client";
import { issueWorkflowKeys } from "../issue-workflows/queries";
import type { FlowDraft } from "../types";
import { flowDraftKeys } from "./queries";
import { useExportFlowDraft, usePublishFlowDraft, useUpdateFlowDraft } from "./mutations";

vi.mock("../hooks", () => ({ useWorkspaceId: () => "ws-1" }));

const fakeApi = { publishFlowDraft: vi.fn(), updateFlowDraft: vi.fn(), exportFlowDraft: vi.fn() };

function setup() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  return { qc, wrapper };
}

const draft = { id: "d-1", squad_id: "sq-1", name: "Old", revision: 1 } as unknown as FlowDraft;

beforeEach(() => setApiInstance(fakeApi as unknown as ApiClient));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("usePublishFlowDraft", () => {
  it("leaves the cache alone until the server has answered, then refreshes drafts and workflows", async () => {
    const { qc, wrapper } = setup();
    const detail = flowDraftKeys.detail("ws-1", "sq-1", "d-1");
    const squadWorkflows = issueWorkflowKeys.squad("ws-1", "sq-1");
    qc.setQueryData(detail, draft);
    qc.setQueryData(squadWorkflows, { workflows: [] });
    let answer!: (row: unknown) => void;
    fakeApi.publishFlowDraft.mockImplementation(() => new Promise((resolve) => (answer = resolve)));

    const { result } = renderHook(() => usePublishFlowDraft(), { wrapper });
    let pending!: Promise<unknown>;
    act(() => {
      pending = result.current.mutateAsync({ squadId: "sq-1", draftId: "d-1", data: { revision: 1 } });
    });

    await waitFor(() => expect(fakeApi.publishFlowDraft).toHaveBeenCalledWith("sq-1", "d-1", { revision: 1 }));
    // Nothing was guessed while the server decided: the draft is as it was and nothing is marked stale.
    expect(qc.getQueryData(detail)).toBe(draft);
    expect(qc.getQueryState(detail)?.isInvalidated).toBe(false);
    expect(qc.getQueryState(squadWorkflows)?.isInvalidated).toBe(false);

    await act(async () => {
      answer({ id: "wf-9" });
      await pending;
    });
    expect(qc.getQueryState(detail)?.isInvalidated).toBe(true);
    expect(qc.getQueryState(squadWorkflows)?.isInvalidated).toBe(true);
  });

  it("changes nothing in the cache when the server refuses", async () => {
    const { qc, wrapper } = setup();
    const detail = flowDraftKeys.detail("ws-1", "sq-1", "d-1");
    qc.setQueryData(detail, draft);
    fakeApi.publishFlowDraft.mockRejectedValue(new Error("stale"));

    const { result } = renderHook(() => usePublishFlowDraft(), { wrapper });
    await act(async () => {
      await result.current.mutateAsync({ squadId: "sq-1", draftId: "d-1", data: { revision: 1 } }).catch(() => undefined);
    });

    expect(qc.getQueryData(detail)).toBe(draft);
    expect(qc.getQueryState(detail)?.isInvalidated).toBe(false);
  });
});

describe("useUpdateFlowDraft", () => {
  it("keeps the draft the server answered with, revision included", async () => {
    const { qc, wrapper } = setup();
    fakeApi.updateFlowDraft.mockResolvedValue({ ...draft, name: "New", revision: 2 });

    const { result } = renderHook(() => useUpdateFlowDraft(), { wrapper });
    await act(async () => {
      await result.current.mutateAsync({ squadId: "sq-1", draftId: "d-1", data: { revision: 1, name: "New" } });
    });

    expect(qc.getQueryData<FlowDraft>(flowDraftKeys.detail("ws-1", "sq-1", "d-1"))).toMatchObject({ name: "New", revision: 2 });
  });
});

describe("useExportFlowDraft", () => {
  it("reads the manifest of the draft from the server", async () => {
    const { wrapper } = setup();
    fakeApi.exportFlowDraft.mockResolvedValue({ format: "multica.flow-manifest/v1", name: "F", doc: {} });

    const { result } = renderHook(() => useExportFlowDraft(), { wrapper });
    await act(async () => {
      await result.current.mutateAsync({ squadId: "sq-1", draftId: "d-1" });
    });

    expect(fakeApi.exportFlowDraft).toHaveBeenCalledWith("sq-1", "d-1");
  });
});
