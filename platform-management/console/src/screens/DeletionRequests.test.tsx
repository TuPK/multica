// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import { DeletionRequests } from "./DeletionRequests";

const mockUseMe = vi.fn();
const mockUseDeletionRequests = vi.fn();
const mockCancelMutateAsync = vi.fn();
const mockRetryMutateAsync = vi.fn();

vi.mock("../api/hooks", () => ({
  useMe: () => mockUseMe(),
  useDeletionRequests: (filters?: { kind?: string; state?: string }) =>
    mockUseDeletionRequests(filters),
  useCancelDeletionRequest: () => ({
    mutateAsync: mockCancelMutateAsync,
    isPending: false,
  }),
  useRetryDeletionRequest: () => ({
    mutateAsync: mockRetryMutateAsync,
    isPending: false,
  }),
}));

const pendingRequest = {
  id: "dr-1",
  kind: "workspace" as const,
  target_id: "ws-1",
  target_label: "acme-ws",
  reason: "Customer requested deletion",
  state: "pending" as const,
  error_code: "",
  attempts: 0,
  tenant_id: "t-1",
  requested_by: "u-admin",
  requested_by_label: "admin@example.com",
  created_at: "2026-11-19T10:00:00Z",
  updated_at: "2026-11-19T10:00:00Z",
};

const failedRequest = {
  id: "dr-2",
  kind: "tenant" as const,
  target_id: "t-2",
  target_label: "Beta Tenant",
  reason: "Contract terminated",
  state: "failed" as const,
  error_code: "storage_cleanup_timeout",
  attempts: 3,
  tenant_id: "t-2",
  requested_by: "u-other",
  requested_by_label: "other@example.com",
  created_at: "2026-11-19T09:00:00Z",
  updated_at: "2026-11-19T09:30:00Z",
};

const completedRequest = {
  id: "dr-3",
  kind: "account" as const,
  target_id: "u-9",
  target_label: "deleted user",
  reason: "GDPR request",
  state: "completed" as const,
  error_code: "",
  attempts: 1,
  tenant_id: null,
  requested_by: "u-sys",
  requested_by_label: "sys@example.com",
  created_at: "2026-11-18T09:00:00Z",
  updated_at: "2026-11-18T09:15:00Z",
};

describe("DeletionRequests screen", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseMe.mockReturnValue({
      data: { user_id: "u-sys", email: "sys@example.com", role: "system_admin", tenant_id: null },
    });
    mockUseDeletionRequests.mockReturnValue({
      data: [pendingRequest, failedRequest, completedRequest],
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    });
  });

  it("retry and cancel call the right routes", async () => {
    mockCancelMutateAsync.mockResolvedValueOnce({ ...pendingRequest, state: "canceled" });
    mockRetryMutateAsync.mockResolvedValueOnce({ ...failedRequest, state: "pending", attempts: 0 });

    render(<DeletionRequests />);

    expect(screen.getByRole("heading", { name: "Deletion requests" })).toBeInTheDocument();

    const pendingRow = screen.getByText("acme-ws").closest("tr") as HTMLElement;
    expect(within(pendingRow).queryByRole("button", { name: "Retry" })).not.toBeInTheDocument();
    fireEvent.click(within(pendingRow).getByRole("button", { name: "Cancel" }));

    await waitFor(() => {
      expect(mockCancelMutateAsync).toHaveBeenCalledWith({ id: "dr-1" });
    });

    const failedRow = screen.getByText("Beta Tenant").closest("tr") as HTMLElement;
    expect(within(failedRow).getByText("storage_cleanup_timeout")).toBeInTheDocument();
    fireEvent.click(within(failedRow).getByRole("button", { name: "Retry" }));

    await waitFor(() => {
      expect(mockRetryMutateAsync).toHaveBeenCalledWith({ id: "dr-2" });
    });

    const completedRow = screen.getByText("deleted user").closest("tr") as HTMLElement;
    expect(within(completedRow).queryByRole("button", { name: "Retry" })).not.toBeInTheDocument();
    expect(within(completedRow).queryByRole("button", { name: "Cancel" })).not.toBeInTheDocument();
  });

  it("admin only sees cancel and retry on workspace requests they created", () => {
    mockUseMe.mockReturnValue({
      data: { user_id: "u-admin", email: "admin@example.com", role: "admin", tenant_id: "t-1" },
    });
    const ownFailed = { ...failedRequest, id: "dr-4", kind: "workspace" as const, target_label: "own-ws", requested_by: "u-admin" };
    const otherPending = { ...pendingRequest, id: "dr-5", target_label: "other-ws", requested_by: "u-sys" };
    mockUseDeletionRequests.mockReturnValue({
      data: [ownFailed, otherPending],
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<DeletionRequests />);

    const ownRow = screen.getByText("own-ws").closest("tr") as HTMLElement;
    expect(within(ownRow).getByRole("button", { name: "Retry" })).toBeInTheDocument();
    expect(within(ownRow).getByRole("button", { name: "Cancel" })).toBeInTheDocument();

    const otherRow = screen.getByText("other-ws").closest("tr") as HTMLElement;
    expect(within(otherRow).queryByRole("button", { name: "Retry" })).not.toBeInTheDocument();
    expect(within(otherRow).queryByRole("button", { name: "Cancel" })).not.toBeInTheDocument();
  });

  it("denies support and billing roles", () => {
    mockUseMe.mockReturnValue({
      data: { user_id: "u-supp", email: "supp@example.com", role: "support", tenant_id: null },
    });
    render(<DeletionRequests userRole="support" />);
    expect(screen.getByRole("region", { name: "Not found" })).toBeInTheDocument();
  });

  it("awaiting_approval legal_hold and export_timeout states render", () => {
    mockUseDeletionRequests.mockReturnValue({
      data: [
        {
          ...pendingRequest,
          id: "dr-await",
          kind: "tenant" as const,
          target_label: "Holdout Tenant",
          state: "awaiting_approval" as const,
        },
        {
          ...failedRequest,
          id: "dr-hold",
          target_label: "Held Workspace",
          state: "failed" as const,
          error_code: "legal_hold",
        },
        {
          ...failedRequest,
          id: "dr-timeout",
          target_label: "Slow Export Workspace",
          state: "failed" as const,
          error_code: "export_timeout",
        },
      ],
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<DeletionRequests />);

    const awaitRow = screen.getByText("Holdout Tenant").closest("tr") as HTMLElement;
    expect(within(awaitRow).getByText("Awaiting approval")).toBeInTheDocument();

    const holdRow = screen.getByText("Held Workspace").closest("tr") as HTMLElement;
    expect(within(holdRow).getByText("Legal hold")).toBeInTheDocument();

    const timeoutRow = screen.getByText("Slow Export Workspace").closest("tr") as HTMLElement;
    expect(within(timeoutRow).getByText("Export timeout")).toBeInTheDocument();
  });
});
