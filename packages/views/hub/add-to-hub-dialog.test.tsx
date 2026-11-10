// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { HubImportPreview } from "@multica/core/hub";
import { renderWithI18n } from "../test/i18n";
import { NavigationProvider, type NavigationAdapter } from "../navigation";

const mocks = vi.hoisted(() => ({
  preview: vi.fn(),
  commit: vi.fn(),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
  push: vi.fn(),
}));

vi.mock("sonner", () => ({
  toast: { success: mocks.toastSuccess, error: mocks.toastError },
}));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("@multica/core/paths", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/paths")>()),
  useWorkspacePaths: () => ({
    hubItem: (kind: string, id: string) => `/acme/hub/${kind}/${id}`,
  }),
}));
vi.mock("@multica/core/hub", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@multica/core/hub")>();
  return {
    ...actual,
    usePreviewHubImport: () => ({ mutateAsync: mocks.preview, isPending: false }),
    useCommitHubImport: () => ({ mutateAsync: mocks.commit, isPending: false }),
  };
});

import { HubPreviewExpiredError } from "@multica/core/hub";
import { AddToHubDialog } from "./add-to-hub-dialog";

function preview(overrides: Partial<HubImportPreview> = {}): HubImportPreview {
  return {
    preview_id: "p1",
    kind: "skill",
    name: "deploy",
    description: "Ship the release",
    version: "1.0.0",
    files: [
      { path: "SKILL.md", size_bytes: 120 },
      { path: "scripts/run.sh", size_bytes: 42 },
    ],
    scan: { status: "clean", findings: [] },
    conflict: { exists: false },
    ...overrides,
  };
}

const committed = { kind: "skill" as const, id: "s-1", hub_item_id: "h-1", skipped: false };

function fileAt(path: string, body = "x") {
  const file = new File([body], path.split("/").pop() ?? path);
  Object.defineProperty(file, "webkitRelativePath", { value: path });
  return file;
}

function renderWizard(onBlank = vi.fn()) {
  const adapter: NavigationAdapter = {
    push: mocks.push,
    replace: vi.fn(),
    back: vi.fn(),
    pathname: "/acme/hub",
    searchParams: new URLSearchParams(),
    hash: "",
    getShareableUrl: (p) => p,
  };
  renderWithI18n(
    <NavigationProvider value={adapter}>
      <AddToHubDialog onBlank={onBlank} />
    </NavigationProvider>,
  );
  return { onBlank };
}

async function openWizard() {
  await userEvent.click(screen.getByRole("button", { name: "Add" }));
  return screen.findByRole("dialog");
}

async function previewGithub(url = "https://github.com/acme/skills") {
  await userEvent.type(screen.getByLabelText("URL"), url);
  await userEvent.click(screen.getByRole("button", { name: "Preview" }));
  return screen.findByRole("heading", { name: "deploy" });
}

beforeEach(() => {
  vi.clearAllMocks();
  mocks.preview.mockResolvedValue(preview());
  mocks.commit.mockResolvedValue(committed);
});

describe("AddToHubDialog", () => {
  it("opens on the From URL tab with the four import paths", async () => {
    renderWizard();
    await openWizard();
    expect(screen.getByRole("heading", { name: "Add to Hub" })).toBeInTheDocument();
    for (const name of ["From URL", "From registry", "Upload", "Blank"]) {
      expect(screen.getByRole("tab", { name })).toBeInTheDocument();
    }
  });

  it("previews a GitHub source, then commits and opens the new item", async () => {
    renderWizard();
    await openWizard();
    await userEvent.type(screen.getByLabelText("Branch or tag"), "v2");
    await userEvent.type(screen.getByLabelText("URL"), "https://github.com/acme/skills");
    await userEvent.click(screen.getByRole("button", { name: "Preview" }));

    await screen.findByRole("heading", { name: "deploy" });
    expect(mocks.preview).toHaveBeenCalledWith({
      wsId: "ws-1",
      source: { type: "github", url: "https://github.com/acme/skills", ref: "v2" },
    });
    expect(screen.getByText("v1.0.0")).toBeInTheDocument();
    expect(screen.getByText("Ship the release")).toBeInTheDocument();
    expect(screen.getByRole("list", { name: "Files in this import" })).toHaveTextContent("run.sh");
    expect(screen.getByText("No issues")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Import" }));
    await waitFor(() =>
      expect(mocks.commit).toHaveBeenCalledWith({
        wsId: "ws-1",
        previewId: "p1",
        onConflict: "rename",
        name: undefined,
        confirmWarnings: false,
      }),
    );
    expect(mocks.toastSuccess).toHaveBeenCalledWith("Imported deploy");
    expect(mocks.push).toHaveBeenCalledWith("/acme/hub/skill/s-1");
  });

  it("rejects a web source that is not an address before previewing", async () => {
    renderWizard();
    await openWizard();
    await userEvent.type(screen.getByLabelText("URL"), "javascript:alert(1)");
    await userEvent.click(screen.getByRole("button", { name: "Preview" }));
    expect(screen.getByRole("alert")).toHaveTextContent("Enter a valid http:// or https:// URL.");
    expect(mocks.preview).not.toHaveBeenCalled();
  });

  it("previews a registry item after listing the registry, and filters the list", async () => {
    mocks.preview
      .mockResolvedValueOnce(
        preview({
          kind: "registry",
          registry: {
            name: "Team",
            items: [
              {
                kind: "skill",
                name: "pulse",
                description: "Monitors things",
                version: "2.0.0",
                url: "items/pulse.zip",
                sha256: "abc",
                tags: ["ops"],
              },
              {
                kind: "plugin",
                name: "lint",
                description: "Checks code",
                version: null,
                url: "https://cdn.example.com/lint.zip",
                sha256: "def",
                tags: ["quality"],
              },
            ],
          },
        }),
      )
      .mockResolvedValueOnce(preview({ name: "pulse", preview_id: "p2" }));

    renderWizard();
    await openWizard();
    await userEvent.click(screen.getByRole("tab", { name: "From registry" }));
    await userEvent.type(screen.getByLabelText("Registry URL"), "https://example.com/hub-index.json");
    await userEvent.click(screen.getByRole("button", { name: "Load" }));

    const list = await screen.findByRole("list");
    expect(list).toHaveTextContent("pulse");
    expect(list).toHaveTextContent("v2.0.0");
    await userEvent.type(screen.getByRole("searchbox"), "quality");
    expect(screen.queryByRole("button", { name: "Preview pulse" })).not.toBeInTheDocument();

    await userEvent.clear(screen.getByRole("searchbox"));
    await userEvent.click(screen.getByRole("button", { name: "Preview pulse" }));
    await screen.findByRole("heading", { name: "pulse" });
    expect(mocks.preview).toHaveBeenLastCalledWith({
      wsId: "ws-1",
      source: {
        type: "url_zip",
        url: "https://example.com/items/pulse.zip",
        sha256: "abc",
      },
    });
  });

  it("previews a dropped zip on the Upload tab", async () => {
    renderWizard();
    await openWizard();
    await userEvent.click(screen.getByRole("tab", { name: "Upload" }));
    const zip = new File(["zip"], "deploy.zip");
    fireEvent.drop(screen.getByText(/Drop a \.zip/).parentElement!, {
      dataTransfer: { files: [zip], items: [] },
    });
    await screen.findByRole("heading", { name: "deploy" });
    expect(mocks.preview).toHaveBeenCalledWith({ wsId: "ws-1", file: zip });
  });

  it("shows the upload error for a folder without SKILL.md and stays on the tab", async () => {
    renderWizard();
    await openWizard();
    await userEvent.click(screen.getByRole("tab", { name: "Upload" }));
    const picker = document.querySelector<HTMLInputElement>('input[type="file"][multiple]')!;
    fireEvent.change(picker, { target: { files: [fileAt("pkg/readme.md", "hi")] } });
    expect(await screen.findByRole("alert")).toHaveTextContent("No SKILL.md found at the top level.");
    expect(mocks.preview).not.toHaveBeenCalled();
  });

  it("disables Import when the scan blocks the import, and explains why", async () => {
    mocks.preview.mockResolvedValueOnce(
      preview({
        scan: {
          status: "blocked",
          findings: [{ rule: "zip-slip", severity: "block", path: "../etc/passwd", message: "Path escapes the archive" }],
        },
      }),
    );
    renderWizard();
    await openWizard();
    await previewGithub();
    const scan = screen.getByRole("heading", { name: "Scan" }).parentElement!;
    expect(within(scan).getByText("Blocked")).toBeInTheDocument();
    expect(screen.getByText("Path escapes the archive")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Import" })).toBeDisabled();
  });

  it("needs an explicit acknowledgement before importing a warned scan", async () => {
    mocks.preview.mockResolvedValueOnce(
      preview({
        scan: {
          status: "warn",
          findings: [{ rule: "curl-pipe-sh", severity: "warn", path: "install.sh", message: "Pipes a download into a shell" }],
        },
      }),
    );
    renderWizard();
    await openWizard();
    await previewGithub();

    const importButton = screen.getByRole("button", { name: "Import" });
    expect(importButton).toBeDisabled();
    await userEvent.click(screen.getByRole("checkbox", { name: "I understand the warnings" }));
    expect(importButton).toBeEnabled();

    await userEvent.click(importButton);
    await waitFor(() => expect(mocks.commit).toHaveBeenCalledWith(expect.objectContaining({ confirmWarnings: true })));
  });

  it("imports under a suggested new name when the name is taken", async () => {
    mocks.preview.mockResolvedValueOnce(
      preview({ conflict: { exists: true, item_id: "old", kind: "skill" } }),
    );
    renderWizard();
    await openWizard();
    await previewGithub();

    expect(screen.getByText("“deploy” already exists in the Hub")).toBeInTheDocument();
    const rename = screen.getByRole("textbox", { name: "New name" });
    expect(rename).toHaveValue("deploy-2");
    await userEvent.clear(rename);
    await userEvent.type(rename, "deploy-prod");
    await userEvent.click(screen.getByRole("button", { name: "Import" }));
    await waitFor(() =>
      expect(mocks.commit).toHaveBeenCalledWith(
        expect.objectContaining({ onConflict: "rename", name: "deploy-prod" }),
      ),
    );
  });

  it("replaces the existing item when that choice is picked", async () => {
    mocks.preview.mockResolvedValueOnce(
      preview({ conflict: { exists: true, item_id: "old", kind: "skill" } }),
    );
    renderWizard();
    await openWizard();
    await previewGithub();

    await userEvent.click(screen.getByRole("radio", { name: "Replace the existing item" }));
    expect(screen.queryByRole("textbox", { name: "New name" })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Import" }));
    await waitFor(() =>
      expect(mocks.commit).toHaveBeenCalledWith(
        expect.objectContaining({ onConflict: "replace", name: undefined }),
      ),
    );
    expect(mocks.toastSuccess).toHaveBeenCalledWith("Replaced deploy");
  });

  it("offers a fresh preview when the old one expired, then imports it", async () => {
    mocks.commit.mockRejectedValueOnce(new HubPreviewExpiredError());
    mocks.preview.mockResolvedValueOnce(preview()).mockResolvedValueOnce(preview({ preview_id: "p-new" }));
    renderWizard();
    await openWizard();
    await previewGithub();

    await userEvent.click(screen.getByRole("button", { name: "Import" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("This preview has expired.");

    await userEvent.click(screen.getByRole("button", { name: "Preview again" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "Import" })).toBeEnabled());
    expect(mocks.preview).toHaveBeenLastCalledWith({
      wsId: "ws-1",
      source: { type: "github", url: "https://github.com/acme/skills" },
    });

    await userEvent.click(screen.getByRole("button", { name: "Import" }));
    await waitFor(() => expect(mocks.commit).toHaveBeenLastCalledWith(expect.objectContaining({ previewId: "p-new" })));
  });

  it("keeps the preview open with the error when the commit fails", async () => {
    mocks.commit.mockRejectedValueOnce(new Error("Storage is full"));
    renderWizard();
    await openWizard();
    await previewGithub();

    await userEvent.click(screen.getByRole("button", { name: "Import" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Storage is full");
    expect(screen.getByRole("heading", { name: "deploy" })).toBeInTheDocument();
    expect(mocks.push).not.toHaveBeenCalled();
  });

  it("returns to the form with its input after Back", async () => {
    renderWizard();
    await openWizard();
    await previewGithub();
    await userEvent.click(screen.getByRole("button", { name: "Back" }));
    expect(screen.getByLabelText("URL")).toHaveValue("https://github.com/acme/skills");
  });

  it("hands the Blank path to the caller and closes itself", async () => {
    const { onBlank } = renderWizard();
    await openWizard();
    await userEvent.click(screen.getByRole("tab", { name: "Blank" }));
    await userEvent.click(screen.getByRole("button", { name: "Create blank skill" }));
    expect(onBlank).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });
});
