import type { BuiltinSkill } from "./sdlc-skills-data";
import type { KitvietAgent } from "./kitviet-skills-data/types";
import { KITVIET_EXTENSIONS } from "./kitviet-skills-data/extensions";
import { KITVIET_PACK_LOADERS as GENERATED_PACK_LOADERS } from "./kitviet-skills-data/packs";
import { KITVIET_SQUADS } from "./kitviet-skills-data/squads";
import { findKitvietRole, kvLeadInstructions, kvRoleInstructionsWithBody, kvRolePack } from "./kitviet-squad-roles";

/** One generated Kit Viet pack: its skills and the agent bodies of its squad, under the shared bundle types. */
export interface KitvietPackModule {
  KITVIET_BUILTIN_SKILLS: readonly BuiltinSkill[];
  KITVIET_AGENTS: readonly KitvietAgent[];
}

/**
 * One chunk per pack, so a squad that uses none of them never downloads them. The quickstart is the
 * only caller, and only when it attaches a Kit Viet skill or creates a Kit Viet agent. The loaders are
 * generated (kitviet-skills-data/packs.ts) from the configs: a pack id is a squad id, an extension id,
 * or `common`, which holds the skills every squad shares.
 */
const KITVIET_PACK_LOADERS: Readonly<Record<string, () => Promise<KitvietPackModule>>> = GENERATED_PACK_LOADERS;

/**
 * The pack of the Kit Viet squad with this template id, or of the extension that attaches to this
 * target (a SquadTemplate id or a company squad key, spec 7). Undefined for any other template.
 */
export function kitvietPackOf(templateId: string): string | undefined {
  return (
    KITVIET_SQUADS.find((squad) => squad.templateId === templateId)?.pack ??
    KITVIET_EXTENSIONS.find((extension) => extension.targets.includes(templateId))?.pack
  );
}

/**
 * Imports the skill chunks a squad of this pack can name: `common` and the squad's own pack. A squad
 * with no pack imports `common` alone. The other packs are not imported, so their chunks never load.
 */
export async function loadKitvietSkillPacks(pack: string | undefined): Promise<readonly KitvietPackModule[]> {
  const ids = pack === undefined || pack === "common" ? ["common"] : ["common", pack];
  return Promise.all(
    ids.map((id) => {
      const load = KITVIET_PACK_LOADERS[id];
      if (!load) throw new Error(`no pack loader for ${id}; add it to KITVIET_PACK_LOADERS`);
      return load();
    }),
  );
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
