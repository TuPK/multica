import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api";
import { effectKeys } from "./queries";
import {
  parsePolicyCell,
  parseUndoResult,
  type Decision,
  type EffectClass,
  type PolicyCell,
  type Reversibility,
  type UndoResult,
} from "./schemas";

export type SavePolicyOverrideRequest = {
  autonomy_level: number;
  effect_class: EffectClass;
  reversibility: Reversibility;
  decision: Decision;
};

/**
 * Takes back an executed effect inside its undo window. The effect log is refreshed once the
 * request settles, whether or not the caller is still mounted.
 */
export function useUndoEffect(wsId: string) {
  const qc = useQueryClient();
  return useMutation<UndoResult | null, Error, string>({
    mutationFn: async (id) =>
      parseUndoResult(
        await api.request(`/api/effects/${encodeURIComponent(id)}/undo`, { method: "POST" }),
        "POST /api/effects/:id/undo",
      ),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: effectKeys.all(wsId) });
    },
  });
}

/** Saves one cell's decision. The server refuses a loosening the grid has locked, and records it. */
export function useSaveEffectPolicyOverride(wsId: string) {
  const qc = useQueryClient();
  return useMutation<PolicyCell | null, Error, SavePolicyOverrideRequest>({
    mutationFn: async (body) =>
      parsePolicyCell(
        await api.request("/api/effects/policy", { method: "PUT", body: JSON.stringify(body) }),
        "PUT /api/effects/policy",
      ),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: effectKeys.policy(wsId) });
    },
  });
}
