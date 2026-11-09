"use client";

import { useMemo, useState, type ReactNode } from "react";
import { useQueries, useQuery } from "@tanstack/react-query";
import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from "recharts";
import { useWorkspaceId } from "@multica/core/hooks";
import type { Agent, AgentStatus, ProjectStatus, Squad } from "@multica/core/types";
import { projectListOptions } from "@multica/core/projects/queries";
import { budgetPace, budgetsOptions, type Budget, type BudgetState } from "@multica/core/governance";
import { dashboardUsageByAgentOptions, dashboardUsageDailyOptions } from "@multica/core/dashboard";
import { Progress } from "@multica/ui/components/ui/progress";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@multica/ui/components/ui/chart";
import { cn } from "@multica/ui/lib/utils";
import { useViewingTimezone } from "../../common/use-viewing-timezone";
import { DailyCostChart } from "../../runtimes/components/charts";
import { aggregateAgentTokens, aggregateDailyCost } from "../../dashboard/utils";
import { Segmented } from "../../dashboard/components/dashboard-shared";
import { useT } from "../../i18n";
import { UNSEATED_DEPARTMENT } from "../lib/company-selection";
import {
  budgetTotals,
  budgetUsed,
  microsToUsd,
  projectStats,
  rollUpCost,
} from "../lib/company-insights";

interface Department {
  id: string;
  name: string;
  squads: Squad[];
}

const RANGES = [7, 30, 90] as const;
type Range = (typeof RANGES)[number];

/** Projects whose own spend is fetched; each costs one request, so the busiest are enough for a ranking. */
const PROJECT_COST_LIMIT = 8;
const TOP_SQUADS = 8;
const TOP_AGENTS = 8;

const PROJECT_STATUS_BAR: Record<ProjectStatus, string> = {
  planned: "bg-chart-4",
  in_progress: "bg-chart-1",
  paused: "bg-chart-3",
  completed: "bg-success",
  cancelled: "bg-muted-foreground/40",
};

const AGENT_STATUS_ORDER: AgentStatus[] = ["working", "idle", "blocked", "error", "offline"];
const AGENT_STATUS_BAR: Record<AgentStatus, string> = {
  working: "bg-chart-1",
  idle: "bg-chart-4",
  blocked: "bg-warning",
  error: "bg-destructive",
  offline: "bg-muted-foreground/40",
};

const BUDGET_STATE_TEXT: Record<BudgetState, string> = {
  ok: "text-muted-foreground",
  soft: "text-warning",
  hard: "text-destructive",
  unknown: "text-muted-foreground",
};

function useLabels() {
  const { t } = useT("company");
  return {
    project: {
      planned: t(($) => $.insights_project_status_planned),
      in_progress: t(($) => $.insights_project_status_in_progress),
      paused: t(($) => $.insights_project_status_paused),
      completed: t(($) => $.insights_project_status_completed),
      cancelled: t(($) => $.insights_project_status_cancelled),
    } satisfies Record<ProjectStatus, string>,
    agent: {
      working: t(($) => $.insights_agent_status_working),
      idle: t(($) => $.insights_agent_status_idle),
      blocked: t(($) => $.insights_agent_status_blocked),
      error: t(($) => $.insights_agent_status_error),
      offline: t(($) => $.insights_agent_status_offline),
    } satisfies Record<AgentStatus, string>,
    scope: {
      company: t(($) => $.insights_budget_scope_company),
      floor: t(($) => $.insights_budget_scope_floor),
      squad: t(($) => $.insights_budget_scope_squad),
      initiative: t(($) => $.insights_budget_scope_initiative),
    } satisfies Record<Budget["scope_type"], string>,
    period: {
      week: t(($) => $.insights_budget_period_week),
      month: t(($) => $.insights_budget_period_month),
      quarter: t(($) => $.insights_budget_period_quarter),
    } satisfies Record<Budget["period"], string>,
    state: {
      ok: t(($) => $.insights_budget_state_ok),
      soft: t(($) => $.insights_budget_state_soft),
      hard: t(($) => $.insights_budget_state_hard),
      unknown: t(($) => $.insights_budget_state_unknown),
    } satisfies Record<BudgetState, string>,
  };
}

const costChartConfig = { cost: { label: "USD", color: "var(--chart-1)" } } satisfies ChartConfig;

const usd = new Intl.NumberFormat(undefined, { style: "currency", currency: "USD", maximumFractionDigits: 2 });
const usdWhole = new Intl.NumberFormat(undefined, { style: "currency", currency: "USD", maximumFractionDigits: 0 });
const formatUsd = (value: number) => (Math.abs(value) >= 1000 ? usdWhole : usd).format(value);
const percent = (share: number) => (Number.isFinite(share) ? `${Math.round(share * 100)}%` : "∞");

/**
 * The company-wide picture for the person running it: how projects stand, what
 * the agents have cost and where, and how the budgets are holding. Everything is
 * read from the dashboard, project and budget queries the app already has, so no
 * figure here is a second source of truth.
 */
export function CompanyInsights({
  departments,
  squads,
  agents,
}: {
  departments: Department[];
  squads: Squad[];
  agents: Agent[];
}) {
  const { t } = useT("company");
  const wsId = useWorkspaceId();
  const tz = useViewingTimezone();
  const labels = useLabels();
  const [days, setDays] = useState<Range>(30);

  const { data: projects = [], isPending: projectsPending } = useQuery(projectListOptions(wsId));
  const { data: budgetList } = useQuery(budgetsOptions(wsId));
  const { data: daily = [] } = useQuery(dashboardUsageDailyOptions(wsId, days, null, tz));
  const { data: byAgent = [], isPending: costPending } = useQuery(dashboardUsageByAgentOptions(wsId, days, null, tz));

  const budgets = useMemo(() => budgetList?.budgets ?? [], [budgetList]);
  const now = useMemo(() => new Date(), []);

  const projectSummary = useMemo(() => projectStats(projects, now), [projects, now]);
  const costProjects = useMemo(
    () =>
      projects
        .filter((project) => project.status === "in_progress")
        .toSorted((a, b) => b.updated_at.localeCompare(a.updated_at))
        .slice(0, PROJECT_COST_LIMIT),
    [projects],
  );
  const projectCosts = useQueries({
    queries: costProjects.map((project) => dashboardUsageByAgentOptions(wsId, days, project.id, tz)),
  });
  const projectCostLines = useMemo(
    () =>
      costProjects
        .map((project, index) => ({
          id: project.id,
          name: project.title,
          cost: aggregateAgentTokens(projectCosts[index]?.data ?? []).reduce((sum, row) => sum + row.cost, 0),
        }))
        .filter((line) => line.cost > 0)
        .toSorted((a, b) => b.cost - a.cost),
    [costProjects, projectCosts],
  );

  const rollup = useMemo(
    () =>
      rollUpCost({
        usage: byAgent,
        agents,
        squads,
        departments: departments.map((d) => ({ id: d.id, name: d.name })),
        unseatedName: t(($) => $.unseated),
      }),
    [byAgent, agents, squads, departments, t],
  );
  const dailyCost = useMemo(() => aggregateDailyCost(daily), [daily]);
  const attributed = rollup.byDepartment.reduce((sum, line) => sum + line.cost, 0);
  const unattributed = Math.max(0, rollup.total - attributed);

  const totals = useMemo(() => budgetTotals(budgets), [budgets]);
  const companyBudgetUsed = totals.companyLimit > 0 ? totals.companySpent / totals.companyLimit : null;

  const agentStatusCounts = useMemo(() => {
    const counts = new Map<AgentStatus, number>();
    for (const agent of agents) counts.set(agent.status, (counts.get(agent.status) ?? 0) + 1);
    return AGENT_STATUS_ORDER.map((status) => ({ status, count: counts.get(status) ?? 0 }));
  }, [agents]);

  const scopeName = useMemo(() => {
    const names = new Map<string, string>();
    for (const d of departments) names.set(d.id, d.name);
    for (const squad of squads) names.set(squad.id, squad.name);
    for (const project of projects) names.set(project.id, project.title);
    return names;
  }, [departments, squads, projects]);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between gap-2">
        <h2 className="text-sm font-medium">{t(($) => $.insights_title)}</h2>
        <Segmented
          label={t(($) => $.insights_range)}
          value={days}
          onChange={setDays}
          options={RANGES.map((range) => ({ value: range, label: t(($) => $.insights_days, { days: range }) }))}
        />
      </div>

      <dl className="grid grid-cols-2 gap-3 lg:grid-cols-3 xl:grid-cols-6">
        <Kpi label={t(($) => $.departments)} value={departments.filter((d) => d.id !== UNSEATED_DEPARTMENT).length} />
        <Kpi label={t(($) => $.squads)} value={squads.length} />
        <Kpi label={t(($) => $.agents)} value={agents.length} />
        <Kpi
          label={t(($) => $.insights_kpi_projects)}
          value={projectSummary.total}
          hint={t(($) => $.insights_kpi_projects_active, {
            active: projectSummary.byStatus.find((s) => s.status === "in_progress")?.count ?? 0,
          })}
        />
        <Kpi
          label={t(($) => $.insights_kpi_spend, { days })}
          value={costPending ? "…" : formatUsd(rollup.total)}
        />
        <Kpi
          label={t(($) => $.insights_kpi_budget)}
          value={companyBudgetUsed === null ? "—" : percent(companyBudgetUsed)}
          hint={
            companyBudgetUsed === null
              ? t(($) => $.insights_kpi_budget_none)
              : `${formatUsd(microsToUsd(totals.companySpent))} / ${formatUsd(microsToUsd(totals.companyLimit))}`
          }
          tone={totals.overLimit > 0 ? "danger" : totals.atRisk > 0 ? "warning" : undefined}
        />
      </dl>

      <div className="grid gap-4 xl:grid-cols-2">
        <Card title={t(($) => $.insights_cost_daily)} className="xl:col-span-2">
          {dailyCost.length === 0 ? (
            <Empty>{t(($) => $.insights_cost_empty)}</Empty>
          ) : (
            <DailyCostChart data={dailyCost} />
          )}
        </Card>

        <Card title={t(($) => $.insights_cost_departments)}>
          {rollup.total === 0 ? (
            <Empty>{t(($) => $.insights_cost_empty)}</Empty>
          ) : (
            <CostBars lines={rollup.byDepartment.filter((line) => line.cost > 0)} />
          )}
          {unattributed > 0.005 ? (
            <p className="mt-2 text-xs text-muted-foreground">
              {t(($) => $.insights_cost_unattributed, { amount: formatUsd(unattributed) })}
            </p>
          ) : null}
        </Card>

        <Card title={t(($) => $.insights_cost_projects)}>
          {projectCostLines.length === 0 ? (
            <Empty>{t(($) => $.insights_cost_empty)}</Empty>
          ) : (
            <CostBars lines={projectCostLines} />
          )}
        </Card>

        <Card title={t(($) => $.insights_cost_squads)}>
          {rollup.total === 0 ? (
            <Empty>{t(($) => $.insights_cost_empty)}</Empty>
          ) : (
            <CostTable
              lines={rollup.bySquad.filter((line) => line.cost > 0).slice(0, TOP_SQUADS)}
              total={rollup.total}
              detail={(line) => line.departmentName}
            />
          )}
        </Card>

        <Card title={t(($) => $.insights_cost_agents)}>
          {rollup.byAgent.length === 0 ? (
            <Empty>{t(($) => $.insights_cost_empty)}</Empty>
          ) : (
            <CostTable lines={rollup.byAgent.slice(0, TOP_AGENTS)} total={rollup.total} />
          )}
        </Card>

        <Card title={t(($) => $.insights_projects)}>
          {projectsPending ? (
            <Skeleton className="h-24 w-full" />
          ) : projectSummary.total === 0 ? (
            <Empty>{t(($) => $.insights_projects_empty)}</Empty>
          ) : (
            <div className="flex flex-col gap-4">
              <StackedBar
                segments={projectSummary.byStatus.map(({ status, count }) => ({
                  key: status,
                  label: labels.project[status],
                  count,
                  className: PROJECT_STATUS_BAR[status],
                }))}
              />
              <div className="flex flex-col gap-1.5">
                <div className="flex items-baseline justify-between text-sm">
                  <span>{t(($) => $.insights_projects_completion)}</span>
                  <span className="tabular-nums text-muted-foreground">
                    {t(($) => $.insights_projects_done_of, {
                      done: projectSummary.done,
                      total: projectSummary.issues,
                    })}
                  </span>
                </div>
                <Progress value={projectSummary.completion * 100} />
                {projectSummary.overdue > 0 ? (
                  <p className="text-xs text-destructive">
                    {t(($) => $.insights_projects_overdue, { overdue: projectSummary.overdue })}
                  </p>
                ) : null}
              </div>
            </div>
          )}
        </Card>

        <Card title={t(($) => $.insights_agents)}>
          {agents.length === 0 ? (
            <Empty>{t(($) => $.insights_agents_empty)}</Empty>
          ) : (
            <StackedBar
              segments={agentStatusCounts.map(({ status, count }) => ({
                key: status,
                label: labels.agent[status],
                count,
                className: AGENT_STATUS_BAR[status],
              }))}
            />
          )}
        </Card>

        <Card title={t(($) => $.insights_budgets)} className="xl:col-span-2">
          {budgets.length === 0 ? (
            <Empty>{t(($) => $.insights_budgets_empty)}</Empty>
          ) : (
            <ul className="grid gap-4 md:grid-cols-2">
              {budgets.map((budget) => (
                <BudgetRow
                  key={budget.id}
                  budget={budget}
                  name={
                    budget.scope_type === "company"
                      ? labels.scope.company
                      : (scopeName.get(budget.scope_id ?? "") ?? labels.scope[budget.scope_type])
                  }
                  now={now}
                />
              ))}
            </ul>
          )}
        </Card>
      </div>
    </div>
  );
}

function Kpi({
  label,
  value,
  hint,
  tone,
}: {
  label: string;
  value: ReactNode;
  hint?: string;
  tone?: "warning" | "danger";
}) {
  return (
    <div className="flex min-w-0 flex-col gap-1 rounded-lg border p-4">
      <dt className="truncate text-xs text-muted-foreground">{label}</dt>
      <dd
        className={cn(
          "truncate text-2xl font-semibold tabular-nums",
          tone === "warning" && "text-warning",
          tone === "danger" && "text-destructive",
        )}
      >
        {value}
      </dd>
      {hint ? <p className="truncate text-xs text-muted-foreground">{hint}</p> : null}
    </div>
  );
}

function Card({ title, className, children }: { title: string; className?: string; children: ReactNode }) {
  return (
    <section className={cn("flex min-w-0 flex-col gap-3 rounded-lg border p-4", className)}>
      <h3 className="text-sm font-medium">{title}</h3>
      {children}
    </section>
  );
}

function Empty({ children }: { children: ReactNode }) {
  return <p className="py-6 text-center text-sm text-muted-foreground">{children}</p>;
}

function CostBars({ lines }: { lines: { id: string; name: string; cost: number }[] }) {
  return (
    <ChartContainer config={costChartConfig} className="w-full" style={{ height: Math.max(80, lines.length * 36 + 16) }}>
      <BarChart data={lines} layout="vertical" margin={{ left: 0, right: 8, top: 0, bottom: 0 }}>
        <CartesianGrid horizontal={false} />
        <XAxis type="number" tickLine={false} axisLine={false} tickFormatter={(v: number) => formatUsd(v)} />
        <YAxis dataKey="name" type="category" tickLine={false} axisLine={false} width={96} />
        <ChartTooltip
          content={<ChartTooltipContent formatter={(value) => (typeof value === "number" ? formatUsd(value) : String(value))} />}
        />
        <Bar dataKey="cost" fill="var(--color-cost)" radius={4} />
      </BarChart>
    </ChartContainer>
  );
}

function CostTable<T extends { id: string; name: string; cost: number }>({
  lines,
  total,
  detail,
}: {
  lines: T[];
  total: number;
  detail?: (line: T) => string;
}) {
  return (
    <ul className="flex flex-col gap-2">
      {lines.map((line) => (
        <li key={line.id} className="flex flex-col gap-1">
          <div className="flex items-baseline justify-between gap-2 text-sm">
            <span className="min-w-0 truncate">
              {line.name}
              {detail ? <span className="ml-2 text-xs text-muted-foreground">{detail(line)}</span> : null}
            </span>
            <span className="shrink-0 tabular-nums">{formatUsd(line.cost)}</span>
          </div>
          <Progress value={total > 0 ? (line.cost / total) * 100 : 0} />
        </li>
      ))}
    </ul>
  );
}

function StackedBar({ segments }: { segments: { key: string; label: string; count: number; className: string }[] }) {
  const total = segments.reduce((sum, s) => sum + s.count, 0);
  return (
    <div className="flex flex-col gap-3">
      <div className="flex h-2.5 w-full overflow-hidden rounded-full bg-muted">
        {segments
          .filter((s) => s.count > 0)
          .map((s) => (
            <div key={s.key} className={s.className} style={{ width: `${(s.count / total) * 100}%` }} />
          ))}
      </div>
      <ul className="flex flex-wrap gap-x-4 gap-y-1 text-sm">
        {segments.map((s) => (
          <li key={s.key} className="flex items-center gap-1.5">
            <span className={cn("size-2 rounded-full", s.className)} />
            <span className="text-muted-foreground">{s.label}</span>
            <span className="tabular-nums">{s.count}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

function BudgetRow({ budget, name, now }: { budget: Budget; name: string; now: Date }) {
  const { t } = useT("company");
  const labels = useLabels();
  const used = budgetUsed(budget);
  const pace = budgetPace(budget, now);
  return (
    <li className="flex flex-col gap-1.5">
      <div className="flex items-baseline justify-between gap-2 text-sm">
        <span className="min-w-0 truncate">
          {name}
          <span className="ml-2 text-xs text-muted-foreground">
            {labels.period[budget.period]}
          </span>
        </span>
        <span className={cn("shrink-0 text-xs", BUDGET_STATE_TEXT[budget.state])}>
          {budget.status === "paused"
            ? t(($) => $.insights_budget_paused)
            : labels.state[budget.state]}
        </span>
      </div>
      <Progress value={Math.min(100, used * 100)} />
      <div className="flex items-baseline justify-between gap-2 text-xs text-muted-foreground">
        <span className="tabular-nums">
          {t(($) => $.insights_budget_spent_of, {
            spent: formatUsd(microsToUsd(budget.spent_micros)),
            limit: formatUsd(microsToUsd(budget.limit_micros)),
          })}
        </span>
        {pace.overrunAt && budget.status === "active" ? (
          <span className="text-warning">
            {t(($) => $.insights_budget_overrun, { date: pace.overrunAt.toLocaleDateString() })}
          </span>
        ) : null}
      </div>
    </li>
  );
}
