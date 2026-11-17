import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import type { MemberGroup } from "./schema";
import { invalidateGroupQueries } from "./queries";

// Create, delete and membership changes await the server before the caches
// move (AGENTS.md "State Rules"): nothing is patched optimistically.

export function useCreateMemberGroup(wsId: string) {
  const qc = useQueryClient();
  return useMutation<MemberGroup, Error, { name: string; description?: string }>({
    mutationFn: (data) => api.createMemberGroup(wsId, data),
    onSettled: () => invalidateGroupQueries(qc, wsId),
  });
}

export function useUpdateMemberGroup(wsId: string, groupId: string) {
  const qc = useQueryClient();
  return useMutation<MemberGroup, Error, { name?: string; description?: string }>({
    mutationFn: (data) => api.updateMemberGroup(wsId, groupId, data),
    onSettled: () => invalidateGroupQueries(qc, wsId, groupId),
  });
}

export function useDeleteMemberGroup(wsId: string) {
  const qc = useQueryClient();
  return useMutation<void, Error, string>({
    mutationFn: (groupId) => api.deleteMemberGroup(wsId, groupId),
    onSettled: (_data, _err, groupId) => invalidateGroupQueries(qc, wsId, groupId),
  });
}

export function useSetMemberGroupPermissions(wsId: string, groupId: string) {
  const qc = useQueryClient();
  return useMutation<MemberGroup, Error, string[]>({
    mutationFn: (permissions) => api.setMemberGroupPermissions(wsId, groupId, permissions),
    onSettled: () => invalidateGroupQueries(qc, wsId, groupId),
  });
}

export function useAddMemberGroupMembers(wsId: string, groupId: string) {
  const qc = useQueryClient();
  return useMutation<string[], Error, string[]>({
    mutationFn: (userIds) => api.addMemberGroupMembers(wsId, groupId, userIds),
    onSettled: () => invalidateGroupQueries(qc, wsId, groupId),
  });
}

export function useRemoveMemberGroupMember(wsId: string, groupId: string) {
  const qc = useQueryClient();
  return useMutation<void, Error, string>({
    mutationFn: (userId) => api.removeMemberGroupMember(wsId, groupId, userId),
    onSettled: () => invalidateGroupQueries(qc, wsId, groupId),
  });
}
