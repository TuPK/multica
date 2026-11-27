// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiClient } from "../api/client";
import { FALLBACK_COMPANY_STATE } from "./schemas";

afterEach(() => {
  vi.unstubAllGlobals();
});

function respond(body: unknown, status = 200) {
  const fetchMock = vi.fn().mockResolvedValue(
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }),
  );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

const client = () => new ApiClient("https://api.test");

function sentOf(fetchMock: ReturnType<typeof respond>) {
  const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
  return {
    method: init.method ?? "GET",
    path: url.replace("https://api.test", ""),
    body: init.body === undefined ? undefined : JSON.parse(init.body as string),
  };
}

const state = {
  state: "paused",
  setting: "paused",
  quiet_hours: { enabled: false, start: "22:00", end: "07:00", timezone: "UTC" },
  updated_at: "2026-11-02T10:00:00Z",
};

describe("company state requests", () => {
  it("PUT sends stop_running with paused when the caller asks to stop running work", async () => {
    const fetchMock = respond(state);
    const updated = await client().setCompanyState({ state: "paused", stop_running: true });
    expect(sentOf(fetchMock)).toEqual({
      method: "PUT",
      path: "/api/company/company-state",
      body: { state: "paused", stop_running: true },
    });
    expect(updated.state).toBe("paused");
  });

  it("PUT quiet hours sends the window and returns the company state", async () => {
    const fetchMock = respond(state);
    await client().setQuietHours({ enabled: true, start: "21:30", end: "06:45", timezone: "Europe/Paris" });
    expect(sentOf(fetchMock)).toEqual({
      method: "PUT",
      path: "/api/company/quiet-hours",
      body: { enabled: true, start: "21:30", end: "06:45", timezone: "Europe/Paris" },
    });
  });

  it("a company state response that cannot be read is unknown, not running", async () => {
    respond({ state: 42 });
    const read = await client().getCompanyState();
    expect(read).toEqual(FALLBACK_COMPANY_STATE);
    expect(read.state).toBe("unknown");
  });
});

describe("governance log requests", () => {
  it("GET sends the filters and cursor, and omits the empty ones", async () => {
    const fetchMock = respond({ events: [], next_cursor: null });
    await client().listGovernanceEvents({ kind: "budget_hold", subject_type: "squad" }, { cursor: "c-1" });
    expect(sentOf(fetchMock)).toEqual({
      method: "GET",
      path: "/api/governance/events?kind=budget_hold&subject_type=squad&cursor=c-1",
      body: undefined,
    });
  });

  it("the CSV export uses the same filters on the CSV endpoint", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response("id,kind\n", { status: 200, headers: { "Content-Type": "text/csv" } }));
    vi.stubGlobal("fetch", fetchMock);
    const blob = await client().exportGovernanceEventsCsv({ actor_type: "agent" });
    expect(sentOf(fetchMock)).toEqual({
      method: "GET",
      path: "/api/governance/events.csv?actor_type=agent",
      body: undefined,
    });
    expect(await blob.text()).toBe("id,kind\n");
  });
});

describe("queued attention requests", () => {
  it("GET reads queued asks from /api/company/attention/queued", async () => {
    const fetchMock = respond({
      asks: [
        {
          subject_type: "effect_intent",
          subject_id: "intent-1",
          reason: "irreversible",
          estimated_minutes: 2,
          created_at: "2026-11-26T09:00:00Z",
        },
      ],
    });
    const out = await client().getQueuedAttention();
    expect(sentOf(fetchMock)).toEqual({
      method: "GET",
      path: "/api/company/attention/queued",
      body: undefined,
    });
    expect(out.asks).toHaveLength(1);
    expect(out.asks[0]?.subject_id).toBe("intent-1");
  });

  it("degrades a malformed queued attention response to an empty list", async () => {
    respond("not-an-object");
    const out = await client().getQueuedAttention();
    expect(out).toEqual({ asks: [] });
  });
});
