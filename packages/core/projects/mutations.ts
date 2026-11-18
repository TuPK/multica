import { onlineManager, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { projectKeys } from "./queries";
import { issueKeys } from "../issues/queries";
import { useWorkspaceId } from "../hooks";
import { useRecentContextStore } from "../chat/recent-context-store";
import { clearIssueSurfaceViewState } from "../issues/stores/surface-view-store";
import { issueScopeKey } from "../issues/surface/scope";
import type { Project, CreateProjectRequest, UpdateProjectRequest, ListProjectsResponse } from "../types";
import type { ProjectAccess } from "../api/schemas";

export function useCreateProject() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();
  return useMutation({
    mutationFn: (data: CreateProjectRequest) => api.createProject(data),
    onSuccess: (newProject) => {
      qc.setQueryData<ListProjectsResponse>(projectKeys.list(wsId), (old) =>
        old && !old.projects.some((p) => p.id === newProject.id)
          ? { ...old, projects: [...old.projects, newProject], total: old.total + 1 }
          : old,
      );
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: projectKeys.list(wsId) });
    },
  });
}

export function useUpdateProject() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();
  return useMutation({
    mutationFn: ({ id, ...data }: { id: string } & UpdateProjectRequest) =>
      api.updateProject(id, data),
    onMutate: ({ id, ...data }) => {
      qc.cancelQueries({ queryKey: projectKeys.list(wsId) });
      const prevList = qc.getQueryData<ListProjectsResponse>(projectKeys.list(wsId));
      const prevDetail = qc.getQueryData<Project>(projectKeys.detail(wsId, id));
      qc.setQueryData<ListProjectsResponse>(projectKeys.list(wsId), (old) =>
        old ? { ...old, projects: old.projects.map((p) => (p.id === id ? { ...p, ...data } : p)) } : old,
      );
      qc.setQueryData<Project>(projectKeys.detail(wsId, id), (old) =>
        old ? { ...old, ...data } : old,
      );
      return { prevList, prevDetail, id };
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.prevList) qc.setQueryData(projectKeys.list(wsId), ctx.prevList);
      if (ctx?.prevDetail) qc.setQueryData(projectKeys.detail(wsId, ctx.id), ctx.prevDetail);
    },
    onSettled: (_data, _err, vars) => {
      qc.invalidateQueries({ queryKey: projectKeys.detail(wsId, vars.id) });
      qc.invalidateQueries({ queryKey: projectKeys.list(wsId) });
      // A project's status is a filter dimension of the issue table, so
      // changing it moves issues in and out of a filtered window. Nothing
      // in the issue payload changes, so only this invalidation can
      // refresh it — the global staleTime is Infinity.
      if ("status" in vars) {
        qc.invalidateQueries({ queryKey: issueKeys.tableAll(wsId) });
      }
    },
  });
}

export class ProjectDeletionOfflineError extends Error {
  constructor() {
    super("You're offline. Reconnect and try deleting again.");
    this.name = "ProjectDeletionOfflineError";
  }
}

export function useDeleteProject() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();
  return useMutation({
    // Fail offline deletions instead of queuing a destructive action behind a modal.
    networkMode: "always",
    retry: false,
    mutationFn: (id: string) => {
      if (!onlineManager.isOnline()) throw new ProjectDeletionOfflineError();
      return api.deleteProject(id);
    },
    onSuccess: async (_data, id) => {
      // Reads started during DELETE may still contain the removed project.
      await qc.cancelQueries({ queryKey: projectKeys.list(wsId) });
      qc.setQueryData<ListProjectsResponse>(projectKeys.list(wsId), (old) =>
        old && old.projects.some((p) => p.id === id)
          ? { ...old, projects: old.projects.filter((p) => p.id !== id), total: old.total - 1 }
          : old,
      );
      qc.removeQueries({ queryKey: projectKeys.detail(wsId, id) });
      useRecentContextStore.getState().forgetContext(wsId, { type: "project", id });
      clearIssueSurfaceViewState(issueScopeKey({ type: "project", projectId: id }));
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: projectKeys.list(wsId) });
      // Deleting a project removes its status from the workspace, so a table
      // window filtered on that status still holds its issues. The realtime
      // event invalidates too, but a delivery gap must not leave the window
      // wrong forever — the global staleTime is Infinity.
      qc.invalidateQueries({ queryKey: issueKeys.tableAll(wsId) });
    },
  });
}

export function useSetProjectAccess() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();
  return useMutation({
    mutationFn: ({
      id,
      access,
      member_ids,
    }: {
      id: string;
      access: ProjectAccess;
      member_ids?: string[];
    }) => api.setProjectAccess(id, { access, member_ids }),
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: projectKeys.detail(wsId, vars.id) });
      qc.invalidateQueries({ queryKey: projectKeys.members(wsId, vars.id) });
      qc.invalidateQueries({ queryKey: projectKeys.list(wsId) });
      qc.invalidateQueries({ queryKey: issueKeys.all(wsId) });
    },
  });
}

export function useAddProjectMembers() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();
  return useMutation({
    mutationFn: ({ id, user_ids }: { id: string; user_ids: string[] }) =>
      api.addProjectMembers(id, user_ids),
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: projectKeys.members(wsId, vars.id) });
    },
  });
}

export function useRemoveProjectMember() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();
  return useMutation({
    mutationFn: ({ id, userId }: { id: string; userId: string }) =>
      api.removeProjectMember(id, userId),
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: projectKeys.members(wsId, vars.id) });
    },
  });
}
