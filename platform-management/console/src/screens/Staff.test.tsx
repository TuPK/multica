// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Staff } from "./Staff";

const mockUseStaff = vi.fn();
const mockAddStaffMutateAsync = vi.fn();
const mockRemoveStaffMutateAsync = vi.fn();

vi.mock("../api/hooks", () => ({
  useStaff: () => mockUseStaff(),
  useAddStaff: () => ({
    mutateAsync: mockAddStaffMutateAsync,
    isPending: false,
  }),
  useRemoveStaff: () => ({
    mutateAsync: mockRemoveStaffMutateAsync,
    isPending: false,
  }),
}));

const mockStaffList = [
  {
    user_id: "u-admin-1",
    email: "admin1@example.com",
    role: "admin",
    created_by: "system",
    created_at: "2026-10-01T00:00:00Z",
  },
  {
    user_id: "u-support-1",
    email: "support1@example.com",
    role: "support",
    created_by: "system",
    created_at: "2026-10-15T00:00:00Z",
  },
];

describe("Staff screen", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseStaff.mockReturnValue({
      data: { staff: mockStaffList },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });
  });

  it("renders active staff list", () => {
    render(<Staff />);

    expect(screen.getByRole("heading", { name: "Staff Management" })).toBeInTheDocument();
    expect(screen.getByText("admin1@example.com")).toBeInTheDocument();
    expect(screen.getByText("support1@example.com")).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: /Remove/i })).toHaveLength(2);
  });

  it("adds a staff member", async () => {
    const user = userEvent.setup();
    mockAddStaffMutateAsync.mockResolvedValue({
      user_id: "u-billing-1",
      email: "billing@example.com",
      role: "billing",
    });

    render(<Staff />);

    await user.click(screen.getByRole("button", { name: /Add staff/i }));
    expect(screen.getByRole("heading", { name: "Add staff member" })).toBeInTheDocument();

    const emailInput = screen.getByLabelText(/Email address/i);
    const roleSelect = screen.getByRole("combobox", { name: /Staff role/i });

    await user.type(emailInput, "billing@example.com");
    await user.click(roleSelect);
    await user.click(await screen.findByRole("option", { name: "Billing" }));

    await user.click(screen.getByRole("button", { name: "Add staff" }));

    await waitFor(() => {
      expect(mockAddStaffMutateAsync).toHaveBeenCalledWith({
        email: "billing@example.com",
        role: "billing",
      });
    });
  });

  it("removing the last admin shows the server's last_admin message", async () => {
    const lastAdminError = new Error("the last active administrator cannot be removed");
    (lastAdminError as { code?: string }).code = "last_admin";
    mockRemoveStaffMutateAsync.mockRejectedValue(lastAdminError);

    render(<Staff />);

    // Click remove on the admin row
    const removeAdminBtn = screen.getByRole("button", {
      name: "Remove staff member admin1@example.com",
    });
    fireEvent.click(removeAdminBtn);

    expect(screen.getByRole("heading", { name: "Remove staff member" })).toBeInTheDocument();

    const confirmRemoveBtn = screen.getByRole("button", { name: "Remove" });
    fireEvent.click(confirmRemoveBtn);

    await waitFor(() => {
      expect(mockRemoveStaffMutateAsync).toHaveBeenCalledWith({
        userId: "u-admin-1",
      });
    });

    expect(
      screen.getByText("the last active administrator cannot be removed")
    ).toBeInTheDocument();
  });
});
