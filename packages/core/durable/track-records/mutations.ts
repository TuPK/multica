import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api";
import { envelopeKeys, trackRecordKeys } from "./queries";
import { parseSquadEnvelope, parseTrackRecord, type SquadEnvelopeView, type TrackRecord } from "./schemas";
import type { VerificationMode } from "../outcomes/schemas";

export type SetEnvelopeRequest = {
  floor_level: number;
  ceiling_level: number;
  reason: string;
};

export type PinTrackRecordRequest = {
  agent_id: string;
  work_class: string;
  /** null removes the pin and returns the agent to the automatic mode. */
  mode: VerificationMode | null;
};

/**
 * Sets a squad's envelope. Owners and admins only; the server answers 403 to an agent. The
 * cache takes the server's view, so a refused range never shows as set.
 */
export function useSetSquadEnvelope(wsId: string, squadId: string) {
  const qc = useQueryClient();
  return useMutation<SquadEnvelopeView, Error, SetEnvelopeRequest>({
    mutationFn: async (data) =>
      parseSquadEnvelope(
        await api.request(`/api/squads/${encodeURIComponent(squadId)}/autonomy/envelope`, {
          method: "PUT",
          body: JSON.stringify(data),
        }),
        "PUT /api/squads/:id/autonomy/envelope",
      ),
    onSuccess: (view) => {
      qc.setQueryData(envelopeKeys.squad(wsId, squadId), view);
    },
  });
}

/** Pins a verification mode for one agent and work class, or unpins it with a null mode. */
export function usePinTrackRecord(wsId: string) {
  const qc = useQueryClient();
  return useMutation<TrackRecord | null, Error, PinTrackRecordRequest>({
    mutationFn: async (data) =>
      parseTrackRecord(
        await api.request("/api/track-records/pin", { method: "PUT", body: JSON.stringify(data) }),
        "PUT /api/track-records/pin",
      ),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: trackRecordKeys.all(wsId) });
    },
  });
}
