import { describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import { renderWithI18n } from "../../test/i18n";
import { CompanyInsights } from "./company-insights";

const mocks = vi.hoisted(() => ({ data: {} as Record<string, unknown> }));

vi.mock("@tanstack/react-query", () => ({
  useQuery: (options: { queryKey?: readonly unknown[]; enabled?: boolean }) => ({
    isPending: false,
    data: options.enabled === false ? undefined : mocks.data[String(options.queryKey?.[0])],
  }),
  useQueries: ({ queries }: { queries: unknown[] }) => queries.map(() => ({ data: undefined })),
}));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("../../common/use-viewing-timezone", () => ({ useViewingTimezone: () => "UTC" }));
vi.mock("@multica/core/projects/queries", () => ({ projectListOptions: () => ({ queryKey: ["projects"] }) }));
vi.mock("@multica/core/company", () => ({ digestOptions: () => ({ queryKey: ["digest"] }) }));
vi.mock("@multica/core/governance", () => ({
  budgetsOptions: () => ({ queryKey: ["budgets"] }),
  budgetPace: () => ({ elapsed: 0, used: 0, overrunAt: null }),
}));
vi.mock("@multica/core/dashboard", () => ({
  dashboardUsageByAgentOptions: () => ({ queryKey: ["by-agent"] }),
  dashboardAgentRunTimeOptions: () => ({ queryKey: ["run-time"] }),
  dashboardUsageDailyOptions: () => ({ queryKey: ["daily"] }),
  dashboardRunTimeDailyOptions: () => ({ queryKey: ["run-time-daily"] }),
  dashboardFailuresDailyOptions: () => ({ queryKey: ["failures-daily"] }),
}));
vi.mock("../../runtimes/components/charts", () => ({
  DailyCostChart: () => <div>daily cost chart</div>,
  DailyTasksChart: () => <div>daily runs chart</div>,
  DailyErrorsChart: () => <div>daily failures chart</div>,
}));

const squads = [
  { id: "s-1", name: "Planning", floor_id: "f-1" },
  { id: "s-2", name: "Discovery", floor_id: "f-2" },
] as never[];
const agents = [
  { id: "a-1", name: "Ada", status: "idle", home_squad_id: "s-1" },
  { id: "a-2", name: "Bo", status: "working", home_squad_id: "s-2" },
] as never[];
const departments = [
  { id: "f-1", name: "Executive", squads: [squads[0]] },
  { id: "f-2", name: "Product", squads: [squads[1]] },
] as never[];

function seed() {
  mocks.data = {
    "by-agent": [
      { agent_id: "a-1", provider: "x", model: "m", input_tokens: 0, output_tokens: 0, cache_read_tokens: 0, cache_write_tokens: 0, cost_usd_ticks: 300_000_000_000, task_count: 1 },
      { agent_id: "a-2", provider: "x", model: "m", input_tokens: 0, output_tokens: 0, cache_read_tokens: 0, cache_write_tokens: 0, cost_usd_ticks: 100_000_000_000, task_count: 1 },
    ],
    "run-time": [
      { agent_id: "a-1", total_seconds: 60, task_count: 4, failed_count: 0, cancelled_count: 0 },
      { agent_id: "a-2", total_seconds: 60, task_count: 6, failed_count: 0, cancelled_count: 0 },
    ],
  };
}

describe("CompanyInsights", () => {
  it("reports the whole company with the company-wide charts", () => {
    seed();
    renderWithI18n(<CompanyInsights departments={departments} squads={squads} agents={agents} department={null} />);
    expect(screen.getByText("Company overview")).toBeInTheDocument();
    expect(screen.getAllByText("$40.00").length).toBeGreaterThan(0);
    expect(screen.getByText("Runs per day")).toBeInTheDocument();
  });

  it("narrows spend, runs and agents to the department and drops company-only charts", () => {
    seed();
    renderWithI18n(
      <CompanyInsights departments={departments} squads={squads} agents={agents} department={departments[1] ?? null} />,
    );
    expect(screen.getByText("Product")).toBeInTheDocument();
    expect(screen.getAllByText("$10.00").length).toBeGreaterThan(0);
    expect(screen.queryByText("$40.00")).not.toBeInTheDocument();
    expect(screen.queryByText("Runs per day")).not.toBeInTheDocument();
    expect(screen.queryByText("Daily spend")).not.toBeInTheDocument();
  });
});
