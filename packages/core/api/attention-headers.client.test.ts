// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiClient } from "./client";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

function respondWith(body: unknown) {
  const fetchMock = vi.fn().mockResolvedValue(
    new Response(JSON.stringify(body), { status: 200, headers: { "Content-Type": "application/json" } }),
  );
  vi.stubGlobal("fetch", fetchMock);
  return { client: new ApiClient("https://api.example.test"), fetchMock };
}

function callDetails(fetchMock: ReturnType<typeof vi.fn>, callIndex = 0) {
  const [url, init] = fetchMock.mock.calls[callIndex] as [string, RequestInit];
  return {
    url,
    method: init.method,
    headers: init.headers as Record<string, string>,
    body: JSON.parse(String(init.body)) as Record<string, unknown>,
  };
}

const proposalRow = {
  id: "p-1",
  workspace_id: "ws-1",
  squad_id: "sq-1",
  issue_id: "iss-1",
  issue_identifier: "MUL-1",
  issue_status: "in_review",
  title: "Ship feature",
  handoff_squad_id: null,
  urgent: false,
  urgent_reason: "",
  state: "approved",
  kind: "decision",
  payload: {},
  created_by_type: "agent",
  created_by_id: "a-1",
  decided_by: "u-1",
  decided_at: "2026-11-28T00:00:00Z",
  decision_note: "",
  created_at: "2026-11-27T00:00:00Z",
  revision: 1,
  expires_at: null,
  snoozed_until: null,
  related: [],
  handoff: null,
};

const appRequestRow = {
  id: "r-1",
  kind: "action",
  status: "approved",
  agent_id: "a-1",
  issue_id: "iss-1",
  approver_user_id: "u-1",
  account_scope: "personal",
  toolkit_slug: "gmail",
  tool_slug: "GMAIL_SEND_EMAIL",
  action_class: "send",
  summary: "Send email",
  arguments: { to: "a@example.com" },
  error: null,
  reason: null,
  redacted: false,
  expires_at: "2026-11-28T01:00:00Z",
  decided_by: "u-1",
  decided_at: "2026-11-28T00:00:00Z",
  created_at: "2026-11-28T00:00:00Z",
};

describe("ApiClient attention opened headers", () => {
  describe("decideProposal", () => {
    it("sends X-Attention-Opened-At with elapsed seconds and keeps openedAt out of body", async () => {
      vi.useFakeTimers({ toFake: ["Date"] });
      vi.setSystemTime(1_700_000_037_900);
      const { client, fetchMock } = respondWith(proposalRow);

      await client.decideProposal("p-1", { decision: "approve", note: "Looks good" }, 1_700_000_000_000);

      const sent = callDetails(fetchMock);
      expect(sent.method).toBe("POST");
      expect(sent.url).toBe("https://api.example.test/api/proposals/p-1/decision");
      expect(sent.headers["X-Attention-Opened-At"]).toBe("37");
      expect(sent.body).toEqual({ decision: "approve", note: "Looks good" });
      expect("openedAt" in sent.body).toBe(false);
    });

    it("omits X-Attention-Opened-At when openedAt is not provided", async () => {
      const { client, fetchMock } = respondWith(proposalRow);

      await client.decideProposal("p-1", { decision: "reject", note: "No" });

      const sent = callDetails(fetchMock);
      expect(sent.headers["X-Attention-Opened-At"]).toBeUndefined();
      expect(sent.body).toEqual({ decision: "reject", note: "No" });
    });
  });

  describe("approveAppRequest", () => {
    it("sends X-Attention-Opened-At with elapsed seconds and keeps openedAt out of body", async () => {
      vi.useFakeTimers({ toFake: ["Date"] });
      vi.setSystemTime(1_700_000_024_500);
      const { client, fetchMock } = respondWith(appRequestRow);

      await client.approveAppRequest("r-1", { to: "b@example.com" }, 1_700_000_000_000);

      const sent = callDetails(fetchMock);
      expect(sent.method).toBe("POST");
      expect(sent.url).toBe("https://api.example.test/api/app-requests/r-1/approve");
      expect(sent.headers["X-Attention-Opened-At"]).toBe("24");
      expect(sent.body).toEqual({ arguments: { to: "b@example.com" } });
      expect("openedAt" in sent.body).toBe(false);
    });

    it("sends X-Attention-Opened-At when arguments are undefined", async () => {
      vi.useFakeTimers({ toFake: ["Date"] });
      vi.setSystemTime(1_700_000_011_200);
      const { client, fetchMock } = respondWith(appRequestRow);

      await client.approveAppRequest("r-1", undefined, 1_700_000_000_000);

      const sent = callDetails(fetchMock);
      expect(sent.headers["X-Attention-Opened-At"]).toBe("11");
      expect(sent.body).toEqual({});
    });

    it("omits X-Attention-Opened-At when openedAt is not provided", async () => {
      const { client, fetchMock } = respondWith(appRequestRow);

      await client.approveAppRequest("r-1");

      const sent = callDetails(fetchMock);
      expect(sent.headers["X-Attention-Opened-At"]).toBeUndefined();
      expect(sent.body).toEqual({});
    });
  });

  describe("rejectAppRequest", () => {
    it("sends X-Attention-Opened-At with elapsed seconds and keeps openedAt out of body", async () => {
      vi.useFakeTimers({ toFake: ["Date"] });
      vi.setSystemTime(1_700_000_052_800);
      const { client, fetchMock } = respondWith({ ...appRequestRow, status: "rejected" });

      await client.rejectAppRequest("r-1", "Wrong recipient", 1_700_000_000_000);

      const sent = callDetails(fetchMock);
      expect(sent.method).toBe("POST");
      expect(sent.url).toBe("https://api.example.test/api/app-requests/r-1/reject");
      expect(sent.headers["X-Attention-Opened-At"]).toBe("52");
      expect(sent.body).toEqual({ reason: "Wrong recipient" });
      expect("openedAt" in sent.body).toBe(false);
    });

    it("omits X-Attention-Opened-At when openedAt is not provided", async () => {
      const { client, fetchMock } = respondWith({ ...appRequestRow, status: "rejected" });

      await client.rejectAppRequest("r-1", "Wrong recipient");

      const sent = callDetails(fetchMock);
      expect(sent.headers["X-Attention-Opened-At"]).toBeUndefined();
      expect(sent.body).toEqual({ reason: "Wrong recipient" });
    });
  });
});
