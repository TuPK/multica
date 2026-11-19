// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, within } from "@testing-library/react";
import { TenantDetail } from "./TenantDetail";

const mockUseTenant = vi.fn();

vi.mock("../api/hooks", () => ({
  useTenant: (id?: string) => mockUseTenant(id),
}));

const tenant = {
  id: "t-1",
  name: "Acme Group",
  status: "active",
  workspace_count: 2,
  max_workspaces: 3,
  best_plan_key: "pro",
  status_summary: { active: 1, suspended: 1 },
  created_by: "u-9",
  created_at: "2026-10-01T00:00:00Z",
  last_activity_at: "2026-11-15T00:00:00Z",
  workspaces: [
    {
      id: "w-1",
      name: "Main workspace",
      slug: "main",
      status: "active",
      plan_key: "pro",
      member_count: 4,
      owner: { id: "u-1", email: "owner@example.com" },
      created_at: "2026-10-01T00:00:00Z",
      usage_day: "2026-11-14",
      usage_members: 3,
      usage_agents: 2,
      usage_tasks_run: 9,
      usage_storage_bytes: 2048,
    },
    {
      id: "w-2",
      name: "Side workspace",
      slug: "side",
      status: "suspended",
      plan_key: "free",
      member_count: 1,
      owner: null,
      created_at: "2026-10-02T00:00:00Z",
    },
  ],
};

function loaded(data: unknown) {
  mockUseTenant.mockReturnValue({ data, isLoading: false, isError: false, refetch: vi.fn() });
}

describe("TenantDetail screen", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("lists workspaces and links to the workspace detail", () => {
    loaded(tenant);
    const onSelect = vi.fn();
    render(<TenantDetail tenantId="t-1" onBack={vi.fn()} onSelectWorkspace={onSelect} />);

    expect(mockUseTenant).toHaveBeenCalledWith("t-1");
    expect(screen.getByRole("heading", { name: "Acme Group" })).toBeInTheDocument();
    const main = screen.getByText("Main workspace").closest("tr") as HTMLElement;
    expect(main).toHaveTextContent("main");
    expect(main).toHaveTextContent("pro");
    expect(main).toHaveTextContent("owner@example.com");
    expect(main).toHaveTextContent("3 members");
    fireEvent.click(within(main).getByRole("button", { name: "View" }));
    expect(onSelect).toHaveBeenCalledWith("w-1");

    const side = screen.getByText("Side workspace").closest("tr") as HTMLElement;
    expect(side).toHaveTextContent("free");
    expect(within(side).queryByText("owner@example.com")).not.toBeInTheDocument();
    fireEvent.click(within(side).getByRole("button", { name: "View" }));
    expect(onSelect).toHaveBeenLastCalledWith("w-2");
  });

  it("shows the tenant's workspace limit and the workspace count", () => {
    loaded(tenant);
    render(<TenantDetail tenantId="t-1" onBack={vi.fn()} onSelectWorkspace={vi.fn()} />);
    expect(screen.getByTestId("tenant-max-workspaces")).toHaveTextContent("3");
    expect(screen.getByTestId("tenant-workspace-count")).toHaveTextContent("2");
    expect(screen.getByTestId("tenant-status-summary")).toHaveTextContent("1 active, 1 suspended");
  });

  it("shows unlimited when max_workspaces is null", () => {
    loaded({ ...tenant, max_workspaces: null });
    render(<TenantDetail tenantId="t-1" onBack={vi.fn()} onSelectWorkspace={vi.fn()} />);
    expect(screen.getByTestId("tenant-max-workspaces")).toHaveTextContent("Unlimited");
  });

  it("shows a workspace with no usage row as an empty value, not zeros", () => {
    loaded(tenant);
    render(<TenantDetail tenantId="t-1" onBack={vi.fn()} onSelectWorkspace={vi.fn()} />);
    const side = screen.getByText("Side workspace").closest("tr") as HTMLElement;
    expect(within(side).getByTestId("workspace-usage").textContent).toBe("-");
    expect(within(screen.getByText("Main workspace").closest("tr") as HTMLElement).getByTestId("workspace-usage")).toHaveTextContent("2.0 KB");
  });

  it("goes back to the tenant list", () => {
    loaded(tenant);
    const onBack = vi.fn();
    render(<TenantDetail tenantId="t-1" onBack={onBack} onSelectWorkspace={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: /Back to tenants/i }));
    expect(onBack).toHaveBeenCalled();
  });

  it("handles loading, error and an empty tenant", () => {
    mockUseTenant.mockReturnValue({ data: undefined, isLoading: true, isError: false, refetch: vi.fn() });
    const { rerender } = render(<TenantDetail tenantId="t-1" onBack={vi.fn()} onSelectWorkspace={vi.fn()} />);
    expect(screen.getByText("Loading tenant...")).toBeInTheDocument();

    mockUseTenant.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      error: new Error("not found"),
      refetch: vi.fn(),
    });
    rerender(<TenantDetail tenantId="t-1" onBack={vi.fn()} onSelectWorkspace={vi.fn()} />);
    expect(screen.getByText("Failed to load tenant")).toBeInTheDocument();
    expect(screen.getByText("not found")).toBeInTheDocument();

    loaded({ ...tenant, workspaces: [], workspace_count: 0 });
    rerender(<TenantDetail tenantId="t-1" onBack={vi.fn()} onSelectWorkspace={vi.fn()} />);
    expect(screen.getByText("This tenant has no workspaces")).toBeInTheDocument();
  });
});
