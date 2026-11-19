/**
 * What a support member and a billing member can see and do in the console.
 * Each role runs its own platform server, seeded with that role, and signs in
 * once. Flow numbers refer to docs/engineering/platform-management-flows.md.
 */
import { expect, test, type Page } from "@playwright/test";
import { CONSOLE_BASE, startPlatform, stopPlatform, type StaffSeed } from "../harness";
import { cleanupRows, saveSignedInState, seedWorkspace, useSignedIn } from "./session";

async function navItems(page: Page): Promise<string[]> {
  const nav = page.getByRole("navigation", { name: "Main Navigation" });
  await expect(nav).toBeVisible();
  return (await nav.getByRole("button").allInnerTexts()).map((t) => t.trim());
}

test.describe("support member", () => {
  test.beforeEach(async ({ page }) => {
    await useSignedIn(page, "support");
  });
  let seed: StaffSeed;
  const workspaceIds: string[] = [];
  let workspaceName = "";

  test.beforeAll(async ({ browser }) => {
    seed = await startPlatform("support");
    const ws = await seedWorkspace("E2E Support Co");
    workspaceIds.push(ws.id);
    workspaceName = "E2E Support Co";
    await saveSignedInState(browser, seed, "support");
  });

  test.afterAll(async () => {
    await stopPlatform(seed ?? null);
    await cleanupRows(workspaceIds, []);
  });

  test("flow 146: support sees only the screens its role allows in the nav", async ({ page }) => {
    await page.goto(CONSOLE_BASE);
    const items = await navItems(page);
    expect(items).toEqual(expect.arrayContaining(["Overview", "Tenants", "Users", "Audit Log"]));
    for (const hidden of ["Orders", "Payment notices", "E-invoices", "Accounts", "Plans", "Staff"]) {
      expect(items).not.toContain(hidden);
    }
  });

  test("flow 27: support changes a workspace status, and Pending deletion is not offered", async ({ page }) => {
    await page.goto(CONSOLE_BASE);
    await page.getByRole("navigation", { name: "Main Navigation" }).getByRole("button", { name: "Tenants" }).click();
    await page.getByPlaceholder("Search by name or slug...").fill(workspaceName);
    await page.getByRole("row", { name: new RegExp(workspaceName) }).getByRole("button", { name: "View" }).click();
    await page.getByRole("button", { name: "Change status" }).click();
    const dialog = page.getByRole("dialog", { name: "Change workspace status" });
    await expect(dialog).toBeVisible();
    const options = await dialog.getByRole("combobox", { name: "Target status" }).locator("option").allInnerTexts();
    expect(options.map((o) => o.trim())).toEqual(["Active", "Read-only", "Suspended"]);
  });

  test("flow 146: a support member who types the staff screen path into the page still sees the Overview", async ({ page }) => {
    // The console has no URL routes; a stale or forced screen state must fall back to the Overview.
    await page.goto(`${CONSOLE_BASE}/staff`);
    await expect(page.getByText("Platform Overview").first()).toBeVisible();
    await expect(page.getByText("Staff Management")).toHaveCount(0);
  });
});

test.describe("billing member", () => {
  test.beforeEach(async ({ page }) => {
    await useSignedIn(page, "billing");
  });
  let seed: StaffSeed;

  test.beforeAll(async ({ browser }) => {
    seed = await startPlatform("billing");
    await saveSignedInState(browser, seed, "billing");
  });

  test.afterAll(async () => {
    await stopPlatform(seed ?? null);
  });

  test("flow 146: billing sees the billing screens but not Accounts, Plans or Staff", async ({ page }) => {
    await page.goto(CONSOLE_BASE);
    const items = await navItems(page);
    expect(items).toEqual(expect.arrayContaining(["Orders", "Payment notices", "E-invoices", "Audit Log"]));
    for (const hidden of ["Accounts", "Plans", "Staff"]) {
      expect(items).not.toContain(hidden);
    }
  });

  test("flow 61: billing opens the payment notice queue and sees either the queue or the empty state", async ({ page }) => {
    await page.goto(CONSOLE_BASE);
    await page.getByRole("navigation", { name: "Main Navigation" }).getByRole("button", { name: "Payment notices" }).click();
    await expect(page.getByRole("heading", { name: "Payment notices" })).toBeVisible();
    // The test database may hold notices from other flows, so accept either screen state.
    const queue = page.getByRole("columnheader", { name: "Transfer code" });
    const empty = page.getByText("No payment notices need review");
    await expect(queue.or(empty)).toBeVisible();
  });
});
