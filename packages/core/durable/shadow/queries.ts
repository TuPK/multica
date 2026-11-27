import { queryOptions, useQuery } from "@tanstack/react-query";
import { api } from "../../api";
import { toQueryString } from "../query-string";
import {
  parseShadowEstimate,
  SHADOW_MAX_TARGET_PAIRS,
  SHADOW_MIN_TARGET_PAIRS,
} from "./schemas";

// Every key starts with "durable" and the workspace id, then scopes to the seat (agentId).
export const shadowKeys = {
  all: (wsId: string) => ["durable", wsId, "shadow"] as const,
  agent: (wsId: string, agentId: string) => [...shadowKeys.all(wsId), agentId] as const,
  estimate: (wsId: string, agentId: string, params: ShadowEstimateParams) =>
    [...shadowKeys.agent(wsId, agentId), "estimate", params] as const,
};

export type ShadowEstimateParams = {
  sample_rate?: number;
  target_pairs?: number;
};

export function isValidShadowEstimateParams(params: ShadowEstimateParams): boolean {
  if (params.sample_rate !== undefined) {
    if (!Number.isFinite(params.sample_rate) || params.sample_rate <= 0 || params.sample_rate > 1) {
      return false;
    }
  }
  if (params.target_pairs !== undefined) {
    if (
      !Number.isInteger(params.target_pairs) ||
      params.target_pairs < SHADOW_MIN_TARGET_PAIRS ||
      params.target_pairs > SHADOW_MAX_TARGET_PAIRS
    ) {
      return false;
    }
  }
  return true;
}

/** Prices a shadow run for a seat before it starts (GET /api/agents/{id}/shadow/estimate). */
export function shadowEstimateOptions(
  wsId: string,
  agentId: string,
  params: ShadowEstimateParams = {},
  enabled = true,
) {
  return queryOptions({
    queryKey: shadowKeys.estimate(wsId, agentId, params),
    queryFn: async () =>
      parseShadowEstimate(
        await api.request(
          `/api/agents/${encodeURIComponent(agentId)}/shadow/estimate${toQueryString(params)}`,
        ),
        "GET /api/agents/:id/shadow/estimate",
      ),
    enabled: enabled && !!wsId && !!agentId && isValidShadowEstimateParams(params),
  });
}

export function useShadowEstimate(
  wsId: string,
  agentId: string,
  params: ShadowEstimateParams = {},
  enabled = true,
) {
  return useQuery(shadowEstimateOptions(wsId, agentId, params, enabled));
}
