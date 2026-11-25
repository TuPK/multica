/**
 * @vitest-environment jsdom
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import userEvent from "@testing-library/user-event";
import { api } from "@multica/core/api";
import { renderWithI18n } from "../../test/i18n";
import { AgentTrackRecordTab } from "./agent-track-record-tab";

const role = vi.hoisted(() => ({ value: "admin" as "owner" | "admin" | "member" }));

vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("@multica/core/permissions", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/permissions")>()),
  useCurrentMember: () => ({ userId: "u-1", role: role.value, member: null, isLoading: false }),
}));
vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api: { request: vi.fn() },
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const request = vi.mocked(api.request);

const record = {
  agent_id: "a-1",
  work_class: "general",
  window_days: 30,
  model: "",
  n_pass: 8,
  n_partial: 1,
  n_fail: 2,
  n_unverified: 4,
  n_high_fail: 1,
  score_lower: 0.71,
  last_high_fail_at: null,
  verification_mode: "sampled",
  mode_pinned: false,
  mode_changed_at: null,
  computed_at: "2026-11-24T02:00:00Z",
};

function renderTab() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <AgentTrackRecordTab agentId="a-1" />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  role.value = "admin";
  request.mockReset();
  request.mockImplementation((path, init) => {
    if (init?.method === "PUT") return Promise.resolve({ ...record, verification_mode: "spot", mode_pinned: true });
    if (String(path).startsWith("/api/track-records")) return Promise.resolve({ track_records: [record] });
    return Promise.reject(new Error(`unexpected ${String(path)}`));
  });
});
afterEach(cleanup);

describe("AgentTrackRecordTab", () => {
  it("shows the verdict counts and the lower bound of each work class", async () => {
    renderTab();

    const row = (await screen.findByText("general")).closest("tr") as HTMLElement;
    expect(within(row).getByText("71%")).toBeInTheDocument();
    expect(within(row).getByText("Sampled")).toBeInTheDocument();
  });

  it("lets an owner or admin pin the check mode for one work class", async () => {
    const user = userEvent.setup({ delay: null });
    renderTab();
    await screen.findByText("general");

    await user.click(screen.getByRole("combobox", { name: "Check mode for general" }));
    await user.click(await screen.findByRole("option", { name: "Spot checks" }));

    await waitFor(() =>
      expect(request).toHaveBeenCalledWith(
        "/api/track-records/pin",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({ agent_id: "a-1", work_class: "general", mode: "spot" }),
        }),
      ),
    );
  });

  it("shows the mode to a member without a pin control", async () => {
    role.value = "member";
    renderTab();

    expect(await screen.findByText("Sampled")).toBeInTheDocument();
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
  });
});
