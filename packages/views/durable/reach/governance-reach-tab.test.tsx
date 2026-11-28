import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { api } from "@multica/core/api";
import { WorkspaceSlugProvider } from "@multica/core/paths";
import { Tabs, TabsList } from "@multica/ui/components/ui/tabs";
import { renderWithI18n } from "../../test/i18n";
import { GovernanceReachTab, useReachGovernanceTab } from "./governance-reach-tab";

const flags = vi.hoisted(() => ({ open: true }));
const role = vi.hoisted(() => ({ value: "admin" as "owner" | "admin" | "member" }));
const nav = vi.hoisted(() => ({
  pathname: "/acme/governance",
  searchParams: new URLSearchParams("tab=reach"),
  push: vi.fn(),
  replace: vi.fn(),
}));

vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("@multica/core/config", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/config")>()),
  useFeatureEnabled: (key: string, fallback: boolean) =>
    key === "durable_open" ? flags.open : fallback,
}));
vi.mock("@multica/core/permissions", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/permissions")>()),
  useCurrentMember: () => ({ userId: "u-1", role: role.value, member: null, isLoading: false }),
}));
vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api: { request: vi.fn() },
}));
vi.mock("../../navigation", () => ({
  AppLink: ({ children, ...props }: React.ComponentProps<"a">) => <a {...props}>{children}</a>,
  useNavigation: () => nav,
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const request = vi.mocked(api.request);

const FULL_SNAPSHOT = {
  snapshot_date: "2026-11-27",
  agent_share_30d: 64.5,
  surfaces: [
    {
      surface: "app:gmail",
      state: "agent_operable",
      effects_30d: 30,
      agent_effects_30d: 30,
      human_minutes_30d: 10,
      gap_asks_30d: 0,
      top_ask_ids: [],
      gap_issue_id: null,
    },
    {
      surface: "channel:slack",
      state: "agent_with_confirm",
      effects_30d: 20,
      agent_effects_30d: 15,
      human_minutes_30d: 45,
      gap_asks_30d: 2,
      top_ask_ids: ["ask-1", "ask-2"],
      gap_issue_id: null,
    },
    {
      surface: "external:stripe.com",
      state: "declared_only",
      effects_30d: 8,
      agent_effects_30d: 0,
      human_minutes_30d: 120,
      gap_asks_30d: 5,
      top_ask_ids: ["ask-3"],
      gap_issue_id: null,
    },
    {
      surface: "area:ops:billing",
      state: "human_only",
      effects_30d: 0,
      agent_effects_30d: 0,
      human_minutes_30d: 80,
      gap_asks_30d: 4,
      top_ask_ids: ["ask-4"],
      gap_issue_id: "issue-existing",
    },
    {
      surface: "app:notion",
      state: "unknown",
      effects_30d: 2,
      agent_effects_30d: 0,
      human_minutes_30d: 5,
      gap_asks_30d: 0,
      top_ask_ids: [],
      gap_issue_id: null,
    },
  ],
};

function renderTab() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <WorkspaceSlugProvider slug="acme">
        <GovernanceReachTab />
      </WorkspaceSlugProvider>
    </QueryClientProvider>,
  );
}

function TabHarness() {
  const reachTab = useReachGovernanceTab();
  return (
    <Tabs defaultValue="goals">
      <TabsList>{reachTab.triggers}</TabsList>
      {reachTab.contents}
    </Tabs>
  );
}

beforeEach(() => {
  flags.open = true;
  role.value = "admin";
  vi.clearAllMocks();
});

afterEach(() => {
  cleanup();
  request.mockReset();
});

describe("GovernanceReachTab", () => {
  it("hides the tab and content when durable_open is off", () => {
    flags.open = false;
    const { container } = renderTab();
    expect(container.innerHTML).toBe("");

    renderWithI18n(<TabHarness />);
    expect(screen.queryByRole("tab", { name: "Reach" })).not.toBeInTheDocument();
  });

  it("renders a distinct state label and class for every state and scales tile size by human_minutes_30d", async () => {
    request.mockResolvedValueOnce(FULL_SNAPSHOT);
    renderTab();

    expect(await screen.findByText("64.5%")).toBeInTheDocument();

    const grid = screen.getByRole("region", { name: "Work surfaces" });
    const tiles = within(grid).getAllByRole("button");
    expect(tiles).toHaveLength(5);

    const expectedTileLabels = new Map([
      ["app:gmail", "Agent operable"],
      ["channel:slack", "Needs confirmation"],
      ["external:stripe.com", "Declared only"],
      ["area:ops:billing", "Human only"],
      ["app:notion", "Unknown"],
    ]);
    for (const tile of tiles) {
      const surfaceName = tile.getAttribute("data-surface") ?? "";
      const expectedLabel = expectedTileLabels.get(surfaceName);
      expect(expectedLabel).toBeTruthy();
      expect(within(tile).getByText(expectedLabel!)).toBeInTheDocument();
    }

    const stateClasses = tiles.map((tile) => tile.className);
    expect(new Set(stateClasses).size).toBe(5);

    const bigTile = within(grid).getByRole("button", { name: /stripe\.com/i });
    const smallTile = within(grid).getByRole("button", { name: /notion/i });
    const bigHeight = Number.parseInt(bigTile.style.minHeight, 10);
    const smallHeight = Number.parseInt(smallTile.style.minHeight, 10);
    expect(bigHeight).toBeGreaterThan(smallHeight);
  });

  it("ranks the gaps list by human_minutes_30d descending", async () => {
    request.mockResolvedValueOnce(FULL_SNAPSHOT);
    renderTab();

    const gapsRegion = await screen.findByRole("region", { name: "Gaps by human time" });
    const items = within(gapsRegion).getAllByRole("listitem");
    expect(items.map((el) => el.getAttribute("data-surface"))).toEqual([
      "external:stripe.com",
      "area:ops:billing",
      "channel:slack",
      "app:gmail",
      "app:notion",
    ]);
  });

  it("shows Close gap only for owners and admins, not members", async () => {
    role.value = "member";
    request.mockResolvedValueOnce(FULL_SNAPSHOT);
    renderTab();

    await screen.findByRole("region", { name: "Gaps by human time" });
    expect(screen.queryByRole("button", { name: "Close gap" })).not.toBeInTheDocument();
  });

  it("posts POST /api/reach/gaps with surface and reuses the same issue on a second click", async () => {
    request
      .mockResolvedValueOnce(FULL_SNAPSHOT)
      .mockResolvedValueOnce({ issue_id: "issue-gap-1" })
      .mockResolvedValueOnce(FULL_SNAPSHOT)
      .mockResolvedValueOnce({ issue_id: "issue-gap-1" })
      .mockResolvedValueOnce(FULL_SNAPSHOT);

    renderTab();

    const gapsRegion = await screen.findByRole("region", { name: "Gaps by human time" });
    const firstRow = within(gapsRegion).getAllByRole("listitem")[0]!;
    const closeBtn = within(firstRow).getByRole("button", { name: "Close gap" });

    fireEvent.click(closeBtn);
    await waitFor(() => expect(nav.push).toHaveBeenCalledTimes(1));

    const postCalls = () =>
      request.mock.calls.filter(([path]) => path === "/api/reach/gaps");
    expect(postCalls()).toHaveLength(1);
    expect(JSON.parse((postCalls()[0]?.[1] as { body: string }).body)).toEqual({
      surface: "external:stripe.com",
    });
    expect(nav.push).toHaveBeenNthCalledWith(1, "/acme/issues/issue-gap-1");

    fireEvent.click(closeBtn);
    await waitFor(() => expect(nav.push).toHaveBeenCalledTimes(2));
    expect(postCalls()).toHaveLength(2);
    expect(nav.push).toHaveBeenNthCalledWith(2, "/acme/issues/issue-gap-1");
  });

  it("still notifies and navigates if unmounted while Close gap is in flight", async () => {
    let finish!: (value: unknown) => void;
    request
      .mockResolvedValueOnce(FULL_SNAPSHOT)
      .mockImplementationOnce(() => new Promise((resolve) => (finish = resolve)));

    const { unmount } = renderTab();
    const gapsRegion = await screen.findByRole("region", { name: "Gaps by human time" });
    const firstRow = within(gapsRegion).getAllByRole("listitem")[0]!;

    fireEvent.click(within(firstRow).getByRole("button", { name: "Close gap" }));
    await waitFor(() =>
      expect(request).toHaveBeenCalledWith(
        "/api/reach/gaps",
        expect.objectContaining({ method: "POST" }),
      ),
    );
    unmount();

    await act(async () => finish({ issue_id: "issue-late" }));
    await waitFor(() => expect(nav.push).toHaveBeenCalledWith("/acme/issues/issue-late"));
    expect(toast.success).toHaveBeenCalled();
  });

  it("shows an empty state when there is no snapshot yet", async () => {
    request.mockResolvedValueOnce({
      snapshot_date: null,
      agent_share_30d: 0,
      surfaces: [],
    });
    renderTab();

    expect(await screen.findByText("No reach snapshot yet.")).toBeInTheDocument();
  });
});
