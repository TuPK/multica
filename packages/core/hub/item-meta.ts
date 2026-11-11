import { queryOptions, useQuery } from "@tanstack/react-query";
import { useMemo } from "react";
import { api } from "../api";
import type { HubItemMeta } from "../types/hub";
import type { HubItemKind } from "./items";
import { hubItemKeys } from "./item-keys";

/**
 * One item's metadata from GET /api/hub/items/{kind}/{id}. Lists refresh it through the
 * `items` prefix, so an invalidated list also refreshes open detail reads.
 */
export function hubItemOptions(wsId: string, kind: HubItemKind, refId: string) {
  return queryOptions({
    queryKey: hubItemKeys.item(wsId, kind, refId),
    queryFn: () => api.getHubItem(wsId, kind, refId),
    enabled: wsId.length > 0 && refId.length > 0,
  });
}

/** Every skill and plugin package of the workspace, with its sync and tracking state. */
export function hubItemMetaListOptions(wsId: string) {
  return queryOptions({
    queryKey: hubItemKeys.items(wsId),
    queryFn: () => api.listHubItems(wsId),
    enabled: wsId.length > 0,
  });
}

/** Finds one item in a list already loaded. Pure, so the lookup is testable without a hook. */
export function findHubItemMeta(
  items: readonly HubItemMeta[],
  kind: HubItemKind,
  refId: string,
): HubItemMeta | undefined {
  return items.find((item) => item.kind === kind && item.ref_id === refId);
}

/** Metadata for one item, derived from the workspace list. Fetches the whole list once. */
export function useHubItemMeta(
  wsId: string,
  kind: HubItemKind,
  refId: string,
): { meta?: HubItemMeta; isLoading: boolean } {
  const query = useQuery(hubItemMetaListOptions(wsId));
  const meta = useMemo(
    () => (query.data ? findHubItemMeta(query.data, kind, refId) : undefined),
    [query.data, kind, refId],
  );
  return { meta, isLoading: query.isLoading };
}
