import type {
  PackAgentAction,
  PackAgentChoice,
  PackCommitRequest,
  PackPreview,
  PackSkillConflictChoice,
} from "@multica/core/types";

/** What the person decided for one agent of the pack. */
export interface AgentChoiceState {
  action: PackAgentAction;
  existingAgentId: string | null;
  runtimeId: string;
  /** A name typed for an agent that is created; empty keeps the one in the pack. */
  name: string;
  /** The skills the person chose for an agent whose pack does not say; null keeps what the pack declares. */
  skills: string[] | null;
}

/** An agent the person adds to a squad that the pack does not have. */
export interface ExtraAgentState {
  key: string;
  name: string;
  runtimeId: string;
  skills: string[];
}

export type LeaderState = { kind: "none" } | { kind: "agent"; key: string } | { kind: "generic" };

export interface PackChoices {
  squadName: string;
  agents: Record<string, AgentChoiceState>;
  extras: ExtraAgentState[];
  leader: LeaderState;
  /** The runtime of a general leader. */
  genericRuntimeId: string;
  skills: Record<string, PackSkillConflictChoice>;
  draftFromFlow: boolean;
  ackScan: boolean;
}

/** Why the install cannot go on, in the order the person should look at them. */
export type InstallBlocker = "blocked" | "errors" | "leader" | "runtime" | "ack" | "name";

export function initialChoices(preview: PackPreview, defaultRuntimeId: string): PackChoices {
  const agents: Record<string, AgentChoiceState> = {};
  for (const agent of preview.agents) {
    agents[agent.key] = {
      action: agent.existing_agent_id ? "use_existing" : "create",
      existingAgentId: agent.existing_agent_id,
      runtimeId: defaultRuntimeId,
      name: "",
      skills: null,
    };
  }
  // A skill the workspace has with the same content is kept; one with other content waits for the person's choice,
  // and keeping what exists is the choice that changes nothing.
  const skills: Record<string, PackSkillConflictChoice> = {};
  for (const skill of preview.skills) if (skill.existing_skill_id) skills[skill.name] = "keep_existing";
  return {
    squadName: preview.name,
    agents,
    extras: [],
    leader: preview.leader && agents[preview.leader] ? { kind: "agent", key: preview.leader } : { kind: "none" },
    genericRuntimeId: defaultRuntimeId,
    skills,
    draftFromFlow: false,
    ackScan: false,
  };
}

const EXTRA_KEY = /^[a-z][a-z0-9_]{1,31}$/;

/** A key the server accepts for an agent added to the squad (2 to 32 of a-z, 0-9, _, a letter first) that is not taken. */
export function extraAgentKey(name: string, taken: Set<string>): string {
  let base = name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "_")
    .replace(/^_+|_+$/g, "");
  if (!/^[a-z]/.test(base)) base = `agent_${base}`.replace(/_+$/, "");
  base = base.slice(0, 28).replace(/_+$/, "");
  if (!EXTRA_KEY.test(base)) base = "agent";
  let key = base;
  for (let n = 2; taken.has(key); n++) key = `${base}_${n}`;
  return key;
}

/** One agent for each skill of the pack, named after it, for a pack that has skills and no agents. */
export function oneAgentPerSkill(preview: PackPreview, defaultRuntimeId: string, existing: ExtraAgentState[]): ExtraAgentState[] {
  const taken = new Set([...preview.agents.map((a) => a.key), ...existing.map((e) => e.key)]);
  const added: ExtraAgentState[] = [];
  for (const skill of preview.skills) {
    const key = extraAgentKey(skill.name, taken);
    taken.add(key);
    added.push({ key, name: skill.name, runtimeId: defaultRuntimeId, skills: [skill.name] });
  }
  return added;
}

/** The agents the squad will have: bundle agents that are created or used, and the ones the person added. */
function agentKeys(preview: PackPreview, choices: PackChoices): string[] {
  const kept = preview.agents.filter((a) => choices.agents[a.key]?.action !== "skip" && choices.agents[a.key]).map((a) => a.key);
  return [...kept, ...choices.extras.map((e) => e.key)];
}

export function leaderIsValid(preview: PackPreview, choices: PackChoices): boolean {
  if (choices.leader.kind === "generic") return true;
  if (choices.leader.kind === "none") return false;
  return agentKeys(preview, choices).includes(choices.leader.key);
}

export function installBlocker(preview: PackPreview, choices: PackChoices): InstallBlocker | null {
  if (preview.scan.some((f) => f.severity === "block")) return "blocked";
  if (preview.issues.some((i) => i.severity === "error")) return "errors";
  const hasAgents = agentKeys(preview, choices).length > 0;
  const leader = leaderIsValid(preview, choices);
  // A pack with no agents to put in a squad installs its skills and makes no squad.
  if ((hasAgents || choices.leader.kind === "generic") && !leader) return "leader";
  const created = preview.agents.some((a) => choices.agents[a.key]?.action === "create" && !choices.agents[a.key]!.runtimeId);
  const extra = choices.extras.some((e) => !e.runtimeId);
  const generic = choices.leader.kind === "generic" && !choices.genericRuntimeId;
  if (created || extra || generic) return "runtime";
  if (preview.scan.some((f) => f.severity === "warn") && !choices.ackScan) return "ack";
  if (leader && choices.squadName.trim() === "") return "name";
  return null;
}

/** The body of the commit: what the person decided, and nothing the server would not read. */
export function buildCommitRequest(preview: PackPreview, choices: PackChoices, newInstall: boolean): PackCommitRequest {
  const leader = leaderIsValid(preview, choices);
  const request: PackCommitRequest = {
    digest: preview.digest,
    agents: preview.agents.flatMap((agent): PackAgentChoice[] => {
      const c = choices.agents[agent.key];
      if (!c) return [];
      if (c.action === "use_existing") return [{ key: agent.key, action: c.action, existing_agent_id: c.existingAgentId ?? "" }];
      if (c.action === "create") {
        const name = c.name.trim();
        return [
          {
            key: agent.key,
            action: c.action,
            runtime_id: c.runtimeId,
            ...(name && name !== agent.name ? { name } : {}),
            ...(c.skills ? { skills: c.skills } : {}),
          },
        ];
      }
      return [{ key: agent.key, action: c.action }];
    }),
    ack_scan_warnings: choices.ackScan,
  };
  if (choices.extras.length > 0) {
    request.extra_agents = choices.extras.map((e) => ({
      key: e.key,
      name: e.name.trim() || e.key,
      instructions: "",
      runtime_id: e.runtimeId,
      skills: e.skills,
    }));
  }
  if (leader) {
    request.squad = { name: choices.squadName.trim() };
    request.leader =
      choices.leader.kind === "generic"
        ? { generic: true, runtime_id: choices.genericRuntimeId }
        : { agent: (choices.leader as { kind: "agent"; key: string }).key };
  }
  if (Object.keys(choices.skills).length > 0) request.skills = choices.skills;
  if (leader && preview.flow.present && choices.draftFromFlow) request.create_draft_from_flow = true;
  if (newInstall) request.new_install = true;
  return request;
}
