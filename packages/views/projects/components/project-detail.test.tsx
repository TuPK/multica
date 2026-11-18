import React from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, screen, waitFor, within } from "@testing-library/react";
import { onlineManager, QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { setApiInstance } from "@multica/core/api";
import type { ApiClient } from "@multica/core/api/client";
import { projectKeys } from "@multica/core/projects/queries";
import userEvent from "@testing-library/user-event";
import type { Project } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { NavigationProvider, type NavigationAdapter } from "../../navigation";
import { ProjectDetail } from "./project-detail";

const mocks = vi.hoisted(() => ({
  role: "admin",
  copyText: vi.fn(),
  deleteProject: vi.fn(),
  getProject: vi.fn(),
  setProjectAccess: vi.fn(),
  listProjectMembers: vi.fn(),
  addProjectMembers: vi.fn(),
  removeProjectMember: vi.fn(),
  getShareableUrl: vi.fn((path: string) => `https://app.example${path}`),
  push: vi.fn(),
  recordVisit: vi.fn(),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
}));

vi.mock("@multica/ui/lib/clipboard", () => ({
  copyText: mocks.copyText,
}));

vi.mock("../../workflow/project-workflow-menu", () => ({
  ProjectWorkflowMenuHost: ({ children }: { children: (submenu: null) => React.ReactNode }) => children(null),
}));
vi.mock("../../workflow/project-workflow-row", () => ({ ProjectWorkflowControl: () => null }));

vi.mock("@multica/core/pins", () => ({
  pinListOptions: () => ({ queryKey: ["pins"], queryFn: async () => [] }),
  useCreatePin: () => ({ mutate: vi.fn() }),
  useDeletePin: () => ({ mutate: vi.fn() }),
}));

vi.mock("@multica/core/workspace/queries", () => ({
  memberListOptions: () => ({
    queryKey: ["members"],
    queryFn: async () => [
      { user_id: "user-1", name: "User One", role: mocks.role },
      { user_id: "user-2", name: "Alice Member", role: "member" },
    ],
  }),
  agentListOptions: () => ({ queryKey: ["agents"], queryFn: async () => [] }),
}));

vi.mock("@multica/core/hooks", () => ({
  useWorkspaceId: () => "workspace-1",
}));

vi.mock("@multica/core/auth", () => ({
  useAuthStore: (selector: (state: { user: { id: string } }) => unknown) =>
    selector({ user: { id: "user-1" } }),
}));

vi.mock("@multica/core/chat", () => ({
  useRecentContextStore: (
    selector: (state: { recordVisit: typeof mocks.recordVisit }) => unknown,
  ) => selector({ recordVisit: mocks.recordVisit }),
}));

vi.mock("@multica/core/paths", () => ({
  useWorkspacePaths: () => ({
    projects: () => "/test-workspace/projects",
  }),
}));

vi.mock("@multica/core/workspace/hooks", () => ({
  useActorName: () => ({ getActorName: () => "User One" }),
}));

vi.mock("sonner", () => ({
  toast: { success: mocks.toastSuccess, error: mocks.toastError },
}));

vi.mock("react-resizable-panels", () => ({
  useDefaultLayout: () => ({
    defaultLayout: undefined,
    onLayoutChanged: vi.fn(),
  }),
  usePanelRef: () => ({
    current: {
      isCollapsed: () => false,
      expand: vi.fn(),
      collapse: vi.fn(),
    },
  }),
}));

vi.mock("@multica/ui/hooks/use-mobile", () => ({
  useIsMobile: () => false,
}));

vi.mock("@multica/ui/components/common/emoji-picker", () => ({
  EmojiPicker: () => null,
}));

vi.mock("@multica/ui/components/ui/resizable", () => ({
  ResizablePanelGroup: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  ResizablePanel: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  ResizableHandle: () => null,
}));

vi.mock("@multica/ui/components/ui/dropdown-menu", () => ({
  DropdownMenu: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  DropdownMenuTrigger: ({ render }: { render: React.ReactNode }) => <>{render}</>,
  DropdownMenuContent: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  DropdownMenuItem: ({
    children,
    onClick,
    disabled,
  }: {
    children: React.ReactNode;
    onClick?: () => void;
    disabled?: boolean;
  }) => (
    <button type="button" onClick={onClick} disabled={disabled}>
      {children}
    </button>
  ),
  DropdownMenuSeparator: () => <hr />,
}));

vi.mock("@multica/ui/components/ui/popover", () => ({
  Popover: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  PopoverTrigger: ({ render }: { render: React.ReactNode }) => <>{render}</>,
  PopoverContent: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}));

vi.mock("@multica/ui/components/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ render }: { render: React.ReactNode }) => <>{render}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}));

vi.mock("@multica/ui/components/ui/sheet", () => ({
  Sheet: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  SheetContent: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}));

vi.mock("../../editor", () => ({
  TitleEditor: ({ defaultValue }: { defaultValue: string }) => (
    <div>{defaultValue}</div>
  ),
  ContentEditor: () => null,
}));

vi.mock("../../common/actor-avatar", () => ({
  ActorAvatar: () => null,
}));

vi.mock("../../issues/components/priority-icon", () => ({
  PriorityIcon: () => null,
}));

vi.mock("./project-resources-section", () => ({
  ProjectResourcesSection: () => null,
}));

vi.mock("./project-start-date-picker", () => ({
  ProjectStartDatePicker: () => null,
}));

vi.mock("./project-due-date-picker", () => ({
  ProjectDueDatePicker: () => null,
}));

vi.mock("../../issues/surface/issue-surface", () => ({
  IssueSurface: ({
    renderHeader,
  }: {
    renderHeader?: (ctx: { controller: unknown }) => React.ReactNode;
  }) => <>{renderHeader?.({ controller: {} })}</>,
}));

vi.mock("../../issue-views/view-header", () => ({
  IssueViewHeader: ({
    baseMenu,
    trailing,
  }: {
    baseMenu?: React.ReactNode;
    trailing?: React.ReactNode;
  }) => (
    <header>
      {baseMenu}
      {trailing}
    </header>
  ),
}));

vi.mock("../../layout/animated-right-sidebar", () => ({
  AnimatedRightSidebar: ({ children }: { children: React.ReactNode }) => (
    <aside>{children}</aside>
  ),
  getAnimatedRightSidebarInitialOpen: () => true,
  rightSidebarPanelMotionProps: {},
  useRightSidebarShortcut: vi.fn(),
  useAnimatedRightSidebarState: () => ({
    open: true,
    visualOpen: true,
    motionEnabled: false,
    beginToggle: vi.fn(),
    handleResize: vi.fn(),
  }),
}));

const PROJECT: Project = {
  id: "project-1",
  workspace_id: "workspace-1",
  title: "Launch Plan",
  description: null,
  icon: null,
  status: "in_progress",
  priority: "high",
  lead_type: null,
  lead_id: null,
  start_date: null,
  due_date: null,
  created_at: "2026-06-01T00:00:00Z",
  updated_at: "2026-06-01T00:00:00Z",
  issue_count: 3,
  done_count: 1,
  resource_count: 0,
};

let qc: QueryClient;
let pending: ReturnType<typeof deferred>;

function deferred() {
  let resolve!: () => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<void>((res, rej) => { resolve = res; reject = rej; });
  return { promise, resolve, reject };
}

function renderProjectDetail() {
  qc.setQueryData(["members"], [
    { user_id: "user-1", name: "User One", role: mocks.role },
    { user_id: "user-2", name: "Alice Member", role: "member" },
  ]);
  const adapter: NavigationAdapter = {
    push: mocks.push,
    replace: vi.fn(),
    back: vi.fn(),
    pathname: "/test-workspace/projects/project-1",
    searchParams: new URLSearchParams(),
    hash: "",
    getShareableUrl: mocks.getShareableUrl,
  };

  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <NavigationProvider value={adapter}>
        <ProjectDetail projectId={PROJECT.id} />
      </NavigationProvider>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  mocks.role = "admin";
  qc = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  qc.setQueryData(projectKeys.detail("workspace-1", PROJECT.id), PROJECT);
  qc.setQueryData(["agents"], []);
  qc.setQueryData(["pins"], []);
  pending = deferred();
  mocks.copyText.mockReset().mockResolvedValue(true);
  mocks.deleteProject.mockReset().mockReturnValue(pending.promise);
  mocks.getProject.mockReset().mockResolvedValue(PROJECT);
  mocks.setProjectAccess.mockReset().mockResolvedValue({
    project_id: PROJECT.id,
    access: "restricted",
    auto_added: 0,
  });
  mocks.listProjectMembers.mockReset().mockResolvedValue({ members: [] });
  mocks.addProjectMembers.mockReset().mockResolvedValue({ added: 1 });
  mocks.removeProjectMember.mockReset().mockResolvedValue(undefined);
  setApiInstance({
    deleteProject: mocks.deleteProject,
    getProject: mocks.getProject,
    setProjectAccess: mocks.setProjectAccess,
    listProjectMembers: mocks.listProjectMembers,
    addProjectMembers: mocks.addProjectMembers,
    removeProjectMember: mocks.removeProjectMember,
  } as unknown as ApiClient);
  mocks.getShareableUrl.mockClear();
  mocks.push.mockReset();
  mocks.recordVisit.mockReset();
  mocks.toastSuccess.mockReset();
  mocks.toastError.mockReset();
});

afterEach(() => {
  onlineManager.setOnline(true);
  qc.clear();
});

describe("ProjectDetail sharing", () => {
  it("copies the platform shareable URL instead of the renderer URL", async () => {
    const user = userEvent.setup();
    renderProjectDetail();

    await user.click(screen.getByRole("button", { name: "Copy link" }));

    expect(mocks.getShareableUrl).toHaveBeenCalledWith(
      "/test-workspace/projects/project-1",
    );
    expect(mocks.copyText).toHaveBeenCalledWith(
      "https://app.example/test-workspace/projects/project-1",
    );
  });
});

describe("ProjectDetail project deletion", () => {
  it("allows cancelling an offline detail deletion without executing it on reconnect", async () => {
    onlineManager.setOnline(false);
    renderProjectDetail();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Delete project" }));
    await user.click(within(screen.getByRole("alertdialog")).getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(mocks.toastError).toHaveBeenCalledWith(
      "You're offline. Reconnect and try deleting again.",
    ));
    expect(mocks.deleteProject).not.toHaveBeenCalled();
    expect(qc.getQueryData(projectKeys.detail("workspace-1", PROJECT.id))).toEqual(PROJECT);
    const dialog = screen.getByRole("alertdialog");
    expect(within(dialog).getByRole("button", { name: "Delete" })).toBeEnabled();
    const cancel = within(dialog).getByRole("button", { name: "Cancel" });
    expect(cancel).toBeEnabled();
    await user.click(cancel);
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    await act(async () => {
      onlineManager.setOnline(true);
      await qc.resumePausedMutations();
    });
    expect(mocks.deleteProject).not.toHaveBeenCalled();
    expect(mocks.push).not.toHaveBeenCalled();
  });

  it("requires confirmation and navigates only after deletion succeeds", async () => {
    const user = userEvent.setup();
    renderProjectDetail();

    await user.click(screen.getByRole("button", { name: "Delete project" }));

    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
    expect(mocks.deleteProject).not.toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: "Delete" }));

    await waitFor(() => expect(mocks.deleteProject).toHaveBeenCalledWith(PROJECT.id));
    expect(mocks.push).not.toHaveBeenCalled();
    const dialog = screen.getByRole("alertdialog");
    const confirm = within(dialog).getByRole("button", { name: "Delete" });
    expect(confirm).toBeDisabled();
    expect(confirm).toHaveAttribute("aria-busy", "true");
    expect(confirm.querySelector("svg")).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Cancel" })).toBeDisabled();
    await user.keyboard("{Escape}");
    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
    await user.click(confirm);
    expect(mocks.deleteProject).toHaveBeenCalledOnce();
    await act(async () => pending.resolve());
    await waitFor(() => expect(mocks.toastSuccess).toHaveBeenCalledWith("Project deleted"));
    expect(mocks.push).toHaveBeenCalledWith("/test-workspace/projects");
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(mocks.getProject).not.toHaveBeenCalled();
    expect(screen.queryByText("Project not found")).not.toBeInTheDocument();
  });

  it.each([
    ["403", new Error("Forbidden")],
    ["network error", new TypeError("Failed to fetch")],
  ])("reports %s, retains the detail and allows retry", async (_label, error) => {
    renderProjectDetail();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Delete project" }));
    await user.click(within(screen.getByRole("alertdialog")).getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(mocks.deleteProject).toHaveBeenCalledOnce());
    await act(async () => pending.reject(error));
    await waitFor(() => expect(mocks.toastError).toHaveBeenCalledWith(error.message));
    const confirm = within(screen.getByRole("alertdialog")).getByRole("button", { name: "Delete" });
    expect(confirm).toBeEnabled();
    expect(qc.getQueryData(projectKeys.detail("workspace-1", PROJECT.id))).toEqual(PROJECT);
    expect(mocks.push).not.toHaveBeenCalled();
    expect(mocks.getProject).not.toHaveBeenCalled();
    mocks.deleteProject.mockResolvedValue(undefined);
    await user.click(confirm);
    await waitFor(() => expect(mocks.push).toHaveBeenCalledWith("/test-workspace/projects"));
    expect(mocks.deleteProject).toHaveBeenCalledTimes(2);
  });

  it("offers deletion to the project creator", () => {
    mocks.role = "member";
    PROJECT.creator_type = "member";
    PROJECT.creator_id = "user-1";
    try {
      renderProjectDetail();
      expect(screen.getByRole("button", { name: "Delete project" })).toBeInTheDocument();
    } finally {
      delete PROJECT.creator_type;
      delete PROJECT.creator_id;
    }
  });

  it("does not offer project deletion to regular members with an unknown creator", () => {
    mocks.role = "member";

    renderProjectDetail();

    expect(
      screen.queryByRole("button", { name: "Delete project" }),
    ).not.toBeInTheDocument();
  });
});

describe("ProjectDetail access", () => {
  it("renders both access options and switches between workspace and restricted", async () => {
    const user = userEvent.setup();
    qc.setQueryData(projectKeys.detail("workspace-1", PROJECT.id), {
      ...PROJECT,
      access: "workspace",
      can_manage_access: true,
    });
    renderProjectDetail();

    expect(screen.queryByRole("img", { name: "Restricted project" })).toBeNull();

    const accessSection = screen.getByRole("region", { name: "Access" });
    const workspaceOption = within(accessSection).getByRole("radio", {
      name: /Everyone in the workspace/i,
    });
    const restrictedOption = within(accessSection).getByRole("radio", {
      name: /Only people added/i,
    });
    expect(workspaceOption).toHaveAttribute("aria-checked", "true");
    expect(restrictedOption).toHaveAttribute("aria-checked", "false");

    await user.click(restrictedOption);
    await waitFor(() =>
      expect(mocks.setProjectAccess).toHaveBeenCalledWith(PROJECT.id, {
        access: "restricted",
        member_ids: [],
      }),
    );
  });

  it("shows lock icon, lists granted members, and sends add/remove grant calls", async () => {
    const user = userEvent.setup();
    qc.setQueryData(projectKeys.detail("workspace-1", PROJECT.id), {
      ...PROJECT,
      access: "restricted",
      can_manage_access: true,
    });
    mocks.listProjectMembers.mockResolvedValue({
      members: [
        {
          user_id: "user-1",
          name: "User One",
          added_by: null,
          created_at: "2026-06-01T00:00:00Z",
        },
      ],
    });
    renderProjectDetail();

    expect(screen.getByRole("img", { name: "Restricted project" })).toBeInTheDocument();

    const accessSection = screen.getByRole("region", { name: "Access" });
    expect(await within(accessSection).findByText("User One")).toBeInTheDocument();

    await user.click(within(accessSection).getByRole("button", { name: "Alice Member" }));
    await waitFor(() =>
      expect(mocks.addProjectMembers).toHaveBeenCalledWith(PROJECT.id, ["user-2"]),
    );

    await user.click(
      within(accessSection).getByRole("button", { name: "Remove User One from project" }),
    );
    await waitFor(() =>
      expect(mocks.removeProjectMember).toHaveBeenCalledWith(PROJECT.id, "user-1"),
    );
  });

  it("keeps access controls read-only when can_manage_access is false", async () => {
    qc.setQueryData(projectKeys.detail("workspace-1", PROJECT.id), {
      ...PROJECT,
      access: "restricted",
      can_manage_access: false,
    });
    mocks.listProjectMembers.mockResolvedValue({
      members: [
        {
          user_id: "user-1",
          name: "User One",
          added_by: null,
          created_at: "2026-06-01T00:00:00Z",
        },
      ],
    });
    renderProjectDetail();

    const accessSection = screen.getByRole("region", { name: "Access" });
    expect(await within(accessSection).findByText("User One")).toBeInTheDocument();
    expect(
      within(accessSection).getByRole("radio", { name: /Everyone in the workspace/i }),
    ).toBeDisabled();
    expect(
      within(accessSection).getByRole("radio", { name: /Only people added/i }),
    ).toBeDisabled();
    expect(within(accessSection).queryByRole("button", { name: /Add member/i })).toBeNull();
    expect(
      within(accessSection).queryByRole("button", { name: /Remove User One from project/i }),
    ).toBeNull();
  });
});
