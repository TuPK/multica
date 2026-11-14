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

export interface IssueFlowSlotStatus {
  slot: string;
  min: number;
  max: number;
  count: number;
}

export interface IssueFlowLink {
  issue_id: string;
  identifier: string;
  task: string;
  slot: string;
}

/** The stop a request is parked in, as `GET /api/issues/{id}/flow` reports it. */
export interface IssueFlowStopInfo {
  /** The declared stop code, or "stalled" for a task that stopped moving. */
  code: string;
  /** The stop number schema 1 gave it (1 or 3); 0 for none. */
  legacy_no: number;
  from: string;
  edge: string;
  at: string;
}

export interface IssueFlowCheckpoint {
  step: string;
  activation: number;
  by?: string;
  at?: string;
}

export interface IssueFlowTrailEntry {
  step: string;
  activation: number;
  edge?: string;
  at?: string;
}

/** One task the engine opened for the request. */
export interface IssueFlowTask {
  issue_id: string;
  identifier: string;
  step: string;
  task: string;
  round: number;
  /** "gate" or "work" as the workflow defines the task; "" when this build does not know it. */
  kind: string;
  status: string;
  /** approved, rejected, blocked or done; null until reported. A value this build does not know is kept. */
  outcome: string | null;
  assignee_id: string | null;
  closed_at: string | null;
  /** Who closed the task; null while it is open. Absent from an older server. */
  closed_by?: IssueFlowClosedBy | null;
  /** The 1-based instance of a task that fans out, 0 for a plain task. */
  instance?: number;
  /** How the closed task counts in its join: pass, fail, block or neutral; empty while open. */
  class?: string;
  /** For each file the task requires, how many it has. */
  slot_status?: IssueFlowSlotStatus[];
  /** Rows a task reads from previous steps. */
  inputs?: IssueFlowLink[];
  /** Rows a task must address after a rejection. */
  feedback?: IssueFlowLink[];
  /** Variables the task may set while it is open. */
  sets?: string[];
  /**
   * The verdicts the task closes with: its own set, or the built-in one of its kind. Empty from an
   * older server, and then a gate offers approved and rejected.
   */
  verdicts?: IssueFlowVerdict[];
}

/** One verdict of a task and how it counts in the join of its step. */
export interface IssueFlowVerdict {
  key: string;
  /** pass, fail or neutral; a class this build does not know is kept as text. */
  class: string;
}

/** Who closed a task. `on_behalf` is true when a member who manages the squad closed it for its assignee. */
export interface IssueFlowClosedBy {
  type: string;
  id: string;
  name?: string;
  on_behalf: boolean;
}

/** What the reader of the state may do. Absent from an older server, which reads as nothing. */
export interface IssueFlowViewer {
  /** The reader manages the squad that runs the request. */
  can_manage: boolean;
  /** The open tasks the reader may close: their own, or any when they manage the squad. */
  closable_task_ids: string[];
}

/** A way out of the current step, for the admin's view of where the request can go. */
export interface IssueFlowNext {
  id: string;
  label: string;
  to: string;
  rework: boolean;
  trigger?: string;
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
  /**
   * The stop the request is parked in (spec 7.1): its declared code and the number schema 1 gave
   * it (`legacy_no` 1 or 3, 0 for none). Null while the request is not parked.
   */
  stop: IssueFlowStopInfo | null;
  switches: Record<string, IssueFlowSwitchValue>;
  /** The background runner advances this request. */
  auto: boolean;
  entered_at: string;
  tasks: IssueFlowTask[];
  next: IssueFlowNext[];
  /**
   * The drafting step a stopped request can return to (resume with to_step). Empty when the
   * request is not stopped or its stop has no such step. Absent from an older server.
   */
  rework_step?: string;
  /** Filled for the reader of GET /api/issues/{id}/flow; absent from an older server. */
  viewer?: IssueFlowViewer;
  /** Schema version of the flow (1 or 2). */
  schema?: number;
  /** Loop counters for the request. */
  loops?: Record<string, number>;
  /** Number of times each step has been entered. */
  activations?: Record<string, number>;
  /** Flow variables. */
  vars?: Record<string, unknown>;
  /** Evaluated facts for decision routing. */
  facts?: Record<string, unknown>;
  /** Human checkpoints recorded along the flow. */
  checkpoints?: IssueFlowCheckpoint[];
  /** True when agent work has not yet been reviewed by a human. */
  unreviewed?: boolean;
  /** Trace of steps and edges taken by this request. */
  trail?: IssueFlowTrailEntry[];
  /** True for trial runs of a workflow. */
  trial?: boolean;
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

/**
 * POST /api/issues/{id}/flow/start. Without a body the request starts on the squad's active
 * workflow.
 */
export interface StartFlowRequest {
  /** Another version of the squad's workflows (any lineage); omitted: the active one. */
  workflow_id?: string;
  /** Start the request as a rehearsal of that version (members who manage the squad). */
  trial?: boolean;
}

/** POST /api/issues/{id}/flow/resume (squad admin only). */
export interface ResumeFlowRequest {
  /** Where to resume; omitted: the step the request was blocked from. */
  to_step?: string;
  reset_rework?: boolean;
  /** Loops whose count starts over on a request of schema 2; "all" names every loop. */
  reset_loops?: string[];
  /** Let the step go on without the work tasks a person cancelled, which are otherwise reopened. */
  skip_cancelled?: boolean;
  /**
   * Resume a request the engine stalled because no person has seen the work of an agent, taking the
   * place of that check. Without it the server refuses with `flow_checkpoint_ack_required`.
   */
  acknowledge_skip_checkpoint?: boolean;
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
