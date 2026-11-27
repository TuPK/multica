/**
 * @vitest-environment jsdom
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { api, ApiError } from "@multica/core/api";
import type { AgentRuntime } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { ShadowTrialDialog } from "./shadow-trial-dialog";

vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api: {
    request: vi.fn(),
    listRuntimes: vi.fn(),
  },
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const request = vi.mocked(api.request);
const listRuntimes = vi.mocked(api.listRuntimes);

const sampleRuntime: AgentRuntime = {
  id: "rt-2",
  workspace_id: "ws-1",
  daemon_id: "d-1",
  name: "Claude Code (sandbox)",
  custom_name: "Sandbox Runner",
  runtime_mode: "local",
  provider: "claude",
  launch_header: "",
  status: "online",
  device_info: "linux",
  metadata: {},
  owner_id: "u-1",
  visibility: "public",
  last_seen_at: "2026-11-26T00:00:00Z",
  created_at: "2026-11-01T00:00:00Z",
  updated_at: "2026-11-26T00:00:00Z",
};

function renderDialog(
  onOpenChange = vi.fn(),
  locale: "en" | "vi" = "en",
) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  renderWithI18n(
    <QueryClientProvider client={qc}>
      <ShadowTrialDialog open onOpenChange={onOpenChange} wsId="ws-1" agentId="a-1" />
    </QueryClientProvider>,
    { locale },
  );
  return { onOpenChange };
}

beforeEach(() => {
  request.mockReset();
  listRuntimes.mockReset();
  listRuntimes.mockResolvedValue([sampleRuntime]);
  vi.mocked(toast.success).mockReset();
  vi.mocked(toast.error).mockReset();
});

afterEach(cleanup);

describe("ShadowTrialDialog", () => {
  it("shows the cost estimate, the 9.4 access-key warning, and no auto-promote toggle", async () => {
    request.mockResolvedValueOnce({
      estimate_micros: 1_500_000,
      remaining_micros: 5_000_000,
      warn: false,
    });
    renderDialog();

    expect(
      screen.getByText(
        "Only use a runtime with no access keys to real systems other than the model key.",
      ),
    ).toBeInTheDocument();
    expect(await screen.findByText(/Estimate:\s*\$1\.50/)).toBeInTheDocument();
    expect(screen.getByText(/Budget remaining:\s*\$5\.00/)).toBeInTheDocument();
    expect(screen.queryByText(/auto.?promote/i)).not.toBeInTheDocument();
  });

  it("shows the verbatim Vietnamese 9.4 warning when locale is vi", async () => {
    request.mockResolvedValueOnce({
      estimate_micros: 0,
      remaining_micros: -1,
      warn: false,
    });
    renderDialog(vi.fn(), "vi");

    expect(
      screen.getByText("Chỉ dùng runtime không có khoá truy cập hệ thống thật ngoài khoá mô hình."),
    ).toBeInTheDocument();
    expect(await screen.findByText("Chưa có ngân sách áp cho ghế này.")).toBeInTheDocument();
  });

  it("gates Start behind explicit confirmation when warn is true and sends confirm_cost=true only then", async () => {
    const user = userEvent.setup({ delay: null });
    request.mockImplementation(async (_path, init) => {
      if (init?.method === "POST") {
        return {
          id: "sr-1",
          agent_id: "a-1",
          candidate_runtime_id: "rt-2",
          candidate_model: "claude-sonnet",
          sample_rate: 0.3,
          target_pairs: 20,
          auto_promote: false,
          state: "running",
        };
      }
      return { estimate_micros: 4_000_000, remaining_micros: 2_000_000, warn: true };
    });

    const { onOpenChange } = renderDialog();
    expect(
      await screen.findByText("Estimated cost exceeds the remaining budget for this seat."),
    ).toBeInTheDocument();

    await user.click(screen.getByRole("combobox", { name: "Candidate runtime" }));
    await user.click(await screen.findByRole("option", { name: "Sandbox Runner (Claude)" }));

    await user.type(screen.getByLabelText("Candidate model"), "claude-sonnet");
    await user.type(screen.getByLabelText("Work class prefix"), "flow:");

    const startButton = screen.getByRole("button", { name: "Start trial" });
    expect(startButton).toBeDisabled();

    await user.click(
      screen.getByRole("checkbox", {
        name: "I confirm starting this shadow trial above the remaining budget.",
      }),
    );
    expect(startButton).toBeEnabled();

    await user.click(startButton);

    await waitFor(() =>
      expect(request).toHaveBeenCalledWith(
        "/api/agents/a-1/shadow",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({
            candidate_runtime_id: "rt-2",
            candidate_model: "claude-sonnet",
            sample_rate: 0.3,
            target_pairs: 20,
            work_class_prefix: "flow:",
            auto_promote: false,
            confirm_cost: true,
          }),
        }),
      ),
    );
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(toast.success).toHaveBeenCalled();
  });

  it("sends auto_promote=false and confirm_cost=false when within budget", async () => {
    const user = userEvent.setup({ delay: null });
    request.mockImplementation(async (_path, init) => {
      if (init?.method === "POST") {
        return {
          id: "sr-2",
          agent_id: "a-1",
          candidate_runtime_id: "rt-2",
          candidate_model: "",
          sample_rate: 0.3,
          target_pairs: 20,
          auto_promote: false,
          state: "running",
        };
      }
      return { estimate_micros: 500_000, remaining_micros: -1, warn: false };
    });

    renderDialog();
    expect(await screen.findByText("No budget covers this seat.")).toBeInTheDocument();

    await user.click(screen.getByRole("combobox", { name: "Candidate runtime" }));
    await user.click(await screen.findByRole("option", { name: "Sandbox Runner (Claude)" }));
    await user.click(screen.getByRole("button", { name: "Start trial" }));

    await waitFor(() =>
      expect(request).toHaveBeenCalledWith(
        "/api/agents/a-1/shadow",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({
            candidate_runtime_id: "rt-2",
            candidate_model: "",
            sample_rate: 0.3,
            target_pairs: 20,
            work_class_prefix: "",
            auto_promote: false,
            confirm_cost: false,
          }),
        }),
      ),
    );
  });

  it("shows mapped server errors and keeps the dialog open on failure", async () => {
    const user = userEvent.setup({ delay: null });
    request.mockImplementation(async (_path, init) => {
      if (init?.method === "POST") {
        throw new ApiError("already running", 409, "Conflict", { code: "shadow_already_running" });
      }
      return { estimate_micros: 500_000, remaining_micros: -1, warn: false };
    });

    const { onOpenChange } = renderDialog();
    await screen.findByText("No budget covers this seat.");

    await user.click(screen.getByRole("combobox", { name: "Candidate runtime" }));
    await user.click(await screen.findByRole("option", { name: "Sandbox Runner (Claude)" }));
    await user.click(screen.getByRole("button", { name: "Start trial" }));

    expect(
      await screen.findByText("A shadow trial is already running on this seat."),
    ).toBeInTheDocument();
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
  });
});
