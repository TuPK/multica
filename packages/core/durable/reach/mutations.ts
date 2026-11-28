import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api";
import { reachKeys } from "./queries";
import { parseCloseReachGapResponse, type CloseReachGapResponse } from "./schemas";

export type CloseReachGapRequest = {
  surface: string;
  squad_id?: string;
};

/**
 * Opens or reuses the gap issue for one Reach Map surface (POST /api/reach/gaps).
 * Invalidates the workspace's Reach Map on settle whether or not the caller is mounted.
 */
export function useCloseReachGap(wsId: string) {
  const qc = useQueryClient();
  return useMutation<CloseReachGapResponse | null, Error, CloseReachGapRequest>({
    mutationFn: async (data) => {
      const payload: { surface: string; squad_id?: string } = { surface: data.surface };
      if (data.squad_id?.trim()) {
        payload.squad_id = data.squad_id.trim();
      }
      return parseCloseReachGapResponse(
        await api.request("/api/reach/gaps", {
          method: "POST",
          body: JSON.stringify(payload),
        }),
        "POST /api/reach/gaps",
      );
    },
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: reachKeys.all(wsId) });
    },
  });
}
