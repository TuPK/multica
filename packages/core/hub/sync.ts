import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { hubSourceKeys } from "./sources";
import { hubItemKeys, invalidateHubItemState } from "./item-keys";

export interface SyncHubSourceVars {
  wsId: string;
  id: string;
}

/**
 * Checks a source for new versions of its items. The server records the sync
 * status even when it fails, so the caches refresh on settle, not only on success.
 */
export function useSyncHubSource() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ wsId, id }: SyncHubSourceVars) => api.syncHubSource(wsId, id),
    onSettled: (_result, _error, vars) => {
      if (!vars) return;
      qc.invalidateQueries({ queryKey: hubSourceKeys.all(vars.wsId) });
      qc.invalidateQueries({ queryKey: hubItemKeys.updates(vars.wsId) });
      invalidateHubItemState(qc, vars.wsId);
    },
  });
}
