import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { useWorkspaceId } from "../hooks";
import { issueWorkflowKeys } from "../issue-workflows/queries";
import type {
  CreateFlowDraftRequest,
  FlowDraft,
  PublishFlowDraftRequest,
  SimulateFlowDraftRequest,
  UpdateFlowDraftRequest,
} from "../types/flow-draft";
import { flowDraftKeys } from "./queries";

export interface CreateFlowDraftVariables {
  squadId: string;
  data: CreateFlowDraftRequest;
}

export function useCreateFlowDraftMutation() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();

  return useMutation({
    mutationFn: async ({ squadId, data }: CreateFlowDraftVariables) => {
      return await api.createFlowDraft(squadId, data);
    },
    onSuccess: (draft, { squadId }) => {
      qc.invalidateQueries({ queryKey: flowDraftKeys.list(wsId, squadId) });
      qc.setQueryData<FlowDraft>(flowDraftKeys.detail(wsId, squadId, draft.id), draft);
    },
  });
}

export interface UpdateFlowDraftVariables {
  squadId: string;
  draftId: string;
  data: UpdateFlowDraftRequest;
}

export function useUpdateFlowDraftMutation() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();

  return useMutation({
    mutationFn: async ({ squadId, draftId, data }: UpdateFlowDraftVariables) => {
      return await api.updateFlowDraft(squadId, draftId, data);
    },
    onSuccess: (draft, { squadId, draftId }) => {
      qc.setQueryData<FlowDraft>(flowDraftKeys.detail(wsId, squadId, draftId), draft);
      qc.invalidateQueries({ queryKey: flowDraftKeys.list(wsId, squadId) });
    },
  });
}

export interface DeleteFlowDraftVariables {
  squadId: string;
  draftId: string;
}

export function useDeleteFlowDraftMutation() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();

  return useMutation({
    mutationFn: async ({ squadId, draftId }: DeleteFlowDraftVariables) => {
      await api.deleteFlowDraft(squadId, draftId);
    },
    onSuccess: (_, { squadId, draftId }) => {
      qc.removeQueries({ queryKey: flowDraftKeys.detail(wsId, squadId, draftId) });
      qc.invalidateQueries({ queryKey: flowDraftKeys.list(wsId, squadId) });
    },
  });
}

export interface ValidateFlowDraftVariables {
  squadId: string;
  draftId: string;
  doc?: unknown;
}

export function useValidateFlowDraftMutation() {
  return useMutation({
    mutationFn: async ({ squadId, draftId, doc }: ValidateFlowDraftVariables) => {
      return await api.validateFlowDraft(squadId, draftId, { doc });
    },
  });
}

export interface SimulateFlowDraftVariables {
  squadId: string;
  draftId: string;
  data: SimulateFlowDraftRequest;
}

export function useSimulateFlowDraftMutation() {
  return useMutation({
    mutationFn: async ({ squadId, draftId, data }: SimulateFlowDraftVariables) => {
      return await api.simulateFlowDraft(squadId, draftId, data);
    },
  });
}

export interface PublishFlowDraftVariables {
  squadId: string;
  draftId: string;
  data: PublishFlowDraftRequest;
}

/**
 * Publishes a draft as a locked version.
 *
 * NOTE: Per AGENTS.md state rules, publish awaits the server before cache
 * invalidation and does NOT update the cache optimistically.
 */
export function usePublishFlowDraftMutation() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();

  return useMutation({
    mutationFn: async ({ squadId, draftId, data }: PublishFlowDraftVariables) => {
      return await api.publishFlowDraft(squadId, draftId, data);
    },
    onSuccess: (_publishedWorkflow, { squadId }) => {
      // Invalidate relevant queries after publish succeeds
      qc.invalidateQueries({ queryKey: flowDraftKeys.all(wsId) });
      qc.invalidateQueries({ queryKey: issueWorkflowKeys.all(wsId) });
      qc.invalidateQueries({ queryKey: issueWorkflowKeys.squad(wsId, squadId) });
    },
  });
}

export interface ExportFlowDraftVariables {
  squadId: string;
  draftId: string;
}

/** Reads the manifest of a saved draft: the document without its people. */
export function useExportFlowDraftMutation() {
  return useMutation({
    mutationFn: ({ squadId, draftId }: ExportFlowDraftVariables) => api.exportFlowDraft(squadId, draftId),
  });
}

/** Reads the manifest of a published version. */
export function useExportIssueWorkflowManifestMutation() {
  return useMutation({
    mutationFn: (workflowId: string) => api.getIssueWorkflowFlowManifest(workflowId),
  });
}

export const useCreateFlowDraft = useCreateFlowDraftMutation;
export const useUpdateFlowDraft = useUpdateFlowDraftMutation;
export const useDeleteFlowDraft = useDeleteFlowDraftMutation;
export const useValidateFlowDraft = useValidateFlowDraftMutation;
export const useSimulateFlowDraft = useSimulateFlowDraftMutation;
export const usePublishFlowDraft = usePublishFlowDraftMutation;
export const useExportFlowDraft = useExportFlowDraftMutation;
export const useExportIssueWorkflowManifest = useExportIssueWorkflowManifestMutation;
