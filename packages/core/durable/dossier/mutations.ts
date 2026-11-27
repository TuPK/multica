import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api";
import { dossierKeys } from "./queries";
import {
  parseSeatDossierEntry,
  parseSeatDossierList,
  type DossierEvidence,
  type DossierKind,
  type SeatDossierEntry,
} from "./schemas";

export type SeatDossierEntryInput = {
  kind: DossierKind;
  text: string;
  work_class: string;
  evidence: DossierEvidence[];
};

export type AddSeatDossierRequest = {
  entries: SeatDossierEntryInput[];
};

export type RetireSeatDossierRequest = {
  entryId: string;
  reason: string;
};

/**
 * Adds one to three entries to a seat's dossier. Refreshes the seat's dossier queries on settle,
 * whether or not the caller is still mounted.
 */
export function useAddSeatDossierEntries(wsId: string, agentId: string) {
  const qc = useQueryClient();
  return useMutation<SeatDossierEntry[], Error, AddSeatDossierRequest>({
    mutationFn: async (body) =>
      parseSeatDossierList(
        await api.request(`/api/agents/${encodeURIComponent(agentId)}/dossier`, {
          method: "POST",
          body: JSON.stringify(body),
        }),
        "POST /api/agents/:id/dossier",
      ),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: dossierKeys.agent(wsId, agentId) });
    },
  });
}

/**
 * Retires an active dossier entry with a reason. Awaits the server before refreshing the list;
 * entries are never removed optimistically.
 */
export function useRetireSeatDossierEntry(wsId: string, agentId: string) {
  const qc = useQueryClient();
  return useMutation<SeatDossierEntry | null, Error, RetireSeatDossierRequest>({
    mutationFn: async ({ entryId, reason }) =>
      parseSeatDossierEntry(
        await api.request(
          `/api/agents/${encodeURIComponent(agentId)}/dossier/${encodeURIComponent(entryId)}/retire`,
          {
            method: "POST",
            body: JSON.stringify({ reason }),
          },
        ),
        "POST /api/agents/:id/dossier/:entryId/retire",
      ),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: dossierKeys.agent(wsId, agentId) });
    },
  });
}
