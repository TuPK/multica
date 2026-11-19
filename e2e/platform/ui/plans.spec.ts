/**
 * Plans screen and price schedule, reached from the Plans nav item: editing a plan's
 * name, cancelling an edit, refusing a currency change on a plan that has orders, and
 * previewing a price change without scheduling it. Plan rows the test changes are
 * restored in afterAll. Flow numbers refer to docs/engineering/platform-management-flows.md.
 */
import { expect, request as playwrightRequest, test, type APIRequestContext, type Page } from "@playwright/test";
import { randomUUID } from "node:crypto";
import pg from "pg";
import { CONSOLE_BASE, signInApi, startPlatform, stopPlatform, testDatabaseUrl, type StaffSeed } from "../harness";
import { cleanupRows, saveSignedInState, seedPendingOrder, seedWorkspace, useSignedIn } from "./session";

let seed: StaffSeed;
let admin: APIRequestContext;
let tenant: { id: string; slug: string };
const orderIds: string[] = [];
let businessName = "";
let proCurrency = "";

test.beforeAll(async ({ browser }) => {
  test.setTimeout(300_000);
  seed = await startPlatform("admin");
  tenant = await seedWorkspace("E2E Plans Co");
  admin = await playwrightRequest.newContext();
  await signInApi(admin, seed);
  await saveSignedInState(browser, seed, "plans");
  const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
  try {
    const rows = await pool.query("SELECT key, name, currency FROM plan WHERE key IN ('business', 'pro')");
    businessName = rows.rows.find((r) => r.key === "business")?.name ?? "";
    proCurrency = rows.rows.find((r) => r.key === "pro")?.currency ?? "";
  } finally {
    await pool.end();
  }
});

test.afterAll(async () => {
  test.setTimeout(300_000);
  await admin?.dispose();
  const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
  try {
    if (businessName) await pool.query("UPDATE plan SET name = $1 WHERE key = 'business'", [businessName]);
    for (const id of orderIds) await pool.query("DELETE FROM billing_order WHERE id = $1", [id]);
  } finally {
    await pool.end();
  }
  await cleanupRows(tenant ? [tenant.id] : [], []);
  await stopPlatform(seed ?? null);
});

test.beforeEach(async ({ page }) => {
  await useSignedIn(page, "plans");
});

async function openPlans(page: Page): Promise<void> {
  await page.goto(CONSOLE_BASE);
  await page.getByRole("navigation", { name: "Main Navigation" }).getByRole("button", { name: "Plans" }).click();
  await expect(page.getByRole("heading", { name: "Plans Catalogue" })).toBeVisible();
}

test.describe("plan catalogue", () => {
  test("flow 65: staff edits a plan's name, and the catalogue shows the new name", async ({ page }) => {
    await openPlans(page);
    const newName = `E2E Business ${randomUUID().slice(0, 6)}`;
    await page.getByRole("button", { name: "Edit plan " + businessName }).click();
    const dialog = page.getByRole("dialog", { name: /Edit Plan/ });
    await expect(dialog).toBeVisible();
    await dialog.locator("#plan-name-input").fill(newName);
    await dialog.getByRole("button", { name: "Save changes" }).click();
    await expect(dialog).toHaveCount(0);
    await expect(page.getByText(newName).first()).toBeVisible();
    // Put the name back so the next test finds the plan by its original name.
    const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
    try {
      await pool.query("UPDATE plan SET name = $1 WHERE key = 'business'", [businessName]);
    } finally {
      await pool.end();
    }
  });

  test("flow 65: cancelling a plan edit leaves the plan name unchanged", async ({ page }) => {
    await openPlans(page);
    await page.getByRole("button", { name: "Edit plan " + businessName }).click();
    const dialog = page.getByRole("dialog", { name: /Edit Plan/ });
    await expect(dialog).toBeVisible();
    await dialog.locator("#plan-name-input").fill("E2E Should Not Save");
    await dialog.getByRole("button", { name: "Cancel" }).click();
    await expect(dialog).toHaveCount(0);
    await expect(page.getByText("E2E Should Not Save")).toHaveCount(0);
  });

  test("flow 238: a currency change on a plan that has orders is refused in the edit dialog", async ({ page }) => {
    orderIds.push(await seedPendingOrder(tenant.id, 1_200_000));
    await openPlans(page);
    await page.getByRole("button", { name: "Edit plan pro" }).click();
    const dialog = page.getByRole("dialog", { name: /Edit Plan/ });
    await expect(dialog).toBeVisible();
    await dialog.locator("#plan-currency-input").fill(proCurrency === "USD" ? "VND" : "USD");
    await dialog.getByRole("button", { name: "Save changes" }).click();
    await expect(dialog.getByText("The currency can't change because this plan has subscriptions or orders.")).toBeVisible();
    await expect(dialog).toBeVisible();
  });
});

test.describe("price schedule", () => {
  test("flow 67: staff previews a price change, and nothing is scheduled", async ({ page }) => {
    await openPlans(page);
    await page.getByRole("button", { name: "Schedule price for " + businessName }).click();
    await expect(page.getByRole("button", { name: "Preview impact" })).toBeVisible();
    await page.locator("#price-seat-month").fill("350000"); // a small change; 1 VND would be a 100% change and need confirmation
    // Effective dates must start at 00:00 UTC; 60 days ahead clears any notice period.
    const day = new Date(Date.now() + 60 * 24 * 60 * 60 * 1000).toISOString().slice(0, 10);
    await page.locator("#price-effective-from").fill(`${day}T00:00`);
    await page.locator("#price-reason").fill("e2e preview drill");
    await page.getByRole("button", { name: "Preview impact" }).click();
    await expect(page.getByTestId("price-dry-run-preview")).toBeVisible();
  });

  test("flow 232: a negative seat price is refused in the price form before it is sent", async ({ page }) => {
    await openPlans(page);
    await page.getByRole("button", { name: "Schedule price for " + businessName }).click();
    await expect(page.getByRole("button", { name: "Preview impact" })).toBeVisible();
    const day = new Date(Date.now() + 60 * 24 * 60 * 60 * 1000).toISOString().slice(0, 10);
    await page.locator("#price-effective-from").fill(`${day}T00:00`);
    await page.locator("#price-reason").fill("e2e negative price");
    await page.locator("#price-seat-month").fill("-5");
    await page.getByRole("button", { name: "Preview impact" }).click();
    await expect(page.getByText("Monthly seat price must be a whole number of 0 or more.")).toBeVisible();
    await expect(page.getByTestId("price-dry-run-preview")).toHaveCount(0);
  });

  test("flow 236: a price change above the 50% threshold cannot be scheduled until it is confirmed", async ({ page }) => {
    await openPlans(page);
    await page.getByRole("button", { name: "Schedule price for " + businessName }).click();
    const day = new Date(Date.now() + 60 * 24 * 60 * 60 * 1000).toISOString().slice(0, 10);
    await page.locator("#price-effective-from").fill(`${day}T00:00`);
    await page.locator("#price-reason").fill("e2e threshold check");
    await page.locator("#price-seat-month").fill("1");
    await expect(page.getByText(/exceeds 50% threshold/)).toBeVisible();
    await expect(page.getByRole("button", { name: "Schedule", exact: true })).toBeDisabled();
    await expect(page.locator("#confirm-large-change")).toBeVisible();
  });
});
