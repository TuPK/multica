// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { TenantDetail } from "./TenantDetail";

const mockUseTenant = vi.fn();
const mockUseTenantNotes = vi.fn();
const mockUseTenantAudit = vi.fn();
const mockChangeTenantStatusMutateAsync = vi.fn();
const mockAddTenantNoteMutateAsync = vi.fn();

vi.mock("../api/hooks", () => ({
  useTenant: () => mockUseTenant(),
  useTenantNotes: () => mockUseTenantNotes(),
  useTenantAudit: () => mockUseTenantAudit(),
  useChangeTenantStatus: () => ({
    mutateAsync: mockChangeTenantStatusMutateAsync,
    isPending: false,
  }),
  useAddTenantNote: () => ({
    mutateAsync: mockAddTenantNoteMutateAsync,
    isPending: false,
  }),
}));

const mockTenantData = {
  id: "t-123",
  name: "Acme Corp",
  slug: "acme",
  status: "active",
  member_count: 8,
  created_at: "2026-11-01T10:00:00Z",
  last_activity_at: "2026-11-15T12:00:00Z",
  state: {
    status: "active",
    reason: "Initial active state",
    changed_by: "system",
    changed_at: "2026-11-01T10:00:00Z",
    deletion_scheduled_at: null,
  },
  notes_count: 1,
  owner: {
    id: "u-owner",
    email: "owner@acme.com",
  },
};

describe("TenantDetail screen", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseTenant.mockReturnValue({
      data: mockTenantData,
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });
    mockUseTenantNotes.mockReturnValue({
      data: [
        {
          id: "n-1",
          workspace_id: "t-123",
          author_id: "u-staff-1",
          author_email: "support@example.com",
          body: "Verified payment issue.",
          created_at: "2026-11-10T10:00:00Z",
        },
      ],
      isLoading: false,
    });
    mockUseTenantAudit.mockReturnValue({
      data: [
        {
          id: "ae-1",
          workspace_id: "t-123",
          actor_type: "staff",
          actor_id: "u-staff-1",
          actor_label: "support@example.com",
          action: "workspace.status_changed",
          target_type: "workspace",
          target_id: "t-123",
          target_label: "Acme Corp",
          detail: { status: "active" },
          ip: "127.0.0.1",
          user_agent: "agent",
          request_id: "req-1",
          created_at: "2026-11-01T10:00:00Z",
        },
      ],
      isLoading: false,
    });
  });

  it("renders tenant detail, notes, and audit events", () => {
    render(
      <TenantDetail tenantId="t-123" onBack={vi.fn()} userRole="support" />
    );

    expect(screen.getByRole("heading", { name: "Acme Corp" })).toBeInTheDocument();
    expect(screen.getByText("acme • ID: t-123")).toBeInTheDocument();
    expect(screen.getByText("owner@acme.com")).toBeInTheDocument();
    expect(screen.getByText("Verified payment issue.")).toBeInTheDocument();
    expect(screen.getByText("workspace.status_changed")).toBeInTheDocument();
  });

  it("role gating: support role does not see pending_deletion option in change status dialog", () => {
    render(
      <TenantDetail tenantId="t-123" onBack={vi.fn()} userRole="support" />
    );

    fireEvent.click(screen.getByRole("button", { name: "Change status" }));
    expect(screen.getByRole("heading", { name: "Change workspace status" })).toBeInTheDocument();

    const options = screen.getAllByRole("option");
    const optionValues = options.map((opt) => (opt as HTMLOptionElement).value);

    expect(optionValues).toContain("active");
    expect(optionValues).toContain("read_only");
    expect(optionValues).toContain("suspended");
    expect(optionValues).not.toContain("pending_deletion");
  });

  it("role gating: admin role sees pending_deletion option in change status dialog", () => {
    render(
      <TenantDetail tenantId="t-123" onBack={vi.fn()} userRole="admin" />
    );

    fireEvent.click(screen.getByRole("button", { name: "Change status" }));
    expect(screen.getByRole("heading", { name: "Change workspace status" })).toBeInTheDocument();

    const options = screen.getAllByRole("option");
    const optionValues = options.map((opt) => (opt as HTMLOptionElement).value);

    expect(optionValues).toContain("pending_deletion");
  });

  it("the reason is required before a status change can be confirmed", async () => {
    mockChangeTenantStatusMutateAsync.mockResolvedValue({ changed: true });
    render(
      <TenantDetail tenantId="t-123" onBack={vi.fn()} userRole="admin" />
    );

    fireEvent.click(screen.getByRole("button", { name: "Change status" }));

    const confirmBtn = screen.getByRole("button", { name: "Update status" });
    expect(confirmBtn).toBeDisabled();

    const reasonInput = screen.getByLabelText(/Reason/i);
    fireEvent.change(reasonInput, { target: { value: "   " } });
    expect(confirmBtn).toBeDisabled();

    fireEvent.change(reasonInput, { target: { value: "Suspending due to TOS violation" } });
    expect(confirmBtn).toBeEnabled();

    fireEvent.click(confirmBtn);

    await waitFor(() => {
      expect(mockChangeTenantStatusMutateAsync).toHaveBeenCalledWith({
        tenantId: "t-123",
        status: "active",
        reason: "Suspending due to TOS violation",
      });
    });
  });

  it("adds an internal note", async () => {
    mockAddTenantNoteMutateAsync.mockResolvedValue({});
    render(
      <TenantDetail tenantId="t-123" onBack={vi.fn()} userRole="support" />
    );

    const noteInput = screen.getByPlaceholderText(/Write an internal note.../i);
    const addBtn = screen.getByRole("button", { name: /Add note/i });
    expect(addBtn).toBeDisabled();

    fireEvent.change(noteInput, { target: { value: "Investigated billing query." } });
    expect(addBtn).toBeEnabled();

    fireEvent.click(addBtn);

    await waitFor(() => {
      expect(mockAddTenantNoteMutateAsync).toHaveBeenCalledWith({
        tenantId: "t-123",
        body: "Investigated billing query.",
      });
    });
  });
});
