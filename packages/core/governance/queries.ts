import { infiniteQueryOptions, queryOptions } from "@tanstack/react-query";
import { api } from "../api";
import type { GovernanceEventFilters } from "./types";
import type { HealthReport } from "./health";
import type { CompanyPulse } from "./scale";

export const governanceKeys = {
  all: (wsId: string) => ["governance", wsId] as const,
  companyState: (wsId: string) => [...governanceKeys.all(wsId), "company-state"] as const,
  digestMode: (wsId: string) => [...governanceKeys.all(wsId), "digest-mode"] as const,
  attention: (wsId: string) => [...governanceKeys.all(wsId), "attention"] as const,
  attentionQueued: (wsId: string) => [...governanceKeys.all(wsId), "attention-queued"] as const,
  events: (wsId: string, filters: GovernanceEventFilters = {}) =>
    [
      ...governanceKeys.all(wsId),
      "events",
      filters.kind ?? "",
      filters.actor_type ?? "",
      filters.subject_type ?? "",
    ] as const,
};

/** The company's state and quiet hours. Shared by the Controls tab and the Company header. */
export function companyStateOptions(wsId: string) {
  return queryOptions({
    queryKey: governanceKeys.companyState(wsId),
    queryFn: () => api.getCompanyState(),
    enabled: !!wsId,
  });
}

/** The Secretary's digest source (spec 4.8). */
export function digestModeOptions(wsId: string) {
  return queryOptions({
    queryKey: governanceKeys.digestMode(wsId),
    queryFn: () => api.getDigestMode(),
    enabled: !!wsId,
  });
}

/** The weekly attention budget and this week's spend (durable spec 7.3). */
export function attentionBudgetOptions(wsId: string) {
  return queryOptions({
    queryKey: governanceKeys.attention(wsId),
    queryFn: () => api.getAttentionBudget(),
    enabled: !!wsId,
  });
}

/** Questions queued for the caller while their squad's weekly attention share is spent (durable spec 7.5). */
export function queuedAttentionOptions(wsId: string, enabled = true) {
  return queryOptions({
    queryKey: governanceKeys.attentionQueued(wsId),
    queryFn: () => api.getQueuedAttention(),
    enabled: !!wsId && enabled,
  });
}

/** The governance log, one page at a time. The next page is requested with the cursor the last one returned. */
export function governanceEventsOptions(wsId: string, filters: GovernanceEventFilters = {}) {
  return infiniteQueryOptions({
    queryKey: governanceKeys.events(wsId, filters),
    queryFn: ({ pageParam }) => api.listGovernanceEvents(filters, { cursor: pageParam }),
    initialPageParam: null as string | null,
    getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
    enabled: !!wsId,
  });
}

export const governanceHealthKey = (wsId: string) => [...governanceKeys.all(wsId), "health"] as const;

/** The system health of the workspace, for the Controls tab (spec 4.9). */
export function governanceHealthOptions(wsId: string) {
  return queryOptions({
    queryKey: governanceHealthKey(wsId),
    queryFn: (): Promise<HealthReport> => api.getGovernanceHealth(),
    enabled: !!wsId,
    staleTime: 30_000,
  });
}

export const companyPulseKey = (wsId: string) => [...governanceKeys.all(wsId), "pulse"] as const;

/** The company pulse, polled once a minute: the server's cache lasts that long. */
export function companyPulseOptions(wsId: string) {
  return queryOptions({
    queryKey: companyPulseKey(wsId),
    queryFn: (): Promise<CompanyPulse> => api.getCompanyPulse(),
    enabled: !!wsId,
    staleTime: 60_000,
    refetchInterval: 60_000,
  });
}
