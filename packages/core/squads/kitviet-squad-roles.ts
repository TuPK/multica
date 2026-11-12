import { composeKvAgentInstructions, KV_LEAD_NOTES, KV_ROLE_NOTES } from "./kitviet-agent-instructions";
import { KITVIET_SQUADS } from "../catalog/generated/kitviet/squads";
import type { KitvietSquad, KitvietSquadRole } from "../catalog/generated/kitviet/types";
import type { SquadRole, SquadRoleKey } from "./templates";

/**
 * The static side of the Kit Viet squads: the names, roles and roster lines the generator wrote
 * to catalog/generated/kitviet/squads.ts. It imports no pack file, so the template registry can use it
 * without pulling the skill and agent text into the main bundle. The agent bodies are loaded by
 * kitviet-packs.ts when the quickstart creates the agents.
 */

const ROLE_INDEX: ReadonlyMap<string, { squad: KitvietSquad; role: KitvietSquadRole }> = new Map(
  KITVIET_SQUADS.flatMap((squad) => squad.roles.map((role) => [role.key, { squad, role }] as const)),
);

/** The generated squad with this template id. Throws when the pack is stale. */
export function kitvietSquad(templateId: string): KitvietSquad {
  const squad = KITVIET_SQUADS.find((candidate) => candidate.templateId === templateId);
  if (!squad) throw new Error(`no Kit Viet squad ${templateId}; run pnpm generate:kitviet`);
  return squad;
}

/** The squad and generated data of a Kit Viet role key, or undefined for any other role. */
export function findKitvietRole(key: string): { squad: KitvietSquad; role: KitvietSquadRole } | undefined {
  return ROLE_INDEX.get(key);
}

function kitvietRole(key: string): { squad: KitvietSquad; role: KitvietSquadRole } {
  const found = findKitvietRole(key);
  if (!found) throw new Error(`no Kit Viet role ${key}; run pnpm generate:kitviet`);
  return found;
}

/**
 * A Kit Viet role's SquadRole. The name, the agent template (the role kind) come from the generated
 * data; the one-line description and the roster line are written by the template file.
 */
export function kvRole(key: string, text: { description: string; roster: string }): SquadRole {
  const { role } = kitvietRole(key);
  return { key: key as SquadRoleKey, agentTemplate: role.kind, name: role.name, description: text.description, role: text.roster };
}

/**
 * The per-role extra skills of a squad, typed by that squad's own role keys. A role the generated
 * data gives skills to, but the squad does not have, is an error.
 */
export function kvRoleSkills<K extends SquadRoleKey>(
  templateId: string,
  roles: readonly K[],
): Partial<Record<K, readonly string[]>> {
  const squad = kitvietSquad(templateId);
  const out: Partial<Record<K, readonly string[]>> = {};
  for (const [key, skills] of Object.entries(squad.roleSkills)) {
    if (!(roles as readonly string[]).includes(key)) {
      throw new Error(`roleSkills names ${key}, which is not a role of ${templateId}`);
    }
    out[key as K] = skills;
  }
  return out;
}

/**
 * The instructions of a Kit Viet lead. A lead has no body in the pack (its procedure is in the
 * squad's skills), so its text is complete without the pack and can be written statically.
 */
export function kvLeadInstructions(key: string): string {
  const { squad, role } = kitvietRole(key);
  if (role.kind !== "kv_lead") throw new Error(`${key} is not a Kit Viet lead`);
  return composeKvAgentInstructions({ kind: role.kind, header: role.header, notes: KV_LEAD_NOTES[squad.templateId] });
}

/** The pack file that holds a role's body, keyed by the squad's pack id. */
export function kvRolePack(key: string): string | undefined {
  const found = findKitvietRole(key);
  return found?.role.hasBody ? found.squad.pack : undefined;
}

/** The composition of a non-lead role's instructions around its pack body (see kitviet-agent-instructions.ts). */
export function kvRoleInstructionsWithBody(key: string, body: string): string {
  const { role } = kitvietRole(key);
  return composeKvAgentInstructions({ kind: role.kind, header: role.header, body, notes: KV_ROLE_NOTES[key] });
}
