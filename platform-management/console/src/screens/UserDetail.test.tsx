// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { UserDetail } from "./UserDetail";
import { PlatformApiError } from "../api/client";

const mockUseMe = vi.fn();
const mockUseUser = vi.fn();
const mockUseTenantUser = vi.fn();
const mockUseAccountDeletePreview = vi.fn();
const mockChangeUserStatusMutateAsync = vi.fn();
const mockRevokeUserSessionsMutateAsync = vi.fn();
const mockResetUserMfaMutateAsync = vi.fn();
const mockDeleteAccountNowMutateAsync = vi.fn();
const mockDeactivateTenantUserMutateAsync = vi.fn();
const mockRemoveWorkspaceMemberMutateAsync = vi.fn();
const mockChangeWorkspaceMemberRoleMutateAsync = vi.fn();
const mockCreateQuotaRequestMutateAsync = vi.fn();

vi.mock("../api/hooks", () => ({
  useMe: () => mockUseMe(),
  useUser: () => mockUseUser(),
  useTenantUser: (tenantId?: string, userId?: string, enabled?: boolean) =>
    mockUseTenantUser(tenantId, userId, enabled),
  useAccountDeletePreview: (id?: string, enabled?: boolean) =>
    mockUseAccountDeletePreview(id, enabled),
  useChangeUserStatus: () => ({
    mutateAsync: mockChangeUserStatusMutateAsync,
    isPending: false,
  }),
  useRevokeUserSessions: () => ({
    mutateAsync: mockRevokeUserSessionsMutateAsync,
    isPending: false,
  }),
  useResetUserMfa: () => ({
    mutateAsync: mockResetUserMfaMutateAsync,
    isPending: false,
  }),
  useDeleteAccountNow: () => ({
    mutateAsync: mockDeleteAccountNowMutateAsync,
    isPending: false,
  }),
  useDeactivateTenantUser: () => ({
    mutateAsync: mockDeactivateTenantUserMutateAsync,
    isPending: false,
  }),
  useRemoveWorkspaceMember: () => ({
    mutateAsync: mockRemoveWorkspaceMemberMutateAsync,
    isPending: false,
  }),
  useChangeWorkspaceMemberRole: () => ({
    mutateAsync: mockChangeWorkspaceMemberRoleMutateAsync,
    isPending: false,
  }),
  useCreateQuotaRequest: () => ({
    mutateAsync: mockCreateQuotaRequestMutateAsync,
    isPending: false,
  }),
}));

const mockActiveUser = {
  id: "u-active",
  email: "active@example.com",
  name: "Active User",
  status: "active",
  memberships_count: 2,
  created_at: "2026-10-15T08:00:00Z",
  memberships: [
    { workspace_id: "ws-1", name: "Engineering", role: "owner" },
    { workspace_id: "ws-2", name: "Product", role: "member" },
  ],
  account: {
    status: "active",
    reason: "",
    changed_at: null,
    sessions_valid_after: "2026-10-15T08:00:00Z",
  },
};

describe("UserDetail screen", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseMe.mockReturnValue({
      data: { user_id: "u-sys", email: "sys@example.com", role: "system_admin", tenant_id: null },
    });
    mockUseTenantUser.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: false,
    });
    mockUseAccountDeletePreview.mockReturnValue({
      data: {
        email: "active@example.com",
        need_choice: [],
        deleted_with: [],
      },
      isLoading: false,
      isError: false,
      error: null,
    });
  });

  it("renders active user account record and memberships", () => {
    mockUseUser.mockReturnValue({
      data: mockActiveUser,
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });

    render(<UserDetail userId="u-active" onBack={vi.fn()} />);

    expect(screen.getByRole("heading", { name: "Active User" })).toBeInTheDocument();
    expect(screen.getByText(/active@example.com • ID: u-active/i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Suspend account" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Sign out everywhere" })).not.toBeInTheDocument();
    expect(screen.getAllByText("Engineering")[0]).toBeInTheDocument();
    expect(screen.getAllByText("Product")[0]).toBeInTheDocument();
  });

  describe("R6 S24 tenant user actions, role rules and allocation", () => {
    const mockTenantUserDetail = {
      user_id: "u-active",
      email: "active@example.com",
      name: "Active User",
      account_status: "active" as const,
      is_staff: false,
      workspaces: [
        { workspace_id: "ws-1", name: "Engineering", member_role: "owner" as const },
        { workspace_id: "ws-2", name: "Product", member_role: "member" as const },
      ],
      seats: 2,
      last_login_at: "2026-11-18T10:00:00Z",
      allocation: [],
      audit_events: [],
    };

    it("tenant deactivate asks reason and lists the workspaces it removes the user from", async () => {
      mockUseUser.mockReturnValue({ data: mockActiveUser, isLoading: false, isError: false, refetch: vi.fn() });
      mockUseTenantUser.mockReturnValue({
        data: {
          ...mockTenantUserDetail,
          workspaces: [
            { workspace_id: "ws-1", name: "Engineering", member_role: "admin" },
            { workspace_id: "ws-2", name: "Product", member_role: "member" },
          ],
        },
        isLoading: false,
        isError: false,
      });
      mockDeactivateTenantUserMutateAsync.mockResolvedValueOnce({
        request_ids: ["mr-1", "mr-2"],
      });

      render(<UserDetail userId="u-active" tenantId="t-1" onBack={vi.fn()} userRole="admin" />);

      fireEvent.click(screen.getByRole("button", { name: "Deactivate in tenant" }));
      const dialog = screen.getByRole("dialog");
      expect(within(dialog).getByText("Engineering")).toBeInTheDocument();
      expect(within(dialog).getByText("Product")).toBeInTheDocument();

      const confirmBtn = within(dialog).getByRole("button", { name: "Deactivate in tenant" });
      expect(confirmBtn).toBeDisabledControl();

      const reasonInput = within(dialog).getByLabelText(/Reason/i);
      fireEvent.change(reasonInput, { target: { value: "   " } });
      expect(confirmBtn).toBeDisabledControl();

      fireEvent.change(reasonInput, { target: { value: "Contractor offboarding" } });
      expect(confirmBtn).not.toBeDisabledControl();
      fireEvent.click(confirmBtn);

      await waitFor(() => {
        expect(mockDeactivateTenantUserMutateAsync).toHaveBeenCalledWith({
          tenantId: "t-1",
          userId: "u-active",
          reason: "Contractor offboarding",
        });
      });
    });

    it("keeps deactivate dialog open and shows mapped error when owner blocks deactivation", async () => {
      mockUseUser.mockReturnValue({ data: mockActiveUser, isLoading: false, isError: false, refetch: vi.fn() });
      mockUseTenantUser.mockReturnValue({
        data: mockTenantUserDetail,
        isLoading: false,
        isError: false,
      });
      mockDeactivateTenantUserMutateAsync.mockRejectedValueOnce(
        new PlatformApiError(409, "owner_role_immutable", "an owner cannot be removed by staff"),
      );

      render(<UserDetail userId="u-active" tenantId="t-1" onBack={vi.fn()} userRole="admin" />);

      fireEvent.click(screen.getByRole("button", { name: "Deactivate in tenant" }));
      const dialog = screen.getByRole("dialog");
      const reasonInput = within(dialog).getByLabelText(/Reason/i);
      fireEvent.change(reasonInput, { target: { value: "Departure" } });
      fireEvent.click(within(dialog).getByRole("button", { name: "Deactivate in tenant" }));

      expect(await within(dialog).findByRole("alert")).toHaveTextContent(/owner/i);
      expect(screen.getByRole("dialog")).toBeInTheDocument();
      expect(reasonInput).toHaveValue("Departure");
    });

    it("change role offers admin member viewer only", async () => {
      const user = userEvent.setup();
      mockUseUser.mockReturnValue({ data: mockActiveUser, isLoading: false, isError: false, refetch: vi.fn() });
      mockUseTenantUser.mockReturnValue({
        data: mockTenantUserDetail,
        isLoading: false,
        isError: false,
      });
      mockChangeWorkspaceMemberRoleMutateAsync.mockResolvedValueOnce({
        request_id: "mr-9",
        kind: "change_role",
      });

      render(<UserDetail userId="u-active" tenantId="t-1" onBack={vi.fn()} userRole="admin" />);

      const ownerRow = screen.getAllByText("Engineering")[0]!.closest("tr") as HTMLElement;
      expect(within(ownerRow).getByRole("button", { name: "Change role" })).toBeDisabledControl();
      expect(within(ownerRow).getByRole("button", { name: "Remove" })).toBeDisabledControl();

      const memberRow = screen.getAllByText("Product")[0]!.closest("tr") as HTMLElement;
      await user.click(within(memberRow).getByRole("button", { name: "Change role" }));

      const dialog = screen.getByRole("dialog");
      await user.click(within(dialog).getByRole("combobox", { name: /Member role/i }));
      const options = await screen.findAllByRole("option");
      const labels = options.map((o) => o.textContent);
      expect(labels).toEqual(["Admin", "Member", "Viewer"]);
      expect(labels).not.toContain("Owner");

      await user.click(options.find((o) => o.textContent === "Viewer")!);
      await user.click(within(dialog).getByRole("button", { name: "Save role" }));

      await waitFor(() => {
        expect(mockChangeWorkspaceMemberRoleMutateAsync).toHaveBeenCalledWith({
          workspaceId: "ws-2",
          userId: "u-active",
          role: "viewer",
          tenantId: "t-1",
        });
      });
    });

    it("prevents changing own role and removes a member from a workspace", async () => {
      const user = userEvent.setup();
      mockUseMe.mockReturnValue({
        data: { user_id: "u-active", email: "active@example.com", role: "admin", tenant_id: "t-1" },
      });
      mockUseUser.mockReturnValue({ data: mockActiveUser, isLoading: false, isError: false, refetch: vi.fn() });
      mockUseTenantUser.mockReturnValue({
        data: mockTenantUserDetail,
        isLoading: false,
        isError: false,
      });
      mockRemoveWorkspaceMemberMutateAsync.mockResolvedValueOnce({
        request_id: "mr-11",
        kind: "remove",
      });

      const { rerender } = render(
        <UserDetail userId="u-active" tenantId="t-1" onBack={vi.fn()} userRole="admin" />,
      );

      const memberRow = screen.getAllByText("Product")[0]!.closest("tr") as HTMLElement;
      expect(within(memberRow).getByRole("button", { name: "Change role" })).toBeDisabledControl();

      mockUseMe.mockReturnValue({
        data: { user_id: "u-other-admin", email: "other@example.com", role: "admin", tenant_id: "t-1" },
      });
      rerender(<UserDetail userId="u-active" tenantId="t-1" onBack={vi.fn()} userRole="admin" />);

      await user.click(within(memberRow).getByRole("button", { name: "Remove" }));
      const dialog = screen.getByRole("dialog");
      expect(within(dialog).getByText(/Product/)).toBeInTheDocument();
      await user.click(within(dialog).getByRole("button", { name: "Remove member" }));

      await waitFor(() => {
        expect(mockRemoveWorkspaceMemberMutateAsync).toHaveBeenCalledWith({
          workspaceId: "ws-2",
          userId: "u-active",
          tenantId: "t-1",
        });
      });
    });

    it("global lock and delete hidden for admin", () => {
      mockUseMe.mockReturnValue({
        data: { user_id: "u-adm", email: "adm@example.com", role: "admin", tenant_id: "t-1" },
      });
      mockUseUser.mockReturnValue({ data: mockActiveUser, isLoading: false, isError: false, refetch: vi.fn() });
      mockUseTenantUser.mockReturnValue({
        data: mockTenantUserDetail,
        isLoading: false,
        isError: false,
      });

      render(<UserDetail userId="u-active" tenantId="t-1" onBack={vi.fn()} userRole="admin" />);

      expect(screen.queryByRole("button", { name: "Suspend account" })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Reinstate account" })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Sign out everywhere" })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Reset two-step sign-in" })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Delete account" })).not.toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Deactivate in tenant" })).toBeInTheDocument();
    });

    it("allocation opens the workspace action not a per-user quota", async () => {
      const user = userEvent.setup();
      mockUseUser.mockReturnValue({ data: mockActiveUser, isLoading: false, isError: false, refetch: vi.fn() });
      mockUseTenantUser.mockReturnValue({
        data: mockTenantUserDetail,
        isLoading: false,
        isError: false,
      });

      const onSelectWorkspace = vi.fn();
      const { unmount } = render(
        <UserDetail
          userId="u-active"
          tenantId="t-1"
          onBack={vi.fn()}
          onSelectWorkspace={onSelectWorkspace}
          userRole="billing"
        />,
      );

      expect(screen.queryByLabelText(/User quota/i)).not.toBeInTheDocument();
      const allocBtns = screen.getAllByRole("button", { name: /Workspace allocation/i });
      await user.click(allocBtns[0]!);
      expect(onSelectWorkspace).toHaveBeenCalledWith("ws-1");
      expect(screen.queryByRole("button", { name: /Request quota increase/i })).not.toBeInTheDocument();
      unmount();

      // Admin has no direct subscription/allocation route; opens workspace quota request form instead
      mockUseMe.mockReturnValue({
        data: { user_id: "u-adm", email: "adm@example.com", role: "admin", tenant_id: "t-1" },
      });
      mockCreateQuotaRequestMutateAsync.mockResolvedValueOnce({
        id: "qr-1",
        workspace_id: "ws-1",
        tenant_id: "t-1",
        gate: "agents",
        current_value: 5,
        requested_value: 15,
        approved_value: null,
        state: "pending",
        reason: "Need more agents",
        decision_reason: "",
        requested_by: "u-adm",
        requested_by_label: "adm@example.com",
        decided_by: "",
        created_at: "2026-11-19T00:00:00Z",
        decided_at: null,
      });

      render(
        <UserDetail
          userId="u-active"
          tenantId="t-1"
          onBack={vi.fn()}
          onSelectWorkspace={onSelectWorkspace}
          userRole="admin"
        />,
      );

      expect(screen.queryByRole("button", { name: /Workspace allocation/i })).not.toBeInTheDocument();
      const requestBtns = screen.getAllByRole("button", { name: /Request quota increase/i });
      await user.click(requestBtns[0]!);

      const dialog = screen.getByRole("dialog");
      expect(within(dialog).getByText(/Engineering/)).toBeInTheDocument();
      await user.click(within(dialog).getByRole("combobox", { name: /Resource gate/i }));
      await user.click(await screen.findByRole("option", { name: "Agents" }));
      fireEvent.change(within(dialog).getByLabelText(/Requested limit/i), { target: { value: "15" } });
      fireEvent.blur(within(dialog).getByLabelText(/Requested limit/i));
      fireEvent.change(within(dialog).getByLabelText(/Reason/i), { target: { value: "Need more agents" } });
      await user.click(within(dialog).getByRole("button", { name: "Submit request" }));

      await waitFor(() => {
        expect(mockCreateQuotaRequestMutateAsync).toHaveBeenCalledWith({
          workspaceId: "ws-1",
          gate: "agents",
          requested_value: 15,
          reason: "Need more agents",
        });
      });
    });
  });
});
