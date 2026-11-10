// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { HubItem, HubItemMeta } from "@multica/core/hub";
import { renderWithI18n } from "../test/i18n";

const mocks = vi.hoisted(() => ({
  check: vi.fn(),
}));

vi.mock("@multica/core/hub", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@multica/core/hub")>();
  return {
    ...actual,
    hubItemMetaListOptions: () => ({ queryKey: ["hub-item-meta"] }),
    hubUpdateCheckOptions: () => ({
      queryKey: ["hub-update-check"],
      queryFn: () => mocks.check(),
    }),
    hubUpdateDiffOptions: () => ({ queryKey: ["hub-update-diff"], queryFn: async () => null }),
    useApplyHubUpdate: () => ({ mutateAsync: vi.fn(), isPending: false }),
    useForkHubItem: () => ({ mutateAsync: vi.fn(), isPending: false }),
  };
});

import { HubItemSourceTab } from "./hub-source-tab";

const item: HubItem = {
  key: "skill:sk1",
  kind: "skill",
  refId: "sk1",
  name: "Team kit",
  description: "",
  source: "github",
  sourceUrl: "https://github.com/acme/kit",
  version: null,
  labels: [],
  updatedAt: "2026-08-01T00:00:00Z",
  createdAt: "2026-07-01T00:00:00Z",
};

function meta(overrides: Partial<HubItemMeta> = {}): HubItemMeta {
  return {
    id: "m1",
    kind: "skill",
    ref_id: "sk1",
    source_id: "src-1",
    source_ref: "acme/kit@main",
    tracking: "tracking",
    latest_version: "1.4.0",
    bundle_digest: "0123456789abcdef0123",
    size_bytes: 2048,
    file_count: 3,
    scan_status: "clean",
    update_available: false,
    remote_digest: "",
    checked_at: "2026-08-02T00:00:00Z",
    local_modified: false,
    created_at: "2026-07-01T00:00:00Z",
    updated_at: "2026-08-02T00:00:00Z",
    ...overrides,
  } as HubItemMeta;
}

function renderTab(props: { meta?: HubItemMeta; canEdit: boolean; item?: HubItem }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  renderWithI18n(
    <QueryClientProvider client={client}>
      <HubItemSourceTab
        wsId="ws-1"
        item={props.item ?? item}
        meta={props.meta}
        metaLoading={false}
        canEdit={props.canEdit}
      />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  mocks.check.mockResolvedValue({ update_available: false, current_digest: "a", remote_digest: "a", local_modified: false, files: [] });
});

describe("HubItemSourceTab provenance", () => {
  it("shows the source link, reference, digest, size and last check", () => {
    renderTab({ meta: meta(), canEdit: true });
    const source = screen.getByRole("link", { name: /GitHub/ });
    expect(source).toHaveAttribute("href", "https://github.com/acme/kit");
    expect(screen.getByText("acme/kit@main")).toBeInTheDocument();
    expect(screen.getByText("0123456789ab")).toBeInTheDocument();
    expect(screen.getByText("2kB")).toBeInTheDocument();
  });

  it("explains a tracked item's link to its source", () => {
    renderTab({ meta: meta(), canEdit: true });
    expect(screen.getByText("Updates from GitHub can be applied after you review them.")).toBeInTheDocument();
  });
});

describe("HubItemSourceTab for an editor", () => {
  it("checks for updates on demand and reports the result", async () => {
    mocks.check.mockResolvedValue({
      update_available: true,
      current_digest: "a",
      remote_digest: "b",
      local_modified: false,
      files: [{ path: "SKILL.md", status: "modified", old_size: 1, new_size: 2 }],
    });
    renderTab({ meta: meta(), canEdit: true });
    expect(mocks.check).not.toHaveBeenCalled();

    await userEvent.click(screen.getByRole("button", { name: "Check for updates" }));
    await waitFor(() => expect(mocks.check).toHaveBeenCalledTimes(1));
    expect(await screen.findByText("Update available")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Review update" })).toBeInTheDocument();
  });

  it("offers fork for tracked items and opens the confirmation", async () => {
    renderTab({ meta: meta(), canEdit: true });
    await userEvent.click(screen.getByRole("button", { name: "Fork" }));
    expect(await screen.findByRole("alertdialog")).toHaveTextContent("It stops receiving updates from GitHub.");
  });

  it("warns that local edits are present", () => {
    renderTab({ meta: meta({ local_modified: true }), canEdit: true });
    expect(
      screen.getByText("This item has edits made here. Applying an update asks what to do with them."),
    ).toBeInTheDocument();
  });
});

describe("HubItemSourceTab for a member who cannot edit", () => {
  it("reads the state without check, update or fork actions", () => {
    renderTab({ meta: meta({ update_available: true }), canEdit: false });
    expect(screen.queryByRole("button", { name: "Check for updates" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Review update" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Fork" })).not.toBeInTheDocument();
    expect(
      screen.getByText("Ask someone who can edit this item to check for or apply updates."),
    ).toBeInTheDocument();
    expect(screen.getByText("Update available")).toBeInTheDocument();
  });
});

describe("HubItemSourceTab for a forked or manual item", () => {
  it("explains a forked copy and offers no update actions", () => {
    renderTab({ meta: meta({ tracking: "forked" }), canEdit: true });
    expect(screen.getByText("This copy no longer receives updates from its source.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Check for updates" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Fork" })).not.toBeInTheDocument();
  });

  it("explains a manual item has no source to check", () => {
    renderTab({ meta: meta({ tracking: "manual", source_id: null }), canEdit: true });
    expect(
      screen.getByText("Not linked to a source, so there are no updates to check."),
    ).toBeInTheDocument();
    expect(screen.getByText("Created here")).toBeInTheDocument();
  });
});
