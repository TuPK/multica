// @vitest-environment jsdom
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { api, ApiError } from "@multica/core/api";
import { renderWithI18n } from "../../test/i18n";
import { DelegationPanel } from "./delegation-panel";

vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api: { request: vi.fn() },
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const request = vi.mocked(api.request);

function renderPanel() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <DelegationPanel />
    </QueryClientProvider>,
  );
}

beforeEach(() => vi.clearAllMocks());
afterEach(() => {
  cleanup();
  request.mockReset();
});

describe("DelegationPanel", () => {
  it("locks every forbidden kind from spec 6.5 and notes irreversible/urgent exclusions", () => {
    renderPanel();

    expect(
      screen.getByText(/Irreversible actions and urgent questions are never delegated/i),
    ).toBeInTheDocument();

    const lockedLabels = [
      "Organization changes",
      "Launch gate",
      "Stage gates",
      "Spend actions",
      "External sharing",
      "Budget changes",
      "Autonomy changes",
      "Autonomy envelopes",
      "Approval policies",
      "Principal profiles",
    ];
    for (const label of lockedLabels) {
      const btn = screen.getByRole("button", { name: `Enable ${label}` });
      expect(btn, label).toBeDisabled();
    }
  });

  it("enables a delegatable kind with default min_confidence 0.90", async () => {
    request.mockResolvedValueOnce({ decision_kind: "proposal:general", enabled: true });
    renderPanel();

    fireEvent.click(screen.getByRole("button", { name: "Enable General proposals" }));

    await waitFor(() =>
      expect(toast.success).toHaveBeenCalledWith("Delegation enabled for General proposals."),
    );
    expect(request).toHaveBeenCalledWith(
      "/api/principal/delegations",
      expect.objectContaining({
        method: "PUT",
        body: JSON.stringify({ decision_kind: "proposal:general", min_confidence: 0.9 }),
      }),
    );
  });

  it("disables a kind via DELETE /api/principal/delegations?decision_kind=", async () => {
    request.mockResolvedValueOnce(undefined);
    renderPanel();

    fireEvent.click(screen.getByRole("button", { name: "Disable General proposals" }));

    await waitFor(() =>
      expect(toast.success).toHaveBeenCalledWith("Delegation disabled for General proposals."),
    );
    expect(request).toHaveBeenCalledWith(
      "/api/principal/delegations?decision_kind=proposal%3Ageneral",
      expect.objectContaining({ method: "DELETE" }),
    );
  });

  it.each([
    [
      "delegation_not_eligible",
      "Needs at least 20 resolved predictions of this kind with at least 85% matched.",
    ],
    ["delegation_forbidden_kind", "This decision kind cannot be delegated."],
  ])("maps 422 %s to localized copy", async (code, expected) => {
    request.mockRejectedValueOnce(new ApiError("err", 422, "Unprocessable Entity", { code }));
    renderPanel();

    fireEvent.click(screen.getByRole("button", { name: "Enable General" }));

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith(expected));
  });
});
