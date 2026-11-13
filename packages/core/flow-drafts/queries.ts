import { queryOptions, useQuery } from "@tanstack/react-query";
import { api } from "../api";
import { useWorkspaceId } from "../hooks";

/**
 * Workspace-scoped query keys for flow drafts and capabilities.
 */
export const flowDraftKeys = {
  all: (wsId: string) => ["flow-drafts", wsId] as const,
  list: (wsId: string, squadId: string) => [...flowDraftKeys.all(wsId), "list", squadId] as const,
  detail: (wsId: string, squadId: string, draftId: string) =>
    [...flowDraftKeys.all(wsId), "detail", squadId, draftId] as const,
  capabilities: (wsId: string) => [...flowDraftKeys.all(wsId), "capabilities"] as const,
  manifest: (wsId: string, workflowId: string) => [...flowDraftKeys.all(wsId), "manifest", workflowId] as const,
};

/** Query options for listing drafts of a squad. */
export function flowDraftsOptions(wsId: string, squadId: string) {
  return queryOptions({
    queryKey: flowDraftKeys.list(wsId, squadId),
    queryFn: () => api.listFlowDrafts(squadId),
    enabled: Boolean(wsId && squadId),
    staleTime: 30_000,
  });
}

/** Hook for listing drafts of a squad. */
export function useFlowDrafts(squadId: string, wsId?: string) {
  const currentWsId = useWorkspaceId();
  const effectiveWsId = wsId || currentWsId;
  return useQuery(flowDraftsOptions(effectiveWsId, squadId));
}

/** Query options for fetching one draft with its document. */
export function flowDraftOptions(wsId: string, squadId: string, draftId: string) {
  return queryOptions({
    queryKey: flowDraftKeys.detail(wsId, squadId, draftId),
    queryFn: () => api.getFlowDraft(squadId, draftId),
    enabled: Boolean(wsId && squadId && draftId),
    staleTime: 10_000,
  });
}

/** Hook for fetching one draft with its document. */
export function useFlowDraft(squadId: string, draftId: string, wsId?: string) {
  const currentWsId = useWorkspaceId();
  const effectiveWsId = wsId || currentWsId;
  return useQuery(flowDraftOptions(effectiveWsId, squadId, draftId));
}

/** Query options for flow capabilities. */
export function flowCapabilitiesOptions(wsId: string) {
  return queryOptions({
    queryKey: flowDraftKeys.capabilities(wsId),
    queryFn: () => api.getFlowCapabilities(),
    enabled: Boolean(wsId),
    staleTime: 60 * 60_000, // Static capabilities rarely change during a session
  });
}

/** Hook for fetching flow engine capabilities. */
export function useFlowCapabilities(wsId?: string) {
  const currentWsId = useWorkspaceId();
  const effectiveWsId = wsId || currentWsId;
  return useQuery(flowCapabilitiesOptions(effectiveWsId));
}

/** Query options for exporting a workflow manifest. */
export function issueWorkflowFlowManifestOptions(wsId: string, workflowId: string) {
  return queryOptions({
    queryKey: flowDraftKeys.manifest(wsId, workflowId),
    queryFn: () => api.getIssueWorkflowFlowManifest(workflowId),
    enabled: Boolean(wsId && workflowId),
    staleTime: 5 * 60_000,
  });
}

/** Hook for exporting a workflow manifest. */
export function useIssueWorkflowFlowManifest(workflowId: string, wsId?: string) {
  const currentWsId = useWorkspaceId();
  const effectiveWsId = wsId || currentWsId;
  return useQuery(issueWorkflowFlowManifestOptions(effectiveWsId, workflowId));
}
