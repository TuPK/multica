import { queryOptions, type QueryClient } from "@tanstack/react-query";
import { api } from "../api";

// Every key starts with the workspace id: groups and permissions belong to one
// workspace, so a switch never shows another workspace's answer.
export const groupKeys = {
  list: (wsId: string) => ["workspaces", wsId, "groups"] as const,
  detail: (wsId: string, groupId: string) => ["workspaces", wsId, "groups", groupId] as const,
  myPermissions: (wsId: string) => ["workspaces", wsId, "permissions", "me"] as const,
  catalogue: (wsId: string) => ["workspaces", wsId, "permission-catalogue"] as const,
};

export function memberGroupListOptions(wsId: string) {
  return queryOptions({
    queryKey: groupKeys.list(wsId),
    queryFn: () => api.listMemberGroups(wsId),
  });
}

export function memberGroupDetailOptions(wsId: string, groupId: string) {
  return queryOptions({
    queryKey: groupKeys.detail(wsId, groupId),
    queryFn: () => api.getMemberGroup(wsId, groupId),
  });
}

/** The caller's role and keys. Read on every surface that shows a control. */
export function myPermissionsOptions(wsId: string) {
  return queryOptions({
    queryKey: groupKeys.myPermissions(wsId),
    queryFn: () => api.getMyPermissions(wsId),
  });
}

export function permissionCatalogueOptions(wsId: string) {
  return queryOptions({
    queryKey: groupKeys.catalogue(wsId),
    queryFn: () => api.getPermissionCatalogue(wsId),
  });
}

/** Invalidates the group lists and the members list, which shows each person's groups. */
export function invalidateGroupQueries(qc: QueryClient, wsId: string, groupId?: string) {
  void qc.invalidateQueries({ queryKey: groupKeys.list(wsId) });
  if (groupId) void qc.invalidateQueries({ queryKey: groupKeys.detail(wsId, groupId) });
  void qc.invalidateQueries({ queryKey: ["workspaces", wsId, "members"] });
}
