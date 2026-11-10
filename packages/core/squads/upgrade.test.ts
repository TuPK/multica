// @vitest-environment node
import { describe, expect, it, vi } from "vitest";
import type { Agent, IssueWorkflow, Squad } from "../types";
import {
  SDLC_PREVIOUS_AGENT_INSTRUCTIONS,
  SDLC_PREVIOUS_SQUAD_INSTRUCTIONS,
  SDLC_PREVIOUS_STEP_INSTRUCTIONS,
} from "./sdlc-previous-texts";
import { instructionsFor, workflowSteps } from "./quickstart";
import { mergeWorkflowSteps, upgradeKitvietSquad, upgradeSdlcSquad, type KitvietUpgradeClient, type SdlcUpgradeClient } from "./upgrade";
import { SQUAD_ROLES, SQUAD_TEMPLATES, type SquadRoleKey } from "./templates";

const template = SQUAD_TEMPLATES.find((t) => t.id === "sdlc_greenfield")!;
const agentIds = Object.fromEntries(template.roles.map((r) => [r, `id-${r}`])) as Record<SquadRoleKey, string>;

/** The workflow as an earlier release stored it: no Environment step, earlier briefs. */
function oldWorkflow(overrides: Record<string, string> = {}): IssueWorkflow {
  const current = workflowSteps(template.workflow!.steps, agentIds)!;
  const steps = current
    .filter((s) => !("new_status" in s && s.new_status.name === "Environment"))
    .map((s) => {
      const key = "status_key" in s ? s.status_key : s.new_status.name.toLowerCase().replaceAll(" ", "_");
      const previous = SDLC_PREVIOUS_STEP_INSTRUCTIONS[key] ?? SDLC_PREVIOUS_STEP_INSTRUCTIONS[
        "new_status" in s ? s.new_status.name : key
      ];
      return {
        status_key: key,
        handler: s.handler ?? { type: "none" as const },
        instructions: overrides[key] ?? previous?.[0] ?? s.instructions ?? "",
        ...(s.next_status_key ? { next_status_key: s.next_status_key } : {}),
        ...(s.back_status_key ? { back_status_key: s.back_status_key } : {}),
      };
    });
  return { id: "wf", name: template.workflow!.name, steps } as unknown as IssueWorkflow;
}

function fakeClient(opts: { workflow: IssueWorkflow; agentInstructions?: Partial<Record<SquadRoleKey, string>>; squadInstructions?: string }) {
  const agents = template.roles.map((role) => ({
    id: agentIds[role],
    name: SQUAD_ROLES[role].name,
    archived_at: null,
    instructions: opts.agentInstructions?.[role] ?? SDLC_PREVIOUS_AGENT_INSTRUCTIONS[role]![0]!,
  }));
  const client = {
    listAgents: vi.fn(async () => agents as unknown as Agent[]),
    listSquads: vi.fn(async () => [
      { id: "sq", name: template.name, archived_at: null, instructions: opts.squadInstructions ?? SDLC_PREVIOUS_SQUAD_INSTRUCTIONS[0]! },
    ] as unknown as Squad[]),
    listIssueWorkflows: vi.fn(async () => ({ workflows: [opts.workflow], total: 1 })),
    updateAgent: vi.fn(async () => ({}) as Agent),
    updateSquad: vi.fn(async () => ({}) as Squad),
    updateIssueWorkflow: vi.fn(async () => opts.workflow),
    dryRunIssueWorkflowUpdate: vi.fn(async () => ({})),
    listSkills: vi.fn(async () => []),
    createSkill: vi.fn(),
    updateSkill: vi.fn(),
    getSkill: vi.fn(),
    listSquadSkills: vi.fn(async () => []),
    setSquadSkills: vi.fn(async () => []),
  };
  return client as unknown as SdlcUpgradeClient & typeof client;
}

describe("mergeWorkflowSteps", () => {
  const wanted = workflowSteps(template.workflow!.steps, agentIds)!;

  it("adds the Environment step before Stage done, keeps handlers, and replaces a known earlier brief", () => {
    const merged = mergeWorkflowSteps(oldWorkflow(), wanted);
    const keys = merged.steps.map((s) => ("status_key" in s ? s.status_key : s.new_status.name));
    expect(merged.added).toEqual(["environment"]);
    expect(keys.indexOf("Environment")).toBe(keys.indexOf("stage_done") - 1);
    expect(merged.updated).toContain("blocked");
    const blocked = merged.steps.find((s) => "status_key" in s && s.status_key === "blocked")!;
    expect(blocked.instructions).toContain("sdlc-squad-lead");
    expect(blocked.handler).toEqual({ type: "agent", id: "id-lead" });
  });

  it("keeps a brief the boss edited and says so", () => {
    const merged = mergeWorkflowSteps(oldWorkflow({ in_review: "Review it my way." }), wanted);
    expect(merged.kept).toContain("in_review");
    const review = merged.steps.find((s) => "status_key" in s && s.status_key === "in_review")!;
    expect(review.instructions).toBe("Review it my way.");
  });

  it("changes nothing the second time", () => {
    const first = mergeWorkflowSteps(oldWorkflow(), wanted);
    const stored = {
      ...oldWorkflow(),
      steps: first.steps.map((s) => ({
        status_key: "status_key" in s ? s.status_key : s.new_status.name.toLowerCase().replaceAll(" ", "_"),
        handler: s.handler,
        instructions: s.instructions,
        next_status_key: s.next_status_key,
        back_status_key: s.back_status_key,
      })),
    } as unknown as IssueWorkflow;
    const second = mergeWorkflowSteps(stored, wanted);
    expect(second.added).toEqual([]);
    expect(second.updated).toEqual([]);
  });
});

describe("upgradeSdlcSquad", () => {
  it("upgrades agents, squad and workflow that still hold an earlier text", async () => {
    const client = fakeClient({ workflow: oldWorkflow() });
    const result = await upgradeSdlcSquad(client, template);
    expect(result.agents.every((a) => a.status === "upgraded" || a.status === "current")).toBe(true);
    expect(client.updateAgent).toHaveBeenCalledWith("id-lead", { instructions: await instructionsFor("lead") });
    // The squad text did not change in this release, so it is already current.
    expect(result.squad.status).toBe("current");
    expect(result.workflow.status).toBe("upgraded");
    expect(client.updateIssueWorkflow).toHaveBeenCalledTimes(1);
  });

  it("keeps an agent or squad text the boss wrote", async () => {
    const client = fakeClient({
      workflow: oldWorkflow(),
      agentInstructions: { developer: "My own developer rules." },
      squadInstructions: "My squad rules.",
    });
    const result = await upgradeSdlcSquad(client, template);
    expect(result.agents.find((a) => a.role === "developer")!.status).toBe("kept");
    expect(result.squad.status).toBe("kept");
    expect(client.updateAgent).not.toHaveBeenCalledWith("id-developer", expect.anything());
    expect(client.updateSquad).not.toHaveBeenCalled();
  });

  it("writes nothing on a dry run", async () => {
    const client = fakeClient({ workflow: oldWorkflow() });
    const result = await upgradeSdlcSquad(client, template, { dryRun: true });
    expect(client.updateAgent).not.toHaveBeenCalled();
    expect(client.updateSquad).not.toHaveBeenCalled();
    expect(client.updateIssueWorkflow).not.toHaveBeenCalled();
    expect(client.dryRunIssueWorkflowUpdate).toHaveBeenCalledTimes(1);
    expect(result.workflow.added).toEqual(["environment"]);
  });

  it("reports a status a workspace admin must allow", async () => {
    const client = fakeClient({ workflow: oldWorkflow() });
    client.updateIssueWorkflow.mockRejectedValueOnce(Object.assign(new Error("forbidden"), { body: { code: "status_create_forbidden" } }));
    const result = await upgradeSdlcSquad(client, template);
    expect(result.workflow.status).toBe("failed");
    expect(result.workflow.detail).toContain("admin");
  });
});

describe("upgradeSdlcSquad for the sales loop", () => {
  const sales = SQUAD_TEMPLATES.find((t) => t.id === "sales_loop")!;

  async function salesClient(overrides: { scoutInstructions?: string; squadInstructions?: string } = {}) {
    const agents = await Promise.all(
      sales.roles.map(async (role, i) => ({
        id: `sales-${i}`,
        name: SQUAD_ROLES[role].name,
        archived_at: null,
        instructions: role === "sales_scout" && overrides.scoutInstructions !== undefined ? overrides.scoutInstructions : await instructionsFor(role),
      })),
    );
    const client = {
      listAgents: vi.fn(async () => agents as unknown as Agent[]),
      listSquads: vi.fn(async () => [
        { id: "sq-sales", name: sales.name, archived_at: null, instructions: overrides.squadInstructions ?? sales.instructions },
      ] as unknown as Squad[]),
      listIssueWorkflows: vi.fn(async () => ({ workflows: [], total: 0 })),
      updateAgent: vi.fn(async () => ({}) as Agent),
      updateSquad: vi.fn(async () => ({}) as Squad),
      updateIssueWorkflow: vi.fn(),
      dryRunIssueWorkflowUpdate: vi.fn(async () => ({})),
      listSkills: vi.fn(async () => []),
      createSkill: vi.fn(async (data: { name: string }) => ({ id: `skill-${data.name}`, name: data.name })),
      updateSkill: vi.fn(),
      getSkill: vi.fn(),
      listSquadSkills: vi.fn(async () => []),
      setSquadSkills: vi.fn(async () => []),
    };
    return client as unknown as SdlcUpgradeClient & typeof client;
  }

  it("finds the sales squad and its agents current, and imports the sales skills", async () => {
    const client = await salesClient();
    const result = await upgradeSdlcSquad(client, sales);
    expect(result.squad.status).toBe("current");
    expect(result.agents.every((a) => a.status === "current")).toBe(true);
    expect(client.updateAgent).not.toHaveBeenCalled();
    expect(result.skills.status).toBe("ready");
    expect(client.createSkill).toHaveBeenCalledTimes(sales.builtinSkills.length);
  });

  it("keeps a sales agent's instructions the boss edited", async () => {
    const client = await salesClient({ scoutInstructions: "Scout, my own rules." });
    const result = await upgradeSdlcSquad(client, sales);
    expect(result.agents.find((a) => a.role === "sales_scout")!.status).toBe("kept");
    expect(client.updateAgent).not.toHaveBeenCalledWith("sales-1", expect.anything());
  });
});

describe("upgradeKitvietSquad", () => {
  const kvTemplate = SQUAD_TEMPLATES.find((t) => t.id === "kv_admin")!;
  const kvIds = Object.fromEntries(kvTemplate.roles.map((r) => [r, `kv-${r}`])) as Record<SquadRoleKey, string>;
  const drafter = kvTemplate.roles.find((r) => r !== kvTemplate.leader)!;
  const previous = {
    agents: { [drafter]: ["the drafter text of an earlier release"] },
    squad: ["the squad text of an earlier release"],
  };

  /** A workspace where the Kit Viet squad exists, led by the template's leader, with the given texts. */
  async function kvWorkspace(
    opts: { agentText?: Partial<Record<SquadRoleKey, string>>; squadText?: string; leaderId?: string; members?: string[] } = {},
  ) {
    const agents = await Promise.all(
      kvTemplate.roles.map(async (role) => ({
        id: kvIds[role],
        name: SQUAD_ROLES[role].name,
        archived_at: null,
        instructions: opts.agentText?.[role] ?? (await instructionsFor(role)),
      })),
    );
    const client = {
      listAgents: vi.fn(async () => agents as unknown as Agent[]),
      listSquads: vi.fn(async () => [
        {
          id: "kv-squad",
          name: kvTemplate.name,
          archived_at: null,
          leader_id: opts.leaderId ?? kvIds[kvTemplate.leader],
          instructions: opts.squadText ?? kvTemplate.instructions,
        },
      ] as unknown as Squad[]),
      listSquadMembers: vi.fn(async () =>
        (opts.members ?? []).map((member_id) => ({ member_type: "agent", member_id, role: "" })),
      ),
      updateAgent: vi.fn(async () => ({}) as Agent),
      updateSquad: vi.fn(async () => ({}) as Squad),
      listSkills: vi.fn(async () => []),
      getSkill: vi.fn(),
      createSkill: vi.fn(async (data: { name: string }) => ({ id: `s-${data.name}` })),
      updateSkill: vi.fn(),
      listSquadSkills: vi.fn(async () => []),
      setSquadSkills: vi.fn(async () => []),
      addAgentSkills: vi.fn(async (_agentId: string, _data: unknown) => undefined),
    };
    return client as unknown as KitvietUpgradeClient & typeof client;
  }

  it("upgrades an agent that still holds an earlier text, and finds the current ones current", async () => {
    const client = await kvWorkspace({ agentText: { [drafter]: previous.agents[drafter]![0]! } });

    const result = await upgradeKitvietSquad(client, kvTemplate, { previous });

    expect(client.updateAgent).toHaveBeenCalledWith(kvIds[drafter], { instructions: await instructionsFor(drafter) });
    expect(result.agents.find((a) => a.role === drafter)!.status).toBe("upgraded");
    expect(result.agents.find((a) => a.role === kvTemplate.leader)!.status).toBe("current");
    expect(client.updateAgent).toHaveBeenCalledTimes(1);
  });

  it("keeps an agent text the boss wrote, and says so", async () => {
    const client = await kvWorkspace({ agentText: { [drafter]: "My own drafter rules." } });

    const result = await upgradeKitvietSquad(client, kvTemplate, { previous });

    expect(client.updateAgent).not.toHaveBeenCalled();
    expect(result.agents.find((a) => a.role === drafter)).toMatchObject({ status: "kept" });
  });

  it("keeps every changed agent when no earlier text is recorded for it", async () => {
    const client = await kvWorkspace({ agentText: { [drafter]: previous.agents[drafter]![0]! } });

    const result = await upgradeKitvietSquad(client, kvTemplate);

    expect(client.updateAgent).not.toHaveBeenCalled();
    expect(result.agents.find((a) => a.role === drafter)!.status).toBe("kept");
  });

  it("upgrades the squad text it recorded, but never a squad another agent leads", async () => {
    const mine = await kvWorkspace({ squadText: previous.squad[0] });
    await upgradeKitvietSquad(mine, kvTemplate, { previous });
    expect(mine.updateSquad).toHaveBeenCalledWith("kv-squad", { instructions: kvTemplate.instructions });

    const theirs = await kvWorkspace({ squadText: previous.squad[0], leaderId: "someone-else" });
    const result = await upgradeKitvietSquad(theirs, kvTemplate, { previous });
    expect(theirs.updateSquad).not.toHaveBeenCalled();
    expect(result.squad.status).toBe("kept");
  });

  it("writes nothing on a dry run", async () => {
    const client = await kvWorkspace({
      agentText: { [drafter]: previous.agents[drafter]![0]! },
      squadText: previous.squad[0],
    });

    const result = await upgradeKitvietSquad(client, kvTemplate, { dryRun: true, previous });

    expect(result.agents.find((a) => a.role === drafter)!.status).toBe("upgraded");
    expect(client.updateAgent).not.toHaveBeenCalled();
    expect(client.updateSquad).not.toHaveBeenCalled();
    expect(client.createSkill).not.toHaveBeenCalled();
    expect(client.addAgentSkills).not.toHaveBeenCalled();
  });

  it("attaches the agent skills only to the leader and the squad's members", async () => {
    const client = await kvWorkspace({ members: [kvIds[drafter]] });

    const result = await upgradeKitvietSquad(client, kvTemplate, { previous });

    const granted = client.addAgentSkills.mock.calls.map((c) => c[0]);
    expect(granted.length).toBeGreaterThan(0);
    for (const id of granted) {
      expect([kvIds[kvTemplate.leader], kvIds[drafter]]).toContain(id);
    }
    expect(result.skills.status).toBe("ready");
  });

  it("changes nothing the second time", async () => {
    const first = await kvWorkspace({
      agentText: { [drafter]: previous.agents[drafter]![0]! },
      squadText: previous.squad[0],
    });
    await upgradeKitvietSquad(first, kvTemplate, { previous });

    const second = await kvWorkspace();
    const result = await upgradeKitvietSquad(second, kvTemplate, { previous });

    expect(result.agents.every((a) => a.status === "current" || a.status === "missing")).toBe(true);
    expect(second.updateAgent).not.toHaveBeenCalled();
    expect(second.updateSquad).not.toHaveBeenCalled();
  });
});

