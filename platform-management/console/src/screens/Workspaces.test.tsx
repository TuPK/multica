// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Workspaces } from "./Workspaces";

const mockUseMe = vi.fn();
const mockUseWorkspaces = vi.fn();
const mockUseTenants = vi.fn();
const mockUsePlans = vi.fn();
const mockFetchNextPage = vi.fn();

vi.mock("../api/hooks", () => ({
  useMe: () => mockUseMe(),
  useWorkspaces: (filters: Record<string, unknown>) => mockUseWorkspaces(filters),
  useTenants: () => mockUseTenants(),
  usePlans: (enabled = true) => mockUsePlans(enabled),
}));

const sampleWorkspaces = [
  {
    id: "ws-1",
    name: "Acme Core",
    slug: "acme-core",
    tenant_id: "t-1",
    tenant_name: "Acme Corp",
    status: "active" as const,
    plan_key: "pro",
    owner_email: "owner@acme.test",
    member_count: 12,
    created_at: "2026-11-10T09:00:00Z",
    last_activity_at: "2026-11-18T12:00:00Z",
  },
  {
    id: "ws-2",
    name: "Beta Sandbox",
    slug: "beta-sandbox",
    tenant_id: "t-2",
    tenant_name: "Beta LLC",
    status: "suspended" as const,
    plan_key: "free",
    owner_email: "admin@beta.test",
    member_count: 2,
    created_at: "2026-11-12T10:00:00Z",
    last_activity_at: "2026-11-15T08:00:00Z",
  },
];

describe("Workspaces screen", () => {
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
    mockUsePlans.mockReturnValue({
      data: [
        { key: "free", name: "Free" },
        { key: "pro", name: "Pro" },
      ],
      isLoading: false,
      isError: false,
    });
    mockUseWorkspaces.mockReturnValue({
      data: sampleWorkspaces,
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
      hasNextPage: false,
      fetchNextPage: mockFetchNextPage,
      isFetchingNextPage: false,
    });
  });

  it("filters by status plan and tenant and pages with the cursor", async () => {
    const user = userEvent.setup();
    mockUseWorkspaces.mockReturnValue({
      data: sampleWorkspaces,
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
      hasNextPage: true,
      fetchNextPage: mockFetchNextPage,
      isFetchingNextPage: false,
    });

    const onSelectWorkspace = vi.fn();
    render(<Workspaces onSelectWorkspace={onSelectWorkspace} userRole="system_admin" />);

    expect(screen.getByRole("heading", { name: "Workspaces" })).toBeInTheDocument();
    expect(screen.getByText("Acme Core")).toBeInTheDocument();
    expect(screen.getByText("Beta Sandbox")).toBeInTheDocument();

    // Filter by status
    await user.click(screen.getByRole("combobox", { name: /Filter by status/i }));
    await user.click(await screen.findByRole("option", { name: "Suspended" }));
    expect(mockUseWorkspaces).toHaveBeenLastCalledWith({
      q: undefined,
      status: "suspended",
      plan_key: undefined,
      tenant_id: undefined,
    });

    // Filter by plan
    await user.click(screen.getByRole("combobox", { name: /Filter by plan/i }));
    await user.click(await screen.findByRole("option", { name: /pro/i }));
    expect(mockUseWorkspaces).toHaveBeenLastCalledWith({
      q: undefined,
      status: "suspended",
      plan_key: "pro",
      tenant_id: undefined,
    });

    // Filter by tenant
    await user.click(screen.getByRole("combobox", { name: /Filter by tenant/i }));
    await user.click(await screen.findByRole("option", { name: /Acme Corp/i }));
    expect(mockUseWorkspaces).toHaveBeenLastCalledWith({
      q: undefined,
      status: "suspended",
      plan_key: "pro",
      tenant_id: "t-1",
    });

    // Search query
    const searchInput = screen.getByRole("searchbox", { name: /Search workspaces/i });
    fireEvent.change(searchInput, { target: { value: "acme" } });
    expect(mockUseWorkspaces).toHaveBeenLastCalledWith({
      q: "acme",
      status: "suspended",
      plan_key: "pro",
      tenant_id: "t-1",
    });

    // Cursor paging
    const loadMoreBtn = screen.getByRole("button", { name: /Load more/i });
    fireEvent.click(loadMoreBtn);
    expect(mockFetchNextPage).toHaveBeenCalledTimes(1);

    // Open workspace detail
    fireEvent.click(screen.getAllByRole("button", { name: "View" })[0]!);
    expect(onSelectWorkspace).toHaveBeenCalledWith("ws-1");
  });

  it("admin sees only the rows the API returns", () => {
    mockUseMe.mockReturnValue({
      data: { user_id: "u-tadmin", email: "tadmin@example.com", role: "admin", tenant_id: "t-1" },
    });
    mockUseWorkspaces.mockReturnValue({
      data: [sampleWorkspaces[0]],
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
      hasNextPage: false,
      fetchNextPage: mockFetchNextPage,
      isFetchingNextPage: false,
    });

    render(<Workspaces onSelectWorkspace={vi.fn()} userRole="admin" />);

    expect(screen.getByText("Acme Core")).toBeInTheDocument();
    expect(screen.queryByText("Beta Sandbox")).not.toBeInTheDocument();
  });

  it("does not request /plans when signed in as support or billing", () => {
    for (const role of ["support", "billing"] as const) {
      mockUseMe.mockReturnValue({
        data: { user_id: `u-${role}`, email: `${role}@example.com`, role, tenant_id: null },
      });
      const { unmount } = render(<Workspaces onSelectWorkspace={vi.fn()} userRole={role} />);
      expect(mockUsePlans).toHaveBeenCalledWith(false);
      expect(screen.getByText("Acme Core")).toBeInTheDocument();
      unmount();
    }
  });
});
