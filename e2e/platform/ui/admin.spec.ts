/**
 * An admin works the console through the screens: tenants, users, staff and the
 * audit log. The session is saved once in beforeAll; every action below is a
 * click, a keystroke or a dialog answer. Flow numbers refer to
 * docs/engineering/platform-management-flows.md.
 */
import { expect, test, type Page } from "@playwright/test";
import { CONSOLE_BASE, startPlatform, stopPlatform, type StaffSeed } from "../harness";
import { cleanupRows, saveSignedInState, seedAccount, seedWorkspace, useSignedIn } from "./session";

test.beforeEach(async ({ page }) => {
  await useSignedIn(page, "admin");
});

let seed: StaffSeed;
const alpha = { name: "E2E Alpha Co", id: "", slug: "" };
const beta = { name: "E2E Beta Co", id: "", slug: "" };
let target: { id: string; email: string };
const createdWorkspaces: string[] = [];
const createdUsers: string[] = [];

test.beforeAll(async ({ browser }) => {
  // The platform server build and start can exceed the 60 s default on a loaded machine.
  test.setTimeout(300_000);
  seed = await startPlatform("admin");
  for (const ws of [alpha, beta]) {
    const row = await seedWorkspace(ws.name);
    ws.id = row.id;
    ws.slug = row.slug;
    createdWorkspaces.push(row.id);
  }
  target = await seedAccount("E2E Target Person");
  createdUsers.push(target.id);
  await saveSignedInState(browser, seed, "admin");
});

test.afterAll(async () => {
  test.setTimeout(300_000);
  await stopPlatform(seed ?? null);
  await cleanupRows(createdWorkspaces, createdUsers);
});

async function openScreen(page: Page, name: string): Promise<void> {
  await page.goto(CONSOLE_BASE);
  await page.getByRole("navigation", { name: "Main Navigation" }).getByRole("button", { name }).click();
}

test.describe("navigation", () => {
  // Product failure: Orders.tsx and Plans.tsx exist, but App.tsx has no route for them, so the nav
  // item shows the Overview instead of its screen. Observed: the Overview heading after each click.
  test.fixme("flow 83: Orders and Plans in the nav open their own screens", async ({ page }) => {
    await page.goto(CONSOLE_BASE);
    await openScreen(page, "Orders");
    await expect(page.getByRole("heading", { name: "Orders", exact: true })).toBeVisible();
    await openScreen(page, "Plans");
    await expect(page.getByRole("heading", { name: "Plans Catalogue" })).toBeVisible();
  });

  test("navigation: an admin opens the Staff and Accounts screens", async ({ page }) => {
    await openScreen(page, "Staff");
    await expect(page.getByText("Staff Management")).toBeVisible();
    await openScreen(page, "Accounts");
    await expect(page.getByRole("button", { name: "Create account" })).toBeVisible();
  });

  test("edge: a reload keeps the admin signed in and returns to the Overview, since screens are not in the URL", async ({ page }) => {
    await openScreen(page, "Tenants");
    await page.reload();
    await expect(page.getByText("Platform Overview").first()).toBeVisible();
  });
});

test.describe("tenants", () => {
  test("flow 26: search narrows the tenant list, shows an empty state, and clearing restores it", async ({ page }) => {
    await openScreen(page, "Tenants");
    const search = page.getByPlaceholder("Search by name or slug...");
    await expect(page.getByRole("cell", { name: alpha.name })).toBeVisible();

    await search.fill(beta.name);
    await expect(page.getByRole("cell", { name: beta.name })).toBeVisible();
    await expect(page.getByRole("cell", { name: alpha.name })).toHaveCount(0);

    await search.fill(alpha.slug);
    await expect(page.getByRole("cell", { name: alpha.name })).toBeVisible();

    await search.fill("no-such-workspace-zzz");
    await expect(page.getByText("No tenants found")).toBeVisible();

    await search.fill("");
    await expect(page.getByRole("cell", { name: alpha.name })).toBeVisible();
  });

  test("flow 26: a failed tenant list shows the error state, not a blank screen", async ({ page }) => {
    await page.route(
      (url) => new URL(url).pathname === "/api/platform/tenants",
      (route) => route.abort(),
    );
    await openScreen(page, "Tenants");
    await expect(page.getByText("Failed to load tenants")).toBeVisible();
  });

  test("flow 27: staff changes a workspace status with a reason, and the badge follows", async ({ page }) => {
    await openScreen(page, "Tenants");
    await page.getByPlaceholder("Search by name or slug...").fill(beta.name);
    await page.getByRole("row", { name: new RegExp(beta.name) }).getByRole("button", { name: "View" }).click();
    await expect(page.getByRole("button", { name: "Back to tenants" })).toBeVisible();

    await page.getByRole("button", { name: "Change status" }).click();
    const dialog = page.getByRole("dialog", { name: "Change workspace status" });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByRole("button", { name: "Update status" })).toBeDisabled();

    await dialog.getByLabel("Target status").selectOption("read_only");
    await expect(dialog.getByRole("button", { name: "Update status" })).toBeDisabled();
    await dialog.locator("#confirm-reason-input").fill("   ");
    await expect(dialog.getByRole("button", { name: "Update status" })).toBeDisabled();
    await dialog.locator("#confirm-reason-input").fill("e2e read-only drill");
    await dialog.getByRole("button", { name: "Update status" }).click();

    await expect(dialog).toHaveCount(0);
    await expect(page.getByText(/read.?only/i).first()).toBeVisible();
  });

  test("flow 27: Cancel in the status dialog leaves the workspace status unchanged", async ({ page }) => {
    await openScreen(page, "Tenants");
    await page.getByPlaceholder("Search by name or slug...").fill(alpha.name);
    await page.getByRole("row", { name: new RegExp(alpha.name) }).getByRole("button", { name: "View" }).click();
    await page.getByRole("button", { name: "Change status" }).click();
    const dialog = page.getByRole("dialog", { name: "Change workspace status" });
    await dialog.getByLabel("Target status").selectOption("suspended");
    await dialog.locator("#confirm-reason-input").fill("should not be saved");
    await dialog.getByRole("button", { name: "Cancel" }).click();
    await expect(dialog).toHaveCount(0);
    await expect(page.getByText("should not be saved")).toHaveCount(0);
  });

  test("flow 27: a reason one character over the limit is refused and shown in the dialog", async ({ page }) => {
    await openScreen(page, "Tenants");
    await page.getByPlaceholder("Search by name or slug...").fill(alpha.name);
    await page.getByRole("row", { name: new RegExp(alpha.name) }).getByRole("button", { name: "View" }).click();
    await page.getByRole("button", { name: "Change status" }).click();
    const dialog = page.getByRole("dialog", { name: "Change workspace status" });
    await dialog.getByLabel("Target status").selectOption("read_only");
    await dialog.locator("#confirm-reason-input").fill("x".repeat(1001));
    await dialog.getByRole("button", { name: "Update status" }).click();
    await expect(dialog.getByRole("alert")).toBeVisible();
    await expect(dialog).toBeVisible();
  });

  test("flow 27: the Escape key closes the status dialog without a change", async ({ page }) => {
    await openScreen(page, "Tenants");
    await page.getByPlaceholder("Search by name or slug...").fill(alpha.name);
    await page.getByRole("row", { name: new RegExp(alpha.name) }).getByRole("button", { name: "View" }).click();
    await page.getByRole("button", { name: "Change status" }).click();
    const dialog = page.getByRole("dialog", { name: "Change workspace status" });
    await expect(dialog).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(dialog).toHaveCount(0);
  });
});

test.describe("users", () => {
  test("flow 21: search finds no one, then the target is suspended with a reason and reinstated", async ({ page }) => {
    await openScreen(page, "Users");
    const search = page.getByPlaceholder("Search by email or name...");
    await search.fill("no-such-person-zzz");
    await expect(page.getByText("No users found")).toBeVisible();

    await search.fill(target.email);
    await page.getByRole("row", { name: new RegExp(target.email) }).getByRole("button", { name: "View" }).click();
    await page.getByRole("button", { name: "Suspend account" }).click();

    const dialog = page.getByRole("dialog", { name: "Suspend user account" });
    await expect(dialog.getByRole("button", { name: "Suspend account" })).toBeDisabled();
    await dialog.locator("#confirm-reason-input").fill("e2e suspension");
    await dialog.getByRole("button", { name: "Suspend account" }).click();

    await expect(page.getByRole("button", { name: "Reinstate account" })).toBeVisible();
    await page.getByRole("button", { name: "Reinstate account" }).click();
    const reinstate = page.getByRole("dialog", { name: "Reinstate user account" });
    await reinstate.locator("#confirm-reason-input").fill("e2e reinstate");
    await reinstate.getByRole("button", { name: "Reinstate account" }).click();
    await expect(page.getByRole("button", { name: "Suspend account" })).toBeVisible();
  });

  test("flow 21: the suspension shows in the audit log under the target's email", async ({ page }) => {
    await openScreen(page, "Users");
    await page.getByPlaceholder("Search by email or name...").fill(target.email);
    await page.getByRole("row", { name: new RegExp(target.email) }).getByRole("button", { name: "View" }).click();
    await page.getByRole("button", { name: "Suspend account" }).click();
    const dialog = page.getByRole("dialog", { name: "Suspend user account" });
    await dialog.locator("#confirm-reason-input").fill("e2e audit check");
    await dialog.getByRole("button", { name: "Suspend account" }).click();
    await expect(page.getByRole("button", { name: "Reinstate account" })).toBeVisible();

    await openScreen(page, "Audit Log");
    await expect(page.getByText(target.email).first()).toBeVisible();

    // Put the account back so later tests start from the same state.
    await openScreen(page, "Users");
    await page.getByPlaceholder("Search by email or name...").fill(target.email);
    await page.getByRole("row", { name: new RegExp(target.email) }).getByRole("button", { name: "View" }).click();
    await page.getByRole("button", { name: "Reinstate account" }).click();
    const reinstate = page.getByRole("dialog", { name: "Reinstate user account" });
    await reinstate.locator("#confirm-reason-input").fill("e2e cleanup");
    await reinstate.getByRole("button", { name: "Reinstate account" }).click();
    await expect(page.getByRole("button", { name: "Suspend account" })).toBeVisible();
  });

  test("flow 252: an admin cannot suspend their own account and the dialog says so", async ({ page }) => {
    await openScreen(page, "Users");
    await page.getByPlaceholder("Search by email or name...").fill(seed.email);
    await page.getByRole("row", { name: new RegExp(seed.email) }).getByRole("button", { name: "View" }).click();
    await page.getByRole("button", { name: "Suspend account" }).click();
    const dialog = page.getByRole("dialog", { name: "Suspend user account" });
    await dialog.locator("#confirm-reason-input").fill("e2e self");
    await dialog.getByRole("button", { name: "Suspend account" }).click();
    await expect(dialog.getByRole("alert")).toContainText("own account");
    await expect(page.getByRole("button", { name: "Suspend account" })).toBeVisible();
  });

  test("flow 22: Sign out everywhere asks for a reason before it revokes sessions", async ({ page }) => {
    await openScreen(page, "Users");
    await page.getByPlaceholder("Search by email or name...").fill(target.email);
    await page.getByRole("row", { name: new RegExp(target.email) }).getByRole("button", { name: "View" }).click();
    await page.getByRole("button", { name: "Sign out everywhere" }).click();
    const dialog = page.getByRole("dialog", { name: "Sign out everywhere" });
    await expect(dialog.getByRole("button", { name: "Revoke all sessions" })).toBeDisabled();
    await dialog.getByRole("button", { name: "Cancel" }).click();
    await expect(dialog).toHaveCount(0);
  });
});

test.describe("staff list", () => {
  test("flow 14: an unknown email is refused in the add dialog and a malformed one is stopped by the form", async ({ page }) => {
    await openScreen(page, "Staff");
    await page.getByRole("button", { name: "Add staff" }).click();
    const dialog = page.getByRole("dialog", { name: "Add staff member" });
    await expect(dialog.getByRole("button", { name: "Add staff" })).toBeDisabled();

    await dialog.getByPlaceholder("user@example.com").fill("not-an-email");
    await dialog.getByRole("button", { name: "Add staff" }).click();
    await expect(dialog).toBeVisible();
    await expect(dialog.getByRole("alert")).toHaveCount(0);

    await dialog.getByPlaceholder("user@example.com").fill(`nobody-${Date.now()}@example.test`);
    await dialog.getByRole("button", { name: "Add staff" }).click();
    await expect(dialog.getByRole("alert")).toContainText("no Multica account");
  });

  test("flow 14: an active staff member cannot be added twice", async ({ page }) => {
    await openScreen(page, "Staff");
    await page.getByRole("button", { name: "Add staff" }).click();
    const dialog = page.getByRole("dialog", { name: "Add staff member" });
    await dialog.getByPlaceholder("user@example.com").fill(seed.email);
    await dialog.getByRole("button", { name: "Add staff" }).click();
    await expect(dialog.getByRole("alert")).toContainText("already has an active staff role");
  });

  test("flow 14: an admin adds a billing staff member, sees the row, and removes it after confirming", async ({ page }) => {
    await openScreen(page, "Staff");
    await page.getByRole("button", { name: "Add staff" }).click();
    const add = page.getByRole("dialog", { name: "Add staff member" });
    await add.getByPlaceholder("user@example.com").fill(target.email);
    await add.getByLabel("Staff role").selectOption("billing");
    await add.getByRole("button", { name: "Add staff" }).click();
    await expect(add).toHaveCount(0);

    const row = page.getByRole("row", { name: new RegExp(target.email) });
    await expect(row).toBeVisible();
    await expect(row).toContainText("Billing");

    await row.getByRole("button", { name: `Remove staff member ${target.email}` }).click();
    const remove = page.getByRole("dialog", { name: "Remove staff member" });
    await remove.getByRole("button", { name: "Cancel" }).click();
    await expect(page.getByRole("row", { name: new RegExp(target.email) })).toBeVisible();

    await page.getByRole("row", { name: new RegExp(target.email) }).getByRole("button", { name: `Remove staff member ${target.email}` }).click();
    await page.getByRole("dialog", { name: "Remove staff member" }).getByRole("button", { name: "Remove", exact: true }).click();
    await expect(page.getByRole("row", { name: new RegExp(target.email) })).toHaveCount(0);
  });
});

test.describe("layout", () => {
  // Product failure: the header fix (2f274a142) cut the overflow from 1026px to 31px at 375px, but the page
  // still scrolls sideways on the Tenants screen. Observed: scrollWidth - innerWidth = 31. Kept as fixme.
  test.fixme("edge: the Tenants screen fits a 375 by 667 phone without horizontal scrolling", async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await openScreen(page, "Tenants");
    await expect(page.getByPlaceholder("Search by name or slug...")).toBeVisible();
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
    expect(overflow).toBeLessThanOrEqual(0);
  });
});
