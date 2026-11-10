import { AGENT_INSTRUCTION_TEMPLATES } from "../agents/instruction-templates";
import type {
  Agent,
  Autopilot,
  AutopilotTrigger,
  CreateAgentRequest,
  CreateSkillRequest,
  CreateAutopilotRequest,
  CreateAutopilotTriggerRequest,
  IssueWorkflow,
  IssueWorkflowHandler,
  IssueWorkflowStepInput,
  IssueWorkflowWriteRequest,
  ListAutopilotsResponse,
  ListIssueWorkflowsResponse,
  Skill,
  SkillSummary,
  Squad,
  SquadMember,
  SquadSkill,
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
import { PLANNING_ROLE_AGENT_INSTRUCTIONS, type PlanningRoleKey } from "./planning-agent-instructions";
import { SDLC_ROLE_AGENT_INSTRUCTIONS, type SdlcRoleKey } from "./sdlc-agent-instructions";
import { SDLC_BUILTIN_SKILLS, type BuiltinSkill } from "./sdlc-skills-data";
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
  updateSquadCharter(squadId: string, data: UpdateSquadCharterRequest): Promise<SquadCharter>;
  createSquadRoutine(squadId: string, data: CreateSquadRoutineRequest): Promise<SquadRoutine>;
  addSquadMember(
    squadId: string,
    data: { member_type: string; member_id: string; role?: string },
  ): Promise<SquadMember>;
  listIssueWorkflows(): Promise<ListIssueWorkflowsResponse>;
  createIssueWorkflow(data: IssueWorkflowWriteRequest): Promise<IssueWorkflow>;
  listAutopilots(params?: { status?: string }): Promise<ListAutopilotsResponse>;
  createAutopilot(data: CreateAutopilotRequest): Promise<Autopilot>;
  createAutopilotTrigger(
    autopilotId: string,
    data: CreateAutopilotTriggerRequest,
  ): Promise<AutopilotTrigger>;
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
  routine: {
    /** "existing": the squad already has an autopilot with the routine's name, or the routine is already linked. */
    status: "created" | "existing" | "skipped" | "failed";
    /** A routine created without its schedule reports that here; add the schedule on the autopilot. */
    error?: string;
  };
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

export function instructionsFor(role: SquadRoleKey): string {
  if (role in SDLC_ROLE_AGENT_INSTRUCTIONS) {
    return SDLC_ROLE_AGENT_INSTRUCTIONS[role as SdlcRoleKey];
  }
  if (role in PLANNING_ROLE_AGENT_INSTRUCTIONS) {
    return PLANNING_ROLE_AGENT_INSTRUCTIONS[role as PlanningRoleKey];
  }
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
): Promise<QuickstartOutcome["routine"]> {
  const { autopilots } = await client.listAutopilots();
  const found = autopilots.find(
    (a) => a.assignee_type === "squad" && a.assignee_id === squadId && a.title === routine.title,
  );
  let autopilotId: string;
  let status: "created" | "existing" = "created";
  if (found) {
    autopilotId = found.id;
    status = "existing";
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
    try {
      await client.createAutopilotTrigger(autopilot.id, {
        kind: "schedule",
        cron_expression: routine.cron,
        ...(timezone ? { timezone } : {}),
      });
    } catch (err) {
      return { status: "failed", error: `schedule: ${errorMessage(err)}` };
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
    if (errorCode(err) === "routine_already_linked") return { status: "existing" };
    return { status: "failed", error: `routine: ${errorMessage(err)}` };
  }
  return { status };
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

function isBuiltinSkill(config: Record<string, unknown> | undefined): boolean {
  return config?.source === SDLC_SKILL_SOURCE;
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
): Promise<QuickstartOutcome["skills"]> {
  if (names.length === 0) return { status: "skipped" };
  const failures: string[] = [];
  const byName = new Map((await client.listSkills()).map((s) => [s.name, s] as const));
  const ids: string[] = [];
  for (const name of names) {
    const bundle = SDLC_BUILTIN_SKILLS.find((b) => b.name === name);
    if (!bundle) {
      failures.push(`${name}: not a built-in skill`);
      continue;
    }
    try {
      const found = byName.get(name);
      if (!found) {
        const created = await client.createSkill({
          name,
          description: bundle.description,
          content: bundle.content,
          files: bundle.files,
          config: { source: SDLC_SKILL_SOURCE },
        });
        ids.push(created.id);
        continue;
      }
      const current = await client.getSkill(found.id);
      if (!isBuiltinSkill(current.config)) {
        // The boss's own copy is attached when it already matches the built-in text,
        // and left alone otherwise.
        if (sameAsBuiltin(current, bundle)) {
          ids.push(found.id);
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
      ids.push(found.id);
    } catch (err) {
      failures.push(`${name}: ${errorMessage(err)}`);
    }
  }
  if (ids.length > 0) {
    try {
      // setSquadSkills replaces the whole set, so the squad's other skills are kept.
      const attached = (await client.listSquadSkills(squadId)).map((s) => s.id);
      const missing = ids.filter((id) => !attached.includes(id));
      if (missing.length > 0) await client.setSquadSkills(squadId, [...attached, ...missing]);
    } catch (err) {
      failures.push(`attach: ${errorMessage(err)}`);
    }
  }
  if (failures.length === 0) return { status: "ready" };
  return { status: ids.length > 0 ? "partial" : "failed", error: failures.join("; ") };
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
    routine: { status: "skipped" },
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
        instructions: instructionsFor(role),
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
        outcome.skills = await attachBuiltinSkills(client, found.id, template.builtinSkills).catch(
          (err): QuickstartOutcome["skills"] => ({ status: "failed", error: errorMessage(err) }),
        );
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
      outcome.skills = await attachBuiltinSkills(client, squad.id, template.builtinSkills).catch(
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
        } catch (err) {
          failures.push(`${SQUAD_ROLES[role].name}: ${errorMessage(err)}`);
        }
      }
      if (failures.length > 0) outcome.squad.error = failures.join("; ");
    }
  } catch (err) {
    outcome.squad = { id: null, status: "failed", error: errorMessage(err) };
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

  if (template.routine && outcome.squad.id) {
    try {
      outcome.routine = await ensureRoutine(
        client,
        outcome.squad.id,
        template.routine,
        options.timezone ?? browserTimezone(),
      );
    } catch (err) {
      outcome.routine = { status: "failed", error: errorMessage(err) };
    }
    report();
  }

  return outcome;
}
