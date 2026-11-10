import type { HubSource, HubSourceType } from "@multica/core/hub";

/** Source types an admin can add by hand. The others are created by the app itself. */
export const USER_SOURCE_TYPES: readonly HubSourceType[] = [
  "github",
  "skills_sh",
  "clawhub",
  "url_zip",
  "registry",
];

const KNOWN_SOURCE_TYPES: ReadonlySet<string> = new Set<string>([
  "github",
  "skills_sh",
  "clawhub",
  "url_zip",
  "registry",
  "upload",
  "manual",
  "runtime_local",
  "plugin",
]);

/** Unknown types from a newer backend fall back to the raw value rather than a wrong label. */
export function isKnownSourceType(type: string): type is HubSourceType {
  return KNOWN_SOURCE_TYPES.has(type);
}

/** The link target for a source URL, or null when it is not a web address (never a javascript: link). */
export function safeSourceLink(raw: string): string | null {
  try {
    const url = new URL(raw);
    return url.protocol === "https:" || url.protocol === "http:" ? url.toString() : null;
  } catch {
    return null;
  }
}

export type SyncStatusKind = "ok" | "syncing" | "error" | "never" | "unknown";

export function syncStatusKind(source: Pick<HubSource, "last_sync_at" | "last_sync_status">): SyncStatusKind {
  const status = source.last_sync_status;
  if (status === "ok" || status === "syncing" || status === "error") return status;
  if (!source.last_sync_at) return "never";
  return "unknown";
}
