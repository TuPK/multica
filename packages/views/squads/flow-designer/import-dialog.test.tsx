// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { setApiInstance } from "@multica/core/api";
import { ApiError, type ApiClient } from "@multica/core/api/client";
import type { Squad } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { ImportDialog } from "./import-dialog";

const mocks = vi.hoisted(() => ({ navPush: vi.fn() }));
const fakeApi = { createFlowDraft: vi.fn() };

vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("../../navigation", () => ({ useNavigation: () => ({ push: mocks.navPush }) }));
vi.mock("@multica/core/paths", () => ({
  useWorkspacePaths: () => ({ squads: () => "/acme/squads", squadDetail: (id: string) => `/acme/squads/${id}` }),
}));

const squad = { id: "sq-1", name: "Design Squad" } as unknown as Squad;
// What Export writes and the server reads (spec 7.6): the format, a name and the document.
const manifest = {
  format: "multica.flow-manifest/v1",
  name: "Imported Flow",
  doc: { version: 2, start: "plan", roles: [], steps: [], terminals: [] },
};

function renderDialog(onOpenChange = vi.fn()) {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  renderWithI18n(
    <QueryClientProvider client={client}>
      <ImportDialog open onOpenChange={onOpenChange} squad={squad} />
    </QueryClientProvider>,
  );
  return onOpenChange;
}

const paste = (value: string) => fireEvent.change(screen.getByPlaceholderText(/multica\.flow-manifest\/v1/), { target: { value } });
const submit = () => fireEvent.click(screen.getByRole("button", { name: /^import$/i }));

describe("ImportDialog", () => {
  beforeEach(() => {
    setApiInstance(fakeApi as unknown as ApiClient);
    fakeApi.createFlowDraft.mockResolvedValue({ id: "draft-imported-1" });
  });
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("rejects text that is not JSON without calling the server", () => {
    renderDialog();
    paste("not-json");
    submit();

    expect(screen.getByText("This is not valid JSON")).toBeInTheDocument();
    expect(fakeApi.createFlowDraft).not.toHaveBeenCalled();
  });

  it("rejects a document that is not a manifest, such as the bare document of a flow", () => {
    renderDialog();
    paste(JSON.stringify(manifest.doc));
    submit();

    expect(screen.getByText("This is not a workflow manifest")).toBeInTheDocument();
    expect(fakeApi.createFlowDraft).not.toHaveBeenCalled();
  });

  it("rejects a manifest over 512 KiB before it is sent", () => {
    renderDialog();
    paste(JSON.stringify({ ...manifest, notes: "x".repeat(512 * 1024) }));
    submit();

    expect(screen.getByText("The file is larger than 512 KiB")).toBeInTheDocument();
    expect(fakeApi.createFlowDraft).not.toHaveBeenCalled();
  });

  it("sends the manifest as it is, named by the manifest, and opens the draft", async () => {
    const onOpenChange = renderDialog();
    paste(JSON.stringify(manifest));
    submit();

    await waitFor(() => expect(fakeApi.createFlowDraft).toHaveBeenCalledWith("sq-1", { name: "Imported Flow", from: { kind: "manifest", manifest } }));
    await waitFor(() => expect(mocks.navPush).toHaveBeenCalledWith("/acme/squads/sq-1/workflows/draft-imported-1"));
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("names the draft after the squad when the manifest has no name, and cuts it to 56 characters", async () => {
    renderDialog();
    paste(JSON.stringify({ ...manifest, name: "" }));
    submit();

    await waitFor(() => expect(fakeApi.createFlowDraft).toHaveBeenCalled());
    expect(fakeApi.createFlowDraft.mock.calls[0]![1].name).toBe("Design Squad");
  });

  it("shows what the server refused in the manifest", async () => {
    fakeApi.createFlowDraft.mockRejectedValue(new ApiError("the format \"x\" is not multica.flow-manifest/v1", 400, "", { code: "flow_manifest_invalid" }));
    renderDialog();
    paste(JSON.stringify(manifest));
    submit();

    expect(await screen.findByText(/is not multica\.flow-manifest\/v1/)).toBeInTheDocument();
    expect(mocks.navPush).not.toHaveBeenCalled();
  });
});
