import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api";
import { dossierKeys } from "../dossier/queries";
import { shadowKeys } from "./queries";
import { parseShadowRun, type ShadowRun } from "./schemas";

export type StartShadowRunRequest = {
  candidate_runtime_id: string;
  candidate_model: string;
  sample_rate: number;
  target_pairs: number;
  work_class_prefix: string;
  auto_promote: boolean;
  confirm_cost: boolean;
};

/**
 * Starts a shadow run on a seat (POST /api/agents/{id}/shadow). Refreshes both the seat's
 * shadow estimate and dossier queries on settle, whether or not the caller is still mounted.
 */
export function useStartShadowRun(wsId: string, agentId: string) {
  const qc = useQueryClient();
  return useMutation<ShadowRun | null, Error, StartShadowRunRequest>({
    mutationFn: async (body) =>
      parseShadowRun(
        await api.request(`/api/agents/${encodeURIComponent(agentId)}/shadow`, {
          method: "POST",
          body: JSON.stringify(body),
        }),
        "POST /api/agents/:id/shadow",
      ),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: shadowKeys.agent(wsId, agentId) });
      void qc.invalidateQueries({ queryKey: dossierKeys.agent(wsId, agentId) });
    },
  });
}
