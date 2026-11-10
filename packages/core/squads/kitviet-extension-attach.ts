import type { Squad } from "../types";
import { COMPANY_SQUAD_CARDS } from "./company-template";
import { KITVIET_EXTENSION_BUILTIN_SKILLS } from "./kitviet-extensions";
import { kitvietPackOf } from "./kitviet-packs";
import {
  attachBuiltinSkills,
  errorMessage,
  type BuiltinSkillsClient,
  type QuickstartOutcome,
} from "./quickstart";

/** The API calls the extension attach makes; the web client satisfies it as is. */
export type KitvietAttachClient = BuiltinSkillsClient;

type AttachSkillsOutcome = QuickstartOutcome["skills"];

export interface KitvietExtensionAttachOutcome {
  squadId: string;
  /** The extension target the squad matched: a company squad key or a SquadTemplate id. */
  target: string;
  status: AttachSkillsOutcome["status"];
  error?: string;
}

const CARD_KEY_BY_NAME: ReadonlyMap<string, string> = new Map(
  COMPANY_SQUAD_CARDS.map((card) => [card.name, card.key] as const),
);

function isExtensionTarget(key: string): boolean {
  return Object.prototype.hasOwnProperty.call(KITVIET_EXTENSION_BUILTIN_SKILLS, key);
}

/**
 * The extension target of a squad, or undefined when it is not one. A squad the quickstart made
 * matches by its template_key. A company starter squad has no template_key (the server does not set
 * one), so it matches by its card name, the same way the server reuses it. Any other squad is left
 * alone, including a Kit Viet squad, whose key is not a target.
 */
export function kitvietExtensionTargetOf(squad: Squad): string | undefined {
  if (squad.archived_at) return undefined;
  if (squad.template_key) return isExtensionTarget(squad.template_key) ? squad.template_key : undefined;
  const key = CARD_KEY_BY_NAME.get(squad.name);
  return key !== undefined && isExtensionTarget(key) ? key : undefined;
}

/**
 * Attaches the Kit Viet extension skills (spec 7) to the existing squads that are extension targets,
 * and to no other squad. Each match adds its extension's bundles to the squad's skills, with the
 * same import and union as the squad quickstart (attachBuiltinSkills): a skill the squad already
 * has, or a skill of the boss's own, is kept, and nothing is removed. Running it again changes
 * nothing. Only the packs of matched targets are loaded.
 *
 * The squads run one after another. Two squads of one target would otherwise create the same
 * skill at once. A failure on one squad is reported in its outcome, and the others still run.
 */
export async function attachKitvietExtensions(
  client: KitvietAttachClient,
  squads: readonly Squad[],
): Promise<KitvietExtensionAttachOutcome[]> {
  const outcomes: KitvietExtensionAttachOutcome[] = [];
  for (const squad of squads) {
    const target = kitvietExtensionTargetOf(squad);
    if (target === undefined) continue;
    const names = KITVIET_EXTENSION_BUILTIN_SKILLS[target] ?? [];
    const skills = await attachBuiltinSkills(client, squad.id, names, kitvietPackOf(target)).catch(
      (err): QuickstartOutcome["skills"] => ({ status: "failed", error: errorMessage(err) }),
    );
    outcomes.push({ squadId: squad.id, target, ...skills });
  }
  return outcomes;
}
