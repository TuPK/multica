// @vitest-environment node
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Agent, IssueWorkflow } from "../types";
import type { CatalogFlow } from "../catalog/types";
import { flowWorkflowName } from "./flow-workflow";
import { runSquadQuickstart, type SquadQuickstartClient } from "./quickstart";
import { SQUAD_TEMPLATES, type SquadTemplate } from "./templates";

// A flow for the template below, standing in for the generated CATALOG_FLOWS (empty until a flow manifest exists).
const flowData = vi.hoisted(() => {
  const flow: CatalogFlow = {
    id: "flow_kv_demo",
    version: "1.0.0",
    rev: "abcd1234",
    squad: "kv_demo",
    config: { version: 1, switches: [{ key: "lookup", type: "bool", default: false }], messages: { stop1: "BLOCKED · stop 1\nbody" } },
    steps: [
      { status: { key: "todo" }, statusKey: "todo", transitions: [{ id: "plan_ok", label: "Plan", when: null, to: "collect" }] },
      {
        status: { name: "Collect", category: "started" },
        statusKey: "collect",
        tasks: [
          { key: "read", title: "Read", kind: "work", role: "analyst", instructions: "Read it." },
          { key: "extra", title: "Look up", kind: "work", role: "analyst", instructions: "", when: { field: "switch.lookup", op: "eq", value: true } },
        ],
        transitions: [{ id: "next", label: "Next", when: null, to: "gate" }],
      },
      {
        status: { name: "Gate", category: "started" },
        statusKey: "gate",
        tasks: [{ key: "check", title: "Check", kind: "gate", role: "developer", instructions: "Check.", requires: ["outcome"] }],
        transitions: [
          { id: "ok", label: "Approved", when: { field: "tasks_outcome", op: "eq", value: "all_approved" }, to: "in_review" },
          { id: "again", label: "Back", when: { all: [{ field: "tasks_outcome", op: "eq", value: "any_rejected" }, { field: "rework_count", op: "lt", value: 2 }] }, to: "collect", rework: true },
          { id: "stop", label: "Stop", when: null, to: "blocked" },
        ],
      },
      { status: { key: "in_review" }, statusKey: "in_review", humanOnly: true },
      { status: { key: "blocked" }, statusKey: "blocked" },
    ],
  };
  return { flow };
});

vi.mock("../catalog/flows", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../catalog/flows")>()),
  flowForSquad: (squadId: string) => (squadId === flowData.flow.squad ? flowData.flow : undefined),
}));

const greenfield = SQUAD_TEMPLATES.find((t) => t.id === "sdlc_greenfield")!;
// A squad with an enforced flow: no floor, no skills, so only the flow calls are of interest.
const flowSquad = {
  ...greenfield,
  id: "kv_demo",
  name: "Demo Desk",
  floorName: null,
  builtinSkills: [],
  leaderSkills: [],
  roleSkills: {},
  routines: [],
  workflow: null,
} as unknown as SquadTemplate;
const options = { runtimeId: "rt-1", createWorkflow: false };

function fakeClient(over: Record<string, unknown> = {}) {
  let n = 0;
  const workflows: Partial<IssueWorkflow>[] = [];
  const base = {
    listAgents: vi.fn(async () => [] as Agent[]),
    createAgent: vi.fn(async (data: { name: string }) => ({ id: `agent-${++n}`, name: data.name, archived_at: null })),
    listSquads: vi.fn(async () => [] as never[]),
    createSquad: vi.fn(async (data: { name: string; leader_id: string }) => ({ id: "squad-1", name: data.name, leader_id: data.leader_id, active_workflow_id: null })),
    updateSquad: vi.fn(async () => ({})),
    updateSquadCharter: vi.fn(async () => ({})),
    addSquadMember: vi.fn(async () => ({})),
    listSkills: vi.fn(async () => []),
    listSquadSkills: vi.fn(async () => [] as never[]),
    setSquadSkills: vi.fn(async () => []),
    listSquadMembers: vi.fn(async () => [] as never[]),
    addAgentSkills: vi.fn(async () => undefined),
    listIssueWorkflows: vi.fn(async () => ({ workflows: [...workflows], total: workflows.length })),
    createIssueWorkflow: vi.fn(async (data: { name?: string; template_ref?: string }) => {
      const row = { id: `wf-${workflows.length + 1}`, name: data.name, template_ref: data.template_ref ?? null };
      workflows.push(row);
      return row;
    }),
    setSquadActiveWorkflow: vi.fn(async () => ({})),
    ...over,
  };
  return base as typeof base & Record<string, unknown>;
}

const run = (client: ReturnType<typeof fakeClient>, template: SquadTemplate = flowSquad, opts = options) =>
  runSquadQuickstart(client as unknown as SquadQuickstartClient, template, opts);

describe("runSquadQuickstart: the flow of a squad", () => {
  beforeEach(() => vi.clearAllMocks());

  it("creates one flow workflow, with the agents of its roles and its template_ref, and makes it the squad's workflow", async () => {
    const client = fakeClient();

    const out = await run(client);

    expect(client.createIssueWorkflow).toHaveBeenCalledTimes(1);
    const body = client.createIssueWorkflow.mock.calls[0]![0] as Record<string, any>;
    const idOf = (role: string) => out.agents.find((a) => a.role === role)!.id;
    expect(body.template_ref).toBe("flow_kv_demo@abcd1234");
    expect(body.name).toBe("Demo Desk · abcd1234");
    expect(body.flow_config).toEqual({ version: 1, switches: [{ key: "lookup", type: "bool", default: false }], messages: { stop1: "BLOCKED · stop 1\nbody" } });
    const steps = body.steps as Record<string, any>[];
    expect(steps.map((s) => s.status_key ?? s.new_status.name)).toEqual(["todo", "Collect", "Gate", "in_review", "blocked"]);
    expect(steps[1]!.tasks.map((t: any) => [t.key, t.handler])).toEqual([
      ["read", { type: "agent", id: idOf("analyst") }],
      ["extra", { type: "agent", id: idOf("analyst") }],
    ]);
    expect(steps[1]!.tasks[1].when).toEqual({ field: "switch.lookup", op: "eq", value: true });
    expect(steps[2]!.tasks[0]).toMatchObject({ kind: "gate", requires: ["outcome"], handler: { type: "agent", id: idOf("developer") } });
    expect(steps[2]!.transitions[1]).toMatchObject({ id: "again", to_status_key: "collect", rework: true });
    expect(steps[3]).toMatchObject({ status_key: "in_review", human_only: true, tasks: [] });
    expect(steps[0]!.handler).toEqual({ type: "none" });
    expect(client.setSquadActiveWorkflow).toHaveBeenCalledTimes(1);
    expect(client.setSquadActiveWorkflow).toHaveBeenCalledWith("squad-1", { workflow_id: "wf-1" });
    expect(out.workflow).toEqual({ status: "created" });
  });

  it("runs the flow even when the caller did not ask for the template's workflow", async () => {
    const client = fakeClient();
    await run(client, flowSquad, { runtimeId: "rt-1", createWorkflow: true });
    expect(client.createIssueWorkflow).toHaveBeenCalledTimes(1);
  });

  it("finds the workflow of a rerun by template_ref and creates nothing twice", async () => {
    const client = fakeClient();
    await run(client);
    const squad = { id: "squad-1", name: "Demo Desk", leader_id: "agent-1", instructions: "", active_workflow_id: "wf-1", archived_at: null };
    client.listSquads.mockResolvedValue([squad] as never[]);
    client.listAgents.mockImplementation(async () => (await Promise.all(client.createAgent.mock.results.map((r) => r.value))) as Agent[]);
    client.createIssueWorkflow.mockClear();
    client.setSquadActiveWorkflow.mockClear();

    const out = await run(client);

    expect(client.createIssueWorkflow).not.toHaveBeenCalled();
    expect(client.setSquadActiveWorkflow).not.toHaveBeenCalled();
    expect(out.workflow).toEqual({ status: "existing" });
  });

  it("finds the workflow by template_ref, not by its name", async () => {
    const client = fakeClient({
      listIssueWorkflows: vi.fn(async () => ({ workflows: [{ id: "wf-9", name: "Demo Desk · abcd1234", template_ref: null }], total: 1 })),
    });
    await run(client);
    expect(client.createIssueWorkflow).toHaveBeenCalledTimes(1);
  });

  it("leaves a squad that runs another workflow on it", async () => {
    const client = fakeClient();
    await run(client);
    client.listSquads.mockResolvedValue([{ id: "squad-1", name: "Demo Desk", leader_id: "agent-1", instructions: "", active_workflow_id: "wf-mine", archived_at: null }] as never[]);
    client.listAgents.mockImplementation(async () => (await Promise.all(client.createAgent.mock.results.map((r) => r.value))) as Agent[]);
    client.setSquadActiveWorkflow.mockClear();

    const out = await run(client);

    expect(client.setSquadActiveWorkflow).not.toHaveBeenCalled();
    expect(out.workflow).toEqual({ status: "existing" });
  });

  it("explains that a workspace admin must create the statuses, and sets nothing", async () => {
    const forbidden = Object.assign(new Error("forbidden"), { body: { code: "status_create_forbidden" } });
    const client = fakeClient({ createIssueWorkflow: vi.fn(async () => { throw forbidden; }) });

    const out = await run(client);

    expect(out.workflow.status).toBe("failed");
    expect(out.workflow.error).toBe("the flow needs a workspace admin to create its statuses: Collect, Gate");
    expect(client.setSquadActiveWorkflow).not.toHaveBeenCalled();
    expect(out.squad.status).toBe("created");
  });

  it("builds no workflow when a role has no agent", async () => {
    const client = fakeClient({
      createAgent: vi.fn(async (data: { name: string }) => {
        if (data.name === "SDLC Developer") throw new Error("no runtime");
        return { id: `agent-${data.name}`, name: data.name, archived_at: null };
      }),
    });

    const out = await run(client);

    expect(client.createIssueWorkflow).not.toHaveBeenCalled();
    expect(out.workflow).toEqual({ status: "failed", error: "a flow task has no agent to hand to" });
  });

  it("does nothing for a squad without a flow: SDLC keeps its own workflow behavior", async () => {
    const client = fakeClient();

    await run(client, greenfield, { runtimeId: "rt-1", createWorkflow: false });

    expect(client.createIssueWorkflow).not.toHaveBeenCalled();
    expect(client.setSquadActiveWorkflow).not.toHaveBeenCalled();
  });
});

describe("flowWorkflowName", () => {
  it("is the squad name and the revision, within 64 characters", () => {
    expect(flowWorkflowName("Demo Desk", "abcd1234")).toBe("Demo Desk · abcd1234");
    const long = flowWorkflowName("x".repeat(100), "abcd1234");
    expect([...long]).toHaveLength(64);
    expect(long.endsWith(" · abcd1234")).toBe(true);
  });
});
