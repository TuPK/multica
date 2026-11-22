import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { useWorkspaceId } from "../hooks";
import { issueKeys } from "../issues/queries";
import { workspaceKeys } from "../workspace/queries";
import type {
  DetachFlowRequest,
  IssueWorkflow,
  IssueWorkflowWriteRequest,
  ResumeFlowRequest,
  StartFlowRequest,
  SetSquadActiveWorkflowRequest,
  ListIssueWorkflowsResponse,
  SetWorkflowHandoffRequest,
} from "../types";
import { issueFlowKeys, issueWorkflowKeys } from "./queries";
import type { RoomWorkflowPolicy } from "../types/room-workflow";

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

/**
 * Saves a workflow's handoff for one column, for every issue the workflow
 * governs (the rooms that use it).
 */
export function useSetWorkflowHandoff() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();
  return useMutation({
    mutationFn: ({ workflowId, statusKey, ...data }: { workflowId: string; statusKey: string } & SetWorkflowHandoffRequest) =>
      api.setWorkflowHandoff(workflowId, statusKey, data),
    onSuccess: (saved, { workflowId }) => {
      if (!saved.id) return;
      qc.setQueryData<ListIssueWorkflowsResponse>(issueWorkflowKeys.list(wsId), (old) =>
        old && { ...old, workflows: old.workflows.map((w) => (w.id === workflowId ? saved : w)) },
      );
    },
    onError: () => {
      qc.invalidateQueries({ queryKey: issueWorkflowKeys.all(wsId) });
    },
  });
}

/**
 * Flow mutations. Each waits for the server and then refreshes the request's
 * flow state and its issue: the server decides every step, so nothing here is
 * patched ahead of the answer.
 */
function useRefreshFlow(wsId: string, issueId: string) {
  const qc = useQueryClient();
  return () => {
    qc.invalidateQueries({ queryKey: issueFlowKeys.detail(wsId, issueId) });
    qc.invalidateQueries({ queryKey: issueKeys.detail(wsId, issueId) });
    qc.invalidateQueries({ queryKey: issueKeys.children(wsId, issueId) });
  };
}

/** Starts a request on a flow: another version of its squad's workflows, or a rehearsal of one. */
export function useStartIssueFlow(wsId: string, issueId: string) {
  const refresh = useRefreshFlow(wsId, issueId);
  return useMutation({
    mutationFn: (data: StartFlowRequest = {}) => api.startIssueFlow(issueId, data),
    onSettled: refresh,
  });
}

/** Runs the engine on a request: opens its tasks, or routes it on when the step is done. */
export function useAdvanceFlow(wsId: string, issueId: string) {
  const refresh = useRefreshFlow(wsId, issueId);
  return useMutation({
    mutationFn: () => api.advanceIssueFlow(issueId),
    onSettled: refresh,
  });
}

/** Continues a stopped request (members who manage the squad). */
export function useResumeFlow(wsId: string, issueId: string) {
  const refresh = useRefreshFlow(wsId, issueId);
  return useMutation({
    mutationFn: (data: ResumeFlowRequest = {}) => api.resumeIssueFlow(issueId, data),
    onSettled: refresh,
  });
}

/**
 * Closes an open task for its assignee: a member who manages the squad, with a verdict of the
 * task's set and an optional note. The server logs it as a close on behalf.
 */
export function useCloseIssueFlowTask(wsId: string, issueId: string) {
  const refresh = useRefreshFlow(wsId, issueId);
  return useMutation({
    mutationFn: ({ taskId, verdict, note }: { taskId: string; verdict: string; note?: string }) =>
      api.closeIssueFlowTask(taskId, note ? { verdict, note } : { verdict }),
    onSettled: refresh,
  });
}

/** Takes a manual exit of the step a request is at (the exit role, or members who manage the squad). */
export function useExitIssueFlow(wsId: string, issueId: string) {
  const refresh = useRefreshFlow(wsId, issueId);
  return useMutation({
    mutationFn: (edgeId: string) => api.exitIssueFlow(issueId, edgeId),
    onSettled: refresh,
  });
}

/** Takes a request off its flow (members who manage the squad). */
export function useDetachFlow(wsId: string, issueId: string) {
  const refresh = useRefreshFlow(wsId, issueId);
  return useMutation({
    mutationFn: (data: DetachFlowRequest = {}) => api.detachIssueFlow(issueId, data),
    onSettled: refresh,
  });
}

/** Turns the background runner on or off for a flow workflow (workspace admins). */
export function useSetFlowAuto(wsId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ workflowId, auto }: { workflowId: string; auto: boolean }) =>
      api.setIssueWorkflowFlowAuto(workflowId, auto),
    onSettled: () => {
      qc.invalidateQueries({ queryKey: issueWorkflowKeys.all(wsId) });
    },
  });
}

/**
 * Makes a workflow the squad's active one. A 409 `workflow_status_mapping_required`
 * carries the plan; retry with `status_mapping` once the user picks where the
 * issues go (see `issueWorkflowMappingPlanFrom`).
 */
export function useSetSquadActiveWorkflow(wsId: string, squadId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: SetSquadActiveWorkflowRequest) => api.setSquadActiveWorkflow(squadId, data),
    onSettled: () => {
      qc.invalidateQueries({ queryKey: issueWorkflowKeys.all(wsId) });
      qc.invalidateQueries({ queryKey: workspaceKeys.squads(wsId) });
    },
  });
}

/**
 * Stores the workspace's non-request workflow choice (admins only). The room
 * previews are refreshed, since the choice changes what they answer.
 */
export function useSetRoomWorkflowPolicy() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();
  return useMutation({
    mutationFn: (policy: RoomWorkflowPolicy) => api.setRoomWorkflowPolicy(wsId, policy),
    onSuccess: (saved) => {
      qc.setQueryData(issueWorkflowKeys.roomPolicy(wsId), saved);
      qc.invalidateQueries({ queryKey: issueWorkflowKeys.roomChecks(wsId) });
    },
  });
}
