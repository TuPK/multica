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
