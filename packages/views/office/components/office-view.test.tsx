/**
 * @vitest-environment jsdom
 *
 * The office draws the board's status columns as rooms and its issues as
 * desks. Drag-and-drop itself lives in dnd-kit; these tests cover what the
 * view decides: which rooms exist, who sits where, and when it hands the
 * board back.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ViewStoreProvider } from "@multica/core/issues/stores/view-store-context";
import { getIssueSurfaceViewStore } from "@multica/core/issues/stores/surface-view-store";
import type { Issue, IssueStatus } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { IssueContextMenuProvider } from "../../issues/actions/issue-actions-context-menu";
import { OfficeView } from "./office-view";

vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));

vi.mock("@multica/core/properties", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/properties")>()),
  propertyListOptions: () => ({ queryKey: ["properties"], queryFn: async () => [] }),
  useSetIssueProperty: () => ({ mutate: () => {} }),
  useUnsetIssueProperty: () => ({ mutate: () => {} }),
}));

vi.mock("@multica/core/workspace/hooks", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/workspace/hooks")>()),
  useActorName: () => ({ getActorName: () => "Ada" }),
}));

vi.mock("@multica/core/auth", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/auth")>()),
  useAuthStore: (selector: (state: { user: { id: string } }) => unknown) =>
    selector({ user: { id: "viewer-1" } }),
}));

vi.mock("@multica/core/agents", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/agents")>()),
  isAgentRuntimeBound: () => true,
  useAgentPresenceDetail: () => ({ availability: "offline", workload: null }),
  agentTaskSnapshotOptions: () => ({ queryKey: ["task-snapshot"], queryFn: async () => [] }),
}));

vi.mock("@multica/core/workspace/queries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/workspace/queries")>()),
  memberListOptions: () => ({
    queryKey: ["members"],
    queryFn: async () => [
      { id: "m-1", workspace_id: "ws-1", user_id: "viewer-1", role: "owner", created_at: "", name: "Grace", email: "", avatar_url: null },
    ],
  }),
  agentListOptions: () => ({
    queryKey: ["agents"],
    queryFn: async () => [
      { id: "agent-1", name: "Hopper", archived_at: null },
      { id: "agent-2", name: "Retired", archived_at: "2026-01-01T00:00:00Z" },
    ],
  }),
}));

vi.mock("@multica/core/paths", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@multica/core/paths")>();
  return {
    ...actual,
    useCurrentWorkspace: () => ({ id: "ws-1", slug: "acme" }),
    useWorkspacePaths: () => actual.paths.workspace("acme"),
  };
});

vi.mock("../../navigation", () => ({
  AppLink: ({ children, ...props }: React.ComponentProps<"a">) => <a {...props}>{children}</a>,
  useNavigation: () => ({
    push: () => {},
    openInNewTab: () => {},
    getShareableUrl: (path: string) => `https://app.example${path}`,
    pathname: "/",
  }),
  resolveClickIntent: () => "push",
  useIntentNavigate: () => () => {},
}));

function makeIssue(id: string, status: IssueStatus, assignee?: string): Issue {
  return {
    id,
    workspace_id: "ws-1",
    number: 1,
    identifier: `MUL-${id}`,
    title: `Task ${id}`,
    description: null,
    status,
    priority: "high",
    assignee_type: assignee ? "agent" : null,
    assignee_id: assignee ?? null,
    creator_type: "member",
    creator_id: "member-1",
    parent_issue_id: null,
    project_id: null,
    position: 1,
    stage: null,
    start_date: null,
    due_date: null,
    labels: [],
    metadata: {},
    properties: {},
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

class ObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
  takeRecords() {
    return [];
  }
}

const STATUSES: IssueStatus[] = ["backlog", "todo", "in_progress", "in_review", "blocked", "done"];

describe("OfficeView", () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    vi.stubGlobal("IntersectionObserver", ObserverStub);
    vi.stubGlobal("ResizeObserver", ObserverStub);
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity } },
    });
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  function setup(issues: Issue[], options: { grouping?: "status" | "assignee"; onCreateIssue?: () => void } = {}) {
    const store = getIssueSurfaceViewStore(`office-${Math.floor(Math.random() * 1e9)}`);
    store.getState().setGrouping(options.grouping ?? "status");
    const view = (list: Issue[]) => (
      <QueryClientProvider client={queryClient}>
        <ViewStoreProvider store={store}>
          <IssueContextMenuProvider>
            <OfficeView
              issues={list}
              visibleStatuses={STATUSES}
              onMoveIssue={() => {}}
              onCreateIssue={options.onCreateIssue}
            />
          </IssueContextMenuProvider>
        </ViewStoreProvider>
      </QueryClientProvider>
    );
    const result = renderWithI18n(view(issues));
    return {
      store,
      container: result.container,
      rerender: (list: Issue[]) => result.rerender(view(list)),
    };
  }

  it("builds one room per visible status and seats each issue in its room", () => {
    setup([makeIssue("a", "todo"), makeIssue("b", "in_progress", "agent-1"), makeIssue("c", "in_progress")]);

    for (const label of ["Backlog", "Todo", "In Progress", "In Review", "Blocked", "Done"]) {
      expect(screen.getByRole("region", { name: label })).toBeTruthy();
    }
    const working = screen.getByRole("region", { name: "In Progress" });
    expect(within(working).getByText("Task b")).toBeTruthy();
    expect(within(working).getByText("Task c")).toBeTruthy();
    expect(within(screen.getByRole("region", { name: "Todo" })).getByText("Task a")).toBeTruthy();
    // The room's badge counts its desks.
    expect(within(working).getByText("2")).toBeTruthy();
  });

  it("seats a worker only at an assigned desk", () => {
    const { container } = setup([makeIssue("a", "todo", "agent-1"), makeIssue("b", "todo")]);

    const desks = container.querySelectorAll("[data-office-desk]");
    expect(desks).toHaveLength(2);
    expect(container.querySelectorAll("[data-office-actor]")).toHaveLength(1);
  });

  it("opens a new issue in the room whose button was pressed", () => {
    const onCreateIssue = vi.fn();
    setup([makeIssue("a", "todo")], { onCreateIssue });

    fireEvent.click(screen.getByRole("button", { name: "New issue in In Review" }));

    expect(onCreateIssue).toHaveBeenCalledWith({ status: "in_review" });
  });

  it("moves an issue's desk to its new room when the status changes", () => {
    const { rerender } = setup([makeIssue("a", "todo")]);
    expect(within(screen.getByRole("region", { name: "Todo" })).getByText("Task a")).toBeTruthy();

    rerender([makeIssue("a", "in_review")]);

    expect(within(screen.getByRole("region", { name: "In Review" })).getByText("Task a")).toBeTruthy();
    expect(within(screen.getByRole("region", { name: "Todo" })).queryByText("Task a")).toBeNull();
  });

  it("puts every non-archived staff member in the corridor", async () => {
    setup([makeIssue("a", "todo")]);

    expect(await screen.findByRole("button", { name: "Filter by Hopper" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Filter by Grace" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Filter by Retired" })).toBeNull();
  });

  it("hands the board back for any grouping other than status", () => {
    const { store } = setup([makeIssue("a", "todo")], { grouping: "assignee" });

    expect(screen.queryByRole("region", { name: "Todo" })).toBeNull();
    expect(screen.getByText(/Group by Status to see the office/)).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Group by Status" }));

    expect(store.getState().grouping).toBe("status");
  });
});
