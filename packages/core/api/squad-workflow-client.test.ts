// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiClient } from "./client";

afterEach(() => vi.unstubAllGlobals());

function stubFetch(body: unknown) {
  const fetchMock = vi.fn().mockImplementation(
    async () => new Response(JSON.stringify(body), { headers: { "Content-Type": "application/json" } }),
  );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

describe("squad workflow API boundary", () => {
  it("lists a squad's library from GET /api/squads/{id}/workflows", async () => {
    const fetchMock = stubFetch({ active_workflow_id: "wf-1", workflows: [], total: 0 });
    const result = await new ApiClient("https://api.example.test").listSquadWorkflows("squad-1");
    expect(result).toEqual({ active_workflow_id: "wf-1", workflows: [], total: 0 });
    expect(String(fetchMock.mock.calls[0]?.[0])).toBe("https://api.example.test/api/squads/squad-1/workflows");
  });

  it("falls back to an empty library on a malformed body", async () => {
    stubFetch("nope");
    expect(await new ApiClient("https://api.example.test").listSquadWorkflows("squad-1")).toEqual({
      active_workflow_id: null,
      workflows: [],
      total: 0,
    });
  });

  it("posts the active-workflow switch and reads the answer", async () => {
    const fetchMock = stubFetch({
      squad_id: "squad-1",
      active_workflow_id: "wf-2",
      updated_issues: 4,
      plan: { required: [], unchanged: [], total_issues: 4, affected_issues: 0 },
    });
    const result = await new ApiClient("https://api.example.test").setSquadActiveWorkflow("squad-1", {
      workflow_id: "wf-2",
      status_mapping: { blocked: "todo" },
    });
    expect(result).toMatchObject({ squad_id: "squad-1", active_workflow_id: "wf-2", updated_issues: 4 });
    const [url, init] = fetchMock.mock.calls[0] ?? [];
    expect(String(url)).toBe("https://api.example.test/api/squads/squad-1/workflow");
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toEqual({ workflow_id: "wf-2", status_mapping: { blocked: "todo" } });
  });

  it("reads a dry run, which carries the plan alone", async () => {
    stubFetch({ dry_run: true, plan: { required: [{ status_key: "qa", issue_count: 2, suggested_status_key: "todo" }] } });
    const result = await new ApiClient("https://api.example.test").setSquadActiveWorkflow("squad-1", {
      workflow_id: "wf-2",
      dry_run: true,
    });
    expect(result.plan.required[0]).toMatchObject({ status_key: "qa", issue_count: 2 });
    expect(result.updated_issues).toBe(0);
  });
});

describe("issue flow API boundary", () => {
  function stubStatus(status: number, body: unknown) {
    const fetchMock = vi.fn().mockImplementation(
      async () => new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }),
    );
    vi.stubGlobal("fetch", fetchMock);
    return fetchMock;
  }
  const client = () => new ApiClient("https://api.example.test");

  it("reads an issue no flow runs as a state that is not managed", async () => {
    stubStatus(404, { error: "not run by a flow", code: "flow_not_managed" });
    const state = await client().getIssueFlow("iss-1");
    expect(state.managed).toBe(false);
    expect(state.tasks).toEqual([]);
  });

  it("does not hide any other failure", async () => {
    stubStatus(404, { error: "issue not found" });
    await expect(client().getIssueFlow("iss-1")).rejects.toThrow("issue not found");
    stubStatus(500, { error: "boom" });
    await expect(client().getIssueFlow("iss-1")).rejects.toThrow("boom");
  });

  it("reads the state from GET /api/issues/{id}/flow", async () => {
    const fetchMock = stubStatus(200, { managed: true, role: "task", step: "gate", stop: 3 });
    const state = await client().getIssueFlow("iss-1");
    expect(state).toMatchObject({ managed: true, role: "task", step: "gate", stop: 3 });
    expect(String(fetchMock.mock.calls[0]?.[0])).toBe("https://api.example.test/api/issues/iss-1/flow");
  });

  it("posts advance, resume and detach to their endpoints", async () => {
    const fetchMock = stubStatus(200, { state: { step: "draft" }, actions: [], pending: [], detached: true });
    const api = client();
    await api.advanceIssueFlow("iss-1");
    await api.resumeIssueFlow("iss-1", { to_step: "draft", reset_rework: true });
    expect(await api.detachIssueFlow("iss-1", { cancel_tasks: true })).toMatchObject({ detached: true });
    const calls = fetchMock.mock.calls.map(([url, init]) => [String(url), init?.method, JSON.parse(String(init?.body))]);
    expect(calls).toEqual([
      ["https://api.example.test/api/issues/iss-1/flow/advance", "POST", {}],
      ["https://api.example.test/api/issues/iss-1/flow/resume", "POST", { to_step: "draft", reset_rework: true }],
      ["https://api.example.test/api/issues/iss-1/flow/detach", "POST", { cancel_tasks: true }],
    ]);
  });

  it("puts the runner switch of a workflow", async () => {
    const fetchMock = stubStatus(200, { id: "wf-1", workspace_id: "ws-1", name: "Trade", created_at: "", updated_at: "", flow_auto: true });
    const workflow = await client().setIssueWorkflowFlowAuto("wf-1", true);
    expect(workflow.flow_auto).toBe(true);
    const [url, init] = fetchMock.mock.calls[0] ?? [];
    expect(String(url)).toBe("https://api.example.test/api/issue-workflows/wf-1/flow-auto");
    expect(init?.method).toBe("PUT");
    expect(JSON.parse(String(init?.body))).toEqual({ auto: true });
  });
});
