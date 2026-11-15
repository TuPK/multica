// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { UserDetail } from "./UserDetail";

const mockUseUser = vi.fn();
const mockChangeUserStatusMutateAsync = vi.fn();
const mockRevokeUserSessionsMutateAsync = vi.fn();

vi.mock("../api/hooks", () => ({
  useUser: () => mockUseUser(),
  useChangeUserStatus: () => ({
    mutateAsync: mockChangeUserStatusMutateAsync,
    isPending: false,
  }),
  useRevokeUserSessions: () => ({
    mutateAsync: mockRevokeUserSessionsMutateAsync,
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

describe("UserDetail screen", () => {
  beforeEach(() => {
    vi.clearAllMocks();
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
    expect(screen.getByRole("button", { name: "Suspend account" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Sign out everywhere" })).toBeInTheDocument();
    expect(screen.getByText("Engineering")).toBeInTheDocument();
    expect(screen.getByText("Product")).toBeInTheDocument();
  });

  it("renders reinstate button for suspended user", () => {
    mockUseUser.mockReturnValue({
      data: mockSuspendedUser,
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });

    render(<UserDetail userId="u-suspended" onBack={vi.fn()} />);

    expect(screen.getByRole("button", { name: "Reinstate account" })).toBeInTheDocument();
    expect(screen.getByText("Fraud investigation")).toBeInTheDocument();
  });

  it("the reason is required before account suspend can be confirmed", async () => {
    mockUseUser.mockReturnValue({
      data: mockActiveUser,
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });
    mockChangeUserStatusMutateAsync.mockResolvedValue({ changed: true });

    render(<UserDetail userId="u-active" onBack={vi.fn()} />);

    fireEvent.click(screen.getByRole("button", { name: "Suspend account" }));
    expect(screen.getByRole("heading", { name: "Suspend user account" })).toBeInTheDocument();

    const confirmBtn = screen.getByRole("button", { name: "Suspend account" });
    expect(confirmBtn).toBeDisabled();

    const reasonInput = screen.getByLabelText(/Reason/i);
    fireEvent.change(reasonInput, { target: { value: "   " } });
    expect(confirmBtn).toBeDisabled();

    fireEvent.change(reasonInput, { target: { value: "Spam behavior detected" } });
    expect(confirmBtn).toBeEnabled();

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

    render(<UserDetail userId="u-active" onBack={vi.fn()} />);

    fireEvent.click(screen.getByRole("button", { name: "Sign out everywhere" }));
    expect(screen.getByRole("heading", { name: "Sign out everywhere" })).toBeInTheDocument();

    const confirmBtn = screen.getByRole("button", { name: "Revoke all sessions" });
    expect(confirmBtn).toBeDisabled();

    const reasonInput = screen.getByLabelText(/Reason/i);
    fireEvent.change(reasonInput, { target: { value: "Credential breach" } });
    expect(confirmBtn).toBeEnabled();

    fireEvent.click(confirmBtn);

    await waitFor(() => {
      expect(mockRevokeUserSessionsMutateAsync).toHaveBeenCalledWith({
        userId: "u-active",
        reason: "Credential breach",
      });
    });
  });
});
