/**
 * E-invoice retry refusals that need a configured provider: a failed invoice whose
 * buyer lacks required fields (flow 229), and a retry on an invoice that is already
 * issued (flow 230). The server runs with the SePay provider configured against an
 * unreachable base URL; these paths refuse before any provider call, so none is made.
 * Flow numbers refer to docs/engineering/platform-management-flows.md.
 */
import { expect, request as playwrightRequest, test, type APIRequestContext } from "@playwright/test";
import { randomUUID } from "node:crypto";
import pg from "pg";
import { CONSOLE_BASE, signInApi, startPlatform, stopPlatform, testDatabaseUrl, type StaffSeed } from "./harness";

const ORIGIN = { Origin: CONSOLE_BASE, "Content-Type": "application/json" };

let seed: StaffSeed;
let admin: APIRequestContext;
const orderIds: string[] = [];

test.beforeAll(async () => {
  test.setTimeout(300_000);
  seed = await startPlatform("admin", { einvoiceSepay: true });
  admin = await playwrightRequest.newContext();
  await signInApi(admin, seed);
});

test.afterAll(async () => {
  test.setTimeout(300_000);
  await admin?.dispose();
  const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
  try {
    if (orderIds.length > 0) await pool.query("DELETE FROM einvoice_issuance WHERE order_id = ANY($1)", [orderIds]);
  } finally {
    await pool.end();
  }
  await stopPlatform(seed ?? null);
});

/** Inserts one e-invoice revision in the given state. The schema has no foreign keys, so the ids are fixtures. */
async function seedInvoice(state: string, buyer: object): Promise<string> {
  const orderId = randomUUID();
  const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
  try {
    await pool.query(
      `INSERT INTO einvoice_issuance (order_id, revision, workspace_id, state, provider, currency, amount, buyer)
       VALUES ($1, 1, $2, $3, 'sepay', 'VND', 100000, $4)`,
      [orderId, randomUUID(), state, JSON.stringify(buyer)],
    );
  } finally {
    await pool.end();
  }
  orderIds.push(orderId);
  return orderId;
}

async function stateOf(orderId: string): Promise<string> {
  const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
  try {
    const row = await pool.query("SELECT state FROM einvoice_issuance WHERE order_id = $1", [orderId]);
    return row.rows[0]?.state ?? "";
  } finally {
    await pool.end();
  }
}

// Product failure: cmd/platform-server/main.go builds the e-invoice service but never passes it to
// console.Deps (no Einvoices field), so the retry route answers 503 einvoice_provider_not_configured even
// with the provider configured. Observed: 503 for both tests below. Both stay fixme until that is wired.
test.describe("e-invoice retry refusals", () => {
  test.fixme("flow 229: retrying a failed invoice whose buyer lacks required fields is refused and names the fields", async () => {
    const orderId = await seedInvoice("failed", {});
    const res = await admin.post(`${CONSOLE_BASE}/api/platform/einvoices/${orderId}/retry`, {
      headers: ORIGIN,
      data: { reason: "e2e buyer incomplete" },
    });
    expect(res.status()).toBe(409);
    const body = (await res.json()) as { code: string; fields: string[] };
    expect(body.code).toBe("einvoice_buyer_incomplete");
    expect(body.fields).toEqual(expect.arrayContaining(["name", "tax_code", "address", "email"]));
    expect(await stateOf(orderId)).toBe("failed");
  });

  test.fixme("flow 230: retrying an invoice that is already issued is refused with einvoice_state_conflict", async () => {
    const orderId = await seedInvoice("issued", { name: "E2E Buyer", tax_code: "0123456789", address: "1 E2E St", email: "buyer@example.test" });
    const res = await admin.post(`${CONSOLE_BASE}/api/platform/einvoices/${orderId}/retry`, {
      headers: ORIGIN,
      data: { reason: "e2e state conflict" },
    });
    expect(res.status()).toBe(409);
    expect((await res.json()).code).toBe("einvoice_state_conflict");
    expect(await stateOf(orderId)).toBe("issued");
  });
});
