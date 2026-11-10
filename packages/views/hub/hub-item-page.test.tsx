// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { PluginPackage } from "@multica/core/types";
import { renderWithI18n } from "../test/i18n";
import { NavigationProvider, type NavigationAdapter } from "../navigation";

const mocks = vi.hoisted(() => ({
  activity: [] as Array<Record<string, unknown>>,
  item: undefined as undefined | Record<string, unknown>,
  packages: [] as PluginPackage[],
  installs: [] as Array<Record<string, unknown>>,
  meta: undefined as undefined | Record<string, unknown>,
}));

vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));

vi.mock("@multica/core/paths", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/paths")>()),
  useWorkspacePaths: () => ({ hub: () => "/acme/hub" }),
}));

vi.mock("@multica/core/auth", () => ({
  useAuthStore: (selector: (state: { user: { id: string } }) => unknown) =>
    selector({ user: { id: "me" } }),
}));

vi.mock("@multica/core/hub", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@multica/core/hub")>();
  return {
    ...actual,
    useHubItem: () => ({ item: mocks.item, isLoading: false, isError: false }),
    useHubItemMeta: () => ({ meta: mocks.meta, isLoading: false }),
    hubItemMetaListOptions: () => ({ queryKey: ["hub-item-meta"], queryFn: async () => [] }),
    hubUpdateCheckOptions: () => ({ queryKey: ["hub-update-check"], queryFn: async () => null }),
    hubUpdateDiffOptions: () => ({ queryKey: ["hub-update-diff"], queryFn: async () => null }),
    useApplyHubUpdate: () => ({ mutateAsync: vi.fn(), isPending: false }),
    useForkHubItem: () => ({ mutateAsync: vi.fn(), isPending: false }),
    hubFileListOptions: () => ({ queryKey: ["hub-list"], queryFn: async () => [] }),
    hubFileContentOptions: () => ({ queryKey: ["hub-content"], queryFn: async () => null }),
    useSaveHubFile: () => ({ mutateAsync: vi.fn(), isPending: false }),
    useDeleteHubFile: () => ({ mutateAsync: vi.fn(), isPending: false }),
    useRenameHubFile: () => ({ mutateAsync: vi.fn(), isPending: false }),
    useSavePluginDraft: () => ({ mutateAsync: vi.fn(), isPending: false }),
    hubActivityInfiniteOptions: () => ({
      queryKey: ["hub-activity-item"],
      queryFn: async () => ({ entries: mocks.activity, next_cursor: null }),
      initialPageParam: null,
      getNextPageParam: () => undefined,
    }),
    fetchHubBundleDownload: vi.fn(async () => ({ kind: "url", url: "https://files.example.com/sales-kit.zip", version: "1" })),
  };
});

vi.mock("@multica/core/plugins", () => ({
  pluginPackagesOptions: () => ({
    queryKey: ["plugin-packages"],
    queryFn: async () => ({ packages: mocks.packages }),
  }),
  pluginInstallationsOptions: () => ({
    queryKey: ["plugin-installs"],
    queryFn: async () => ({ plugins: mocks.installs }),
  }),
}));

vi.mock("@multica/core/workspace/queries", () => ({
  memberListOptions: () => ({ queryKey: ["members"], queryFn: async () => [] }),
  skillDetailOptions: () => ({ queryKey: ["skill"], queryFn: async () => null }),
  skillVersionsOptions: () => ({ queryKey: ["skill-versions"], queryFn: async () => [] }),
  agentListOptions: () => ({ queryKey: ["agents"], queryFn: async () => [] }),
  selectSkillAssignments: () => new Map(),
}));

vi.mock("../skills/hooks/use-can-edit-skill", () => ({ useCanEditSkill: () => false }));

vi.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getTotalSize: () => count * 32,
    getVirtualItems: () =>
      Array.from({ length: count }, (_, index) => ({ index, key: index, start: index * 32 })),
  }),
}));

vi.mock("@multica/ui/hooks/use-mobile", () => ({ useIsMobile: () => false }));

import { HubItemPage } from "./hub-item-page";

const adapter: NavigationAdapter = {
  push: vi.fn(),
  replace: vi.fn(),
  back: vi.fn(),
  pathname: "/acme/hub/plugin/p1",
  searchParams: new URLSearchParams(),
  hash: "",
  getShareableUrl: (p) => p,
};

function renderPage(kind: string, refId: string) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return renderWithI18n(
    <QueryClientProvider client={client}>
      <NavigationProvider value={adapter}>
        <HubItemPage kind={kind} refId={refId} />
      </NavigationProvider>
    </QueryClientProvider>,
  );
}

const plugin: PluginPackage = {
  id: "p1",
  plugin_key: "acme.sales-kit",
  name: "Sales kit",
  created_at: "2026-07-01T00:00:00Z",
  versions: [
    {
      id: "v2",
      version: "2.0.0",
      digest: "abcdef1234567890abcdef",
      size_bytes: 2048,
      published_at: "2026-07-02T00:00:00Z",
      installed: true,
    },
  ],
};

beforeEach(() => {
  mocks.activity = [];
  mocks.item = undefined;
  mocks.packages = [];
  mocks.installs = [];
  mocks.meta = undefined;
});

describe("HubItemPage", () => {
  it("says the item is not in the Hub for an unknown kind", () => {
    renderPage("widget", "x1");
    expect(screen.getByText("This item isn't in the Hub.")).toBeInTheDocument();
  });

  it("says the item is not in the Hub when the list does not contain it", () => {
    renderPage("plugin", "missing");
    expect(screen.getByText("This item isn't in the Hub.")).toBeInTheDocument();
  });

  it("shows an update banner without a review action for a member who cannot edit", () => {
    mocks.item = {
      key: "plugin:p1",
      kind: "plugin",
      refId: "p1",
      name: "Sales kit",
      description: "acme.sales-kit",
      source: "plugin",
      sourceUrl: null,
      version: "2.0.0",
      labels: [],
      updatedAt: "2026-07-02T00:00:00Z",
      createdAt: "2026-07-01T00:00:00Z",
    };
    mocks.packages = [plugin];
    mocks.meta = { tracking: "tracking", update_available: true, local_modified: false };
    renderPage("plugin", "p1");
    expect(
      screen.getByText("A newer version of this item is available from its source."),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Review update" })).not.toBeInTheDocument();
  });

  it("shows a plugin's identity, its source, and its installed state under Usage", async () => {
    mocks.item = {
      key: "plugin:p1",
      kind: "plugin",
      refId: "p1",
      name: "Sales kit",
      description: "acme.sales-kit",
      source: "plugin",
      sourceUrl: null,
      version: "2.0.0",
      labels: [],
      updatedAt: "2026-07-02T00:00:00Z",
      createdAt: "2026-07-01T00:00:00Z",
    };
    mocks.packages = [plugin];
    mocks.installs = [
      {
        id: "i1",
        plugin_key: "acme.sales-kit",
        name: "Sales kit",
        version: "2.0.0",
        package_version_id: "v2",
        enabled: true,
      },
    ];
    const user = userEvent.setup();
    renderPage("plugin", "p1");

    expect(screen.getByRole("heading", { name: "Sales kit" })).toBeInTheDocument();
    expect(screen.getByText("Published plugin")).toBeInTheDocument();
    // The version and checksum come from the package query, which resolves after the first paint.
    expect(await screen.findByText("v2.0.0")).toBeInTheDocument();
    expect(screen.getByText("abcdef123456")).toBeInTheDocument();

    const tabs = screen.getByRole("tablist");
    expect(within(tabs).getByRole("tab", { name: "Files" })).toBeInTheDocument();
    expect(within(tabs).getByRole("tab", { name: "Overview" })).toBeInTheDocument();
    expect(within(tabs).getByRole("tab", { name: "Versions" })).toBeInTheDocument();
    expect(within(tabs).getByRole("tab", { name: "Usage" })).toBeInTheDocument();

    await user.click(within(tabs).getByRole("tab", { name: "Usage" }));
    await waitFor(() =>
      expect(screen.getByText("Installed in this workspace")).toBeInTheDocument(),
    );
    expect(screen.getByText("Version 2.0.0")).toBeInTheDocument();
    expect(screen.getByText("Enabled")).toBeInTheDocument();
  });

  it("lists this item's activity, newest first, with the detail of each entry", async () => {
    mocks.item = pluginItem();
    mocks.packages = [plugin];
    mocks.activity = [
      activityEntry({ id: "a1", action: "update_applied", actor_name: "Ada", detail: { version: "2.0.0" } }),
      activityEntry({ id: "a2", action: "future_action", actor_name: "", detail: {} }),
    ];
    const user = userEvent.setup();
    renderPage("plugin", "p1");

    await user.click(await screen.findByRole("tab", { name: "Activity" }));
    const list = await screen.findByRole("list", { name: "Activity" });
    const entries = within(list).getAllByRole("listitem");
    expect(entries).toHaveLength(2);
    expect(within(entries[0]!).getByText("Update applied")).toBeInTheDocument();
    expect(within(entries[0]!).getByText("Version 2.0.0")).toBeInTheDocument();
    expect(within(entries[0]!).getByText("Ada")).toBeInTheDocument();
    // An action the client does not know yet still gets a readable label.
    expect(within(entries[1]!).getByText("Activity recorded")).toBeInTheDocument();
    expect(within(entries[1]!).getByText("Someone")).toBeInTheDocument();
  });

  it("says so when the item has no activity yet", async () => {
    mocks.item = pluginItem();
    mocks.packages = [plugin];
    const user = userEvent.setup();
    renderPage("plugin", "p1");

    await user.click(await screen.findByRole("tab", { name: "Activity" }));
    expect(await screen.findByText("No activity yet.")).toBeInTheDocument();
  });

  it("opens a fresh bundle link for the item", async () => {
    const open = vi.spyOn(window, "open").mockImplementation(() => null);
    const { fetchHubBundleDownload } = await import("@multica/core/hub");
    mocks.item = pluginItem();
    mocks.packages = [plugin];
    const user = userEvent.setup();
    renderPage("plugin", "p1");

    await user.click(screen.getByRole("button", { name: "Download bundle" }));
    await waitFor(() =>
      expect(open).toHaveBeenCalledWith("https://files.example.com/sales-kit.zip", "_blank", "noopener,noreferrer"),
    );
    expect(fetchHubBundleDownload).toHaveBeenCalledWith("ws-1", "plugin", "p1");
    open.mockRestore();
  });
});

function pluginItem() {
  return {
    key: "plugin:p1",
    kind: "plugin",
    refId: "p1",
    name: "Sales kit",
    description: "acme.sales-kit",
    source: "plugin",
    sourceUrl: null,
    version: "2.0.0",
    labels: [],
    updatedAt: "2026-07-02T00:00:00Z",
    createdAt: "2026-07-01T00:00:00Z",
  };
}

function activityEntry(overrides: Record<string, unknown>) {
  return {
    actor_type: "member",
    actor_id: null,
    kind: "plugin",
    ref_id: "p1",
    item_name: "Sales kit",
    source_id: null,
    created_at: "2026-07-02T00:00:00Z",
    ...overrides,
  };
}
