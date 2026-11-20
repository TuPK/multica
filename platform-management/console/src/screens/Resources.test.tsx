// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Resources } from "./Resources";

const mockUseMe = vi.fn();
const mockUseSystemResources = vi.fn();
const mockFetchNextPage = vi.fn();

vi.mock("../api/hooks", () => ({
  useMe: () => mockUseMe(),
  useSystemResources: (filters?: Record<string, unknown>, enabled = true) =>
    mockUseSystemResources(filters, enabled),
}));

const sampleResourcesData = {
  by_type: [
    { type: "agents", label: "Agents", unit: "count", used: 42, limit: 100, percent: 42 },
    { type: "storage_bytes", label: "Storage", unit: "bytes", used: 2048, limit: 4096, percent: 50 },
  ],
  by_plan: [
    { plan_key: "pro", tenant_count: 10, workspace_count: 18, totals: { agents: 30 } },
    { plan_key: "enterprise", tenant_count: 2, workspace_count: 8, totals: { agents: 12 } },
  ],
  top_tenants: [
    {
      tenant_id: "t-1",
      tenant_name: "Acme Corp",
      best_plan_key: "pro",
      workspace_count: 3,
      members: 15,
      agents: 12,
      runtimes: 4,
      issues_total: 120,
      storage_bytes: 2048,
      autopilot_runs: 45,
      state: "over" as const,
      resources: [],
    },
    {
      tenant_id: "t-2",
      tenant_name: "Beta Labs",
      best_plan_key: "business",
      workspace_count: 2,
      members: 8,
      agents: 6,
      runtimes: 2,
      issues_total: 60,
      storage_bytes: 1024,
      autopilot_runs: 10,
      state: "ok" as const,
      resources: [],
    },
  ],
  tenants: [],
  over_limit: [
    {
      tenant_id: "t-1",
      tenant_name: "Acme Corp",
      workspace_id: "ws-1",
      workspace_name: "Core",
      type: "agents",
      used: 12,
      limit: 10,
      percent: 120,
      state: "over" as const,
    },
    {
      tenant_id: "t-3",
      tenant_name: "Gamma Inc",
      workspace_id: "ws-3",
      workspace_name: "Ops",
      type: "runtimes",
      used: 4,
      limit: 5,
      percent: 80,
      state: "warning" as const,
    },
  ],
  complete: true,
};

describe("Resources screen", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseMe.mockReturnValue({
      data: { user_id: "u-sys", email: "sys@example.com", role: "system_admin", tenant_id: null },
    });
    mockUseSystemResources.mockReturnValue({
      data: sampleResourcesData,
      isLoading: false,
      isFetching: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
      hasNextPage: false,
      fetchNextPage: mockFetchNextPage,
      isFetchingNextPage: false,
    });
  });

  it("shows top tenants and over limit list for system_admin only", () => {
    const onSelectTenant = vi.fn();
    const { unmount } = render(
      <Resources onSelectTenant={onSelectTenant} userRole="system_admin" />,
    );

    expect(screen.getByRole("heading", { name: "Resources" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: /Over-limit/i })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: /Top Tenants/i })).toBeInTheDocument();

    const overRow = screen.getByText("Core").closest("tr") as HTMLElement;
    expect(overRow).toHaveTextContent("Acme Corp");
    expect(overRow).toHaveTextContent("120%");
    expect(overRow).toHaveTextContent("Over limit");

    const warnRow = screen.getByText("Ops").closest("tr") as HTMLElement;
    expect(warnRow).toHaveTextContent("Gamma Inc");
    expect(warnRow).toHaveTextContent("80%");
    expect(warnRow).toHaveTextContent("Warning");

    const topTenantRow = screen.getByText("Beta Labs").closest("tr") as HTMLElement;
    expect(topTenantRow).toHaveTextContent("business");
    expect(topTenantRow).toHaveTextContent("1.0 KB");
    fireEvent.click(topTenantRow);
    expect(onSelectTenant).toHaveBeenCalledWith("t-2");

    unmount();

    for (const role of ["admin", "support", "billing"] as const) {
      mockUseMe.mockReturnValue({
        data: {
          user_id: `u-${role}`,
          email: `${role}@example.com`,
          role,
          tenant_id: role === "admin" ? "t-1" : null,
        },
      });
      const { unmount: unmountRole } = render(<Resources userRole={role} />);
      expect(screen.getByRole("region", { name: "Not found" })).toBeInTheDocument();
      expect(screen.queryByRole("heading", { name: "Resources" })).not.toBeInTheDocument();
      expect(mockUseSystemResources).toHaveBeenLastCalledWith(expect.anything(), false);
      unmountRole();
    }
  });

  it("supports sort selection, cursor paging, and partial completeness notice", async () => {
    const user = userEvent.setup();
    mockUseSystemResources.mockReturnValue({
      data: { ...sampleResourcesData, complete: false },
      isLoading: false,
      isFetching: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
      hasNextPage: true,
      fetchNextPage: mockFetchNextPage,
      isFetchingNextPage: false,
    });

    render(<Resources userRole="system_admin" />);

    expect(screen.getByText(/partial|incomplete/i)).toBeInTheDocument();

    await user.click(screen.getByRole("combobox", { name: /Sort by/i }));
    await user.click(await screen.findByRole("option", { name: "Agents" }));
    expect(mockUseSystemResources).toHaveBeenLastCalledWith({ sort: "agents" }, true);

    fireEvent.click(screen.getByRole("button", { name: /Load more/i }));
    expect(mockFetchNextPage).toHaveBeenCalledTimes(1);
  });

  it("handles loading, error, and empty states", () => {
    mockUseSystemResources.mockReturnValue({
      data: undefined,
      isLoading: true,
      isFetching: true,
      isError: false,
      error: null,
      refetch: vi.fn(),
      hasNextPage: false,
      fetchNextPage: mockFetchNextPage,
      isFetchingNextPage: false,
    });
    const { rerender } = render(<Resources userRole="system_admin" />);
    expect(screen.getByText("Loading resources...")).toBeInTheDocument();

    mockUseSystemResources.mockReturnValue({
      data: undefined,
      isLoading: false,
      isFetching: false,
      isError: true,
      error: new Error("Service unavailable"),
      refetch: vi.fn(),
      hasNextPage: false,
      fetchNextPage: mockFetchNextPage,
      isFetchingNextPage: false,
    });
    rerender(<Resources userRole="system_admin" />);
    expect(screen.getByText("Failed to load resources")).toBeInTheDocument();
    expect(screen.getByText("Service unavailable")).toBeInTheDocument();
  });
});
