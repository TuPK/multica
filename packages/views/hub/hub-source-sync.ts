import type { HubSyncResult } from "@multica/core/hub";

/** Auto-sync choices in seconds. 0 turns auto-sync off. */
export const AUTO_SYNC_INTERVALS = [0, 900, 3600, 21600, 86400] as const;

export type AutoSyncKey = "off" | "every_15m" | "every_1h" | "every_6h" | "every_24h" | "custom";

export function autoSyncKey(seconds: number): AutoSyncKey {
  switch (seconds) {
    case 0:
      return "off";
    case 900:
      return "every_15m";
    case 3600:
      return "every_1h";
    case 21600:
      return "every_6h";
    case 86400:
      return "every_24h";
    default:
      // A value set by another client or a newer backend keeps its own label instead of snapping to a choice.
      return "custom";
  }
}

export interface SyncCounts {
  checked: number;
  updates: number;
  unchanged: number;
  errors: number;
}

/** Counts for the sync result toast. Item-level errors are counted from `items`, not trusted from `checked`. */
export function syncCounts(result: HubSyncResult): SyncCounts {
  return {
    checked: result.checked,
    updates: result.updates_available,
    unchanged: result.unchanged,
    errors: result.items.filter((item) => item.change === "error").length,
  };
}
