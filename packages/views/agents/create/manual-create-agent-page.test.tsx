// @vitest-environment jsdom

import type { ReactNode } from "react";
import { act, cleanup, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { EMPTY_AGENT_DRAFT } from "@multica/core/agents";
import { renderWithI18n } from "../../test/i18n";
import { ManualCreateAgentPage } from "./manual-create-agent-page";

const nav = vi.hoisted(() => ({
  pathname: "/acme/agents/new/manual",
  searchParams: new URLSearchParams(),
  push: vi.fn(),
  replace: vi.fn(),
}));

// What the page reads from the server, and what it hands to its children.
const fixtures = vi.hoisted(() => ({
  agents: [] as Array<{ id: string; name: string; home_squad_id: string | null }>,
  projects: [] as Array<{ id: string; title: string; squad_id: string | null; status: string }>,
  panel: null as null | {
    projectField?: {
      projects: Array<{ id: string }>;
      value: string | null;
      onChange: (id: string | null) => void;
    };
  },
  submit: null as null | { squadId: string | null },
}));

vi.mock("@multica/core/api", () => ({ api: {} }));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-test" }));
vi.mock("@multica/core/paths", () => ({
  useWorkspacePaths: () => ({
    newAgent: () => "/acme/agents/new",
    newAgentManual: () => "/acme/agents/new/manual",
  }),
}));
vi.mock("@multica/core/workspace/queries", () => ({
  agentListOptions: () => ({ queryKey: ["agents"], queryFn: () => fixtures.agents }),
}));
vi.mock("@multica/core/projects/queries", () => ({
  projectListOptions: () => ({ queryKey: ["projects"], queryFn: () => fixtures.projects }),
}));
vi.mock("../../navigation", () => ({
  useNavigation: () => nav,
  useBackOrReplace: () => vi.fn(),
}));
vi.mock("./use-create-agent-form", () => ({
  useCreateAgentForm: () => ({
    draft: { ...EMPTY_AGENT_DRAFT, runtimeId: "rt-1" },
    setDraft: vi.fn(),
    runtimes: [],
    runtimesLoading: false,
    runtimesSettled: true,
    usableRuntimes: [],
    selectedRuntime: null,
    members: [],
    currentUserId: "user-1",
    draftReady: true,
  }),
}));
vi.mock("./use-create-agent-submit", () => ({
  useCreateAgentSubmit: (options: { squadId: string | null }) => {
    fixtures.submit = options;
    return {
    creating: false,
    nameError: null,
    formError: null,
    clearNameError: vi.fn(),
    create: vi.fn(),
    };
  },
}));
vi.mock("./use-duplicate-draft-seed", () => ({ useDuplicateDraftSeed: () => {} }));
vi.mock("./use-manual-draft-sync", () => ({
  useManualDraftSync: () => {},
  clearManualDraftForOwner: vi.fn(),
}));
vi.mock("./agent-configuration-panel", () => ({
  AgentConfigurationPanel: (props: typeof fixtures.panel) => {
    fixtures.panel = props;
    return null;
  },
}));
vi.mock("./create-agent-footer", () => ({ CreateAgentFooter: () => null }));
vi.mock("./create-shell", () => ({
  AgentCreateShell: ({ children }: { children: ReactNode }) => <>{children}</>,
  AgentCreateChip: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

function mount() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const page = () => (
    <QueryClientProvider client={qc}>
      <ManualCreateAgentPage />
    </QueryClientProvider>
  );
  const view = renderWithI18n(page());
  return { refresh: () => view.rerender(page()) };
}

describe("ManualCreateAgentPage runtime guard", () => {
  beforeEach(() => {
    nav.pathname = "/acme/agents/new/manual";
    nav.searchParams = new URLSearchParams();
    nav.replace.mockClear();
  });
  afterEach(cleanup);

  it("sends a visit without a runtime back to step 1", () => {
    mount();
    expect(nav.replace).toHaveBeenCalledWith("/acme/agents/new");
  });

  it("stays put when a runtime is chosen", () => {
    nav.searchParams = new URLSearchParams("runtime=rt-1");
    mount();
    expect(nav.replace).not.toHaveBeenCalled();
  });

  // Desktop: creating replaces the tab URL with the done page while this
  // route is still rendered.
  it("does not redirect once the location has moved to the done page", () => {
    nav.searchParams = new URLSearchParams("runtime=rt-1");
    const view = mount();
    nav.pathname = "/acme/agents/new/done/agent-1";
    nav.searchParams = new URLSearchParams();
    view.refresh();
    expect(nav.replace).not.toHaveBeenCalled();
  });
});

describe("ManualCreateAgentPage duplicate project", () => {
  const source = { id: "src", name: "Source", home_squad_id: "sq-1" };

  beforeEach(() => {
    nav.pathname = "/acme/agents/new/manual";
    nav.searchParams = new URLSearchParams("duplicate=src");
    fixtures.agents = [source];
    fixtures.projects = [
      { id: "p1", title: "Alpha", squad_id: "sq-1", status: "in_progress" },
      { id: "p2", title: "Beta", squad_id: "sq-2", status: "in_progress" },
      { id: "done", title: "Old", squad_id: "sq-3", status: "completed" },
    ];
    fixtures.panel = null;
    fixtures.submit = null;
  });
  afterEach(cleanup);

  it("joins the source's open project squad until the user picks another", async () => {
    mount();
    await waitFor(() => expect(fixtures.submit?.squadId).toBe("sq-1"));
    expect(fixtures.panel?.projectField?.value).toBe("p1");
  });

  it("joins the squad of the project the user picks", async () => {
    mount();
    await waitFor(() => expect(fixtures.submit?.squadId).toBe("sq-1"));
    act(() => fixtures.panel?.projectField?.onChange("p2"));
    await waitFor(() => expect(fixtures.submit?.squadId).toBe("sq-2"));
  });

  it("joins no squad when the user picks no project", async () => {
    mount();
    await waitFor(() => expect(fixtures.submit?.squadId).toBe("sq-1"));
    act(() => fixtures.panel?.projectField?.onChange(null));
    await waitFor(() => expect(fixtures.submit?.squadId).toBeNull());
  });

  it("does not offer the picker to a new agent, which uses the ?squad= param", async () => {
    nav.searchParams = new URLSearchParams("runtime=rt-1&squad=sq-9");
    mount();
    await waitFor(() => expect(fixtures.submit?.squadId).toBe("sq-9"));
    expect(fixtures.panel?.projectField).toBeUndefined();
  });

  it("offers only open projects that have a squad", async () => {
    mount();
    await waitFor(() =>
      expect(fixtures.panel?.projectField?.projects.map((p) => p.id)).toEqual(["p1", "p2"]),
    );
  });
});
