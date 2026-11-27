import { queryOptions, useQuery } from "@tanstack/react-query";
import { api } from "../../api";
import { parseJuryPanelList } from "./schemas";

// Every key starts with "durable" and the workspace id so workspaces never share cache entries.
export const juryKeys = {
  all: (wsId: string) => ["durable", wsId, "jury"] as const,
  outcome: (wsId: string, outcomeId: string) => [...juryKeys.all(wsId), outcomeId] as const,
};

/** Jury panels for one outcome (GET /api/outcomes/{id}/jury). */
export function outcomeJuryOptions(wsId: string, outcomeId: string, enabled = true) {
  return queryOptions({
    queryKey: juryKeys.outcome(wsId, outcomeId),
    queryFn: async () =>
      parseJuryPanelList(
        await api.request(`/api/outcomes/${encodeURIComponent(outcomeId)}/jury`),
        "GET /api/outcomes/:id/jury",
      ),
    enabled: enabled && !!wsId && !!outcomeId,
  });
}

export function useOutcomeJury(wsId: string, outcomeId: string, enabled = true) {
  return useQuery(outcomeJuryOptions(wsId, outcomeId, enabled));
}
