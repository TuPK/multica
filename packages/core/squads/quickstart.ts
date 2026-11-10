import { AGENT_INSTRUCTION_TEMPLATES } from "../agents/instruction-templates";
import type {
  CreateKnowledgeDocumentRequest,
  KnowledgeDocument,
  KnowledgeStatus,
} from "../knowledge/types";
import type {
  Agent,
  Autopilot,
  AutopilotTrigger,
  CreateAgentRequest,
  GetAutopilotResponse,
  CreateSkillRequest,
  CreateAutopilotRequest,
  CreateAutopilotTriggerRequest,
  IssueWorkflow,
  IssueWorkflowHandler,
  IssueWorkflowStepInput,
  IssueWorkflowWriteRequest,
  ListAutopilotsResponse,
  ListIssueWorkflowsResponse,
  SetAgentSkillsRequest,
  Skill,
  SkillSummary,
  Squad,
  SquadMember,
  SquadSkill,
  UpdateAutopilotRequest,
  UpdateSkillRequest,
} from "../types";
import {
  CHARTER_DEFAULT_RESPONSE_HOURS,
  type CreateSquadRoutineRequest,
  type SquadCharter,
  type SquadRoutine,
  type UpdateSquadCharterRequest,
} from "../company/charter-schema";
import { freeSlots } from "../company/seat-plan";
import type {
  CompanyFloor,
  CompanySeatAssignment,
  CompanySeatsResponse,
} from "../company/types";
import { kitvietPackOf, loadKitvietRoleInstructions, loadKitvietSkillPacks } from "./kitviet-packs";
import { PLANNING_ROLE_AGENT_INSTRUCTIONS, type PlanningRoleKey } from "./planning-agent-instructions";
import { SDLC_ROLE_AGENT_INSTRUCTIONS, type SdlcRoleKey } from "./sdlc-agent-instructions";
import { SDLC_BUILTIN_SKILLS, type BuiltinSkill } from "./sdlc-skills-data";
import { SALES_ROLE_AGENT_INSTRUCTIONS, type SalesRoleKey } from "./sales-agent-instructions";
import { SALES_STARTER_SOURCES, starterSourceDocument } from "./sales-starter-sources";
import {
  SQUAD_ROLES,
  type SquadCharterTemplate,
  type SquadRoleKey,
  type SquadRoutineTemplate,
  type SquadTemplate,
  type SquadWorkflowStep,
} from "./templates";

/** The API calls the quickstart makes; the web client satisfies it as is. */
export interface SquadQuickstartClient {
  listAgents(params?: { include_archived?: boolean }): Promise<Agent[]>;
  createAgent(data: CreateAgentRequest): Promise<Agent>;
  listSquads(): Promise<Squad[]>;
  createSquad(data: {
    name: string;
    description?: string;
    leader_id: string;
    template_key?: string;
  }): Promise<Squad>;
  listCompanyFloors(): Promise<CompanyFloor[]>;
  createCompanyFloor(data: { name: string }): Promise<CompanyFloor>;
  setCompanySeats(seats: CompanySeatAssignment[]): Promise<CompanySeatsResponse>;
  updateSquad(id: string, data: { instructions?: string }): Promise<Squad>;
  listSkills(): Promise<SkillSummary[]>;
  getSkill(id: string): Promise<Skill>;
  createSkill(data: CreateSkillRequest): Promise<Skill>;
  updateSkill(id: string, data: UpdateSkillRequest): Promise<Skill>;
  listSquadSkills(squadId: string): Promise<SquadSkill[]>;
  setSquadSkills(squadId: string, skillIds: string[]): Promise<SquadSkill[]>;
  addAgentSkills(agentId: string, data: SetAgentSkillsRequest): Promise<void>;
  updateSquadCharter(squadId: string, data: UpdateSquadCharterRequest): Promise<SquadCharter>;
  createSquadRoutine(squadId: string, data: CreateSquadRoutineRequest): Promise<SquadRoutine>;
  addSquadMember(
    squadId: string,
    data: { member_type: string; member_id: string; role?: string },
  ): Promise<SquadMember>;
  listSquadMembers(squadId: string): Promise<SquadMember[]>;
  listIssueWorkflows(): Promise<ListIssueWorkflowsResponse>;
  createIssueWorkflow(data: IssueWorkflowWriteRequest): Promise<IssueWorkflow>;
  listAutopilots(params?: { status?: string }): Promise<ListAutopilotsResponse>;
  getAutopilot(id: string): Promise<GetAutopilotResponse>;
  createAutopilot(data: CreateAutopilotRequest): Promise<Autopilot>;
  updateAutopilot(id: string, data: UpdateAutopilotRequest): Promise<Autopilot>;
  createAutopilotTrigger(
    autopilotId: string,
    data: CreateAutopilotTriggerRequest,
  ): Promise<AutopilotTrigger>;
  getKnowledgeStatus(): Promise<KnowledgeStatus>;
  createKnowledgeDocument(data: CreateKnowledgeDocumentRequest): Promise<KnowledgeDocument>;
}

export type QuickstartAgentStatus = "created" | "reused" | "failed";

export interface QuickstartAgentOutcome {
  role: SquadRoleKey;
  name: string;
  id: string | null;
  status: QuickstartAgentStatus;
  error?: string;
}

export interface QuickstartOutcome {
  agents: QuickstartAgentOutcome[];
  squad: {
    id: string | null;
    /** "existing": a squad with this name was already there and is left as it is. */
    status: "created" | "existing" | "failed";
    /** Members that could not be added, or why the squad was not made. */
    error?: string;
  };
  workflow: {
    status: "created" | "existing" | "skipped" | "failed";
    /** "qa_step_left_out": the QA status could not be created, so the workflow has no QA step. */
    note?: "qa_step_left_out";
    error?: string;
  };
  /** The built-in SDLC skills: imported into the workspace and attached to every member of the squad. */
  skills: {
    /** "partial": some skills were not attached; the error names them. */
    status: "ready" | "partial" | "skipped" | "failed";
    error?: string;
  };
  /** One entry per routine the template creates. */
  routines: QuickstartRoutineOutcome[];
  /** The starter sources of a sales squad, written to the knowledge base. Skipped for other templates. */
  sources: {
    status: "created" | "skipped" | "failed";
    /** Documents written; a rerun writes the same documents again under the same keys. */
    written: number;
    /** Documents that were refused, with the first reason. */
    failed: number;
    error?: string;
  };
}

export interface QuickstartRoutineOutcome {
  title: string;
  /** "existing": the squad already has an autopilot with the routine's name, or the routine is already linked. */
  status: "created" | "existing" | "failed";
  /** A routine created without its schedule reports that here; add the schedule on the autopilot. */
  error?: string;
}

export interface QuickstartOptions {
  runtimeId: string;
  /** Also create the template's workflow when the workspace has none by that name. */
  createWorkflow: boolean;
  /** IANA timezone for the routine's schedule. Defaults to the browser's; the server uses UTC when none is sent. */
  timezone?: string;
}

export function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : "unknown error";
}

function errorCode(err: unknown): string | undefined {
  const body = (err as { body?: unknown } | null)?.body;
  if (body && typeof body === "object" && "code" in body) {
    const code = (body as { code?: unknown }).code;
    return typeof code === "string" ? code : undefined;
  }
  return undefined;
}

/** The browser's IANA timezone, or undefined when the runtime cannot tell. */
function browserTimezone(): string | undefined {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || undefined;
  } catch {
    return undefined;
  }
}

/**
 * The instructions an agent of this role starts from. A Kit Viet role is composed from its pack
 * (header, kind text, body, squad notes, shared blocks; see kitviet-agent-instructions.ts), which
 * loads on demand. Other roles use the text of their SDLC, sales or planning set, or the standalone
 * instruction template of their agent kind.
 */
export async function instructionsFor(role: SquadRoleKey): Promise<string> {
  if (role in SALES_ROLE_AGENT_INSTRUCTIONS) {
    return SALES_ROLE_AGENT_INSTRUCTIONS[role as SalesRoleKey];
  }
  if (role in SDLC_ROLE_AGENT_INSTRUCTIONS) {
    return SDLC_ROLE_AGENT_INSTRUCTIONS[role as SdlcRoleKey];
  }
  if (role in PLANNING_ROLE_AGENT_INSTRUCTIONS) {
    return PLANNING_ROLE_AGENT_INSTRUCTIONS[role as PlanningRoleKey];
  }
  const kitviet = await loadKitvietRoleInstructions(role);
  if (kitviet !== undefined) return kitviet;
  const id = SQUAD_ROLES[role].agentTemplate;
  const template = AGENT_INSTRUCTION_TEMPLATES.find((t) => t.id === id);
  if (!template) throw new Error(`no instruction template "${id}"`);
  return template.instructions;
}

/** What the quickstart would do with the agents the workspace already has. */
export function planQuickstartAgents(
  template: SquadTemplate,
  agents: readonly Agent[],
): { role: SquadRoleKey; name: string; reuse: boolean }[] {
  const live = new Set(agents.filter((a) => !a.archived_at).map((a) => a.name));
  return template.roles.map((role) => {
    const { name } = SQUAD_ROLES[role];
    return { role, name, reuse: live.has(name) };
  });
}

export function workflowSteps(
  steps: readonly SquadWorkflowStep[],
  agentIds: Partial<Record<SquadRoleKey, string>>,
): IssueWorkflowStepInput[] | null {
  const out: IssueWorkflowStepInput[] = [];
  for (const step of steps) {
    let handler: IssueWorkflowHandler;
    if (step.handler) {
      const id = agentIds[step.handler];
      if (!id) return null;
      handler = { type: "agent", id };
    } else {
      handler = { type: "none" };
    }
    const base = {
      handler,
      instructions: step.instructions,
      ...(step.next ? { next_status_key: step.next } : {}),
      ...(step.back ? { back_status_key: step.back } : {}),
    };
    out.push(
      "key" in step.status
        ? { status_key: step.status.key, ...base }
        : {
            new_status: { name: step.status.name, category: step.status.category ?? "started" },
            ...base,
          },
    );
  }
  return out;
}

/** The new statuses a workflow creates, as a range for an error message: "Analysis to Plan ready". */
function newStatusRange(steps: readonly SquadWorkflowStep[]): string {
  const names = steps.flatMap((s) => ("name" in s.status ? [s.status.name] : []));
  const last = names.includes("Plan ready") ? "Plan ready" : names[names.length - 1];
  return names.length > 1 ? `${names[0]} to ${last}` : (names[0] ?? "");
}

/**
 * The company floor with this name, created when the workspace has none. The
 * server only applies a squad's floor name when every floor is full, so the
 * squad is seated on this floor by a separate seat request.
 */
async function ensureCompanyFloor(
  client: SquadQuickstartClient,
  name: string,
): Promise<CompanyFloor> {
  const floors = await client.listCompanyFloors();
  return floors.find((floor) => floor.name === name) ?? client.createCompanyFloor({ name });
}

/**
 * Creates the routine's autopilot for the squad, assigned to the squad so each
 * run opens an issue for its leader, and its weekly schedule. Then links it as
 * a routine of the squad, so the charter and the digest see it. A squad that
 * already has an autopilot with the routine's name keeps it, and it is linked
 * when it is not a routine yet.
 */
async function ensureRoutine(
  client: SquadQuickstartClient,
  squadId: string,
  routine: SquadRoutineTemplate,
  timezone: string | undefined,
): Promise<QuickstartRoutineOutcome> {
  const scheduleRequest = {
    kind: "schedule" as const,
    cron_expression: routine.cron,
    ...(timezone ? { timezone } : {}),
  };
  const { autopilots } = await client.listAutopilots();
  const found = autopilots.find(
    (a) => a.assignee_type === "squad" && a.assignee_id === squadId && a.title === routine.title,
  );
  let autopilotId: string;
  let status: "created" | "existing" = "created";
  // Set when the autopilot runs, or is left active, without the paused start the template asks for.
  let warning: string | undefined;
  if (found) {
    autopilotId = found.id;
    status = "existing";
    // A schedule the boss disabled still counts: adding one would switch the boss's choice back on.
    const { triggers } = await client.getAutopilot(found.id);
    if (!triggers.some((t) => t.kind === "schedule")) {
      const active = found.status === "active";
      const pausedStart = routine.startPaused === true && active;
      try {
        await client.createAutopilotTrigger(found.id, scheduleRequest);
      } catch (err) {
        // The autopilot is active and has no schedule, so it does not run. Left as it is for the boss to see.
        const note = pausedStart ? "; the autopilot is active without a schedule, pause or delete it" : "";
        return { title: routine.title, status: "failed", error: `schedule: ${errorMessage(err)}${note}` };
      }
      // The schedule is now set on an autopilot that should have started paused. It is not unpaused here.
      if (pausedStart) {
        warning = "the schedule was added to an active autopilot that should start paused; pause it on the autopilot if it should wait";
      }
    }
  } else {
    const autopilot = await client.createAutopilot({
      title: routine.title,
      description: routine.description,
      assignee_type: "squad",
      assignee_id: squadId,
      execution_mode: "create_issue",
      issue_title_template: routine.issueTitleTemplate,
    });
    autopilotId = autopilot.id;
    // Paused before the schedule is set, so the routine cannot fire while it is being made.
    if (routine.startPaused) {
      try {
        await client.updateAutopilot(autopilot.id, { status: "paused" });
      } catch (err) {
        // No schedule is set, so the autopilot cannot fire. A rerun adds the schedule, see above.
        return {
          title: routine.title,
          status: "failed",
          error: `pause: ${errorMessage(err)}; the autopilot is active without a schedule, pause or delete it`,
        };
      }
    }
    try {
      await client.createAutopilotTrigger(autopilot.id, scheduleRequest);
    } catch (err) {
      return { title: routine.title, status: "failed", error: `schedule: ${errorMessage(err)}` };
    }
  }
  try {
    await client.createSquadRoutine(squadId, {
      autopilot_id: autopilotId,
      purpose: routine.title,
      expected_output: "report",
      required: true,
    });
  } catch (err) {
    // Already linked: an earlier run finished the job.
    if (errorCode(err) === "routine_already_linked") {
      return warning ? { title: routine.title, status: "failed", error: warning } : { title: routine.title, status: "existing" };
    }
    return { title: routine.title, status: "failed", error: `routine: ${errorMessage(err)}` };
  }
  if (warning) return { title: routine.title, status: "failed", error: warning };
  return { title: routine.title, status };
}

/**
 * Writes the sales squad's starter sources into the knowledge base, one document each, under the
 * source's idempotency key. Knowledge that is not enabled skips them with the reason, so the squad
 * is still made and the rerun after enabling finishes the job.
 */
async function ensureStarterSources(client: SquadQuickstartClient): Promise<QuickstartOutcome["sources"]> {
  const knowledge = await client.getKnowledgeStatus();
  if (knowledge.state !== "ready") {
    return {
      status: "skipped",
      written: 0,
      failed: 0,
      error: "Enable the company knowledge base, then run the quickstart again to add the starter sources.",
    };
  }
  let written = 0;
  let failed = 0;
  let error: string | undefined;
  for (const source of SALES_STARTER_SOURCES) {
    try {
      await client.createKnowledgeDocument(starterSourceDocument(source));
      written += 1;
    } catch (err) {
      failed += 1;
      error ??= `${source.id}: ${errorMessage(err)}`;
    }
  }
  return { status: failed > 0 ? "failed" : "created", written, failed, ...(error ? { error } : {}) };
}

/** The charter a template saves on creation. Revision 0 is the first save; operating review stays on. */
function charterRequest(charter: SquadCharterTemplate): UpdateSquadCharterRequest {
  return {
    revision: 0,
    mission: charter.mission,
    responsibilities: charter.responsibilities,
    outputs: charter.outputs.map((o) => ({
      key: o.key,
      kind: o.kind,
      kb_key: "",
      cadence_hint: o.cadenceHint,
    })),
    boundaries: charter.boundaries,
    escalation: charter.escalation,
    intake: charter.intake,
    response_hours: CHARTER_DEFAULT_RESPONSE_HOURS,
  };
}

/**
 * Whether a squad with the template's name was made by the quickstart itself:
 * it has the template's leader and the template's instructions, both written
 * by the first run. Matching on the name alone would charter a hand-made squad.
 */
function isQuickstartSquad(squad: Squad, leaderId: string, template: SquadTemplate): boolean {
  return squad.leader_id === leaderId && squad.instructions.trim() === template.instructions.trim();
}

/** The config marker of a skill the quickstart imported, so a rerun may update it. */
export const SDLC_SKILL_SOURCE = "sdlc_built_in";

/** The config marker of a Kit Viet skill the quickstart imported. Reruns treat it the same way. */
export const KITVIET_SKILL_SOURCE = "kitviet";

function isBuiltinSkill(config: Record<string, unknown> | undefined): boolean {
  return config?.source === SDLC_SKILL_SOURCE || config?.source === KITVIET_SKILL_SOURCE;
}

/** A Kit Viet skill name: `kv--<name>` as stored, or `kv:<name>` as the upstream writes it. */
export function isKitvietSkillName(name: string): boolean {
  return name.startsWith("kv--") || name.startsWith("kv:");
}

const kitvietSkillsByPack = new Map<string, Promise<ReadonlyMap<string, BuiltinSkill>>>();

/**
 * The Kit Viet skills a squad of this pack can name, by name: `common` and its own pack. A failed
 * load is dropped, so the next call tries again.
 */
function loadKitvietSkills(pack: string | undefined): Promise<ReadonlyMap<string, BuiltinSkill>> {
  const key = pack ?? "";
  let skills = kitvietSkillsByPack.get(key);
  if (!skills) {
    skills = loadKitvietSkillPacks(pack)
      .then(
        (packs) =>
          new Map(packs.flatMap((p) => p.KITVIET_BUILTIN_SKILLS).map((skill) => [skill.name, skill] as const)),
      )
      .catch((err: unknown) => {
        kitvietSkillsByPack.delete(key);
        throw err;
      });
    kitvietSkillsByPack.set(key, skills);
  }
  return skills;
}

/** The built-in skill with this name: a Kit Viet skill from the squad's pack, or an SDLC one. */
async function builtinSkillNamed(name: string, pack: string | undefined): Promise<BuiltinSkill | undefined> {
  if (isKitvietSkillName(name)) return (await loadKitvietSkills(pack)).get(name);
  return SDLC_BUILTIN_SKILLS.find((b) => b.name === name);
}

/** Whether a workspace skill already holds the built-in skill's description, body, and files. */
function sameAsBuiltin(skill: Skill, bundle: BuiltinSkill): boolean {
  const files = (skill.files ?? []).map((f) => [f.path, f.content]).sort();
  const wanted = bundle.files.map((f) => [f.path, f.content]).sort();
  return (
    skill.description === bundle.description &&
    skill.content === bundle.content &&
    JSON.stringify(files) === JSON.stringify(wanted)
  );
}

/**
 * Imports the named built-in skills into the workspace and returns the workspace id of each one
 * it holds. A skill the quickstart made before is updated to the built-in text. A skill the boss
 * made under the same name is used when its text already matches the built-in one, and otherwise
 * left as it is and named in the failures. `pack` is the Kit Viet pack of the squad, if any.
 */
async function importBuiltinSkills(
  client: SquadQuickstartClient,
  names: readonly string[],
  pack: string | undefined,
): Promise<{ ids: Map<string, string>; failures: string[] }> {
  const failures: string[] = [];
  const ids = new Map<string, string>();
  const byName = new Map((await client.listSkills()).map((s) => [s.name, s] as const));
  for (const name of names) {
    try {
      const bundle = await builtinSkillNamed(name, pack);
      if (!bundle) {
        failures.push(`${name}: not a built-in skill`);
        continue;
      }
      const source = isKitvietSkillName(name) ? KITVIET_SKILL_SOURCE : SDLC_SKILL_SOURCE;
      const found = byName.get(name);
      if (!found) {
        const created = await client.createSkill({
          name,
          description: bundle.description,
          content: bundle.content,
          files: bundle.files,
          config: { source },
        });
        ids.set(name, created.id);
        continue;
      }
      const current = await client.getSkill(found.id);
      if (!isBuiltinSkill(current.config)) {
        // The boss's own copy is attached when it already matches the built-in text,
        // and left alone otherwise.
        if (sameAsBuiltin(current, bundle)) {
          ids.set(name, found.id);
          continue;
        }
        failures.push(`${name}: the workspace has its own skill under this name`);
        continue;
      }
      if (!sameAsBuiltin(current, bundle)) {
        await client.updateSkill(found.id, {
          description: bundle.description,
          content: bundle.content,
          files: bundle.files,
        });
      }
      ids.set(name, found.id);
    } catch (err) {
      failures.push(`${name}: ${errorMessage(err)}`);
    }
  }
  return { ids, failures };
}

/**
 * Imports the built-in skills a template names into the workspace, and attaches
 * them to the squad, so every member gets them. A skill the quickstart made
 * before is updated to the built-in text. A skill the boss made under the same
 * name is attached when its text already matches the built-in one, and otherwise
 * left as it is and named in the error; the squad keeps the others.
 */
export async function attachBuiltinSkills(
  client: SquadQuickstartClient,
  squadId: string,
  names: readonly string[],
  pack: string | undefined,
): Promise<QuickstartOutcome["skills"]> {
  if (names.length === 0) return { status: "skipped" };
  const { ids, failures } = await importBuiltinSkills(client, names, pack);
  if (ids.size > 0) {
    try {
      // setSquadSkills replaces the whole set, so the squad's other skills are kept.
      const attached = (await client.listSquadSkills(squadId)).map((s) => s.id);
      const missing = [...ids.values()].filter((id) => !attached.includes(id));
      if (missing.length > 0) await client.setSquadSkills(squadId, [...attached, ...missing]);
    } catch (err) {
      failures.push(`attach: ${errorMessage(err)}`);
    }
  }
  if (failures.length === 0) return { status: "ready" };
  return { status: ids.size > 0 ? "partial" : "failed", error: failures.join("; ") };
}

/**
 * Attaches the leader's skills (`leaderSkills`) and each role's skills (`roleSkills`) to that
 * agent alone, with addAgentSkills, which adds to what the agent has. The server keeps one row
 * per skill, so a rerun adds nothing twice. Only the agents in `eligible` get them: the ones this
 * quickstart made, and the members of its own squad. A reused agent elsewhere keeps its skills.
 * Returns the failures; a skill that cannot be imported is attached to nobody, and the rest still are.
 */
export async function attachAgentSkills(
  client: SquadQuickstartClient,
  template: SquadTemplate,
  agentIds: Partial<Record<SquadRoleKey, string>>,
  eligible: ReadonlySet<string>,
  pack: string | undefined,
): Promise<string[]> {
  const grants = template.roles.flatMap((role) => {
    const agentId = agentIds[role];
    const names = [
      ...new Set([
        ...(role === template.leader ? (template.leaderSkills ?? []) : []),
        ...(template.roleSkills?.[role] ?? []),
      ]),
    ];
    return agentId && eligible.has(agentId) && names.length > 0 ? [{ role, agentId, names }] : [];
  });
  if (grants.length === 0) return [];
  const { ids, failures } = await importBuiltinSkills(
    client,
    [...new Set(grants.flatMap((grant) => grant.names))],
    pack,
  );
  for (const grant of grants) {
    const skillIds = grant.names.flatMap((name) => ids.get(name) ?? []);
    if (skillIds.length === 0) continue;
    try {
      await client.addAgentSkills(grant.agentId, { skill_ids: skillIds });
    } catch (err) {
      failures.push(`${SQUAD_ROLES[grant.role].name}: ${errorMessage(err)}`);
    }
  }
  return failures;
}

/** Adds the agents' skill failures to the squad's skill outcome, which they make partial. */
export function withAgentSkillFailures(
  skills: QuickstartOutcome["skills"],
  failures: readonly string[],
): QuickstartOutcome["skills"] {
  const error = [skills.error, ...failures].filter(Boolean).join("; ");
  return { status: skills.status === "failed" ? "failed" : "partial", error };
}

/**
 * Creates the agents a squad template needs (reusing agents that already have
 * the template's names), the squad with its instructions and roster, and
 * optionally its workflow and routine. It never edits something that already
 * exists, except the charter of a squad that an earlier run made and left
 * without one. Running it again after a partial failure finishes the job.
 */
export async function runSquadQuickstart(
  client: SquadQuickstartClient,
  template: SquadTemplate,
  options: QuickstartOptions,
  onProgress?: (outcome: QuickstartOutcome) => void,
): Promise<QuickstartOutcome> {
  const outcome: QuickstartOutcome = {
    agents: [],
    squad: { id: null, status: "failed" },
    workflow: { status: "skipped" },
    skills: { status: "skipped" },
    routines: [],
    sources: { status: "skipped", written: 0, failed: 0 },
  };
  const report = () => onProgress?.(structuredClone(outcome));

  const existing = await client.listAgents({ include_archived: false });
  const byName = new Map(
    existing.filter((a) => !a.archived_at).map((a) => [a.name, a] as const),
  );
  const agentIds: Partial<Record<SquadRoleKey, string>> = {};

  for (const role of template.roles) {
    const { name, description } = SQUAD_ROLES[role];
    const found = byName.get(name);
    if (found) {
      agentIds[role] = found.id;
      outcome.agents.push({ role, name, id: found.id, status: "reused" });
      report();
      continue;
    }
    try {
      const created = await client.createAgent({
        name,
        description,
        instructions: await instructionsFor(role),
        runtime_id: options.runtimeId,
      });
      agentIds[role] = created.id;
      outcome.agents.push({ role, name, id: created.id, status: "created" });
    } catch (err) {
      outcome.agents.push({
        role,
        name,
        id: null,
        status: "failed",
        error: errorMessage(err),
      });
    }
    report();
  }

  const leaderId = agentIds[template.leader];
  if (!leaderId) {
    outcome.squad = {
      id: null,
      status: "failed",
      error: "the squad leader could not be created",
    };
    report();
    return outcome;
  }

  // The agents a Kit Viet agent skill may go to: the ones this run made, and the members of the
  // quickstart's own squad (filled in below). A reused agent elsewhere is left with its skills.
  const eligibleAgents = new Set(
    outcome.agents.flatMap((a) => (a.status === "created" && a.id ? [a.id] : [])),
  );
  const agentSkillFailures: string[] = [];

  try {
    const squads = await client.listSquads();
    const found = squads.find((s) => !s.archived_at && s.name === template.name);
    if (found) {
      outcome.squad = { id: found.id, status: "existing" };
      // A run that stopped after the squad was made (for example, at the
      // charter save) leaves it without a charter, so the finishing run sends
      // the revision-0 save here. A hand-made squad with the same name is
      // neither led by the template's leader nor carries its instructions, so
      // it is left as it is.
      if (isQuickstartSquad(found, leaderId, template)) {
        eligibleAgents.add(leaderId);
        try {
          for (const member of await client.listSquadMembers(found.id)) eligibleAgents.add(member.member_id);
        } catch (err) {
          agentSkillFailures.push(`members: ${errorMessage(err)}`);
        }
        outcome.skills = await attachBuiltinSkills(
          client,
          found.id,
          template.builtinSkills,
          kitvietPackOf(template.id),
        ).catch((err): QuickstartOutcome["skills"] => ({ status: "failed", error: errorMessage(err) }));
        try {
          await client.updateSquadCharter(found.id, charterRequest(template.charter));
        } catch (err) {
          // A charter that already exists refuses a revision-0 save: already chartered.
          if (errorCode(err) !== "charter_revision_conflict") {
            outcome.squad.error = `charter: ${errorMessage(err)}`;
          }
        }
      }
    } else {
      const failures: string[] = [];
      // Set up the floor first. If that fails the squad is still created, and
      // its error says it has no seat on the floor.
      let floor: CompanyFloor | null = null;
      if (template.floorName) {
        try {
          floor = await ensureCompanyFloor(client, template.floorName);
        } catch (err) {
          failures.push(`could not set up the ${template.floorName} floor: ${errorMessage(err)}`);
        }
      }
      const squad = await client.createSquad({
        name: template.name,
        description: template.description,
        leader_id: leaderId,
        template_key: template.id,
      });
      outcome.squad = { id: squad.id, status: "created" };
      if (floor) {
        // Free slots come from the squads listed before the create, so the
        // new squad's own auto seat (if it is on this floor) shows as free.
        // The squad is not moved when it already holds the chosen slot.
        const slot = freeSlots(floor.id, squads)[0];
        if (slot === undefined) {
          failures.push(`no free seat on the ${template.floorName} floor`);
        } else if (squad.floor_id !== floor.id || squad.company_slot !== slot) {
          try {
            await client.setCompanySeats([{ squad_id: squad.id, floor_id: floor.id, slot }]);
          } catch (err) {
            failures.push(`seat on the ${template.floorName} floor: ${errorMessage(err)}`);
          }
        }
      }
      try {
        await client.updateSquad(squad.id, { instructions: template.instructions });
      } catch (err) {
        failures.push(`instructions: ${errorMessage(err)}`);
      }
      eligibleAgents.add(leaderId);
      outcome.skills = await attachBuiltinSkills(client, squad.id, template.builtinSkills, kitvietPackOf(template.id)).catch(
        (err): QuickstartOutcome["skills"] => ({ status: "failed", error: errorMessage(err) }),
      );
      // The first charter save also creates the squad's operating review.
      try {
        await client.updateSquadCharter(squad.id, charterRequest(template.charter));
      } catch (err) {
        failures.push(`charter: ${errorMessage(err)}`);
      }
      for (const role of template.roles) {
        const memberId = agentIds[role];
        if (role === template.leader || !memberId) continue;
        try {
          await client.addSquadMember(squad.id, {
            member_type: "agent",
            member_id: memberId,
            role: SQUAD_ROLES[role].role,
          });
          eligibleAgents.add(memberId);
        } catch (err) {
          failures.push(`${SQUAD_ROLES[role].name}: ${errorMessage(err)}`);
        }
      }
      if (failures.length > 0) outcome.squad.error = failures.join("; ");
    }
  } catch (err) {
    outcome.squad = { id: null, status: "failed", error: errorMessage(err) };
  }
  try {
    agentSkillFailures.push(
      ...(await attachAgentSkills(client, template, agentIds, eligibleAgents, kitvietPackOf(template.id))),
    );
  } catch (err) {
    agentSkillFailures.push(`skills: ${errorMessage(err)}`);
  }
  if (agentSkillFailures.length > 0) {
    outcome.skills = withAgentSkillFailures(outcome.skills, agentSkillFailures);
  }
  report();

  if (options.createWorkflow && template.workflow) {
    try {
      const { workflows } = await client.listIssueWorkflows();
      if (workflows.some((w) => w.name === template.workflow!.name)) {
        outcome.workflow = { status: "existing" };
      } else {
        // The planning and QA statuses are created with the workflow. Without
        // a workspace admin they cannot be, and the steps that hand to them
        // would point at nothing, so the workflow is not built at all.
        const steps = workflowSteps(template.workflow.steps, agentIds);
        if (!steps) throw new Error("a workflow step has no agent to hand to");
        try {
          await client.createIssueWorkflow({
            name: template.workflow.name,
            description: template.workflow.description,
            steps,
          });
          outcome.workflow = { status: "created" };
        } catch (err) {
          if (errorCode(err) === "status_create_forbidden") {
            throw new Error(`the workflow needs statuses a workspace admin must allow: ${newStatusRange(template.workflow.steps)}`);
          }
          throw err;
        }
      }
    } catch (err) {
      outcome.workflow = { status: "failed", error: errorMessage(err) };
    }
    report();
  }

  for (const routine of template.routines) {
    if (!outcome.squad.id) break;
    try {
      outcome.routines.push(
        await ensureRoutine(client, outcome.squad.id, routine, options.timezone ?? browserTimezone()),
      );
    } catch (err) {
      outcome.routines.push({ title: routine.title, status: "failed", error: errorMessage(err) });
    }
    report();
  }

  if (template.id === "sales_loop" && outcome.squad.id) {
    try {
      outcome.sources = await ensureStarterSources(client);
    } catch (err) {
      outcome.sources = { status: "failed", written: 0, failed: 0, error: errorMessage(err) };
    }
    report();
  }

  return outcome;
}
