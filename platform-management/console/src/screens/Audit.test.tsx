// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Audit } from "./Audit";

const mockUseAuditEvents = vi.fn();
const mockUseMe = vi.fn();
const mockExportAuditMutateAsync = vi.fn();

vi.mock("../api/hooks", () => ({
  useMe: () => mockUseMe(),
  useAuditEvents: (filters: Record<string, unknown>) => mockUseAuditEvents(filters),
  useExportAudit: () => ({
    mutateAsync: mockExportAuditMutateAsync,
    isPending: false,
  }),
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
    mockUseMe.mockReturnValue({
      data: { user_id: "u-sys", email: "sys@example.com", role: "system_admin", tenant_id: null },
    });
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

  it("uses shared calendar date pickers instead of native date inputs for From/To filters", async () => {
    const user = userEvent.setup();
    mockUseAuditEvents.mockReturnValue({
      data: { events: [], next_cursor: null },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });

    const { container } = render(<Audit />);

    expect(container.querySelector('input[type="date"]')).toBeNull();

    const fromTrigger = screen.getByLabelText(/From Date/i);
    await user.click(fromTrigger);

    await screen.findByRole("button", { name: /Any date/i });
    const day15 = Array.from(document.querySelectorAll<HTMLButtonElement>("button[data-day]")).find(
      (btn) => btn.textContent?.trim() === "15",
    );
    expect(day15).toBeDefined();
    await user.click(day15!);

    const lastCall = mockUseAuditEvents.mock.calls.at(-1)?.[0] as { from?: string; to?: string };
    expect(lastCall?.from).toMatch(/^\d{4}-\d{2}-15T00:00:00\.000Z$/);

    const toTrigger = screen.getByLabelText(/To Date/i);
    await user.click(toTrigger);
    await screen.findByRole("button", { name: /Any date/i });
    const day20 = Array.from(document.querySelectorAll<HTMLButtonElement>("button[data-day]")).find(
      (btn) => btn.textContent?.trim() === "20",
    );
    expect(day20).toBeDefined();
    await user.click(day20!);

    const withToCall = mockUseAuditEvents.mock.calls.at(-1)?.[0] as { from?: string; to?: string };
    expect(withToCall?.to).toMatch(/^\d{4}-\d{2}-20T00:00:00\.000Z$/);

    await user.click(fromTrigger);
    const anyDateBtn = await screen.findByRole("button", { name: /Any date/i });
    await user.click(anyDateBtn);

    expect(mockUseAuditEvents).toHaveBeenLastCalledWith(
      expect.objectContaining({ from: undefined }),
    );
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

  it("export form rejects ranges over 31 days", async () => {
    const user = userEvent.setup();
    mockUseAuditEvents.mockReturnValue({
      data: { events: [], next_cursor: null },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });

    render(<Audit />);

    const exportFromTrigger = screen.getByLabelText(/Export from/i);
    await user.click(exportFromTrigger);
    const prevBtn = await screen.findByRole("button", { name: /previous/i });
    await user.click(prevBtn);
    await user.click(prevBtn);
    const day1 = Array.from(document.querySelectorAll<HTMLButtonElement>("button[data-day]")).find(
      (btn) => btn.textContent?.trim() === "1",
    );
    expect(day1).toBeDefined();
    await user.click(day1!);

    const exportToTrigger = screen.getByLabelText(/Export to/i);
    await user.click(exportToTrigger);
    await screen.findByRole("button", { name: /Any date/i });
    const day15 = Array.from(document.querySelectorAll<HTMLButtonElement>("button[data-day]")).find(
      (btn) => btn.textContent?.trim() === "15",
    );
    expect(day15).toBeDefined();
    await user.click(day15!);

    await user.click(screen.getByRole("button", { name: /Export audit log/i }));

    const alert = screen
      .getAllByRole("alert")
      .find((el) => (el.textContent ?? "").trim() !== "");
    expect(alert).toHaveTextContent(/31 days/i);
    expect(mockExportAuditMutateAsync).not.toHaveBeenCalled();
  });

  it("hides export form for support and billing roles", () => {
    mockUseAuditEvents.mockReturnValue({
      data: { events: [], next_cursor: null },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });
    mockUseMe.mockReturnValue({
      data: { user_id: "u-supp", email: "supp@example.com", role: "support", tenant_id: null },
    });
    const { rerender } = render(<Audit />);
    expect(screen.queryByRole("button", { name: /Export audit log/i })).not.toBeInTheDocument();

    mockUseMe.mockReturnValue({
      data: { user_id: "u-bill", email: "bill@example.com", role: "billing", tenant_id: null },
    });
    rerender(<Audit />);
    expect(screen.queryByRole("button", { name: /Export audit log/i })).not.toBeInTheDocument();
  });
});
