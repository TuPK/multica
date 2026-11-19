/**
 * Refusals the console API gives for requests the screens do not send: an invalid
 * e-invoice state filter, an invalid plan key, an invalid tenant subscription, and
 * an e-invoice retry while the provider is not configured. The UI cannot send these
 * (the screens offer only valid values), so each test calls the API directly and
 * checks the refusal code. Flow numbers refer to docs/engineering/platform-management-flows.md.
 */
import { expect, request as playwrightRequest, test, type APIRequestContext, type APIResponse } from "@playwright/test";
import { randomUUID } from "node:crypto";
import { CONSOLE_BASE, signInApi, startPlatform, stopPlatform, type StaffSeed } from "./harness";

const ORIGIN = { Origin: CONSOLE_BASE, "Content-Type": "application/json" };

let seed: StaffSeed;
let admin: APIRequestContext;

test.beforeAll(async () => {
  test.setTimeout(300_000);
  seed = await startPlatform("admin");
  admin = await playwrightRequest.newContext();
  await signInApi(admin, seed);
});

test.afterAll(async () => {
  test.setTimeout(300_000);
  await admin?.dispose();
  await stopPlatform(seed ?? null);
});

async function expectCode(res: APIResponse, status: number, code: string): Promise<void> {
  expect(res.status()).toBe(status);
  expect((await res.json()).code).toBe(code);
}

test.describe("API refusals the screens cannot send", () => {
  test("flow 271: an e-invoice list filter on a state that does not exist is refused with invalid_state", async () => {
    const res = await admin.get(`${CONSOLE_BASE}/api/platform/einvoices?state=cancelled`);
    await expectCode(res, 400, "invalid_state");
  });

  test("flow 272: a plan key with capital letters is refused with invalid_plan", async () => {
    const res = await admin.put(`${CONSOLE_BASE}/api/platform/plans/Bad-Key`, {
      headers: ORIGIN,
      data: { name: "E2E Bad Key" },
    });
    await expectCode(res, 400, "invalid_plan");
  });

  test("flow 273: a tenant subscription with more seats than the limit is refused with invalid_subscription", async () => {
    const res = await admin.put(`${CONSOLE_BASE}/api/platform/tenants/${randomUUID()}/subscription`, {
      headers: ORIGIN,
      data: { plan_key: "pro", seats: 2_000_000, interval: "month", reason: "e2e seats limit" },
    });
    await expectCode(res, 400, "invalid_subscription");
  });

  test("flow 228: an e-invoice retry while the provider is not configured is refused with einvoice_provider_not_configured", async () => {
    const res = await admin.post(`${CONSOLE_BASE}/api/platform/einvoices/${randomUUID()}/retry`, {
      headers: ORIGIN,
      data: { reason: "e2e provider check" },
    });
    await expectCode(res, 503, "einvoice_provider_not_configured");
  });

  test("flow 235: cancelling a scheduled price change when none is scheduled is refused with price_change_not_found", async () => {
    const res = await admin.delete(`${CONSOLE_BASE}/api/platform/plans/business/price-changes/scheduled`, {
      headers: ORIGIN,
      data: { reason: "e2e cancel without schedule" },
    });
    await expectCode(res, 404, "price_change_not_found");
  });

  test("flow 234: a second price change while one is scheduled is refused with price_change_pending", async () => {
    const day = new Date(Date.now() + 60 * 24 * 60 * 60 * 1000).toISOString().slice(0, 10);
    const body = {
      effective_from: `${day}T00:00:00Z`,
      new: { seat_price_month: 350000 },
      reason: "e2e first schedule",
      confirm_large_change: false,
    };
    const first = await admin.post(`${CONSOLE_BASE}/api/platform/plans/business/price-changes`, { headers: ORIGIN, data: body });
    expect(first.status(), `scheduling the first change: ${await first.text()}`).toBeLessThan(300);
    try {
      const second = await admin.post(`${CONSOLE_BASE}/api/platform/plans/business/price-changes`, {
        headers: ORIGIN,
        data: { ...body, reason: "e2e second schedule" },
      });
      await expectCode(second, 409, "price_change_pending");
    } finally {
      const cancel = await admin.delete(`${CONSOLE_BASE}/api/platform/plans/business/price-changes/scheduled`, {
        headers: ORIGIN,
        data: { reason: "e2e cleanup" },
      });
      expect(cancel.status()).toBeLessThan(300);
    }
  });
});
