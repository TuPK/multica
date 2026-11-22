import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";
import { ISSUE_WORKFLOW_TEMPLATE_IDS } from "../api/schemas";
import { agentListOptions, squadListOptions } from "../workspace/queries";
import type { Agent, IssueWorkflow, IssueWorkflowTemplate, IssueWorkflowUsage, Squad } from "../types";
import {
  issueWorkflowDetailOptions,
  issueWorkflowListOptions,
  issueWorkflowTemplatesOptions,
  issueWorkflowUsageOptions,
  roomWorkflowCheckOptions,
  roomWorkflowPolicyOptions,
} from "./queries";
import type { RoomWorkflowCheck, RoomWorkflowCheckParams, RoomWorkflowPolicy } from "../types/room-workflow";

const EMPTY: IssueWorkflow[] = [];
const EMPTY_TEMPLATES: IssueWorkflowTemplate[] = [];

/** The workspace's workflows, in name order. Empty until loaded. */
export function useIssueWorkflows(wsId: string): { workflows: IssueWorkflow[]; isLoaded: boolean } {
  const { data } = useQuery({ ...issueWorkflowListOptions(wsId), enabled: Boolean(wsId) });
  return useMemo(() => ({ workflows: data ?? EMPTY, isLoaded: data !== undefined }), [data]);
}

/**
 * One workflow as the server stores it. `workflow` is null while loading, when
 * the id is missing, and when the server has no such workflow.
 */
export function useIssueWorkflowDetail(
  wsId: string,
  workflowId: string | null | undefined,
): { workflow: IssueWorkflow | null; isLoading: boolean; isError: boolean } {
  const query = useQuery({
    ...issueWorkflowDetailOptions(wsId, workflowId ?? ""),
    enabled: Boolean(wsId) && Boolean(workflowId),
  });
  return {
    workflow: query.data && query.data.id ? query.data : null,
    isLoading: Boolean(workflowId) && query.isPending,
    isError: query.isError || (query.data !== undefined && !query.data.id),
  };
}

/** Each workflow's assignments by workflow id. Empty until loaded. */
export function useIssueWorkflowUsage(wsId: string): {
  usageById: Record<string, IssueWorkflowUsage>;
  isLoaded: boolean;
} {
  const { data } = useQuery({ ...issueWorkflowUsageOptions(wsId), enabled: Boolean(wsId) });
  return useMemo(() => {
    const usageById: Record<string, IssueWorkflowUsage> = {};
    for (const usage of data ?? []) usageById[usage.workflow_id] = usage;
    return { usageById, isLoaded: data !== undefined };
  }, [data]);
}

/**
 * The squads and agents that decide which workflow an issue follows (the room
 * rules). Each list is undefined until loaded.
 */
export function useRoomRoster(wsId: string): {
  squads: Pick<Squad, "id" | "active_workflow_id">[] | undefined;
  agents: Pick<Agent, "id" | "home_squad_id">[] | undefined;
} {
  const { data: squads } = useQuery({ ...squadListOptions(wsId), enabled: Boolean(wsId) });
  const { data: agents } = useQuery({ ...agentListOptions(wsId), enabled: Boolean(wsId) });
  return useMemo(() => ({ squads, agents }), [squads, agents]);
}

/**
 * The system templates a new workflow can start from. A template id this build
 * does not know is skipped. Empty until loaded.
 */
export function useIssueWorkflowTemplates(wsId: string): {
  templates: IssueWorkflowTemplate[];
  isLoading: boolean;
  isError: boolean;
  refetch: () => void;
} {
  const query = useQuery({ ...issueWorkflowTemplatesOptions(wsId), enabled: Boolean(wsId) });
  const refetch = query.refetch;
  return useMemo(
    () => ({
      templates: query.data ? query.data.filter((t) => ISSUE_WORKFLOW_TEMPLATE_IDS.has(t.id)) : EMPTY_TEMPLATES,
      isLoading: query.isPending && Boolean(wsId),
      isError: query.isError,
      refetch: () => void refetch(),
    }),
    [query.data, query.isPending, query.isError, refetch, wsId],
  );
}

/** The workspace's non-request workflow choice. `policy` is null until loaded. */
export function useRoomWorkflowPolicy(wsId: string): { policy: RoomWorkflowPolicy | null; isLoaded: boolean } {
  const { data } = useQuery({ ...roomWorkflowPolicyOptions(wsId), enabled: Boolean(wsId) });
  return useMemo(() => ({ policy: data ?? null, isLoaded: data !== undefined }), [data]);
}

/**
 * Whether the room rules would refuse a write with these inputs. `check` is null
 * until the preview answers; callers treat null as "nothing to ask".
 */
export function useRoomWorkflowCheck(
  wsId: string,
  params: RoomWorkflowCheckParams,
  enabled = true,
): { check: RoomWorkflowCheck | null; isLoaded: boolean } {
  const { data } = useQuery({ ...roomWorkflowCheckOptions(wsId, params), enabled: enabled && Boolean(wsId) });
  return useMemo(() => ({ check: data ?? null, isLoaded: data !== undefined }), [data]);
}
