import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";
import type { Agent, IssueWorkflow, IssueWorkflowStep, Squad } from "../types";
import type { RoomWorkflowCheckParams } from "../types/room-workflow";

/**
 * Workspace workflows (MUL-7420).
 *
 * A project's board is its workflow's columns — the named one it picked, or
 * the workspace Default — with that workflow's handoffs on them. Helpers
 * return null only while workflows are unknown (still loading, or a server
 * without workflows), and callers then keep their pre-workflow behavior.
 */

export const issueWorkflowKeys = {
  all: (wsId: string) => ["issue-workflows", wsId] as const,
  list: (wsId: string) => [...issueWorkflowKeys.all(wsId), "list"] as const,
  detail: (wsId: string, id: string) => [...issueWorkflowKeys.all(wsId), "detail", id] as const,
  usage: (wsId: string) => [...issueWorkflowKeys.all(wsId), "usage"] as const,
  templates: (wsId: string) => [...issueWorkflowKeys.all(wsId), "templates"] as const,
  handoffPreview: (wsId: string, issueId: string, status: string, projectId = "") =>
    [...issueWorkflowKeys.all(wsId), "handoff-preview", issueId, status, projectId] as const,
  /** A squad's workflow library. Under the workflow keys, so a workflow change refreshes it. */
  squad: (wsId: string, squadId: string) => [...issueWorkflowKeys.all(wsId), "squad", squadId] as const,
  /** The workspace's choice for issues that are not requests of a flow room. */
  roomPolicy: (wsId: string) => [...issueWorkflowKeys.all(wsId), "room-policy"] as const,
  /** Previews of the room rules. Refreshed with the policy, since a choice changes them. */
  roomChecks: (wsId: string) => [...issueWorkflowKeys.all(wsId), "room-check"] as const,
  roomCheck: (wsId: string, params: RoomWorkflowCheckParams) =>
    [...issueWorkflowKeys.roomChecks(wsId), params.projectId ?? "", params.assigneeType ?? "", params.assigneeId ?? "", params.parentIssueId ?? ""] as const,
};

/**
 * The flow state of a request. Under the workflow keys as well: a workflow
 * change can change a request's steps, and the `issue_flow:updated` event
 * refreshes one request's state.
 */
export const issueFlowKeys = {
  all: (wsId: string) => [...issueWorkflowKeys.all(wsId), "flow"] as const,
  detail: (wsId: string, issueId: string) => [...issueFlowKeys.all(wsId), issueId] as const,
};

/**
 * The flow state of a request, or of the request a task belongs to. `managed`
 * is false for an issue no flow runs. The server keeps it current and tells
 * clients through `issue_flow:updated`, so a short stale time only covers the
 * gap between a write and its event.
 */
export function issueFlowOptions(wsId: string, issueId: string) {
  return queryOptions({
    queryKey: issueFlowKeys.detail(wsId, issueId),
    queryFn: () => api.getIssueFlow(issueId),
    staleTime: 15_000,
  });
}

/** The workspace's non-request workflow choice (squad-room-workflow spec, section 4). */
export function roomWorkflowPolicyOptions(wsId: string) {
  return queryOptions({
    queryKey: issueWorkflowKeys.roomPolicy(wsId),
    queryFn: () => api.getRoomWorkflowPolicy(wsId),
    staleTime: 60_000,
  });
}

/** Whether a write with these inputs would be refused by the room rules (spec section 6). */
export function roomWorkflowCheckOptions(wsId: string, params: RoomWorkflowCheckParams) {
  return queryOptions({
    queryKey: issueWorkflowKeys.roomCheck(wsId, params),
    queryFn: () => api.checkRoomWorkflow(wsId, params),
    staleTime: 0,
  });
}

/** A squad's workflow library and which workflow is active. */
export function squadWorkflowsOptions(wsId: string, squadId: string) {
  return queryOptions({
    queryKey: issueWorkflowKeys.squad(wsId, squadId),
    queryFn: () => api.listSquadWorkflows(squadId),
    staleTime: 5 * 60_000,
  });
}

/**
 * What moving an issue to `status` — or into project `projectId` — would do.
 * Short-lived: the answer depends on the issue's assignee and runs, which
 * change under it.
 */
export function workflowHandoffPreviewOptions(wsId: string, issueId: string, status: string, projectId?: string) {
  return queryOptions({
    queryKey: issueWorkflowKeys.handoffPreview(wsId, issueId, status, projectId),
    queryFn: () => api.previewWorkflowHandoff(issueId, status, projectId),
    staleTime: 0,
    gcTime: 30_000,
  });
}

export function issueWorkflowListOptions(wsId: string) {
  return queryOptions({
    queryKey: issueWorkflowKeys.list(wsId),
    queryFn: () => api.listIssueWorkflows(),
    select: (data) => data.workflows,
    // Workflows change only when an admin edits one; the
    // `issue_workflow:changed` event refreshes every client when that happens.
    staleTime: 5 * 60_000,
  });
}

/**
 * One workflow, read from the server. The designer opens on this so it edits
 * the stored copy rather than a list snapshot.
 */
export function issueWorkflowDetailOptions(wsId: string, id: string) {
  return queryOptions({
    queryKey: issueWorkflowKeys.detail(wsId, id),
    queryFn: () => api.getIssueWorkflow(id),
    staleTime: 5 * 60_000,
  });
}

/**
 * Where each workflow is assigned: the projects, squads and agents that use it.
 * Refreshed with the workflow list through the same `issue-workflows` prefix.
 */
export function issueWorkflowUsageOptions(wsId: string) {
  return queryOptions({
    queryKey: issueWorkflowKeys.usage(wsId),
    queryFn: () => api.listIssueWorkflowUsage(),
    select: (data) => data.usage,
    staleTime: 60_000,
  });
}

/**
 * The system templates a new workflow can start from. They change only with a
 * server release, so a long stale time is safe.
 */
export function issueWorkflowTemplatesOptions(wsId: string) {
  return queryOptions({
    queryKey: issueWorkflowKeys.templates(wsId),
    queryFn: () => api.listIssueWorkflowTemplates(),
    select: (data) => data.templates,
    staleTime: 5 * 60_000,
  });
}

/** The workspace Default workflow, once loaded. */
export function defaultWorkflow(workflows: readonly IssueWorkflow[] | undefined): IssueWorkflow | null {
  return workflows?.find((w) => w.is_default) ?? null;
}

/** Statuses with fixed places at the ends of a workflow, in the order they sit. */
export const WORKFLOW_HEAD_STATUSES = ["backlog", "todo"] as const;
export const WORKFLOW_TAIL_STATUSES = ["blocked", "done", "cancelled"] as const;
/** Fixed statuses a workflow may leave out; the others every workflow lists. */
export const WORKFLOW_OPTIONAL_STATUSES = ["backlog", "blocked"] as const;
const FIXED_STATUSES = new Set<string>([...WORKFLOW_HEAD_STATUSES, ...WORKFLOW_TAIL_STATUSES]);
const OPTIONAL_STATUSES = new Set<string>(WORKFLOW_OPTIONAL_STATUSES);
const NO_HANDOFF_STATUSES = new Set(["backlog", "done", "cancelled"]);

/** Whether a status has a fixed place when a workflow lists it. */
export function isFixedWorkflowStatus(statusKey: string): boolean {
  return FIXED_STATUSES.has(statusKey);
}

/** Whether a fixed status may be left out of a workflow. */
export function isOptionalWorkflowStatus(statusKey: string): boolean {
  return OPTIONAL_STATUSES.has(statusKey);
}

/** Whether a column can have a handler: parking and closing columns cannot. */
export function statusAcceptsHandoff(statusKey: string): boolean {
  return !NO_HANDOFF_STATUSES.has(statusKey);
}

/** The columns between the fixed head and tail, in order. */
export function workflowMiddleKeys(workflow: Pick<IssueWorkflow, "steps">): string[] {
  return workflow.steps.map((s) => s.status_key).filter((k) => !FIXED_STATUSES.has(k));
}

/**
 * The column a step's handler is told to move to when done: the step's own
 * choice, else the next column, skipping blocked and closed ones; empty at the
 * end. With `categoryOf`, it matches the agent's brief for custom statuses
 * too: a closed column such as Won't Fix is never where finished work goes,
 * and a done or closed column routes only where its author chose (MUL-9913).
 */
export function workflowNextStatus(
  workflow: Pick<IssueWorkflow, "steps">,
  statusKey: string,
  categoryOf?: (statusKey: string) => string,
): string {
  const index = workflow.steps.findIndex((s) => s.status_key === statusKey);
  if (index < 0) return "";
  const own = workflow.steps[index]?.next_status_key;
  if (own) return own;
  const category = categoryOf?.(statusKey);
  if (category === "done" || category === "closed") return "";
  for (const s of workflow.steps.slice(index + 1)) {
    if (s.status_key !== "blocked" && categoryOf?.(s.status_key) !== "closed") return s.status_key;
  }
  return "";
}

/** The squads and agents the room rules read (see `useRoomRoster`). */
export interface RoomRoster {
  squads: readonly Pick<Squad, "id" | "active_workflow_id">[] | undefined;
  agents: readonly Pick<Agent, "id" | "home_squad_id">[] | undefined;
}

/** The issue fields the room rules read. */
export interface RoomIssueRef {
  project_id: string | null;
  assignee_type: string | null;
  assignee_id: string | null;
}

/**
 * The workflow an issue follows (squad-room-workflow spec, section 3): the
 * active workflow of the squad it is assigned to, or of the agent's home squad.
 * An issue with no project or no room, a room with no workflow, and a flow room
 * (whose requests run on the flow) all read the workspace Default, as the server
 * does for statuses. Null while workflows or the roster are not loaded.
 */
export function issueRoomWorkflow(
  workflows: readonly IssueWorkflow[] | undefined,
  roster: RoomRoster,
  issue: RoomIssueRef,
): IssueWorkflow | null {
  if (!workflows) return null;
  const fallback = defaultWorkflow(workflows);
  if (!issue.project_id) return fallback;
  if (!roster.squads || !roster.agents) return null;
  let squadID: string | null = null;
  if (issue.assignee_type === "squad") {
    squadID = issue.assignee_id;
  } else if (issue.assignee_type === "agent" && issue.assignee_id) {
    squadID = roster.agents.find((a) => a.id === issue.assignee_id)?.home_squad_id ?? null;
  }
  if (!squadID) return fallback;
  const activeID = roster.squads.find((s) => s.id === squadID)?.active_workflow_id ?? null;
  const room = activeID ? workflows.find((w) => w.id === activeID) : undefined;
  if (!room || room.flow_config) return fallback;
  return room;
}

/**
 * The statuses every one of `issues` may take, or null when none is limited.
 * Each issue takes its room's workflow statuses, as the server enforces. A batch
 * status change offers this set, so it never shows a status the server would skip.
 */
export function sharedWorkflowStatusKeys(
  workflows: readonly IssueWorkflow[] | undefined,
  roster: RoomRoster,
  issues: readonly RoomIssueRef[],
): Set<string> | null {
  let shared = null as Set<string> | null;
  for (const issue of issues) {
    const workflow = issueRoomWorkflow(workflows, roster, issue);
    if (!workflow) continue;
    const keys = new Set(workflow.steps.map((s) => s.status_key));
    shared = shared ? new Set([...shared].filter((key) => keys.has(key))) : keys;
  }
  return shared;
}

export function workflowStep(
  workflow: IssueWorkflow | null | undefined,
  statusKey: string,
): IssueWorkflowStep | undefined {
  return workflow?.steps.find((s) => s.status_key === statusKey);
}

/** Whether entering the step reassigns the issue. */
export function stepHandsOff(step: IssueWorkflowStep | undefined): boolean {
  return !!step && step.handler.type !== "none";
}

/**
 * Whether issues on `workflow` may enter a status: the ones it lists, Default
 * included (MUL-8250). An issue already on a status its workflow no longer
 * lists keeps it, so callers exempt the issue's current status. Null —
 * workflows still loading — allows every status.
 */
export function workflowAllowsStatus(
  workflow: IssueWorkflow | null | undefined,
  statusKey: string,
): boolean {
  if (!workflow) return true;
  return workflow.steps.some((s) => s.status_key === statusKey);
}

/**
 * Whether moving an issue on `statusKey` into a project using `workflow` may
 * hand it off, when the move enters the column it lands on: the column it
 * keeps hands off, or it lands on another column and some column does. The
 * server preview has the final word.
 */
export function projectMoveMayHandOff(workflow: IssueWorkflow | null | undefined, statusKey: string): boolean {
  if (!workflow) return false;
  // A move into a Default project keeps the issue's status, listed or not.
  if (workflow.is_default || workflowAllowsStatus(workflow, statusKey)) {
    return stepHandsOff(workflowStep(workflow, statusKey));
  }
  return workflow.steps.some(stepHandsOff);
}

/** Steps whose entry reassigns the issue. */
export function handoffStepCount(workflow: IssueWorkflow): number {
  return workflow.steps.filter((s) => s.handler.type !== "none").length;
}
