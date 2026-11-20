// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TenantDetail } from "./TenantDetail";

const mockUseMe = vi.fn();
const mockUseTenant = vi.fn();
const mockUseTenantUsers = vi.fn();
const mockUseTenantResources = vi.fn();
const mockSetTenantLimitOverrideMutateAsync = vi.fn();
const mockDeleteTenantNowMutateAsync = vi.fn();
const mockSetTenantStatusMutateAsync = vi.fn();
const mockUseLegalHolds = vi.fn();

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
  useTenant: (id?: string) => mockUseTenant(id),
  useTenantUsers: (tenantId?: string, filters?: Record<string, unknown>) =>
    mockUseTenantUsers(tenantId, filters),
  useTenantResources: (tenantId?: string, enabled = true) =>
    mockUseTenantResources(tenantId, enabled),
  useSetTenantLimitOverride: () => ({
    mutateAsync: mockSetTenantLimitOverrideMutateAsync,
    isPending: false,
  }),
  useDeleteTenantNow: () => ({
    mutateAsync: mockDeleteTenantNowMutateAsync,
    isPending: false,
  }),
  useLegalHolds: () => mockUseLegalHolds(),
  useSetTenantStatus: () => ({
    mutateAsync: mockSetTenantStatusMutateAsync,
    isPending: false,
  }),
  useTenants: () => ({
    data: [],
    isLoading: false,
    isError: false,
  }),
  useTenantPolicy: () => ({
    data: {
      policy: {
        allowed_invite_domains: [],
        share_links_enabled: true,
        require_sso: false,
        require_mfa: false,
        session_max_age_hours: 0,
      },
      version: 1,
      sources: {},
      tenant_fields: [],
    },
    isLoading: false,
    isError: false,
    error: null,
    refetch: vi.fn(),
  }),
  useUpdateTenantPolicy: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
  }),
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
    mockUseLegalHolds.mockReturnValue({ data: [], isLoading: false, isError: false });
    mockUseMe.mockReturnValue({ data: { user_id: "u-sys", email: "sys@example.com", role: "system_admin", tenant_id: null } });
    mockUseTenantUsers.mockReturnValue({
      data: [],
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
      hasNextPage: false,
      fetchNextPage: vi.fn(),
      isFetchingNextPage: false,
    });
    mockUseTenantResources.mockReturnValue({
      data: {
        tenant_id: "t-1",
        resources: [],
        workspaces: [],
        complete: true,
      },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });
  });

  it("lists workspaces and links to the workspace detail", () => {
    loaded(tenant);
    const onSelect = vi.fn();
    render(<TenantDetail tenantId="t-1" onBack={vi.fn()} onSelectWorkspace={onSelect} />);

    expect(mockUseTenant).toHaveBeenCalledWith("t-1");
    expect(screen.getByRole("heading", { name: "Acme Group" })).toBeInTheDocument();
    const main = screen.getAllByText("Main workspace")[0]!.closest("tr") as HTMLElement;
    expect(main).toHaveTextContent("main");
    expect(main).toHaveTextContent("pro");
    expect(main).toHaveTextContent("owner@example.com");
    expect(main).toHaveTextContent("3 members");
    fireEvent.click(main);
    expect(onSelect).toHaveBeenCalledWith("w-1");

    const side = screen.getAllByText("Side workspace")[0]!.closest("tr") as HTMLElement;
    expect(side).toHaveTextContent("free");
    expect(within(side).queryByText("owner@example.com")).not.toBeInTheDocument();
    fireEvent.click(side);
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
    const side = screen.getAllByText("Side workspace")[0]!.closest("tr") as HTMLElement;
    expect(within(side).getByTestId("workspace-usage").textContent).toBe("-");
    expect(within(screen.getAllByText("Main workspace")[0]!.closest("tr") as HTMLElement).getByTestId("workspace-usage")).toHaveTextContent("2.0 KB");
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

  it("switches to the Users tab and renders TenantUsers", async () => {
    const user = userEvent.setup();
    loaded(tenant);
    mockUseTenantUsers.mockReturnValue({
      data: [
        {
          user_id: "u-1",
          email: "member@acme.test",
          name: "Member One",
          account_status: "active",
          is_staff: false,
          workspaces: [{ workspace_id: "w-1", name: "Main workspace", member_role: "member" }],
          seats: 1,
          last_login_at: null,
        },
      ],
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
      hasNextPage: false,
      fetchNextPage: vi.fn(),
      isFetchingNextPage: false,
    });

    const onSelectUser = vi.fn();
    render(
      <TenantDetail
        tenantId="t-1"
        onBack={vi.fn()}
        onSelectWorkspace={vi.fn()}
        onSelectUser={onSelectUser}
      />,
    );

    const usersTab = screen.getByRole("tab", { name: /Users/i });
    await user.click(usersTab);

    expect(mockUseTenantUsers).toHaveBeenCalledWith("t-1", {
      q: undefined,
      account_status: undefined,
      workspace_id: undefined,
      member_role: undefined,
    });
    expect(screen.getByText("member@acme.test")).toBeInTheDocument();

    fireEvent.click(screen.getByText("member@acme.test").closest("tr")!);
    expect(onSelectUser).toHaveBeenCalledWith("u-1");
  });

  it("resource bars show used limit percent and warning over states", async () => {
    const user = userEvent.setup();
    loaded(tenant);
    mockUseTenantResources.mockReturnValue({
      data: {
        tenant_id: "t-1",
        complete: true,
        resources: [
          { type: "members", label: "Members", unit: "count", used: 3, limit: 10, percent: 30, state: "ok", trend_7d: [] },
          { type: "agents", label: "Agents", unit: "count", used: 8, limit: 10, percent: 80, state: "warning", trend_7d: [] },
          { type: "runtimes", label: "Runtimes", unit: "count", used: 12, limit: 10, percent: 120, state: "over", trend_7d: [] },
          { type: "issue_count", label: "Issues", unit: "count", used: 50, limit: null, percent: null, state: "ok", trend_7d: [] },
        ],
        workspaces: [
          {
            workspace_id: "w-1",
            workspace_name: "Main workspace",
            plan_key: "pro",
            resources: [
              { type: "agents", label: "Agents", unit: "count", used: 8, limit: 10, percent: 80, state: "warning", trend_7d: [] },
            ],
          },
        ],
      },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });

    render(
      <TenantDetail
        tenantId="t-1"
        onBack={vi.fn()}
        onSelectWorkspace={vi.fn()}
        userRole="system_admin"
      />,
    );

    await user.click(screen.getByRole("tab", { name: /Resources/i }));

    expect(mockUseTenantResources).toHaveBeenCalledWith("t-1", true);

    const membersRow = screen.getByTestId("resource-row-members");
    expect(membersRow).toHaveTextContent("3 / 10");
    expect(membersRow).toHaveTextContent("30%");
    expect(membersRow).toHaveTextContent("OK");

    const agentsRow = screen.getByTestId("resource-row-agents");
    expect(agentsRow).toHaveTextContent("8 / 10");
    expect(agentsRow).toHaveTextContent("80%");
    expect(agentsRow).toHaveTextContent("Warning");
    expect(agentsRow).toHaveAttribute("data-resource-state", "warning");

    const runtimesRow = screen.getByTestId("resource-row-runtimes");
    expect(runtimesRow).toHaveTextContent("12 / 10");
    expect(runtimesRow).toHaveTextContent("120%");
    expect(runtimesRow).toHaveTextContent("Over limit");
    expect(runtimesRow).toHaveAttribute("data-resource-state", "over");

    const issuesRow = screen.getByTestId("resource-row-issue_count");
    expect(issuesRow).toHaveTextContent("50 / Unlimited");
  });

  it("limit override form hidden for admin", async () => {
    const user = userEvent.setup();
    loaded(tenant);

    const { unmount } = render(
      <TenantDetail
        tenantId="t-1"
        onBack={vi.fn()}
        onSelectWorkspace={vi.fn()}
        userRole="admin"
      />,
    );

    await user.click(screen.getByRole("tab", { name: /Resources/i }));
    expect(screen.queryByRole("button", { name: /Set extra limit/i })).not.toBeInTheDocument();
    unmount();

    mockUseMe.mockReturnValue({
      data: { user_id: "u-supp", email: "support@example.com", role: "support", tenant_id: null },
    });
    const { unmount: unmountSupp } = render(
      <TenantDetail
        tenantId="t-1"
        onBack={vi.fn()}
        onSelectWorkspace={vi.fn()}
        userRole="support"
      />,
    );
    await user.click(screen.getByRole("tab", { name: /Resources/i }));
    expect(screen.queryByRole("button", { name: /Set extra limit/i })).not.toBeInTheDocument();
    unmountSupp();
  });

  it("allows system_admin and billing to set and release limit overrides", async () => {
    const user = userEvent.setup();
    loaded(tenant);
    mockSetTenantLimitOverrideMutateAsync.mockResolvedValue({
      gate: "agents",
      workspace_ids: ["w-1", "w-2"],
    });

    render(
      <TenantDetail
        tenantId="t-1"
        onBack={vi.fn()}
        onSelectWorkspace={vi.fn()}
        userRole="billing"
      />,
    );

    await user.click(screen.getByRole("tab", { name: /Resources/i }));
    const openBtn = screen.getByRole("button", { name: /Set extra limit/i });
    expect(openBtn).toBeInTheDocument();
    await user.click(openBtn);

    const dialog = screen.getByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText(/Extra limit value/i), {
      target: { value: "25" },
    });
    fireEvent.blur(within(dialog).getByLabelText(/Extra limit value/i));
    fireEvent.change(within(dialog).getByLabelText(/Reason/i), {
      target: { value: "Enterprise add-on" },
    });
    await user.click(within(dialog).getByRole("button", { name: /Save limit/i }));

    await waitFor(() => {
      expect(mockSetTenantLimitOverrideMutateAsync).toHaveBeenCalledWith({
        tenantId: "t-1",
        gate: "agents",
        value: 25,
        workspace_ids: undefined,
        reason: "Enterprise add-on",
      });
    });

    // Release override (value: null)
    await user.click(screen.getByRole("button", { name: /Set extra limit/i }));
    const dialog2 = screen.getByRole("dialog");
    await user.click(within(dialog2).getByLabelText(/Release limit/i));
    fireEvent.change(within(dialog2).getByLabelText(/Reason/i), {
      target: { value: "Contract expired" },
    });
    await user.click(within(dialog2).getByRole("button", { name: /Save limit/i }));

    await waitFor(() => {
      expect(mockSetTenantLimitOverrideMutateAsync).toHaveBeenLastCalledWith({
        tenantId: "t-1",
        gate: "agents",
        value: null,
        workspace_ids: undefined,
        reason: "Contract expired",
      });
    });
  });

  it("shows Policy tab and legal hold mark", async () => {
    const user = userEvent.setup();
    mockUseLegalHolds.mockReturnValue({
      data: [
        {
          id: "lh-1",
          scope_kind: "tenant",
          scope_id: "t-1",
          reason: "Subpoena",
          case_ref: "CASE-1",
          placed_by: "u-sys",
          released: false,
          released_by: "",
          placed_at: null,
          released_at: null,
        },
      ],
      isLoading: false,
      isError: false,
    });
    mockSetTenantStatusMutateAsync.mockResolvedValueOnce({
      status: "suspended",
      workspace_ids: ["ws-1"],
    });

    const { rerender } = render(
      <TenantDetail
        tenantId="t-1"
        onBack={vi.fn()}
        onSelectWorkspace={vi.fn()}
        userRole="system_admin"
      />,
    );

    expect(screen.getByText("Legal hold")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Lock tenant|Delete tenant/i })).not.toBeInTheDocument();

    await user.click(screen.getByRole("tab", { name: /Policy/i }));
    expect(screen.getByRole("heading", { name: /Effective policy/i })).toBeInTheDocument();

    mockUseMe.mockReturnValue({
      data: { user_id: "u-admin", email: "admin@example.com", role: "admin", tenant_id: "t-1" },
    });
    rerender(
      <TenantDetail
        tenantId="t-1"
        onBack={vi.fn()}
        onSelectWorkspace={vi.fn()}
        userRole="admin"
      />,
    );
    expect(screen.queryByText("Legal hold")).not.toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /Policy/i })).toBeInTheDocument();
  });
});
