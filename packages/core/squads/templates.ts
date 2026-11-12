import type { AgentInstructionTemplate } from "../agents/instruction-templates";
import { skillsetMembers } from "../catalog/skillsets";
import type { SquadTemplateCategory } from "../catalog/generated/collections";
import { CATALOG_WORKFLOWS } from "../catalog/generated/workflows";
import type { SquadRoleKey, SquadTemplateId } from "../catalog/generated/squads";
import { SQUAD_TEMPLATE_CATEGORIES as CATALOG_CATEGORIES, assembleSquadRoles, assembleSquadTemplates } from "../catalog/squads";

/**
 * Ready-made squads a user can start from in the Squads quickstart. A template
 * names the roles it needs, the instructions its leader follows, and the
 * workflow its build project uses. Card names and descriptions are translated
 * in the views package, keyed by `id`; the agents it creates keep the English
 * names below so that running the quickstart twice reuses them.
 */

// The ids and role keys are generated from catalog/squads (pnpm catalog:build), so a new squad needs no edit here.
export type { SquadRoleKey, SquadTemplateId };

export interface SquadRole {
  key: SquadRoleKey;
  /** The agent instruction template the created agent starts from. */
  agentTemplate: AgentInstructionTemplate["id"];
  /** Name of the created agent. A non-archived agent with this name is reused. */
  name: string;
  description: string;
  /** Roster text the leader reads; context only, it schedules nothing. */
  role: string;
}

/**
 * Every role the quickstart can create, by key, assembled from the roles in catalog/squads. A Kit Viet
 * role's agent text is loaded on demand.
 */
export const SQUAD_ROLES: Record<SquadRoleKey, SquadRole> = assembleSquadRoles();

export interface SquadWorkflowStep {
  /**
   * An existing status key, or a status to create under this name. The key of a
   * created status is its name in lower case with underscores, so `next` and
   * `back` can name it before it exists.
   */
  status: { key: string } | { name: string; category?: "started" | "done" };
  /** The role that takes the issue when it enters the step; null keeps the assignee. */
  handler: SquadRoleKey | null;
  instructions: string;
  /** The status key the handler moves the issue to when its step is done. */
  next?: string;
  back?: string;
}

export interface SquadWorkflowTemplate {
  name: string;
  description: string;
  steps: SquadWorkflowStep[];
}

const workflowCache = new Map<string, SquadWorkflowTemplate>();

/** One object per workflow id, so every squad naming a workflow shares it. */
function workflowTemplate(id: string): SquadWorkflowTemplate {
  const cached = workflowCache.get(id);
  if (cached) return cached;
  const workflow = CATALOG_WORKFLOWS[id];
  if (!workflow) throw new Error(`no workflow ${id} in the catalog; run pnpm catalog:build`);
  const built: SquadWorkflowTemplate = {
    name: workflow.name,
    description: workflow.description,
    steps: workflow.steps.map((step) => ({
      status: step.status,
      handler: step.role as SquadRoleKey | null,
      instructions: step.instructions,
      ...(step.next ? { next: step.next } : {}),
      ...(step.back ? { back: step.back } : {}),
    })),
  };
  workflowCache.set(id, built);
  return built;
}

/**
 * The workflows a squad's build project can use, by id (catalog/workflows). SDLC Auto is also the
 * workflow the server builds for a project given to an SDLC squad (initiative_build.go), from the same
 * manifest, so the two cannot drift.
 */
export const SQUAD_WORKFLOWS: Readonly<Record<string, SquadWorkflowTemplate>> = Object.fromEntries(
  Object.keys(CATALOG_WORKFLOWS).map((id) => [id, workflowTemplate(id)]),
);
export const SDLC_AUTO_WORKFLOW: SquadWorkflowTemplate = workflowTemplate("sdlc_auto");
export const PLANNING_AUTO_WORKFLOW: SquadWorkflowTemplate = workflowTemplate("planning_auto");

/** A recurring autopilot the template creates for its squad. */
export interface SquadRoutineTemplate {
  /** Name of the autopilot. A non-archived autopilot with this name on the squad is reused. */
  title: string;
  /** Title of each issue the routine opens. Supports {{date}}. */
  issueTitleTemplate: string;
  /** Five-field cron expression, in the timezone the quickstart passes. */
  cron: string;
  /** The issue body each run starts from: the scan scope and the bar for a proposal. */
  description: string;
  /**
   * Create the autopilot paused. A rerun re-pauses only the quickstart's own autopilot that a
   * failed pause left active and unscheduled (ensureRoutine); any other existing autopilot keeps its
   * status, so a rerun never resumes a routine the boss paused or enabled.
   */
  startPaused?: boolean;
}

/** The charter a template saves for its squad on creation. */
export interface SquadCharterTemplate {
  mission: string;
  responsibilities: { key: string; text: string }[];
  outputs: { key: string; kind: "issue" | "proposal" | "kb_document" | "report"; cadenceHint: string }[];
  boundaries: string;
  escalation: string;
  intake: string;
}

/**
 * The group a template is listed under in the Squads quickstart: a collection id (catalog/collections).
 * The base squads are "core", the Kit Viet squads for general office work "kitviet", the Kit Viet squads
 * for a trade or sector "vertical".
 */
export type { SquadTemplateCategory };

/** The quickstart's groups, in the order they are listed. */
export const SQUAD_TEMPLATE_CATEGORIES: readonly SquadTemplateCategory[] = CATALOG_CATEGORIES;

/** The group of a template. A template with no category is listed with the verticals. */
export function squadTemplateCategory(template: SquadTemplate): SquadTemplateCategory {
  return template.category ?? "vertical";
}

export interface SquadTemplate {
  id: SquadTemplateId;
  /** Name of the created squad. */
  name: string;
  description: string;
  /** The quickstart group. Missing means "vertical"; see squadTemplateCategory. */
  category?: SquadTemplateCategory;
  /**
   * True when a qualified person (a clinician, a licensed agent, a credit officer, a safety
   * engineer) must read the squad's output before it is used. The quickstart asks the user to
   * acknowledge that before it creates the squad. Missing means false.
   */
  expertReview?: boolean;
  /** Roles in the squad; the leader leads and the others are members. */
  roles: SquadRoleKey[];
  /** The role that leads. It is created first and is the squad's leader. */
  leader: SquadRoleKey;
  /**
   * Floor name passed as the squad's floor_name. The server uses it only when
   * it has to make a new floor, so the squad may land on an existing floor.
   */
  floorName: string | null;
  /** The recurring autopilots the template creates for its squad. Empty when it has none. */
  routines: readonly SquadRoutineTemplate[];
  /** The charter saved on creation. Saving it also creates the squad's operating review. */
  charter: SquadCharterTemplate;
  /**
   * Whether the first charter save turns on the squad's weekly operating review. Omitted: the server
   * default (on). A template with no routine sets false, so no review routine is created behind its back.
   */
  operatingReview?: boolean;
  /** Squad instructions: what the leader follows. */
  instructions: string;
  /** The workflow its build project needs, when it has one. */
  workflow: SquadWorkflowTemplate | null;
  /**
   * Built-in skill names the quickstart imports into the workspace and attaches
   * to the squad, so every member of the squad gets them.
   */
  builtinSkills: readonly string[];
  /**
   * Built-in skill names the leader agent gets on top of the squad's skills. The
   * quickstart attaches them to the leader alone with addAgentSkills.
   */
  leaderSkills?: readonly string[];
  /**
   * Built-in skill names each role's agent gets on top of the squad's skills, attached
   * to that agent alone with addAgentSkills. Roles without an entry get none.
   */
  roleSkills?: Partial<Record<SquadRoleKey, readonly string[]>>;
}

/** The built-in skills the planning squad imports: the catalog skills whose `sets` include `planning`. */
export const PLANNING_SKILL_NAMES: readonly string[] = skillsetMembers("planning");

/** The built-in skills of the sales squad: the catalog skills whose `sets` include `sales`. */
export const SALES_SKILL_NAMES: readonly string[] = skillsetMembers("sales");

/**
 * The built-in SDLC skills, by name. Every SDLC squad imports all of them. The planning, sales and
 * Research Lab skills are in their own sets and are not in this one.
 */
export const SDLC_SKILL_NAMES: readonly string[] = skillsetMembers("sdlc");

/**
 * The templates the Squads quickstart offers, in the order it lists them within each group: assembled
 * from catalog/squads/<id>/template.json (and, for a Kit Viet squad, from its generated pack).
 */
export const SQUAD_TEMPLATES: readonly SquadTemplate[] = assembleSquadTemplates(SQUAD_WORKFLOWS);
