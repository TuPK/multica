import type { WorkflowCondition } from "./issue-workflow";

/**
 * Flow Draft and general flow engine schema 2 types
 * (docs/engineering/flow-engine-generalization-spec.md).
 */

export interface FlowDraft {
  id: string;
  squad_id: string;
  name: string;
  lineage_id: string | null;
  base_workflow_id: string | null;
  revision: number;
  doc: FlowDoc;
  origin: "manual" | "fork" | "import" | "proposal" | string;
  source_ref: unknown;
  created_by: string;
  updated_by: string;
  created_at: string;
  updated_at: string;
}

export interface FlowDraftSummary {
  id: string;
  squad_id: string;
  name: string;
  lineage_id: string | null;
  base_workflow_id: string | null;
  revision: number;
  origin: "manual" | "fork" | "import" | "proposal" | string;
  created_by: string;
  updated_by: string;
  created_at: string;
  updated_at: string;
  summary: {
    steps: number;
    tasks: number;
    gates: number;
  };
}

export interface ListFlowDraftsResponse {
  drafts: FlowDraftSummary[];
  total: number;
}

export interface FlowDoc {
  version: number;
  start: string;
  roles?: FlowDocRole[];
  vars?: FlowDocVar[];
  verdicts?: FlowDocVerdictSet[];
  loops?: FlowDocLoop[];
  stops?: FlowDocStopKind[];
  terminals: FlowDocTerminal[];
  steps: FlowDocStep[];
  messages?: Record<string, string>;
  policy?: FlowDocPolicy;
}

export type FlowRoleKind = "agent" | "member" | "lead" | "requester" | "managers" | (string & {});

export interface FlowDocRole {
  key: string;
  label?: string;
  kind: FlowRoleKind;
  pool?: number;
  bind?: {
    agent_ids?: string[];
    member_ids?: string[];
  };
  source?: {
    bundle?: string;
    agent_key?: string;
  };
}

export type FlowVarType = "bool" | "enum" | "int" | "string" | "list_string" | (string & {});

export interface FlowDocVar {
  key: string;
  type: FlowVarType;
  values?: string[];
  min?: number;
  max?: number;
  max_len?: number;
  max_items?: number;
  default: unknown;
  set_by?: string[];
  set_at?: string[];
}

export type FlowVerdictClass = "pass" | "fail" | "neutral" | "block" | (string & {});

export interface FlowDocVerdict {
  key: string;
  class: FlowVerdictClass;
  label?: string;
}

export interface FlowDocVerdictSet {
  key: string;
  verdicts: FlowDocVerdict[];
}

export type FlowLoopReset = "on_entry" | "never" | (string & {});

export type FlowLoopMax = number | { var: string; cap: number };

export interface FlowDocLoop {
  key: string;
  header: string;
  max: FlowLoopMax;
  reset?: FlowLoopReset;
}

export type FlowStopClass = "missing_grounds" | "loop_exhausted" | "custom" | (string & {});

export interface FlowDocStopKind {
  code: string;
  class: FlowStopClass;
  label?: string;
  message?: string;
  status?: string;
  resume_by?: string;
  legacy_no?: number;
}

export interface FlowDocTerminal {
  key: string;
  status: string;
  leaves_workspace?: boolean;
}

export type FlowStepKind = "tasks" | "lead" | "manual" | (string & {});

export interface FlowDocStepStatus {
  key?: string;
  name?: string;
  category?: string;
}

export interface FlowDocStepLead {
  sets?: string[];
  verdicts?: string;
  requires?: FlowDocRequires;
}

export interface FlowDocStepLayout {
  x: number;
  y: number;
}

export interface FlowDocStep {
  key: string;
  title: string;
  status?: FlowDocStepStatus;
  kind: FlowStepKind;
  tasks?: FlowDocTask[];
  join?: FlowDocJoin;
  lead?: FlowDocStepLead;
  exit_role?: string;
  notice?: string;
  edges: FlowDocEdge[];
  layout?: FlowDocStepLayout;
}

export type FlowTaskKind = "work" | "gate" | (string & {});

export interface FlowDocInstances {
  count?: number;
  from_var?: string;
  min?: number;
  max?: number;
  distinct?: boolean;
}

export interface FlowDocFileReq {
  slot: string;
  min: number;
  max?: number;
  ext?: string[];
  content_types?: string[];
}

export interface FlowDocRequires {
  files?: FlowDocFileReq[];
  verdict?: boolean | null;
  vars?: string[];
}

export interface FlowDocInput {
  from: string;
  slot?: string;
  required?: boolean;
}

export interface FlowDocTaskDeadline {
  idle_minutes?: number;
  hard_cap_minutes?: number;
  max_attempts?: number;
  remind_after_minutes?: number;
  stall_after_minutes?: number;
}

export interface FlowDocTask {
  key: string;
  title: string;
  kind: FlowTaskKind;
  role: string;
  instances?: FlowDocInstances;
  after?: string[];
  when?: WorkflowCondition | null;
  verdicts?: string;
  requires?: FlowDocRequires;
  inputs?: FlowDocInput[];
  sets?: string[];
  skills?: string[];
  deadline?: FlowDocTaskDeadline;
  instructions?: string;
}

export type FlowJoinMode = "all" | "any" | "quorum" | (string & {});
export type FlowJoinBlock = "immediate" | "at_close" | (string & {});

export interface FlowDocJoin {
  over: "gates" | "all" | string[];
  mode?: FlowJoinMode;
  k?: number;
  block?: FlowJoinBlock;
}

export type FlowEdgeTrigger = "auto" | "manual" | (string & {});

export interface FlowDocEdge {
  id: string;
  label?: string;
  to?: string;
  when?: WorkflowCondition | null;
  loop?: string;
  stop?: string;
  trigger?: FlowEdgeTrigger;
}

export interface FlowDocAck {
  kind: string;
  by?: string;
  at?: string;
}

export interface FlowDocPolicy {
  task_idle_minutes?: number;
  task_hard_cap_minutes?: number;
  task_max_attempts?: number;
  human_remind_minutes?: number;
  human_stall_minutes?: number;
  max_activations?: number;
  max_task_issues?: number;
  lead_may_work?: boolean;
  allow_unattended_release?: boolean;
  acks?: FlowDocAck[];
}

export type FlowIssueSeverity = "error" | "warning" | "info" | (string & {});

export interface FlowIssue {
  code: string;
  severity: FlowIssueSeverity;
  path?: string;
  message: string;
  params?: Record<string, unknown>;
  ack?: string;
}

export interface FlowStepBound {
  step: string;
  max_entries: number;
  max_tasks: number;
}

export interface FlowLoopInfo {
  key: string;
  header: string;
  max: number;
  body: string[];
}

export interface FlowStopInfo {
  code: string;
  class?: string;
  status: string;
  edges: string[];
}

export interface FlowReleasePath {
  from: string;
  terminal: string;
  path: string[];
}

export interface FlowAnalysis {
  worst_case_task_issues: number;
  worst_case_activations: number;
  steps: FlowStepBound[];
  loops: FlowLoopInfo[];
  stops: FlowStopInfo[];
  release_paths_without_human: FlowReleasePath[];
}

export interface FlowFactInfo {
  key: string;
  type: string;
  source: string;
  ops: string[];
  values?: string[];
}

export interface FlowValidationResponse {
  ok: boolean;
  revision: number;
  issues: FlowIssue[];
  analysis?: FlowAnalysis | null;
  fact_catalog: Record<string, FlowFactInfo[]>;
}

export interface FlowTraceHop {
  n: number;
  step: string;
  activation: number;
  opened?: string[];
  closed?: Record<string, string>;
  join?: string;
  edge?: string;
  to?: string;
  loops?: Record<string, number>;
}

export interface FlowTraceEnd {
  kind: "terminal" | "stop" | "stalled" | "waiting" | (string & {});
  code?: string;
  from?: string;
}

export interface FlowTraceResponse {
  hops: FlowTraceHop[];
  end: FlowTraceEnd;
  truncated: boolean;
}

export interface FlowManifestResponse {
  format: string;
  name: string;
  doc: FlowDoc;
}

export interface FlowLimits {
  [x: string]: unknown;
  doc_bytes: number;
  steps: number;
  status_names: number;
  tasks_per_step: number;
  edges_per_step: number;
  roles: number;
  vars: number;
  verdict_sets: number;
  loops: number;
  loop_max: number;
  instances: number;
  expr_depth: number;
  after_depth: number;
  message_runes: number;
  instructions_runes: number;
  inputs_per_task: number;
  sets_per_task: number;
  skills_per_task: number;
  default_max_task_issues: number;
  max_task_issues: number;
  default_max_activations: number;
  schema1_steps: number;
  schema1_tasks_per_step: number;
  schema1_rework_max: number;
  drafts_per_squad: number;
  versions_per_lineage: number;
  draft_name_runes: number;
  workflow_name_runes: number;
  task_title_runes: number;
  default_task_idle_minutes: number;
}

export interface FlowCapabilityFeatures {
  [x: string]: unknown;
  packs: boolean;
  proposals: boolean;
  system_performer: boolean;
  subflow: boolean;
  fork_join: boolean;
  authoring: boolean;
  graph_v2: boolean;
}

export interface FlowCapabilitiesResponse {
  [x: string]: unknown;
  schemas: number[];
  limits: FlowLimits;
  step_kinds: string[];
  task_kinds: string[];
  join_modes: string[];
  join_over: string[];
  join_block: string[];
  role_kinds: string[];
  verdict_classes: string[];
  var_types: string[];
  stop_classes: string[];
  edge_triggers: string[];
  loop_resets: string[];
  ops_by_type: Record<string, string[]>;
  builtin_facts: FlowFactInfo[];
  features: FlowCapabilityFeatures;
}

export interface CreateFlowDraftRequest {
  /** At most 56 characters. Optional for a copy of a workflow: the server takes its name. */
  name?: string;
  from: {
    kind: "blank" | "workflow" | "manifest" | "proposal" | (string & {});
    workflow_id?: string;
    keep_bind?: boolean;
    manifest?: unknown;
    agent_id?: string;
    references?: unknown[];
    note?: string;
  };
}

export interface UpdateFlowDraftRequest {
  revision: number;
  name?: string;
  doc?: unknown;
}

export interface SimulateFlowDraftRequest {
  doc?: unknown;
  vars?: Record<string, unknown>;
  script?: Record<string, Array<Record<string, string>>>;
  max_hops?: number;
}

export interface PublishFlowDraftRequest {
  revision: number;
  ack?: string[];
  make_active?: boolean;
}

export interface CompleteIssueFlowStepRequest {
  verdict: string;
  vars?: Record<string, unknown>;
  blocked?: {
    missing: string[];
    reason: string;
  };
}
