import { queryOptions, useQuery } from "@tanstack/react-query";
import { api } from "../../api";
import { toQueryString } from "../query-string";
import { parseSquadEnvelope, parseTrackRecordList } from "./schemas";

export const trackRecordKeys = {
  all: (wsId: string) => ["durable", wsId, "track-records"] as const,
  list: (wsId: string, filters: TrackRecordFilters) => [...trackRecordKeys.all(wsId), "list", filters] as const,
};

export const envelopeKeys = {
  squad: (wsId: string, squadId: string) => ["durable", wsId, "envelope", squadId] as const,
};

export type TrackRecordFilters = {
  agent_id?: string;
  squad_id?: string;
  work_class?: string;
  window_days?: 30 | 90;
};

/** Track records for the given agent, squad or work class. */
export function trackRecordListOptions(wsId: string, filters: TrackRecordFilters = {}) {
  return queryOptions({
    queryKey: trackRecordKeys.list(wsId, filters),
    queryFn: async () =>
      parseTrackRecordList(await api.request(`/api/track-records${toQueryString(filters)}`), "GET /api/track-records"),
    enabled: !!wsId,
  });
}

export function useTrackRecords(wsId: string, filters: TrackRecordFilters = {}) {
  return useQuery(trackRecordListOptions(wsId, filters));
}

/** A squad's envelope (null when none is set) and the reputation it reads. */
export function squadEnvelopeOptions(wsId: string, squadId: string) {
  return queryOptions({
    queryKey: envelopeKeys.squad(wsId, squadId),
    queryFn: async () =>
      parseSquadEnvelope(
        await api.request(`/api/squads/${encodeURIComponent(squadId)}/autonomy/envelope`),
        "GET /api/squads/:id/autonomy/envelope",
      ),
    enabled: !!wsId && !!squadId,
  });
}

export function useSquadEnvelope(wsId: string, squadId: string) {
  return useQuery(squadEnvelopeOptions(wsId, squadId));
}
