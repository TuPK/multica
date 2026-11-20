// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, within, waitFor } from "@testing-library/react";
import { RuntimeDetail } from "./RuntimeDetail";

const mockUseMe = vi.fn();
const mockUseRuntime = vi.fn();
const mockSetStatusMutateAsync = vi.fn();
const mockUnregisterMutateAsync = vi.fn();

vi.mock("../api/hooks", () => ({
  useMe: () => mockUseMe(),
  useRuntime: (id?: string, enabled = true) => mockUseRuntime(id, enabled),
  useSetRuntimeStatus: () => ({
    mutateAsync: mockSetStatusMutateAsync,
    isPending: false,
  }),
  useUnregisterRuntime: () => ({
    mutateAsync: mockUnregisterMutateAsync,
    isPending: false,
  }),
}));

const baseRuntime = {
  id: "rt-1",
  name: "mac-studio-01",
  workspace_id: "ws-1",
  tenant_id: "t-1",
  status: "online" as const,
  os: "darwin",
  daemon_version: "0.9.1",
  agent_version: "0.4.2",
  owner_id: "u-1",
  disabled: false,
  running_tasks: 1,
  last_seen_at: "2026-11-19T10:15:00Z",
  created_at: "2026-11-01T00:00:00Z",
  failed_tasks_7d: 2,
  tasks: [
    {
      id: "task-1",
      status: "failed" as const,
      has_error: true,
      created_at: "2026-11-19T09:00:00Z",
      completed_at: "2026-11-19T09:02:00Z",
    },
    {
      id: "task-2",
      status: "completed" as const,
      has_error: false,
      created_at: "2026-11-19T08:00:00Z",
      completed_at: "2026-11-19T08:01:00Z",
    },
  ],
  audit_events: [
    {
      id: "ae-1",
      workspace_id: "ws-1",
      actor_type: "staff" as const,
      actor_id: "u-sys",
      actor_label: "sys@example.com",
      action: "runtime.enabled",
      target_type: "runtime",
      target_id: "rt-1",
      target_label: "mac-studio-01",
      detail: { reason: "Verified recovery" },
      ip: "",
      user_agent: "",
      request_id: "req-1",
      created_at: "2026-11-19T07:00:00Z",
    },
  ],
};

describe("RuntimeDetail screen", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseMe.mockReturnValue({
      data: { user_id: "u-sys", email: "sys@example.com", role: "system_admin", tenant_id: null },
    });
    mockUseRuntime.mockReturnValue({
      data: baseRuntime,
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    });
  });

  it("disable asks reason and shows disabled badge", async () => {
    mockSetStatusMutateAsync.mockResolvedValueOnce({ changed: true, disabled: true });

    const { rerender } = render(
      <RuntimeDetail runtimeId="rt-1" onBack={vi.fn()} userRole="system_admin" />,
    );

    expect(screen.getByRole("heading", { name: "mac-studio-01" })).toBeInTheDocument();
    expect(screen.queryByText("Disabled")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /Disable runtime/i }));

    const dialog = screen.getByRole("dialog");
    const submitBtn = within(dialog).getByRole("button", { name: /Disable runtime/i });
    expect(submitBtn).toBeDisabledControl();

    fireEvent.change(within(dialog).getByLabelText(/Reason/i), {
      target: { value: "Abusive daemon traffic" },
    });
    expect(submitBtn).not.toBeDisabledControl();
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(mockSetStatusMutateAsync).toHaveBeenCalledWith({
        runtimeId: "rt-1",
        status: "disabled",
        reason: "Abusive daemon traffic",
      });
    });

    mockUseRuntime.mockReturnValue({
      data: { ...baseRuntime, disabled: true },
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    });
    rerender(<RuntimeDetail runtimeId="rt-1" onBack={vi.fn()} userRole="system_admin" />);

    expect(screen.getAllByText("Disabled").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByRole("button", { name: /Enable runtime/i })).toBeInTheDocument();
  });

  it("unregister requires typing the runtime name", async () => {
    mockUnregisterMutateAsync.mockResolvedValueOnce({ request_id: "dr-9", state: "pending" });

    render(<RuntimeDetail runtimeId="rt-1" onBack={vi.fn()} userRole="system_admin" />);

    fireEvent.click(screen.getByRole("button", { name: /Unregister runtime/i }));

    const dialog = screen.getByRole("dialog");
    const confirmBtn = within(dialog).getByRole("button", { name: /Unregister runtime/i });
    expect(confirmBtn).toBeDisabledControl();

    fireEvent.change(within(dialog).getByLabelText(/Reason/i), {
      target: { value: "Machine decommissioned" },
    });
    expect(confirmBtn).toBeDisabledControl();

    const confirmInput = within(dialog).getByLabelText(/Runtime name/i);
    fireEvent.change(confirmInput, { target: { value: "wrong-name" } });
    expect(confirmBtn).toBeDisabledControl();

    fireEvent.change(confirmInput, { target: { value: "mac-studio-01" } });
    expect(confirmBtn).not.toBeDisabledControl();
    fireEvent.click(confirmBtn);

    await waitFor(() => {
      expect(mockUnregisterMutateAsync).toHaveBeenCalledWith({
        runtimeId: "rt-1",
        reason: "Machine decommissioned",
        confirm: "mac-studio-01",
      });
    });
  });

  it("hides disable and unregister actions for support and admin, and denies billing", () => {
    for (const role of ["support", "admin"] as const) {
      mockUseMe.mockReturnValue({
        data: {
          user_id: `u-${role}`,
          email: `${role}@example.com`,
          role,
          tenant_id: role === "admin" ? "t-1" : null,
        },
      });
      const { unmount } = render(
        <RuntimeDetail runtimeId="rt-1" onBack={vi.fn()} userRole={role} />,
      );
      expect(screen.getByRole("heading", { name: "mac-studio-01" })).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: /Disable runtime/i })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: /Unregister runtime/i })).not.toBeInTheDocument();
      unmount();
    }

    mockUseMe.mockReturnValue({
      data: { user_id: "u-bill", email: "billing@example.com", role: "billing", tenant_id: null },
    });
    render(<RuntimeDetail runtimeId="rt-1" onBack={vi.fn()} userRole="billing" />);
    expect(screen.getByRole("region", { name: "Not found" })).toBeInTheDocument();
  });

  it("displays recent tasks without content and 7-day failed count", () => {
    render(<RuntimeDetail runtimeId="rt-1" onBack={vi.fn()} userRole="system_admin" />);

    const task1Row = screen.getByText("task-1").closest("tr")!;
    const task2Row = screen.getByText("task-2").closest("tr")!;
    expect(within(task1Row).getByText("Failed")).toBeInTheDocument();
    expect(within(task1Row).getByText("Error")).toBeInTheDocument();
    expect(within(task2Row).getByText("Completed")).toBeInTheDocument();
    expect(screen.getByText("Verified recovery")).toBeInTheDocument();
    expect(screen.getByTestId("runtime-failed-tasks-7d")).toHaveTextContent("2");
  });
});
