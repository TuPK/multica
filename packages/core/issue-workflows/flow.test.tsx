/**
 * @vitest-environment jsdom
 */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../api";
import {
  EMPTY_ISSUE_FLOW_STATE,
  IssueFlowAdvanceResponseSchema,
  IssueFlowStateSchema,
} from "../api/schemas";
import {
  issueFlowKeys,
  issueFlowOptions,
  issueWorkflowKeys,
  squadWorkflowsOptions,
  useAdvanceFlow,
  useDetachFlow,
  useResumeFlow,
  useSetFlowAuto,
  useSetSquadActiveWorkflow,
} from "./index";

const mocks = vi.hoisted(() => ({
  getIssueFlow: vi.fn(),
  advanceIssueFlow: vi.fn(),
  resumeIssueFlow: vi.fn(),
  detachIssueFlow: vi.fn(),
  setIssueWorkflowFlowAuto: vi.fn(),
  listSquadWorkflows: vi.fn(),
  setSquadActiveWorkflow: vi.fn(),
}));

vi.mock("../api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../api")>()),
  api: mocks,
}));

const stateWire = {
  managed: true,
  role: "request",
  workflow_id: "wf-1",
  workflow_name: "Trade",
  step: "gate",
  rework: 1,
  stop: null,
  switches: { route: "shipment_dossier", customs_lookup: true },
  auto: false,
  entered_at: "2026-11-13T00:00:00Z",
  tasks: [
    {
      issue_id: "t-1",
      identifier: "MUL-2",
      step: "gate",
      task: "docs_gate",
      round: 1,
      status: "done",
      outcome: "rejected",
      assignee_id: "agent-1",
      closed_at: "2026-11-13T01:00:00Z",
    },
  ],
  next: [{ id: "pass", label: "Pass", to: "handover", rework: false }],
};

function wrapperFor(qc: QueryClient) {
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

let qc: QueryClient;
beforeEach(() => {
  Object.values(mocks).forEach((m) => m.mockReset());
  qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
});

describe("IssueFlowStateSchema", () => {
  it("reads a request's state", () => {
    const parsed = IssueFlowStateSchema.parse(stateWire);
    expect(parsed).toMatchObject({ managed: true, role: "request", step: "gate", rework: 1, stop: null, auto: false });
    expect(parsed.switches).toEqual({ route: "shipment_dossier", customs_lookup: true });
    expect(parsed.tasks[0]).toMatchObject({ task: "docs_gate", outcome: "rejected", closed_at: "2026-11-13T01:00:00Z" });
    expect(parsed.next[0]).toEqual({ id: "pass", label: "Pass", to: "handover", rework: false });
  });

  it("reads every stop and treats an unknown one as stalled", () => {
    for (const [wire, want] of [[1, 1], [3, 3], ["stalled", "stalled"], ["later", "stalled"], [7, "stalled"], [null, null]] as const) {
      expect(IssueFlowStateSchema.parse({ ...stateWire, stop: wire }).stop).toBe(want);
    }
  });

  it("defaults what an older or newer server leaves out", () => {
    const parsed = IssueFlowStateSchema.parse({ step: "collect" });
    expect(parsed).toMatchObject({
      managed: true, role: "request", workflow_id: "", workflow_name: "", rework: 0, stop: null,
      switches: {}, auto: false, entered_at: "", tasks: [], next: [],
    });
  });

  it("keeps an outcome it does not know and drops a task it cannot read", () => {
    const parsed = IssueFlowStateSchema.parse({
      ...stateWire,
      tasks: [{ ...stateWire.tasks[0], outcome: "deferred" }, { identifier: "no key" }, "nope"],
    });
    expect(parsed.tasks).toHaveLength(1);
    expect(parsed.tasks[0]?.outcome).toBe("deferred");
  });

  it("falls back to a state that is not managed", () => {
    expect(EMPTY_ISSUE_FLOW_STATE.managed).toBe(false);
    expect(EMPTY_ISSUE_FLOW_STATE.tasks).toEqual([]);
  });
});

describe("IssueFlowAdvanceResponseSchema", () => {
  it("reads the actions and what the request waits for", () => {
    const parsed = IssueFlowAdvanceResponseSchema.parse({
      state: stateWire,
      actions: [
        { kind: "opened_tasks", step: "gate", tasks: ["docs_gate"] },
        { kind: "stopped", step: "blocked", tasks: [], code: "flow_agent_unavailable", agent_id: "a-1" },
      ],
      pending: [{ task: "docs_gate", issue_id: "t-1", status: "todo" }],
    });
    expect(parsed.actions).toHaveLength(2);
    expect(parsed.actions[1]).toMatchObject({ code: "flow_agent_unavailable", agent_id: "a-1" });
    expect(parsed.pending[0]).toEqual({ task: "docs_gate", issue_id: "t-1", status: "todo" });
  });

  it("tolerates missing or malformed lists", () => {
    const parsed = IssueFlowAdvanceResponseSchema.parse({ state: stateWire, actions: [{ step: "x" }, 3], pending: null });
    expect(parsed.actions).toEqual([]);
    expect(parsed.pending).toEqual([]);
  });
});

describe("issueFlowOptions", () => {
  it("keys the state under the workflow keys of the workspace", () => {
    expect(issueFlowKeys.detail("ws-1", "iss-1")).toEqual([...issueWorkflowKeys.all("ws-1"), "flow", "iss-1"]);
    expect(issueFlowKeys.all("ws-1")).toEqual([...issueWorkflowKeys.all("ws-1"), "flow"]);
  });

  it("reads the state of an issue", async () => {
    mocks.getIssueFlow.mockResolvedValue(IssueFlowStateSchema.parse(stateWire));
    const data = await qc.fetchQuery(issueFlowOptions("ws-1", "iss-1"));
    expect(mocks.getIssueFlow).toHaveBeenCalledWith("iss-1");
    expect(data.step).toBe("gate");
  });

  it("does not hide a failure of the read", async () => {
    mocks.getIssueFlow.mockRejectedValue(new ApiError("boom", 500, "Server Error"));
    await expect(qc.fetchQuery(issueFlowOptions("ws-1", "iss-3"))).rejects.toThrow("boom");
  });
});

describe("squadWorkflowsOptions", () => {
  it("reads a squad's library under the workflow keys", async () => {
    mocks.listSquadWorkflows.mockResolvedValue({ active_workflow_id: "wf-1", workflows: [], total: 0 });
    const options = squadWorkflowsOptions("ws-1", "squad-1");
    expect(options.queryKey).toEqual([...issueWorkflowKeys.all("ws-1"), "squad", "squad-1"]);
    const data = await qc.fetchQuery(options);
    expect(mocks.listSquadWorkflows).toHaveBeenCalledWith("squad-1");
    expect(data.active_workflow_id).toBe("wf-1");
  });
});

describe("flow mutations", () => {
  it("advance waits for the server, then invalidates the state", async () => {
    qc.setQueryData(issueFlowKeys.detail("ws-1", "iss-1"), IssueFlowStateSchema.parse(stateWire));
    let release!: (value: unknown) => void;
    mocks.advanceIssueFlow.mockReturnValue(new Promise((resolve) => { release = resolve; }));
    const invalidate = vi.spyOn(qc, "invalidateQueries");
    const { result } = renderHook(() => useAdvanceFlow("ws-1", "iss-1"), { wrapper: wrapperFor(qc) });

    act(() => { result.current.mutate(); });
    await waitFor(() => expect(mocks.advanceIssueFlow).toHaveBeenCalledWith("iss-1"));
    // Nothing is patched while the server works: no optimistic update.
    expect(qc.getQueryData<{ step: string }>(issueFlowKeys.detail("ws-1", "iss-1"))?.step).toBe("gate");
    expect(invalidate).not.toHaveBeenCalled();

    release({ state: IssueFlowStateSchema.parse({ ...stateWire, step: "handover" }), actions: [], pending: [] });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(invalidate).toHaveBeenCalledWith({ queryKey: issueFlowKeys.detail("ws-1", "iss-1") });
  });

  it("resume and detach pass their body and invalidate the state", async () => {
    mocks.resumeIssueFlow.mockResolvedValue(IssueFlowStateSchema.parse(stateWire));
    mocks.detachIssueFlow.mockResolvedValue({ detached: true });
    const invalidate = vi.spyOn(qc, "invalidateQueries");
    const resume = renderHook(() => useResumeFlow("ws-1", "iss-1"), { wrapper: wrapperFor(qc) });
    await act(async () => { await resume.result.current.mutateAsync({ to_step: "draft", reset_rework: true }); });
    expect(mocks.resumeIssueFlow).toHaveBeenCalledWith("iss-1", { to_step: "draft", reset_rework: true });
    const detach = renderHook(() => useDetachFlow("ws-1", "iss-1"), { wrapper: wrapperFor(qc) });
    await act(async () => { await detach.result.current.mutateAsync({ cancel_tasks: true }); });
    expect(mocks.detachIssueFlow).toHaveBeenCalledWith("iss-1", { cancel_tasks: true });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: issueFlowKeys.detail("ws-1", "iss-1") });
  });

  it("a failed mutation reports the error and still refreshes the state", async () => {
    mocks.resumeIssueFlow.mockRejectedValue(new ApiError("forbidden", 403, "Forbidden", { code: "flow_forbidden" }));
    const invalidate = vi.spyOn(qc, "invalidateQueries");
    const { result } = renderHook(() => useResumeFlow("ws-1", "iss-1"), { wrapper: wrapperFor(qc) });
    await act(async () => { await expect(result.current.mutateAsync({})).rejects.toThrow("forbidden"); });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: issueFlowKeys.detail("ws-1", "iss-1") });
  });

  it("flow auto saves the switch and refreshes the workflows", async () => {
    mocks.setIssueWorkflowFlowAuto.mockResolvedValue({ id: "wf-1" });
    const invalidate = vi.spyOn(qc, "invalidateQueries");
    const { result } = renderHook(() => useSetFlowAuto("ws-1"), { wrapper: wrapperFor(qc) });
    await act(async () => { await result.current.mutateAsync({ workflowId: "wf-1", auto: true }); });
    expect(mocks.setIssueWorkflowFlowAuto).toHaveBeenCalledWith("wf-1", true);
    expect(invalidate).toHaveBeenCalledWith({ queryKey: issueWorkflowKeys.all("ws-1") });
  });

  it("setting a squad's active workflow refreshes the library and the squads", async () => {
    mocks.setSquadActiveWorkflow.mockResolvedValue({ squad_id: "squad-1", active_workflow_id: "wf-2", updated_issues: 0, plan: {} });
    const invalidate = vi.spyOn(qc, "invalidateQueries");
    const { result } = renderHook(() => useSetSquadActiveWorkflow("ws-1", "squad-1"), { wrapper: wrapperFor(qc) });
    await act(async () => { await result.current.mutateAsync({ workflow_id: "wf-2", status_mapping: { qa: "todo" } }); });
    expect(mocks.setSquadActiveWorkflow).toHaveBeenCalledWith("squad-1", { workflow_id: "wf-2", status_mapping: { qa: "todo" } });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: issueWorkflowKeys.all("ws-1") });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["workspaces", "ws-1", "squads"] });
  });
});
