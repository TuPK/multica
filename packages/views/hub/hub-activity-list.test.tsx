// @vitest-environment jsdom

import { describe, expect, it, vi } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { HubActivityEntry } from "@multica/core/hub";
import { renderWithI18n } from "../test/i18n";
import { HubActivityList } from "./hub-activity-list";

const mocks = vi.hoisted(() => ({
  pages: [] as Array<{ entries: Array<Record<string, unknown>>; next_cursor: string | null }>,
  calls: [] as Array<{ kind?: string; refId?: string; before: string | null }>,
  failFirst: false,
}));

vi.mock("@multica/core/hub", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@multica/core/hub")>();
  return {
    ...actual,
    hubActivityInfiniteOptions: (_wsId: string, filter: { kind?: string; refId?: string } = {}) => ({
      queryKey: ["hub-activity-test", filter.kind ?? null, filter.refId ?? null],
      queryFn: async ({ pageParam }: { pageParam: string | null }) => {
        mocks.calls.push({ kind: filter.kind, refId: filter.refId, before: pageParam });
        if (mocks.failFirst && pageParam === null) throw new Error("offline");
        const index = pageParam === null ? 0 : Number(pageParam);
        return mocks.pages[index] ?? { entries: [], next_cursor: null };
      },
      initialPageParam: null as string | null,
      getNextPageParam: (last: { next_cursor: string | null }) => last.next_cursor ?? undefined,
      retry: false,
    }),
  };
});

function entry(id: string, overrides: Partial<HubActivityEntry> = {}): Record<string, unknown> {
  return {
    id,
    action: "file_saved",
    actor_type: "member",
    actor_id: "u1",
    actor_name: "Ada",
    kind: "skill",
    ref_id: "s1",
    item_name: "Deploy",
    source_id: null,
    detail: { path: "SKILL.md" },
    created_at: "2026-11-01T00:00:00Z",
    ...overrides,
  };
}

function renderList(props: { showItem?: boolean; filter?: { kind: "skill"; refId: string } } = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return renderWithI18n(
    <QueryClientProvider client={client}>
      <HubActivityList wsId="ws-1" {...props} />
    </QueryClientProvider>,
  );
}

describe("HubActivityList", () => {
  it("shows the empty state when there is no activity", async () => {
    mocks.pages = [{ entries: [], next_cursor: null }];
    mocks.calls = [];
    renderList();
    expect(await screen.findByText("No activity yet.")).toBeInTheDocument();
  });

  it("loads the next page on Load more and appends it", async () => {
    mocks.pages = [
      { entries: [entry("a1")], next_cursor: "1" },
      { entries: [entry("a2", { action: "source_synced", detail: { checked: 2, updates_available: 1 } })], next_cursor: null },
    ];
    mocks.calls = [];
    const user = userEvent.setup();
    renderList();

    const list = await screen.findByRole("list", { name: "Activity" });
    expect(within(list).getAllByRole("listitem")).toHaveLength(1);
    await user.click(screen.getByRole("button", { name: "Load more" }));

    await waitFor(() => expect(within(list).getAllByRole("listitem")).toHaveLength(2));
    expect(within(list).getByText("Source synced")).toBeInTheDocument();
    expect(within(list).getByText("2 checked · 1 with updates")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Load more" })).not.toBeInTheDocument();
    expect(mocks.calls.map((call) => call.before)).toEqual([null, "1"]);
  });

  it("names the item only when the list spans items", async () => {
    mocks.pages = [{ entries: [entry("a1")], next_cursor: null }];
    renderList({ showItem: true });
    const list = await screen.findByRole("list", { name: "Activity" });
    expect(within(list).getByText("Deploy")).toBeInTheDocument();
  });

  it("passes the item filter to the query", async () => {
    mocks.pages = [{ entries: [entry("a1")], next_cursor: null }];
    mocks.calls = [];
    renderList({ filter: { kind: "skill", refId: "s1" } });
    await screen.findByRole("list", { name: "Activity" });
    expect(mocks.calls[0]).toMatchObject({ kind: "skill", refId: "s1" });
  });

  it("offers a retry when the first page fails", async () => {
    mocks.pages = [{ entries: [entry("a1")], next_cursor: null }];
    mocks.failFirst = true;
    const user = userEvent.setup();
    renderList();
    expect(await screen.findByText("Couldn't load activity.")).toBeInTheDocument();

    mocks.failFirst = false;
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByRole("list", { name: "Activity" })).toBeInTheDocument();
  });
});
