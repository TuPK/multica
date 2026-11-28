// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiClient, api } from "@multica/core/api";
import { snoozeDateBounds, type Proposal } from "@multica/core/company";
import { renderWithI18n } from "../../test/i18n";
import { CompanyProposalDecision } from "./company-proposal-decision";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api: { decideProposal: vi.fn() },
}));

const PROPOSAL: Proposal = {
  id: "prop-1",
  workspace_id: "ws-1",
  squad_id: "research",
  issue_id: "issue-1",
  issue_identifier: "MUL-1",
  issue_status: "in_review",
  title: "Launch pilot",
  handoff_squad_id: null,
  urgent: false,
  urgent_reason: "",
  state: "pending",
  kind: "decision",
  payload: {},
  created_by_type: "agent",
  created_by_id: "ada",
  decided_by: null,
  decided_at: null,
  decision_note: "",
  created_at: "2026-11-01T00:00:00Z",
  revision: 1,
  expires_at: null,
  snoozed_until: null,
  related: [],
  handoff: null,
};

function stubFetch() {
  const fetchMock = vi.fn().mockResolvedValue(
    new Response(JSON.stringify({ ...PROPOSAL, state: "approved" }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }),
  );
  vi.stubGlobal("fetch", fetchMock);
  const client = new ApiClient("https://api.example.test");
  vi.mocked(api.decideProposal).mockImplementation((id, data, openedAt) =>
    client.decideProposal(id, data, openedAt),
  );
  return fetchMock;
}

function renderDecision(proposal: Proposal = PROPOSAL) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <CompanyProposalDecision proposal={proposal} canDecide />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("CompanyProposalDecision attention header", () => {
  it("sends X-Attention-Opened-At with elapsed seconds on approve and omits openedAt from body", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(1_700_000_000_000);
    const fetchMock = stubFetch();
    renderDecision();

    vi.setSystemTime(1_700_000_023_700);
    fireEvent.click(screen.getByRole("button", { name: "Approve" }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect((init.headers as Record<string, string>)["X-Attention-Opened-At"]).toBe("23");
    const body = JSON.parse(String(init.body)) as Record<string, unknown>;
    expect(body).toEqual({ decision: "approve" });
    expect("openedAt" in body).toBe(false);
  });

  it("sends X-Attention-Opened-At with elapsed seconds on reject", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(1_700_000_000_000);
    const fetchMock = stubFetch();
    renderDecision();

    vi.setSystemTime(1_700_000_014_200);
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "Not now" } });
    fireEvent.click(screen.getByRole("button", { name: "Reject" }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect((init.headers as Record<string, string>)["X-Attention-Opened-At"]).toBe("14");
    expect(JSON.parse(String(init.body))).toEqual({ decision: "reject", note: "Not now" });
  });

  it("sends X-Attention-Opened-At with elapsed seconds on ask for changes (revise)", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(1_700_000_000_000);
    const fetchMock = stubFetch();
    renderDecision();

    vi.setSystemTime(1_700_000_031_900);
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "Add pricing" } });
    fireEvent.click(screen.getByRole("button", { name: "Ask for changes" }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect((init.headers as Record<string, string>)["X-Attention-Opened-At"]).toBe("31");
    expect(JSON.parse(String(init.body))).toEqual({ decision: "revise", note: "Add pricing" });
  });

  it("omits X-Attention-Opened-At on snooze", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(1_700_000_000_000);
    const fetchMock = stubFetch();
    renderDecision();
    const bounds = snoozeDateBounds(new Date());

    vi.setSystemTime(1_700_000_019_000);
    fireEvent.click(screen.getByRole("button", { name: "Decide later" }));
    fireEvent.change(screen.getByLabelText("Until"), { target: { value: bounds.min } });
    fireEvent.click(screen.getByRole("button", { name: "Set date" }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect((init.headers as Record<string, string>)["X-Attention-Opened-At"]).toBeUndefined();
    expect(JSON.parse(String(init.body))).toEqual({ decision: "snooze", until: bounds.min });
  });
});
