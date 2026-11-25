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

function route(path: string) {
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
