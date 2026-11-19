/**
 * Refusals the console API gives for requests the screens do not send: an invalid
 * e-invoice state filter, an invalid plan key, an invalid tenant subscription, and
 * an e-invoice retry while the provider is not configured. The UI cannot send these
 * (the screens offer only valid values), so each test calls the API directly and
 * checks the refusal code. Flow numbers refer to docs/engineering/platform-management-flows.md.
 */
import { expect, request as playwrightRequest, test, type APIRequestContext, type APIResponse } from "@playwright/test";
import { randomUUID } from "node:crypto";
import pg from "pg";
import { CONSOLE_BASE, signInApi, startPlatform, stopPlatform, testDatabaseUrl, type StaffSeed } from "./harness";

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

  test("flow 144: a caller with an origin that is not the console is refused with bad_origin", async () => {
    const res = await admin.post(`${CONSOLE_BASE}/api/platform/auth/send-code`, {
      headers: { Origin: "https://evil.example", "Content-Type": "application/json" },
      data: { email: seed.email },
    });
    await expectCode(res, 403, "bad_origin");
  });

  test("flow 145: a path that is not under /api/platform/ is answered by the console proxy as not found", async () => {
    const res = await admin.get(`${CONSOLE_BASE}/api/other/anything`);
    expect(res.status()).toBe(404);
  });

  test("flow 145: a dot-segment path under /api/platform/ never reaches the API and is refused", async () => {
    const res = await admin.get(`${CONSOLE_BASE}/api/platform/%2e%2e/healthz`);
    expect([400, 404]).toContain(res.status());
  });

  test("flow 223: confirming an add-seats order when the workspace has no subscription is refused with order_not_applicable", async () => {
    const id = randomUUID();
    const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
    try {
      await pool.query(
        `INSERT INTO billing_order (id, workspace_id, kind, plan_key, interval, seats, amount, vat_amount, currency,
                                    status, provider, transfer_code, buyer, expires_at, created_at, period_start, period_end)
         VALUES ($1, $2, 'add_seats', 'pro', 'month', 1, 100000, 0, 'VND', 'pending', 'sepay', $3, '{}',
                 now() + interval '7 days', now(), now(), now() + interval '1 month')`,
        [id, randomUUID(), `E2E${id.replace(/-/g, "").slice(0, 12).toUpperCase()}`],
      );
    } finally {
      await pool.end();
    }
    try {
      const res = await admin.post(`${CONSOLE_BASE}/api/platform/orders/${id}/confirm`, {
        headers: ORIGIN,
        data: { reason: "e2e no subscription", transfer_reference: "E2E-NOSUB" },
      });
      await expectCode(res, 409, "order_not_applicable");
    } finally {
      const cleanup = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
      try {
        await cleanup.query("DELETE FROM billing_order WHERE id = $1", [id]);
      } finally {
        await cleanup.end();
      }
    }
  });
});

test.describe("plan and subscription refusals on a copy of the free plan", () => {
  const PLAN = "e2e_copy";
  const WS_IN_USE = randomUUID();
  const WS_OVERRIDE = randomUUID();
  const WS_PAID = randomUUID();

  test.beforeAll(async () => {
    const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
    try {
      // Copy the free plan under a test key, so no real plan's prices or archive state change.
      const cols = (await pool.query(
        "SELECT column_name FROM information_schema.columns WHERE table_name = 'plan' AND table_schema = current_schema() ORDER BY ordinal_position",
      )).rows.map((r) => r.column_name as string);
      const select = cols.map((c) => (c === "key" ? `'${PLAN}'` : c === "name" ? `'E2E Copy'` : `"${c}"`)).join(", ");
      await pool.query(`DELETE FROM plan WHERE key = '${PLAN}'`);
      await pool.query(`INSERT INTO plan (${cols.map((c) => `"${c}"`).join(", ")}) SELECT ${select} FROM plan WHERE key = 'free'`);
      // Active subscriptions on the copy: one to block deletion, one paid subscriber for the notice rule.
      for (const ws of [WS_IN_USE, WS_PAID]) {
        await pool.query(
          `INSERT INTO workspace_subscription (workspace_id, plan_key, status, seats, interval, current_period_start, current_period_end)
           VALUES ($1, $2, 'active', 1, 'month', now(), now() + interval '1 month')`,
          [ws, PLAN],
        );
      }
    } finally {
      await pool.end();
    }
  });

  test.afterAll(async () => {
    const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
    try {
      await pool.query("DELETE FROM workspace_subscription WHERE workspace_id = ANY($1)", [[WS_IN_USE, WS_OVERRIDE, WS_PAID]]);
      await pool.query("DELETE FROM workspace WHERE id = $1", [WS_OVERRIDE]);
      await pool.query(`DELETE FROM plan WHERE key = '${PLAN}'`);
    } finally {
      await pool.end();
    }
  });

  test("flow 240: deleting a plan that a subscription still uses is refused with plan_in_use", async () => {
    const res = await admin.delete(`${CONSOLE_BASE}/api/platform/plans/${PLAN}`, { headers: ORIGIN });
    await expectCode(res, 409, "plan_in_use");
  });

  test("flow 241: a tenant subscription override to an archived plan is refused with plan_archived", async () => {
    const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
    try {
      await pool.query(`UPDATE plan SET archived_at = now() WHERE key = '${PLAN}'`);
      await pool.query("INSERT INTO workspace (id, name, slug) VALUES ($1, 'E2E Override Co', $2) ON CONFLICT DO NOTHING", [
        WS_OVERRIDE,
        `e2e-ov-${WS_OVERRIDE.slice(0, 8)}`,
      ]);
      const res = await admin.put(`${CONSOLE_BASE}/api/platform/tenants/${WS_OVERRIDE}/subscription`, {
        headers: ORIGIN,
        data: { plan_key: PLAN, seats: 1, interval: "month", reason: "e2e archived plan" },
      });
      await expectCode(res, 400, "plan_archived");
    } finally {
      await pool.query(`UPDATE plan SET archived_at = NULL WHERE key = '${PLAN}'`);
      await pool.end();
    }
  });

  test("flow 237: a price increase on a plan with paid subscribers, effective before the notice period, is refused with price_notice_too_short", async () => {
    // Tomorrow at 00:00 UTC: inside the notice period, so a price increase for paid subscribers is refused.
    const tomorrow = new Date(Date.now() + 24 * 60 * 60 * 1000).toISOString().slice(0, 10);
    const day = `${tomorrow}T00:00:00Z`;
    const res = await admin.post(`${CONSOLE_BASE}/api/platform/plans/${PLAN}/price-changes`, {
      headers: ORIGIN,
      data: { effective_from: day, new: { seat_price_month: 1000, seat_price_year: 10000 }, reason: "e2e notice", confirm_large_change: true },
    });
    expect(res.status(), `price change answered: ${await res.text()}`).toBe(409);
    expect((await res.json()).code).toBe("price_notice_too_short");
  });
});
