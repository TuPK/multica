// @vitest-environment jsdom

import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { setApiInstance } from "@multica/core/api";
import { ApiClient, ApiError } from "@multica/core/api/client";
import type { PackCommitResponse, PackPreview } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { PackImportDialog } from "./pack-import-dialog";

const mocks = vi.hoisted(() => ({ toastError: vi.fn() }));

vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("@multica/core/paths", () => ({
  useWorkspacePaths: () => ({ squads: () => "/acme/squads", squadDetail: (id: string) => `/acme/squads/${id}` }),
}));
vi.mock("../../navigation", () => ({
  useNavigation: () => ({ push: vi.fn() }),
  AppLink: ({ href, children }: { href: string; children: React.ReactNode }) => <a href={href}>{children}</a>,
}));
vi.mock("sonner", () => ({ toast: { error: mocks.toastError, success: vi.fn() } }));

const fakeApi = { previewPack: vi.fn(), previewPackUpload: vi.fn(), commitPack: vi.fn(), listRuntimes: vi.fn() };

const digest = "sha256:" + "c".repeat(64);
const preview = (over: Partial<PackPreview> = {}): PackPreview => ({
  digest,
  format: "claude_agents",
  id: "",
  name: "Writing pack",
  version: "",
  agents: [
    { key: "writer", name: "Writer", description: "", instructions_bytes: 400, skills: ["style"], skills_declared: true, existing_agent_id: null },
    { key: "editor", name: "Editor", description: "", instructions_bytes: 200, skills: [], skills_declared: false, existing_agent_id: null },
  ],
  skills: [
    { name: "style", files: 2, existing_skill_id: null, same_content: false },
    { name: "tone", files: 1, existing_skill_id: "sk-1", same_content: false },
  ],
  leader: null,
  flow: { present: false, validation: null, issues: [] },
  references: [],
  references_omitted: 0,
  ignored: [{ agent: "writer", key: "model" }, { skill: "style", key: "allowed-tools" }],
  scan: [],
  issues: [],
  ...over,
});

const done = (over: Partial<PackCommitResponse> = {}): PackCommitResponse => ({
  install_id: "inst-1",
  already_installed: false,
  squad_id: "sq-1",
  agents: [{ key: "writer", agent_id: "a-1", created: true }],
  skills: [{ name: "style", skill_id: "s-1", created: true }],
  draft_id: null,
  flow_issues: [],
  ...over,
});

function renderDialog(onOpenChange = vi.fn()) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  renderWithI18n(
    <QueryClientProvider client={client}>
      <PackImportDialog open onOpenChange={onOpenChange} />
    </QueryClientProvider>,
  );
  return onOpenChange;
}

async function readPack(url = "https://github.com/acme/writing-pack") {
  fireEvent.change(screen.getByPlaceholderText(/github\.com/), { target: { value: url } });
  fireEvent.click(screen.getByRole("button", { name: /read the pack/i }));
  await screen.findByText("Ignored settings");
}

const install = () => screen.getByRole("button", { name: /^install$/i });

describe("PackImportDialog", () => {
  beforeEach(() => {
    setApiInstance(fakeApi as unknown as ApiClient);
    fakeApi.listRuntimes.mockResolvedValue([{ id: "rt-1", name: "Laptop", custom_name: null, status: "online" }]);
    fakeApi.previewPack.mockResolvedValue(preview());
    fakeApi.commitPack.mockResolvedValue(done());
  });
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("reads a link and shows the agents, the skills and the ignored settings", async () => {
    renderDialog();
    await readPack();

    expect(fakeApi.previewPack).toHaveBeenCalledWith({ kind: "github", url: "https://github.com/acme/writing-pack" });
    expect(screen.getByRole("heading", { name: "Agents" })).toBeInTheDocument();
    expect(screen.getAllByText("Writer").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Editor").length).toBeGreaterThan(0);
    expect(screen.getByRole("heading", { name: "Skills" })).toBeInTheDocument();
    expect(screen.getAllByText("style").length).toBeGreaterThan(0);
    expect(screen.getByText(/different content/i)).toBeInTheDocument();
    const ignored = screen.getByText("Ignored settings").closest("section")!;
    expect(within(ignored).getByText(/model/)).toBeInTheDocument();
    expect(within(ignored).getByText(/allowed-tools/)).toBeInTheDocument();
  });

  it("sends a link to a zip as a zip source and a chosen file as an upload", async () => {
    renderDialog();
    fireEvent.change(screen.getByPlaceholderText(/github\.com/), { target: { value: "https://example.com/pack.zip" } });
    fireEvent.click(screen.getByRole("button", { name: /read the pack/i }));
    await screen.findByText("Ignored settings");
    expect(fakeApi.previewPack).toHaveBeenCalledWith({ kind: "url_zip", url: "https://example.com/pack.zip" });

    cleanup();
    renderDialog();
    fakeApi.previewPackUpload.mockResolvedValue(preview());
    const file = new File(["zip"], "pack.zip", { type: "application/zip" });
    fireEvent.change(document.querySelector('input[type="file"]')!, { target: { files: [file] } });
    fireEvent.click(screen.getByRole("button", { name: /read the pack/i }));
    await screen.findByText("Ignored settings");
    expect(fakeApi.previewPackUpload).toHaveBeenCalledWith(file);
  });

  it("shows Add agent and One agent per skill for a pack that has skills and no agents", async () => {
    fakeApi.previewPack.mockResolvedValue(preview({ format: "skills_only", agents: [] }));
    renderDialog();
    await readPack();

    expect(screen.getByRole("button", { name: "Add agent" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "One agent per skill" }));
    // One agent for each of the two skills, each with a name to edit and a runtime.
    const names = screen.getAllByRole("textbox", { name: /^name$/i });
    expect(names.map((n) => (n as HTMLInputElement).value)).toEqual(["style", "tone"]);
    // The agents need a leader before the squad can be made.
    expect(install()).toBeDisabled();
    expect(screen.getByText("Choose a leader to make the squad")).toBeInTheDocument();
  });

  it("disables the install and says why while no leader is chosen, and enables it once one is", async () => {
    renderDialog();
    await readPack();

    expect(install()).toBeDisabled();
    expect(screen.getByText("Choose a leader to make the squad")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("radio", { name: "Writer" }));
    await waitFor(() => expect(install()).toBeEnabled());
    expect(screen.queryByText("Choose a leader to make the squad")).toBeNull();
  });

  it("starts with the leader the pack names", async () => {
    fakeApi.previewPack.mockResolvedValue(preview({ format: "native_bundle", leader: "editor" }));
    renderDialog();
    await readPack();
    expect(screen.getByRole("radio", { name: "Editor" })).toBeChecked();
    await waitFor(() => expect(install()).toBeEnabled());
  });

  it("disables the install for a scan that blocks, whatever else is chosen", async () => {
    fakeApi.previewPack.mockResolvedValue(preview({ leader: "writer", scan: [{ path: "skills/style/SKILL.md", severity: "block", rule: "secret_key" }] }));
    renderDialog();
    await readPack();

    expect(install()).toBeDisabled();
    expect(screen.getByText("The security scan blocked this pack")).toBeInTheDocument();
    expect(screen.getByText(/skills\/style\/SKILL\.md/)).toBeInTheDocument();
  });

  it("needs a tick for the warnings of the scan and sends it", async () => {
    fakeApi.previewPack.mockResolvedValue(preview({ leader: "writer", scan: [{ path: "a.md", severity: "warn", rule: "shell_pipe" }] }));
    renderDialog();
    await readPack();

    expect(install()).toBeDisabled();
    expect(screen.getByText("Tick that you have read the warnings")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("checkbox", { name: /i have read the warnings/i }));
    await waitFor(() => expect(install()).toBeEnabled());

    fireEvent.click(install());
    await waitFor(() => expect(fakeApi.commitPack).toHaveBeenCalled());
    expect(fakeApi.commitPack.mock.calls[0]![0]).toMatchObject({ digest, ack_scan_warnings: true, leader: { agent: "writer" } });
  });

  it("sends the choices of the person for agents, skills and the squad", async () => {
    renderDialog();
    await readPack();
    fireEvent.click(screen.getByRole("radio", { name: "Writer" }));
    fireEvent.change(screen.getByRole("textbox", { name: /squad name/i }), { target: { value: "Writers" } });
    fireEvent.change(screen.getByRole("combobox", { name: /editor.*action/i }), { target: { value: "skip" } });
    fireEvent.change(screen.getByRole("combobox", { name: /tone/i }), { target: { value: "import_as_new" } });
    await waitFor(() => expect(install()).toBeEnabled());

    fireEvent.click(install());
    await waitFor(() => expect(fakeApi.commitPack).toHaveBeenCalled());
    expect(fakeApi.commitPack.mock.calls[0]![0]).toEqual({
      digest,
      squad: { name: "Writers" },
      agents: [
        { key: "writer", action: "create", runtime_id: "rt-1" },
        { key: "editor", action: "skip" },
      ],
      leader: { agent: "writer" },
      skills: { tone: "import_as_new" },
      ack_scan_warnings: false,
    });
  });

  it("shows the choice when the pack is installed already, and installs it as a new squad when asked", async () => {
    fakeApi.commitPack.mockRejectedValueOnce(
      new ApiError("conflict", 409, "", { code: "pack_conflict", previous: { install_id: "i-0", version: "1.0.0", squad_id: "sq-0", installed_at: "2026-11-01T00:00:00Z" } }),
    );
    renderDialog();
    await readPack();
    fireEvent.click(screen.getByRole("radio", { name: "Writer" }));
    await waitFor(() => expect(install()).toBeEnabled());
    fireEvent.click(install());

    expect(await screen.findByText("This pack is installed already")).toBeInTheDocument();
    expect(screen.getByText(/1\.0\.0/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Keep it" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Install as a new squad" }));
    await waitFor(() => expect(fakeApi.commitPack).toHaveBeenCalledTimes(2));
    expect(fakeApi.commitPack.mock.calls[1]![0]).toMatchObject({ digest, new_install: true });
    expect(await screen.findByRole("link", { name: "Design the workflow" })).toBeInTheDocument();
  });

  it("says how many documentation files the list left out, and nothing when none", async () => {
    fakeApi.previewPack.mockResolvedValueOnce(preview({ references_omitted: 3 }));
    renderDialog();
    await readPack();
    expect(screen.getByText("3 more documentation files are not listed")).toBeInTheDocument();
    cleanup();

    fakeApi.previewPack.mockResolvedValueOnce(preview({ references_omitted: 1 }));
    renderDialog();
    await readPack();
    expect(screen.getByText("1 more documentation file is not listed")).toBeInTheDocument();
    cleanup();

    renderDialog();
    await readPack();
    expect(screen.queryByText(/documentation file/)).toBeNull();
  });

  it("has one control named Close, the X, on every step", async () => {
    fakeApi.commitPack.mockResolvedValueOnce(done());
    renderDialog();
    expect(screen.getAllByRole("button", { name: "Close" })).toHaveLength(1);

    await readPack();
    expect(screen.getAllByRole("button", { name: "Close" })).toHaveLength(1);

    fireEvent.click(screen.getByRole("radio", { name: "Writer" }));
    await waitFor(() => expect(install()).toBeEnabled());
    fireEvent.click(install());
    await screen.findByRole("link", { name: "Open the squad" });
    expect(screen.getAllByRole("button", { name: "Close" })).toHaveLength(1);
  });

  it("closes without installing when the person keeps the pack that is there", async () => {
    fakeApi.commitPack.mockRejectedValueOnce(new ApiError("conflict", 409, "", { code: "pack_conflict", previous: { install_id: "i-0", version: "1.0.0" } }));
    const onOpenChange = renderDialog();
    await readPack();
    fireEvent.click(screen.getByRole("radio", { name: "Writer" }));
    await waitFor(() => expect(install()).toBeEnabled());
    fireEvent.click(install());
    fireEvent.click(await screen.findByRole("button", { name: "Keep it" }));
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(fakeApi.commitPack).toHaveBeenCalledTimes(1);
  });

  it("says which names are taken and stays on the choices", async () => {
    fakeApi.commitPack.mockRejectedValueOnce(new ApiError("taken", 409, "", { code: "pack_name_conflict", agents: ["Writer"], skills: ["tone"] }));
    renderDialog();
    await readPack();
    fireEvent.click(screen.getByRole("radio", { name: "Writer" }));
    await waitFor(() => expect(install()).toBeEnabled());
    fireEvent.click(install());

    expect(await screen.findByText(/these names are taken: Writer, tone/i)).toBeInTheDocument();
    expect(install()).toBeEnabled();
  });

  it("links to the draft of the workflow after the install, and to the squad when there is no draft", async () => {
    fakeApi.commitPack.mockResolvedValueOnce(done({ draft_id: "d-1" }));
    renderDialog();
    await readPack();
    fireEvent.click(screen.getByRole("radio", { name: "Writer" }));
    await waitFor(() => expect(install()).toBeEnabled());
    fireEvent.click(install());

    const link = await screen.findByRole("link", { name: "Design the workflow" });
    expect(link).toHaveAttribute("href", "/acme/squads/sq-1/workflows/d-1");
    expect(screen.getByText("Installed")).toBeInTheDocument();
    expect(screen.getByText("1 agent · 1 skill")).toBeInTheDocument();

    cleanup();
    fakeApi.commitPack.mockResolvedValueOnce(done({ draft_id: null }));
    renderDialog();
    await readPack();
    fireEvent.click(screen.getByRole("radio", { name: "Writer" }));
    await waitFor(() => expect(install()).toBeEnabled());
    fireEvent.click(install());
    expect(await screen.findByRole("link", { name: "Design the workflow" })).toHaveAttribute("href", "/acme/squads/sq-1");
  });

  it("says the pack was installed before when the server changed nothing", async () => {
    fakeApi.commitPack.mockResolvedValueOnce(done({ already_installed: true }));
    renderDialog();
    await readPack();
    fireEvent.click(screen.getByRole("radio", { name: "Writer" }));
    await waitFor(() => expect(install()).toBeEnabled());
    fireEvent.click(install());
    expect(await screen.findByText("This pack was installed before. Nothing changed.")).toBeInTheDocument();
  });

  it("keeps the person at the source with a message when the pack cannot be read", async () => {
    fakeApi.previewPack.mockRejectedValue(new ApiError("bad", 400, "", { code: "pack_invalid" }));
    renderDialog();
    fireEvent.change(screen.getByPlaceholderText(/github\.com/), { target: { value: "https://github.com/acme/x" } });
    fireEvent.click(screen.getByRole("button", { name: /read the pack/i }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Could not read the pack");
  });

  it("does not break on an answer of the server that is not a preview", async () => {
    // The real client, so the answer goes through the schema: a body that is not a preview is an error, not an empty screen.
    vi.stubGlobal("fetch", vi.fn().mockImplementation(async () => new Response(JSON.stringify({ agents: "many", digest: 4 }), { status: 200, headers: { "Content-Type": "application/json" } })));
    setApiInstance(new ApiClient("https://api.example.test"));
    renderDialog();
    fireEvent.change(screen.getByPlaceholderText(/github\.com/), { target: { value: "https://github.com/acme/x" } });
    fireEvent.click(screen.getByRole("button", { name: /read the pack/i }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Could not read the pack");
    expect(screen.queryByText("Ignored settings")).toBeNull();
    vi.unstubAllGlobals();
  });
});
