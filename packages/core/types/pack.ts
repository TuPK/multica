import type { FlowIssue, FlowValidationResponse } from "./flow-draft";

/**
 * The pack routes of the server (docs/engineering/flow-engine-generalization-spec.md, 16.1 to 16.3),
 * the proposal of a draft by an agent (16.4) and the operating numbers of a version (16.6). Field names
 * are the JSON tags of server/internal/handler/{squad_pack,flow_proposal,flow_insights}.go.
 */

/** Where a pack is read from. A zip the person uploads is sent as a file, not as a source. */
export interface PackSourceRequest {
  kind: "github" | "url_zip";
  url: string;
  ref?: string;
  subdir?: string;
  sha256?: string;
}

export interface PackPreviewAgent {
  key: string;
  name: string;
  description: string;
  instructions_bytes: number;
  skills: string[];
  /** False when the pack does not say which skills the agent has: the person chooses. */
  skills_declared: boolean;
  existing_agent_id: string | null;
}

export interface PackPreviewSkill {
  name: string;
  files: number;
  existing_skill_id: string | null;
  same_content: boolean;
}

export interface PackPreviewFlow {
  present: boolean;
  validation: FlowValidationResponse | null;
  issues: PackIssue[];
}

export interface PackPreviewReference {
  path: string;
  bytes: number;
  kind: string;
}

/** A finding of the security scan. `block` keeps the pack from being installed; `warn` needs a tick. */
export type PackScanSeverity = "info" | "warn" | "block";

export interface PackPreviewFinding {
  path: string;
  severity: PackScanSeverity;
  rule: string;
}

/** A setting of the pack that the install does not apply (tools, model, environment, MCP). */
export interface PackIgnored {
  agent?: string;
  skill?: string;
  key: string;
}

export interface PackIssue {
  code: string;
  severity: "error" | "warning" | "info";
  path?: string;
  message: string;
  params?: Record<string, unknown>;
}

export interface PackPreview {
  digest: string;
  format: string;
  id: string;
  name: string;
  version: string;
  agents: PackPreviewAgent[];
  skills: PackPreviewSkill[];
  /** The key of the lead the pack names; null when it names none. */
  leader: string | null;
  flow: PackPreviewFlow;
  references: PackPreviewReference[];
  /** How many doc files (README, docs/, workflows/) the list of references left out. 0 from a server that does not say. */
  references_omitted: number;
  ignored: PackIgnored[];
  scan: PackPreviewFinding[];
  issues: PackIssue[];
}

export type PackAgentAction = "create" | "use_existing" | "skip";

export interface PackAgentChoice {
  key: string;
  action: PackAgentAction;
  existing_agent_id?: string;
  runtime_id?: string;
  /** Overrides the name of an agent that is created. */
  name?: string;
  /** Overrides the skills of an agent that is created; absent means the ones the pack declares. */
  skills?: string[];
}

/** An agent the person adds to the squad that the pack does not have. */
export interface PackExtraAgent {
  key: string;
  name: string;
  instructions: string;
  runtime_id: string;
  skills: string[];
}

export interface PackLeaderChoice {
  agent?: string;
  generic?: boolean;
  runtime_id?: string;
}

export type PackSkillConflictChoice = "keep_existing" | "import_as_new";

export interface PackCommitRequest {
  digest: string;
  squad?: { name: string };
  agents?: PackAgentChoice[];
  extra_agents?: PackExtraAgent[];
  leader?: PackLeaderChoice;
  skills?: Record<string, PackSkillConflictChoice>;
  create_draft_from_flow?: boolean;
  ack_scan_warnings?: boolean;
  /** Installs a pack again, or another version of one that is installed, as a new squad. */
  new_install?: boolean;
}

export interface PackCommitAgent {
  key: string;
  agent_id: string;
  created: boolean;
}

export interface PackCommitSkill {
  name: string;
  skill_id: string;
  created: boolean;
}

export interface PackCommitResponse {
  install_id: string;
  /** True when the same pack was installed before and nothing was written. */
  already_installed: boolean;
  squad_id: string | null;
  agents: PackCommitAgent[];
  skills: PackCommitSkill[];
  draft_id: string | null;
  /** The errors that kept the flow of the pack from becoming a draft. */
  flow_issues: FlowIssue[];
}

/** The install a `pack_conflict` answer says is there already. */
export interface PackPreviousInstall {
  install_id: string;
  version: string;
  squad_id: string | null;
  installed_at: string;
  digest?: string;
}

/** What `proposal` of a draft says it was made from: the issue the agent was given, and the agent. */
export interface FlowProposalSource {
  issue_id: string;
  agent_id: string;
}

export interface FlowInsightsRequests {
  total: number;
  open: number;
  /** "terminal:<key>", "stop:<code>" or "stalled", with the number of requests that are there. */
  by_end: Record<string, number>;
}

export interface FlowInsightsStep {
  key: string;
  entered: number;
  /** Null while no visit of the step has a usable length. */
  median_minutes: number | null;
  p90_minutes: number | null;
}

export interface FlowInsightsEdge {
  step: string;
  edge: string;
  taken: number;
}

export interface FlowInsightsLoop {
  key: string;
  max: number;
  /** How many requests used 0, 1, 2, … rounds of the loop. */
  histogram: number[];
}

export interface FlowInsightsStall {
  reason: string;
  count: number;
}

export interface FlowInsights {
  requests: FlowInsightsRequests;
  steps: FlowInsightsStep[];
  edges: FlowInsightsEdge[];
  loops: FlowInsightsLoop[];
  stalls: FlowInsightsStall[];
  manual_closes: number;
  /** True when more requests matched than the report reads. */
  truncated: boolean;
}
