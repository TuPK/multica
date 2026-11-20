// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, within } from "@testing-library/react";
import { RuntimeDetail } from "./RuntimeDetail";

const mockUseMe = vi.fn();
const mockUseRuntime = vi.fn();
const mockSetStatusMutateAsync = vi.fn();
const mockUnregisterMutateAsync = vi.fn();

vi.mock("../api/hooks", () => ({
  useExportWorkspace: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useChangeWorkspaceStatus: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateSubscription: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useAddWorkspaceMember: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useDeleteWorkspaceNow: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useCreateQuotaRequest: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useWorkspace: () => ({ data: undefined, isLoading: false, isError: false, refetch: vi.fn() }),
  useDeactivateTenantUser: () => ({ mutateAsync: vi.fn(), isPending: false }),
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
