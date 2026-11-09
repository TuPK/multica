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
