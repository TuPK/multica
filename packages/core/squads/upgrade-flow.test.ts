// @vitest-environment node
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Agent, IssueWorkflow, Squad } from "../types";
import type { CatalogFlow } from "../catalog/types";
import { SQUAD_ROLES, SQUAD_TEMPLATES, type SquadRoleKey, type SquadTemplate } from "./templates";
import { upgradeKitvietSquad, upgradeKitvietWorkspace, type KitvietUpgradeClient } from "./upgrade";

// The flow of the template below, standing in for the generated CATALOG_FLOWS.
const data = vi.hoisted(() => {
  const flowAt = (rev: string): CatalogFlow => ({
    id: "flow_kv_demo",
    version: "1.0.0",
    rev,
    squad: "kv_demo",
    config: { version: 1, switches: [] },
    steps: [
      { status: { key: "todo" }, statusKey: "todo", transitions: [{ id: "plan_ok", label: "Plan", when: null, to: "work" }] },
      {
        status: { name: "Work", category: "started" },
        statusKey: "work",
        tasks: [{ key: "do", title: "Do", kind: "work", role: "analyst", instructions: "Do it." }],
        transitions: [{ id: "next", label: "Next", when: null, to: "blocked" }],
      },
      { status: { key: "blocked" }, statusKey: "blocked" },
    ],
  });
  return { flowAt, current: { flow: flowAt("bbbb2222") } };
});

vi.mock("../catalog/flows", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../catalog/flows")>()),
  flowForSquad: (squadId: string) => (squadId === "kv_demo" ? data.current.flow : undefined),
}));

const greenfield = SQUAD_TEMPLATES.find((t) => t.id === "sdlc_greenfield")!;
const template = {
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
const previous = { agents: {}, squad: [] };
const idOf = (role: SquadRoleKey) => `a-${role}`;

type Row = Partial<IssueWorkflow> & { id: string };

/** A workspace with the squad (found by name or by template_key) and the workflow rows it has. */
function workspace(opts: { squad?: Partial<Squad>; workflows?: Row[] } = {}) {
  const workflows: Row[] = [...(opts.workflows ?? [])];
  const agents = template.roles.map((role) => ({
    id: idOf(role),
    name: SQUAD_ROLES[role].name,
    archived_at: null,
    instructions: "kept as is",
  }));
  const squad = {
    id: "kv-squad",
    name: template.name,
    archived_at: null,
    leader_id: idOf(template.leader),
    instructions: "kept as is",
    template_key: "kv_demo",
    active_workflow_id: null,
    ...opts.squad,
  };
  const client = {
    listAgents: vi.fn(async () => agents as unknown as Agent[]),
    listSquads: vi.fn(async () => [squad] as unknown as Squad[]),
    listSquadMembers: vi.fn(async () => [] as never[]),
    updateAgent: vi.fn(async () => ({}) as Agent),
    updateSquad: vi.fn(async () => ({}) as Squad),
    listSkills: vi.fn(async () => []),
    getSkill: vi.fn(),
    createSkill: vi.fn(),
    updateSkill: vi.fn(),
    listSquadSkills: vi.fn(async () => []),
    setSquadSkills: vi.fn(async () => []),
    addAgentSkills: vi.fn(async () => undefined),
    listIssueWorkflows: vi.fn(async () => ({ workflows: [...workflows], total: workflows.length })),
    createIssueWorkflow: vi.fn(async (body: { name?: string; template_ref?: string }) => {
      const row = { id: `wf-new-${workflows.length + 1}`, name: body.name, template_ref: body.template_ref ?? null };
      workflows.push(row);
      return row;
    }),
    updateIssueWorkflow: vi.fn(),
    setSquadActiveWorkflow: vi.fn(async (_id: string, body: { workflow_id: string }) => {
      squad.active_workflow_id = body.workflow_id;
      return {};
    }),
  };
  return { client: client as unknown as KitvietUpgradeClient & typeof client, squad };
}

describe("upgradeKitvietSquad: the flow", () => {
  beforeEach(() => {
    data.current.flow = data.flowAt("bbbb2222");
  });

  it("finds the squad by template_key when it was renamed", async () => {
    const { client } = workspace({ squad: { name: "My renamed desk" } });

    const result = await upgradeKitvietSquad(client, template, { previous });

    expect(result.squad.status).not.toBe("missing");
    expect(client.createIssueWorkflow).toHaveBeenCalledTimes(1);
  });

  it("still finds a squad with no template_key by its name", async () => {
    const { client } = workspace({ squad: { template_key: undefined } });
    const result = await upgradeKitvietSquad(client, template, { previous });
    expect(result.squad.status).not.toBe("missing");
  });

  it("is current, with no write, when the squad already runs this revision", async () => {
    const { client } = workspace({
      squad: { active_workflow_id: "wf-1" },
      workflows: [{ id: "wf-1", template_ref: "flow_kv_demo@bbbb2222" }],
    });

    const result = await upgradeKitvietSquad(client, template, { previous });

    expect(result.workflow).toEqual({ status: "current" });
    expect(client.createIssueWorkflow).not.toHaveBeenCalled();
    expect(client.setSquadActiveWorkflow).not.toHaveBeenCalled();
    expect(client.updateIssueWorkflow).not.toHaveBeenCalled();
  });

  it("makes a new row for a new revision and sets it, without editing the old row", async () => {
    const { client } = workspace({
      squad: { active_workflow_id: "wf-old" },
      workflows: [{ id: "wf-old", template_ref: "flow_kv_demo@aaaa1111" }],
    });

    const result = await upgradeKitvietSquad(client, template, { previous });

    expect(result.workflow).toEqual({ status: "upgraded" });
    expect(client.createIssueWorkflow).toHaveBeenCalledTimes(1);
    expect(client.createIssueWorkflow.mock.calls[0]![0]).toMatchObject({ template_ref: "flow_kv_demo@bbbb2222", name: "Demo Desk · bbbb2222", squad_id: "kv-squad" });
    expect(client.setSquadActiveWorkflow).toHaveBeenCalledTimes(1);
    expect(client.setSquadActiveWorkflow).toHaveBeenCalledWith("kv-squad", { workflow_id: "wf-new-2" });
    expect(client.updateIssueWorkflow).not.toHaveBeenCalled();
  });

  it("uses the row of the revision when the workspace already has it", async () => {
    const { client } = workspace({
      squad: { active_workflow_id: "wf-old" },
      workflows: [
        { id: "wf-old", template_ref: "flow_kv_demo@aaaa1111" },
        { id: "wf-have", template_ref: "flow_kv_demo@bbbb2222" },
      ],
    });

    await upgradeKitvietSquad(client, template, { previous });

    expect(client.createIssueWorkflow).not.toHaveBeenCalled();
    expect(client.setSquadActiveWorkflow).toHaveBeenCalledWith("kv-squad", { workflow_id: "wf-have" });
  });

  it("installs the flow on a squad that has no workflow yet", async () => {
    const { client } = workspace();

    const result = await upgradeKitvietSquad(client, template, { previous });

    expect(result.workflow).toEqual({ status: "upgraded" });
    expect(client.createIssueWorkflow).toHaveBeenCalledTimes(1);
    expect(client.setSquadActiveWorkflow).toHaveBeenCalledTimes(1);
  });

  it("keeps a workflow the template did not make, and says why", async () => {
    const { client } = workspace({
      squad: { active_workflow_id: "wf-mine" },
      workflows: [{ id: "wf-mine", template_ref: null }],
    });

    const result = await upgradeKitvietSquad(client, template, { previous });

    expect(result.workflow?.status).toBe("kept");
    expect(result.workflow?.detail).toMatch(/not made from this template/);
    expect(client.createIssueWorkflow).not.toHaveBeenCalled();
    expect(client.setSquadActiveWorkflow).not.toHaveBeenCalled();
  });

  it("changes nothing the second time", async () => {
    const { client } = workspace({
      squad: { active_workflow_id: "wf-old" },
      workflows: [{ id: "wf-old", template_ref: "flow_kv_demo@aaaa1111" }],
    });
    await upgradeKitvietSquad(client, template, { previous });
    client.createIssueWorkflow.mockClear();
    client.setSquadActiveWorkflow.mockClear();

    const again = await upgradeKitvietSquad(client, template, { previous });

    expect(again.workflow).toEqual({ status: "current" });
    expect(client.createIssueWorkflow).not.toHaveBeenCalled();
    expect(client.setSquadActiveWorkflow).not.toHaveBeenCalled();
  });

  it("writes nothing on a dry run, and still reports what it would do", async () => {
    const { client } = workspace({ squad: { active_workflow_id: "wf-old" }, workflows: [{ id: "wf-old", template_ref: "flow_kv_demo@aaaa1111" }] });

    const result = await upgradeKitvietSquad(client, template, { previous, dryRun: true });

    expect(result.workflow).toEqual({ status: "upgraded" });
    expect(client.createIssueWorkflow).not.toHaveBeenCalled();
    expect(client.setSquadActiveWorkflow).not.toHaveBeenCalled();
  });

  it("names the admin the new statuses need, and reports the failure", async () => {
    const { client } = workspace();
    client.createIssueWorkflow.mockRejectedValueOnce(Object.assign(new Error("forbidden"), { body: { code: "status_create_forbidden" } }));

    const result = await upgradeKitvietSquad(client, template, { previous });

    expect(result.workflow?.status).toBe("failed");
    expect(result.workflow?.detail).toMatch(/workspace admin/);
    expect(client.setSquadActiveWorkflow).not.toHaveBeenCalled();
  });

  it("leaves the workflow alone for a hand-made squad led by another agent", async () => {
    const { client } = workspace({ squad: { leader_id: "someone-else" } });

    const result = await upgradeKitvietSquad(client, template, { previous });

    expect(result.squad.status).toBe("kept");
    expect(result.workflow).toBeUndefined();
    expect(client.createIssueWorkflow).not.toHaveBeenCalled();
  });

  it("reports nothing about a workflow for a squad without a flow", async () => {
    const { client } = workspace();
    const plain = { ...template, id: "kv_plain" } as unknown as SquadTemplate;

    const result = await upgradeKitvietSquad(client, plain, { previous });

    expect(result.workflow).toBeUndefined();
    expect(client.listIssueWorkflows).not.toHaveBeenCalled();
  });

  it("is part of the whole-workspace upgrade", async () => {
    const { client } = workspace();
    const entries = await upgradeKitvietWorkspace(client, [template], { previous });
    expect(entries[0]!.result.workflow).toEqual({ status: "upgraded" });
  });
});
