import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen, within } from "@testing-library/react";
import { renderWithI18n } from "../../test/i18n";
import { GroupsTab } from "./groups-tab";

// Account groups tab (spec 7b.2): owner and admin only. The list and the
// create dialog come from core/groups; these tests mock the query by key.
const data = vi.hoisted(() => ({
  groups: [] as Array<Record<string, unknown>>,
  role: "owner" as "owner" | "admin",
}));

vi.mock("@tanstack/react-query", () => ({
  queryOptions: <T,>(opts: T) => opts,
  useQuery: (opts: { queryKey: unknown[] }) => {
    const key = JSON.stringify(opts.queryKey);
    if (key.includes('"groups"')) return { data: data.groups, isLoading: false };
    return { data: undefined, isLoading: false };
  },
  useMutation: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useQueryClient: () => ({ invalidateQueries: vi.fn() }),
}));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("@multica/core/paths", () => ({
  useCurrentWorkspace: () => ({ id: "ws-1", name: "Acme", slug: "acme" }),
}));
vi.mock("@multica/core/permissions", () => ({
  useCurrentMember: () => ({ userId: "user-1", role: data.role, isLoading: false }),
  useCan: () => ({ allowed: true, reason: "allowed", message: "" }),
}));

beforeEach(() => {
  data.role = "owner";
  data.groups = [
    {
      id: "g1",
      name: "Accounting",
      description: "Month-end close",
      permissions: ["billing.view", "audit.read"],
      member_count: 2,
    },
  ];
});

describe("GroupsTab", () => {
  it("lists each group with its member count", () => {
    renderWithI18n(<GroupsTab />);
    const row = screen.getByRole("button", { name: /Accounting/ });
    expect(within(row).getByText("2")).toBeInTheDocument();
  });

  it("offers group creation to an owner or admin", () => {
    data.role = "admin";
    renderWithI18n(<GroupsTab />);
    expect(screen.getByRole("button", { name: "Create group" })).toBeInTheDocument();
  });
});
