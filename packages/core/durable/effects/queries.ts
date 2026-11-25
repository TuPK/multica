import { queryOptions, useQuery } from "@tanstack/react-query";
import { api } from "../../api";
import { toQueryString } from "../query-string";
import { parseEffectList, parsePolicyGrid, type EffectClass } from "./schemas";

// Every key starts with "durable" and the workspace id, so a workspace never reads another's
// effects. Server data stays in TanStack Query; the mutations invalidate it.
export const effectKeys = {
  all: (wsId: string) => ["durable", wsId, "effects"] as const,
  list: (wsId: string, filters: EffectFilters) => [...effectKeys.all(wsId), "list", filters] as const,
  policy: (wsId: string) => [...effectKeys.all(wsId), "policy"] as const,
};

export type EffectFilters = {
  task_id?: string;
  effect_class?: EffectClass;
  limit?: number;
};

/** The workspace's effect events, newest first, filtered as the caller chose. */
export function effectListOptions(wsId: string, filters: EffectFilters = {}) {
  return queryOptions({
    queryKey: effectKeys.list(wsId, filters),
    queryFn: async () =>
      parseEffectList(await api.request(`/api/effects${toQueryString(filters)}`), "GET /api/effects"),
    enabled: !!wsId,
  });
}

/** The policy grid: each cell's default, the workspace's override, and which decisions it may take. */
export function effectPolicyOptions(wsId: string) {
  return queryOptions({
    queryKey: effectKeys.policy(wsId),
    queryFn: async () =>
      parsePolicyGrid(await api.request("/api/effects/policy"), "GET /api/effects/policy"),
    enabled: !!wsId,
  });
}

export function useEffects(wsId: string, filters: EffectFilters = {}) {
  return useQuery(effectListOptions(wsId, filters));
}

export function useEffectPolicy(wsId: string) {
  return useQuery(effectPolicyOptions(wsId));
}
