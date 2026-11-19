// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, within } from "@testing-library/react";
import { Tenants } from "./Tenants";

const mockUseTenants = vi.fn();

vi.mock("../api/hooks", () => ({
  useTenants: (filters: { q?: string }) => mockUseTenants(filters),
}));

const mockTenantsList = [
  {
    id: "t-1",
    name: "Beta Alpha",
    slug: "beta-alpha",
    status: "active",
    member_count: 5,
    created_at: "2026-10-01T00:00:00Z",
    last_activity_at: "2026-11-15T00:00:00Z",
  },
  {
    id: "t-2",
    name: "Gamma Delta",
    slug: "gamma-delta",
    status: "suspended",
    member_count: 2,
    created_at: "2026-10-05T00:00:00Z",
    last_activity_at: "2026-11-10T00:00:00Z",
  },
];

const withUsage = {
  ...mockTenantsList[0]!,
  usage_day: "2026-11-14",
  usage_members: 3,
  usage_agents: 2,
  usage_runtimes: 1,
  usage_issues_total: 40,
  usage_issues_created: 5,
  usage_tasks_run: 9,
  usage_storage_bytes: 2048,
};

const noUsageRow = {
  ...mockTenantsList[1]!,
  usage_day: null,
  usage_members: 0,
  usage_agents: 0,
  usage_runtimes: 0,
  usage_issues_total: 0,
  usage_issues_created: 0,
  usage_tasks_run: 0,
  usage_storage_bytes: 0,
};

function usageCell(name: string): HTMLElement {
  const row = screen.getByText(name).closest("tr");
  expect(row).not.toBeNull();
  return within(row as HTMLElement).getByTestId("tenant-usage");
}

describe("Tenants screen usage column", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders the members, agents, tasks run and storage of the latest day", () => {
    mockUseTenants.mockReturnValue({
      data: { tenants: [withUsage, noUsageRow], next_cursor: null },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });
    render(<Tenants onSelectTenant={vi.fn()} />);

    expect(screen.getByRole("columnheader", { name: "Usage" })).toBeInTheDocument();
    const cell = usageCell("Beta Alpha");
    expect(cell).toHaveTextContent("3 members");
    expect(cell).toHaveTextContent("2 agents");
    expect(cell).toHaveTextContent("9 tasks run");
    expect(cell).toHaveTextContent("2.0 KB");
    expect(cell).toHaveAttribute("title", expect.stringContaining("2026-11-14"));
  });

  it("shows an empty value, not zero and not an error, for a tenant with no usage", () => {
    mockUseTenants.mockReturnValue({
      data: { tenants: [withUsage, noUsageRow], next_cursor: null },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });
    render(<Tenants onSelectTenant={vi.fn()} />);

    const cell = usageCell("Gamma Delta");
    expect(cell.textContent).toBe("-");
    expect(cell.textContent).not.toMatch(/0/);
    expect(screen.queryByText("Failed to load tenants")).not.toBeInTheDocument();
    expect(screen.getByText("Beta Alpha")).toBeInTheDocument();
  });

  it("handles a tenant that carries no usage fields at all", () => {
    mockUseTenants.mockReturnValue({
      data: { tenants: mockTenantsList, next_cursor: null },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });
    render(<Tenants onSelectTenant={vi.fn()} />);

    expect(usageCell("Beta Alpha").textContent).toBe("-");
    expect(usageCell("Gamma Delta").textContent).toBe("-");
  });
});

describe("Tenants screen", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders tenants list", () => {
    mockUseTenants.mockReturnValue({
      data: { tenants: mockTenantsList, next_cursor: null },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });

    render(<Tenants onSelectTenant={vi.fn()} />);

    expect(screen.getByRole("heading", { name: "Tenants" })).toBeInTheDocument();
    expect(screen.getByText("Beta Alpha")).toBeInTheDocument();
    expect(screen.getByText("beta-alpha")).toBeInTheDocument();
    expect(screen.getByText("Gamma Delta")).toBeInTheDocument();
    expect(screen.getByText("gamma-delta")).toBeInTheDocument();
  });

  it("calls onSelectTenant when View button is clicked", () => {
    mockUseTenants.mockReturnValue({
      data: { tenants: mockTenantsList, next_cursor: null },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });
    const onSelect = vi.fn();

    render(<Tenants onSelectTenant={onSelect} />);

    const viewButtons = screen.getAllByRole("button", { name: "View" });
    expect(viewButtons[0]).toBeDefined();
    fireEvent.click(viewButtons[0]!);

    expect(onSelect).toHaveBeenCalledWith("t-1");
  });

  it("handles loading, error, and empty states", () => {
    mockUseTenants.mockReturnValue({
      data: null,
      isLoading: true,
      isError: false,
      refetch: vi.fn(),
    });

    const { rerender } = render(<Tenants onSelectTenant={vi.fn()} />);
    expect(screen.getByText("Loading tenants...")).toBeInTheDocument();

    mockUseTenants.mockReturnValue({
      data: null,
      isLoading: false,
      isError: true,
      error: new Error("Network timeout"),
      refetch: vi.fn(),
    });

    rerender(<Tenants onSelectTenant={vi.fn()} />);
    expect(screen.getByText("Failed to load tenants")).toBeInTheDocument();
    expect(screen.getByText("Network timeout")).toBeInTheDocument();

    mockUseTenants.mockReturnValue({
      data: { tenants: [], next_cursor: null },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });

    rerender(<Tenants onSelectTenant={vi.fn()} />);
    expect(screen.getByText("No tenants found")).toBeInTheDocument();
  });

  it("offers Clear search when a search query returns no tenants", () => {
    mockUseTenants.mockReturnValue({
      data: { tenants: [], next_cursor: null },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });

    render(<Tenants onSelectTenant={vi.fn()} />);
    const input = screen.getByPlaceholderText("Search by name or slug...");
    fireEvent.change(input, { target: { value: "nonexistent" } });
    expect(mockUseTenants).toHaveBeenLastCalledWith({ q: "nonexistent" });

    const clearButtons = screen.getAllByRole("button", { name: /Clear search/i });
    fireEvent.click(clearButtons[clearButtons.length - 1]!);
    expect(input).toHaveValue("");
    expect(mockUseTenants).toHaveBeenLastCalledWith({ q: undefined });
  });
});
