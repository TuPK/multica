// @vitest-environment node
import { describe, expect, it, vi } from "vitest";
import type { Agent } from "../types";
import { instructionsFor, runSquadQuickstart, KITVIET_SKILL_SOURCE, SDLC_SKILL_SOURCE, type SquadQuickstartClient } from "./quickstart";
import { SQUAD_ROLES, SQUAD_TEMPLATES, type SquadTemplate } from "./templates";
import { KITVIET_AGENTS as KITVIET_AGENTS_ADMIN } from "./kitviet-skills-data/admin";

// The Kit Viet skill packs are faked at the loader: the tests need a few skills, not the real packs.
// The loader records the pack each call asks for, so a test can tell which pack a squad loads.
const kv = vi.hoisted(() => {
  const bundle = (name: string) => ({
    name,
    description: `${name}: a Kit Viet skill`,
    content: `---\nname: ${name}\n---\nbody of ${name}`,
    files: [] as { path: string; content: string }[],
  });
  return { bundle, loads: [] as (string | undefined)[] };
});

// Only the skill packs are faked. The agent instruction loader is the real one, so the agent test
// below creates the Office Admin agents with their generated bodies. The chunk-level loading is
// tested against the real loader in kitviet-packs.test.ts.
vi.mock("./kitviet-packs", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./kitviet-packs")>()),
  loadKitvietSkillPacks: async (pack: string | undefined) => {
    kv.loads.push(pack);
    return [
      { KITVIET_BUILTIN_SKILLS: [kv.bundle("kv--viet-email"), kv.bundle("kv--squad-lead")], KITVIET_AGENTS: [] },
      // The colon form, as the upstream writes the name in a reference.
      { KITVIET_BUILTIN_SKILLS: [kv.bundle("kv:dev-extra")], KITVIET_AGENTS: [] },
    ];
  },
}));

const greenfield = SQUAD_TEMPLATES.find((t) => t.id === "sdlc_greenfield")!;
const options = { runtimeId: "rt-1", createWorkflow: false };

// A Kit Viet squad: the Office Admin template id (so its pack is `admin`), no floor (so no seat
// calls), the common skill for the squad, the leader's own skill, and one role's skill.
const kvSquad: SquadTemplate = {
  ...greenfield,
  id: "kv_admin",
  floorName: null,
  builtinSkills: ["kv--viet-email"],
  leaderSkills: ["kv--squad-lead"],
  roleSkills: { developer: ["kv:dev-extra"] },
};

type StoredSkill = {
  id: string;
  name: string;
  description: string;
  content: string;
  files: { path: string; content: string }[];
  config: Record<string, unknown>;
};

/** A workspace fake that keeps the skills it creates, so a second run sees them. */
function fakeClient(over: Record<string, unknown> = {}) {
  let n = 0;
  const skills: StoredSkill[] = [];
  const base = {
    listAgents: vi.fn(async () => [] as Agent[]),
    createAgent: vi.fn(async (data: { name: string }) => ({
      id: `agent-${++n}`,
      name: data.name,
      archived_at: null,
    })),
    listSquads: vi.fn(async () => [] as never[]),
    createSquad: vi.fn(async (data: { name: string; leader_id: string }) => ({
      id: "squad-1",
      name: data.name,
      leader_id: data.leader_id,
    })),
    updateSquad: vi.fn(async () => ({})),
    updateSquadCharter: vi.fn(async () => ({})),
    addSquadMember: vi.fn(async () => ({})),
    listAutopilots: vi.fn(async () => ({ autopilots: [], total: 0 })),
    createAutopilot: vi.fn(async () => ({ id: "ap-1" })),
    createAutopilotTrigger: vi.fn(async () => ({})),
    updateAutopilot: vi.fn(async () => ({})),
    createSquadRoutine: vi.fn(async () => ({})),
    listSkills: vi.fn(async () =>
      skills.map(({ id, name, description }) => ({ id, name, description })),
    ),
    getSkill: vi.fn(async (id: string) => skills.find((s) => s.id === id)),
    createSkill: vi.fn(
      async (data: Omit<StoredSkill, "id" | "files"> & { files?: StoredSkill["files"] }) => {
        const skill = { id: `skill-${data.name}`, files: [], ...data };
        skills.push(skill);
        return skill;
      },
    ),
    updateSkill: vi.fn(async (id: string, data: Partial<StoredSkill>) => {
      Object.assign(skills.find((s) => s.id === id)!, data);
      return {};
    }),
    listSquadSkills: vi.fn(async () => [] as never[]),
    setSquadSkills: vi.fn(async () => []),
    listSquadMembers: vi.fn(async () => [] as never[]),
    addAgentSkills: vi.fn(async (_agentId: string, _data: unknown) => undefined),
    setAgentSkills: vi.fn(async () => undefined),
    ...over,
  };
  return base as typeof base & Record<string, unknown>;
}

const run = (client: ReturnType<typeof fakeClient>, template: SquadTemplate) =>
  runSquadQuickstart(client as unknown as SquadQuickstartClient, template, options);

describe("runSquadQuickstart Kit Viet skills: loading", () => {
  it("does not load the Kit Viet packs for an SDLC squad, and loads them on the first Kit Viet skill", async () => {
    // A fresh module, so the packs are not already loaded by an earlier test.
    vi.resetModules();
    const fresh = await import("./quickstart");
    const before = kv.loads.length;

    await fresh.runSquadQuickstart(fakeClient() as unknown as SquadQuickstartClient, greenfield, options);
    expect(kv.loads.length).toBe(before);

    await fresh.runSquadQuickstart(fakeClient() as unknown as SquadQuickstartClient, kvSquad, options);
    expect(kv.loads.length).toBe(before + 1);
    // The squad's own pack, not the others.
    expect(kv.loads.at(-1)).toBe("admin");
  });

  it("imports the Kit Viet skill under its name, tagged kitviet, and attaches it to the squad", async () => {
    const client = fakeClient();

    const out = await run(client, kvSquad);

    expect(client.createSkill).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "kv--viet-email",
        config: { source: KITVIET_SKILL_SOURCE },
      }),
    );
    expect(client.setSquadSkills).toHaveBeenCalledWith("squad-1", ["skill-kv--viet-email"]);
    expect(out.skills).toEqual({ status: "ready" });
  });

  it("still imports SDLC skills from the SDLC bundle, tagged sdlc_built_in, next to Kit Viet ones", async () => {
    const client = fakeClient();
    const mixed = { ...kvSquad, builtinSkills: [greenfield.builtinSkills[0]!, "kv--viet-email"] };

    const out = await run(client, mixed);

    expect(client.createSkill).toHaveBeenCalledWith(
      expect.objectContaining({ name: greenfield.builtinSkills[0], config: { source: SDLC_SKILL_SOURCE } }),
    );
    expect(out.skills).toEqual({ status: "ready" });
  });

  it("names a Kit Viet skill that no pack holds, and imports the rest", async () => {
    const client = fakeClient();
    const missing = { ...kvSquad, builtinSkills: ["kv--not-there", "kv--viet-email"] };

    const out = await run(client, missing);

    expect(out.skills.status).toBe("partial");
    expect(out.skills.error).toBe("kv--not-there: not a built-in skill");
    expect(client.setSquadSkills).toHaveBeenCalledWith("squad-1", ["skill-kv--viet-email"]);
  });
});

describe("runSquadQuickstart Kit Viet skills: per agent", () => {
  it("attaches the leader's skills to the leader and a role's skills to that role, with addAgentSkills", async () => {
    const client = fakeClient();

    const out = await run(client, kvSquad);

    const idOf = (role: string) => out.agents.find((a) => a.role === role)!.id;
    expect(client.addAgentSkills).toHaveBeenCalledTimes(2);
    expect(client.addAgentSkills).toHaveBeenCalledWith(idOf("lead"), { skill_ids: ["skill-kv--squad-lead"] });
    expect(client.addAgentSkills).toHaveBeenCalledWith(idOf("developer"), { skill_ids: ["skill-kv:dev-extra"] });
    expect(client.setAgentSkills).not.toHaveBeenCalled();
    // The role skill is an agent's own skill, not the squad's.
    expect(client.setSquadSkills).toHaveBeenCalledWith("squad-1", ["skill-kv--viet-email"]);
  });

  it("gives a rerun the same agent skills and no second copy of any skill", async () => {
    const client = fakeClient();
    const first = await run(client, kvSquad);
    const skillsAfterFirst = client.createSkill.mock.calls.length;
    client.listAgents.mockResolvedValue(
      first.agents.map((a) => ({ id: a.id, name: a.name, archived_at: null }) as Agent),
    );
    client.listSquads.mockResolvedValue([
      { id: "squad-1", name: kvSquad.name, leader_id: "agent-1", instructions: kvSquad.instructions, archived_at: null } as never,
    ]);
    client.listSquadMembers.mockResolvedValue(
      first.agents
        .filter((a) => a.role !== kvSquad.leader)
        .map((a) => ({ member_type: "agent", member_id: a.id, role: "" }) as never),
    );

    const second = await run(client, kvSquad);

    expect(second.squad.status).toBe("existing");
    expect(client.createSkill).toHaveBeenCalledTimes(skillsAfterFirst);
    expect(client.updateSkill).not.toHaveBeenCalled();
    // The server keeps one row per skill, so the same ids again add nothing new.
    expect(client.addAgentSkills.mock.calls.slice(2)).toEqual(client.addAgentSkills.mock.calls.slice(0, 2));
    expect(second.skills).toEqual({ status: "ready" });
  });

  it("names the agent whose skills the server refuses, and keeps the squad's skills", async () => {
    const client = fakeClient({
      addAgentSkills: vi
        .fn()
        .mockResolvedValueOnce(undefined)
        .mockRejectedValueOnce(new Error("agent refused")),
    });

    const out = await run(client, kvSquad);

    expect(out.skills.status).toBe("partial");
    expect(out.skills.error).toBe("SDLC Developer: agent refused");
    expect(client.setSquadSkills).toHaveBeenCalledWith("squad-1", ["skill-kv--viet-email"]);
  });

  it("keeps the squad's other skills when it attaches the Kit Viet ones", async () => {
    const client = fakeClient({
      listSquadSkills: vi.fn(async () => [{ id: "skill-boss", name: "boss", description: "" }] as never[]),
    });

    await run(client, kvSquad);

    expect(client.setSquadSkills).toHaveBeenCalledWith("squad-1", ["skill-boss", "skill-kv--viet-email"]);
  });

  it("updates a Kit Viet skill an earlier run made when its text has changed", async () => {
    const stale: StoredSkill = {
      id: "skill-old",
      name: "kv--viet-email",
      description: "old",
      content: "old body",
      files: [],
      config: { source: KITVIET_SKILL_SOURCE },
    };
    const client = fakeClient({
      listSkills: vi.fn(async () => [{ id: stale.id, name: stale.name, description: stale.description }]),
      getSkill: vi.fn(async () => stale),
    });

    await run(client, kvSquad);

    expect(client.createSkill.mock.calls.map((c) => (c[0] as { name: string }).name)).not.toContain("kv--viet-email");
    expect(client.updateSkill).toHaveBeenCalledWith(
      "skill-old",
      expect.objectContaining({ content: expect.stringContaining("body of kv--viet-email") }),
    );
  });
});

describe("runSquadQuickstart Kit Viet agents: instructions", () => {
  it("creates each Office Admin agent with its header, kind text and generated body; the lead has no body", async () => {
    const admin = SQUAD_TEMPLATES.find((t) => t.id === "kv_admin")!;
    const bodies = new Map(KITVIET_AGENTS_ADMIN.map((agent) => [agent.name, agent.instructions]));
    const client = fakeClient();

    await run(client, admin);

    const created = new Map(
      client.createAgent.mock.calls.map((c) => {
        const data = c[0] as { name: string; instructions: string };
        return [data.name, data] as const;
      }),
    );
    expect(created.size).toBe(admin.roles.length);
    for (const role of admin.roles) {
      const name = SQUAD_ROLES[role].name;
      const instructions = created.get(name)?.instructions;
      expect(instructions, name).toBe(await instructionsFor(role));
      expect(instructions?.startsWith(`You are the ${name} in an Office Admin`), name).toBe(true);
      expect(instructions?.match(/^You are /gm)?.length ?? 0, name).toBe(1);
      const body = bodies.get(role);
      if (body === undefined) {
        expect(role, "only the lead has no body").toBe(admin.leader);
      } else {
        expect(instructions?.includes(body), name).toBe(true);
      }
    }
  });
});

describe("runSquadQuickstart Kit Viet skills: which agents get them", () => {
  const developerName = SQUAD_ROLES.developer.name;
  const leaderName = SQUAD_ROLES[kvSquad.leader].name;

  it("does not attach a role's skills to a same-named agent in a squad the quickstart did not make", async () => {
    // The boss's own developer agent and their own squad under the template's name.
    const client = fakeClient({
      listAgents: vi.fn(async () => [{ id: "boss-dev", name: developerName, archived_at: null }] as Agent[]),
      listSquads: vi.fn(async () => [
        { id: "hand", name: kvSquad.name, leader_id: "boss-lead", instructions: "mine", archived_at: null } as never,
      ]),
    });

    const out = await run(client, kvSquad);

    const granted = client.addAgentSkills.mock.calls.map((c) => c[0]);
    expect(granted).not.toContain("boss-dev");
    expect(out.agents.find((a) => a.role === "developer")).toMatchObject({ id: "boss-dev", status: "reused" });
    // The leader is made by this run, so it still gets its skill.
    expect(granted).toEqual([out.agents.find((a) => a.role === kvSquad.leader)!.id]);
    expect(out.skills).toEqual({ status: "skipped" });
  });

  it("does not attach skills to a reused agent that is not a member of the quickstart's squad", async () => {
    // The squad is the quickstart's (leader and instructions match), but the developer agent was
    // reused from outside it and never joined.
    const client = fakeClient({
      listAgents: vi.fn(async () => [{ id: "boss-dev", name: developerName, archived_at: null }] as Agent[]),
      listSquads: vi.fn(async () => [
        { id: "squad-1", name: kvSquad.name, leader_id: "agent-1", instructions: kvSquad.instructions, archived_at: null } as never,
      ]),
      listSquadMembers: vi.fn(async () => []),
    });

    await run(client, kvSquad);

    expect(client.addAgentSkills.mock.calls.map((c) => c[0])).not.toContain("boss-dev");
  });

  it("attaches skills to a reused agent that is a member of the quickstart's own squad", async () => {
    const client = fakeClient({
      listAgents: vi.fn(async () => [{ id: "member-dev", name: developerName, archived_at: null }] as Agent[]),
      listSquads: vi.fn(async () => [
        { id: "squad-1", name: kvSquad.name, leader_id: "agent-1", instructions: kvSquad.instructions, archived_at: null } as never,
      ]),
      listSquadMembers: vi.fn(async () => [{ member_type: "agent", member_id: "member-dev", role: "" }]),
    });

    await run(client, kvSquad);

    expect(client.addAgentSkills).toHaveBeenCalledWith("member-dev", { skill_ids: ["skill-kv:dev-extra"] });
  });

  it("names the members the quickstart could not read, and still attaches to the agents it made", async () => {
    const client = fakeClient({
      listAgents: vi.fn(async () => [{ id: "boss-dev", name: developerName, archived_at: null }] as Agent[]),
      listSquads: vi.fn(async () => [
        { id: "squad-1", name: kvSquad.name, leader_id: "agent-1", instructions: kvSquad.instructions, archived_at: null } as never,
      ]),
      listSquadMembers: vi.fn(async () => {
        throw new Error("members unavailable");
      }),
    });

    const out = await run(client, kvSquad);

    expect(out.skills.status).toBe("partial");
    expect(out.skills.error).toBe("members: members unavailable");
    expect(client.addAgentSkills.mock.calls.map((c) => c[0])).toEqual([out.agents.find((a) => a.role === kvSquad.leader)!.id]);
  });

  it("does not attach skills to a reused leader that leads a squad the quickstart did not make", async () => {
    const client = fakeClient({
      listAgents: vi.fn(async () => [{ id: "boss-lead", name: leaderName, archived_at: null }] as Agent[]),
      listSquads: vi.fn(async () => [
        { id: "hand", name: kvSquad.name, leader_id: "boss-lead", instructions: "mine", archived_at: null } as never,
      ]),
    });

    await run(client, kvSquad);

    expect(client.addAgentSkills.mock.calls.map((c) => c[0])).not.toContain("boss-lead");
  });
});

