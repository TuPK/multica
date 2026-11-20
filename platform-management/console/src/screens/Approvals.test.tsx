// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import { Approvals } from "./Approvals";
import { PlatformApiError } from "../api/client";
import { findAlert } from "../test/alert";

const mockUseMe = vi.fn();
const mockUseApprovals = vi.fn();
const mockApproveMutateAsync = vi.fn();
const mockRejectMutateAsync = vi.fn();

vi.mock("../api/hooks", () => ({
  useMe: () => mockUseMe(),
  useApprovals: (enabled = true) => mockUseApprovals(enabled),
  useApproveApproval: () => ({
    mutateAsync: mockApproveMutateAsync,
    isPending: false,
  }),
  useRejectApproval: () => ({
    mutateAsync: mockRejectMutateAsync,
    isPending: false,
  }),
}));

const ownRequest = {
  id: "ap-own",
  kind: "tenant" as const,
  target_id: "t-1",
  target_label: "Own Tenant",
  reason: "Requested by current sysadmin",
  state: "awaiting_approval" as const,
  error_code: "",
  attempts: 0,
  tenant_id: "t-1",
  requested_by: "u-sys-1",
  requested_by_label: "sys1@example.com",
  created_at: "2026-11-19T10:00:00Z",
  updated_at: "2026-11-19T10:00:00Z",
};

const otherRequest = {
  id: "ap-other",
  kind: "account" as const,
  target_id: "u-9",
  target_label: "leaver@example.com",
  reason: "GDPR erasure",
  state: "awaiting_approval" as const,
  error_code: "",
  attempts: 0,
  tenant_id: null,
  requested_by: "u-sys-2",
  requested_by_label: "sys2@example.com",
  created_at: "2026-11-19T09:00:00Z",
  updated_at: "2026-11-19T09:00:00Z",
};

describe("Approvals screen", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseMe.mockReturnValue({
      data: { user_id: "u-sys-1", email: "sys1@example.com", role: "system_admin", tenant_id: null },
    });
    mockUseApprovals.mockReturnValue({
      data: [ownRequest, otherRequest],
      isLoading: false,
      isFetching: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    });
  });

  it("approver must differ message and refuses own request", async () => {
    mockApproveMutateAsync.mockRejectedValueOnce(
      new PlatformApiError(409, "approver_must_differ", "the approver must differ from the requester"),
    );

    render(<Approvals userRole="system_admin" />);

    expect(screen.getByRole("heading", { name: "Approvals" })).toBeInTheDocument();

    const ownRow = screen.getByText("Own Tenant").closest("tr") as HTMLElement;
    expect(ownRow).toHaveTextContent(/approver must differ/i);
    const ownApproveBtn = within(ownRow).getByRole("button", { name: "Approve" });
    const ownRejectBtn = within(ownRow).getByRole("button", { name: "Reject" });
    expect(ownApproveBtn).toBeDisabledControl();
    expect(ownRejectBtn).toBeDisabledControl();
    fireEvent.click(ownApproveBtn);
    expect(mockApproveMutateAsync).not.toHaveBeenCalled();

    const otherRow = screen.getByText("leaver@example.com").closest("tr") as HTMLElement;
    const otherApproveBtn = within(otherRow).getByRole("button", { name: "Approve" });
    expect(otherApproveBtn).not.toBeDisabledControl();
    fireEvent.click(otherApproveBtn);

    await waitFor(() => {
      expect(findAlert()).toHaveTextContent("the approver must differ from the requester");
    });

    mockApproveMutateAsync.mockResolvedValueOnce({ request_id: "ap-other", state: "pending" });
    fireEvent.click(otherApproveBtn);
    await waitFor(() => {
      expect(mockApproveMutateAsync).toHaveBeenLastCalledWith({ id: "ap-other" });
    });
  });

  it("denies non-system_admin roles", () => {
    for (const role of ["admin", "support", "billing"] as const) {
      mockUseMe.mockReturnValue({
        data: { user_id: `u-${role}`, email: `${role}@example.com`, role, tenant_id: role === "admin" ? "t-1" : null },
      });
      const { unmount } = render(<Approvals userRole={role} />);
      expect(screen.getByRole("region", { name: "Not found" })).toBeInTheDocument();
      unmount();
    }
  });
});
