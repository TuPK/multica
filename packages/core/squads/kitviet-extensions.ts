import { KITVIET_EXTENSIONS } from "../catalog/generated/kitviet/extensions";

/**
 * The Kit Viet skills attached to existing company squads (spec section 7): the extra builtin skills of
 * each extension target. The key is a company squad key (company-template.ts). Each value is the bundle
 * names an attach call takes; the target's pack is loaded with kitvietPackOf(target).
 *
 * Attach design: kitviet-extension-attach.ts matches each existing squad to its target (by template_key,
 * or by card name for a company starter squad, which has no template_key), and adds the bundles with the
 * squad quickstart's attachBuiltinSkills. The client runs it after every company apply
 * (useApplyCompanyTemplate in company/company-template-mutations.ts). A Kit Viet template squad does not
 * read this map. The generator checks that a target is in one extension only.
 *
 * The names come from catalog/generated/kitviet/extensions.ts, which the generator writes from
 * catalog/_sources/kitviet/extensions/*.json. This file itself is hand-written.
 */
export const KITVIET_EXTENSION_BUILTIN_SKILLS: Readonly<Record<string, readonly string[]>> = Object.fromEntries(
  KITVIET_EXTENSIONS.flatMap((extension) => extension.targets.map((target) => [target, extension.builtinSkills] as const)),
);
