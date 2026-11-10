// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { PluginPackage, SkillSummary } from "@multica/core/types";
import { renderWithI18n } from "../test/i18n";
import { NavigationProvider, type NavigationAdapter } from "../navigation";

const mocks = vi.hoisted(() => ({
  skills: [] as SkillSummary[],
  packages: [] as PluginPackage[],
  skillsError: false,
}));

vi.mock("@tanstack/react-query", () => ({
  useQuery: (options: { queryKey?: readonly unknown[] }) =>
    options.queryKey?.[0] === "skills"
      ? { data: mocks.skills, isLoading: false, isError: mocks.skillsError, refetch: vi.fn() }
      : { data: { packages: mocks.packages }, isLoading: false, isError: false },
}));
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

  it("summarizes sources on the Sources tab", async () => {
    renderPage();
    await userEvent.click(screen.getByRole("button", { name: "Sources" }));
    expect(screen.getByText("Created here")).toBeInTheDocument();
    expect(screen.getByText("GitHub")).toBeInTheDocument();
  });

  it("shows an error state when skills fail to load", () => {
    mocks.skillsError = true;
    renderPage();
    expect(screen.getByText("Couldn't load the Hub")).toBeInTheDocument();
  });
});
