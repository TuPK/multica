/** The shape of the data `pnpm catalog:build` writes for squads (catalog/squads/<id>/template.json). */

export interface CatalogRole {
  key: string;
  /** The agent instruction template the role starts from. Absent for a Kit Viet role: the pack gives it. */
  agentTemplate?: string;
  /** Name of the created agent. Absent for a Kit Viet role: the pack gives it. */
  name?: string;
  description: string;
  /** The roster line the leader reads. */
  role: string;
}

export interface CatalogSquadBody {
  name: string;
  description: string;
  floorName: string | null;
  leader: string;
  operatingReview?: boolean;
  /** A workflow template the squad's build project uses, by id. */
  workflow: string | null;
  roles: CatalogRole[];
  routines: unknown[];
  charter: unknown;
  /** The skillsets (and extra skills) a non-Kit-Viet squad imports. A Kit Viet squad takes its skills from the pack. */
  skills?: { sets: string[]; extra?: string[]; leader?: string[]; byRole?: Record<string, string[]> };
  /** The leader's instructions (catalog/squads/<id>/instructions.md). A Kit Viet squad composes them from the pack. */
  instructions?: string;
}

export interface CatalogSquad {
  id: string;
  version: string;
  status: "stable" | "beta" | "deprecated" | "hidden";
  collection: string;
  expertReview?: boolean;
  adapter?: { type: "kitviet" };
  squad: CatalogSquadBody;
}

export interface CatalogWorkflowStep {
  /** An existing status key, or a status to create under this name. */
  status: { key: string } | { name: string; category?: "started" | "done" };
  /** The key of the status (the key of a created status is its name in lower case with underscores). */
  statusKey: string;
  /** The squad role that takes the issue when it enters the step; null keeps the assignee. */
  role: string | null;
  /** The Go build skips the step when the squad has no agent for the role. */
  optionalRole?: boolean;
  instructions: string;
  next?: string;
  back?: string;
  /** The Go build leaves this step's forward target to the board order (spec 5.4). */
  implicitNext?: boolean;
}

export interface CatalogWorkflow {
  name: string;
  description: string;
  steps: CatalogWorkflowStep[];
}

/** A condition on a flow transition or task, in the shape issueworkflow reads (field, op, value or all/any/not). */
export interface CatalogFlowCondition {
  all?: CatalogFlowCondition[];
  any?: CatalogFlowCondition[];
  not?: CatalogFlowCondition;
  field?: string;
  op?: string;
  value?: unknown;
}

export interface CatalogFlowTask {
  key: string;
  title: string;
  kind: "work" | "gate";
  /** A role key of the squad; the quickstart swaps it for the agent that has the role. */
  role: string;
  instructions: string;
  when?: CatalogFlowCondition;
  requires?: Array<"outcome" | "attachment">;
}

export interface CatalogFlowTransition {
  id: string;
  label: string;
  /** null is the default branch. */
  when: CatalogFlowCondition | null;
  /** The status key the request moves to. */
  to: string;
  rework?: boolean;
}

export interface CatalogFlowStep {
  status: { key: string } | { name: string; category?: "started" | "done" };
  statusKey: string;
  humanOnly?: boolean;
  tasks?: CatalogFlowTask[];
  transitions?: CatalogFlowTransition[];
}

export interface CatalogFlowConfig {
  version: number;
  task_timeout_minutes?: number;
  task_hard_cap_minutes?: number;
  task_max_attempts?: number;
  switches?: Array<{ key: string; type: "bool" | "enum"; values?: string[]; default: boolean | string }>;
  messages?: { stop1?: string; stop2?: string; stop3?: string; stalled?: string; human?: string };
}

/** An enforced flow (catalog/workflows/flow_<squad id>); `rev` names the version a workspace runs. */
export interface CatalogFlow {
  id: string;
  version: string;
  /** First 8 hex digits of the sha256 of the canonical flow block; the template_ref of its workflow row is `<id>@<rev>`. */
  rev: string;
  /** The id of the squad template the flow belongs to. */
  squad: string;
  config: CatalogFlowConfig;
  steps: CatalogFlowStep[];
}
