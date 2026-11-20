// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, within, waitFor } from "@testing-library/react";
import { Tenants } from "./Tenants";

const mockUseTenants = vi.fn();
const mockUseMe = vi.fn();
const mockSetTenantStatusMutateAsync = vi.fn();
const mockDeleteTenantNowMutateAsync = vi.fn();

vi.mock("../api/hooks", () => ({
  useTenants: (filters: { q?: string }) => mockUseTenants(filters),
  useMe: () => mockUseMe(),
  useSetTenantStatus: () => ({ mutateAsync: mockSetTenantStatusMutateAsync, isPending: false }),
  useDeleteTenantNow: () => ({ mutateAsync: mockDeleteTenantNowMutateAsync, isPending: false }),
}));

function asRole(role: string) {
  mockUseMe.mockReturnValue({ data: { user_id: "u-1", email: "u@example.com", role, tenant_id: null } });
}

const mockTenantsList = [
  {
    id: "t-1",
    name: "Beta Alpha",
    status: "active",
    workspace_count: 3,
    max_workspaces: 10,
    best_plan_key: "business",
    status_summary: { active: 2, suspended: 1 },
    created_at: "2026-10-01T00:00:00Z",
    last_activity_at: "2026-11-15T00:00:00Z",
  },
  {
    id: "t-2",
    name: "Gamma Delta",
    status: "active",
    workspace_count: 1,
    max_workspaces: null,
    best_plan_key: "enterprise",
    status_summary: { active: 1 },
    created_at: "2026-10-05T00:00:00Z",
    last_activity_at: "2026-11-10T00:00:00Z",
  },
];

function rowOf(name: string): HTMLElement {
  const row = screen.getByText(name).closest("tr");
  expect(row).not.toBeNull();
  return row as HTMLElement;
}

function listOf(tenants: unknown[]) {
  mockUseTenants.mockReturnValue({
    data: { tenants, next_cursor: null },
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
  });
}

describe("Tenants screen workspace columns", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    asRole("system_admin");
  });

  it("renders one row per tenant with workspace count and max_workspaces", () => {
    listOf(mockTenantsList);
    render(<Tenants onSelectTenant={vi.fn()} />);

    expect(screen.getAllByRole("row")).toHaveLength(3); // the header and one row per tenant
    expect(screen.getByRole("columnheader", { name: "Workspaces" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "Max workspaces" })).toBeInTheDocument();
    const beta = rowOf("Beta Alpha");
    expect(within(beta).getByTestId("tenant-workspace-count")).toHaveTextContent("3");
    expect(within(beta).getByTestId("tenant-max-workspaces")).toHaveTextContent("10");
    expect(within(beta).getByText("business")).toBeInTheDocument();
    expect(beta).toHaveTextContent("2 active, 1 suspended");
  });

  it("shows unlimited when max_workspaces is null", () => {
    listOf(mockTenantsList);
    render(<Tenants onSelectTenant={vi.fn()} />);

    const cell = within(rowOf("Gamma Delta")).getByTestId("tenant-max-workspaces");
    expect(cell).toHaveTextContent("Unlimited");
    expect(cell.textContent).not.toMatch(/0|null/);
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
    expect(screen.getByText("Gamma Delta")).toBeInTheDocument();
  });

  it("opens the tenant detail when a row is clicked, with no View action", () => {
    listOf(mockTenantsList);
    const onSelect = vi.fn();

    render(<Tenants onSelectTenant={onSelect} />);

    expect(screen.queryByRole("button", { name: "View" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByText("Beta Alpha"));
    expect(onSelect).toHaveBeenCalledWith("t-1");
  });

  it("locks an active tenant from its row action", async () => {
    listOf(mockTenantsList);
    mockSetTenantStatusMutateAsync.mockResolvedValueOnce({ status: "suspended", workspace_ids: ["ws-1"] });

    render(<Tenants onSelectTenant={vi.fn()} />);

    fireEvent.click(within(rowOf("Beta Alpha")).getByRole("button", { name: /Lock tenant/i }));
    const dialog = screen.getByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText(/Reason/i), { target: { value: "Fraud hold" } });
    fireEvent.click(within(dialog).getByRole("button", { name: /Apply lock/i }));

    await waitFor(() => {
      expect(mockSetTenantStatusMutateAsync).toHaveBeenCalledWith({
        tenantId: "t-1",
        status: "suspended",
        reason: "Fraud hold",
      });
    });
  });

  it("deletes a tenant from its row action after typing its name", async () => {
    listOf(mockTenantsList);
    mockDeleteTenantNowMutateAsync.mockResolvedValueOnce({ request_id: "dr-9", state: "pending" });

    render(<Tenants onSelectTenant={vi.fn()} />);

    fireEvent.click(within(rowOf("Gamma Delta")).getByRole("button", { name: /Delete tenant/i }));
    const dialog = screen.getByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText(/Reason/i), { target: { value: "Contract ended" } });
    fireEvent.change(within(dialog).getByLabelText(/Tenant name/i), { target: { value: "Gamma Delta" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Delete tenant" }));

    await waitFor(() => {
      expect(mockDeleteTenantNowMutateAsync).toHaveBeenCalledWith({
        tenantId: "t-2",
        reason: "Contract ended",
        confirm: "Gamma Delta",
        export_first: false,
      });
    });
  });

  it("shows no tenant actions to an admin", () => {
    asRole("admin");
    listOf(mockTenantsList);

    render(<Tenants onSelectTenant={vi.fn()} />);

    expect(screen.queryByRole("button", { name: /Lock tenant|Delete tenant/i })).not.toBeInTheDocument();
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
    const input = screen.getByPlaceholderText("Search by tenant, workspace or owner email...");
    fireEvent.change(input, { target: { value: "nonexistent" } });
    expect(mockUseTenants).toHaveBeenLastCalledWith({ q: "nonexistent" });

    const clearButtons = screen.getAllByRole("button", { name: /Clear search/i });
    fireEvent.click(clearButtons[clearButtons.length - 1]!);
    expect(input).toHaveValue("");
    expect(mockUseTenants).toHaveBeenLastCalledWith({ q: undefined });
  });

  it("admin sees only the tenant returned by the API and no delete-tenant action", () => {
    asRole("admin");
    listOf([mockTenantsList[0]]);
    render(<Tenants onSelectTenant={vi.fn()} />);

    expect(screen.getAllByRole("row")).toHaveLength(2); // header + single scoped tenant
    expect(screen.getByText("Beta Alpha")).toBeInTheDocument();
    expect(screen.queryByText("Gamma Delta")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /delete/i })).not.toBeInTheDocument();
  });
});
