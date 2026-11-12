import { CATALOG_ROLE_INSTRUCTIONS_BY_SQUAD } from "./generated/role-instructions";

/**
 * The instructions of the roles that write their own, read from catalog/squads/<id>/roles/<role>.md.
 * `keys` are the role keys the caller expects; a missing or extra file is a build error, not a
 * silent gap in a squad.
 */
export function catalogRoleInstructions<K extends string>(
  squadIds: readonly string[],
  keys: readonly K[],
): Record<K, string> {
  const found: Record<string, string> = {};
  for (const squadId of squadIds) {
    const roles = CATALOG_ROLE_INSTRUCTIONS_BY_SQUAD[squadId];
    if (!roles) throw new Error(`catalog: no role instructions for squad "${squadId}"`);
    Object.assign(found, roles);
  }
  const out = {} as Record<K, string>;
  for (const key of keys) {
    const text = found[key];
    if (text === undefined) throw new Error(`catalog: no instructions for role "${key}"`);
    out[key] = text;
  }
  const extra = Object.keys(found).filter((k) => !(keys as readonly string[]).includes(k));
  if (extra.length > 0) throw new Error(`catalog: instructions for unknown roles: ${extra.join(", ")}`);
  return out;
}
