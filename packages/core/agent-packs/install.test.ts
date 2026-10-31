import { describe, expect, it, vi } from "vitest";
import { runAgentPackInstall, type AgentPackClient } from "./install";
import type { AgentPack } from "./schema";

const pack: AgentPack = {
  id: "demo",
  agent: {
    name: "Demo Agent",
    description: "Demo description",
    instructions: "Do the demo.",
    max_concurrent_tasks: 2,
  },
  skills: [
    {
      name: "alpha",
      files: [
        { path: "SKILL.md", content: "---\nname: alpha\n---\n" },
        { path: "references/notes.md", content: "notes" },
      ],
    },
    {
      name: "beta",
      files: [{ path: "SKILL.md", content: "---\nname: beta\n---\n" }],
    },
  ],
};

function fakeClient(overrides: Partial<AgentPackClient> = {}): AgentPackClient {
  let nextId = 1;
  return {
    listSkills: vi.fn(async () => []),
    importSkillArchive: vi.fn(async (file: File) => ({
      id: `skill-${nextId++}`,
      name: file.name.replace(/\.skill$/, ""),
    })),
    listAgents: vi.fn(async () => []),
    createAgent: vi.fn(async () => ({ id: "agent-1" })),
    listAgentSkills: vi.fn(async () => []),
    addAgentSkills: vi.fn(async () => {}),
    ...overrides,
  };
}

describe("runAgentPackInstall", () => {
  it("imports skills, creates the agent and attaches the skills on a fresh workspace", async () => {
    const client = fakeClient();

    const outcome = await runAgentPackInstall(client, pack, { runtimeId: "rt-1" });

    expect(outcome.skills).toEqual([
      { name: "alpha", id: "skill-1", status: "created" },
      { name: "beta", id: "skill-2", status: "created" },
    ]);
    expect(outcome.agent).toEqual({ id: "agent-1", status: "created" });
    expect(outcome.attach).toEqual({ status: "attached", count: 2 });
    expect(client.createAgent).toHaveBeenCalledWith({
      name: "Demo Agent",
      description: "Demo description",
      instructions: "Do the demo.",
      runtime_id: "rt-1",
      max_concurrent_tasks: 2,
    });
    expect(client.addAgentSkills).toHaveBeenCalledWith("agent-1", {
      skill_ids: ["skill-1", "skill-2"],
    });
  });

  it("uploads each skill as a zip archive named after the skill", async () => {
    const client = fakeClient();

    await runAgentPackInstall(client, pack, { runtimeId: "rt-1" });

    const [file, onConflict] = vi.mocked(client.importSkillArchive).mock.calls[0]!;
    expect(file.name).toBe("alpha.skill");
    const bytes = new Uint8Array(await file.arrayBuffer());
    expect(Array.from(bytes.slice(0, 4))).toEqual([0x50, 0x4b, 0x03, 0x04]);
    expect(onConflict).toBe("fail");
  });

  it("keeps everything that already exists and writes nothing on a second run", async () => {
    const client = fakeClient({
      listSkills: vi.fn(async () => [
        { id: "skill-a", name: "alpha" },
        { id: "skill-b", name: "beta" },
      ]),
      listAgents: vi.fn(async () => [{ id: "agent-1", name: "Demo Agent", archived_at: null }]),
      listAgentSkills: vi.fn(async () => [{ id: "skill-a" }, { id: "skill-b" }]),
    });

    const outcome = await runAgentPackInstall(client, pack, { runtimeId: null });

    expect(outcome.skills.map((s) => s.status)).toEqual(["kept", "kept"]);
    expect(outcome.agent).toEqual({ id: "agent-1", status: "kept" });
    expect(outcome.attach).toEqual({ status: "kept", count: 0 });
    expect(client.importSkillArchive).not.toHaveBeenCalled();
    expect(client.createAgent).not.toHaveBeenCalled();
    expect(client.addAgentSkills).not.toHaveBeenCalled();
  });

  it("reuses an existing agent and attaches only the skills it is missing", async () => {
    const client = fakeClient({
      listSkills: vi.fn(async () => [{ id: "skill-a", name: "alpha" }]),
      listAgents: vi.fn(async () => [{ id: "agent-1", name: "Demo Agent", archived_at: null }]),
      listAgentSkills: vi.fn(async () => [{ id: "skill-a" }]),
    });

    const outcome = await runAgentPackInstall(client, pack, { runtimeId: null });

    expect(outcome.agent.status).toBe("kept");
    expect(client.createAgent).not.toHaveBeenCalled();
    expect(outcome.attach).toEqual({ status: "attached", count: 1 });
    expect(client.addAgentSkills).toHaveBeenCalledWith("agent-1", { skill_ids: ["skill-1"] });
  });

  it("ignores an archived agent with the same name and creates a fresh one", async () => {
    const client = fakeClient({
      listAgents: vi.fn(async () => [
        { id: "old", name: "Demo Agent", archived_at: "2026-01-01T00:00:00Z" },
      ]),
    });

    const outcome = await runAgentPackInstall(client, pack, { runtimeId: "rt-1" });

    expect(outcome.agent).toEqual({ id: "agent-1", status: "created" });
  });

  it("keeps going when one skill fails to import and attaches the rest", async () => {
    const client = fakeClient({
      importSkillArchive: vi.fn(async (file: File) => {
        if (file.name === "alpha.skill") throw new Error("413 too large");
        return { id: "skill-2", name: "beta" };
      }),
    });

    const outcome = await runAgentPackInstall(client, pack, { runtimeId: "rt-1" });

    expect(outcome.skills[0]).toEqual({
      name: "alpha",
      id: null,
      status: "failed",
      error: "413 too large",
    });
    expect(outcome.skills[1]).toEqual({ name: "beta", id: "skill-2", status: "created" });
    expect(outcome.agent.status).toBe("created");
    expect(client.addAgentSkills).toHaveBeenCalledWith("agent-1", { skill_ids: ["skill-2"] });
  });

  it("reports a failed agent creation without attaching skills", async () => {
    const client = fakeClient({
      createAgent: vi.fn(async () => {
        throw new Error("runtime offline");
      }),
    });

    const outcome = await runAgentPackInstall(client, pack, { runtimeId: "rt-1" });

    expect(outcome.agent).toEqual({ id: null, status: "failed", error: "runtime offline" });
    expect(outcome.attach).toEqual({ status: "skipped", count: 0 });
    expect(client.addAgentSkills).not.toHaveBeenCalled();
  });

  it("refuses to start when a new agent needs a runtime and none was chosen", async () => {
    const client = fakeClient();

    await expect(runAgentPackInstall(client, pack, { runtimeId: null })).rejects.toThrow(
      "runtime",
    );
    expect(client.importSkillArchive).not.toHaveBeenCalled();
    expect(client.createAgent).not.toHaveBeenCalled();
  });
});
