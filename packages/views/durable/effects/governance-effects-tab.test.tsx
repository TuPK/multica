import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { api, ApiError } from "@multica/core/api";
import { renderWithI18n } from "../../test/i18n";
import { GovernanceEffectsTab } from "./governance-effects-tab";

vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api: { request: vi.fn() },
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

function effect(id: string, patch: Record<string, unknown> = {}) {
  return {
    id,
    created_at: "2026-11-25T08:00:00Z",
    source: "connected_app",
    target: `gmail:${id}`,
    effect_class: "send",
    reversibility: "irreversible",
    decision: "allow",
    status: "executed",
    summary: `Summary ${id}`,
    task_id: null,
    agent_id: null,
    member_id: "u-1",
    undo_available: false,
    undo_until: null,
    ...patch,
  };
}

const request = vi.mocked(api.request);

function renderTab() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <GovernanceEffectsTab />
    </QueryClientProvider>,
  );
}

beforeEach(() => vi.clearAllMocks());
afterEach(() => request.mockReset());

describe("GovernanceEffectsTab", () => {
  it("keeps the effects the platform controlled apart from the ones agents only declared", async () => {
    request.mockResolvedValueOnce({
      effects: [
        effect("e-1"),
        effect("e-2", { source: "declared", status: "declared", decision: "rehearse", summary: "Declared mail" }),
      ],
    });
    renderTab();

    const controlled = await screen.findByRole("region", { name: "Controlled by the platform" });
    expect(await within(controlled).findByText("Summary e-1")).toBeInTheDocument();
    expect(within(controlled).queryByText("Declared mail")).not.toBeInTheDocument();

    const declared = screen.getByRole("region", { name: "Declared by agents" });
    expect(within(declared).getByText("Declared mail")).toBeInTheDocument();
    expect(within(declared).getByText("The platform recorded these but could not stop them.")).toBeInTheDocument();
  });

  it("offers undo only while the undo window is open", async () => {
    request.mockResolvedValueOnce({
      effects: [effect("e-1", { undo_available: true, undo_until: "2026-11-26T08:00:00Z" }), effect("e-2")],
    });
    renderTab();

    await screen.findByText("Summary e-1");
    const controlled = screen.getByRole("region", { name: "Controlled by the platform" });
    const rows = within(controlled).getAllByRole("listitem");
    expect(within(rows[0]!).getByRole("button", { name: "Undo" })).toBeInTheDocument();
    expect(within(rows[1]!).queryByRole("button", { name: "Undo" })).not.toBeInTheDocument();
  });

  it("undoes an effect through its route and reports it", async () => {
    request
      .mockResolvedValueOnce({ effects: [effect("e-1", { undo_available: true, undo_until: "2026-11-26T08:00:00Z" })] })
      .mockResolvedValueOnce({ effect_id: "e-1", status: "undone", undo_event_id: "e-9" });
    renderTab();

    fireEvent.click(await screen.findByRole("button", { name: "Undo" }));

    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Effect undone."));
    expect(request).toHaveBeenCalledWith("/api/effects/e-1/undo", expect.objectContaining({ method: "POST" }));
  });

  it("says when the undo window has closed", async () => {
    request
      .mockResolvedValueOnce({ effects: [effect("e-1", { undo_available: true, undo_until: "2026-11-26T08:00:00Z" })] })
      .mockRejectedValueOnce(new ApiError("closed", 409, "Conflict", { code: "undo_window_closed" }));
    renderTab();

    fireEvent.click(await screen.findByRole("button", { name: "Undo" }));

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("The undo window for this effect has closed."));
  });
});
