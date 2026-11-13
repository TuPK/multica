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
  /** Canvas position in the designer, in finite numbers within ±100000. Absent: the designer auto-places the step. */
  layout?: { x: number; y: number };
  /**
   * The sub-issues a request on a flow opens when it enters the step. Always
   * present on a parsed response (empty for an ordinary workflow); optional here
   * so a step built by hand needs none.
   */
  tasks?: IssueWorkflowTask[];
  /** Only a member may move the request out of this step. Always present on a parsed response. */
  human_only?: boolean;
}

export type IssueWorkflowTaskKind = "work" | "gate";

/** What a task must have before it closes. A gate must report an outcome. */
export type IssueWorkflowTaskRequirement = "outcome" | "attachment";

/**
 * One unit of work the engine opens as a sub-issue (spec section 3.3). The agent
 * is `handler.id`. `when` absent: always opened; else opened when the condition
 * holds, usually a `switch.<key>` check.
 */
export interface IssueWorkflowTask {
  key: string;
  title: string;
  kind: IssueWorkflowTaskKind;
  handler: IssueWorkflowHandler;
  instructions: string;
  when?: WorkflowCondition | null;
  requires: (IssueWorkflowTaskRequirement | (string & {}))[];
}

/** A choice the lead makes at the plan step. */
export interface IssueWorkflowSwitch {
  key: string;
  type: "bool" | "enum" | (string & {});
  values?: string[];
  default: boolean | string | null;
}

/** The workflow-level configuration of a flow (spec section 3.4). */
export interface IssueWorkflowFlowConfig {
  version: number;
  task_timeout_minutes?: number;
  task_hard_cap_minutes?: number;
  task_max_attempts?: number;
  switches: IssueWorkflowSwitch[];
  /** Templates of the stop comments; the first line is the title. */
  messages?: { stop1?: string; stop2?: string; stop3?: string; stalled?: string; human?: string };
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
  | "assignee_type"
  | "tasks_outcome"
  | `switch.${string}`;

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
  /** A flow branch that sends the request back for another round (adds one to its rework count). */
  rework?: boolean;
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
  /** Set on a workflow that runs requests on a flow; always present on a parsed response, null for an ordinary workflow. */
  flow_config?: IssueWorkflowFlowConfig | null;
  /** `flow_<squad>@<rev>` for a workflow made from a catalog flow: it is read-only. Null otherwise. */
  template_ref?: string | null;
  /** The background runner may advance requests on this workflow. */
  flow_auto?: boolean;
  /** The squad whose library holds the workflow; null for a workspace-level workflow. */
  squad_id?: string | null;
  /** Requests pinned to this workflow that are not closed. */
  open_flow_requests?: number;
  /** Lineage of the published versions. Null for legacy or catalog workflows. */
  lineage_id?: string | null;
  /** Sequential version number within the lineage. */
  version?: number | null;
  /** When this version was published; non-null means locked against edits. */
  published_at?: string | null;
  /** Who published this version. */
  published_by?: string | null;
  /** The compiled schema 2 flow document when version is 2. */
  flow_doc?: Record<string, unknown> | null;
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
    /** Canvas position in the designer. Omitted: the designer places the step. */
    layout?: { x: number; y: number };
    /** Omitted: the step keeps its stored tasks; an empty list clears them. */
    tasks?: IssueWorkflowTask[];
    /** Omitted: the step keeps its stored flag. */
    human_only?: boolean;
  };

export interface IssueWorkflowWriteRequest {
  name?: string;
  description?: string;
  /** Creates a flow workflow. Workspace admins only; an update never changes it. */
  flow_config?: IssueWorkflowFlowConfig;
  /** Marks the workflow as made from a catalog flow (needs `flow_config`). */
  template_ref?: string;
  /**
   * The squad the new workflow is written for. With a `flow_config`, the server checks the flow
   * against the squad's roster (who the squad has, the lead takes no task) and keeps the row as
   * the squad's. Without it the roster is not checked.
   */
  squad_id?: string;
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

/** A system template's id. Server-owned: add ids only through the server. */
export type IssueWorkflowTemplateId =
  | "delivery"
  | "content"
  | "community"
  | "sdlc_auto"
  | "bmad_planning_auto"
  | "sales_loop"
  | "quick_fix";

/**
 * One column of a template. A column names an existing status by `status_key`,
 * or a status the save creates by `new_status`; the created status takes the key
 * the server derives from its name, and the template's transitions name that key.
 * The handler is always none: a template never names an agent. `role_hint` is a
 * squad role id shown as a suggestion, never stored.
 */
export type IssueWorkflowTemplateStep = ({ status_key: string } | { new_status: NewWorkflowStatus }) & {
  handler: IssueWorkflowHandler;
  role_hint?: string;
  instructions: string;
  transitions?: IssueWorkflowTransition[];
  /** Suggested canvas position. Templates omit it, so the designer auto-places their columns. */
  layout?: { x: number; y: number };
};

/**
 * A system starting point for a new workflow (GET /api/issue-workflows/templates).
 * The client translates the name and description by `id`. Always lists todo, done
 * and cancelled, directly or through a created status.
 */
export interface IssueWorkflowTemplate {
  id: IssueWorkflowTemplateId;
  steps: IssueWorkflowTemplateStep[];
}

export interface ListIssueWorkflowTemplatesResponse {
  templates: IssueWorkflowTemplate[];
}

