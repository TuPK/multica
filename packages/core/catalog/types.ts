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
