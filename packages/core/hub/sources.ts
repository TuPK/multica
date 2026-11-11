import { queryOptions, useMutation, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { pluginKeys } from "../plugins/queries";
import type {
  CreateHubSourceRequest,
  HubSourceDeleteItems,
  UpdateHubSourceRequest,
} from "../types/hub";
import type { HubSourceType } from "./items";
import { workspaceKeys } from "../workspace/queries";
import { hubKeys } from "./files";
import { invalidateHubItemState } from "./item-keys";

/** Source types that accept a private-access token. The server rejects `credential` for any other type. */
export const HUB_CREDENTIAL_SOURCE_TYPES: readonly HubSourceType[] = ["github", "url_zip", "registry"];

export function supportsHubCredential(type: HubSourceType): boolean {
  return HUB_CREDENTIAL_SOURCE_TYPES.includes(type);
}

export const hubSourceKeys = {
  all: (wsId: string) => [...hubKeys.all(wsId), "sources"] as const,
  list: (wsId: string) => [...hubSourceKeys.all(wsId), "list"] as const,
};

export function hubSourcesOptions(wsId: string) {
  return queryOptions({
    queryKey: hubSourceKeys.list(wsId),
    queryFn: () => api.listHubSources(wsId),
    enabled: wsId.length > 0,
  });
}

/**
 * Source changes can add or remove items, so the skill and plugin lists are
 * refreshed alongside the sources themselves.
 */
export function invalidateHubCatalog(qc: QueryClient, wsId: string) {
  qc.invalidateQueries({ queryKey: hubSourceKeys.all(wsId) });
  qc.invalidateQueries({ queryKey: workspaceKeys.skills(wsId) });
  qc.invalidateQueries({ queryKey: pluginKeys.packages(wsId) });
  invalidateHubItemState(qc, wsId);
}

export interface CreateHubSourceVars extends CreateHubSourceRequest {
  wsId: string;
}

export interface UpdateHubSourceVars extends UpdateHubSourceRequest {
  wsId: string;
  id: string;
}

export interface DeleteHubSourceVars {
  wsId: string;
  id: string;
  items: HubSourceDeleteItems;
}

/** Awaits the server before the caches change. Nothing is added optimistically. */
export function useCreateHubSource() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ wsId, ...data }: CreateHubSourceVars) => api.createHubSource(wsId, data),
    onSuccess: (_source, vars) => invalidateHubCatalog(qc, vars.wsId),
  });
}

export function useUpdateHubSource() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ wsId, id, ...patch }: UpdateHubSourceVars) => api.updateHubSource(wsId, id, patch),
    onSuccess: (_source, vars) => invalidateHubCatalog(qc, vars.wsId),
  });
}

/** Rejects with HubSourceInUseError when items from the source are still used by agents. */
export function useDeleteHubSource() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: DeleteHubSourceVars) => api.deleteHubSource(vars.wsId, vars.id, vars.items),
    onSuccess: (_result, vars) => invalidateHubCatalog(qc, vars.wsId),
  });
}
