"use client";

import { useMemo, useState } from "react";
import { useQueries, useQuery } from "@tanstack/react-query";
import { useWorkspaceId } from "@multica/core/hooks";
import type { Agent, AgentStatus, ProjectStatus, Squad } from "@multica/core/types";
import { projectListOptions } from "@multica/core/projects/queries";
import { budgetPace, budgetsOptions, type Budget, type BudgetState } from "@multica/core/governance";
import { digestOptions } from "@multica/core/company";
import {
  dashboardAgentRunTimeOptions,
  dashboardFailuresDailyOptions,
  dashboardRunTimeDailyOptions,
  dashboardUsageByAgentOptions,
  dashboardUsageDailyOptions,
} from "@multica/core/dashboard";
import { Progress } from "@multica/ui/components/ui/progress";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import { cn } from "@multica/ui/lib/utils";
import { useViewingTimezone } from "../../common/use-viewing-timezone";
import { DailyCostChart, DailyErrorsChart, DailyTasksChart } from "../../runtimes/components/charts";
import {
  aggregateAgentTokens,
  aggregateDailyCost,
  aggregateDailyErrors,
  aggregateDailyTasks,
  formatDuration,
} from "../../dashboard/utils";
import { Segmented } from "../../dashboard/components/dashboard-shared";
import { useT } from "../../i18n";
import { UNSEATED_DEPARTMENT } from "../lib/company-selection";
import {
  budgetTotals,
  budgetUsed,
  budgetsInScope,
  microsToUsd,
  pipelineSummary,
  projectStats,
  rollUpCost,
  runStats,
  runStatsBySquad,
  workHealth,
  inScope,
  type InsightScope,
} from "../lib/company-insights";
import { Card, CostBars, CostTable, Empty, Kpi, StackedBar, formatUsd, percent } from "./company-insights-parts";

interface Department {
  id: string;
  name: string;
  squads: Squad[];
}

const RANGES = [7, 30, 90] as const;
type Range = (typeof RANGES)[number];

/** The digest looks back at most 35 days, so its work-health figures cap there. */
const DIGEST_MAX_DAYS = 35;
/** Projects whose own spend is fetched; each costs one request, so the busiest are enough for a ranking. */
const PROJECT_COST_LIMIT = 8;
const TOP_ROWS = 8;

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

const CONFIDENCE_TEXT: Record<string, string> = {
  on_track: "text-success",
  at_risk: "text-warning",
  off_track: "text-destructive",
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
    confidence: {
      on_track: t(($) => $.insights_goal_on_track),
      at_risk: t(($) => $.insights_goal_at_risk),
      off_track: t(($) => $.insights_goal_off_track),
    } as Record<string, string>,
  };
}

/**
 * The picture for the person running the company, or for one department when
 * `department` is given: how projects and issues stand, what the agents have
 * cost and where, how reliable their runs are, and how the budgets are holding.
 * Everything is read from the dashboard, project, digest and budget queries the
 * app already has, so no figure here is a second source of truth. Series the
 * server only keeps company-wide (daily charts) are shown at company level only.
 */
export function CompanyInsights({
  departments,
  squads,
  agents,
  department,
}: {
  departments: Department[];
  squads: Squad[];
  agents: Agent[];
  department: Department | null;
}) {
  const { t } = useT("company");
  const wsId = useWorkspaceId();
  const tz = useViewingTimezone();
  const labels = useLabels();
  const [days, setDays] = useState<Range>(30);
  const now = useMemo(() => new Date(), []);

  const scope = useMemo<InsightScope>(
    () => ({
      departmentId: department?.id ?? null,
      squadIds: new Set((department ? department.squads : squads).map((s) => s.id)),
    }),
    [department, squads],
  );
  const scopedSquads = department ? department.squads : squads;
  const scopedAgents = useMemo(
    () => (department ? agents.filter((a) => inScope(scope, a.home_squad_id)) : agents),
    [department, agents, scope],
  );
  const agentIds = useMemo(() => new Set(scopedAgents.map((a) => a.id)), [scopedAgents]);

  const digestDays = Math.min(days, DIGEST_MAX_DAYS);
  const since = useMemo(() => new Date(now.getTime() - digestDays * 86_400_000).toISOString(), [now, digestDays]);

  const { data: allProjects = [], isPending: projectsPending } = useQuery(projectListOptions(wsId));
  const { data: budgetList } = useQuery(budgetsOptions(wsId));
  const { data: digest } = useQuery(digestOptions(wsId, { since }));
  const { data: byAgent = [], isPending: costPending } = useQuery(dashboardUsageByAgentOptions(wsId, days, null, tz));
  const { data: runTime = [] } = useQuery(dashboardAgentRunTimeOptions(wsId, days, null, tz));
  const companyWide = department === null;
  const { data: daily = [] } = useQuery({
    ...dashboardUsageDailyOptions(wsId, days, null, tz),
    enabled: companyWide && !!wsId,
  });
  const { data: runTimeDaily = [] } = useQuery({
    ...dashboardRunTimeDailyOptions(wsId, days, null, tz),
    enabled: companyWide && !!wsId,
  });
  const { data: failuresDaily = [] } = useQuery({
    ...dashboardFailuresDailyOptions(wsId, days, null, tz),
    enabled: companyWide && !!wsId,
  });

  const projects = useMemo(
    () => (department ? allProjects.filter((p) => inScope(scope, p.squad_id)) : allProjects),
    [department, allProjects, scope],
  );
  const projectIds = useMemo(() => new Set(projects.map((p) => p.id)), [projects]);
  const budgets = useMemo(() => budgetsInScope(budgetList?.budgets ?? [], scope, projectIds), [budgetList, scope, projectIds]);

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
        usage: department ? byAgent.filter((row) => agentIds.has(row.agent_id)) : byAgent,
        agents,
        squads,
        departments: departments.map((d) => ({ id: d.id, name: d.name })),
        unseatedName: t(($) => $.unseated),
      }),
    [byAgent, agentIds, department, agents, squads, departments, t],
  );
  const dailyCost = useMemo(() => aggregateDailyCost(daily), [daily]);
  const dailyTasks = useMemo(() => aggregateDailyTasks(runTimeDaily), [runTimeDaily]);
  const dailyErrors = useMemo(() => aggregateDailyErrors(failuresDaily), [failuresDaily]);
  const attributed = rollup.byDepartment.reduce((sum, line) => sum + line.cost, 0);
  const unattributed = department ? 0 : Math.max(0, rollup.total - attributed);

  const runs = useMemo(() => runStats(runTime, department ? agentIds : null), [runTime, department, agentIds]);
  const squadRuns = useMemo(
    () => runStatsBySquad(runTime, agents, scopedSquads),
    [runTime, agents, scopedSquads],
  );
  const squadCostById = useMemo(() => new Map(rollup.bySquad.map((s) => [s.id, s.cost])), [rollup]);
  const costPerRun = runs.runs > 0 ? rollup.total / runs.runs : null;

  const health = useMemo(
    () => workHealth(digest?.squads ?? [], digest?.quiet.squads ?? [], scopedSquads, scope),
    [digest, scopedSquads, scope],
  );
  const pipeline = useMemo(() => pipelineSummary(digest?.pipeline ?? [], scope), [digest, scope]);
  const goals = useMemo(
    () =>
      (digest?.goals ?? []).filter((goal) => {
        if (goal.scope_type === "company") return department === null;
        if (goal.scope_type === "floor") return department === null || goal.scope_id === department.id;
        if (goal.scope_type === "squad") return inScope(scope, goal.scope_id);
        return department === null;
      }),
    [digest, department, scope],
  );
  const overdue = useMemo(
    () => (digest?.overdue_requests ?? []).filter((r) => inScope(scope, r.squad_id)),
    [digest, scope],
  );

  const totals = useMemo(() => budgetTotals(budgets), [budgets]);
  const budgetLimit = department
    ? budgets.reduce((sum, b) => sum + (b.status === "active" ? b.limit_micros : 0), 0)
    : totals.companyLimit;
  const budgetSpent = department
    ? budgets.reduce((sum, b) => sum + (b.status === "active" ? b.spent_micros : 0), 0)
    : totals.companySpent;
  const budgetShare = budgetLimit > 0 ? budgetSpent / budgetLimit : null;

  const agentStatusCounts = useMemo(() => {
    const counts = new Map<AgentStatus, number>();
    for (const agent of scopedAgents) counts.set(agent.status, (counts.get(agent.status) ?? 0) + 1);
    return AGENT_STATUS_ORDER.map((status) => ({ status, count: counts.get(status) ?? 0 }));
  }, [scopedAgents]);

  const scopeName = useMemo(() => {
    const names = new Map<string, string>();
    for (const d of departments) names.set(d.id, d.name);
    for (const squad of squads) names.set(squad.id, squad.name);
    for (const project of allProjects) names.set(project.id, project.title);
    return names;
  }, [departments, squads, allProjects]);

  const duration = (seconds: number) => formatDuration(seconds, t(($) => $.insights_under_a_minute));
  const successShare = (rate: number | null) => (rate === null ? "—" : percent(rate));

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between gap-2">
        <h2 className="truncate text-sm font-medium">
          {department ? department.name : t(($) => $.insights_title)}
        </h2>
        <Segmented
          label={t(($) => $.insights_range)}
          value={days}
          onChange={setDays}
          options={RANGES.map((range) => ({ value: range, label: t(($) => $.insights_days, { days: range }) }))}
        />
      </div>

      <dl className="grid grid-cols-2 gap-3 lg:grid-cols-3 xl:grid-cols-6">
        {department ? null : (
          <Kpi label={t(($) => $.departments)} value={departments.filter((d) => d.id !== UNSEATED_DEPARTMENT).length} />
        )}
        <Kpi label={t(($) => $.squads)} value={scopedSquads.length} />
        <Kpi label={t(($) => $.agents)} value={scopedAgents.length} />
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
          hint={costPerRun === null ? undefined : t(($) => $.insights_kpi_cost_per_run, { amount: formatUsd(costPerRun) })}
        />
        <Kpi
          label={department ? t(($) => $.insights_kpi_budget_department) : t(($) => $.insights_kpi_budget)}
          value={budgetShare === null ? "—" : percent(budgetShare)}
          hint={
            budgetShare === null
              ? t(($) => $.insights_kpi_budget_none)
              : `${formatUsd(microsToUsd(budgetSpent))} / ${formatUsd(microsToUsd(budgetLimit))}`
          }
          tone={
            budgets.some((b) => b.state === "hard") ? "danger" : budgets.some((b) => b.state === "soft") ? "warning" : undefined
          }
        />
      </dl>

      <dl className="grid grid-cols-2 gap-3 lg:grid-cols-3 xl:grid-cols-6">
        <Kpi label={t(($) => $.insights_kpi_done, { days: digestDays })} value={health.done} />
        <Kpi label={t(($) => $.insights_kpi_in_progress)} value={health.inProgress} />
        <Kpi
          label={t(($) => $.insights_kpi_blocked)}
          value={health.blocked}
          tone={health.blocked > 0 ? "warning" : undefined}
        />
        <Kpi
          label={t(($) => $.insights_kpi_stale)}
          value={health.stale}
          tone={health.stale > 0 ? "warning" : undefined}
        />
        <Kpi
          label={t(($) => $.insights_kpi_success, { days })}
          value={successShare(runs.successRate)}
          hint={t(($) => $.insights_kpi_runs, { runs: runs.runs, time: duration(runs.seconds) })}
          tone={runs.successRate !== null && runs.successRate < 0.8 ? "warning" : undefined}
        />
        <Kpi label={t(($) => $.insights_kpi_decisions)} value={health.pendingProposals} />
      </dl>

      <div className="grid gap-4 xl:grid-cols-2">
        {companyWide ? (
          <Card title={t(($) => $.insights_cost_daily)} className="xl:col-span-2">
            {dailyCost.length === 0 ? <Empty>{t(($) => $.insights_cost_empty)}</Empty> : <DailyCostChart data={dailyCost} />}
          </Card>
        ) : null}

        <Card title={companyWide ? t(($) => $.insights_cost_departments) : t(($) => $.insights_cost_squads)}>
          {rollup.total === 0 ? (
            <Empty>{t(($) => $.insights_cost_empty)}</Empty>
          ) : (
            <CostBars
              lines={(companyWide ? rollup.byDepartment : rollup.bySquad.filter((s) => inScope(scope, s.id))).filter(
                (line) => line.cost > 0,
              )}
            />
          )}
          {unattributed > 0.005 ? (
            <p className="mt-2 text-xs text-muted-foreground">
              {t(($) => $.insights_cost_unattributed, { amount: formatUsd(unattributed) })}
            </p>
          ) : null}
        </Card>

        <Card title={t(($) => $.insights_cost_projects)}>
          {projectCostLines.length === 0 ? <Empty>{t(($) => $.insights_cost_empty)}</Empty> : <CostBars lines={projectCostLines} />}
        </Card>

        {companyWide ? (
          <Card title={t(($) => $.insights_cost_squads)}>
            {rollup.total === 0 ? (
              <Empty>{t(($) => $.insights_cost_empty)}</Empty>
            ) : (
              <CostTable
                lines={rollup.bySquad.filter((line) => line.cost > 0).slice(0, TOP_ROWS)}
                total={rollup.total}
                detail={(line) => line.departmentName}
              />
            )}
          </Card>
        ) : null}

        <Card title={t(($) => $.insights_cost_agents)}>
          {rollup.byAgent.length === 0 ? (
            <Empty>{t(($) => $.insights_cost_empty)}</Empty>
          ) : (
            <CostTable lines={rollup.byAgent.slice(0, TOP_ROWS)} total={rollup.total} />
          )}
        </Card>

        <Card title={t(($) => $.insights_work_health, { days: digestDays })} className="xl:col-span-2">
          {health.rows.length === 0 ? (
            <Empty>{t(($) => $.insights_work_empty)}</Empty>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-left text-xs text-muted-foreground">
                    <th className="py-1 pr-3 font-normal">{t(($) => $.squads)}</th>
                    <th className="px-2 py-1 text-right font-normal">{t(($) => $.insights_col_done)}</th>
                    <th className="px-2 py-1 text-right font-normal">{t(($) => $.insights_col_in_progress)}</th>
                    <th className="px-2 py-1 text-right font-normal">{t(($) => $.insights_col_blocked)}</th>
                    <th className="px-2 py-1 text-right font-normal">{t(($) => $.insights_col_stale)}</th>
                    <th className="px-2 py-1 text-right font-normal">{t(($) => $.insights_col_failed_runs)}</th>
                    <th className="py-1 pl-2 text-right font-normal">{t(($) => $.insights_col_decisions)}</th>
                  </tr>
                </thead>
                <tbody className="tabular-nums">
                  {health.rows.slice(0, 12).map((row) => (
                    <tr key={row.id} className="border-t">
                      <td className="max-w-48 truncate py-1.5 pr-3">{row.name}</td>
                      <td className="px-2 text-right">{row.done}</td>
                      <td className="px-2 text-right">{row.inProgress}</td>
                      <td className={cn("px-2 text-right", row.blocked > 0 && "text-warning")}>{row.blocked}</td>
                      <td className={cn("px-2 text-right", row.stale > 0 && "text-warning")}>{row.stale}</td>
                      <td className={cn("px-2 text-right", row.failedRuns > 0 && "text-destructive")}>{row.failedRuns}</td>
                      <td className="pl-2 text-right">{row.pendingProposals}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {health.quiet > 0 ? (
                <p className="mt-2 text-xs text-muted-foreground">{t(($) => $.insights_quiet, { quiet: health.quiet })}</p>
              ) : null}
            </div>
          )}
        </Card>

        <Card title={t(($) => $.insights_reliability)} className="xl:col-span-2">
          {squadRuns.length === 0 ? (
            <Empty>{t(($) => $.insights_runs_empty)}</Empty>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-left text-xs text-muted-foreground">
                    <th className="py-1 pr-3 font-normal">{t(($) => $.squads)}</th>
                    <th className="px-2 py-1 text-right font-normal">{t(($) => $.insights_col_runs)}</th>
                    <th className="px-2 py-1 text-right font-normal">{t(($) => $.insights_col_success)}</th>
                    <th className="px-2 py-1 text-right font-normal">{t(($) => $.insights_col_run_time)}</th>
                    <th className="py-1 pl-2 text-right font-normal">{t(($) => $.insights_col_cost_per_run)}</th>
                  </tr>
                </thead>
                <tbody className="tabular-nums">
                  {squadRuns.slice(0, 12).map((row) => {
                    const cost = squadCostById.get(row.id) ?? 0;
                    return (
                      <tr key={row.id} className="border-t">
                        <td className="max-w-48 truncate py-1.5 pr-3">{row.name}</td>
                        <td className="px-2 text-right">{row.runs}</td>
                        <td
                          className={cn(
                            "px-2 text-right",
                            row.successRate !== null && row.successRate < 0.8 && "text-destructive",
                          )}
                        >
                          {successShare(row.successRate)}
                        </td>
                        <td className="px-2 text-right">{duration(row.seconds)}</td>
                        <td className="pl-2 text-right">{cost > 0 ? formatUsd(cost / row.runs) : "—"}</td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </Card>

        {companyWide ? (
          <>
            <Card title={t(($) => $.insights_runs_daily)}>
              {dailyTasks.length === 0 ? <Empty>{t(($) => $.insights_runs_empty)}</Empty> : <DailyTasksChart data={dailyTasks} />}
            </Card>
            <Card title={t(($) => $.insights_failures_daily)}>
              {dailyErrors.length === 0 ? (
                <Empty>{t(($) => $.insights_failures_empty)}</Empty>
              ) : (
                <DailyErrorsChart data={dailyErrors} />
              )}
            </Card>
          </>
        ) : null}

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
                    {t(($) => $.insights_projects_done_of, { done: projectSummary.done, total: projectSummary.issues })}
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
          {scopedAgents.length === 0 ? (
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

        <Card title={t(($) => $.insights_pipeline)}>
          {pipeline.total === 0 ? (
            <Empty>{t(($) => $.insights_pipeline_empty)}</Empty>
          ) : (
            <ul className="flex flex-col gap-1.5 text-sm">
              {pipeline.byStage.map((stage) => (
                <li key={stage.key} className="flex items-baseline justify-between gap-2">
                  <span className="truncate">{stage.name}</span>
                  <span className="shrink-0 tabular-nums text-muted-foreground">
                    {stage.stalled > 0 ? (
                      <span className="mr-2 text-warning">{t(($) => $.insights_pipeline_stalled, { stalled: stage.stalled })}</span>
                    ) : null}
                    {stage.count}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </Card>

        <Card title={t(($) => $.insights_goals)}>
          {goals.length === 0 ? (
            <Empty>{t(($) => $.insights_goals_empty)}</Empty>
          ) : (
            <ul className="flex flex-col gap-3">
              {goals.slice(0, TOP_ROWS).map((goal) => (
                <li key={goal.id} className="flex flex-col gap-1">
                  <div className="flex items-baseline justify-between gap-2 text-sm">
                    <span className="min-w-0 truncate">{goal.title}</span>
                    <span className={cn("shrink-0 text-xs", CONFIDENCE_TEXT[goal.confidence])}>
                      {labels.confidence[goal.confidence] ?? goal.confidence}
                    </span>
                  </div>
                  <Progress value={(goal.progress ?? 0) * 100} />
                </li>
              ))}
            </ul>
          )}
        </Card>

        <Card title={t(($) => $.insights_overdue)}>
          {overdue.length === 0 ? (
            <Empty>{t(($) => $.insights_overdue_empty)}</Empty>
          ) : (
            <ul className="flex flex-col gap-1.5 text-sm">
              {overdue.slice(0, TOP_ROWS).map((request) => (
                <li key={request.issue_id} className="flex items-baseline justify-between gap-2">
                  <span className="min-w-0 truncate">
                    <span className="mr-2 text-xs text-muted-foreground">{request.identifier}</span>
                    {request.title}
                  </span>
                  <span className="shrink-0 text-xs tabular-nums text-warning">
                    {t(($) => $.insights_overdue_hours, { hours: Math.round(request.hours_overdue) })}
                  </span>
                </li>
              ))}
            </ul>
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
                  labels={labels}
                />
              ))}
            </ul>
          )}
        </Card>
      </div>
    </div>
  );
}

function BudgetRow({
  budget,
  name,
  now,
  labels,
}: {
  budget: Budget;
  name: string;
  now: Date;
  labels: ReturnType<typeof useLabels>;
}) {
  const { t } = useT("company");
  const used = budgetUsed(budget);
  const pace = budgetPace(budget, now);
  return (
    <li className="flex flex-col gap-1.5">
      <div className="flex items-baseline justify-between gap-2 text-sm">
        <span className="min-w-0 truncate">
          {name}
          <span className="ml-2 text-xs text-muted-foreground">{labels.period[budget.period]}</span>
        </span>
        <span className={cn("shrink-0 text-xs", BUDGET_STATE_TEXT[budget.state])}>
          {budget.status === "paused" ? t(($) => $.insights_budget_paused) : labels.state[budget.state]}
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
