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
  sync: vi.fn(),
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
    useSyncHubSource: () => ({ mutateAsync: mocks.sync, isPending: false }),
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

  it("shows a custom interval as its real duration", () => {
    mocks.sources = [source({ auto_sync_interval_sec: 600 })];
    renderManager(true);
    const row = screen.getByRole("row", { name: /Team skills/ });
    expect(within(row).getByText("Auto-sync: Every 10 minutes")).toBeInTheDocument();
  });

  it("says a disabled source's schedule is paused, not running", () => {
    mocks.sources = [source({ enabled: false, auto_sync_interval_sec: 3600 })];
    renderManager(true);
    const row = screen.getByRole("row", { name: /Team skills/ });
    expect(
      within(row).getByText("Auto-sync: Every hour (paused while the source is disabled)"),
    ).toBeInTheDocument();
  });

  it("shows auto-sync as off when the interval is zero, whatever the enabled state", () => {
    renderManager(true);
    const row = screen.getByRole("row", { name: /Team skills/ });
    expect(within(row).getByText("Auto-sync: Off")).toBeInTheDocument();
  });

  it("shows the edit dialog's current interval as its label", async () => {
    mocks.sources = [source({ auto_sync_interval_sec: 21600 })];
    renderManager(true);
    await userEvent.click(screen.getByRole("button", { name: "Edit Team skills" }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByRole("combobox", { name: "Auto-sync" })).toHaveTextContent("Every 6 hours");
  });

  it("sets an auto-sync schedule while adding a source", async () => {
    renderManager(true);
    await userEvent.click(screen.getByRole("button", { name: "Add source" }));
    const dialog = await screen.findByRole("dialog");
    await userEvent.type(within(dialog).getByLabelText("Name"), "Partner skills");
    await userEvent.type(within(dialog).getByLabelText("URL"), "https://example.com/partner.zip");
    await userEvent.click(within(dialog).getByRole("combobox", { name: "Auto-sync" }));
    await userEvent.click(await screen.findByRole("option", { name: "Every hour" }));
    await userEvent.click(within(dialog).getByRole("button", { name: "Add" }));

    await waitFor(() =>
      expect(mocks.create).toHaveBeenCalledWith({
        wsId: "ws-1",
        name: "Partner skills",
        type: "github",
        url: "https://example.com/partner.zip",
        auto_sync_interval_sec: 3600,
      }),
    );
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
    expect(screen.queryByRole("button", { name: "Sync Team skills now" })).not.toBeInTheDocument();
    expect(screen.queryByRole("switch")).not.toBeInTheDocument();
    expect(within(screen.getByRole("row", { name: /Team skills/ })).getByText("Enabled")).toBeInTheDocument();
  });
});

describe("HubSourcesManager sync", () => {
  it("runs a manual sync and reports the counts in a toast", async () => {
    mocks.sync.mockResolvedValueOnce({
      status: "ok",
      checked: 4,
      updates_available: 1,
      unchanged: 3,
      items: [
        { kind: "skill", id: "s1", name: "deploy", change: "update_available" },
        { kind: "skill", id: "s2", name: "review", change: "error", error: "gone" },
      ],
    });
    renderManager(true);
    await userEvent.click(screen.getByRole("button", { name: "Sync Team skills now" }));
    await waitFor(() => expect(mocks.sync).toHaveBeenCalledWith({ wsId: "ws-1", id: "src-1" }));
    expect(mocks.toastSuccess).toHaveBeenCalledWith("Checked 4. Updates: 1. Unchanged: 3. Errors: 1.");
  });

  it("shows the sync error in a toast when the sync fails", async () => {
    mocks.sync.mockResolvedValueOnce({
      status: "error",
      checked: 0,
      updates_available: 0,
      unchanged: 0,
      error: "GitHub rate limit reached",
      items: [],
    });
    renderManager(true);
    await userEvent.click(screen.getByRole("button", { name: "Sync Team skills now" }));
    await waitFor(() => expect(mocks.toastError).toHaveBeenCalledWith("GitHub rate limit reached"));
    expect(mocks.toastSuccess).not.toHaveBeenCalled();
  });

  it("reports a thrown sync error", async () => {
    mocks.sync.mockRejectedValueOnce(new Error("Network down"));
    renderManager(true);
    await userEvent.click(screen.getByRole("button", { name: "Sync Team skills now" }));
    await waitFor(() => expect(mocks.toastError).toHaveBeenCalledWith("Network down"));
  });

  it("disables sync for a source that is switched off", () => {
    mocks.sources = [source({ enabled: false })];
    renderManager(true);
    expect(screen.getByRole("button", { name: "Sync Team skills now" })).toBeDisabled();
  });

  it("shows the auto-sync schedule beside the last sync", () => {
    mocks.sources = [source({ auto_sync_interval_sec: 3600 })];
    renderManager(true);
    expect(screen.getByText("Auto-sync: Every hour")).toBeInTheDocument();
  });
});
