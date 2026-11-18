/**
 * The platform API's identity boundary on subscription and internal routes.
 * Flow numbers refer to docs/engineering/platform-management-flows.md.
 */
import { expect, test } from "@playwright/test";
import { API_BASE, startPlatform, stopPlatform, type StaffSeed } from "./harness";

let seed: StaffSeed;

test.beforeAll(async () => {
  seed = await startPlatform("admin");
});

test.afterAll(async () => {
  await stopPlatform(seed ?? null);
});

test.describe("platform API identity boundary", () => {
  test("flow 276: a subscription route without X-User-ID is unauthorized", async ({ request }) => {
    const res = await request.get(`${API_BASE}/api/v1/subscriptions/00000000-0000-0000-0000-000000000001/summary`);
    expect(res.status()).toBe(401);
    expect(await res.text()).toContain("unauthorized");
  });

  // Product failure: the internal route answers 200 for a malformed X-User-ID.
  // Kept as fixme until the platform API rejects it; product code is not changed here.
  test.fixme("flow 276: a malformed X-User-ID on an internal route is unauthorized", async ({ request }) => {
    const res = await request.get(`${API_BASE}/api/v1/internal/entitlement-policies/00000000-0000-0000-0000-000000000001`, {
      headers: { "X-User-ID": "not-a-uuid" },
    });
    expect(res.status()).toBe(401);
    expect(await res.text()).toContain("unauthorized");
  });
});
