// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderWithI18n } from "../test/i18n";

type FakeFile = { content: string | null; is_binary: boolean; etag: string; size_bytes: number };

const mocks = vi.hoisted(() => ({
  list: [] as Array<{ path: string; size_bytes: number }>,
  files: {} as Record<string, FakeFile>,
  saveMutate: vi.fn(),
  deleteMutate: vi.fn(),
  renameMutate: vi.fn(),
  publishMutate: vi.fn(),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
}));

vi.mock("@multica/core/hub", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@multica/core/hub")>();
  return {
    ...actual,
    hubFileListOptions: (_wsId: string, kind: string, refId: string, versionId?: string) => ({
      queryKey: ["hub-list", kind, refId, versionId ?? "current"],
      queryFn: async () => mocks.list,
    }),
    hubFileContentOptions: (
      _wsId: string,
      kind: string,
      refId: string,
      path: string,
      versionId?: string,
    ) => ({
      queryKey: ["hub-content", kind, refId, versionId ?? "current", path],
      queryFn: async () => {
        const file = mocks.files[path];
        if (!file) throw new Error(`missing ${path}`);
        return { path, ...file };
      },
    }),
    useSaveHubFile: () => ({ mutateAsync: mocks.saveMutate, isPending: false }),
    useDeleteHubFile: () => ({ mutateAsync: mocks.deleteMutate, isPending: false }),
    useRenameHubFile: () => ({ mutateAsync: mocks.renameMutate, isPending: false }),
    useSavePluginDraft: () => ({ mutateAsync: mocks.publishMutate, isPending: false }),
  };
});

vi.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getTotalSize: () => count * 32,
    getVirtualItems: () =>
      Array.from({ length: count }, (_, index) => ({ index, key: index, start: index * 32 })),
  }),
}));

vi.mock("@multica/ui/hooks/use-mobile", () => ({ useIsMobile: () => false }));

vi.mock("sonner", () => ({
  toast: { success: mocks.toastSuccess, error: mocks.toastError },
}));

vi.mock("../rich-content", () => ({
  RichContent: ({ content }: { content: string }) => <div data-testid="rich-content">{content}</div>,
}));

import { HubFileConflictError } from "@multica/core/hub";
import { HubItemFiles } from "./hub-item-files";
import { useHubFiles } from "./use-hub-files";

function Harness({
  kind,
  canEdit,
  versionId,
  currentVersion = null,
  publishedVersions = [],
}: {
  kind: "skill" | "plugin";
  canEdit: boolean;
  versionId?: string;
  currentVersion?: string | null;
  publishedVersions?: string[];
}) {
  const files = useHubFiles({
    wsId: "ws-1",
    kind,
    refId: "item-1",
    versionId,
    ready: kind === "skill" || versionId !== undefined,
    canEdit,
  });
  return (
    <HubItemFiles
      kind={kind}
      canEdit={canEdit}
      files={files}
      reservedPath={kind === "skill" ? "SKILL.md" : "multica.plugin.json"}
      currentVersion={currentVersion}
      publishedVersions={publishedVersions}
    />
  );
}

function renderFiles(props: Parameters<typeof Harness>[0]) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return renderWithI18n(
    <QueryClientProvider client={client}>
      <Harness {...props} />
    </QueryClientProvider>,
  );
}

const SKILL_MD = "---\nname: Deploy\n---\n# Deploy\nSteps";

function seedSkill() {
  mocks.list = [
    { path: "SKILL.md", size_bytes: 40 },
    { path: "docs/guide.md", size_bytes: 9 },
  ];
  mocks.files = {
    "SKILL.md": { content: SKILL_MD, is_binary: false, etag: "e-skill", size_bytes: 40 },
    "docs/guide.md": { content: "# Guide", is_binary: false, etag: "e-guide", size_bytes: 9 },
  };
}

function seedPlugin() {
  mocks.list = [
    { path: "multica.plugin.json", size_bytes: 30 },
    { path: "index.js", size_bytes: 12 },
    { path: "logo.png", size_bytes: 900 },
  ];
  mocks.files = {
    "multica.plugin.json": { content: '{"key":"x"}', is_binary: false, etag: "h1", size_bytes: 30 },
    "index.js": { content: "export const a = 1;", is_binary: false, etag: "h2", size_bytes: 12 },
    "logo.png": { content: null, is_binary: true, etag: "h3", size_bytes: 900 },
  };
}

function editor(name: string) {
  return screen.findByRole("textbox", { name: `Contents of ${name}` });
}

/**
 * Opens a tree row's menu. The menu opens on mousedown, which user-event's
 * click does not reach in this environment.
 */
function openRowMenu(trigger: HTMLElement) {
  fireEvent.mouseDown(trigger, { button: 0 });
}

/** Sets a controlled input's value through a change event, as typeInto does. */
function setValue(element: HTMLElement, value: string) {
  fireEvent.change(element, { target: { value } });
}

/**
 * Appends text to a controlled textarea. user-event does not reliably drive a
 * controlled textarea here (the change never reaches React), so the edit is a
 * change event on the element.
 */
function typeInto(element: HTMLElement, text: string) {
  fireEvent.change(element, {
    target: { value: `${(element as HTMLTextAreaElement).value}${text}` },
  });
}

beforeEach(() => {
  vi.clearAllMocks();
  mocks.saveMutate.mockResolvedValue({});
  mocks.deleteMutate.mockResolvedValue(undefined);
  mocks.renameMutate.mockResolvedValue({});
  mocks.publishMutate.mockResolvedValue({});
});

describe("skill files: tree and preview", () => {
  it("selects a file from the tree and shows its rendered markdown", async () => {
    seedSkill();
    const user = userEvent.setup();
    renderFiles({ kind: "skill", canEdit: true });

    const guide = await screen.findByRole("treeitem", { name: "guide.md" });
    await user.click(guide);

    expect(guide).toHaveAttribute("aria-selected", "true");
    expect(await screen.findByTestId("rich-content")).toHaveTextContent("# Guide");
  });

  it("gives SKILL.md no rename or delete actions, but gives other files a menu", async () => {
    seedSkill();
    renderFiles({ kind: "skill", canEdit: true });

    await screen.findByRole("treeitem", { name: "guide.md" });
    expect(screen.queryByRole("button", { name: "Actions for SKILL.md" })).toBeNull();
    expect(screen.getByRole("button", { name: "Actions for docs/guide.md" })).toBeInTheDocument();
  });
});

describe("skill files: editing, dirty state and save", () => {
  it("shows the unsaved indicator while typing and saves against the loaded etag", async () => {
    seedSkill();
    const user = userEvent.setup();
    renderFiles({ kind: "skill", canEdit: true });

    await user.click(await screen.findByRole("button", { name: "Edit" }));
    const text = await editor("SKILL.md");
    await waitFor(() => expect(text).toHaveValue(SKILL_MD));

    typeInto(text, "\nMore");
    expect(screen.getByText("Unsaved changes")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(mocks.saveMutate).toHaveBeenCalledWith({
        wsId: "ws-1",
        refId: "item-1",
        path: "SKILL.md",
        content: `${SKILL_MD}\nMore`,
        etag: "e-skill",
      }),
    );
    expect(mocks.toastSuccess).toHaveBeenCalledWith("Saved SKILL.md");
    await waitFor(() => expect(screen.queryByText("Unsaved changes")).toBeNull());
  });

  it("asks before switching files with unsaved text, and discards on request", async () => {
    seedSkill();
    const user = userEvent.setup();
    renderFiles({ kind: "skill", canEdit: true });

    await user.click(await screen.findByRole("button", { name: "Edit" }));
    typeInto(await editor("SKILL.md"), "!");

    await user.click(screen.getByRole("treeitem", { name: "guide.md" }));
    const dialog = await screen.findByRole("dialog", { name: "Save changes to SKILL.md?" });
    await user.click(within(dialog).getByRole("button", { name: "Discard changes" }));

    expect(screen.getByRole("treeitem", { name: "guide.md" })).toHaveAttribute("aria-selected", "true");
    expect(mocks.saveMutate).not.toHaveBeenCalled();
  });

  it("keeps the current file when the guard is dismissed with Keep editing", async () => {
    seedSkill();
    const user = userEvent.setup();
    renderFiles({ kind: "skill", canEdit: true });

    await user.click(await screen.findByRole("button", { name: "Edit" }));
    typeInto(await editor("SKILL.md"), "!");
    await user.click(screen.getByRole("treeitem", { name: "guide.md" }));

    const dialog = await screen.findByRole("dialog", { name: "Save changes to SKILL.md?" });
    await user.click(within(dialog).getByRole("button", { name: "Keep editing" }));

    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: "Save changes to SKILL.md?" })).toBeNull(),
    );
    expect(screen.getByRole("treeitem", { name: /^SKILL\.md/ })).toHaveAttribute("aria-selected", "true");
  });
});

describe("skill files: save conflicts", () => {
  it("offers reload, which discards the local edit", async () => {
    seedSkill();
    mocks.saveMutate.mockRejectedValueOnce(new HubFileConflictError());
    const user = userEvent.setup();
    renderFiles({ kind: "skill", canEdit: true });

    await user.click(await screen.findByRole("button", { name: "Edit" }));
    typeInto(await editor("SKILL.md"), "!");
    await user.click(screen.getByRole("button", { name: "Save" }));

    const dialog = await screen.findByRole("dialog", { name: "SKILL.md changed on the server" });
    await user.click(within(dialog).getByRole("button", { name: "Reload" }));

    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: "SKILL.md changed on the server" })).toBeNull(),
    );
    expect(screen.queryByText("Unsaved changes")).toBeNull();
    expect(mocks.saveMutate).toHaveBeenCalledTimes(1);
  });

  it("offers overwrite, which saves the local text over the newer etag", async () => {
    seedSkill();
    mocks.saveMutate
      .mockRejectedValueOnce(new HubFileConflictError())
      .mockResolvedValueOnce({});
    const user = userEvent.setup();
    renderFiles({ kind: "skill", canEdit: true });

    await user.click(await screen.findByRole("button", { name: "Edit" }));
    typeInto(await editor("SKILL.md"), "!");
    await user.click(screen.getByRole("button", { name: "Save" }));

    // Someone else saved in the meantime: the server now reports a newer etag.
    mocks.files["SKILL.md"] = { ...mocks.files["SKILL.md"]!, etag: "e-skill-2" };
    const dialog = await screen.findByRole("dialog", { name: "SKILL.md changed on the server" });
    await user.click(within(dialog).getByRole("button", { name: "Overwrite" }));

    await waitFor(() =>
      expect(mocks.saveMutate).toHaveBeenLastCalledWith(
        expect.objectContaining({ path: "SKILL.md", etag: "e-skill-2" }),
      ),
    );
    await waitFor(() => expect(mocks.toastSuccess).toHaveBeenCalled());
  });
});

describe("skill files: create and delete", () => {
  it("rejects SKILL.md as a new file name and creates a valid one as a draft", async () => {
    seedSkill();
    const user = userEvent.setup();
    renderFiles({ kind: "skill", canEdit: true });

    await screen.findByRole("treeitem", { name: "guide.md" });
    await user.click(screen.getByRole("button", { name: "New file" }));
    const input = screen.getByRole("textbox", { name: "New file" });

    setValue(input, "SKILL.md");
    await user.click(screen.getByRole("button", { name: "Create" }));
    expect(screen.getByRole("alert")).toHaveTextContent("SKILL.md already exists.");

    setValue(input, "notes.md");
    await user.click(screen.getByRole("button", { name: "Create" }));

    // The dirty marker joins the row's name, so match the path by prefix.
    expect(await screen.findByRole("treeitem", { name: /^notes\.md/ })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByText("Unsaved changes")).toBeInTheDocument();
    expect(mocks.saveMutate).not.toHaveBeenCalled();
  });

  it("confirms a delete before calling the server", async () => {
    seedSkill();
    const user = userEvent.setup();
    renderFiles({ kind: "skill", canEdit: true });

    openRowMenu(await screen.findByRole("button", { name: "Actions for docs/guide.md" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete" }));

    const dialog = await screen.findByRole("alertdialog", { name: "Delete docs/guide.md?" });
    expect(mocks.deleteMutate).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    await waitFor(() =>
      expect(mocks.deleteMutate).toHaveBeenCalledWith({
        wsId: "ws-1",
        refId: "item-1",
        path: "docs/guide.md",
      }),
    );
  });
});

describe("plugin files: immutable versions and draft publish", () => {
  it("says published versions cannot change and collects edits into a draft", async () => {
    seedPlugin();
    const user = userEvent.setup();
    renderFiles({
      kind: "plugin",
      canEdit: true,
      versionId: "ver-2",
      currentVersion: "2.0.0",
      publishedVersions: ["2.0.0", "1.0.0"],
    });

    expect(
      await screen.findByText(
        "Published versions can't change. Edits here form a draft until you publish a new version.",
      ),
    ).toBeInTheDocument();

    await user.click(await screen.findByRole("treeitem", { name: "index.js" }));
    const text = await editor("index.js");
    await waitFor(() => expect(text).toHaveValue("export const a = 1;"));
    typeInto(text, "\nexport const b = 2;");

    expect(screen.getByText("1 unpublished change")).toBeInTheDocument();
    expect(mocks.publishMutate).not.toHaveBeenCalled();
  });

  it("publishes the draft as a typed new version, validating format and duplicates", async () => {
    seedPlugin();
    const user = userEvent.setup();
    renderFiles({
      kind: "plugin",
      canEdit: true,
      versionId: "ver-2",
      currentVersion: "2.0.0",
      publishedVersions: ["2.0.0", "1.0.0"],
    });

    await user.click(await screen.findByRole("treeitem", { name: "index.js" }));
    const text = await editor("index.js");
    await waitFor(() => expect(text).toHaveValue("export const a = 1;"));
    typeInto(text, "!");

    await user.click(screen.getByRole("button", { name: "Publish new version" }));
    const dialog = await screen.findByRole("dialog", { name: "Publish new version" });
    const version = within(dialog).getByRole("textbox", { name: "Version" });

    await user.type(version, "v3");
    await user.click(within(dialog).getByRole("button", { name: "Publish" }));
    expect(within(dialog).getByRole("alert")).toHaveTextContent("Use a version like 1.2.0.");

    await user.clear(version);
    await user.type(version, "2.0.0");
    await user.click(within(dialog).getByRole("button", { name: "Publish" }));
    expect(within(dialog).getByRole("alert")).toHaveTextContent("Version 2.0.0 is already published.");
    expect(mocks.publishMutate).not.toHaveBeenCalled();

    await user.clear(version);
    await user.type(version, "2.1.0");
    await user.click(within(dialog).getByRole("button", { name: "Publish" }));

    await waitFor(() =>
      expect(mocks.publishMutate).toHaveBeenCalledWith({
        wsId: "ws-1",
        refId: "item-1",
        baseVersionId: "ver-2",
        changes: { "index.js": "export const a = 1;!" },
        newVersion: "2.1.0",
      }),
    );
    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: "Publish new version" })).toBeNull(),
    );
    expect(mocks.toastSuccess).toHaveBeenCalledWith("Published version 2.1.0");
  });

  it("tells a viewer who cannot publish why, and offers no publish action", async () => {
    seedPlugin();
    renderFiles({ kind: "plugin", canEdit: false, versionId: "ver-2" });

    expect(
      await screen.findByText("Only workspace admins and owners can edit a plugin."),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Publish new version" })).toBeNull();
    await screen.findByRole("treeitem", { name: "index.js" });
    expect(await editor("index.js")).toHaveAttribute("readonly");
  });

  it("shows a binary file as unavailable rather than as text", async () => {
    seedPlugin();
    const user = userEvent.setup();
    renderFiles({ kind: "plugin", canEdit: true, versionId: "ver-2" });

    await user.click(await screen.findByRole("treeitem", { name: "logo.png" }));
    expect(
      await screen.findByText("Binary file. Preview and editing aren't available here."),
    ).toBeInTheDocument();
  });
});
