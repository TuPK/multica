/**
 * Usage debt on a tenant: an admin writes off an unpaid usage invoice from the
 * tenant detail. The invoice is seeded in SQL; the write-off is done through the
 * dialog. Flow numbers refer to docs/engineering/platform-management-flows.md.
 */
import { expect, request as playwrightRequest, test, type APIRequestContext, type Page } from "@playwright/test";
import { randomUUID } from "node:crypto";
import pg from "pg";
import { CONSOLE_BASE, signInApi, startPlatform, stopPlatform, testDatabaseUrl, type StaffSeed } from "../harness";
import { cleanupRows, saveSignedInState, seedWorkspace, useSignedIn } from "./session";

let seed: StaffSeed;
let admin: APIRequestContext;
const tenantName = "E2E Usage Co";
let tenant: { id: string; slug: string };
const orderIds: string[] = [];

test.beforeAll(async ({ browser }) => {
  test.setTimeout(300_000);
  seed = await startPlatform("admin");
  tenant = await seedWorkspace(tenantName);
  admin = await playwrightRequest.newContext();
  await signInApi(admin, seed);
  await saveSignedInState(browser, seed, "usage");
});

test.afterAll(async () => {
  test.setTimeout(300_000);
  await admin?.dispose();
  const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
  try {
    for (const id of orderIds) await pool.query("DELETE FROM billing_order WHERE id = $1", [id]);
  } finally {
    await pool.end();
  }
  await cleanupRows(tenant ? [tenant.id] : [], []);
  await stopPlatform(seed ?? null);
});

test.beforeEach(async ({ page }) => {
  await useSignedIn(page, "usage");
});

/** Inserts one pending usage invoice for the tenant, dated in the current month. Returns its id. */
async function seedUsageOrder(amount: number): Promise<string> {
  const id = randomUUID();
  const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
  try {
    await pool.query(
      `INSERT INTO billing_order (id, workspace_id, kind, plan_key, interval, seats, amount, vat_amount, currency,
                                  status, provider, transfer_code, buyer, expires_at, created_at,
                                  period_start, period_end)
       VALUES ($1, $2, 'usage', 'pro', 'month', 1, $3, 0, 'VND',
               'pending', 'sepay', $4, '{}', now() + interval '30 days', now(),
               date_trunc('month', now()), date_trunc('month', now()) + interval '1 month')`,
      [id, tenant.id, amount, `E2E${id.replace(/-/g, "").slice(0, 12).toUpperCase()}`],
    );
  } finally {
    await pool.end();
  }
  orderIds.push(id);
  return id;
}

async function openTenant(page: Page): Promise<void> {
  await page.goto(CONSOLE_BASE);
  await page.getByRole("navigation", { name: "Main Navigation" }).getByRole("button", { name: "Tenants" }).click();
  await page.getByPlaceholder("Search by name or slug...").fill(tenantName);
  await page.getByRole("row", { name: new RegExp(tenantName) }).getByRole("button", { name: "View" }).click();
  await expect(page.getByRole("button", { name: "Back to tenants" })).toBeVisible();
}

test.describe("usage debt", () => {
  test("flow 225: an admin writes off an unpaid usage order and it leaves the unpaid list", async ({ page }) => {
    const amount = 300_000 + Math.floor(Math.random() * 500_000);
    const orderId = await seedUsageOrder(amount);

    await openTenant(page);
    const unpaid = page.getByTestId("usage-unpaid-orders");
    await expect(unpaid).toContainText(orderId);

    await unpaid.getByRole("row", { name: new RegExp(orderId) }).getByRole("button", { name: "Write off" }).click();
    const dialog = page.getByRole("dialog", { name: "Write off usage order" });
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText(orderId);
    await dialog.locator("#confirm-reason-input").fill("e2e write-off drill");
    await dialog.getByRole("button", { name: "Write off order" }).click();

    await expect(dialog).toHaveCount(0);
    await expect(page.getByTestId("usage-unpaid-orders")).not.toContainText(orderId);
  });

  test("flow 64: the write-off dialog refuses a blank reason", async ({ page }) => {
    const orderId = await seedUsageOrder(250_000);

    await openTenant(page);
    await page.getByTestId("usage-unpaid-orders").getByRole("row", { name: new RegExp(orderId) }).getByRole("button", { name: "Write off" }).click();
    const dialog = page.getByRole("dialog", { name: "Write off usage order" });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByRole("button", { name: "Write off order" })).toBeDisabled();
    await dialog.locator("#confirm-reason-input").fill("   ");
    await expect(dialog.getByRole("button", { name: "Write off order" })).toBeDisabled();
    await dialog.getByRole("button", { name: "Cancel" }).click();
    await expect(dialog).toHaveCount(0);
    await expect(page.getByTestId("usage-unpaid-orders")).toContainText(orderId);
  });
});
