// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TenantUserDeactivateMenu } from "./TenantUserDeactivateMenu";
import { PlatformApiError } from "../api/client";
import type { TenantUser } from "../api/schemas";

const mockDeactivateTenantUserMutateAsync = vi.fn();

vi.mock("../api/hooks", () => ({
  useDeactivateTenantUser: () => ({ mutateAsync: mockDeactivateTenantUserMutateAsync, isPending: false }),
}));

const activeUser: TenantUser = {
  user_id: "u-active",
  email: "active@example.com",
  name: "Active User",
  account_status: "active",
  is_staff: false,
  workspaces: [
    { workspace_id: "ws-1", name: "Engineering", member_role: "admin" },
    { workspace_id: "ws-2", name: "Product", member_role: "member" },
  ],
  seats: 2,
  last_login_at: "2026-11-18T10:00:00Z",
};

function renderMenu(canManageMembers = true, user = activeUser) {
  return render(
    <TenantUserDeactivateMenu tenantId="t-1" user={user} canManageMembers={canManageMembers} />,
  );
}

describe("TenantUserDeactivateMenu", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("deactivate asks for a reason and lists the workspaces it removes the user from", async () => {
    mockDeactivateTenantUserMutateAsync.mockResolvedValueOnce({ request_ids: ["mr-1", "mr-2"] });
    renderMenu();

    fireEvent.click(screen.getByRole("button", { name: /^Deactivate in tenant/ }));
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

  it("keeps the dialog open and shows the mapped error when an owner blocks deactivation", async () => {
    const user = userEvent.setup();
    mockDeactivateTenantUserMutateAsync.mockRejectedValueOnce(
      new PlatformApiError(409, "owner_role_immutable", "an owner cannot be removed by staff"),
    );
    renderMenu();

    fireEvent.click(screen.getByRole("button", { name: /^Deactivate in tenant/ }));
    const dialog = screen.getByRole("dialog");
    const reasonInput = within(dialog).getByLabelText(/Reason/i);
    fireEvent.change(reasonInput, { target: { value: "Departure" } });
    await user.click(within(dialog).getByRole("button", { name: "Deactivate in tenant" }));

    expect(await within(dialog).findByRole("alert")).toHaveTextContent(/owner/i);
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(reasonInput).toHaveValue("Departure");
  });

  it("shows no deactivate action without member-management rights, or for staff targets", () => {
    const { unmount } = renderMenu(false);
    expect(screen.queryByRole("button", { name: /^Deactivate in tenant/ })).not.toBeInTheDocument();
    unmount();

    renderMenu(true, { ...activeUser, is_staff: true });
    expect(screen.queryByRole("button", { name: /^Deactivate in tenant/ })).not.toBeInTheDocument();
  });
});
