/**
 * Workspace workflows (MUL-7420).
 *
 * A workflow is a whole pipeline: an ordered pick from the shared status
 * catalog plus who each column hands an issue to. Every workflow lists todo,
 * done and cancelled; backlog and blocked are optional, and the five keep fixed
 * places at the ends. Entering a column through a status change assigns the
 * issue to the column's handler and, for an agent, starts its run with the
 * column's instructions. A project only picks a workflow; projects that name
 * none use the workspace's Default workflow.
 */

export type IssueWorkflowHandlerType =
  | "none"
  | "agent"
  | "squad"
  | "member"
  | "project_lead"
  | "creator";

export interface IssueWorkflowHandler {
  type: IssueWorkflowHandlerType;
  /** Set only for agent, squad and member handlers. */
  id?: string;
}

/** A column with its handoff; handler "none" keeps the assignee. */
export interface IssueWorkflowStep {
  status_key: string;
  handler: IssueWorkflowHandler;
  instructions: string;
  /**
   * Where the issue goes when the column is left, in evaluation order: the
   * first branch whose condition holds wins, and a branch with no condition
   * is the default. Canonical routing (workflow-tab §3).
   */
  transitions?: IssueWorkflowTransition[];
  /** Target of the default branch; derived from `transitions` by the server. */
  next_status_key?: string;
  /** Status to send the issue back to when it needs changes; derived from `transitions`. */
  back_status_key?: string;
}

/** What a handler reports when it moves an issue out of a column. */
export type IssueOutcome = "approved" | "rejected" | "blocked" | "done";

export type WorkflowConditionField =
  | "outcome"
  | "priority"
  | "labels"
  | "project_id"
  | "rework_count"
  | "creator_type"
  | "assignee_type";

export type WorkflowConditionOperator = "eq" | "neq" | "in" | "gt" | "gte" | "lt" | "lte" | "exists";

/**
 * A routing condition: a group (`all` is AND, `any` is OR), a negation, or a
 * field check. Exactly one form is set. Field and op accept a string beyond the
 * known values, so a newer server's condition still reaches the designer.
 */
export type WorkflowCondition =
  | { all: WorkflowCondition[] }
  | { any: WorkflowCondition[] }
  | { not: WorkflowCondition }
  | {
      field: WorkflowConditionField | (string & {});
      op: WorkflowConditionOperator | (string & {});
      value?: unknown;
    };

/** One routing branch of a column. `when` null is the default branch. */
export interface IssueWorkflowTransition {
  id: string;
  label: string;
  when: WorkflowCondition | null;
  to_status_key: string;
  /** Replaces the target column's handler for this branch only. */
  handler?: IssueWorkflowHandler;
}

/** Where one workflow is assigned (GET /api/issue-workflows/usage). */
export interface IssueWorkflowUsage {
  workflow_id: string;
  project_ids: string[];
  /** Squads whose active workflow this is. */
  squad_ids: string[];
  /** Agents the workflow's columns and branches hand work to. */
  agent_ids: string[];
}

export interface ListIssueWorkflowUsageResponse {
  usage: IssueWorkflowUsage[];
  total: number;
}

/** One reason a workflow cannot be deleted, from the delete conflict body. */
export interface IssueWorkflowBlocker {
  code: string;
  count: number;
  ids: string[];
  message: string;
}

export interface IssueWorkflow {
  id: string;
  workspace_id: string;
  name: string;
  description: string;
  /** The workspace Default: used by every project that names no workflow. */
  is_default: boolean;
  /** The member who owns the workflow and may edit it; null for Default. */
  owner_id: string | null;
  steps: IssueWorkflowStep[];
  /** Projects currently using this workflow. */
  project_ids: string[];
  created_at: string;
  updated_at: string;
}

export interface ListIssueWorkflowsResponse {
  workflows: IssueWorkflow[];
  total: number;
}

/** A status for a new column, created with the workflow write. */
export interface NewWorkflowStatus {
  name: string;
  category?: string;
  color?: string;
}

/** A column of a workflow write: an existing status or a new one, with its handoff. */
export type IssueWorkflowStepInput = ({ status_key: string } | { new_status: NewWorkflowStatus }) &
  Partial<SetWorkflowHandoffRequest> & {
    /** Canonical routing. The server reads it over next_status_key and back_status_key when sent. */
    transitions?: IssueWorkflowTransition[];
  };

export interface IssueWorkflowWriteRequest {
  name?: string;
  description?: string;
  /**
   * Every column with its handoff. The server adds todo, done and cancelled
   * when missing and places the fixed columns; backlog and blocked are listed
   * only when included.
   */
  steps?: IssueWorkflowStepInput[];
  /** Destination for issues on statuses an edit removes, keyed by removed status. */
  status_mapping?: Record<string, string>;
  dry_run?: boolean;
}

/** A status some issues are on that the target workflow does not list. */
export interface IssueWorkflowMappingRequirement {
  status_key: string;
  issue_count: number;
  suggested_status_key: string;
}

/** A status some issues are on that both workflows list, so they keep it. */
export interface IssueWorkflowStatusCount {
  status_key: string;
  issue_count: number;
}

export interface IssueWorkflowMappingPlan {
  required: IssueWorkflowMappingRequirement[];
  unchanged: IssueWorkflowStatusCount[];
  total_issues: number;
  affected_issues: number;
}

export interface SetProjectWorkflowRequest {
  /** null switches the project back to the Default workflow. */
  workflow_id: string | null;
  status_mapping?: Record<string, string>;
  dry_run?: boolean;
}

export interface IssueWorkflowDryRunResponse {
  dry_run: true;
  plan: IssueWorkflowMappingPlan;
}

/** An active run of the agent a handoff takes the issue from. */
export interface WorkflowHandoffRun {
  task_id: string;
  agent_id: string;
  status: string;
  started_at: string | null;
}

/**
 * What moving an issue to a status would do under its project's workflow
 * (MUL-7420). `handoff` is false for the Default workflow and for steps that
 * keep the assignee.
 */
export interface WorkflowHandoffPreview {
  handoff: boolean;
  workflow_name: string;
  from_status: string;
  to_status: string;
  handler_type: "agent" | "squad" | "member" | null;
  handler_id: string | null;
  previous_assignee_type: string | null;
  previous_assignee_id: string | null;
  previous_runs: WorkflowHandoffRun[];
  /** The brief the handler's run receives; empty for a member handler. */
  brief: string;
}

/** A workflow's handoff for one column; handler "none" clears it. */
export interface SetWorkflowHandoffRequest {
  handler: IssueWorkflowHandler;
  instructions?: string;
  next_status_key?: string;
  back_status_key?: string;
}
