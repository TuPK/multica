import { z } from "zod";
import {
  AGENT_INSTRUCTION_TEMPLATES,
  type AgentInstructionTemplate,
} from "../agents/instruction-templates";
import {
  COMPANY_FLOOR_KEYS,
  COMPANY_FLOOR_NAMES as FLOOR_NAMES,
  COMPANY_SQUAD_CARDS_DATA,
  COMPANY_SQUAD_KEYS,
  COMPANY_STAGE_KEYS,
  COMPANY_STAGE_OWNERS_DATA,
  COMPANY_START_OPTIONS,
} from "../catalog/generated/company";

/**
 * The starter company (spec 3.4): one squad card per squad, the pipeline's
 * stage owners, and the arithmetic the wizard shows before anything is
 * created. Cards are data. The apply service on the server matches squads and
 * agents by these keys and names, so changing a card changes what a rerun does.
 */

// The floors, squad keys, stages, stage owners, start options and squad cards are data in
// catalog/companies/starter, generated into ../catalog/generated/company.ts by `pnpm catalog:build`.
export { COMPANY_FLOOR_KEYS, COMPANY_SQUAD_KEYS, COMPANY_STAGE_KEYS, COMPANY_START_OPTIONS };
export type CompanyFloorKey = (typeof COMPANY_FLOOR_KEYS)[number];
export type CompanySquadKey = (typeof COMPANY_SQUAD_KEYS)[number];
export type CompanyStageKey = (typeof COMPANY_STAGE_KEYS)[number];

/**
 * The floor names the apply service creates when a workspace lacks them. They
 * stay English, like the squad and agent names, so a rerun finds them.
 */
export const COMPANY_FLOOR_NAMES: Record<CompanyFloorKey, string> = FLOOR_NAMES;

export interface CompanyStageOwner {
  key: CompanyStageKey;
  name: string;
  /**
   * The squad the stage's `owner` names in the pipeline. Stages a second squad
   * co-owns list it in that squad's card, not here. Learn is owned by whichever
   * squad owned Build; the starter company gives Build to the Build squad.
   */
  owner: CompanySquadKey;
}

export const COMPANY_STAGE_OWNERS: readonly CompanyStageOwner[] = COMPANY_STAGE_OWNERS_DATA;

/**
 * How the wizard starts routines. "all" activates every routine of the chosen
 * squads. "per_floor" activates the routines created active plus each floor's
 * lead routine. "leave_paused" keeps the creation default: only the Secretary's
 * brief and the market scan run.
 */
export type CompanyStart = (typeof COMPANY_START_OPTIONS)[number];

const KEY_PATTERN = /^[a-z][a-z0-9_]*$/;
const CRON_PATTERN = /^\S+( \S+){4}$/;

const CompanyRoleSchema = z.object({
  key: z.string().regex(KEY_PATTERN),
  /** Name of the agent the apply service creates. A non-archived agent with this name is reused. */
  name: z.string().min(1).max(64),
  description: z.string().min(1).max(200),
  agentTemplate: z.custom<AgentInstructionTemplate["id"]>(
    (value) => AGENT_INSTRUCTION_TEMPLATES.some((t) => t.id === value),
    { message: "unknown instruction template" },
  ),
  /**
   * "secretary" means the workspace's existing Secretary agent is the role;
   * the apply service links it and does not create a second one.
   */
  runtime: z.enum(["worker", "secretary"]),
  /** Roster text the leader reads; context only. */
  role: z.string().min(1),
});

const CompanyRoutineSchema = z.object({
  key: z.string().regex(KEY_PATTERN),
  /** Name of the autopilot. Reused by name, as the squad quickstart does. */
  title: z.string().min(1).max(120),
  cadence: z.enum(["daily", "weekly", "monthly"]),
  /** Five-field cron, in the timezone the wizard sends. */
  cron: z.string().regex(CRON_PATTERN),
  /** The issue body each run starts from. */
  description: z.string().min(1),
  /** Created active. Every other routine is created paused (spec 3.4: no surprise spend). */
  activeAtCreate: z.boolean(),
  /** Activated by "Start per floor": the lead routine of its floor. */
  floorStart: z.boolean(),
});

const CompanyCharterSchema = z.object({
  mission: z.string().min(1),
  responsibilities: z
    .array(z.object({ key: z.string().min(1), text: z.string().min(1) }))
    .min(1),
  outputs: z.array(
    z.object({
      key: z.string().min(1),
      kind: z.enum(["issue", "proposal", "kb_document", "report"]),
      cadenceHint: z.string(),
    }),
  ),
  boundaries: z.string().min(1),
  escalation: z.string().min(1),
  intake: z.string().min(1),
});

/**
 * Phase 4 stubs. Phase 3 installs squads at autonomy level 1, with no goals and
 * no budget share, so these values change nothing yet. They sit on the card so
 * phase 4 fills them in one place.
 */
const GovernanceStubSchema = z.object({
  /** Stub until phase 4 (spec 4.4): the autonomy level a squad starts at. */
  autonomyLevel: z.literal(1),
  /** Stub until phase 4 (spec 4.6): the squad's default share of the budget, in percent. */
  budgetSharePct: z.number().min(0).max(100),
  /** Stub until phase 4 (spec 4.1): goals the company scaffolds for the squad. Phase 3 scaffolds none. */
  goalsScaffold: z.array(z.string()),
});

export const CompanySquadCardSchema = z
  .object({
    key: z.enum(COMPANY_SQUAD_KEYS),
    /** Name of the squad. A squad with this name is reused. */
    name: z.string().min(1).max(64),
    description: z.string().min(1).max(200),
    floor: z.enum(COMPANY_FLOOR_KEYS),
    leader: z.string().regex(KEY_PATTERN),
    roles: z.array(CompanyRoleSchema).min(1),
    routines: z.array(CompanyRoutineSchema),
    /** Pipeline stages this squad owns or co-owns. The stage's primary owner is in COMPANY_STAGE_OWNERS. */
    stages: z.array(z.enum(COMPANY_STAGE_KEYS)),
    charter: CompanyCharterSchema,
    governance: GovernanceStubSchema,
  })
  .superRefine((card, ctx) => {
    const roleKeys = card.roles.map((role) => role.key);
    if (new Set(roleKeys).size !== roleKeys.length) {
      ctx.addIssue({ code: "custom", message: "role keys must be unique", path: ["roles"] });
    }
    if (!roleKeys.includes(card.leader)) {
      ctx.addIssue({ code: "custom", message: "leader must be one of the roles", path: ["leader"] });
    }
    const titles = card.routines.map((routine) => routine.title);
    if (new Set(titles).size !== titles.length) {
      ctx.addIssue({ code: "custom", message: "routine titles must be unique", path: ["routines"] });
    }
    const routineKeys = card.routines.map((routine) => routine.key);
    if (new Set(routineKeys).size !== routineKeys.length) {
      ctx.addIssue({ code: "custom", message: "routine keys must be unique", path: ["routines"] });
    }
  });

export type CompanySquadCard = z.infer<typeof CompanySquadCardSchema>;
export type CompanyRole = CompanySquadCard["roles"][number];
export type CompanyRoutine = CompanySquadCard["routines"][number];
export type CompanyRoutineCadence = CompanyRoutine["cadence"];

export const COMPANY_SQUAD_CARDS: readonly CompanySquadCard[] = COMPANY_SQUAD_CARDS_DATA.map((card) =>
  CompanySquadCardSchema.parse(card),
);

/** The card for a squad key. Throws for a key that has no card, which the type prevents. */
export function companySquadCard(key: CompanySquadKey): CompanySquadCard {
  const card = COMPANY_SQUAD_CARDS.find((c) => c.key === key);
  if (!card) throw new Error(`no company squad card "${key}"`);
  return card;
}

/**
 * The stages with no owner among the selected squads. A stage is unowned when
 * the squad that owns it in the pipeline is not selected.
 */
export function unownedStages(selected: readonly CompanySquadKey[]): CompanyStageOwner[] {
  const chosen = new Set(selected);
  return COMPANY_STAGE_OWNERS.filter((stage) => !chosen.has(stage.owner));
}

/** The routines `start` activates for the selected squads, each with its squad. */
export function routinesStartedBy(
  selected: readonly CompanySquadKey[],
  start: CompanyStart,
): { squad: CompanySquadKey; routine: CompanyRoutine }[] {
  const started: { squad: CompanySquadKey; routine: CompanyRoutine }[] = [];
  for (const key of selected) {
    for (const routine of companySquadCard(key).routines) {
      const on =
        start === "all" ||
        (start === "per_floor" && (routine.activeAtCreate || routine.floorStart)) ||
        (start === "leave_paused" && routine.activeAtCreate);
      if (on) started.push({ squad: key, routine });
    }
  }
  return started;
}

/** Runs per week for each cadence. A month is 52 / 12 weeks. */
export const RUNS_PER_WEEK: Record<CompanyRoutineCadence, number> = {
  daily: 7,
  weekly: 1,
  monthly: 12 / 52,
};

/**
 * The cost of one agent run, stated as a default. The wizard uses the
 * workspace's measured average once the server reports one; until then this
 * figure is shown and labelled as a default.
 */
export const DEFAULT_RUN_COST_USD = 0.5;

export interface CompanyEstimate {
  runsPerWeek: number;
  costPerWeekUsd: number;
  /** True when the cost uses DEFAULT_RUN_COST_USD rather than a measured average. */
  costIsDefault: boolean;
}

/** Runs and cost per week that the chosen start option adds for the selected squads. */
export function estimateCompanyRuns(
  selected: readonly CompanySquadKey[],
  start: CompanyStart,
  measuredRunCostUsd: number | null = null,
): CompanyEstimate {
  const runsPerWeek = routinesStartedBy(selected, start).reduce(
    (sum, { routine }) => sum + RUNS_PER_WEEK[routine.cadence],
    0,
  );
  const costIsDefault = measuredRunCostUsd === null;
  const perRun = measuredRunCostUsd ?? DEFAULT_RUN_COST_USD;
  return { runsPerWeek, costPerWeekUsd: runsPerWeek * perRun, costIsDefault };
}
