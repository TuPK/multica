// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TenantUsers } from "./TenantUsers";

const mockUseMe = vi.fn();
const mockUseTenantUsers = vi.fn();
const mockFetchNextPage = vi.fn();

vi.mock("../api/hooks", () => ({
  useExportWorkspace: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useChangeWorkspaceStatus: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateSubscription: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useAddWorkspaceMember: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useDeleteWorkspaceNow: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useCreateQuotaRequest: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useWorkspace: () => ({ data: undefined, isLoading: false, isError: false, refetch: vi.fn() }),
  useSetRuntimeStatus: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUnregisterRuntime: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useDeactivateTenantUser: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useMe: () => mockUseMe(),
  useTenantUsers: (tenantId: string | undefined, filters: Record<string, unknown>) =>
    mockUseTenantUsers(tenantId, filters),
}));

const sampleTenantUsers = [
  {
    user_id: "u-1",
    email: "alice@acme.test",
    name: "Alice Nguyen",
    account_status: "active" as const,
    is_staff: false,
    workspaces: [
      { workspace_id: "ws-1", name: "Engineering", member_role: "owner" as const },
      { workspace_id: "ws-2", name: "Product", member_role: "member" as const },
    ],
    seats: 2,
    last_login_at: "2026-11-18T09:30:00Z",
  },
  {
    user_id: "u-2",
    email: "bob@acme.test",
    name: "Bob Tran",
    account_status: "suspended" as const,
    is_staff: false,
    workspaces: [{ workspace_id: "ws-2", name: "Product", member_role: "admin" as const }],
    seats: 1,
    last_login_at: null,
  },
];

const sampleWorkspaces = [
  { id: "ws-1", name: "Engineering" },
  { id: "ws-2", name: "Product" },
];

describe("TenantUsers screen", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseMe.mockReturnValue({
      data: { user_id: "u-sys", email: "sys@example.com", role: "system_admin", tenant_id: null },
    });
    mockUseTenantUsers.mockReturnValue({
      data: sampleTenantUsers,
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
      hasNextPage: false,
      fetchNextPage: mockFetchNextPage,
      isFetchingNextPage: false,
    });
  });

  it("lists users with workspaces roles status seats and last login", () => {
    const onSelectUser = vi.fn();
    render(
      <TenantUsers
        tenantId="t-1"
        workspaces={sampleWorkspaces}
        onSelectUser={onSelectUser}
        userRole="system_admin"
      />,
    );

    expect(mockUseTenantUsers).toHaveBeenCalledWith("t-1", {
      q: undefined,
      account_status: undefined,
      workspace_id: undefined,
      member_role: undefined,
    });

    for (const header of ["User", "Workspaces & Roles", "Status", "Seats", "Last login", "Action"]) {
      expect(screen.getByRole("columnheader", { name: header })).toBeInTheDocument();
    }

    const aliceRow = screen.getByText("alice@acme.test").closest("tr") as HTMLElement;
    expect(aliceRow).toHaveTextContent("Alice Nguyen");
    expect(aliceRow).toHaveTextContent("Engineering");
    expect(aliceRow).toHaveTextContent("Owner");
    expect(aliceRow).toHaveTextContent("Product");
    expect(aliceRow).toHaveTextContent("Member");
    expect(aliceRow).toHaveTextContent("Active");
    expect(aliceRow).toHaveTextContent("2");
    expect(aliceRow).toHaveTextContent(new Date("2026-11-18T09:30:00Z").toLocaleString());

    const bobRow = screen.getByText("bob@acme.test").closest("tr") as HTMLElement;
    expect(bobRow).toHaveTextContent("Bob Tran");
    expect(bobRow).toHaveTextContent("Product");
    expect(bobRow).toHaveTextContent("Admin");
    expect(bobRow).toHaveTextContent("Suspended");
    expect(bobRow).toHaveTextContent("1");
    expect(within(bobRow).getAllByText("-").length).toBeGreaterThanOrEqual(1);

    fireEvent.click(aliceRow);
    expect(onSelectUser).toHaveBeenCalledWith("u-1");
  });

  it("search filter and paging", async () => {
    const user = userEvent.setup();
    mockUseTenantUsers.mockReturnValue({
      data: sampleTenantUsers,
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
      hasNextPage: true,
      fetchNextPage: mockFetchNextPage,
      isFetchingNextPage: false,
    });

    render(
      <TenantUsers
        tenantId="t-1"
        workspaces={sampleWorkspaces}
        onSelectUser={vi.fn()}
        userRole="admin"
      />,
    );

    const searchInput = screen.getByRole("searchbox", { name: /Search users/i });
    fireEvent.change(searchInput, { target: { value: "alice" } });
    expect(mockUseTenantUsers).toHaveBeenLastCalledWith("t-1", {
      q: "alice",
      account_status: undefined,
      workspace_id: undefined,
      member_role: undefined,
    });

    await user.click(screen.getByRole("combobox", { name: /Filter by status/i }));
    await user.click(await screen.findByRole("option", { name: "Suspended" }));
    expect(mockUseTenantUsers).toHaveBeenLastCalledWith("t-1", {
      q: "alice",
      account_status: "suspended",
      workspace_id: undefined,
      member_role: undefined,
    });
    expect(screen.queryByText("alice@acme.test")).not.toBeInTheDocument();
    expect(screen.getByText("bob@acme.test")).toBeInTheDocument();

    await user.click(screen.getByRole("combobox", { name: /Filter by workspace/i }));
    await user.click(await screen.findByRole("option", { name: "Product" }));
    expect(mockUseTenantUsers).toHaveBeenLastCalledWith("t-1", {
      q: "alice",
      account_status: "suspended",
      workspace_id: "ws-2",
      member_role: undefined,
    });

    await user.click(screen.getByRole("combobox", { name: /Filter by role/i }));
    await user.click(await screen.findByRole("option", { name: "Admin" }));
    expect(mockUseTenantUsers).toHaveBeenLastCalledWith("t-1", {
      q: "alice",
      account_status: "suspended",
      workspace_id: "ws-2",
      member_role: "admin",
    });
    expect(screen.getByText("bob@acme.test")).toBeInTheDocument();

    const loadMoreBtn = screen.getByRole("button", { name: /Load more/i });
    fireEvent.click(loadMoreBtn);
    expect(mockFetchNextPage).toHaveBeenCalledTimes(1);

    await user.click(screen.getByRole("button", { name: /Clear filters/i }));
    expect(mockUseTenantUsers).toHaveBeenLastCalledWith("t-1", {
      q: undefined,
      account_status: undefined,
      workspace_id: undefined,
      member_role: undefined,
    });
    expect(screen.getByText("alice@acme.test")).toBeInTheDocument();
  });

  it("handles loading, error, and empty states", () => {
    mockUseTenantUsers.mockReturnValue({
      data: [],
      isLoading: true,
      isError: false,
      refetch: vi.fn(),
      hasNextPage: false,
      fetchNextPage: mockFetchNextPage,
      isFetchingNextPage: false,
    });
    const { rerender } = render(
      <TenantUsers tenantId="t-1" onSelectUser={vi.fn()} userRole="system_admin" />,
    );
    expect(screen.getByText("Loading tenant users...")).toBeInTheDocument();

    mockUseTenantUsers.mockReturnValue({
      data: [],
      isLoading: false,
      isError: true,
      error: new Error("Tenant unavailable"),
      refetch: vi.fn(),
      hasNextPage: false,
      fetchNextPage: mockFetchNextPage,
      isFetchingNextPage: false,
    });
    rerender(<TenantUsers tenantId="t-1" onSelectUser={vi.fn()} userRole="system_admin" />);
    expect(screen.getByText("Failed to load tenant users")).toBeInTheDocument();
    expect(screen.getByText("Tenant unavailable")).toBeInTheDocument();

    mockUseTenantUsers.mockReturnValue({
      data: [],
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
      hasNextPage: false,
      fetchNextPage: mockFetchNextPage,
      isFetchingNextPage: false,
    });
    rerender(<TenantUsers tenantId="t-1" onSelectUser={vi.fn()} userRole="system_admin" />);
    expect(screen.getByText("No users found")).toBeInTheDocument();
  });
});
