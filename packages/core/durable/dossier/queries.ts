import { queryOptions, useQuery } from "@tanstack/react-query";
import { api } from "../../api";
import { toQueryString } from "../query-string";
import { parseSeatDossierList, type DossierStatusFilter } from "./schemas";

// Every key starts with "durable" and the workspace id, then scopes to the seat (agentId),
// so neither workspaces nor seats share cache entries.
export const dossierKeys = {
  all: (wsId: string) => ["durable", wsId, "dossier"] as const,
  agent: (wsId: string, agentId: string) => [...dossierKeys.all(wsId), agentId] as const,
  list: (wsId: string, agentId: string, filters: SeatDossierFilters) =>
    [...dossierKeys.agent(wsId, agentId), "list", filters] as const,
};

export type SeatDossierFilters = {
  status?: DossierStatusFilter;
  limit?: number;
};

/** A seat's dossier entries, newest first, filtered by status. */
export function seatDossierListOptions(
  wsId: string,
  agentId: string,
  filters: SeatDossierFilters = {},
) {
  return queryOptions({
    queryKey: dossierKeys.list(wsId, agentId, filters),
    queryFn: async () =>
      parseSeatDossierList(
        await api.request(
          `/api/agents/${encodeURIComponent(agentId)}/dossier${toQueryString(filters)}`,
        ),
        "GET /api/agents/:id/dossier",
      ),
    enabled: !!wsId && !!agentId,
  });
}

export function useSeatDossier(
  wsId: string,
  agentId: string,
  filters: SeatDossierFilters = {},
) {
  return useQuery(seatDossierListOptions(wsId, agentId, filters));
}
