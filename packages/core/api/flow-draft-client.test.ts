// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiClient } from "./client";

afterEach(() => vi.unstubAllGlobals());

function stubFetch(body: unknown, status = 200) {
  const fetchMock = vi.fn().mockImplementation(
    async () => new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }),
  );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

const client = () => new ApiClient("https://api.example.test");
const call = (fetchMock: ReturnType<typeof stubFetch>) => {
  const [url, init] = fetchMock.mock.calls[0] ?? [];
  return { url: String(url), method: init?.method ?? "GET", body: init?.body ? JSON.parse(String(init.body)) : undefined };
};

// The routes of spec 7.1 and 7.6, with the bodies the server reads.
describe("flow draft routes", () => {
  it("lists, reads, creates, saves and deletes a draft", async () => {
    let fetchMock = stubFetch({ drafts: [], total: 0 });
    await client().listFlowDrafts("sq-1");
    expect(call(fetchMock)).toMatchObject({ url: "https://api.example.test/api/squads/sq-1/flow-drafts", method: "GET" });

    fetchMock = stubFetch({ id: "d-1", revision: 1 });
    await client().getFlowDraft("sq-1", "d-1");
    expect(call(fetchMock).url).toBe("https://api.example.test/api/squads/sq-1/flow-drafts/d-1");

    fetchMock = stubFetch({ id: "d-1", revision: 1 });
    await client().createFlowDraft("sq-1", { name: "A", from: { kind: "workflow", workflow_id: "wf-1", keep_bind: true } });
    expect(call(fetchMock)).toMatchObject({ method: "POST", body: { name: "A", from: { kind: "workflow", workflow_id: "wf-1", keep_bind: true } } });

    fetchMock = stubFetch({ id: "d-1", revision: 4 });
    await client().updateFlowDraft("sq-1", "d-1", { revision: 3, name: "B", doc: { version: 2 } });
    expect(call(fetchMock)).toMatchObject({ method: "PATCH", body: { revision: 3, name: "B", doc: { version: 2 } } });

    fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);
    await client().deleteFlowDraft("sq-1", "d-1");
    expect(call(fetchMock)).toMatchObject({ method: "DELETE", url: "https://api.example.test/api/squads/sq-1/flow-drafts/d-1" });
  });

  it("checks, simulates, publishes and exports a draft", async () => {
    let fetchMock = stubFetch({ ok: true, revision: 2, issues: [] });
    await client().validateFlowDraft("sq-1", "d-1", { doc: { version: 2 } });
    expect(call(fetchMock)).toMatchObject({ method: "POST", url: "https://api.example.test/api/squads/sq-1/flow-drafts/d-1/validate", body: { doc: { version: 2 } } });

    fetchMock = stubFetch({ hops: [], end: { kind: "terminal" }, truncated: false });
    await client().simulateFlowDraft("sq-1", "d-1", { doc: {}, vars: { x: 1 }, script: { a: [{ t: "done" }] }, max_hops: 60 });
    expect(call(fetchMock).body).toEqual({ doc: {}, vars: { x: 1 }, script: { a: [{ t: "done" }] }, max_hops: 60 });
    expect(call(fetchMock).body).not.toHaveProperty("switches");
    expect(call(fetchMock).body).not.toHaveProperty("path_choices");

    fetchMock = stubFetch({ id: "wf-9", workspace_id: "ws-1", name: "Flow · v1", version: 1, created_at: "2026-11-13T00:00:00Z", updated_at: "2026-11-13T00:00:00Z" });
    const published = await client().publishFlowDraft("sq-1", "d-1", { revision: 3, ack: ["no_gate"] });
    expect(call(fetchMock)).toMatchObject({ method: "POST", url: "https://api.example.test/api/squads/sq-1/flow-drafts/d-1/publish", body: { revision: 3, ack: ["no_gate"] } });
    expect(published).toMatchObject({ id: "wf-9", version: 1 });

    fetchMock = stubFetch({ format: "multica.flow-manifest/v1", name: "F", doc: { version: 2 } });
    await client().exportFlowDraft("sq-1", "d-1");
    expect(call(fetchMock).url).toBe("https://api.example.test/api/squads/sq-1/flow-drafts/d-1/export");

    fetchMock = stubFetch({ format: "multica.flow-manifest/v1", name: "F", doc: { version: 2 } });
    await client().getIssueWorkflowFlowManifest("wf-9");
    expect(call(fetchMock).url).toBe("https://api.example.test/api/issue-workflows/wf-9/flow-manifest");
  });

  it("reads the capabilities from GET /api/flow/capabilities", async () => {
    const fetchMock = stubFetch({ features: { authoring: true } });
    const caps = await client().getFlowCapabilities();
    expect(call(fetchMock).url).toBe("https://api.example.test/api/flow/capabilities");
    expect(caps.features.authoring).toBe(true);
  });
});

// POST /api/flow/validate: the check without a draft, which an agent may call (spec 7.1, 16.4).
describe("validateFlowStateless", () => {
  it("posts the manifest and the squad to the route the server has", async () => {
    const manifest = { format: "multica.flow-manifest/v1", name: "F", doc: { version: 2 } };
    const fetchMock = stubFetch({ ok: false, revision: 0, issues: [{ code: "role_unused", severity: "error", message: "x" }], analysis: null, fact_catalog: {} });

    const result = await client().validateFlowStateless({ manifest, squad_id: "sq-1" });

    expect(call(fetchMock)).toMatchObject({ method: "POST", url: "https://api.example.test/api/flow/validate", body: { manifest, squad_id: "sq-1" } });
    expect(result.ok).toBe(false);
    expect(result.issues[0]).toMatchObject({ code: "role_unused", severity: "error" });
  });

  it("falls back to an answer that says nothing is ok when the body is malformed", async () => {
    for (const body of ["nope", null, 7, [], { ok: "yes", issues: "many", analysis: "deep", fact_catalog: [] }]) {
      stubFetch(body);
      const result = await client().validateFlowStateless({ manifest: {} });
      expect(result.ok).toBe(false);
      expect(result.issues).toEqual([]);
      expect(result.analysis).toBeNull();
      expect(result.fact_catalog).toEqual({});
    }
  });

  it("surfaces the server's refusal of a manifest as an error with its code", async () => {
    stubFetch({ error: "the format \"x\" is not multica.flow-manifest/v1", code: "flow_manifest_invalid" }, 400);
    await expect(client().validateFlowStateless({ manifest: {} })).rejects.toMatchObject({ status: 400, body: { code: "flow_manifest_invalid" } });
  });
});

// The routes of the running side (spec 7.1).
describe("flow run routes", () => {
  it("sends the fields of resume the server reads", async () => {
    const fetchMock = stubFetch({ managed: true });
    await client().resumeIssueFlow("iss-1", { to_step: "draft", reset_rework: true, reset_loops: ["fixes"], skip_cancelled: true });
    expect(call(fetchMock)).toMatchObject({
      method: "POST",
      url: "https://api.example.test/api/issues/iss-1/flow/resume",
      body: { to_step: "draft", reset_rework: true, reset_loops: ["fixes"], skip_cancelled: true },
    });
  });

  it("completes a lead step, sets a variable, takes a manual exit and closes a task on behalf", async () => {
    let fetchMock = stubFetch({ managed: true });
    await client().completeIssueFlowStep("iss-1", "plan", { verdict: "planned", vars: { a: 1 } });
    expect(call(fetchMock)).toMatchObject({ method: "POST", url: "https://api.example.test/api/issues/iss-1/flow/steps/plan/complete", body: { verdict: "planned", vars: { a: 1 } } });

    fetchMock = stubFetch({ managed: true });
    await client().setIssueFlowVar("iss-1", "topics", ["a"]);
    expect(call(fetchMock)).toMatchObject({ method: "PUT", url: "https://api.example.test/api/issues/iss-1/flow/vars/topics", body: { value: ["a"] } });

    fetchMock = stubFetch({ managed: true });
    await client().exitIssueFlow("iss-1", "edge-1");
    expect(call(fetchMock)).toMatchObject({ method: "POST", url: "https://api.example.test/api/issues/iss-1/flow/exits/edge-1" });

    fetchMock = stubFetch({ task: { task: "docs_gate" }, state: { managed: true } });
    await client().closeIssueFlowTask("task-1", { verdict: "approved", note: "by hand" });
    expect(call(fetchMock)).toMatchObject({ method: "POST", url: "https://api.example.test/api/issues/task-1/flow/close", body: { verdict: "approved", note: "by hand" } });
  });
});

// Choosing the workflow of a request and rehearsing a version (spec 7.1, 16.5).
describe("starting a request on a chosen version", () => {
  it("sends nothing but an empty body when no version is chosen", async () => {
    const fetchMock = stubFetch({ managed: true, workflow_id: "wf-active", step: "todo" });
    const state = await client().startIssueFlow("iss-1");
    expect(call(fetchMock)).toMatchObject({ method: "POST", url: "https://api.example.test/api/issues/iss-1/flow/start", body: {} });
    expect(state).toMatchObject({ managed: true, workflow_id: "wf-active", trial: false });
  });

  it("sends the chosen version and the trial flag", async () => {
    const fetchMock = stubFetch({ managed: true, workflow_id: "wf-2", trial: true });
    const state = await client().startIssueFlow("iss-1", { workflow_id: "wf-2", trial: true });
    expect(call(fetchMock).body).toEqual({ workflow_id: "wf-2", trial: true });
    expect(state.trial).toBe(true);
    expect(state.workflow_id).toBe("wf-2");
  });

  it("reads a start answer that lacks or garbles the new fields as an ordinary request", async () => {
    stubFetch({ managed: true, workflow_id: "wf-1", trial: "yes" });
    expect((await client().startIssueFlow("iss-1")).trial).toBe(false);
    stubFetch({ managed: true, workflow_id: "wf-1", trial: null });
    expect((await client().startIssueFlow("iss-1")).trial).toBe(false);
    stubFetch("not an object");
    const empty = await client().startIssueFlow("iss-1");
    expect(empty.trial).toBe(false);
    expect(empty.managed).toBe(false);
  });

  it("surfaces a refused choice as an error with its code", async () => {
    stubFetch({ error: "the workflow does not belong to the squad of this request", code: "flow_workflow_invalid" }, 422);
    await expect(client().startIssueFlow("iss-1", { workflow_id: "other" })).rejects.toMatchObject({ status: 422, body: { code: "flow_workflow_invalid" } });
  });

  it("creates an issue with the chosen version and the trial flag", async () => {
    const issue = {
      id: "iss-9", workspace_id: "ws-1", number: 9, identifier: "MUL-9", title: "Sample", description: null, status: "todo", priority: "none",
      assignee_type: "squad", assignee_id: "sq-1", creator_type: "member", creator_id: "user-1", parent_issue_id: null, project_id: null,
      position: 0, stage: null, start_date: null, due_date: null, metadata: {}, properties: {},
      created_at: "2025-01-01T00:00:00Z", updated_at: "2025-01-01T00:00:00Z",
    };
    const fetchMock = stubFetch(issue, 201);
    await client().createIssue({ title: "Sample", assignee_type: "squad", assignee_id: "sq-1", flow_workflow_id: "wf-2", flow_trial: true });
    expect(call(fetchMock)).toMatchObject({
      method: "POST",
      url: "https://api.example.test/api/issues",
      body: { title: "Sample", assignee_id: "sq-1", flow_workflow_id: "wf-2", flow_trial: true },
    });
  });
});
