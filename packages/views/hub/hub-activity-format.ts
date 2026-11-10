import type { HubActivityEntry } from "@multica/core/hub";
import { formatHubSize } from "./hub-files";

/** Actions the Hub writes today. Anything else from the server falls back to `other`. */
const KNOWN_ACTIONS = [
  "source_created",
  "source_updated",
  "source_deleted",
  "import_committed",
  "source_synced",
  "update_applied",
  "item_forked",
  "item_exported",
  "file_saved",
  "plugin_published",
] as const;

export type HubActivityActionKey = (typeof KNOWN_ACTIONS)[number] | "other";

/** Maps a server action to its locale key. Unknown actions keep a generic label. */
export function hubActivityActionKey(action: string): HubActivityActionKey {
  return (KNOWN_ACTIONS as readonly string[]).includes(action)
    ? (action as HubActivityActionKey)
    : "other";
}

/** One short fact about an entry, already reduced to what the copy can show. */
export type HubActivityFact =
  | { kind: "version"; version: string }
  | { kind: "items"; count: number }
  | { kind: "files"; count: number }
  | { kind: "checked"; count: number }
  | { kind: "updates"; count: number }
  | { kind: "path"; path: string }
  | { kind: "size"; size: string };

function text(value: unknown): string | null {
  return typeof value === "string" && value.trim() !== "" ? value.trim() : null;
}

function count(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) && value >= 0 ? value : null;
}

/**
 * The detail fields the Hub writes, in display order. The server's detail object
 * is loosely typed, so each field is read only when it has the expected type.
 */
export function hubActivityFacts(entry: HubActivityEntry): HubActivityFact[] {
  const detail = entry.detail ?? {};
  const facts: HubActivityFact[] = [];

  const version = text(detail.version);
  if (version) facts.push({ kind: "version", version });

  const items = count(detail.count) ?? count(detail.imported);
  if (items !== null) facts.push({ kind: "items", count: items });

  const files = count(detail.file_count);
  if (files !== null) facts.push({ kind: "files", count: files });

  const checked = count(detail.checked);
  if (checked !== null) facts.push({ kind: "checked", count: checked });

  const updates = count(detail.updates_available);
  if (updates !== null) facts.push({ kind: "updates", count: updates });

  const path = text(detail.path);
  if (path) facts.push({ kind: "path", path });

  const size = count(detail.size_bytes);
  if (size !== null) facts.push({ kind: "size", size: formatHubSize(size) });

  return facts;
}
