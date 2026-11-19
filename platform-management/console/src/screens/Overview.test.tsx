// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { Overview } from "./Overview";

const mockUseOverview = vi.fn();

vi.mock("../api/hooks", () => ({
  useOverview: () => mockUseOverview(),
}));

describe("Overview screen", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders workspace counts by status and 7d signups", () => {
    mockUseOverview.mockReturnValue({
      data: {
        workspaces_by_status: {
          active: 42,
          read_only: 5,
          suspended: 3,
          pending_deletion: 1,
        },
        signups_7d: 19,
      },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });

    render(<Overview />);

    expect(screen.getByRole("heading", { name: "Platform Overview" })).toBeInTheDocument();
    const kpiBand = screen.getByLabelText("Workspace metrics");
    expect(within(kpiBand).getByText("Total Workspaces")).toBeInTheDocument();
    expect(within(kpiBand).getByText("51")).toBeInTheDocument(); // 42+5+3+1
    expect(within(kpiBand).getByText("Active Workspaces")).toBeInTheDocument();
    expect(within(kpiBand).getByText("42")).toBeInTheDocument();
    expect(within(kpiBand).getByText("Sign-ups (7 Days)")).toBeInTheDocument();
    expect(within(kpiBand).getByText("19")).toBeInTheDocument();
    expect(within(kpiBand).getByText("Read-Only")).toBeInTheDocument();
    expect(within(kpiBand).getByText("5")).toBeInTheDocument();
    expect(within(kpiBand).getByText("Suspended")).toBeInTheDocument();
    expect(within(kpiBand).getByText("3")).toBeInTheDocument();
    expect(within(kpiBand).getByText("Pending Deletion")).toBeInTheDocument();
    expect(within(kpiBand).getByText("1")).toBeInTheDocument();
  });

  it("renders pending orders, subscriptions by plan, and platform usage snapshot when present", () => {
    mockUseOverview.mockReturnValue({
      data: {
        workspaces_by_status: {
          active: 10,
          read_only: 0,
          suspended: 0,
          pending_deletion: 0,
        },
        signups_7d: 4,
        pending_orders: 3,
        pending_by_currency: {
          vnd: { orders: 3, amount: 600000, vat_amount: 60000 },
        },
        by_plan: {
          free: 7,
          pro: 3,
        },
        usage_day: "2026-11-18",
        usage_workspaces: 8,
        usage_members: 24,
        usage_agents: 6,
        usage_runtimes: 4,
        usage_issues_total: 120,
        usage_issues_created: 15,
        usage_tasks_run: 45,
        usage_storage_bytes: 2048,
      },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });

    render(<Overview />);

    expect(screen.getByText("Pending Orders")).toBeInTheDocument();
    expect(screen.getByText(/660,000 VND/)).toBeInTheDocument();

    expect(screen.getByRole("heading", { name: "Subscriptions by Plan" })).toBeInTheDocument();
    expect(screen.getAllByText("free").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("pro").length).toBeGreaterThanOrEqual(1);

    expect(screen.getByRole("heading", { name: "Platform Usage Snapshot" })).toBeInTheDocument();
    const supportingBand = screen.getByLabelText("Supporting tables");
    expect(within(supportingBand).getByText("2026-11-18")).toBeInTheDocument();
    expect(within(supportingBand).getByText("24")).toBeInTheDocument();
    expect(within(supportingBand).getByText("120")).toBeInTheDocument();
    expect(within(supportingBand).getByText("2.0 KB")).toBeInTheDocument();

    // Analytics Dashboard charts (@astryxdesign/charts) and supporting tables bands
    expect(
      screen.getByRole("img", { name: /Workspace status distribution/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("img", { name: /Subscriptions by plan/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Pending Orders by Currency" }),
    ).toBeInTheDocument();
  });

  it("refreshes overview metrics from the header Refresh button", () => {
    const refetch = vi.fn();
    mockUseOverview.mockReturnValue({
      data: {
        workspaces_by_status: { active: 2, read_only: 0, suspended: 0, pending_deletion: 0 },
        signups_7d: 1,
      },
      isLoading: false,
      isFetching: false,
      isError: false,
      refetch,
    });

    render(<Overview />);
    fireEvent.click(screen.getByRole("button", { name: /Refresh/i }));
    expect(refetch).toHaveBeenCalledTimes(1);
  });

  it("handles loading, error, and empty states", () => {
    mockUseOverview.mockReturnValue({
      data: null,
      isLoading: true,
      isError: false,
      refetch: vi.fn(),
    });

    const { rerender } = render(<Overview />);
    expect(screen.getByText("Loading overview...")).toBeInTheDocument();

    mockUseOverview.mockReturnValue({
      data: null,
      isLoading: false,
      isError: true,
      error: new Error("Metrics failure"),
      refetch: vi.fn(),
    });

    rerender(<Overview />);
    expect(screen.getByText("Failed to load overview")).toBeInTheDocument();
    expect(screen.getByText("Metrics failure")).toBeInTheDocument();

    mockUseOverview.mockReturnValue({
      data: null,
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });

    rerender(<Overview />);
    expect(screen.getByText("No overview data")).toBeInTheDocument();
  });
});
