// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Runtimes } from "./Runtimes";

const mockUseMe = vi.fn();
const mockUseRuntimes = vi.fn();
const mockUseTenants = vi.fn();
const mockFetchNextPage = vi.fn();

vi.mock("../api/hooks", () => ({
  useExportWorkspace: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useChangeWorkspaceStatus: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateSubscription: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useAddWorkspaceMember: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useDeleteWorkspaceNow: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useCreateQuotaRequest: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useWorkspace: () => ({ data: undefined, isLoading: false, isError: false, refetch: vi.fn() }),
  useSetRuntimeStatus: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUnregisterRuntime: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useDeactivateTenantUser: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useMe: () => mockUseMe(),
  useRuntimes: (filters?: Record<string, unknown>, enabled = true) =>
    mockUseRuntimes(filters, enabled),
  useTenants: (filters?: Record<string, unknown>) => mockUseTenants(filters),
}));

const sampleRuntimes = [
  {
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
    running_tasks: 2,
    last_seen_at: "2026-11-19T10:15:00Z",
    created_at: "2026-11-01T00:00:00Z",
  },
  {
    id: "rt-2",
    name: "linux-worker-02",
    workspace_id: "ws-2",
    tenant_id: "t-2",
    status: "offline" as const,
    os: "linux",
    daemon_version: "0.8.0",
    agent_version: "0.4.0",
    owner_id: "u-2",
    disabled: true,
    running_tasks: 0,
    last_seen_at: null,
    created_at: "2026-11-02T00:00:00Z",
  },
];

describe("Runtimes screen", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseMe.mockReturnValue({
      data: { user_id: "u-sys", email: "sys@example.com", role: "system_admin", tenant_id: null },
    });
    mockUseTenants.mockReturnValue({
      data: [
        { id: "t-1", name: "Acme Corp" },
        { id: "t-2", name: "Beta LLC" },
      ],
      isLoading: false,
      isError: false,
    });
    mockUseRuntimes.mockReturnValue({
      data: sampleRuntimes,
      isLoading: false,
      isFetching: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
      hasNextPage: false,
      fetchNextPage: mockFetchNextPage,
      isFetchingNextPage: false,
    });
  });

  it("lists runtimes with status last seen versions and no ip column", () => {
    const onSelectRuntime = vi.fn();
    render(<Runtimes onSelectRuntime={onSelectRuntime} userRole="system_admin" />);

    expect(screen.getByRole("heading", { name: "Runtimes" })).toBeInTheDocument();

    const headers = screen.getAllByRole("columnheader").map((h) => h.textContent?.trim() ?? "");
    expect(headers.some((h) => /\bip\b|address|device/i.test(h))).toBe(false);
    expect(headers).toContain("Status");
    expect(headers).toContain("Last seen");
    expect(headers.some((h) => /version/i.test(h))).toBe(true);

    const row1 = screen.getByText("mac-studio-01").closest("tr") as HTMLElement;
    expect(row1).toHaveTextContent("Online");
    expect(row1).toHaveTextContent("0.9.1");
    expect(row1).toHaveTextContent("0.4.2");
    expect(row1).toHaveTextContent("darwin");
    expect(row1).toHaveTextContent("2");
    expect(row1).toHaveTextContent(new Date("2026-11-19T10:15:00Z").toLocaleString());

    const row2 = screen.getByText("linux-worker-02").closest("tr") as HTMLElement;
    expect(row2).toHaveTextContent("Offline");
    expect(row2).toHaveTextContent("Disabled");
    expect(row2).toHaveTextContent("0.8.0");
    expect(row2).toHaveTextContent("0.4.0");
    expect(within(row2).getAllByText("-").length).toBeGreaterThanOrEqual(1);

    fireEvent.click(row1);
    expect(onSelectRuntime).toHaveBeenCalledWith("rt-1");
  });

  it("filters by search, status, disabled state and tenant, and pages with cursor", async () => {
    const user = userEvent.setup();
    mockUseRuntimes.mockReturnValue({
      data: sampleRuntimes,
      isLoading: false,
      isFetching: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
      hasNextPage: true,
      fetchNextPage: mockFetchNextPage,
      isFetchingNextPage: false,
    });

    render(<Runtimes onSelectRuntime={vi.fn()} userRole="system_admin" />);

    fireEvent.change(screen.getByRole("searchbox", { name: /Search runtimes/i }), {
      target: { value: "mac" },
    });
    expect(mockUseRuntimes).toHaveBeenLastCalledWith(
      { q: "mac", status: undefined, disabled: undefined, tenant_id: undefined },
      true,
    );

    await user.click(screen.getByRole("combobox", { name: /Filter by status/i }));
    await user.click(await screen.findByRole("option", { name: "Online" }));
    expect(mockUseRuntimes).toHaveBeenLastCalledWith(
      { q: "mac", status: "online", disabled: undefined, tenant_id: undefined },
      true,
    );

    await user.click(screen.getByRole("combobox", { name: /Filter by access state/i }));
    await user.click(await screen.findByRole("option", { name: "Disabled only" }));
    expect(mockUseRuntimes).toHaveBeenLastCalledWith(
      { q: "mac", status: "online", disabled: true, tenant_id: undefined },
      true,
    );

    await user.click(screen.getByRole("combobox", { name: /Filter by tenant/i }));
    await user.click(await screen.findByRole("option", { name: /Acme Corp/i }));
    expect(mockUseRuntimes).toHaveBeenLastCalledWith(
      { q: "mac", status: "online", disabled: true, tenant_id: "t-1" },
      true,
    );

    fireEvent.click(screen.getByRole("button", { name: /Load more/i }));
    expect(mockFetchNextPage).toHaveBeenCalledTimes(1);
  });

  it("allows admin and support to read runtimes while hiding runtimes from billing", () => {
    for (const role of ["admin", "support"] as const) {
      mockUseMe.mockReturnValue({
        data: {
          user_id: `u-${role}`,
          email: `${role}@example.com`,
          role,
          tenant_id: role === "admin" ? "t-1" : null,
        },
      });
      const { unmount } = render(<Runtimes onSelectRuntime={vi.fn()} userRole={role} />);
      expect(screen.getByRole("heading", { name: "Runtimes" })).toBeInTheDocument();
      if (role === "admin") {
        expect(screen.queryByRole("combobox", { name: /Filter by tenant/i })).not.toBeInTheDocument();
      }
      unmount();
    }

    mockUseMe.mockReturnValue({
      data: { user_id: "u-bill", email: "billing@example.com", role: "billing", tenant_id: null },
    });
    render(<Runtimes onSelectRuntime={vi.fn()} userRole="billing" />);
    expect(screen.getByRole("region", { name: "Not found" })).toBeInTheDocument();
    expect(mockUseRuntimes).toHaveBeenLastCalledWith(expect.anything(), false);
  });
});
