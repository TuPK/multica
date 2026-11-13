import type { IssueWorkflow, IssueWorkflowMappingPlan } from "./issue-workflow";

export type SquadMemberType = "agent" | "member";

export type SquadActivityOutcome = "action" | "no_action" | "failed";

export interface SquadMemberPreview {
  member_type: SquadMemberType;
  member_id: string;
  role: string;
}

export interface Squad {
  /** The template the squad was made from, or null for a squad made by hand (spec 7.5). */
  template_key?: string | null;
  id: string;
  workspace_id: string;
  name: string;
  description: string;
  instructions: string;
  avatar_url: string | null;
  leader_id: string;
  creator_id: string;
  created_at: string;
  updated_at: string;
  archived_at: string | null;
  archived_by: string | null;
  member_count?: number;
  member_preview?: SquadMemberPreview[];
  /** The company floor this squad is seated on. Null while unseated; older backends omit it. */
  floor_id?: string | null;
  /** The room slot (0 to 5) on that floor. Set exactly when `floor_id` is. */
  company_slot?: number | null;
  /** The squad's autonomy level, 0 (Suggest) to 3 (Autonomous). Older backends omit it: the default is 1. */
  autonomy_level?: number;
  /** The squad's active workflow. Always present on a parsed response; null when it has none. */
  active_workflow_id?: string | null;
  active_workflow_name?: string | null;
  /** The size of the squad's workflow library. Always present on a parsed response; 0 when empty. */
  workflow_count?: number;
  /** The active workflow runs requests on a flow the server enforces. Always present on a parsed response. */
  flow_enforced?: boolean;
}

/** GET /api/squads/{id}/workflows: the squad's library and which workflow is active. */
export interface SquadWorkflowsResponse {
  active_workflow_id: string | null;
  workflows: IssueWorkflow[];
  total: number;
}

/**
 * POST /api/squads/{id}/workflow. A dry run answers `{ dry_run: true, plan }`; a
 * real switch answers the ids and how many issues moved.
 */
export interface SetSquadActiveWorkflowRequest {
  workflow_id: string;
  status_mapping?: Record<string, string>;
  dry_run?: boolean;
}

export interface SetSquadActiveWorkflowResponse {
  squad_id: string;
  active_workflow_id: string;
  updated_issues: number;
  plan: IssueWorkflowMappingPlan;
}

export interface SquadMember {
  id: string;
  squad_id: string;
  member_type: SquadMemberType;
  member_id: string;
  role: string;
  created_at: string;
}

/** A workspace skill applied to a squad: every member runs with it on every task. */
export interface SquadSkill {
  id: string;
  name: string;
  description: string;
}

/**
 * A plugin the squad leader must have on the runtime of each project the squad
 * works on. `install_hint` is an optional source or command for installing it.
 */
export interface SquadPlugin {
  name: string;
  install_hint: string;
}

export interface SquadActivityLog {
  id: string;
  squad_id: string;
  issue_id: string;
  trigger_comment_id: string | null;
  leader_id: string;
  outcome: SquadActivityOutcome;
  details: unknown;
  created_at: string;
}

export interface CreateSquadRequest {
  name: string;
  description?: string;
  leader_id: string;
  avatar_url?: string;
  /**
   * Names the floor this squad creates when every existing floor is full. A
   * template uses it to name its floor; ignored when a free seat exists.
   */
  floor_name?: string;
  /** The template the squad is made from (spec 7.5). Empty for a squad made by hand. */
  template_key?: string;
}

export interface UpdateSquadRequest {
  name?: string;
  description?: string;
  instructions?: string;
  leader_id?: string;
  avatar_url?: string;
}

export interface AddSquadMemberRequest {
  member_type: SquadMemberType;
  member_id: string;
  role?: string;
}

export interface RemoveSquadMemberRequest {
  member_type: SquadMemberType;
  member_id: string;
}

export interface UpdateSquadMemberRoleRequest {
  member_type: SquadMemberType;
  member_id: string;
  role: string;
}

export interface CreateSquadActivityLogRequest {
  squad_id: string;
  issue_id: string;
  trigger_comment_id?: string;
  outcome: SquadActivityOutcome;
  details?: unknown;
}

// SquadMemberStatus mirrors the five-way bucket the back-end derives in
// handler/squad.go::deriveSquadMemberStatus. Kept as a string union here
// (rather than re-derived from snapshot data) so the squad page can render
// the freshest server-side judgement without re-fetching the agent
// snapshot / runtime list. `archived` wins over every runtime/task signal.
export type SquadMemberStatusValue =
  | "working"
  | "idle"
  | "offline"
  | "unstable"
  | "archived";

export interface SquadActiveIssueBrief {
  issue_id: string;
  identifier: string;
  title: string;
  issue_status: string;
}

export interface SquadMemberStatus {
  member_type: SquadMemberType;
  member_id: string;
  // Human members are returned with status === null so the UI can render
  // them in the same list without showing a status pill (v1 has no
  // presence signal for humans).
  status: SquadMemberStatusValue | null;
  active_issues: SquadActiveIssueBrief[];
  last_active_at: string | null;
}

export interface SquadMemberStatusListResponse {
  members: SquadMemberStatus[];
}

export interface SetSquadSkillsRequest {
  skill_ids: string[];
}

export interface SetSquadPluginsRequest {
  plugins: SquadPlugin[];
}

/** Result of a squad-wide runtime change. Each agent lands in exactly one list. */
export interface SquadRuntimeAgentResult {
  agent_id: string;
  agent_name: string;
  reason: string;
}

export interface SetSquadRuntimeResponse {
  runtime_id: string;
  updated: SquadRuntimeAgentResult[];
  unchanged: SquadRuntimeAgentResult[];
  skipped: SquadRuntimeAgentResult[];
}
