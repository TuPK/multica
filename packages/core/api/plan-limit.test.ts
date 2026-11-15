import { afterEach, describe, expect, it, vi } from "vitest";
import {
  ApiClient,
  ApiError,
  PLAN_LIMIT_GATES,
  PlanLimitReachedError,
  errorCode,
  isPlanLimitGate,
  planLimitReached,
} from "./client";

afterEach(() => {
  vi.unstubAllGlobals();
});

function respondWith(status: number, body: unknown) {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(
      new Response(typeof body === "string" ? body : JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/json" },
      }),
    ),
  );
}

// Any request that goes through the shared error path will do.
async function failedRequest(): Promise<unknown> {
  const client = new ApiClient("https://api.example.test");
  return client.createAgent({ name: "Reviewer", runtime_id: "runtime-1" }).catch((e: unknown) => e);
}

const body = (gate: string, limit: number, used: number) => ({
  error: "this workspace has reached its plan limit",
  code: "plan_limit_reached",
  gate,
  limit,
  used,
});

describe("plan_limit_reached", () => {
  it("is parsed into an error that carries gate, limit and used", async () => {
    respondWith(402, body("agents", 3, 3));
    const err = await failedRequest();

    expect(err).toBeInstanceOf(PlanLimitReachedError);
    // Callers that only know ApiError keep working.
    expect(err).toBeInstanceOf(ApiError);
    const limit = err as PlanLimitReachedError;
    expect(limit.gate).toBe("agents");
    expect(limit.limit).toBe(3);
    expect(limit.used).toBe(3);
    expect(limit.status).toBe(402);
    expect(limit.message).toBe("this workspace has reached its plan limit");
    expect(errorCode(limit)).toBe("plan_limit_reached");
    expect(limit.body).toMatchObject({ gate: "agents" });
  });

  it.each(PLAN_LIMIT_GATES)("carries the %s gate", async (gate) => {
    respondWith(402, body(gate, 10, 10));
    const err = await failedRequest();
    expect(planLimitReached(err)?.gate).toBe(gate);
    expect(isPlanLimitGate(gate)).toBe(true);
  });

  it("keeps a byte count above 32 bits exact", async () => {
    respondWith(402, body("storage_bytes", 214_748_364_800, 214_748_364_000));
    const err = planLimitReached(await failedRequest());
    expect(err?.limit).toBe(214_748_364_800);
    expect(err?.used).toBe(214_748_364_000);
  });

  it("accepts a limit of 0, which a plan without the quota serves", async () => {
    respondWith(402, body("runtimes", 0, 0));
    const err = planLimitReached(await failedRequest());
    expect(err).toMatchObject({ gate: "runtimes", limit: 0, used: 0 });
  });

  it("still parses a gate this client does not know, and does not treat it as one of the three", async () => {
    respondWith(402, body("future_gate", 5, 5));
    const err = planLimitReached(await failedRequest());
    expect(err?.gate).toBe("future_gate");
    expect(isPlanLimitGate("future_gate")).toBe(false);
  });

  it("ignores fields it does not know", async () => {
    respondWith(402, { ...body("agents", 3, 3), policy_revision: 2, hint: "later" });
    expect(planLimitReached(await failedRequest())).toMatchObject({ gate: "agents", limit: 3, used: 3 });
  });

  it.each([
    ["a missing gate", { code: "plan_limit_reached", limit: 3, used: 3 }],
    ["an empty gate", body("", 3, 3)],
    ["a string limit", { ...body("agents", 3, 3), limit: "3" }],
    ["a missing used", { code: "plan_limit_reached", gate: "agents", limit: 3 }],
    ["a negative limit", body("agents", -1, 0)],
    ["a fractional used", body("agents", 3, 1.5)],
  ])("falls back to a plain ApiError for %s", async (_name, malformed) => {
    respondWith(402, malformed);
    const err = await failedRequest();
    expect(err).toBeInstanceOf(ApiError);
    expect(err).not.toBeInstanceOf(PlanLimitReachedError);
    expect(planLimitReached(err)).toBeUndefined();
    expect((err as ApiError).status).toBe(402);
    expect(errorCode(err)).toBe("plan_limit_reached");
  });

  it("is only a 402: the same code on another status is a plain ApiError", async () => {
    respondWith(400, body("agents", 3, 3));
    const err = await failedRequest();
    expect(err).not.toBeInstanceOf(PlanLimitReachedError);
  });

  it("leaves the issue limit's 402 to its own path", async () => {
    respondWith(402, { code: "issue_limit_reached", error: "workspace has reached its issue limit", limit: 200 });
    const err = await failedRequest();
    expect(err).toBeInstanceOf(ApiError);
    expect(err).not.toBeInstanceOf(PlanLimitReachedError);
    expect(errorCode(err)).toBe("issue_limit_reached");
  });

  it("is not triggered by a 402 whose body is not JSON", async () => {
    respondWith(402, "payment required");
    const err = await failedRequest();
    expect(err).toBeInstanceOf(ApiError);
    expect(err).not.toBeInstanceOf(PlanLimitReachedError);
  });

  it("planLimitReached is undefined for a value that is not an ApiError", () => {
    expect(planLimitReached(new Error("boom"))).toBeUndefined();
    expect(planLimitReached(undefined)).toBeUndefined();
    expect(planLimitReached(new ApiError("x", 402, "Payment Required", body("agents", 1, 1)))).toBeUndefined();
  });
});

describe("isPlanLimitGate", () => {
  it("names exactly the three resource gates", () => {
    expect([...PLAN_LIMIT_GATES]).toEqual(["agents", "runtimes", "storage_bytes"]);
    for (const gate of ["issue_count", "autopilot_runs", "sso", "", "AGENTS"]) {
      expect(isPlanLimitGate(gate)).toBe(false);
    }
  });
});
