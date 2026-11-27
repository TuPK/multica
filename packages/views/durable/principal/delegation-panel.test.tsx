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

const sampleStatus = {
  delegations: [
    {
      decision_kind: "general",
      enabled: true,
      min_confidence: 0.92,
      enabled_at: "2026-11-26T08:00:00Z",
      disabled_reason: "",
    },
  ],
  kinds: [
    { decision_kind: "general", matched: 23, n: 25, lower: 0.75, eligible: true },
    { decision_kind: "proposal:general", matched: 20, n: 22, lower: 0.72, eligible: true },
    { decision_kind: "inquiry", matched: 8, n: 10, lower: 0.49, eligible: false },
  ],
};

function renderPanel() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <DelegationPanel wsId="ws-1" />
    </QueryClientProvider>,
  );
}

beforeEach(() => vi.clearAllMocks());
afterEach(() => {
  cleanup();
  request.mockReset();
});

describe("DelegationPanel", () => {
  it("reads GET /api/principal/delegations, shows on/off state, min_confidence, accuracy, and locks forbidden kinds", async () => {
    request.mockResolvedValueOnce(sampleStatus);
    renderPanel();

    expect(await screen.findByText("On")).toBeInTheDocument();
    expect(screen.getByText("Min 92%")).toBeInTheDocument();
    expect(screen.getByText("23 of 25 matched · lower bound 75%")).toBeInTheDocument();
    expect(screen.getByText("20 of 22 matched · lower bound 72%")).toBeInTheDocument();
    expect(screen.getByText("8 of 10 matched · lower bound 49%")).toBeInTheDocument();

    expect(screen.getByRole("button", { name: "Disable General" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Enable General proposals" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Enable Secretary inquiries" })).toBeDisabled();
    expect(
      screen.getAllByText("Needs at least 20 resolved predictions with 85% matched.").length,
    ).toBeGreaterThan(0);

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

  it("enables an eligible kind with PUT /api/principal/delegations and invalidates delegations", async () => {
    let getCalls = 0;
    request.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path === "/api/principal/delegations" && !init?.method) {
        getCalls += 1;
        return sampleStatus;
      }
      if (path === "/api/principal/delegations" && init?.method === "PUT") {
        return { decision_kind: "proposal:general", enabled: true };
      }
      throw new Error(`unexpected ${init?.method ?? "GET"} ${path}`);
    });
    renderPanel();

    fireEvent.click(await screen.findByRole("button", { name: "Enable General proposals" }));

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
    await waitFor(() => expect(getCalls).toBeGreaterThanOrEqual(2));
  });

  it("disables an active kind via DELETE /api/principal/delegations?decision_kind= and invalidates delegations", async () => {
    let getCalls = 0;
    request.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path === "/api/principal/delegations" && !init?.method) {
        getCalls += 1;
        return sampleStatus;
      }
      if (path === "/api/principal/delegations?decision_kind=general" && init?.method === "DELETE") {
        return undefined;
      }
      throw new Error(`unexpected ${init?.method ?? "GET"} ${path}`);
    });
    renderPanel();

    fireEvent.click(await screen.findByRole("button", { name: "Disable General" }));

    await waitFor(() =>
      expect(toast.success).toHaveBeenCalledWith("Delegation disabled for General."),
    );
    expect(request).toHaveBeenCalledWith(
      "/api/principal/delegations?decision_kind=general",
      expect.objectContaining({ method: "DELETE" }),
    );
    await waitFor(() => expect(getCalls).toBeGreaterThanOrEqual(2));
  });

  it.each([
    [
      "delegation_not_eligible",
      "Needs at least 20 resolved predictions of this kind with at least 85% matched.",
    ],
    ["delegation_forbidden_kind", "This decision kind cannot be delegated."],
  ])("maps 422 %s to localized copy", async (code, expected) => {
    request.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path === "/api/principal/delegations" && !init?.method) return sampleStatus;
      if (path === "/api/principal/delegations" && init?.method === "PUT") {
        throw new ApiError("err", 422, "Unprocessable Entity", { code });
      }
      throw new Error(`unexpected ${init?.method ?? "GET"} ${path}`);
    });
    renderPanel();

    fireEvent.click(await screen.findByRole("button", { name: "Enable General proposals" }));

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith(expected));
  });
});
