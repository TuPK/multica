// Earlier texts of the Kit Viet agents and squads, as the release before last generated them. The
// upgrade (upgrade.ts, upgradeKitvietSquad) replaces a text only when it still equals one of these,
// so a text the boss edited is never overwritten. When a release changes a generated text, append
// the text it replaces here, under the role key; never edit an entry. Empty until a release has
// shipped a text that later changes.

export const KITVIET_PREVIOUS_AGENT_INSTRUCTIONS: Record<string, readonly string[]> = {};

export const KITVIET_PREVIOUS_SQUAD_INSTRUCTIONS: readonly string[] = [];
