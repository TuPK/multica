/**
 * Platform staff sign-in and sign-out through the console, plus the console's
 * send-code and verify-code answers. Flow numbers refer to
 * docs/engineering/platform-management-flows.md.
 */
import { expect, test, type Page } from "@playwright/test";
import {
  API_BASE,
  CONSOLE_BASE,
  devVerificationCode,
  freshTotp,
  startPlatform,
  stopPlatform,
  type StaffSeed,
} from "./harness";

let seed: StaffSeed;

test.beforeAll(async () => {
  seed = await startPlatform("admin");
});

test.afterAll(async () => {
  await stopPlatform(seed ?? null);
});

/** Signs in through the console: email, email code, then the authenticator code. */
async function signInThroughConsole(page: Page): Promise<void> {
  await page.goto(CONSOLE_BASE);
  await page.locator('input[type="email"]').first().fill(seed.email);
  await page.getByRole("button", { name: "Send sign-in code" }).click();
  await page.locator("#code-input").fill(devVerificationCode());
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.locator('input[autocomplete="one-time-code"]').fill(await freshTotp(seed));
  await page.getByRole("button", { name: "Sign in" }).click();
}

test.describe("staff sign-in", () => {
  test("flow 12: staff signs in with the email code and the authenticator code", async ({ page }) => {
    test.setTimeout(120_000);
    await signInThroughConsole(page);
    await expect(page.getByRole("button", { name: /sign out/i })).toBeVisible();
    const me = await page.request.get(`${CONSOLE_BASE}/api/platform/me`);
    expect(me.status()).toBe(200);
    expect(JSON.stringify(await me.json())).toContain(seed.email);
  });

  test("flow 143: a wrong email code is rejected with invalid_code", async ({ request }) => {
    const origin = { Origin: CONSOLE_BASE };
    const send = await request.post(`${CONSOLE_BASE}/api/platform/auth/send-code`, { headers: origin, data: { email: seed.email } });
    expect(send.status()).toBe(204);
    const wrong = devVerificationCode() === "000000" ? "111111" : "000000";
    const verify = await request.post(`${CONSOLE_BASE}/api/platform/auth/verify-code`, { headers: origin, data: { email: seed.email, code: wrong } });
    expect(verify.status()).toBe(400);
    expect(await verify.text()).toContain("invalid_code");
  });

  test("flow 142: an unknown email gets a silent 204 on send-code", async ({ request }) => {
    const res = await request.post(`${CONSOLE_BASE}/api/platform/auth/send-code`, {
      headers: { Origin: CONSOLE_BASE },
      data: { email: `nobody-${Date.now()}@example.test` },
    });
    expect(res.status()).toBe(204);
  });

  test("flow 13: staff signs out and the console returns to Login", async ({ page }) => {
    test.setTimeout(120_000);
    await signInThroughConsole(page);
    await page.getByRole("button", { name: /sign out/i }).click();
    await expect(page.locator('input[type="email"]').first()).toBeVisible();
    const me = await page.request.get(`${CONSOLE_BASE}/api/platform/me`);
    // Flow 13 does not name the status; the staff session must no longer be accepted.
    expect(me.status()).not.toBe(200);
  });
});

test.describe("public boundary", () => {
  test("flow 15: /healthz answers on the API and the console", async ({ request }) => {
    expect((await request.get(`${API_BASE}/healthz`)).status()).toBe(200);
    expect((await request.get(`${CONSOLE_BASE}/healthz`)).status()).toBe(200);
  });
});
