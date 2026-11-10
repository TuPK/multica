// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderWithI18n } from "../test/i18n";

const mocks = vi.hoisted(() => ({
  check: undefined as undefined | Record<string, unknown>,
  diffs: {} as Record<string, Record<string, unknown>>,
  apply: vi.fn(),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
}));

vi.mock("sonner", () => ({
  toast: { success: mocks.toastSuccess, error: mocks.toastError },
}));

vi.mock("@multica/core/hub", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@multica/core/hub")>();
  class HubLocalChangesError extends Error {
    constructor(message = "local changes") {
      super(message);
      this.name = "HubLocalChangesError";
    }
  }
  class HubQuotaExceededError extends Error {
    constructor(message = "quota") {
      super(message);
      this.name = "HubQuotaExceededError";
    }
  }
  return {
    ...actual,
    HubLocalChangesError,
    HubQuotaExceededError,
    hubItemMetaListOptions: () => ({ queryKey: ["hub-item-meta"] }),
    hubUpdateCheckOptions: () => ({
      queryKey: ["hub-update-check"],
      queryFn: async () => mocks.check,
    }),
    hubUpdateDiffOptions: (_wsId: string, _kind: string, _refId: string, path: string) => ({
      queryKey: ["hub-update-diff", path],
      queryFn: async () => mocks.diffs[path],
    }),
    useApplyHubUpdate: () => ({ mutateAsync: mocks.apply, isPending: false }),
  };
});

import { HubLocalChangesError, HubQuotaExceededError } from "@multica/core/hub";
import { HubUpdateDialog } from "./hub-update-dialog";

const files = [
  { path: "SKILL.md", status: "modified", old_size: 1200, new_size: 1500 },
  { path: "notes.md", status: "added", old_size: null, new_size: 40 },
  { path: "logo.png", status: "modified", old_size: 900, new_size: 950 },
  { path: "README.md", status: "unchanged", old_size: 80, new_size: 80 },
];

function check(overrides: Record<string, unknown> = {}) {
  return {
    update_available: true,
    current_digest: "aaaaaaaaaaaa1111",
    remote_digest: "bbbbbbbbbbbb2222",
    local_modified: false,
    files,
    ...overrides,
  };
}

function renderDialog(open = true) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const onClose = vi.fn();
  renderWithI18n(
    <QueryClientProvider client={client}>
      <HubUpdateDialog wsId="ws-1" kind="skill" refId="sk1" name="Team kit" open={open} onClose={onClose} />
    </QueryClientProvider>,
  );
  return { onClose };
}

beforeEach(() => {
  vi.clearAllMocks();
  mocks.check = check();
  mocks.diffs = {
    "SKILL.md": { path: "SKILL.md", old: "intro\nold line\noutro", new: "intro\nnew line\noutro", binary: false },
    "notes.md": { path: "notes.md", old: null, new: "hello", binary: false },
    "logo.png": { path: "logo.png", old: null, new: null, binary: true },
  };
  mocks.apply.mockResolvedValue({ kind: "skill", id: "sk1", version: "2.0.0", tracking: "tracking" });
});

describe("HubUpdateDialog change list", () => {
  it("lists only changed files with their change kind and sizes", async () => {
    renderDialog();
    const list = await screen.findByRole("list", { name: "Changed files" });
    const items = within(list).getAllByRole("button");
    expect(items).toHaveLength(3);
    expect(within(list).queryByText("README.md")).not.toBeInTheDocument();
    expect(within(list).getByText("Added")).toBeInTheDocument();
    expect(within(list).getByText("1.2kB → 1.5kB")).toBeInTheDocument();
    expect(within(list).getByText("— → 40B")).toBeInTheDocument();
    expect(screen.getByText("Added: 1 · Modified: 2 · Removed: 0")).toBeInTheDocument();
  });

  it("shows the first changed file's diff with added and removed lines labelled in text", async () => {
    renderDialog();
    const region = await screen.findByRole("region", { name: "Changes to SKILL.md" });
    expect(within(region).getByText("old line")).toBeInTheDocument();
    expect(within(region).getByText("new line")).toBeInTheDocument();
    expect(within(region).getByText("Removed:")).toBeInTheDocument();
    expect(within(region).getByText("Added:")).toBeInTheDocument();
  });

  it("loads another file's diff when it is selected", async () => {
    renderDialog();
    await screen.findByRole("region", { name: "Changes to SKILL.md" });
    await userEvent.click(screen.getByRole("button", { name: /notes\.md/ }));
    const region = await screen.findByRole("region", { name: "Changes to notes.md" });
    expect(within(region).getByText("hello")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /notes\.md/ })).toHaveAttribute("aria-pressed", "true");
  });

  it("explains a binary file instead of showing text", async () => {
    renderDialog();
    await screen.findByRole("region", { name: "Changes to SKILL.md" });
    await userEvent.click(screen.getByRole("button", { name: /logo\.png/ }));
    expect(await screen.findByText("Binary file. Compare it outside the Hub.")).toBeInTheDocument();
    expect(screen.queryByRole("region")).not.toBeInTheDocument();
  });
});

describe("HubUpdateDialog without local edits", () => {
  it("applies with overwrite, refreshes, and closes", async () => {
    const { onClose } = renderDialog();
    const update = await screen.findByRole("button", { name: "Update" });
    expect(update).toBeEnabled();
    expect(screen.queryByRole("radio")).not.toBeInTheDocument();

    await userEvent.click(update);
    await waitFor(() =>
      expect(mocks.apply).toHaveBeenCalledWith({
        wsId: "ws-1",
        kind: "skill",
        refId: "sk1",
        onLocalChanges: "overwrite",
      }),
    );
    expect(mocks.toastSuccess).toHaveBeenCalledWith("Updated Team kit");
    expect(onClose).toHaveBeenCalled();
  });

  it("says the item is up to date when the check finds no update", async () => {
    mocks.check = check({ update_available: false, files: [] });
    renderDialog();
    expect(await screen.findByText("Up to date")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Update" })).not.toBeInTheDocument();
  });
});

describe("HubUpdateDialog with local edits", () => {
  beforeEach(() => {
    mocks.check = check({ local_modified: true });
  });

  it("requires a choice before the update can be applied", async () => {
    renderDialog();
    const update = await screen.findByRole("button", { name: "Update" });
    expect(update).toBeDisabled();
    expect(screen.getByText("Choose an option to continue.")).toBeInTheDocument();

    const group = screen.getByRole("group", { name: "Local edits" });
    expect(within(group).getByRole("radio", { name: "Overwrite my edits" })).not.toBeChecked();
    expect(within(group).getByRole("radio", { name: "Fork (keep my edits, stop tracking)" })).not.toBeChecked();
  });

  it("applies with fork when that choice is made", async () => {
    renderDialog();
    await screen.findByRole("button", { name: "Update" });
    await userEvent.click(screen.getByRole("radio", { name: "Fork (keep my edits, stop tracking)" }));
    const update = screen.getByRole("button", { name: "Update" });
    expect(update).toBeEnabled();
    await userEvent.click(update);
    await waitFor(() => expect(mocks.apply).toHaveBeenCalledWith(expect.objectContaining({ onLocalChanges: "fork" })));
  });

  it("applies with overwrite when that choice is made", async () => {
    renderDialog();
    await screen.findByRole("button", { name: "Update" });
    await userEvent.click(screen.getByRole("radio", { name: "Overwrite my edits" }));
    await userEvent.click(screen.getByRole("button", { name: "Update" }));
    await waitFor(() => expect(mocks.apply).toHaveBeenCalledWith(expect.objectContaining({ onLocalChanges: "overwrite" })));
  });

  it("keeps the dialog open with a message when the server refuses for local changes", async () => {
    mocks.apply.mockRejectedValueOnce(new HubLocalChangesError());
    const { onClose } = renderDialog();
    await screen.findByRole("button", { name: "Update" });
    await userEvent.click(screen.getByRole("radio", { name: "Overwrite my edits" }));
    await userEvent.click(screen.getByRole("button", { name: "Update" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Choose an option to continue.");
    expect(onClose).not.toHaveBeenCalled();
  });

  it("shows the quota message when the workspace is at its limit", async () => {
    mocks.apply.mockRejectedValueOnce(new HubQuotaExceededError());
    const { onClose } = renderDialog();
    await screen.findByRole("button", { name: "Update" });
    await userEvent.click(screen.getByRole("radio", { name: "Fork (keep my edits, stop tracking)" }));
    await userEvent.click(screen.getByRole("button", { name: "Update" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("This workspace has reached its Hub item limit.");
    expect(onClose).not.toHaveBeenCalled();
  });
});
