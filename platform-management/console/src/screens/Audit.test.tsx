// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Audit } from "./Audit";

const mockUseAuditEvents = vi.fn();

vi.mock("../api/hooks", () => ({
  useAuditEvents: (filters: Record<string, unknown>) => mockUseAuditEvents(filters),
}));

const mockAuditList = [
  {
    id: "ae-1",
    workspace_id: "ws-1",
    actor_type: "staff",
    actor_id: "u-staff",
    actor_label: "admin@example.com",
    action: "workspace.status_changed",
    target_type: "workspace",
    target_id: "ws-1",
    target_label: "Acme Corp",
    detail: { status: "suspended", reason: "Billing issue" },
    ip: "10.0.0.1",
    user_agent: "Mozilla",
    request_id: "req-1",
    created_at: "2026-11-15T10:00:00Z",
  },
];

describe("Audit screen", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders audit log and displays action catalog code", () => {
    mockUseAuditEvents.mockReturnValue({
      data: { events: mockAuditList, next_cursor: null },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });

    render(<Audit />);

    expect(screen.getByRole("heading", { name: "Audit Log" })).toBeInTheDocument();
    expect(screen.getByRole("cell", { name: "workspace.status_changed" })).toBeInTheDocument();
    expect(screen.getByText("admin@example.com")).toBeInTheDocument();
    expect(screen.getByText("Acme Corp")).toBeInTheDocument();
  });

  it("applies filters (action Select, actor id) and clears filters", async () => {
    const user = userEvent.setup();
    mockUseAuditEvents.mockReturnValue({
      data: { events: [], next_cursor: null },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });

    render(<Audit />);

    const actionTrigger = screen.getByRole("combobox", { name: "Action" });
    await user.click(actionTrigger);
    const staffAddedOption = await screen.findByRole("option", { name: "staff.added" });
    await user.click(staffAddedOption);

    const actorInput = screen.getByLabelText(/Actor ID/i);
    await user.type(actorInput, "u-staff-9");

    expect(mockUseAuditEvents).toHaveBeenLastCalledWith(
      expect.objectContaining({
        action: "staff.added",
        actor_id: "u-staff-9",
      }),
    );

    const clearButtons = screen.getAllByRole("button", { name: "Clear filters" });
    await user.click(clearButtons[0]!);
    expect(mockUseAuditEvents).toHaveBeenLastCalledWith({
      action: undefined,
      actor_id: undefined,
      from: undefined,
      to: undefined,
    });
  });

  it("shows 'Load more' button when hasNextPage is true and clicking fetches next page", () => {
    const mockFetchNextPage = vi.fn();
    mockUseAuditEvents.mockReturnValue({
      data: { events: mockAuditList },
      isLoading: false,
      isError: false,
      hasNextPage: true,
      fetchNextPage: mockFetchNextPage,
      refetch: vi.fn(),
    });

    render(<Audit />);

    const loadMoreBtn = screen.getByRole("button", { name: /Load more/i });
    expect(loadMoreBtn).toBeInTheDocument();

    fireEvent.click(loadMoreBtn);

    expect(mockFetchNextPage).toHaveBeenCalled();
  });

  it("handles loading, error, and empty states", () => {
    mockUseAuditEvents.mockReturnValue({
      data: null,
      isLoading: true,
      isError: false,
      refetch: vi.fn(),
    });

    const { rerender } = render(<Audit />);
    expect(screen.getByText("Loading audit log...")).toBeInTheDocument();

    mockUseAuditEvents.mockReturnValue({
      data: null,
      isLoading: false,
      isError: true,
      error: new Error("Server error"),
      refetch: vi.fn(),
    });

    rerender(<Audit />);
    expect(screen.getByText("Failed to load audit events")).toBeInTheDocument();

    mockUseAuditEvents.mockReturnValue({
      data: { events: [], next_cursor: null },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });

    rerender(<Audit />);
    expect(screen.getByText("No audit events found")).toBeInTheDocument();
  });
});
