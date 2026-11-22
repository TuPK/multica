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

const api = () => new ApiClient("https://api.example.test");

describe("room workflow policy API boundary", () => {
  it("reads the stored choice from GET /api/workspaces/{id}/room-workflow", async () => {
    const fetchMock = stubFetch({ non_request_workflow: { kind: "workflow", workflow_id: "wf-1" } });
    expect(await api().getRoomWorkflowPolicy("ws-1")).toEqual({ kind: "workflow", workflowId: "wf-1" });
    expect(String(fetchMock.mock.calls[0]?.[0])).toBe("https://api.example.test/api/workspaces/ws-1/room-workflow");
  });

  it("falls back to no_action on a malformed body", async () => {
    stubFetch("nope");
    expect(await api().getRoomWorkflowPolicy("ws-1")).toEqual({ kind: "no_action", workflowId: null });
  });

  it("falls back to no_action on an unknown kind", async () => {
    stubFetch({ non_request_workflow: { kind: "fallback" } });
    expect(await api().getRoomWorkflowPolicy("ws-1")).toEqual({ kind: "no_action", workflowId: null });
  });

  it("drops a workflow id that a default choice does not carry", async () => {
    stubFetch({ non_request_workflow: { kind: "default", workflow_id: "wf-stale" } });
    expect(await api().getRoomWorkflowPolicy("ws-1")).toEqual({ kind: "default", workflowId: null });
  });

  it("puts the choice in the server's shape and reads the answer", async () => {
    const fetchMock = stubFetch({ non_request_workflow: { kind: "default" } });
    const saved = await api().setRoomWorkflowPolicy("ws-1", { kind: "default", workflowId: null });
    expect(saved).toEqual({ kind: "default", workflowId: null });
    const [url, init] = fetchMock.mock.calls[0] ?? [];
    expect(String(url)).toBe("https://api.example.test/api/workspaces/ws-1/room-workflow");
    expect(init?.method).toBe("PUT");
    expect(JSON.parse(String(init?.body))).toEqual({ non_request_workflow: { kind: "default" } });
  });

  it("answers the saved choice it sent when the reply cannot be read", async () => {
    stubFetch(null);
    const sent = { kind: "workflow" as const, workflowId: "wf-2" };
    expect(await api().setRoomWorkflowPolicy("ws-1", sent)).toEqual(sent);
  });
});

describe("room workflow check API boundary", () => {
  it("sends the write's inputs as query parameters and maps the refusal", async () => {
    const fetchMock = stubFetch({
      needs_workflow: true,
      code: "room_workflow_required",
      message: "this squad has no workflow yet",
      squad_id: "squad-1",
    });
    const check = await api().checkRoomWorkflow("ws-1", {
      projectId: "proj-1",
      assigneeType: "squad",
      assigneeId: "squad-1",
    });
    expect(check).toEqual({
      needsWorkflow: true,
      code: "room_workflow_required",
      message: "this squad has no workflow yet",
      squadId: "squad-1",
    });
    const url = new URL(String(fetchMock.mock.calls[0]?.[0]));
    expect(url.pathname).toBe("/api/workspaces/ws-1/room-workflow/check");
    expect(url.searchParams.get("project_id")).toBe("proj-1");
    expect(url.searchParams.get("assignee_type")).toBe("squad");
    expect(url.searchParams.get("assignee_id")).toBe("squad-1");
    expect(url.searchParams.has("parent_issue_id")).toBe(false);
  });

  it("says nothing needs a workflow on a malformed body", async () => {
    stubFetch({ needs_workflow: "maybe", code: "made_up_code" });
    expect(await api().checkRoomWorkflow("ws-1", {})).toEqual({
      needsWorkflow: false,
      code: null,
      message: "",
      squadId: null,
    });
  });
});
