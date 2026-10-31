import { packStoreZip, uint8ToArrayBuffer } from "../skills/pack-archive";
import type { CreateAgentRequest, SetAgentSkillsRequest } from "../types";
import type { AgentPack } from "./schema";

/** The API calls an install makes; the web client satisfies it as is. */
export interface AgentPackClient {
  listSkills(): Promise<{ id: string; name: string }[]>;
  importSkillArchive(file: File, onConflict: "fail"): Promise<{ id: string }>;
  listAgents(params?: { include_archived?: boolean }): Promise<
    { id: string; name: string; archived_at?: string | null }[]
  >;
  createAgent(data: CreateAgentRequest): Promise<{ id: string }>;
  listAgentSkills(agentId: string): Promise<{ id: string }[]>;
  addAgentSkills(agentId: string, data: SetAgentSkillsRequest): Promise<void>;
}

export type AgentPackItemStatus = "created" | "kept" | "failed";

export interface AgentPackOutcome {
  skills: {
    name: string;
    id: string | null;
    status: AgentPackItemStatus;
    error?: string;
  }[];
  agent: { id: string | null; status: AgentPackItemStatus; error?: string };
  /** "kept": every skill was already on the agent. "skipped": there is no agent to attach to. */
  attach: { status: "attached" | "kept" | "skipped" | "failed"; count: number; error?: string };
}

export interface AgentPackInstallOptions {
  /** Runtime for the agent when it has to be created. Null only when the agent already exists. */
  runtimeId: string | null;
}

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : "unknown error";
}

/**
 * Installs a pack into the workspace: skills the workspace lacks are imported,
 * the agent is created unless a live agent with its name exists, and the
 * skills it is missing are attached. Nothing that exists is edited, so running
 * it again after a partial failure finishes the job. A failed skill does not
 * stop the agent from being created with the others.
 */
export async function runAgentPackInstall(
  client: AgentPackClient,
  pack: AgentPack,
  options: AgentPackInstallOptions,
): Promise<AgentPackOutcome> {
  const agents = await client.listAgents({ include_archived: false });
  const existingAgent = agents.find((a) => !a.archived_at && a.name === pack.agent.name);
  const runtimeId = options.runtimeId;
  if (!existingAgent && !runtimeId) {
    throw new Error("Choose a runtime to create the agent on.");
  }

  const existingSkillIds = new Map(
    (await client.listSkills()).map((s) => [s.name, s.id] as const),
  );
  const skills: AgentPackOutcome["skills"] = [];
  for (const skill of pack.skills) {
    const existingId = existingSkillIds.get(skill.name);
    if (existingId) {
      skills.push({ name: skill.name, id: existingId, status: "kept" });
      continue;
    }
    try {
      const encoder = new TextEncoder();
      const zip = packStoreZip(
        skill.files.map((f) => ({ path: f.path, data: encoder.encode(f.content) })),
      );
      const file = new File([uint8ToArrayBuffer(zip)], `${skill.name}.skill`, {
        type: "application/zip",
      });
      const created = await client.importSkillArchive(file, "fail");
      skills.push({ name: skill.name, id: created.id, status: "created" });
    } catch (err) {
      skills.push({ name: skill.name, id: null, status: "failed", error: errorMessage(err) });
    }
  }

  let agent: AgentPackOutcome["agent"];
  if (existingAgent) {
    agent = { id: existingAgent.id, status: "kept" };
  } else {
    try {
      const created = await client.createAgent({
        name: pack.agent.name,
        description: pack.agent.description,
        instructions: pack.agent.instructions,
        runtime_id: runtimeId!,
        max_concurrent_tasks: pack.agent.max_concurrent_tasks,
      });
      agent = { id: created.id, status: "created" };
    } catch (err) {
      agent = { id: null, status: "failed", error: errorMessage(err) };
    }
  }

  let attach: AgentPackOutcome["attach"];
  if (!agent.id) {
    attach = { status: "skipped", count: 0 };
  } else {
    try {
      const attached = new Set((await client.listAgentSkills(agent.id)).map((s) => s.id));
      const missing = skills.flatMap((s) => (s.id && !attached.has(s.id) ? [s.id] : []));
      if (missing.length === 0) {
        attach = { status: "kept", count: 0 };
      } else {
        await client.addAgentSkills(agent.id, { skill_ids: missing });
        attach = { status: "attached", count: missing.length };
      }
    } catch (err) {
      attach = { status: "failed", count: 0, error: errorMessage(err) };
    }
  }

  return { skills, agent, attach };
}
