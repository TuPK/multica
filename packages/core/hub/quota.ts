import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";
import { hubItemKeys } from "./item-keys";

/** Item count and storage use against the workspace limits. A limit of 0 or less is unlimited. */
export function hubQuotaOptions(wsId: string) {
  return queryOptions({
    queryKey: hubItemKeys.quota(wsId),
    queryFn: () => api.getHubQuota(wsId),
    enabled: wsId.length > 0,
  });
}
