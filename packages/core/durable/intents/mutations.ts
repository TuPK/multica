import { useMutation, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { api } from "../../api";
import { effectKeys } from "../effects/queries";
import { intentKeys } from "./queries";
import {
  parseBatchResults,
  parseIntent,
  type BatchResult,
  type CanonicalCall,
  type Intent,
} from "./schemas";

export type ApproveIntentRequest = {
  id: string;
  /** The call with the person's edited text. Omitted when the call runs as the agent proposed it. */
  editedCall?: CanonicalCall;
  note?: string;
};

export type RejectIntentRequest = { id: string; note: string };

// A decision changes both the intent and the effect it belongs to. The follow-up lives on
// useMutation's onSettled, so it runs even when the caller has unmounted.
function refreshDecisions(qc: QueryClient, wsId: string) {
  void qc.invalidateQueries({ queryKey: intentKeys.all(wsId) });
  void qc.invalidateQueries({ queryKey: effectKeys.all(wsId) });
}

/** Approves an intent: the platform runs its call once. The answer is the intent after the run. */
export function useApproveIntent(wsId: string) {
  const qc = useQueryClient();
  return useMutation<Intent | null, Error, ApproveIntentRequest>({
    mutationFn: async ({ id, editedCall, note }) =>
      parseIntent(
        await api.request(`/api/intents/${encodeURIComponent(id)}/approve`, {
          method: "POST",
          body: JSON.stringify({ edited_call: editedCall, note: note ?? "" }),
        }),
        "POST /api/intents/:id/approve",
      ),
    onSettled: () => refreshDecisions(qc, wsId),
  });
}

/** Rejects an intent with a note the agent reads. The server refuses an empty note. */
export function useRejectIntent(wsId: string) {
  const qc = useQueryClient();
  return useMutation<Intent | null, Error, RejectIntentRequest>({
    mutationFn: async ({ id, note }) =>
      parseIntent(
        await api.request(`/api/intents/${encodeURIComponent(id)}/reject`, {
          method: "POST",
          body: JSON.stringify({ note }),
        }),
        "POST /api/intents/:id/reject",
      ),
    onSettled: () => refreshDecisions(qc, wsId),
  });
}

/** Approves several intents in one request. Each one is decided and reported on its own. */
export function useApproveIntentBatch(wsId: string) {
  const qc = useQueryClient();
  return useMutation<BatchResult[], Error, { ids: string[] }>({
    mutationFn: async ({ ids }) =>
      parseBatchResults(
        await api.request("/api/intents/approve-batch", {
          method: "POST",
          body: JSON.stringify({ ids }),
        }),
        "POST /api/intents/approve-batch",
      ),
    onSettled: () => refreshDecisions(qc, wsId),
  });
}
