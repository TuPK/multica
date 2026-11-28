import { queryOptions, useQuery } from "@tanstack/react-query";
import { api } from "../../api";
import { parseReachMap } from "./schemas";

// Every key starts with "durable" and the workspace id so caches stay isolated
// across workspaces.
export const reachKeys = {
  all: (wsId: string) => ["durable", wsId, "reach"] as const,
  map: (wsId: string) => [...reachKeys.all(wsId), "map"] as const,
};

/** Latest Reach Map snapshot of the workspace (GET /api/reach). */
export function reachMapOptions(wsId: string, enabled = true) {
  return queryOptions({
    queryKey: reachKeys.map(wsId),
    queryFn: async () => parseReachMap(await api.request("/api/reach"), "GET /api/reach"),
    enabled: enabled && !!wsId,
  });
}

export function useReachMap(wsId: string, enabled = true) {
  return useQuery(reachMapOptions(wsId, enabled));
}
