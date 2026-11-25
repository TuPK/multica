import { fireEvent, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "@multica/core/api";
import { renderWithI18n } from "../../test/i18n";
import { RunEffectsButton } from "./run-effects-button";

const { flag } = vi.hoisted(() => ({ flag: { on: false } }));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("@multica/core/config", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/config")>()),
  useFeatureEnabled: () => flag.on,
}));
vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api: { request: vi.fn() },
}));

const request = vi.mocked(api.request);

function renderButton(open: boolean) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <RunEffectsButton taskId="t-1" open={open} />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  flag.on = false;
  request.mockReset();
});
afterEach(() => request.mockReset());

describe("RunEffectsButton", () => {
  it("is not shown, and asks for nothing, while durable effects is off", () => {
    renderButton(true);
    expect(screen.queryByRole("button", { name: "Effects of this run" })).not.toBeInTheDocument();
    expect(request).not.toHaveBeenCalled();
  });

  it("lists the effects of this run once it is opened", async () => {
    flag.on = true;
    request.mockResolvedValueOnce({
      effects: [
        {
          id: "e-1",
          created_at: "2026-11-25T08:00:00Z",
          source: "connected_app",
          target: "gmail",
          effect_class: "send",
          reversibility: "irreversible",
          decision: "allow",
          status: "executed",
          summary: "Sent the quote",
          task_id: "t-1",
          agent_id: null,
          member_id: null,
          undo_available: false,
          undo_until: null,
        },
      ],
    });
    renderButton(true);

    fireEvent.click(screen.getByRole("button", { name: "Effects of this run" }));

    expect(await screen.findByText("Sent the quote")).toBeInTheDocument();
    await waitFor(() => expect(request).toHaveBeenCalledWith("/api/effects?task_id=t-1&limit=50"));
  });

  it("asks for nothing while the run's transcript is closed", () => {
    flag.on = true;
    renderButton(false);
    expect(request).not.toHaveBeenCalled();
  });
});
