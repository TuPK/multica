// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { UserAccountMenu } from "./UserAccountMenu";

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

const mockSuspendedUser = {
  ...mockActiveUser,
  id: "u-suspended",
  email: "suspended@example.com",
  name: "Suspended User",
  status: "suspended",
  account: {
    status: "suspended",
    reason: "Fraud investigation",
    changed_at: "2026-11-01T12:00:00Z",
    sessions_valid_after: null,
  },
};

function AccountMenuHarness({ userRole }: { userId: string; userRole?: string; onBack?: () => void }) {
  const { data: user } = mockUseUser() as { data: typeof mockActiveUser };
  const role = userRole ?? mockUseMe().data.role;
  const accountStatus = user.account?.status || user.status;
  return (
    <UserAccountMenu
      userId={user.id}
      email={user.email}
      accountStatus={accountStatus}
      canControlAccount={role === "system_admin" || role === "support"}
      canDeleteAccount={role === "system_admin" && accountStatus !== "deleted"}
    />
  );
}

describe("user account menu actions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseMe.mockReturnValue({
      data: { user_id: "u-sys", email: "sys@example.com", role: "system_admin", tenant_id: null },
    });
    mockUseTenantUser.mockReturnValue({ data: undefined, isLoading: false, isError: false });
    mockUseAccountDeletePreview.mockReturnValue({
      data: { email: "active@example.com", need_choice: [], deleted_with: [] },
      isLoading: false,
      isError: false,
      error: null,
    });
  });

  it("renders reinstate button for suspended user", () => {
    mockUseUser.mockReturnValue({
      data: mockSuspendedUser,
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });

    render(<AccountMenuHarness userId="u-suspended" onBack={vi.fn()} />);

    expect(screen.getByRole("button", { name: "Reinstate account" })).toBeInTheDocument();
  });

  it("the reason is required before account suspend can be confirmed", async () => {
    mockUseUser.mockReturnValue({
      data: mockActiveUser,
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });
    mockChangeUserStatusMutateAsync.mockResolvedValue({ changed: true });

    render(<AccountMenuHarness userId="u-active" onBack={vi.fn()} />);

    fireEvent.click(screen.getByRole("button", { name: "Suspend account" }));
    expect(screen.getByRole("heading", { name: "Suspend user account" })).toBeInTheDocument();

    const confirmBtn = within(screen.getByRole("dialog")).getByRole("button", { name: "Suspend account" });
    expect(confirmBtn).toBeDisabledControl();

    const reasonInput = within(screen.getByRole("dialog")).getByLabelText(/Reason/i);
    fireEvent.change(reasonInput, { target: { value: "   " } });
    expect(confirmBtn).toBeDisabledControl();

    fireEvent.change(reasonInput, { target: { value: "Spam behavior detected" } });
    expect(confirmBtn).not.toBeDisabledControl();

    fireEvent.click(confirmBtn);

    await waitFor(() => {
      expect(mockChangeUserStatusMutateAsync).toHaveBeenCalledWith({
        userId: "u-active",
        status: "suspended",
        reason: "Spam behavior detected",
      });
    });
  });

  it("the reason is required before sign out everywhere can be confirmed", async () => {
    mockUseUser.mockReturnValue({
      data: mockActiveUser,
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });
    mockRevokeUserSessionsMutateAsync.mockResolvedValue({
      sessions_valid_after: "2026-11-15T12:00:00Z",
    });

    render(<AccountMenuHarness userId="u-active" onBack={vi.fn()} />);

    fireEvent.click(screen.getByRole("button", { name: "Sign out everywhere" }));
    expect(screen.getByRole("heading", { name: "Sign out everywhere" })).toBeInTheDocument();

    const confirmBtn = screen.getByRole("button", { name: "Revoke all sessions" });
    expect(confirmBtn).toBeDisabledControl();

    const reasonInput = within(screen.getByRole("dialog")).getByLabelText(/Reason/i);
    fireEvent.change(reasonInput, { target: { value: "Credential breach" } });
    expect(confirmBtn).not.toBeDisabledControl();

    fireEvent.click(confirmBtn);

    await waitFor(() => {
      expect(mockRevokeUserSessionsMutateAsync).toHaveBeenCalledWith({
        userId: "u-active",
        reason: "Credential breach",
      });
    });
  });

  it("resets the user's two-step sign-in only after a confirmation", async () => {
    mockUseUser.mockReturnValue({ data: mockActiveUser, isLoading: false, isError: false, refetch: vi.fn() });
    mockResetUserMfaMutateAsync.mockResolvedValue({ reset: true });

    render(<AccountMenuHarness userId="u-active" onBack={vi.fn()} />);

    fireEvent.click(screen.getByRole("button", { name: "Reset two-step sign-in" }));
    expect(mockResetUserMfaMutateAsync).not.toHaveBeenCalled();
    expect(screen.getByRole("heading", { name: "Reset two-step sign-in" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Reset and email the user" }));
    await waitFor(() => {
      expect(mockResetUserMfaMutateAsync).toHaveBeenCalledWith({ userId: "u-active" });
    });
  });

  it("shows the server's refusal when the reset fails", async () => {
    mockUseUser.mockReturnValue({ data: mockActiveUser, isLoading: false, isError: false, refetch: vi.fn() });
    mockResetUserMfaMutateAsync.mockRejectedValue(new Error("you cannot reset your own two-step sign-in"));

    render(<AccountMenuHarness userId="u-active" onBack={vi.fn()} />);

    fireEvent.click(screen.getByRole("button", { name: "Reset two-step sign-in" }));
    fireEvent.click(screen.getByRole("button", { name: "Reset and email the user" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("you cannot reset your own two-step sign-in");
  });

  it("delete account action is shown for system_admin only", () => {
    mockUseUser.mockReturnValue({ data: mockActiveUser, isLoading: false, isError: false, refetch: vi.fn() });

    for (const role of ["admin", "support", "billing"] as const) {
      mockUseMe.mockReturnValue({
        data: { user_id: `u-${role}`, email: `${role}@example.com`, role, tenant_id: role === "admin" ? "t-1" : null },
      });
      const { unmount } = render(<AccountMenuHarness userId="u-active" onBack={vi.fn()} userRole={role} />);
      expect(screen.queryByRole("button", { name: "Delete account" })).not.toBeInTheDocument();
      unmount();
    }

    mockUseMe.mockReturnValue({
      data: { user_id: "u-sys", email: "sys@example.com", role: "system_admin", tenant_id: null },
    });
    render(<AccountMenuHarness userId="u-active" onBack={vi.fn()} userRole="system_admin" />);
    expect(screen.getByRole("button", { name: "Delete account" })).toBeInTheDocument();
  });

  it("lists blocking workspaces and requires a choice for each before enabling submit", async () => {
    const user = userEvent.setup();
    mockUseUser.mockReturnValue({ data: mockActiveUser, isLoading: false, isError: false, refetch: vi.fn() });
    mockUseAccountDeletePreview.mockReturnValue({
      data: {
        email: "active@example.com",
        need_choice: [
          {
            workspace_id: "ws-1",
            slug: "engineering",
            role: "owner",
            owner_count: 1,
            member_count: 4,
            successors: [
              { user_id: "u-adm", email: "lead@example.com", name: "Lead", role: "admin", status: "active" },
              { user_id: "u-mem", email: "dev@example.com", name: "Dev", role: "member", status: "active" },
            ],
          },
          {
            workspace_id: "ws-2",
            slug: "side-proj",
            role: "owner",
            owner_count: 1,
            member_count: 2,
            successors: [],
          },
        ],
        deleted_with: [
          {
            workspace_id: "ws-solo",
            slug: "personal-ws",
            role: "owner",
            owner_count: 1,
            member_count: 1,
            successors: [],
          },
        ],
      },
      isLoading: false,
      isError: false,
      error: null,
    });
    mockDeleteAccountNowMutateAsync.mockResolvedValueOnce({ request_id: "dr-10", state: "pending" });

    render(<AccountMenuHarness userId="u-active" onBack={vi.fn()} userRole="system_admin" />);

    await user.click(screen.getByRole("button", { name: "Delete account" }));
    const dialog = screen.getByRole("dialog");

    expect(within(dialog).getByText("engineering")).toBeInTheDocument();
    expect(within(dialog).getByText("side-proj")).toBeInTheDocument();
    expect(within(dialog).getByText("personal-ws")).toBeInTheDocument();

    fireEvent.change(within(dialog).getByLabelText(/Reason/i), { target: { value: "Legal erasure" } });
    fireEvent.change(within(dialog).getByLabelText(/Account email/i), { target: { value: "active@example.com" } });

    const submitBtn = within(dialog).getByRole("button", { name: "Delete account" });
    // Still disabled because neither blocking workspace is resolved
    expect(submitBtn).toBeDisabledControl();

    // Choose Transfer for ws-1 (still disabled without successor AND ws-2 unresolved)
    const ws1Group = within(dialog).getByRole("radiogroup", { name: /Resolution for engineering/i });
    await user.click(within(ws1Group).getByRole("radio", { name: /Transfer ownership/i }));
    expect(submitBtn).toBeDisabledControl();

    // Pick successor for ws-1
    await user.click(within(dialog).getByRole("combobox", { name: /Successor for engineering/i }));
    await user.click(await screen.findByRole("option", { name: /Lead/i }));
    expect(submitBtn).toBeDisabledControl(); // ws-2 still unresolved

    // Choose Delete for ws-2 -> submit now enabled!
    const ws2Group = within(dialog).getByRole("radiogroup", { name: /Resolution for side-proj/i });
    await user.click(within(ws2Group).getByRole("radio", { name: /Delete workspace now/i }));
    expect(submitBtn).not.toBeDisabledControl();

    await user.click(submitBtn);
    await waitFor(() => {
      expect(mockDeleteAccountNowMutateAsync).toHaveBeenCalledWith({
        userId: "u-active",
        reason: "Legal erasure",
        confirm: "active@example.com",
        resolutions: [
          { workspace_id: "ws-1", action: "transfer", successor_user_id: "u-adm" },
          { workspace_id: "ws-2", action: "delete" },
        ],
      });
    });
  });

  it("successor list excludes viewers", async () => {
    const user = userEvent.setup();
    mockUseUser.mockReturnValue({ data: mockActiveUser, isLoading: false, isError: false, refetch: vi.fn() });
    mockUseAccountDeletePreview.mockReturnValue({
      data: {
        email: "active@example.com",
        need_choice: [
          {
            workspace_id: "ws-1",
            slug: "engineering",
            role: "owner",
            owner_count: 1,
            member_count: 4,
            successors: [
              { user_id: "u-adm", email: "lead@example.com", name: "Lead Admin", role: "admin", status: "active" },
              { user_id: "u-mem", email: "dev@example.com", name: "Dev Member", role: "member", status: "active" },
              { user_id: "u-view", email: "guest@example.com", name: "Guest Viewer", role: "viewer", status: "active" },
            ],
          },
        ],
        deleted_with: [],
      },
      isLoading: false,
      isError: false,
      error: null,
    });

    render(<AccountMenuHarness userId="u-active" onBack={vi.fn()} userRole="system_admin" />);
    await user.click(screen.getByRole("button", { name: "Delete account" }));
    const dialog = screen.getByRole("dialog");

    const ws1Group = within(dialog).getByRole("radiogroup", { name: /Resolution for engineering/i });
    await user.click(within(ws1Group).getByRole("radio", { name: /Transfer ownership/i }));
    await user.click(within(dialog).getByRole("combobox", { name: /Successor for engineering/i }));

    const options = await screen.findAllByRole("option");
    const labels = options.map((o) => o.textContent);
    expect(labels.some((t) => t?.includes("Lead Admin"))).toBe(true);
    expect(labels.some((t) => t?.includes("Dev Member"))).toBe(true);
    expect(labels.some((t) => t?.includes("Guest Viewer"))).toBe(false);
    expect(labels.some((t) => t?.includes("viewer"))).toBe(false);
  });

});
