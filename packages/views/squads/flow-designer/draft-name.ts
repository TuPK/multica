/** The most characters a draft name may have (`limits.draft_name_runes`). */
export const MAX_DRAFT_NAME_RUNES = 56;

/** Cuts a name to the length of a draft name, by characters rather than code units. */
export function cutDraftName(name: string, max = MAX_DRAFT_NAME_RUNES): string {
  const runes = [...name.trim()];
  return runes.length <= max ? runes.join("") : runes.slice(0, max).join("").trimEnd();
}
