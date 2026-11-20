// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { WorkspaceAccountMenu } from "./WorkspaceAccountMenu";
import type { StaffRole } from "../components/types";

const mockUseWorkspace = vi.fn();
const mockChangeWorkspaceStatusMutateAsync = vi.fn();
const mockUpdateSubscriptionMutateAsync = vi.fn();
const mockAddWorkspaceMemberMutateAsync = vi.fn();
const mockDeleteWorkspaceNowMutateAsync = vi.fn();
const mockCreateQuotaRequestMutateAsync = vi.fn();

vi.mock("../api/hooks", () => ({
  useExportWorkspace: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useWorkspace: (id?: string) => mockUseWorkspace(id),
  useChangeWorkspaceStatus: () => ({ mutateAsync: mockChangeWorkspaceStatusMutateAsync, isPending: false }),
  useUpdateSubscription: () => ({ mutateAsync: mockUpdateSubscriptionMutateAsync, isPending: false }),
  useAddWorkspaceMember: () => ({ mutateAsync: mockAddWorkspaceMemberMutateAsync, isPending: false }),
  useDeleteWorkspaceNow: () => ({ mutateAsync: mockDeleteWorkspaceNowMutateAsync, isPending: false }),
  useCreateQuotaRequest: () => ({ mutateAsync: mockCreateQuotaRequestMutateAsync, isPending: false }),
}));

const workspaceDetail = {
  id: "t-123",
  name: "Acme Corp",
  slug: "acme",
  status: "active",
  subscription: {
    plan_key: "pro",
    seats: 5,
    interval: "month",
    trial_end: "2026-11-15T00:00:00Z",
  },
};

function renderMenu(role: StaffRole | undefined) {
  return render(
    <WorkspaceAccountMenu
      workspaceId="t-123"
      workspaceName="Acme Corp"
      workspaceSlug="acme"
      workspaceStatus="active"
      role={role}
    />,
  );
}

// Row actions render as inline buttons (the sidebar is collapsed in tests).
// Icon actions carry the workspace name in their accessible label.
function action(name: RegExp) {
  return screen.getByRole("button", { name });
}

describe("WorkspaceAccountMenu", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseWorkspace.mockImplementation((id?: string) => ({
      data: id ? workspaceDetail : undefined,
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    }));
  });

  it("support gets the status shortcuts but not schedule deletion", () => {
    const { unmount } = renderMenu("support");
    expect(action(/^Suspend/)).toBeInTheDocument();
    expect(action(/^Read-only/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Schedule deletion/ })).not.toBeInTheDocument();
    unmount();
  });

  it("admin can schedule deletion of an active workspace", () => {
    renderMenu("admin");
    expect(action(/^Schedule deletion/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Cancel deletion/ })).not.toBeInTheDocument();
  });

  it("a pending-deletion workspace offers cancel deletion instead of schedule deletion", () => {
    render(
      <WorkspaceAccountMenu
        workspaceId="t-123"
        workspaceName="Acme Corp"
        workspaceSlug="acme"
        workspaceStatus="pending_deletion"
        role="admin"
      />,
    );
    expect(action(/^Cancel deletion/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Schedule deletion/ })).not.toBeInTheDocument();
  });

  it("the reason is required before a status change can be confirmed", async () => {
    mockChangeWorkspaceStatusMutateAsync.mockResolvedValue({ changed: true });
    renderMenu("admin");

    fireEvent.click(action(/^Suspend/));

    const confirmBtn = screen.getByRole("button", { name: "Update status" });
    expect(confirmBtn).toBeDisabledControl();

    const reasonInput = within(screen.getByRole("dialog")).getByLabelText(/Reason/i);
    fireEvent.change(reasonInput, { target: { value: "   " } });
    expect(confirmBtn).toBeDisabledControl();

    fireEvent.change(reasonInput, { target: { value: "Suspending due to TOS violation" } });
    expect(confirmBtn).not.toBeDisabledControl();

    fireEvent.click(confirmBtn);

    await waitFor(() => {
      expect(mockChangeWorkspaceStatusMutateAsync).toHaveBeenCalledWith({
        workspaceId: "t-123",
        status: "suspended",
        reason: "Suspending due to TOS violation",
      });
    });
  });

  it("edit subscription requires reason and submits PUT /workspaces/{id}/subscription", async () => {
    mockUpdateSubscriptionMutateAsync.mockResolvedValue({ changed: true });
    renderMenu("billing");

    fireEvent.click(action(/^Edit subscription/));
    expect(await screen.findByRole("heading", { name: "Edit Subscription" })).toBeInTheDocument();

    const seatsInput = screen.getByLabelText(/Seats/i);
    fireEvent.change(seatsInput, { target: { value: "10" } });
    fireEvent.blur(seatsInput);

    const submitBtn = screen.getByRole("button", { name: "Save subscription" });
    expect(submitBtn).toBeDisabledControl();

    const reasonInput = within(screen.getByRole("dialog")).getByLabelText(/Reason/i);
    fireEvent.change(reasonInput, { target: { value: "Customer upgraded over phone" } });
    expect(submitBtn).not.toBeDisabledControl();

    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(mockUpdateSubscriptionMutateAsync).toHaveBeenCalledWith({
        workspaceId: "t-123",
        planKey: "pro",
        seats: 10,
        interval: "month",
        trialEnd: "2026-11-15T00:00:00Z",
        reason: "Customer upgraded over phone",
      });
    });
  });

  it("the subscription is not fetched until its dialog opens", () => {
    renderMenu("billing");
    expect(mockUseWorkspace).toHaveBeenLastCalledWith(undefined);
    fireEvent.click(action(/^Edit subscription/));
    expect(mockUseWorkspace).toHaveBeenLastCalledWith("t-123");
  });

  it("add member form offers the member roles admin, member and viewer but not owner", async () => {
    const user = userEvent.setup();
    mockAddWorkspaceMemberMutateAsync.mockResolvedValueOnce({ id: "mr-1" });
    renderMenu("system_admin");

    await user.click(action(/^Add member/));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByRole("heading", { name: "Add member" })).toBeInTheDocument();

    await user.click(within(dialog).getByRole("combobox", { name: /Member role/i }));
    const options = await screen.findAllByRole("option");
    const labels = options.map((opt) => opt.textContent);
    expect(labels).toEqual(["Admin", "Member", "Viewer"]);
    expect(labels).not.toContain("Owner");

    await user.click(options.find((opt) => opt.textContent === "Viewer")!);
    await user.type(within(dialog).getByLabelText(/Email address/i), "newbie@example.com");
    await user.click(within(dialog).getByRole("button", { name: "Add member" }));

    await waitFor(() => {
      expect(mockAddWorkspaceMemberMutateAsync).toHaveBeenCalledWith({
        workspaceId: "t-123",
        email: "newbie@example.com",
        role: "viewer",
      });
    });
  });

  it("admin sees add member and delete workspace, and delete needs the slug and a reason", async () => {
    mockDeleteWorkspaceNowMutateAsync.mockResolvedValueOnce({ request_id: "dr-1", state: "pending" });
    const { unmount } = renderMenu("admin");

    expect(action(/^Add member/)).toBeInTheDocument();
    fireEvent.click(action(/^Delete workspace/));

    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByRole("button", { name: "Delete workspace" })).toBeDisabledControl();

    fireEvent.change(within(dialog).getByLabelText(/Reason/i), { target: { value: "Customer offboarding" } });
    fireEvent.change(within(dialog).getByLabelText(/Workspace slug/i), { target: { value: "acme" } });
    const confirmBtn = within(dialog).getByRole("button", { name: "Delete workspace" });
    expect(confirmBtn).not.toBeDisabledControl();

    fireEvent.click(confirmBtn);
    await waitFor(() => {
      expect(mockDeleteWorkspaceNowMutateAsync).toHaveBeenCalledWith({
        workspaceId: "t-123",
        reason: "Customer offboarding",
        confirm: "acme",
        export_first: false,
      });
    });
    unmount();

    for (const role of ["support", "billing"] as const) {
      const view = renderMenu(role);
      expect(screen.queryByRole("button", { name: /^Add member/ })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: /^Delete workspace/ })).not.toBeInTheDocument();
      view.unmount();
    }
  });

  it("system_admin and admin can request quota increase while support cannot", async () => {
    mockCreateQuotaRequestMutateAsync.mockResolvedValueOnce({ id: "qr-new" });
    const { unmount } = renderMenu("admin");

    fireEvent.click(action(/^Request quota increase/));
    const dialog = screen.getByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText(/Requested value/i), { target: { value: "25" } });
    fireEvent.blur(within(dialog).getByLabelText(/Requested value/i));
    fireEvent.change(within(dialog).getByLabelText(/Reason/i), { target: { value: "Growing agent pool" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Submit request" }));

    await waitFor(() => {
      expect(mockCreateQuotaRequestMutateAsync).toHaveBeenCalledWith({
        workspaceId: "t-123",
        gate: "agents",
        requested_value: 25,
        reason: "Growing agent pool",
      });
    });
    unmount();

    renderMenu("support");
    expect(screen.queryByRole("button", { name: /^Request quota increase/ })).not.toBeInTheDocument();
  });

  it("forwards export_first=true when delete workspace switch is enabled", async () => {
    mockDeleteWorkspaceNowMutateAsync.mockResolvedValueOnce({ request_id: "dr-exp", state: "pending" });
    renderMenu("system_admin");

    fireEvent.click(action(/^Delete workspace/));
    const dialog = screen.getByRole("dialog");
    fireEvent.click(within(dialog).getByRole("switch", { name: /Export data before deleting/i }));
    fireEvent.change(within(dialog).getByLabelText(/Reason/i), { target: { value: "Export archive before delete" } });
    fireEvent.change(within(dialog).getByLabelText(/Workspace slug/i), { target: { value: "acme" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Delete workspace" }));

    await waitFor(() => {
      expect(mockDeleteWorkspaceNowMutateAsync).toHaveBeenCalledWith({
        workspaceId: "t-123",
        reason: "Export archive before delete",
        confirm: "acme",
        export_first: true,
      });
    });
  });
});
