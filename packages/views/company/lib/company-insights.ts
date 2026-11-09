import type { Agent, DashboardUsageByAgent, Project, ProjectStatus, Squad } from "@multica/core/types";
import type { Budget } from "@multica/core/governance";
import { PROJECT_STATUS_ORDER } from "@multica/core/projects/config";
import { aggregateAgentTokens } from "../../dashboard/utils";
import { UNSEATED_DEPARTMENT } from "./company-selection";

export interface CostLine {
  id: string;
  name: string;
  cost: number;
}

export interface CostRollup {
  total: number;
  byDepartment: CostLine[];
  bySquad: (CostLine & { departmentId: string; departmentName: string })[];
  byAgent: CostLine[];
}

interface DepartmentRef {
  id: string;
  name: string;
}

/** Spend by agent folded up to its home squad and that squad's department. Cost with no home squad stays in the total and the agent list only. */
export function rollUpCost(input: {
  usage: DashboardUsageByAgent[];
  agents: Pick<Agent, "id" | "name" | "home_squad_id">[];
  squads: Pick<Squad, "id" | "name" | "floor_id">[];
  departments: DepartmentRef[];
  unseatedName: string;
}): CostRollup {
  const { usage, agents, squads, departments, unseatedName } = input;
  const agentById = new Map(agents.map((agent) => [agent.id, agent]));
  const squadById = new Map(squads.map((squad) => [squad.id, squad]));
  const departmentName = new Map(departments.map((d) => [d.id, d.name]));

  const squadCost = new Map<string, number>();
  const departmentCost = new Map<string, number>();
  const byAgent: CostLine[] = [];
  let total = 0;

  for (const row of aggregateAgentTokens(usage)) {
    total += row.cost;
    const agent = agentById.get(row.agentId);
    if (!agent) continue;
    byAgent.push({ id: agent.id, name: agent.name, cost: row.cost });
    const squad = agent.home_squad_id ? squadById.get(agent.home_squad_id) : undefined;
    if (!squad) continue;
    squadCost.set(squad.id, (squadCost.get(squad.id) ?? 0) + row.cost);
    const departmentId = squad.floor_id ?? UNSEATED_DEPARTMENT;
    departmentCost.set(departmentId, (departmentCost.get(departmentId) ?? 0) + row.cost);
  }

  const nameOf = (id: string) => (id === UNSEATED_DEPARTMENT ? unseatedName : (departmentName.get(id) ?? unseatedName));
  const desc = (a: CostLine, b: CostLine) => b.cost - a.cost;

  return {
    total,
    byDepartment: departments
      .map((d) => ({ id: d.id, name: d.name, cost: departmentCost.get(d.id) ?? 0 }))
      .toSorted(desc),
    bySquad: squads
      .map((squad) => {
        const departmentId = squad.floor_id ?? UNSEATED_DEPARTMENT;
        return {
          id: squad.id,
          name: squad.name,
          cost: squadCost.get(squad.id) ?? 0,
          departmentId,
          departmentName: nameOf(departmentId),
        };
      })
      .toSorted(desc),
    byAgent: byAgent.toSorted(desc),
  };
}

export interface ProjectStats {
  total: number;
  byStatus: { status: ProjectStatus; count: number }[];
  issues: number;
  done: number;
  /** 0 to 1; 0 when no project has issues. */
  completion: number;
  overdue: number;
}

export function projectStats(projects: Project[], now: Date): ProjectStats {
  const counts = new Map<ProjectStatus, number>();
  let issues = 0;
  let done = 0;
  let overdue = 0;
  const today = now.toISOString().slice(0, 10);
  for (const project of projects) {
    counts.set(project.status, (counts.get(project.status) ?? 0) + 1);
    issues += project.issue_count;
    done += project.done_count;
    const open = project.status === "planned" || project.status === "in_progress";
    if (open && project.due_date && project.due_date < today) overdue += 1;
  }
  return {
    total: projects.length,
    byStatus: PROJECT_STATUS_ORDER.map((status) => ({ status, count: counts.get(status) ?? 0 })),
    issues,
    done,
    completion: issues > 0 ? done / issues : 0,
    overdue,
  };
}

export interface BudgetTotals {
  /** Active budgets' limits and spend, summed per scope type so a company cap is not double counted with its squads'. */
  companyLimit: number;
  companySpent: number;
  atRisk: number;
  overLimit: number;
}

const MICROS_PER_USD = 1_000_000;

export function microsToUsd(micros: number): number {
  return micros / MICROS_PER_USD;
}

export function budgetTotals(budgets: Budget[]): BudgetTotals {
  const active = budgets.filter((b) => b.status === "active");
  const company = active.filter((b) => b.scope_type === "company");
  return {
    companyLimit: company.reduce((sum, b) => sum + b.limit_micros, 0),
    companySpent: company.reduce((sum, b) => sum + b.spent_micros, 0),
    atRisk: active.filter((b) => b.state === "soft").length,
    overLimit: active.filter((b) => b.state === "hard").length,
  };
}

/** Share of the limit already spent; 0 for a zero limit with no spend. */
export function budgetUsed(budget: Pick<Budget, "limit_micros" | "spent_micros">): number {
  if (budget.limit_micros > 0) return budget.spent_micros / budget.limit_micros;
  return budget.spent_micros > 0 ? Number.POSITIVE_INFINITY : 0;
}

/** The part of the company a report covers: all of it, or one department's squads. */
export interface InsightScope {
  /** Null for the whole company. */
  departmentId: string | null;
  squadIds: ReadonlySet<string>;
}

export function inScope(scope: InsightScope, squadId: string | null | undefined): boolean {
  return scope.departmentId === null ? true : !!squadId && scope.squadIds.has(squadId);
}

export interface WorkHealth {
  done: number;
  inProgress: number;
  blocked: number;
  stale: number;
  failedRuns: number;
  pendingProposals: number;
  rows: {
    id: string;
    name: string;
    done: number;
    inProgress: number;
    blocked: number;
    stale: number;
    failedRuns: number;
    pendingProposals: number;
  }[];
  quiet: number;
}

interface DigestSquadLike {
  id: string;
  name: string;
  done: number;
  in_progress: number;
  blocked: number;
  stale: number;
  failed_runs: number;
  pending_proposal_count: number;
}

/** Issue flow of the squads in scope, from the digest's per-squad counters. Squads with no news are the quiet ones. */
export function workHealth(
  squads: DigestSquadLike[],
  quiet: { id: string }[],
  scope: InsightScope,
  quietCount: number,
): WorkHealth {
  const rows = squads
    .filter((s) => inScope(scope, s.id))
    .map((s) => ({
      id: s.id,
      name: s.name,
      done: s.done,
      inProgress: s.in_progress,
      blocked: s.blocked,
      stale: s.stale,
      failedRuns: s.failed_runs,
      pendingProposals: s.pending_proposal_count,
    }))
    .toSorted((a, b) => b.blocked + b.stale + b.failedRuns - (a.blocked + a.stale + a.failedRuns) || b.done - a.done);
  const sum = (pick: (r: (typeof rows)[number]) => number) => rows.reduce((n, r) => n + pick(r), 0);
  return {
    done: sum((r) => r.done),
    inProgress: sum((r) => r.inProgress),
    blocked: sum((r) => r.blocked),
    stale: sum((r) => r.stale),
    failedRuns: sum((r) => r.failedRuns),
    pendingProposals: sum((r) => r.pendingProposals),
    rows,
    quiet: scope.departmentId === null ? quietCount : quiet.filter((q) => inScope(scope, q.id)).length,
  };
}

export interface RunStats {
  runs: number;
  failed: number;
  cancelled: number;
  seconds: number;
  /** 0 to 1 over finished runs; null with no runs. */
  successRate: number | null;
}

interface RunTimeRowLike {
  agent_id: string;
  total_seconds: number;
  task_count: number;
  failed_count: number;
  cancelled_count: number;
}

export function runStats(rows: RunTimeRowLike[], agentIds: ReadonlySet<string> | null): RunStats {
  let runs = 0;
  let failed = 0;
  let cancelled = 0;
  let seconds = 0;
  for (const row of rows) {
    if (agentIds && !agentIds.has(row.agent_id)) continue;
    runs += row.task_count;
    failed += row.failed_count;
    cancelled += row.cancelled_count;
    seconds += row.total_seconds;
  }
  return { runs, failed, cancelled, seconds, successRate: runs > 0 ? (runs - failed - cancelled) / runs : null };
}

/** Run totals per squad, folded from agent rows through each agent's home squad. */
export function runStatsBySquad(
  rows: RunTimeRowLike[],
  agents: { id: string; home_squad_id?: string | null }[],
  squads: { id: string; name: string }[],
): (RunStats & { id: string; name: string })[] {
  const home = new Map(agents.map((a) => [a.id, a.home_squad_id ?? null]));
  const bySquad = new Map<string, RunTimeRowLike[]>();
  for (const row of rows) {
    const squadId = home.get(row.agent_id);
    if (!squadId) continue;
    bySquad.set(squadId, [...(bySquad.get(squadId) ?? []), row]);
  }
  return squads
    .map((s) => ({ id: s.id, name: s.name, ...runStats(bySquad.get(s.id) ?? [], null) }))
    .filter((s) => s.runs > 0)
    .toSorted((a, b) => b.runs - a.runs);
}

/** Budgets that belong to the scope: the company's own, or those of the department, its squads and its projects. */
export function budgetsInScope(
  budgets: Budget[],
  scope: InsightScope,
  projectIds: ReadonlySet<string>,
): Budget[] {
  if (scope.departmentId === null) return budgets;
  return budgets.filter((b) => {
    if (b.scope_type === "company") return false;
    if (b.scope_type === "floor") return b.scope_id === scope.departmentId;
    if (b.scope_type === "squad") return inScope(scope, b.scope_id);
    return !!b.scope_id && projectIds.has(b.scope_id);
  });
}

export interface PipelineSummary {
  total: number;
  stalled: number;
  byStage: { key: string; name: string; count: number; stalled: number }[];
}

export function pipelineSummary(
  items: { owner_squad_id: string | null; stage_key: string; stage_name: string; stalled: boolean }[],
  scope: InsightScope,
): PipelineSummary {
  const stages = new Map<string, { key: string; name: string; count: number; stalled: number }>();
  let total = 0;
  let stalled = 0;
  for (const item of items) {
    if (scope.departmentId !== null && !inScope(scope, item.owner_squad_id)) continue;
    total += 1;
    if (item.stalled) stalled += 1;
    const stage = stages.get(item.stage_key) ?? { key: item.stage_key, name: item.stage_name, count: 0, stalled: 0 };
    stage.count += 1;
    if (item.stalled) stage.stalled += 1;
    stages.set(item.stage_key, stage);
  }
  return { total, stalled, byStage: [...stages.values()] };
}
