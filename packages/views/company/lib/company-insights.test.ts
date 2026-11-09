// @vitest-environment node
import { describe, expect, it } from "vitest";
import type { DashboardUsageByAgent, Project } from "@multica/core/types";
import type { Budget } from "@multica/core/governance";
import { budgetTotals, budgetUsed, projectStats, rollUpCost } from "./company-insights";

const row = (agent_id: string, ticks: number): DashboardUsageByAgent => ({
  agent_id,
  provider: "claude",
  model: "unpriced-model",
  input_tokens: 0,
  output_tokens: 0,
  cache_read_tokens: 0,
  cache_write_tokens: 0,
  cost_usd_ticks: ticks,
  task_count: 1,
});

describe("rollUpCost", () => {
  const ticks = 10_000_000_000; // 1 USD
  const input = {
    usage: [row("a-1", ticks), row("a-2", 2 * ticks), row("a-ghost", ticks), row("a-3", 4 * ticks)],
    agents: [
      { id: "a-1", name: "One", home_squad_id: "s-1" },
      { id: "a-2", name: "Two", home_squad_id: "s-2" },
      { id: "a-3", name: "Three", home_squad_id: null },
    ],
    squads: [
      { id: "s-1", name: "Alpha", floor_id: "f-1" },
      { id: "s-2", name: "Beta", floor_id: null },
    ],
    departments: [
      { id: "f-1", name: "Build" },
      { id: "__unseated__", name: "Loose" },
    ],
    unseatedName: "Loose",
  };

  it("folds agent spend into squads and departments, keeping unattributed spend in the total", () => {
    const out = rollUpCost(input);
    expect(out.total).toBeCloseTo(8);
    expect(out.bySquad.map((s) => [s.name, s.cost])).toEqual([
      ["Beta", 2],
      ["Alpha", 1],
    ]);
    expect(out.bySquad[0]?.departmentName).toBe("Loose");
    expect(out.byDepartment.find((d) => d.id === "f-1")?.cost).toBe(1);
    expect(out.byAgent.map((a) => a.name)).toEqual(["Three", "Two", "One"]);
  });
});

describe("projectStats", () => {
  const project = (over: Partial<Project>): Project =>
    ({ status: "in_progress", issue_count: 0, done_count: 0, due_date: null, ...over }) as Project;

  it("counts statuses, completion and overdue open projects", () => {
    const out = projectStats(
      [
        project({ issue_count: 10, done_count: 5, due_date: "2026-01-01" }),
        project({ status: "completed", issue_count: 10, done_count: 10, due_date: "2026-01-01" }),
        project({ status: "planned" }),
      ],
      new Date("2026-06-01T00:00:00Z"),
    );
    expect(out.total).toBe(3);
    expect(out.byStatus.find((s) => s.status === "in_progress")?.count).toBe(1);
    expect(out.completion).toBe(0.75);
    expect(out.overdue).toBe(1);
  });

  it("reports zero completion with no issues", () => {
    expect(projectStats([], new Date()).completion).toBe(0);
  });
});

describe("budgets", () => {
  const budget = (over: Partial<Budget>): Budget =>
    ({ scope_type: "company", status: "active", state: "ok", limit_micros: 0, spent_micros: 0, ...over }) as Budget;

  it("totals only active company budgets and counts risk states across all scopes", () => {
    const out = budgetTotals([
      budget({ limit_micros: 100, spent_micros: 40 }),
      budget({ scope_type: "squad", limit_micros: 50, spent_micros: 60, state: "hard" }),
      budget({ limit_micros: 999, status: "paused" }),
      budget({ scope_type: "floor", state: "soft" }),
    ]);
    expect(out).toEqual({ companyLimit: 100, companySpent: 40, atRisk: 1, overLimit: 1 });
  });

  it("treats spend against a zero limit as an overrun", () => {
    expect(budgetUsed({ limit_micros: 0, spent_micros: 0 })).toBe(0);
    expect(budgetUsed({ limit_micros: 0, spent_micros: 5 })).toBe(Number.POSITIVE_INFINITY);
    expect(budgetUsed({ limit_micros: 200, spent_micros: 50 })).toBe(0.25);
  });
});

import {
  budgetsInScope,
  inScope,
  pipelineSummary,
  runStats,
  runStatsBySquad,
  workHealth,
  type InsightScope,
} from "./company-insights";

const dept: InsightScope = { departmentId: "f-1", squadIds: new Set(["s-1", "s-2"]) };
const all: InsightScope = { departmentId: null, squadIds: new Set(["s-1", "s-2", "s-9"]) };

describe("scope", () => {
  it("covers every squad for the company and only its own for a department", () => {
    expect(inScope(all, "anything")).toBe(true);
    expect(inScope(dept, "s-1")).toBe(true);
    expect(inScope(dept, "s-9")).toBe(false);
    expect(inScope(dept, null)).toBe(false);
  });
});

describe("workHealth", () => {
  const squad = (id: string, over: object = {}) => ({
    id,
    name: id,
    done: 1,
    in_progress: 2,
    blocked: 0,
    stale: 0,
    failed_runs: 0,
    pending_proposal_count: 1,
    ...over,
  });
  const scopeSquads = [
    { id: "s-1", name: "s-1" },
    { id: "s-2", name: "s-2" },
    { id: "s-3", name: "s-3" },
    { id: "s-9", name: "s-9" },
  ];

  it("sums the squads in scope and ranks the ones in trouble first", () => {
    const out = workHealth([squad("s-1"), squad("s-2", { blocked: 3 }), squad("s-9")], [], scopeSquads, dept);
    expect(out.rows.map((r) => r.id)).toEqual(["s-2", "s-1"]);
    expect(out).toMatchObject({ done: 2, inProgress: 4, blocked: 3, pendingProposals: 2, quiet: 0 });
  });

  it("takes in-progress counts from the quiet squads and counts squads without activity", () => {
    const out = workHealth([], [{ id: "s-1", in_progress: 5 }], scopeSquads, all);
    expect(out.inProgress).toBe(5);
    expect(out.rows.map((r) => r.id)).toEqual(["s-1"]);
    expect(out.quiet).toBe(3);
  });
});

describe("run stats", () => {
  const rows = [
    { agent_id: "a-1", total_seconds: 60, task_count: 10, failed_count: 1, cancelled_count: 1 },
    { agent_id: "a-2", total_seconds: 30, task_count: 5, failed_count: 0, cancelled_count: 0 },
  ];

  it("computes success over finished runs and can narrow to some agents", () => {
    expect(runStats(rows, null)).toMatchObject({ runs: 15, seconds: 90, successRate: 13 / 15 });
    expect(runStats(rows, new Set(["a-2"]))).toMatchObject({ runs: 5, successRate: 1 });
    expect(runStats([], null).successRate).toBeNull();
  });

  it("folds agents into their home squads and drops squads with no runs", () => {
    const out = runStatsBySquad(
      rows,
      [
        { id: "a-1", home_squad_id: "s-1" },
        { id: "a-2", home_squad_id: null },
      ],
      [
        { id: "s-1", name: "Alpha" },
        { id: "s-2", name: "Beta" },
      ],
    );
    expect(out.map((s) => [s.name, s.runs])).toEqual([["Alpha", 10]]);
  });
});

describe("budgetsInScope", () => {
  const b = (scope_type: string, scope_id: string | null) => ({ scope_type, scope_id }) as Budget;

  it("keeps a department's floor, squad and project budgets and drops the company's", () => {
    const list = [b("company", null), b("floor", "f-1"), b("floor", "f-2"), b("squad", "s-1"), b("squad", "s-9"), b("initiative", "p-1")];
    expect(budgetsInScope(list, dept, new Set(["p-1"]))).toHaveLength(3);
    expect(budgetsInScope(list, all, new Set())).toHaveLength(6);
  });
});

describe("pipelineSummary", () => {
  it("groups initiatives by stage within scope and counts the stalled", () => {
    const item = (owner: string | null, stage: string, stalled = false) => ({
      owner_squad_id: owner,
      stage_key: stage,
      stage_name: stage,
      stalled,
    });
    const out = pipelineSummary([item("s-1", "build", true), item("s-2", "build"), item("s-9", "build"), item(null, "plan")], dept);
    expect(out).toEqual({ total: 2, stalled: 1, byStage: [{ key: "build", name: "build", count: 2, stalled: 1 }] });
  });
});
