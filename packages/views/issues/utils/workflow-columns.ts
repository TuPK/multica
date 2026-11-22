import { useMemo } from "react";
import {
  defaultWorkflow,
  issueRoomWorkflow,
  useIssueWorkflows,
  useRoomRoster,
} from "@multica/core/issue-workflows";
import type { RoomIssueRef, RoomRoster } from "@multica/core/issue-workflows/queries";
import type { IssueStatus, IssueWorkflow, IssueWorkflowStep } from "@multica/core/types";

export interface ColumnWorkflowResult {
  /**
   * The workflow (or union of room workflows) whose steps define the board's
   * columns, or null when columns pass through from the catalog/filter.
   */
  workflow: IssueWorkflow | null;
  /**
   * Which workflow owns each column's status key (for column handoff metadata).
   * When multiple room workflows share a status key, the first workflow in
   * appearance order owns the column.
   */
  workflowByStatus: ReadonlyMap<IssueStatus, IssueWorkflow>;
}

const EMPTY_WORKFLOW_BY_STATUS: ReadonlyMap<IssueStatus, IssueWorkflow> = new Map();

/**
 * Combines one or more workflows into a single ordered step list (union in
 * order of first appearance) and records which workflow owns each status key.
 */
export function mergeColumnWorkflows(
  workflows: readonly IssueWorkflow[],
): ColumnWorkflowResult {
  if (workflows.length === 0) {
    return { workflow: null, workflowByStatus: EMPTY_WORKFLOW_BY_STATUS };
  }
  if (workflows.length === 1) {
    const only = workflows[0]!;
    const workflowByStatus = new Map<IssueStatus, IssueWorkflow>();
    for (const step of only.steps) {
      const key = step.status_key as IssueStatus;
      if (!workflowByStatus.has(key)) {
        workflowByStatus.set(key, only);
      }
    }
    return { workflow: only, workflowByStatus };
  }

  const seen = new Set<string>();
  const steps: IssueWorkflowStep[] = [];
  const workflowByStatus = new Map<IssueStatus, IssueWorkflow>();
  for (const wf of workflows) {
    for (const step of wf.steps) {
      if (seen.has(step.status_key)) continue;
      seen.add(step.status_key);
      steps.push(step);
      workflowByStatus.set(step.status_key as IssueStatus, wf);
    }
  }
  const base = workflows[0]!;
  return {
    workflow: { ...base, steps },
    workflowByStatus,
  };
}

/**
 * The distinct workflows followed by a project's issues, in the order they
 * first appear (squad-room-workflow spec, O-3). A workflow no issue in the
 * project uses is excluded. Falls back to workspace Default when the project
 * has no issues yet; returns empty while workflows or the roster are loading.
 */
export function projectBoardWorkflows(
  workflows: readonly IssueWorkflow[] | undefined,
  roster: RoomRoster,
  issues: readonly RoomIssueRef[] | undefined,
): IssueWorkflow[] {
  if (!workflows || workflows.length === 0) return [];
  const fallback = defaultWorkflow(workflows);
  if (!issues || issues.length === 0) {
    return fallback ? [fallback] : [];
  }
  if (!roster.squads || !roster.agents) return [];

  const seen = new Set<string>();
  const resolved: IssueWorkflow[] = [];
  for (const issue of issues) {
    const wf = issueRoomWorkflow(workflows, roster, issue);
    if (wf && !seen.has(wf.id)) {
      seen.add(wf.id);
      resolved.push(wf);
    }
  }
  if (resolved.length === 0) {
    return fallback ? [fallback] : [];
  }
  return resolved;
}

/**
 * The status columns of a surface whose columns follow a workflow (MUL-7420):
 * the workflow's steps, in workflow order, narrowed to `visible` (hidden-column
 * preferences and status filters). Without a workflow the catalog-ordered
 * `visible` list passes through unchanged. `visible === null` returns every
 * step.
 */
export function workflowColumns(
  visible: readonly IssueStatus[] | null,
  workflow: IssueWorkflow | null,
): IssueStatus[] {
  if (!workflow) return visible ? [...visible] : [];
  const keys = workflow.steps.map((s) => s.status_key as IssueStatus);
  if (!visible) return keys;
  const allowed = new Set(visible);
  return keys.filter((key) => allowed.has(key));
}

/**
 * The workflow(s) a surface's columns follow.
 * - With an explicit status filter: returns null so filtered statuses always show
 *   (including statuses outside the room workflows).
 * - Outside a project (workspace-wide): follows the workspace Default workflow.
 * - On a project board: union of the room workflows its issues follow, in the
 *   order each workflow's statuses first appear among the project's issues.
 */
export function useColumnWorkflow(
  wsId: string,
  projectId: string | null | undefined,
  hasStatusFilter: boolean,
  issues?: readonly RoomIssueRef[],
): ColumnWorkflowResult {
  const { workflows } = useIssueWorkflows(wsId);
  const roster = useRoomRoster(wsId);
  return useMemo(() => {
    if (hasStatusFilter) {
      return { workflow: null, workflowByStatus: EMPTY_WORKFLOW_BY_STATUS };
    }
    if (!projectId) {
      const def = defaultWorkflow(workflows);
      return mergeColumnWorkflows(def ? [def] : []);
    }
    return mergeColumnWorkflows(projectBoardWorkflows(workflows, roster, issues));
  }, [hasStatusFilter, issues, projectId, roster, workflows]);
}
