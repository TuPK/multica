import { queryOptions, useMutation, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { pluginKeys } from "../plugins/queries";
import { workspaceKeys } from "../workspace/queries";
import type { HubLocalChangesPolicy } from "../types/hub";
import { hubKeys } from "./files";
import type { HubItemKind } from "./items";
import { hubItemKeys, invalidateHubItemState } from "./item-keys";

/** What changed between the installed bundle and the source's latest, file by file. */
export function hubUpdateCheckOptions(wsId: string, kind: HubItemKind, refId: string) {
  return queryOptions({
    queryKey: hubItemKeys.update(wsId, kind, refId),
    queryFn: () => api.getHubUpdateCheck(wsId, kind, refId),
    enabled: wsId.length > 0 && refId.length > 0,
  });
}

/** One file's old and new text for the update diff view. */
export function hubUpdateDiffOptions(wsId: string, kind: HubItemKind, refId: string, path: string) {
  return queryOptions({
    queryKey: hubItemKeys.updateDiff(wsId, kind, refId, path),
    queryFn: () => api.getHubUpdateDiff(wsId, kind, refId, path),
    enabled: wsId.length > 0 && refId.length > 0 && path.length > 0,
  });
}

export interface ApplyHubUpdateVars {
  wsId: string;
  kind: HubItemKind;
  refId: string;
  onLocalChanges: HubLocalChangesPolicy;
}

export interface ForkHubItemVars {
  wsId: string;
  kind: HubItemKind;
  refId: string;
}

/** Skill and plugin lists, the item's files and its update checks all change when an update lands. */
function invalidateItemContent(qc: QueryClient, wsId: string, kind: HubItemKind, refId: string) {
  qc.invalidateQueries({ queryKey: hubKeys.files(wsId, kind, refId) });
  qc.invalidateQueries({ queryKey: workspaceKeys.skills(wsId) });
  qc.invalidateQueries({ queryKey: pluginKeys.packages(wsId) });
  qc.invalidateQueries({ queryKey: hubItemKeys.update(wsId, kind, refId) });
}

/**
 * Applies a source update. Throws HubLocalChangesError when the item has local
 * edits and no policy was chosen (the server answers 409 local_changes).
 * Refreshes on settle: a failed apply may still have moved part of the item.
 */
export function useApplyHubUpdate() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ wsId, kind, refId, onLocalChanges }: ApplyHubUpdateVars) =>
      api.applyHubUpdate(wsId, kind, refId, onLocalChanges),
    onSettled: (_result, _error, vars) => {
      if (!vars) return;
      invalidateItemContent(qc, vars.wsId, vars.kind, vars.refId);
      invalidateHubItemState(qc, vars.wsId);
    },
  });
}

/** Detaches an item from its source so later syncs stop offering updates for it. */
export function useForkHubItem() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ wsId, kind, refId }: ForkHubItemVars) => api.forkHubItem(wsId, kind, refId),
    onSettled: (_result, _error, vars) => {
      if (!vars) return;
      qc.invalidateQueries({ queryKey: hubItemKeys.update(vars.wsId, vars.kind, vars.refId) });
      invalidateHubItemState(qc, vars.wsId);
    },
  });
}
