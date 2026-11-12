import type {
  Agent,
  IssueWorkflow,
  IssueWorkflowStepInput,
  ListIssueWorkflowsResponse,
  Squad,
  SquadMember,
  UpdateAgentRequest,
} from "../types";
import { kitvietPackOf } from "./kitviet-packs";
import {
  attachAgentSkills,
  attachBuiltinSkills,
  errorMessage,
  instructionsFor,
  type QuickstartOutcome,
  type SquadQuickstartClient,
  withAgentSkillFailures,
  workflowSteps,
} from "./quickstart";
import {
  SDLC_PREVIOUS_AGENT_INSTRUCTIONS,
  SDLC_PREVIOUS_SQUAD_INSTRUCTIONS,
  SDLC_PREVIOUS_STEP_INSTRUCTIONS,
} from "./sdlc-previous-texts";
import { SQUAD_ROLES, SQUAD_TEMPLATES, squadTemplateCategory, type SquadRoleKey, type SquadTemplate } from "./templates";

/**
 * Brings a squad the quickstart made earlier up to the current SDLC template: the built-in
 * skills, the agents' and the squad's instructions, and the workflow's missing steps and briefs.
 * The quickstart itself never edits something that exists. This does, but only where the
 * existing text still equals an earlier text of ours; anything the boss edited is kept and named
 * in the result. Running it twice changes nothing the second time.
 */

export interface SdlcUpgradeClient extends SquadQuickstartClient {
  updateAgent(id: string, data: UpdateAgentRequest): Promise<Agent>;
  updateIssueWorkflow(id: string, data: { steps: IssueWorkflowStepInput[] }): Promise<IssueWorkflow>;
  dryRunIssueWorkflowUpdate(id: string, data: { steps: IssueWorkflowStepInput[] }): Promise<unknown>;
}

export type UpgradeStatus = "upgraded" | "current" | "kept" | "missing" | "failed";

export interface SdlcUpgradeResult {
  squad: { status: UpgradeStatus; detail?: string };
  skills: QuickstartOutcome["skills"];
  agents: { role: SquadRoleKey; name: string; status: UpgradeStatus; detail?: string }[];
  workflow: {
    status: UpgradeStatus;
    detail?: string;
    /** Steps the upgrade added. */
    added: string[];
    /** Steps whose brief it replaced. */
    updated: string[];
    /** Steps whose brief differs from every text of ours, so it was kept. */
    kept: string[];
  };
}

const statusKeyOfName = (name: string) => name.toLowerCase().replaceAll(" ", "_");

const PREVIOUS_STEP_BRIEFS: Record<string, readonly string[]> = Object.fromEntries(
  Object.entries(SDLC_PREVIOUS_STEP_INSTRUCTIONS).map(([key, texts]) => [statusKeyOfName(key), texts]),
);

// The server returns text as stored; trailing whitespace is not a customisation.
const same = (a: string, b: string) => a.trim() === b.trim();

function decide(
  existing: string,
  target: string,
  previous: readonly string[],
): "current" | "replace" | "kept" {
  if (same(existing, target)) return "current";
  return previous.some((p) => same(p, existing)) ? "replace" : "kept";
}

/** The workflow's steps as a write request, with the template's missing steps added and known old briefs replaced. */
export function mergeWorkflowSteps(
  existing: IssueWorkflow,
  wanted: IssueWorkflowStepInput[],
): { steps: IssueWorkflowStepInput[]; added: string[]; updated: string[]; kept: string[] } {
  const keyOf = (s: IssueWorkflowStepInput) => ("status_key" in s ? s.status_key : statusKeyOfName(s.new_status.name));
  const wantedByKey = new Map(wanted.map((s) => [keyOf(s), s] as const));
  const have = new Set(existing.steps.map((s) => s.status_key));
  const updated: string[] = [];
  const kept: string[] = [];

  const merged: IssueWorkflowStepInput[] = existing.steps.map((step) => {
    const base: IssueWorkflowStepInput = {
      status_key: step.status_key,
      handler: step.handler,
      instructions: step.instructions,
      ...(step.next_status_key ? { next_status_key: step.next_status_key } : {}),
      ...(step.back_status_key ? { back_status_key: step.back_status_key } : {}),
    };
    const target = wantedByKey.get(step.status_key);
    if (!target || target.instructions === undefined || target.instructions === "") return base;
    const verdict = decide(step.instructions, target.instructions, PREVIOUS_STEP_BRIEFS[step.status_key] ?? []);
    if (verdict === "replace") {
      updated.push(step.status_key);
      return { ...base, instructions: target.instructions };
    }
    if (verdict === "kept") kept.push(step.status_key);
    return base;
  });

  // A missing step goes before the first of the template's later steps that the workflow has, so the
  // board order follows the template.
  const added: string[] = [];
  const order = wanted.map(keyOf);
  for (const [i, step] of wanted.entries()) {
    const key = keyOf(step);
    if (have.has(key)) continue;
    const successor = order.slice(i + 1).find((k) => have.has(k) || merged.some((m) => keyOf(m) === k));
    const at = successor === undefined ? merged.length : merged.findIndex((m) => keyOf(m) === successor);
    merged.splice(at < 0 ? merged.length : at, 0, step);
    have.add(key);
    added.push(key);
  }
  return { steps: merged, added, updated, kept };
}

export async function upgradeSdlcSquad(
  client: SdlcUpgradeClient,
  template: SquadTemplate,
  options: { dryRun?: boolean } = {},
): Promise<SdlcUpgradeResult> {
  const dry = options.dryRun === true;
  const result: SdlcUpgradeResult = {
    squad: { status: "missing" },
    skills: { status: "skipped" },
    agents: [],
    workflow: { status: "missing", added: [], updated: [], kept: [] },
  };

  const agents = (await client.listAgents({ include_archived: false })).filter((a) => !a.archived_at);
  const byName = new Map(agents.map((a) => [a.name, a] as const));
  const agentIds: Partial<Record<SquadRoleKey, string>> = {};

  for (const role of template.roles) {
    const { name } = SQUAD_ROLES[role];
    const agent = byName.get(name);
    if (!agent) {
      result.agents.push({ role, name, status: "missing" });
      continue;
    }
    agentIds[role] = agent.id;
    const wanted = await instructionsFor(role);
    const verdict = decide(agent.instructions ?? "", wanted, SDLC_PREVIOUS_AGENT_INSTRUCTIONS[role] ?? []);
    if (verdict !== "replace") {
      result.agents.push({ role, name, status: verdict, ...(verdict === "kept" ? { detail: "instructions differ from every earlier text; kept" } : {}) });
      continue;
    }
    try {
      if (!dry) await client.updateAgent(agent.id, { instructions: wanted });
      result.agents.push({ role, name, status: "upgraded" });
    } catch (err) {
      result.agents.push({ role, name, status: "failed", detail: errorMessage(err) });
    }
  }

  const squads: Squad[] = await client.listSquads();
  const squad = squads.find((s) => !s.archived_at && s.name === template.name);
  if (squad) {
    const verdict = decide(squad.instructions ?? "", template.instructions, SDLC_PREVIOUS_SQUAD_INSTRUCTIONS);
    if (verdict === "replace") {
      try {
        if (!dry) await client.updateSquad(squad.id, { instructions: template.instructions });
        result.squad = { status: "upgraded" };
      } catch (err) {
        result.squad = { status: "failed", detail: errorMessage(err) };
      }
    } else {
      result.squad = { status: verdict, ...(verdict === "kept" ? { detail: "instructions differ from every earlier text; kept" } : {}) };
    }
    if (!dry) {
      result.skills = await attachBuiltinSkills(client, squad.id, template.builtinSkills, undefined).catch(
        (err): QuickstartOutcome["skills"] => ({ status: "failed", error: errorMessage(err) }),
      );
    }
  }

  if (template.workflow) {
    try {
      const { workflows }: ListIssueWorkflowsResponse = await client.listIssueWorkflows();
      const existing = workflows.find((w) => w.name === template.workflow!.name);
      const wanted = workflowSteps(template.workflow.steps, agentIds);
      if (!existing) {
        result.workflow = { ...result.workflow, status: "missing" };
      } else if (!wanted) {
        result.workflow = { ...result.workflow, status: "failed", detail: "a workflow step has no agent to hand to" };
      } else {
        const merged = mergeWorkflowSteps(existing, wanted);
        result.workflow = { status: "current", added: merged.added, updated: merged.updated, kept: merged.kept };
        if (merged.added.length + merged.updated.length > 0) {
          try {
            if (dry) await client.dryRunIssueWorkflowUpdate(existing.id, { steps: merged.steps });
            else await client.updateIssueWorkflow(existing.id, { steps: merged.steps });
            result.workflow.status = "upgraded";
          } catch (err) {
            result.workflow.status = "failed";
            result.workflow.detail =
              (err as { body?: { code?: string } } | null)?.body?.code === "status_create_forbidden"
                ? `a workspace admin must allow the new status (${merged.added.join(", ")})`
                : errorMessage(err);
          }
        } else if (merged.kept.length > 0) {
          result.workflow.status = "kept";
        }
      }
    } catch (err) {
      result.workflow = { ...result.workflow, status: "failed", detail: errorMessage(err) };
    }
  }
  return result;
}

export interface KitvietUpgradeClient extends SquadQuickstartClient {
  updateAgent(id: string, data: UpdateAgentRequest): Promise<Agent>;
}

export interface KitvietUpgradeResult {
  squad: { status: UpgradeStatus; detail?: string };
  skills: QuickstartOutcome["skills"];
  agents: { role: SquadRoleKey; name: string; status: UpgradeStatus; detail?: string }[];
}

/**
 * Brings a Kit Viet squad the quickstart made earlier up to the current text: the agents' and the
 * squad's instructions, and the skills of the squad and of its agents. The quickstart reuses an
 * agent that already has the template's name, so a rerun alone never changes its text. This does,
 * but only where the existing text still equals an earlier text of ours (kitviet-previous-texts.ts).
 * Anything the boss edited is kept and named in the result. The squad is touched only when it has
 * the template's leader, so a hand-made squad with the same name is left alone. Running it twice
 * changes nothing the second time. `previous` defaults to the registry; a test passes its own.
 */
export async function upgradeKitvietSquad(
  client: KitvietUpgradeClient,
  template: SquadTemplate,
  options: {
    dryRun?: boolean;
    previous?: { agents: Record<string, readonly string[]>; squad: readonly string[] };
  } = {},
): Promise<KitvietUpgradeResult> {
  const dry = options.dryRun === true;
  // The registry is about 2.6 MB of earlier texts, so it loads when an upgrade runs, not with the page.
  const previous: { agents: Record<string, readonly string[]>; squad: readonly string[] } =
    options.previous ??
    (await import("./kitviet-previous-texts").then((m) => ({
      agents: m.KITVIET_PREVIOUS_AGENT_INSTRUCTIONS,
      squad: m.KITVIET_PREVIOUS_SQUAD_INSTRUCTIONS,
    })));
  const result: KitvietUpgradeResult = { squad: { status: "missing" }, skills: { status: "skipped" }, agents: [] };

  const agents = (await client.listAgents({ include_archived: false })).filter((a) => !a.archived_at);
  const byName = new Map(agents.map((a) => [a.name, a] as const));
  const agentIds: Partial<Record<SquadRoleKey, string>> = {};

  for (const role of template.roles) {
    const { name } = SQUAD_ROLES[role];
    const agent = byName.get(name);
    if (!agent) {
      result.agents.push({ role, name, status: "missing" });
      continue;
    }
    agentIds[role] = agent.id;
    const wanted = await instructionsFor(role);
    const verdict = decide(agent.instructions ?? "", wanted, previous.agents[role] ?? []);
    if (verdict !== "replace") {
      result.agents.push({ role, name, status: verdict, ...(verdict === "kept" ? { detail: "instructions differ from every earlier text; kept" } : {}) });
      continue;
    }
    try {
      if (!dry) await client.updateAgent(agent.id, { instructions: wanted });
      result.agents.push({ role, name, status: "upgraded" });
    } catch (err) {
      result.agents.push({ role, name, status: "failed", detail: errorMessage(err) });
    }
  }

  const leaderId = agentIds[template.leader];
  const squads: Squad[] = await client.listSquads();
  const squad = squads.find((s) => !s.archived_at && s.name === template.name);
  if (!squad) return result;
  if (!leaderId || squad.leader_id !== leaderId) {
    result.squad = { status: "kept", detail: "led by another agent; not the quickstart's squad" };
    return result;
  }
  const verdict = decide(squad.instructions ?? "", template.instructions, previous.squad);
  if (verdict === "replace") {
    try {
      if (!dry) await client.updateSquad(squad.id, { instructions: template.instructions });
      result.squad = { status: "upgraded" };
    } catch (err) {
      result.squad = { status: "failed", detail: errorMessage(err) };
    }
  } else {
    result.squad = { status: verdict, ...(verdict === "kept" ? { detail: "instructions differ from every earlier text; kept" } : {}) };
  }

  if (!dry) {
    const pack = kitvietPackOf(template.id);
    result.skills = await attachBuiltinSkills(client, squad.id, template.builtinSkills, pack).catch(
      (err): QuickstartOutcome["skills"] => ({ status: "failed", error: errorMessage(err) }),
    );
    try {
      // Only the quickstart's own squad gives its leader and members the agent skills.
      const members: SquadMember[] = await client.listSquadMembers(squad.id);
      const eligible = new Set([leaderId, ...members.map((m) => m.member_id)]);
      const failures = await attachAgentSkills(client, template, agentIds, eligible, pack);
      if (failures.length > 0) result.skills = withAgentSkillFailures(result.skills, failures);
    } catch (err) {
      result.skills = { status: "failed", error: errorMessage(err) };
    }
  }
  return result;
}

export interface KitvietWorkspaceUpgradeEntry {
  templateId: string;
  name: string;
  result: KitvietUpgradeResult;
}

/**
 * Runs upgradeKitvietSquad for each Kit Viet template and returns the squads the workspace has.
 * A template whose squad is not in the workspace is skipped, so a workspace that never ran the
 * quickstart for a pack gets nothing from it. Options are the same as upgradeKitvietSquad's.
 */
export async function upgradeKitvietWorkspace(
  client: KitvietUpgradeClient,
  templates: readonly SquadTemplate[],
  options: Parameters<typeof upgradeKitvietSquad>[2] = {},
): Promise<KitvietWorkspaceUpgradeEntry[]> {
  const entries: KitvietWorkspaceUpgradeEntry[] = [];
  for (const template of templates) {
    const result = await upgradeKitvietSquad(client, template, options);
    if (result.squad.status === "missing") continue;
    entries.push({ templateId: template.id, name: template.name, result });
  }
  return entries;
}

/** Whether a squad with this name is one of the Kit Viet quickstart squads. */
export function hasKitvietSquad(squads: readonly { name: string; archived_at?: string | null }[]): boolean {
  const names = new Set(SQUAD_TEMPLATES.filter((tpl) => squadTemplateCategory(tpl) === "kitviet").map((tpl) => tpl.name));
  return squads.some((s) => !s.archived_at && names.has(s.name));
}
