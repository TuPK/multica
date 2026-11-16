import type { FlowDocRole } from "@multica/core/types";

export interface AgentWithSkills {
  id: string;
  skills?: { name: string; enabled?: boolean }[];
}

/**
 * The skills a role has (spec R09): the skills of the squad, which every agent of the squad runs with, and the
 * enabled skills attached to every agent bound to the role. A skill that only some of the bound agents have is
 * not the role's: a task that asks for it would be refused at publish.
 */
export function skillsOfRole(role: FlowDocRole | undefined, agents: AgentWithSkills[], squadSkillNames: string[]): string[] {
  const names = new Set(squadSkillNames);
  const bound = role?.kind === "agent" ? (role.bind?.agent_ids ?? []) : [];
  if (bound.length > 0) {
    const own = bound.map((id) => new Set((agents.find((a) => a.id === id)?.skills ?? []).filter((s) => s.enabled !== false).map((s) => s.name)));
    for (const name of own[0] ?? []) if (own.every((set) => set.has(name))) names.add(name);
  }
  return [...names].sort();
}
