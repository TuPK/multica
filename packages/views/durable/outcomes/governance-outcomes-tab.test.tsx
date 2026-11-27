/**
 * @vitest-environment jsdom
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import userEvent from "@testing-library/user-event";
import { api } from "@multica/core/api";
import { renderWithI18n } from "../../test/i18n";
import { GovernanceOutcomesTab } from "./governance-outcomes-tab";

vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api: { request: vi.fn(), listAgents: vi.fn(), listSquads: vi.fn() },
}));

const request = vi.mocked(api.request);
const listAgents = vi.mocked(api.listAgents);
const listSquads = vi.mocked(api.listSquads);

const outcome = (id: string, verdict: string) => ({
  id,
  issue_id: `i-${id}`,
  agent_id: "a-1",
  member_id: null,
  squad_id: null,
  work_class: "general",
  verification_mode: "full",
  sampled: false,
  verdict,
  severity: "none",
  fail_cause: "",
  settle_at: "2026-11-20T10:00:00Z",
  settled_at: null,
  superseded_by: null,
  created_at: "2026-11-18T10:00:00Z",
});

const DAY = 24 * 60 * 60 * 1000;

function route(path: string) {
  // The weekly chart asks for the outcomes since eight weeks back; those are the rows the chart counts.
  if (path.includes("since=")) {
    const now = Date.now();
    return Promise.resolve({
      outcomes: [
        { ...outcome("w-1", "pass"), work_class: "weekly", created_at: new Date(now).toISOString() },
        { ...outcome("w-2", "fail"), work_class: "weekly", created_at: new Date(now).toISOString() },
        { ...outcome("w-3", "pass"), work_class: "weekly", created_at: new Date(now).toISOString() },
        { ...outcome("w-4", "partial"), work_class: "weekly", created_at: new Date(now - 8 * DAY).toISOString() },
        { ...outcome("w-5", "pending"), work_class: "weekly", created_at: new Date(now).toISOString() },
      ],
      next_cursor: "",
    });
  }
  if (path.startsWith("/api/outcomes/summary")) {
    return Promise.resolve({
      group_by: "work_class",
      groups: [
        { key: "general", total: 6, pass: 3, partial: 1, fail: 1, unverified: 1, pending: 0 },
        { key: "research", total: 2, pass: 0, partial: 0, fail: 0, unverified: 0, pending: 2 },
      ],
    });
  }
  if (path.startsWith("/api/outcomes")) return Promise.resolve({ outcomes: [outcome("o-1", "pass")], next_cursor: "" });
  return Promise.reject(new Error(`unexpected ${path}`));
}

function renderTab() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <GovernanceOutcomesTab />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  request.mockReset();
  request.mockImplementation((path) => route(String(path)));
  listAgents.mockReset().mockResolvedValue([{ id: "a-1", name: "Ada", archived_at: null }] as never);
  listSquads.mockReset().mockResolvedValue([] as never);
});
afterEach(cleanup);

describe("GovernanceOutcomesTab", () => {
  it("shows verified coverage as pass, partial and fail over the total", async () => {
    renderTab();

    const coverage = await screen.findByRole("region", { name: "Verified coverage" });
    expect(await within(coverage).findByText("63%")).toBeInTheDocument();
  });

  it("charts the pass rate of checked outcomes for each of the last eight weeks", async () => {
    renderTab();

    const chart = await screen.findByRole("region", { name: "Pass rate by week" });
    // This week: two passed of three checked, the pending one left out. Eight days back: none passed of one.
    expect(await within(chart).findByText(/67% \(2 of 3 checked\)/)).toBeInTheDocument();
    expect(within(chart).getByText(/0% \(0 of 1 checked\)/)).toBeInTheDocument();
  });

  it("loads later list pages and counts later chart pages", async () => {
    request.mockImplementation((path) => {
      const url = String(path);
      if (url.startsWith("/api/outcomes/summary")) return route(url);
      const second = url.includes("cursor=next");
      if (url.includes("since=")) return Promise.resolve({
        outcomes: [{ ...outcome(second ? "chart-2" : "chart-1", second ? "fail" : "pass"), created_at: new Date().toISOString() }],
        next_cursor: second ? "" : "next",
      });
      return Promise.resolve({
        outcomes: [{ ...outcome(second ? "list-2" : "list-1", "pass"), work_class: second ? "second row" : "first row" }],
        next_cursor: second ? "" : "next",
      });
    });
    const user = userEvent.setup({ delay: null });
    renderTab();
    expect(await screen.findByText("first row")).toBeInTheDocument();
    await user.click(await screen.findByRole("button", { name: "Load more" }));
    expect(await screen.findByText("second row")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Load more" })).not.toBeInTheDocument();
    expect(await screen.findByText(/50% \(1 of 2 checked\)/)).toBeInTheDocument();
    expect(request.mock.calls.filter(([path]) => String(path).includes("cursor=next"))).toHaveLength(2);
  });

  it("filters the outcomes by verdict through the server", async () => {
    const user = userEvent.setup({ delay: null });
    renderTab();
    await screen.findByText("general");

    await user.click(screen.getByRole("combobox", { name: "Verdict" }));
    await user.click(await screen.findByRole("option", { name: "Failed" }));

    await waitFor(() =>
      expect(request.mock.calls.some(([path]) => String(path).startsWith("/api/outcomes?") && String(path).includes("verdict=fail"))).toBe(true),
    );
  });
});
