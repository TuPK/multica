import type { Intent } from "./schemas";

export type IntentGroup =
  | { kind: "single"; intent: Intent }
  | { kind: "batch"; key: string; intents: Intent[] };

/**
 * The cards for the decisions still open: pending intents that share a batch key (agent,
 * target, class and day) become one batch card; a lone intent, or one with no key, stays a
 * single card. Intents that are no longer pending are not decisions, so they are left out.
 */
export function groupIntents(intents: Intent[]): IntentGroup[] {
  const open = intents.filter((intent) => intent.state === "pending");
  const byKey = new Map<string, Intent[]>();
  for (const intent of open) {
    if (intent.batch_key === "") continue;
    const list = byKey.get(intent.batch_key) ?? [];
    list.push(intent);
    byKey.set(intent.batch_key, list);
  }
  const groups: IntentGroup[] = [];
  const placed = new Set<string>();
  for (const intent of open) {
    if (placed.has(intent.id)) continue;
    const members = intent.batch_key === "" ? [] : (byKey.get(intent.batch_key) ?? []);
    if (members.length >= 2) {
      groups.push({ kind: "batch", key: intent.batch_key, intents: members });
      for (const member of members) placed.add(member.id);
    } else {
      groups.push({ kind: "single", intent });
      placed.add(intent.id);
    }
  }
  return groups;
}
