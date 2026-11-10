import type { BuiltinSkill } from "./sdlc-skills-data";

/** One generated Kit Viet pack: the skills its generator wrote, under the shared bundle type. */
export interface KitvietPackModule {
  KITVIET_BUILTIN_SKILLS: readonly BuiltinSkill[];
}

/**
 * One chunk per pack, so a squad that uses none of them never downloads them.
 * Only the quickstart calls this, and only when it attaches a Kit Viet skill.
 */
const KITVIET_PACK_LOADERS: readonly (() => Promise<KitvietPackModule>)[] = [
  () => import("./kitviet-skills-data/common"),
  () => import("./kitviet-skills-data/admin"),
  () => import("./kitviet-skills-data/b2b"),
  () => import("./kitviet-skills-data/marketing"),
  () => import("./kitviet-skills-data/legal"),
  () => import("./kitviet-skills-data/accounting"),
];

/** Imports every Kit Viet pack. A name lookup can hit any of them, so they load together. */
export function loadKitvietPacks(): Promise<readonly KitvietPackModule[]> {
  return Promise.all(KITVIET_PACK_LOADERS.map((load) => load()));
}
