// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { HubSource } from "@multica/core/hub";
import { renderWithI18n } from "../test/i18n";

const mocks = vi.hoisted(() => ({
  sources: [] as HubSource[],
  queryError: false,
  create: vi.fn(),
  update: vi.fn(),
  remove: vi.fn(),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
}));

vi.mock("sonner", () => ({
  toast: { success: mocks.toastSuccess, error: mocks.toastError },
}));
vi.mock("@tanstack/react-query", () => ({
  useQuery: () => ({
    data: mocks.sources,
    isLoading: false,
    isError: mocks.queryError,
    refetch: vi.fn(),
  }),
}));
vi.mock("@multica/core/hub", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@multica/core/hub")>();
  return {
    ...actual,
    hubSourcesOptions: () => ({ queryKey: ["hub-sources"] }),
    useCreateHubSource: () => ({ mutateAsync: mocks.create, isPending: false }),
    useUpdateHubSource: () => ({ mutateAsync: mocks.update, isPending: false }),
    useDeleteHubSource: () => ({ mutateAsync: mocks.remove, isPending: false }),
  };
});

import { HubSourceInUseError } from "@multica/core/hub";
import { HubSourcesManager } from "./hub-sources-manager";

function source(overrides: Partial<HubSource> = {}): HubSource {
  return {
    id: "src-1",
    name: "Team skills",
    type: "github",
    url: "https://github.com/acme/skills",
    config: {},
    enabled: true,
    auto_sync_interval_sec: 0,
    last_sync_at: "2026-08-01T10:00:00Z",
    last_sync_status: "ok",
    last_sync_error: null,
    item_count: 4,
    created_at: "2026-07-01T00:00:00Z",
    updated_at: "2026-08-01T10:00:00Z",
    ...overrides,
  };
}

function renderManager(canManage: boolean) {
  renderWithI18n(<HubSourcesManager wsId="ws-1" canManage={canManage} />);
}

beforeEach(() => {
  vi.clearAllMocks();
  mocks.queryError = false;
  mocks.sources = [source()];
  mocks.create.mockResolvedValue(source({ id: "src-2" }));
  mocks.update.mockResolvedValue(source());
  mocks.remove.mockResolvedValue({});
});

describe("HubSourcesManager for admins", () => {
  it("lists real sources with type, safe link, item count and sync status", () => {
    renderManager(true);
    const row = screen.getByRole("row", { name: /Team skills/ });
    expect(within(row).getByText("GitHub")).toBeInTheDocument();
    expect(within(row).getByRole("link", { name: /github\.com\/acme\/skills/ })).toHaveAttribute(
      "href",
      "https://github.com/acme/skills",
    );
    expect(within(row).getByText("4")).toBeInTheDocument();
    expect(within(row).getByText("OK")).toBeInTheDocument();
    expect(screen.getByRole("switch", { name: "Enable Team skills" })).toBeChecked();
  });

  it("shows the last sync error on a failed source", () => {
    mocks.sources = [source({ last_sync_status: "error", last_sync_error: "401 from GitHub" })];
    renderManager(true);
    expect(screen.getByText("401 from GitHub")).toBeInTheDocument();
    expect(screen.getByText("Error")).toBeInTheDocument();
  });

  it("never links a stored URL that is not a web address", () => {
    mocks.sources = [source({ url: "javascript:alert(1)" })];
    renderManager(true);
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
    expect(screen.getByText("javascript:alert(1)")).toBeInTheDocument();
  });

  it("adds a source and closes only after the server accepts it", async () => {
    renderManager(true);
    await userEvent.click(screen.getByRole("button", { name: "Add source" }));
    const dialog = await screen.findByRole("dialog");
    await userEvent.type(within(dialog).getByLabelText("Name"), "Partner skills");
    await userEvent.type(within(dialog).getByLabelText("URL"), "https://example.com/partner.zip");
    await userEvent.click(within(dialog).getByRole("button", { name: "Add" }));

    await waitFor(() =>
      expect(mocks.create).toHaveBeenCalledWith({
        wsId: "ws-1",
        name: "Partner skills",
        type: "github",
        url: "https://example.com/partner.zip",
      }),
    );
    expect(mocks.toastSuccess).toHaveBeenCalledWith("Source added");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("refuses an add with a bad URL and keeps the dialog open", async () => {
    renderManager(true);
    await userEvent.click(screen.getByRole("button", { name: "Add source" }));
    const dialog = await screen.findByRole("dialog");
    await userEvent.type(within(dialog).getByLabelText("Name"), "Partner skills");
    await userEvent.type(within(dialog).getByLabelText("URL"), "ftp://example.com");
    await userEvent.click(within(dialog).getByRole("button", { name: "Add" }));
    expect(within(dialog).getByText("Enter a valid http:// or https:// URL.")).toBeInTheDocument();
    expect(mocks.create).not.toHaveBeenCalled();
  });

  it("keeps the add dialog open with the server error when the save fails", async () => {
    mocks.create.mockRejectedValueOnce(new Error("A source with this URL already exists"));
    renderManager(true);
    await userEvent.click(screen.getByRole("button", { name: "Add source" }));
    const dialog = await screen.findByRole("dialog");
    await userEvent.type(within(dialog).getByLabelText("Name"), "Dupe");
    await userEvent.type(within(dialog).getByLabelText("URL"), "https://github.com/acme/skills");
    await userEvent.click(within(dialog).getByRole("button", { name: "Add" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("already exists");
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("edits the name and URL but not the type", async () => {
    renderManager(true);
    await userEvent.click(screen.getByRole("button", { name: "Edit Team skills" }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByRole("heading", { name: "Edit source" })).toBeInTheDocument();
    const name = within(dialog).getByLabelText("Name");
    expect(name).toHaveValue("Team skills");
    await userEvent.clear(name);
    await userEvent.type(name, "Core skills");
    await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(mocks.update).toHaveBeenCalledWith({
        wsId: "ws-1",
        id: "src-1",
        name: "Core skills",
        url: "https://github.com/acme/skills",
      }),
    );
  });

  it("toggles auto-sync enabled from the row", async () => {
    renderManager(true);
    await userEvent.click(screen.getByRole("switch", { name: "Enable Team skills" }));
    await waitFor(() => expect(mocks.update).toHaveBeenCalledWith({ wsId: "ws-1", id: "src-1", enabled: false }));
  });

  it("removes a source by detaching its items by default", async () => {
    renderManager(true);
    await userEvent.click(screen.getByRole("button", { name: "Remove Team skills" }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByRole("heading", { name: "Remove Team skills?" })).toBeInTheDocument();
    expect(within(dialog).getByText("4 items come from this source.")).toBeInTheDocument();
    await userEvent.click(within(dialog).getByRole("button", { name: "Remove" }));
    await waitFor(() => expect(mocks.remove).toHaveBeenCalledWith({ wsId: "ws-1", id: "src-1", items: "keep" }));
  });

  it("deletes items only when the user picks that option", async () => {
    renderManager(true);
    await userEvent.click(screen.getByRole("button", { name: "Remove Team skills" }));
    const dialog = await screen.findByRole("dialog");
    await userEvent.click(within(dialog).getByRole("radio", { name: /^Delete items/ }));
    await userEvent.click(within(dialog).getByRole("button", { name: "Remove" }));
    await waitFor(() => expect(mocks.remove).toHaveBeenCalledWith({ wsId: "ws-1", id: "src-1", items: "delete" }));
  });

  it("lists the agents blocking removal and keeps the dialog open", async () => {
    mocks.remove.mockRejectedValueOnce(
      new HubSourceInUseError([
        { kind: "agent", id: "a1", name: "Release writer" },
        { kind: "agent", id: "a2", name: "Triage bot" },
      ]),
    );
    renderManager(true);
    await userEvent.click(screen.getByRole("button", { name: "Remove Team skills" }));
    const dialog = await screen.findByRole("dialog");
    await userEvent.click(within(dialog).getByRole("radio", { name: /^Delete items/ }));
    await userEvent.click(within(dialog).getByRole("button", { name: "Remove" }));

    const alert = await within(dialog).findByRole("alert");
    expect(alert).toHaveTextContent("Agents use these items, so the source can't be removed.");
    expect(within(alert).getByText("Release writer")).toBeInTheDocument();
    expect(within(alert).getByText("Triage bot")).toBeInTheDocument();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("shows an empty state with the add action when there are no sources", () => {
    mocks.sources = [];
    renderManager(true);
    expect(screen.getByText("No sources yet")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Add source" })).toBeInTheDocument();
  });

  it("shows a retry state when the sources fail to load", () => {
    mocks.queryError = true;
    renderManager(true);
    expect(screen.getByText("Couldn't load sources.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
  });
});

describe("HubSourcesManager for members", () => {
  it("hides every change action and says why", () => {
    renderManager(false);
    expect(screen.getByText("Only workspace admins and owners can manage sources.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Add source" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Edit Team skills" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Remove Team skills" })).not.toBeInTheDocument();
    expect(screen.queryByRole("switch")).not.toBeInTheDocument();
    expect(within(screen.getByRole("row", { name: /Team skills/ })).getByText("Enabled")).toBeInTheDocument();
  });
});
