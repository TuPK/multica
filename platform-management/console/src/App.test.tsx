// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
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
    data: [],
    isLoading: false,
    isError: false,
  }),
  useUsers: () => ({
    data: [],
    isLoading: false,
    isError: false,
  }),
  useStaff: () => ({
    data: [],
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
    data: [],
    isLoading: false,
    isError: false,
  }),
  usePlans: () => ({
    data: [],
    isLoading: false,
    isError: false,
  }),
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
  });

  it("navigates between screens via tabs", () => {
    mockUseMe.mockReturnValue({
      data: { user_id: "u-admin", email: "admin@example.com", role: "admin" },
      isLoading: false,
      refetch: vi.fn(),
    });

    render(<App />);
    expect(screen.getByRole("heading", { name: "Platform Overview" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /Tenants/i }));
    expect(screen.getByRole("heading", { name: "Tenants" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /Users/i }));
    expect(screen.getByRole("heading", { name: "Users" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /Audit Log/i }));
    expect(screen.getByRole("heading", { name: "Audit Log" })).toBeInTheDocument();
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

  it("clicking each nav item renders its own screen, not Overview", () => {
    mockUseMe.mockReturnValue({
      data: { user_id: "u-admin", email: "admin@example.com", role: "admin" },
      isLoading: false,
      refetch: vi.fn(),
    });

    render(<App />);

    fireEvent.click(screen.getByRole("button", { name: /^Orders$/i }));
    expect(screen.getByRole("heading", { name: "Orders" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Platform Overview" })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /^Plans$/i }));
    expect(screen.getByRole("heading", { name: "Plans Catalogue" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Platform Overview" })).not.toBeInTheDocument();
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
