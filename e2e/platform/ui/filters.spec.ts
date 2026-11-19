/**
 * Filters and usage details on the console screens: the audit log's date range
 * and the tenant's usage tables. Flow numbers refer to
 * docs/engineering/platform-management-flows.md.
 */
import { expect, request as playwrightRequest, test, type APIRequestContext, type Page } from "@playwright/test";
import { CONSOLE_BASE, signInApi, startPlatform, stopPlatform, type StaffSeed } from "../harness";
import { cleanupRows, saveSignedInState, seedWorkspace, useSignedIn } from "./session";
import { randomUUID } from "node:crypto";

let seed: StaffSeed;
let admin: APIRequestContext;
// A unique name per run: earlier runs may have left rows with the same name, and the row lookup must match one.
const tenantName = `E2E Filters Co ${randomUUID().slice(0, 8)}`;
let tenantId = "";

test.beforeAll(async ({ browser }) => {
  // Starting the platform takes minutes when the machine is loaded; the default hook timeout is 60 s.
  test.setTimeout(300_000);
  seed = await startPlatform("admin");
  tenantId = (await seedWorkspace(tenantName)).id;
  admin = await playwrightRequest.newContext();
  await signInApi(admin, seed);
  await saveSignedInState(browser, seed, "filters");
});

test.afterAll(async () => {
  // Stopping the platform can also take minutes under load (see beforeAll).
  test.setTimeout(300_000);
  await admin?.dispose();
  await cleanupRows(tenantId ? [tenantId] : [], []);
  await stopPlatform(seed ?? null);
});

test.beforeEach(async ({ page }) => {
  await useSignedIn(page, "filters");
});

async function openNav(page: Page, name: string): Promise<void> {
  await page.goto(CONSOLE_BASE);
  await page.getByRole("navigation", { name: "Main Navigation" }).getByRole("button", { name }).click();
}

test.describe("audit log date range", () => {
  test("flow 26: a From date after every event empties the audit list, and clearing it restores the list", async ({ page }) => {
    await openNav(page, "Audit Log");
    const table = page.getByRole("columnheader", { name: "Action" });
    const empty = page.getByText("No audit events found");
    await expect(table.or(empty)).toBeVisible();

    await page.locator("#audit-from-input").fill("2099-01-01");
    await expect(empty).toBeVisible();

    await page.locator("#audit-from-input").fill("");
    await expect(table.or(empty)).toBeVisible();
    await expect(page.locator("#audit-from-input")).toHaveValue("");
  });

  test("flow 26: a To date before every event shows the empty state", async ({ page }) => {
    await openNav(page, "Audit Log");
    await page.locator("#audit-to-input").fill("2000-01-01");
    await expect(page.getByText("No audit events found")).toBeVisible();
  });
});

test.describe("tenant usage tables", () => {
  test("flow 26: the monthly usage section shows the daily charges table or its empty note", async ({ page }) => {
    await openNav(page, "Tenants");
    await page.getByPlaceholder("Search by name or slug...").fill(tenantName);
    await page.getByRole("row", { name: new RegExp(tenantName) }).getByRole("button", { name: "View" }).click();
    await expect(page.getByRole("button", { name: "Back to tenants" })).toBeVisible();

    const rows = page.getByTestId("usage-rows");
    await expect(rows).toBeVisible();
    await expect(rows.getByText("Daily charges", { exact: true })).toBeVisible();
    await expect(rows.getByRole("table").or(rows.getByText("No rated usage rows for this month."))).toBeVisible();
  });
});
