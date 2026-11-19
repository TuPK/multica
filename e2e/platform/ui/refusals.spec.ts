/**
 * Refusals the console shows in its own dialogs and screens: a billing profile
 * that cannot be verified, a malformed staff email, and a payment notice that
 * was closed by someone else while this screen still shows it. Each test acts
 * through the screen and reads the message the user sees.
 * Flow numbers refer to docs/engineering/platform-management-flows.md.
 */
import { expect, request as playwrightRequest, test, type APIRequestContext, type Page } from "@playwright/test";
import { randomUUID } from "node:crypto";
import pg from "pg";
import { CONSOLE_BASE, serverOutput, signInApi, startPlatform, stopPlatform, testDatabaseUrl, type StaffSeed } from "../harness";
import { cleanupRows, queueReviewNotice, saveSignedInState, seedWorkspace, useSignedIn } from "./session";

let seed: StaffSeed;
let admin: APIRequestContext;
let tenant: { id: string; slug: string };
const tenantName = "E2E Refusals Co";
const noticeEventIds: string[] = [];

test.beforeAll(async ({ browser }) => {
  test.setTimeout(300_000);
  seed = await startPlatform("admin");
  tenant = await seedWorkspace(tenantName);
  admin = await playwrightRequest.newContext();
  await signInApi(admin, seed);
  await saveSignedInState(browser, seed, "refusals");
});

test.afterAll(async () => {
  test.setTimeout(300_000);
  await admin?.dispose();
  const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
  try {
    if (noticeEventIds.length > 0) {
      await pool.query("DELETE FROM payment_notice WHERE provider = 'sepay' AND provider_event_id = ANY($1)", [noticeEventIds]);
    }
  } finally {
    await pool.end();
  }
  await cleanupRows(tenant ? [tenant.id] : [], []);
  await stopPlatform(seed ?? null);
});

test.beforeEach(async ({ page }) => {
  await useSignedIn(page, "refusals");
});

async function openNav(page: Page, name: string): Promise<void> {
  await page.goto(CONSOLE_BASE);
  await page.getByRole("navigation", { name: "Main Navigation" }).getByRole("button", { name }).click();
}

test.describe("billing profile", () => {
  test("flow 190: verifying a tenant whose billing profile is incomplete is refused and names the missing fields", async ({ page }) => {
    await openNav(page, "Tenants");
    await page.getByPlaceholder("Search by name or slug...").fill(tenantName);
    await page.getByRole("row", { name: new RegExp(tenantName) }).getByRole("button", { name: "View" }).click();
    await expect(page.getByRole("button", { name: "Back to tenants" })).toBeVisible();

    await page.getByRole("button", { name: "Verify", exact: true }).click();
    await expect(page.getByText(/Missing billing profile fields:/)).toBeVisible();
    await expect(page.getByRole("button", { name: "Verify", exact: true })).toBeVisible();
  });
});

test.describe("staff", () => {
  test("flow 259: the add-staff form stops a malformed email before it is sent, and the dialog stays open", async ({ page }) => {
    await openNav(page, "Staff");
    await page.getByRole("button", { name: "Add staff" }).click();
    const dialog = page.getByRole("dialog", { name: "Add staff member" });
    await expect(dialog).toBeVisible();
    const input = dialog.locator("#staff-email-input");
    await input.fill("not-an-email");
    await dialog.getByRole("button", { name: "Add staff", exact: true }).click();
    // The email field is type=email, so the browser refuses the submit; no request reaches the server.
    expect(await input.evaluate((el) => (el as HTMLInputElement).validity.valid)).toBe(false);
    await expect(dialog).toBeVisible();
    await expect(dialog.getByRole("alert")).toHaveCount(0);
  });
});

test.describe("accounts", () => {
  test("flow 188: a plan that is not sold for the account kind is refused with the field error", async ({ page }) => {
    // Every seeded plan is sold to any kind; mark pro as business-only for this test, then restore it.
    const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
    try {
      await pool.query("UPDATE plan SET audience = 'business' WHERE key = 'pro'");
      await openNav(page, "Accounts");
      await page.getByRole("button", { name: "Create account" }).click();
      const dialog = page.getByRole("dialog", { name: "Create account" });
      await expect(dialog).toBeVisible();
      await dialog.locator("#account-kind").selectOption("individual");
      await dialog.locator("#account-plan-key").selectOption("pro");
      await dialog.locator("#account-comp-reason").fill("e2e audience check");
      const email = `e2e-aud-${randomUUID().slice(0, 8)}@example.test`;
      await dialog.locator("#account-email").fill(email);
      await dialog.locator("#account-confirm-email").fill(email);
      await dialog.locator("#account-display-name").fill("E2E Audience Person");
      await dialog.locator("#account-workspace-name").fill("E2E Audience Co");
      await dialog.getByRole("button", { name: "Submit request" }).click();
      await expect(dialog.getByText("Not available for this account kind.")).toBeVisible();
      await expect(dialog).toBeVisible();
    } finally {
      await pool.query("UPDATE plan SET audience = 'any' WHERE key = 'pro'");
      await pool.end();
    }
  });
});

test.describe("payment notices", () => {
  test("flow 216: dismissing a notice that someone else already closed shows the closed message", async ({ page }) => {
    const amount = 1_000_000 + Math.floor(Math.random() * 900_000);
    const amountText = amount.toLocaleString();
    await queueReviewNotice(amount, noticeEventIds);

    // The notice is open and its dismiss dialog is open on this screen; then another staff member closes it.
    await openNav(page, "Payment notices");
    await page.getByRole("row", { name: new RegExp(`${amountText} VND`) }).getByRole("button", { name: "View" }).click();
    await expect(page.getByText("Notice details")).toBeVisible();
    await page.getByRole("button", { name: "Dismiss", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "Dismiss payment notice" });
    await expect(dialog).toBeVisible();

    const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
    let noticeId = "";
    try {
      const row = await pool.query("SELECT id FROM payment_notice WHERE provider = 'sepay' AND provider_event_id = ANY($1)", [noticeEventIds]);
      noticeId = row.rows[0]?.id ?? "";
    } finally {
      await pool.end();
    }
    expect(noticeId, `notice row not found; server log tail:\n${serverOutput().slice(-1000)}`).not.toBe("");
    const closed = await admin.post(`${CONSOLE_BASE}/api/platform/payment-notices/${noticeId}/dismiss`, {
      headers: { Origin: CONSOLE_BASE, "Content-Type": "application/json" },
      data: { reason: "e2e closed elsewhere" },
    });
    expect(closed.status()).toBe(200);

    await dialog.locator("#confirm-reason-input").fill("e2e stale view");
    await dialog.getByRole("button", { name: "Dismiss notice" }).click();
    await expect(dialog.getByRole("alert").filter({ hasText: "no longer waiting for review" })).toBeVisible();
  });
});

// Not covered here: flow 217 (a concurrent state change needs two staff acting in the same instant) and
// flow 221 (the candidate list does not offer an expired order, so the screen never sends it). Both are
// refused by the API in console-orders.spec.ts and the Go tests.
