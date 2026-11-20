// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Staff } from "./Staff";
import { PlatformApiError } from "../api/client";

const mockUseStaff = vi.fn();
const mockUseTenants = vi.fn();
const mockAddStaffMutateAsync = vi.fn();
const mockUpdateStaffMutateAsync = vi.fn();
const mockRemoveStaffMutateAsync = vi.fn();

vi.mock("../api/hooks", () => ({
  useStaff: () => mockUseStaff(),
  useTenants: () => mockUseTenants(),
  useAddStaff: () => ({
    mutateAsync: mockAddStaffMutateAsync,
    isPending: false,
  }),
  useUpdateStaff: () => ({
    mutateAsync: mockUpdateStaffMutateAsync,
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
    tenant_id: "t-1",
    tenant_name: null,
    created_by: "system",
    created_at: "2026-10-01T00:00:00Z",
  },
  {
    user_id: "u-support-1",
    email: "support1@example.com",
    role: "support",
    tenant_id: null,
    tenant_name: null,
    created_by: "system",
    created_at: "2026-10-15T00:00:00Z",
  },
];

const mockTenants = [
  { id: "t-1", name: "Acme Tenant", status: "active", workspace_count: 1, max_workspaces: null, best_plan_key: "pro", status_summary: {}, created_at: "", last_activity_at: "" },
  { id: "t-2", name: "Beta Tenant", status: "active", workspace_count: 2, max_workspaces: 3, best_plan_key: "pro", status_summary: {}, created_at: "", last_activity_at: "" },
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
    mockUseTenants.mockReturnValue({
      data: mockTenants,
      isLoading: false,
      isError: false,
    });
  });

  it("renders active staff list including each staff member's tenant", () => {
    render(<Staff />);

    expect(screen.getByRole("heading", { name: "Staff Management" })).toBeInTheDocument();
    expect(screen.getByText("admin1@example.com")).toBeInTheDocument();
    expect(screen.getByText("support1@example.com")).toBeInTheDocument();
    const adminRow = screen.getByText("admin1@example.com").closest("tr") as HTMLElement;
    expect(within(adminRow).getByText("Acme Tenant")).toBeInTheDocument();
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
    const adminRow = screen.getByRole("row", { name: /admin1@example\.com/ });
    const removeAdminBtn = within(adminRow).getByRole("button", { name: "Remove" });
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

  it("admin role requires a tenant and other roles hide the tenant field", async () => {
    const user = userEvent.setup();
    render(<Staff />);

    await user.click(screen.getByRole("button", { name: /Add staff/i }));
    const dialog = screen.getByRole("dialog");

    // Support role hides tenant field
    expect(within(dialog).queryByRole("combobox", { name: /Tenant/i })).not.toBeInTheDocument();

    await user.type(within(dialog).getByLabelText(/Email address/i), "newadmin@example.com");

    // Switching to Admin reveals required tenant picker and keeps submit disabled until chosen
    await user.click(within(dialog).getByRole("combobox", { name: /Staff role/i }));
    await user.click(await screen.findByRole("option", { name: "Admin" }));

    const tenantSelect = within(dialog).getByRole("combobox", { name: /Tenant/i });
    expect(tenantSelect).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Add staff" })).toBeDisabled();

    await user.click(tenantSelect);
    await user.click(await screen.findByRole("option", { name: /Beta Tenant/i }));
    expect(within(dialog).getByRole("button", { name: "Add staff" })).toBeEnabled();

    // Switching to System Admin hides the tenant field again
    await user.click(within(dialog).getByRole("combobox", { name: /Staff role/i }));
    await user.click(await screen.findByRole("option", { name: "System Admin" }));
    expect(within(dialog).queryByRole("combobox", { name: /Tenant/i })).not.toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Add staff" })).toBeEnabled();
  });

  it("shows the 400 tenant_required message", async () => {
    const user = userEvent.setup();
    mockUpdateStaffMutateAsync.mockRejectedValueOnce(
      new PlatformApiError(400, "tenant_required", "the admin role needs a tenant_id"),
    );

    render(<Staff />);

    const supportRow = screen.getByText("support1@example.com").closest("tr") as HTMLElement;
    await user.click(within(supportRow).getByRole("button", { name: /Edit/i }));

    const dialog = screen.getByRole("dialog");
    await user.click(within(dialog).getByRole("combobox", { name: /Staff role/i }));
    await user.click(await screen.findByRole("option", { name: "Admin" }));
    await user.click(within(dialog).getByRole("combobox", { name: /Tenant/i }));
    await user.click(await screen.findByRole("option", { name: /Acme Tenant/i }));

    await user.click(within(dialog).getByRole("button", { name: "Save changes" }));

    await waitFor(() => {
      expect(within(dialog).getByRole("alert")).toHaveTextContent("the admin role needs a tenant_id");
    });
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("changes a staff member's role and tenant with PUT /staff/{user_id}", async () => {
    const user = userEvent.setup();
    mockUpdateStaffMutateAsync.mockResolvedValueOnce({
      user_id: "u-support-1",
      role: "admin",
      tenant_id: "t-2",
    });

    render(<Staff />);

    const supportRow = screen.getByText("support1@example.com").closest("tr") as HTMLElement;
    await user.click(within(supportRow).getByRole("button", { name: /Edit/i }));

    const dialog = screen.getByRole("dialog");
    await user.click(within(dialog).getByRole("combobox", { name: /Staff role/i }));
    await user.click(await screen.findByRole("option", { name: "Admin" }));
    await user.click(within(dialog).getByRole("combobox", { name: /Tenant/i }));
    await user.click(await screen.findByRole("option", { name: /Beta Tenant/i }));
    await user.click(within(dialog).getByRole("button", { name: "Save changes" }));

    await waitFor(() => {
      expect(mockUpdateStaffMutateAsync).toHaveBeenCalledWith({
        userId: "u-support-1",
        role: "admin",
        tenant_id: "t-2",
      });
    });
  });
});
