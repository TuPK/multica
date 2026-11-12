import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { useWorkspaceId } from "../hooks";
import { issueKeys } from "../issues/queries";
import { projectKeys } from "../projects/queries";
import type {
  IssueWorkflow,
  IssueWorkflowWriteRequest,
  ListIssueWorkflowsResponse,
  SetProjectWorkflowRequest,
  SetWorkflowHandoffRequest,
} from "../types";
import { issueWorkflowKeys } from "./queries";

/**
 * Workflow mutations (MUL-7420).
 *
 * A writer patches its own cache entries on success. Other clients refresh
 * through the `issue_workflow:changed` realtime event. A failed write
 * invalidates, because the local copy is the likely stale one.
 */

function replaceWorkflowInList(
  qc: ReturnType<typeof useQueryClient>,
  wsId: string,
  saved: IssueWorkflow,
) {
  // A malformed response parses to the empty fallback; caching it would hide the workflow.
  if (!saved.id) return;
  qc.setQueryData<ListIssueWorkflowsResponse>(issueWorkflowKeys.list(wsId), (old) =>
    old && {
      ...old,
      workflows: old.workflows.some((w) => w.id === saved.id)
        ? old.workflows.map((w) => (w.id === saved.id ? saved : w))
        : [...old.workflows, saved],
    },
  );
  qc.setQueryData<IssueWorkflow>(issueWorkflowKeys.detail(wsId, saved.id), saved);
}

export function useCreateIssueWorkflow() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();
  return useMutation({
    mutationFn: (data: IssueWorkflowWriteRequest) => api.createIssueWorkflow(data),
    onSuccess: (saved) => {
      replaceWorkflowInList(qc, wsId, saved);
    },
    onError: () => {
      qc.invalidateQueries({ queryKey: issueWorkflowKeys.all(wsId) });
    },
  });
}

/**
 * Saves a workflow. A 409 `workflow_status_mapping_required` error means issues
 * sit on statuses the edit removes: retry with `status_mapping` once the user
 * picks where they go (see `issueWorkflowMappingPlanFrom`).
 */
export function useUpdateIssueWorkflow() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();
  return useMutation({
    mutationFn: ({ id, ...data }: { id: string } & IssueWorkflowWriteRequest) =>
      api.updateIssueWorkflow(id, data),
    onSuccess: (saved) => {
      replaceWorkflowInList(qc, wsId, saved);
    },
    onError: () => {
      qc.invalidateQueries({ queryKey: issueWorkflowKeys.all(wsId) });
    },
  });
}

/**
 * Deletes a workflow. A refused delete (409 with `reasons`, or 400
 * `workflow_is_default`) rejects with the ApiError; read the reasons with
 * `parseIssueWorkflowBlockers(err.body)`.
 */
export function useDeleteIssueWorkflow() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();
  return useMutation({
    mutationFn: (id: string) => api.deleteIssueWorkflow(id),
    onSuccess: (_data, id) => {
      qc.setQueryData<ListIssueWorkflowsResponse>(issueWorkflowKeys.list(wsId), (old) =>
        old && { ...old, workflows: old.workflows.filter((w) => w.id !== id) },
      );
      qc.removeQueries({ queryKey: issueWorkflowKeys.detail(wsId, id) });
    },
  });
}

export function useSetProjectWorkflow() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();
  return useMutation({
    mutationFn: ({ projectId, ...data }: { projectId: string } & SetProjectWorkflowRequest) =>
      api.setProjectWorkflow(projectId, data),
    onSettled: (_data, _error, { projectId }) => {
      qc.invalidateQueries({ queryKey: projectKeys.list(wsId) });
      qc.invalidateQueries({ queryKey: projectKeys.detail(wsId, projectId) });
      // project_ids on each workflow row changed.
      qc.invalidateQueries({ queryKey: issueWorkflowKeys.all(wsId) });
      // Remapped issues moved columns.
      qc.invalidateQueries({ queryKey: issueKeys.all(wsId) });
    },
  });
}

/**
 * Saves a workflow's handoff for one column, for every project using it — or,
 * with `copyForProjectId`, on a copy only that project uses.
 */
export function useSetWorkflowHandoff() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();
  return useMutation({
    mutationFn: ({
      workflowId,
      statusKey,
      copyForProjectId,
      ...data
    }: { workflowId: string; statusKey: string; copyForProjectId?: string } & SetWorkflowHandoffRequest) =>
      api.setWorkflowHandoff(workflowId, statusKey, data, copyForProjectId),
    onSuccess: (saved, { workflowId, copyForProjectId }) => {
      if (!saved.id) return;
      if (copyForProjectId) {
        // A new workflow row, and the project now points at it.
        qc.invalidateQueries({ queryKey: issueWorkflowKeys.all(wsId) });
        qc.invalidateQueries({ queryKey: projectKeys.list(wsId) });
        qc.invalidateQueries({ queryKey: projectKeys.detail(wsId, copyForProjectId) });
        return;
      }
      qc.setQueryData<ListIssueWorkflowsResponse>(issueWorkflowKeys.list(wsId), (old) =>
        old && { ...old, workflows: old.workflows.map((w) => (w.id === workflowId ? saved : w)) },
      );
    },
    onError: () => {
      qc.invalidateQueries({ queryKey: issueWorkflowKeys.all(wsId) });
    },
  });
}
