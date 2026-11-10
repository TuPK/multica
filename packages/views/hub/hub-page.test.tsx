// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { HubSource } from "@multica/core/hub";
import type { PluginPackage, SkillSummary } from "@multica/core/types";
import { renderWithI18n } from "../test/i18n";
import { NavigationProvider, type NavigationAdapter } from "../navigation";

const mocks = vi.hoisted(() => ({
  skills: [] as SkillSummary[],
  packages: [] as PluginPackage[],
  sources: [] as HubSource[],
  metas: [] as Array<Record<string, unknown>>,
  skillsError: false,
  role: "member" as "member" | "admin" | "owner",
  quota: null as Record<string, unknown> | null,
  activityEntries: [] as Array<Record<string, unknown>>,
}));

vi.mock("@tanstack/react-query", () => ({
  useQuery: (options: { queryKey?: readonly unknown[] }) => {
    const key = options.queryKey?.[0];
    if (key === "skills") {
      return { data: mocks.skills, isLoading: false, isError: mocks.skillsError, refetch: vi.fn() };
    }
    if (key === "hub-sources") {
      return { data: mocks.sources, isLoading: false, isError: false, refetch: vi.fn() };
    }
    if (key === "hub-item-meta") {
      return { data: mocks.metas, isLoading: false, isError: false, refetch: vi.fn() };
    }
    if (key === "hub-quota") {
      return { data: mocks.quota, isLoading: false, isError: false, refetch: vi.fn() };
    }
    return { data: { packages: mocks.packages }, isLoading: false, isError: false };
  },
  useInfiniteQuery: () => ({
    data: { pages: [{ entries: mocks.activityEntries, next_cursor: null }] },
    isPending: false,
    isError: false,
    hasNextPage: false,
    isFetchingNextPage: false,
    fetchNextPage: vi.fn(),
    refetch: vi.fn(),
  }),
  useMutation: () => ({ mutate: vi.fn(), isPending: false }),
}));
vi.mock("@multica/core/permissions", () => ({
  useCurrentMember: () => ({ userId: "u1", role: mocks.role, member: null, isLoading: false }),
}));
vi.mock("@multica/core/hub", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@multica/core/hub")>();
  const idle = { mutateAsync: vi.fn(), isPending: false };
  return {
    ...actual,
    hubSourcesOptions: () => ({ queryKey: ["hub-sources"] }),
    hubItemMetaListOptions: () => ({ queryKey: ["hub-item-meta"] }),
    hubQuotaOptions: () => ({ queryKey: ["hub-quota"] }),
    hubActivityInfiniteOptions: () => ({ queryKey: ["hub-activity"] }),
    usePreviewHubImport: () => idle,
    useCommitHubImport: () => idle,
    useCreateHubSource: () => idle,
    useUpdateHubSource: () => idle,
    useDeleteHubSource: () => idle,
  };
});
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("@multica/core/paths", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/paths")>()),
  useWorkspacePaths: () => ({
    skillDetail: (id: string) => `/acme/skills/${id}`,
    hubItem: (kind: string, id: string) => `/acme/hub/${kind}/${id}`,
  }),
}));
vi.mock("@multica/core/workspace/queries", () => ({
  skillListOptions: () => ({ queryKey: ["skills"] }),
}));
vi.mock("@multica/core/plugins", () => ({
  pluginPackagesOptions: () => ({ queryKey: ["plugin-packages"] }),
}));

import { HubPage } from "./hub-page";

const skill = (id: string, name: string, config: Record<string, unknown> = {}): SkillSummary =>
  ({
    id,
    workspace_id: "ws-1",
    name,
    description: `${name} description`,
    config,
    created_by: null,
    created_at: "2026-07-28T18:11:37Z",
    updated_at: "2026-07-28T18:14:40Z",
  }) as SkillSummary;

function renderPage() {
  const adapter: NavigationAdapter = {
    push: vi.fn(),
    replace: vi.fn(),
    back: vi.fn(),
    pathname: "/acme/hub",
    searchParams: new URLSearchParams(),
    hash: "",
    getShareableUrl: (p) => p,
  };
  renderWithI18n(
    <NavigationProvider value={adapter}>
      <HubPage />
    </NavigationProvider>,
  );
}

beforeEach(() => {
  mocks.skillsError = false;
  mocks.role = "member";
  mocks.quota = null;
  mocks.activityEntries = [];
  mocks.sources = [];
  mocks.metas = [];
  mocks.skills = [
    skill("s1", "deploy", { origin: { type: "github", source_url: "https://github.com/a/b" } }),
    skill("s2", "review"),
  ];
  mocks.packages = [
    {
      id: "p1",
      plugin_key: "acme.pulse",
      name: "Pulse",
      versions: [{ id: "v1", version: "1.2.0", digest: "", size_bytes: 1, published_at: "2026-08-01T00:00:00Z", installed: false }],
      created_at: "2026-08-01T00:00:00Z",
    },
  ];
});

describe("HubPage", () => {
  it("lists skills and plugins together with their sources", () => {
    renderPage();
    expect(screen.getByRole("link", { name: "deploy" })).toHaveAttribute("href", "/acme/skills/s1");
    expect(screen.getByText("Pulse")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "GitHub" })).toHaveAttribute("href", "https://github.com/a/b");
    expect(screen.getByText("Published plugin")).toBeInTheDocument();
  });

  it("filters by type", async () => {
    renderPage();
    await userEvent.click(screen.getByRole("button", { name: "Plugins" }));
    expect(screen.queryByText("deploy")).not.toBeInTheDocument();
    expect(screen.getByText("Pulse")).toBeInTheDocument();
  });

  it("shows the real sources on the Sources tab", async () => {
    mocks.sources = [
      {
        id: "src-1",
        name: "Team skills",
        type: "github",
        url: "https://github.com/acme/skills",
        config: {},
        enabled: true,
        auto_sync_interval_sec: 0,
        last_sync_at: null,
        last_sync_status: "",
        last_sync_error: null,
        item_count: 2,
        created_at: "2026-07-01T00:00:00Z",
        updated_at: "2026-07-01T00:00:00Z",
      },
    ];
    renderPage();
    await userEvent.click(screen.getByRole("button", { name: "Sources" }));
    expect(screen.getByRole("row", { name: /Team skills/ })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "deploy" })).not.toBeInTheDocument();
  });

  it("marks items with an update and filters the list to them", async () => {
    mocks.metas = [
      { id: "m1", kind: "skill", ref_id: "s1", tracking: "tracking", update_available: true },
      { id: "m2", kind: "skill", ref_id: "s2", tracking: "tracking", update_available: false },
    ];
    renderPage();
    const deploy = screen.getByRole("link", { name: "deploy" }).closest("td")!;
    expect(within(deploy).getByText("Update available")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "review" }).closest("td")!).not.toHaveTextContent("Update available");

    const chip = screen.getByRole("button", { name: "Updates available" });
    expect(chip).toHaveAttribute("aria-pressed", "false");
    await userEvent.click(chip);
    expect(chip).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("link", { name: "deploy" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "review" })).not.toBeInTheDocument();
  });

  it("offers the Add action on the header", () => {
    renderPage();
    expect(screen.getByRole("button", { name: "Add" })).toBeInTheDocument();
  });

  it("shows usage against the limits, warning when items are nearly full", () => {
    mocks.quota = {
      items: { used: 9, limit: 10 },
      storage_bytes: { used: 1000, limit: 0 },
    };
    renderPage();
    const usage = screen.getByRole("group", { name: "Usage" });
    const items = within(usage).getByRole("meter", { name: "Items" });
    expect(items).toHaveAttribute("aria-valuenow", "9");
    expect(items).toHaveAttribute("aria-valuemax", "10");
    expect(within(usage).getByText("Nearly full")).toBeInTheDocument();
    // A storage limit of 0 is unlimited: no meter, just the usage.
    expect(within(usage).queryAllByRole("meter")).toHaveLength(1);
    expect(within(usage).getByText(/used$/)).toBeInTheDocument();
  });

  it("reports a full quota as reached", () => {
    mocks.quota = { items: { used: 10, limit: 10 }, storage_bytes: { used: 1, limit: 10 } };
    renderPage();
    expect(screen.getByText("Limit reached")).toBeInTheDocument();
    expect(screen.queryByText("Nearly full")).not.toBeInTheDocument();
  });

  it("hides Export all from members", () => {
    renderPage();
    expect(screen.queryByRole("button", { name: "Export all" })).not.toBeInTheDocument();
  });

  it("offers Export all to admins", () => {
    mocks.role = "admin";
    renderPage();
    expect(screen.getByRole("button", { name: "Export all" })).toBeInTheDocument();
  });

  it("shows the workspace activity on the Activity tab", async () => {
    renderPage();
    await userEvent.click(screen.getByRole("button", { name: "Activity" }));
    expect(screen.getByText("No activity yet.")).toBeInTheDocument();
  });

  it("shows an error state when skills fail to load", () => {
    mocks.skillsError = true;
    renderPage();
    expect(screen.getByText("Couldn't load the Hub")).toBeInTheDocument();
  });
});
