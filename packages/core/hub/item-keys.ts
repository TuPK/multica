import type { QueryClient } from "@tanstack/react-query";
import type { HubActivityFilter } from "../types/hub";
import type { HubItemKind } from "./items";
import { hubKeys } from "./files";

/** Query keys for item metadata, updates, activity and quota. All are workspace-scoped. */
export const hubItemKeys = {
  items: (wsId: string) => [...hubKeys.all(wsId), "items"] as const,
  updates: (wsId: string) => [...hubKeys.all(wsId), "update"] as const,
  update: (wsId: string, kind: HubItemKind, refId: string) =>
    [...hubItemKeys.updates(wsId), kind, refId] as const,
  updateDiff: (wsId: string, kind: HubItemKind, refId: string, path: string) =>
    [...hubItemKeys.update(wsId, kind, refId), "diff", path] as const,
  activityAll: (wsId: string) => [...hubKeys.all(wsId), "activity"] as const,
  activity: (wsId: string, filter: HubActivityFilter = {}) =>
    [
      ...hubItemKeys.activityAll(wsId),
      {
        kind: filter.kind ?? null,
        refId: filter.refId ?? null,
        sourceId: filter.sourceId ?? null,
      },
    ] as const,
  quota: (wsId: string) => [...hubKeys.all(wsId), "quota"] as const,
};

/**
 * Refreshes what an item-level change touches: the item list, quota and activity.
 * Update checks are dropped too, since most item changes move the digests they compare.
 */
export function invalidateHubItemState(qc: QueryClient, wsId: string) {
  qc.invalidateQueries({ queryKey: hubItemKeys.items(wsId) });
  qc.invalidateQueries({ queryKey: hubItemKeys.quota(wsId) });
  qc.invalidateQueries({ queryKey: hubItemKeys.activityAll(wsId) });
}
