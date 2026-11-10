import { KITVIET_EXTENSIONS } from "./kitviet-skills-data/extensions";

/**
 * The Kit Viet skills attached to existing squads that are not Kit Viet squads (spec section 7): the
 * extra builtin skills of each target. The key is the target: a SquadTemplate id (templates.ts) or a
 * company squad key (company-template.ts). Each value is the bundle names an attach call takes; the
 * skills of the extension's pack are loaded with kitvietPackOf(target) and loadKitvietSkillPacks.
 *
 * Wiring (the integrator's step): where a squad's builtin skills are attached, append
 * KITVIET_EXTENSION_BUILTIN_SKILLS[target] to the template's builtinSkills, and pass
 * kitvietPackOf(target) as the pack. A target is in one extension only (the generator checks it).
 * Generated from kitviet-extensions/*.json; do not edit by hand.
 */
export const KITVIET_EXTENSION_BUILTIN_SKILLS: Readonly<Record<string, readonly string[]>> = Object.fromEntries(
  KITVIET_EXTENSIONS.flatMap((extension) => extension.targets.map((target) => [target, extension.builtinSkills] as const)),
);
