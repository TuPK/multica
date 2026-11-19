// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import { App } from "./App";

const mockUseMe = vi.fn();
const mockLogoutMutateAsync = vi.fn();
const mockUseLogout = vi.fn(() => ({
  mutateAsync: mockLogoutMutateAsync,
  isPending: false,
}));

vi.mock("./api/hooks", () => ({
  useMe: () => mockUseMe(),
  useLogout: () => mockUseLogout(),
  useSendCode: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useVerifyCode: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useOverview: () => ({
    data: { workspaces_by_status: { active: 5 }, signups_7d: 12 },
    isLoading: false,
    isError: false,
  }),
  useTenants: () => ({
    data: { tenants: [], next_cursor: null },
    isLoading: false,
    isError: false,
  }),
  useTenant: (id: string) => ({
    data: {
      id,
      name: `Workspace ${id}`,
      slug: `ws-${id}`,
      status: "active",
      member_count: 1,
      group_count: 0,
      created_at: "2026-11-01T00:00:00Z",
      last_activity_at: "2026-11-01T00:00:00Z",
      state: { status: "active", reason: "", changed_by: null, changed_at: null, deletion_scheduled_at: null },
      subscription: null,
      recent_orders: [],
      notes_count: 0,
      owner: null,
    },
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
  }),
  useTenantUsage: () => ({ data: null, isLoading: false, isError: false, refetch: vi.fn() }),
  useTenantBillingProfile: () => ({ data: null, isLoading: false, isError: false }),
  useTenantNotes: () => ({ data: [], isLoading: false }),
  useTenantAudit: () => ({ data: [], isLoading: false }),
  useChangeTenantStatus: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useAddTenantNote: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateSubscription: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useSetBillingProfileVerified: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateTenantBillingProfile: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUsers: () => ({
    data: { users: [], next_cursor: null },
    isLoading: false,
    isError: false,
  }),
  useUser: (id: string) => ({
    data: {
      id,
      email: `${id}@example.com`,
      name: `User ${id}`,
      status: "active",
      memberships_count: 0,
      created_at: "2026-11-01T00:00:00Z",
      memberships: [],
      account: { status: "active", reason: "", changed_at: null, sessions_valid_after: null },
    },
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
  }),
  useChangeUserStatus: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useRevokeUserSessions: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useResetUserMfa: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useStaff: () => ({
    data: { staff: [] },
    isLoading: false,
    isError: false,
  }),
  useAddStaff: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
  }),
  useRemoveStaff: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
  }),
  useAuditEvents: () => ({
    data: { events: [], next_cursor: null },
    isLoading: false,
    isError: false,
  }),
  usePlans: () => ({
    data: [],
    isLoading: false,
    isError: false,
  }),
  usePriceChanges: () => ({
    data: [],
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
  }),
  useSchedulePriceChange: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useCancelScheduledPriceChange: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useAccountRequests: () => ({
    data: [],
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
  }),
  useCreateAccountRequest: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
  }),
  useCancelAccountRequest: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
  }),
  useResendWelcome: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
  }),
  useOrders: () => ({
    data: [],
    isLoading: false,
    isError: false,
    error: null,
    refetch: vi.fn(),
  }),
  useOrderEinvoices: () => ({
    data: [],
    isLoading: false,
    isError: false,
    error: null,
    refetch: vi.fn(),
  }),
  useConfirmOrder: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
  }),
  useCancelOrder: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
  }),
  usePaymentNotices: () => ({
    data: [],
    isLoading: false,
    isError: false,
    error: null,
    refetch: vi.fn(),
  }),
  usePaymentNotice: () => ({
    data: undefined,
    isLoading: false,
    isError: false,
    error: null,
    refetch: vi.fn(),
  }),
  useResolvePaymentNotice: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useDismissPaymentNotice: () => ({ mutateAsync: vi.fn(), isPending: false }),
  usePutPlan: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
  }),
  useDeletePlan: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
  }),
  useEinvoices: () => ({
    data: [],
    isLoading: false,
    isError: false,
    error: null,
    refetch: vi.fn(),
  }),
}));

describe("App", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    window.history.replaceState(null, "", "/");
  });

  it("shows neutral loading state while useMe is loading", () => {
    mockUseMe.mockReturnValue({
      data: null,
      isLoading: true,
      refetch: vi.fn(),
    });

    render(<App />);
    expect(screen.getByText("Loading platform console...")).toBeInTheDocument();
  });

  it("shows sign-in screen when useMe returns null", () => {
    mockUseMe.mockReturnValue({
      data: null,
      isLoading: false,
      refetch: vi.fn(),
    });

    render(<App />);
    expect(screen.getByText("Platform Console")).toBeInTheDocument();
    expect(screen.getByLabelText(/Email address/i)).toBeInTheDocument();
  });

  it("role gating: billing role does not see staff tab", () => {
    mockUseMe.mockReturnValue({
      data: { user_id: "u-billing", email: "billing@example.com", role: "billing" },
      isLoading: false,
      refetch: vi.fn(),
    });

    render(<App />);
    expect(screen.getByRole("button", { name: /Overview/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Tenants/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Users/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Audit Log/i })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Staff/i })).not.toBeInTheDocument();
  });

  it("role gating: support role does not see staff tab", () => {
    mockUseMe.mockReturnValue({
      data: { user_id: "u-support", email: "support@example.com", role: "support" },
      isLoading: false,
      refetch: vi.fn(),
    });

    render(<App />);
    expect(screen.queryByRole("button", { name: /Staff/i })).not.toBeInTheDocument();
  });

  it("role gating: admin role sees staff tab and can navigate to it", () => {
    mockUseMe.mockReturnValue({
      data: { user_id: "u-admin", email: "admin@example.com", role: "admin" },
      isLoading: false,
      refetch: vi.fn(),
    });

    render(<App />);
    const staffTab = screen.getByRole("button", { name: /Staff/i });
    expect(staffTab).toBeInTheDocument();

    fireEvent.click(staffTab);
    expect(screen.getByRole("heading", { name: "Staff Management" })).toBeInTheDocument();
    expect(window.location.pathname).toBe("/staff");
  });

  it("clicking every SideNav item opens its distinctive screen and updates URL", () => {
    mockUseMe.mockReturnValue({
      data: { user_id: "u-admin", email: "admin@example.com", role: "admin" },
      isLoading: false,
      refetch: vi.fn(),
    });

    render(<App />);
    const nav = screen.getByRole("navigation", { name: "Main Navigation" });

    const navCases: Array<{
      buttonName: RegExp;
      expectedPath: string;
      headingName: string;
    }> = [
      { buttonName: /^Tenants$/i, expectedPath: "/tenants", headingName: "Tenants" },
      { buttonName: /^Users$/i, expectedPath: "/users", headingName: "Users" },
      { buttonName: /^Orders$/i, expectedPath: "/orders", headingName: "Orders" },
      { buttonName: /^Payment notices$/i, expectedPath: "/payment-notices", headingName: "Payment notices" },
      { buttonName: /^E-invoices$/i, expectedPath: "/einvoices", headingName: "E-invoices" },
      { buttonName: /^Accounts$/i, expectedPath: "/accounts", headingName: "Accounts" },
      { buttonName: /^Plans$/i, expectedPath: "/plans", headingName: "Plans Catalogue" },
      { buttonName: /^Staff$/i, expectedPath: "/staff", headingName: "Staff Management" },
      { buttonName: /^Audit Log$/i, expectedPath: "/audit", headingName: "Audit Log" },
      { buttonName: /^Overview$/i, expectedPath: "/", headingName: "Platform Overview" },
    ];

    for (const item of navCases) {
      const btn = within(nav).getByRole("button", { name: item.buttonName });
      fireEvent.click(btn);
      expect(window.location.pathname).toBe(item.expectedPath);
      expect(screen.getByRole("heading", { name: item.headingName })).toBeInTheDocument();
      expect(btn).toHaveAttribute("aria-current", "page");
    }
  });

  it("restores screen from initial URL (F5 reload) and responds to popstate", () => {
    window.history.replaceState(null, "", "/tenants/t-42");
    mockUseMe.mockReturnValue({
      data: { user_id: "u-admin", email: "admin@example.com", role: "admin" },
      isLoading: false,
      refetch: vi.fn(),
    });

    render(<App />);
    expect(screen.getByRole("heading", { name: "Workspace t-42" })).toBeInTheDocument();

    window.history.pushState(null, "", "/users/u-9");
    fireEvent(window, new PopStateEvent("popstate"));
    expect(screen.getByRole("heading", { name: "User u-9" })).toBeInTheDocument();

    window.history.pushState(null, "", "/orders");
    fireEvent(window, new PopStateEvent("popstate"));
    expect(screen.getByRole("heading", { name: "Orders" })).toBeInTheDocument();
  });

  it("redirects unauthorized direct URL access to Overview", () => {
    window.history.replaceState(null, "", "/staff");
    mockUseMe.mockReturnValue({
      data: { user_id: "u-support", email: "support@example.com", role: "support" },
      isLoading: false,
      refetch: vi.fn(),
    });

    render(<App />);
    expect(screen.getByRole("heading", { name: "Platform Overview" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Staff Management" })).not.toBeInTheDocument();
  });

  it("handles logout", async () => {
    const mockRefetch = vi.fn();
    mockUseMe.mockReturnValue({
      data: { user_id: "u-admin", email: "admin@example.com", role: "admin" },
      isLoading: false,
      refetch: mockRefetch,
    });
    mockLogoutMutateAsync.mockResolvedValue({});

    render(<App />);
    const logoutBtn = screen.getByRole("button", { name: /Sign out/i });
    fireEvent.click(logoutBtn);

    await waitFor(() => {
      expect(mockLogoutMutateAsync).toHaveBeenCalledTimes(1);
      expect(mockRefetch).toHaveBeenCalledTimes(1);
    });
  });

  it("role gating: admin sees Accounts tab and can navigate to it, non-admins do not", () => {
    mockUseMe.mockReturnValue({
      data: { user_id: "u-billing", email: "billing@example.com", role: "billing" },
      isLoading: false,
      refetch: vi.fn(),
    });

    const { rerender } = render(<App />);
    expect(screen.queryByRole("button", { name: /Accounts/i })).not.toBeInTheDocument();

    mockUseMe.mockReturnValue({
      data: { user_id: "u-admin", email: "admin@example.com", role: "admin" },
      isLoading: false,
      refetch: vi.fn(),
    });

    rerender(<App />);
    const accountsTab = screen.getByRole("button", { name: /Accounts/i });
    expect(accountsTab).toBeInTheDocument();
    fireEvent.click(accountsTab);
    expect(screen.getByRole("heading", { name: "Accounts" })).toBeInTheDocument();
  });

  it("role gating: billing and admin can navigate to E-invoices, support cannot", () => {
    mockUseMe.mockReturnValue({
      data: { user_id: "u-support", email: "support@example.com", role: "support" },
      isLoading: false,
      refetch: vi.fn(),
    });

    const { rerender } = render(<App />);
    expect(screen.queryByRole("button", { name: /E-invoices/i })).not.toBeInTheDocument();

    mockUseMe.mockReturnValue({
      data: { user_id: "u-billing", email: "billing@example.com", role: "billing" },
      isLoading: false,
      refetch: vi.fn(),
    });
    rerender(<App />);
    const einvoicesTab = screen.getByRole("button", { name: /E-invoices/i });
    expect(einvoicesTab).toBeInTheDocument();
    fireEvent.click(einvoicesTab);
    expect(screen.getByRole("heading", { name: "E-invoices" })).toBeInTheDocument();
  });

  it("role gating: billing sees Orders but not Plans", () => {
    mockUseMe.mockReturnValue({
      data: { user_id: "u-billing", email: "billing@example.com", role: "billing" },
      isLoading: false,
      refetch: vi.fn(),
    });

    render(<App />);
    expect(screen.queryByRole("button", { name: /^Plans$/i })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /^Orders$/i }));
    expect(screen.getByRole("heading", { name: "Orders" })).toBeInTheDocument();
  });
});
