// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import { QuotaRequests } from "./QuotaRequests";

const mockUseMe = vi.fn();
const mockUseQuotaRequests = vi.fn();
const mockApproveMutateAsync = vi.fn();
const mockDenyMutateAsync = vi.fn();

vi.mock("../api/hooks", () => ({
  useMe: () => mockUseMe(),
  useQuotaRequests: (filters?: { state?: string }, enabled = true) =>
    mockUseQuotaRequests(filters, enabled),
  useApproveQuotaRequest: () => ({
    mutateAsync: mockApproveMutateAsync,
    isPending: false,
  }),
  useDenyQuotaRequest: () => ({
    mutateAsync: mockDenyMutateAsync,
    isPending: false,
  }),
}));

const pendingQuotaRequest = {
  id: "qr-1",
  workspace_id: "ws-1",
  tenant_id: "t-1",
  gate: "agents",
  current_value: 5,
  requested_value: 20,
  approved_value: null,
  state: "pending" as const,
  reason: "Need more agents for onboarding",
  decision_reason: "",
  requested_by: "u-owner",
  requested_by_label: "owner@acme.test",
  decided_by: "",
  created_at: "2026-11-19T10:00:00Z",
  decided_at: null,
};

describe("QuotaRequests screen", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseMe.mockReturnValue({
      data: { user_id: "u-sys", email: "sys@example.com", role: "system_admin", tenant_id: null },
    });
    mockUseQuotaRequests.mockReturnValue({
      data: [pendingQuotaRequest],
      isLoading: false,
      isFetching: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
      hasNextPage: false,
      fetchNextPage: vi.fn(),
      isFetchingNextPage: false,
    });
  });

  it("system_admin approves with a lower value and gives a reason", async () => {
    mockApproveMutateAsync.mockResolvedValueOnce({
      ...pendingQuotaRequest,
      state: "approved",
      approved_value: 12,
      decision_reason: "Approved partial increase for Q4",
    });

    render(<QuotaRequests userRole="system_admin" />);

    expect(screen.getByRole("heading", { name: "Quota requests" })).toBeInTheDocument();
    const row = screen.getByText("agents").closest("tr") as HTMLElement;
    fireEvent.click(within(row).getByRole("button", { name: "Approve" }));

    const dialog = screen.getByRole("dialog");
    const valueInput = within(dialog).getByLabelText(/Approved value/i);
    expect(valueInput).toHaveValue("20");

    fireEvent.change(valueInput, { target: { value: "12" } });
    fireEvent.blur(valueInput);
    fireEvent.change(within(dialog).getByLabelText(/Reason/i), {
      target: { value: "Approved partial increase for Q4" },
    });

    fireEvent.click(within(dialog).getByRole("button", { name: "Approve request" }));

    await waitFor(() => {
      expect(mockApproveMutateAsync).toHaveBeenCalledWith({
        id: "qr-1",
        value: 12,
        reason: "Approved partial increase for Q4",
      });
    });
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("admin cannot approve", () => {
    mockUseMe.mockReturnValue({
      data: { user_id: "u-admin", email: "admin@example.com", role: "admin", tenant_id: "t-1" },
    });

    const { rerender } = render(<QuotaRequests userRole="admin" />);
    expect(screen.getByText("Need more agents for onboarding")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Approve" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Deny" })).not.toBeInTheDocument();

    mockUseMe.mockReturnValue({
      data: { user_id: "u-bill", email: "billing@example.com", role: "billing", tenant_id: null },
    });
    rerender(<QuotaRequests userRole="billing" />);
    expect(screen.queryByRole("button", { name: "Approve" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Deny" })).not.toBeInTheDocument();

    mockUseMe.mockReturnValue({
      data: { user_id: "u-supp", email: "support@example.com", role: "support", tenant_id: null },
    });
    rerender(<QuotaRequests userRole="support" />);
    expect(screen.getByRole("region", { name: "Not found" })).toBeInTheDocument();
  });

  it("system_admin denies a pending request with a required reason", async () => {
    mockDenyMutateAsync.mockResolvedValueOnce({
      ...pendingQuotaRequest,
      state: "denied",
      decision_reason: "Upgrade plan instead",
    });

    render(<QuotaRequests userRole="system_admin" />);
    const row = screen.getByText("agents").closest("tr") as HTMLElement;
    fireEvent.click(within(row).getByRole("button", { name: "Deny" }));

    const dialog = screen.getByRole("dialog");
    const confirmBtn = within(dialog).getByRole("button", { name: "Deny request" });
    expect(confirmBtn).toBeDisabledControl();

    fireEvent.change(within(dialog).getByLabelText(/Reason/i), {
      target: { value: "Upgrade plan instead" },
    });
    expect(confirmBtn).not.toBeDisabledControl();
    fireEvent.click(confirmBtn);

    await waitFor(() => {
      expect(mockDenyMutateAsync).toHaveBeenCalledWith({
        id: "qr-1",
        reason: "Upgrade plan instead",
      });
    });
  });
});
