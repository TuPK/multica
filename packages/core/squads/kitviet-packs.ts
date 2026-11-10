import type { BuiltinSkill } from "./sdlc-skills-data";
import type { KitvietAgent } from "./kitviet-skills-data/types";
import { findKitvietRole, kvLeadInstructions, kvRoleInstructionsWithBody, kvRolePack } from "./kitviet-squad-roles";

/** One generated Kit Viet pack: its skills and the agent bodies of its squad, under the shared bundle types. */
export interface KitvietPackModule {
  KITVIET_BUILTIN_SKILLS: readonly BuiltinSkill[];
  KITVIET_AGENTS: readonly KitvietAgent[];
}

/**
 * One chunk per pack, so a squad that uses none of them never downloads them. The quickstart is the
 * only caller, and only when it attaches a Kit Viet skill or creates a Kit Viet agent. The keys are
 * pack ids (the squad ids in kitviet-squads.json); `common` holds the skills every squad shares.
 */
const KITVIET_PACK_LOADERS: Readonly<Record<string, () => Promise<KitvietPackModule>>> = {
  common: () => import("./kitviet-skills-data/common"),
  admin: () => import("./kitviet-skills-data/admin"),
  b2b: () => import("./kitviet-skills-data/b2b"),
  marketing: () => import("./kitviet-skills-data/marketing"),
  legal: () => import("./kitviet-skills-data/legal"),
  accounting: () => import("./kitviet-skills-data/accounting"),
};

/** Imports every Kit Viet pack. A name lookup can hit any of them, so they load together. */
export function loadKitvietPacks(): Promise<readonly KitvietPackModule[]> {
  return Promise.all(Object.values(KITVIET_PACK_LOADERS).map((load) => load()));
}

/**
 * The instructions of a Kit Viet role's agent, or undefined when the role is not a Kit Viet role.
 * A lead is composed without loading a pack. Any other role loads its squad's pack for its body.
 */
export async function loadKitvietRoleInstructions(role: string): Promise<string | undefined> {
  const pack = kvRolePack(role);
  if (pack === undefined) {
    return findKitvietRole(role) === undefined ? undefined : kvLeadInstructions(role);
  }
  const load = KITVIET_PACK_LOADERS[pack];
  if (!load) throw new Error(`no pack loader for ${pack}; add it to KITVIET_PACK_LOADERS`);
  const loaded = await load();
  const agent = loaded.KITVIET_AGENTS.find((candidate) => candidate.name === role);
  if (!agent) throw new Error(`pack ${pack} has no agent ${role}; run pnpm generate:kitviet`);
  return kvRoleInstructionsWithBody(role, agent.instructions);
}
