/**
 * Plan catalogue writes through the API: creating a plan (flow 66), refusing a plan
 * update whose expected_version is stale (flow 239), and refusing a duplicate key.
 * The screens offer no create form, so these calls go to the API directly. Each plan
 * the tests create is removed in afterAll. Flow numbers refer to
 * docs/engineering/platform-management-flows.md.
 */
import { expect, request as playwrightRequest, test, type APIRequestContext } from "@playwright/test";
import pg from "pg";
import { CONSOLE_BASE, signInApi, startPlatform, stopPlatform, testDatabaseUrl, type StaffSeed } from "./harness";

const ORIGIN = { Origin: CONSOLE_BASE, "Content-Type": "application/json" };
const KEY = "e2e_create";

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
  const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
  try {
    await pool.query("DELETE FROM plan WHERE key = $1", [KEY]);
  } finally {
    await pool.end();
  }
  await stopPlatform(seed ?? null);
});

/** The create body. Prices may be set here; the PUT body may not carry them (they change through the schedule). */
function createBody(name: string): Record<string, unknown> {
  return {
    key: KEY,
    name,
    currency: "vnd",
    seat_price_month: 0,
    seat_price_year: 0,
    limits: {},
    features: {},
    trial_days: 0,
    is_public: false,
    sort: 900,
    audience: "any",
    billing_model: "seat",
  };
}

function updateBody(name: string): Record<string, unknown> {
  return {
    name,
    currency: "vnd",
    limits: {},
    features: {},
    trial_days: 0,
    is_public: false,
    sort: 900,
    archived: false,
  };
}

async function versionOf(key: string): Promise<number> {
  const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
  try {
    const row = await pool.query("SELECT version FROM plan WHERE key = $1", [key]);
    return Number(row.rows[0]?.version ?? -1);
  } finally {
    await pool.end();
  }
}

test.describe("plan catalogue writes", () => {
  test("flow 66: an admin creates a private plan with POST and it is stored under its key", async () => {
    const res = await admin.post(`${CONSOLE_BASE}/api/platform/plans`, { headers: ORIGIN, data: createBody("E2E Create") });
    expect(res.status(), `create answered: ${await res.text()}`).toBe(201);
    expect(await versionOf(KEY)).toBeGreaterThanOrEqual(0);
  });

  test("flow 66: creating a plan with a key that exists is refused with plan_exists", async () => {
    const res = await admin.post(`${CONSOLE_BASE}/api/platform/plans`, { headers: ORIGIN, data: createBody("E2E Again") });
    expect(res.status()).toBe(409);
    expect((await res.json()).code).toBe("plan_exists");
  });

  test("flow 239: a plan update with a stale expected_version is refused with plan_version_conflict", async () => {
    const current = await versionOf(KEY);
    const res = await admin.put(`${CONSOLE_BASE}/api/platform/plans/${KEY}`, {
      headers: ORIGIN,
      data: { ...updateBody("E2E Stale"), expected_version: current + 100 },
    });
    expect(res.status()).toBe(409);
    expect((await res.json()).code).toBe("plan_version_conflict");
  });

  test("flow 239: a plan update with the current expected_version is accepted", async () => {
    const current = await versionOf(KEY);
    const res = await admin.put(`${CONSOLE_BASE}/api/platform/plans/${KEY}`, {
      headers: ORIGIN,
      data: { ...updateBody("E2E Current"), expected_version: current },
    });
    expect(res.status(), `update answered: ${await res.text()}`).toBeLessThan(300);
  });
});
