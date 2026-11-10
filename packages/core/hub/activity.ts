import { infiniteQueryOptions } from "@tanstack/react-query";
import { api } from "../api";
import type { HubActivityFilter } from "../types/hub";
import { hubItemKeys } from "./item-keys";

export const HUB_ACTIVITY_PAGE_SIZE = 50;

/**
 * Hub activity, newest first, paged by the server's cursor. Each page is
 * requested with the previous page's `next_cursor`; `getNextPageParam` stops
 * when the cursor is null.
 */
export function hubActivityInfiniteOptions(wsId: string, filter: HubActivityFilter = {}) {
  return infiniteQueryOptions({
    queryKey: hubItemKeys.activity(wsId, filter),
    queryFn: ({ pageParam }) =>
      api.listHubActivity(wsId, {
        kind: filter.kind,
        refId: filter.refId,
        sourceId: filter.sourceId,
        limit: HUB_ACTIVITY_PAGE_SIZE,
        before: pageParam,
      }),
    initialPageParam: null as string | null,
    getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
    enabled: wsId.length > 0,
  });
}
