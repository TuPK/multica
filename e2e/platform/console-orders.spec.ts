/**
 * Staff actions on billing orders: cancel a pending order, and the refusals
 * for a canceled order and an expired order. Each test seeds its own order row
 * in the test database, so no real payment provider is involved. Flow numbers
 * refer to docs/engineering/platform-management-flows.md.
 */
import { expect, request as playwrightRequest, test, type APIRequestContext } from "@playwright/test";
import { randomUUID } from "node:crypto";
import pg from "pg";
import { CONSOLE_BASE, signInApi, startPlatform, stopPlatform, testDatabaseUrl, type StaffSeed } from "./harness";

const ORIGIN = { Origin: CONSOLE_BASE, "Content-Type": "application/json" };

let seed: StaffSeed;
let admin: APIRequestContext;
const orders: string[] = [];

test.beforeAll(async () => {
  seed = await startPlatform("admin");
  admin = await playwrightRequest.newContext();
  await signInApi(admin, seed);
});

test.afterAll(async () => {
  await admin?.dispose();
  const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
  try {
    for (const id of orders) await pool.query("DELETE FROM billing_order WHERE id = $1", [id]);
  } finally {
    await pool.end();
  }
  await stopPlatform(seed ?? null);
});

/** Inserts one pending or canceled order. The workspace id is a fixture id; the schema has no foreign keys. */
async function seedOrder(status: "pending" | "canceled", expiresAt: Date): Promise<string> {
  const id = randomUUID();
  const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
  try {
    await pool.query(
      `INSERT INTO billing_order (id, workspace_id, kind, plan_key, interval, seats, amount, vat_amount, currency,
                                  status, provider, transfer_code, buyer, expires_at, created_at)
       VALUES ($1, $2, 'subscribe', 'e2e-plan', 'month', 1, 100000, 0, 'VND',
               $3, 'sepay', $4, '{}', $5, now())`,
      [id, randomUUID(), status, `E2E${id.replace(/-/g, "").slice(0, 12).toUpperCase()}`, expiresAt],
    );
  } finally {
    await pool.end();
  }
  orders.push(id);
  return id;
}

const DAY_MS = 24 * 60 * 60 * 1000;

test.describe("order actions", () => {
  test("flow 62: staff cancels a pending order and it is no longer pending", async () => {
    const id = await seedOrder("pending", new Date(Date.now() + 7 * DAY_MS));
    const res = await admin.post(`${CONSOLE_BASE}/api/platform/orders/${id}/cancel`, {
      headers: ORIGIN,
      data: { reason: "e2e cancel" },
    });
    expect(res.status()).toBe(200);
    expect(JSON.stringify(await res.json())).toContain("canceled");
  });

  test("flow 220: a canceled order cannot be canceled or confirmed again", async () => {
    const id = await seedOrder("canceled", new Date(Date.now() + 7 * DAY_MS));
    const cancel = await admin.post(`${CONSOLE_BASE}/api/platform/orders/${id}/cancel`, {
      headers: ORIGIN,
      data: { reason: "e2e again" },
    });
    expect(cancel.status()).toBe(409);
    expect((await cancel.json()).code).toBe("order_not_pending");

    const confirm = await admin.post(`${CONSOLE_BASE}/api/platform/orders/${id}/confirm`, {
      headers: ORIGIN,
      data: { reason: "e2e confirm", transfer_reference: "E2E-REF-2" },
    });
    expect(confirm.status()).toBe(409);
    expect((await confirm.json()).code).toBe("order_not_pending");
  });

  test("flow 221: an order past its expiry cannot be confirmed", async () => {
    const id = await seedOrder("pending", new Date(Date.now() - DAY_MS));
    const confirm = await admin.post(`${CONSOLE_BASE}/api/platform/orders/${id}/confirm`, {
      headers: ORIGIN,
      data: { reason: "e2e late", transfer_reference: "E2E-REF-3" },
    });
    expect(confirm.status()).toBe(409);
    expect((await confirm.json()).code).toBe("order_expired");
  });
});
