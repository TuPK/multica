import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";
import { projectListOptions } from "../projects/queries";
import type { IssueWorkflow, IssueWorkflowUsage, Project } from "../types";
import {
  defaultWorkflow,
  issueWorkflowDetailOptions,
  issueWorkflowListOptions,
  issueWorkflowUsageOptions,
  resolveProjectWorkflow,
} from "./queries";

const EMPTY: IssueWorkflow[] = [];

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
 * A project and the workflow it uses. `workflow` is null for the Default
 * workflow — including while either list is still loading, which renders the
 * Default behavior rather than an empty board.
 *
 * `defaultWorkflow` is the workspace Default itself, null until loaded or on a
 * server without workflows. An issue outside any project offers its statuses.
 * (MUL-8121)
 */
export function useProjectWithWorkflow(
  wsId: string,
  projectId: string | null | undefined,
): { project: Project | null; workflow: IssueWorkflow | null; defaultWorkflow: IssueWorkflow | null } {
  const { workflows } = useIssueWorkflows(wsId);
  const { data: projects } = useQuery({
    ...projectListOptions(wsId),
    enabled: Boolean(wsId) && Boolean(projectId),
  });
  return useMemo(() => {
    const project = projectId ? projects?.find((p) => p.id === projectId) ?? null : null;
    return {
      project,
      workflow: resolveProjectWorkflow(workflows, project),
      defaultWorkflow: defaultWorkflow(workflows),
    };
  }, [projectId, projects, workflows]);
}

/** The workflow a project uses, or null for the Default workflow. */
export function useProjectWorkflow(
  wsId: string,
  projectId: string | null | undefined,
): IssueWorkflow | null {
  return useProjectWithWorkflow(wsId, projectId).workflow;
}
