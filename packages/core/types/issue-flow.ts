/**
 * The enforced squad workflow engine, as the web client sees it
 * (docs/engineering/kitviet-workflow-spec.md, section 7).
 *
 * A request given to a squad whose active workflow is a flow runs on it: each
 * step opens one sub-issue per task, and the server routes the request on the
 * verdicts. Only the server writes this state.
 */

/**
 * Why a request waits for a person: 1 is missing grounds, 3 is the end of the
 * rework rounds, "stalled" is a task that stopped moving. A stop this build
 * does not know reads as "stalled".
 */
export type IssueFlowStop = 1 | 3 | "stalled";

/** The flow fields an issue response carries (`Issue.flow`). */
export interface IssueFlowRef {
  /** "request" is the issue given to the squad; "task" is a sub-issue the engine opened under it. */
  role: "request" | "task";
  step: string;
  /** The task key; set on a task. */
  task?: string;
  /** The rework round the task was opened in; set on a task. */
  round?: number;
  stop?: IssueFlowStop | null;
  workflow_id?: string;
  rework?: number;
}

/** A switch value: a bool switch is on or off, an enum switch holds one of its values. */
export type IssueFlowSwitchValue = boolean | string;

/** One task the engine opened for the request. */
export interface IssueFlowTask {
  issue_id: string;
  identifier: string;
  step: string;
  task: string;
  round: number;
  status: string;
  /** approved, rejected, blocked or done; null until reported. A value this build does not know is kept. */
  outcome: string | null;
  assignee_id: string | null;
  closed_at: string | null;
}

/** A way out of the current step, for the admin's view of where the request can go. */
export interface IssueFlowNext {
  id: string;
  label: string;
  to: string;
  rework: boolean;
}

/** GET /api/issues/{id}/flow */
export interface IssueFlowState {
  managed: boolean;
  /** "task" when the issue asked about is a task: the state is its request's. */
  role: "request" | "task";
  workflow_id: string;
  workflow_name: string;
  step: string;
  /** Rework rounds used so far. */
  rework: number;
  stop: IssueFlowStop | null;
  switches: Record<string, IssueFlowSwitchValue>;
  /** The background runner advances this request. */
  auto: boolean;
  entered_at: string;
  tasks: IssueFlowTask[];
  next: IssueFlowNext[];
}

export type IssueFlowActionKind = "opened_tasks" | "routed" | "stopped" | "noop";

export interface IssueFlowAction {
  kind: IssueFlowActionKind | (string & {});
  step: string;
  tasks: string[];
}

/** A task the request still waits for. */
export interface IssueFlowPending {
  task: string;
  issue_id: string;
  status: string;
}

/** POST /api/issues/{id}/flow/advance */
export interface IssueFlowAdvanceResponse {
  state: IssueFlowState;
  actions: IssueFlowAction[];
  pending: IssueFlowPending[];
}

/** POST /api/issues/{id}/flow/resume (squad admin only). */
export interface ResumeFlowRequest {
  /** Where to resume; omitted: the step the request was blocked from. */
  to_step?: string;
  reset_rework?: boolean;
}

/** POST /api/issues/{id}/flow/detach (squad admin only). */
export interface DetachFlowRequest {
  /** Also cancel the open tasks; omitted or false: they stay as ordinary sub-issues. */
  cancel_tasks?: boolean;
}

export interface DetachFlowResponse {
  detached: boolean;
}

/** PUT /api/issue-workflows/{id}/flow-auto (workspace admin or owner). */
export interface SetFlowAutoRequest {
  auto: boolean;
}
